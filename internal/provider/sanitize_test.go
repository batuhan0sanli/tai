package provider

import (
	"strings"
	"testing"
)

func TestSanitizeCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
		// errSubstr, when non-empty, must appear in the returned error.
		errSubstr string
	}{
		{
			name:  "simple single line",
			input: "ls -la",
			want:  "ls -la",
		},
		{
			name:  "trims surrounding whitespace",
			input: "   ls -la   ",
			want:  "ls -la",
		},
		{
			name:  "trims surrounding newlines",
			input: "\n\nls -la\n\n",
			want:  "ls -la",
		},
		{
			name:  "strips fenced code block with language",
			input: "```bash\nls -la\n```",
			want:  "ls -la",
		},
		{
			name:  "strips fenced code block without language",
			input: "```\nls -la\n```",
			want:  "ls -la",
		},
		{
			name:  "strips fenced code block with extra surrounding text trimmed first",
			input: "  ```sh\necho hi\n```  ",
			want:  "echo hi",
		},
		{
			name:  "strips inline backticks around the command",
			input: "`ls -la`",
			want:  "ls -la",
		},
		{
			name:  "strips backticks embedded in command (subshells become unquoted)",
			input: "echo `date`",
			want:  "echo date",
		},
		{
			name:  "passes through command with pipes and flags",
			input: "find . -name '*.go' | xargs grep -l TODO",
			want:  "find . -name '*.go' | xargs grep -l TODO",
		},
		{
			name:  "passes through command with double quotes",
			input: `echo "hello world"`,
			want:  `echo "hello world"`,
		},
		{
			name:      "rejects empty input",
			input:     "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "rejects whitespace-only input",
			input:     "   \t  ",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "rejects newline-only input",
			input:     "\n\n\n",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "rejects fenced block containing only blank lines",
			input:     "```\n\n\n```",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "rejects prose preamble followed by command (multi-line)",
			input:     "I notice your message...\n\nls -la",
			wantErr:   true,
			errSubstr: "multi-line",
		},
		{
			name:      "rejects two commands separated by newline",
			input:     "ls -la\necho done",
			wantErr:   true,
			errSubstr: "multi-line",
		},
		{
			name:      "rejects multi-line response even with blank lines between",
			input:     "ls\n\n\necho hi",
			wantErr:   true,
			errSubstr: "multi-line",
		},
		{
			name:  "accepts fenced block that wraps a single line",
			input: "```bash\nfind . -type f -name '*.go'\n```",
			want:  "find . -type f -name '*.go'",
		},
		{
			name:      "rejects fenced block that wraps multiple lines",
			input:     "```bash\nls\ncd /tmp\n```",
			wantErr:   true,
			errSubstr: "multi-line",
		},
		{
			name:  "fenced block with mixed-case language tag",
			input: "```Bash\nuptime\n```",
			want:  "uptime",
		},
		{
			name:  "fenced block with no trailing newline before closing fence",
			input: "```\nuptime```",
			want:  "uptime",
		},
		{
			name:  "single line wrapped in a partial fence is normalised",
			input: "```ls -la```",
			want:  "ls -la",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeCommand(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (output=%q)", got)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSanitizeCommand_NoTrailingWhitespace makes sure the final command is
// trimmed even when fences leave inner padding behind.
func TestSanitizeCommand_NoTrailingWhitespace(t *testing.T) {
	got, err := SanitizeCommand("```\n   ls -la   \n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ls -la" {
		t.Fatalf("got %q, want %q", got, "ls -la")
	}
}
