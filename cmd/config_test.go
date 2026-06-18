package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tai/internal/config"
)

// withConfigInjections snapshots and restores the config subcommand injection
// vars (and the --force flag) so tests don't leak state.
func withConfigInjections(t *testing.T) {
	t.Helper()
	origPath, origSave, origForce := configFilePath, configSave, configForce
	t.Cleanup(func() {
		configFilePath, configSave, configForce = origPath, origSave, origForce
	})
	configForce = false
}

func TestRunConfigPath_PrintsPath(t *testing.T) {
	withConfigInjections(t)
	configFilePath = func() (string, error) { return "/tmp/tai/config.json", nil }

	var code int
	out := captureStdout(t, func() { code = runConfigPath() })
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "/tmp/tai/config.json") {
		t.Errorf("output should contain the path, got %q", out)
	}
}

func TestRunConfigPath_Error(t *testing.T) {
	withConfigInjections(t)
	configFilePath = func() (string, error) { return "", errors.New("no home") }

	var code int
	out := captureStdout(t, func() { code = runConfigPath() })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "no home") {
		t.Errorf("output should surface the error, got %q", out)
	}
}

func TestRunConfigInit_WritesTemplate(t *testing.T) {
	withConfigInjections(t)
	path := filepath.Join(t.TempDir(), "config.json")
	configFilePath = func() (string, error) { return path, nil }

	var saved bool
	configSave = func(c config.Config) error {
		saved = true
		if len(c.Providers) == 0 {
			t.Error("template should have providers")
		}
		return nil
	}

	var code int
	out := captureStdout(t, func() { code = runConfigInit() })
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !saved {
		t.Error("configSave was not called")
	}
	if !strings.Contains(out, "wrote config template") {
		t.Errorf("output should confirm write, got %q", out)
	}
}

func TestRunConfigInit_RefusesExisting(t *testing.T) {
	withConfigInjections(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	configFilePath = func() (string, error) { return path, nil }
	configSave = func(config.Config) error {
		t.Fatal("configSave must not be called when file exists without --force")
		return nil
	}

	var code int
	out := captureStdout(t, func() { code = runConfigInit() })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "already exists") {
		t.Errorf("output should warn about existing file, got %q", out)
	}
}

func TestRunConfigInit_ForceOverwrites(t *testing.T) {
	withConfigInjections(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	configFilePath = func() (string, error) { return path, nil }
	configForce = true

	var saved bool
	configSave = func(config.Config) error { saved = true; return nil }

	var code int
	_ = captureStdout(t, func() { code = runConfigInit() })
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !saved {
		t.Error("configSave should be called with --force")
	}
}

func TestRunConfigInit_StatError(t *testing.T) {
	withConfigInjections(t)
	// Put a regular file where a parent directory is expected, so os.Stat on the
	// child path fails with ENOTDIR (not "does not exist").
	dir := t.TempDir()
	notADir := filepath.Join(dir, "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	configFilePath = func() (string, error) { return filepath.Join(notADir, "config.json"), nil }
	configSave = func(config.Config) error {
		t.Fatal("configSave must not be called when stat returns a non-NotExist error")
		return nil
	}

	var code int
	out := captureStdout(t, func() { code = runConfigInit() })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "cannot check config path") {
		t.Errorf("output should surface the stat error, got %q", out)
	}
}

func TestRunConfigInit_FilePathError(t *testing.T) {
	withConfigInjections(t)
	configFilePath = func() (string, error) { return "", errors.New("no home") }

	var code int
	out := captureStdout(t, func() { code = runConfigInit() })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "no home") {
		t.Errorf("output should surface the error, got %q", out)
	}
}

func TestRunConfigInit_SaveError(t *testing.T) {
	withConfigInjections(t)
	path := filepath.Join(t.TempDir(), "config.json")
	configFilePath = func() (string, error) { return path, nil }
	configSave = func(config.Config) error { return errors.New("disk full") }

	var code int
	out := captureStdout(t, func() { code = runConfigInit() })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "failed to write") {
		t.Errorf("output should mention write failure, got %q", out)
	}
}

func TestConfigCmd_Registered(t *testing.T) {
	var found bool
	for _, c := range rootCmd.Commands() {
		if c.Name() == "config" {
			found = true
			sub := map[string]bool{}
			for _, s := range c.Commands() {
				sub[s.Name()] = true
			}
			if !sub["init"] || !sub["path"] {
				t.Errorf("config subcommands missing: %v", sub)
			}
		}
	}
	if !found {
		t.Error("config command not registered on root")
	}
}
