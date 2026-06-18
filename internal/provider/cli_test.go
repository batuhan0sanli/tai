package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tai/internal/config"
)

// withPATH replaces $PATH with exactly the given value, restoring the original
// on test end.
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

// writeStub creates an executable shell script at dir/name that prints the
// requested stdout, optionally to stderr, and exits with exitCode. Used to
// simulate a CLI backend without a network round-trip.
func writeStub(t *testing.T, dir, name, stdout, stderr string, exitCode int) {
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
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
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

func TestNewClaudeCLIProvider_ReturnsNonNil(t *testing.T) {
	if p := NewClaudeCLIProvider(); p == nil {
		t.Fatal("NewClaudeCLIProvider returned nil")
	}
}

func TestCLIProvider_SatisfiesAIProvider(t *testing.T) {
	var _ AIProvider = (*CLIProvider)(nil)
	var _ AIProvider = NewClaudeCLIProvider()
}

func TestNewCLIProvider_RequiresCommand(t *testing.T) {
	if _, err := NewCLIProvider(config.ProviderConfig{Type: config.TypeCLI}); err == nil {
		t.Fatal("expected error when command is empty")
	}
	p, err := NewCLIProvider(config.ProviderConfig{Type: config.TypeCLI, Command: "codex", Args: []string{"exec"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.command != "codex" || len(p.args) != 1 || p.args[0] != "exec" {
		t.Fatalf("unexpected provider: %+v", p)
	}
}

func TestCLIProvider_ErrorsWhenBinaryMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH stubbing differs on windows")
	}
	withPATH(t, t.TempDir()) // empty PATH guarantees lookup fails
	p := NewClaudeCLIProvider()
	if out, err := p.GenerateCommand("anything"); err == nil {
		t.Fatalf("expected error when binary is missing, got %q", out)
	}
}

func TestCLIProvider_SuccessReturnsSanitizedCommand(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "claude", "```bash\nls -la\n```", "", 0)
	prependToPATH(t, dir)

	got, err := NewClaudeCLIProvider().GenerateCommand("list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}

func TestCLIProvider_UsesConfiguredCommandAndArgs(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "codex", "echo hi", "", 0)
	prependToPATH(t, dir)

	p, err := NewCLIProvider(config.ProviderConfig{Type: config.TypeCLI, Command: "codex", Args: []string{"exec"}})
	if err != nil {
		t.Fatalf("NewCLIProvider: %v", err)
	}
	got, err := p.GenerateCommand("greet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "echo hi" {
		t.Fatalf("got %q, want %q", got, "echo hi")
	}
}

func TestCLIProvider_PropagatesSanitizeError(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "claude", "I would suggest:\nls -la", "", 0)
	prependToPATH(t, dir)

	out, err := NewClaudeCLIProvider().GenerateCommand("list files")
	if err == nil {
		t.Fatalf("expected sanitize error on multi-line response, got %q", out)
	}
	if !strings.Contains(err.Error(), "multi-line") {
		t.Fatalf("error does not look like a sanitize error: %v", err)
	}
}

func TestCLIProvider_NonZeroExitIsReportedAsError(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "claude", "", "boom", 1)
	prependToPATH(t, dir)

	if out, err := NewClaudeCLIProvider().GenerateCommand("anything"); err == nil {
		t.Fatalf("expected error on non-zero exit, got %q", out)
	}
}

func TestCLIProvider_EmptyStdoutIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeStub(t, dir, "claude", "", "", 0)
	prependToPATH(t, dir)

	out, err := NewClaudeCLIProvider().GenerateCommand("anything")
	if err == nil {
		t.Fatalf("expected error on empty stdout, got %q", out)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error does not mention empty response: %v", err)
	}
}
