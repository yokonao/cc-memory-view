// Package memory loads Claude Code's auto memory from ~/.claude/projects/*/memory/
// or the directories autoMemoryDirectory chooses.
package memory

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const indexFile = "MEMORY.md"

type Project struct {
	// Dir is the project's directory under ~/.claude/projects, or "" for a
	// memory directory several projects share.
	Dir string
	// MemoryDir holds the memory files: Dir/memory or autoMemoryDirectory.
	MemoryDir string
	// Path is the working directory the project was created for, or "" when
	// unknown.
	Path     string
	Memories []*Memory
}

// Name is Path with the home directory shortened to ~, or else the directory
// name of Dir, or else MemoryDir.
func (p *Project) Name() string {
	switch {
	case p.Path != "":
		return shortenHome(p.Path)
	case p.Dir != "":
		return filepath.Base(p.Dir)
	default:
		return shortenHome(p.MemoryDir)
	}
}

func shortenHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return path
}

type Memory struct {
	File        string
	Name        string
	Description string
	Type        string
	Modified    time.Time
	Body        string
}

// ConfigDir returns Claude Code's configuration directory: $CLAUDE_CONFIG_DIR
// or ~/.claude.
func ConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// Root returns the directory holding project directories.
func Root() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects"), nil
}

// Load reads the memory directory of every project under root, which is
// <config>/projects. A project's memory directory is the autoMemoryDirectory
// of its managed, local, shared project or user (<config>/settings.json)
// settings, in that order, or else <root>/<project>/memory. A directory the
// managed or user settings choose is loaded even when no project uses it.
func Load(root string) ([]*Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	managed := managedMemoryDir(home)
	user := settingsMemoryDir(filepath.Join(filepath.Dir(root), "settings.json"), home)

	type member struct{ dir, cwd string }
	byMemDir := map[string][]member{}
	if global := cmp.Or(managed, user); global != "" {
		byMemDir[global] = nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cwd := sessionCwd(dir)
		memDir := cmp.Or(managed, projectMemoryDir(cwd, home), user, filepath.Join(dir, "memory"))
		byMemDir[memDir] = append(byMemDir[memDir], member{dir, cwd})
	}

	var projects []*Project
	for memDir, members := range byMemDir {
		p := &Project{MemoryDir: memDir}
		if len(members) == 1 {
			p.Dir, p.Path = members[0].dir, members[0].cwd
		}
		err := loadProject(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		if a, b := projects[i].Name(), projects[j].Name(); a != b {
			return a < b
		}
		return projects[i].MemoryDir < projects[j].MemoryDir
	})
	return projects, nil
}

func loadProject(p *Project) error {
	memDir := p.MemoryDir
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == indexFile {
			continue
		}
		file := filepath.Join(memDir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		p.Memories = append(p.Memories, parseMemory(file, string(data), info.ModTime()))
	}
	if p.Memories == nil {
		return fs.ErrNotExist
	}
	return nil
}

func parseMemory(file, data string, mtime time.Time) *Memory {
	fm, body := splitFrontmatter(data)
	m := &Memory{
		File:        file,
		Name:        fm["name"],
		Description: fm["description"],
		Type:        fm["metadata.type"],
		Modified:    mtime,
		Body:        body,
	}
	if m.Name == "" {
		m.Name = strings.TrimSuffix(filepath.Base(file), ".md")
	}
	if m.Type == "" {
		m.Type = fm["type"]
	}
	if t, err := time.Parse(time.RFC3339, fm["metadata.modified"]); err == nil {
		m.Modified = t
	}
	return m
}

// splitFrontmatter parses the flat YAML Claude Code writes: top-level
// "key: value" lines and one level of nesting, keyed as "parent.key".
func splitFrontmatter(s string) (map[string]string, string) {
	fm := map[string]string{}
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return fm, s
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return fm, s
	}
	var parent string
	for _, line := range strings.Split(head, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			fm[parent+"."+k] = v
			continue
		}
		parent = k
		fm[k] = v
	}
	return fm, strings.TrimLeft(body, "\n")
}

// sessionCwd returns the cwd recorded in the project's session logs.
func sessionCwd(dir string) string {
	logs, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, log := range logs {
		if cwd := firstCwd(log); cwd != "" {
			return cwd
		}
	}
	return ""
}

func firstCwd(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	r := bufio.NewReader(f)
	for range 50 {
		line, err := r.ReadBytes('\n')
		if bytes.Contains(line, []byte(`"cwd"`)) {
			var v struct{ Cwd string }
			if json.Unmarshal(line, &v) == nil && v.Cwd != "" {
				return v.Cwd
			}
		}
		if err != nil {
			return ""
		}
	}
	return ""
}
