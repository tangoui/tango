package chart

import (
	"math"
	"strconv"
	"strings"
)

// Formatter turns a value into a label.
type Formatter func(v float64) string

// FormatNumber formats v with thousands separators and the given number of
// decimals (negative: shortest exact representation).
func FormatNumber(v float64, decimals int) string {
	if math.IsNaN(v) {
		return "-"
	}
	if math.IsInf(v, 0) {
		if v > 0 {
			return "∞"
		}
		return "-∞"
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	out := b.String() + frac
	if neg && strings.Trim(out, "0.,") != "" {
		out = "-" + out
	}
	return out
}

// Thousands returns a formatter with thousands separators.
func Thousands(decimals int) Formatter {
	return func(v float64) string { return FormatNumber(v, decimals) }
}

// Percent formats a ratio (0.256 → "25.6%").
func Percent(decimals int) Formatter {
	return func(v float64) string { return FormatNumber(v*100, decimals) + "%" }
}

// Unit appends a unit suffix to a thousands-separated value.
func Unit(unit string, decimals int) Formatter {
	return func(v float64) string { return FormatNumber(v, decimals) + unit }
}

// FormatCompact abbreviates large values: 万 / 亿 for Chinese locales
// ("zh-CN"), K / M / B otherwise. Values below the first unit keep
// thousands separators.
func FormatCompact(v float64, locale string) string {
	a := math.Abs(v)
	type unit struct {
		div float64
		sym string
	}
	units := []unit{{1e9, "B"}, {1e6, "M"}, {1e3, "K"}}
	// East Asian locales group by 10^4 and 10^8 (CLDR compact short forms);
	// these are number-format data keyed by locale, not UI text.
	switch lc := strings.ToLower(locale); {
	case strings.HasPrefix(lc, "zh-tw"), strings.HasPrefix(lc, "zh-hk"), strings.HasPrefix(lc, "zh-mo"), strings.HasPrefix(lc, "zh-hant"):
		units = []unit{{1e8, "億"}, {1e4, "萬"}} // i18n:ok CLDR compact units
	case strings.HasPrefix(lc, "zh"):
		units = []unit{{1e8, "亿"}, {1e4, "万"}} // i18n:ok CLDR compact units
	case strings.HasPrefix(lc, "ja"):
		units = []unit{{1e8, "億"}, {1e4, "万"}} // i18n:ok CLDR compact units
	}
	for _, u := range units {
		if a >= u.div {
			return trimZeros(strconv.FormatFloat(v/u.div, 'f', 1, 64)) + u.sym
		}
	}
	return FormatNumber(v, decimalsOf(v))
}

// Compact returns FormatCompact bound to a locale code (c.Locale().Code).
func Compact(locale string) Formatter {
	return func(v float64) string { return FormatCompact(v, locale) }
}

func trimZeros(s string) string {
	if strings.IndexByte(s, '.') < 0 {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// decimalsOf returns the decimals needed to show v exactly (max 15).
func decimalsOf(v float64) int {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	for d := 0; d < 15; d++ {
		x := v * math.Pow(10, float64(d))
		if math.Abs(x) >= 0.5 && math.Abs(math.Round(x)-x) < 1e-6*math.Max(1, math.Abs(x)) {
			return d
		}
	}
	return 15
}

// stepDecimals returns the decimals that tick labels with step need.
func stepDecimals(step float64) int {
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return 0
	}
	return decimalsOf(step)
}
