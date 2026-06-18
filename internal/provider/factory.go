package provider

import (
	"fmt"

	"tai/internal/config"
)

// New builds the AIProvider described by pc. It is the single place the
// config's provider "type" is mapped to a concrete implementation; cmd/root.go
// calls this after resolving the config and any --provider/--model overrides.
func New(pc config.ProviderConfig) (AIProvider, error) {
	switch pc.Type {
	case config.TypeCLI:
		return NewCLIProvider(pc)
	case config.TypeOpenAI:
		return NewOpenAIProvider(pc), nil
	case config.TypeGemini:
		return NewGeminiProvider(pc), nil
	case config.TypeAnthropic:
		return NewAnthropicProvider(pc), nil
	case "":
		return nil, fmt.Errorf("provider type is empty")
	default:
		return nil, fmt.Errorf("unknown provider type %q", pc.Type)
	}
}
