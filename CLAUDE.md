# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`tai` ("Terminal AI") is a Go CLI that turns a natural-language request into a shell command, shows it to the user, and optionally executes or copies it. Invocation shape:

```
tai "[request]" [-y|--yes] [-c|--copy] [--no-tui] [--provider NAME] [-m|--model NAME]
tai history [-y|--yes]
tai config [edit]
tai config init [--force]
tai config path
tai --version
```

- `-y` / `--yes`: skip the confirmation and run immediately.
- `-c` / `--copy`: do not run; pipe the command into the OS clipboard tool (`pbcopy` on darwin, `xclip` on linux, `clip` on windows) and exit.
- `--no-tui`: skip the Bubble Tea TUI and use the plain `fmt.Scanln` y/N prompt instead. Intended for terminals where the TUI doesn't render correctly (limited TTYs, CI logs, some embedded shells). `-y` and `-c` still take precedence over this flag.
- `--provider NAME`: override the config's `default_provider` for this run. `NAME` is a key under `providers` in the config file.
- `-m` / `--model NAME`: override the selected provider's `model` for this run.
- `config` (no subcommand, or `config edit`): interactive Bubble Tea editor (`internal/tui` `ConfigModel`) to pick the default provider and edit each provider's model/key/URL/command; on the Model field of an API provider, `enter` fetches the live model list to pick from (`provider.ModelLister`). Saves only on confirm. `config init` / `config path`: write a starter config (all providers stubbed) / print the config file path. See **Providers & config** below.
- `-v` / `--version`: print version, commit, and build date and exit. The three values are package-level vars in `cmd/root.go` (`version`, `commit`, `date`) defaulting to `dev` / `none` / `unknown`; goreleaser overrides them at link time via `-ldflags "-X tai/cmd.version=... -X tai/cmd.commit=... -X tai/cmd.date=..."`. Cobra's built-in version handling short-circuits before `Args` validation, so no positional prompt is required.
- `history` (alias `h`): open a fuzzy-searchable `bubbles/list` of past prompts → commands stored in `~/.config/tai/history.json`. Pressing Enter on an entry copies the command to the clipboard by default, or runs it directly under `-y`. Every successful run / copy on the root command appends an entry; cancelled and rejected commands are not saved.

## Commands

```bash
go run . "list files in current dir"   # run locally without building
go build -o tai .                       # produce ./tai binary
go test ./... -race -cover              # run the full test suite with race + coverage
go vet ./...                            # static checks
gofmt -w .                              # format
go mod tidy                             # sync deps after edits
```

Runtime dependency: the **default** provider (and the no-config fallback) shells out to the `claude` CLI, so `claude` must be on PATH for the out-of-the-box experience. Other providers have their own requirements (an API key, or the relevant CLI on PATH) — see **Providers & config**. Execution of suggested commands uses `bash -c`, so the run path assumes a POSIX shell (the clipboard path handles Windows; the exec path does not yet — see `executeCommand` in `cmd/root.go`).

## Providers & config

tai supports multiple AI backends, selected via `~/.config/tai/config.json` (resolved through `internal/config`, which mirrors `internal/history`'s atomic-write + `configDirFn` injection pattern). A missing/empty config falls back to `config.Default()` (the `claude` CLI), so tai works with no config present.

Config shape: a `default_provider` name plus a `providers` map of `name → ProviderConfig`. `ProviderConfig.Type` is the discriminator the factory (`provider.New`) switches on:

| `type` | Impl | Covers | Key fields |
|---|---|---|---|
| `cli` | `CLIProvider` | Claude Code (`claude -p`), OpenAI Codex (`codex exec`), Gemini CLI (`gemini -p`) | `command`, `args` |
| `openai` | `OpenAIProvider` (raw `net/http`) | OpenAI API, **Ollama & any OpenAI-compatible local server** (via `base_url`) | `model`, `base_url`, `api_key`/`api_key_env` |
| `gemini` | `GeminiProvider` (raw `net/http`) | Google Gemini `generateContent` | `model`, `base_url`, `api_key`/`api_key_env` |
| `anthropic` | `AnthropicProvider` (`anthropic-sdk-go`) | Anthropic Messages API | `model`, `base_url`, `api_key`/`api_key_env` |

API keys resolve in this order: inline `api_key` → `api_key_env` env var → a type-default env var (`OPENAI_API_KEY` / `GEMINI_API_KEY` / `ANTHROPIC_API_KEY`). `Config.Resolve(name)` returns the resolved `ProviderConfig` (applying `default_provider` when name is empty); `cmd/root.go`'s `defaultNewProvider` calls it, applies the `--model` override, and hands the result to `provider.New`. Both config loading (`loadConfig`) and provider construction (`newProvider`) are injection points in `cmd/root.go` so the cobra path stays testable without a real config or backend.

**Lightweight by design:** OpenAI/Gemini/Ollama use hand-rolled `net/http` calls (those endpoints are stable JSON; the dependency cost of their SDKs isn't justified). Anthropic is the one exception — it uses the official `anthropic-sdk-go` per the Claude API guidance. Tests point each HTTP provider's `base_url` (and the Anthropic SDK's base URL via `option.WithBaseURL`) at an `httptest.Server`, so the whole provider package is covered without network access.

`tai config init` writes `config.Template()` (every provider stubbed) so users can fill in keys and set `default_provider`; it refuses to clobber an existing file without `--force`.

## Architecture

Six layers, top-down:

1. **`main.go`** — single-line entry that delegates to `cmd.Execute()`.
2. **`cmd/`** — Cobra command definitions (`rootCmd`, `historyCmd`, `configCmd`), flag wiring, the execute/copy dispatch, and the platform-specific clipboard helper. The default (no-flag) root path hands off to `internal/tui` for confirmation; `-y` and `-c` short-circuit before the TUI is started. `defaultNewProvider` loads the config, resolves provider+model (with `--provider`/`--model` overrides) and builds the provider. After every executed/copied command, `recordHistory` calls `history.SaveEntry` (best-effort: failures warn on stderr but don't fail the run).
3. **`internal/tui/`** — Bubble Tea / Bubbles / Lipgloss UI layer. `tui.Run(prompt, command, provider)` is the confirmation TUI (textinput + spinner) used by the root command. `tui.RunHistory(entries)` is the history browser built on `bubbles/list`. `tui.RunConfig(cfg)` is the `config` editor (`ConfigModel`): a hand-rolled three-screen TUI — a provider list (cursor nav, `d` sets default, `s` saves), a per-provider detail form of `textinput`s whose fields are tailored by provider type (`fieldsFor`/`valueFor`/`applyField`), and a model picker (`modeModels`) that fetches the provider's model list asynchronously via an injectable `fetch` func (default `defaultFetchModels` → `provider.New` + `provider.ModelLister`) and feeds the result back as a `modelsMsg`. It returns the edited `config.Config` and a save flag. Each `Run*` is a thin launcher around `tea.NewProgram(...).Run()`; the testable logic lives on the underlying `Model.Update` / `View` methods.
4. **`internal/provider/`** — pluggable AI backends behind the `AIProvider` interface (`GenerateCommand(prompt string) (string, error)`). `provider.New(config.ProviderConfig)` (in `factory.go`) maps a config `type` to a concrete impl: `CLIProvider` (`cli.go`), `OpenAIProvider` (`openai.go`), `GeminiProvider` (`gemini.go`), `AnthropicProvider` (`anthropic.go`). `NewClaudeCLIProvider()` is the zero-config default. The provider built from config is reused by the TUI for revisions. The three API providers also implement the optional `ModelLister` interface (`models.go`) — `ListModels(ctx)` enumerates available models (OpenAI/Ollama `GET /models`, Gemini `GET /models` filtered to `generateContent`, Anthropic SDK `Models.List`); CLI providers don't. See **Providers & config** above for the full table.
5. **`internal/config/`** — provider configuration persisted to `~/.config/tai/config.json` (atomic write + rename, `configDirFn` injection point, same pattern as `internal/history`). `Load` returns `Default()` when the file is missing/empty; `Resolve` picks a provider and resolves its API key from the environment; `Template` is the `config init` starter.
6. **`internal/history/`** — persistence layer for prompt → command pairs. `SaveEntry` prepends to a JSON array in `~/.config/tai/history.json` (capped at `MaxEntries = 500`, newest first) using a write-temp + rename so the file is never left half-written. `GetEntries` reads the same file; missing or empty files return an empty slice (not an error). The config directory is resolved via the package-level `configDirFn` injection point so tests can redirect to `t.TempDir()` without touching the real `$HOME`.

The system prompt that constrains the model to emit only a raw command lives in `internal/provider/prompt.go` (`systemPrompt`, shared by every provider; `cliPrompt` combines it with the request for CLI backends). If you change provider behavior, keep the "output must be ready to run as-is" contract — `cmd/root.go` feeds the returned string straight into `bash -c` and into clipboards.

**Mandatory for every new provider:** the final step of `GenerateCommand` must be `return SanitizeCommand(rawModelOutput)` (defined in `internal/provider/sanitize.go`). It strips markdown fences and backticks, and rejects multi-line responses — without it, a prose preamble like `I notice your message...\n\nls` gets tokenised by `bash -c` and the prose lines run as commands. This is the only thing standing between a model-side prompt injection and arbitrary shell execution under `-y`.

## Testing

**Mandatory after every change:** run `go test ./... -race -cover` and make it pass before reporting work as done. If the change adds or modifies behaviour, also add or update tests for it in the same edit cycle. Treat a missing test for new code the same as broken code — neither ships.

If tests fail, fix the root cause; don't delete or skip the failing test to make it green. The one exception is when the test itself encoded a buggy expectation — in that case fix the test *and* note in the commit message what the real behaviour is.

Coverage targets (current baseline, do not regress):

- `internal/provider`: **100%** — pure logic, no excuses. HTTP providers are tested against `httptest.Server` (and the Anthropic SDK via `option.WithBaseURL`); cover every branch (NewRequest error via a malformed `base_url`, transport error via a closed server, non-2xx, decode error, empty response, sanitize-rejected, happy path).
- `internal/config`: **≥ 87%** — same carve-outs as `internal/history` (unreachable `UserHomeDir` / `tmpfile.Write` / `tmpfile.Close` faults). Drive everything through the `configDirFn` injection point.
- `internal/history`: **≥ 88%** — only carve-outs are unreachable error branches (`UserHomeDir` failure, `tmpfile.Write` / `tmpfile.Close` failures that require a kernel-level I/O fault to surface).
- `internal/tui`: **≥ 87%** — the carve-outs are `tui.Run` and `tui.RunHistory`, the thin Bubble Tea program launchers that need a real TTY. Every `Update` / `View` / model-construction branch must stay covered.
- `cmd`: **≥ 90%** — carve-outs are `Execute()` (a 4-line cobra wrapper) and the `Run` fields of `rootCmd` / `historyCmd` / `configCmd` (1-line dispatch to `runRoot` / `runHistory` / `runConfig*`).

### How the test suite is structured

- **`internal/provider/sanitize_test.go`** — table-driven tests for every `SanitizeCommand` branch (fences with/without language, backticks, empty/whitespace/newline-only input, multi-line rejection, the same-line ` ```ls -la``` ` case). When you touch the sanitiser, add a row.
- **`internal/provider/cli_test.go`** — `CLIProvider` is exercised end-to-end via a shell-script stub binary (`writeStub(dir, name, ...)`) written into `t.TempDir()` and prepended to `$PATH` (`prependToPATH` / `withPATH` helpers). Tests cover the missing-binary, non-zero-exit, empty-stdout, sanitize-rejected, happy, and configurable-command/args paths.
- **`internal/provider/{openai,gemini,anthropic}_test.go`** — the HTTP/SDK providers, each against an `httptest.Server` (Anthropic via the SDK pointed at the test server with `option.WithBaseURL`). Note: backtick-fenced content can't live in a backtick raw-string JSON literal, so success-path responses use plain commands (fence-stripping itself is covered in `sanitize_test.go`); the Anthropic test server must set `Content-Type: application/json` or the SDK rejects the body.
- **`internal/provider/factory_test.go`** — table-driven check that `provider.New` maps each `type` to a working impl and errors on empty/unknown types and a `cli` provider with no command.
- **`internal/provider/models_test.go`** — `ListModels` for the three API providers against `httptest.Server` (Anthropic via the SDK): success (sorted, empty IDs dropped, Gemini `generateContent` filter + `models/` strip), non-200, decode error, malformed-URL, and transport-error paths. Set an API key on at least one happy-path test so the auth-header branch is covered.
- **`internal/tui/mock_provider_test.go`** — `mockProvider` is the in-package fake used by every TUI test. Configure `defaultResp` / `defaultErr` or queue per-call responses via `responses`. It is concurrency-safe and tracks call count.
- **`internal/tui/tui_test.go`** — `Model.Update` is tested directly by feeding it `tea.KeyMsg`, `tea.WindowSizeMsg`, `aiResponseMsg`, and `spinner.TickMsg` values, then inspecting the returned state. `View()` is asserted on substrings. `reviseCmd` is invoked directly to verify the combined-prompt format.
- **`internal/tui/config_test.go`** — drives `ConfigModel.Update` with `tea.KeyMsg` values (helpers `cfgStep` / `cfgStepCmd`, key vars `kUp`/`kDown`/`kEnter`/…): list nav + bounds, `d`/`s`/quit, entering detail and building inputs, field-nav wrap, edit-and-commit (esc + enter-on-last), the empty-providers and no-inputs defensive paths. The **model picker** is driven by overriding `m.fetch` with a fake, pressing `enter` on the Model field, then running the returned `tea.Cmd` and feeding its `modelsMsg` back in: select/cancel/error/empty/loading-ignores-nav/ctrl+c, plus that non-Model and CLI fields don't open it. `defaultFetchModels` is tested via the real factory (new-error, not-a-lister, and a `httptest`-backed success). Pure helpers (`fieldsFor`/`valueFor`/`applyField`/`modelOrDash`/`isAPIType`) and accessors are unit-tested directly. `RunConfig` is the TTY-launcher carve-out.
- **`internal/tui/history_test.go`** — same recipe as `tui_test.go` for the `HistoryModel`: drive `Update` with key messages, assert the filter-state guard against Enter, and verify `Selected()` returns the row under the cursor (not always row 0). `list.Filtering` state is entered programmatically via `m.list.SetFilterState(list.Filtering)` so the test doesn't depend on the `/` keybinding.
- **`internal/history/history_test.go`** — every code path is driven through the `configDirFn` injection point pointing at `t.TempDir()`: round-trip save/get, prepend ordering, the `MaxEntries` cap, missing/empty/corrupt file behaviour, atomic-write failure modes (read-only dir, blocker file at the dir path, directory-as-file at `history.json`).
- **`internal/config/config_test.go`** — `Load`/`Save`/`Resolve` driven through the `configDirFn` injection point (`withConfigDir` / `withConfigDirErr`): missing/empty/empty-providers/invalid-JSON load, round-trip, MkdirAll failure, and key-resolution precedence (inline > `api_key_env` > type-default env, via `t.Setenv`).
- **`cmd/root_test.go`** — `runRoot` is the testable extraction of the cobra `Run` callback; it returns an exit code instead of calling `os.Exit`. Tests cover every branch (`-y`, `-c`, `--no-tui` accept + reject, default TUI accept + cancel + error, provider build error, generate error, clipboard failure) and assert that `saveHistory` fires only on accepted paths. `defaultNewProvider` is tested separately via the `loadConfig` injection (load error, resolve error, success, `--model` override). Injection points: `loadConfig`, `newProvider`, `runTUI`, `saveHistory`, `stdin` — always reset with `withInjections(t)` (installs a no-op `saveHistory`) and `withFlagsReset(t)` (also resets `providerFlag` / `modelFlag`). Note `newProvider` is now `func() (provider.AIProvider, error)`.
- **`cmd/config_test.go`** — `runConfigInit` / `runConfigPath` / `runConfigEdit` through the `configFilePath` / `configSave` / `configLoad` / `runConfigTUI` injection points (`withConfigInjections`): init new-file write, refuse-existing, `--force` overwrite, stat error; edit seeds-template-when-missing vs loads-existing, TUI error, save-on-confirm, save error, no-save; path print + error.
- **`cmd/history_test.go`** — drives `runHistory` through the `getHistoryEntries` and `runHistoryTUI` injection points to cover load errors, the empty-history short-circuit, TUI errors, cancellation, the default copy-to-clipboard path (via the same shell-script stub as the root tests), copy failure, and `-y` execution. Use `withHistoryInjections(t)` + `withHistoryFlagsReset(t)`.

### Writing new tests

- Prefer **table-driven tests** for any function with several input shapes. Sub-test names should describe the *behaviour* under test (`rejects_multi-line_response`), not the input.
- Cover **edge cases**: empty input, whitespace-only input, unicode, multi-line, very long input, error returns from every downstream call.
- For anything that shells out, write a **stub binary** into `t.TempDir()` and use `prependToPATH` so system utilities (`cat`, `sh`) inside the stub still resolve. Never call `os.Setenv("PATH", "")` — you'll break the script interpreter.
- When extending `cmd/`, route new dependencies through an injection var (like `newProvider` / `runTUI` / `stdin`) so the cobra `Run` branch you add stays testable. The pattern: package-level `var dep = realImpl`, override in tests with `withInjections(t)` for safe rollback.
- Refactors that exist purely to enable testing (returning an exit code instead of calling `os.Exit`, extracting a `runShellCommand` from a print-and-run wrapper) are explicitly welcome — do not avoid them.

## Releasing

Cross-platform binaries are built by [GoReleaser](https://goreleaser.com) and triggered by pushing a `vX.Y.Z` tag.

- **`.goreleaser.yaml`** — builds Linux/macOS/Windows × amd64/arm64, archives as `tar.gz` (zip on Windows), publishes to a Homebrew tap (`batuhan0sanli/homebrew-tap`), a Scoop bucket (`batuhan0sanli/scoop-bucket`), and Linux `.deb` / `.rpm` packages. Version metadata is injected into `tai/cmd.version` / `.commit` / `.date` via `ldflags`.
- **`.github/workflows/release.yml`** — fires on tag push (`on.push.tags: ['*']`), checks out with full history, sets up Go, and runs `goreleaser release --clean`. Needs a `GH_PAT` repo secret with write access to the `homebrew-tap` and `scoop-bucket` repos (passed as both `GITHUB_TOKEN` and `GH_PAT` so the brew / scoop publishers authenticate correctly).
- **`CHANGELOG.md`** — maintained in [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. The skill below regenerates the new release section from `git log <prev-tag>..HEAD`; the GitHub Release body is auto-generated by GoReleaser from the same range.
- **`.claude/skills/release/SKILL.md`** — the `/release vX.Y.Z` skill. It runs `go test ./... -race -cover`, verifies the working tree / branch / origin sync, drafts the changelog from commits since the previous tag, asks the user to approve the diff, then commits, tags annotated, and pushes both. Don't bypass the skill's preconditions — they exist to prevent shipping with a dirty tree, a behind branch, or failing tests.

## Conventions worth knowing

- Module path is the bare name `tai` (see `go.mod`), so internal imports look like `tai/cmd`, `tai/internal/tui`, and `tai/internal/provider` rather than a domain-prefixed path.
- Go version pin is `1.26.4`.
- Direct deps: `spf13/cobra` (CLI), `charmbracelet/bubbletea` + `charmbracelet/bubbles` (textinput, spinner) + `charmbracelet/lipgloss` (TUI). Run `go mod tidy` after touching imports.
