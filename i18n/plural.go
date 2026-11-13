package i18n

import (
	"strings"
	"sync"
)

// A PluralRule maps a count to a CLDR plural category: "zero", "one",
// "two", "few", "many" or "other".
type PluralRule func(n int) string

var plurals = struct {
	mu    sync.RWMutex
	rules map[string]PluralRule // base language -> rule
}{rules: map[string]PluralRule{}}

func otherOnly(int) string { return "other" }

// oneOther is the rule of English and most Germanic and Romance languages
// for integers: 1 is "one", everything else "other".
func oneOther(n int) string {
	if n == 1 {
		return "one"
	}
	return "other"
}

// zeroOneOther is French/Portuguese style: 0 and 1 are "one".
func zeroOneOther(n int) string {
	if n == 0 || n == 1 {
		return "one"
	}
	return "other"
}

func init() {
	for _, l := range []string{"zh", "ja", "ko", "vi", "th", "id", "ms", "lo", "my", "km", "yue"} {
		plurals.rules[l] = otherOnly
	}
	for _, l := range []string{"en", "de", "nl", "sv", "da", "nb", "nn", "no", "fi", "et", "it", "es", "ca", "gl", "el", "hu", "tr", "bg", "af"} {
		plurals.rules[l] = oneOther
	}
	for _, l := range []string{"fr", "pt", "hi", "bn", "fa"} {
		plurals.rules[l] = zeroOneOther
	}
}

// RegisterPluralRule sets the rule for a base language ("ru", "ar", ...),
// replacing a built-in one. Languages without a rule use English's.
func RegisterPluralRule(lang string, rule PluralRule) {
	lang = baseLang(lang)
	plurals.mu.Lock()
	plurals.rules[lang] = rule
	plurals.mu.Unlock()
}

// PluralCategory returns the CLDR plural category of n in locale.
func PluralCategory(locale string, n int) string {
	plurals.mu.RLock()
	rule, ok := plurals.rules[baseLang(locale)]
	plurals.mu.RUnlock()
	if !ok {
		rule = oneOther
	}
	if n < 0 {
		n = -n
	}
	return rule(n)
}

func baseLang(tag string) string {
	tag = Canonical(tag)
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		tag = tag[:i]
	}
	return tag
}
