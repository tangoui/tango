package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newResolveTestServer(t *testing.T) *mcpServer {
	t.Helper()
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	return s
}

func TestResolveRootInside(t *testing.T) {
	s := newResolveTestServer(t)
	app := filepath.Join(s.root, "app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ in, want string }{
		{"", s.root},
		{".", s.root},
		{"app", app},
		{"app/../app", app},
		{app, app},
		{s.root, s.root},
	}
	for _, c := range cases {
		got, err := s.resolve(c.in)
		if err != nil {
			t.Errorf("resolve(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolve(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveRootOutside(t *testing.T) {
	s := newResolveTestServer(t)
	for _, in := range []string{"..", "/etc", "../x"} {
		got, err := s.resolve(in)
		if err == nil {
			t.Errorf("resolve(%q) = %q, want error", in, got)
			continue
		}
		if !strings.Contains(err.Error(), "outside the MCP root") {
			t.Errorf("resolve(%q) error = %v", in, err)
		}
	}
}

func TestResolveRootSymlinkEscape(t *testing.T) {
	s := newResolveTestServer(t)
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(s.root, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := s.resolve("link/x")
	if err == nil {
		t.Fatalf("resolve(link/x) = %q, want error", got)
	}
	if !strings.Contains(err.Error(), filepath.Join(other, "x")) {
		t.Errorf("error %q does not name the real path %s", err, filepath.Join(other, "x"))
	}
}

func TestResolveRootNotExistYet(t *testing.T) {
	s := newResolveTestServer(t)
	got, err := s.resolve("new/sub")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(s.root, "new", "sub"); got != want {
		t.Errorf("resolve(new/sub) = %q, want %q", got, want)
	}
}

func TestResolveRootDanglingSymlink(t *testing.T) {
	s := newResolveTestServer(t)
	target := filepath.Join(t.TempDir(), "missing")
	if err := os.Symlink(target, filepath.Join(s.root, "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{"dangling", "dangling/x"} {
		if got, err := s.resolve(in); err == nil {
			t.Errorf("resolve(%q) = %q, want error", in, got)
		}
	}
}
