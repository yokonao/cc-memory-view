package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

const (
	KindOrphanProject = "orphan_project"
	KindNoIndex       = "no_index"
	KindUnindexed     = "unindexed"
	KindIndexMissing  = "index_missing"
	KindBrokenLink    = "broken_link"
	KindStale         = "stale"
)

type Issue struct {
	Kind    string `json:"kind"`
	Project string `json:"project"`
	// File is the memory file, or the memory directory for project-wide issues.
	File   string `json:"file"`
	Detail string `json:"detail"`
}

// Check finds audit candidates. Memories not modified within staleAfter of
// now are reported as stale.
func Check(projects []*Project, now time.Time, staleAfter time.Duration) []Issue {
	var issues []Issue
	for _, p := range projects {
		issues = append(issues, checkProject(p, now, staleAfter)...)
	}
	return issues
}

func checkProject(p *Project, now time.Time, staleAfter time.Duration) []Issue {
	var issues []Issue
	add := func(kind, file, format string, args ...any) {
		issues = append(issues, Issue{Kind: kind, Project: p.Name(), File: file, Detail: fmt.Sprintf(format, args...)})
	}
	memDir := filepath.Join(p.Dir, "memory")

	if p.Path != "" && !exists(p.Path) {
		add(KindOrphanProject, memDir, "project directory %s no longer exists", p.Path)
	}

	names := map[string]bool{}
	files := map[string]bool{}
	for _, m := range p.Memories {
		names[m.Name] = true
		files[filepath.Base(m.File)] = true
	}

	if p.Index == nil {
		add(KindNoIndex, memDir, "%s is missing", indexFile)
	} else {
		for _, f := range p.Index {
			if !files[f] {
				add(KindIndexMissing, filepath.Join(memDir, indexFile), "%s links to missing %s", indexFile, f)
			}
		}
	}

	for _, m := range p.Memories {
		if p.Index != nil && !slices.Contains(p.Index, filepath.Base(m.File)) {
			add(KindUnindexed, m.File, "not listed in %s", indexFile)
		}
		for _, name := range Links(m.Body) {
			if !names[name] {
				add(KindBrokenLink, m.File, "[[%s]] matches no memory", name)
			}
		}
		if age := now.Sub(m.Modified); age > staleAfter {
			add(KindStale, m.File, "not modified for %d days", int(age.Hours()/24))
		}
	}
	return issues
}

var linkRe = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// Links returns the names referenced as [[name]].
func Links(body string) []string {
	var names []string
	for _, m := range linkRe.FindAllStringSubmatch(body, -1) {
		names = append(names, m[1])
	}
	return names
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
