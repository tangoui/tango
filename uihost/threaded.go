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
	"sync"

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

// threadedRunners maps the runner of each open threaded window to its
// threadedHost (AcceptDrops).
var threadedRunners sync.Map // *ui.Runner -> *threadedHost

// threadedOf returns r's threadedHost, nil for a runner on the main thread.
func threadedOf(r *ui.Runner) *threadedHost {
	if t, ok := threadedRunners.Load(r); ok {
		return t.(*threadedHost)
	}
	return nil
}

// post queues fn on the UI thread; a panic in it goes to app's panic
// handling. Dropped once the thread stopped (the window closed).
func (t *threadedHost) post(app *tango.App, fn func()) {
	t.th.Post(func() { corehook.Guard(app, "event handler", fn) })
}

// openThreaded starts a UI thread for a threaded window and the canvas's
// frame clock on it, then creates the runner (h.r) on the main thread and
// hands it to the UI thread. It leaves h unchanged, with the thread stopped,
// when the clock cannot start: the window then uses the main
// thread. Main thread.
//
// Only StartFrameClock waits for the UI thread (it must not wait for the
// main thread). The GPU switch (UseMetalLayer, UseCoreGraphicsLayer) needs
// the main thread, so setupGPU runs here; so do newA11y (it reads the
// system's reduce-motion preference) and everything else that calls the
// runner, before BindThread. After BindThread nothing here calls a checked
// runner method; the first frame is the UI thread's.
func (h *host) openThreaded(s *tango.Surface, ct platform.CanvasThreaded, page func(*ui.Context), ropts ui.RunnerOptions) {
	t := &threadedHost{th: uithread.Start(fmt.Sprintf("tango uihost surface %d", h.id))}
	attached := false // UI thread: ticks before the hand-over draw nothing
	tick := func() {
		if attached {
			h.frame() // CanvasFrame of a main-thread window
		}
	}
	var err error
	t.th.Do(func() { t.request, t.stopClock, err = ct.StartFrameClock(tick) })
	if err != nil {
		t.th.Stop()
		log.Printf("tango: uihost: surface %d: no frame clock, drawing on the main thread: %v", h.id, err)
		return
	}
	h.gpu = setupGPU(h.cv, ropts) // gpu.go; nil: CPU
	if h.gpu != nil {
		ropts.Renderer = h.gpu
	}
	r := ui.NewRunner(page, ropts)
	h.ax = newA11y(h.app, h.cv, r, ropts)
	r.BindThread(t.th)
	h.win = &window{app: h.app, cv: h.cv, id: h.id, gpu: h.gpu, sc: h.sc, ax: h.ax, t: t, geo: ct}
	h.win.watchFrame(s) // frame.go
	r.Attach(h.win)
	h.watchRenderer(r)
	threadedRunners.Store(r, t)
	// Until here sink callbacks took the main-thread path, which ignores
	// them while h.r is nil. From here they are queued on the UI thread.
	h.t, h.r = t, r
	// The hand-over, posted once every host field a frame reads is written:
	// all of it happens before the UI thread's first frame, which this task
	// asks for (a tick that came earlier drew nothing). Open writes no host
	// field the UI thread reads after this.
	t.th.Post(func() {
		attached = true
		t.request()
	})
}

// closeThreaded stops the clock and releases the window's renderer and
// accessibility bridge on the UI thread, then lets the thread exit. It does
// not wait. Main thread; the runner is already closed.
//
// TODO(Task 6): the full teardown rules (native objects released after the
// thread exits, app quit waiting for UI threads).
func (h *host) closeThreaded() {
	t, ax, gpu := h.t, h.ax, h.gpu
	threadedRunners.Delete(h.r)
	t.th.Post(func() {
		t.stopClock()
		ax.close()
		if gpu != nil {
			_ = gpu.Close()
		}
	})
	t.th.Stop()
}
