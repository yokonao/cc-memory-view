package audit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The web UI and the audit session exchange files under
// <workspace>/audits/<token>/:
//
//	session.json   written by the server when it starts the session
//	state.json     suggestions from the session (audit update)
//	replies.jsonl  replies from the web UI, one per line (audit watch)
//	cursor         how many replies audit watch has delivered

const (
	StatusWorking = "working"
	StatusWaiting = "waiting"
	StatusDone    = "done"
)

var (
	statuses           = []string{StatusWorking, StatusWaiting, StatusDone}
	actions            = []string{"keep", "update", "merge", "promote", "delete"}
	suggestionStatuses = []string{"", "open", "applied", "dismissed"}
	decisions          = []string{"approve", "comment"}
	tokenRe            = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

type Session struct {
	Token   string    `json:"token"`
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
}

type Suggestion struct {
	ID       string   `json:"id"`
	Action   string   `json:"action"`
	Files    []string `json:"files"`
	Reason   string   `json:"reason"`
	Proposed string   `json:"proposed,omitempty"`
	// Status is open (or empty) until the suggestion is applied or dismissed.
	Status string `json:"status,omitempty"`
}

// Open reports whether the suggestion still awaits a decision.
func (s Suggestion) Open() bool { return s.Status == "" || s.Status == "open" }

type State struct {
	Status      string       `json:"status"`
	Message     string       `json:"message"`
	Suggestions []Suggestion `json:"suggestions"`
	Updated     time.Time    `json:"updated"`
}

type Decision struct {
	ID       string `json:"id"`
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

type Reply struct {
	Time      time.Time  `json:"time"`
	Message   string     `json:"message,omitempty"`
	Decisions []Decision `json:"decisions,omitempty"`
}

type Audit struct {
	Session Session `json:"session"`
	// State is nil until the session posts.
	State   *State  `json:"state"`
	Replies []Reply `json:"replies"`
}

// Store keeps audits under a directory, <workspace>/audits.
type Store struct{ Dir string }

func NewToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s Store) dir(token string) (string, error) {
	if !tokenRe.MatchString(token) {
		return "", fmt.Errorf("invalid audit token %q", token)
	}
	return filepath.Join(s.Dir, token), nil
}

func (s Store) file(token, name string) (string, error) {
	dir, err := s.dir(token)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("unknown audit %s", token)
	}
	return filepath.Join(dir, name), nil
}

// Create starts an audit's directory.
func (s Store) Create(token string) error {
	dir, err := s.dir(token)
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func (s Store) SaveSession(sess Session) error {
	path, err := s.file(sess.Token, "session.json")
	if err != nil {
		return err
	}
	return writeJSON(path, sess)
}

// Update merges an update from the session into the stored state: status
// and message replace the old ones, and suggestions are merged by ID, field
// by field, so suggestions left out are kept.
func (s Store) Update(token string, r io.Reader) error {
	path, err := s.file(token, "state.json")
	if err != nil {
		return err
	}
	var up State
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&up); err != nil {
		return fmt.Errorf("invalid update JSON: %w", err)
	}
	a, err := s.Load(token)
	if err != nil {
		return err
	}
	st := State{}
	if a.State != nil {
		st = *a.State
	}
	st.Status, st.Message = up.Status, up.Message
	for _, sg := range up.Suggestions {
		i := slices.IndexFunc(st.Suggestions, func(old Suggestion) bool { return old.ID == sg.ID })
		if i < 0 {
			st.Suggestions = append(st.Suggestions, sg)
			continue
		}
		old := &st.Suggestions[i]
		if sg.Action != "" {
			old.Action = sg.Action
		}
		if sg.Files != nil {
			old.Files = sg.Files
		}
		if sg.Reason != "" {
			old.Reason = sg.Reason
		}
		if sg.Proposed != "" {
			old.Proposed = sg.Proposed
		}
		if sg.Status != "" {
			old.Status = sg.Status
		}
	}
	if err := st.validate(); err != nil {
		return err
	}
	st.Updated = time.Now()
	return writeJSON(path, st)
}

func (st *State) validate() error {
	if !slices.Contains(statuses, st.Status) {
		return fmt.Errorf("status must be one of %v", statuses)
	}
	if st.Suggestions == nil {
		st.Suggestions = []Suggestion{}
	}
	seen := map[string]bool{}
	for _, sg := range st.Suggestions {
		switch {
		case sg.ID == "" || seen[sg.ID]:
			return fmt.Errorf("suggestion IDs must be unique and non-empty: %q", sg.ID)
		case !slices.Contains(actions, sg.Action):
			return fmt.Errorf("suggestion %s: action must be one of %v", sg.ID, actions)
		case !slices.Contains(suggestionStatuses, sg.Status):
			return fmt.Errorf("suggestion %s: status must be open, applied or dismissed", sg.ID)
		}
		for _, f := range sg.Files {
			if !filepath.IsAbs(f) {
				return fmt.Errorf("suggestion %s: file %q must be absolute", sg.ID, f)
			}
		}
		seen[sg.ID] = true
	}
	return nil
}

// AddReply appends a reply from the web UI after checking it against the
// current state.
func (s Store) AddReply(token string, rep Reply) error {
	a, err := s.Load(token)
	if err != nil {
		return err
	}
	if a.State == nil || a.State.Status != StatusWaiting {
		return errors.New("the session isn't waiting for a reply")
	}
	open := map[string]bool{}
	for _, sg := range a.State.Suggestions {
		open[sg.ID] = sg.Open()
	}
	for _, d := range rep.Decisions {
		if !open[d.ID] {
			return fmt.Errorf("no open suggestion %q", d.ID)
		}
		if !slices.Contains(decisions, d.Decision) {
			return fmt.Errorf("decision must be approve or comment")
		}
		if d.Decision == "comment" && strings.TrimSpace(d.Comment) == "" {
			return fmt.Errorf("comment on %s is empty", d.ID)
		}
	}
	if strings.TrimSpace(rep.Message) == "" && len(rep.Decisions) == 0 {
		return errors.New("empty reply")
	}
	rep.Time = time.Now()
	line, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	path, err := s.file(token, "replies.jsonl")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (s Store) Load(token string) (*Audit, error) {
	dir, err := s.dir(token)
	if err != nil {
		return nil, err
	}
	a := &Audit{Session: Session{Token: token}, Replies: []Reply{}}
	if err := readJSON(filepath.Join(dir, "session.json"), &a.Session); err != nil {
		return nil, err
	}
	var st State
	switch err := readJSON(filepath.Join(dir, "state.json"), &st); {
	case err == nil:
		a.State = &st
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	lines, err := readLines(filepath.Join(dir, "replies.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		var r Reply
		if json.Unmarshal(l, &r) == nil {
			a.Replies = append(a.Replies, r)
		}
	}
	return a, nil
}

// List returns the audits whose session started, newest first.
func (s Store) List() ([]*Audit, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var audits []*Audit
	for _, e := range entries {
		if !tokenRe.MatchString(e.Name()) {
			continue
		}
		a, err := s.Load(e.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		audits = append(audits, a)
	}
	sort.Slice(audits, func(i, j int) bool { return audits[i].Session.Started.After(audits[j].Session.Started) })
	return audits, nil
}

// Watch writes each reply not yet delivered to w as one line, polling every
// interval, until the session posts status done or ctx ends. A cursor file
// remembers what was delivered, so a restarted watch neither repeats nor
// misses replies.
func (s Store) Watch(ctx context.Context, token string, w io.Writer, interval time.Duration) error {
	cursorPath, err := s.file(token, "cursor")
	if err != nil {
		return err
	}
	dir := filepath.Dir(cursorPath)
	for {
		lines, err := readLines(filepath.Join(dir, "replies.jsonl"))
		if err != nil {
			return err
		}
		cursor := 0
		if b, err := os.ReadFile(cursorPath); err == nil {
			cursor, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		for ; cursor < len(lines); cursor++ {
			if _, err := fmt.Fprintf(w, "%s\n", lines[cursor]); err != nil {
				return err
			}
			if err := os.WriteFile(cursorPath, []byte(strconv.Itoa(cursor+1)), 0o644); err != nil {
				return err
			}
		}

		var st State
		if readJSON(filepath.Join(dir, "state.json"), &st) == nil && st.Status == StatusDone {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// readLines returns the complete, non-empty lines of a file, or none when it
// doesn't exist.
func readLines(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lines [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) > 0 {
			lines = append(lines, bytes.Clone(sc.Bytes()))
		}
	}
	return lines, sc.Err()
}
