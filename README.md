# cc-memory-view

Browse and audit [Claude Code](https://claude.com/claude-code)'s auto memory
(`~/.claude/projects/*/memory/`) across projects. Read-only: it never
modifies memory files.

## Install

See [docs/install.md](docs/install.md).

## Browse

```
cc-memory-view [serve] [--addr ADDR] [--no-open] [--stale-days N]
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
| `missing_path`   | a path in a code span doesn't exist (relative to the project)    |
| `stale`          | not modified for `--stale-days` days (default 90)                |

Deciding what to keep, merge, promote to `CLAUDE.md` or delete is left to
Claude Code. For example:

```
Run `cc-memory-view check --json` and audit my memory with me.
```

**Audit with Claude Code** in the web UI does this for you: it starts
`claude --bg` with a fixed audit prompt and shows the `claude attach <id>`
command to continue the session. Only same-origin requests to a loopback or
`*.localhost` host can start it.

The session runs in a dedicated workspace,
`$XDG_DATA_HOME/cc-memory-view` (`~/.local/share/cc-memory-view` by default),
which `claude --bg` requires to be trusted. Prepare it once:

```
cc-memory-view setup
```

This creates the directory and starts `claude` there; accept the trust prompt,
then type `/exit`.

The project directory is taken from the `cwd` recorded in its session logs.
`$CLAUDE_CONFIG_DIR` is honored in place of `~/.claude`.

## Development

```sh
go test ./...
golangci-lint run
golangci-lint fmt
```

`air` rebuilds and restarts `serve` on http://127.0.0.1:8080/ whenever a Go
or HTML file changes.
