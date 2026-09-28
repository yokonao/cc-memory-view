package audit

import (
	"bytes"
	"context"
	"slices"
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

func update(t *testing.T, s Store, state string) error {
	t.Helper()
	return s.Update(token, strings.NewReader(state))
}

func TestUpdateValidates(t *testing.T) {
	s := newStore(t)
	for _, bad := range []string{
		`{"status": "thinking"}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "action": "burn", "files": []}]}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "action": "keep", "files": ["relative.md"]}]}`,
		`{"status": "waiting", "extra": 1}`,
	} {
		if err := update(t, s, bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if err := update(t, s, `{"status": "waiting", "message": "hi", "suggestions": [{"id": "s1", "action": "delete", "files": ["/a.md"], "reason": "old"}]}`); err != nil {
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

func TestUpdateMerges(t *testing.T) {
	s := newStore(t)
	if err := update(t, s, `{"status": "waiting", "suggestions": [{"id": "s1", "action": "delete", "files": ["/a.md"], "reason": "old"}, {"id": "s2", "action": "keep", "files": []}]}`); err != nil {
		t.Fatal(err)
	}
	for _, up := range []string{
		`{"status": "working", "suggestions": []}`,
		`{"status": "waiting", "suggestions": [{"id": "s1", "status": "applied"}, {"id": "s3", "action": "keep", "files": []}]}`,
	} {
		if err := update(t, s, up); err != nil {
			t.Fatal(err)
		}
	}
	if err := update(t, s, `{"status": "waiting", "suggestions": [{"id": "s4", "status": "open"}]}`); err == nil {
		t.Error("accepted a new suggestion without an action")
	}
	a, err := s.Load(token)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sg := range a.State.Suggestions {
		got = append(got, sg.ID+" "+sg.Action+" "+sg.Reason+" "+sg.Status)
	}
	want := []string{"s1 delete old applied", "s2 keep  ", "s3 keep  "}
	if !slices.Equal(got, want) {
		t.Errorf("suggestions:\ngot  %q\nwant %q", got, want)
	}
}

func TestReplyAndWatch(t *testing.T) {
	s := newStore(t)
	approve := Reply{Decisions: []Decision{{ID: "s1", Decision: "approve"}}}
	if err := s.AddReply(token, approve); err == nil {
		t.Error("accepted a reply before the session posted")
	}
	if err := update(t, s, `{"status": "waiting", "suggestions": [{"id": "s1", "action": "keep", "files": []}, {"id": "s2", "action": "keep", "files": [], "status": "applied"}]}`); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Reply{
		{},
		{Decisions: []Decision{{ID: "s2", Decision: "approve"}}},
		{Decisions: []Decision{{ID: "s1", Decision: "reject"}}},
		{Decisions: []Decision{{ID: "s1", Decision: "comment", Comment: " "}}},
	} {
		if err := s.AddReply(token, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if err := s.AddReply(token, approve); err != nil {
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
	if err := update(t, s, `{"status": "done"}`); err != nil {
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
		if err := s.Update(tok, strings.NewReader(`{"status": "done"}`)); err == nil {
			t.Errorf("updated %q", tok)
		}
	}
}
