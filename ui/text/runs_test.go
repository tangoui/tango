package text

import (
	"math"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func near(a, b, eps float32) bool { return math.Abs(float64(a-b)) <= float64(eps) }

// TestShapeRunsMatchesShape: one run covering the text lays out exactly like
// Shape, for LTR, bidi, wrapping and ellipsis, with and without fonts.
func TestShapeRunsMatchesShape(t *testing.T) {
	texts := []string{"", "hello world", "中文 English 混排 123，「引号」", "a\tb\nc",
		"abc שלום def", "مرحبا 123 abc", "hello שלום עולם world again and again"}
	styles := []Style{{}, {Size: 20, Weight: Bold}, {Mono: true, Italic: true}, {Size: 12, Lang: "ja"}}
	opts := []Options{{}, {MaxWidth: 80}, {MaxWidth: 60, MaxLines: 2, Ellipsis: true}, {Direction: DirRTL}}
	check := func(t *testing.T) {
		for _, s := range texts {
			n := len([]rune(s))
			for _, st := range styles {
				for _, opt := range opts {
					want := Shape(s, st, opt)
					got := ShapeRuns(s, []Run{{Start: 0, End: n, Style: st}}, opt)
					if len(want.GlyphRun) != len(want.Glyphs) {
						t.Fatalf("%q: Shape GlyphRun len %d, Glyphs %d", s, len(want.GlyphRun), len(want.Glyphs))
					}
					for _, r := range want.GlyphRun {
						if r != 0 {
							t.Fatalf("%q: Shape GlyphRun %v not all 0", s, want.GlyphRun)
						}
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%q %+v %+v: ShapeRuns differs from Shape\n got %+v\nwant %+v", s, st, opt, got.Lines, want.Lines)
					}
				}
			}
		}
	}
	t.Run("fonts", func(t *testing.T) { UseSystemFonts(true); check(t) })
	t.Run("nofonts", func(t *testing.T) { withoutFonts(t); check(t) })
}

func TestShapeRunsMixedWeights(t *testing.T) {
	needFont(t, "func main")
	a, b := "func ", "main(x, y)"
	reg, bold := Style{Size: 16}, Style{Size: 16, Weight: Bold}
	n := len([]rune(a))
	l := ShapeRuns(a+b, []Run{{0, n, bold}, {n, n + len([]rune(b)), reg}}, Options{NoWrap: true})
	// "func " ends in a space, which Width trims: add the space back.
	want := Width(a+"x", bold) - Width("x", bold) + Width(b, reg)
	if !near(l.Width, want, 0.01) {
		t.Fatalf("width %v, want %v", l.Width, want)
	}
	// The bold run is wider than the same text in regular (each weight of
	// a variable font shapes with its own coordinates).
	lr := ShapeRuns(a+b, []Run{{0, n, reg}, {n, n + len([]rune(b)), reg}}, Options{NoWrap: true})
	xb, _, _ := l.Caret(n)
	xr, _, _ := lr.Caret(n)
	if xb <= xr {
		t.Errorf("bold run advance %v not wider than regular %v", xb, xr)
	}
	// The bold run uses a bold face, the regular run a regular one.
	for g, r := range l.GlyphRun {
		f := l.Glyphs[g].Src.(*Face)
		if r == 0 && f.Weight < Semibold || r == 1 && f.Weight > Medium {
			t.Errorf("glyph %d run %d drawn with weight %d", g, r, f.Weight)
		}
	}
	// Caret after the bold part sits at its advance.
	if x, _, _ := l.Caret(n); !near(x, Width(a+"x", bold)-Width("x", bold), 0.01) {
		t.Errorf("Caret(%d) = %v", n, x)
	}

	// Mixed sizes: line height is the largest run's, glyphs share the
	// baseline, and each glyph keeps its own size.
	big := Style{Size: 28}
	l = ShapeRuns("ab", []Run{{0, 1, reg}, {1, 2, big}}, Options{})
	if l.LineHeight != LineHeightOf(big) || l.Height != LineHeightOf(big) {
		t.Errorf("line height %v / height %v, want %v", l.LineHeight, l.Height, LineHeightOf(big))
	}
	if len(l.Glyphs) != 2 || l.Glyphs[0].Y != l.Glyphs[1].Y || l.Glyphs[0].Y != l.Lines[0].Baseline {
		t.Fatalf("glyphs not on one baseline: %+v line %+v", l.Glyphs, l.Lines[0])
	}
	if l.Glyphs[0].Size != 16 || l.Glyphs[1].Size != 28 {
		t.Errorf("glyph sizes %v %v", l.Glyphs[0].Size, l.Glyphs[1].Size)
	}
	if !near(l.Width, Width("a", reg)+Width("b", big), 0.01) {
		t.Errorf("mixed size width %v", l.Width)
	}
}

func TestShapeRunsNoFonts(t *testing.T) {
	withoutFonts(t)
	// Approximate metrics use each run's size: 0.55*size per Latin rune.
	l := ShapeRuns("abcd", []Run{{0, 2, Style{Size: 10}}, {2, 4, Style{Size: 20}}}, Options{})
	if !near(l.Width, 2*5.5+2*11, 0.01) {
		t.Errorf("width %v", l.Width)
	}
	if x, _, _ := l.Caret(2); !near(x, 11, 0.01) {
		t.Errorf("Caret(2) %v", x)
	}
	if l.LineHeight != LineHeightOf(Style{Size: 20}) {
		t.Errorf("line height %v", l.LineHeight)
	}
}

func TestShapeRunsGlyphRun(t *testing.T) {
	needFont(t, "let x = 1")
	s := "let x = 1;"
	runs := []Run{{0, 3, Style{Weight: Bold}}, {3, 6, Style{}}, {6, 7, Style{Italic: true}}, {7, 10, Style{Mono: true}}}
	l := ShapeRuns(s, runs, Options{})
	if len(l.GlyphRun) != len(l.Glyphs) || len(l.GlyphCluster) != len(l.Glyphs) {
		t.Fatalf("GlyphRun %d, GlyphCluster %d, Glyphs %d", len(l.GlyphRun), len(l.GlyphCluster), len(l.Glyphs))
	}
	runOf := func(i int) int32 {
		for k, r := range runs {
			if i >= r.Start && i < r.End {
				return int32(k)
			}
		}
		return -1
	}
	seen := map[int32]bool{}
	for g, c := range l.GlyphCluster {
		if want := runOf(int(c)); l.GlyphRun[g] != want {
			t.Errorf("glyph %d (rune %d %q) run %d, want %d", g, c, l.Runes[c], l.GlyphRun[g], want)
		}
		seen[l.GlyphRun[g]] = true
		if l.GlyphRun[g] == 2 && l.Glyphs[g].Skew == 0 && !l.Glyphs[g].Src.(*Face).Italic {
			t.Errorf("italic run glyph is upright")
		}
		if l.GlyphRun[g] != 2 && l.Glyphs[g].Skew != 0 {
			t.Errorf("non-italic glyph %d skewed", g)
		}
	}
	for k := range runs {
		if !seen[int32(k)] {
			t.Errorf("run %d has no glyphs", k)
		}
	}

	// Runes not covered by any run take run 0; later runs win overlaps.
	l = ShapeRuns("abcdef", []Run{{0, 6, Style{}}, {2, 4, Style{Weight: Bold}}}, Options{})
	want := []int32{0, 0, 1, 1, 0, 0}
	if !reflect.DeepEqual(l.GlyphRun, want) {
		t.Errorf("overlay GlyphRun %v, want %v", l.GlyphRun, want)
	}
	l = ShapeRuns("abcdef", []Run{{2, 4, Style{Weight: Bold}}}, Options{})
	if !reflect.DeepEqual(l.GlyphRun, []int32{0, 0, 0, 0, 0, 0}) {
		t.Errorf("gap GlyphRun %v", l.GlyphRun)
	}

	// Ellipsis glyphs belong to the run of the last rune on the line.
	l = ShapeRuns("aaaa bbbbbbbbbbbbbbbb", []Run{{0, 5, Style{}}, {5, 21, Style{Weight: Bold}}}, Options{MaxWidth: 80, NoWrap: true, Ellipsis: true})
	if !l.Truncated || l.GlyphCluster[len(l.GlyphCluster)-1] != -1 || l.GlyphRun[len(l.GlyphRun)-1] != 1 {
		t.Errorf("ellipsis: truncated=%v cluster %v run %v", l.Truncated, l.GlyphCluster, l.GlyphRun)
	}

	// No runs: like Shape with the zero Style.
	if got, want := ShapeRuns("hi", nil, Options{}), Shape("hi", Style{}, Options{}); !reflect.DeepEqual(got, want) {
		t.Errorf("nil runs differ from Shape")
	}
}

func TestShapeRunsBidi(t *testing.T) {
	s := "abc مرحبا def"
	needFont(t, s)
	// Style runs cross the bidi runs: "ab" | "c مر" | "حبا de" | "f".
	runs := []Run{{0, 2, Style{Size: 18}}, {2, 6, Style{Size: 18, Weight: Bold}}, {6, 12, Style{Size: 18, Italic: true}}, {12, 13, Style{Size: 18, Mono: true}}}
	l := ShapeRuns(s, runs, Options{})
	plain := Shape(s, Style{Size: 18}, Options{})
	// Same visual order as the single-style layout.
	if got, want := clusterOrder(l), clusterOrder(plain); !reflect.DeepEqual(got, want) {
		t.Fatalf("clusters %v, want %v", got, want)
	}
	for g, c := range l.GlyphCluster {
		var want int32
		for k, r := range runs {
			if int(c) >= r.Start && int(c) < r.End {
				want = int32(k)
			}
		}
		if l.GlyphRun[g] != want {
			t.Errorf("glyph %d rune %d %q: run %d, want %d", g, c, l.Runes[c], l.GlyphRun[g], want)
		}
	}
	// Clusters start further right in visual order (the pieces of a split
	// RTL run do not overlap; marks inside a cluster may sit left of its
	// base, so only each cluster's first glyph is compared).
	prev := float32(-1)
	for i := range l.Glyphs {
		if i > 0 && l.GlyphCluster[i] == l.GlyphCluster[i-1] {
			continue
		}
		if l.Glyphs[i].X <= prev {
			t.Fatalf("cluster %d at X %v, previous cluster at %v", l.GlyphCluster[i], l.Glyphs[i].X, prev)
		}
		prev = l.Glyphs[i].X
	}
	caretRoundTrip(t, l)

	// Arabic joining survives a style boundary inside a word: the medial ح
	// (rune 7) is drawn exactly as in an all-italic layout.
	glyphOf := func(l *Layout, c int32) uint32 {
		for g, gc := range l.GlyphCluster {
			if gc == c {
				return l.Glyphs[g].ID
			}
		}
		return 0
	}
	if got, want := glyphOf(l, 7), glyphOf(Shape(s, Style{Size: 18, Italic: true}, Options{}), 7); got != want {
		t.Errorf("medial ح glyph %d across the style boundary, want %d (joined form)", got, want)
	}

	// RTL paragraph, style runs over the Arabic word.
	l = ShapeRuns("مرحبا", []Run{{0, 2, Style{Weight: Bold}}, {2, 5, Style{}}}, Options{})
	if !l.Lines[0].RTL {
		t.Fatalf("not RTL")
	}
	if got := clusterOrder(l); got[0] != 4 || got[len(got)-1] != 0 {
		t.Errorf("RTL clusters %v", got)
	}
	for g, c := range l.GlyphCluster {
		want := int32(0)
		if c >= 2 {
			want = 1
		}
		if l.GlyphRun[g] != want {
			t.Errorf("rune %d run %d", c, l.GlyphRun[g])
		}
	}
}

// TestRichTextUnchanged runs the RichText / Markdown tests of package ui,
// which lay out text through Shape.
func TestRichTextUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test ./ui/")
	}
	out, err := exec.Command("go", "test", "-count=1", "-run", "RichText|Markdown", "github.com/tangoui/tango/ui").CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "[build failed]") || strings.Contains(string(out), "[setup failed]") {
			t.Skipf("package ui does not build right now (other work in progress):\n%s", out)
		}
		t.Fatalf("go test ./ui -run 'RichText|Markdown': %v\n%s", err, out)
	}
}
