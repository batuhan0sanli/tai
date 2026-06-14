package provider

import (
	"fmt"
	"regexp"
	"strings"
)

// fencedCodeBlock matches a triple-backtick fence with an optional language
// tag, requiring a newline between the opening fence and the body. The
// trailing newline before the closing fence is optional.
//
// Requiring the leading newline is what stops a same-line fence like
// "```ls -la```" from being parsed as language="ls", body=" -la". Without it,
// SanitizeCommand silently returned a truncated, dangerous command.
var fencedCodeBlock = regexp.MustCompile("(?s)```[a-zA-Z]*\\n(.*?)\\n?```")

// SanitizeCommand normalises a raw AI response into a single executable shell
// command, or returns an error if the response can't be safely executed. It is
// the last line of defence before the result is piped into `bash -c`, so every
// provider should run its output through this helper.
//
// The contract: exactly one non-empty line, with markdown fences and stray
// backticks stripped. Multi-line responses are rejected outright — otherwise a
// prose preamble like "I notice..." would be tokenised and run by the shell.
func SanitizeCommand(raw string) (string, error) {
	out := strings.TrimSpace(raw)

	if match := fencedCodeBlock.FindStringSubmatch(out); len(match) > 1 {
		out = strings.TrimSpace(match[1])
	}
	out = strings.ReplaceAll(out, "`", "")

	var nonEmpty []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) == 0 {
		return "", fmt.Errorf("AI returned an empty response")
	}
	if len(nonEmpty) > 1 {
		return "", fmt.Errorf("AI returned a multi-line response (%d non-empty lines); refusing to execute for safety", len(nonEmpty))
	}

	return strings.TrimSpace(nonEmpty[0]), nil
}
