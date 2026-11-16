package i18n

import "testing"

func BenchmarkLocaleMatch(b *testing.B) {
	preferred := []string{"zh-Hant-HK", "en-US"}
	supported := []string{"en", "zh-CN", "zh-TW", "ja"}
	b.ReportAllocs()
	for b.Loop() {
		if got := Match(preferred, supported); got != "zh-TW" {
			b.Fatal(got)
		}
	}
}

func BenchmarkMessageFormat(b *testing.B) {
	args := []any{"name", "Tom", "count", 42}
	b.ReportAllocs()
	for b.Loop() {
		if got := format("Hello {name}, {count} files", args); got != "Hello Tom, 42 files" {
			b.Fatal(got)
		}
	}
}
