package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// withTempDir redirects configDirFn at a fresh t.TempDir() so each test gets an
// isolated history file. The original resolver is restored on cleanup.
func withTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return dir, nil }
	return dir
}

func TestSaveEntry_CreatesDirAndPersists(t *testing.T) {
	dir := withTempDir(t)
	if err := SaveEntry("list files", "ls -la"); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	got, err := GetEntries()
	if err != nil {
		t.Fatalf("GetEntries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if got[0].Prompt != "list files" || got[0].Command != "ls -la" {
		t.Errorf("entry = %+v", got[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "history.json")); err != nil {
		t.Errorf("history.json missing: %v", err)
	}
}

func TestSaveEntry_CreatesNestedDir(t *testing.T) {
	// Point configDirFn at a path that doesn't exist yet so MkdirAll is exercised.
	parent := t.TempDir()
	nested := filepath.Join(parent, "deep", "nest", "tai")
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return nested, nil }

	if err := SaveEntry("p", "c"); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, "history.json")); err != nil {
		t.Errorf("history.json not created in nested dir: %v", err)
	}
}

func TestSaveEntry_PrependsMostRecentFirst(t *testing.T) {
	withTempDir(t)
	if err := SaveEntry("old prompt", "old cmd"); err != nil {
		t.Fatal(err)
	}
	if err := SaveEntry("new prompt", "new cmd"); err != nil {
		t.Fatal(err)
	}
	got, err := GetEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2", len(got))
	}
	if got[0].Prompt != "new prompt" {
		t.Errorf("first = %q, want new prompt", got[0].Prompt)
	}
	if got[1].Prompt != "old prompt" {
		t.Errorf("second = %q, want old prompt", got[1].Prompt)
	}
}

func TestSaveEntry_CapsAtMaxEntries(t *testing.T) {
	withTempDir(t)
	for i := 0; i < MaxEntries+10; i++ {
		if err := SaveEntry("p", "c"); err != nil {
			t.Fatalf("SaveEntry %d: %v", i, err)
		}
	}
	got, err := GetEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxEntries {
		t.Errorf("len = %d, want %d", len(got), MaxEntries)
	}
}

func TestSaveEntry_TimestampInUTC(t *testing.T) {
	withTempDir(t)
	before := time.Now().UTC().Add(-2 * time.Second)
	if err := SaveEntry("p", "c"); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(2 * time.Second)
	got, _ := GetEntries()
	if len(got) != 1 {
		t.Fatal("missing entry")
	}
	ts := got[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not in [%v, %v]", ts, before, after)
	}
}

func TestGetEntries_MissingFileReturnsEmpty(t *testing.T) {
	withTempDir(t)
	got, err := GetEntries()
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("entries = %d, want 0", len(got))
	}
}

func TestGetEntries_EmptyFileReturnsEmpty(t *testing.T) {
	dir := withTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "history.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := GetEntries()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("entries = %d, want 0", len(got))
	}
}

func TestGetEntries_CorruptFileReturnsError(t *testing.T) {
	dir := withTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetEntries(); err == nil {
		t.Error("expected error for corrupt file")
	}
}

func TestSaveEntry_RecoversFromCorruptExistingFile(t *testing.T) {
	dir := withTempDir(t)
	// Pre-populate with broken JSON; SaveEntry should refuse to clobber it.
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveEntry("p", "c"); err == nil {
		t.Error("SaveEntry should refuse to overwrite a corrupt history file")
	}
}

func TestSaveEntry_PersistsValidJSON(t *testing.T) {
	dir := withTempDir(t)
	if err := SaveEntry("p", "c"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "[") {
		t.Errorf("expected JSON array, got %q", string(data))
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Errorf("on-disk JSON invalid: %v", err)
	}
}

func TestSaveEntry_ConfigDirError(t *testing.T) {
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return "", errors.New("no home") }
	if err := SaveEntry("p", "c"); err == nil {
		t.Error("expected error when configDirFn fails")
	}
}

func TestGetEntries_ConfigDirError(t *testing.T) {
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return "", errors.New("no home") }
	if _, err := GetEntries(); err == nil {
		t.Error("expected error when configDirFn fails")
	}
}

func TestFilePath_UsesConfigDir(t *testing.T) {
	dir := withTempDir(t)
	got, err := FilePath()
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if want := filepath.Join(dir, "history.json"); got != want {
		t.Errorf("FilePath = %q, want %q", got, want)
	}
}

func TestFilePath_ConfigDirError(t *testing.T) {
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return "", errors.New("boom") }
	if _, err := FilePath(); err == nil {
		t.Error("expected error when configDirFn fails")
	}
}

func TestDefaultConfigDir_RespectsHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("UserHomeDir uses USERPROFILE on windows, not HOME")
	}
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", orig) })
	os.Setenv("HOME", dir)

	got, err := defaultConfigDir()
	if err != nil {
		t.Fatalf("defaultConfigDir: %v", err)
	}
	if want := filepath.Join(dir, ".config", "tai"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveEntry_MkdirAllError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits behave differently on windows")
	}
	parent := t.TempDir()
	// Place a regular file where SaveEntry expects a directory tree to live;
	// MkdirAll will refuse to descend through a file.
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return filepath.Join(blocker, "tai"), nil }

	if err := SaveEntry("p", "c"); err == nil {
		t.Error("expected MkdirAll error when path traverses a file")
	}
}

func TestGetEntries_PathIsDirectoryReturnsError(t *testing.T) {
	dir := withTempDir(t)
	// history.json is itself a directory — ReadFile will fail with a non-ENOENT error.
	if err := os.Mkdir(filepath.Join(dir, "history.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := GetEntries(); err == nil {
		t.Error("expected error when history.json is a directory")
	}
}

func TestSaveEntry_WriteFails_WhenParentMissing(t *testing.T) {
	// Point configDirFn at a non-existent path AND make MkdirAll succeed by
	// not adding a blocker — but then remove the parent right after MkdirAll
	// is supposed to run. Easier: make CreateTemp fail by pointing parent at
	// a path that becomes unwritable. We approximate by making the dir read-only
	// after creating it.
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("read-only dir trick requires non-root POSIX")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	orig := configDirFn
	t.Cleanup(func() { configDirFn = orig })
	configDirFn = func() (string, error) { return dir, nil }

	if err := SaveEntry("p", "c"); err == nil {
		t.Error("expected write failure on read-only dir")
	}
}

func TestSaveEntry_RoundTripPreservesFields(t *testing.T) {
	withTempDir(t)
	if err := SaveEntry("the prompt", "the command --flag value"); err != nil {
		t.Fatal(err)
	}
	got, err := GetEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Prompt != "the prompt" {
		t.Errorf("Prompt = %q", got[0].Prompt)
	}
	if got[0].Command != "the command --flag value" {
		t.Errorf("Command = %q", got[0].Command)
	}
	if got[0].Timestamp.IsZero() {
		t.Errorf("Timestamp should be set")
	}
}
