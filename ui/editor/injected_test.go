package editor

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

// injHost shows doc (CSS, color decorators on by default) focused.
func injHost(t *testing.T, doc string, size geom.Size) (*State, *hintHost) {
	t.Helper()
	goText(t)
	st := NewState(doc, "a.css", theme.Base("vs"))
	h := newHintHost(t, st, size, 1)
	h.focusAt(0)
	return st, h
}

// gapsOf returns the content x ranges of the injected gaps of lv.
func gapsOf(lv *lineView) (out [][2]float32, cols []int) {
	lv.eachGap(func(in injection, x0, x1 float32) {
		out = append(out, [2]float32{x0, x1})
		cols = append(cols, in.col)
	})
	return out, cols
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// TestColorInjectedLayout: a color literal gets a real gap before it, as
// Monaco's `before` injected text: "#" and everything after it move right
// by colorSwatchW (round(1.2em): the 0.8em box and its 0.2em margins), the
// text before it does not move, and the swatch is drawn inside the gap.
func TestColorInjectedLayout(t *testing.T) {
	const doc = "a #ff0000 b\n"
	st, h := injHost(t, doc, geom.Size{W: 480, H: 200})
	snap := st.Snapshot()
	em := h.vw.opts.fontSize()
	sw := colorSwatchW(em)
	if want := float32(math.Round(1.2 * float64(em))); sw != want {
		t.Fatalf("colorSwatchW(%v) = %v, want %v", em, sw, want)
	}
	if got := h.vw.lineInjections(0, snap.Line(0), nil); len(got) != 1 || got[0] != (injection{col: 2, w: sw}) {
		t.Fatalf("lineInjections %+v, want one at column 2 of width %v", got, sw)
	}
	on := h.vw.rowAt(snap, 0, 0)
	vs := h.vw.vs
	if len(vs.swatches) != 1 {
		t.Fatalf("swatches %v", vs.swatches)
	}
	box := vs.swatches[0].r
	st.Options.ColorDecorators = false
	h.frame()
	off := h.vw.rowAt(snap, 0, 0)
	if len(off.segs) != 1 || len(off.inj) != 0 {
		t.Fatalf("colors off: %d segs, injections %v", len(off.segs), off.inj)
	}
	for col := 0; col <= len(snap.Line(0)); col++ {
		xo, _ := off.caretX(off.runeAt(col))
		xl, _ := on.caretXSide(on.runeAt(col), false)
		xr, _ := on.caretXSide(on.runeAt(col), true)
		wl, wr := float32(0), float32(0)
		switch {
		case col == 2:
			wr = sw
		case col > 2:
			wl, wr = sw, sw
		}
		if !near(xl-xo, wl) || !near(xr-xo, wr) {
			t.Errorf("column %d: x %v / %v with colors, %v without (shift want %v / %v)", col, xl, xr, xo, wl, wr)
		}
	}
	if !near(on.endX-off.endX, sw) {
		t.Errorf("row end %v, want %v + %v", on.endX, off.endX, sw)
	}
	gaps, _ := gapsOf(&on)
	if len(gaps) != 1 || !near(gaps[0][1]-gaps[0][0], sw) {
		t.Fatalf("gaps %v", gaps)
	}
	tx := vs.text.X - st.Viewport.ScrollX
	if box.X < tx+gaps[0][0] || box.Right() > tx+gaps[0][1] {
		t.Errorf("swatch %v outside its gap [%v, %v]", box, tx+gaps[0][0], tx+gaps[0][1])
	}
	if want := float32(math.Round(0.8 * float64(em))); box.W != want || box.H != want {
		t.Errorf("swatch %vx%v, want %v square", box.W, box.H, want)
	}
}

// TestColorInjectedCaretStops: the caret never stops inside the gap. As
// Monaco's injected text with the default cursor stops (both sides),
// the literal's start is one caret position drawn on either side of the
// gap: Right arrives on its left side and leaves past "#" with one press;
// Left arrives on its right side; Down arrives on its right side, Up on
// its left side; a click on the gap's left half lands on the left side,
// on its right half on the right side (both at the literal's start).
func TestColorInjectedCaretStops(t *testing.T) {
	const doc = "a #ff0000 b\nab\n  #00ff00\n  #0000ff\n"
	st, h := injHost(t, doc, geom.Size{W: 480, H: 200})
	snap := st.Snapshot()
	st.MoveCaret(0, false)
	h.frame()
	lv := h.vw.rowAt(snap, 0, 0)
	gaps, _ := gapsOf(&lv)
	if len(gaps) != 1 {
		t.Fatalf("gaps %v", gaps)
	}
	check := func(what string, off int, right bool) {
		t.Helper()
		h.frame()
		if got := st.Primary().Head; got != off || st.caretRight() != right {
			t.Fatalf("%s: caret %d (right of the gap %v), want %d (%v)", what, got, st.caretRight(), off, right)
		}
		x, _ := h.vw.contentX(st.Snapshot(), off)
		l := st.Snapshot().LineOf(off)
		row := h.vw.rowAt(st.Snapshot(), l, 0)
		want, _ := row.caretXSide(row.runeAt(off-st.Snapshot().LineStart(l)), right)
		if !near(x, want) {
			t.Fatalf("%s: caret x %v, want %v", what, x, want)
		}
	}
	key := func(k input.Key) { h.key(k, 0) }
	key(input.KeyRight)
	check("Right", 1, false)
	key(input.KeyRight)
	check("Right to the literal", 2, false)
	if x, _ := h.vw.contentX(st.Snapshot(), 2); !near(x, gaps[0][0]) {
		t.Fatalf("caret before the gap at %v, gap starts at %v", x, gaps[0][0])
	}
	key(input.KeyRight)
	check("Right past the gap and #", 3, false)
	key(input.KeyLeft)
	check("Left back", 2, true)
	if x, _ := h.vw.contentX(st.Snapshot(), 2); !near(x, gaps[0][1]) {
		t.Fatalf("caret after the gap at %v, gap ends at %v", x, gaps[0][1])
	}
	key(input.KeyLeft)
	check("Left past the gap", 1, false)
	key(input.KeyRight)
	check("Right again", 2, false)
	key(input.KeyEnd)
	check("End", 11, false)
	key(input.KeyHome)
	check("Home", 0, false)

	// Vertical moves: Down arrives right of the gap, Up left of it.
	l1 := snap.LineStart(1)
	st.MoveCaret(l1+2, false)
	key(input.KeyDown)
	check("Down onto the literal", snap.LineStart(2)+2, true)
	st.MoveCaret(snap.LineStart(3)+2, false)
	key(input.KeyUp)
	check("Up onto the literal", snap.LineStart(2)+2, false)

	// Clicks on the gap's halves (its margins: the box takes the press).
	vs := h.vw.vs
	st.MoveCaret(0, false)
	h.frame()
	tx := vs.text.X - st.Viewport.ScrollX
	y := vs.text.Y - st.Viewport.OffsetY + st.Viewport.LineHeight/2
	for _, c := range []struct {
		x     float32
		right bool
	}{{gaps[0][0] + 1, false}, {gaps[0][1] - 1, true}} {
		p := geom.Pt(tx+c.x, y)
		for _, sw := range vs.swatches {
			if sw.r.Contains(p) {
				t.Fatalf("test point %v on the swatch %v", p, sw.r)
			}
		}
		h.now = h.now.Add(time.Second) // not a double click
		h.click(p, 0)
		check("click on the gap", 2, c.right)
		if vs.picker != nil {
			t.Fatal("a click on the gap's margin opened the picker")
		}
	}
}

// TestColorInjectedWrap: wrapping counts the gap at its width in pixels
// (a fraction of a column), rows never exceed the wrapping column, and the
// measured rows equal a full recompute, also after an edit on the line.
func TestColorInjectedWrap(t *testing.T) {
	cfg := wrapConfig{col: 10, tab: 4, indent: WrapIndentNone}
	if lb := computeBreaks("aaaa #fff bbb", cfg); len(lb.starts) != 1 || lb.starts[0] != 10 {
		t.Fatalf("without the gap: breaks %v, want [10]", lb.starts)
	}
	cfg.inj = injectCfg{w: 17, colW: 8.5}
	if lb := computeBreaks("aaaa #fff bbb", cfg); len(lb.starts) != 1 || lb.starts[0] != 5 {
		t.Fatalf("with a two column gap: breaks %v, want [5]", lb.starts)
	}
	// A tab after the gap keeps its stop on the model column grid.
	if lb := computeBreaks("#ff\tx yyyy", wrapConfig{col: 8, tab: 4, indent: WrapIndentNone, inj: injectCfg{w: 17, colW: 8.5}}); len(lb.starts) != 1 || lb.starts[0] != 6 {
		t.Fatalf("tab after the gap: breaks %v, want [6]", lb.starts)
	}
	// Fractional gaps: two gaps of 1.5 columns fill 12 columns with the
	// 9 characters exactly (whole columns would need 13), one more column
	// of text breaks before the second literal.
	half := injectCfg{w: 12, colW: 8}
	if lb := computeBreaks("#fff #fff", wrapConfig{col: 12, tab: 4, inj: half}); len(lb.starts) != 0 {
		t.Fatalf("fractional gaps filling the row: breaks %v, want none", lb.starts)
	}
	if lb := computeBreaks("#fff #fff", wrapConfig{col: 11, tab: 4, inj: half}); len(lb.starts) != 1 || lb.starts[0] != 5 {
		t.Fatalf("fractional gaps one column over: breaks %v, want [5]", lb.starts)
	}

	line := "  color: #ff0000; background: rgb(0, 128, 0); border: 1px solid #00f; x: hsl(120, 100%, 25%);"
	doc := line + "\n" + line + "\n"
	goText(t)
	st := NewState(doc, "a.css", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	h := newHintHost(t, st, geom.Size{W: 260, H: 300}, 1)
	h.focusAt(0)
	vl := &st.vl
	colW := h.vw.vs.colW
	sw := colorSwatchW(h.vw.opts.fontSize())
	if vl.cfg.inj != (injectCfg{w: sw, colW: colW}) {
		t.Fatalf("wrap gap %+v, want %v px on %v px columns", vl.cfg.inj, sw, colW)
	}
	plain := vl.cfg
	plain.inj = injectCfg{}
	check := func(what string) {
		t.Helper()
		snap := st.Snapshot()
		for l := range 2 {
			lb := vl.breaks[l]
			if lb == nil {
				t.Fatalf("%s: line %d not measured", what, l)
			}
			full := computeBreaks(snap.Line(l), vl.cfg)
			if !equalInts(lb.starts, full.starts) || int(vl.rows[l]) != full.rows() {
				t.Fatalf("%s: line %d breaks %v (%d rows), full recompute %v", what, l, lb.starts, vl.rows[l], full.starts)
			}
			if equalInts(full.starts, computeBreaks(snap.Line(l), plain).starts) {
				t.Fatalf("%s: line %d: the gaps did not change the breaks %v", what, l, full.starts)
			}
		}
		maxX := float32(vl.cfg.col) * colW
		for r := 0; r < vl.Count(); r++ {
			line, row := vl.find(r)
			lv := h.vw.rowAt(snap, line, row)
			if lv.endX > maxX+0.5 {
				t.Fatalf("%s: row %d ends at %v past the wrapping column's %v", what, r, lv.endX, maxX)
			}
		}
	}
	check("open")
	st.MoveCaret(4, false)
	h.typeText("zz")
	h.frame()
	check("after typing")
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestColorInjectedSelectionRect: a selection of the literal covers its
// characters, not the gap (Monaco converts a range's start with Right and
// its end with Left affinity); one across the gap covers it too.
func TestColorInjectedSelectionRect(t *testing.T) {
	const doc = "a #ff0000 b\n"
	st, h := injHost(t, doc, geom.Size{W: 480, H: 200})
	lv := h.vw.rowAt(st.Snapshot(), 0, 0)
	gaps, _ := gapsOf(&lv)
	if len(gaps) != 1 {
		t.Fatalf("gaps %v", gaps)
	}
	g0, g1 := gaps[0][0], gaps[0][1]
	span := func(a, b int) (lo, hi float32, covered float32) {
		lo, hi = float32(math.MaxFloat32), 0
		lv.selRects(a, b, -1, func(x0, x1 float32) {
			lo, hi = min(lo, x0), max(hi, x1)
			covered += max(0, min(x1, g1)-max(x0, g0))
		})
		return
	}
	lo, hi, cov := span(2, 9)
	x9, _ := lv.caretX(lv.runeAt(9))
	if !near(lo, g1) || !near(hi, x9) || cov > 0.01 {
		t.Fatalf("literal selection [%v, %v] covering %v of the gap [%v, %v]", lo, hi, cov, g0, g1)
	}
	if _, _, cov := span(0, 2); cov > 0.01 {
		t.Fatalf("selection up to the literal covers %v of the gap", cov)
	}
	lo, hi, cov = span(1, 10)
	x1, _ := lv.caretX(lv.runeAt(1))
	x10, _ := lv.caretX(lv.runeAt(10))
	if !near(lo, x1) || !near(hi, x10) || !near(cov, g1-g0) {
		t.Fatalf("selection across the gap [%v, %v] covering %v, want [%v, %v] covering %v", lo, hi, cov, x1, x10, g1-g0)
	}
	// Painted: the strict redraw check holds with the gap selected.
	st.SetSelections([]Selection{{Anchor: 1, Head: 10}})
	h.frame()
	st.SetSelections([]Selection{{Anchor: 2, Head: 9}})
	h.frame()
}

// TestColorInjectedLargeFile: a model over the worker sync limit has no
// color decorators, so no gaps: its layout and wrapping are those of
// colors off.
func TestColorInjectedLargeFile(t *testing.T) {
	const doc = "a #ff0000 b\n"
	goText(t)
	st := NewState(doc, "a.css", theme.Base("vs"))
	st.Model.syncLarge = true
	h := newHintHost(t, st, geom.Size{W: 480, H: 200}, 1)
	h.frame()
	snap := st.Snapshot()
	lv := h.vw.rowAt(snap, 0, 0)
	if len(lv.segs) != 1 || len(lv.inj) != 0 || len(h.vw.vs.swatches) != 0 || st.vl.cfg.inj != (injectCfg{}) {
		t.Fatalf("large file: %d segs, injections %v, swatches %v, wrap gaps %+v", len(lv.segs), lv.inj, h.vw.vs.swatches, st.vl.cfg.inj)
	}
	if got := h.vw.lineInjections(0, snap.Line(0), nil); len(got) != 0 {
		t.Fatalf("lineInjections %v", got)
	}
	st.Options.ColorDecorators = false
	h.frame()
	off := h.vw.rowAt(snap, 0, 0)
	for col := 0; col <= len(snap.Line(0)); col++ {
		a, _ := lv.caretX(lv.runeAt(col))
		b, _ := off.caretX(off.runeAt(col))
		if a != b {
			t.Fatalf("column %d: x %v, %v with colors off", col, a, b)
		}
	}
}

// goldenColorSource is the document of the color golden images.
const goldenColorSource = "a {\n" +
	"  color: #ff0000;\n" +
	"  background: rgba(0, 0, 255, 0.5);\n" +
	"  border: 1px solid hsl(120, 100%, 25%);\n" +
	"}\n" +
	"#fff at the line start, \"#0f08\" in quotes\n"

// TestColorInjectedGolden: the swatches in their gaps (Monaco's
// .colorpicker-color-decoration: a 0.8em box with a 0.1em border, black or
// #eee on dark themes, 0.2em margins), a translucent one over the
// background, a selection across a gap (covered), one of a literal's "#"
// (not covered) and its caret right of the gap after Shift+Left.
func TestColorInjectedGolden(t *testing.T) {
	goText(t)
	if want, err := os.ReadFile(filepath.Join("testdata", "golden_fonts.txt")); err != nil || string(want) != strings.Join(text.Families(), "\n") {
		t.Skip("golden images were recorded with different fonts")
	}
	for _, name := range []string{"vs", "vs-dark"} {
		t.Run(name, func(t *testing.T) {
			st := NewState(goldenColorSource, "demo.css", theme.Base(name))
			d, _ := viewDriver(st, 520, 200, nil)
			d.Frame()
			d.Click(geom.Pt(400, 190)) // focus
			snap := st.Snapshot()
			a := snap.LineStart(3) + strings.Index(snap.Line(3), "solid")
			st.SetSelections([]Selection{
				{Anchor: a, Head: a + len("solid hsl")},
				{Anchor: snap.LineStart(1) + 10, Head: snap.LineStart(1) + 10},
			})
			d.Frame()
			// Shift+Left: the selection still crosses its gap; the caret
			// selects "#" and is drawn right of its gap.
			d.Key(input.KeyLeft, input.ModShift)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+"_colors.png")
		})
	}
}

// TestColorInjectedSelectionToRightSide: a selection whose head is drawn
// right of a gap (Shift+Left onto the literal) covers that gap, up to the
// caret (Monaco draws the cursor's view selection).
func TestColorInjectedSelectionToRightSide(t *testing.T) {
	const doc = "a #ff0000 b\n"
	st, h := injHost(t, doc, geom.Size{W: 480, H: 200})
	st.MoveCaret(1, false)
	h.frame()
	for _, k := range []input.Key{input.KeyRight, input.KeyRight, input.KeyLeft} {
		h.key(k, input.ModShift)
		h.frame()
	}
	if sel := st.Primary(); sel.Anchor != 1 || sel.Head != 2 || !st.caretRight() {
		t.Fatalf("selection %+v, right of the gap %v; want [1, 2) with the caret right of it", sel, st.caretRight())
	}
	lv := h.vw.rowAt(st.Snapshot(), 0, 0)
	gaps, _ := gapsOf(&lv)
	hi, cov := float32(0), float32(0)
	lv.selRects(1, 2, 2, func(x0, x1 float32) {
		hi = max(hi, x1)
		cov += max(0, min(x1, gaps[0][1])-max(x0, gaps[0][0]))
	})
	if !near(hi, gaps[0][1]) || !near(cov, gaps[0][1]-gaps[0][0]) {
		t.Fatalf("selection to the right side ends at %v covering %v of the gap %v", hi, cov, gaps[0])
	}
	if x, _ := h.vw.contentX(st.Snapshot(), 2); !near(x, hi) {
		t.Fatalf("caret at %v, selection ends at %v", x, hi)
	}
	// Without the head there (Shift+Left again, then back right: the left
	// side), the gap is not covered.
	cov = 0
	lv.selRects(1, 2, -1, func(x0, x1 float32) { cov += max(0, min(x1, gaps[0][1])-max(x0, gaps[0][0])) })
	if cov > 0.01 {
		t.Fatalf("selection ending left of the gap covers %v of it", cov)
	}
}

// TestColorInjectedRightsNoAlloc: recording the carets' gap sides after a
// Left or vertical move allocates nothing when no caret is at a gap.
func TestColorInjectedRightsNoAlloc(t *testing.T) {
	st := NewState("let plain = fetch(x).then(h => h.ok)\nnext\n", "a.ts", theme.Base("vs"))
	st.vl.cfg.inj = injectCfg{w: 17, colW: 8}
	st.MoveCaret(5, false)
	out, right := slices.Clone(st.Sels), []bool{true}
	if n := testing.AllocsPerRun(100, func() { st.setRights(out, right) }); n != 0 {
		t.Fatalf("setRights on a line without colors: %v allocations", n)
	}
}

// TestColorInjectedRightsKept: adding or removing another cursor keeps the
// primary caret's side of its gap (Monaco keeps the surviving cursors'
// view state).
func TestColorInjectedRightsKept(t *testing.T) {
	const doc = "a #ff0000 b\nsecond line\n"
	st, h := injHost(t, doc, geom.Size{W: 480, H: 200})
	st.MoveCaret(3, false)
	h.key(input.KeyLeft, 0)
	h.frame()
	if st.Primary().Head != 2 || !st.caretRight() {
		t.Fatalf("caret %d right %v, want 2 right of the gap", st.Primary().Head, st.caretRight())
	}
	other := st.Snapshot().LineStart(1) + 3
	st.AddCursor(other)
	h.frame()
	if len(st.Sels) != 2 || st.Primary().Head != 2 || !st.caretRight() {
		t.Fatalf("after adding a cursor: %+v, primary right of the gap %v", st.Sels, st.caretRight())
	}
	st.AddCursor(other) // on a caret: removes it
	h.frame()
	if len(st.Sels) != 1 || !st.caretRight() {
		t.Fatalf("after removing the cursor: %+v, right of the gap %v", st.Sels, st.caretRight())
	}
	h.key(input.KeyRight, 0)
	h.frame()
	if st.caretRight() {
		t.Fatal("Right kept the side")
	}
}

// BenchmarkComputeBreaksPlain wraps a typical code line without color
// literals with the gaps off ("off": the code before injections) and on.
func BenchmarkComputeBreaksPlain(b *testing.B) {
	line := "  return await fetch(`/api/items/42`, { method: \"POST\", body: JSON.stringify({ id: 42 }) }); // then handle it"
	for _, c := range []struct {
		name string
		inj  injectCfg
	}{{"off", injectCfg{}}, {"on", injectCfg{w: 17, colW: 8.4}}} {
		cfg := wrapConfig{col: 40, tab: 4, indent: WrapIndentSame, inj: c.inj}
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				computeBreaks(line, cfg)
			}
		})
	}
}

// TestColorInjectedRightsMoves: a move decides every caret's side: one
// that ends where a caret was drawn right of a gap does not inherit that
// side (Home, Up with several cursors, Right onto the old position).
func TestColorInjectedRightsMoves(t *testing.T) {
	t.Run("home", func(t *testing.T) {
		st, h := injHost(t, "#ff0000 b\n", geom.Size{W: 480, H: 200})
		st.MoveCaret(1, false)
		h.key(input.KeyLeft, 0)
		h.frame()
		if st.Primary().Head != 0 || !st.caretRight() {
			t.Fatalf("Left: caret %d right %v, want 0 right of the gap", st.Primary().Head, st.caretRight())
		}
		h.key(input.KeyHome, 0)
		h.frame()
		if st.Primary().Head != 0 || st.caretRight() {
			t.Fatalf("Home: caret %d right %v, want 0 left of the gap", st.Primary().Head, st.caretRight())
		}
	})
	t.Run("up", func(t *testing.T) {
		st, h := injHost(t, "#ff0000 b\nsecond line\nthird line\n", geom.Size{W: 480, H: 200})
		st.MoveCaret(1, false)
		h.key(input.KeyLeft, 0)
		h.frame()
		st.AddCursor(st.Snapshot().LineStart(1))
		h.frame()
		if len(st.Sels) != 2 || !st.caretRight() {
			t.Fatalf("setup: %+v right %v", st.Sels, st.caretRight())
		}
		h.key(input.KeyUp, 0) // the primary stays at the document start
		h.frame()
		if st.Primary().Head != 0 || st.caretRight() {
			t.Fatalf("Up: %+v, primary right of the gap %v; want left", st.Sels, st.caretRight())
		}
	})
	t.Run("restore", func(t *testing.T) {
		st, h := injHost(t, "#ff0000 b\n", geom.Size{W: 480, H: 200})
		st.MoveCaret(1, false)
		h.key(input.KeyLeft, 0)
		h.frame()
		if st.Primary().Head != 0 || !st.caretRight() {
			t.Fatalf("setup: caret %d right %v", st.Primary().Head, st.caretRight())
		}
		st.RestoreViewState(st.SaveViewState()) // the same selection, a new caret
		h.frame()
		if st.Primary().Head != 0 || st.caretRight() {
			t.Fatalf("RestoreViewState: caret %d right %v, want 0 left of the gap", st.Primary().Head, st.caretRight())
		}
	})
	t.Run("right", func(t *testing.T) {
		st, h := injHost(t, "a #ff0000 b\n", geom.Size{W: 480, H: 200})
		st.MoveCaret(3, false)
		h.key(input.KeyLeft, 0)
		h.frame()
		st.AddCursor(1)
		h.frame()
		if len(st.Sels) != 2 || !st.caretRight() {
			t.Fatalf("setup: %+v right %v", st.Sels, st.caretRight())
		}
		h.key(input.KeyRight, 0)
		h.frame()
		for k, sel := range st.Sels {
			if sel.Head == 2 && st.headRight(k) {
				t.Fatalf("Right: the caret arriving at the literal (%+v) inherited the right side", st.Sels)
			}
		}
	})
}
