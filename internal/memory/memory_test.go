package memory

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAndCheck(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()
	write(t, filepath.Join(work, "exists.txt"), "")

	proj := filepath.Join(root, "-work")
	write(t, filepath.Join(proj, "s.jsonl"), `{"type":"summary"}`+"\n"+`{"cwd":"`+work+`"}`+"\n")
	write(t, filepath.Join(proj, "memory", "MEMORY.md"), "- [A](a.md) — a\n- [Gone](gone.md) — gone\n")
	write(t, filepath.Join(proj, "memory", "a.md"), `---
name: a
description: first
metadata:
  type: feedback
  modified: 2026-01-01T00:00:00Z
---

See [[b]] and [[nope]]. Files: `+"`exists.txt`, `missing/file.go`, `gh pr checks`, `https://x/y`"+`.
`)
	write(t, filepath.Join(proj, "memory", "b.md"), "---\nname: b\ndescription: second\nmetadata:\n  type: project\n---\nbody\n")

	orphan := filepath.Join(root, "-gone")
	write(t, filepath.Join(orphan, "s.jsonl"), `{"cwd":"/nonexistent/cc-memory-view-test"}`+"\n")
	write(t, filepath.Join(orphan, "memory", "c.md"), "no frontmatter\n")

	write(t, filepath.Join(root, "-no-memory", "s.jsonl"), "{}\n")
	if err := os.MkdirAll(filepath.Join(root, "-empty-memory", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(projects))
	}

	var p *Project
	for _, q := range projects {
		if q.Path == work {
			p = q
		}
	}
	if p == nil {
		t.Fatal("project path not recovered from session log")
	}
	a := p.Memories[0]
	if a.Name != "a" || a.Description != "first" || a.Type != "feedback" || !a.Modified.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected memory: %+v", a)
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var got []string
	for _, is := range Check(projects, now, 90*24*time.Hour) {
		got = append(got, is.Kind+" "+filepath.Base(is.File)+" "+is.Detail)
	}
	want := []string{
		"orphan_project memory project directory /nonexistent/cc-memory-view-test no longer exists",
		"no_index memory MEMORY.md is missing",
		"index_missing MEMORY.md MEMORY.md links to missing gone.md",
		"broken_link a.md [[nope]] matches no memory",
		"missing_path a.md missing/file.go does not exist",
		"stale a.md not modified for 151 days",
		"unindexed b.md not listed in MEMORY.md",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("issues:\ngot  %q\nwant %q", got, want)
	}
}
