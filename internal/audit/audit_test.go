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

	id, err := Start(context.Background(), Params{ConfigDir: dir, Exe: "/bin/cc-memory-view", StaleDays: 30})
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
	lines := strings.SplitN(string(got), "\n", 3)
	if lines[0] != dir {
		t.Errorf("ran in %q, want %q", lines[0], dir)
	}
	if lines[1] != "--bg" {
		t.Errorf("first arg = %q", lines[1])
	}
	if !strings.Contains(lines[2], "`/bin/cc-memory-view check --json --stale-days 30`") {
		t.Errorf("prompt lacks the check command:\n%s", lines[2])
	}
}

func TestStartFailure(t *testing.T) {
	fakeClaude(t, `echo "Workspace not trusted." >&2; exit 1`)

	_, err := Start(context.Background(), Params{ConfigDir: t.TempDir()})
	if err == nil || err.Error() != "Workspace not trusted." {
		t.Errorf("err = %v", err)
	}
}
