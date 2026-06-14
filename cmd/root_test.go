package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"tai/internal/provider"
)

// fakeProvider implements provider.AIProvider for cmd tests. Inline because
// cmd's tests are the only consumer in this package.
type fakeProvider struct {
	mu     sync.Mutex
	out    string
	err    error
	prompt string
	calls  int
}

func (f *fakeProvider) GenerateCommand(prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.prompt = prompt
	return f.out, f.err
}

// withFlagsReset snapshots and restores the package-level flag vars so tests
// don't leak state into each other.
func withFlagsReset(t *testing.T) {
	t.Helper()
	a, b, c := skipPermission, copyToClipboard, noTUI
	t.Cleanup(func() {
		skipPermission, copyToClipboard, noTUI = a, b, c
	})
	skipPermission, copyToClipboard, noTUI = false, false, false
}

// withInjections snapshots and restores the injection vars used by runRoot.
func withInjections(t *testing.T) {
	t.Helper()
	origProv, origTUI, origSave, origStdin := newProvider, runTUI, saveHistory, stdin
	t.Cleanup(func() {
		newProvider, runTUI, saveHistory, stdin = origProv, origTUI, origSave, origStdin
	})
	// Default to a no-op recorder so tests don't accidentally write to the real
	// history file under $HOME. Individual tests can override to inspect calls.
	saveHistory = func(string, string) error { return nil }
}

func TestClipboardCommand(t *testing.T) {
	tests := []struct {
		goos     string
		wantBin  string
		wantArgs []string
		wantErr  bool
	}{
		{goos: "darwin", wantBin: "pbcopy", wantArgs: []string{"pbcopy"}},
		{goos: "linux", wantBin: "xclip", wantArgs: []string{"xclip", "-selection", "clipboard"}},
		{goos: "windows", wantBin: "clip", wantArgs: []string{"clip"}},
		{goos: "freebsd", wantErr: true},
		{goos: "", wantErr: true},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			cmd, err := clipboardCommand(tt.goos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for goos=%q, got cmd=%v", tt.goos, cmd)
				}
				if !strings.Contains(err.Error(), "unsupported") {
					t.Errorf("error %q should mention 'unsupported'", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd == nil {
				t.Fatal("cmd is nil")
			}
			// cmd.Args[0] is the binary; remaining are flags.
			if filepath.Base(cmd.Args[0]) != tt.wantBin {
				t.Errorf("binary = %q, want %q", cmd.Args[0], tt.wantBin)
			}
			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("Args = %v, want %v", cmd.Args, tt.wantArgs)
			}
			for i, want := range tt.wantArgs {
				// First arg may be resolved to an absolute path by exec.Command.
				if i == 0 {
					if filepath.Base(cmd.Args[i]) != want {
						t.Errorf("Args[0] base = %q, want %q", filepath.Base(cmd.Args[i]), want)
					}
					continue
				}
				if cmd.Args[i] != want {
					t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want)
				}
			}
		})
	}
}

func TestRunShellCommand_SuccessCapturesStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	var out, errBuf bytes.Buffer
	err := runShellCommand("echo hello", &out, &errBuf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr should be empty, got %q", errBuf.String())
	}
}

func TestRunShellCommand_FailureReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	err := runShellCommand("exit 7", io.Discard, io.Discard, nil)
	if err == nil {
		t.Fatal("expected non-nil error for non-zero exit")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if code := exitErr.ExitCode(); code != 7 {
			t.Errorf("exit code = %d, want 7", code)
		}
	} else {
		t.Errorf("error is not *exec.ExitError: %T", err)
	}
}

func TestRunShellCommand_RoutesStdinToCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	var out bytes.Buffer
	err := runShellCommand("cat", &out, io.Discard, strings.NewReader("piped input"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); got != "piped input" {
		t.Errorf("stdout = %q, want %q", got, "piped input")
	}
}

func TestRunShellCommand_CapturesStderrSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	var out, errBuf bytes.Buffer
	err := runShellCommand("echo out; echo err >&2", &out, &errBuf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "out" {
		t.Errorf("stdout = %q, want %q", got, "out")
	}
	if got := strings.TrimSpace(errBuf.String()); got != "err" {
		t.Errorf("stderr = %q, want %q", got, "err")
	}
}

// writeStubBinary creates an executable shell script at dir/name that copies
// stdin to outFile and exits 0. Lets the clipboard end-to-end test verify
// the right text reaches the clipboard tool without touching the real one.
func writeStubBinary(t *testing.T, dir, name, outFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stubs do not work on windows")
	}
	script := "#!/bin/sh\ncat > " + outFile + "\n"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", dir+string(os.PathListSeparator)+orig)
}

func TestCopyCommandToClipboard_PipesTextToPlatformBinary(t *testing.T) {
	// Pick a stub name matching the current platform's clipboard binary so the
	// real copyCommandToClipboard path is exercised end-to-end.
	var stubName string
	switch runtime.GOOS {
	case "darwin":
		stubName = "pbcopy"
	case "linux":
		stubName = "xclip"
	default:
		t.Skipf("no clipboard stub configured for %s", runtime.GOOS)
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "received.txt")
	writeStubBinary(t, dir, stubName, outFile)
	prependPATH(t, dir)

	if err := copyCommandToClipboard("hello clipboard"); err != nil {
		t.Fatalf("copyCommandToClipboard error: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading stub output: %v", err)
	}
	if string(got) != "hello clipboard" {
		t.Errorf("clipboard received %q, want %q", string(got), "hello clipboard")
	}
}

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever
// was written. Needed because executeCommand prints status text through the
// global fmt.* APIs and we want to assert on it without changing the API.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestExecuteCommand_PrintsRunningHeaderOnSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	out := captureStdout(t, func() { executeCommand("true") })
	if !strings.Contains(out, "Running command...") {
		t.Errorf("missing 'Running command...' header in output: %q", out)
	}
	if strings.Contains(out, "exited with error") {
		t.Errorf("success path should not print error: %q", out)
	}
}

func TestExecuteCommand_PrintsErrorOnNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	out := captureStdout(t, func() { executeCommand("exit 3") })
	if !strings.Contains(out, "Command exited with error") {
		t.Errorf("error path should print failure message: %q", out)
	}
}

func TestExecuteCommand_StreamsCommandStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	out := captureStdout(t, func() { executeCommand("echo streamed-output") })
	if !strings.Contains(out, "streamed-output") {
		t.Errorf("inner command stdout was not propagated, got: %q", out)
	}
}

func TestRootCmd_RequiresAtLeastOneArg(t *testing.T) {
	err := rootCmd.Args(rootCmd, []string{})
	if err == nil {
		t.Error("rootCmd should reject zero args")
	}

	err = rootCmd.Args(rootCmd, []string{"do", "a", "thing"})
	if err != nil {
		t.Errorf("rootCmd should accept multi-word args, got: %v", err)
	}
}

func TestReadYesNo(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"y\n", "y"},
		{"Y\n", "y"},
		{"yes\n", "yes"},
		{"YES\n", "yes"},
		{"  y  \n", "y"},
		{"n\n", "n"},
		{"\n", ""},
		{"", ""},
		{"garbage\n", "garbage"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := readYesNo(strings.NewReader(tt.in))
			if got != tt.want {
				t.Errorf("readYesNo(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunRoot_ProviderErrorReturns1(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)
	newProvider = func() provider.AIProvider {
		return &fakeProvider{err: errors.New("claude offline")}
	}

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"list", "files"})
	})

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "claude offline") {
		t.Errorf("output should mention the provider error, got: %q", out)
	}
}

func TestRunRoot_CopyPathExitsAfterClipboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("clipboard stub uses shell script")
	}
	withFlagsReset(t)
	withInjections(t)
	copyToClipboard = true

	fp := &fakeProvider{out: "ls -la"}
	newProvider = func() provider.AIProvider { return fp }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		t.Fatal("TUI must not be invoked in copy path")
		return "", false, nil
	}

	// Stub the platform clipboard binary so we can verify it was called with the
	// suggested command and avoid mutating the real clipboard.
	dir := t.TempDir()
	outFile := filepath.Join(dir, "clip.txt")
	switch runtime.GOOS {
	case "darwin":
		writeStubBinary(t, dir, "pbcopy", outFile)
	case "linux":
		writeStubBinary(t, dir, "xclip", outFile)
	default:
		t.Skipf("no clipboard stub for %s", runtime.GOOS)
	}
	prependPATH(t, dir)

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"list files"})
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "ls -la") {
		t.Errorf("output should echo the suggested command, got: %q", out)
	}
	if !strings.Contains(out, "copied to clipboard") {
		t.Errorf("output should confirm clipboard, got: %q", out)
	}

	clip, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("clipboard stub did not record stdin: %v", err)
	}
	if string(clip) != "ls -la" {
		t.Errorf("clipboard received %q, want %q", string(clip), "ls -la")
	}
}

func TestRunRoot_CopyFailureReturns1(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on windows")
	}
	withFlagsReset(t)
	withInjections(t)
	copyToClipboard = true

	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }

	// Empty PATH guarantees the clipboard tool lookup fails.
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", t.TempDir())

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"x"})
	})

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Failed to copy") {
		t.Errorf("output should mention the copy failure, got: %q", out)
	}
}

func TestRunRoot_YesFlagExecutesImmediately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)
	skipPermission = true

	fp := &fakeProvider{out: "echo hi-from-yes"}
	newProvider = func() provider.AIProvider { return fp }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		t.Fatal("TUI must not be invoked with --yes")
		return "", false, nil
	}

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"say", "hi"})
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "hi-from-yes") {
		t.Errorf("expected command output to appear, got: %q", out)
	}
	if fp.calls != 1 {
		t.Errorf("provider should be called exactly once, got %d", fp.calls)
	}
	if fp.prompt != "say hi" {
		t.Errorf("provider prompt = %q, want %q", fp.prompt, "say hi")
	}
}

func TestRunRoot_NoTUIPath_AcceptsYes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)
	noTUI = true
	stdin = strings.NewReader("y\n")

	newProvider = func() provider.AIProvider { return &fakeProvider{out: "echo no-tui-output"} }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		t.Fatal("TUI must not run when --no-tui is set")
		return "", false, nil
	}

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"x"})
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "no-tui-output") {
		t.Errorf("command output should appear when user answers y, got: %q", out)
	}
}

func TestRunRoot_NoTUIPath_RejectsWithN(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)
	noTUI = true
	stdin = strings.NewReader("n\n")

	// Use a command that writes a sentinel file when executed, so we can
	// detect execution unambiguously (the suggested command text itself is
	// always echoed back in the header).
	sentinel := filepath.Join(t.TempDir(), "ran.flag")
	newProvider = func() provider.AIProvider {
		return &fakeProvider{out: "touch " + sentinel}
	}

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"x"})
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Error("command was executed despite the user answering 'n'")
	}
	if strings.Contains(out, "Running command...") {
		t.Error("executeCommand must not be entered on rejection")
	}
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("output should say Cancelled, got: %q", out)
	}
}

func TestRunRoot_DefaultPathHandsOffToTUI_Accept(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)

	tuiCalls := 0
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "echo from-tui"} }
	runTUI = func(orig, suggested string, _ provider.AIProvider) (string, bool, error) {
		tuiCalls++
		if orig != "tui prompt" {
			t.Errorf("TUI got origPrompt %q, want %q", orig, "tui prompt")
		}
		if suggested != "echo from-tui" {
			t.Errorf("TUI got suggested %q, want %q", suggested, "echo from-tui")
		}
		return "echo revised-tui", true, nil
	}

	var code int
	out := captureStdout(t, func() {
		code = runRoot([]string{"tui", "prompt"})
	})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if tuiCalls != 1 {
		t.Errorf("runTUI invoked %d times, want 1", tuiCalls)
	}
	if !strings.Contains(out, "revised-tui") {
		t.Errorf("revised command output should appear, got: %q", out)
	}
}

func TestRunRoot_DefaultPath_Cancel(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		return "ls", false, nil
	}

	var code int
	out := captureStdout(t, func() { code = runRoot([]string{"x"}) })

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("output should say Cancelled, got: %q", out)
	}
}

func TestRunRoot_DefaultPath_TUIError(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		return "", false, errors.New("terminal closed")
	}

	var code int
	out := captureStdout(t, func() { code = runRoot([]string{"x"}) })

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "TUI error") {
		t.Errorf("output should mention TUI error, got: %q", out)
	}
}

// historyRecorder is a helper that captures every saveHistory invocation so
// tests can assert which branches persist an entry and which don't.
type historyRecorder struct {
	calls []struct{ prompt, command string }
	err   error
}

func (r *historyRecorder) save(prompt, command string) error {
	r.calls = append(r.calls, struct{ prompt, command string }{prompt, command})
	return r.err
}

func TestRunRoot_YesFlagRecordsHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)
	skipPermission = true

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "echo ok"} }

	_ = captureStdout(t, func() { runRoot([]string{"do", "thing"}) })

	if len(rec.calls) != 1 {
		t.Fatalf("saveHistory calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].prompt != "do thing" {
		t.Errorf("recorded prompt = %q, want %q", rec.calls[0].prompt, "do thing")
	}
	if rec.calls[0].command != "echo ok" {
		t.Errorf("recorded command = %q, want %q", rec.calls[0].command, "echo ok")
	}
}

func TestRunRoot_CopyPathRecordsHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("clipboard stub uses shell script")
	}
	withFlagsReset(t)
	withInjections(t)
	copyToClipboard = true

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }

	dir := t.TempDir()
	switch runtime.GOOS {
	case "darwin":
		writeStubBinary(t, dir, "pbcopy", filepath.Join(dir, "clip.txt"))
	case "linux":
		writeStubBinary(t, dir, "xclip", filepath.Join(dir, "clip.txt"))
	default:
		t.Skipf("no clipboard stub for %s", runtime.GOOS)
	}
	prependPATH(t, dir)

	_ = captureStdout(t, func() { runRoot([]string{"x"}) })

	if len(rec.calls) != 1 {
		t.Fatalf("saveHistory calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].command != "ls" {
		t.Errorf("recorded command = %q, want %q", rec.calls[0].command, "ls")
	}
}

func TestRunRoot_NoTUIAcceptRecordsHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)
	noTUI = true
	stdin = strings.NewReader("y\n")

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "echo accepted"} }

	_ = captureStdout(t, func() { runRoot([]string{"x"}) })

	if len(rec.calls) != 1 {
		t.Errorf("saveHistory calls = %d, want 1", len(rec.calls))
	}
}

func TestRunRoot_NoTUIRejectSkipsHistory(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)
	noTUI = true
	stdin = strings.NewReader("n\n")

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }

	_ = captureStdout(t, func() { runRoot([]string{"x"}) })

	if len(rec.calls) != 0 {
		t.Errorf("rejected commands must not be saved, got %d calls", len(rec.calls))
	}
}

func TestRunRoot_TUICancelSkipsHistory(t *testing.T) {
	withFlagsReset(t)
	withInjections(t)

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "ls"} }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		return "ls", false, nil
	}

	_ = captureStdout(t, func() { runRoot([]string{"x"}) })

	if len(rec.calls) != 0 {
		t.Errorf("cancelled TUI must not save history, got %d calls", len(rec.calls))
	}
}

func TestRunRoot_TUIAcceptRecordsRevisedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withFlagsReset(t)
	withInjections(t)

	rec := &historyRecorder{}
	saveHistory = rec.save
	newProvider = func() provider.AIProvider { return &fakeProvider{out: "echo original"} }
	runTUI = func(string, string, provider.AIProvider) (string, bool, error) {
		return "echo revised", true, nil
	}

	_ = captureStdout(t, func() { runRoot([]string{"tui", "prompt"}) })

	if len(rec.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(rec.calls))
	}
	if rec.calls[0].command != "echo revised" {
		t.Errorf("recorded command = %q, want %q (the revised one)", rec.calls[0].command, "echo revised")
	}
	if rec.calls[0].prompt != "tui prompt" {
		t.Errorf("recorded prompt = %q, want %q", rec.calls[0].prompt, "tui prompt")
	}
}

func TestRecordHistory_WarnsOnError(t *testing.T) {
	withInjections(t)
	saveHistory = func(string, string) error { return errors.New("disk full") }

	// Capture stderr to verify the warning surfaces there (and not stdout).
	origErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	recordHistory("p", "c")

	w.Close()
	os.Stderr = origErr
	got := <-done

	if !strings.Contains(got, "history not saved") {
		t.Errorf("stderr should warn about save failure, got %q", got)
	}
	if !strings.Contains(got, "disk full") {
		t.Errorf("stderr should surface the underlying error, got %q", got)
	}
}

func TestRecordHistory_SilentOnSuccess(t *testing.T) {
	withInjections(t)
	called := false
	saveHistory = func(p, c string) error { called = true; return nil }

	origErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	recordHistory("p", "c")

	w.Close()
	os.Stderr = origErr
	got := <-done

	if !called {
		t.Error("saveHistory should be called")
	}
	if got != "" {
		t.Errorf("stderr should be empty on success, got %q", got)
	}
}

func TestRootCmd_FlagsRegistered(t *testing.T) {
	for _, name := range []string{"yes", "copy", "no-tui"} {
		f := rootCmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}

	if f := rootCmd.Flags().ShorthandLookup("y"); f == nil || f.Name != "yes" {
		t.Errorf("expected -y to map to --yes, got %v", f)
	}
	if f := rootCmd.Flags().ShorthandLookup("c"); f == nil || f.Name != "copy" {
		t.Errorf("expected -c to map to --copy, got %v", f)
	}
}
