# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`tai` ("Terminal AI") is a Go CLI that turns a natural-language request into a shell command, shows it to the user, and optionally executes or copies it. Invocation shape:

```
tai "[request]" [-y|--yes] [-c|--copy] [--no-tui]
```

- `-y` / `--yes`: skip the confirmation and run immediately.
- `-c` / `--copy`: do not run; pipe the command into the OS clipboard tool (`pbcopy` on darwin, `xclip` on linux, `clip` on windows) and exit.
- `--no-tui`: skip the Bubble Tea TUI and use the plain `fmt.Scanln` y/N prompt instead. Intended for terminals where the TUI doesn't render correctly (limited TTYs, CI logs, some embedded shells). `-y` and `-c` still take precedence over this flag.

## Commands

```bash
go run . "list files in current dir"   # run locally without building
go build -o tai .                       # produce ./tai binary
go test ./...                           # run all tests (none exist yet)
go vet ./...                            # static checks
gofmt -w .                              # format
go mod tidy                             # sync deps after edits
```

Runtime dependency: the default provider shells out to the `claude` CLI, so `claude` must be on PATH for `tai` to function end-to-end. Execution of suggested commands uses `bash -c`, so the run path assumes a POSIX shell (the clipboard path handles Windows; the exec path does not yet — see `executeCommand` in `cmd/root.go`).

## Architecture

Four layers, top-down:

1. **`main.go`** — single-line entry that delegates to `cmd.Execute()`.
2. **`cmd/`** — Cobra command definition (`rootCmd`), flag wiring, the execute/copy dispatch, and the platform-specific clipboard helper. The default (no-flag) path hands off to `internal/tui` for confirmation; `-y` and `-c` short-circuit before the TUI is started.
3. **`internal/tui/`** — Bubble Tea / Bubbles / Lipgloss confirmation UI. `tui.Run(prompt, command, provider)` shows the suggested command, lets the user accept it (Enter on an empty input), revise it via a textinput (which triggers an async `provider.GenerateCommand` call with original prompt + previous command + revision spliced together, with a spinner during the call), or quit (Esc/Ctrl+C). It returns the final command and a `shouldExecute` flag; the actual `bash -c` execution still happens in `cmd/root.go` after the program exits.
4. **`internal/provider/`** — pluggable AI backends behind the `AIProvider` interface (`GenerateCommand(prompt string) (string, error)`). The only current implementation, `ClaudeCLIProvider`, subprocesses `claude -p <systemInstruction+prompt>` and post-processes the output to strip markdown fences and stray backticks. New providers (e.g. direct Anthropic API, OpenAI, local model) should implement `AIProvider` and be selectable from `cmd/root.go`; the provider is currently hard-wired to `NewClaudeCLIProvider()` and the same instance is reused by the TUI for revisions.

The system prompt that constrains the model to emit only a raw command lives in `internal/provider/claude.go`. If you change provider behavior, keep the "output must be ready to run as-is" contract — `cmd/root.go` feeds the returned string straight into `bash -c` and into clipboards.

## Conventions worth knowing

- Module path is the bare name `tai` (see `go.mod`), so internal imports look like `tai/cmd`, `tai/internal/tui`, and `tai/internal/provider` rather than a domain-prefixed path.
- Go version pin is `1.26.4`.
- Direct deps: `spf13/cobra` (CLI), `charmbracelet/bubbletea` + `charmbracelet/bubbles` (textinput, spinner) + `charmbracelet/lipgloss` (TUI). Run `go mod tidy` after touching imports.
