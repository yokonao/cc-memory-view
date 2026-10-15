package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestData(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	mem := filepath.Join(root, "-p", "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"MEMORY.md": "- [A](a.md)\n",
		"a.md":      "---\nname: a\n---\nSee [[b]] and [[nope]]. <script>x</script>\n",
		"b.md":      "---\nname: b\n---\nb\n",
	} {
		if err := os.WriteFile(filepath.Join(mem, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := httptest.NewRecorder()
	(&Server{Root: root}).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/data", nil))
	var got dataJSON
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Memories) != 2 {
		t.Fatalf("got %d memories, want 2", len(got.Memories))
	}
	a := got.Memories[0]
	if a.ID != "-p/memory/a.md" {
		t.Errorf("id: %q", a.ID)
	}
	for _, want := range []string{`<a href="#-p/memory/b.md">b</a>`, "[[nope]]"} {
		if !strings.Contains(a.HTML, want) {
			t.Errorf("html %q lacks %q", a.HTML, want)
		}
	}
	if strings.Contains(a.HTML, "<script>") {
		t.Errorf("raw HTML not escaped: %q", a.HTML)
	}
}
