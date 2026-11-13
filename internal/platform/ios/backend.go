//go:build ios

// Package ios is Tango's iOS backend: UIKit + WKWebView, written in cgo and
// Objective-C (bridge.m) and linked into the app as part of its Go
// c-archive.
//
// The native host (mobile/ios/main.m) owns the main thread: it runs
// UIApplicationMain and calls TangoStart from
// application:didFinishLaunchingWithOptions:. TangoStart installs the
// UIKit side and runs the program's main.main on a new goroutine, so
// App.Run executes off the main thread: Backend.Init and Backend.Run are
// called on that goroutine, everything else on the main thread as the
// contract says. See docs/mobile.md.
package ios

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fmodules
#cgo LDFLAGS: -framework Foundation -framework UIKit -framework WebKit -framework UniformTypeIdentifiers
#include <stdlib.h>
#include "bridge.h"
#include "startup.h"
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

const (
	assetScheme = "tango"
	assetOrigin = assetScheme + "://app"
)

// backend implements platform.Backend plus Clipboard and Dialogs.
//
// Fields below mu are touched on the main thread only.
type backend struct {
	h    platform.Host
	opts platform.AppOptions

	done     chan struct{} // closed when Run must return
	doneOnce sync.Once
	ready    atomic.Bool // Init done: drains may reach h

	started    bool // LifecycleStarted delivered
	foreground bool // last delivered state was Foreground
	terminated bool // LifecycleTerminating delivered
	quitting   bool

	surfaces map[platform.SurfaceID]*surface
	tasks    map[uint64]*schemeTask
	dialogs  map[int]func() // pending dialogs: cancel functions
	nextTok  int
}

var (
	_ platform.Backend   = (*backend)(nil)
	_ platform.Clipboard = (*backend)(nil)
	_ platform.Dialogs   = (*backend)(nil)
)

// current is the backend UIKit callbacks are delivered to. Set by Init.
var current atomic.Pointer[backend]

// New returns the iOS backend. Only one may be initialised per process.
func New() platform.Backend {
	return &backend{
		done:     make(chan struct{}),
		surfaces: map[platform.SurfaceID]*surface{},
		tasks:    map[uint64]*schemeTask{},
		dialogs:  map[int]func(){},
	}
}

func (b *backend) AssetOrigin() string { return assetOrigin }

var errNotStarted = errors.New("tango/ios: TangoStart was not called by the native host")

// Init runs on App.Run's goroutine (not the main thread, see the package
// comment). It only records h; LifecycleStarted is delivered from the
// first drain on the main thread.
func (b *backend) Init(h platform.Host, opts platform.AppOptions) error {
	if !hostStarted.Load() {
		return errNotStarted
	}
	if !current.CompareAndSwap(nil, b) {
		return errors.New("tango/ios: a backend is already initialised in this process")
	}
	b.h, b.opts = h, opts
	b.ready.Store(true)
	C.tg_wake()
	return nil
}

// Run blocks App.Run's goroutine until Quit or app termination. The main
// thread keeps running UIKit meanwhile.
func (b *backend) Run() error {
	<-b.done
	return nil
}

// Quit ends the app: Terminating is delivered, Run returns, and when main
// returns the process exits (see TangoStart). iOS apps rarely quit
// themselves; Apple's guidelines discourage it.
func (b *backend) Quit() {
	b.quitting = true
	b.finish()
}

// finish delivers Terminating once and releases Run. Main thread.
func (b *backend) finish() {
	b.cancelDialogs()
	if !b.terminated {
		b.terminated = true
		if b.started {
			b.h.OnLifecycle(platform.LifecycleTerminating)
		}
	}
	b.doneOnce.Do(func() { close(b.done) })
}

// Wake is safe from any goroutine (CFRunLoopSourceSignal and
// CFRunLoopWakeUp are thread-safe).
func (b *backend) Wake() { C.tg_wake() }

// Step runs the main run loop once in the default mode. A Wake ends a
// blocking Step since the drain source fires in it.
func (b *backend) Step(block bool) bool {
	if b.quitting {
		return false
	}
	blk := C.int(0)
	if block {
		blk = 1
	}
	C.tg_step(blk)
	return !b.quitting
}

// drain is the wake source's callback. The first one after Init delivers
// LifecycleStarted (TangoStart ran from didFinishLaunching, so the app has
// launched by then) and Foreground if the app is already active.
func (b *backend) drain() {
	if !b.started && !b.terminated {
		b.started = true
		C.tg_trace(traceStarted)
		b.h.OnLifecycle(platform.LifecycleStarted)
		if C.tg_app_active() != 0 {
			b.setForeground(true)
		}
		b.flushAppEvents() // mobile.go: links and pushes that arrived before Started
	}
	b.h.Drain()
}

// traceStarted is the label of the first drain (C memory, never freed).
var traceStarted = C.CString("first Drain: Started")

func (b *backend) setForeground(fg bool) {
	if !b.started || b.terminated || b.foreground == fg {
		return
	}
	b.foreground = fg
	if fg {
		b.h.OnLifecycle(platform.LifecycleForeground)
	} else {
		b.h.OnLifecycle(platform.LifecycleBackground)
	}
}

//export tangoIOSDrain
func tangoIOSDrain() {
	if b := current.Load(); b != nil && b.ready.Load() {
		b.drain()
	}
}

//export tangoIOSLifecycle
func tangoIOSLifecycle(code C.int) {
	b := current.Load()
	if b == nil || !b.started {
		return // the first drain reads the state itself
	}
	switch code {
	case C.TG_DID_BECOME_ACTIVE:
		b.setForeground(true)
	case C.TG_WILL_RESIGN_ACTIVE, C.TG_DID_ENTER_BACKGROUND:
		b.setForeground(false)
	}
}

//export tangoIOSTerminate
func tangoIOSTerminate() {
	b := current.Load()
	if b == nil {
		return
	}
	b.finish()
	// UIKit kills the process when this notification handler returns: give
	// main a moment to run its deferred cleanup after Run returned.
	waitMainReturn()
}

func (b *backend) NewSurface(id platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
	return b.newSurface(id, opts)
}

// cstring returns a C copy of s and the function freeing it.
func cstring(s string) (*C.char, func()) {
	p := C.CString(s)
	return p, func() { C.free(unsafePointer(p)) }
}
