package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	KindOrphanProject = "orphan_project"
	KindNoIndex       = "no_index"
	KindUnindexed     = "unindexed"
	KindIndexMissing  = "index_missing"
	KindBrokenLink    = "broken_link"
	KindStale         = "stale"
	KindIndexTooLarge = "index_too_large"
	KindHiddenChars   = "hidden_chars"
)

// Claude Code loads MEMORY.md up to the first of these limits.
const (
	indexMaxLines = 200
	indexMaxBytes = 25_000
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
	memDir := p.MemoryDir

	if p.Path != "" && !exists(p.Path) {
		add(KindOrphanProject, memDir, "project directory %s no longer exists", p.Path)
	}

	names := map[string]bool{}
	files := map[string]bool{}
	for _, m := range p.Memories {
		names[m.Name] = true
		files[filepath.Base(m.File)] = true
	}

	index := filepath.Join(memDir, indexFile)
	if p.Index == nil {
		add(KindNoIndex, memDir, "%s is missing", indexFile)
	} else {
		for _, f := range p.Index {
			if !files[f] {
				add(KindIndexMissing, index, "%s links to missing %s", indexFile, f)
			}
		}
		if detail := indexTooLarge(p.index); detail != "" {
			add(KindIndexTooLarge, index, "%s", detail)
		}
		for _, d := range hiddenDetails(p.index) {
			add(KindHiddenChars, index, "%s", d)
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
		for _, d := range hiddenDetails(m.data) {
			add(KindHiddenChars, m.File, "%s", d)
		}
	}
	return issues
}

var blockCommentRe = regexp.MustCompile(`(?ms)^[ \t]*<!--.*?-->[ \t]*(?:\n|\z)`)

// indexTooLarge describes how MEMORY.md exceeds the load limits, measured
// like Claude Code without frontmatter and block-level HTML comments, or
// returns "" when it fits.
func indexTooLarge(data string) string {
	_, body := splitFrontmatter(data)
	body = blockCommentRe.ReplaceAllString(body, "")
	lines := strings.Count(strings.TrimSuffix(body, "\n"), "\n") + 1
	var over []string
	if lines > indexMaxLines {
		over = append(over, fmt.Sprintf("%d lines, over the %d-line limit", lines, indexMaxLines))
	}
	if len(body) > indexMaxBytes {
		over = append(over, fmt.Sprintf("%d bytes, over the %d-byte limit", len(body), indexMaxBytes))
	}
	if over == nil {
		return ""
	}
	return strings.Join(over, " and ") + "; the rest isn't loaded"
}

func hiddenDetails(data string) []string {
	var details []string
	for _, h := range hiddenChars(data) {
		details = append(details, h.String())
	}
	for _, m := range markup(data) {
		tags := "tag"
		if m.count > 1 {
			tags = "tags"
		}
		details = append(details, fmt.Sprintf("%d %s %s imitating Claude Code markup, first at line %d", m.count, m.tag, tags, m.line))
	}
	return details
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
