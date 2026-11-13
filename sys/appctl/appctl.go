// Package appctl controls the app as a whole in the window manager: hide
// and show all its windows, the dock / taskbar badge and progress bar,
// bouncing the dock icon or flashing the taskbar button, and the dock
// icon itself (tray-only apps).
//
//	appctl.SetBadge(app, "3")      // "" clears
//	appctl.SetProgress(app, 0.5)   // negative hides
//	appctl.RequestAttention(app, false)
//
// Support by platform:
//
//	                  macOS   Windows                    Linux
//	Hide / Show       yes     yes (all windows)          yes (all windows)
//	SetBadge          yes     digits, as overlay icon    digits, Unity launcher API (Ubuntu Dock, KDE)
//	SetProgress       yes     yes (taskbar)              Unity launcher API
//	RequestAttention  yes     yes (FlashWindowEx)        yes (urgency hint)
//	SetDockVisible    yes     no                         no
//
// Functions are safe from any goroutine once the app runs. They return
// ErrUnsupported when the platform cannot do it (Supports tells
// beforehand), and tango.ErrNotRunning after the app quit. The badge and
// progress also appear on windows created after they were set.
package appctl

import (
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

func backend(a *tango.App, op platform.AppControlOp) (platform.AppControl, error) {
	c, ok := a.Backend().(platform.AppControl)
	if !ok || c.AppControlOps()&op != op {
		return nil, ErrUnsupported
	}
	return c, nil
}

func do(a *tango.App, op platform.AppControlOp, fn func(platform.AppControl)) error {
	c, err := backend(a, op)
	if err != nil {
		return err
	}
	if !tango.Await(a, func() bool { fn(c); return true }) {
		return tango.ErrNotRunning
	}
	return nil
}

// shown is the badge and progress last set for an app, re-applied to
// windows created later (a Windows taskbar button starts without them).
type shown struct {
	mu       sync.Mutex
	badge    string
	progress float64
	hooked   bool
}

var states sync.Map // *tango.App -> *shown

// remember records what was set and, the first time, hooks surface
// creation to re-apply it.
func remember(a *tango.App, fn func(st *shown)) {
	v, _ := states.LoadOrStore(a, &shown{progress: -1})
	st := v.(*shown)
	st.mu.Lock()
	fn(st)
	hook := !st.hooked
	st.hooked = true
	st.mu.Unlock()
	if !hook {
		return
	}
	a.OnSurfaceCreated(func(*tango.Surface) {
		c, ok := a.Backend().(platform.AppControl)
		if !ok {
			return
		}
		st.mu.Lock()
		badge, p := st.badge, st.progress
		st.mu.Unlock()
		ops := c.AppControlOps()
		if badge != "" && ops&CtlBadge != 0 {
			c.SetBadge(badge)
		}
		if p >= 0 && ops&CtlProgress != 0 {
			c.SetProgress(p)
		}
	})
}

// Operations, for Supports.
const (
	CtlHide      = platform.CtlHide      // Hide, Show
	CtlBadge     = platform.CtlBadge     // SetBadge with counts
	CtlBadgeText = platform.CtlBadgeText // SetBadge with any text
	CtlProgress  = platform.CtlProgress  // SetProgress
	CtlAttention = platform.CtlAttention // RequestAttention
	CtlDock      = platform.CtlDock      // SetDockVisible
)

// Supports reports whether the platform performs every operation in op.
func Supports(a *tango.App, op platform.AppControlOp) bool {
	_, err := backend(a, op)
	return err == nil
}

// Hide hides all of the app's windows (Cmd+H on macOS).
func Hide(a *tango.App) error {
	return do(a, CtlHide, func(c platform.AppControl) { c.HideApp() })
}

// Show shows the windows Hide hid and brings the app to the front.
func Show(a *tango.App) error {
	return do(a, CtlHide, func(c platform.AppControl) { c.ShowApp() })
}

// SetBadge shows text on the dock icon / taskbar button; "" clears it.
// Windows and Linux only show numbers: other text returns ErrUnsupported
// there (and clears the badge).
func SetBadge(a *tango.App, text string) error {
	if text != "" && !isCount(text) {
		if _, err := backend(a, CtlBadgeText); err != nil {
			if do(a, CtlBadge, func(c platform.AppControl) { c.SetBadge("") }) == nil {
				remember(a, func(st *shown) { st.badge = "" })
			}
			return err
		}
	}
	err := do(a, CtlBadge, func(c platform.AppControl) { c.SetBadge(text) })
	if err == nil {
		remember(a, func(st *shown) { st.badge = text })
	}
	return err
}

// SetProgress shows a progress bar in [0, 1] on the dock icon / taskbar
// button; values above 1 are clamped, a negative value hides it.
func SetProgress(a *tango.App, p float64) error {
	if p > 1 {
		p = 1
	}
	if p != p { // NaN
		p = -1
	}
	err := do(a, CtlProgress, func(c platform.AppControl) { c.SetProgress(p) })
	if err == nil {
		remember(a, func(st *shown) { st.progress = p })
	}
	return err
}

// RequestAttention bounces the dock icon or flashes the taskbar button
// until the user activates the app. critical keeps it going on macOS
// (informational requests bounce once).
func RequestAttention(a *tango.App, critical bool) error {
	return do(a, CtlAttention, func(c platform.AppControl) { c.RequestAttention(critical) })
}

// SetDockVisible shows or hides the app's dock icon (macOS). Without it
// the app has no menu bar either and lives in a tray icon.
func SetDockVisible(a *tango.App, visible bool) error {
	return do(a, CtlDock, func(c platform.AppControl) { c.SetDockVisible(visible) })
}

// isCount reports whether s is a badge every platform can show: 1 to 9
// ASCII digits, optionally followed by "+".
func isCount(s string) bool {
	s = trimPlus(s)
	if len(s) == 0 || len(s) > 9 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func trimPlus(s string) string {
	if len(s) > 0 && s[len(s)-1] == '+' {
		return s[:len(s)-1]
	}
	return s
}
