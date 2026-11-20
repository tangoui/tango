// Package uihost shows Tango's self-drawn UI (package ui) in native
// windows: it adapts a tango.App to the ui.Window contract and drives a
// ui.Runner from the backend's canvas callbacks. See
// docs/specs/canvas-surface.md.
//
// The UI thread for Open, Attach and windows shown by the tan dev host is
// the app's main thread. Page functions, Runner methods and callbacks run
// there. Dedicated macOS UI threads are currently used only for internal
// testing.
//
// Use Runner.Post to update page state from background goroutines. It runs
// fn on the window's UI thread and then asks for a frame. Main-thread
// callbacks can also use Runner.Post to queue page updates. Calls that need
// the main thread (Open, Attach, FollowSystemTheme) use App.Post or
// tango.Await. Runner.Invalidate, InvalidateAt and Close are safe from any
// goroutine. Development builds (-tags tango_dev) enforce thread affinity
// only for runners bound to a dedicated UI thread in internal tests. On
// macOS a window's first frame runs after the main-thread callback that
// called Open returns. Variables set right after Open in that callback
// are visible to the page, unless
// the callback first runs a nested event loop (a dialog, or AwaitAsync on
// main), during which frames may already start.
//
// API tiers: only the symbols listed in api/stable/uihost.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package uihost

import (
	"errors"
	"image"
	"log"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/guiprobe"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/clipboard"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Options configure a UI window.
type Options struct {
	// Surface configures the window like App.NewSurface. URL and Debug are
	// ignored: there is no page, the content is drawn by the runner.
	Surface tango.SurfaceOptions

	// Theme says whether the window follows the system's light/dark
	// appearance (follow.go). The zero value, ThemeSystem, follows it,
	// unless RunnerOptions.Theme is set: then the window keeps that theme
	// (ThemeFixed), as before these options existed.
	Theme ThemeMode
	// Flavor builds the theme while following: dark is the appearance to
	// draw, accent the system accent color when AccentFromSystem is set
	// and the system has one (else nil). nil: theme.AntDesign with
	// theme.Dark() and theme.WithPrimary(*accent) as needed.
	Flavor func(dark bool, accent *theme.Color) *theme.Theme
	// AccentFromSystem uses the system accent color as the primary color.
	AccentFromSystem bool
	// Customize, when set, adjusts every generated theme's tokens.
	Customize func(*theme.Theme)
	// Contrast says whether a generated theme follows the system
	// high-contrast preference (contrast.go). The zero value,
	// ContrastSystem, draws theme.HighContrast of the light or dark theme
	// while the system asks for more contrast; ContrastStandard and
	// ContrastHigh fix it. Like Flavor and Customize it only applies while
	// the window generates its theme: an explicit theme is never changed.
	Contrast ContrastMode
	// Locale says whether the built-in texts of components follow
	// i18n.Locale() (LocaleSystem, the zero value) or are left alone.
	Locale LocaleMode
}

// Open creates a window whose content is page, drawn by a new ui.Runner,
// and returns the runner and the window's surface. The surface is an
// ordinary *tango.Surface (title, size, state, appearance, OnClose,
// OnClosed, ...); WebView-only methods (Navigate, Eval, Send) do nothing.
// The runner is closed when the surface closes.
//
// By default the window's theme follows the system's light/dark appearance
// and the components' built-in texts follow i18n.Locale(); see
// Options.Theme, Options.Locale and FollowSystemTheme.
//
// Open must be called on the main thread: from App.OnReady, a function
// passed to App.Post, or another main-thread callback. It returns an error
// wrapping tango.ErrUnsupported when the platform cannot create such
// windows.
func Open(app *tango.App, opts Options, page func(*ui.Context), ropts ui.RunnerOptions) (*ui.Runner, *tango.Surface, error) {
	if app == nil {
		return nil, nil, errors.New("uihost: nil app")
	}
	ropts, fl := followSetup(app, opts, ropts) // theme and locale (follow.go)

	if r, s, ok, err := devOpen(app, opts, page, ropts); ok { // tan dev: window in the dev host
		if err == nil {
			fl.start(r, s)
			automationWatch(app, s, r, nil) // automation_dev.go: reports hot reload as unsupported
		}
		return r, s, err
	}
	h := &host{app: app}
	so, cv, err := corehook.NewCanvasSurface(app, opts.Surface, h)
	if err != nil {
		return nil, nil, err
	}
	s := so.(*tango.Surface)
	h.id = s.ID()
	h.cv = cv
	h.sc = newShortcuts(app, s) // shortcuts.go
	if ct, ok := cv.(platform.CanvasThreaded); ok {
		h.openThreaded(s, ct, page, ropts)
		h.startOpts = ropts
	}
	if h.r == nil {
		h.openMain(s, page, ropts)
	}
	r := h.r
	s.OnClosed(h.close)
	watchMemory(app, r) // memory.go
	fl.start(r, s)
	automationWatch(app, s, r, h) // TANGO_AUTOMATION (automation_dev.go)
	if h.t != nil {
		h.startThreaded()
	}
	// Callbacks before Attach found no window; make sure a frame follows.
	h.invalidate()
	return r, s, nil
}

// openMain creates the runner (h.r) of a window whose UI thread is the
// main thread.
func (h *host) openMain(s *tango.Surface, page func(*ui.Context), ropts ui.RunnerOptions) {
	h.gpu = setupGPU(h.cv, ropts) // gpu.go; nil: CPU
	if h.gpu != nil {
		ropts.Renderer = h.gpu
	}
	r := ui.NewRunner(page, ropts)
	h.ax = newA11y(h.app, h.cv, r, ropts)
	h.win = &window{app: h.app, cv: h.cv, id: h.id, gpu: h.gpu, sc: h.sc, ax: h.ax}
	h.win.emb = newEmbeds(h, s) // embedded WebViews (webview.go)
	h.win.watchFrame(s)         // frame.go: ui.WindowFrame
	r.Attach(h.win)
	h.watchRenderer(r)
	h.r = r
}

// watchRenderer registers r's GPU renderer (Renderer) and starts its frame
// trace. Main thread, before the window's first frame.
func (h *host) watchRenderer(r *ui.Runner) {
	if h.gpu == nil {
		frameTraceSkipped(h.id, "drawn by the CPU (TANGO_RENDERER=cpu, no GPU presenter, or a renderer set in RunnerOptions); only GPU windows are traced")
		return
	}
	if threadedOf(r) == nil {
		renderers.Store(r, h.gpu)
	}
	if frameTraceOn() {
		h.trace = newFrameTrace(h.id)
		h.gpu.setTrace(h.trace)
	}
}

// host is the platform.CanvasSink of one window. Its fields are main-thread
// state, except on a threaded window (t non-nil, threaded.go): there gpu,
// ax, win and trace belong to the UI thread once Open returns, and the
// callbacks below queue their work onto it.
type host struct {
	app       *tango.App
	r         *ui.Runner      // nil until the surface exists
	cv        platform.Canvas // nil until the surface exists
	gpu       *gpuRenderer    // nil: CPU path
	id        platform.SurfaceID
	sc        *shortcuts // window shortcuts (shortcuts.go); nil for Attach
	closed    bool
	lastErr   string // last frame error logged
	ax        *a11yBridge
	win       *window       // the runner's window; nil until attached
	trace     *frameTrace   // TANGO_FRAME_TRACE (frametrace.go); nil: off
	t         *threadedHost // non-nil: the window has its own UI thread
	startOpts ui.RunnerOptions
}

func (h *host) CanvasFrame() {
	if h.closed || h.r == nil { // nil: a callback while Open sets up
		return
	}
	if h.t != nil { // the frame clock draws on the UI thread
		h.win.Invalidate()
		return
	}
	h.frame()
}

// frame renders and presents one frame. UI thread.
func (h *host) frame() {
	if h.gpu != nil {
		if h.win.resizePresented != nil {
			// Threaded resize records its acknowledgment on this same UI
			// thread, before requesting the frame that presents the new size.
			h.gpu.native.noteInput()
		}
		h.gpu.native.maybePromoteAnimation()
	}
	if h.trace != nil {
		h.trace.begin()
		// The renderer is read when the frame ends: a frame that loses the
		// GPU is summarized as cpu.
		defer func() { h.trace.end(h.gpu.kind()) }()
	}
	if h.t != nil {
		defer func() { h.t.renderer.Store(h.gpu.kind()) }()
	}
	var err error
	// A panic in the frame (page functions included) goes to the app's
	// panic handling (tango.Options.OnPanic); no frame is scheduled after it.
	if !corehook.Guard(h.app, "ui frame", func() { err = h.r.Frame() }) {
		return
	}
	if err != nil {
		// Keep running; log each distinct error once in a row so a failing
		// animation does not flood the log.
		if msg := err.Error(); msg != h.lastErr {
			h.lastErr = msg
			log.Printf("tango: uihost: surface %d: frame: %v", h.id, err)
		}
		if h.gpu != nil {
			h.gpu.frameError(h.cv, h.id, err)
		}
	} else {
		h.lastErr = ""
	}
	h.win.emb.sync() // place embedded WebViews on this frame's layout (webview.go)
	if h.t != nil {
		if h.t.switching.Load() {
			return // handed back to the main thread (webview_threaded.go)
		}
		h.publishSnapshot() // main-thread decisions (snapshot.go)
	}
	if h.r.Animating() {
		h.invalidate()
	}
	if t := h.r.NextWake(); !t.IsZero() && (h.t == nil || !h.t.closing.Load()) {
		h.cv.InvalidateAt(t)
	}
}

func (h *host) CanvasInput(ev input.Event) {
	if h.closed || h.r == nil {
		return
	}
	guiprobe.Observe(ev) // no-op unless built explicitly with tango_guiprobe
	// A window shortcut consumes its key (shortcuts.go). The table is
	// matched on the main thread and its functions run there (spec §5.7),
	// so a threaded window matches before queuing, against the UI state of
	// its latest frame (snapshot.go): a consumed key never reaches the UI
	// thread. A press on a title bar's drag area moves the window from
	// here as well.
	// threaded is decided now: queued work may run on main after a
	// fallback (stopStarting) and must not match the key a second time.
	var consumed, rendererInputQueued bool
	threaded := h.t != nil
	if threaded {
		if p, ok := ev.(input.Pointer); ok && p.Kind == input.Press && p.Button == input.ButtonLeft {
			// A native title-bar drag can run a nested event loop. Tell
			// the renderer about the press before entering that loop.
			h.onUI(func() { h.noteRendererInput(ev) })
			rendererInputQueued = true
		}
		if h.dragOnMain(ev) {
			return
		}
		consumed = h.sc.handleSnapshot(ev, h.t.snapshot())
	}
	h.onUI(func() {
		if !rendererInputQueued {
			h.noteRendererInput(ev)
		}
		if !threaded {
			consumed = h.sc.handle(ev) // the main thread is the UI thread
		}
		if !consumed {
			h.r.HandleEvent(ev)
		}
		h.invalidate()
	})
}

// noteRendererInput runs on the window's UI thread, including for
// input consumed before it reaches the runner.
func (h *host) noteRendererInput(ev input.Event) {
	if h.gpu == nil {
		return
	}
	if isKeyboardInput(ev) {
		h.gpu.keyboardInput()
	}
	if isUserDrivenInput(ev) {
		h.gpu.native.noteInput()
	}
}

// CanvasResized only asks for a frame: the runner reads the size and
// scale from the window every frame.
func (h *host) CanvasResized(w, hgt, scale float32) {
	if h.closed {
		return
	}
	if h.t != nil {
		h.t.resizedOnMain(w, hgt, scale)
		return
	}
	h.onUI(func() {
		h.win.resized() // the first frame after a resize is presented whole (damage.go)
		if h.gpu != nil {
			h.gpu.native.noteInput()
		}
		h.invalidate()
	})
}

func (h *host) CanvasFocus(focused bool) {
	if h.closed || h.r == nil {
		return
	}
	threaded := h.t != nil // as in CanvasInput
	if threaded {
		h.sc.handleSnapshot(input.WindowFocus{Focused: focused}, h.t.snapshot()) // main thread, as in CanvasInput
	}
	h.onUI(func() {
		if !threaded {
			h.sc.handle(input.WindowFocus{Focused: focused})
		}
		h.r.HandleEvent(input.WindowFocus{Focused: focused})
		h.invalidate()
	})
}

// WantsUIThread implements platform.CanvasThreadedSink. Open uses the
// main-thread canvas while the dedicated UI thread is under validation.
func (h *host) WantsUIThread() bool { return false }

// onUI runs fn, a sink callback's work, on the window's UI thread: now on
// the main thread, or queued in order on a threaded window's thread.
func (h *host) onUI(fn func()) {
	if h.t != nil {
		h.t.post(h.app, fn)
		return
	}
	fn()
}

// invalidate asks for a frame. Any goroutine on a threaded window, else the
// main thread.
func (h *host) invalidate() {
	if h.t != nil {
		h.win.Invalidate()
		return
	}
	if h.cv != nil {
		h.cv.Invalidate()
	}
}

func (h *host) close() {
	if h.closed {
		return
	}
	h.closed = true
	fallbackRunners.Delete(h.r)
	if h.t != nil {
		h.t.closing.Store(true)
		h.t.embeds.close() // queued placements are dropped (webview_threaded.go)
		if h.ax.postNative != nil {
			h.ax.published.Store(nil)
		}
		h.r.Close() // Post drops tasks from now on; queued frames draw nothing
		renderers.Delete(h.r)
		memRunners.Delete(h.r)
		if h.t.phase.Load() == starting {
			threadedRunners.Delete(h.r)
			delete(h.t.windows, h)
			h.stopStarting(h.t)
		} else {
			h.closeThreaded()
		}
		return
	}
	h.ax.close()
	if h.win != nil {
		h.win.emb.close()
	}
	if h.r != nil {
		h.r.Close()
		renderers.Delete(h.r)
		memRunners.Delete(h.r) // memory.go
	}
	if h.gpu != nil {
		// Free the Metal device, render targets and atlas now; Runner.Close
		// only stops frames. close runs on the main thread.
		_ = h.gpu.Close()
	}
}

// window implements ui.Window on a platform.Canvas.
type window struct {
	app *tango.App
	cv  platform.Canvas
	id  platform.SurfaceID
	gpu *gpuRenderer   // nil: CPU path
	sc  *shortcuts     // nil: no window shortcuts
	ax  *a11yBridge    // accessibility (a11y.go)
	dmg damageState    // partial presentation of CPU frames (damage.go)
	emb *embeds        // embedded WebViews (webview.go); nil: not supported here
	s   *tango.Surface // the window's surface (frame.go); nil for Attach

	t *threadedHost // non-nil: the window has its own UI thread (threaded.go)

	// Threaded geometry and acknowledgment are set together on the UI
	// thread. An in-flight frame cannot consume a later resize's ack.
	size            geom.Size
	scale           float32
	resizePresented chan struct{}
	startup         *threadedHost // immutable once attached; serializes Runner.Post across startup

	// The UI state of a threaded window that its snapshot publishes
	// (snapshot.go); UI thread.
	textOn    bool // text input on (SetTextInput)
	capturing bool // shortcut recording on (SetShortcutCapture)
}

// The ui.Window methods run on the window's UI thread. On a threaded
// window the calls that need the main thread are posted to it (onMain)
// without waiting, except ReadClipboard, which waits for its answer (spec
// §4.2, §5.3).

// onMain runs fn, a call that needs the main thread and whose result
// nobody waits for (cursor, input method, window state, clipboard
// writes): now on a main-thread window, posted to the main thread in call
// order on a threaded one. The last call wins.
func (w *window) onMain(fn func()) {
	if w.t != nil {
		w.app.Post(func() {
			if !w.t.closing.Load() {
				fn()
			}
		})
		return
	}
	fn()
}

func (w *window) Size() geom.Size {
	if w.t != nil {
		return w.size
	}
	x, y := w.cv.Size()
	return geom.Size{W: x, H: y}
}

func (w *window) Scale() float32 {
	if w.t != nil {
		return w.scale
	}
	return w.cv.Scale()
}

func (w *window) Invalidate() {
	if t := w.startup; t != nil {
		if t.closing.Load() {
			return
		}
		switch t.phase.Load() {
		case starting:
			return // handoff asks for the first frame
		case running:
			t.request()
			return
		}
	}
	w.cv.Invalidate()
}

func (w *window) PostUI(fn func()) {
	if w.startup != nil {
		w.startup.dispatch(fn)
		return
	}
	w.app.Post(fn)
}

func (w *window) InvalidateAt(t time.Time) { w.cv.InvalidateAt(t) }
func (w *window) SetCursor(c input.Cursor) { w.onMain(func() { w.cv.SetCursor(c) }) }

// SetShortcutCapture: window shortcuts yield to the recorder from the
// frame that publishes it (threaded windows: the snapshot, snapshot.go),
// native menu accelerators once the main thread applies it.
func (w *window) SetShortcutCapture(on bool) {
	if w.t != nil {
		w.capturing = on
	}
	w.onMain(func() {
		if w.t == nil {
			w.sc.setCapture(on)
		}
		if cv, ok := w.cv.(platform.CanvasShortcutCapture); ok {
			cv.SetShortcutCapture(on)
		}
	})
}

func (w *window) SetTextInput(on bool, caret geom.Rect) {
	if w.t != nil {
		w.textOn = on
	} else {
		w.sc.setText(on)
	}
	w.onMain(func() { w.cv.SetTextInput(on, caret) })
}
func (w *window) Present(img *image.RGBA) error {
	var err error
	switch {
	case img == nil && w.gpu != nil && w.t != nil:
		err = w.gpu.presentFrameAt(w.cv, w.id, w.size, w.scale, true)
	case img == nil && w.gpu != nil:
		err = w.gpu.presentFrame(w.cv, w.id)
	default:
		err = w.presentFull(img) // damage.go
	}
	if w.resizePresented != nil {
		close(w.resizePresented)
		w.resizePresented = nil
	}
	return err
}

// ReadClipboard returns the clipboard's text, "" when it holds none or
// cannot be read. A threaded window's UI thread waits for the main thread
// (clipboard.ReadText awaits it).
func (w *window) ReadClipboard() string {
	s, _ := clipboard.ReadText(w.app)
	return s
}

func (w *window) WriteClipboard(s string) {
	w.onMain(func() { _ = clipboard.WriteText(w.app, s) })
}
