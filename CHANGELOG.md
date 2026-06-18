# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `tai config edit` — an interactive TUI to choose the default provider and edit each provider's model, API key, base URL, and command without hand-editing JSON.

## [0.2.0] - 2026-06-18

### Added
- Multi-provider support, selectable via `~/.config/tai/config.json`: Claude Code (default), OpenAI Codex, and Gemini CLI (`type: cli`); the OpenAI API and any OpenAI-compatible local server such as Ollama (`type: openai`); the Google Gemini API (`type: gemini`); and the Anthropic Messages API via the official SDK (`type: anthropic`).
- `tai config init` / `tai config path` subcommands to scaffold and locate the config file.
- `--provider NAME` and `-m` / `--model NAME` flags to override the configured default provider/model for a single run.
- API keys resolve from an inline `api_key`, a configurable `api_key_env`, or the conventional `OPENAI_API_KEY` / `GEMINI_API_KEY` / `ANTHROPIC_API_KEY` env vars.

### Changed
- A missing config file falls back to the `claude` CLI, so existing behaviour is unchanged with no config present.

### Fixed
- CLI-provider failures now surface the backend command's stderr (auth/setup hints) instead of a bare `exit status 1`.
- Anthropic API requests are bounded by a 60s timeout so a hung request can't block `tai` indefinitely.
- A whitespace-only config file is treated as empty (falls back to the default) instead of failing to parse.

## [0.1.0] - 2026-06-14

First public release.

### Added
- `tai "[request]"` — translate a natural-language request into a shell command via the [`claude` CLI](https://docs.claude.com/en/docs/claude-code/overview) and run it interactively.
- `-y` / `--yes` flag to skip the confirmation prompt and run the suggested command immediately.
- `-c` / `--copy` flag to copy the suggested command to the OS clipboard (`pbcopy` on macOS, `xclip` on Linux, `clip` on Windows) instead of running it.
- `--no-tui` flag that falls back to a plain `y/N` prompt for terminals where the Bubble Tea TUI doesn't render correctly.
- Bubble Tea TUI for reviewing and **iteratively revising** the suggested command — type a refinement and Claude re-asks with the additional constraint.
- `tai history` (alias `h`) subcommand: fuzzy-searchable list of past prompts → commands persisted to `~/.config/tai/history.json` (newest first, capped at 500 entries). Selecting an entry copies it to the clipboard by default, or runs it under `-y`.
- `SanitizeCommand` safety net that strips markdown fences and **rejects multi-line model responses** before execution, so a chatty model reply can't smuggle extra commands into `bash -c`.
- `-v` / `--version` flag prints the binary version, commit hash, and build date.
- GoReleaser configuration (`.goreleaser.yaml`) for cross-platform builds (Linux/macOS/Windows × amd64/arm64), Homebrew tap, Scoop bucket, and Linux `.deb` / `.rpm` packages.
- GitHub Actions workflow (`.github/workflows/release.yml`) that triggers GoReleaser on tag push.
- `/release` Claude skill at `.claude/skills/release/SKILL.md` that runs the test suite, updates `CHANGELOG.md`, tags, and pushes a new version.

[Unreleased]: https://github.com/batuhan0sanli/tai/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/batuhan0sanli/tai/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/batuhan0sanli/tai/releases/tag/v0.1.0
