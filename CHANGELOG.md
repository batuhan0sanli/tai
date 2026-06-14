# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `--version` / `-v` flag prints the binary version, commit hash, and build date.
- GoReleaser configuration (`.goreleaser.yaml`) for cross-platform builds (Linux/macOS/Windows × amd64/arm64), Homebrew tap, Scoop bucket, and Linux `.deb` / `.rpm` packages.
- GitHub Actions workflow (`.github/workflows/release.yml`) that triggers GoReleaser on tag push.
- `/release` Claude skill at `.claude/skills/release/SKILL.md` that runs the test suite, updates `CHANGELOG.md`, tags, and pushes a new version.

[Unreleased]: https://github.com/batuhan0sanli/tai/compare/HEAD...HEAD
