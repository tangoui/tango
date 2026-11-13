package i18n

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

// recorder collects the locales OnChange reports.
type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) add(l string) { r.mu.Lock(); r.got = append(r.got, l); r.mu.Unlock() }
func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.got)
}

func TestWatchBackendLocales(t *testing.T) {
	setup(t, nil)
	b := fake.New()
	b.SetSystemLocales("en-US")
	Init(fakeApp{b})
	if got := Locale(); got != "en" {
		t.Fatalf("Locale() = %q, want en", got)
	}
	var r recorder
	cancel := OnChange(r.add)
	defer cancel()

	b.SetSystemLocales("zh-Hans-CN", "en-US") // the user changes the system language
	if got := Locale(); got != "zh-CN" {
		t.Errorf("after the change Locale() = %q, want zh-CN", got)
	}
	if got := Preferred(); !slices.Equal(got, []string{"zh-Hans-CN", "en-US"}) {
		t.Errorf("Preferred() = %v", got)
	}
	b.SetSystemLocales("zh-Hans-CN") // effective locale unchanged: no event
	b.SetSystemLocales("ja-JP")
	if got, want := r.list(), []string{"zh-CN", "ja"}; !slices.Equal(got, want) {
		t.Errorf("OnChange saw %v, want %v", got, want)
	}

	// An override wins over the system; the watcher keeps the list fresh.
	SetLocale("zh-TW")
	b.SetSystemLocales("en-US")
	if got := Locale(); got != "zh-TW" {
		t.Errorf("with override Locale() = %q, want zh-TW", got)
	}
	if got := Preferred(); !slices.Equal(got, []string{"en-US"}) {
		t.Errorf("Preferred() with override = %v", got)
	}
}

func TestWatchRegisteredSource(t *testing.T) {
	setup(t, nil)
	b := fake.New() // stands in for the desktop backend package's registration
	b.SetSystemLocales("en-GB")
	prevWatch := platform.LocaleWatch()
	platform.SetLocaleSource(b.PreferredLocales)
	platform.SetLocaleWatch(b.WatchLocales)
	t.Cleanup(func() { platform.SetLocaleWatch(prevWatch) })
	resetForTest()

	var r recorder
	cancel := OnChange(r.add)
	defer cancel()
	if got := Locale(); got != "en" { // the first read subscribes, no Init needed
		t.Fatalf("Locale() = %q, want en", got)
	}
	b.SetSystemLocales("ja")
	if got := r.list(); !slices.Equal(got, []string{"ja"}) {
		t.Errorf("OnChange saw %v, want [ja]", got)
	}

	// Init with a backend watcher replaces the registered one.
	other := fake.New()
	other.SetSystemLocales("zh-TW")
	Init(fakeApp{other})
	b.SetSystemLocales("en") // no longer watched
	if got := Locale(); got != "zh-TW" {
		t.Errorf("after Init Locale() = %q, want zh-TW", got)
	}
	other.SetSystemLocales("en")
	if got := Locale(); got != "en" {
		t.Errorf("after the backend's change Locale() = %q, want en", got)
	}
}

func TestWatchCancelOnReset(t *testing.T) {
	setup(t, nil)
	b := fake.New()
	Init(fakeApp{b})
	resetForTest() // cancels the subscription
	var r recorder
	state.mu.Lock()
	state.listeners = append(state.listeners, listener{-1, r.add})
	state.mu.Unlock()
	b.SetSystemLocales("ja")
	if got := r.list(); len(got) != 0 {
		t.Errorf("a cancelled watcher still refreshed: %v", got)
	}
}
