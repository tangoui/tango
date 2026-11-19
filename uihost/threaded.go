package uihost

// Per-window UI threads (docs/superpowers/specs/2026-11-18-per-window-ui-thread-design.md
// §5.1, §5.2). A window whose canvas implements platform.CanvasThreaded
// runs its page, layout, rendering and presentation on its own UI thread
// (internal/uithread): the runner is bound to that thread, the canvas's
// frame clock ticks there, and the sink callbacks that arrive on the main
// thread (input, resize, focus, frame requests) are queued onto it in
// order; only resize may wait up to one refresh. Other canvases, Attach
// and the tan dev host keep the main thread as the UI thread.
//
// The window's main-thread calls (cursor, text input, clipboard, window
// state) are posted to the main thread (window.onMain); shortcuts are
// matched and run on the main thread; accessibility actions and the
// global theme, locale, contrast, motion and memory notifications reach
// the runner on its UI thread (callRunner).
//
// Not threaded yet (later steps of the plan): embedded WebViews and
// automation are not supported on these windows.

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// threadedHost is the UI-thread side of a host whose canvas implements
// platform.CanvasThreaded.
type threadedHost struct {
	host       *host
	geometry   platform.CanvasThreaded
	windows    map[*host]struct{} // this app's open threaded windows; main thread
	th         *uithread.Thread
	request    func()       // asks the frame clock for a tick; any goroutine
	stopClock  func()       // UI thread
	attached   bool         // UI thread: ticks before hand-over draw nothing
	gpu        *gpuRenderer // UI thread, including cancelled initialization
	startMu    sync.Mutex
	startQueue []func()
	phase      atomic.Int32
	finishing  bool // main: a failed or closed startup already owns Done cleanup

	// theme is the runner's theme as of the last frame, for main-thread
	// readers (FollowingSystemTheme). TODO(Task 8): part of the per-frame
	// UI-state snapshot.
	theme    atomic.Pointer[theme.Theme]
	renderer atomic.Value // string, published after each frame

	frameInfo atomic.Pointer[ui.WindowFrameInfo] // published by main, read by UI
	closing   atomic.Bool                        // set by main before draining queued work
}

// threadedRunners maps the runner of each open threaded window to its
// threadedHost (AcceptDrops).
var threadedRunners sync.Map // *ui.Runner -> *threadedHost

// Only the main thread accesses each app's set; distinct apps in tests
// can run their main loops on different goroutines.
type threadedApp struct {
	windows  map[*host]struct{}
	stopping bool
}

var threadedApps sync.Map // *tango.App -> *threadedApp

func (h *host) watchThreadedQuit() {
	app := h.app
	v, loaded := threadedApps.LoadOrStore(app, &threadedApp{windows: make(map[*host]struct{})})
	state := v.(*threadedApp)
	h.t.windows = state.windows
	state.windows[h] = struct{}{}
	if !loaded {
		corehook.OnQuit(app, func() { threadedApps.Delete(app) })
		corehook.OnQuitStart(app, func() {
			state.stopping = true
			for window := range state.windows {
				window.close()
			}
		})
	} else if state.stopping {
		h.close()
	}
}

// threadedOf returns r's threadedHost, nil for a runner on the main thread.
func threadedOf(r *ui.Runner) *threadedHost {
	if t, ok := threadedRunners.Load(r); ok {
		return t.(*threadedHost)
	}
	return nil
}

// callRunner runs fn, main-thread code that calls r's UI-thread methods
// (a system notification arrived), on r's UI thread: now on a main-thread
// window, queued with Runner.Post on a threaded one (spec §5.9), where it
// is dropped once the window closed. threaded is threadedOf(r) != nil as
// the caller recorded it when it registered r, so it stays right after
// the window closed. Main thread.
func callRunner(threaded bool, r *ui.Runner, fn func()) {
	if threaded {
		r.Post(fn)
		return
	}
	fn()
}

// post queues fn on the UI thread; a panic in it goes to app's panic
// handling. Dropped once the thread stopped (the window closed).
func (t *threadedHost) post(app *tango.App, fn func()) {
	t.dispatch(func() {
		if !t.closing.Load() {
			corehook.Guard(app, "event handler", fn)
		}
	})
}

const (
	starting int32 = iota
	running
	mainFallback
)

// dispatch shares a queue for main-thread events and Runner.Post until the
// owner is committed; the initial batch is posted before publishing ownership.
func (t *threadedHost) dispatch(fn func()) {
	t.startMu.Lock()
	defer t.startMu.Unlock()
	if t.closing.Load() {
		return
	}
	switch t.phase.Load() {
	case starting:
		t.startQueue = append(t.startQueue, fn)
	case running:
		t.th.Post(fn)
	case mainFallback:
		t.host.app.Post(fn)
	}
}

func (h *host) openThreaded(s *tango.Surface, ct platform.CanvasThreaded, page func(*ui.Context), ropts ui.RunnerOptions) {
	t := &threadedHost{host: h, geometry: ct}
	r := ui.NewRunner(page, ropts)
	t.renderer.Store("cpu")
	threadedRunners.Store(r, t)
	t.theme.Store(r.Context().Theme)
	h.ax = newA11y(h.app, h.cv, r, ropts)
	if _, ok := h.cv.(interface{ MainThreadAccessibility() }); ok {
		h.ax.postNative = func(fn func()) {
			h.app.Post(func() {
				if !t.closing.Load() {
					fn()
				}
			})
		}
	}
	w, ht, scale := ct.MainGeometry()
	h.win = &window{app: h.app, cv: h.cv, id: h.id, sc: h.sc, ax: h.ax, t: t, startup: t, size: geom.Size{W: w, H: ht}, scale: scale}
	h.win.watchFrame(s)
	r.Attach(h.win)
	h.t, h.r = t, r
	t.th = uithread.Start(fmt.Sprintf("tango uihost surface %d", h.id))
}

func (h *host) startThreaded() {
	t := h.t
	h.watchThreadedQuit()
	if h.closed {
		return
	}
	// Install AppKit layers on main, then do all presenter work off-main.
	build := prepareThreadedGPU(h.cv, h.startOpts)
	h.startOpts = ui.RunnerOptions{}
	t.th.Post(func() {
		if t.closing.Load() {
			return
		}
		var err error
		ok := corehook.Guard(h.app, "ui startup", func() {
			t.request, t.stopClock, err = t.geometry.StartFrameClock(func() {
				if t.attached && !t.closing.Load() {
					h.frame()
				}
			})
			if err != nil || t.closing.Load() {
				return
			}
			if build != nil {
				t.gpu = build()
			}
			if t.gpu != nil {
				t.gpu.main = h.app
			}
		})
		if !ok {
			err = fmt.Errorf("UI initialization panicked")
		}
		if !t.closing.Load() {
			h.app.Post(func() { h.finishThreadedStartup(t, err) })
		}
	})
}

func (h *host) finishThreadedStartup(t *threadedHost, err error) {
	if h.closed || t.closing.Load() {
		return
	}
	if err != nil {
		log.Printf("tango: uihost: surface %d: UI startup failed, drawing on main: %v", h.id, err)
		h.stopStarting(t)
		return
	}
	t.startMu.Lock()
	defer t.startMu.Unlock()
	batch := t.startQueue
	t.startQueue = nil
	var trace *frameTrace
	if t.gpu != nil && frameTraceOn() {
		trace = newFrameTrace(h.id)
	}
	h.trace = trace // written before the UI task that reads it is posted
	// Queued before binding; the lock keeps even an eager UI thread from
	// observing partially published host fields or rendering before the batch.
	t.th.Post(func() {
		t.startMu.Lock()
		t.startMu.Unlock()
		if t.closing.Load() {
			return
		}
		if trace != nil {
			t.gpu.setTrace(trace) // presenter timeline belongs to the UI thread
		}
		for _, fn := range batch {
			if !t.closing.Load() {
				corehook.Guard(h.app, "event handler", fn)
			}
		}
		t.attached = true
		t.request()
	})
	h.gpu, h.win.gpu = t.gpu, t.gpu
	t.renderer.Store(t.gpu.kind())
	var renderer any
	if t.gpu != nil {
		renderer = t.gpu
	}
	corehook.FinishRunnerStartup(h.r, t.th, renderer)
	t.phase.Store(running)
}

// stopStarting handles an unpublished UI owner. AfterUIThread both drains
// normal completion and supports shutdown without pumping the native loop.
func (h *host) stopStarting(t *threadedHost) {
	if t.finishing {
		return
	}
	t.finishing = true
	t.th.Post(func() {
		if t.stopClock != nil {
			t.stopClock()
		}
		if t.gpu != nil {
			_ = t.gpu.Close()
		}
	})
	t.th.Stop()
	corehook.AfterUIThread(h.app, t.th.Done(), func() {
		if h.closed {
			h.ax.close()
			if cv, ok := h.cv.(platform.CanvasThreadedRelease); ok {
				cv.ReleaseThreadedOnMain()
			}
			return
		}
		if cv, ok := h.cv.(interface{ CancelThreadedOnMain() }); ok {
			cv.CancelThreadedOnMain() // does not ask for a frame: the drain below does
		}
		// This callback is itself a main-thread task: switching to the
		// main-thread model and running the work queued during startup here,
		// in one step, leaves no point where new native input (which reads
		// h.t) or a frame could run ahead of that work.
		t.startMu.Lock()
		batch := t.startQueue
		t.startQueue = nil
		h.t, h.win.t = nil, nil
		h.ax.postNative = nil
		h.win.emb = newEmbeds(h, h.win.s)
		threadedRunners.Delete(h.r)
		delete(t.windows, h) // the quit hook no longer closes it as threaded
		t.phase.Store(mainFallback)
		t.startMu.Unlock()
		for _, fn := range batch {
			if !h.closed {
				corehook.Guard(h.app, "event handler", fn)
			}
		}
		h.invalidate()
	})
}

const resizeWait = 16700 * time.Microsecond

// resizedOnMain queues a resize and waits at most one refresh for the
// matching Present call to return. The clock still schedules the frame.
// In a nested main-thread call from this window, waiting would prevent
// its UI thread from returning to the clock, so skip the wait entirely.
func (t *threadedHost) resizedOnMain(w, hgt, scale float32) {
	h := t.host
	// A Surface.OnResized callback can resize again before the backend
	// delivers this notification. Never overwrite the newer geometry.
	if currentW, currentH, currentScale := t.geometry.MainGeometry(); w != currentW || hgt != currentH || scale != currentScale {
		return
	}
	if t.phase.Load() == starting {
		t.post(h.app, func() { h.win.size, h.win.scale = geom.Size{W: w, H: hgt}, scale; h.win.resized() })
		return
	}
	awaiting, entered := t.th.WatchAwaitingMain()
	deadline := time.Now().Add(resizeWait)
	presented := make(chan struct{})
	if !t.th.Post(func() {
		if t.closing.Load() {
			return
		}
		corehook.Guard(h.app, "event handler", func() {
			h.win.size, h.win.scale = geom.Size{W: w, H: hgt}, scale
			h.win.resizePresented = presented
			h.win.resized()
			t.request()
		})
	}) || awaiting {
		return
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-presented:
	case <-entered:
	case <-timer.C:
	}
}

// closeThreaded stops the clock and releases the window's renderer and
// accessibility bridge on the UI thread, then lets it exit. Main thread;
// the runner is already closed and no new window work can reach its UI.
func (h *host) closeThreaded() {
	t, ax, gpu := h.t, h.ax, h.gpu
	delete(t.windows, h)
	threadedRunners.Delete(h.r)
	t.th.Post(func() {
		corehook.Guard(h.app, "event handler", func() {
			if gpu != nil {
				defer gpu.Close()
			}
			// The bridge (including fanOut and tap cancellation) is owned
			// by the UI thread. It must not publish native AX from close.
			defer ax.close()
			t.stopClock()
		})
	})
	t.th.Stop()
	var release func()
	if cv, ok := h.cv.(platform.CanvasThreadedRelease); ok {
		release = cv.ReleaseThreadedOnMain
	}
	corehook.AfterUIThread(h.app, t.th.Done(), release)
}

// prepareThreadedGPU selects the same initial policy as setupGPU, but splits
// AppKit layer installation from presenter creation. Neither builder nor the
// clock may await main: main is still in Open. If native construction fails
// after its layer was installed, CPU can use that layer without a second hop.
func prepareThreadedGPU(cv platform.Canvas, opts ui.RunnerOptions) func() *gpuRenderer {
	mode := os.Getenv("TANGO_RENDERER")
	if opts.Renderer != nil || mode == "cpu" {
		return nil
	}
	if (mode == "native" || mode == "") && nativeNewPresenter != nil {
		if target, ok := cv.(platform.CanvasCoreGraphics); ok {
			if layer, err := target.UseCoreGraphicsLayer(); err == nil {
				return func() *gpuRenderer {
					g, err := buildNative(cv, mode == "native", opts.Content, layer)
					if err == nil {
						return g
					}
					logGPUUnavailable(err, false)
					return &gpuRenderer{cpu: paint.NewCPU()}
				}
			}
		}
	}
	if mode == "auto" && autoCPUWindow(cv) {
		return func() *gpuRenderer { return &gpuRenderer{cpu: paint.NewCPU(), auto: &autoRendererState{canvas: cv}} }
	}
	target, err := gpuTarget(cv)
	return func() *gpuRenderer { return buildGPU(cv, mode == "gpu", target, err) }
}
