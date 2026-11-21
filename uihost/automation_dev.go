//go:build tango_dev

package uihost

// Whole-app automation of self-drawn windows (docs/automation.md): every
// window Open creates is registered with the control channel, which the
// root package starts when TANGO_AUTOMATION is set. The semantics tree
// comes from the window's accessibility bridge (a11y.go): automation is
// one of its subscribers (tap) and keeps the tree built (force) since no
// assistive technology asks for it.
//
// Threads: a window drawing on the main thread is driven as before: the
// control channel calls autoCanvas on the main thread. A window opened on
// its own UI thread (spec §5.11) reports automation.OffMain: the control
// channel calls it on its own goroutine and waits there. Each request is
// handed to the main thread (tango.Await from that goroutine), which only
// forwards it: operations on the runner and the bridge (Tree, Perform,
// Snapshot) run on the window's UI thread and input enters on main as the
// platform's does. Results come back over a channel, at most autoWait
// later (automation.CodeBusy otherwise); the main thread never waits for
// a UI thread. Only built with -tags tango_dev; production builds, the
// default, compile it out (automation_release.go).

import (
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// automationWatch registers the window s showing r; h is nil for windows
// shown by the tan dev host, which automation does not support. Main
// thread, before a threaded window's UI thread starts.
func automationWatch(app *tango.App, s *tango.Surface, r *ui.Runner, h *host) {
	c := newAutoCanvas(r, h)
	unregister := automation.Register(app, uint64(s.ID()), c)
	s.OnClosed(func() {
		unregister()
		c.stop()
	})
}

func newAutoCanvas(r *ui.Runner, h *host) *autoCanvas {
	return &autoCanvas{r: r, h: h, threaded: h != nil && h.t != nil, gone: make(chan struct{}), wait: autoWait}
}

// autoWait bounds how long the control channel's goroutine waits for one
// request to run on a threaded window's UI thread (busy or blocked).
// Read when a window opens (main thread); tests shorten it.
var autoWait = 5 * time.Second

var (
	errAutoClosed = &automation.Error{Code: automation.CodeNoWindow, Msg: "automation: the window has closed"}
	errAutoQuit   = &automation.Error{Code: automation.CodeQuit, Msg: "automation: the app has quit or the call panicked (see the app's log)"}
	errAutoPanic  = &automation.Error{Code: automation.CodeInternal, Msg: "automation: the request panicked on the window's thread (see the app's log)"}
	errAutoOffUI  = &automation.Error{Code: automation.CodeInternal, Msg: "automation: accessibility tap requested off the window's UI thread"}
	errAutoBusy   = func(what string, d time.Duration) error {
		return &automation.Error{Code: automation.CodeBusy, Msg: fmt.Sprintf("automation: %s: the window's UI thread is busy or blocked (no answer within %v)", what, d)}
	}
)

// autoCanvas implements automation.Canvas, OffMain and Settler. For a
// main-thread window its methods run on the main thread. For a threaded
// one they run on the control channel's goroutine; the subscription state
// below is then shared with the bridge's tap (UI thread) and stop (main),
// so it is locked in both cases.
type autoCanvas struct {
	r        *ui.Runner
	h        *host         // nil under tan dev
	threaded bool          // h had a UI thread when the window opened
	gone     chan struct{} // closed by stop
	wait     time.Duration // autoWait when the window opened

	mu      sync.Mutex
	on      bool // the tap is attached (or being attached)
	stopped bool
	tree    *a11y.Tree
	gen     uint64
	release func() // drops the force reference; bridge's thread
	cancel  func() // ends the subscription; bridge's thread
}

// OffMain implements automation.OffMain: threaded windows are driven off
// the main thread. Fixed when the window opens: a threaded window that
// later goes back to the main thread keeps this path, whose forwarding
// then runs the work on main.
func (c *autoCanvas) OffMain() bool { return c.threaded }

// bridge returns the window's accessibility bridge for the main-thread
// path. Main thread.
func (c *autoCanvas) bridge() (*a11yBridge, error) {
	if c.h == nil {
		return nil, automation.ErrHotReload
	}
	if c.h.closed {
		return nil, errAutoClosed
	}
	b := a11yFor(c.r)
	if b == nil {
		return nil, automation.ErrHotReload
	}
	return b, nil
}

// usable reports why automation cannot drive a threaded window, if it
// cannot. Any goroutine.
func (c *autoCanvas) usable() error {
	if c.h == nil || a11yFor(c.r) == nil {
		return automation.ErrHotReload
	}
	select {
	case <-c.gone:
		return errAutoClosed
	default:
		return nil
	}
}

// onMain runs fn on the main thread from the control channel's goroutine
// (threaded windows) and returns its result; CodeQuit when the app has
// quit or fn panicked, as for main-thread windows.
func (c *autoCanvas) onMain(fn func() error) error {
	if err := c.usable(); err != nil {
		return err
	}
	var err error
	if !corehook.Await(c.h.app, func() { err = fn() }) {
		return errAutoQuit
	}
	return err
}

// onUI runs fn on a threaded window's UI thread and waits for it on the
// control channel's goroutine, at most c.wait (CodeBusy). The main
// thread only forwards the request (tango.Await from this goroutine, so a
// quitting app reports CodeQuit at once): to the UI thread with the
// window's own queue (threadedHost.post, which Runner.Post also reaches,
// without its extra frame per poll), or runs it at once when the window
// went back to drawing on main.
// A nil return synchronizes with fn's completion. On error fn may still run,
// so callers must only read results written by fn after a nil return.
func (c *autoCanvas) onUI(what string, fn func() error) error {
	if err := c.usable(); err != nil {
		return err
	}
	done := make(chan error, 1)
	run := func() {
		err := error(errAutoPanic) // stays if fn panics: the app's panic handling reports it
		defer func() { done <- err }()
		err = fn()
	}
	closed := false
	if !corehook.Await(c.h.app, func() {
		if c.h.closed {
			closed = true
			return
		}
		if t := c.h.t; t != nil {
			t.post(c.h.app, run)
			return
		}
		run()
	}) {
		return errAutoQuit
	}
	if closed {
		return errAutoClosed
	}
	timer := time.NewTimer(c.wait)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-c.gone:
		return errAutoClosed
	case <-corehook.Done(c.h.app):
		return errAutoQuit
	case <-timer.C:
		return errAutoBusy(what, c.wait)
	}
}

// onOwner reports whether the caller is on the thread that owns the
// window's runner and accessibility bridge: a threaded window's UI thread
// while it has one. The bridge is not locked, so a tap from any other
// thread would race with frame publication and close. Called only from
// onUI's function.
func (c *autoCanvas) onOwner() bool {
	if t := c.h.t; t != nil {
		return t.th.On()
	}
	return uithread.Current() == nil
}

// subscribe attaches the tap and the force reference to b, once. On the
// bridge's thread.
func (c *autoCanvas) subscribe(b *a11yBridge) {
	c.mu.Lock()
	if c.on || c.stopped {
		c.mu.Unlock()
		return
	}
	c.on = true
	c.mu.Unlock()
	// Subscribe first: an existing tree (assistive technology already
	// active) arrives synchronously as the baseline.
	cancel := b.tap(func(t *a11y.Tree, _ []a11y.NodeID) {
		c.mu.Lock()
		if !c.stopped {
			c.tree = t
			c.gen++
		}
		c.mu.Unlock()
	})
	release := b.force()
	c.mu.Lock()
	c.cancel, c.release = cancel, release
	c.mu.Unlock()
}

// subscribeUI is subscribe for a threaded window, refusing any thread but
// its UI thread. UI thread.
func (c *autoCanvas) subscribeUI() error {
	if !c.onOwner() {
		return errAutoOffUI
	}
	b := a11yFor(c.r)
	if b == nil {
		return automation.ErrHotReload
	}
	c.subscribe(b)
	return nil
}

// current returns the last tree and its generation.
func (c *autoCanvas) current() (*a11y.Tree, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tree, c.gen
}

func (c *autoCanvas) Tree() (*a11y.Tree, uint64, error) {
	if !c.threaded {
		b, err := c.bridge()
		if err != nil {
			return nil, 0, err
		}
		c.subscribe(b)
		t, gen := c.current()
		return t, gen, nil
	}
	var t *a11y.Tree
	var gen uint64
	err := c.onUI("Tree", func() error {
		if err := c.subscribeUI(); err != nil {
			return err
		}
		t, gen = c.current()
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return t, gen, nil
}

// Perform asks node id to perform act. On a threaded window, a Perform
// that reports CodeBusy is still queued on the UI thread and may run
// later: a client that retries it may perform the action twice.
func (c *autoCanvas) Perform(id a11y.NodeID, act a11y.Action, arg string) error {
	if !c.threaded {
		if _, err := c.bridge(); err != nil {
			return err
		}
		return c.r.PerformAction(id, act, arg)
	}
	_, err := c.PerformSettle(id, act, arg)
	return err
}

// PerformSettle implements automation.Settler: the action and the base
// generation in one UI-thread task, so a frame that runs after it is the
// one that handles the action (PerformAction runs it at the start of the
// next frame). The CodeBusy caveat of Perform applies.
func (c *autoCanvas) PerformSettle(id a11y.NodeID, act a11y.Action, arg string) (uint64, error) {
	var gen uint64
	err := c.onUI("Perform", func() error {
		if err := c.subscribeUI(); err != nil {
			return err
		}
		if err := c.r.PerformAction(id, act, arg); err != nil {
			return err
		}
		_, gen = c.current()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return gen, nil
}

// deliver hands evs to the window as the platform does: window shortcuts
// first, then the window's queue (its UI thread when it has one). Main
// thread.
func (c *autoCanvas) deliver(evs []input.Event) {
	for _, ev := range evs {
		c.h.CanvasInput(ev)
	}
}

func (c *autoCanvas) Input(evs []input.Event) error {
	if !c.threaded {
		if _, err := c.bridge(); err != nil {
			return err
		}
		c.deliver(evs)
		return nil
	}
	_, err := c.InputSettle(evs)
	return err
}

// InputSettle implements automation.Settler: the events enter on main,
// and the base generation is read by a UI-thread task queued right behind
// them, so any frame after it has received them. When the UI thread does
// not run that task in time the events stay delivered and CodeBusy is
// reported.
func (c *autoCanvas) InputSettle(evs []input.Event) (uint64, error) {
	if err := c.onMain(func() error {
		if c.h.closed {
			return errAutoClosed
		}
		c.deliver(evs)
		return nil
	}); err != nil {
		return 0, err
	}
	var gen uint64
	err := c.onUI("Input", func() error {
		if err := c.subscribeUI(); err != nil {
			return err
		}
		_, gen = c.current()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return gen, nil
}

// Snapshot paints the last frame's display list on the CPU, on the thread
// that owns it, so it works the same whether the window renders with the
// CPU or the GPU.
func (c *autoCanvas) Snapshot() (*image.RGBA, error) {
	if !c.threaded {
		if _, err := c.bridge(); err != nil {
			return nil, err
		}
		size, scale := c.r.FrameMetrics()
		cpu := paint.NewCPU()
		if err := cpu.Render(c.r.List(), size, scale); err != nil {
			return nil, err
		}
		return cpu.Image(), nil
	}
	var img *image.RGBA
	err := c.onUI("Snapshot", func() error {
		cpu := paint.NewCPU()
		size, scale := c.r.FrameMetrics()
		if err := cpu.Render(c.r.List(), size, scale); err != nil {
			return err
		}
		img = cpu.Image()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return img, nil
}

// stop ends the subscription when the window closes. Main thread. The
// bridge belongs to the window's UI thread: a threaded window's cancel
// and release are queued there (dropped when the window is already past
// its last frame, where the bridge goes away with it); the tap ignores
// late trees.
func (c *autoCanvas) stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	cancel, release := c.cancel, c.release
	c.cancel, c.release, c.tree = nil, nil, nil
	c.mu.Unlock()
	close(c.gone)
	end := func() {
		if cancel != nil {
			cancel()
		}
		if release != nil {
			release()
		}
	}
	if c.h != nil && c.h.t != nil {
		c.h.t.post(c.h.app, end)
		return
	}
	end()
}
