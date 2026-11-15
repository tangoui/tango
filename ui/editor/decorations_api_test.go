package editor

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// fillsOf returns the FillRect rectangles of color c in ops.
func fillsOf(ops []paint.Op, c color.NRGBA) []geom.Rect {
	var out []geom.Rect
	for _, op := range ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == c {
			out = append(out, f.Rect)
		}
	}
	return out
}

// textsOf returns the Text ops of color c in ops.
func textsOf(ops []paint.Op, c color.NRGBA) []paint.Text {
	var out []paint.Text
	for _, op := range ops {
		if t, ok := op.(paint.Text); ok && t.Color == c {
			out = append(out, t)
		}
	}
	return out
}

var (
	decoRed    = color.NRGBA{0xff, 0x00, 0x00, 0xff}
	decoGreen  = color.NRGBA{0x00, 0xc0, 0x00, 0xff}
	decoBlue   = color.NRGBA{0x00, 0x00, 0xff, 0xff}
	decoPurple = color.NRGBA{0x80, 0x00, 0x80, 0xff}
	decoOrange = color.NRGBA{0xff, 0x80, 0x00, 0xff}
	decoTeal   = color.NRGBA{0x00, 0x80, 0x80, 0xff}
)

// TestDecorationsAPI: Model decorations move with the edits of every view
// and are seen by all; a view's decorations are its own (Monaco's ownerId),
// go away with Close, and DecorationsIn leaves them out.
func TestDecorationsAPI(t *testing.T) {
	m := NewModel("a.txt", "hello world\nsecond line\n", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	o := &DecorationOptions{Background: decoRed}
	snap := m.Snapshot()
	m.SetDecorations("lint", []Decoration{{Start: 6, End: 11, Options: o}, LineDecoration(snap, 1, 1, o)})
	a.SetDecorations("diff", []Decoration{{Start: 0, End: 5, Options: o}})

	if got := m.Decorations("lint"); len(got) != 2 || got[0] != (Decoration{6, 11, o}) || got[1] != (Decoration{12, 23, o}) {
		t.Fatalf("Decorations = %v", got)
	}
	// An edit through b before every range moves all of them.
	b.MoveCaret(0, false)
	b.Insert(">> ")
	if got := m.Decorations("lint"); got[0] != (Decoration{9, 14, o}) || got[1] != (Decoration{15, 26, o}) {
		t.Fatalf("after insert: %v", got)
	}
	// Typed at the start of an AlwaysGrows range: inside it.
	if got := a.Decorations("diff"); len(got) != 1 || got[0] != (Decoration{0, 8, o}) {
		t.Fatalf("view decorations after insert: %v", got)
	}
	if got := b.Decorations("diff"); got != nil {
		t.Fatalf("b sees a's decorations: %v", got)
	}
	if got := m.DecorationsIn(0, m.Snapshot().Len(), nil); len(got) != 2 {
		t.Fatalf("DecorationsIn = %v, want the 2 Model decorations", got)
	}
	// Deleting a range's text collapses it.
	a.SetSelections([]Selection{{Anchor: 9, Head: 14}})
	a.Delete(Backward, Char)
	if got := m.Decorations("lint")[0]; got.Start != 9 || got.End != 9 {
		t.Fatalf("deleted range: %v", got)
	}
	a.Close()
	if got := a.Decorations("diff"); got != nil {
		t.Fatalf("after Close: %v", got)
	}
	if m.decos.len() != 2 {
		t.Fatalf("store holds %d decorations, want 2", m.decos.len())
	}
	m.SetDecorations("lint", nil)
	if m.hasDecorations() {
		t.Fatal("decorations left after removing the owner")
	}
	// LSP ranges: UTF-16 columns.
	snap = buffer.New("aé😀b\n").Snapshot()
	d := DecorationFromLSP(snap, 0, 1, 0, 4, o) // é (1 unit) and 😀 (2 units)
	if d.Start != 1 || d.End != 7 {
		t.Fatalf("DecorationFromLSP = [%d, %d), want [1, 7)", d.Start, d.End)
	}
}

// TestDecorationsUndoRedo: undo and redo are edits like any other, so the
// decorations move through them as anchors do (Monaco: decorations are not
// part of the undo history). Checked against buffer anchors over random
// edits, undos and redos away from the ranges' ends; a deleted range's
// text coming back by undo does not grow it back (as Monaco).
func TestDecorationsUndoRedo(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 9))
	text := strings.Repeat("abcdefghij\n", 40)
	m := NewModel("a.txt", text, nil)
	s := NewView(m, nil)
	buf := m.Buffer()
	type ref struct{ s, e buffer.Anchor }
	var ds []Decoration
	var refs []ref
	o := &DecorationOptions{Background: decoRed, Stickiness: NeverGrowsWhenTypingAtEdges}
	for i := range 20 {
		a := 22*i + 3
		ds = append(ds, Decoration{Start: a, End: a + 4, Options: o})
		refs = append(refs, ref{buf.Anchor(a, buffer.Right), buf.Anchor(a+4, buffer.Left)})
	}
	m.SetDecorations("x", ds)
	check := func(step string) {
		t.Helper()
		got := m.Decorations("x")
		for i, rf := range refs {
			ws, we := buf.Resolve(rf.s), buf.Resolve(rf.e)
			we = max(ws, we)
			if got[i].Start != ws || got[i].End != we {
				t.Fatalf("%s: decoration %d = [%d, %d), anchors [%d, %d)", step, i, got[i].Start, got[i].End, ws, we)
			}
		}
	}
	for step := range 300 {
		snap := m.Snapshot()
		switch k := r.IntN(10); {
		case k < 5:
			// Insert or delete at the start of a line (never at a range
			// end: anchors and batches agree there).
			l := r.IntN(snap.LineCount() - 1)
			at := snap.LineStart(l)
			s.SetSelections([]Selection{{Anchor: at, Head: at}})
			if r.IntN(2) == 0 {
				s.Insert("xy\n")
			} else if at+1 < snap.Len() {
				s.Delete(Forward, Char)
			}
		case k < 8:
			s.Undo()
		default:
			s.Redo()
		}
		check(fmt.Sprintf("step %d", step))
	}
	// Undo after deleting a decorated range: it stays collapsed.
	got := m.Decorations("x")[5]
	s.SetSelections([]Selection{{Anchor: got.Start - 1, Head: got.End + 1}})
	s.Delete(Backward, Char)
	s.Undo()
	if d := m.Decorations("x")[5]; d.Start != d.End {
		t.Fatalf("undo restored a deleted range: %v", d)
	}
}

// decoHost shows st in a strict verify host focused on line 0.
func decoHost(t *testing.T, st *State, size geom.Size) *hintHost {
	t.Helper()
	goText(t)
	h := newHintHost(t, st, size, 1)
	h.frame()
	h.frame()
	return h
}

// TestDecorationsPaintLayers: a whole-line background spans the text area
// of each of its lines, a range background its characters (above the
// selection), inline colors recolor the glyphs, and the gutter shows the
// margin background, the mark's bar and sign and the line number color;
// the overview ruler marks the lines. A view's decorations are not drawn
// in another view of the Model.
func TestDecorationsPaintLayers(t *testing.T) {
	st := NewState("alpha beta gamma\nsecond line here\nthird\nfourth\n", "a.txt", theme.Base("vs"))
	h := decoHost(t, st, geom.Size{W: 480, H: 240})
	snap := st.Snapshot()
	whole := &DecorationOptions{WholeLine: true, Background: decoRed, MarginBackground: decoGreen,
		Gutter: &GutterMark{Bar: decoBlue, Sign: "+", SignColor: decoPurple}, LineNumberColor: decoOrange,
		Overview: &OverviewStyle{Color: decoTeal, Lane: 4}}
	rng := &DecorationOptions{Background: decoBlue, Inline: &InlineStyle{Color: decoPurple, Strikethrough: true}, ZIndex: 1}
	st.SetDecorations("t", []Decoration{
		LineDecoration(snap, 1, 2, whole),
		{Start: 6, End: 10, Options: rng}, // "beta"
	})
	st.SetSelections([]Selection{{Anchor: 0, Head: 8}})
	h.frame()
	ops := h.r.List().Ops
	g := h.vw.vs
	lh := st.Viewport.LineHeight

	rows := fillsOf(ops, decoRed)
	if len(rows) != 2 || rows[0].W != g.text.W || rows[0].X != g.text.X || rows[1].Y-rows[0].Y != lh {
		t.Fatalf("whole-line fills %v, want 2 rows across the text area (%v)", rows, g.text)
	}
	boxes := fillsOf(ops, decoBlue)
	var rangeBox geom.Rect
	for _, b := range boxes {
		if b.X >= g.text.X {
			rangeBox = b
		}
	}
	if w := 4 * g.colW; rangeBox.Empty() || absf(rangeBox.W-w) > 0.5 || absf(rangeBox.X-(g.text.X+6*g.colW)) > 0.5 {
		t.Fatalf("range box %v, want x %v width %v", rangeBox, g.text.X+6*g.colW, w)
	}
	// The range box is drawn after the selection (over it).
	selBg := defaultColor(themeOf(st), "editor.inactiveSelectionBackground")
	selAt, boxAt := -1, -1
	for i, op := range ops {
		if f, ok := op.(paint.FillRect); ok {
			if f.Color == selBg && selAt < 0 {
				selAt = i
			}
			if f.Color == decoBlue && f.Rect == rangeBox {
				boxAt = i
			}
		}
	}
	if selAt < 0 || boxAt < selAt {
		t.Fatalf("selection op %d, range box op %d: want the box after the selection", selAt, boxAt)
	}
	if len(textsOf(ops, decoPurple)) < 2 {
		t.Fatal("no glyphs in the inline color (and the sign)")
	}
	// Strikethrough: a purple line over "beta".
	strike := 0
	for _, f := range fillsOf(ops, decoPurple) {
		if f.H <= 1.01 && absf(f.W-4*g.colW) < 1 {
			strike++
		}
	}
	if strike != 1 {
		t.Fatalf("%d strikethroughs, want 1", strike)
	}
	margins := fillsOf(ops, decoGreen)
	if len(margins) != 2 || margins[0].X != g.gutter.X || margins[0].W != g.gutter.W {
		t.Fatalf("margin fills %v, want 2 rows across the gutter %v", margins, g.gutter)
	}
	bars := 0
	for _, b := range boxes {
		if b.X < g.text.X && b.W == 3 {
			bars++
		}
	}
	if bars != 2 {
		t.Fatalf("%d gutter bars, want 2", bars)
	}
	if n := len(textsOf(ops, decoOrange)); n != 2 {
		t.Fatalf("%d line numbers in the decoration color, want 2", n)
	}
	if len(fillsOf(ops, decoTeal)) != 1 {
		t.Fatalf("overview marks %v, want one (two adjacent lines merged)", fillsOf(ops, decoTeal))
	}

	// Another view of the Model does not draw st's decorations; Model
	// decorations show in both.
	other := NewView(st.Model, theme.Base("vs"))
	h2 := decoHost(t, other, geom.Size{W: 480, H: 240})
	if n := len(fillsOf(h2.r.List().Ops, decoRed)); n != 0 {
		t.Fatalf("other view draws %d of st's whole-line fills", n)
	}
	st.Model.SetDecorations("m", []Decoration{LineDecoration(snap, 3, 3, whole)})
	h2.frame()
	if n := len(fillsOf(h2.r.List().Ops, decoRed)); n != 1 {
		t.Fatalf("other view draws %d Model whole-line fills, want 1", n)
	}
}

// TestDecorationsPaintGapsWrapLong: a range background covers the color
// gap inside it (injected.go's column to x), spans every row of a wrapped
// line, and on a long line is drawn at its place in the shaped chunks.
func TestDecorationsPaintGapsWrapLong(t *testing.T) {
	t.Run("gap", func(t *testing.T) {
		st, h := injHost(t, "a #ff0000 b\n", geom.Size{W: 480, H: 200})
		st.SetDecorations("t", []Decoration{{Start: 0, End: 11, Options: &DecorationOptions{Background: decoBlue}}})
		h.frame()
		g := h.vw.vs
		boxes := fillsOf(h.r.List().Ops, decoBlue)
		x0, x1 := float32(1e9), float32(-1e9)
		for _, b := range boxes {
			x0, x1 = min(x0, b.X), max(x1, b.Right())
		}
		want := 11*g.colW + colorSwatchW(st.Options.fontSize())
		if absf((x1-x0)-want) > 1 {
			t.Fatalf("range boxes %v span %v, want the text and the gap %v", boxes, x1-x0, want)
		}
	})
	t.Run("wrap", func(t *testing.T) {
		goText(t)
		st := NewState(strings.Repeat("word ", 60)+"\nend\n", "a.txt", theme.Base("vs"))
		st.Options.WordWrap = WrapOn
		h := decoHost(t, st, geom.Size{W: 300, H: 300})
		st.SetDecorations("t", []Decoration{{Start: 2, End: 100, Options: &DecorationOptions{Background: decoBlue}}})
		h.frame()
		lb := st.breaks(0)
		want := 0
		for k := range lb.rows() {
			if b0, b1 := lb.rowRange(k, 300); b0 < 100 && b1 > 2 {
				want++
			}
		}
		if want < 3 {
			t.Fatalf("the range is on %d rows", want)
		}
		ys := map[float32]bool{}
		for _, b := range fillsOf(h.r.List().Ops, decoBlue) {
			ys[b.Y] = true
		}
		if len(ys) != want {
			t.Fatalf("boxes on %d rows, want %d", len(ys), want)
		}
	})
	t.Run("long", func(t *testing.T) {
		goText(t)
		long := strings.Repeat("abcdefghij", 1000) // 10 000 runes: a long line
		st := NewState(long+"\n", "a.txt", theme.Base("vs"))
		h := decoHost(t, st, geom.Size{W: 400, H: 200})
		st.Viewport.ScrollX = 5000 * h.vw.vs.colW
		st.SetDecorations("t", []Decoration{{Start: 5005, End: 5010, Options: &DecorationOptions{Background: decoBlue}}})
		h.frame()
		g := h.vw.vs
		boxes := fillsOf(h.r.List().Ops, decoBlue)
		want := g.text.X - st.Viewport.ScrollX + 5005*g.colW
		if len(boxes) != 1 || absf(boxes[0].X-want) > 1 || absf(boxes[0].W-5*g.colW) > 1 {
			t.Fatalf("long line boxes %v, want x %v width %v", boxes, want, 5*g.colW)
		}
	})
}

// TestDecorationsDamageVerified runs decorations through the strict
// verifier (spec 6.8): setting them, typing before and inside them (they
// move), another owner, a view-only owner, changing one line's decoration
// (only that row is damaged), scrolling and removing them. Every frame but
// those changing many rows is drawn partially.
func TestDecorationsDamageVerified(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("scale%v", scale), func(t *testing.T) {
			goText(t)
			st := NewState(hintDoc(80), "main.go", theme.Base("vs-dark"))
			h := newHintHost(t, st, geom.Size{W: 480, H: 360}, scale)
			h.focusAt(3)
			// The frames the decorations change little in must be drawn
			// partially; the others (whole-line decorations set on or
			// removed from many rows, a line count change moving every
			// row below) may be full redraws.
			mayBeFull := map[string]bool{"set": true, "enter": true, "backspace": true, "undo": true, "redo": true, "remove": true}
			var full []string
			step := func(what string) {
				t.Helper()
				before := h.partial
				h.step(what)
				if h.partial == before && !mayBeFull[what] {
					full = append(full, what)
				}
			}
			snap := st.Snapshot()
			whole := &DecorationOptions{WholeLine: true, Background: color.NRGBA{0x30, 0x60, 0x30, 0xff}, MarginBackground: decoGreen,
				Gutter: &GutterMark{Sign: "+", SignColor: decoGreen, Bar: decoGreen}, LineNumberColor: decoOrange,
				Overview: &OverviewStyle{Color: decoGreen, Lane: 4}}
			inner := &DecorationOptions{Background: color.NRGBA{0x60, 0x30, 0x30, 0xff}, Border: decoRed, Inline: &InlineStyle{Color: decoOrange, Underline: true}}
			var ds []Decoration
			for l := 4; l < 40; l += 3 {
				ds = append(ds, LineDecoration(snap, l, l, whole))
				ds = append(ds, Decoration{Start: snap.LineStart(l+1) + 5, End: snap.LineStart(l+1) + 12, Options: inner})
			}
			st.Model.SetDecorations("diff", ds)
			step("set")
			for i := range 6 {
				h.typeText(string(rune('a' + i)))
				step("type")
			}
			h.key(input.KeyEnter, 0)
			step("enter")
			h.key(input.KeyBackspace, 0)
			step("backspace")
			st.SetDecorations("view", []Decoration{{Start: snap.LineStart(9), End: snap.LineStart(9) + 20, Options: inner}})
			step("view owner")
			st.Model.SetDecorations("other", []Decoration{LineDecoration(st.Snapshot(), 12, 12, whole)})
			step("other owner")

			// Changing one line's decoration damages that row only.
			cur := st.Model.Decorations("diff")
			cur[1].End++ // inside line 5
			st.Model.SetDecorations("diff", cur)
			step("one line")
			lh := st.Viewport.LineHeight
			dam, _ := h.lineHints()
			for _, d := range dam {
				if d.X < h.vw.vs.overviewRect.X && d.H > lh+2*rowPad(lh)+0.5 {
					t.Fatalf("one decoration changed: damage %v taller than a row (%v)", d, lh)
				}
			}
			p := geom.Pt(240, 180)
			for _, dy := range []float32{12, 30, -8, 40} {
				h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
				step("scroll")
			}
			h.r.HandleEvent(input.Scroll{Pos: p, DX: 16})
			step("scroll x")
			// The renderer copies a horizontal scroll only when the text
			// area's edges are whole device pixels (scrollSafe matches its
			// clip edges; the gutter's width follows the font's digits, so
			// the text area of a 1.5 scale starts at a half pixel with some
			// fonts): otherwise it redraws the text area, a full frame.
			if tx := h.vw.vs.text; !devAligned(tx.X, scale) || !devAligned(tx.Right(), scale) {
				if n := len(full); n > 0 && full[n-1] == "scroll x" {
					full = full[:n-1]
					t.Logf("scroll x drawn in full: text area %v at scale %v is not on device pixels", tx, scale)
				}
			}
			st.Undo()
			step("undo")
			st.Redo()
			step("redo")
			st.Model.SetDecorations("diff", nil)
			step("remove")
			if n, last := h.ren.VerifyFailures(); n != 0 {
				t.Fatalf("%d verify failures, last %v", n, last)
			}
			if len(full) > 0 {
				t.Fatalf("frames drawn in full: %v (%d of %d frames partial)", full, h.partial, h.frames)
			}
			t.Logf("%d of %d frames partial", h.partial, h.frames)
		})
	}
}

// devAligned reports whether logical coordinate x is a whole device pixel
// at scale.
func devAligned(x, scale float32) bool {
	d := float64(x) * float64(scale)
	return math.Abs(d-math.Round(d)) < 1e-3
}

// TestDecorationsOverviewDelay: the overview marks follow a new set at
// once, and the edits that move the decorations at most every
// overviewDecoDelay (a wake-up then draws them in place).
func TestDecorationsOverviewDelay(t *testing.T) {
	st := NewState(manyLines(200), "a.txt", theme.Base("vs"))
	h := decoHost(t, st, geom.Size{W: 480, H: 240})
	o := &DecorationOptions{Overview: &OverviewStyle{Color: decoTeal, Lane: 4}}
	st.Model.SetDecorations("t", []Decoration{LineDecoration(st.Snapshot(), 100, 100, o)})
	h.frame()
	y0 := fillsOf(h.r.List().Ops, decoTeal)
	if len(y0) != 1 {
		t.Fatalf("marks %v", y0)
	}
	st.MoveCaret(0, false)
	st.Insert(strings.Repeat("\n", 60))
	h.frame()
	// Still line 100's mark (the ruler's scale changed with the line count).
	y1 := fillsOf(h.r.List().Ops, decoTeal)
	if len(y1) != 1 || y1[0].Y >= y0[0].Y {
		t.Fatalf("marks right after typing %v, want line 100's (above %v)", y1, y0)
	}
	next := h.r.NextWake()
	if next.After(h.now.Add(overviewDecoDelay)) {
		t.Fatalf("no wake-up for the marks: next %v", next.Sub(h.now))
	}
	h.now = h.now.Add(overviewDecoDelay)
	h.frame()
	if y := fillsOf(h.r.List().Ops, decoTeal); len(y) != 1 || y[0].Y <= y1[0].Y+10 {
		t.Fatalf("marks after the delay %v, want line 160's (below %v)", y, y1)
	}
}
