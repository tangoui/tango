package uihost

// Per-window UI threads (docs/superpowers/specs/2026-11-18-per-window-ui-thread-design.md
// §5.1, §5.2). A window whose canvas implements platform.CanvasThreaded
// runs its page, layout, rendering and presentation on its own UI thread
// (internal/uithread): the runner is bound to that thread, the canvas's
// frame clock ticks there, and the sink callbacks that arrive on the main
// thread (input, resize, focus, frame requests) are queued onto it in
// order and return at once. Other canvases, Attach and the tan dev host
// keep the main thread as the UI thread.
//
// Not threaded yet (later steps of the plan): shortcut functions and the
// window's main-thread calls (cursor, text input, clipboard, window state)
// run on the UI thread as the runner makes them; global theme, contrast,
// motion and memory notifications still call the runner on the main
// thread; embedded WebViews are not placed (the window has no embeds);
// accessibility actions and automation reach the runner from the main
// thread.

import (
	"fmt"
	"log"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
)

// threadedHost is the UI-thread side of a host whose canvas implements
// platform.CanvasThreaded.
type threadedHost struct {
	th        *uithread.Thread
	request   func() // asks the frame clock for a tick; any goroutine
	stopClock func() // UI thread
}

// post queues fn on the UI thread; a panic in it goes to app's panic
// handling. Dropped once the thread stopped (the window closed).
func (t *threadedHost) post(app *tango.App, fn func()) {
	t.th.Post(func() { corehook.Guard(app, "event handler", fn) })
}

// openThreaded creates the runner of a threaded window on a new UI thread
// and starts the canvas's frame clock there. It returns nil, with the
// thread stopped and h unchanged, when the clock cannot start: the window
// then uses the main thread. Main thread.
func (h *host) openThreaded(s *tango.Surface, ct platform.CanvasThreaded, page func(*ui.Context), ropts ui.RunnerOptions) *ui.Runner {
	t := &threadedHost{th: uithread.Start(fmt.Sprintf("tango uihost surface %d", h.id))}
	h.t = t // before the thread can tick: h.invalidate reads it there
	var r *ui.Runner
	var err error
	attached := false // UI thread: ticks before Attach draw nothing
	tick := func() {
		if attached {
			h.frame() // CanvasFrame of a main-thread window
		}
	}
	t.th.Do(func() {
		t.request, t.stopClock, err = ct.StartFrameClock(tick)
		if err != nil {
			return
		}
		// The GPU switch (UseMetalLayer, UseCoreGraphicsLayer) and the
		// presenter are made on the UI thread, which presents with them.
		h.gpu = setupGPU(h.cv, ropts)
		if h.gpu != nil {
			ropts.Renderer = h.gpu
		}
		r = ui.NewRunner(page, ropts)
	})
	if err != nil {
		t.th.Stop()
		h.t = nil
		log.Printf("tango: uihost: surface %d: no frame clock, drawing on the main thread: %v", h.id, err)
		return nil
	}
	// newA11y reads the system's reduce-motion preference, which needs the
	// main thread, and applies it to the runner: before BindThread, so the
	// runner is not yet checked.
	h.ax = newA11y(h.app, h.cv, r, ropts)
	r.BindThread(t.th)
	h.win = &window{app: h.app, cv: h.cv, id: h.id, gpu: h.gpu, sc: h.sc, ax: h.ax, t: t, geo: ct}
	h.win.watchFrame(s) // frame.go
	t.th.Do(func() {
		r.Attach(h.win)
		h.watchRenderer(r)
		attached = true
	})
	return r
}

// closeThreaded stops the clock and releases the window's renderer and
// accessibility bridge on the UI thread, then lets the thread exit. It does
// not wait. Main thread; the runner is already closed.
//
// TODO(Task 6): the full teardown rules (native objects released after the
// thread exits, app quit waiting for UI threads).
func (h *host) closeThreaded() {
	t, ax, gpu := h.t, h.ax, h.gpu
	t.th.Post(func() {
		t.stopClock()
		ax.close()
		if gpu != nil {
			_ = gpu.Close()
		}
	})
	t.th.Stop()
}
