package memory

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
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

// isolate keeps the machine's managed settings out of the test.
func isolate(t *testing.T) {
	t.Helper()
	old := managedSettingsDir
	managedSettingsDir = t.TempDir()
	t.Cleanup(func() { managedSettingsDir = old })
}

func TestLoad(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	work := t.TempDir()

	proj := filepath.Join(root, "-work")
	write(t, filepath.Join(proj, "s.jsonl"), `{"type":"summary"}`+"\n"+`{"cwd":"`+work+`"}`+"\n")
	write(t, filepath.Join(proj, "memory", "MEMORY.md"), "- [A](a.md) — a\n")
	write(t, filepath.Join(proj, "memory", "a.md"), `---
name: a
description: first
metadata:
  type: feedback
  modified: 2026-01-01T00:00:00Z
---

body
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
	if len(p.Memories) != 2 {
		t.Fatalf("got %d memories, want 2 without MEMORY.md", len(p.Memories))
	}
	a := p.Memories[0]
	if a.Name != "a" || a.Description != "first" || a.Type != "feedback" || !a.Modified.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected memory: %+v", a)
	}
}

func TestLoadMemoryDirectory(t *testing.T) {
	setting := func(dir string) string { return `{"autoMemoryDirectory":"` + dir + `"}` }
	tests := []struct {
		name string
		// files maps paths relative to a temp dir to their content; $TMP in
		// the content is the temp dir.
		files map[string]string
		// want maps project names (or memory dirs relative to the temp dir)
		// to the memory dir relative to the temp dir.
		want map[string]string
	}{
		{
			name: "default",
			files: map[string]string{
				"config/projects/-work/s.jsonl":          `{"cwd":"$TMP/work"}`,
				"config/projects/-work/memory/a.md":      "a",
				"config/projects/-other/memory/b.md":     "b",
				"config/projects/-none/s.jsonl":          "{}",
				"work/.claude/settings.json":             `{"autoMemoryDirectory":"relative/dir"}`,
				"config/projects/-other/s.jsonl":         "{}",
				"config/projects/-work/memory/MEMORY.md": "",
			},
			want: map[string]string{"work": "config/projects/-work/memory", "-other": "config/projects/-other/memory"},
		},
		{
			name: "project settings",
			files: map[string]string{
				"config/projects/-work/s.jsonl":     `{"cwd":"$TMP/work"}`,
				"config/projects/-work/memory/a.md": "ignored",
				"work/.claude/settings.json":        setting("$TMP/custom"),
				"custom/a.md":                       "a",
			},
			want: map[string]string{"work": "custom"},
		},
		{
			name: "local settings at the repository root win over project settings",
			files: map[string]string{
				"config/projects/-work/s.jsonl":    `{"cwd":"$TMP/repo/sub"}`,
				"repo/.git/HEAD":                   "",
				"repo/sub/.claude/settings.json":   setting("$TMP/shared"),
				"repo/.claude/settings.local.json": setting("$TMP/local"),
				"shared/a.md":                      "a",
				"local/a.md":                       "a",
			},
			want: map[string]string{"repo/sub": "local"},
		},
		{
			name: "worktree uses the main checkout's local settings",
			files: map[string]string{
				"config/projects/-wt/s.jsonl":      `{"cwd":"$TMP/wt"}`,
				"repo/.git/worktrees/wt/commondir": "../..",
				"wt/.git":                          "gitdir: $TMP/repo/.git/worktrees/wt",
				"repo/.claude/settings.local.json": setting("$TMP/local"),
				"local/a.md":                       "a",
			},
			want: map[string]string{"wt": "local"},
		},
		{
			name: "user settings are shared by every project",
			files: map[string]string{
				"config/settings.json":           setting("$TMP/global"),
				"config/projects/-a/s.jsonl":     `{"cwd":"$TMP/a"}`,
				"config/projects/-b/s.jsonl":     `{"cwd":"$TMP/b"}`,
				"config/projects/-b/memory/x.md": "ignored",
				"config/projects/-c/s.jsonl":     `{"cwd":"$TMP/c"}`,
				"c/.claude/settings.json":        setting("$TMP/own"),
				"global/a.md":                    "a",
				"own/a.md":                       "a",
			},
			want: map[string]string{"global": "global", "c": "own"},
		},
		{
			name: "managed settings win and load without projects",
			files: map[string]string{
				"managed/managed-settings.json":          setting("$TMP/first"),
				"managed/managed-settings.d/10-mem.json": setting("$TMP/policy"),
				"config/settings.json":                   setting("$TMP/global"),
				"config/projects/-a/s.jsonl":             `{"cwd":"$TMP/a"}`,
				"a/.claude/settings.json":                setting("$TMP/own"),
				"policy/a.md":                            "a",
				"first/a.md":                             "a",
				"global/a.md":                            "a",
				"own/a.md":                               "a",
			},
			want: map[string]string{"a": "policy"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			old := managedSettingsDir
			managedSettingsDir = filepath.Join(tmp, "managed")
			t.Cleanup(func() { managedSettingsDir = old })
			for path, content := range tt.files {
				write(t, filepath.Join(tmp, path), strings.ReplaceAll(content, "$TMP", tmp))
			}
			if err := os.MkdirAll(filepath.Join(tmp, "config", "projects"), 0o755); err != nil {
				t.Fatal(err)
			}

			projects, err := Load(filepath.Join(tmp, "config", "projects"))
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, p := range projects {
				name := p.Name()
				if rel, err := filepath.Rel(tmp, name); err == nil && !strings.HasPrefix(rel, "..") {
					name = rel
				}
				mem, _ := filepath.Rel(tmp, p.MemoryDir)
				got[name] = mem
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandMemoryDir(t *testing.T) {
	tests := []struct {
		value, home, want string
	}{
		{"~/mem", "/home/u", "/home/u/mem"},
		{"/abs/mem/", "/home/u", "/abs/mem"},
		{"~/mem", "", ""},
		{"~", "/home/u", ""},
		{"~other/mem", "/home/u", ""},
		{"relative/mem", "/home/u", ""},
		{"", "/home/u", ""},
	}
	for _, tt := range tests {
		if got := expandMemoryDir(tt.value, tt.home); got != tt.want {
			t.Errorf("expandMemoryDir(%q, %q) = %q, want %q", tt.value, tt.home, got, tt.want)
		}
	}
}
