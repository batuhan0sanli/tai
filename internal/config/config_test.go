package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withConfigDir points configDirFn at a fresh temp dir for the duration of the
// test, restoring the original afterwards.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := configDirFn
	configDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDirFn = orig })
	return dir
}

// withConfigDirErr makes configDirFn fail, to exercise error branches.
func withConfigDirErr(t *testing.T) {
	t.Helper()
	orig := configDirFn
	configDirFn = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { configDirFn = orig })
}

func TestFilePath(t *testing.T) {
	dir := withConfigDir(t)
	got, err := FilePath()
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	if want := filepath.Join(dir, fileName); got != want {
		t.Errorf("FilePath = %q, want %q", got, want)
	}
}

func TestFilePathDirError(t *testing.T) {
	withConfigDirErr(t)
	if _, err := FilePath(); err == nil {
		t.Fatal("expected error from FilePath when configDirFn fails")
	}
}

func TestDefaultConfigDir(t *testing.T) {
	dir, err := defaultConfigDir()
	if err != nil {
		t.Fatalf("defaultConfigDir: %v", err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".config", dirName); dir != want {
		t.Errorf("defaultConfigDir = %q, want %q", dir, want)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	withConfigDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProvider != "claude-code" {
		t.Errorf("default provider = %q, want claude-code", cfg.DefaultProvider)
	}
	if _, ok := cfg.Providers["claude-code"]; !ok {
		t.Error("default config missing claude-code provider")
	}
}

func TestLoadEmptyFileReturnsDefault(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, fileName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProvider != "claude-code" {
		t.Errorf("empty file should yield default, got %q", cfg.DefaultProvider)
	}
}

func TestLoadEmptyProvidersReturnsDefault(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"default_provider":"x","providers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProvider != "claude-code" {
		t.Errorf("config with no providers should yield default, got %q", cfg.DefaultProvider)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := withConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error loading invalid JSON")
	}
}

func TestLoadDirError(t *testing.T) {
	withConfigDirErr(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error from Load when configDirFn fails")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withConfigDir(t)
	in := Template()
	in.DefaultProvider = "openai"
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.DefaultProvider != "openai" {
		t.Errorf("default provider = %q, want openai", out.DefaultProvider)
	}
	if len(out.Providers) != len(in.Providers) {
		t.Errorf("got %d providers, want %d", len(out.Providers), len(in.Providers))
	}
	if out.Providers["openai"].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("openai base url not round-tripped: %q", out.Providers["openai"].BaseURL)
	}
}

func TestSaveDirError(t *testing.T) {
	withConfigDirErr(t)
	if err := Save(Default()); err == nil {
		t.Fatal("expected error from Save when configDirFn fails")
	}
}

func TestSaveMkdirError(t *testing.T) {
	dir := withConfigDir(t)
	// Make the config dir path a regular file so MkdirAll fails.
	blocker := filepath.Join(dir, "sub")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := configDirFn
	configDirFn = func() (string, error) { return blocker, nil }
	t.Cleanup(func() { configDirFn = orig })
	if err := Save(Default()); err == nil {
		t.Fatal("expected MkdirAll error when dir path is a file")
	}
}

func TestResolveDefaultProvider(t *testing.T) {
	cfg := Default()
	name, pc, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if name != "claude-code" {
		t.Errorf("name = %q, want claude-code", name)
	}
	if pc.Type != TypeCLI || pc.Command != "claude" {
		t.Errorf("unexpected provider config: %+v", pc)
	}
}

func TestResolveNoDefault(t *testing.T) {
	cfg := Config{Providers: map[string]ProviderConfig{"x": {Type: TypeCLI}}}
	if _, _, err := cfg.Resolve(""); err == nil {
		t.Fatal("expected error when no provider and no default")
	}
}

func TestResolveNotFound(t *testing.T) {
	cfg := Default()
	if _, _, err := cfg.Resolve("nope"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestResolveInlineKeyWins(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "from-env")
	cfg := Config{
		DefaultProvider: "openai",
		Providers:       map[string]ProviderConfig{"openai": {Type: TypeOpenAI, APIKey: "inline"}},
	}
	_, pc, err := cfg.Resolve("openai")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pc.APIKey != "inline" {
		t.Errorf("APIKey = %q, want inline (inline should win over env)", pc.APIKey)
	}
}

func TestResolveExplicitEnv(t *testing.T) {
	t.Setenv("MY_KEY", "secret")
	cfg := Config{
		DefaultProvider: "x",
		Providers:       map[string]ProviderConfig{"x": {Type: TypeOpenAI, APIKeyEnv: "MY_KEY"}},
	}
	_, pc, err := cfg.Resolve("x")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pc.APIKey != "secret" {
		t.Errorf("APIKey = %q, want secret", pc.APIKey)
	}
}

func TestResolveDefaultEnvByType(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "ant-secret")
	cfg := Config{
		DefaultProvider: "a",
		Providers:       map[string]ProviderConfig{"a": {Type: TypeAnthropic}},
	}
	_, pc, err := cfg.Resolve("a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pc.APIKey != "ant-secret" {
		t.Errorf("APIKey = %q, want ant-secret", pc.APIKey)
	}
}

func TestResolveCLINoKey(t *testing.T) {
	cfg := Default()
	_, pc, err := cfg.Resolve("claude-code")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pc.APIKey != "" {
		t.Errorf("cli provider should have no API key, got %q", pc.APIKey)
	}
}

func TestDefaultAPIKeyEnv(t *testing.T) {
	cases := map[string]string{
		TypeOpenAI:    "OPENAI_API_KEY",
		TypeGemini:    "GEMINI_API_KEY",
		TypeAnthropic: "ANTHROPIC_API_KEY",
		TypeCLI:       "",
		"unknown":     "",
	}
	for typ, want := range cases {
		if got := defaultAPIKeyEnv(typ); got != want {
			t.Errorf("defaultAPIKeyEnv(%q) = %q, want %q", typ, got, want)
		}
	}
}

func TestTemplateHasAllProviders(t *testing.T) {
	tpl := Template()
	for _, name := range []string{"claude-code", "codex", "gemini-cli", "openai", "gemini", "anthropic", "ollama"} {
		if _, ok := tpl.Providers[name]; !ok {
			t.Errorf("template missing provider %q", name)
		}
	}
}
