// Package device holds small device controls of mobile apps: asking for
// a store rating, opening the app's settings page, keeping the screen on,
// screen brightness, the status bar, and the software keyboard.
//
//	device.KeepScreenOn(a, true)
//	device.SetStatusBar(a, device.StatusBar{Style: device.StatusBarLight})
//	device.OnKeyboard(a, func(h float64) { ... }) // h = 0: hidden
//
// | | iOS | Android |
// |---|---|---|
// | RequestReview | SKStoreReviewController (the system decides whether to show it; never in TestFlight) | opens the Play Store page (market://, else https://play.google.com) |
// | OpenSettings | UIApplicationOpenSettingsURLString | ACTION_APPLICATION_DETAILS_SETTINGS |
// | KeepScreenOn | UIApplication.idleTimerDisabled | FLAG_KEEP_SCREEN_ON |
// | Brightness | UIScreen.brightness (the whole screen, until the device locks) | the window's screenBrightness (only while the app is in front); read: the system setting |
// | SetStatusBar | the pages' preferredStatusBarStyle / prefersStatusBarHidden; Color: ErrUnsupported | WindowInsetsController appearance / hide; Color fills the bar's background |
// | keyboard | UIKeyboardWillChangeFrame / WillHide notifications | IME window insets (API 30+; older: 0) |
//
// Every function returns ErrUnsupported on desktop and tango.ErrNotRunning
// after the app quit.
package device

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var ErrUnsupported = platform.ErrUnsupported

// StatusBar describes how the status bar looks.
type StatusBar = platform.StatusBar

type StatusBarStyle = platform.StatusBarStyle

const (
	StatusBarDefault = platform.StatusBarDefault // follows light/dark mode
	StatusBarLight   = platform.StatusBarLight   // white content, for dark pages
	StatusBarDark    = platform.StatusBarDark    // dark content, for light pages
)

func call[I any](a *tango.App, fn func(I) error) error {
	i, err := capcall.Backend[I](a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, fn(i) })
	return err
}

// RequestReview asks the user to rate the app. Ask at a natural pause, not
// in response to a button labelled "Rate us" on iOS (the prompt may not
// appear); Android opens the store page.
func RequestReview(a *tango.App) error {
	return call(a, platform.StoreReview.RequestReview)
}

// OpenSettings opens the app's page in the system settings, e.g. after the
// user denied a permission.
func OpenSettings(a *tango.App) error {
	return call(a, platform.SettingsOpener.OpenAppSettings)
}

// KeepScreenOn stops (on) or allows (off) the screen from dimming and
// locking while the app is in front.
func KeepScreenOn(a *tango.App, on bool) error {
	return call(a, func(s platform.ScreenControl) error { return s.SetKeepScreenOn(on) })
}

// Brightness returns the screen brightness, 0 to 1.
func Brightness(a *tango.App) (float64, error) {
	s, err := capcall.Backend[platform.ScreenControl](a)
	if err != nil {
		return 0, err
	}
	return capcall.Sync(a, s.Brightness)
}

// SetBrightness sets the screen brightness (clamped to 0-1).
func SetBrightness(a *tango.App, v float64) error {
	v = min(max(v, 0), 1)
	return call(a, func(s platform.ScreenControl) error { return s.SetBrightness(v) })
}

// ResetBrightness gives the brightness back to the system (Android; on iOS
// the system restores its own value when the device locks).
func ResetBrightness(a *tango.App) error {
	return call(a, func(s platform.ScreenControl) error { return s.SetBrightness(-1) })
}

// SetStatusBar sets the status bar's content style, visibility and (Android)
// background color.
func SetStatusBar(a *tango.App, s StatusBar) error {
	return call(a, func(c platform.StatusBarControl) error { return c.SetStatusBar(s) })
}

// KeyboardHeight returns the height of the software keyboard in CSS
// pixels, 0 when hidden.
func KeyboardHeight(a *tango.App) (float64, error) {
	k, err := capcall.Backend[platform.Keyboard](a)
	if err != nil {
		return 0, err
	}
	return capcall.Sync(a, func() (float64, error) { return k.KeyboardHeight(), nil })
}

// HideKeyboard dismisses the software keyboard.
func HideKeyboard(a *tango.App) error {
	return call(a, platform.Keyboard.HideKeyboard)
}

// OnKeyboard calls fn on the main thread when the keyboard appears,
// disappears (height 0) or changes height. It does nothing where the
// backend reports no keyboard.
func OnKeyboard(a *tango.App, fn func(height float64)) {
	if _, ok := a.Backend().(platform.Keyboard); !ok {
		return
	}
	a.OnEventKind(platform.AppKeyboardChanged, func(ev tango.AppEvent) { fn(ev.Insets.Bottom) })
}
