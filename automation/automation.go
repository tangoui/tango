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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
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

// EnvLaunchBackground is set to 1 for the launched program: on macOS it
// starts without activating and shows its windows behind the others.
// Pass EnvLaunchBackground+"=0" in Options.Env to launch it in front.
const EnvLaunchBackground = client.EnvLaunchBackground

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
	c       *client.Conn
	timeout time.Duration

	proc *client.Process // nil for Connect
	out  *tailBuffer
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
	bin, err := client.Build(target, opts.Dir, opts.BuildFlags, dir)
	if err != nil {
		t.Fatalf("automation: %v", err)
	}
	a := &App{t: t, timeout: opts.Timeout, out: &tailBuffer{max: 256 << 10}}
	if a.timeout <= 0 {
		a.timeout = DefaultTimeout
	}
	var w io.Writer = a.out
	if opts.Output != nil {
		w = io.MultiWriter(a.out, opts.Output)
	} else if os.Getenv("TANGO_E2E_VERBOSE") != "" {
		w = io.MultiWriter(a.out, os.Stderr)
	}
	t.Cleanup(a.cleanup)
	p, err := client.Launch(client.LaunchOptions{
		Target:       bin,
		Dir:          opts.Dir,
		Args:         opts.Args,
		Env:          opts.Env,
		Addr:         opts.Addr,
		Output:       w,
		StartTimeout: opts.StartTimeout,
		TempDir:      dir,
	})
	if err != nil {
		t.Fatalf("automation: %v\n%s", err, a.outputTail())
	}
	a.proc, a.c = p, p.Conn
	return a
}

// Connect attaches to an app that is already running with automation on
// (addr and token as the app wrote them to TANGO_AUTOMATION_TOKEN_FILE or
// printed them). The app is not quit at the end of the test; the
// connection is closed.
func Connect(t testing.TB, addr, token string) *App {
	t.Helper()
	c, err := client.Dial(addr, token)
	if err != nil {
		t.Fatalf("automation: connect to %s: %v", addr, err)
	}
	a := &App{t: t, c: c, timeout: DefaultTimeout}
	t.Cleanup(c.Close)
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
	if a.proc == nil {
		return
	}
	select {
	case <-a.proc.Exited():
	case <-time.After(10 * time.Second):
		a.proc.Cmd.Process.Kill()
		<-a.proc.Exited()
		a.t.Fatalf("automation: the app did not exit within 10s after quit\n%s", a.outputTail())
	}
}

// Exited reports whether the program has exited (always false for
// Connect).
func (a *App) Exited() bool {
	if a.proc == nil {
		return false
	}
	select {
	case <-a.proc.Exited():
		return true
	default:
		return false
	}
}

func (a *App) cleanup() {
	if a.proc != nil {
		a.proc.Quit(5 * time.Second)
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
		return nil, fmt.Errorf("the app has exited (%v)", a.proc.ExitErr())
	}
	r, err := a.c.Do(req, time.Duration(req.Timeout)*time.Millisecond+30*time.Second)
	if err != nil {
		if a.proc != nil {
			select { // a connection error usually means the app died
			case <-a.proc.Exited():
				return nil, fmt.Errorf("the app has exited (%v)", a.proc.ExitErr())
			case <-time.After(200 * time.Millisecond):
			}
		}
		return nil, err
	}
	return r, r.Err()
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
