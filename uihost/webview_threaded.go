package uihost

// Embedded WebViews of threaded windows (design §5.6). The page builds
// WebView boxes on the window's UI thread, which owns the embeds and their
// views (webview.go) and decides each frame where every view goes. It
// hands that immutable placement list to the main thread in one task per
// frame (mainEmbeds.post); the main thread owns the native views and
// creates, places, focuses and closes them. The canvas and the WebViews
// may be a frame apart.
//
// threadedNativeViews == false is the fallback the design keeps: a window
// whose page shows a native view goes back to the main-thread model, at the
// first frame that has one.

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
)

// threadedNativeViews keeps threaded windows on their UI thread when their
// page shows a native view (ui.NativeView, WebView). false: such a window
// switches to the main-thread model at the first frame with one, once.
var threadedNativeViews = true

// embedFrame is one frame's placement list: every native view the UI
// keeps, in stacking order. Views missing from it are closed.
type embedFrame struct {
	views []embedPlace
}

// mainEmbeds is the main-thread side of a threaded window's WebViews.
type mainEmbeds struct {
	h      *host
	t      *threadedHost // immutable: h.t changes on main during a fallback
	parent *tango.Surface

	mu      sync.Mutex
	pending *embedFrame // the latest frame not applied yet
	posted  bool        // a task to apply pending is queued on main

	// Main thread.
	views  map[ui.ID]*mainView
	dead   map[*embedView]bool // closed by the app: never recreate for this UI view
	closed bool
	frames int          // placement tasks applied (tests)
	posts  atomic.Int32 // placement tasks queued on main (tests)
}

// mainView is one native WebView on the main thread.
type mainView struct {
	me     *mainEmbeds
	ui     *embedView // its UI side (identity; touched only on the UI thread)
	s      *tango.Surface
	ev     platform.EmbeddedWebView
	failed bool
}

// newThreadedEmbeds creates both sides for a threaded window. Main thread.
func newThreadedEmbeds(h *host, t *threadedHost, parent *tango.Surface) (*embeds, *mainEmbeds) {
	e := newEmbeds(h, parent)
	e.idle = true // main has nothing yet
	me := &mainEmbeds{h: h, t: t, parent: parent, views: map[ui.ID]*mainView{}, dead: map[*embedView]bool{}}
	e.main = me
	return e, me
}

// toUI runs fn, which touches UI-side embed state, on the window's UI
// thread: posted there on a threaded window, else on the main thread later.
func (e *embeds) toUI(fn func()) {
	if e.main != nil {
		e.main.toUI(fn)
		return
	}
	e.h.app.Post(fn)
}

func (me *mainEmbeds) toUI(fn func()) {
	me.t.post(me.h.app, fn) // dropped once closed; on main after a fallback
}

// syncThreaded is sync for a threaded window: it decides this frame's
// placements and posts them to the main thread. UI thread.
func (e *embeds) syncThreaded() {
	regs := e.h.r.NativeRegions()
	if !threadedNativeViews && len(regs) > 0 {
		e.h.switchToMain()
		return
	}
	now := time.Now()
	fr := &embedFrame{}
	for _, reg := range regs {
		v := e.views[reg.ID]
		if v == nil || v.built != e.frame {
			continue // a native view that is not a WebView
		}
		v.lastSeen = now
		if v.err != nil {
			continue
		}
		fr.views = append(fr.views, v.decide(reg))
	}
	for id, v := range e.views {
		if v.built == e.frame {
			continue
		}
		v.covered, v.frozen, v.snap = false, false, nil
		if keep := v.opts.KeepAlive; keep > 0 && v.s != nil && now.Sub(v.lastSeen) < keep {
			e.h.win.InvalidateAt(v.lastSeen.Add(keep))
			fr.views = append(fr.views, embedPlace{id: id, v: v, opts: v.opts}) // hidden
			continue
		}
		delete(e.views, id)
		v.s = nil // closed by the main thread: it is not in fr
	}
	e.frame++
	empty := len(fr.views) == 0
	if empty && e.idle {
		return // nothing on main to place or close
	}
	e.idle = empty
	e.main.post(fr)
}

// post hands fr to the main thread. Frames that the main thread has not
// applied yet are replaced (the latest placement wins) and keep their
// focus requests; at most one task is queued. UI thread.
func (me *mainEmbeds) post(fr *embedFrame) {
	me.mu.Lock()
	if old := me.pending; old != nil {
		for _, p := range old.views {
			if !p.focus {
				continue
			}
			for i := range fr.views {
				if q := &fr.views[i]; q.v == p.v && !q.focus {
					q.focus, q.keyboard, q.last = true, p.keyboard, p.last
				}
			}
		}
	}
	me.pending = fr
	queue := !me.posted
	me.posted = true
	me.mu.Unlock()
	if queue {
		me.posts.Add(1)
		me.h.app.Post(me.apply)
	}
}

// apply places the native views as the latest frame asks. Main thread;
// dropped once the window closed.
func (me *mainEmbeds) apply() {
	me.mu.Lock()
	fr := me.pending
	me.pending, me.posted = nil, false
	me.mu.Unlock()
	h := me.h
	if fr == nil || me.closed || h.closed || h.t == nil || h.t.closing.Load() {
		return
	}
	if len(fr.views) == 0 && len(me.views) == 0 {
		clear(me.dead)
		return
	}
	me.frames++
	kept := make(map[ui.ID]bool, len(fr.views))
	live := make(map[*embedView]bool, len(fr.views))
	focusedWin := false
	for _, p := range fr.views {
		live[p.v] = true
		if me.dead[p.v] {
			continue
		}
		mv := me.views[p.id]
		if mv != nil && mv.ui != p.v { // a new box with the same key
			mv.close()
			mv = nil
		}
		if mv == nil {
			mv = &mainView{me: me, ui: p.v}
			me.views[p.id] = mv
			mv.create(p.id, p.opts)
		}
		kept[p.id] = true
		if mv.ev == nil {
			continue
		}
		corehook.CommitWebPlacement(mv.s, mv.ev, p.placement)
		if p.focus {
			if !focusedWin {
				focusedWin = me.parent.IsFocused()
			}
			if focusedWin {
				mv.ev.FocusWeb()
				if p.keyboard {
					mv.s.Eval(enterScript(p.last))
				}
			}
		}
	}
	for id, mv := range me.views {
		if !kept[id] {
			delete(me.views, id)
			mv.close()
		}
	}
	for v := range me.dead {
		if !live[v] {
			delete(me.dead, v)
		}
	}
}

// create makes the view's surface and tells the UI side. Main thread.
func (mv *mainView) create(id ui.ID, opts WebViewOptions) {
	me, v := mv.me, mv.ui
	s, ev, err := createWebView(me.parent, opts)
	if err != nil {
		mv.failed = true
		me.toUI(func() { v.err = err; me.h.invalidate() })
		return
	}
	mv.s, mv.ev = s, ev
	bySurface[s] = mv
	h := me.h
	s.OnFocused(func() { me.toUI(func() { v.webFocused = true; h.invalidate() }) })
	s.OnBlurred(func() { me.toUI(func() { v.webFocused = false; h.invalidate() }) })
	s.OnClosed(func() {
		delete(bySurface, s)
		if mv.s != s || me.closed {
			return
		}
		// Closed by the app (Surface.Close): forget it; the UI creates a
		// new one if the box is still built.
		mv.s, mv.ev = nil, nil
		if cur := me.views[id]; cur == mv {
			delete(me.views, id)
		}
		me.dead[v] = true
		me.toUI(func() {
			if v.s == s {
				v.s, v.webFocused = nil, false
			}
			if cur, ok := v.e.views[id]; ok && cur == v {
				delete(v.e.views, id)
			}
			h.invalidate()
		})
	})
	if fn := opts.Created; fn != nil {
		corehook.Guard(h.app, "uihost WebView Created", func() { fn(s) })
	}
	if mv.s == s {
		me.toUI(func() {
			if v.s == nil && v.err == nil {
				v.s = s // WebViewW.Surface
				h.invalidate()
			}
		})
	}
}

// focusOut hands the focus back to the UI after the page asked to leave.
// Main thread.
func (mv *mainView) focusOut(back bool) {
	if mv.ev == nil || mv.me.closed {
		return
	}
	mv.ev.BlurWeb()
	mods := input.Modifiers(0)
	if back {
		mods = input.ModShift
	}
	h := mv.me.h
	mv.me.toUI(func() {
		h.r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true, Mods: mods})
		h.r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Mods: mods})
		h.invalidate()
	})
}

func (mv *mainView) close() {
	if mv.s == nil {
		return
	}
	s := mv.s
	mv.s, mv.ev = nil, nil
	delete(bySurface, s)
	s.Close()
}

// close forgets the window's native views (the backend closes them with
// their parent) and drops placements still queued. Main thread.
func (me *mainEmbeds) close() {
	if me == nil || me.closed {
		return
	}
	me.closed = true
	for _, mv := range me.views {
		if mv.s != nil {
			delete(bySurface, mv.s)
		}
	}
	me.views = nil
}

// switchToMain hands a running threaded window back to the main thread
// after the first frame with a native view (threadedNativeViews false),
// through the startup fallback (stopStarting): work for the UI is buffered
// from now on and runs on main, in order, once the UI thread has exited;
// the canvas goes back to drawing on main. At most once. UI thread, at the
// end of a frame.
func (h *host) switchToMain() {
	t := h.t
	if !t.switching.CompareAndSwap(false, true) {
		return
	}
	t.startMu.Lock()
	t.phase.Store(starting) // dispatch buffers, Invalidate waits for the handover
	t.startMu.Unlock()
	t.attached = false // later ticks draw nothing
	corehook.ReturnRunnerToMain(h.r)
	h.app.Post(func() {
		if !h.closed && h.t == t {
			h.stopStarting(t)
		}
	})
}
