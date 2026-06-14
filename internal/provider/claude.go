package provider

import (
	"bytes"
	"os/exec"
	"regexp"
	"strings"
)

type ClaudeCLIProvider struct{}

func NewClaudeCLIProvider() *ClaudeCLIProvider {
	return &ClaudeCLIProvider{}
}

func (c *ClaudeCLIProvider) GenerateCommand(prompt string) (string, error) {
	// System prompt instructing Claude to return only the raw command.
	systemInstruction := "You are a terminal command generator. For the request below, output ONLY the raw, executable terminal command. Do not include greetings, explanations, markdown formatting (```), or backticks. The output must be a command ready to run as-is.\n\nRequest: " + prompt

	// Invoke 'claude -p \"...\"' as a subprocess.
	cmd := exec.Command("claude", "-p", systemInstruction)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(stdout.String())

	// Safety: strip markdown fences (```bash ... ```) if Claude insists on returning them.
	re := regexp.MustCompile("(?s)```[a-zA-Z]*\\n?(.*?)\\n?```")
	if match := re.FindStringSubmatch(output); len(match) > 1 {
		output = strings.TrimSpace(match[1])
	}
	output = strings.ReplaceAll(output, "`", "")

	return output, nil
}
