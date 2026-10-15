# cc-memory-view

Browse [Claude Code](https://claude.com/claude-code)'s auto memory
(`~/.claude/projects/*/memory/`, or the directory `autoMemoryDirectory`
chooses) across projects. Read-only: it never modifies memory files.

## Install

Download a prebuilt binary from [GitHub Releases](https://github.com/yokonao/cc-memory-view/releases).
Every release ships with a build provenance attestation.

See [docs/install.md](docs/install.md) for other install methods and how to verify a release.

## Browse

```
cc-memory-view serve [--addr ADDR] [--no-open]
```

Starts a local web server (a random port on `127.0.0.1` by default), opens it
in the browser, and serves until interrupted. Search, filter by project and
type, and read each memory rendered as Markdown.

`--addr unix:///absolute/path.sock` listens on a Unix socket instead, in the
form [devproxy](https://github.com/yokonao/devproxy) takes for its targets,
so it can be served as `<route>.localhost`. The browser isn't opened then.

A project is named after the `cwd` recorded in its session logs.
`$CLAUDE_CONFIG_DIR` is honored in place of `~/.claude`.

A project's memory directory is the `autoMemoryDirectory` of the first
settings that set it: managed (`managed-settings.json` and
`managed-settings.d/`), the project's `.claude/settings.local.json` (at the
repository root) and `.claude/settings.json`, then `<config>/settings.json`.
A directory chosen by managed or user settings is shared by every project.

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
