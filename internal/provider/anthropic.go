package provider

import (
	"context"
	"sort"
	"strings"
	"time"

	"tai/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider talks to the Anthropic Messages API via the official Go
// SDK. Generating a shell command is a simple, low-latency task, so thinking is
// left off and max_tokens kept small.
type AnthropicProvider struct {
	client anthropic.Client
	model  anthropic.Model
}

func NewAnthropicProvider(pc config.ProviderConfig) *AnthropicProvider {
	var opts []option.RequestOption
	if pc.APIKey != "" {
		opts = append(opts, option.WithAPIKey(pc.APIKey))
	}
	if pc.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(pc.BaseURL))
	}

	model := anthropic.Model(pc.Model)
	if model == "" {
		model = anthropic.ModelClaudeOpus4_8
	}

	return &AnthropicProvider{client: anthropic.NewClient(opts...), model: model}
}

func (p *AnthropicProvider) GenerateCommand(prompt string) (string, error) {
	// Bound the request like the HTTP providers' 60s client timeout, so a hung
	// API call can't block tai indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(tb.Text)
		}
	}

	return SanitizeCommand(b.String())
}

// ListModels returns the Anthropic model IDs available to the account.
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]string, error) {
	page, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(page.Data))
	for _, m := range page.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}
