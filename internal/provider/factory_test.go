package provider

import (
	"testing"

	"tai/internal/config"
)

func TestNew(t *testing.T) {
	cases := []struct {
		name    string
		pc      config.ProviderConfig
		wantErr bool
	}{
		{"cli", config.ProviderConfig{Type: config.TypeCLI, Command: "claude", Args: []string{"-p"}}, false},
		{"cli_missing_command", config.ProviderConfig{Type: config.TypeCLI}, true},
		{"openai", config.ProviderConfig{Type: config.TypeOpenAI, Model: "gpt-4o"}, false},
		{"gemini", config.ProviderConfig{Type: config.TypeGemini, Model: "gemini-2.0-flash"}, false},
		{"anthropic", config.ProviderConfig{Type: config.TypeAnthropic, Model: "claude-opus-4-8"}, false},
		{"empty_type", config.ProviderConfig{}, true},
		{"unknown_type", config.ProviderConfig{Type: "mystery"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.pc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %+v", tc.pc)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p == nil {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}
