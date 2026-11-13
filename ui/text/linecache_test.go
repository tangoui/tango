package text

import (
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/go-text/typesetting/shaping"
)

// lineRef is what LineCache.Shape must return: ShapeRuns with the options
// LineCache derives from lo (tab stops, ligatures, language, direction).
func lineRef(line string, runs []Run, lo LineOptions) *Layout {
	rs, opt := lineShapeArgs(runs, lo)
	return ShapeRuns(line, rs, opt)
}

// layoutDiff names the first Layout field where a and b differ.
func layoutDiff(a, b *Layout) string {
	switch {
	case !reflect.DeepEqual(a.Glyphs, b.Glyphs):
		return fmt.Sprintf("Glyphs\n got %+v\nwant %+v", a.Glyphs, b.Glyphs)
	case !reflect.DeepEqual(a.GlyphCluster, b.GlyphCluster):
		return fmt.Sprintf("GlyphCluster\n got %v\nwant %v", a.GlyphCluster, b.GlyphCluster)
	case !reflect.DeepEqual(a.GlyphRun, b.GlyphRun):
		return fmt.Sprintf("GlyphRun\n got %v\nwant %v", a.GlyphRun, b.GlyphRun)
	case !reflect.DeepEqual(a.Lines, b.Lines):
		return fmt.Sprintf("Lines\n got %+v\nwant %+v", a.Lines, b.Lines)
	}
	return fmt.Sprintf("Layout\n got %+v\nwant %+v", *a, *b)
}

func carets(l *Layout) []float32 {
	out := make([]float32, len(l.Runes)+1)
	for i := range out {
		out[i], _, _ = l.Caret(i)
	}
	return out
}

// countShaping counts runs handed to the shaper while f runs.
func countShaping(f func()) int {
	n := 0
	shapeHook = func(shaping.Input) { n++ }
	defer func() { shapeHook = nil }()
	f()
	return n
}

func monoRun(s string, st Style) []Run {
	st.Mono = true
	return []Run{{0, len([]rune(s)), st}}
}

func TestLineCacheHit(t *testing.T) {
	needFont(t, "abc")
	c := NewLineCache(2)
	line := "func main() { return 1 }"
	runs := monoRun(line, Style{Size: 13})
	lo := LineOptions{Ligatures: true} // general path: shaping is visible to shapeHook
	var l1, l2 *Layout
	if n := countShaping(func() { l1 = c.Shape(line, runs, lo) }); n == 0 {
		t.Fatalf("first Shape did not shape")
	}
	if n := countShaping(func() { l2 = c.Shape(line, runs, lo) }); n != 0 {
		t.Errorf("second Shape reshaped (%d runs)", n)
	}
	if l1 != l2 {
		t.Errorf("hit returned a different layout")
	}
	if c.stats.hits != 1 || c.stats.misses != 1 {
		t.Errorf("stats %+v, want 1 hit 1 miss", c.stats)
	}
	// A hit allocates nothing.
	if a := testing.AllocsPerRun(100, func() { c.Shape(line, runs, lo) }); a != 0 {
		t.Errorf("hit allocates %v", a)
	}
	// Another style is another key.
	bold := monoRun(line, Style{Size: 13, Weight: Bold})
	if c.Shape(line, bold, lo) == l1 {
		t.Errorf("bold run hit the regular entry")
	}
	// LRU: capacity 2 holds {line bold, line regular}; touching regular
	// then adding a third evicts bold.
	c.Shape(line, runs, lo)
	c.Shape("other", monoRun("other", Style{Size: 13}), lo)
	m := c.stats.misses
	c.Shape(line, runs, lo)
	if c.stats.misses != m {
		t.Errorf("recently used entry was evicted")
	}
	c.Shape(line, bold, lo)
	if c.stats.misses != m+1 {
		t.Errorf("least recently used entry was not evicted")
	}
	if c.Len() != 2 {
		t.Errorf("Len %d, want 2", c.Len())
	}
	// Font registry changes flush the cache.
	UseSystemFonts(true)
	reg.mu.Lock()
	reg.version++
	reg.mu.Unlock()
	if c.Shape(line, runs, lo) == l1 {
		t.Errorf("layout survived a font change")
	}
}

// fastCases are mono lines the fast path covers: ASCII, tabs, CJK, zero
// width, combining marks, emoji, and several mono styles in one line.
func fastCases() []struct {
	name string
	line string
	runs []Run
} {
	type tc = struct {
		name string
		line string
		runs []Run
	}
	one := func(name, s string) tc { return tc{name, s, monoRun(s, Style{Size: 14})} }
	toks := "if err != nil { return err }"
	return []tc{
		one("ascii", "func (c *LineCache) Shape(line string) *Layout { return nil }"),
		one("tab", "\tif x {\t// a\tbc\td"),
		one("leading-tabs", "\t\t\tx"),
		one("cjk", "中文注释：测试 abc，「引号」 x"),
		one("zero-width", "a\u200bb\u200cc\u200dd\ufeffe"),
		one("combining", "éẍ café ṇ̃"),
		one("emoji", "ok 😀 x 👍🏽 🇨🇳 ❤️ 👨\u200d👩\u200d👧 #️⃣"),
		one("spaces-only", "    "),
		one("cyrillic-greek", "Привет αβγ"),
		{"bold-italic-runs", toks, []Run{
			{0, 2, Style{Mono: true, Size: 14, Weight: Bold}},
			{2, 3, Style{Mono: true, Size: 14}},
			{3, 6, Style{Mono: true, Size: 14, Italic: true}},
			{6, 16, Style{Mono: true, Size: 14}},
			{16, 22, Style{Mono: true, Size: 14, Weight: Bold}},
			{22, 28, Style{Mono: true, Size: 14}},
		}},
		{"bold-cjk-emoji", "名前 😀 x\ty", []Run{
			{0, 2, Style{Mono: true, Size: 14, Weight: Bold}},
			{2, 5, Style{Mono: true, Size: 14}},
			{5, 8, Style{Mono: true, Size: 14, Weight: Bold, Italic: true}},
		}},
		{"sizes", "ab\tcd", []Run{{0, 2, Style{Mono: true, Size: 12}}, {2, 5, Style{Mono: true, Size: 20}}}},
	}
}

func TestMonoFastPathMatchesShapeRuns(t *testing.T) {
	needFont(t, "abc")
	for _, tc := range fastCases() {
		for _, lo := range []LineOptions{{}, {TabSize: 2, Lang: "ja"}, {TabSize: 8, Direction: DirLTR}} {
			t.Run(fmt.Sprintf("%s/%d%s", tc.name, lo.TabSize, lo.Lang), func(t *testing.T) {
				c := NewLineCache(8)
				got := c.Shape(tc.line, tc.runs, lo)
				want := lineRef(tc.line, tc.runs, lo)
				if c.stats.fast != 1 {
					if runtime.GOOS == "darwin" {
						t.Errorf("fast path not taken")
					} else {
						t.Logf("fast path not taken with this machine's fonts")
					}
				}
				if gc, wc := carets(got), carets(want); !reflect.DeepEqual(gc, wc) {
					t.Fatalf("carets\n got %v\nwant %v", gc, wc)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("layout differs from ShapeRuns: %s", layoutDiff(got, want))
				}
			})
		}
	}
}

// Tabs advance to the next multiple of TabSize columns (the advance of a
// space in the first run's mono face).
func TestLineCacheTabStops(t *testing.T) {
	check := func(t *testing.T) {
		c := NewLineCache(8)
		st := Style{Mono: true, Size: 14}
		col := Shape(" x", st, Options{NoWrap: true})
		cw, _, _ := col.Caret(1)
		for _, ts := range []int{0, 2, 4, 8} {
			n := ts
			if n <= 0 {
				n = 4
			}
			line := "\tab\tc\t\td"
			l := c.Shape(line, monoRun(line, st), LineOptions{TabSize: ts})
			col := 0 // expected caret column
			for i, r := range []rune(line) {
				if x, _, _ := l.Caret(i); !near(x, float32(col)*cw, 0.01) {
					t.Errorf("TabSize %d: Caret(%d) = %v, want column %d (%v)", ts, i, x, col, float32(col)*cw)
				}
				if r == '\t' {
					col = (col/n + 1) * n
				} else {
					col++
				}
			}
		}
		// A tab already at a stop's left edge still advances a full stop
		// (no zero-width tabs).
		l := c.Shape("abcd\tx", monoRun("abcd\tx", st), LineOptions{TabSize: 4})
		if x, _, _ := l.Caret(5); !near(x, 8*cw, 0.01) {
			t.Errorf("tab at a stop: Caret(5) = %v, want %v", x, 8*cw)
		}
	}
	t.Run("fonts", func(t *testing.T) { needFont(t, "abc"); check(t) })
	t.Run("nofonts", func(t *testing.T) { withoutFonts(t); check(t) })
}

func TestLigaturesDisableFastPath(t *testing.T) {
	needFont(t, "abc")
	c := NewLineCache(8)
	line := "a -> b != c == d"
	runs := monoRun(line, Style{})
	got := c.Shape(line, runs, LineOptions{Ligatures: true})
	if c.stats.fast != 0 || c.stats.slow != 1 {
		t.Errorf("stats %+v: ligatures must use the general path", c.stats)
	}
	if want := lineRef(line, runs, LineOptions{Ligatures: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("ligature layout differs from ShapeRuns")
	}
	c.Shape(line, runs, LineOptions{})
	if c.stats.fast != 1 {
		t.Errorf("stats %+v: no ligatures should take the fast path", c.stats)
	}
	// Other cases the fast path leaves to ShapeRuns.
	for _, tc := range []struct {
		name string
		line string
		runs []Run
		lo   LineOptions
	}{
		{"sans run", "abc", []Run{{0, 3, Style{}}}, LineOptions{}},
		{"mixed mono and sans", "abc", []Run{{0, 1, Style{Mono: true}}, {1, 3, Style{}}}, LineOptions{}},
		{"rtl paragraph", "abc", monoRun("abc", Style{}), LineOptions{Direction: DirRTL}},
		{"hebrew", "abc שלום", monoRun("abc שלום", Style{}), LineOptions{}},
		{"arabic", "x = \"مرحبا\"", monoRun("x = \"مرحبا\"", Style{}), LineOptions{}},
		{"newline", "a\nb", monoRun("a\nb", Style{}), LineOptions{}},
	} {
		c := NewLineCache(8)
		got := c.Shape(tc.line, tc.runs, tc.lo)
		if c.stats.fast != 0 {
			t.Errorf("%s: took the fast path", tc.name)
		}
		if want := lineRef(tc.line, tc.runs, tc.lo); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: layout differs from ShapeRuns", tc.name)
		}
	}
}

func TestCacheKeyIncludesLangDirection(t *testing.T) {
	needFont(t, "abc")
	c := NewLineCache(16)
	line := "直 abc"
	runs := monoRun(line, Style{})
	ja := c.Shape(line, runs, LineOptions{Lang: "ja"})
	zh := c.Shape(line, runs, LineOptions{Lang: "zh-Hans"})
	ltr := c.Shape(line, runs, LineOptions{Lang: "ja", Direction: DirLTR})
	rtl := c.Shape(line, runs, LineOptions{Lang: "ja", Direction: DirRTL})
	tab := c.Shape(line, runs, LineOptions{Lang: "ja", TabSize: 8})
	lig := c.Shape(line, runs, LineOptions{Lang: "ja", Ligatures: true})
	all := []*Layout{ja, zh, ltr, rtl, tab, lig}
	for i := range all {
		for j := range i {
			if all[i] == all[j] {
				t.Errorf("layouts %d and %d share an entry", i, j)
			}
		}
	}
	if c.stats.misses != len(all) {
		t.Errorf("misses %d, want %d", c.stats.misses, len(all))
	}
	if !rtl.Lines[0].RTL || ja.Lines[0].RTL {
		t.Errorf("direction not applied: rtl %v ja %v", rtl.Lines[0].RTL, ja.Lines[0].RTL)
	}
	// The language reaches the shaper: Japanese and Chinese forms of 直
	// differ in fonts with locl (PingFang, Noto CJK).
	if ja.Glyphs[0].ID == zh.Glyphs[0].ID {
		t.Logf("直 has the same glyph for ja and zh-Hans in this font")
	}
	if want := lineRef(line, runs, LineOptions{Lang: "ja"}); !reflect.DeepEqual(ja, want) {
		t.Errorf("ja layout differs from ShapeRuns")
	}
	// A run's own Lang wins over LineOptions.Lang.
	own := []Run{{0, 4, Style{Mono: true, Lang: "zh-Hans"}}}
	if l := c.Shape(line, own, LineOptions{Lang: "ja"}); !reflect.DeepEqual(l.Glyphs, zh.Glyphs) {
		t.Errorf("run Lang did not win over LineOptions.Lang")
	}
	// DefaultLang changes flush entries that relied on it.
	def := c.Shape(line, runs, LineOptions{})
	old := DefaultLang()
	SetDefaultLang("ja")
	defer SetDefaultLang(old)
	if c.Shape(line, runs, LineOptions{}) == def {
		t.Errorf("layout survived a DefaultLang change")
	}
}

// TestFastPathRandomMatchesShapeRuns shapes random mono lines from a mixed
// alphabet with random style runs; whichever path LineCache takes, the
// layout must equal ShapeRuns'.
func TestFastPathRandomMatchesShapeRuns(t *testing.T) {
	needFont(t, "abc")
	alpha := []string{"a", "Z", "x", "0", " ", " ", "\t", "(", ")", "[", "]", "{", "}", "\"", "'", ".", ",", ";", "=", "-", ">",
		"中", "文", "，", "。", "「", "」", "\u3000", "あ", "カ", "é", "́", "̈", "😀", "👍🏽", "❤️", "\u200b", "\u200d",
		"α", "Ж", "ß", "ﬁ", "→", "…", "│", "─", "\u00a0", "ı", "ǅ"}
	styles := []Style{{Mono: true}, {Mono: true, Weight: Bold}, {Mono: true, Italic: true}, {Mono: true, Size: 18}, {Mono: true, Lang: "ja"}}
	rnd := rand.New(rand.NewSource(1))
	c := NewLineCache(64)
	for k := range 400 {
		var b strings.Builder
		for range rnd.Intn(40) {
			b.WriteString(alpha[rnd.Intn(len(alpha))])
		}
		line := b.String()
		n := len([]rune(line))
		var runs []Run
		for a := 0; a < n || len(runs) == 0; {
			e := min(n, a+1+rnd.Intn(8))
			runs = append(runs, Run{a, e, styles[rnd.Intn(len(styles))]})
			a = e
		}
		lo := LineOptions{TabSize: rnd.Intn(9)}
		got := c.Shape(line, runs, lo)
		want := lineRef(line, runs, lo)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d %q %U runs %v: differs from ShapeRuns: %s", k, line, []rune(line), runs, layoutDiff(got, want))
		}
	}
	t.Logf("stats %+v", c.stats)
	if runtime.GOOS == "darwin" && c.stats.fast < c.stats.misses/2 {
		t.Errorf("fast path taken for only %d of %d lines", c.stats.fast, c.stats.misses)
	}
}

// Adjacent runs whose styles shape alike are shaped as one piece (kerning
// crosses token boundaries) while GlyphRun keeps every run.
func TestShapeRunsMergesEquivalentRuns(t *testing.T) {
	s := "AVATAR To WAVY AYA"
	needFont(t, s)
	st := Style{Size: 24}
	one := ShapeRuns(s, []Run{{0, 18, st}}, Options{NoWrap: true})
	cuts := []int{0, 1, 2, 3, 9, 18}
	var five []Run
	for k := range 5 {
		five = append(five, Run{cuts[k], cuts[k+1], st})
	}
	l := ShapeRuns(s, five, Options{NoWrap: true})
	if !reflect.DeepEqual(l.Glyphs, one.Glyphs) || !reflect.DeepEqual(carets(l), carets(one)) {
		t.Errorf("5 same-style runs shape differently from one run\n got %v\nwant %v", carets(l), carets(one))
	}
	seen := map[int32]bool{}
	for g, r := range l.GlyphRun {
		c := int(l.GlyphCluster[g])
		var want int32
		for k := range five {
			if c >= five[k].Start && c < five[k].End {
				want = int32(k)
			}
		}
		if r != want {
			t.Errorf("glyph %d (rune %d) run %d, want %d", g, c, r, want)
		}
		seen[r] = true
	}
	if len(seen) != 5 {
		t.Errorf("GlyphRun reports %d runs, want 5", len(seen))
	}
	// Lang empty and Lang equal to the default are the same shaping.
	lang := []Run{{0, 9, st}, {9, 18, Style{Size: 24, Lang: DefaultLang()}}}
	if l := ShapeRuns(s, lang, Options{NoWrap: true}); !reflect.DeepEqual(l.Glyphs, one.Glyphs) {
		t.Errorf("resolved-equal languages not merged")
	}
	// A style difference still cuts.
	diff := []Run{{0, 1, st}, {1, 18, Style{Size: 24, Weight: Bold}}}
	if l := ShapeRuns(s, diff, Options{NoWrap: true}); reflect.DeepEqual(carets(l), carets(one)) {
		t.Logf("bold cut changed nothing measurable")
	}
}
