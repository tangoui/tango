package ui

import "strings"

// p2eCurrency returns the currency of money values, an ISO 4217 code or a
// symbol (see FormatMoney): override when set, else Locale.Currency, else
// (a locale without one) the currency of the locale's region ("de-CH" CHF,
// "fr-FR" EUR, "zh-HK" HKD, "ja-JP" JPY), else of its language (zh CNY,
// zh-Hant TWD, ja JPY, ko KRW, de / fr / it / nl / fi / el EUR), else "$".
// A nil locale is zh-CN.
func p2eCurrency(loc *Locale, override string) string {
	if override != "" {
		return override
	}
	if loc != nil && loc.Currency != "" {
		return loc.Currency
	}
	if loc == nil || loc.Code == "" {
		return "CNY"
	}
	parts := strings.FieldsFunc(strings.ToLower(loc.Code), func(r rune) bool { return r == '-' || r == '_' })
	for _, p := range parts[1:] {
		if len(p) == 2 {
			if cur, ok := regionCurrency[p]; ok {
				return cur
			}
			if euroRegions[p] {
				return "EUR"
			}
		}
	}
	switch parts[0] {
	case "zh":
		if strings.Contains(strings.ToLower(loc.Code), "hant") {
			return "TWD"
		}
		return "CNY"
	case "ja":
		return "JPY"
	case "ko":
		return "KRW"
	case "de", "fr", "it", "nl", "fi", "el":
		return "EUR"
	}
	return "$"
}

var regionCurrency = map[string]string{
	"us": "USD", "cn": "CNY", "tw": "TWD", "hk": "HKD", "mo": "MOP", "jp": "JPY",
	"kr": "KRW", "gb": "GBP", "ch": "CHF", "ca": "CAD", "au": "AUD", "nz": "NZD",
	"sg": "SGD", "in": "INR", "br": "BRL", "mx": "MXN", "ru": "RUB", "pl": "PLN",
	"se": "SEK", "no": "NOK", "dk": "DKK", "tr": "TRY", "ua": "UAH", "id": "IDR",
	"th": "THB", "vn": "VND", "il": "ILS", "sa": "SAR", "ae": "AED",
}

var euroRegions = map[string]bool{
	"de": true, "fr": true, "es": true, "it": true, "nl": true, "pt": true, "at": true,
	"be": true, "fi": true, "ie": true, "gr": true, "sk": true, "si": true, "ee": true,
	"lv": true, "lt": true, "hr": true, "lu": true, "mt": true, "cy": true,
}
