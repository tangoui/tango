//go:build android

package android

/*
#include "jni_android.h"
*/
import "C"

import (
	"github.com/tangoui/tango/internal/platform"
)

// System settings the core follows (docs/specs/mobile-canvas.md §3.3), over
// Mobile.call: night mode and the system accent color (SystemTheme,
// AppThemeChanged from onConfigurationChanged), the animator duration
// scale set to off (MotionPreference, AppReduceMotionChanged from a
// ContentObserver) and the language list (Locales, LocaleWatcher from
// onConfigurationChanged). uihost's theme, reduce-motion and locale
// following then work unchanged. Android 14+ also has a contrast level
// (UiModeManager.getContrast, public API): medium or high reports
// HighContrast (ContrastPreference, AppContrastChanged). Older versions
// have no public high-contrast setting ("High contrast text" is not
// readable by apps) and report false.

var (
	_ platform.SystemTheme      = (*Backend)(nil)
	_ platform.MotionPreference = (*Backend)(nil)
	_ platform.Locales          = (*Backend)(nil)
	_ platform.LocaleWatcher    = (*Backend)(nil)

	_ platform.ContrastPreference = (*Backend)(nil)
)

// SystemTheme reports night mode and, on Android 12+, the accent color of
// the system palette. Any goroutine.
func (b *Backend) SystemTheme() platform.Theme {
	s, err := mobileCall(0, "systemTheme", nil)
	if err != nil {
		return platform.Theme{}
	}
	return decodeTheme(s)
}

// ReduceMotion reports whether animations are off (Developer options or
// Accessibility "Remove animations": the animator duration scale is 0).
// Any goroutine.
func (b *Backend) ReduceMotion() bool {
	s, err := mobileCall(0, "reduceMotion", nil)
	return err == nil && s == "1"
}

// PreferredLocales returns the system language list (LocaleList), most
// preferred first. Any goroutine.
func (b *Backend) PreferredLocales() []string {
	s, err := mobileCall(0, "locales", nil)
	if err != nil {
		return nil
	}
	return decodeLocales(s)
}

// HighContrast reports an Android 14+ contrast level of medium or high.
// Any goroutine.
func (b *Backend) HighContrast() bool {
	s, err := mobileCall(0, "highContrast", nil)
	return err == nil && s == "1"
}

var localeListeners platform.LocaleListeners

// WatchLocales calls fn on the main thread after the language list
// changed (Activity.onConfigurationChanged with a new locale).
func (b *Backend) WatchLocales(fn func()) (cancel func()) {
	cancel, _ = localeListeners.Add(fn)
	return cancel
}

//export Java_org_tango_host_Mobile_nativeLocalesChanged
func Java_org_tango_host_Mobile_nativeLocalesChanged(penv *C.JNIEnv, cls C.jclass) {
	localeListeners.Notify()
}
