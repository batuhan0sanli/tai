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

// GeminiProvider talks to the Google Gemini generateContent REST endpoint.
type GeminiProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewGeminiProvider(pc config.ProviderConfig) *GeminiProvider {
	base := pc.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	model := pc.Model
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiProvider{
		apiKey:  pc.APIKey,
		model:   model,
		baseURL: strings.TrimRight(base, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"system_instruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

func (p *GeminiProvider) GenerateCommand(prompt string) (string, error) {
	body, _ := json.Marshal(geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
	})

	url := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, p.model)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// Pass the key via header rather than ?key= so it doesn't leak into URLs/logs.
	if p.apiKey != "" {
		req.Header.Set("x-goog-api-key", p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var parsed geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response (no candidates)")
	}

	return SanitizeCommand(parsed.Candidates[0].Content.Parts[0].Text)
}
