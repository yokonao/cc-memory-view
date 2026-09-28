package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAudit(t *testing.T) {
	started := 0
	s := &Server{StartAudit: func(context.Context) (string, error) {
		started++
		return "0ebad0f0", nil
	}}
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
		req := httptest.NewRequest(http.MethodPost, "/api/audit", nil)
		req.Host = tc.host
		if tc.fetchSite != "" {
			req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("host %q, Sec-Fetch-Site %q: status %d, want %d", tc.host, tc.fetchSite, rec.Code, tc.want)
		}
		if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), `"id":"0ebad0f0"`) {
			t.Errorf("body = %s", rec.Body)
		}
	}
	if started != 4 {
		t.Errorf("started %d sessions, want 4", started)
	}
}
