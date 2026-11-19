//go:build tango_dev

package uihost_test

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/automation"
	iauto "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/uihost"
)

// Automation drives a window on its own UI thread: Find, Perform and
// Snapshot run there, and a request waiting for a blocked UI thread never
// holds the main thread.
func TestAutomationFindAndPerformOnUIThread(t *testing.T) {
	a, fb, conn := connectAuto(t)
	canvases := useThreadedCanvases(t, false, clockCfg{eager: true})
	var mu sync.Mutex
	clicks := 0
	var clickGIDs []uint64
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: 240, Height: 160},
		}, func(c *ui.Context) {
			if ui.Button(c, "Add").Clicked() {
				mu.Lock()
				clicks++
				clickGIDs = append(clickGIDs, uithread.GoroutineID())
				mu.Unlock()
			}
			mu.Lock()
			n := clicks
			mu.Unlock()
			ui.Text(c, fmt.Sprintf("clicks %d", n))
		}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	for deadline := time.Now().Add(faketest.Timeout); !tango.Await(a, func() bool { return uihost.ThreadedStarted(o.r) }) && time.Now().Before(deadline); {
		fb.Do(func() {})
	}
	tc := canvases(o.s.ID())
	if tc == nil || !tc.started() {
		t.Fatal("no threaded canvas with a running frame clock")
	}
	closeAtEnd(t, fb, o.s, tc)
	main := mainGID(a)

	app := conn()
	ws := app.Windows()
	if len(ws) != 1 || ws[0].Kind != automation.KindCanvas {
		t.Fatalf("windows %+v", ws)
	}
	win := app.Window("UI")
	btn := win.Find(a11y.RoleButton, "Add")
	if n := btn.Node(); n.Bounds.W <= 0 || !n.Actions.Has(a11y.Press) {
		t.Fatalf("button %+v", n)
	}
	btn.Click() // a11y Press: Runner.PerformAction, which checks its thread in dev builds
	win.Find(a11y.RoleText, "clicks 1").Wait()
	btn.ClickPointer() // input enters on main and is queued to the UI thread
	win.Find(a11y.RoleText, "clicks 2").Wait()
	img, err := png.Decode(bytes.NewReader(win.Screenshot()))
	if err != nil {
		t.Fatal(err)
	}
	if sz := img.Bounds().Size(); sz.X != 240 || sz.Y != 160 {
		t.Fatalf("screenshot %v", sz)
	}
	mu.Lock()
	gids := append([]uint64(nil), clickGIDs...)
	mu.Unlock()
	if len(gids) != 2 {
		t.Fatalf("%d clicks handled, want 2", len(gids))
	}
	for _, g := range gids {
		if g != tc.gid || g == main {
			t.Fatalf("click handled on goroutine %d, UI thread %d, main %d", g, tc.gid, main)
		}
	}

	// A blocked UI thread: the request times out on the waiting goroutine
	// while the main thread stays free.
	c := uihost.AutoCanvasForTest(o.r, 300*time.Millisecond)
	blocked, unblock := make(chan struct{}), make(chan struct{})
	o.r.Post(func() { close(blocked); <-unblock })
	<-blocked
	res := make(chan error, 1)
	go func() { _, _, err := c.Tree(); res <- err }()
	start := time.Now()
	tango.Await(a, func() struct{} { return struct{}{} })
	if d := time.Since(start); d > 150*time.Millisecond {
		close(unblock)
		t.Fatalf("main thread busy for %v while automation waited for the UI thread", d)
	}
	err = <-res
	close(unblock)
	var ae *iauto.Error
	if !errors.As(err, &ae) || ae.Code != iauto.CodeBusy {
		t.Fatalf("Tree with a blocked UI thread: %v, want CodeBusy", err)
	}

	// Unblocked, the same canvas works: its tree has the button.
	var tree *a11y.Tree
	for deadline := time.Now().Add(faketest.Timeout); tree == nil && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if tree, _, err = c.Tree(); err != nil {
			t.Fatal(err)
		}
	}
	found := false
	if tree != nil {
		for _, n := range tree.Nodes() {
			found = found || (n.Role == a11y.RoleButton && n.Name == "Add")
		}
	}
	if !found {
		t.Fatal("no Add button in the tree read on the UI thread")
	}
	snap, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if sz := snap.Bounds().Size(); sz.X != 240 || sz.Y != 160 {
		t.Fatalf("snapshot %v", sz)
	}
}

// openThreadedAuto opens a window titled "UI" on a threaded canvas whose
// clock ticks on request, and waits for its UI thread to start.
func openThreadedAuto(t *testing.T, a *tango.App, fb *fake.Backend, page func(*ui.Context)) (*ui.Runner, *threadedCanvas) {
	t.Helper()
	canvases := useThreadedCanvases(t, false, clockCfg{eager: true})
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: 240, Height: 160},
		}, page, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	for deadline := time.Now().Add(faketest.Timeout); !tango.Await(a, func() bool { return uihost.ThreadedStarted(o.r) }) && time.Now().Before(deadline); {
		fb.Do(func() {})
	}
	tc := canvases(o.s.ID())
	if tc == nil || !tc.started() {
		t.Fatal("no threaded canvas with a running frame clock")
	}
	closeAtEnd(t, fb, o.s, tc)
	return o.r, tc
}

// fatalRecorder is a testing.TB whose Fatal and Fatalf record the message
// and end the calling goroutine, so a test can check that a client call
// failed.
type fatalRecorder struct {
	testing.TB
	mu  sync.Mutex
	msg string
}

func (r *fatalRecorder) Helper() {}
func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.mu.Lock()
	r.msg = fmt.Sprintf(format, args...)
	r.mu.Unlock()
	runtime.Goexit()
}
func (r *fatalRecorder) Fatal(args ...any) { r.Fatalf("%s", fmt.Sprint(args...)) }

// A node that exists in a window whose UI thread is busy is not reported
// as missing: Exists fails with the busy error instead of returning false.
func TestAutomationExistsReportsBusyUIThread(t *testing.T) {
	t.Cleanup(uihost.SetAutoWaitForTest(200 * time.Millisecond)) // runs after the app's cleanup
	a, fb, dial := connectAutoTB(t)
	r, _ := openThreadedAuto(t, a, fb, func(c *ui.Context) { ui.Button(c, "Add") })
	app := dial(t)
	app.Window("UI").Find(a11y.RoleButton, "Add").Node() // the tree is on and has the button

	blocked, unblock := make(chan struct{}), make(chan struct{})
	r.Post(func() { close(blocked); <-unblock })
	<-blocked
	rec := &fatalRecorder{TB: t}
	busy := dial(rec)
	got := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		got <- busy.Window("UI").Find(a11y.RoleButton, "Add").Exists()
	}()
	<-done
	close(unblock)
	select {
	case v := <-got:
		t.Fatalf("Exists returned %v while the UI thread was busy; want an error", v)
	default:
	}
	rec.mu.Lock()
	msg := rec.msg
	rec.mu.Unlock()
	if !strings.Contains(msg, "busy") {
		t.Fatalf("Exists failed with %q, want the busy UI thread error", msg)
	}
}
