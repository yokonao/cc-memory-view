# Changelog

## Unreleased

## v0.0.1 - 2026-09-28

### Added

- `serve` browses memory across projects in the browser, styled after Claude.
- `check` lists audit candidates, as text or JSON.
- `serve --addr unix:///path.sock` listens on a Unix socket.
- **Audit with Claude Code** in the web UI starts a background `claude --bg` session that audits memory with you: its suggestions show in the web UI, where you accept, reject or comment on them and reply.
- `setup` prepares the workspace the audit session runs in.
