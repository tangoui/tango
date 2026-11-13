package ui

import "github.com/tangoui/tango/ui/geom"

// SafeAreaWindow is implemented by windows that know the parts of their
// content area covered by system UI: the status bar, a display notch or
// camera cut-out, rounded corners, the home indicator, on-screen
// navigation buttons. The Runner polls it every frame, like Size.
type SafeAreaWindow interface {
	// SafeAreaInsets returns the distances, in logical pixels, from each
	// edge of the content area to the region that is fully visible and
	// free of system controls.
	SafeAreaInsets() geom.Insets
}

// safeState is the safe area in effect and the host override.
type safeState struct {
	cur      geom.Insets
	override geom.Insets
	set      bool // override in use
	seen     bool // cur was computed by a frame
}

// SetSafeArea sets the window's safe-area insets (logical pixels), taking
// precedence over SafeAreaWindow, and requests a frame. Hosts that are
// told about inset changes (iOS safeAreaInsetsDidChange, Android
// WindowInsets listeners) call it instead of implementing SafeAreaWindow.
func (r *Runner) SetSafeArea(in geom.Insets) {
	r.c.safe.override, r.c.safe.set = in, true
	r.Invalidate()
}

// ClearSafeArea removes the SetSafeArea override.
func (r *Runner) ClearSafeArea() {
	r.c.safe.set = false
	r.Invalidate()
}

// SafeArea returns the safe-area insets of the window in logical pixels
// (zero on desktops). Edge-to-edge widgets (navigation and tab bars,
// bottom sheets, full-screen pages) pad themselves by it. A change
// redraws the whole frame.
func (c *Context) SafeArea() geom.Insets { return c.safe.cur }

// updateSafeArea reads the insets for this frame; a change makes the
// frame a full redraw.
func (r *Runner) updateSafeArea() {
	s := &r.c.safe
	in := s.override
	if !s.set {
		in = geom.Insets{}
		if w, ok := r.win.(SafeAreaWindow); ok {
			in = w.SafeAreaInsets()
		}
	}
	if s.seen && in != s.cur {
		r.dmg.started = false // layout may move everywhere: redraw it all
	}
	s.cur, s.seen = in, true
}
