package audit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const token = "aaaabbbbccccdddd"

func newStore(t *testing.T) Store {
	t.Helper()
	s := Store{Dir: t.TempDir()}
	if err := s.Create(token); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(Session{Token: token, ID: "abc", Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return s
}

func post(t *testing.T, s Store, state string) error {
	t.Helper()
	return s.Post(token, strings.NewReader(state))
}

func TestPostValidates(t *testing.T) {
	s := newStore(t)
	for _, bad := range []string{
		`{"status": "thinking"}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "action": "burn", "files": []}]}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "action": "keep"}, {"id": "s1", "action": "keep"}]}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "action": "keep", "files": ["relative.md"]}]}`,
		`{"status": "waiting", "extra": 1}`,
	} {
		if err := post(t, s, bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if err := post(t, s, `{"status": "waiting", "message": "hi", "suggestions": [{"id": "s1", "action": "delete", "files": ["/a.md"], "reason": "old"}]}`); err != nil {
		t.Fatal(err)
	}
	a, err := s.Load(token)
	if err != nil {
		t.Fatal(err)
	}
	if a.Session.ID != "abc" || a.State.Message != "hi" || len(a.State.Suggestions) != 1 || a.State.Updated.IsZero() {
		t.Errorf("unexpected audit: %+v", a)
	}
}

func TestReplyAndWatch(t *testing.T) {
	s := newStore(t)
	accept := Reply{Decisions: []Decision{{ID: "s1", Decision: "accept"}}}
	if err := s.AddReply(token, accept); err == nil {
		t.Error("accepted a reply before the session posted")
	}
	if err := post(t, s, `{"status": "waiting", "suggestions": [{"id": "s1", "action": "keep", "files": []}, {"id": "s2", "action": "keep", "files": [], "status": "applied"}]}`); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Reply{
		{},
		{Decisions: []Decision{{ID: "s2", Decision: "accept"}}},
		{Decisions: []Decision{{ID: "s1", Decision: "maybe"}}},
	} {
		if err := s.AddReply(token, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if err := s.AddReply(token, accept); err != nil {
		t.Fatal(err)
	}
	if err := s.AddReply(token, Reply{Message: "and more"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	if err := s.Watch(ctx, token, &out, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "\n"); got != 2 || !strings.Contains(out.String(), `"id":"s1"`) {
		t.Errorf("first watch printed:\n%s", out.String())
	}

	// A restarted watch only prints new replies, and ends once the session is done.
	if err := s.AddReply(token, Reply{Message: "third"}); err != nil {
		t.Fatal(err)
	}
	if err := post(t, s, `{"status": "done"}`); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := s.Watch(context.Background(), token, &out, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" && !strings.Contains(out.String(), "third") || strings.Count(out.String(), "\n") != 1 {
		t.Errorf("second watch printed:\n%s", out.String())
	}
}

func TestInvalidToken(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, tok := range []string{"../x", "ABC", "aaaabbbbccccdddd"} {
		if err := s.Post(tok, strings.NewReader(`{"status": "done"}`)); err == nil {
			t.Errorf("posted to %q", tok)
		}
	}
}

func TestRemove(t *testing.T) {
	cfg := t.TempDir()
	mem := filepath.Join(cfg, "projects", "-p", "memory")
	for _, f := range []string{"a.md", "MEMORY.md", "notes.txt"} {
		write(t, filepath.Join(mem, f))
	}
	write(t, filepath.Join(cfg, "CLAUDE.md"))

	for _, bad := range []string{
		filepath.Join(mem, "MEMORY.md"),
		filepath.Join(mem, "notes.txt"),
		filepath.Join(cfg, "CLAUDE.md"),
		filepath.Join(mem, "..", "memory", "..", "..", "..", "CLAUDE.md"),
	} {
		if err := Remove(cfg, bad); err == nil {
			t.Errorf("removed %s", bad)
		}
	}
	if err := Remove(cfg, filepath.Join(mem, "a.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mem, "a.md")); !os.IsNotExist(err) {
		t.Error("a.md still exists")
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
