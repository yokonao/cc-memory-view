package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yokonao/cc-memory-view/internal/audit"
)

func newServer(t *testing.T) (*Server, *int) {
	t.Helper()
	started := 0
	return &Server{
		Root:   "/cfg/projects",
		Audits: audit.Store{Dir: t.TempDir()},
		StartAudit: func(context.Context, string) (string, error) {
			started++
			return "0ebad0f0", nil
		},
	}, &started
}

func do(h http.Handler, method, path, host, fetchSite, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = host
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStartAuditGuard(t *testing.T) {
	s, started := newServer(t)
	h := s.Handler()
	for _, tc := range []struct {
		host, fetchSite string
		want            int
	}{
		{"127.0.0.1:8080", "same-origin", http.StatusOK},
		{"[::1]:8080", "same-origin", http.StatusOK},
		{"localhost", "same-origin", http.StatusOK},
		{"memory.localhost:3000", "same-origin", http.StatusOK},
		{"127.0.0.1:8080", "cross-site", http.StatusForbidden},
		{"127.0.0.1:8080", "", http.StatusForbidden},
		{"evil.example:8080", "same-origin", http.StatusForbidden},
		{"localhost.evil.example", "same-origin", http.StatusForbidden},
	} {
		rec := do(h, http.MethodPost, "/api/audits", tc.host, tc.fetchSite, "")
		if rec.Code != tc.want {
			t.Errorf("host %q, Sec-Fetch-Site %q: status %d, want %d", tc.host, tc.fetchSite, rec.Code, tc.want)
		}
	}
	if *started != 4 {
		t.Errorf("started %d sessions, want 4", *started)
	}
}

func TestAuditFlow(t *testing.T) {
	s, _ := newServer(t)
	h := s.Handler()

	rec := do(h, http.MethodPost, "/api/audits", "localhost", "same-origin", "")
	var started struct{ Token, ID string }
	if err := json.NewDecoder(rec.Body).Decode(&started); err != nil || started.ID != "0ebad0f0" {
		t.Fatalf("start: %v %+v", err, started)
	}
	path := "/api/audits/" + started.Token

	if rec := do(h, http.MethodGet, "/api/audits", "localhost", "", ""); !strings.Contains(rec.Body.String(), `"session":{"token":"`+started.Token) || !strings.Contains(rec.Body.String(), `"status":"starting"`) {
		t.Errorf("list: %s", rec.Body)
	}
	if rec := do(h, http.MethodPost, path+"/reply", "localhost", "same-origin", `{"message": "hi"}`); rec.Code != http.StatusConflict {
		t.Errorf("reply before the session posted: %d", rec.Code)
	}

	state := `{"status": "waiting", "message": "**Hello**", "suggestions": [{"id": "s1", "action": "delete", "files": ["/cfg/projects/-p/memory/a.md", "/elsewhere.md"], "reason": "old"}]}`
	if err := s.Audits.Post(started.Token, strings.NewReader(state)); err != nil {
		t.Fatal(err)
	}
	rec = do(h, http.MethodGet, path, "localhost", "", "")
	var got auditJSON
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "waiting" || !strings.Contains(got.MessageHTML, "<strong>Hello</strong>") ||
		len(got.Suggestions) != 1 || !got.Suggestions[0].Open ||
		got.Suggestions[0].MemoryIDs[0] != "-p/memory/a.md" || got.Suggestions[0].MemoryIDs[1] != "" {
		t.Errorf("audit: %+v", got)
	}

	if rec := do(h, http.MethodPost, path+"/reply", "localhost", "cross-site", `{"message": "hi"}`); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site reply: %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, path+"/reply", "localhost", "same-origin", `{"decisions": [{"id": "s1", "decision": "accept"}]}`); rec.Code != http.StatusNoContent {
		t.Errorf("reply: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodGet, path, "localhost", "", ""); !strings.Contains(rec.Body.String(), `"decision":"accept"`) {
		t.Errorf("reply not recorded: %s", rec.Body)
	}
	if rec := do(h, http.MethodGet, "/api/audits/0000000000000000", "localhost", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown audit: %d", rec.Code)
	}
}
