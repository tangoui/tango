// Package i18n gives a Tango app the user's system languages, a current
// locale the app may override, and a small message catalog with named
// arguments and CLDR plural categories.
//
// Everything works without a tango.App: the desktop backend packages register
// their system-language source when imported (the root tango package imports
// the one for the build platform), and without any backend — unit tests,
// command-line tools — the POSIX variables LANGUAGE, LC_ALL, LC_MESSAGES and
// LANG are used. Init(app) is only needed where the answer comes from the
// running Backend value (the mobile backends).
//
// All functions are safe for concurrent use.
package i18n

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// DefaultLocale is the locale everything falls back to.
const DefaultLocale = "en"

// Builtin are the locales Tango itself ships strings for (native menu roles,
// dialog buttons). Locale picks from them until the app loads a catalog.
var Builtin = []string{"en", "zh-CN", "zh-TW", "ja"}

// AppBackend is satisfied by *tango.App. It is an interface so that i18n
// does not import the root package.
type AppBackend interface {
	Backend() platform.Backend
}

type listener struct {
	id int
	fn func(locale string)
}

var state struct {
	mu        sync.RWMutex
	source    func() []string // from Init; nil = registered source or env
	preferred []string        // cached system list; nil = not read yet
	override  string          // SetLocale; "" = follow the system
	current   string          // effective locale; "" = not computed yet

	listeners []listener
	nextID    int
}

// Init makes the app's backend the source of system languages when it
// implements platform.Locales, and the watcher of their changes when it
// implements platform.LocaleWatcher. Optional on desktop (see the package
// doc).
// It re-reads the system list and notifies OnChange listeners if the
// effective locale changed.
func Init(app AppBackend) {
	if app == nil {
		return
	}
	if l, ok := app.Backend().(platform.Locales); ok {
		state.mu.Lock()
		state.source = l.PreferredLocales
		state.mu.Unlock()
	}
	if w, ok := app.Backend().(platform.LocaleWatcher); ok {
		watchBackend(w) // watch.go
	}
	Refresh()
}

func systemLocales(src func() []string) []string {
	if src == nil {
		src = platform.LocaleSource()
	}
	var out []string
	if src != nil {
		out = src()
	}
	if len(out) == 0 {
		out = platform.EnvLocales()
	}
	return normalizeList(out)
}

func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t = Canonical(t); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// Preferred returns the user's preferred languages as BCP 47 tags, most
// preferred first ("zh-Hans-CN", "en-US"). The list is read once and cached;
// Refresh reads it again. It may be empty.
func Preferred() []string {
	ensureWatch()
	state.mu.RLock()
	p := state.preferred
	state.mu.RUnlock()
	if p == nil {
		state.mu.Lock()
		if state.preferred == nil {
			state.preferred = systemLocales(state.source)
		}
		p = state.preferred
		state.mu.Unlock()
	}
	return slices.Clone(p)
}

// Locale returns the effective locale: the SetLocale override if any,
// otherwise the best match of Preferred among Supported. It is always one of
// Supported (or the override as given, canonicalized).
func Locale() string {
	state.mu.RLock()
	cur := state.current
	state.mu.RUnlock()
	if cur != "" {
		return cur
	}
	Preferred() // fills the cache outside the lock below
	state.mu.Lock()
	if state.current == "" {
		state.current = effectiveLocked()
	}
	cur = state.current
	state.mu.Unlock()
	return cur
}

// effectiveLocked computes the effective locale; state.mu is held.
func effectiveLocked() string {
	if state.override != "" {
		return state.override
	}
	return Match(state.preferred, supported())
}

// SetLocale overrides the system languages for this app. tag is matched
// against Supported ("zh-Hans" -> "zh-CN"); a tag nothing matches is used
// as given. An empty tag goes back to following the system. OnChange
// listeners run (on the calling goroutine) if the effective locale changed.
func SetLocale(tag string) {
	tag = Canonical(tag)
	if tag != "" {
		if m, ok := MatchOK([]string{tag}, supported()); ok {
			tag = m
		}
	}
	update(func() { state.override = tag })
}

// Refresh re-reads the system languages, e.g. after the user changed them
// in the system settings, and notifies listeners if the effective locale
// changed. On macOS and Windows Tango calls it itself when the system
// reports a change (see watch.go); elsewhere (Linux) the app calls it, for
// example when its window regains focus.
func Refresh() {
	state.mu.RLock()
	src := state.source
	state.mu.RUnlock()
	p := systemLocales(src)
	update(func() { state.preferred = p })
}

// update applies change under the lock, recomputes the effective locale and
// notifies listeners outside the lock.
func update(change func()) {
	Preferred()
	state.mu.Lock()
	old := state.current
	if old == "" {
		old = effectiveLocked()
	}
	change()
	state.current = effectiveLocked()
	cur := state.current
	var fns []func(string)
	if cur != old {
		for _, l := range state.listeners {
			fns = append(fns, l.fn)
		}
	}
	state.mu.Unlock()
	for _, fn := range fns {
		fn(cur)
	}
}

// OnChange registers fn to run with the new effective locale whenever it
// changes (SetLocale, Refresh, Init, or Load changing Supported). fn runs on
// the goroutine that caused the change; for a change of the system
// languages that the platform reports, the main thread on macOS and
// Windows. The returned function unregisters.
func OnChange(fn func(locale string)) (cancel func()) {
	state.mu.Lock()
	state.nextID++
	id := state.nextID
	state.listeners = append(state.listeners, listener{id, fn})
	state.mu.Unlock()
	return func() {
		state.mu.Lock()
		state.listeners = slices.DeleteFunc(state.listeners, func(l listener) bool { return l.id == id })
		state.mu.Unlock()
	}
}

// Supported returns the locales Locale chooses from: those of the loaded
// catalogs, or Builtin when none is loaded.
func Supported() []string {
	return slices.Clone(supported())
}

// supported reads the catalog's language list, an atomic snapshot, so it can
// be called with state.mu held. Load publishes the new list first and then
// runs update, which recomputes the effective locale.
func supported() []string {
	if l := cat.langsSnapshot(); len(l) > 0 {
		return l
	}
	return Builtin
}

// resetForTest clears all state.
func resetForTest() {
	resetWatchForTest()
	state.mu.Lock()
	state.source, state.preferred, state.override, state.current = nil, nil, "", ""
	state.listeners = nil
	state.mu.Unlock()
	cat.mu.Lock()
	cat.msgs = map[string]map[string]message{}
	cat.setLangs(nil)
	cat.mu.Unlock()
	warned = sync.Map{}
}
