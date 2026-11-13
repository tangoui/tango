package text

import (
	"strconv"
	"strings"
	"testing"
)

// mono200 returns n distinct 200-column code lines.
func mono200(n int) []string {
	base := "\tif err := c.Shape(line, runs, opt); err != nil { return fmt.Errorf(\"shape %q: %w\", line, err) } // "
	out := make([]string, n)
	for i := range out {
		s := base + strconv.Itoa(i) + " "
		s += strings.Repeat("x", 200-len(s))
		out[i] = s
	}
	return out
}

// tokenRuns splits a line into token-like runs of 6 runes alternating
// regular and bold mono (as an editor passes one run per token).
func tokenRuns(s string) []Run {
	n := len([]rune(s))
	var runs []Run
	for a := 0; a < n; a += 6 {
		st := Style{Mono: true, Size: 13}
		if a/6%3 == 1 {
			st.Weight = Bold
		}
		runs = append(runs, Run{a, min(n, a+6), st})
	}
	return runs
}

func BenchmarkLineCacheMono200(b *testing.B) {
	UseSystemFonts(true)
	lines := mono200(1024)
	one := make([][]Run, len(lines))
	toks := make([][]Run, len(lines))
	for i, s := range lines {
		one[i] = []Run{{0, len([]rune(s)), Style{Mono: true, Size: 13}}}
		toks[i] = tokenRuns(s)
	}
	bench := func(name string, runs [][]Run, lo LineOptions, capacity, distinct int) {
		b.Run(name, func(b *testing.B) {
			c := NewLineCache(capacity)
			for i := range distinct {
				c.Shape(lines[i], runs[i], lo)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c.Shape(lines[i%distinct], runs[i%distinct], lo)
				i++
			}
		})
	}
	// hit: 64 visible lines, all cached.
	bench("hit", one, LineOptions{}, 128, 64)
	bench("hit-tokens", toks, LineOptions{}, 128, 64)
	// miss: cycling 1024 lines through a 64-entry cache misses every time.
	bench("miss-fast", one, LineOptions{}, 64, 1024)
	bench("miss-fast-tokens", toks, LineOptions{}, 64, 1024)
	// The same misses through the general path (ligatures force it).
	bench("miss-general", one, LineOptions{Ligatures: true}, 64, 1024)
	bench("miss-general-tokens", toks, LineOptions{Ligatures: true}, 64, 1024)
}

// BenchmarkLineCacheMixed200 measures misses on non-ASCII mono lines (CJK
// comments, emoji), which take the fast path's segmenting lane.
func BenchmarkLineCacheMixed200(b *testing.B) {
	UseSystemFonts(true)
	lines := make([]string, 1024)
	for i := range lines {
		s := "\tx := f(y) // 中文注释：测试 " + strconv.Itoa(i) + " 😀 émoji "
		for len([]rune(s)) < 200 {
			s += "数据 data "
		}
		lines[i] = string([]rune(s)[:200])
	}
	for _, lig := range []bool{false, true} {
		name := "miss-fast"
		if lig {
			name = "miss-general"
		}
		b.Run(name, func(b *testing.B) {
			c := NewLineCache(64)
			lo := LineOptions{Ligatures: lig}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				s := lines[i%len(lines)]
				c.Shape(s, []Run{{0, 200, Style{Mono: true, Size: 13}}}, lo)
				i++
			}
		})
	}
}
