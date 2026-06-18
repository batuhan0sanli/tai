package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tai/internal/config"
)

// OpenAIProvider talks to any OpenAI-compatible /chat/completions endpoint.
// This covers the OpenAI API itself, Ollama, and other local OpenAI-compatible
// servers — only the BaseURL (and optionally the API key) differ.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewOpenAIProvider(pc config.ProviderConfig) *OpenAIProvider {
	base := pc.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey:  pc.APIKey,
		model:   pc.Model,
		baseURL: strings.TrimRight(base, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func (p *OpenAIProvider) GenerateCommand(prompt string) (string, error) {
	body, _ := json.Marshal(openAIRequest{
		Model: p.model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
	})

	req, err := http.NewRequest(http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var parsed openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai: empty response (no choices)")
	}

	return SanitizeCommand(parsed.Choices[0].Message.Content)
}
