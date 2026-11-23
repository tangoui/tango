package editor

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// guideDoc is three nested multi-line pairs (and single-line ones):
//
//	0 func f() {        pair A: lines 0..6, guide column 0
//	1 	if x {          pair B: lines 1..5, guide column 4
//	2 		for {       pair C: lines 2..4, guide column 8
//	3 			y(1)
//	4 		}
//	5 	}
//	6 }
const guideDoc = "func f() {\n\tif x {\n\t\tfor {\n\t\t\ty(1)\n\t\t}\n\t}\n}\n"

// guideView shows text with the bracket guide options set by mods and
// returns the driver and the ViewW (after one frame).
func guideView(t *testing.T, text string, w, h float32, mods func(o *Options)) (*State, func() *ViewW, func() *image.RGBA) {
	t.Helper()
	return guideViewLang(t, text, "a.go", w, h, mods)
}

// guideViewLang is guideView for a file name of another language.
func guideViewLang(t *testing.T, text, name string, w, h float32, mods func(o *Options)) (*State, func() *ViewW, func() *image.RGBA) {
	t.Helper()
	goText(t)
	st := NewState(text, name, theme.Base("vs"))
	if mods != nil {
		mods(&st.Options)
	}
	d, vw := viewDriver(st, w, h, nil)
	d.Frame()
	d.Click(geom.Pt(w/3, h-10)) // focus (left of the minimap)
	return st, func() *ViewW { return *vw }, d.Frame
}

// vertical returns the vertical guides of the rows of each model line
// ("line:row" -> "col/level[*]", * for the active color).
func verticals(w *ViewW) map[string][]string {
	out := map[string][]string{}
	for _, r := range w.vs.bgRows {
		for _, g := range r.g {
			if g.kind != bgVertical {
				continue
			}
			s := fmt.Sprintf("%d/%d", g.col, g.level)
			if g.hl {
				s += "*"
			}
			k := fmt.Sprintf("%d:%d", r.line, r.row)
			out[k] = append(out[k], s)
		}
	}
	for _, v := range out {
		slices.Sort(v)
	}
	return out
}

func horizontals(w *ViewW) map[int][]bracketGuide {
	out := map[int][]bracketGuide{}
	for _, r := range w.vs.bgRows {
		for _, g := range r.g {
			if g.kind != bgVertical {
				out[r.line] = append(out[r.line], g)
			}
		}
	}
	return out
}

func eqGuides(t *testing.T, what string, got, want map[string][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: guides %v, want %v", what, got, want)
		return
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Errorf("%s: guides %v, want %v", what, got, want)
			return
		}
	}
}

func TestBracketGuidesBasic(t *testing.T) {
	st, vw, frame := guideView(t, guideDoc, 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
		o.Guides.HighlightActiveBracketPair = false
	})
	caretAt(st, 3, 3) // in pair C (before y)
	frame()
	eqGuides(t, "on", verticals(vw()), map[string][]string{
		"1:0": {"0/0"},
		"2:0": {"0/0", "4/1"},
		"3:0": {"0/0", "4/1", "8/2"},
		"4:0": {"0/0", "4/1"},
		"5:0": {"0/0"},
	})
	if h := horizontals(vw()); len(h) != 0 {
		t.Errorf("horizontal guides with bracketPairsHorizontal false: %v", h)
	}
	// The colors: the nesting levels of the bracket colors, the guide
	// colors falling back to the bracket colors at 30% (vs has no
	// editorBracketPairGuide colors).
	bc := bracketGuideColorsOf(themeOf(st))
	pal := paletteOf(themeOf(st))
	if len(bc.bg) != len(pal.levels) || len(bc.bg) != 3 {
		t.Fatalf("%d guide colors, %d bracket colors, want 3", len(bc.bg), len(pal.levels))
	}
	for k, c := range pal.levels {
		want := c
		want.A = uint8(math.Round(float64(c.A) * 0.3))
		if bc.bg[k] != want || bc.active[k] != c {
			t.Errorf("level %d: guide %v active %v, want %v and %v", k, bc.bg[k], bc.active[k], want, c)
		}
	}
	// Drawn: the guide of pair B (level 1) on line 3 is a column of the
	// level 1 color blended over the background.
	w := vw()
	img := frame()
	lh := w.lineHeight()
	x := w.vs.text.X + float32(4)*w.vs.colW
	y := w.vs.text.Y + 3*lh - w.st.Viewport.OffsetY
	if n := countColorNear(img, geom.Rect{X: float32(math.Round(float64(x))) - 1, Y: y + 2, W: 3, H: lh - 4}, blendOver(bc.bg[1], defaultColor(themeOf(st), "editor.background"))); n == 0 {
		t.Errorf("guide of pair B not drawn in its color")
	}
}

func blendOver(c, bg color.NRGBA) color.NRGBA {
	a := float64(c.A) / 255
	mix := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	return color.NRGBA{mix(c.R, bg.R), mix(c.G, bg.G), mix(c.B, bg.B), 255}
}

// countColorNear counts the pixels of r within 3 of c on every channel.
func countColorNear(img interface {
	RGBAAt(x, y int) color.RGBA
}, r geom.Rect, c color.NRGBA) int {
	n := 0
	d := func(a, b uint8) int { return max(int(a), int(b)) - min(int(a), int(b)) }
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			p := img.RGBAAt(x, y)
			if d(p.R, c.R) <= 3 && d(p.G, c.G) <= 3 && d(p.B, c.B) <= 3 {
				n++
			}
		}
	}
	return n
}

func TestBracketGuidesActive(t *testing.T) {
	st, vw, frame := guideView(t, guideDoc, 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesActive
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
	})
	caretAt(st, 3, 3) // in pair C
	frame()
	eqGuides(t, "caret in C", verticals(vw()), map[string][]string{"3:0": {"8/2*"}})
	caretAt(st, 5, 1) // before the "}" of B: inside B
	frame()
	eqGuides(t, "caret in B", verticals(vw()), map[string][]string{
		"2:0": {"4/1*"}, "3:0": {"4/1*"}, "4:0": {"4/1*"},
	})
	caretAt(st, 1, 0) // line 1's indentation: in A only
	frame()
	eqGuides(t, "caret in A", verticals(vw()), map[string][]string{
		"1:0": {"0/0*"}, "2:0": {"0/0*"}, "3:0": {"0/0*"}, "4:0": {"0/0*"}, "5:0": {"0/0*"},
	})
	// The active pair is the innermost one, even on one line: no
	// vertical guide (as Monaco).
	caretAt(st, 3, 6) // inside y(1)
	frame()
	eqGuides(t, "caret in y(1)", verticals(vw()), nil)
	caretAt(st, 6, 1) // after the last "}": in no pair
	frame()
	eqGuides(t, "caret outside", verticals(vw()), nil)

	// With every pair drawn, only the active one is in the active color;
	// without highlightActiveBracketPair none is.
	st.Options.Guides.BracketPairs = BracketGuidesOn
	caretAt(st, 3, 3)
	frame()
	eqGuides(t, "on, caret in C", verticals(vw()), map[string][]string{
		"1:0": {"0/0"}, "2:0": {"0/0", "4/1"}, "3:0": {"0/0", "4/1", "8/2*"}, "4:0": {"0/0", "4/1"}, "5:0": {"0/0"},
	})
	st.Options.Guides.HighlightActiveBracketPair = false
	frame()
	eqGuides(t, "on, no highlight", verticals(vw()), map[string][]string{
		"1:0": {"0/0"}, "2:0": {"0/0", "4/1"}, "3:0": {"0/0", "4/1", "8/2"}, "4:0": {"0/0", "4/1"}, "5:0": {"0/0"},
	})
}

func TestBracketGuidesHorizontal(t *testing.T) {
	st, vw, frame := guideView(t, guideDoc, 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
	})
	caretAt(st, 3, 3) // in pair C
	w := vw()
	snap := st.Snapshot()
	colW := w.vs.colW
	// xAt is the content x of byte b of line i (one column per byte
	// after the tabs of guideDoc's lines).
	xAt := func(i, b int) float32 {
		s := snap.Line(i)
		return float32(lineColumnTab(s, b, 4)) * colW
	}
	type hz struct {
		line, kind int
		x0, x1     float32
		active     bool
	}
	got := func() []hz {
		var out []hz
		for line, gs := range horizontals(vw()) {
			for _, g := range gs {
				out = append(out, hz{line, g.kind, g.x0, g.x1, g.active})
			}
		}
		slices.SortFunc(out, func(a, b hz) int { return a.line - b.line })
		return out
	}
	near := func(a, b []hz) bool {
		if len(a) != len(b) {
			return false
		}
		for k := range a {
			if a[k].line != b[k].line || a[k].kind != b[k].kind || a[k].active != b[k].active ||
				math.Abs(float64(a[k].x0-b[k].x0)) > 0.5 || math.Abs(float64(a[k].x1-b[k].x1)) > 0.5 {
				return false
			}
		}
		return true
	}
	brace := func(i int) int { return strings.Index(snap.Line(i), "{") }

	// "active": the active pair C only, under its opening line from its
	// guide column to the bracket (its closing bracket is at the guide).
	st.Options.Guides.BracketPairsHorizontal = BracketGuidesActive
	frame()
	if g, want := got(), []hz{{2, bgBottom, 8 * colW, xAt(2, brace(2)), true}}; !near(g, want) {
		t.Errorf("active: %v, want %v", g, want)
	}
	// "true": every pair, and the single-line pair y(1) between its
	// brackets ("()" in "f()" is empty: nothing to draw).
	st.Options.Guides.BracketPairsHorizontal = BracketGuidesOn
	frame()
	y0 := strings.Index(snap.Line(3), "(")
	want := []hz{
		{0, bgBottom, 0, xAt(0, brace(0)), false},
		{1, bgBottom, 4 * colW, xAt(1, brace(1)), false},
		{2, bgBottom, 8 * colW, xAt(2, brace(2)), true},
		{3, bgBottom, xAt(3, y0+1), xAt(3, y0+2), false},
	}
	if g := got(); !near(g, want) {
		t.Errorf("true: %v, want %v", g, want)
	}
	// "false": none.
	st.Options.Guides.BracketPairsHorizontal = BracketGuidesOff
	frame()
	if g := got(); len(g) != 0 {
		t.Errorf("false: %v", g)
	}

	// A closing bracket after text: the vertical guide reaches the
	// closing line and the horizontal line is drawn at its bottom.
	st2, vw2, frame2 := guideView(t, "x := g(\n\ta,\n\tb)\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOn
	})
	caretAt(st2, 1, 1)
	frame2()
	eqGuides(t, "text before the closing bracket", verticals(vw2()), map[string][]string{"1:0": {"4/0*"}, "2:0": {"4/0*"}})
	h := horizontals(vw2())
	s2 := st2.Snapshot()
	c2 := vw2().vs.colW
	if len(h[0]) != 1 || h[0][0].kind != bgBottom || math.Abs(float64(h[0][0].x1-float32(strings.Index(s2.Line(0), "("))*c2)) > 0.5 {
		t.Errorf("opening line: %v", h[0])
	}
	if len(h[2]) != 1 || h[2][0].kind != bgBottom || math.Abs(float64(h[2][0].x1-float32(lineColumnTab(s2.Line(2), 2, 4))*c2)) > 0.5 {
		t.Errorf("closing line after text: %v", h[2])
	}
	// The same pair closed on a line of its own: the horizontal line is at
	// the top of the closing line, from the guide (the closing bracket is
	// at column 0, the guide's): none.
	st3, vw3, frame3 := guideView(t, "x := g(\n\ta,\n\t)\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOn
	})
	caretAt(st3, 1, 1)
	frame3()
	h3 := horizontals(vw3())
	// Guide column: min(opening column 6, closing column 4, indent 4) = 4.
	if len(h3[2]) != 0 {
		t.Errorf("closing bracket at the guide column: %v", h3[2])
	}
	if len(h3[0]) != 1 || h3[0][0].kind != bgBottom || math.Abs(float64(h3[0][0].x0-4*vw3().vs.colW)) > 0.5 {
		t.Errorf("opening line from the guide column: %v", h3[0])
	}
	eqGuides(t, "closing line of its own", verticals(vw3()), map[string][]string{"1:0": {"4/0*"}})
	// A closing bracket right of the guide on a line of its own: a top
	// line.
	st4, vw4, frame4 := guideView(t, "x := g(\na,\n\t)\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOn
	})
	caretAt(st4, 1, 1)
	frame4()
	if h4 := horizontals(vw4()); len(h4[2]) != 1 || h4[2][0].kind != bgTop || h4[2][0].x0 != 0 || math.Abs(float64(h4[2][0].x1-4*vw4().vs.colW)) > 0.5 {
		t.Errorf("closing line right of the guide: %v", h4[2])
	}
}

func TestBracketGuidesFoldWrap(t *testing.T) {
	// Folding B (lines 2..4 hidden): its guide is not drawn (its header is
	// its opening line, its closing line has the bracket only), A's
	// continues past the folded lines.
	st, vw, frame := guideView(t, guideDoc, 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
		o.Guides.HighlightActiveBracketPair = false
	})
	caretAt(st, 1, 2)
	if !st.Execute("editor.fold", nil) {
		t.Fatal("editor.fold did not run")
	}
	frame()
	eqGuides(t, "folded", verticals(vw()), map[string][]string{"1:0": {"0/0"}, "5:0": {"0/0"}})
	for _, r := range vw().vs.bgRows {
		if r.line >= 2 && r.line <= 4 {
			t.Errorf("guides on hidden line %d", r.line)
		}
	}

	// Wrapping: the continuation rows of a long line inside the pairs
	// repeat the guides (continuous lines); the opening line's rows after
	// the bracket's row have the guide too.
	long := strings.Repeat("word ", 40)
	doc := "func f() {\n\tif x {\n\t\t" + long + "\n\t}\n}\n"
	st2 := NewState(doc, "a.go", theme.Base("vs"))
	defer st2.Close()
	st2.Options.Guides.BracketPairs = BracketGuidesOn
	st2.Options.Guides.BracketPairsHorizontal = BracketGuidesOff
	st2.Options.Guides.HighlightActiveBracketPair = false
	st2.Options.WordWrap = WrapColumn
	st2.Options.WordWrapColumn = 23
	st2.Options.LineHeight = 20
	// The outer closer can fall below the laid-out rows. Make both
	// closers available before checking how their guides span the wrap.
	snap := st2.Snapshot()
	if !st2.Highlighter().Highlight(snap, snap.LineCount(), time.Second) {
		t.Fatal("failed to tokenize the wrap fixture")
	}
	d2, vw2 := viewDriver(st2, 360, 300, nil)
	d2.Frame()
	vs := verticals(*vw2)
	rows := 0
	for _, r := range (*vw2).vs.bgRows {
		if r.line != 2 {
			continue
		}
		rows++
		k := fmt.Sprintf("%d:%d", r.line, r.row)
		if v := vs[k]; !slices.Equal(v, []string{"0/0", "4/1"}) {
			t.Errorf("row %s of the wrapped line: %v", k, v)
		}
	}
	if rows < 2 {
		t.Fatalf("the long line is not wrapped (%d rows: %v)", rows, vs)
	}
	// An opening line wrapping after its bracket: the rows after the
	// bracket's row have the guide, the bracket's row not.
	doc3 := "f(" + long + "\n\tx\n)\n"
	st3, vw3, frame3 := guideView(t, doc3, 360, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
		o.Guides.HighlightActiveBracketPair = false
		o.WordWrap = WrapOn
	})
	caretAt(st3, 1, 0)
	frame3()
	v3 := verticals(vw3())
	if len(v3["0:0"]) != 0 || !slices.Equal(v3["0:1"], []string{"0/0"}) || !slices.Equal(v3["1:0"], []string{"0/0"}) {
		t.Errorf("wrapped opening line: %v", v3)
	}
	_ = st3
}

func TestBracketGuidesUnclosed(t *testing.T) {
	// An unclosed pair has no guide; one popped unclosed by an outer
	// closing bracket neither, the outer pair has its own.
	_, vw, _ := guideView(t, "f(\n\tx\n\ty\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOn
	})
	if len(vw().vs.bgRows) != 0 {
		for _, r := range vw().vs.bgRows {
			if len(r.g) > 0 {
				t.Errorf("guides of an unclosed pair: line %d %v", r.line, r.g)
			}
		}
	}
	_, vw2, _ := guideView(t, "{\n\t(\n\tx\n}\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
		o.Guides.HighlightActiveBracketPair = false
	})
	eqGuides(t, "popped unclosed", verticals(vw2()), map[string][]string{"1:0": {"0/0"}, "2:0": {"0/0"}})
}

func TestBracketGuidesOffByDefault(t *testing.T) {
	if o := DefaultOptions(); o.Guides.BracketPairs != BracketGuidesOff || o.Guides.BracketPairsHorizontal != BracketGuidesActive || !o.Guides.HighlightActiveBracketPair {
		t.Fatalf("defaults %+v, want bracketPairs false, bracketPairsHorizontal active, highlightActiveBracketPair true", o.Guides)
	}
	render := func(mods func(o *Options)) ([]uint8, *ViewW) {
		st, vw, frame := guideView(t, guideDoc, 520, 300, mods)
		caretAt(st, 3, 3)
		return append([]uint8(nil), frame().Pix...), vw()
	}
	def, w := render(nil)
	if w.vs.bgRows != nil {
		t.Errorf("bracket guides by default: %v", w.vs.bgRows)
	}
	for _, mods := range []func(o *Options){
		func(o *Options) { o.Guides.BracketPairs = "" },
		func(o *Options) { o.Guides.BracketPairs = "bogus" },
		func(o *Options) {
			o.Guides.BracketPairs = BracketGuidesOff
			o.Guides.BracketPairsHorizontal = BracketGuidesOn
			o.Guides.HighlightActiveBracketPair = false
		},
	} {
		img, w := render(mods)
		if w.vs.bgRows != nil || !slices.Equal(img, def) {
			t.Errorf("bracket guides off (%+v): drawn differently from the default", w.opts.Guides)
		}
	}
	on, _ := render(func(o *Options) { o.Guides.BracketPairs = BracketGuidesOn })
	if slices.Equal(on, def) {
		t.Errorf("bracket guides on: drawn as the default")
	}
}

// damageBracketGuides runs the bracket pair guides through the strict
// verifier (TestViewDamageVerified): the caret moving into another pair
// changes the active guides (only the active pair, then every pair with
// the active one highlighted), typing, unbalancing and rebalancing the
// brackets, word wrap and scrolling.
func damageBracketGuides(t *testing.T) {
	th := theme.Base("vs-dark")
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := range 30 {
		fmt.Fprintf(&sb, "func f%d(x []int) int {\n\tif x[0] > %d {\n\t\treturn g(x[1],\n\t\t\t%d)\n\t}\n\treturn 0\n}\n", i, i, i)
	}
	st := NewState(sb.String(), "main.go", th)
	o := &st.Options
	o.Guides.BracketPairs = BracketGuidesActive
	o.Guides.BracketPairsHorizontal = BracketGuidesActive
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, 1.25)
	h.focusAt(4)
	snap := st.Snapshot()
	rest := func() {
		h.now = h.now.Add(matchDelay)
		h.frame()
	}
	// Into the if, the call, the function, another function.
	for _, off := range []int{snap.LineStart(4) + 3, snap.LineStart(5) + 4, snap.LineStart(4) + 1,
		snap.LineStart(3) + 2, snap.LineStart(10) + 3, snap.LineStart(11) + 5, snap.LineStart(8) + 1} {
		st.MoveCaret(off, false)
		h.step("caret into another pair")
	}
	for _, k := range []input.Key{input.KeyDown, input.KeyDown, input.KeyUp, input.KeyRight, input.KeyEnd, input.KeyDown} {
		h.key(k, 0)
		h.step(fmt.Sprintf("key %v", k))
	}
	o.Guides.BracketPairs = BracketGuidesOn
	h.frame() // an option change: a full frame
	for _, off := range []int{snap.LineStart(4) + 3, snap.LineStart(5) + 4, snap.LineStart(3) + 2, snap.LineStart(11) + 5} {
		st.MoveCaret(off, false)
		h.step("caret into another pair (all pairs)")
	}
	h.typeText("y")
	h.step("type")
	st.MoveCaret(snap.LineStart(5), false)
	st.Insert("{")
	h.step("unbalanced")
	rest()
	st.Undo()
	h.step("undo")
	rest()
	p := geom.Pt(240, 180)
	for _, dy := range []float32{8, 24, -16, 40, 6} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step(fmt.Sprintf("scroll %v", dy))
	}
	o.Guides.BracketPairsHorizontal = BracketGuidesOn
	o.WordWrap = WrapOn
	h.frame()
	for _, off := range []int{snap.LineStart(12) + 3, snap.LineStart(13) + 4} {
		st.MoveCaret(off, false)
		h.step("caret (wrap)")
	}
	h.verified()
}

// jsonGuideDoc is a JSON document of n lines with two nested objects
// spanning almost all of them: {"packages": {"a": {...}}}, the entries
// indented by 6.
func jsonGuideDoc(n int) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"packages\": {\n    \"a\": {\n")
	for i := range n - 6 {
		fmt.Fprintf(&sb, "      \"key%06d\": \"value-%06d\",\n", i, i)
	}
	sb.WriteString("      \"last\": 0\n    }\n  }\n}\n")
	return sb.String()
}

// bruteMinIndent is the smallest indentation of the non-blank lines a..b,
// limit when none is below it.
func bruteMinIndent(st *State, a, b, limit int) int {
	snap := st.Snapshot()
	g := limit
	for i := a; i <= b; i++ {
		if ind := indentOf(snap.Line(i), 4); ind >= 0 {
			g = min(g, ind)
		}
	}
	return g
}

// TestBracketGuidesIndexEdits checks that the indentation index follows
// edits: a one character edit inside a pair of 125 000 lines reads a
// block of lines, not the pair, and the results equal a full count after
// edits of every kind.
func TestBracketGuidesIndexEdits(t *testing.T) {
	st := NewState(jsonGuideDoc(125_000), "a.json", nil)
	x := st.Model.indents(4)
	far := time.Now().Add(time.Hour)
	query := func(a, b, limit int) int {
		t.Helper()
		g, ok := x.minIndent(st.Snapshot(), 4, a, b, limit, far)
		if !ok {
			t.Fatal("undecided without a deadline")
		}
		if want := bruteMinIndent(st, a, b, limit); g != want {
			t.Fatalf("lines %d..%d: %d, want %d", a, b, g, want)
		}
		return g
	}
	n := st.Snapshot().LineCount()
	// "packages": { ... } and "a": { ... } (closing lines n-3, n-4).
	if g := query(2, n-3, 15); g != 2 {
		t.Fatalf("packages: %d", g)
	}
	query(3, n-4, 9)
	before := x.scanned
	st.MoveCaret(st.Snapshot().LineStart(5)+8, false)
	st.Insert("x")
	query(2, n-3, 15)
	query(3, n-4, 9)
	// At most: the edited block (up to 2*indentBlock lines) and, for each
	// of the two pairs, the parts of the blocks at its ends.
	if d := x.scanned - before; d > 4*indentBlock {
		t.Fatalf("a one character edit read %d lines (pair of %d lines)", d, n)
	}
	t.Logf("a one character edit near the pairs' start read %d lines (pair of %d lines)", x.scanned-before, n)
	// In the middle: the edited block only (the parts at the pairs' ends
	// are kept).
	before = x.scanned
	st.MoveCaret(st.Snapshot().LineStart(60_000)+8, false)
	st.Insert("x")
	query(2, n-3, 15)
	query(3, n-4, 9)
	if d := x.scanned - before; d > 2*indentBlock {
		t.Fatalf("a one character edit in the middle read %d lines", d)
	}
	t.Logf("a one character edit in the middle read %d lines", x.scanned-before)
	// Edits that change the minimum and the line count.
	snap := st.Snapshot()
	st.MoveCaret(snap.LineStart(70_000), false)
	st.Insert("\n\n")
	st.MoveCaret(st.Snapshot().LineStart(70_001), false)
	st.Insert("   y\n") // indentation 3 inside "a"
	if g := query(3, st.Snapshot().LineCount()-4, 9); g != 3 {
		t.Fatalf("after an indentation 3 line: %d", g)
	}
	st.Undo()
	query(3, st.Snapshot().LineCount()-4, 9)
	// A paste of many lines, a deletion across blocks.
	st.MoveCaret(st.Snapshot().LineStart(1000), false)
	st.Insert(strings.Repeat("        z,\n", 3000))
	query(3, st.Snapshot().LineCount()-4, 9)
	snap = st.Snapshot()
	st.SetSelections([]Selection{{Anchor: snap.LineStart(500), Head: snap.LineStart(9000) + 3}})
	st.Insert("")
	query(3, st.Snapshot().LineCount()-4, 9)
	query(0, st.Snapshot().LineCount()-1, 9)
	for _, r := range [][2]int{{10, 600}, {255, 257}, {4000, 4100}, {100_000, st.Snapshot().LineCount() - 1}} {
		query(r[0], r[1], 100)
	}
}

// TestBracketGuidesIndexDeadline checks the index's deadline: past it,
// a frame reads indentFrameLines lines and reports the minimum undecided;
// the next frames go on where it stopped.
func TestBracketGuidesIndexDeadline(t *testing.T) {
	prev := indentFrameLines
	indentFrameLines = 1000
	t.Cleanup(func() { indentFrameLines = prev })
	st := NewState(jsonGuideDoc(20_000), "a.json", nil)
	x := st.Model.indents(4)
	snap := st.Snapshot()
	n := snap.LineCount()
	past := time.Now().Add(-time.Second)
	frames := 0
	for {
		x.beginFrame()
		frames++
		before := x.scanned
		g, ok := x.minIndent(snap, 4, 2, n-3, 15, past)
		if d := x.scanned - before; d > indentFrameLines+2*indentBlock {
			t.Fatalf("frame %d read %d lines past the deadline", frames, d)
		}
		if ok {
			if g != 2 {
				t.Fatalf("minimum %d, want 2", g)
			}
			break
		}
		if frames > 100 {
			t.Fatal("never decided")
		}
	}
	if frames < 10 {
		t.Fatalf("decided in %d frames: the deadline was not honored", frames)
	}

	// In the view: a pair whose column is undecided is not drawn (no
	// guessed column) until it is, then at its column ("{" at 0,
	// "packages" at 2, "a" at 4 on line 3).
	// (Plain text: no tokenization to wait for, every pair at level 0.)
	past0 := indentPast
	indentPast = func(time.Time) bool { return true }
	t.Cleanup(func() { indentPast = past0 })
	st2, vw, frame := guideViewLang(t, jsonGuideDoc(20_000), "a.txt", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesOn
		o.Guides.BracketPairsHorizontal = BracketGuidesOff
		o.Guides.HighlightActiveBracketPair = false
	})
	_ = st2
	decided, undecided := false, 0
	for k := 0; k < 200 && !decided; k++ {
		frame()
		if len(verticals(vw())["3:0"]) < 3 {
			undecided++
		}
		for key, v := range verticals(vw()) {
			if key == "3:0" {
				// Each pair at its column, or not yet.
				for _, g := range v {
					if !slices.Contains([]string{"0/0", "2/0", "4/0"}, g) {
						t.Fatalf("frame %d: line 3 guides %v", k, v)
					}
				}
				decided = len(v) == 3
			}
		}
	}
	if !decided {
		t.Fatal("the pair's guide was never drawn")
	}
	if undecided == 0 {
		t.Fatal("decided in the first frame: the deadline was not honored")
	}
}

// TestBracketGuidesActiveUnclosed: an unclosed pair innermost around the
// caret makes no pair active (Monaco), not the closed pair around it.
func TestBracketGuidesActiveUnclosed(t *testing.T) {
	st, vw, frame := guideView(t, "{\n  f(\n  x\n}\n", 520, 300, func(o *Options) {
		o.Guides.BracketPairs = BracketGuidesActive
	})
	caretAt(st, 1, 4) // after "("
	frame()
	if r := vw().vs.bgRows; slices.ContainsFunc(r, func(r bgRow) bool { return len(r.g) > 0 }) {
		t.Errorf("guides with the caret in an unclosed pair: %v", r)
	}
	caretAt(st, 1, 1) // before "f": in the closed pair only
	frame()
	eqGuides(t, "closed pair", verticals(vw()), map[string][]string{"1:0": {"0/0*"}, "2:0": {"0/0*"}})
}

// treeMatchesFresh fails t when st's bracket tree, scanned to the end,
// differs from a tree built from scratch on the same text.
func treeMatchesFresh(t *testing.T, st *State, what string) {
	t.Helper()
	tr := st.Model.brackets()
	if tr == nil {
		t.Fatalf("%s: no bracket tree", what)
	}
	snap := st.Snapshot()
	src := modelBrackets{st.Model}
	n := snap.LineCount()
	if len(tr.start) != n || len(tr.minD) != n {
		t.Fatalf("%s: tree of %d lines (%d), text of %d", what, len(tr.start), len(tr.minD), n)
	}
	tr.ensure(src, n, time.Time{})
	fresh := newBracketTree(tr.conf, snap)
	fresh.ensure(src, n, time.Time{})
	for i := range n {
		if !stackEqual(tr.start[i], fresh.start[i]) || tr.minD[i] != fresh.minD[i] {
			t.Fatalf("%s: line %d: depth %d min %d, fresh %d min %d", what, i,
				stackDepth(tr.start[i]), tr.minD[i], stackDepth(fresh.start[i]), fresh.minD[i])
		}
	}
}

// indexMatches fails t when the indentation index did not follow the
// last change (it would start over) or differs from a full count.
func indexMatches(t *testing.T, st *State, tab int, what string) {
	t.Helper()
	snap := st.Snapshot()
	x := st.Model.indents(tab)
	if x.ver != snap.Version() || x.lines != snap.LineCount() {
		t.Fatalf("%s: index at version %d (%d lines), text %d (%d lines)", what, x.ver, x.lines, snap.Version(), snap.LineCount())
	}
	sum := 0
	for _, b := range x.blocks {
		sum += b.n
	}
	if sum != snap.LineCount() {
		t.Fatalf("%s: blocks of %d lines, text of %d", what, sum, snap.LineCount())
	}
	far := time.Now().Add(time.Hour)
	n := snap.LineCount()
	brute := func(a, b, limit int) int {
		g := limit
		for i := a; i <= b; i++ {
			if ind := indentOf(snap.Line(i), tab); ind >= 0 {
				g = min(g, ind)
			}
		}
		return g
	}
	for _, r := range [][2]int{{0, n - 1}, {1, n / 2}, {n / 3, n - 1}, {n / 2, n/2 + 3}} {
		a, b := min(r[0], n-1), min(r[1], n-1)
		g, ok := x.minIndent(snap, tab, a, b, 1000, far)
		if want := brute(a, b, 1000); !ok || g != want {
			t.Fatalf("%s: lines %d..%d: %d (%v), want %d", what, a, b, g, ok, want)
		}
	}
	// Every block's minimum, as read so far.
	s := 0
	for _, b := range x.blocks {
		if b.min != indDirty {
			if want := brute(s, s+b.n-1, noIndent); int(b.min) != want {
				t.Fatalf("%s: block at %d (%d lines): %d, want %d", what, s, b.n, b.min, want)
			}
		}
		s += b.n
	}
}

// TestBracketGuidesMergedEdits: a change whose edits touch the same line
// (a caret, and a selection starting on its line and spanning lines) and
// its undo keep the index and the bracket tree exact.
func TestBracketGuidesMergedEdits(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("{\n")
	for i := range 297 {
		fmt.Fprintf(&sb, "\t\t(item %03d) {x}\n", i)
	}
	sb.WriteString("  }\n")
	st := NewState(sb.String(), "a.txt", nil)
	far := time.Now().Add(time.Hour)
	st.Model.indents(4).minIndent(st.Snapshot(), 4, 0, 1, 100, far) // start the index
	treeMatchesFresh(t, st, "before")
	snap := st.Snapshot()
	st.SetSelections([]Selection{
		{Anchor: snap.LineStart(5), Head: snap.LineStart(5)},
		{Anchor: snap.LineStart(5) + 10, Head: snap.LineStart(7)},
	})
	st.Insert("y")
	if n := st.Snapshot().LineCount(); n != 298 {
		t.Fatalf("%d lines after the change", n)
	}
	indexMatches(t, st, 4, "two carets")
	treeMatchesFresh(t, st, "two carets")
	st.Undo()
	indexMatches(t, st, 4, "undo")
	treeMatchesFresh(t, st, "undo")
	st.Redo()
	indexMatches(t, st, 4, "redo")
	treeMatchesFresh(t, st, "redo")
	// The same with line breaks in the text: the merged region becomes
	// more lines than the line it ends on (the bracket tree kept at least
	// one line per region, which hid the error without them).
	snap = st.Snapshot()
	st.SetSelections([]Selection{
		{Anchor: snap.LineStart(20) + 2, Head: snap.LineStart(20) + 2},
		{Anchor: snap.LineStart(20) + 10, Head: snap.LineStart(23) + 1},
	})
	st.Insert("y\n\tz")
	indexMatches(t, st, 4, "two carets, line breaks")
	treeMatchesFresh(t, st, "two carets, line breaks")
	st.Undo()
	indexMatches(t, st, 4, "undo, line breaks")
	treeMatchesFresh(t, st, "undo, line breaks")
}

// TestBracketGuidesRandomEdits runs random changes of several edits
// (carets and selections, text with LF, CRLF, lone CR and tabs), undo and
// redo, checking after each that the indentation index followed it
// exactly and that the bracket tree equals a fresh one.
func TestBracketGuidesRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 14))
	var sb strings.Builder
	for i := range 150 {
		fmt.Fprintf(&sb, "%s{ (a%d) [\n", strings.Repeat("\t", i%4), i)
		fmt.Fprintf(&sb, "%s  x }\n", strings.Repeat(" ", i%7))
	}
	st := NewState(sb.String(), "a.txt", nil)
	far := time.Now().Add(time.Hour)
	st.Model.indents(4).minIndent(st.Snapshot(), 4, 0, 1, 100, far)
	pieces := []string{"", "a", "\n", "\r\n", "\r", "\t(", "  }", "{\n\t", "x\r\ny", "\n\n\t\t", ")\r]\n  "}
	steps := 3000
	if testing.Short() {
		steps = 500
	}
	for step := range steps {
		snap := st.Snapshot()
		n := snap.Len()
		switch r := rng.IntN(10); {
		case snap.LineCount() < 150:
			// Keep the text a few blocks long: a paste of lines.
			a := rng.IntN(n + 1)
			st.SetSelections([]Selection{{Anchor: a, Head: a}})
			st.Insert(sb.String()[:rng.IntN(sb.Len())])
		case r == 0:
			st.Undo()
		case r == 1:
			st.Redo()
		default:
			k := 1 + rng.IntN(4)
			var sels []Selection
			for range k {
				a := rng.IntN(n + 1)
				b := a
				if rng.IntN(2) == 0 {
					b = min(n, a+rng.IntN(80))
				}
				if snap.LineCount() > 500 {
					b = min(n, a+rng.IntN(400))
				}
				sels = append(sels, Selection{Anchor: a, Head: b})
			}
			st.SetSelections(sels)
			text := pieces[rng.IntN(len(pieces))] + pieces[rng.IntN(len(pieces))]
			st.Insert(text)
		}
		what := fmt.Sprintf("step %d", step)
		indexMatches(t, st, 4, what)
		treeMatchesFresh(t, st, what)
	}
}

// TestBracketGuidesIndexTwoTabSizes: two views of one Model with other
// tab sizes keep an index each, so an edit still reads a block, not the
// pair, in both.
func TestBracketGuidesIndexTwoTabSizes(t *testing.T) {
	goText(t)
	st1 := NewState(jsonGuideDoc(20_000), "a.txt", theme.Base("vs"))
	st2 := NewView(st1.Model, theme.Base("vs"))
	for _, st := range []*State{st1, st2} {
		st.Options.Guides.BracketPairs = BracketGuidesOn
	}
	d1, vw1 := viewDriver(st1, 520, 300, func(w *ViewW) { w.TabSize(2) })
	d2, _ := viewDriver(st2, 520, 300, func(w *ViewW) { w.TabSize(4) })
	drawn := func() bool {
		return slices.ContainsFunc((*vw1).vs.bgRows, func(r bgRow) bool { return len(r.g) >= 3 })
	}
	// At least 5 frames; more while the guides wait for the bracket and
	// indentation work of a slow build (-race, a loaded machine).
	for i := 0; i < 5 || (i < 50 && !drawn()); i++ {
		d1.Frame()
		d2.Frame()
	}
	scanned := func() int { return int(indentLinesRead.Load()) }
	if !drawn() {
		t.Fatalf("guides not drawn: %v", (*vw1).vs.bgRows)
	}
	before := scanned()
	for i := range 3 {
		st1.MoveCaret(st1.Snapshot().LineStart(5000+i)+8, false)
		st1.Insert("x")
		d1.Frame()
		d2.Frame()
	}
	if d := scanned() - before; d > 3*2*4*indentBlock {
		t.Fatalf("3 edits with two tab sizes read %d lines (pair of 20000)", d)
	}
}

// TestBracketGuidesIndexLongLines: the index reads a line's first
// indentPrefix bytes only, and its per-frame floor counts bytes.
func TestBracketGuidesIndexLongLines(t *testing.T) {
	long := strings.Repeat("x", 4<<20)
	doc := "{\n  a,\n    " + long + "\n  b\n" + strings.Repeat(" ", 5000) + "c\n}\n"
	st := NewState(doc, "a.txt", nil)
	x := st.Model.indents(4)
	snap := st.Snapshot()
	far := time.Now().Add(time.Hour)
	g, ok := x.minIndent(snap, 4, 1, 4, 100, far)
	if !ok || g != 2 {
		t.Fatalf("minimum %d (%v), want 2", g, ok)
	}
	if x.bytes > 6*indentPrefix { // the block, then the part asked for
		t.Fatalf("read %d bytes of a %d byte text", x.bytes, snap.Len())
	}
	// A line of more blanks than the prefix: the prefix's columns.
	if g, _ := x.minIndent(snap, 4, 4, 4, 1<<20, far); g != indentPrefix {
		t.Fatalf("line of 5000 blanks then text: %d, want %d", g, indentPrefix)
	}
	// Typing in the long line reads its prefix again, not the line.
	before := x.bytes
	st.MoveCaret(snap.LineStart(2)+1000, false)
	st.Insert("y")
	if g, ok := x.minIndent(st.Snapshot(), 4, 1, 4, 100, far); !ok || g != 2 {
		t.Fatalf("after typing: %d (%v)", g, ok)
	}
	if d := x.bytes - before; d > 6*indentPrefix {
		t.Fatalf("typing in a 4 MB line read %d bytes", d)
	}

	// Past the deadline a frame reads about indentFrameBytes.
	var sb strings.Builder
	sb.WriteString("{\n")
	for range 1000 {
		sb.WriteString("  " + strings.Repeat("z", 5000) + "\n")
	}
	sb.WriteString("}\n")
	st2 := NewState(sb.String(), "a.txt", nil)
	x2 := st2.Model.indents(4)
	past := time.Now().Add(-time.Second)
	frames := 0
	for {
		x2.beginFrame()
		frames++
		_, ok := x2.minIndent(st2.Snapshot(), 4, 1, 1001, 100, past)
		if x2.frameBytes > indentFrameBytes+2*indentBlock*indentPrefix {
			t.Fatalf("frame %d read %d bytes past the deadline", frames, x2.frameBytes)
		}
		if ok {
			break
		}
		if frames > 100 {
			t.Fatal("never decided")
		}
	}
	if frames < 3 {
		t.Fatalf("1000 lines of 4 KB read in %d frames past the deadline", frames)
	}
}
