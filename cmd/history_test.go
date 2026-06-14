package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tai/internal/history"
)

func withHistoryFlagsReset(t *testing.T) {
	t.Helper()
	orig := historyYes
	t.Cleanup(func() { historyYes = orig })
	historyYes = false
}

func withHistoryInjections(t *testing.T) {
	t.Helper()
	origGet, origTUI := getHistoryEntries, runHistoryTUI
	t.Cleanup(func() {
		getHistoryEntries, runHistoryTUI = origGet, origTUI
	})
}

func TestRunHistory_LoadError(t *testing.T) {
	withHistoryFlagsReset(t)
	withHistoryInjections(t)
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return nil, errors.New("disk on fire")
	}
	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(out, "disk on fire") {
		t.Errorf("output should surface the load error, got %q", out)
	}
}

func TestRunHistory_EmptyHistorySkipsTUI(t *testing.T) {
	withHistoryFlagsReset(t)
	withHistoryInjections(t)
	getHistoryEntries = func() ([]history.HistoryEntry, error) { return nil, nil }

	tuiCalls := 0
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		tuiCalls++
		return nil, nil
	}

	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if tuiCalls != 0 {
		t.Errorf("TUI should not run when history is empty, got %d calls", tuiCalls)
	}
	if !strings.Contains(out, "No history") {
		t.Errorf("output should mention empty history, got %q", out)
	}
}

func TestRunHistory_TUIError(t *testing.T) {
	withHistoryFlagsReset(t)
	withHistoryInjections(t)
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return []history.HistoryEntry{{Prompt: "p", Command: "c"}}, nil
	}
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		return nil, errors.New("term gone")
	}

	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(out, "term gone") {
		t.Errorf("output should mention TUI error, got %q", out)
	}
}

func TestRunHistory_CancelExitsCleanly(t *testing.T) {
	withHistoryFlagsReset(t)
	withHistoryInjections(t)
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return []history.HistoryEntry{{Prompt: "p", Command: "c"}}, nil
	}
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		return nil, nil
	}

	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("output should say Cancelled, got %q", out)
	}
}

func TestRunHistory_DefaultCopiesToClipboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("clipboard stub uses shell script")
	}
	withHistoryFlagsReset(t)
	withHistoryInjections(t)

	selected := &history.HistoryEntry{Prompt: "list files", Command: "ls -la"}
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return []history.HistoryEntry{*selected}, nil
	}
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		return selected, nil
	}

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
	out := captureStdout(t, func() { code = runHistory() })
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(out, "Copied to clipboard") {
		t.Errorf("expected clipboard confirmation, got %q", out)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("clip stub did not capture text: %v", err)
	}
	if string(got) != "ls -la" {
		t.Errorf("clipboard got %q, want %q", string(got), "ls -la")
	}
}

func TestRunHistory_CopyFailureReturns1(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on windows")
	}
	withHistoryFlagsReset(t)
	withHistoryInjections(t)

	selected := &history.HistoryEntry{Prompt: "p", Command: "ls"}
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return []history.HistoryEntry{*selected}, nil
	}
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		return selected, nil
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", t.TempDir())

	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(out, "Failed to copy") {
		t.Errorf("expected copy failure message, got %q", out)
	}
}

func TestRunHistory_YesFlagExecutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash -c not available on windows")
	}
	withHistoryFlagsReset(t)
	withHistoryInjections(t)
	historyYes = true

	selected := &history.HistoryEntry{Prompt: "p", Command: "echo from-history"}
	getHistoryEntries = func() ([]history.HistoryEntry, error) {
		return []history.HistoryEntry{*selected}, nil
	}
	runHistoryTUI = func([]history.HistoryEntry) (*history.HistoryEntry, error) {
		return selected, nil
	}

	var code int
	out := captureStdout(t, func() { code = runHistory() })
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(out, "from-history") {
		t.Errorf("expected command stdout, got %q", out)
	}
	if !strings.Contains(out, "Running command...") {
		t.Errorf("expected execute path to fire, got %q", out)
	}
}

func TestHistoryCmd_RegisteredAsSubcommand(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == "history" {
			found = true
			break
		}
	}
	if !found {
		t.Error("historyCmd should be registered as a subcommand of rootCmd")
	}
}

func TestHistoryCmd_HasYesFlagAndAlias(t *testing.T) {
	if f := historyCmd.Flags().Lookup("yes"); f == nil {
		t.Error("history --yes flag not registered")
	}
	if f := historyCmd.Flags().ShorthandLookup("y"); f == nil || f.Name != "yes" {
		t.Errorf("history -y should map to --yes, got %v", f)
	}
	hasAlias := false
	for _, a := range historyCmd.Aliases {
		if a == "h" {
			hasAlias = true
			break
		}
	}
	if !hasAlias {
		t.Error("historyCmd should expose 'h' as an alias")
	}
}

func TestHistoryCmd_RejectsArgs(t *testing.T) {
	if err := historyCmd.Args(historyCmd, []string{"unexpected"}); err == nil {
		t.Error("historyCmd should reject positional args")
	}
}
