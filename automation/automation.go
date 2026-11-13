// Package automation drives a running Tango application from Go tests,
// like Playwright drives a browser: it starts the program (a development
// build: -tags tango_dev, which Launch adds when it builds a package) with
// the automation control channel turned on, finds windows,
// locates nodes of self-drawn windows through their semantics tree,
// clicks, types, waits, evaluates JavaScript in WebView windows, takes
// screenshots and quits the app at the end of the test.
//
//	func TestAdd(t *testing.T) {
//		app := automation.Launch(t, ".", "-data", filepath.Join(t.TempDir(), "ledger.json"))
//		win := app.Window("记账本")
//		win.Find(a11y.RoleButton, "记一笔").Click()
//		win.Find(a11y.RoleSpinButton, "金额").SetValue("88.8")
//		win.Find(a11y.RoleButton, "保存").Click()
//		win.FindContains(a11y.RoleRow, "打车回家").Wait()
//		win.SaveScreenshot(filepath.Join(t.TempDir(), "ledger.png"))
//	}
//
// Launch skips the test unless TANGO_E2E is set (and with -short), so
// `go test ./...` never opens windows. See docs/automation.md for the
// security model (development builds only, loopback, token), hot reload
// (unsupported) and CI.
//
// Every method fails the test (t.Fatal) when the app reports an error or a
// wait times out; call them from the test goroutine.
package automation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
)

// WindowInfo describes an open window of the app.
type WindowInfo = wire.WindowInfo

// Query selects nodes of a self-drawn window's semantics tree; see
// Window.Locate.
type Query = wire.Query

// Error is an error reported by the app (Code is one of the Code*
// constants of the protocol: "timeout", "hot_reload", ...).
type Error = wire.Error

// Window kinds.
const (
	KindCanvas  = wire.KindCanvas  // self-drawn (package ui)
	KindWebView = wire.KindWebView // WebView page
)

// Name matching of Query.Match.
const (
	MatchExact    = wire.MatchExact
	MatchContains = wire.MatchContains
	MatchRegexp   = wire.MatchRegexp
)

// DefaultTimeout is how long waits (Locator, Window, Eval) last unless
// Options.Timeout or SetTimeout says otherwise.
const DefaultTimeout = 10 * time.Second

// EnvE2E must be set for Launch to run (it skips the test otherwise).
const EnvE2E = "TANGO_E2E"

// Options configure Launch.
type Options struct {
	// Args are the program's arguments.
	Args []string
	// Env adds KEY=VALUE entries to the inherited environment.
	Env []string
	// Dir is the working directory of the build and the program ("" the
	// test's).
	Dir string
	// BuildFlags are extra `go build` flags (e.g. "-tags", "mytag") when
	// the target is a package. Launch always adds the tango_dev tag:
	// production builds (the default) have no control channel.
	BuildFlags []string
	// Timeout is the default for waits; 0 means DefaultTimeout.
	Timeout time.Duration
	// StartTimeout bounds the wait for the program to open its control
	// channel after it started (the build is not counted); 0 means 30s.
	StartTimeout time.Duration
	// Addr is the TANGO_AUTOMATION value; "" means "127.0.0.1:0" (a free
	// loopback port).
	Addr string
	// Output, when set, also receives the program's stdout and stderr
	// (they are always kept and logged when the test fails). Setting
	// TANGO_E2E_VERBOSE has the same effect with os.Stderr.
	Output io.Writer
}

// App is a running application under test.
type App struct {
	t       testing.TB
	c       *client
	timeout time.Duration

	cmd    *exec.Cmd
	out    *tailBuffer
	exited chan struct{}
	runErr error
}

// Launch builds and starts target with args and connects to it. target is
// a Go package (".", "./examples/ledger", an import path), built with `go
// build -tags tango_dev` into a temporary directory, or the path of an
// executable, which must have been built with -tags tango_dev. The
// test is skipped unless TANGO_E2E is set, and with -short. The program is
// asked to quit (then killed) in t.Cleanup.
func Launch(t testing.TB, target string, args ...string) *App {
	t.Helper()
	return LaunchOptions(t, target, Options{Args: args})
}

// LaunchOptions is Launch with options.
func LaunchOptions(t testing.TB, target string, opts Options) *App {
	t.Helper()
	if os.Getenv(EnvE2E) == "" {
		t.Skipf("whole-app test: set %s=1 to run it (it opens windows)", EnvE2E)
	}
	if testing.Short() {
		t.Skip("whole-app test skipped with -short")
	}
	dir := t.TempDir()
	bin, err := resolveTarget(target, opts, dir)
	if err != nil {
		t.Fatalf("automation: %v", err)
	}
	tokenFile := filepath.Join(dir, "automation.json")
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	cmd := exec.Command(bin, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Env = append(filterEnv(os.Environ()), opts.Env...)
	cmd.Env = append(cmd.Env, wire.EnvAddr+"="+addr, wire.EnvTokenFile+"="+tokenFile)
	a := &App{t: t, timeout: opts.Timeout, cmd: cmd, out: &tailBuffer{max: 256 << 10}, exited: make(chan struct{})}
	if a.timeout <= 0 {
		a.timeout = DefaultTimeout
	}
	var w io.Writer = a.out
	if opts.Output != nil {
		w = io.MultiWriter(a.out, opts.Output)
	} else if os.Getenv("TANGO_E2E_VERBOSE") != "" {
		w = io.MultiWriter(a.out, os.Stderr)
	}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		t.Fatalf("automation: start %s: %v", bin, err)
	}
	go func() {
		a.runErr = cmd.Wait()
		close(a.exited)
	}()
	t.Cleanup(a.cleanup)

	start := opts.StartTimeout
	if start <= 0 {
		start = 30 * time.Second
	}
	tf, err := a.waitTokenFile(tokenFile, start)
	if err != nil {
		t.Fatalf("automation: %v\n%s", err, a.outputTail())
	}
	c, err := dial(tf.Addr, tf.Token)
	if err != nil {
		t.Fatalf("automation: connect to %s: %v\n%s", target, err, a.outputTail())
	}
	a.c = c
	return a
}

// Connect attaches to an app that is already running with automation on
// (addr and token as the app wrote them to TANGO_AUTOMATION_TOKEN_FILE or
// printed them). The app is not quit at the end of the test; the
// connection is closed.
func Connect(t testing.TB, addr, token string) *App {
	t.Helper()
	c, err := dial(addr, token)
	if err != nil {
		t.Fatalf("automation: connect to %s: %v", addr, err)
	}
	a := &App{t: t, c: c, timeout: DefaultTimeout}
	t.Cleanup(func() { c.close() })
	return a
}

// SetTimeout changes the default wait of later calls (Locators created
// before keep theirs unless they had none).
func (a *App) SetTimeout(d time.Duration) {
	if d > 0 {
		a.timeout = d
	}
}

// Timeout returns the default wait.
func (a *App) Timeout() time.Duration { return a.timeout }

// Output returns what the program printed so far (the last 256 KiB);
// empty for Connect.
func (a *App) Output() string {
	if a.out == nil {
		return ""
	}
	return a.out.String()
}

// Windows lists the open windows.
func (a *App) Windows() []WindowInfo {
	a.t.Helper()
	r := a.call(&wire.Request{Op: wire.OpWindows}, "list windows")
	return r.Windows
}

// Window waits for a window titled title and returns it.
func (a *App) Window(title string) *Window {
	a.t.Helper()
	return a.WindowMatching(fmt.Sprintf("titled %q", title), func(w WindowInfo) bool { return w.Title == title })
}

// WindowMatching waits for a window for which match reports true; what
// describes it in the failure message.
func (a *App) WindowMatching(what string, match func(WindowInfo) bool) *Window {
	a.t.Helper()
	deadline := time.Now().Add(a.timeout)
	for {
		ws := a.Windows()
		for _, w := range ws {
			if match(w) {
				return &Window{app: a, info: w}
			}
		}
		if time.Now().After(deadline) {
			var titles []string
			for _, w := range ws {
				titles = append(titles, fmt.Sprintf("%d %q (%s)", w.ID, w.Title, w.Kind))
			}
			a.t.Fatalf("automation: no window %s after %v; open: [%s]", what, a.timeout, strings.Join(titles, ", "))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Quit asks the app to quit and waits until the program exits (killing it
// after 10s).
func (a *App) Quit() {
	a.t.Helper()
	a.call(&wire.Request{Op: wire.OpQuit}, "quit")
	if a.cmd == nil {
		return
	}
	select {
	case <-a.exited:
	case <-time.After(10 * time.Second):
		a.cmd.Process.Kill()
		<-a.exited
		a.t.Fatalf("automation: the app did not exit within 10s after quit\n%s", a.outputTail())
	}
}

// Exited reports whether the program has exited (always false for
// Connect).
func (a *App) Exited() bool {
	if a.exited == nil {
		return false
	}
	select {
	case <-a.exited:
		return true
	default:
		return false
	}
}

func (a *App) cleanup() {
	if a.c != nil && !a.Exited() {
		a.c.do(&wire.Request{Op: wire.OpQuit}, 3*time.Second)
	}
	if a.c != nil {
		a.c.close()
	}
	select {
	case <-a.exited:
	case <-time.After(5 * time.Second):
		a.cmd.Process.Kill()
		<-a.exited
	}
	if a.t.Failed() && a.out != nil {
		a.t.Logf("automation: program output:\n%s", a.out.String())
	}
}

func (a *App) outputTail() string {
	if a.out == nil {
		return ""
	}
	s := a.out.String()
	if len(s) > 4000 {
		s = "…" + s[len(s)-4000:]
	}
	if s == "" {
		return "(no output)"
	}
	return "program output:\n" + s
}

// call sends req and fails the test on any error; what names the
// operation in the message.
func (a *App) call(req *wire.Request, what string) *wire.Response {
	a.t.Helper()
	r, err := a.try(req)
	if err != nil {
		msg := fmt.Sprintf("automation: %s: %v", what, err)
		if r != nil && r.Dump != "" {
			msg += "\nsemantics tree:\n" + r.Dump
		}
		if a.Exited() {
			msg += "\n" + a.outputTail()
		}
		a.t.Fatal(msg)
	}
	return r
}

// try sends req and returns the app's error, if any.
func (a *App) try(req *wire.Request) (*wire.Response, error) {
	if a.Exited() {
		return nil, fmt.Errorf("the app has exited (%v)", a.runErr)
	}
	r, err := a.c.do(req, time.Duration(req.Timeout)*time.Millisecond+30*time.Second)
	if err != nil {
		if a.cmd != nil {
			select { // a connection error usually means the app died
			case <-a.exited:
				return nil, fmt.Errorf("the app has exited (%v)", a.runErr)
			case <-time.After(200 * time.Millisecond):
			}
		}
		return nil, err
	}
	return r, r.Err()
}

func (a *App) waitTokenFile(path string, timeout time.Duration) (wire.TokenFile, error) {
	deadline := time.Now().Add(timeout)
	for {
		tf, err := wire.ReadTokenFile(path)
		if err == nil {
			return tf, nil
		}
		select {
		case <-a.exited:
			return tf, fmt.Errorf("the program exited before opening its control channel (%v); is it a Tango app built with -tags tango_dev?", a.runErr)
		default:
		}
		if time.Now().After(deadline) {
			return tf, fmt.Errorf("no control channel after %v (does the program create a tango.App, and was it built with -tags tango_dev?)", timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// resolveTarget returns an executable for target, building packages.
func resolveTarget(target string, opts Options, dir string) (string, error) {
	p := target
	if opts.Dir != "" && !filepath.IsAbs(p) {
		p = filepath.Join(opts.Dir, p)
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
		if !hasChannel(p) {
			return "", fmt.Errorf("%s has no automation channel: build it with -tags tango_dev (production builds, the default, leave the channel out)", p)
		}
		return filepath.Abs(p)
	}
	bin := filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	goTool := "go"
	if p, err := exec.LookPath("go"); err == nil {
		goTool = p
	} else if r := runtime.GOROOT(); r != "" {
		goTool = filepath.Join(r, "bin", "go")
	}
	args := append([]string{"build", "-o", bin}, withDevTag(opts.BuildFlags)...)
	args = append(args, target)
	cmd := exec.Command(goTool, args...)
	cmd.Dir = opts.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build %s: %v\n%s", target, err, out)
	}
	return bin, nil
}

// devTag links the automation channel into a build.
const devTag = "tango_dev"

// withDevTag returns build flags with tango_dev added to their -tags (the
// last -tags flag wins in go build, so it is merged into that one).
func withDevTag(flags []string) []string {
	out := slices.Clone(flags)
	last := -1 // index of the last -tags value
	for i := 0; i < len(out); i++ {
		f := out[i]
		switch {
		case f == "-tags" || f == "--tags":
			if i+1 < len(out) {
				last = i + 1
				i++
			}
		case strings.HasPrefix(f, "-tags=") || strings.HasPrefix(f, "--tags="):
			last = i
		}
	}
	if last < 0 {
		return append([]string{"-tags", devTag}, out...)
	}
	prefix, v := "", out[last]
	if k, val, ok := strings.Cut(v, "="); ok && strings.HasPrefix(k, "-") {
		prefix, v = k+"=", val
	}
	tags := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	if !slices.Contains(tags, devTag) {
		tags = append(tags, devTag)
	}
	out[last] = prefix + strings.Join(tags, ",")
	return out
}

// hasChannel reports whether the executable at path links the automation
// channel (it names the channel's environment variable); a file that
// cannot be read is given the benefit of the doubt.
func hasChannel(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return bytes.Contains(b, []byte(wire.EnvTokenFile))
}

func filterEnv(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, wire.EnvAddr+"=") || strings.HasPrefix(kv, wire.EnvTokenFile+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// ---------------------------------------------------------------------------
// Connection

type client struct {
	mu     sync.Mutex
	conn   net.Conn
	nextID uint64
	broken error
}

func dial(addr, token string) (*client, error) {
	conn, err := wire.Dial(addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	c := &client{conn: conn}
	r, err := c.do(&wire.Request{Op: wire.OpHello, Token: token}, 10*time.Second)
	if err == nil {
		err = r.Err()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	if r.Version != wire.Version {
		conn.Close()
		return nil, fmt.Errorf("protocol version %d, want %d (rebuild the app with this Tango)", r.Version, wire.Version)
	}
	return c, nil
}

// do sends one request and reads its response; a transport error breaks
// the connection for good.
func (c *client) do(req *wire.Request, timeout time.Duration) (*wire.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return nil, c.broken
	}
	c.nextID++
	req.ID = c.nextID
	c.conn.SetDeadline(time.Now().Add(timeout))
	defer c.conn.SetDeadline(time.Time{})
	if err := wire.WriteFrame(c.conn, req); err != nil {
		c.broken = fmt.Errorf("connection lost: %v", err)
		return nil, c.broken
	}
	var r wire.Response
	if err := wire.ReadFrame(c.conn, &r); err != nil {
		c.broken = fmt.Errorf("connection lost: %v", err)
		return nil, c.broken
	}
	if r.ID != req.ID {
		c.broken = fmt.Errorf("protocol error: response %d for request %d", r.ID, req.ID)
		return nil, c.broken
	}
	return &r, nil
}

func (c *client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken == nil {
		c.broken = errors.New("connection closed")
	}
	c.conn.Close()
}

// tailBuffer keeps the last max bytes written.
type tailBuffer struct {
	mu  sync.Mutex
	b   bytes.Buffer
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b.Write(p)
	if over := t.b.Len() - t.max; over > 0 {
		t.b.Next(over)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.String()
}
