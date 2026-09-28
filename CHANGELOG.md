# Changelog

## Unreleased

### Changed

- **Audit with Claude Code** audits one project at a time, in the project's directory, and suggests moving memory to a better home: the repository's docs or code, issues, pull requests, `CLAUDE.md` or a skill.
- Audit replies approve or comment on a suggestion; accept and reject are gone.
- The audit session updates the web UI with `audit update`, which merges suggestions by ID, in place of `audit post`.

### Removed

- `setup`: the audit session runs in the project's directory, which is already trusted.
- `audit rm`: the audit session deletes memory with `rm`.

- `check` no longer reports `missing_path`; path detection in code spans was too noisy to be useful.

## v0.0.1 - 2026-09-28

### Added

- `serve` browses memory across projects in the browser, styled after Claude.
- `check` lists audit candidates, as text or JSON.
- `serve --addr unix:///path.sock` listens on a Unix socket.
- **Audit with Claude Code** in the web UI starts a background `claude --bg` session that audits memory with you: its suggestions show in the web UI, where you approve or comment on them and reply.
- `setup` prepares the workspace the audit session runs in.
