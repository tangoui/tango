package i18n

import (
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

// The system language list is watched where the platform can say when it
// changes (platform.LocaleWatcher: macOS, Windows); a change runs Refresh,
// which notifies OnChange listeners if the effective locale changed. On
// Linux there is no standard notification and apps call Refresh themselves.
//
// Watching starts the first time the system languages are read (Preferred,
// Locale and everything built on them, such as the native menu role
// labels) with the watcher the desktop backend registered; Init switches
// to the app backend's own watcher when it has one. Registering an
// OnChange listener alone starts nothing, so packages may do it from their
// init functions without side effects.

var watch struct {
	started atomic.Bool
	mu      sync.Mutex
	cancel  func()
}

// ensureWatch subscribes to the registered system-language watcher once.
func ensureWatch() {
	if watch.started.Load() {
		return
	}
	watch.mu.Lock()
	defer watch.mu.Unlock()
	if watch.started.Load() {
		return
	}
	// Marked first: a watcher that calls back at once must not re-enter.
	watch.started.Store(true)
	if w := platform.LocaleWatch(); w != nil {
		watch.cancel = w(Refresh)
	}
}

// watchBackend replaces the current subscription with w's.
func watchBackend(w platform.LocaleWatcher) {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	if watch.cancel != nil {
		watch.cancel()
	}
	watch.cancel = w.WatchLocales(Refresh)
	watch.started.Store(true)
}

// resetWatchForTest drops the subscription; the next use subscribes again.
func resetWatchForTest() {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	if watch.cancel != nil {
		watch.cancel()
	}
	watch.cancel = nil
	watch.started.Store(false)
}
