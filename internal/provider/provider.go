package provider

// AIProvider defines the contract that every AI backend implementation must follow.
type AIProvider interface {
	GenerateCommand(prompt string) (string, error)
}
