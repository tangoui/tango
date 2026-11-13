//go:build ios

package ios

/*
#include <stdlib.h>
#include "system.h"
*/
import "C"

import (
	"strings"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// System settings the self-drawn UI follows (docs/specs/mobile-canvas.md
// §3.3): uihost's theme, reduce-motion, contrast and language following
// work unchanged once the backend has these capabilities. Implemented in
// system.m; the values are cached there, so every method may be called
// from any goroutine.

var (
	_ platform.SystemTheme        = (*backend)(nil)
	_ platform.MotionPreference   = (*backend)(nil)
	_ platform.ContrastPreference = (*backend)(nil)
	_ platform.Locales            = (*backend)(nil)
	_ platform.LocaleWatcher      = (*backend)(nil)
)

// SystemTheme reports the system light/dark appearance. iOS has no
// system accent color, so Accent stays 0 (unknown).
func (b *backend) SystemTheme() platform.Theme { return platform.Theme{Dark: C.tgs_dark() != 0} }

// ReduceMotion reports Settings > Accessibility > Motion > Reduce Motion.
func (b *backend) ReduceMotion() bool { return C.tgs_reduce_motion() != 0 }

// HighContrast reports Settings > Accessibility > Display & Text Size >
// Increase Contrast.
func (b *backend) HighContrast() bool { return C.tgs_high_contrast() != 0 }

// PreferredLocales returns NSLocale.preferredLanguages.
func (b *backend) PreferredLocales() []string {
	p := C.tgs_locales()
	defer C.free(unsafe.Pointer(p))
	return splitLocales(C.GoString(p))
}

func splitLocales(s string) []string {
	var out []string
	for _, t := range strings.Split(s, "\n") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

var localeListeners platform.LocaleListeners

// WatchLocales calls fn on the main thread after the current locale
// changed (NSCurrentLocaleDidChangeNotification). Changing the preferred
// language list itself relaunches iOS apps.
func (b *backend) WatchLocales(fn func()) (cancel func()) {
	cancel, first := localeListeners.Add(fn)
	if first {
		C.tgs_watch_locales()
	}
	return cancel
}

//export tangoIOSSystemChanged
func tangoIOSSystemChanged(kind C.int) {
	switch kind {
	case C.TGS_THEME:
		emitEvent(platform.AppEvent{Kind: platform.AppThemeChanged, Theme: platform.Theme{Dark: C.tgs_dark() != 0}})
	case C.TGS_MOTION:
		emitEvent(platform.AppEvent{Kind: platform.AppReduceMotionChanged})
	case C.TGS_CONTRAST:
		emitEvent(platform.AppEvent{Kind: platform.AppContrastChanged})
	case C.TGS_LOCALE:
		localeListeners.Notify()
	}
}
