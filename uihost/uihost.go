// Package uihost shows Tango's self-drawn UI (package ui) in native
// windows: it adapts a tango.App to the ui.Window contract and drives a
// ui.Runner from the backend's canvas callbacks. See
// docs/specs/canvas-surface.md.
//
// The UI thread is the app's main thread: the page function, Runner
// methods and the returned Runner's callbacks all run there.
package uihost

import (
	"errors"
	"image"
	"log"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
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
	h.gpu = setupGPU(cv, ropts) // gpu.go; nil: CPU
	if h.gpu != nil {
		ropts.Renderer = h.gpu
	}
	r := ui.NewRunner(page, ropts)
	h.r = r
	h.id = s.ID()
	h.cv = cv
	h.sc = newShortcuts(app, s) // shortcuts.go
	h.ax = newA11y(app, cv, r, ropts)
	h.win = &window{app: app, cv: cv, id: h.id, gpu: h.gpu, sc: h.sc, ax: h.ax}
	r.Attach(h.win)
	if h.gpu != nil {
		renderers.Store(r, h.gpu)
	}
	s.OnClosed(h.close)
	watchMemory(app, r) // memory.go
	fl.start(r, s)
	automationWatch(app, s, r, h) // TANGO_AUTOMATION (automation_dev.go)
	// Callbacks before Attach found no window; make sure a frame follows.
	cv.Invalidate()
	return r, s, nil
}

// host is the platform.CanvasSink of one window. Main thread only.
type host struct {
	app     *tango.App
	r       *ui.Runner      // nil until the surface exists
	cv      platform.Canvas // nil until the surface exists
	gpu     *gpuRenderer    // nil: CPU path
	id      platform.SurfaceID
	sc      *shortcuts // window shortcuts (shortcuts.go); nil for Attach
	closed  bool
	lastErr string // last frame error logged
	ax      *a11yBridge
	win     *window // the runner's window; nil until attached
}

func (h *host) CanvasFrame() {
	if h.closed || h.cv == nil {
		return
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
	if h.r.Animating() {
		h.cv.Invalidate()
	}
	if t := h.r.NextWake(); !t.IsZero() {
		h.cv.InvalidateAt(t)
	}
}

func (h *host) CanvasInput(ev input.Event) {
	if h.closed || h.r == nil {
		return
	}
	if !h.sc.handle(ev) { // a window shortcut consumes its key (shortcuts.go)
		h.r.HandleEvent(ev)
	}
	h.invalidate()
}

// CanvasResized only asks for a frame: the runner reads the size and
// scale from the window every frame.
func (h *host) CanvasResized(w, hgt, scale float32) {
	if h.closed {
		return
	}
	h.win.resized() // the first frame after a resize is presented whole (damage.go)
	h.invalidate()
}

func (h *host) CanvasFocus(focused bool) {
	if h.closed || h.r == nil {
		return
	}
	h.sc.handle(input.WindowFocus{Focused: focused})
	h.r.HandleEvent(input.WindowFocus{Focused: focused})
	h.invalidate()
}

func (h *host) invalidate() {
	if h.cv != nil {
		h.cv.Invalidate()
	}
}

func (h *host) close() {
	if h.closed {
		return
	}
	h.closed = true
	h.ax.close()
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
	gpu *gpuRenderer // nil: CPU path
	sc  *shortcuts   // nil: no window shortcuts
	ax  *a11yBridge  // accessibility (a11y.go)
	dmg damageState  // partial presentation of CPU frames (damage.go)
}

func (w *window) Size() geom.Size {
	x, y := w.cv.Size()
	return geom.Size{W: x, H: y}
}

func (w *window) Scale() float32           { return w.cv.Scale() }
func (w *window) Invalidate()              { w.cv.Invalidate() }
func (w *window) InvalidateAt(t time.Time) { w.cv.InvalidateAt(t) }
func (w *window) SetCursor(c input.Cursor) { w.cv.SetCursor(c) }
func (w *window) SetTextInput(on bool, caret geom.Rect) {
	w.sc.setText(on)
	w.cv.SetTextInput(on, caret)
}
func (w *window) Present(img *image.RGBA) error {
	if img == nil && w.gpu != nil {
		return w.gpu.presentFrame(w.cv, w.id) // gpu.go
	}
	return w.presentFull(img) // damage.go
}

// ReadClipboard returns the clipboard's text, "" when it holds none or
// cannot be read.
func (w *window) ReadClipboard() string {
	s, _ := clipboard.ReadText(w.app)
	return s
}

func (w *window) WriteClipboard(s string) { _ = clipboard.WriteText(w.app, s) }
