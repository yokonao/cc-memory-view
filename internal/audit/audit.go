// Package audit starts a background Claude Code session that audits memory
// with the user.
package audit

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Params struct {
	// ConfigDir is Claude Code's configuration directory. The session runs
	// there, so it must be a trusted workspace.
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
	cmd := exec.CommandContext(ctx, "claude", "--bg", Prompt(p))
	cmd.Dir = p.ConfigDir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(ansiRe.ReplaceAllString(string(out), ""))
	if err != nil {
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
