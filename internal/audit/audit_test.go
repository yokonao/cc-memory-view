package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaude puts a claude script first in PATH that records its working
// directory and arguments, then runs body.
func fakeClaude(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	script := "#!/bin/sh\npwd > " + log + "\nprintf '%s\\n' \"$@\" >> " + log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestStart(t *testing.T) {
	log := fakeClaude(t, `printf 'backgrounded · \033[36m0ebad0f0\033[39m\n  claude attach 0ebad0f0\n'`)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	id, err := Start(context.Background(), Params{Dir: dir, ConfigDir: "/cfg", MemoryDir: "/cfg/projects/-p/memory", Exe: "/bin/cc-memory-view", Token: "aaaabbbbccccdddd"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "0ebad0f0" {
		t.Errorf("id = %q", id)
	}

	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	args := string(got)
	lines := strings.SplitN(args, "\n", 3)
	if lines[0] != dir {
		t.Errorf("ran in %q, want %q", lines[0], dir)
	}
	if lines[1] != "--bg" {
		t.Errorf("first arg = %q", lines[1])
	}
	for _, want := range []string{
		"`/bin/cc-memory-view audit watch aaaabbbbccccdddd`",
		"auto memory in /cfg/projects/-p/memory with me",
		"--permission-mode\nacceptEdits\n--add-dir\n/cfg\n--allowedTools\nRead,Glob,Grep,Monitor,Bash(/bin/cc-memory-view audit:*),Bash(rm:*),Bash(gh issue view:*),Bash(gh issue list:*),Bash(gh pr view:*),Bash(gh pr list:*),Bash(gh search:*)\n",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("claude args lack %q:\n%s", want, args)
		}
	}
}

func TestStartFailure(t *testing.T) {
	fakeClaude(t, `echo "Workspace not trusted. Run claude there." >&2; exit 1`)
	if _, err := Start(context.Background(), Params{Dir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "isn't trusted") {
		t.Errorf("untrusted: err = %v", err)
	}
	if _, err := Start(context.Background(), Params{Dir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("started in a missing directory")
	}

	fakeClaude(t, `echo "boom" >&2; exit 1`)
	if _, err := Start(context.Background(), Params{Dir: t.TempDir()}); err == nil || err.Error() != "boom" {
		t.Errorf("other failure: err = %v", err)
	}
}

func TestDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if dir, _ := Dir(); dir != "/data/cc-memory-view" {
		t.Errorf("Dir() = %q", dir)
	}
}
