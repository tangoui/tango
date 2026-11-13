package ui

import (
	"strings"
	"sync/atomic"
)

// Layout direction defaults and the locale → direction table
// (docs/specs/rtl-and-component-tokens.md §3.1).

var defaultDir atomic.Uint32 // Direction; DirAuto out of the box

// SetDefaultDirection sets the window-level direction used where no
// ConfigProvider decides: pages without a provider and the global layers.
// DirAuto (the default) derives it from the default locale. Hosts that
// follow the system layout direction inject it here. It takes effect on
// the next frame.
func SetDefaultDirection(d Direction) { defaultDir.Store(uint32(d)) }

// DefaultDirection returns the value set by SetDefaultDirection.
func DefaultDirection() Direction { return Direction(defaultDir.Load()) }

// resolveRTL resolves a scope's direction: the explicit direction of the
// nearest provider, else the scope's locale, else SetDefaultDirection,
// else the default locale.
func resolveRTL(explicit Direction, loc *Locale) bool {
	switch explicit {
	case LTR:
		return false
	case RTL:
		return true
	}
	if loc != nil {
		return loc.rtl()
	}
	switch DefaultDirection() {
	case LTR:
		return false
	case RTL:
		return true
	}
	if l := defaultLocaleNow(); l != nil {
		return l.rtl()
	}
	return false
}

// IsRTL reports whether the locale lays out right to left: Dir when set,
// else inferred from Code.
func (l *Locale) IsRTL() bool { return l != nil && l.rtl() }

func (l *Locale) rtl() bool {
	switch l.Dir {
	case LTR:
		return false
	case RTL:
		return true
	}
	return localeRTL(l.Code)
}

// rtlLanguages are language subtags whose default script runs right to
// left.
var rtlLanguages = [...]string{
	"ar", "arc", "ckb", "dv", "fa", "he", "iw", "ks", "nqo", "ps", "sd", "syr", "ug", "ur", "yi",
}

// rtlScripts are ISO 15924 script subtags written right to left.
var rtlScripts = [...]string{"Adlm", "Arab", "Hebr", "Mand", "Nkoo", "Rohg", "Syrc", "Thaa"}

// localeRTL infers the direction of a BCP 47 / POSIX locale tag
// ("ar-EG", "fa_IR.UTF-8", "az-Arab", "ku-Latn"). An explicit script
// subtag decides on its own (so -Latn / -Cyrl force LTR); otherwise the
// language subtag does. It does not allocate.
func localeRTL(code string) bool {
	// Drop POSIX encoding / modifier suffixes.
	if i := strings.IndexAny(code, ".@"); i >= 0 {
		code = code[:i]
	}
	lang, rest, _ := cutTag(code)
	for rest != "" {
		var sub string
		sub, rest, _ = cutTag(rest)
		if len(sub) == 4 && isAlpha(sub) {
			for _, s := range rtlScripts {
				if strings.EqualFold(sub, s) {
					return true
				}
			}
			return false // an explicit non-RTL script (Latn, Cyrl, Hans ...)
		}
	}
	for _, l := range rtlLanguages {
		if strings.EqualFold(lang, l) {
			return true
		}
	}
	return false
}

// cutTag splits a locale tag at the first '-' or '_'.
func cutTag(s string) (head, tail string, found bool) {
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i] | 0x20; b < 'a' || b > 'z' {
			return false
		}
	}
	return true
}
