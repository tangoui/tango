package text

import (
	"math"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/shaping"
)

// visualRunes returns the logical indices of line n in visual order, from
// the per-rune boxes (works without fonts: the approximate metrics give
// every rune a width).
func visualRunes(l *Layout, n int) []int {
	ln := l.Lines[n]
	end := ln.Start + len(ln.carets) - 1
	idx := make([]int, 0, end-ln.Start)
	for i := ln.Start; i < end; i++ {
		idx = append(idx, i)
	}
	if ln.bidi == nil {
		return idx
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		xa, xb := ln.bidi.xl[a-ln.Start], ln.bidi.xl[b-ln.Start]
		switch {
		case xa < xb:
			return -1
		case xa > xb:
			return 1
		}
		return 0
	})
	return idx
}

func TestVisualOrderNoFonts(t *testing.T) {
	withoutFonts(t)
	cases := []struct {
		s    string
		dir  Direction
		want []int
		rtl  bool
	}{
		{"שלום", DirAuto, []int{3, 2, 1, 0}, true},
		{"مرحبا", DirAuto, []int{4, 3, 2, 1, 0}, true},
		// LTR paragraph with an RTL word.
		{"abc שלום def", DirAuto, []int{0, 1, 2, 3, 7, 6, 5, 4, 8, 9, 10, 11}, false},
		// RTL paragraph with digits and Latin: both at level 2.
		{"مرحبا 123 abc", DirAuto, []int{10, 11, 12, 9, 6, 7, 8, 5, 4, 3, 2, 1, 0}, true},
		// Forced directions.
		{"abc", DirRTL, []int{0, 1, 2}, true},
		{"abc def", DirRTL, []int{0, 1, 2, 3, 4, 5, 6}, true},
		{"שלום abc", DirLTR, []int{3, 2, 1, 0, 4, 5, 6, 7}, false},
		// Hebrew paragraph with numbers inside an LTR one: "שלום 123" is
		// one level-1 stretch with the number at level 2.
		{"abc שלום 123 def", DirAuto, []int{0, 1, 2, 3, 9, 10, 11, 8, 7, 6, 5, 4, 12, 13, 14, 15}, false},
	}
	for _, c := range cases {
		l := Shape(c.s, Style{Size: 14}, Options{Direction: c.dir})
		if got := visualRunes(l, 0); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q dir %d: visual %v, want %v", c.s, c.dir, got, c.want)
		}
		if l.Lines[0].RTL != c.rtl {
			t.Errorf("%q: Line.RTL = %v, want %v", c.s, l.Lines[0].RTL, c.rtl)
		}
	}
}

// TestBidiExactLevels guards runLevels: the levels must be per rune, not
// per direction run (see runLevels).
func TestBidiExactLevels(t *testing.T) {
	r := []rune("abc שלום 123 def")
	lv := make([]bidi.Level, len(r))
	mu.Lock()
	base, mixed := paraLevels(r, 0, len(r), DirAuto, lv)
	mu.Unlock()
	want := []bidi.Level{0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2, 0, 0, 0, 0}
	if base != 0 || !mixed || !reflect.DeepEqual(lv, want) {
		t.Fatalf("levels %v base %d, want %v", lv, base, want)
	}
	r = []rune("مرحبا 123 abc")
	lv = make([]bidi.Level, len(r))
	mu.Lock()
	base, _ = paraLevels(r, 0, len(r), DirAuto, lv)
	mu.Unlock()
	want = []bidi.Level{1, 1, 1, 1, 1, 1, 2, 2, 2, 1, 2, 2, 2}
	if base != 1 || !reflect.DeepEqual(lv, want) {
		t.Fatalf("levels %v base %d, want %v", lv, base, want)
	}
}

func TestPureLTRHasNoBidiState(t *testing.T) {
	withoutFonts(t)
	for _, s := range []string{"hello world", "中文 English 123", "a\tb", ""} {
		l := Shape(s, Style{}, Options{})
		for _, ln := range l.Lines {
			if ln.bidi != nil || ln.RTL {
				t.Fatalf("%q: LTR line got bidi state", s)
			}
		}
	}
}

func TestCaretSemanticsNoFonts(t *testing.T) {
	withoutFonts(t)
	// 0.55*14 = 7.7px per rune.
	const w = 7.7
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

	// Pure RTL: position 0 is at the right edge, the end at the left.
	l := Shape("שלום", Style{Size: 14}, Options{})
	for i := 0; i <= 4; i++ {
		x, _, _ := l.Caret(i)
		if want := float32(4-i) * w; !near(x, want) {
			t.Errorf("RTL Caret(%d) = %v, want %v", i, x, want)
		}
	}

	// "abc שלום def": positions follow the previous rune's direction.
	l = Shape("abc שלום def", Style{Size: 14}, Options{})
	want := map[int]float32{
		0: 0, 3: 3 * w, // after "abc"
		4:  4 * w, // after the space (LTR): left edge of the Hebrew run
		5:  7 * w, // after ש (RTL): its left edge; ש is visually rightmost
		8:  4 * w, // after ם: the left end of the Hebrew run
		9:  9 * w, // after the space following the Hebrew word
		12: 12 * w,
	}
	for i, wx := range want {
		if x, _, _ := l.Caret(i); !near(x, wx) {
			t.Errorf("mixed Caret(%d) = %v, want %v", i, x, wx)
		}
	}
	// IndexAt: the nearest caret. Inside ש ([7w,8w]) the nearest carets
	// are 7w (after ש, 5) and 9w (after the space, 9).
	for _, c := range []struct {
		x    float32
		want int
	}{
		{7.1 * w, 5}, {7.9 * w, 5}, {8.6 * w, 9},
		// 4w is shared by 4 (after the space) and 8 (after ם): the rune
		// under the point decides.
		{3.8 * w, 4}, {4.2 * w, 8},
		{1.4 * w, 1}, {1.6 * w, 2},
	} {
		if got := l.IndexAt(c.x, 0); got != c.want {
			t.Errorf("IndexAt(%.1fw) = %d, want %d", c.x/w, got, c.want)
		}
	}
	if got := l.IndexAt(-5, 0); got != 0 {
		t.Errorf("IndexAt left of line = %d", got)
	}
	if got := l.IndexAt(1000, 0); got != 12 {
		t.Errorf("IndexAt right of line = %d", got)
	}
}

// caretRoundTrip checks that every logical position's caret maps back to a
// position drawn at the same x.
func caretRoundTrip(t *testing.T, l *Layout) {
	t.Helper()
	for i := 0; i <= len(l.Runes); i++ {
		x, top, _ := l.Caret(i)
		j := l.IndexAt(x, top+l.LineHeight/2)
		x2, top2, _ := l.Caret(j)
		if math.Abs(float64(x-x2)) > 0.01 || top != top2 {
			t.Errorf("%q: Caret(%d)=(%v,%v) -> IndexAt %d -> (%v,%v)", l.Text, i, x, top, j, x2, top2)
		}
	}
}

func TestCaretRoundTripNoFonts(t *testing.T) {
	withoutFonts(t)
	for _, s := range []string{"שלום", "abc שלום def", "مرحبا 123 abc", "abc שלום 123 def", "שלום עולם abc def\nhello"} {
		caretRoundTrip(t, Shape(s, Style{Size: 14}, Options{}))
		caretRoundTrip(t, Shape(s, Style{Size: 14}, Options{MaxWidth: 50}))
	}
	// Away from direction boundaries the round trip is exact.
	l := Shape("abc שלום def", Style{Size: 14}, Options{})
	for _, i := range []int{1, 2, 6, 7, 10, 11} {
		x, _, _ := l.Caret(i)
		if j := l.IndexAt(x, 1); j != i {
			t.Errorf("IndexAt(Caret(%d)) = %d", i, j)
		}
	}
}

func TestSelectionRectsNoFonts(t *testing.T) {
	withoutFonts(t)
	const w = 7.7
	l := Shape("abc שלום def", Style{Size: 14}, Options{})
	// "c" + " " + "של": the space and c are on the left, של at the right
	// end of the Hebrew word.
	rs := l.SelectionRects(2, 6)
	if len(rs) != 2 {
		t.Fatalf("rects %v, want 2", rs)
	}
	if math.Abs(float64(rs[0].X-2*w)) > 0.01 || math.Abs(float64(rs[0].W-2*w)) > 0.01 {
		t.Errorf("first rect %v", rs[0])
	}
	if math.Abs(float64(rs[1].X-6*w)) > 0.01 || math.Abs(float64(rs[1].W-2*w)) > 0.01 {
		t.Errorf("second rect %v", rs[1])
	}
	if rs[0].H != l.LineHeight {
		t.Errorf("rect height %v", rs[0].H)
	}
	// Whole word: one rect; reversed bounds are accepted.
	if rs := l.SelectionRects(8, 4); len(rs) != 1 || math.Abs(float64(rs[0].W-4*w)) > 0.01 {
		t.Errorf("Hebrew word rects %v", rs)
	}
	// LTR line: a single rect from caret to caret, as before.
	l = Shape("hello", Style{Size: 14}, Options{})
	if rs := l.SelectionRects(1, 3); len(rs) != 1 || math.Abs(float64(rs[0].X-w)) > 0.01 || math.Abs(float64(rs[0].W-2*w)) > 0.01 {
		t.Errorf("LTR rects %v", rs)
	}
	// Across lines: one rect per line.
	l = Shape("abc שלום\nשלום abc", Style{Size: 14}, Options{})
	if rs := l.SelectionRects(1, 15); len(rs) < 3 {
		t.Errorf("multi-line rects %v", rs)
	}
}

func TestWrapThenReorderNoFonts(t *testing.T) {
	withoutFonts(t)
	// RTL paragraph: words break in logical order, each line reads right
	// to left; 60px fits 7 runes (7.7px each).
	s := "אבג דהו abc זחט"
	l := Shape(s, Style{Size: 14}, Options{MaxWidth: 60})
	got := lines(l)
	if strings.Join(got, "") != s || len(got) < 2 {
		t.Fatalf("lines %q", got)
	}
	for n, ln := range l.Lines {
		vis := visualRunes(l, n)
		if !ln.RTL {
			t.Errorf("line %d not RTL", n)
		}
		// Visual left-to-right, the Hebrew runes must have decreasing
		// logical indices and the Latin ones increasing.
		var heb, lat []int
		for _, i := range vis {
			switch r := l.Runes[i]; {
			case r >= 0x5d0 && r <= 0x5ea:
				heb = append(heb, i)
			case r >= 'a' && r <= 'z':
				lat = append(lat, i)
			}
		}
		if !slices.IsSortedFunc(heb, func(a, b int) int { return b - a }) || !slices.IsSorted(lat) {
			t.Errorf("line %d %q visual %v", n, got[n], vis)
		}
		// Visible content starts at 0 (trailing spaces hang left).
		minX := float32(1e9)
		for i := ln.Start; i < ln.Start+len(ln.carets)-1; i++ {
			if l.Runes[i] != ' ' {
				minX = min(minX, ln.bidi.xl[i-ln.Start])
			}
		}
		if math.Abs(float64(minX)) > 0.01 {
			t.Errorf("line %d starts at %v", n, minX)
		}
	}
	// Line 0 is "אבג " -> first line shows ג ב א from left to right.
	if v := visualRunes(l, 0); v[len(v)-1] != 0 {
		t.Errorf("first line visual %v: rune 0 must be rightmost", v)
	}
}

func TestRTLEllipsisOnTheLeft(t *testing.T) {
	withoutFonts(t)
	l := Shape("אבגדהוזחט", Style{Size: 14}, Options{MaxWidth: 40, NoWrap: true, Ellipsis: true})
	ln := l.Lines[0]
	if !l.Truncated || ln.End >= 9 {
		t.Fatalf("not truncated: %+v", ln)
	}
	// The kept runes are shifted right by the ellipsis width.
	if x, _, _ := l.Caret(ln.End); x < 5 {
		t.Errorf("end caret %v: ellipsis room missing on the left", x)
	}
}

// --- With real fonts ------------------------------------------------------

// needFont skips unless the layout of s shapes every rune with a face that
// has it.
func needFont(t *testing.T, s string) {
	t.Helper()
	UseSystemFonts(true)
	l := Shape(s, Style{Size: 20}, Options{})
	if len(l.Glyphs) == 0 {
		t.Skip("no fonts available")
	}
	for _, g := range l.Glyphs {
		if g.ID == 0 {
			t.Skipf("no font covering %q on this machine", s)
		}
	}
}

// clusterOrder returns GlyphCluster with consecutive duplicates removed.
func clusterOrder(l *Layout) []int {
	var out []int
	for _, c := range l.GlyphCluster {
		if len(out) == 0 || out[len(out)-1] != int(c) {
			out = append(out, int(c))
		}
	}
	return out
}

func TestGlyphVisualOrderHebrew(t *testing.T) {
	needFont(t, "שלום")
	l := Shape("שלום", Style{Size: 20}, Options{})
	if len(l.GlyphCluster) != len(l.Glyphs) {
		t.Fatalf("GlyphCluster len %d, Glyphs %d", len(l.GlyphCluster), len(l.Glyphs))
	}
	if got := clusterOrder(l); !reflect.DeepEqual(got, []int{3, 2, 1, 0}) {
		t.Fatalf("clusters %v", got)
	}
	for i := 1; i < len(l.Glyphs); i++ {
		if l.Glyphs[i].X < l.Glyphs[i-1].X {
			t.Fatalf("glyph X not increasing: %+v", l.Glyphs)
		}
	}
	x, _, _ := l.Caret(0)
	if math.Abs(float64(x-l.Width)) > 0.5 {
		t.Errorf("Caret(0)=%v, want right edge %v", x, l.Width)
	}
	caretRoundTrip(t, l)
}

func TestGlyphVisualOrderArabic(t *testing.T) {
	needFont(t, "مرحبا")
	l := Shape("مرحبا", Style{Size: 20}, Options{})
	got := clusterOrder(l)
	if !slices.IsSortedFunc(got, func(a, b int) int { return b - a }) || got[len(got)-1] != 0 {
		t.Fatalf("Arabic clusters %v not right-to-left", got)
	}
	// Contextual shaping happened: the joined forms differ from the
	// isolated ones shaped one by one.
	iso := Shape("ح", Style{Size: 20}, Options{}).Glyphs[0].ID
	for i, c := range l.GlyphCluster {
		if c == 2 && l.Glyphs[i].ID == iso {
			t.Errorf("medial ح drawn with its isolated glyph")
		}
	}
	caretRoundTrip(t, l)
}

func TestMixedGlyphOrderWithFonts(t *testing.T) {
	needFont(t, "abc שלום def")
	l := Shape("abc שלום def", Style{Size: 20}, Options{})
	if got := clusterOrder(l); !reflect.DeepEqual(got, []int{0, 1, 2, 3, 7, 6, 5, 4, 8, 9, 10, 11}) {
		t.Errorf("abc שלום def clusters %v", got)
	}
	caretRoundTrip(t, l)
	if rs := l.SelectionRects(2, 6); len(rs) != 2 {
		t.Errorf("selection across the direction boundary: %v", rs)
	}

	needFont(t, "مرحبا 123 abc")
	l = Shape("مرحبا 123 abc", Style{Size: 20}, Options{})
	got := clusterOrder(l)
	// Latin and digits keep their order, the whole reads right to left.
	wantPrefix := []int{10, 11, 12, 9, 6, 7, 8, 5}
	if len(got) < len(wantPrefix) || !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Errorf("مرحبا 123 abc clusters %v, want prefix %v", got, wantPrefix)
	}
	if !slices.IsSortedFunc(got[len(wantPrefix):], func(a, b int) int { return b - a }) {
		t.Errorf("Arabic part not right-to-left: %v", got)
	}
	if !l.Lines[0].RTL {
		t.Errorf("paragraph should be RTL")
	}
	caretRoundTrip(t, l)
}

func TestWrappedMixedWithFonts(t *testing.T) {
	needFont(t, "שלום")
	s := "hello שלום עולם world again"
	l := Shape(s, Style{Size: 20}, Options{MaxWidth: 120})
	if len(l.Lines) < 2 {
		t.Fatalf("expected wrapping: %q", lines(l))
	}
	// Glyphs stay grouped by line (paintAlignedGlyphs relies on it) and
	// every glyph maps into its own line's rune range.
	ln := 0
	for g, c := range l.GlyphCluster {
		for ln < len(l.Lines)-1 && l.Glyphs[g].Y > l.Lines[ln].Baseline+0.5 {
			ln++
		}
		if int(c) < l.Lines[ln].Start || int(c) >= l.Lines[ln].End {
			t.Fatalf("glyph %d cluster %d outside line %d %+v", g, c, ln, l.Lines[ln])
		}
	}
	caretRoundTrip(t, l)
}

func TestShapeLanguage(t *testing.T) {
	UseSystemFonts(true)
	if len(Shape("字", Style{}, Options{}).Glyphs) == 0 {
		t.Skip("no fonts")
	}
	var langs []string
	shapeHook = func(in shaping.Input) { langs = append(langs, string(in.Language)+"/"+in.Script.String()) }
	defer func() { shapeHook = nil }()
	shapeWith := func(s string, st Style) []string {
		langs = nil
		clearCache()
		Shape(s, st, Options{})
		return langs
	}
	if got := shapeWith("直", Style{Lang: "ja"}); len(got) != 1 || !strings.HasPrefix(got[0], "ja/") {
		t.Errorf("ja: %v", got)
	}
	if got := shapeWith("直", Style{Lang: "zh-Hans"}); len(got) != 1 || !strings.HasPrefix(got[0], "zh-hans/") {
		t.Errorf("zh-Hans: %v", got)
	}
	// Empty Lang follows the default.
	if got := shapeWith("直", Style{}); len(got) != 1 || !strings.HasPrefix(got[0], "zh-hans/") {
		t.Errorf("default: %v", got)
	}
	SetDefaultLang("ko")
	defer SetDefaultLang("")
	if got := shapeWith("直", Style{}); len(got) != 1 || !strings.HasPrefix(got[0], "ko/") {
		t.Errorf("default ko: %v", got)
	}
	// The cache keys on the resolved language.
	a := Shape("直", Style{}, Options{})
	SetDefaultLang("ja")
	if b := Shape("直", Style{}, Options{}); a == b {
		t.Errorf("changing the default language reused the cached layout")
	}
	// Each bidi item gets its script; a foreign-script run gets that
	// script's language.
	if got := shapeWith("abc مرحبا", Style{Lang: "en"}); len(got) != 2 || !strings.HasPrefix(got[0], "en/") || !strings.HasPrefix(got[1], "ar/") {
		t.Errorf("en + Arabic: %v", got)
	}

	// Glyph forms: the same ideograph picks its Japanese form under "ja"
	// when the CJK face has locl forms (PingFang on macOS does).
	differ := 0
	for _, r := range []string{"直", "骨", "化", "写"} {
		ja := Shape(r, Style{Lang: "ja"}, Options{}).Glyphs
		zh := Shape(r, Style{Lang: "zh-Hans"}, Options{}).Glyphs
		if len(ja) == 1 && len(zh) == 1 && ja[0].ID != zh[0].ID {
			differ++
			t.Logf("%s: ja glyph %d, zh-Hans glyph %d (locl applied)", r, ja[0].ID, zh[0].ID)
		}
	}
	// ui.SetDefaultLocale(ZhCN) sets "zh-CN": same forms as the initial
	// "zh-Hans".
	for _, r := range []string{"直骨化写，。", "门户 English"} {
		a := Shape(r, Style{Lang: "zh-CN"}, Options{}).Glyphs
		b := Shape(r, Style{Lang: "zh-Hans"}, Options{}).Glyphs
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%q: zh-CN and zh-Hans glyphs differ", r)
		}
	}
	if differ == 0 {
		if runtime.GOOS == "darwin" {
			t.Errorf("no ja/zh-Hans glyph difference with the macOS CJK face")
		} else {
			t.Logf("CJK face has no ja locl forms; only Input.Language is checked")
		}
	}
}

// TestPureLTRUnchanged lays out left-to-right text with the current code
// and with the frozen pre-bidi copy (legacy_layout_test.go) and requires
// identical layouts, with and without fonts.
func TestPureLTRUnchanged(t *testing.T) {
	texts := []string{
		"", "hello", "hello world again and again and again",
		"一二三四五，六七八九十。甲乙丙", "中文 English 混排 123，标点「引号」",
		"a\tb\tc", "line one\nline two\n\nline four", "é café ﬁ office",
		"emoji 😀👍🏽 mix", "  leading and trailing  ", "ＡＢＣ全角　空格",
	}
	styles := []Style{{}, {Size: 12}, {Size: 20, Weight: Bold}, {Mono: true}, {Italic: true}}
	opts := []Options{{}, {MaxWidth: 80}, {MaxWidth: 60, MaxLines: 2, Ellipsis: true}, {MaxWidth: 50, NoWrap: true, Ellipsis: true}, {MaxWidth: 120, MaxLines: 1}}
	check := func(t *testing.T) {
		glyphs := 0
		defer func() {
			if len(reg.chain(false)) > 0 && glyphs == 0 {
				t.Errorf("no glyphs laid out with fonts")
			}
		}()
		for _, s := range texts {
			for _, st := range styles {
				for _, opt := range opts {
					st := st.norm()
					chain := reg.chain(st.Mono)
					mu.Lock()
					got := layout(s, st, opt, chain, resolveLang(st))
					want := legacyLayout(s, st, opt, chain)
					mu.Unlock()
					if len(got.GlyphCluster) != len(got.Glyphs) {
						t.Fatalf("%q: GlyphCluster len %d != %d", s, len(got.GlyphCluster), len(got.Glyphs))
					}
					got.GlyphCluster = nil
					if len(got.GlyphRun) != len(got.Glyphs) || slices.ContainsFunc(got.GlyphRun, func(r int32) bool { return r != 0 }) {
						t.Fatalf("%q: GlyphRun %v, want %d zeros", s, got.GlyphRun, len(got.Glyphs))
					}
					got.GlyphRun = nil
					glyphs += len(got.Glyphs)
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%q %+v %+v: layout changed\n got %+v\nwant %+v", s, st, opt, got.Lines, want.Lines)
					}
				}
			}
		}
	}
	t.Run("fonts", func(t *testing.T) { UseSystemFonts(true); check(t) })
	t.Run("nofonts", func(t *testing.T) { withoutFonts(t); check(t) })
}
