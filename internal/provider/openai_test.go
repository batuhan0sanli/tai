package provider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tai/internal/config"
)

func TestNewOpenAIProvider_DefaultBaseURL(t *testing.T) {
	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI})
	if p.baseURL != "https://api.openai.com/v1" {
		t.Errorf("default base url = %q", p.baseURL)
	}
}

func TestOpenAIProvider_Success(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		gotBody = string(buf)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ls -la"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, APIKey: "sk-test", Model: "gpt-4o-mini", BaseURL: srv.URL})
	got, err := p.GenerateCommand("list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"gpt-4o-mini"`) {
		t.Errorf("request body missing model: %s", gotBody)
	}
}

func TestOpenAIProvider_NoKeyOmitsAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"pwd"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIProvider_SanitizeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"first line\nsecond line"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	if out, err := p.GenerateCommand("x"); err == nil {
		t.Fatalf("expected sanitize error, got %q", out)
	}
}

func TestOpenAIProvider_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	_, err := p.GenerateCommand("x")
	if err == nil || !strings.Contains(err.Error(), "openai") {
		t.Fatalf("expected openai status error, got %v", err)
	}
}

func TestOpenAIProvider_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestOpenAIProvider_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	_, err := p.GenerateCommand("x")
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("expected empty-choices error, got %v", err)
	}
}

func TestOpenAIProvider_NewRequestError(t *testing.T) {
	// A control char in the URL makes http.NewRequest fail before any I/O.
	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: "http://bad\x00url"})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected NewRequest error for malformed URL")
	}
}

func TestOpenAIProvider_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // close so the connection is refused

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: url})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected transport error against a closed server")
	}
}
