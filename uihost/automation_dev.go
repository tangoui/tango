//go:build tango_dev

package uihost

// Whole-app automation of self-drawn windows (docs/automation.md): every
// window Open creates is registered with the control channel, which the
// root package starts when TANGO_AUTOMATION is set. The semantics tree
// comes from the window's accessibility bridge (a11y.go): automation is
// one of its subscribers (tap) and keeps the tree built (force) since no
// assistive technology asks for it.
//
// Threads (spec §5.11): the control channel calls autoCanvas on its own
// goroutine (automation.OffMain) and waits there. Each request is handed
// to the main thread, which only forwards it: operations on the runner and
// the bridge (Tree, Perform, Snapshot) run on the window's UI thread (the
// main thread for windows without one) and input enters on main as the
// platform's does. Results come back over a channel; the main thread never
// waits for a UI thread. Only built with -tags tango_dev; production
// builds, the default, compile it out (automation_release.go).

import (
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// automationWatch registers the window s showing r; h is nil for windows
// shown by the tan dev host, which automation does not support.
func automationWatch(app *tango.App, s *tango.Surface, r *ui.Runner, h *host) {
	c := newAutoCanvas(r, h)
	unregister := automation.Register(app, uint64(s.ID()), c)
	s.OnClosed(func() {
		unregister()
		c.stop()
	})
}

func newAutoCanvas(r *ui.Runner, h *host) *autoCanvas {
	return &autoCanvas{r: r, h: h, gone: make(chan struct{}), wait: autoWait}
}

// autoWait bounds how long the control channel's goroutine waits for one
// request to run on the window's thread (a busy or blocked UI thread).
const autoWait = 5 * time.Second

var (
	errAutoClosed  = &automation.Error{Code: automation.CodeNoWindow, Msg: "automation: the window has closed"}
	errAutoPanic   = &automation.Error{Code: automation.CodeInternal, Msg: "automation: the request panicked on the window's thread (see the app's log)"}
	errAutoOffUI   = &automation.Error{Code: automation.CodeInternal, Msg: "automation: accessibility tap requested off the window's UI thread"}
	errAutoTimeout = func(what string, d time.Duration) error {
		return &automation.Error{Code: automation.CodeTimeout, Msg: fmt.Sprintf("automation: %s did not run within %v (the window's thread is busy or blocked)", what, d)}
	}
)

// autoCanvas implements automation.Canvas and automation.OffMain. Its
// methods run on the control channel's goroutine; the subscription state
// below is shared with the bridge's tap (UI thread) and stop (main), so
// it is locked.
type autoCanvas struct {
	r    *ui.Runner
	h    *host         // nil under tan dev
	gone chan struct{} // closed by stop
	wait time.Duration // autoWait (tests shorten it)

	mu      sync.Mutex
	on      bool // the tap is attached (or being attached)
	stopped bool
	tree    *a11y.Tree
	gen     uint64
	release func() // drops the force reference; UI thread
	cancel  func() // ends the subscription; UI thread
}

// OffMain implements automation.OffMain.
func (c *autoCanvas) OffMain() {}

// usable reports why automation cannot drive the window, if it cannot.
// Any goroutine.
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

// onMain runs fn on the main thread and waits for it, at most autoWait.
// Called on the control channel's goroutine.
func (c *autoCanvas) onMain(what string, fn func() error) error {
	return c.await(what, func(run func()) { c.h.app.Post(run) }, fn)
}

// onUI runs fn on the window's UI thread and waits for it, at most
// autoWait. The main thread only forwards the request: to the UI thread
// with the window's own queue (threadedHost.post, which Runner.Post also
// reaches, without its extra frame per poll), or runs it at once when the
// window draws on main. Called on the control channel's goroutine.
func (c *autoCanvas) onUI(what string, fn func() error) error {
	return c.await(what, func(run func()) {
		c.h.app.Post(func() {
			if c.h.closed {
				return // stop closes gone
			}
			if t := c.h.t; t != nil {
				t.post(c.h.app, run)
				return
			}
			run()
		})
	}, fn)
}

func (c *autoCanvas) await(what string, send func(run func()), fn func() error) error {
	if err := c.usable(); err != nil {
		return err
	}
	done := make(chan error, 1)
	send(func() {
		err := error(errAutoPanic) // stays if fn panics: the app's panic handling reports it
		defer func() { done <- err }()
		err = fn()
	})
	timer := time.NewTimer(c.wait)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-c.gone:
		return errAutoClosed
	case <-timer.C:
		return errAutoTimeout(what, c.wait)
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

// subscribe attaches the tap and the force reference, once. UI thread.
func (c *autoCanvas) subscribe() error {
	if !c.onOwner() {
		return errAutoOffUI
	}
	c.mu.Lock()
	if c.on || c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.on = true
	c.mu.Unlock()
	b := a11yFor(c.r)
	if b == nil {
		c.mu.Lock()
		c.on = false
		c.mu.Unlock()
		return automation.ErrHotReload
	}
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
	return nil
}

func (c *autoCanvas) Tree() (*a11y.Tree, uint64, error) {
	var t *a11y.Tree
	var gen uint64
	err := c.onUI("Tree", func() error {
		if err := c.subscribe(); err != nil {
			return err
		}
		c.mu.Lock()
		t, gen = c.tree, c.gen
		c.mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return t, gen, nil
}

func (c *autoCanvas) Perform(id a11y.NodeID, act a11y.Action, arg string) error {
	return c.onUI("Perform", func() error { return c.r.PerformAction(id, act, arg) })
}

// Input delivers events on the main thread, as the platform does: window
// shortcuts first, then the window's queue (its UI thread when it has
// one).
func (c *autoCanvas) Input(evs []input.Event) error {
	return c.onMain("Input", func() error {
		if c.h.closed {
			return errAutoClosed
		}
		for _, ev := range evs {
			c.h.CanvasInput(ev)
		}
		return nil
	})
}

// Snapshot paints the last frame's display list on the CPU, on the UI
// thread that owns it, so it works the same whether the window renders
// with the CPU or the GPU.
func (c *autoCanvas) Snapshot() (*image.RGBA, error) {
	var img *image.RGBA
	err := c.onUI("Snapshot", func() error {
		cpu := paint.NewCPU()
		if err := cpu.Render(c.r.List(), c.h.win.Size(), c.h.win.Scale()); err != nil {
			return err
		}
		img = cpu.Image()
		return nil
	})
	return img, err
}

// stop ends the subscription when the window closes. Main thread. The
// bridge belongs to the UI thread: a threaded window's cancel and release
// are queued there (dropped when the window is already past its last
// frame, where the bridge goes away with it); the tap ignores late trees.
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
