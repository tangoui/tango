package linux

import "github.com/tangoui/tango/internal/platform"

// focusTracker aggregates per-window focus into app Foreground/Background.
//
// Moving focus between two of the app's windows produces focus-out on one
// and focus-in on the other, possibly in separate main-loop iterations. A
// focus-out therefore only asks for a deferred settle(); Background is
// reported if, by then, no window has focus.
type focusTracker struct {
	focused    map[platform.SurfaceID]bool
	foreground bool
}

// in records that id gained focus. It returns LifecycleForeground when the
// app was not in the foreground.
func (t *focusTracker) in(id platform.SurfaceID) (platform.LifecycleEvent, bool) {
	if t.focused == nil {
		t.focused = map[platform.SurfaceID]bool{}
	}
	t.focused[id] = true
	if t.foreground {
		return 0, false
	}
	t.foreground = true
	return platform.LifecycleForeground, true
}

// out records that id lost focus (or closed). It reports whether a settle
// should be scheduled.
func (t *focusTracker) out(id platform.SurfaceID) bool {
	delete(t.focused, id)
	return t.foreground && len(t.focused) == 0
}

// settle returns LifecycleBackground if the app is in the foreground but no
// window has focus.
func (t *focusTracker) settle() (platform.LifecycleEvent, bool) {
	if !t.foreground || len(t.focused) > 0 {
		return 0, false
	}
	t.foreground = false
	return platform.LifecycleBackground, true
}
