package i18n

import "strings"

// Tango matches languages itself instead of depending on
// golang.org/x/text/language: the rules it needs (exact tag, same language,
// and the Simplified/Traditional Chinese split) fit in a page, and the full
// CLDR matcher would add a dependency and ~1 MB of tables to every app.

// Canonical normalizes a BCP 47 tag or POSIX locale name: "zh_hans_cn.UTF-8"
// -> "zh-Hans-CN", "EN-us" -> "en-US". It returns "" for "", "C" and
// "POSIX". It does not validate the tag.
func Canonical(tag string) string {
	tag = strings.TrimSpace(tag)
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	if tag == "" || tag == "C" || tag == "POSIX" {
		return ""
	}
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToLower(p)
		case len(p) == 4 && isAlpha(p):
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case len(p) == 2 && isAlpha(p), len(p) == 3 && isDigit(p):
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, "-")
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func isDigit(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type tagParts struct{ lang, script, region string }

func parseTag(tag string) tagParts {
	var t tagParts
	for i, p := range strings.Split(Canonical(tag), "-") {
		switch {
		case i == 0:
			t.lang = p
		case len(p) == 4 && t.script == "" && t.region == "":
			t.script = p
		case (len(p) == 2 || len(p) == 3) && t.region == "":
			t.region = p
		}
	}
	return t
}

// chineseVariant returns "Hant" for Traditional Chinese (zh-Hant, zh-TW,
// zh-HK, zh-MO) and "Hans" for every other Chinese tag.
func chineseVariant(t tagParts) string {
	switch {
	case t.script != "":
		return t.script
	case t.region == "TW" || t.region == "HK" || t.region == "MO":
		return "Hant"
	}
	return "Hans"
}

// Match returns the entry of available that best serves the user's
// preference list prefs (most preferred first). Each preference is tried in
// turn against available:
//
//  1. the same tag;
//  2. for Chinese, the same script family: zh, zh-CN, zh-SG, zh-Hans-* go
//     to the Simplified entry (zh-CN), zh-TW, zh-HK, zh-MO, zh-Hant-* to
//     the Traditional one (zh-TW); the two are never substituted;
//  3. the same language and region ("pt-BR" for "pt-Hant-BR");
//  4. the same language, preferring an entry without a region ("en" for
//     "en-GB").
//
// If no preference matches, Match returns DefaultLocale when available has
// it, else available[0], else DefaultLocale.
func Match(prefs, available []string) string {
	if m, ok := MatchOK(prefs, available); ok {
		return m
	}
	for _, a := range available {
		if Canonical(a) == DefaultLocale {
			return a
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return DefaultLocale
}

// MatchOK is Match without the fallback: ok is false when no preference
// matches any entry.
func MatchOK(prefs, available []string) (string, bool) {
	for _, p := range prefs {
		if m, ok := matchOne(p, available); ok {
			return m, true
		}
	}
	return "", false
}

func matchOne(pref string, available []string) (string, bool) {
	cp := Canonical(pref)
	if cp == "" {
		return "", false
	}
	for _, a := range available {
		if Canonical(a) == cp {
			return a, true
		}
	}
	p := parseTag(cp)
	same := func(a tagParts) bool { return a.lang == p.lang }
	if p.lang == "zh" {
		v := chineseVariant(p)
		same = func(a tagParts) bool { return a.lang == "zh" && chineseVariant(a) == v }
	}
	var langOnly, anyRegion string
	for _, a := range available {
		at := parseTag(a)
		if !same(at) {
			continue
		}
		if p.region != "" && at.region == p.region {
			return a, true
		}
		if at.region == "" && langOnly == "" {
			langOnly = a
		}
		if anyRegion == "" {
			anyRegion = a
		}
	}
	if langOnly != "" {
		return langOnly, true
	}
	if anyRegion != "" {
		return anyRegion, true
	}
	return "", false
}
