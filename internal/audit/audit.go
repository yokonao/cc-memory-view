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
	// Exe is the cc-memory-view binary the session runs.
	Exe string
	// Token names the audit the session reports to.
	Token string
}

// Prompt is the fixed instruction the session starts with.
func Prompt(p Params) string {
	cmd := func(args string) string { return "`" + p.Exe + " " + args + "`" }
	return fmt.Sprintf(`Audit my Claude Code auto memory with me. I follow along and reply in the cc-memory-view web UI, which talks to you only through these pre-approved commands. Run each of them, and rm, as its own Bash call, never combined with other commands (no ;, &&, pipes or loops), or it will wait for a permission prompt nobody sees. Read memory files with the Read, Glob and Grep tools, not the shell.

- %[1]s: update the web UI. Pass JSON on stdin (e.g. a quoted heredoc):
  {"status": "working" | "waiting" | "done", "message": "Markdown for me", "suggestions": [{"id": "s1", "action": "keep" | "update" | "merge" | "promote" | "delete", "files": ["/absolute/path.md"], "reason": "Markdown", "proposed": "Markdown: the new content or the change", "status": "open" | "applied" | "dismissed"}]}
  Status and message replace the previous ones. Suggestions are merged by ID, field by field: send new ones in full, and only the changed fields of existing ones (e.g. {"id": "s1", "status": "applied"}). Suggestions you leave out are kept; dismiss one instead of dropping it.
- %[2]s: prints each reply from the web UI as one JSON line: {"message": "...", "decisions": [{"id": "s1", "decision": "approve" | "comment", "comment": "..."}]}. "approve" may carry a comment to take into account; "comment" asks you to revise the suggestion, or dismiss it if the comment says so. It exits once you set status "done".

Steps:
1. Read every memory file under %[3]s with Glob and Read, and send your suggestions with status "waiting". Suggest promoting to %[4]s when the same rule appears in several projects.
2. Start the Monitor tool with %[2]s as its command and the longest timeout it allows. Its events are my replies. Whenever it expires before you set status "done", start it again: it resumes where it left off, so no reply is repeated or lost.
3. Don't change any file until a reply approves it. On each reply, set status "working", apply the approved suggestions (taking comments into account, and keeping each project's MEMORY.md in sync), then update the suggestions you applied or revised and set status "waiting", or "done" when I say we're finished.`,
		cmd("audit update "+p.Token),
		cmd("audit watch "+p.Token),
		filepath.Join(p.ConfigDir, "projects", "*", "memory"),
		filepath.Join(p.ConfigDir, "CLAUDE.md"))
}

// allowedTools lets the session work in the background without permission
// prompts: reading, the cc-memory-view commands, rm and Monitor. Edits are
// accepted by the permission mode, within the workspace and ConfigDir.
func allowedTools(p Params) string {
	return strings.Join([]string{
		"Read", "Glob", "Grep", "Monitor",
		"Bash(" + p.Exe + " audit:*)",
		"Bash(rm:*)",
	}, ",")
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
	// The prompt goes first: --add-dir and --allowedTools take several values.
	cmd := exec.CommandContext(ctx, "claude", "--bg", Prompt(p),
		"--permission-mode", "acceptEdits",
		"--add-dir", p.ConfigDir,
		"--allowedTools", allowedTools(p))
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
