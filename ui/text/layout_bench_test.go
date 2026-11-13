package text

import "testing"

// BenchmarkShapeLegacyPath times the uncached single-style layout (what
// Shape does on a cache miss), to keep its allocations in check.
func BenchmarkShapeLegacyPath(b *testing.B) {
	UseSystemFonts(true)
	cases := []struct {
		name string
		s    string
		st   Style
		opt  Options
	}{
		{"latin", "The quick brown fox jumps over the lazy dog, again and again.", Style{}, Options{NoWrap: true}},
		{"mixed-wrap", "中文 English 混排 123，「引号」 and some more words here", Style{}, Options{MaxWidth: 120}},
		{"mono80", "\tfunc (c *LineCache) Shape(line string, runs []Run, opt LineOptions) *Layout {", Style{Mono: true}, Options{NoWrap: true}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			st := c.st.norm()
			chain := reg.chain(st.Mono)
			lang := resolveLang(st)
			mu.Lock()
			defer mu.Unlock()
			b.ReportAllocs()
			for b.Loop() {
				layout(c.s, st, c.opt, chain, lang)
			}
		})
	}
}
