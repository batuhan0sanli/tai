# tai

![GitHub Release](https://img.shields.io/github/v/release/batuhan0sanli/tai?color=blue)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/batuhan0sanli/tai/release.yml?label=Release)
![License](https://img.shields.io/github/license/batuhan0sanli/tai)

**Terminal AI — turn natural-language requests into shell commands, right from your terminal.**

`tai` ("Terminal AI") is a tiny CLI that asks Claude to translate a plain-English request into the exact shell command
you would have typed, shows it to you, and then runs it, copies it, or lets you refine it through an interactive TUI.

```bash
$ tai "show me the 10 largest files under this directory"
🤖 tai is thinking...

👉 Suggested command:
du -ah . | sort -rh | head -n 10

Do you want to run this command? [y/N]:
```

## Why tai?

- **No context switch.** Skip the trip to a browser tab — the answer arrives where the command will run.
- **You stay in control.** Every command is shown before execution; nothing runs without your say-so (unless you pass `-y`).
- **Refine instead of retry.** The default TUI lets you tweak the command in-place ("make it case-insensitive", "limit to .go files") and re-asks Claude with the additional constraint.
- **History as a launchpad.** `tai history` opens a fuzzy-searchable list of your past prompts → commands; pick one to copy or run again.
- **Safe by construction.** Multi-line model output is rejected before it touches a shell, so a chatty model response can't smuggle in extra commands.

## Installation

### macOS

**Using Homebrew (Recommended):**

```bash
brew tap batuhan0sanli/tap
brew install tai
```

**Manual install:**

1. Download `tai_Darwin_x86_64.tar.gz` (Intel) or `tai_Darwin_arm64.tar.gz` (Apple Silicon) from [Releases](https://github.com/batuhan0sanli/tai/releases).
2. Extract and move into your `PATH`:

   ```bash
   tar -xvf tai_Darwin_arm64.tar.gz
   sudo mv tai /usr/local/bin/
   ```

### Windows

**Using Scoop (Recommended):**

```powershell
scoop bucket add tai https://github.com/batuhan0sanli/scoop-bucket
scoop install tai
```

**Manual install:**

1. Download `tai_Windows_x86_64.zip` from [Releases](https://github.com/batuhan0sanli/tai/releases).
2. Extract and run `tai.exe`. Add the folder to your `PATH` if you want it available everywhere.

> [!NOTE]
> The execute path (default and `-y`) shells out via `bash -c`, so on Windows you'll need Git Bash / WSL for command execution. `tai -c` (copy-to-clipboard) works in any shell.

### Linux

**Using Homebrew (Linuxbrew):**

```bash
brew tap batuhan0sanli/tap
brew install tai
```

**Using DEB / RPM packages:** download the appropriate file from Releases and run:

- **Debian / Ubuntu:** `sudo dpkg -i tai_amd64.deb`
- **Fedora / RHEL:** `sudo rpm -i tai_amd64.rpm`

**Manual install:**

```bash
tar -xvf tai_Linux_x86_64.tar.gz
sudo mv tai /usr/local/bin/
```

### From source (all platforms)

**Prerequisites:** [Go](https://go.dev/dl/) 1.26 or newer.

```bash
git clone https://github.com/batuhan0sanli/tai.git
cd tai
go build -o tai .
sudo mv tai /usr/local/bin/   # optional
```

### Runtime dependency

`tai` shells out to the [`claude` CLI](https://docs.claude.com/en/docs/claude-code/overview) to generate commands, so `claude` must be on your `PATH` and authenticated.

## Usage

```
tai "[request]" [-y|--yes] [-c|--copy] [--no-tui]
tai history     [-y|--yes]
tai --version
```

### Generate and review a command (default)

```bash
tai "find all PDFs modified in the last 7 days"
```

The Bubble Tea TUI shows the suggested command. From there you can:

- Press **Enter** to run it.
- Press **Esc** / **Ctrl-C** to cancel.
- Type a refinement (e.g. `"only under ~/Documents"`) and press **Enter** to ask Claude to revise the command.

### Run without confirmation

```bash
tai -y "create a tarball of the current dir excluding node_modules"
```

### Copy to clipboard instead of running

```bash
tai -c "show docker images that haven't been used in 30 days"
```

The command is piped into `pbcopy` (macOS), `xclip` (Linux), or `clip` (Windows).

### Plain `y/N` prompt (no TUI)

```bash
tai --no-tui "list open ports"
```

Useful in terminals where the TUI doesn't render well — CI logs, limited TTYs, embedded shells. `-y` and `-c` still take precedence.

### Browse and re-use past commands

```bash
tai history          # opens a fuzzy-searchable list; Enter copies to clipboard
tai history -y       # … or runs the selected command immediately
```

History is stored in `~/.config/tai/history.json` (most recent first, capped at 500 entries). Only commands that were actually executed or copied are saved — cancelled or rejected ones are not.

### Check the installed version

```bash
tai --version
# tai version 0.1.0
# commit:  abc1234
# built:   2026-06-14T12:00:00Z
```

### Flags

#### Root command

| Flag | Description |
| :--- | :--- |
| `-y`, `--yes` | Run the suggested command immediately, skipping the confirmation. |
| `-c`, `--copy` | Copy the suggested command to the clipboard instead of running it. |
| `--no-tui` | Use a plain `y/N` prompt instead of the Bubble Tea TUI. |
| `-v`, `--version` | Print version, commit, and build date and exit. |

#### `tai history`

| Flag | Description |
| :--- | :--- |
| `-y`, `--yes` | Execute the selected entry instead of copying it to the clipboard. |

## How it works

1. The default provider (`internal/provider/claude.go`) shells out to `claude -p "<system prompt> <user request>"`.
2. The system prompt constrains the model to emit **only** the raw command — no markdown, no prose.
3. `SanitizeCommand` (`internal/provider/sanitize.go`) strips any leftover code fences and **rejects multi-line responses**, so a chatty model reply can't smuggle in extra commands when running under `-y`.
4. The Bubble Tea TUI (`internal/tui/`) handles the confirmation / revision loop.
5. On accept, the command is executed via `bash -c`; on copy, it's piped to the platform clipboard tool.
6. Executed and copied commands are appended to `~/.config/tai/history.json` for `tai history`.

The `provider.AIProvider` interface is the extension point — implementing it would let `tai` target the direct Anthropic API, OpenAI, or a local model.

## Development

### Building and running locally

```bash
go run . "list files in the current dir"   # run without building
go build -o tai .                            # produce ./tai
```

### Tests

```bash
go test ./... -race -cover                   # full suite, mandatory before every commit
go vet ./...
gofmt -w .
```

Coverage targets (see [`CLAUDE.md`](CLAUDE.md) for the full breakdown):

| Package | Target |
| :--- | :--- |
| `internal/provider` | 100% |
| `internal/history` | ≥ 88% |
| `internal/tui` | ≥ 87% |
| `cmd` | ≥ 90% |

### Cutting a release (the `/release` skill)

This repository ships a Claude Code skill at [`.claude/skills/release/SKILL.md`](.claude/skills/release/SKILL.md) that automates the version-bump workflow end-to-end. From inside Claude Code:

```
/release v0.2.0
```

The skill will:

1. Verify the working tree is clean and you're on `main`, then pull the latest changes.
2. Run `go test ./... -race -cover` and abort if anything fails.
3. List every commit since the previous tag and group them into Keep-a-Changelog sections (Added / Changed / Fixed / Removed) inside `CHANGELOG.md`.
4. Commit the `CHANGELOG.md` update.
5. Create an annotated git tag (`vX.Y.Z`) and push both the commit and the tag to `origin`.

Pushing the tag triggers [`.github/workflows/release.yml`](.github/workflows/release.yml), which runs GoReleaser to build cross-platform binaries, publish a GitHub Release with checksums, and update the Homebrew tap, Scoop bucket, and `.deb` / `.rpm` packages.

If you'd rather do it by hand, the manual equivalent is:

```bash
go test ./... -race -cover            # 1. tests must pass
# … hand-edit CHANGELOG.md …
git add CHANGELOG.md && git commit -m "chore: changelog for v0.2.0"
git tag -a v0.2.0 -m "v0.2.0"
git push origin main --follow-tags
```

### Repository layout

```
.
├── main.go                       # entry point — calls cmd.Execute()
├── cmd/                          # cobra commands (root + history) and dispatch
├── internal/
│   ├── provider/                 # AIProvider interface + ClaudeCLIProvider + SanitizeCommand
│   ├── tui/                      # Bubble Tea confirmation TUI and history browser
│   └── history/                  # JSON-on-disk command history
├── .goreleaser.yaml              # build / packaging / distribution config
├── .github/workflows/release.yml # GoReleaser CI on tag push
├── .claude/skills/release/       # /release Claude Code skill
└── CHANGELOG.md
```

## Contributing

Contributions are welcome. Please open an issue first for non-trivial changes, and make sure `go test ./... -race -cover` passes before submitting a PR.

## License

[MIT](LICENSE) © Batuhan Sanli.
