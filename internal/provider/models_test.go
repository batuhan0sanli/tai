package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tai/internal/config"
)

func TestOpenAIProvider_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		// Out of order + an empty id that must be dropped.
		w.Write([]byte(`{"data":[{"id":"gpt-b"},{"id":"gpt-a"},{"id":""}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "gpt-a,gpt-b" {
		t.Errorf("models = %v, want [gpt-a gpt-b] sorted, empty dropped", got)
	}
}

func TestOpenAIProvider_ListModels_Errors(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("nope"))
		}))
		defer srv.Close()
		p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
		if _, err := p.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "openai") {
			t.Fatalf("expected openai status error, got %v", err)
		}
	})
	t.Run("bad-json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()
		p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected decode error")
		}
	})
	t.Run("new-request", func(t *testing.T) {
		p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: "http://bad\x00url"})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected NewRequest error")
		}
	})
	t.Run("transport", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		p := NewOpenAIProvider(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: url, APIKey: "k"})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected transport error")
		}
	})
}

func TestGeminiProvider_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("x-goog-api-key = %q, want k", r.Header.Get("x-goog-api-key"))
		}
		w.Write([]byte(`{"models":[
			{"name":"models/gemini-2.0-flash","supportedGenerationMethods":["generateContent","countTokens"]},
			{"name":"models/embedding-001","supportedGenerationMethods":["embedContent"]},
			{"name":"models/gemini-1.5-pro","supportedGenerationMethods":["generateContent"]}
		]}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL, APIKey: "k"})
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// embedding-001 filtered out (no generateContent); prefix stripped; sorted.
	if strings.Join(got, ",") != "gemini-1.5-pro,gemini-2.0-flash" {
		t.Errorf("models = %v, want generateContent-only, prefix-stripped, sorted", got)
	}
}

func TestGeminiProvider_ListModels_Errors(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()
		p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
		if _, err := p.ListModels(context.Background()); err == nil || !strings.Contains(err.Error(), "gemini") {
			t.Fatalf("expected gemini status error, got %v", err)
		}
	})
	t.Run("bad-json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()
		p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: srv.URL})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected decode error")
		}
	})
	t.Run("new-request", func(t *testing.T) {
		p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: "http://bad\x00url"})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected NewRequest error")
		}
	})
	t.Run("transport", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		p := NewGeminiProvider(config.ProviderConfig{Type: config.TypeGemini, BaseURL: url})
		if _, err := p.ListModels(context.Background()); err == nil {
			t.Fatal("expected transport error")
		}
	})
}

func TestAnthropicProvider_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"type":"model","id":"claude-z","display_name":"Z","created_at":"2024-01-01T00:00:00Z","max_input_tokens":1,"max_tokens":1,"capabilities":{}},
			{"type":"model","id":"claude-a","display_name":"A","created_at":"2024-01-01T00:00:00Z","max_input_tokens":1,"max_tokens":1,"capabilities":{}}
		],"has_more":false,"first_id":"claude-z","last_id":"claude-a"}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic, APIKey: "k", BaseURL: srv.URL})
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "claude-a,claude-z" {
		t.Errorf("models = %v, want sorted [claude-a claude-z]", got)
	}
}

func TestAnthropicProvider_ListModels_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(config.ProviderConfig{Type: config.TypeAnthropic, APIKey: "k", BaseURL: srv.URL})
	if _, err := p.ListModels(context.Background()); err == nil {
		t.Fatal("expected API error on 400")
	}
}
