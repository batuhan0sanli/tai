package provider

import (
	"context"
	"strings"

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
	resp, err := p.client.Messages.New(context.Background(), anthropic.MessageNewParams{
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
