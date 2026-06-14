package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewClaudeCLIProvider_ReturnsNonNil(t *testing.T) {
	p := NewClaudeCLIProvider()
	if p == nil {
		t.Fatal("NewClaudeCLIProvider returned nil")
	}
}

// TestClaudeCLIProvider_SatisfiesAIProvider is a compile-time + runtime check
// that the type still implements the interface contract the cmd/ and tui/
// layers rely on.
func TestClaudeCLIProvider_SatisfiesAIProvider(t *testing.T) {
	var _ AIProvider = (*ClaudeCLIProvider)(nil)
	var _ AIProvider = NewClaudeCLIProvider()
}

// withPATH replaces $PATH with exactly the given value, restoring the
// original on test end.
func withPATH(t *testing.T, value string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", value)
}

// prependToPATH adds dir to the front of $PATH so a stub binary placed there
// shadows any real one further down the path, while leaving system utilities
// like `cat` and `sh` reachable inside the stub script.
func prependToPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", dir+string(os.PathListSeparator)+orig)
}

func TestClaudeCLIProvider_ErrorsWhenBinaryMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH stubbing differs on windows")
	}
	// Empty PATH guarantees `claude` lookup fails.
	withPATH(t, t.TempDir())

	p := NewClaudeCLIProvider()
	out, err := p.GenerateCommand("anything")
	if err == nil {
		t.Fatalf("expected error when claude binary is missing, got output %q", out)
	}
}

// writeStubClaude creates an executable shell script at dir/claude that prints
// the requested stdout, optionally to stderr, and exits with exitCode. It is
// used to simulate the real claude CLI without a network round-trip.
func writeStubClaude(t *testing.T, dir, stdout, stderr string, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stub does not work on windows")
	}
	script := "#!/bin/sh\n"
	if stdout != "" {
		script += "cat <<'EOF_TAI_STUB'\n" + stdout + "\nEOF_TAI_STUB\n"
	}
	if stderr != "" {
		script += "cat >&2 <<'EOF_TAI_STUB_ERR'\n" + stderr + "\nEOF_TAI_STUB_ERR\n"
	}
	script += "exit " + itoa(exitCode) + "\n"
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func TestClaudeCLIProvider_SuccessReturnsSanitizedCommand(t *testing.T) {
	dir := t.TempDir()
	writeStubClaude(t, dir, "```bash\nls -la\n```", "", 0)
	prependToPATH(t, dir)

	p := NewClaudeCLIProvider()
	got, err := p.GenerateCommand("list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}

func TestClaudeCLIProvider_PropagatesSanitizeError(t *testing.T) {
	dir := t.TempDir()
	// Multi-line response — sanitize must reject this so a prose preamble is
	// never piped into `bash -c`.
	writeStubClaude(t, dir, "I would suggest:\nls -la", "", 0)
	prependToPATH(t, dir)

	p := NewClaudeCLIProvider()
	out, err := p.GenerateCommand("list files")
	if err == nil {
		t.Fatalf("expected sanitize error on multi-line response, got %q", out)
	}
	if !strings.Contains(err.Error(), "multi-line") {
		t.Fatalf("error does not look like a sanitize error: %v", err)
	}
}

func TestClaudeCLIProvider_NonZeroExitIsReportedAsError(t *testing.T) {
	dir := t.TempDir()
	writeStubClaude(t, dir, "", "boom", 1)
	prependToPATH(t, dir)

	p := NewClaudeCLIProvider()
	out, err := p.GenerateCommand("anything")
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got %q", out)
	}
}

func TestClaudeCLIProvider_EmptyStdoutIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeStubClaude(t, dir, "", "", 0)
	prependToPATH(t, dir)

	p := NewClaudeCLIProvider()
	out, err := p.GenerateCommand("anything")
	if err == nil {
		t.Fatalf("expected error on empty stdout, got %q", out)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error does not mention empty response: %v", err)
	}
}
