package platform

import (
	"os"
	"strings"
	"sync"
)

// Locales is an optional Backend capability: the user's preferred UI
// languages as BCP 47 tags ("zh-Hans-CN", "en-US"), most preferred first.
// Unlike the other capabilities it may be called from any goroutine.
type Locales interface {
	PreferredLocales() []string
}

// A desktop backend package can also register a backend-independent source
// of the same list from its init function, so the public i18n package works
// before (or without) a tango.App. Mobile backends, whose answer depends on
// the running host, implement Locales on the Backend value instead.

var (
	localeMu     sync.RWMutex
	localeSource func() []string
)

// SetLocaleSource registers the system-language source used when no Backend
// value is at hand. The last registration wins.
func SetLocaleSource(fn func() []string) {
	localeMu.Lock()
	localeSource = fn
	localeMu.Unlock()
}

// LocaleSource returns the registered source, or nil.
func LocaleSource() func() []string {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return localeSource
}

// EnvLocales reads the POSIX locale variables in gettext priority order:
// LANGUAGE (a colon-separated list) first, then the first non-empty of
// LC_ALL, LC_MESSAGES and LANG. Values are converted to BCP 47
// ("zh_CN.UTF-8" -> "zh-CN"); "C" and "POSIX" are skipped. As in gettext,
// LANGUAGE is ignored when the locale is explicitly set to "C" or "POSIX".
func EnvLocales() []string {
	var primary string
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			primary = v
			break
		}
	}
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		if t := PosixToBCP47(v); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if p := PosixToBCP47(primary); p != "" || primary == "" {
		for _, v := range strings.Split(os.Getenv("LANGUAGE"), ":") {
			add(v)
		}
	}
	add(primary)
	return out
}

// PosixToBCP47 converts a POSIX locale name ("zh_TW.UTF-8@euro") to a
// BCP 47 tag ("zh-TW"). It returns "" for "C", "POSIX" and empty input.
func PosixToBCP47(v string) string {
	if i := strings.IndexAny(v, ".@"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if v == "" || v == "C" || v == "POSIX" {
		return ""
	}
	return strings.ReplaceAll(v, "_", "-")
}

// LocaleWatcher is an optional Backend capability: WatchLocales calls fn
// after the system language list may have changed (fn may also run for
// changes that leave the list as it was). fn runs on the main thread when
// the backend knows one, otherwise on any goroutine. cancel stops the
// calls; it may be called more than once.
//
// Like Locales, a desktop backend package can also register a
// backend-independent watcher with SetLocaleWatch from its init function.
// Backends whose system has no change notification (Linux) implement
// neither, and apps call i18n.Refresh themselves.
type LocaleWatcher interface {
	WatchLocales(fn func()) (cancel func())
}

var localeWatch func(fn func()) (cancel func())

// SetLocaleWatch registers the watcher used when no Backend value is at
// hand (the WatchLocales of a LocaleWatcher). The last registration wins.
func SetLocaleWatch(watch func(fn func()) (cancel func())) {
	localeMu.Lock()
	localeWatch = watch
	localeMu.Unlock()
}

// LocaleWatch returns the registered watcher, or nil.
func LocaleWatch() func(fn func()) (cancel func()) {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return localeWatch
}

// LocaleListeners is a set of change callbacks for backends implementing
// LocaleWatcher: Add registers one, Notify calls them all. The zero value
// is ready to use and safe for concurrent use.
type LocaleListeners struct {
	mu   sync.Mutex
	next int
	fns  map[int]func()
}

// Add registers fn and returns its cancel function. first reports whether
// fn is the first listener ever added, so a backend can start observing
// lazily.
func (l *LocaleListeners) Add(fn func()) (cancel func(), first bool) {
	l.mu.Lock()
	if l.fns == nil {
		l.fns = map[int]func(){}
		first = l.next == 0
	}
	l.next++
	k := l.next
	l.fns[k] = fn
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		delete(l.fns, k)
		l.mu.Unlock()
	}, first
}

// Notify calls every registered fn, outside the lock, on the calling
// goroutine.
func (l *LocaleListeners) Notify() {
	l.mu.Lock()
	fns := make([]func(), 0, len(l.fns))
	for _, fn := range l.fns {
		fns = append(fns, fn)
	}
	l.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
