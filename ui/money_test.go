package ui

import "testing"

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		loc  *Locale
		amt  float64
		cur  string
		want string
	}{
		{nil, 1234.5, "", "¥1,234.50"},
		{nil, -3, "", "-¥3.00"},
		{ZhCN, 1234.5, "", "¥1,234.50"},
		{EnUS, 1234.5, "", "$1,234.50"},
		{ZhTW, 99, "", "NT$99.00"},
		{EnUS, 1234.5, "JPY", "¥1,235"},
		{EnUS, 1234.5, "EUR", "€1,234.50"},
		{EnUS, 10, "XYZ", "XYZ 10.00"},
		{EnUS, 10, "US$", "US$10.00"},
		{&Locale{Code: "de-DE"}, 1234.5, "EUR", "1.234,50 €"},
		{&Locale{Code: "fr-FR"}, -1234.5, "EUR", "-1 234,50 €"},
		{&Locale{Code: "nl-NL"}, 1234.5, "EUR", "€ 1.234,50"},
		{&Locale{Code: "en-US", Currency: "GBP"}, 5, "", "£5.00"},
		{EnUS, -0.001, "", "$0.00"}, // no "-$0.00"
		{&Locale{Code: "de-CH"}, 1234.5, "", "1.234,50\u00a0CHF"},
		{&Locale{Code: "ja-JP"}, 1234.5, "", "¥1,235"},
	}
	for _, c := range cases {
		code := "<nil>"
		if c.loc != nil {
			code = c.loc.Code
		}
		if got := FormatMoney(c.loc, c.amt, c.cur); got != c.want {
			t.Errorf("FormatMoney(%s, %v, %q) = %q, want %q", code, c.amt, c.cur, got, c.want)
		}
	}
}

func TestProFormatLocaleMoney(t *testing.T) {
	if got := ProFormat(ValueMoney, nil, 3); got != "¥3.00" {
		t.Errorf("ProFormat = %q", got)
	}
	if got := ProFormatLocale(EnUS, ValueMoney, nil, 3); got != "$3.00" {
		t.Errorf("ProFormatLocale(en-US) = %q", got)
	}
	if got := ProFormatLocale(Ja, ValueMoney, nil, 1500); got != "¥1,500" {
		t.Errorf("ProFormatLocale(ja) = %q", got) // JPY has no decimals
	}
}
