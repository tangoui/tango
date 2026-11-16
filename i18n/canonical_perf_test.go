package i18n

import "testing"

func TestCanonicalNormalizedTagsAllocateNothing(t *testing.T) {
	for _, tag := range []string{"en", "zh-CN", "zh-Hant-HK", "sr-Latn-RS", "es-419", "de-DE-u-CO-phonebk"} {
		t.Run(tag, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				if Canonical(tag) != tag {
					panic("changed canonical tag")
				}
			})
			if allocs != 0 {
				t.Fatalf("canonical tag allocated %.0f times", allocs)
			}
		})
	}
}

func TestLocaleMatchNormalizedTagsAllocateNothing(t *testing.T) {
	prefs := []string{"zh-Hant-HK", "en-US"}
	available := []string{"en", "zh-CN", "zh-TW", "ja"}
	allocs := testing.AllocsPerRun(100, func() {
		if Match(prefs, available) != "zh-TW" {
			panic("wrong locale")
		}
	})
	if allocs != 0 {
		t.Fatalf("matching canonical tags allocated %.0f times", allocs)
	}
}

func BenchmarkCanonicalTags(b *testing.B) {
	for _, tag := range []string{"en", "zh-Hant-HK", "sr-Latn-RS", "zh_hant_hk.UTF-8"} {
		b.Run(tag, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if Canonical(tag) == "" {
					b.Fatal("lost tag")
				}
			}
		})
	}
}

func TestCanonicalAndMatchPreserveLegacyBehavior(t *testing.T) {
	parts := []string{"en", "EN", "zh", "hant", "Hant", "CN", "Tw", "", "C", "c", "POSIX", "Latn", "RS", "419", "utf8", "ab12", "İ", "ß", "汉", "\x00", "u", "co", "phonebk"}
	separators := []string{"-", "_", "--", "__"}
	suffixes := []string{"", ".UTF-8", "@euro", " \t", ".x@y"}
	var seed uint32 = 913
	for n := 0; n < 8000; n++ {
		seed = seed*1664525 + 1013904223
		s := parts[int(seed)%len(parts)]
		for i := 0; i < 3; i++ {
			seed = seed*1664525 + 1013904223
			s += separators[int(seed)%len(separators)] + parts[int(seed>>8)%len(parts)]
		}
		seed = seed*1664525 + 1013904223
		s += suffixes[int(seed)%len(suffixes)]
		if n%7 == 0 {
			s = "\u00a0 " + s + "\n"
		}
		if got, want := Canonical(s), referenceCanonical(s); got != want {
			t.Fatalf("Canonical(%q)=%q want %q", s, got, want)
		}
		// Include sets without s so exact matches cannot hide parsing,
		// script/region selection or the preference and default fallbacks.
		for _, prefs := range [][]string{{s}, {s, "zh-Hant-HK", "en-US"}} {
			for _, available := range [][]string{
				{"en", "zh-CN", "zh-TW", "ja", "pt-PT", "sr_Latn_RS", s},
				{"en", "zh-CN", "zh-TW", "ja", "pt-PT", "sr_Latn_RS"},
				{"zh-Hans", "zh-Hant", "zh-Hant-TW", "sr-Cyrl", "sr-Latn"},
				{"zh-HK", "zh-TW", "en-GB", "pt-BR", "sr-RS"},
				{"xx", "zz-ZZ"},
				nil,
			} {
				if got, want := Match(prefs, available), referenceMatch(prefs, available); got != want {
					t.Fatalf("Match(%q, %q)=%q want %q", prefs, available, got, want)
				}
				got, ok := MatchOK(prefs, available)
				want, wantOK := referenceMatchOK(prefs, available)
				if got != want || ok != wantOK {
					t.Fatalf("MatchOK(%q, %q)=%q,%v want %q,%v", prefs, available, got, ok, want, wantOK)
				}
			}
		}
	}
}
