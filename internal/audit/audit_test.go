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

	id, err := Start(context.Background(), Params{Dir: dir, ConfigDir: "/cfg", Exe: "/bin/cc-memory-view", StaleDays: 30})
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
	fakeClaude(t, `echo "Workspace not trusted. Run claude there." >&2; exit 1`)
	if _, err := Start(context.Background(), Params{Dir: t.TempDir()}); err != errSetup {
		t.Errorf("untrusted: err = %v", err)
	}
	if _, err := Start(context.Background(), Params{Dir: filepath.Join(t.TempDir(), "missing")}); err != errSetup {
		t.Errorf("missing workspace: err = %v", err)
	}

	fakeClaude(t, `echo "boom" >&2; exit 1`)
	if _, err := Start(context.Background(), Params{Dir: t.TempDir()}); err == nil || err.Error() != "boom" {
		t.Errorf("other failure: err = %v", err)
	}
}

func TestSetup(t *testing.T) {
	log := fakeClaude(t, "")
	dir := filepath.Join(t.TempDir(), "a", "cc-memory-view")

	if err := Setup(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if first := strings.SplitN(string(got), "\n", 2)[0]; first != dir {
		t.Errorf("claude ran in %q, want %q", first, dir)
	}
}

func TestDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if dir, _ := Dir(); dir != "/data/cc-memory-view" {
		t.Errorf("Dir() = %q", dir)
	}
}
