# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/batuhan0sanli/tai/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/batuhan0sanli/tai/releases/tag/v0.1.0
