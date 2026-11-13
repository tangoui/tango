package monarch

import (
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui/editor/regex"
)

// ts20k 把 Monaco typescript 用例里的所有行按用例顺序重复拼成 2 万行。
func ts20k(b testing.TB) []string {
	cases := loadFixture(b, filepath.Join(fixtureDir, "typescript.json"))
	var src []string
	for _, c := range cases {
		for _, l := range c {
			src = append(src, l.Line)
		}
	}
	lines := make([]string, 0, 20000)
	for len(lines) < 20000 {
		lines = append(lines, src[len(lines)%len(src)])
	}
	return lines
}

func BenchmarkTokenizeTS20k(b *testing.B) {
	lines := ts20k(b)
	s0, b0, _ := regex.Stats()
	lx := mustLexer(b, "typescript") // 新 Registry：统计这份语法编译出的两种引擎正则数
	s1, b1, _ := regex.Stats()
	_, _, t0 := regex.Stats()
	b.ResetTimer()
	var toks []Token
	for range b.N {
		st := lx.Initial()
		for _, l := range lines {
			toks, st = lx.TokenizeLine(l, st, toks[:0])
		}
	}
	b.StopTimer()
	_, _, t1 := regex.Stats()
	b.ReportMetric(float64(s1-s0), "std-regexps")
	b.ReportMetric(float64(b1-b0), "backtrack-regexps")
	b.ReportMetric(float64(t1-t0), "timeouts")
}
