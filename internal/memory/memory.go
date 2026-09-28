// Package memory loads Claude Code's auto memory from ~/.claude/projects/*/memory/.
package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const indexFile = "MEMORY.md"

type Project struct {
	// Dir is the project's directory under ~/.claude/projects.
	Dir string
	// Path is the working directory the project was created for, or "" when
	// no session log records it.
	Path     string
	Memories []*Memory
	// Index holds the file names MEMORY.md links to, or nil without MEMORY.md.
	Index []string
}

// Name is Path with the home directory shortened to ~, or the directory name
// when Path is unknown.
func (p *Project) Name() string {
	if p.Path == "" {
		return filepath.Base(p.Dir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p.Path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return p.Path
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

// Load reads every project under root that has a memory directory.
func Load(root string) ([]*Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var projects []*Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		p, err := loadProject(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name() < projects[j].Name() })
	return projects, nil
}

func loadProject(dir string) (*Project, error) {
	memDir := filepath.Join(dir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil, err
	}
	p := &Project{Dir: dir, Path: sessionCwd(dir)}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		file := filepath.Join(memDir, e.Name())
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if e.Name() == indexFile {
			p.Index = parseIndex(string(data))
			if p.Index == nil {
				p.Index = []string{}
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		p.Memories = append(p.Memories, parseMemory(file, string(data), info.ModTime()))
	}
	if p.Index == nil && p.Memories == nil {
		return nil, fs.ErrNotExist
	}
	return p, nil
}

var indexLinkRe = regexp.MustCompile(`\]\(([^)]+\.md)\)`)

func parseIndex(s string) []string {
	var files []string
	for _, m := range indexLinkRe.FindAllStringSubmatch(s, -1) {
		files = append(files, m[1])
	}
	return files
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
