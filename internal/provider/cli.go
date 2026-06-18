package provider

import (
	"bytes"
	"fmt"
	"os/exec"

	"tai/internal/config"
)

// CLIProvider runs a local CLI binary (Claude Code, OpenAI Codex, Gemini CLI)
// as a subprocess, passing the combined system+user prompt as the final
// argument and sanitising stdout into a single command.
type CLIProvider struct {
	command string
	args    []string
}

// NewCLIProvider builds a CLIProvider from config. The command is required.
func NewCLIProvider(pc config.ProviderConfig) (*CLIProvider, error) {
	if pc.Command == "" {
		return nil, fmt.Errorf("cli provider requires a non-empty command")
	}
	return &CLIProvider{command: pc.Command, args: pc.Args}, nil
}

// NewClaudeCLIProvider is the zero-config default: `claude -p`. Retained so the
// no-config path matches tai's original behaviour.
func NewClaudeCLIProvider() *CLIProvider {
	return &CLIProvider{command: "claude", args: []string{"-p"}}
}

func (c *CLIProvider) GenerateCommand(prompt string) (string, error) {
	// args... then the combined prompt as the final positional argument.
	args := append(append([]string{}, c.args...), cliPrompt(prompt))
	cmd := exec.Command(c.command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return SanitizeCommand(stdout.String())
}
