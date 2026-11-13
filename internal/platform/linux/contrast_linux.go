//go:build linux && !android

package linux

// platform.ContrastPreference: the XDG settings portal's
// org.freedesktop.appearance contrast (xdg-desktop-portal 1.19+; GNOME
// 47+ maps Settings > Accessibility > High Contrast to it), else a GTK theme
// named like "HighContrast" (contrastIsHigh in integration.go). Changes
// arrive through the signals installSystemWatch already listens to for the
// theme — the portal's SettingChanged on that namespace and
// notify::gtk-theme-name — and are reported as AppContrastChanged.
//
// Desktops with neither source (KDE Plasma's own high-contrast color
// schemes, a GNOME older than 47 with only the GSettings key
// org.gnome.desktop.a11y.interface high-contrast and a non-HighContrast
// GTK theme) read as false: the backend cannot tell, and the GSettings
// schema is not read directly because it may be absent.

import (
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.ContrastPreference = (*backend)(nil)

var contrast struct {
	high    atomic.Bool
	watched bool // main thread
}

// HighContrast implements platform.ContrastPreference. Any goroutine: it
// reads the value the main thread last saw (Init reads it first).
func (b *backend) HighContrast() bool { return contrast.high.Load() }

// installContrastWatch reads the starting value. Called from Init after
// installSystemWatch (main thread), whose signals call
// contrastMaybeChanged.
func (b *backend) installContrastWatch() {
	if contrast.watched || !bindIntegration() {
		return
	}
	contrast.watched = true
	contrast.high.Store(readHighContrast())
}

// readHighContrast asks the portal, then GtkSettings. Main thread.
func readHighContrast() bool {
	var value uint32
	have := false
	if v := portalRead("contrast"); v != 0 {
		if goStr(ix.variantGetTypeString(v)) == "u" {
			value, have = gVariantGetUint32(v), true
		}
		gVariantUnref(v)
	}
	themeName := ""
	if st := ix.settingsGetDefault(); st != 0 {
		_, themeName = gtkSetting(st, "gtk-theme-name", gTypeString)
	}
	return contrastIsHigh(value, have, themeName)
}

// contrastMaybeChanged runs on the main thread for the theme signals.
func (b *backend) contrastMaybeChanged() {
	if !contrast.watched {
		return
	}
	v := readHighContrast()
	if contrast.high.Swap(v) == v {
		return
	}
	if hx, ok := b.host.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppContrastChanged})
	}
}
