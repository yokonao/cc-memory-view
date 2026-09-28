// Package audit starts a background Claude Code session that audits memory
// with the user.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// errSetup tells the user how to fix a missing or untrusted workspace.
var errSetup = errors.New("the audit workspace isn't set up: run `cc-memory-view setup`")

// Dir returns the workspace audit sessions run in:
// $XDG_DATA_HOME/cc-memory-view, falling back to ~/.local/share.
func Dir() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "cc-memory-view"), nil
}

// Setup creates the workspace and runs claude there interactively, so the
// user can accept the workspace trust prompt that `claude --bg` requires.
func Setup(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if trusted(dir) {
		fmt.Printf("%s is already set up.\n", dir)
		return nil
	}
	fmt.Printf("Starting claude in %s.\nAccept the trust prompt if asked, then type /exit.\n\n", dir)
	cmd := exec.Command("claude")
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// trusted reports whether Claude Code recorded dir as a trusted workspace in
// its global config ($CLAUDE_CONFIG_DIR/.claude.json or ~/.claude.json). It
// only reads the file, and any doubt counts as untrusted.
func trusted(dir string) bool {
	path := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json")
	if os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		path = filepath.Join(home, ".claude.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		Projects map[string]struct {
			HasTrustDialogAccepted bool `json:"hasTrustDialogAccepted"`
		} `json:"projects"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	return cfg.Projects[dir].HasTrustDialogAccepted
}

type Params struct {
	// Dir is the trusted workspace the session runs in.
	Dir string
	// ConfigDir is Claude Code's configuration directory.
	ConfigDir string
	// Exe is the cc-memory-view binary the session runs for check.
	Exe       string
	StaleDays int
}

// Prompt is the fixed instruction the session starts with.
func Prompt(p Params) string {
	return fmt.Sprintf(`Audit my Claude Code auto memory with me.

1. Run `+"`%s check --json --stale-days %d`"+` for rule-based candidates: orphaned projects, MEMORY.md inconsistencies, broken [[links]], missing paths and stale memories.
2. Read the memory files under %s across all projects, including ones without issues.
3. For each memory worth acting on, propose one of: keep, update, merge with another memory, promote to %s (when the same rule appears in several projects), or delete. Say briefly why.
4. Wait for my confirmation before changing any file. When a memory file is added, renamed or deleted, keep its project's MEMORY.md in sync.`,
		p.Exe, p.StaleDays,
		filepath.Join(p.ConfigDir, "projects", "*", "memory"),
		filepath.Join(p.ConfigDir, "CLAUDE.md"))
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	idRe   = regexp.MustCompile(`backgrounded · (\S+)`)
)

// Start runs `claude --bg` and returns the session ID it prints.
func Start(ctx context.Context, p Params) (string, error) {
	if _, err := os.Stat(p.Dir); errors.Is(err, fs.ErrNotExist) {
		return "", errSetup
	}
	cmd := exec.CommandContext(ctx, "claude", "--bg", Prompt(p))
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(ansiRe.ReplaceAllString(string(out), ""))
	if err != nil {
		if strings.Contains(text, "not trusted") {
			return "", errSetup
		}
		if text == "" {
			return "", err
		}
		return "", errors.New(text)
	}
	m := idRe.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("no session ID in claude output: %s", text)
	}
	return m[1], nil
}
