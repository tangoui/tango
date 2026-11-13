//go:build windows && (amd64 || arm64)

package windows

import "github.com/tangoui/tango/internal/platform"

// Windows announces a change of the display languages or regional format
// with WM_SETTINGCHANGE whose lParam is "intl", broadcast to top-level
// windows. The hidden application window receives it (systemMessage in
// integration_windows.go calls localeSettingChanged), so the watcher only
// fires while the app runs; on the main thread.
func init() { platform.SetLocaleWatch(watchLocales) }

var _ platform.LocaleWatcher = (*Backend)(nil)

// WatchLocales implements platform.LocaleWatcher.
func (b *Backend) WatchLocales(fn func()) (cancel func()) { return watchLocales(fn) }

var localeListeners platform.LocaleListeners

func watchLocales(fn func()) (cancel func()) {
	cancel, _ = localeListeners.Add(fn)
	return cancel
}

// localeSettingChanged handles one WM_SETTINGCHANGE; area is its lParam
// string ("" when lParam is 0).
func localeSettingChanged(area string) {
	if area == "intl" {
		localeListeners.Notify()
	}
}
