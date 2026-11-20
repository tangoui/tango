package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/devhost"
)

func TestDevHotSessionFallsBackWithFreshBuild(t *testing.T) {
	d := newDevTest(t, `
if [ "$version" = hot ]; then
    /bin/mkdir -p "${out%/*}"
    /bin/cat > "$out" <<'APP'
#!/bin/sh
# `+devhost.Marker+`
printf '%s|%s\n' "$TANGO_DEV_ROLE" "$0" >> "$TAN_TEST_HOT_RUNS"
if [ "$TANGO_DEV_ROLE" = host ]; then
    exec "$TAN_TEST_SELF" -test.run '^TestDevHotHostHelper$'
fi
exec /bin/sleep 30
APP
    /bin/chmod 755 "$out"
    exit 0
fi`)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hotRuns := filepath.Join(t.TempDir(), "hot-runs")
	t.Setenv("TAN_TEST_SELF", self)
	t.Setenv("TAN_TEST_HOT_RUNS", hotRuns)
	writeDevTestFile(t, d.cfg.Path("main.go"), "// hot\npackage main\nfunc main() {}\n", 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := d.start(ctx)
	defer func() { cancel(); <-done }()
	waitDevTestLines(t, ctx, hotRuns, 2)
	d.checkBuilds(t, "probe")
	worker := devTestLines(t, d.builds)[0]
	if got, want := devTestLines(t, hotRuns), []string{"host|" + d.out, "worker|" + worker}; !slices.Equal(got, want) {
		t.Fatalf("hot executions = %q, want %q", got, want)
	}
	writeDevTestFile(t, d.cfg.Path("main.go"), "// plain\npackage main\nfunc main() {}\n", 0o644)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
	d.checkBuilds(t, "probe", "probe", "plain")
	d.checkRuns(t, "plain", "")
}

// TestDevHotHostHelper is run by the hot fixture as a windowless host. It
// acknowledges the controller handshake and holds the connection open.
func TestDevHotHostHelper(t *testing.T) {
	if os.Getenv(devhost.EnvRole) != devhost.RoleHost || os.Getenv("TAN_TEST_HOT_RUNS") == "" {
		return
	}
	l, err := devhost.Listen(os.Getenv(devhost.EnvAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	raw, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	c := devhost.NewConn(raw)
	defer c.Close()
	m, err := c.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if hello, ok := m.(devhost.Hello); !ok || hello.Role != devhost.RoleCtl || hello.Token != os.Getenv(devhost.EnvToken) {
		t.Fatalf("unexpected handshake: %T", m)
	}
	if err := c.Send(devhost.Welcome{}); err != nil {
		t.Fatal(err)
	}
	_, _ = c.Recv()
}

func TestInstallBinaryCleansUpOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	old := filepath.Join(dst, "old")
	writeDevTestFile(t, src, "new", 0o755)
	writeDevTestFile(t, old, "old", 0o644)
	if err := installBinary(context.Background(), src, dst); err == nil {
		t.Fatal("replaced a nonempty directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "dst" || entries[1].Name() != "src" {
		t.Fatalf("temporary files remain after failed install: %v", entries)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old" {
		t.Fatalf("old destination changed: %q, %v", data, err)
	}
}

func TestInstallBinaryCancellationKeepsOldTarget(t *testing.T) {
	for _, stage := range []string{"during copy", "before rename"} {
		t.Run(stage, func(t *testing.T) {
			if stage == "before rename" && runtime.GOOS == "windows" {
				t.Skip("uses the executable mode to observe the pre-rename stage")
			}
			dir := t.TempDir()
			src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
			data := bytes.Repeat([]byte("new"), 128<<10)
			writeDevTestFile(t, src, string(data), 0o755)
			writeDevTestFile(t, dst, "old", 0o755)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Cancel when the first bytes have been staged, or after the copy
			// and chmod are both complete. No polling or timing is involved.
			ctx := observedInstallContext{Context: parent, observe: func() {
				paths, err := filepath.Glob(filepath.Join(dir, ".tango-bin-*"))
				if err != nil || len(paths) != 1 {
					return
				}
				info, err := os.Stat(paths[0])
				if err != nil {
					return
				}
				size := info.Size()
				if stage == "during copy" && size > 0 && size < int64(len(data)) ||
					stage == "before rename" && size == int64(len(data)) && info.Mode()&0o100 != 0 {
					cancel()
				}
			}}
			if err := installBinary(ctx, src, dst); !errors.Is(err, context.Canceled) {
				t.Errorf("install = %v, want context.Canceled", err)
			}
			if got, err := os.ReadFile(dst); err != nil || !bytes.Equal(got, []byte("old")) {
				t.Errorf("old target was replaced, target length %d (%v)", len(got), err)
			}
			if paths, err := filepath.Glob(filepath.Join(dir, ".tango-bin-*")); err != nil || len(paths) != 0 {
				t.Errorf("temporary files remain: %v (%v)", paths, err)
			}
		})
	}
}

type observedInstallContext struct {
	context.Context
	observe func()
}

func (c observedInstallContext) Err() error {
	c.observe()
	return c.Context.Err()
}

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
	b := newTailBuffer(32)
	for i := range 10 {
		b.Write([]byte(strings.Repeat("x", i) + "\n"))
	}
	b.Write([]byte("panic: boom\n"))
	if got := b.lastLines(2); got != "xxxxxxxxx\npanic: boom" {
		t.Fatalf("tail %q", got)
	}
	if got, _ := b.Tail(0); len(got) > 32 {
		t.Fatalf("kept %d bytes", len(got))
	}
	// Trailing blank lines do not take the place of output.
	c := newTailBuffer(64)
	c.Write([]byte("one\ntwo\nthree\n\n\n\n"))
	if got := c.lastLines(2); got != "two\nthree" {
		t.Fatalf("tail with blank lines %q", got)
	}
	if got := c.lastLines(5); got != "one\ntwo\nthree" {
		t.Fatalf("all lines %q", got)
	}
	if got := c.lastLines(1); got != "three" {
		t.Fatalf("one line %q", got)
	}
	if got := newTailBuffer(8).lastLines(3); got != "" {
		t.Fatalf("empty %q", got)
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
