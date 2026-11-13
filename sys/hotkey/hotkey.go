// Package hotkey registers keyboard shortcuts.
//
// Register makes a system-wide (global) shortcut that fires even when the
// app is in the background:
//
//	hk, err := hotkey.Register(a, "CmdOrCtrl+Shift+K", func() { win.Show() })
//	...
//	hk.Unregister()
//
// Bind makes a shortcut that fires only while a surface's page has the
// keyboard focus; it needs no platform support (it listens to keydown in
// the page and swallows the key):
//
//	b, err := hotkey.Bind(win, "CmdOrCtrl+K", openPalette)
//
// Accelerators use the menu syntax: modifiers CmdOrCtrl, Cmd/Command/Super/
// Meta, Ctrl, Alt/Option, Shift joined with "+" to one key (A–Z, 0–9,
// F1–F24, Enter, Tab, Space, Backspace, Delete, Insert, Esc, Up/Down/Left/
// Right, Home, End, PageUp, PageDown, punctuation, num0–num9). CmdOrCtrl is
// Command on macOS and Control elsewhere.
//
// Callbacks run on the main thread. Global shortcuts: macOS (Carbon hot
// keys, no accessibility permission needed), Windows (RegisterHotKey),
// Linux on X11 (XGrabKey); Wayland has no global shortcut API for apps and
// returns ErrUnsupported.
package hotkey

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// ErrUnregistered is returned by Unregister on a hotkey that is no longer
// registered.
var ErrUnregistered = errors.New("hotkey: not registered")

// ErrBadAccelerator is wrapped by the errors of Validate, Register and
// Bind for an accelerator that does not parse.
var ErrBadAccelerator = errors.New("hotkey: bad accelerator")

// Validate reports whether accelerator is well formed for this platform,
// without registering anything. Its error wraps ErrBadAccelerator and says
// what is wrong.
func Validate(accelerator string) error {
	_, err := parse(accelerator)
	return err
}

// Spec is an accelerator parsed for the running platform (Parse):
// CmdOrCtrl is resolved to Meta on macOS and Ctrl elsewhere.
type Spec struct {
	Ctrl, Alt, Shift, Meta bool
	// Code names the key like KeyboardEvent.code: "KeyK", "Digit1", "F5",
	// "ArrowUp", "Equal", "Numpad5".
	Code string
	// Key is the lower-cased KeyboardEvent.key that also matches ("k",
	// "+", "escape"), "" if only Code does.
	Key string
}

// Parse parses accelerator for this platform without registering
// anything. Other shortcut tables (uihost.Shortcut for self-drawn windows)
// use it so every Tango shortcut API accepts the same syntax. Its error
// wraps ErrBadAccelerator.
func Parse(accelerator string) (Spec, error) {
	a, err := parse(accelerator)
	return Spec(a), err
}

// parse is parseAccel for this platform, wrapping ErrBadAccelerator.
func parse(accelerator string) (accel, error) {
	a, err := parseAccel(accelerator, runtime.GOOS)
	if err != nil {
		return a, fmt.Errorf("%w: %w", ErrBadAccelerator, err)
	}
	return a, nil
}

var nextID atomic.Uint32

// Hotkey is a registered global shortcut.
type Hotkey struct {
	app   *tango.App
	hk    platform.Hotkeys
	id    uint32
	accel string
	live  atomic.Bool
}

// Register registers a system-wide shortcut; fn runs on the main thread
// each time it is pressed. It fails if the accelerator is malformed, if
// the platform cannot register it (taken by another app or by the system)
// or with ErrUnsupported.
func Register(a *tango.App, accelerator string, fn func()) (*Hotkey, error) {
	if _, err := parse(accelerator); err != nil {
		return nil, err
	}
	hk, ok := a.Backend().(platform.Hotkeys)
	if !ok {
		return nil, ErrUnsupported
	}
	h := &Hotkey{app: a, hk: hk, id: nextID.Add(1), accel: accelerator}
	type result struct {
		err error
		ran bool
	}
	r := tango.Await(a, func() result {
		return result{hk.RegisterHotkey(h.id, accelerator, func() {
			if h.live.Load() {
				// The backend calls this directly: guard it.
				corehook.Guard(a, "hotkey", fn)
			}
		}), true}
	})
	if !r.ran {
		return nil, tango.ErrNotRunning
	}
	if r.err != nil {
		return nil, r.err
	}
	h.live.Store(true)
	return h, nil
}

// Accelerator returns the accelerator h was registered with.
func (h *Hotkey) Accelerator() string { return h.accel }

// Unregister removes the shortcut. Unregistering twice returns
// ErrUnregistered.
func (h *Hotkey) Unregister() error {
	if !h.live.CompareAndSwap(true, false) {
		return ErrUnregistered
	}
	if !tango.Await(h.app, func() bool { h.hk.UnregisterHotkey(h.id); return true }) {
		return tango.ErrNotRunning
	}
	return nil
}

// Unregister is h.Unregister().
func Unregister(h *Hotkey) error { return h.Unregister() }
