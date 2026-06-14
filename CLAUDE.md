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
go test ./... -race -cover              # run the full test suite with race + coverage
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
4. **`internal/provider/`** — pluggable AI backends behind the `AIProvider` interface (`GenerateCommand(prompt string) (string, error)`). The only current implementation, `ClaudeCLIProvider`, subprocesses `claude -p <systemInstruction+prompt>` and hands the raw output to `SanitizeCommand`. New providers (e.g. direct Anthropic API, OpenAI, local model) should implement `AIProvider` and be selectable from `cmd/root.go`; the provider is currently hard-wired to `NewClaudeCLIProvider()` and the same instance is reused by the TUI for revisions.

The system prompt that constrains the model to emit only a raw command lives in `internal/provider/claude.go`. If you change provider behavior, keep the "output must be ready to run as-is" contract — `cmd/root.go` feeds the returned string straight into `bash -c` and into clipboards.

**Mandatory for every new provider:** the final step of `GenerateCommand` must be `return SanitizeCommand(rawModelOutput)` (defined in `internal/provider/sanitize.go`). It strips markdown fences and backticks, and rejects multi-line responses — without it, a prose preamble like `I notice your message...\n\nls` gets tokenised by `bash -c` and the prose lines run as commands. This is the only thing standing between a model-side prompt injection and arbitrary shell execution under `-y`.

## Testing

**Mandatory after every change:** run `go test ./... -race -cover` and make it pass before reporting work as done. If the change adds or modifies behaviour, also add or update tests for it in the same edit cycle. Treat a missing test for new code the same as broken code — neither ships.

If tests fail, fix the root cause; don't delete or skip the failing test to make it green. The one exception is when the test itself encoded a buggy expectation — in that case fix the test *and* note in the commit message what the real behaviour is.

Coverage targets (current baseline, do not regress):

- `internal/provider`: **100%** — pure logic, no excuses.
- `internal/tui`: **≥ 89%** — the only carve-out is `tui.Run`, the thin Bubble Tea program launcher that needs a real TTY.
- `cmd`: **≥ 90%** — the carve-out is `Execute()`, a 4-line cobra wrapper.

### How the test suite is structured

- **`internal/provider/sanitize_test.go`** — table-driven tests for every `SanitizeCommand` branch (fences with/without language, backticks, empty/whitespace/newline-only input, multi-line rejection, the same-line ` ```ls -la``` ` case). When you touch the sanitiser, add a row.
- **`internal/provider/claude_test.go`** — the Claude CLI provider is exercised end-to-end via a shell-script stub binary written into `t.TempDir()` and prepended to `$PATH` (`prependToPATH` / `withPATH` helpers). Tests cover the missing-binary, non-zero-exit, empty-stdout, sanitize-rejected, and happy paths.
- **`internal/tui/mock_provider_test.go`** — `mockProvider` is the in-package fake used by every TUI test. Configure `defaultResp` / `defaultErr` or queue per-call responses via `responses`. It is concurrency-safe and tracks call count.
- **`internal/tui/tui_test.go`** — `Model.Update` is tested directly by feeding it `tea.KeyMsg`, `tea.WindowSizeMsg`, `aiResponseMsg`, and `spinner.TickMsg` values, then inspecting the returned state. `View()` is asserted on substrings. `reviseCmd` is invoked directly to verify the combined-prompt format.
- **`cmd/root_test.go`** — `runRoot` is the testable extraction of the cobra `Run` callback; it returns an exit code instead of calling `os.Exit`. Tests cover every branch (`-y`, `-c`, `--no-tui` accept + reject, default TUI accept + cancel + error, provider error, clipboard failure). Injection points for tests: `newProvider`, `runTUI`, and `stdin` package-level vars — always reset with `withInjections(t)` and `withFlagsReset(t)`.

### Writing new tests

- Prefer **table-driven tests** for any function with several input shapes. Sub-test names should describe the *behaviour* under test (`rejects_multi-line_response`), not the input.
- Cover **edge cases**: empty input, whitespace-only input, unicode, multi-line, very long input, error returns from every downstream call.
- For anything that shells out, write a **stub binary** into `t.TempDir()` and use `prependToPATH` so system utilities (`cat`, `sh`) inside the stub still resolve. Never call `os.Setenv("PATH", "")` — you'll break the script interpreter.
- When extending `cmd/`, route new dependencies through an injection var (like `newProvider` / `runTUI` / `stdin`) so the cobra `Run` branch you add stays testable. The pattern: package-level `var dep = realImpl`, override in tests with `withInjections(t)` for safe rollback.
- Refactors that exist purely to enable testing (returning an exit code instead of calling `os.Exit`, extracting a `runShellCommand` from a print-and-run wrapper) are explicitly welcome — do not avoid them.

## Conventions worth knowing

- Module path is the bare name `tai` (see `go.mod`), so internal imports look like `tai/cmd`, `tai/internal/tui`, and `tai/internal/provider` rather than a domain-prefixed path.
- Go version pin is `1.26.4`.
- Direct deps: `spf13/cobra` (CLI), `charmbracelet/bubbletea` + `charmbracelet/bubbles` (textinput, spinner) + `charmbracelet/lipgloss` (TUI). Run `go mod tidy` after touching imports.
