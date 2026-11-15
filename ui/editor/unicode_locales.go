package editor

import (
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// Per-locale ambiguous characters (Monaco's unicodeHighlight.allowedLocales
// and AmbiguousCharacters.getInstance):
//
// Monaco's data (unicode_gen.go) has a _common table, in every locale's
// table, a _default table and a table per locale ("de", "ru", "zh-hans",
// ...). The locales in effect are the names of AllowedLocales, with "_os"
// standing for the system's locales and "_vscode" for the user interface's
// (SetSystemLocales). As getInstance: names starting with "_" and names
// that are not exactly a key of the data are dropped ("en-US" names no
// table, nor does "pt": the key is "pt-BR"); with none left the _default
// table is used; the tables of the locales left are intersected (a
// character is ambiguous only when every locale finds it so) and merged
// into _common. So a Russian locale stops marking the Cyrillic "а", and
// English (no table of its own) keeps the _default table.
//
// Monaco's "_os" is the browser's resolved Intl locale and "_vscode" the
// UI language; Monaco matches them as they are, so an OS locale "ru-RU"
// selects no table there. Without SetSystemLocales "_os" here is the
// locale of $LC_ALL, $LC_MESSAGES or $LANG (the first set) both with its
// region ("ru-RU") and as its language alone ("ru"): the language part
// is what selects a table for most locales, the full form selects
// "pt-BR"; "C" and "POSIX" are "en". "_vscode" is "en" (the standalone
// editor's UI language).
//
// Resolved tables are cached per set of names (Monaco caches its last one).

// LocaleSet is the set of locale names of
// UnicodeHighlightOptions.AllowedLocales (Monaco's allowedLocales record:
// the names whose value is true). The zero value is Monaco's default
// {"_os": true, "_vscode": true}; AllowLocales() with no names is the
// empty set (the _default table). LocaleSets compare with ==, so Options
// stays comparable.
type LocaleSet struct {
	set   bool
	names string // the sorted distinct names, each followed by "\x00"
}

// AllowLocales returns the set of the names (Monaco's allowedLocales with
// each name true).
func AllowLocales(names ...string) LocaleSet {
	s := slices.Clone(names)
	slices.Sort(s)
	s = slices.Compact(s)
	var sb strings.Builder
	for _, n := range s {
		sb.WriteString(n)
		sb.WriteByte(0)
	}
	return LocaleSet{set: true, names: sb.String()}
}

// LocaleSetOf returns the set of the names m maps to true, as Monaco
// validates allowedLocales; nil is the default set.
func LocaleSetOf(m map[string]bool) LocaleSet {
	if m == nil {
		return LocaleSet{}
	}
	var names []string
	for n, on := range m {
		if on {
			names = append(names, n)
		}
	}
	return AllowLocales(names...)
}

// IsDefault reports whether s is the default set {"_os", "_vscode"}.
func (s LocaleSet) IsDefault() bool { return !s.set }

// Names returns the names of s, sorted.
func (s LocaleSet) Names() []string {
	if !s.set {
		return []string{"_os", "_vscode"}
	}
	if s.names == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(s.names, "\x00"), "\x00")
}

// systemLocales are the locales of SetSystemLocales (nil: the default).
type systemLocales struct{ os, ui []string }

var sysLocales atomic.Pointer[systemLocales]

// SetSystemLocales sets the locales "_os" (the system's) and "_vscode"
// (the user interface's) stand for in
// UnicodeHighlightOptions.AllowedLocales, as BCP 47 names Monaco's data
// is keyed by ("de", "pt-BR", "zh-hans"; matched exactly). A nil slice
// restores that name's default: for os the locale of $LC_ALL, $LC_MESSAGES
// or $LANG, for ui "en". The host calls it at startup; it is safe to call
// from any goroutine, and views drawn afterwards use the new tables.
func SetSystemLocales(os, ui []string) {
	if os == nil && ui == nil {
		sysLocales.Store(nil)
		return
	}
	s := &systemLocales{}
	if os != nil {
		s.os = slices.Clone(os)
	}
	if ui != nil {
		s.ui = slices.Clone(ui)
	}
	sysLocales.Store(s)
}

// envLocaleNames returns the locale names of a POSIX locale value
// (language[_territory][.codeset][@modifier]): the territory form in BCP
// 47 spelling and the language alone; "C" and "POSIX" are "en".
func envLocaleNames(v string) []string {
	if i := strings.IndexAny(v, ".@"); i >= 0 {
		v = v[:i]
	}
	switch v {
	case "":
		return nil
	case "C", "POSIX":
		return []string{"en"}
	}
	lang, region, ok := strings.Cut(v, "_")
	if !ok || lang == "" || region == "" {
		return []string{strings.ReplaceAll(v, "_", "-")}
	}
	return []string{lang + "-" + region, lang}
}

// envLocale returns the first set of $LC_ALL, $LC_MESSAGES and $LANG.
func envLocale() string {
	for _, k := range [...]string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// lastLocales memoizes the last resolution of LocaleSet.ambiguous: the
// views resolve their options every frame.
type lastLocales struct {
	set LocaleSet
	sys *systemLocales
	env string
	amb *ambiguousSet
}

var lastResolved atomic.Pointer[lastLocales]

// ambiguous returns the ambiguous characters of the locales of s.
func (s LocaleSet) ambiguous() *ambiguousSet {
	sys := sysLocales.Load()
	env := ""
	usesOS := !s.set || strings.HasPrefix(s.names, "_os\x00") || strings.Contains(s.names, "\x00_os\x00")
	if usesOS && (sys == nil || sys.os == nil) {
		env = envLocale()
	}
	if l := lastResolved.Load(); l != nil && l.set == s && l.sys == sys && l.env == env {
		return l.amb
	}
	var names []string
	for _, n := range s.Names() {
		switch {
		case n == "_os" && sys != nil && sys.os != nil:
			names = append(names, sys.os...)
		case n == "_os":
			names = append(names, envLocaleNames(env)...)
		case n == "_vscode" && sys != nil && sys.ui != nil:
			names = append(names, sys.ui...)
		case n == "_vscode":
			names = append(names, "en")
		default:
			names = append(names, n)
		}
	}
	amb := ambiguousFor(strings.Join(names, ","))
	lastResolved.Store(&lastLocales{set: s, sys: sys, env: env, amb: amb})
	return amb
}

// ambiguousSet is a resolved table of ambiguous characters: pairs of a
// code point and the basic character it can be confused with, sorted by
// code point.
type ambiguousSet struct{ pairs []uint32 }

// maxAmbiguousSets bounds the cached tables (each about 13 KB).
const maxAmbiguousSets = 16

var ambiguousCache struct {
	mu   sync.Mutex
	sets map[string]*ambiguousSet
}

// ambiguousFor returns the table of the comma-separated locale names
// (AmbiguousCharacters.getInstance of the set of them).
func ambiguousFor(locales string) *ambiguousSet {
	c := &ambiguousCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if a := c.sets[locales]; a != nil {
		return a
	}
	if c.sets == nil || len(c.sets) >= maxAmbiguousSets {
		c.sets = map[string]*ambiguousSet{}
	}
	a := buildAmbiguous(strings.Split(locales, ","))
	c.sets[locales] = a
	return a
}

// localePairs returns the pairs of a key of the data (nil: no such key).
func localePairs(key string) []uint32 {
	for _, d := range ambiguousLocaleData {
		if d.locale == key {
			return d.pairs
		}
	}
	return nil
}

// buildAmbiguous is the body of Monaco's AmbiguousCharacters cache.
func buildAmbiguous(locales []string) *ambiguousSet {
	var keys []string
	for _, l := range locales {
		if !strings.HasPrefix(l, "_") && localePairs(l) != nil && !slices.Contains(keys, l) {
			keys = append(keys, l)
		}
	}
	if len(keys) == 0 {
		keys = []string{"_default"}
	}
	var specific map[uint32]uint32
	for _, k := range keys {
		p := localePairs(k)
		m := make(map[uint32]uint32, len(p)/2)
		for i := 0; i+1 < len(p); i += 2 {
			if specific == nil {
				m[p[i]] = p[i+1]
			} else if v, ok := specific[p[i]]; ok {
				m[p[i]] = v
			}
		}
		specific = m
	}
	all := make(map[uint32]uint32, len(ambiguousCommon)/2+len(specific))
	for i := 0; i+1 < len(ambiguousCommon); i += 2 {
		all[ambiguousCommon[i]] = ambiguousCommon[i+1]
	}
	for k, v := range specific {
		all[k] = v
	}
	cps := make([]uint32, 0, len(all))
	for k := range all {
		cps = append(cps, k)
	}
	slices.Sort(cps)
	pairs := make([]uint32, 0, 2*len(cps))
	for _, k := range cps {
		pairs = append(pairs, k, all[k])
	}
	return &ambiguousSet{pairs: pairs}
}
