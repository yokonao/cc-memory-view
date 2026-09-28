// Package audit starts a background Claude Code session that audits memory
// with the user.
package audit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Dir returns where audits are stored: $XDG_DATA_HOME/cc-memory-view,
// falling back to ~/.local/share.
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

type Params struct {
	// Dir is the project directory the session runs in.
	Dir string
	// ConfigDir is Claude Code's configuration directory.
	ConfigDir string
	// MemoryDir is the project's memory directory.
	MemoryDir string
	// Exe is the cc-memory-view binary the session runs.
	Exe string
	// Token names the audit the session reports to.
	Token string
}

// Prompt is the fixed instruction the session starts with.
func Prompt(p Params) string {
	cmd := func(args string) string { return "`" + p.Exe + " " + args + "`" }
	return fmt.Sprintf(`Audit this project's Claude Code auto memory in %[3]s with me. I follow along and reply in the cc-memory-view web UI, which talks to you only through these pre-approved commands. Run each of them, and rm and gh, as its own Bash call, never combined with other commands (no ;, &&, pipes or loops), or it will wait for a permission prompt nobody sees. Read files with the Read, Glob and Grep tools, not the shell.

- %[1]s: update the web UI. Pass JSON on stdin (e.g. a quoted heredoc):
  {"status": "working" | "waiting" | "done", "message": "Markdown for me", "suggestions": [{"id": "s1", "action": "keep" | "update" | "merge" | "move" | "delete", "files": ["/absolute/path.md"], "reason": "Markdown", "proposed": "Markdown: the new content or the change", "status": "open" | "applied" | "dismissed"}]}
  Status and message replace the previous ones. Suggestions are merged by ID, field by field: send new ones in full, and only the changed fields of existing ones (e.g. {"id": "s1", "status": "applied"}). Suggestions you leave out are kept; dismiss one instead of dropping it.
- %[2]s: prints each reply from the web UI as one JSON line: {"message": "...", "decisions": [{"id": "s1", "decision": "approve" | "comment", "comment": "..."}]}. "approve" may carry a comment to take into account; "comment" asks you to revise the suggestion, or dismiss it if the comment says so. It exits once you set status "done".

Memory is the last resort. For each memory, check whether it has a better home:
- this repository's docs or source code (comments, tests, config),
- issues or pull request descriptions (gh issue view/list, gh pr view/list, gh search),
- this project's CLAUDE.md, or %[4]s when it holds across projects (other projects' memory under %[5]s shows whether it does),
- a skill, in this repository's .claude/skills or %[6]s.
If it's already there, suggest "delete". If it belongs there but isn't, suggest "move", with the destination and content in "proposed". Otherwise suggest "keep", "update" or "merge".

Steps:
1. Read every memory file in %[3]s, look for better homes, and send your suggestions with status "waiting".
2. Start the Monitor tool with %[2]s as its command and the longest timeout it allows. Its events are my replies. Whenever it expires before you set status "done", start it again: it resumes where it left off, so no reply is repeated or lost.
3. Don't change any file until a reply approves it. On each reply, set status "working", apply the approved suggestions (taking comments into account, and keeping MEMORY.md in sync), then update the suggestions you applied or revised and set status "waiting", or "done" when I say we're finished. Don't write to GitHub: for a move to an issue or pull request, put the text in your message and delete the memory once I say I've posted it.`,
		cmd("audit update "+p.Token),
		cmd("audit watch "+p.Token),
		p.MemoryDir,
		filepath.Join(p.ConfigDir, "CLAUDE.md"),
		filepath.Join(p.ConfigDir, "projects", "*", "memory"),
		filepath.Join(p.ConfigDir, "skills"))
}

// allowedTools pre-approves what the session needs: reading, the
// cc-memory-view commands, rm, read-only gh and Monitor. Anything else is left
// to auto mode, within the project and ConfigDir.
func allowedTools(p Params) string {
	return strings.Join([]string{
		"Read", "Glob", "Grep", "Monitor",
		"Bash(" + p.Exe + " audit:*)",
		"Bash(rm:*)",
		"Bash(gh issue view:*)", "Bash(gh issue list:*)",
		"Bash(gh pr view:*)", "Bash(gh pr list:*)",
		"Bash(gh search:*)",
	}, ",")
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	idRe   = regexp.MustCompile(`backgrounded · (\S+)`)
)

// Start runs `claude --bg` and returns the session ID it prints.
func Start(ctx context.Context, p Params) (string, error) {
	if _, err := os.Stat(p.Dir); err != nil {
		return "", err
	}
	// The prompt goes first: --add-dir and --allowedTools take several values.
	cmd := exec.CommandContext(ctx, "claude", "--bg", Prompt(p),
		"--permission-mode", "auto",
		"--add-dir", p.ConfigDir,
		"--allowedTools", allowedTools(p))
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(ansiRe.ReplaceAllString(string(out), ""))
	if err != nil {
		if strings.Contains(text, "not trusted") {
			return "", fmt.Errorf("%s isn't trusted: run claude there once and accept the trust prompt", p.Dir)
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
