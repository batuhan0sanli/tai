// Package history persists the prompt → command pairs tai has produced so they
// can be browsed and reused later via the `tai history` subcommand.
//
// Entries are stored as a JSON array in ~/.config/tai/history.json, newest
// first, capped at MaxEntries. The file is rewritten atomically on every save
// (write-to-temp + rename) so a partial write never leaves a corrupt file.
package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// MaxEntries caps how many entries are persisted; the oldest are dropped on save.
const MaxEntries = 500

const (
	dirName  = "tai"
	fileName = "history.json"
)

// HistoryEntry is a single saved invocation: the user's natural-language prompt
// and the shell command tai generated for it.
type HistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Prompt    string    `json:"prompt"`
	Command   string    `json:"command"`
}

// configDirFn resolves the directory that holds the history file. It is a
// package-level var so tests can redirect it to t.TempDir() without touching
// the real $HOME.
var configDirFn = defaultConfigDir

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", dirName), nil
}

// FilePath returns the absolute path to the persisted history file.
func FilePath() (string, error) {
	dir, err := configDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// SaveEntry prepends a new entry to the history file, capped at MaxEntries.
// The config directory is created on demand and the rewrite is atomic.
func SaveEntry(prompt, command string) error {
	dir, err := configDirFn()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, fileName)
	entries, err := readEntries(path)
	if err != nil {
		return err
	}

	entry := HistoryEntry{
		Timestamp: time.Now().UTC(),
		Prompt:    prompt,
		Command:   command,
	}
	entries = append([]HistoryEntry{entry}, entries...)
	if len(entries) > MaxEntries {
		entries = entries[:MaxEntries]
	}

	return writeEntries(path, entries)
}

// GetEntries returns the saved entries, most-recent first. A missing or empty
// file is treated as "no history yet" rather than an error.
func GetEntries() ([]HistoryEntry, error) {
	path, err := FilePath()
	if err != nil {
		return nil, err
	}
	return readEntries(path)
}

func readEntries(path string) ([]HistoryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func writeEntries(path string, entries []HistoryEntry) error {
	// json.MarshalIndent only errors on cyclic / unsupported types; HistoryEntry
	// is a flat struct of stdlib types, so we can ignore the error.
	data, _ := json.MarshalIndent(entries, "", "  ")

	tmp, err := os.CreateTemp(filepath.Dir(path), ".history-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
