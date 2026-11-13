package fake

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// The fake backend's system languages: platform.Locales and
// platform.LocaleWatcher, with SetSystemLocales to simulate the user
// changing the language list in the system settings. State lives beside
// Backend (keyed by it), as in integration.go.

var (
	_ platform.Locales       = (*Backend)(nil)
	_ platform.LocaleWatcher = (*Backend)(nil)
)

type localeState struct {
	mu        sync.Mutex
	langs     []string
	listeners platform.LocaleListeners
}

var localeStates sync.Map // *Backend -> *localeState

func (b *Backend) locales() *localeState {
	s, _ := localeStates.LoadOrStore(b, &localeState{langs: []string{"en-US"}})
	return s.(*localeState)
}

// PreferredLocales implements platform.Locales: ["en-US"] until
// SetSystemLocales changes it. Any goroutine.
func (b *Backend) PreferredLocales() []string {
	s := b.locales()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.langs)
}

// WatchLocales implements platform.LocaleWatcher. fn runs on the goroutine
// that calls SetSystemLocales.
func (b *Backend) WatchLocales(fn func()) (cancel func()) {
	cancel, _ = b.locales().listeners.Add(fn)
	return cancel
}

// SetSystemLocales simulates a change of the system language list: it
// replaces what PreferredLocales returns and then notifies the watchers,
// as the real backends do on their system notification.
func (b *Backend) SetSystemLocales(tags ...string) {
	s := b.locales()
	s.mu.Lock()
	s.langs = slices.Clone(tags)
	s.mu.Unlock()
	s.listeners.Notify()
}
