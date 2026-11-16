package monarch

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/regex"
)

// Matches MyGo v0.9.0's published Go/HTML benchmark inputs. Both keep their
// destination token capacity between documents; language setup is excluded.
func BenchmarkMyGoCompareWarm(b *testing.B) {
	goSource, _ := mygoCompareGoChunk()
	htmlSource := strings.Repeat("<div class=\"card\" id='x' data-a=\"{b}\">\n\t<a href=\"/p?q=1&amp;r\">Hi &lt;there&gt;</a> <!-- note -->\n"+
		"\t<script>if (a > 1) { run(`x${y}`) }</script>\n\t<style>.a { color: red; }</style>\n</div>\n", 200)
	for _, tc := range []struct{ language, source string }{{"go", goSource}, {"html", htmlSource}} {
		b.Run(tc.language, func(b *testing.B) {
			lx := mustLexer(b, tc.language)
			lines := strings.Split(tc.source, "\n")
			var tokens []Token
			b.SetBytes(int64(len(tc.source)))
			b.ReportAllocs()
			_, _, timeouts := regex.Stats()
			_, _, degraded := Stats()
			b.ResetTimer()
			for range b.N {
				state := lx.Initial()
				for _, line := range lines {
					tokens, state = lx.TokenizeLine(line, state, tokens[:0])
				}
			}
			b.StopTimer()
			_, _, endTimeouts := regex.Stats()
			_, _, endDegraded := Stats()
			b.ReportMetric(float64(endTimeouts-timeouts)/float64(b.N), "timeouts/op")
			b.ReportMetric(float64(endDegraded-degraded)/float64(b.N), "degraded/op")
		})
	}
}
