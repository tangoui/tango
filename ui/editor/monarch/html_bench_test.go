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

// Unlike the warm workload, include grammar compilation and lazy JS/CSS
// compilation, then tokenize one representative mixed chunk exactly once.
func BenchmarkHTMLCompileAndFirstChunk(b *testing.B) {
	source := "<div class=\"card\">\n<script>if (a > 1) { run(`x${y}`) }</script>\n<style>.a { color: red; }</style>\n</div>\n"
	lines := strings.Split(source, "\n")
	b.SetBytes(int64(len(source)))
	b.ReportAllocs()
	_, _, timeouts := regex.Stats()
	_, _, degraded := Stats()
	b.ResetTimer()
	for range b.N {
		lx := mustLexer(b, "html")
		state := lx.Initial()
		var tokens []Token
		for _, line := range lines {
			tokens, state = lx.TokenizeLine(line, state, tokens[:0])
		}
	}
	b.StopTimer()
	_, _, endTimeouts := regex.Stats()
	_, _, endDegraded := Stats()
	b.ReportMetric(float64(endTimeouts-timeouts)/float64(b.N), "timeouts/op")
	b.ReportMetric(float64(endDegraded-degraded)/float64(b.N), "degraded/op")
}
