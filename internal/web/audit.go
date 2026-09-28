package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/yokonao/cc-memory-view/internal/audit"
)

// local accepts only same-origin requests from a page served on a local host
// name, so other sites can't start or steer sessions, even through DNS
// rebinding.
func (s *Server) local(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) || r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) startAudit(w http.ResponseWriter, r *http.Request) {
	var req struct{ Project string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.Project == "" {
		http.Error(w, "choose a project to audit", http.StatusBadRequest)
		return
	}
	token := audit.NewToken()
	started := time.Now()
	id, err := s.StartAudit(r.Context(), token, req.Project)
	if err == nil {
		if err = s.Audits.Create(token); err == nil {
			err = s.Audits.SaveSession(audit.Session{Token: token, ID: id, Project: req.Project, Started: started})
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"token": token, "id": id})
}

type auditSummary struct {
	Session audit.Session `json:"session"`
	Status  string        `json:"status"`
	Updated time.Time     `json:"updated"`
}

func (s *Server) listAudits(w http.ResponseWriter, _ *http.Request) {
	audits, err := s.Audits.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []auditSummary{}
	for _, a := range audits {
		sum := auditSummary{Session: a.Session, Status: "starting"}
		if a.State != nil {
			sum.Status, sum.Updated = a.State.Status, a.State.Updated
		}
		out = append(out, sum)
	}
	writeJSON(w, out)
}

type suggestionJSON struct {
	audit.Suggestion
	Open         bool     `json:"open"`
	ReasonHTML   string   `json:"reasonHTML"`
	ProposedHTML string   `json:"proposedHTML"`
	MemoryIDs    []string `json:"memoryIDs"`
}

type auditJSON struct {
	Session     audit.Session    `json:"session"`
	Status      string           `json:"status"`
	Updated     time.Time        `json:"updated"`
	MessageHTML string           `json:"messageHTML"`
	Suggestions []suggestionJSON `json:"suggestions"`
	Replies     []audit.Reply    `json:"replies"`
}

func (s *Server) getAudit(w http.ResponseWriter, r *http.Request) {
	a, err := s.Audits.Load(r.PathValue("token"))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out := auditJSON{Session: a.Session, Status: "starting", Suggestions: []suggestionJSON{}, Replies: a.Replies}
	if st := a.State; st != nil {
		out.Status, out.Updated = st.Status, st.Updated
		if out.MessageHTML, err = render(st.Message, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, sg := range st.Suggestions {
			sj := suggestionJSON{Suggestion: sg, Open: sg.Open(), MemoryIDs: []string{}}
			if sj.ReasonHTML, err = render(sg.Reason, nil); err == nil {
				sj.ProposedHTML, err = render(sg.Proposed, nil)
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, f := range sg.Files {
				sj.MemoryIDs = append(sj.MemoryIDs, s.memoryID(f))
			}
			out.Suggestions = append(out.Suggestions, sj)
		}
	}
	writeJSON(w, out)
}

// memoryID returns the UI's ID for a memory file, or "" for files outside
// the projects directory.
func (s *Server) memoryID(file string) string {
	rel, err := filepath.Rel(s.Root, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (s *Server) reply(w http.ResponseWriter, r *http.Request) {
	var rep audit.Reply
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&rep); err != nil {
		http.Error(w, "invalid reply: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Audits.AddReply(r.PathValue("token"), rep); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
