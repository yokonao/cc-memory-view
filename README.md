# cc-memory-view

Browse and audit [Claude Code](https://claude.com/claude-code)'s auto memory
(`~/.claude/projects/*/memory/`) across projects. Read-only: it never
modifies memory files.

## Install

Download a prebuilt binary from [GitHub Releases](https://github.com/yokonao/cc-memory-view/releases).
Every release ships with a build provenance attestation.

See [docs/install.md](docs/install.md) for other install methods and how to verify a release.

## Browse

```
cc-memory-view serve [--addr ADDR] [--no-open] [--stale-days N]
```

Starts a local web server (a random port on `127.0.0.1` by default), opens it
in the browser, and serves until interrupted. Search, filter by project and
type, and read each memory rendered as Markdown with its audit issues.

`--addr unix:///absolute/path.sock` listens on a Unix socket instead, in the
form [devproxy](https://github.com/yokonao/devproxy) takes for its targets,
so it can be served as `<route>.localhost`. The browser isn't opened then.

## Audit

```
cc-memory-view check [--json] [--stale-days N]
```

Lists audit candidates, found by rules without judging content:

| kind             | meaning                                                          |
| ---------------- | ---------------------------------------------------------------- |
| `orphan_project` | the project's directory no longer exists                         |
| `no_index`       | the memory directory has no `MEMORY.md`                          |
| `index_missing`  | `MEMORY.md` links to a file that doesn't exist                   |
| `unindexed`      | a memory file isn't listed in `MEMORY.md`                        |
| `broken_link`    | a `[[name]]` link matches no memory in the project               |
| `stale`          | not modified for `--stale-days` days (default 90)                |

Deciding what to keep, merge, move or delete is left to Claude Code.

The home page lists the projects with the status of their latest audit.
**Audit** next to a project starts auditing it, and audits of different
projects run in parallel. Each audit starts a `claude --bg` session in the
project's directory with a fixed audit prompt. For each memory, Claude looks for a better home in
the repository's docs and code, issues and pull requests, `CLAUDE.md` or a
skill, and suggests deleting the memory if it's already there or moving it if
not. Claude posts its suggestions to the web UI, where you approve or comment
on each one and reply; the session receives your replies through a `Monitor`
and applies only what you approved. It doesn't write to GitHub: moves to an
issue or pull request are left to you. You can also continue it in a terminal
with `claude attach <id>`. Only same-origin requests to a loopback or
`*.localhost` host can start or reply to it.

The session talks to the web UI through `cc-memory-view audit update` and
`audit watch`. It starts with those commands, `rm` and read-only `gh`
commands allowed, `--permission-mode auto` and the Claude config
directory added, so it runs without permission prompts. `claude --bg`
requires the project directory to be trusted, which it is once you've
accepted the trust prompt there.

The project directory is taken from the `cwd` recorded in its session logs.
`$CLAUDE_CONFIG_DIR` is honored in place of `~/.claude`.

## Development

```sh
go test ./...
golangci-lint run
golangci-lint fmt
```

`air` rebuilds and restarts `serve` on http://127.0.0.1:8080/ whenever a Go
or HTML file changes. Flags after `--` override it, e.g. to avoid a port clash:

```sh
air -- --addr unix:///tmp/cc-memory-view.sock
```
