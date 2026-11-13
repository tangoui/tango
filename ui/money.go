package ui

import (
	"math"
	"strconv"
	"strings"
)

// Money formatting shared by ProFormat, ProTable, ProDescriptions and the
// other Pro components.
//
// The currency is either an ISO 4217 code ("USD", "JPY", "EUR") or a plain
// symbol ("$", "US$"). A code picks the symbol and the number of decimals
// (JPY / KRW / VND: none); a symbol always shows two decimals. The locale
// decides the separators and where the symbol goes: European languages put
// it after the number ("1.234,56 €"), the others before it ("¥1,234.56").

// currencyInfo describes an ISO 4217 currency.
type currencyInfo struct {
	sym      string
	decimals int
}

var currencies = map[string]currencyInfo{
	"CNY": {"¥", 2}, "JPY": {"¥", 0}, "USD": {"$", 2}, "EUR": {"€", 2},
	"GBP": {"£", 2}, "HKD": {"HK$", 2}, "TWD": {"NT$", 2}, "MOP": {"MOP$", 2},
	"KRW": {"₩", 0}, "VND": {"₫", 0}, "INR": {"₹", 2}, "RUB": {"₽", 2},
	"CAD": {"CA$", 2}, "AUD": {"A$", 2}, "NZD": {"NZ$", 2}, "SGD": {"S$", 2},
	"CHF": {"CHF", 2}, "SEK": {"kr", 2}, "NOK": {"kr", 2}, "DKK": {"kr", 2},
	"PLN": {"zł", 2}, "BRL": {"R$", 2}, "MXN": {"MX$", 2}, "THB": {"฿", 2},
	"ILS": {"₪", 2}, "TRY": {"₺", 2}, "UAH": {"₴", 2}, "IDR": {"Rp", 2},
	"SAR": {"SAR", 2}, "AED": {"AED", 2},
}

// isCurrencyCode reports three ASCII upper-case letters.
func isCurrencyCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

// moneyStyle is how a locale writes amounts.
type moneyStyle struct {
	group, decimal string
	suffix         bool // symbol after the number, separated by a space
}

// localeMoneyStyle returns the money style of a locale (nil: Chinese).
func localeMoneyStyle(loc *Locale) moneyStyle {
	lang := ""
	if loc != nil {
		lang = strings.ToLower(loc.Code)
		if i := strings.IndexAny(lang, "-_"); i >= 0 {
			lang = lang[:i]
		}
	}
	switch lang {
	case "de", "es", "it", "pt", "id", "tr", "da", "el", "ro", "hr", "sl", "vi":
		return moneyStyle{group: ".", decimal: ",", suffix: true}
	case "nl":
		return moneyStyle{group: ".", decimal: ","}
	case "fr", "ru", "pl", "cs", "sk", "sv", "nb", "no", "fi", "uk", "hu", "bg":
		return moneyStyle{group: " ", decimal: ",", suffix: true}
	}
	return moneyStyle{group: ",", decimal: "."}
}

// FormatMoney formats amount in a currency for loc. currency is an ISO 4217
// code or a symbol; "" means loc.Currency (itself a code or a symbol), else
// the default of the locale's region (see ProColumn.Currency). A nil loc
// formats like zh-CN.
func FormatMoney(loc *Locale, amount float64, currency string) string {
	sym, decimals := p2eCurrency(loc, currency), 2
	if isCurrencyCode(sym) {
		if ci, ok := currencies[sym]; ok {
			sym, decimals = ci.sym, ci.decimals
		}
	}
	st := localeMoneyStyle(loc)
	num := formatGrouped(math.Abs(amount), decimals, st.group, st.decimal)
	neg := amount < 0 && strings.Trim(num, "0., ") != ""
	var out string
	switch {
	case st.suffix:
		out = num + " " + sym
	case isCurrencyCode(sym) || st.group == ".":
		out = sym + " " + num // "USD 1,234.56", "€ 1.234,56" (nl)
	default:
		out = sym + num
	}
	if neg {
		return "-" + out
	}
	return out
}

// formatGrouped formats a non-negative v with decimals digits and the given
// separators.
// Halves round away from zero, as money usually does (FormatFloat would
// round 0.5 to even).
func formatGrouped(v float64, decimals int, group, decimal string) string {
	decimals = max(0, decimals)
	scale := math.Pow10(decimals)
	if r := math.Round(v * scale); !math.IsInf(r, 0) {
		v = r / scale
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(ch)
	}
	if frac != "" {
		b.WriteString(decimal)
		b.WriteString(frac)
	}
	return b.String()
}
