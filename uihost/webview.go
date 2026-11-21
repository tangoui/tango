package uihost

// Native WebViews embedded in the layout of a UI window
// (docs/specs/webview-embed.md). WebView reserves a box with
// ui.NativeView; after every frame the window places each embedded
// WebView (a child surface of the window) on its box: position, size and
// the visible part left by scroll containers and clipping ancestors. A
// WebView whose box is covered by a layer (dropdown, tooltip, dialog) is
// replaced by a snapshot drawn in the canvas while the layer is open.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// CoverMode says what an embedded WebView does while a layer of the UI
// (dropdown, tooltip, popover, dialog, message) overlaps its box. A
// native view is always drawn above the canvas, so without a strategy it
// would hide the layer.
type CoverMode uint8

const (
	// CoverSnapshot (the zero value) freezes the WebView: the window
	// takes a snapshot, draws it in the canvas under the layer and hides
	// the native view until nothing covers the box any more. The page
	// keeps running but its updates do not show while frozen. Until the
	// first snapshot arrives (a few milliseconds) the native view stays
	// on top; if the platform cannot take snapshots it behaves like
	// CoverHide.
	CoverSnapshot CoverMode = iota
	// CoverHide hides the native view while covered; the box shows what
	// the canvas draws there (its Bg, Paint).
	CoverHide
	// CoverIgnore keeps the native view on top of everything.
	CoverIgnore
)

// WebViewOptions configure an embedded WebView.
type WebViewOptions struct {
	// URL is the first page, resolved like SurfaceOptions.URL (relative
	// paths load from the app's assets). Changing it later has no effect:
	// navigate with Surface().Navigate.
	URL string
	// Debug allows the inspector.
	Debug bool
	// WebView configures the user agent, headers, browsing mode and profile.
	WebView tango.WebViewOptions
	// Adopt places an existing native popup from the same canvas.
	// The box takes ownership and URL, Debug, and WebView are ignored.
	Adopt *tango.Surface
	// Created runs once on the main thread right after the WebView's
	// surface exists, before its first frame shows: register OnMessage
	// handlers, ipc, OnLoaded there.
	Created func(s *tango.Surface)
	// KeepAlive keeps the WebView, hidden, for this long after its box
	// stopped being built, so that it comes back with its page state
	// (tabs, collapsible panels). Zero closes it at the end of the first
	// frame without its box.
	KeepAlive time.Duration
	// Covered chooses the strategy while a layer overlaps the box.
	Covered CoverMode
}

// WebViewW is the builder of WebView. The embedded ui.NativeViewW styles
// the box (Bg, Radius, ...) drawn where the native view does not show.
type WebViewW struct {
	*ui.NativeViewW
	v *embedView
	e *embeds
}

// ErrNoEmbedding is reported by WebViewW.Err in windows that cannot host
// embedded WebViews, such as a headless runner.
var ErrNoEmbedding = fmt.Errorf("uihost: the window cannot host embedded WebViews: %w", tango.ErrUnsupported)

// WebView embeds a native WebView in the layout. key identifies it in its
// scope (like ui.Key): building WebView with the same key in later frames
// keeps the same WebView. The WebView is created after the first frame
// that builds it (WebViewOptions.Created reports it) and follows its box
// every frame: scrolling, animation, window resizing and scale changes.
// It is hidden while its box is scrolled or clipped out of view and is
// closed when its box is no longer built (see KeepAlive); it closes with
// the window.
//
// Focus: the box is a Tab stop. Tabbing onto it gives the page keyboard
// focus (its first focusable element; the last one with Shift+Tab);
// clicking the page focuses it too, and the UI's focus follows to the
// box. Tab past the page's last focusable element (Shift+Tab before the
// first) or F6 / Shift+F6 return focus to the UI, to the control after
// (before) the box. Key events go to whichever side has focus, never to
// both.
//
// WebView works with Open and with Attach on platforms that support child
// WebViews. On other hosts the box shows its background and Err reports
// the reason. Call from a page function on the window's UI thread.
//
// Experimental: see docs/release.md.
func WebView(c *ui.Context, key string, opts WebViewOptions, lopts ...layout.Option) *WebViewW {
	nv := ui.NativeView(c, key, lopts...)
	nv.Bg(c.Theme.BgContainer)
	w := &WebViewW{NativeViewW: nv}
	host, ok := c.Window().(interface{ webEmbeds() *embeds })
	if !ok || host.webEmbeds() == nil {
		return w
	}
	e := host.webEmbeds()
	w.e = e
	id := nv.ID()
	v := e.views[id]
	if v == nil {
		v = &embedView{e: e, id: id}
		e.views[id] = v
	}
	w.v = v
	v.opts = opts
	v.built = e.frame
	v.manual = false
	if v.webFocused && c.Focused() != id {
		c.SetFocus(id) // the page took focus (click): the UI's focus follows
	}
	// The UI's focus moved onto the box: the next sync hands keyboard
	// focus to the page, to its first focusable element (the last one
	// after Shift+Tab) unless a click on the box moved it.
	if f := c.Focused() == id; f != v.uiFocusedBuild {
		v.uiFocusedBuild = f
		if f && !v.webFocused {
			v.enter = true
			v.enterKeyboard = !nv.Clicked() && !c.Dragging(id)
			v.enterLast = c.PointerMods().Has(input.ModShift)
		}
	}
	nv.Paint(func(l *paint.List, r geom.Rect) {
		if v.covered && v.snap != nil {
			l.DrawImage(v.snap, r, geom.Radii{})
			v.snapDrawn = e.frame
		}
	})
	return w
}

// EmbeddedIn returns the window an embedded WebView surface (from
// WebViewW.Surface) lives in, or nil when s is not an embedded WebView.
//
// Experimental: see docs/release.md.
func EmbeddedIn(s *tango.Surface) *tango.Surface {
	if s == nil || corehook.EmbeddedParent == nil {
		return nil
	}
	p, _ := corehook.EmbeddedParent(s).(*tango.Surface)
	return p
}

// Surface returns the WebView's surface (Navigate, Eval, Send, OnMessage
// through App.OnMessage, the WebView extensions), or nil before it exists
// (the first frame) or when it could not be created (see Err).
func (w *WebViewW) Surface() *tango.Surface {
	if w.v == nil {
		return nil
	}
	return w.v.s
}

// Err reports why the WebView does not exist: ErrNoEmbedding, an error
// wrapping tango.ErrUnsupported when the platform cannot embed WebViews,
// or a creation error. nil while it exists or is about to be created.
func (w *WebViewW) Err() error {
	if w.v == nil {
		return ErrNoEmbedding
	}
	return w.v.err
}

// Frozen reports whether the WebView is currently replaced by its
// snapshot (or hidden) because something covers its box.
func (w *WebViewW) Frozen() bool { return w.v != nil && w.v.frozen }

// Freeze treats the box as covered this frame (see CoverMode), for
// content the window cannot see as a layer: something drawn over the box
// in the page itself, a drag preview.
func (w *WebViewW) Freeze(on bool) *WebViewW {
	if w.v != nil && on {
		w.v.manual = true
	}
	return w
}

// ---------------------------------------------------------------------------

// embedChannel is the internal message channel of the focus script.
const embedChannel = "webembed"

// embedFocusScript runs in every embedded page: Tab past the last
// focusable element (Shift+Tab before the first) and F6 / Shift+F6 hand
// focus back to the UI; __tangoEmbedFocus(last) focuses the first or
// last focusable element when the UI tabs into the page.
const embedFocusScript = `(function(){
var P=function(d){try{window.__tangoNative.post("\u0000tango-int:` + embedChannel + `:"+d)}catch(e){}};
var S='a[href],area[href],button:not([disabled]),input:not([disabled]):not([type=hidden]),select:not([disabled]),textarea:not([disabled]),iframe,summary,[contenteditable=""],[contenteditable=true],[tabindex]';
function L(){return Array.prototype.filter.call(document.querySelectorAll(S),function(e){var t=e.getAttribute('tabindex');if(t!==null&&+t<0)return false;if(!e.getClientRects().length)return false;var s=getComputedStyle(e);return s.visibility!=='hidden'&&s.display!=='none'})}
addEventListener('keydown',function(e){if(e.ctrlKey||e.metaKey||e.altKey)return;
if(e.key==='F6'){e.preventDefault();P(e.shiftKey?'prev':'next');return}
if(e.key!=='Tab'||e.defaultPrevented)return;
var l=L(),i=l.indexOf(document.activeElement);
if(!l.length||(e.shiftKey?i===0:i===l.length-1)){e.preventDefault();P(e.shiftKey?'prev':'next')}},true);
window.__tangoEmbedFocus=function(last){var l=L(),el=last?l[l.length-1]:l[0];if(el)el.focus()};
})();`

// embeds holds the embedded WebViews of one window, on its UI thread: the
// main thread, or a threaded window's own UI thread (main non-nil), where
// the native views live on main in main (webview_threaded.go).
type embeds struct {
	h            *host
	parent       *tango.Surface
	canvas       platform.Canvas // non-nil when embedded in an attached layer
	r            *ui.Runner
	invalidate   func()
	invalidateAt func(time.Time)
	views        map[ui.ID]*embedView
	frame        uint64 // frames synced so far; builds mark views with it
	closed       bool
	main         *mainEmbeds // threaded window: the native side
	idle         bool        // threaded: the last frame posted to main was empty (or none was)
}

// embedView is one embedded WebView, on the window's UI thread. On a
// threaded window s and err mirror the native side, ev stays nil.
type embedView struct {
	e     *embeds
	id    ui.ID
	opts  WebViewOptions
	built uint64 // e.frame of the last build that had the box

	s   *tango.Surface
	ev  platform.EmbeddedWebView
	err error

	lastSeen   time.Time
	manual     bool // Freeze(true) this frame
	covered    bool // the last sync found the box covered
	frozen     bool // the native view is hidden in favour of the canvas
	snap       image.Image
	snapDrawn  uint64 // e.frame in which the box drew snap
	snapWait   bool   // a snapshot is on its way
	snapFailed bool
	webFocused bool // the page has keyboard focus (SurfaceFocused)

	uiFocusedBuild bool // the box had the UI's focus in the last build
	enter          bool // the UI's focus moved onto the box: focus the page
	enterKeyboard  bool // ... by keyboard (Tab) or program, not a click
	enterLast      bool // ... backwards (Shift+Tab): focus the last element
}

// bySurface maps embedded surfaces to their views for the focus
// script's messages. Main thread.
var bySurface = map[*tango.Surface]focusLeaver{}

// focusLeaver handles the focus script's request to leave the page.
type focusLeaver interface{ focusOut(back bool) }

func newEmbeds(h *host, parent *tango.Surface) *embeds {
	e := newWindowEmbeds(parent, h.r, h.invalidate, h.cv.InvalidateAt)
	e.h = h
	return e
}

func newWindowEmbeds(parent *tango.Surface, r *ui.Runner, invalidate func(), at func(time.Time)) *embeds {
	corehook.HandleInternal(parent.App(), embedChannel, func(surface any, payload string) {
		s, _ := surface.(*tango.Surface)
		if v := bySurface[s]; v != nil {
			v.focusOut(payload == "prev")
		}
	})
	return &embeds{parent: parent, r: r, invalidate: invalidate, invalidateAt: at, views: map[ui.ID]*embedView{}}
}

func (w *window) webEmbeds() *embeds { return w.emb }

// sync places every embedded WebView after a frame.
func (e *embeds) sync() {
	if e == nil || e.closed {
		return
	}
	if e.main != nil {
		e.syncThreaded() // webview_threaded.go
		return
	}
	now := time.Now()
	regs := e.runner().NativeRegions()
	for _, reg := range regs {
		v := e.views[reg.ID]
		if v == nil || v.built != e.frame {
			continue // a native view that is not a WebView
		}
		v.lastSeen = now
		if v.s == nil && v.err == nil {
			v.create()
		}
		if v.ev == nil {
			continue
		}
		v.place(reg)
	}
	for id, v := range e.views {
		if v.built == e.frame {
			continue
		}
		if v.ev != nil {
			corehook.CommitWebPlacement(v.s, v.ev, platform.WebPlacement{})
		}
		v.covered, v.frozen, v.snap = false, false, nil
		if keep := v.opts.KeepAlive; keep > 0 && v.s != nil && now.Sub(v.lastSeen) < keep {
			e.invalidateAt(v.lastSeen.Add(keep))
			continue
		}
		delete(e.views, id)
		v.close()
	}
	e.frame++
}

func (e *embeds) runner() *ui.Runner {
	if e.h != nil {
		return e.h.r
	}
	return e.r
}

// place applies one frame's region to the view.
func (v *embedView) place(reg ui.NativeRegion) {
	p := v.decide(reg)
	corehook.CommitWebPlacement(v.s, v.ev, p.placement)
	if p.focus && v.e.parent.IsFocused() {
		v.ev.FocusWeb()
		if p.keyboard {
			v.s.Eval(enterScript(p.last))
		}
	}
}

// embedPlace is what one frame asks of a native view: where it goes and
// whether it takes the keyboard focus.
type embedPlace struct {
	id        ui.ID
	v         *embedView     // the UI side (identity only off the UI thread)
	opts      WebViewOptions // to create it
	placement platform.WebPlacement
	focus     bool // the UI's focus moved onto the box: focus the page
	keyboard  bool // ... by keyboard: focus its first (last) element
	last      bool
}

func enterScript(last bool) string {
	return fmt.Sprintf("window.__tangoEmbedFocus&&window.__tangoEmbedFocus(%t)", last)
}

// decide works out one frame's placement of the view from its region and
// cover state. The window's UI thread.
func (v *embedView) decide(reg ui.NativeRegion) embedPlace {
	covered := (reg.Covered || v.manual) && !reg.Visible.Empty()
	hide := false
	if covered {
		switch v.opts.Covered {
		case CoverHide:
			hide = true
		case CoverSnapshot:
			switch {
			case v.snap != nil && v.snapDrawn == v.e.frame:
				hide = true // the canvas shows the snapshot under the layer
			case v.snapFailed:
				hide = true
			case v.snap == nil && !v.snapWait && v.s != nil:
				v.takeSnapshot()
			}
		}
	} else if v.snap != nil {
		v.snap = nil // the next freeze takes a fresh one
	}
	if !covered {
		v.snapFailed = false
	}
	v.covered = covered && v.opts.Covered == CoverSnapshot
	v.frozen = hide
	clip := reg.Visible
	if hide {
		clip = geom.Rect{}
	}
	p := embedPlace{id: v.id, v: v, opts: v.opts, placement: platform.WebPlacement{Frame: rectF(reg.Rect), Clip: rectF(clip)}}
	if v.enter {
		v.enter = false
		if reg.Focused && !v.webFocused && !clip.Empty() {
			p.focus, p.keyboard, p.last = true, v.enterKeyboard, v.enterLast
		}
	}
	return p
}

func rectF(r geom.Rect) platform.RectF {
	return platform.RectF{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)}
}

func createWebView(parent *tango.Surface, canvas platform.Canvas, opts WebViewOptions) (*tango.Surface, platform.EmbeddedWebView, error) {
	if s := opts.Adopt; s != nil {
		if bySurface[s] != nil {
			return nil, nil, errors.New("uihost: WebView is already placed")
		}
		ev, err := corehook.AdoptEmbeddedWebView(parent, canvas, s)
		return s, ev, err
	}
	s, ev, err := corehook.NewEmbeddedWebView(parent, canvas, tango.SurfaceOptions{
		URL: opts.URL, Debug: opts.Debug, WebView: opts.WebView,
	}, embedFocusScript)
	if err != nil {
		return nil, nil, err
	}
	return s.(*tango.Surface), ev, nil
}

// create makes the WebView's surface.
func (v *embedView) create() {
	s, ev, err := createWebView(v.e.parent, v.e.canvas, v.opts)
	if err != nil {
		v.err = err
		return
	}
	v.s, v.ev = s, ev
	bySurface[s] = v
	e := v.e
	s.OnFocused(func() { v.webFocused = true; e.invalidate() })
	s.OnBlurred(func() { v.webFocused = false; e.invalidate() })
	s.OnClosed(func() {
		delete(bySurface, s)
		if v.s == s && !v.e.closed {
			// Closed by the app (Surface.Close): forget it; a new one is
			// created if the box is still built.
			v.s, v.ev, v.webFocused = nil, nil, false
			if ev, ok := v.e.views[v.id]; ok && ev == v {
				delete(v.e.views, v.id)
			}
			e.invalidate()
		}
	})
	if fn := v.opts.Created; fn != nil {
		corehook.Guard(e.parent.App(), "uihost WebView Created", func() { fn(s) })
	}
}

// takeSnapshot asks the page for a snapshot off the main thread
// (Surface.Screenshot waits for the platform) and frames again when it
// arrives.
func (v *embedView) takeSnapshot() {
	s := v.s
	v.snapWait = true
	go func() {
		data, err := s.Screenshot()
		var img image.Image
		if err == nil {
			img, err = png.Decode(bytes.NewReader(data))
		}
		v.e.toUI(func() {
			v.snapWait = false
			if v.s != s {
				return
			}
			if err != nil {
				v.snapFailed = true
			} else if v.covered {
				v.snap = img
			}
			v.e.invalidate()
		})
	}()
}

// focusOut handles the focus script's request to leave the page.
func (v *embedView) focusOut(back bool) {
	if v.ev == nil || v.e.closed {
		return
	}
	v.ev.BlurWeb()
	mods := input.Modifiers(0)
	if back {
		mods = input.ModShift
	}
	r := v.e.runner()
	r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true, Mods: mods})
	r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Mods: mods})
	v.e.invalidate()
}

func (v *embedView) close() {
	if v.s == nil {
		return
	}
	s := v.s
	v.s, v.ev = nil, nil
	delete(bySurface, s)
	s.Close()
}

// close releases the WebViews when the window or its attached canvas closes.
func (e *embeds) close() {
	if e == nil || e.closed {
		return
	}
	e.closed = true
	for _, v := range e.views {
		v.close()
	}
	e.views = nil
}
