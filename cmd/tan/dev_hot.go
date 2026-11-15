package main

// Hot reload of self-drawn UI (docs/specs/ui-hotreload.md). When the app
// links package uihost, tan dev runs the first build twice: once as the
// dev host (TANGO_DEV_ROLE=host), which owns the canvas windows and lives
// across rebuilds, and once as a worker (TANGO_DEV_ROLE=worker), the app
// itself, drawing into the host's windows. Each rebuild starts a new
// worker; the host swaps it in once it has drawn its first frame and the
// old worker exits. The host is restarted only when framework code (or
// go.mod) changes, or when it crashes.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/devhost"
)

// errNotHot means the app does not link the dev host: use plain restarts.
var errNotHot = errors.New("the app does not link uihost")

// hasDevHost reports whether the binary at path links the dev host.
func hasDevHost(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte(devhost.Marker))
}

// attachSymbol is devhost.AttachSymbol as it appears in a binary's function
// table (NUL-terminated there).
const attachSymbol = devhost.AttachSymbol + "\x00"

// usesAttach reports whether the binary at path links uihost.Attach(With).
func usesAttach(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte(attachSymbol))
}

// hotCapable reports whether the app built at path can run under the dev
// host, logging why not when it links uihost but cannot.
func hotCapable(path string) bool {
	if !hasDevHost(path) {
		return false
	}
	if usesAttach(path) {
		logf("The app uses uihost.Attach, which hot reload cannot show; restarting the whole app on changes instead")
		return false
	}
	return true
}

// hotSession is a dev session in hot-reload mode.
type hotSession struct {
	*devSession
	watchFramework bool

	tmp   string // socket and frame buffers (a 0700 directory)
	addr  string
	token string // session secret every peer of the host must present

	host    *process
	ctl     *devhost.Conn
	events  chan any      // from the host
	ctlDone chan struct{} // closed when ctl ends

	workers map[*workerProc]bool
	active  *workerProc
	pending *workerProc
	exited  chan *workerProc
	built   string // last good worker binary
	slot    int

	fw       *fileSet // framework sources the host links
	mods     *fileSet // go.mod, go.sum, go.work
	changeAt time.Time
	buildDur time.Duration
}

// workerProc is a running worker.
type workerProc struct {
	*process
	path    string
	pid     int
	start   time.Time
	retired bool // replaced or being shut down: its exit means nothing
	stderr  *tailBuffer
}

func (s *devSession) hotLoop(ctx context.Context, watchFramework bool) error {
	h := &hotSession{devSession: s, watchFramework: watchFramework,
		workers: map[*workerProc]bool{}, exited: make(chan *workerProc, 8)}
	token, err := devhost.NewToken()
	if err != nil {
		return err
	}
	h.token = token
	tmp, err := os.MkdirTemp("", "tango-dev-")
	if err != nil {
		return err
	}
	h.tmp = tmp
	defer os.RemoveAll(tmp)
	defer h.stopAll()
	return h.loop(ctx)
}

func (h *hotSession) workerPath(i int) string {
	exe := h.cfg.Exe()
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	base := filepath.Dir(h.out) // devBinary's per-app directory
	if runtime.GOOS == "darwin" {
		base = filepath.Dir(h.app())
	}
	return filepath.Join(base, "workers", fmt.Sprintf("w%d", i), exe)
}

// buildWorker builds into the worker slot not used by the running worker.
// It returns the compiler output on failure.
func (h *hotSession) buildWorker(ctx context.Context) (string, string, error) {
	i := h.slot
	if h.active != nil && h.active.path == h.workerPath(i) {
		i ^= 1
	}
	path := h.workerPath(i)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", buildTags(true), "-o", path, ".")
	cmd.Dir = h.cfg.dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	start := time.Now()
	err := cmd.Run()
	h.buildDur = time.Since(start)
	if err != nil {
		return "", strings.TrimSpace(out.String()), err
	}
	h.slot = i ^ 1
	return path, "", nil
}

func (h *hotSession) loop(ctx context.Context) error {
	w := newWatcher(h.cfg.dir, h.cfg.FrontendDir())
	h.mods = newFileSet(goModFiles(h.cfg.dir))
	// First build: decides the mode.
	for {
		logf("Building")
		h.changeAt = time.Now()
		path, _, err := h.buildWorker(ctx)
		if err == nil {
			if !hotCapable(path) {
				return errNotHot
			}
			if err := h.startAll(ctx, path, nil); err != nil {
				return err
			}
			break
		}
		if ctx.Err() != nil {
			return nil
		}
		warnf("Build failed; waiting for changes")
		if !waitChange(ctx, w) {
			return nil
		}
	}

	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		var hostDone <-chan struct{}
		if h.host != nil {
			hostDone = h.host.done
		}
		select {
		case <-ctx.Done():
			return nil
		case <-hostDone:
			if h.host.err == nil {
				logf("The app quit")
				return nil
			}
			warnf("The dev host exited: %v; restarting it", h.host.err)
			if err := h.startAll(ctx, h.built, nil); err != nil {
				return err
			}
		case <-h.ctlDone:
			// The host is going away; its exit is handled above.
			h.ctlDone = nil
		case ev := <-h.events:
			h.event(ev)
		case wp := <-h.exited:
			if done := h.workerExited(wp); done {
				return nil
			}
		case <-tick.C:
			if !w.changed() {
				continue
			}
			h.changeAt = time.Now()
			time.Sleep(150 * time.Millisecond) // let editors finish writing
			w.changed()
			if err := h.rebuild(ctx); err != nil {
				return err
			}
		}
	}
}

func waitChange(ctx context.Context, w *watcher) bool {
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
			if w.changed() {
				time.Sleep(150 * time.Millisecond)
				w.changed()
				return true
			}
		}
	}
}

// rebuild builds after a change and starts the new worker, or reports the
// error on the host's windows.
func (h *hotSession) rebuild(ctx context.Context) error {
	logf("Building")
	path, out, err := h.buildWorker(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		warnf("Build failed; the running app stays on screen")
		h.status("build", out)
		return nil
	}
	if !hasDevHost(path) {
		warnf("The app no longer links uihost; restarting without hot reload")
		h.stopAll()
		return errNotHot
	}
	if !hotCapable(path) {
		h.stopAll()
		return errNotHot
	}
	if h.mods.changed() || (h.fw != nil && h.fw.changed()) {
		logf("Framework or module files changed; restarting the dev host")
		state := h.saveState()
		if err := h.startAll(ctx, path, state); err != nil {
			return err
		}
		return nil
	}
	if p := h.pending; p != nil {
		h.retire(p)
	}
	return h.startWorker(path)
}

// saveState fetches the running app's Persist state through the host, so
// it survives a host restart.
func (h *hotSession) saveState() []devhost.Entry {
	if h.ctl == nil || h.active == nil {
		return nil
	}
	const req = 1
	if err := h.ctl.Send(devhost.SnapshotGet{Req: req}); err != nil {
		return nil
	}
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-h.events:
			if !ok {
				return nil
			}
			if d, isData := ev.(devhost.SnapshotData); isData && d.Req == req {
				return d.Entries
			}
			h.event(ev)
		case <-h.ctlDone:
			return nil
		case <-timeout:
			warnf("The app did not hand over its state in time")
			return nil
		}
	}
}

// startAll (re)starts the host and a first worker from the binary at path.
// state, if any, seeds the new host's Persist state.
func (h *hotSession) startAll(ctx context.Context, path string, state []devhost.Entry) error {
	h.stopAll()
	h.built = path
	if err := h.startHost(ctx, path); err != nil {
		return err
	}
	if len(state) > 0 {
		_ = h.ctl.Send(devhost.SnapshotData{Entries: state})
	}
	if h.watchFramework {
		h.fw = newFileSet(frameworkFiles(ctx, h.cfg.dir))
	}
	h.mods = newFileSet(goModFiles(h.cfg.dir))
	return h.startWorker(path)
}

// startHost installs path as the host executable (the macOS bundle at
// h.out; workers never overwrite it while it runs) and starts it.
func (h *hotSession) startHost(ctx context.Context, path string) error {
	if err := installBinary(path, h.out); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		if !h.bundled {
			h.bundled = true
			if err := writeBundle(h.cfg, h.app()); err != nil {
				return err
			}
		}
		if err := adhocSign(ctx, h.app()); err != nil {
			return err
		}
	}
	addr, err := h.newAddr()
	if err != nil {
		return err
	}
	h.addr = addr
	cmd := exec.Command(h.out)
	cmd.Dir = h.cfg.dir
	cmd.Env = append(os.Environ(), h.env...)
	cmd.Env = append(cmd.Env,
		devhost.EnvRole+"="+devhost.RoleHost, devhost.EnvAddr+"="+addr,
		devhost.EnvName+"="+h.cfg.Name, devhost.EnvID+"="+h.cfg.ID, devhost.EnvDir+"="+h.tmp,
		devhost.EnvToken+"="+h.token)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	p, err := startProcess(cmd)
	if err != nil {
		return fmt.Errorf("starting the dev host: %w", err)
	}
	h.host = p
	c, err := devhost.Dial(addr, 15*time.Second)
	if err == nil {
		_, err = devhost.HandshakeToken(c, devhost.RoleCtl, h.token)
	}
	if err != nil {
		p.stop(time.Second)
		h.host = nil
		return fmt.Errorf("connecting to the dev host: %w", err)
	}
	h.ctl = c
	events := make(chan any, 16)
	done := make(chan struct{})
	h.events, h.ctlDone = events, done
	go func() {
		defer close(done)
		for {
			m, err := c.Recv()
			if err != nil {
				return
			}
			events <- m
		}
	}()
	logf("Dev host running (pid %d); hot reload is on", cmd.Process.Pid)
	return nil
}

func (h *hotSession) newAddr() (string, error) {
	if runtime.GOOS == "windows" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		a := ln.Addr().String()
		ln.Close()
		return "tcp:" + a, nil
	}
	return "unix:" + filepath.Join(h.tmp, "host.sock"), nil
}

func (h *hotSession) startWorker(path string) error {
	cmd := exec.Command(path, h.args...)
	cmd.Dir = h.cfg.dir
	cmd.Env = append(os.Environ(), h.env...)
	cmd.Env = append(cmd.Env, devhost.EnvRole+"="+devhost.RoleWorker, devhost.EnvAddr+"="+h.addr, devhost.EnvDir+"="+h.tmp,
		devhost.EnvToken+"="+h.token)
	tail := &tailBuffer{max: 4 << 10}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, io.MultiWriter(os.Stderr, tail)
	p, err := startProcess(cmd)
	if err != nil {
		warnf("Starting the app: %v", err)
		return nil
	}
	wp := &workerProc{process: p, path: path, pid: cmd.Process.Pid, start: time.Now(), stderr: tail}
	h.workers[wp] = true
	h.pending = wp
	h.built = path
	go func() {
		<-p.done
		h.exited <- wp
	}()
	return nil
}

// event handles a message from the host.
func (h *hotSession) event(ev any) {
	switch ev := ev.(type) {
	case devhost.Swapped:
		var nw *workerProc
		for wp := range h.workers {
			if wp.pid == ev.New {
				nw = wp
			}
		}
		if nw == nil {
			return
		}
		old := h.active
		h.active = nw
		if h.pending == nw {
			h.pending = nil
		}
		if old != nil && old != nw {
			h.retire(old)
		}
		h.status("", "")
		total := time.Since(h.changeAt)
		launch := max(time.Since(nw.start)-ev.Took, 0)
		logf("%s on screen (pid %d): %v after the change (build %v, process launch %v, connect to first frame %v)",
			h.cfg.Name, nw.pid, total.Round(time.Millisecond), h.buildDur.Round(time.Millisecond),
			launch.Round(time.Millisecond), ev.Took.Round(time.Millisecond))
	case devhost.WorkerLeft:
	}
}

// workerExited handles a worker's exit; it reports whether the session ends.
func (h *hotSession) workerExited(wp *workerProc) bool {
	delete(h.workers, wp)
	if wp.retired {
		return false
	}
	var ee *exec.ExitError
	if errors.As(wp.err, &ee) && ee.ExitCode() == 3 {
		return false // lost the host; the host's exit is handled separately
	}
	switch wp {
	case h.active:
		h.active = nil
		if wp.err == nil {
			logf("The app quit")
			return true
		}
		warnf("The app exited: %v; waiting for changes", wp.err)
		h.status("crash", fmt.Sprintf("%v\n%s", wp.err, wp.stderr.Tail(12)))
	case h.pending:
		h.pending = nil
		warnf("The new build exited before showing a frame: %v; the previous one stays on screen", wp.err)
		h.status("start", fmt.Sprintf("%v\n%s", wp.err, wp.stderr.Tail(12)))
	}
	return false
}

func (h *hotSession) retire(wp *workerProc) {
	if wp.retired {
		return
	}
	wp.retired = true
	if h.pending == wp {
		h.pending = nil
	}
	if h.active == wp {
		h.active = nil
	}
	go wp.stop(2 * time.Second)
}

func (h *hotSession) status(kind, text string) {
	if h.ctl != nil {
		_ = h.ctl.Send(devhost.Status{Kind: kind, Text: text})
	}
}

// stopAll stops the workers and the host.
func (h *hotSession) stopAll() {
	var wg sync.WaitGroup
	for wp := range h.workers {
		wp.retired = true
		wg.Add(1)
		go func() { defer wg.Done(); wp.stop(2 * time.Second) }()
	}
	h.active, h.pending = nil, nil
	if h.ctl != nil {
		h.ctl.Close()
		h.ctl = nil
	}
	if h.host != nil {
		h.host.stop(3 * time.Second)
		h.host = nil
	}
	wg.Wait()
	h.ctlDone, h.events = nil, nil
}

// ---- helpers ----

func installBinary(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst) // a new inode: never rewrite a running binary
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = append([]byte(nil), t.b[len(t.b)-t.max:]...)
	}
	t.mu.Unlock()
	return len(p), nil
}

// Tail returns the last n lines.
func (t *tailBuffer) Tail(n int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimRight(string(t.b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// fileSet fingerprints a list of files or directories (a directory's
// non-test .go files, not recursive).
type fileSet struct {
	paths []string
	last  string
}

func newFileSet(paths []string) *fileSet {
	f := &fileSet{paths: paths}
	f.last = f.snapshot()
	return f
}

func (f *fileSet) snapshot() string {
	var b strings.Builder
	stat := func(p string, info fs.FileInfo) {
		fmt.Fprintf(&b, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
	}
	for _, p := range f.paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(&b, "%s|-\n", p)
			continue
		}
		if !info.IsDir() {
			stat(p, info)
			continue
		}
		ents, _ := os.ReadDir(p)
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			if fi, err := e.Info(); err == nil {
				stat(filepath.Join(p, n), fi)
			}
		}
	}
	return b.String()
}

func (f *fileSet) changed() bool {
	s := f.snapshot()
	if s == f.last {
		return false
	}
	f.last = s
	return true
}

func goModFiles(dir string) []string {
	return []string{filepath.Join(dir, "go.mod"), filepath.Join(dir, "go.sum"), filepath.Join(dir, "go.work")}
}

// frameworkFiles lists the package directories of the Tango module that
// the dev host links, when that module is a local checkout (a replace or
// workspace) rather than a read-only module cache copy.
func frameworkFiles(ctx context.Context, dir string) []string {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps",
		"-f", "{{if .Module}}{{.Module.Path}}\t{{.Dir}}{{end}}", "github.com/tangoui/tango/uihost")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	cache := os.Getenv("GOMODCACHE")
	if cache == "" {
		if b, err := exec.Command("go", "env", "GOMODCACHE").Output(); err == nil {
			cache = strings.TrimSpace(string(b))
		}
	}
	var dirs []string
	for _, line := range strings.Split(string(out), "\n") {
		mod, d, ok := strings.Cut(line, "\t")
		if !ok || mod != "github.com/tangoui/tango" || d == "" {
			continue
		}
		if cache != "" && strings.HasPrefix(d, cache) {
			return nil // immutable
		}
		dirs = append(dirs, d)
	}
	return dirs
}
