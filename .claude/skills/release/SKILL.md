---
name: release
description: Cut a new tagged release of tai. Runs the full test suite, drafts a CHANGELOG.md entry from every commit since the previous tag, commits it, creates an annotated git tag, and pushes both so the GoReleaser workflow fires. Invoke as `/release vX.Y.Z` (the leading `v` is required).
---

# /release — cut a new tagged release

You are cutting a new release of `tai`. The user invokes this skill with a target version (`/release v0.2.0`). Follow the steps below **in order** and stop immediately if any step fails — never paper over a failure.

## 0. Parse and validate the requested version

- The argument MUST match `^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$` (semver with a leading `v`). If it doesn't, tell the user the format and stop.
- Read the most recent tag with `git describe --tags --abbrev=0 2>/dev/null` (it's fine if there is none — this is the first release).
- If the previous tag exists, refuse to proceed when the requested version is less than or equal to it. Use a simple lexicographic-after-normalization check (split on `.`, compare numerically). Ask the user to confirm if the major bump is large (e.g. `v1.0.0` → `v3.0.0`).

## 1. Repo preconditions

Run these checks. If any fail, report the exact problem and stop:

```bash
git rev-parse --is-inside-work-tree   # must be a git repo
git symbolic-ref --short HEAD         # must be `main`
git status --porcelain                # must be empty (clean working tree)
git fetch --tags origin
git rev-list --count HEAD..origin/main  # must be 0 (local is up-to-date with origin/main)
```

If the working tree is dirty, do NOT stash or discard changes — ask the user to commit or stash first.

## 2. Run the full test suite

This is non-negotiable. Project policy in [`CLAUDE.md`](../../../CLAUDE.md) is "mandatory after every change". Run:

```bash
go test ./... -race -cover
```

- If anything fails, stop and report the failure verbatim. Do **not** retry, skip, or `-run` a subset to get green.
- If everything passes, capture the per-package coverage line so you can include it in the release notes if useful.

## 3. Gather commits since the previous tag

```bash
PREV_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -n "$PREV_TAG" ]; then
  git log --no-merges --pretty=format:'%h %s' "$PREV_TAG"..HEAD
else
  git log --no-merges --pretty=format:'%h %s'
fi
```

Read the resulting commit list carefully — don't skim. For each commit, also inspect the diff when the subject is ambiguous:

```bash
git show --stat <sha>
git show <sha> -- <file>   # for specific files when needed
```

You're producing **release notes for users**, not a commit dump. So:

- Translate conventional-commit prefixes into Keep-a-Changelog sections:
  - `feat:` / `add:` → **Added**
  - `fix:` → **Fixed**
  - `refactor:` / `perf:` / `change:` (user-visible) → **Changed**
  - `remove:` / `deprecate:` → **Removed** / **Deprecated**
  - `docs:` / `test:` / `chore:` / `ci:` → usually omitted unless user-visible. Roll genuinely user-facing doc changes (new README sections, install instructions) into **Changed**.
- Merge multiple commits that touch the same feature into a single bullet that describes the end state, not the journey.
- Phrase bullets as outcomes ("Add `--no-tui` flag for terminals without TUI support"), not actions taken ("Added a new flag").
- If a commit reverts an earlier one in the same range, drop both — they net to zero.

## 4. Update CHANGELOG.md

The file already exists in [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format with an `[Unreleased]` heading.

- If a `## [Unreleased]` heading exists, merge any bullets under it into the new release's sections (don't duplicate) and then **remove the `[Unreleased]` heading entirely** — this project's CHANGELOG.md does not keep an empty placeholder between releases.
- Add a new `## [X.Y.Z] - YYYY-MM-DD` heading using today's date in ISO format (`date -u +%Y-%m-%d`). Use the version **without** the leading `v` in the heading (Keep-a-Changelog convention) but **with** the `v` in the link reference at the bottom.
- Update the reference-link footer:
  - Add a new `[X.Y.Z]: https://github.com/batuhan0sanli/tai/compare/vPREV...vX.Y.Z` line (if there is no previous tag, use `releases/tag/vX.Y.Z` as the target instead).
  - Do **not** add an `[Unreleased]` link reference — there is no `[Unreleased]` heading to point at.

Show the resulting CHANGELOG.md diff to the user and **ask them to confirm** before committing. The user is the final editor of release notes — never push without their sign-off.

## 5. Commit and tag

Once the user approves the changelog:

```bash
git add CHANGELOG.md
git commit -m "chore(release): vX.Y.Z"
git tag -a vX.Y.Z -m "vX.Y.Z"
```

- Use an **annotated** tag (`-a`), not a lightweight one — goreleaser and GitHub both prefer annotated tags.
- The tag message can just be the version; the release notes live in CHANGELOG.md and the auto-generated GitHub release body.

## 6. Push

```bash
git push origin main
git push origin vX.Y.Z
```

Push the branch first, then the tag — pushing the tag is what triggers `.github/workflows/release.yml`, and the workflow needs the commit it points at to already be on `origin/main`.

After pushing, surface the GoReleaser workflow URL so the user can watch it:

```bash
echo "https://github.com/batuhan0sanli/tai/actions/workflows/release.yml"
```

## 7. Don't do these things

- **Never** skip step 2 (tests). A release where the tests didn't pass is worse than no release.
- **Never** force-push, amend the tag commit, or move an existing tag — if step 6 reveals a mistake, cut a new patch release (`vX.Y.Z+1`) with the fix.
- **Never** push the tag without first showing the user the changelog diff and getting explicit confirmation.
- **Never** use `--no-verify` on the commit; if a hook fails, fix the underlying issue.
- **Never** invent CHANGELOG entries that aren't backed by a commit in the range. If the range is empty (no commits since the previous tag), tell the user and abort — there's nothing to release.
