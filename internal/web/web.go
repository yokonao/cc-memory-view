// Package web serves a browser UI over the memory files.
package web

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/yokonao/cc-memory-view/internal/audit"
	"github.com/yokonao/cc-memory-view/internal/memory"
	"github.com/yuin/goldmark"
)

//go:embed index.html
var indexHTML []byte

type Server struct {
	Root       string
	StaleAfter time.Duration
	Audits     audit.Store
	// StartAudit starts a Claude Code session auditing memory that reports
	// to the audit token, and returns the session's ID.
	StartAudit func(ctx context.Context, token string) (string, error)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("GET /api/data", s.data)
	mux.HandleFunc("POST /api/audits", s.local(s.startAudit))
	mux.HandleFunc("GET /api/audits", s.listAudits)
	mux.HandleFunc("GET /api/audits/{token}", s.getAudit)
	mux.HandleFunc("POST /api/audits/{token}/reply", s.local(s.reply))
	return mux
}

type memoryJSON struct {
	ID          string         `json:"id"`
	Project     string         `json:"project"`
	File        string         `json:"file"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Modified    time.Time      `json:"modified"`
	Body        string         `json:"body"`
	HTML        string         `json:"html"`
	Issues      []memory.Issue `json:"issues"`
}

type dataJSON struct {
	Memories []memoryJSON   `json:"memories"`
	Issues   []memory.Issue `json:"issues"`
}

func (s *Server) data(w http.ResponseWriter, _ *http.Request) {
	projects, err := memory.Load(s.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	issues := memory.Check(projects, time.Now(), s.StaleAfter)
	byFile := map[string][]memory.Issue{}
	for _, is := range issues {
		byFile[is.File] = append(byFile[is.File], is)
	}

	out := dataJSON{Memories: []memoryJSON{}, Issues: issues}
	if out.Issues == nil {
		out.Issues = []memory.Issue{}
	}
	for _, p := range projects {
		ids := map[string]string{}
		for _, m := range p.Memories {
			ids[m.Name] = s.memoryID(m.File)
		}
		for _, m := range p.Memories {
			html, err := render(m.Body, ids)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out.Memories = append(out.Memories, memoryJSON{
				ID:          s.memoryID(m.File),
				Project:     p.Name(),
				File:        m.File,
				Name:        m.Name,
				Description: m.Description,
				Type:        m.Type,
				Modified:    m.Modified,
				Body:        m.Body,
				HTML:        html,
				Issues:      byFile[m.File],
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func localHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

var memoryLinkRe = regexp.MustCompile(`\[\[[^\[\]]+\]\]`)

// render converts Markdown to HTML, turning [[name]] into links to the
// memory's page when it exists. Raw HTML in the source is escaped.
func render(body string, ids map[string]string) (string, error) {
	src := memoryLinkRe.ReplaceAllStringFunc(body, func(s string) string {
		name := s[2 : len(s)-2]
		if id, ok := ids[name]; ok {
			return "[" + name + "](#" + id + ")"
		}
		return s
	})
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
