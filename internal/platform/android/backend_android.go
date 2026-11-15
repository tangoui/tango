//go:build android

package android

/*
#include "jni_android.h"
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// AssetOrigin is where pages load the app's assets from. Requests to it
// never reach the network: the host intercepts them (Page.java).
const AssetOrigin = "https://tango.localhost"

// stepWait bounds how long Step(true) waits for a Wake.
const stepWait = 10 * time.Millisecond

// Backend is the Android platform.Backend. There is one per process: the
// JNI entry points are global.
type Backend struct {
	hostP atomic.Pointer[platform.Host]
	opts  platform.AppOptions

	runRequested atomic.Bool   // Run started; the lifecycle replay is due
	quitting     atomic.Bool   // Terminating delivered
	quitCh       chan struct{} // closed with quitting
	quitOnce     sync.Once

	wakePending atomic.Bool   // a Drain is posted to the main Looper
	laneDirty   atomic.Bool   // a down lane packet was queued (lanes_android.go)
	stepCh      chan struct{} // Wake signal for Step(true)

	mu       sync.Mutex // guards surfaces (the request thread reads it)
	surfaces map[platform.SurfaceID]*surface

	// Main thread only.
	life      lifecycle
	stepSpins int // blocking Steps since the Looper last called in
	nextCB    int
	callbacks map[int]func(button int, paths []string, errMsg string)
}

var backend = &Backend{
	quitCh:    make(chan struct{}),
	stepCh:    make(chan struct{}, 1),
	surfaces:  map[platform.SurfaceID]*surface{},
	callbacks: map[int]func(int, []string, string){},
}

// New returns the process' backend.
func New() platform.Backend { return backend }

var _ interface {
	platform.Backend
	platform.Clipboard
	platform.Dialogs
} = (*Backend)(nil)

func (b *Backend) host() platform.Host {
	if p := b.hostP.Load(); p != nil {
		return *p
	}
	return nil
}

func (b *Backend) Init(h platform.Host, opts platform.AppOptions) error {
	if !hostStarted.Load() {
		return errors.New("tango/android: the app must be started by the Java host (mobile/android), not run as an executable")
	}
	if !b.hostP.CompareAndSwap(nil, &h) {
		return errors.New("tango/android: only one App can run per process")
	}
	b.opts = opts
	return nil
}

// Run does not own the main thread on Android: it asks the main thread to
// deliver Started (and whatever the Activity did meanwhile), then waits
// until the Activity is finishing and Terminating has been delivered.
func (b *Backend) Run() error {
	b.runRequested.Store(true)
	b.Wake()
	<-b.quitCh
	return nil
}

// Quit finishes the Activity; its onDestroy delivers Terminating, which
// ends Run.
func (b *Backend) Quit() {
	withEnv(func(e env) { e.callStaticVoid(mQuit) })
}

func (b *Backend) Wake() {
	select {
	case b.stepCh <- struct{}{}:
	default:
	}
	if b.wakePending.CompareAndSwap(false, true) {
		if !withEnv(func(e env) { e.callStaticVoid(mWake) }) {
			b.wakePending.Store(false)
		}
	}
}

var _ platform.NestedLoop = (*Backend)(nil)

// NestedLoopSupported is false: Step cannot run the Looper's native events
// from inside a callback (see Step), so the core refuses AwaitAsync on the
// main thread with tango.ErrNestedWait instead of letting it wait forever.
func (b *Backend) NestedLoopSupported() bool { return false }

// Step cannot run the Android Looper: a nested Looper.loop() cannot be
// left without quitting the main Looper, and MessageQueue.next is hidden
// API. So Step only runs what Wake asked for (the core's queue) and, if
// block, waits briefly for another Wake. Native events, including dialog
// buttons, are not processed until the current callback returns: code on
// the main thread must not wait for them (see docs/mobile.md).
func (b *Backend) Step(block bool) bool {
	if b.quitting.Load() {
		return false
	}
	main := onMainThread()
	if main {
		b.pump()
		if block {
			b.warnStuck()
		}
	}
	if block {
		t := time.NewTimer(stepWait)
		select {
		case <-b.stepCh:
		case <-b.quitCh:
		case <-t.C:
		}
		t.Stop()
		if main {
			b.pump()
		}
	}
	return !b.quitting.Load()
}

// warnStuck logs once when the main thread has been waiting in Step for a
// few seconds without returning to the Looper: almost certainly AwaitAsync
// (a dialog, say) called from a main-thread callback, which cannot finish
// on Android. Main thread.
func (b *Backend) warnStuck() {
	b.stepSpins++
	if b.stepSpins == int(3*time.Second/stepWait) {
		logf(logError, "the main thread has waited in AwaitAsync for 3s: native events (dialogs, input) "+
			"cannot run until it returns; call blocking APIs such as dialogs from a goroutine (see docs/mobile.md)")
	}
}

// pump runs a pending Drain now. Main thread.
func (b *Backend) pump() {
	if b.wakePending.Swap(false) {
		b.drain()
	}
}

// drain delivers the lifecycle replay once Run started, then the core's
// queue. Main thread.
func (b *Backend) drain() {
	h := b.host()
	if h == nil {
		return
	}
	if b.runRequested.Load() && !b.life.running {
		b.deliver(h, b.life.start())
		b.flushMobileEvents(h) // mobile_android.go
	}
	b.flushLanes() // lanes_android.go
	h.Drain()
}

// deliver passes lifecycle events to the core; Terminating ends Run.
// Main thread.
func (b *Backend) deliver(h platform.Host, evs []platform.LifecycleEvent) {
	for _, ev := range evs {
		h.OnLifecycle(ev)
		if ev == platform.LifecycleTerminating {
			b.quitting.Store(true)
			b.quitOnce.Do(func() { close(b.quitCh) })
		}
	}
}

func (b *Backend) AssetOrigin() string { return AssetOrigin }

func (b *Backend) surface(id platform.SurfaceID) *surface {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.surfaces[id]
}

func (b *Backend) NewSurface(id platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
	u := opts.URL
	if u == "" {
		u = "about:blank"
	}
	// The lanes are decided before the surface is published: the request
	// thread builds the first document's page script from them.
	s := &surface{b: b, id: id, bridge: opts.BridgeScript, tokens: newDocTokens(AssetOrigin, u),
		laneCaps: envLaneCaps(), lanes: newLaneState()}
	b.mu.Lock()
	b.surfaces[id] = s
	b.mu.Unlock()
	var err error
	ok := withEnv(func(e env) {
		var page C.jobject
		page, err = e.callStaticObject(mNewPage, jvLong(int64(id)), jvObj(e.str(u)),
			jvBool(opts.Visible), jvBool(opts.Debug), jvObj(e.str(opts.Title)))
		if err == nil && page == 0 {
			err = errors.New("tango/android: no Activity to show the page in")
		}
		if err == nil {
			s.page = e.global(page)
		}
	})
	if !ok && err == nil {
		err = errJava
	}
	if err != nil {
		b.mu.Lock()
		delete(b.surfaces, id)
		b.mu.Unlock()
		return nil, err
	}
	return s, nil
}

// surface is one page: a full-screen WebView in the Activity.
type surface struct {
	b      *Backend
	id     platform.SurfaceID
	bridge string     // the core's BridgeScript, after the shim (pageScript)
	tokens *docTokens // per-document bridge tokens and trusted origins

	// IPC fast lanes (lanes_android.go). laneCaps is set before the surface
	// is published and never changes; lanes is safe from any thread.
	laneCaps []string
	lanes    *laneState

	// Main thread only.
	page   C.jobject // global ref to the Java Page
	view   C.jobject // global ref to its WebView, made by Native
	w, h   int       // dp, from the last Resized
	closed bool

	overlays []*overlay // attached canvases (canvas_attach_android.go)
}

func (s *surface) call(m *jmethod, args ...C.jvalue) {
	if s.closed {
		return
	}
	if !onMainThread() {
		logf(logWarn, "Surface.%s called off the main thread", m.name)
	}
	withEnv(func(e env) { e.callVoid(s.page, m, args...) })
}

func (s *surface) callString(m *jmethod, v string) {
	if s.closed {
		return
	}
	withEnv(func(e env) { e.callVoid(s.page, m, jvObj(e.str(v))) })
}

func (s *surface) Show()  { s.call(mShow) }
func (s *surface) Hide()  { s.call(mHide) }
func (s *surface) Close() { s.call(mClose) }
func (s *surface) Focus() { s.call(mFocus) }

func (s *surface) SetTitle(title string) { s.callString(mSetTitle, title) }
func (s *surface) Navigate(url string) {
	if !s.closed {
		s.tokens.navigated(url) // the core trusts url's origin from now on
	}
	s.callString(mNavigate, url)
}
func (s *surface) Eval(js string) { s.callString(mEval, js) }

// SetSize does nothing: pages fill the Activity.
func (s *surface) SetSize(width, height int) {}

// Size is the page's size in dp as last laid out (0, 0 before the first
// layout pass).
func (s *surface) Size() (int, int) { return s.w, s.h }

// OpenDevTools does nothing: with SurfaceOptions.Debug the WebView can be
// inspected from chrome://inspect on a computer.
func (s *surface) OpenDevTools() {}

// Native returns a JNI global reference to the page's android.webkit.WebView
// (a jobject), valid until the surface closes.
func (s *surface) Native() uintptr {
	if s.closed {
		return 0
	}
	if s.view == 0 {
		withEnv(func(e env) {
			if v, err := e.callObject(s.page, mWebView); err == nil && v != 0 {
				s.view = e.global(v)
			}
		})
	}
	return uintptr(s.view)
}

// release drops the Java references once the page is gone. Main thread.
func (s *surface) release() {
	if s.closed {
		return
	}
	s.closed = true
	s.lanes.close()
	page, view := s.page, s.view
	s.page, s.view = 0, 0
	withEnv(func(e env) {
		e.deleteGlobal(view)
		e.deleteGlobal(page)
	})
}
