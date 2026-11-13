//go:build darwin && !ios

package darwin

import (
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// Like the language list (locale.go), watching it needs no running app:
// the package registers the watcher for i18n when imported. Notifications
// are delivered by the main run loop, so nothing arrives before tango.Run.
func init() { platform.SetLocaleWatch(watchLocales) }

var _ platform.LocaleWatcher = (*backend)(nil)

// WatchLocales implements platform.LocaleWatcher.
func (b *backend) WatchLocales(fn func()) (cancel func()) { return watchLocales(fn) }

// localeChangeNotifications are observed for language changes:
// NSCurrentLocaleDidChangeNotification (default center; symbol is the
// Foundation constant, whose value is not its name) is posted when the
// region or the language list changes and Foundation has reset its caches;
// AppleLanguagePreferencesChangedNotification (distributed center) is what
// System Settings broadcasts when the preferred languages are edited, and
// covers systems where the former only follows region changes. Redundant
// calls are harmless: i18n only reports an actual change.
var localeChangeNotifications = []struct{ center, symbol, name string }{
	{center: "NSNotificationCenter", symbol: "NSCurrentLocaleDidChangeNotification"},
	{center: "NSDistributedNotificationCenter", name: "AppleLanguagePreferencesChangedNotification"},
}

var localeListeners platform.LocaleListeners

// watchLocales calls fn, on the main thread, whenever one of
// localeChangeNotifications arrives. The observer is installed on the
// first call and stays for the life of the process.
func watchLocales(fn func()) (cancel func()) {
	cancel, first := localeListeners.Add(fn)
	if first {
		installLocaleObserver()
	}
	return cancel
}

// installLocaleObserver registers TangoLocaleObserver with both centers.
// addObserver:selector:name:object: is thread-safe, so any goroutine may
// call it.
func installLocaleObserver() {
	load()
	onChange := func(_ id, _ objc.SEL, _ id) { localeListeners.Notify() }
	classDef("TangoLocaleObserver", "NSObject", nil, []objc.MethodDef{
		method("localeChanged:", onChange),
	})
	withPool(func() {
		obs := alloc("TangoLocaleObserver") // kept for the life of the process
		for _, n := range localeChangeNotifications {
			if !hasClass(n.center) {
				continue
			}
			name := uintptr(0)
			switch {
			case n.symbol != "":
				name = constPtr(libFoundation, n.symbol)
			default:
				name = uintptr(nsString(n.name))
			}
			if name == 0 {
				continue
			}
			center := send(class(n.center), "defaultCenter")
			send(center, "addObserver:selector:name:object:",
				uintptr(obs), uintptr(sel("localeChanged:")), name, 0)
		}
	})
}
