package tango

// Internal hooks: the mechanisms behind what package tango registers in
// internal/corehook, so Tango's own packages (uihost, menu, ipc,
// internal/devfeat, ...) reach core internals without the root package
// exporting them. In order: internal page→Go messages, adjusting options
// before a surface is created, canvas surfaces, canvases attached to
// WebView surfaces, and the hooks of the development features.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// ---------------------------------------------------------------------------
// Internal messages (corehook.HandleInternal)

// Internal messages are page→Go messages the core's own injected scripts
// send (context menus, drag regions, ...):
//
//	"\x00tango-int:" + channel + ":" + payload
//
// They are routed by channel to handlers registered with handleInternal
// and never reach App.OnMessage (nor ipc). An internal message for a
// channel nobody handles is dropped.

type internalRoutes struct {
	mu sync.Mutex
	m  map[string]func(*Surface, string)
}

func init() {
	corehook.HandleInternal = func(app any, channel string, fn func(surface any, payload string)) {
		app.(*App).handleInternal(channel, func(s *Surface, p string) { fn(s, p) })
	}
	corehook.OnSurfaceCreated = func(app any, fn func(surface any)) {
		app.(*App).OnSurfaceCreated(func(s *Surface) { fn(s) })
	}
}

// handleInternal registers fn (main thread) for internal messages on
// channel, replacing any earlier handler. Safe from any goroutine.
func (a *App) handleInternal(channel string, fn func(s *Surface, payload string)) {
	a.intl.mu.Lock()
	if a.intl.m == nil {
		a.intl.m = map[string]func(*Surface, string){}
	}
	a.intl.m[channel] = fn
	a.intl.mu.Unlock()
}

// dispatchInternal reports whether msg was an internal message (handled or
// dropped). Main thread.
func (a *App) dispatchInternal(s *Surface, msg string) bool {
	rest, ok := strings.CutPrefix(msg, corehook.InternalPrefix)
	if !ok {
		return false
	}
	channel, payload, _ := strings.Cut(rest, ":")
	a.intl.mu.Lock()
	fn := a.intl.m[channel]
	a.intl.mu.Unlock()
	if fn != nil {
		a.guard(whereEvent, func() { fn(s, payload) })
	}
	return true
}

// ---------------------------------------------------------------------------
// Preparing surface options

// Internal hooks that adjust SurfaceOptions before the native surface is
// created (OnSurfaceCreated runs only after it exists). Window placement
// across tan dev restarts (internal/devfeat) uses one to put a window
// straight at its saved place instead of moving it after creation, which
// could show it at the default place for a moment.

// onPrepareSurface registers fn to adjust the options of every surface
// (WebView or canvas) about to be created. Main thread when called.
func (a *App) onPrepareSurface(fn func(*SurfaceOptions)) {
	a.mu.Lock()
	a.onPrep = append(a.onPrep, fn)
	a.mu.Unlock()
}

// prepareOptions runs the onPrepareSurface hooks on a copy of opts. Main
// thread.
func (a *App) prepareOptions(opts SurfaceOptions) SurfaceOptions {
	a.mu.Lock()
	fns := slices.Clone(a.onPrep)
	a.mu.Unlock()
	for _, fn := range fns {
		a.guard(whereEvent, func() { fn(&opts) })
	}
	return opts
}

// ---------------------------------------------------------------------------
// Canvas surfaces (corehook.NewCanvasSurface)

// Canvas surfaces: windows drawn by package ui instead of a WebView,
// created through corehook.NewCanvasSurface by package uihost. See
// docs/specs/canvas-surface.md.

var (
	errCanvasNotMain = errors.New("tango: canvas surfaces must be created on the main thread")
	errCanvasClosed  = errors.New("tango: canvas surface closed while being created")
)

func init() {
	corehook.NewCanvasSurface = func(app any, opts any, sink platform.CanvasSink) (any, platform.Canvas, error) {
		s, cv, err := app.(*App).createCanvasSurface(opts.(SurfaceOptions), sink)
		if err != nil {
			return nil, nil, err
		}
		return s, cv, nil
	}
}

// createCanvasSurface is createSurface for a canvas surface. Main thread.
func (a *App) createCanvasSurface(opts SurfaceOptions, sink platform.CanvasSink) (*Surface, platform.Canvas, error) {
	if a.b == nil {
		return nil, nil, ErrNoBackend
	}
	if !a.isMain() {
		return nil, nil, errCanvasNotMain
	}
	cc, ok := a.b.(platform.CanvasCreator)
	if !ok {
		return nil, nil, fmt.Errorf("tango: canvas surfaces: %w", ErrUnsupported)
	}
	opts = a.prepareOptions(opts) // see onPrepareSurface
	var cv platform.Canvas
	s, err := a.addSurface(opts, "", true, func(s *Surface) (platform.Surface, error) {
		ex, err := a.prepareNative(s, opts)
		if err != nil {
			return nil, err
		}
		ns, c, err := cc.NewCanvasSurface(s.id, platform.SurfaceOptions{
			Title:     opts.Title,
			Width:     opts.Width,
			Height:    opts.Height,
			MinWidth:  opts.MinWidth,
			MinHeight: opts.MinHeight,
			Resizable: opts.Resizable,
			Visible:   !opts.Hidden,
		}, ex, canvasSink{a, sink})
		cv = c
		return ns, err
	})
	if err != nil {
		return nil, nil, err
	}
	if s.closed.Load() {
		// OnClosed handlers registered from now on would never run.
		return nil, nil, errCanvasClosed
	}
	return s, cv, nil
}

// canvasSink marks the calling goroutine as the main thread for the
// duration of each callback, as host does, so code run from the sink can
// use the App's main-thread fast paths (Await, Surface methods). A panic in
// a callback is reported (panic.go) instead of crashing the native loop.
type canvasSink struct {
	a    *App
	sink platform.CanvasSink
}

func (c canvasSink) CanvasFrame() {
	defer c.a.enterMain()()
	c.a.guard(whereCanvas, c.sink.CanvasFrame)
}

func (c canvasSink) CanvasInput(ev input.Event) {
	defer c.a.enterMain()()
	c.a.guard(whereCanvas, func() { c.sink.CanvasInput(ev) })
}

func (c canvasSink) CanvasResized(w, h, scale float32) {
	defer c.a.enterMain()()
	c.a.guard(whereCanvas, func() { c.sink.CanvasResized(w, h, scale) })
}

func (c canvasSink) CanvasFocus(focused bool) {
	defer c.a.enterMain()()
	c.a.guard(whereCanvas, func() { c.sink.CanvasFocus(focused) })
}

// CanvasDragOver and CanvasDragLeave forward platform.CanvasDropTarget to
// the wrapped sink when it implements it.
func (c canvasSink) CanvasDragOver(x, y float32) {
	if d, ok := c.sink.(platform.CanvasDropTarget); ok {
		defer c.a.enterMain()()
		c.a.guard(whereCanvas, func() { d.CanvasDragOver(x, y) })
	}
}

func (c canvasSink) CanvasDragLeave() {
	if d, ok := c.sink.(platform.CanvasDropTarget); ok {
		defer c.a.enterMain()()
		c.a.guard(whereCanvas, d.CanvasDragLeave)
	}
}

var errCanvasNoA11y = errors.New("tango: canvas sink has no accessibility actions")

// AccessibilityAction forwards platform.CanvasAccessibleSink to the
// wrapped sink when it implements it (docs/specs/a11y-bridge.md).
func (c canvasSink) AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error {
	as, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return errCanvasNoA11y
	}
	defer c.a.enterMain()()
	var err error
	c.a.guard(whereCanvas, func() { err = as.AccessibilityAction(id, act, arg) })
	return err
}

// ---------------------------------------------------------------------------
// Canvases attached to WebView surfaces (corehook.AttachCanvas)

// Canvases attached to WebView surfaces (docs/specs/canvas-surface.md,
// step D), reached through corehook.AttachCanvas by package uihost.

var (
	errAttachNotMain = errors.New("tango: canvases must be attached on the main thread")
	errAttachClosed  = errors.New("tango: surface is closed")
)

func init() {
	corehook.AttachCanvas = func(surface any, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
		s, ok := surface.(*Surface)
		if !ok || s == nil {
			return nil, errors.New("tango: AttachCanvas: not a *tango.Surface")
		}
		return s.attachCanvas(layer, sink)
	}
}

// attachCanvas asks the backend for a canvas inside s. Main thread.
func (s *Surface) attachCanvas(layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	a := s.app
	if a.b == nil {
		return nil, ErrNoBackend
	}
	if !a.isMain() {
		return nil, errAttachNotMain
	}
	if s.closed.Load() || s.ns == nil {
		return nil, errAttachClosed
	}
	ca, ok := a.b.(platform.CanvasAttacher)
	if !ok {
		return nil, fmt.Errorf("tango: attach canvas: %w", ErrUnsupported)
	}
	return ca.AttachCanvas(s.id, layer, canvasSink{a, sink})
}

// ---------------------------------------------------------------------------
// Development features (internal/devfeat)

// Hooks of the development features (internal/devfeat: window placement
// across tan dev restarts, the automation channel). They are registered in
// every build but only called in builds with -tags tango_dev.
func init() {
	corehook.OnPrepareSurface = func(app any, fn func(p *corehook.SurfacePlacement)) {
		app.(*App).onPrepareSurface(func(o *SurfaceOptions) {
			p := corehook.SurfacePlacement{
				Title: o.Title,
				X:     o.X, Y: o.Y, Width: o.Width, Height: o.Height,
				Centered: o.Centered,
				State:    o.State,
			}
			fn(&p)
			o.X, o.Y, o.Width, o.Height = p.X, p.Y, p.Width, p.Height
			o.Centered, o.State = p.Centered, p.State
		})
	}
	corehook.Await = func(app any, fn func()) bool { return app.(*App).awaitVoid(fn) }
	corehook.Done = func(app any) <-chan struct{} { return app.(*App).closedCh }
	corehook.Surfaces = func(app any) []any {
		ss := app.(*App).Surfaces()
		out := make([]any, len(ss))
		for i, s := range ss {
			out[i] = s
		}
		return out
	}
	corehook.SurfaceOf = func(app any, id platform.SurfaceID) any {
		if s := app.(*App).surface(id); s != nil {
			return s
		}
		return nil // not a typed nil *Surface
	}
	corehook.SurfaceClosed = func(surface any) bool { return surface.(*Surface).closed.Load() }
	corehook.SurfaceLoaded = func(surface any) bool { return surface.(*Surface).loaded }
}
