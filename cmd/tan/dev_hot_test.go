package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/devhost"
)

func TestBuildTags(t *testing.T) {
	for goflags, want := range map[string][2]string{ // release, dev
		"":                              {"tango_release", "tango_dev"},
		"-mod=mod":                      {"tango_release", "tango_dev"},
		"-tags=foo,bar -mod=mod":        {"foo,bar,tango_release", "foo,bar,tango_dev"},
		"-tags=a -tags=b,tango_release": {"b,tango_release", "b,tango_dev"},
		"--tags=x,x":                    {"x,tango_release", "x,tango_dev"},
		// tan build is always a production build.
		"-tags=tango_dev,y": {"y,tango_release", "y,tango_dev"},
	} {
		t.Setenv("GOFLAGS", goflags)
		if got := buildTags(false); got != want[0] {
			t.Errorf("GOFLAGS=%q: release tags %q, want %q", goflags, got, want[0])
		}
		if got := buildTags(true); got != want[1] {
			t.Errorf("GOFLAGS=%q: dev tags %q, want %q", goflags, got, want[1])
		}
	}
}

func TestHasDevHost(t *testing.T) {
	dir := t.TempDir()
	yes, no := filepath.Join(dir, "yes"), filepath.Join(dir, "no")
	os.WriteFile(yes, []byte("\x00\x01ELF..."+devhost.Marker+"..."), 0o755)
	os.WriteFile(no, []byte("\x00\x01ELF... nothing here"), 0o755)
	if !hasDevHost(yes) || hasDevHost(no) || hasDevHost(filepath.Join(dir, "missing")) {
		t.Fatal("marker detection wrong")
	}
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{max: 32}
	for i := range 10 {
		b.Write([]byte(strings.Repeat("x", i) + "\n"))
	}
	b.Write([]byte("panic: boom\n"))
	if got := b.Tail(2); got != "xxxxxxxxx\npanic: boom" {
		t.Fatalf("tail %q", got)
	}
	if len(b.b) > 32 {
		t.Fatalf("kept %d bytes", len(b.b))
	}
}

func TestFileSet(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "pkg")
	os.Mkdir(pkg, 0o755)
	mod := filepath.Join(dir, "go.mod")
	os.WriteFile(mod, []byte("module x\n"), 0o644)
	os.WriteFile(filepath.Join(pkg, "a.go"), []byte("package pkg\n"), 0o644)
	fs := newFileSet([]string{pkg, mod, filepath.Join(dir, "go.work")})
	if fs.changed() {
		t.Fatal("changed without changes")
	}
	// Tests and subdirectories do not count.
	os.WriteFile(filepath.Join(pkg, "a_test.go"), []byte("package pkg\n"), 0o644)
	os.MkdirAll(filepath.Join(pkg, "sub"), 0o755)
	os.WriteFile(filepath.Join(pkg, "sub", "b.go"), []byte("package sub\n"), 0o644)
	if fs.changed() {
		t.Fatal("test file or subpackage counted")
	}
	os.WriteFile(filepath.Join(pkg, "a.go"), []byte("package pkg // changed\n"), 0o644)
	if !fs.changed() || fs.changed() {
		t.Fatal("source change not seen once")
	}
	os.WriteFile(filepath.Join(dir, "go.work"), []byte("go 1.27\n"), 0o644)
	if !fs.changed() {
		t.Fatal("new go.work not seen")
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(mod, later, later)
	if !fs.changed() {
		t.Fatal("go.mod touch not seen")
	}
}

func TestWorkerPathsAlternate(t *testing.T) {
	cfg := &Config{Name: "Hello App", ID: "com.example.hello"}
	s := &devSession{cfg: cfg, out: filepath.Join(t.TempDir(), "Hello App.app", "Contents", "MacOS", cfg.Exe())}
	h := &hotSession{devSession: s}
	a, b := h.workerPath(0), h.workerPath(1)
	if a == b || filepath.Base(a) != filepath.Base(b) || !strings.Contains(a, "workers") {
		t.Fatalf("paths %q %q", a, b)
	}
	// The running worker's binary is never the build output.
	h.active = &workerProc{path: a}
	h.slot = 0
	i := h.slot
	if h.active != nil && h.active.path == h.workerPath(i) {
		i ^= 1
	}
	if h.workerPath(i) != b {
		t.Fatal("would overwrite the running worker")
	}
	if !slices.Contains(goModFiles("/p"), filepath.Join("/p", "go.sum")) {
		t.Fatal("go.sum not watched")
	}
}

func TestUsesAttach(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("\x00ELF..."+devhost.Marker+body), 0o755)
		return p
	}
	attach := write("attach", "\x00github.com/tangoui/tango/uihost.AttachWith\x00runtime.main\x00")
	// Open-only apps name the Attached type and other uihost functions,
	// never AttachWith itself.
	open := write("open", "\x00github.com/tangoui/tango/uihost.Open\x00*uihost.Attached\x00github.com/tangoui/tango/uihost.AttachWithout\x00")
	if !usesAttach(attach) || usesAttach(open) || usesAttach(filepath.Join(dir, "missing")) {
		t.Fatal("Attach detection wrong")
	}
	if hotCapable(attach) || !hotCapable(open) {
		t.Fatal("an app using Attach must run in restart mode, an Open-only app hot")
	}
}
