package monarch

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/regex"
)

func BenchmarkHTMLSegments(b *testing.B) {
	for _, tc := range []struct{ name, chunk string }{
		{"text", "hello &amp; world\n"},
		{"tags", "<div class=\"card\" id='x' data-a=\"{b}\"><a href=\"/p?q=1&amp;r\">Hi &lt;there&gt;</a><!-- note --></div>\n"},
		{"script", "<script>if (a > 1) { run(`x${y}`) }</script>\n"},
		{"style", "<style>.a { color: red; }</style>\n"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			lx := mustLexer(b, "html")
			source := strings.Repeat(tc.chunk, 200)
			lines := strings.Split(source, "\n")
			var tokens []Token
			b.SetBytes(int64(len(source)))
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
