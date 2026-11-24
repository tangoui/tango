package uihost

// Application native views embedded in a UI window's layout
// (docs/specs/webview-embed.md, "应用原生视图"). NativeHost reserves a box
// with ui.NativeView like WebView does; after every frame the window places
// the application's view on its box. The UI thread decides the placements
// of a frame and the main thread applies them: directly when the window's
// UI thread is the main thread, posted (the latest frame wins) when the
// window is threaded.

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// NativeHostOptions configure a NativeHost.
type NativeHostOptions struct {
	// Create makes the application's native view, on the main thread,
	// after the first frame that builds the box: an NSView* on macOS, a
	// child-capable HWND on Windows, a GtkWidget* on Linux (hold a
	// reference: the window only parents it). It runs once per box
	// lifetime; an error is reported by Err and not retried.
	Create func() (view uintptr, err error)
	// Release runs on the main thread once the window no longer shows
	// the view: its box stopped being built (after KeepAlive), the window
	// closed, or embedding failed. The view is already removed from the
	// window; destroy it here.
	Release func(view uintptr)
	// PassInput lets mouse events over the view reach the UI below, as if
	// the view were not there (video, remote screens). The box is then not
	// a Tab stop. Without it the view takes the input over its area.
	PassInput bool
	// KeepAlive keeps the view, hidden, for this long after its box
	// stopped being built, so that it comes back without a new Create.
	KeepAlive time.Duration
	// Covered chooses what happens while a layer of the UI overlaps the
	// box. Native views have no snapshot: CoverSnapshot (the zero value)
	// behaves like CoverHide.
	Covered CoverMode
}

// NativeHostW is the builder of NativeHost. The embedded ui.NativeViewW
// styles the box (Bg, Radius, ...) drawn where the native view does not
// show.
type NativeHostW struct {
	*ui.NativeViewW
	h *nativeHost
}

// ErrNoNativeHost is reported by NativeHostW.Err in windows that cannot
// host native views, such as a headless runner or an attached layer.
var ErrNoNativeHost = fmt.Errorf("uihost: the window cannot host native views: %w", tango.ErrUnsupported)

// NativeHost embeds a native view made by the application in the layout.
// key identifies it in its scope (like ui.Key): building NativeHost with
// the same key in later frames keeps the same view. The view is created
// (NativeHostOptions.Create) after the first frame that builds the box and
// follows the box every frame: scrolling, animation, window resizing and
// scale changes. It is hidden while the box is scrolled or clipped out of
// view and released (NativeHostOptions.Release) when the box is no longer
// built (see KeepAlive) or the window closes.
//
// Use it for content the system must compose itself, such as hardware
// decoded video. The view sits above the canvas: UI layers drawn over the
// box hide it (see Covered); native menus (sys/menu) show above it.
//
// NativeHost works with Open on platforms that support native child views.
// On other hosts the box shows its background and Err reports the reason.
// Call from a page function on the window's UI thread.
//
// Experimental: see docs/release.md.
func NativeHost(c *ui.Context, key string, opts NativeHostOptions, lopts ...layout.Option) *NativeHostW {
	nv := ui.NativeView(c, key, lopts...)
	nv.Bg(c.Theme.BgContainer)
	if opts.PassInput {
		nv.Focusable(false)
	}
	w := &NativeHostW{NativeViewW: nv}
	host, ok := c.Window().(interface{ webEmbeds() *embeds })
	if !ok || host.webEmbeds() == nil || host.webEmbeds().canvas != nil {
		return w
	}
	e := host.webEmbeds()
	if e.natives == nil {
		e.natives = map[ui.ID]*nativeHost{}
	}
	id := nv.ID()
	h := e.natives[id]
	if h == nil {
		h = &nativeHost{id: id}
		e.natives[id] = h
	}
	h.opts = opts
	h.built = e.frame
	w.h = h
	return w
}

// Err reports why the view is not shown: the window cannot host native
// views (ErrNoNativeHost), Create failed, or the platform refused the
// view. Nil while it is shown or not created yet.
func (w *NativeHostW) Err() error {
	if w.h == nil {
		return ErrNoNativeHost
	}
	return w.h.err
}

// nativeHost is one NativeHost box, on the window's UI thread.
type nativeHost struct {
	id       ui.ID
	opts     NativeHostOptions
	built    uint64 // embeds.frame of the last build that had the box
	lastSeen time.Time
	err      error
	failed   bool // Create or embedding failed: never retried
}

// nativePlace is one frame's placement of a native view.
type nativePlace struct {
	id    ui.ID
	h     *nativeHost // identity: a new box with the same key is a new view
	opts  NativeHostOptions
	place platform.WebPlacement
}

// decide works out the view's placement from its region. UI thread.
func (h *nativeHost) decide(reg ui.NativeRegion) platform.WebPlacement {
	clip := reg.Visible
	if reg.Covered && h.opts.Covered != CoverIgnore {
		clip = geom.Rect{}
	}
	return platform.WebPlacement{Frame: rectF(reg.Rect), Clip: rectF(clip)}
}

// syncNatives decides this frame's native view placements and hands them
// to the main thread. UI thread, before e.frame advances.
func (e *embeds) syncNatives() {
	if len(e.natives) == 0 && e.nativeIdle {
		return
	}
	now := time.Now()
	var list []nativePlace
	for _, reg := range e.runner().NativeRegions() {
		h := e.natives[reg.ID]
		if h == nil || h.built != e.frame || h.failed {
			continue
		}
		h.lastSeen = now
		list = append(list, nativePlace{id: reg.ID, h: h, opts: h.opts, place: h.decide(reg)})
	}
	for id, h := range e.natives {
		if h.built == e.frame {
			continue
		}
		if keep := h.opts.KeepAlive; keep > 0 && !h.failed && now.Sub(h.lastSeen) < keep {
			e.invalidateAt(h.lastSeen.Add(keep))
			list = append(list, nativePlace{id: id, h: h, opts: h.opts}) // hidden
			continue
		}
		delete(e.natives, id)
	}
	e.nativeIdle = len(list) == 0
	e.nm.submit(list)
}

// newNativeMain creates the main-thread side of e's native views. post is
// nil when the window's UI thread is the main thread. Main thread, while
// the window is created.
func newNativeMain(e *embeds, post func(func())) *nativeMain {
	return &nativeMain{parent: e.parent, post: post, views: map[ui.ID]*mainNative{}, toUI: e.toUI, invalidate: e.invalidate}
}

// closeNatives releases the native views when the window closes. Main
// thread.
func (e *embeds) closeNatives() {
	if e.nm != nil {
		e.nm.closeAll()
	}
	e.natives = nil
}

// nativeMain is the main-thread side of a window's native views.
type nativeMain struct {
	parent     *tango.Surface
	post       func(func()) // threaded window: queue a task on main; nil runs in place
	toUI       func(func())
	invalidate func()

	mu      sync.Mutex
	pending []nativePlace
	posted  bool

	// Main thread.
	views  map[ui.ID]*mainNative
	failed map[*nativeHost]bool // Create or embedding failed: not retried
	closed bool
}

// mainNative is one embedded native view on the main thread.
type mainNative struct {
	h        *nativeHost
	view     uintptr
	ev       platform.EmbeddedNativeView
	release  func(uintptr)
	released bool
}

// submit hands a frame's placements to the main thread. UI thread.
func (m *nativeMain) submit(list []nativePlace) {
	if m.post == nil {
		m.apply(list)
		return
	}
	m.mu.Lock()
	m.pending = list
	queue := !m.posted
	m.posted = true
	m.mu.Unlock()
	if queue {
		m.post(m.applyPending)
	}
}

func (m *nativeMain) applyPending() {
	m.mu.Lock()
	list := m.pending
	m.pending, m.posted = nil, false
	m.mu.Unlock()
	m.apply(list)
}

// apply creates, places and removes the views as the frame asks. Main
// thread.
func (m *nativeMain) apply(list []nativePlace) {
	if m.closed {
		return
	}
	kept := make(map[ui.ID]bool, len(list))
	for _, p := range list {
		mv := m.views[p.id]
		if mv != nil && mv.h != p.h { // a new box with the same key
			m.remove(p.id, mv)
			mv = nil
		}
		if mv == nil {
			if m.failed[p.h] {
				continue
			}
			if mv = m.create(p); mv == nil {
				continue
			}
		}
		kept[p.id] = true
		mv.ev.SetPlacement(p.place)
	}
	for id, mv := range m.views {
		if !kept[id] {
			m.remove(id, mv)
		}
	}
}

// create makes the application's view and embeds it; nil on failure,
// reported to the UI side. Main thread.
func (m *nativeMain) create(p nativePlace) *mainNative {
	fail := func(err error) *mainNative {
		h := p.h
		if m.failed == nil {
			m.failed = map[*nativeHost]bool{}
		}
		m.failed[h] = true
		m.toUI(func() { h.err, h.failed = err, true; m.invalidate() })
		return nil
	}
	if p.opts.Create == nil {
		return fail(errors.New("uihost: NativeHostOptions.Create is nil"))
	}
	view, err := p.opts.Create()
	if err != nil {
		return fail(err)
	}
	mv := &mainNative{h: p.h, view: view, release: p.opts.Release}
	id := p.id
	ev, err := corehook.NewEmbeddedNativeView(m.parent, view, platform.NativeViewOptions{PassInput: p.opts.PassInput}, func() {
		// The backend removed the view with its parent window.
		if m.views[id] == mv {
			delete(m.views, id)
		}
		mv.giveBack(false)
	})
	if err != nil {
		mv.giveBack(false)
		if errors.Is(err, tango.ErrUnsupported) {
			err = fmt.Errorf("%w (%w)", ErrNoNativeHost, err)
		}
		return fail(err)
	}
	mv.ev = ev
	m.views[id] = mv
	return mv
}

// remove takes a view out of the window and gives it back. Main thread.
func (m *nativeMain) remove(id ui.ID, mv *mainNative) {
	delete(m.views, id)
	mv.giveBack(true)
}

// giveBack removes the view (unless the backend already did) and runs
// Release, once. Main thread.
func (mv *mainNative) giveBack(close bool) {
	if mv.released {
		return
	}
	mv.released = true
	if close && mv.ev != nil {
		mv.ev.Close()
	}
	if mv.release != nil {
		mv.release(mv.view)
	}
}

// closeAll releases every view: the window is closing. Main thread.
func (m *nativeMain) closeAll() {
	if m.closed {
		return
	}
	m.closed = true
	for id, mv := range m.views {
		m.remove(id, mv)
	}
}
