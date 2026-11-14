package tango

// Window capabilities (contract round 2): geometry, window state,
// appearance, page controls, drag regions, creation extras and the events
// that go with them.
//
// Style: methods that change something return an error — ErrUnsupported
// when the platform's surface does not implement the capability, nil
// otherwise (also after the surface closed, when they do nothing, like the
// rest of Surface). Getters return a zero value when unsupported or closed
// (Zoom returns 1).

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is returned when the platform does not support a
// capability. It is platform.ErrUnsupported, so errors.Is matches the
// capability packages' ErrUnsupported too.
var ErrUnsupported = platform.ErrUnsupported

// Rect is a rectangle in logical pixels: X/Y is the window's top-left
// corner on the desktop (origin top-left of the primary display),
// Width/Height the content size (the same size SetSize and Size use).
type Rect = platform.Rect

// WindowState is normal, minimized, maximized or fullscreen.
type WindowState = platform.WindowState

const (
	StateNormal     = platform.StateNormal
	StateMinimized  = platform.StateMinimized
	StateMaximized  = platform.StateMaximized
	StateFullscreen = platform.StateFullscreen
)

// SurfaceKind is the kind of window a surface is created as.
type SurfaceKind = platform.SurfaceKind

const (
	KindWindow = platform.KindWindow // a normal window
	KindPanel  = platform.KindPanel  // floating utility panel that does not activate the app
	KindModal  = platform.KindModal  // modal to SurfaceOptions.Parent
	KindSheet  = platform.KindSheet  // macOS sheet on Parent; KindModal elsewhere
)

// TitleBar is the style of a window's title bar.
type TitleBar = platform.TitleBar

const (
	TitleBarDefault = platform.TitleBarDefault
	// TitleBarHidden extends the content under the title area; the system
	// buttons stay where the platform can keep them (macOS). Elsewhere it is
	// TitleBarNone.
	TitleBarHidden = platform.TitleBarHidden
	// TitleBarNone is a frameless window. Mark draggable areas of the page
	// with the CSS property `--tango-drag: drag` (`no-drag` inside them for
	// buttons); a double click on them toggles maximize. Resizable frameless
	// windows resize from the page's edges.
	TitleBarNone = platform.TitleBarNone
)

// Material is a system backdrop shown behind a transparent page.
type Material = platform.Material

const (
	MaterialNone = platform.MaterialNone
	MaterialBlur = platform.MaterialBlur // macOS vibrancy, Windows acrylic
	MaterialMica = platform.MaterialMica // Windows 11 Mica, macOS window-background vibrancy
)

// Appearance is how a window looks: title bar, transparency, background,
// material, always-on-top, opacity, and the shadow and system buttons. The
// zero value is an ordinary window (the switches are negative: NoShadow,
// NoClose, NoMinimize, NoMaximize). Fields a platform cannot honour are
// ignored.
type Appearance = platform.Appearance

// surfaceExt is the round-2 state of a Surface.
type surfaceExt struct {
	// Main thread only.
	appearance Appearance
	resizable  bool
	edges      bool   // the page's edges resize the window (frameless + resizable)
	edgesSent  bool   // value of edges last sent to the current page
	navSeq     uint64 // bumped by every navigation start; tags liveness probes

	// Guarded by Surface.mu.
	onResized    []func(w, h int)
	onMoved      []func(x, y int)
	onState      []func(WindowState)
	onFiles      []func(paths []string, x, y int)
	onNavigating []func(url string)
	onLoadFailed []func(url, err string)
}

var errNeedParent = errors.New("tango: KindModal and KindSheet need a Parent surface")

// extras derives the creation extras from opts. Main thread.
func (a *App) extras(opts SurfaceOptions) (platform.SurfaceExtras, error) {
	ex := platform.SurfaceExtras{
		Kind:        opts.Kind,
		Bounds:      platform.Rect{X: opts.X, Y: opts.Y, Width: opts.Width, Height: opts.Height},
		Centered:    opts.Centered || (opts.X == 0 && opts.Y == 0),
		MaxWidth:    opts.MaxWidth,
		MaxHeight:   opts.MaxHeight,
		State:       opts.State,
		Appearance:  opts.Appearance,
		AcceptFiles: opts.AcceptFiles,
	}
	if p := opts.Parent; p != nil && p.app == a && !p.closed.Load() {
		ex.Parent = p.id
	}
	if (ex.Kind == KindModal || ex.Kind == KindSheet) && ex.Parent == 0 {
		return ex, errNeedParent
	}
	return ex, nil
}

// newNative creates the native surface, with the extras when the backend
// takes them. Main thread.
func (a *App) newNative(s *Surface, opts SurfaceOptions, po platform.SurfaceOptions) (platform.Surface, error) {
	ex, err := a.prepareNative(s, opts)
	if err != nil {
		return nil, err
	}
	a.handleInternal(frameChannel, (*Surface).handleFrameMsg)
	po.BridgeScript += "\n;" + frameShim
	return a.createNative(s, opts, po, ex) // webview.go
}

// prepareNative derives the creation extras from opts and records the
// window state they set up. Main thread.
func (a *App) prepareNative(s *Surface, opts SurfaceOptions) (platform.SurfaceExtras, error) {
	ex, err := a.extras(opts)
	if err != nil {
		return ex, err
	}
	s.ext.appearance = opts.Appearance
	s.ext.resizable = opts.Resizable
	return ex, nil
}

// afterCreate applies what the backend could not take at creation
// (extrasTaken false: no CreatorEx), through the surface's own
// capabilities. Main thread.
func (s *Surface) afterCreate(opts SurfaceOptions, extrasTaken bool) {
	if !extrasTaken {
		if g, ok := s.ns.(platform.Geometry); ok {
			if !(opts.Centered || (opts.X == 0 && opts.Y == 0)) {
				w, h := s.ns.Size()
				g.SetBounds(platform.Rect{X: opts.X, Y: opts.Y, Width: w, Height: h})
			}
			if opts.MaxWidth > 0 || opts.MaxHeight > 0 {
				g.SetSizeLimits(opts.MinWidth, opts.MinHeight, opts.MaxWidth, opts.MaxHeight)
			}
		}
		if st, ok := s.ns.(platform.Styled); ok && opts.Appearance != (Appearance{}) {
			st.SetAppearance(opts.Appearance)
		}
		if st, ok := s.ns.(platform.Stateful); ok && opts.State != StateNormal {
			st.SetState(opts.State)
		}
	}
	s.updateEdges()
}

// updateEdges recomputes whether the page's edges resize the window and
// tells the page when that changed. Main thread.
func (s *Surface) updateEdges() {
	er, native := s.ns.(platform.EdgeResizer)
	_, dragger := s.ns.(platform.Dragger)
	s.ext.edges = dragger && s.ext.resizable && s.ext.appearance.TitleBar == TitleBarNone &&
		!(native && er.HandlesEdgeResize())
	if s.ext.edges != s.ext.edgesSent {
		s.sendFrame()
	}
}

// sendFrame tells the current page whether to resize from its edges.
func (s *Surface) sendFrame() {
	s.ext.edgesSent = s.ext.edges
	s.ns.Eval("window.__tangoFrame&&window.__tangoFrame(" + strconv.FormatBool(s.ext.edges) + ")")
}

// capDo runs fn with the native surface's capability T on the main thread.
func capDo[T any](s *Surface, fn func(T)) error {
	var err error
	s.do(func(ns platform.Surface) {
		c, ok := ns.(T)
		if !ok {
			err = ErrUnsupported
			return
		}
		fn(c)
	})
	return err
}

// Bounds returns the window's position and content size.
func (s *Surface) Bounds() (r Rect) {
	capDo(s, func(g platform.Geometry) { r = g.Bounds() })
	return
}

// SetBounds moves and resizes the window; r is as Bounds returns it.
func (s *Surface) SetBounds(r Rect) error {
	return capDo(s, func(g platform.Geometry) { g.SetBounds(r) })
}

// Center centers the window on its display.
func (s *Surface) Center() error {
	return capDo(s, platform.Geometry.Center)
}

// SetSizeLimits sets the minimum and maximum content size; 0 means no
// limit.
func (s *Surface) SetSizeLimits(minW, minH, maxW, maxH int) error {
	return capDo(s, func(g platform.Geometry) { g.SetSizeLimits(minW, minH, maxW, maxH) })
}

// State returns the window state.
func (s *Surface) State() (st WindowState) {
	capDo(s, func(c platform.Stateful) { st = c.State() })
	return
}

// SetState minimizes, maximizes, makes fullscreen or restores the window.
func (s *Surface) SetState(st WindowState) error {
	return capDo(s, func(c platform.Stateful) { c.SetState(st) })
}

func (s *Surface) Minimize() error   { return s.SetState(StateMinimized) }
func (s *Surface) Maximize() error   { return s.SetState(StateMaximized) }
func (s *Surface) Fullscreen() error { return s.SetState(StateFullscreen) }
func (s *Surface) Restore() error    { return s.SetState(StateNormal) }

// SetAppearance changes how the window looks. The value is remembered (see
// Appearance) even when the platform cannot apply it.
func (s *Surface) SetAppearance(ap Appearance) error {
	err := ErrUnsupported
	s.do(func(ns platform.Surface) {
		s.ext.appearance = ap
		if st, ok := ns.(platform.Styled); ok {
			st.SetAppearance(ap)
			err = nil
		}
		s.updateEdges()
	})
	if s.closed.Load() && errors.Is(err, ErrUnsupported) {
		return nil
	}
	return err
}

// Appearance returns the appearance last set (at creation or with
// SetAppearance).
func (s *Surface) Appearance() (ap Appearance) {
	s.do(func(platform.Surface) { ap = s.ext.appearance })
	return
}

// Reload reloads the page. Like Navigate, messages sent afterwards wait
// for the reloaded page.
func (s *Surface) Reload() error {
	return capDo(s, func(p platform.PageControl) {
		s.loaded = false
		p.Reload()
	})
}

// SetZoom sets the page zoom; 1 is 100%.
func (s *Surface) SetZoom(factor float64) error {
	if factor <= 0 {
		return errors.New("tango: zoom factor must be positive")
	}
	return capDo(s, func(p platform.PageControl) { p.SetZoom(factor) })
}

// Zoom returns the page zoom (1 when unsupported).
func (s *Surface) Zoom() float64 {
	z := 1.0
	capDo(s, func(p platform.PageControl) { z = p.Zoom() })
	return z
}

// Print opens the system print dialog for the page.
func (s *Surface) Print() error {
	return capDo(s, platform.PageControl.Print)
}

// ---- events ----

// OnResized registers fn for content size changes (logical pixels).
func (s *Surface) OnResized(fn func(w, h int)) {
	s.mu.Lock()
	s.ext.onResized = append(s.ext.onResized, fn)
	s.mu.Unlock()
}

// OnMoved registers fn for window moves; x, y as in Bounds.
func (s *Surface) OnMoved(fn func(x, y int)) {
	s.mu.Lock()
	s.ext.onMoved = append(s.ext.onMoved, fn)
	s.mu.Unlock()
}

// OnStateChanged registers fn for window state changes, by the user or
// by code.
func (s *Surface) OnStateChanged(fn func(WindowState)) {
	s.mu.Lock()
	s.ext.onState = append(s.ext.onState, fn)
	s.mu.Unlock()
}

// OnFilesDropped registers fn for files dropped on the page (only with
// SurfaceOptions.AcceptFiles). x, y is the drop point in page coordinates
// (logical pixels), 0, 0 where the platform does not tell.
func (s *Surface) OnFilesDropped(fn func(paths []string, x, y int)) {
	s.mu.Lock()
	s.ext.onFiles = append(s.ext.onFiles, fn)
	s.mu.Unlock()
}

// OnNavigating registers fn for the start of every main-frame navigation
// (link, reload, Navigate). From then on, messages sent with Send wait for
// the new page.
func (s *Surface) OnNavigating(fn func(url string)) {
	s.mu.Lock()
	s.ext.onNavigating = append(s.ext.onNavigating, fn)
	s.mu.Unlock()
}

// OnLoadFailed registers fn for main-frame navigations that failed.
//
// Messages sent with Send are kept, not dropped: they go to the next page
// that loads, or — when the failed navigation left the previous page in
// place, as WebKit does for a cancelled or failed provisional load — to
// that page as soon as it answers a liveness probe.
func (s *Surface) OnLoadFailed(fn func(url, err string)) {
	s.mu.Lock()
	s.ext.onLoadFailed = append(s.ext.onLoadFailed, fn)
	s.mu.Unlock()
}

func (s *Surface) clearExtHandlers() { // s.mu held
	s.ext.onResized, s.ext.onMoved, s.ext.onState, s.ext.onFiles = nil, nil, nil, nil
	s.ext.onNavigating, s.ext.onLoadFailed = nil, nil
}

func fire[F any](s *Surface, list *[]F, call func(F)) {
	s.mu.Lock()
	fns := slices.Clone(*list)
	s.mu.Unlock()
	for _, fn := range fns {
		s.app.guard(whereEvent, func() { call(fn) })
	}
}

// handleExtEvent handles the surface events not in the round-1 set. Main
// thread.
func (s *Surface) handleExtEvent(ev platform.SurfaceEvent) {
	if s.closed.Load() {
		return
	}
	switch ev.Kind {
	case platform.SurfaceResized:
		fire(s, &s.ext.onResized, func(fn func(int, int)) { fn(ev.Width, ev.Height) })
	case platform.SurfaceMoved:
		fire(s, &s.ext.onMoved, func(fn func(int, int)) { fn(ev.X, ev.Y) })
	case platform.SurfaceStateChanged:
		fire(s, &s.ext.onState, func(fn func(WindowState)) { fn(ev.State) })
	case platform.SurfaceFilesDropped:
		fire(s, &s.ext.onFiles, func(fn func([]string, int, int)) { fn(slices.Clone(ev.Paths), ev.X, ev.Y) })
	case platform.SurfaceNavigating:
		// The current page is about to go away: hold messages until the
		// next one has loaded instead of evaluating them in a page that is
		// unloading.
		s.loaded = false
		// Likewise no fast lane bytes for whatever page comes next until
		// it says hello (lanes.go).
		s.lane.SuspendDown()
		s.ext.navSeq++
		fire(s, &s.ext.onNavigating, func(fn func(string)) { fn(ev.URL) })
	case platform.SurfaceLoadFailed:
		s.ext.navSeq++
		if s.ns != nil {
			// If the previous page is still there it answers; see
			// handleFrameMsg "alive".
			s.ns.Eval("window.__tango&&window.__tango.post(" +
				jsString(corehook.InternalPrefix+frameChannel+":alive:"+strconv.FormatUint(s.ext.navSeq, 10)) + ")")
		}
		fire(s, &s.ext.onLoadFailed, func(fn func(string, string)) { fn(ev.URL, ev.Error) })
	}
}

// onLoadedExt runs on every SurfaceLoaded, before OnLoaded handlers. A new
// document starts with edge resizing off.
func (s *Surface) onLoadedExt() {
	s.ext.edgesSent = false
	if s.ext.edges && s.ns != nil {
		s.sendFrame()
	}
}

// ---- internal messages ----

// frameChannel is the internal-message channel (hooks.go) of
// frameShim and the liveness probe.
const frameChannel = "frame"

var resizeEdges = map[string]bool{"n": true, "s": true, "e": true, "w": true, "ne": true, "nw": true, "se": true, "sw": true}

// handleFrameMsg handles a frame-channel internal message. Main thread;
// the sender's origin is already trusted.
func (s *Surface) handleFrameMsg(cmd string) {
	if s.closed.Load() || s.ns == nil {
		return
	}
	switch {
	case cmd == "move":
		if d, ok := s.ns.(platform.Dragger); ok {
			d.StartMove()
		}
	case strings.HasPrefix(cmd, "resize:"):
		edge := cmd[len("resize:"):]
		if d, ok := s.ns.(platform.Dragger); ok && s.ext.edges && resizeEdges[edge] {
			d.StartResize(edge)
		}
	case cmd == "toggle-max":
		if st, ok := s.ns.(platform.Stateful); ok && !s.ext.appearance.NoMaximize {
			switch st.State() {
			case StateMaximized:
				st.SetState(StateNormal)
			case StateNormal:
				st.SetState(StateMaximized)
			}
		}
	case strings.HasPrefix(cmd, "alive:"):
		seq, err := strconv.ParseUint(cmd[len("alive:"):], 10, 64)
		if err == nil && seq == s.ext.navSeq && !s.loaded {
			s.loaded = true
			s.flush()
		}
	}
}

func jsString(v string) string {
	b, _ := json.Marshal(v)
	return lineSepEscaper.Replace(string(b))
}

// frameShim is added to every surface's bridge script, after the core
// shim. It turns mouse-downs on drag regions into native window moves, a
// double click on them into maximize/restore, and — for frameless
// resizable windows, when the core turns it on with
// window.__tangoFrame(true) — mouse-downs on the page's edges into native
// resizes.
//
// A drag region is an element whose computed `--tango-drag` is "drag"
// (custom properties inherit, so `no-drag` on a descendant cuts a hole);
// `app-region` / `-webkit-app-region` are honoured on the element or an
// ancestor. Double clicks are detected here by time and distance, since a
// native move loop may swallow the second click's detail count.
const frameShim = `(function () {
  "use strict";
  if (window.__tangoFrame || !window.__tango) return;
  var P = "\u0000tango-int:frame:";
  var edges = false, B = 5, last = 0, lx = 0, ly = 0, style = null, cur = "";
  var CUR = { n: "ns-resize", s: "ns-resize", e: "ew-resize", w: "ew-resize",
    ne: "nesw-resize", sw: "nesw-resize", nw: "nwse-resize", se: "nwse-resize" };
  function post(m) { try { window.__tango.post(P + m); } catch (e) {} }
  function region(el) {
    for (var first = true; el && el.nodeType === 1; el = el.parentElement, first = false) {
      var cs = getComputedStyle(el);
      var v = first ? cs.getPropertyValue("--tango-drag").trim() : "";
      if (v === "drag" || v === "no-drag") return v === "drag";
      v = (cs.getPropertyValue("app-region") || cs.getPropertyValue("-webkit-app-region") || "").trim();
      if (v === "drag" || v === "no-drag") return v === "drag";
    }
    return false;
  }
  function edgeAt(e) {
    if (!edges) return "";
    var w = window.innerWidth, h = window.innerHeight;
    return (e.clientY < B ? "n" : e.clientY >= h - B ? "s" : "") +
      (e.clientX < B ? "w" : e.clientX >= w - B ? "e" : "");
  }
  function cursor(c) {
    if (c === cur) return;
    cur = c;
    if (!style) {
      style = document.createElement("style");
      (document.head || document.documentElement).appendChild(style);
    }
    style.textContent = c ? "*{cursor:" + c + "!important}" : "";
  }
  window.addEventListener("mousemove", function (e) {
    var edge = e.buttons ? "" : edgeAt(e);
    cursor(edge ? CUR[edge] : "");
  }, true);
  window.addEventListener("mousedown", function (e) {
    if (e.button !== 0) return;
    var edge = edgeAt(e);
    if (edge) { e.preventDefault(); post("resize:" + edge); return; }
    if (!region(e.target)) return;
    e.preventDefault();
    var now = Date.now();
    if (now - last < 450 && Math.abs(e.screenX - lx) < 5 && Math.abs(e.screenY - ly) < 5) {
      last = 0;
      post("toggle-max");
      return;
    }
    last = now; lx = e.screenX; ly = e.screenY;
    post("move");
  }, true);
  Object.defineProperty(window, "__tangoFrame", { value: function (on) {
    edges = !!on;
    if (!edges) cursor("");
  } });
})();
`
