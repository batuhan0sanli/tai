// Package provider defines the AIProvider contract and shared helpers used by
// every AI backend.
//
// When adding a new provider (OpenAI, direct Anthropic API, local model, …),
// the returned string from GenerateCommand is piped straight into `bash -c`.
// The final step of any new implementation MUST be SanitizeCommand — it strips
// markdown fences/backticks and rejects multi-line responses, which is what
// prevents a prose preamble like "I notice..." from being tokenised by the
// shell and executed.
package provider

// AIProvider defines the contract that every AI backend implementation must follow.
type AIProvider interface {
	GenerateCommand(prompt string) (string, error)
}
