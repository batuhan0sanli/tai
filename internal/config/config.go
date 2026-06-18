// Package config persists tai's provider configuration: which AI backends are
// available, their credentials/models, and which one to use by default.
//
// The file lives at ~/.config/tai/config.json (alongside history.json) and is
// rewritten atomically (write-to-temp + rename) so a partial write never leaves
// a corrupt file. A missing file is treated as "use the built-in default"
// rather than an error, so tai works out of the box with the Claude CLI.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	dirName  = "tai"
	fileName = "config.json"
)

// Provider type discriminators. The factory in internal/provider switches on
// these to decide how to talk to the backend.
const (
	// TypeCLI shells out to a local CLI binary (Claude Code, OpenAI Codex,
	// Gemini CLI). The command + args are taken from the ProviderConfig.
	TypeCLI = "cli"
	// TypeOpenAI talks to an OpenAI-compatible /chat/completions endpoint.
	// This also covers Ollama and other local OpenAI-compatible servers via a
	// custom BaseURL.
	TypeOpenAI = "openai"
	// TypeGemini talks to the Google Gemini generateContent REST endpoint.
	TypeGemini = "gemini"
	// TypeAnthropic talks to the Anthropic Messages API via the official SDK.
	TypeAnthropic = "anthropic"
)

// ProviderConfig describes a single configured backend. Which fields are
// meaningful depends on Type:
//
//   - cli:       Command, Args (Model is informational only)
//   - openai:    Model, BaseURL, APIKey/APIKeyEnv
//   - gemini:    Model, BaseURL (optional), APIKey/APIKeyEnv
//   - anthropic: Model, BaseURL (optional), APIKey/APIKeyEnv
type ProviderConfig struct {
	Type    string   `json:"type"`
	Model   string   `json:"model,omitempty"`
	APIKey  string   `json:"api_key,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// APIKeyEnv names an environment variable to read the key from when APIKey
	// is empty. When both are empty a type-specific default env var is used
	// (e.g. OPENAI_API_KEY) so keys never have to be stored in plaintext.
	APIKeyEnv string `json:"api_key_env,omitempty"`
}

// Config is the whole config file: a set of named providers plus the name of
// the one to use when no --provider flag is given.
type Config struct {
	DefaultProvider string                    `json:"default_provider"`
	Providers       map[string]ProviderConfig `json:"providers"`
}

// configDirFn resolves the directory that holds the config file. It is a
// package-level var so tests can redirect it to t.TempDir() without touching
// the real $HOME. Mirrors internal/history.
var configDirFn = defaultConfigDir

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", dirName), nil
}

// FilePath returns the absolute path to the config file.
func FilePath() (string, error) {
	dir, err := configDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Default is the zero-config fallback: the Claude CLI, matching tai's original
// behaviour before multi-provider support existed.
func Default() Config {
	return Config{
		DefaultProvider: "claude-code",
		Providers: map[string]ProviderConfig{
			"claude-code": {Type: TypeCLI, Command: "claude", Args: []string{"-p"}},
		},
	}
}

// Template is the starter config written by `tai config init`: every supported
// provider stubbed out so the user only has to fill in keys/models and point
// default_provider at the one they want.
func Template() Config {
	return Config{
		DefaultProvider: "claude-code",
		Providers: map[string]ProviderConfig{
			"claude-code": {Type: TypeCLI, Command: "claude", Args: []string{"-p"}},
			"codex":       {Type: TypeCLI, Command: "codex", Args: []string{"exec"}},
			"gemini-cli":  {Type: TypeCLI, Command: "gemini", Args: []string{"-p"}},
			"openai":      {Type: TypeOpenAI, Model: "gpt-4o-mini", BaseURL: "https://api.openai.com/v1", APIKeyEnv: "OPENAI_API_KEY"},
			"gemini":      {Type: TypeGemini, Model: "gemini-2.0-flash", APIKeyEnv: "GEMINI_API_KEY"},
			"anthropic":   {Type: TypeAnthropic, Model: "claude-opus-4-8", APIKeyEnv: "ANTHROPIC_API_KEY"},
			"ollama":      {Type: TypeOpenAI, Model: "llama3.2", BaseURL: "http://localhost:11434/v1"},
		},
	}
}

// Load reads the config file. A missing or empty file returns Default() rather
// than an error so tai keeps working with no config present.
func Load() (Config, error) {
	path, err := FilePath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, err
	}
	if len(data) == 0 {
		return Default(), nil
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if len(cfg.Providers) == 0 {
		return Default(), nil
	}
	return cfg, nil
}

// Save writes cfg to the config file atomically, creating the directory on
// demand.
func Save(cfg Config) error {
	dir, err := configDirFn()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// json.MarshalIndent only errors on cyclic / unsupported types; Config is a
	// flat struct of stdlib types, so the error is unreachable here.
	data, _ := json.MarshalIndent(cfg, "", "  ")

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
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
	return os.Rename(tmpPath, filepath.Join(dir, fileName))
}

// Resolve returns the provider name and its config for the given name, falling
// back to DefaultProvider when name is empty. The API key is resolved from the
// environment when not set inline (APIKeyEnv, or a type-specific default).
func (c Config) Resolve(name string) (string, ProviderConfig, error) {
	if name == "" {
		name = c.DefaultProvider
	}
	if name == "" {
		return "", ProviderConfig{}, errors.New("no provider specified and no default_provider configured")
	}
	pc, ok := c.Providers[name]
	if !ok {
		return "", ProviderConfig{}, fmt.Errorf("provider %q not found in config (run `tai config init` to create a template)", name)
	}
	if pc.APIKey == "" {
		env := pc.APIKeyEnv
		if env == "" {
			env = defaultAPIKeyEnv(pc.Type)
		}
		if env != "" {
			pc.APIKey = os.Getenv(env)
		}
	}
	return name, pc, nil
}

// defaultAPIKeyEnv maps a provider type to its conventional API-key env var,
// used when a provider has no explicit APIKey or APIKeyEnv.
func defaultAPIKeyEnv(t string) string {
	switch t {
	case TypeOpenAI:
		return "OPENAI_API_KEY"
	case TypeGemini:
		return "GEMINI_API_KEY"
	case TypeAnthropic:
		return "ANTHROPIC_API_KEY"
	default:
		return ""
	}
}
