package provider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tai/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNewAnthropicProvider_DefaultModel(t *testing.T) {
	// Empty config: no key, no base url, no model -> default model.
	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic})
	if p.model != anthropic.ModelClaudeOpus4_8 {
		t.Errorf("default model = %q, want %q", p.model, anthropic.ModelClaudeOpus4_8)
	}
}

func TestAnthropicProvider_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"ls -la"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic, APIKey: "k", Model: "claude-opus-4-8", BaseURL: srv.URL})
	got, err := p.GenerateCommand("list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}

func TestAnthropicProvider_NonTextContentYieldsEmpty(t *testing.T) {
	// A response with only a thinking block produces no text, so SanitizeCommand
	// rejects it as empty. Also exercises the non-TextBlock branch of the loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-8","content":[{"type":"thinking","thinking":"hmm","signature":"s"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic, APIKey: "k", BaseURL: srv.URL})
	_, err := p.GenerateCommand("x")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-response error, got %v", err)
	}
}

func TestAnthropicProvider_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic, APIKey: "k", BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected API error on 400 response")
	}
}
