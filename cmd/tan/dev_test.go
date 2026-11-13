package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindURL(t *testing.T) {
	for line, want := range map[string]string{
		"  \x1b[32m➜\x1b[39m  \x1b[1mLocal\x1b[22m:   \x1b[36mhttp://localhost:\x1b[1m5173\x1b[22m/\x1b[39m": "http://localhost:5173/",
		"  ➜  Network: use --host to expose":                               "",
		"ready - started server on 0.0.0.0:3000, url: http://0.0.0.0:3000": "http://localhost:3000",
		"Listening on http://127.0.0.1:8080/app/":                          "http://127.0.0.1:8080/app/",
		"Local: https://[::1]:4443/":                                       "https://[::1]:4443/",
		"see https://vitejs.dev/config for help":                           "",
	} {
		if got := findURL(line); got != want {
			t.Errorf("findURL(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestScanURL(t *testing.T) {
	in := "VITE v7.1.0  ready in 120 ms\n\n  ➜  Local:   http://localhost:5173/\n  ➜  Local:   http://localhost:9999/\nhmr update\n"
	var out bytes.Buffer
	found := make(chan string, 2)
	scanURL(strings.NewReader(in), &out, found)
	if u := <-found; u != "http://localhost:5173/" {
		t.Errorf("url = %q", u)
	}
	if len(found) != 0 {
		t.Error("more than one URL sent")
	}
	if out.String() != in {
		t.Errorf("output not passed through:\n%q", out.String())
	}
}

func TestDevServerURLAndStop(t *testing.T) {
	if testing.Short() {
		t.Skip("starts processes")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	srv, err := startDevServer(dir, fakeDevServer, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case u := <-srv.url:
		if u != "http://localhost:4321/" {
			t.Fatalf("url = %q", u)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no URL")
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	srv.stop()
	if time.Since(start) > 2*time.Second {
		t.Errorf("stop took %v; SIGTERM should end it", time.Since(start))
	}
	select {
	case <-srv.exited:
	default:
		t.Fatal("exited not closed after stop")
	}
	assertGone(t, strings.TrimSpace(string(b)))
}

func TestServeDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	u, closeFn, err := serveDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	res, err := http.Get(u + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "<h1>hi</h1>" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("got %q %v", body, res.Header)
	}
	if _, _, err := serveDir(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir served")
	}
}

func TestWatcher(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main")
	write("frontend/node_modules/x/x.go", "package x")
	w := newWatcher(root, filepath.Join(root, "frontend"))
	if w.changed() {
		t.Fatal("changed with no edits")
	}
	for _, ignored := range []string{"frontend/a.go", "node_modules/y.go", "build/z.go", ".git/h.go", "main_test.go", "README.md"} {
		write(ignored, "x")
		if w.changed() {
			t.Errorf("edit to %s noticed", ignored)
		}
	}
	write("internal/svc/svc.go", "package svc")
	if !w.changed() {
		t.Error("new Go file missed")
	}
	future := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(root, "main.go"), future, future)
	if !w.changed() {
		t.Error("mtime change missed")
	}
	write("go.mod", "module x")
	if !w.changed() {
		t.Error("go.mod missed")
	}
	os.Remove(filepath.Join(root, "internal/svc/svc.go"))
	if !w.changed() {
		t.Error("removal missed")
	}
}

func TestProcessExitStatus(t *testing.T) {
	p, err := startProcess(shellCommand("exit 3"))
	if err != nil {
		t.Fatal(err)
	}
	<-p.done
	if p.err == nil {
		t.Error("exit 3 reported as success")
	}
	p.stop(time.Second) // after exit: must not block
}

func TestDevDir(t *testing.T) {
	cfg := &Config{ID: "com.example.devdir", Name: "Dev Dir"}
	out, err := devBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	want := filepath.Join(cache, "tango", "dev", cfg.ID)
	if got := devDir(out); got != want {
		t.Fatalf("devDir(%s) = %s, want %s", out, got, want)
	}
	os.RemoveAll(want)
}
