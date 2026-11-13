package uihost

// Following the system high-contrast preference (docs/accessibility.md,
// "Increase contrast"): platform.ContrastPreference gives the current
// value, AppContrastChanged reports changes, like the reduce-motion
// preference in motion.go. It is part of theme following (follow.go): a
// window that follows the system theme draws theme.HighContrast of the
// generated light or dark theme while the system asks for more contrast,
// and the plain theme otherwise. Windows with an explicit theme
// (RunnerOptions.Theme, ThemeFixed, a theme the app set later) are left
// alone, as for light/dark.

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	systheme "github.com/tangoui/tango/sys/theme"
	"github.com/tangoui/tango/ui/theme"
)

// ContrastMode says how a window's generated theme relates to the system
// high-contrast preference (macOS Increase contrast, Windows contrast
// themes, the Linux appearance portal).
type ContrastMode int

const (
	// ContrastSystem follows the system: theme.HighContrast of the
	// generated theme while the system asks for more contrast, switching
	// when the user does. The default.
	ContrastSystem ContrastMode = iota
	// ContrastStandard never raises the contrast, whatever the system
	// says.
	ContrastStandard
	// ContrastHigh always draws the high-contrast variant.
	ContrastHigh
)

// highContrast reports whether a theme generated with these options is
// the high-contrast variant while the system preference is system.
func (o Options) highContrast(system bool) bool {
	switch o.Contrast {
	case ContrastHigh:
		return true
	case ContrastStandard:
		return false
	default:
		return system
	}
}

// themeForContrast is ThemeFor with the high-contrast decision made: hc
// passes the generated theme (Customize applied) through
// theme.HighContrast, so light/dark, accent and the app's adjustments
// all reach the contrast minimums.
func (o Options) themeForContrast(sys systheme.Theme, hc bool) *theme.Theme {
	th := o.baseTheme(sys)
	if th != nil && hc && !th.HighContrast {
		th = theme.HighContrast(th)
	}
	return th
}

// systemHighContrast reads the backend's preference (false, false when it
// has none).
func systemHighContrast(app *tango.App) (on, ok bool) {
	if app == nil {
		return false, false
	}
	cp, ok := app.Backend().(platform.ContrastPreference)
	if !ok {
		return false, false
	}
	return cp.HighContrast(), true
}

// SystemHighContrast reports whether the system asks for increased
// contrast; ok is false when the platform cannot tell. Main thread.
func SystemHighContrast(app *tango.App) (on, ok bool) { return systemHighContrast(app) }

// contrastChanged runs on the main thread after AppContrastChanged.
func (af *appFollow) contrastChanged() {
	on, ok := systemHighContrast(af.app)
	if !ok {
		return
	}
	for _, f := range af.snapshot() {
		if !f.theme || f.opts.Contrast != ContrastSystem || f.hc == on {
			continue
		}
		if f.r.Context().Theme != f.last {
			f.theme = false // the app set its own theme: leave it alone
			continue
		}
		f.applyContrast(f.sys, on)
	}
}
