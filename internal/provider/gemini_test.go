package provider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tai/internal/config"
)

func TestNewGeminiProvider_Defaults(t *testing.T) {
	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini})
	if p.baseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Errorf("default base url = %q", p.baseURL)
	}
	if p.model != "gemini-2.0-flash" {
		t.Errorf("default model = %q", p.model)
	}
}

func TestGeminiProvider_Success(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-2.0-flash:generateContent" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotKey = r.Header.Get("x-goog-api-key")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ls -la"}]}}]}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, APIKey: "k", Model: "gemini-2.0-flash", BaseURL: srv.URL})
	got, err := p.GenerateCommand("list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
	if gotKey != "k" {
		t.Errorf("x-goog-api-key = %q, want k", gotKey)
	}
}

func TestGeminiProvider_NoKeyOmitsHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "" {
			t.Errorf("expected no api key header, got %q", r.Header.Get("x-goog-api-key"))
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"pwd"}]}}]}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGeminiProvider_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
	_, err := p.GenerateCommand("x")
	if err == nil || !strings.Contains(err.Error(), "gemini") {
		t.Fatalf("expected gemini status error, got %v", err)
	}
}

func TestGeminiProvider_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGeminiProvider_EmptyCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
	_, err := p.GenerateCommand("x")
	if err == nil || !strings.Contains(err.Error(), "no candidates") {
		t.Fatalf("expected empty-candidates error, got %v", err)
	}
}

func TestGeminiProvider_EmptyParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"candidates":[{"content":{"parts":[]}}]}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected empty-parts error")
	}
}

func TestGeminiProvider_NewRequestError(t *testing.T) {
	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: "http://bad\x00url"})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected NewRequest error for malformed URL")
	}
}

func TestGeminiProvider_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: url})
	if _, err := p.GenerateCommand("x"); err == nil {
		t.Fatal("expected transport error against a closed server")
	}
}
