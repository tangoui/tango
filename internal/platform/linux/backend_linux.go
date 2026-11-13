//go:build linux && !android

// Portions of the main-loop and window handling follow MyGo's
// internal/linux/app.go and window.go (https://github.com/egoist/mygo, MIT
// License, Copyright (c) 2026 MyGo contributors).

package linux

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

type backend struct {
	host platform.Host
	opts platform.AppOptions

	loaded      atomic.Bool // native libraries bound, gtk initialised
	wakePending atomic.Bool // a Wake idle source is scheduled (coalescing)

	// Main-thread state.
	quitting bool
	surfaces map[platform.SurfaceID]*surface
	byWeb    map[ptr]*surface
	focus    focusTracker
	settling bool // a focus settle timeout is scheduled
	appMenu  *appMenu
}

// New returns the Linux (GTK 3 + WebKitGTK 4.1) backend. Native libraries
// are loaded by Init, so a program still starts and reports a clear error
// on machines without them.
func New() platform.Backend {
	return &backend{
		surfaces: map[platform.SurfaceID]*surface{},
		byWeb:    map[ptr]*surface{},
	}
}

var (
	_ platform.Backend   = (*backend)(nil)
	_ platform.Clipboard = (*backend)(nil)
	_ platform.Dialogs   = (*backend)(nil)
	_ platform.AppMenu   = (*backend)(nil)
)

func (b *backend) Init(h platform.Host, opts platform.AppOptions) error {
	if h == nil {
		return errors.New("tango/linux: nil Host")
	}
	b.host, b.opts = h, opts
	if err := load(); err != nil {
		return err
	}
	// GTK names the window class / app id after the program name and
	// WebKitGTK its data directories.
	prg := opts.ID
	if prg == "" {
		if exe, err := os.Executable(); err == nil {
			prg = filepath.Base(exe)
		}
	}
	if prg != "" {
		gSetPrgname(prg)
	}
	if opts.Name != "" {
		gSetApplicationName(opts.Name)
	}
	if !gtkInitCheck(0, 0) {
		return errors.New("tango/linux: gtk_init failed: cannot open display (is DISPLAY or WAYLAND_DISPLAY set?)")
	}
	initCallbacks()
	wakeTarget = b.drain
	b.registerScheme()
	b.installSystemWatch()   // integration_linux.go: theme / display change events
	b.installMotionWatch()   // motion_linux.go: reduce-motion preference
	b.installContrastWatch() // contrast_linux.go: high-contrast preference
	b.loaded.Store(true)
	// A Wake that came before the libraries were loaded scheduled nothing.
	if b.wakePending.Load() {
		gIdleAddFull(gPriorityDefault, cbWake, 0, 0)
	}
	return nil
}

func (b *backend) Run() error {
	if !b.loaded.Load() {
		return errors.New("tango/linux: Run before Init")
	}
	b.host.OnLifecycle(platform.LifecycleStarted)
	if !b.quitting {
		gtkMain()
	}
	b.quitting = true
	b.host.OnLifecycle(platform.LifecycleTerminating)
	return nil
}

func (b *backend) Quit() {
	b.quitting = true
	if b.loaded.Load() && gtkMainLevel() > 0 {
		gtkMainQuit()
	}
}

// Wake schedules one idle source; further Wakes coalesce into it until it
// runs. The flag is cleared before Drain, so a Wake from inside Drain (or
// racing with it) schedules another.
func (b *backend) Wake() {
	if !b.wakePending.CompareAndSwap(false, true) {
		return
	}
	if b.loaded.Load() {
		gIdleAddFull(gPriorityDefault, cbWake, 0, 0) // g_idle_add_full is thread safe
	}
}

func (b *backend) drain() {
	b.wakePending.Store(false)
	b.host.Drain()
}

func (b *backend) Step(block bool) bool {
	if b.quitting || !b.loaded.Load() {
		return false
	}
	gMainContextIteration(0, block)
	return !b.quitting
}

func (b *backend) AssetOrigin() string { return assetOrigin }

// lifecycle helpers ------------------------------------------------------

func (b *backend) focusIn(id platform.SurfaceID) {
	if ev, ok := b.focus.in(id); ok {
		b.host.OnLifecycle(ev)
	}
}

func (b *backend) focusOut(id platform.SurfaceID) {
	if !b.focus.out(id) || b.settling {
		return
	}
	// Focus moving between two of our windows arrives as out+in; wait a
	// little before deciding the app went to the background.
	b.settling = true
	later(50, func() {
		b.settling = false
		if ev, ok := b.focus.settle(); ok {
			b.host.OnLifecycle(ev)
		}
	})
}

func (b *backend) parentWindow(id platform.SurfaceID) ptr {
	if s := b.surfaces[id]; s != nil && !s.closed {
		return s.win
	}
	return 0
}
