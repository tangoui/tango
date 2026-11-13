package fileserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setup(t *testing.T) (root string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "pub")
	for name, body := range map[string]string{
		"pub/a.txt":           "A",
		"pub/sub/b.txt":       "B",
		"pub/site/index.html": "INDEX",
		"pub/.secret":         "S",
		"secret.txt":          "OUTSIDE",
	} {
		p := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(root, "escape.txt")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func get(h http.Handler, method, target string) (int, string) {
	req := httptest.NewRequest(method, "http://x/", nil)
	req.URL.Path = target // bypass client-side cleaning
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, string(b)
}

func TestServe(t *testing.T) {
	s, err := New(setup(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tests := []struct {
		path string
		code int
		body string
	}{
		{"/a.txt", 200, "A"},
		{"/sub/b.txt", 200, "B"},
		{"/site/", 200, "INDEX"},
		{"/sub/", 404, ""},
		{"/.secret", 404, ""},
		{"/missing", 404, ""},
		{"/../secret.txt", 400, ""},
		{"/sub/../../secret.txt", 400, ""},
		{`/..\secret.txt`, 400, ""},
		{"/escape.txt", 0, ""}, // symlink out of the root: must not be served
	}
	for _, tt := range tests {
		code, body := get(s, "GET", tt.path)
		if tt.code == 0 {
			if code == 200 || body == "OUTSIDE" {
				t.Errorf("%s: served %d %q", tt.path, code, body)
			}
			continue
		}
		if code != tt.code || (tt.body != "" && body != tt.body) {
			t.Errorf("%s: got %d %q, want %d %q", tt.path, code, body, tt.code, tt.body)
		}
	}
	if code, _ := get(s, "POST", "/a.txt"); code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", code)
	}
}

func TestOptions(t *testing.T) {
	s, err := New(setup(t), Options{List: true, Dotfiles: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if code, body := get(s, "GET", "/sub/"); code != 200 || body == "" {
		t.Errorf("listing: %d %q", code, body)
	}
	if code, body := get(s, "GET", "/.secret"); code != 200 || body != "S" {
		t.Errorf("dotfile: %d %q", code, body)
	}
}

func TestMount(t *testing.T) {
	s, err := New(setup(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "app "+r.URL.Path) })
	h := Mount("files", s, next)
	if code, body := get(h, "GET", "/files/a.txt"); code != 200 || body != "A" {
		t.Errorf("mounted: %d %q", code, body)
	}
	if code, body := get(h, "GET", "/index.html"); code != 200 || body != "app /index.html" {
		t.Errorf("fallthrough: %d %q", code, body)
	}
	if code, _ := get(h, "GET", "/files/../secret.txt"); code != 400 {
		t.Errorf("mounted traversal: %d", code)
	}
	if code, _ := get(Mount("/f/", s, nil), "GET", "/other"); code != 404 {
		t.Errorf("nil next: %d", code)
	}
}

func TestMissingDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "nope"), Options{}); err == nil {
		t.Fatal("New on a missing dir succeeded")
	}
}
