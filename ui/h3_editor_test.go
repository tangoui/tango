package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// H3: visual caret movement in mixed-direction editor text (RTL spec Q12
// phase 2), the Input placeholder token, and multi-line RTL alignment.
//
// The tests shape without fonts (every rune 0.55em wide) so the bidi
// geometry is deterministic.

func h3NoFonts(t *testing.T) {
	t.Helper()
	text.UseSystemFonts(false)
	t.Cleanup(func() { text.UseSystemFonts(true) })
}

// h3Walk steps the caret from start (with lead) steps times and returns
// the positions visited, "*" marking the lead (second visual) position and
// "logical" a step visualStep leaves to the logical fallback.
func h3Walk(lay *text.Layout, start int, lead, right bool, steps int) string {
	e := &editState{lay: lay, cur: start, lead: lead}
	var out []string
	add := func() {
		s := fmt.Sprint(e.cur)
		if e.lead {
			s += "*"
		}
		out = append(out, s)
	}
	add()
	for range steps {
		p, l, ok := e.visualStep(len(lay.Runes), e.lead, right)
		if !ok {
			out = append(out, "logical")
			break
		}
		e.cur, e.lead = p, l
		add()
	}
	return strings.Join(out, " ")
}

func TestH3VisualStops(t *testing.T) {
	h3NoFonts(t)
	st := text.Style{Size: 14}
	one := text.Options{NoWrap: true}
	cases := []struct {
		name  string
		s     string
		opt   text.Options
		start int
		right bool
		steps int
		want  string
	}{
		// "abc אבג def" (LTR paragraph) shows as "abc גבא def". Right walks
		// the screen left to right: into the Hebrew word at its left end
		// (4, after the space), across ג ב א (6, 5), then the right end of
		// the word, which only the leading edge of א (4*) reaches.
		{"mixed right", "abc אבג def", one, 0, true, 13, "0 1 2 3 4 6 5 4* 8 9 10 11 11 11"},
		{"mixed left", "abc אבג def", one, 11, false, 13, "11 10 9 8 7* 5 6 7 3 2 1 0 0 0"},
		// Hebrew at the end: 4* is the visual right end; End (7) is drawn
		// left of the Hebrew word.
		{"hebrew tail right", "abc אבג", one, 0, true, 8, "0 1 2 3 4 6 5 4* 4*"},
		{"hebrew tail left from end", "abc אבג", one, 7, false, 3, "7 3 2 1"},
		// "مرحبا 123" (RTL paragraph) shows as "123 ابحرم". Left from the
		// logical start (right edge) crosses the Arabic word, the space,
		// then the digits left to right in reverse (8, 7) up to their left
		// edge (6*).
		{"arabic digits left", "مرحبا 123", one, 0, false, 10, "0 1 2 3 4 5 6 8 7 6* 6*"},
		{"arabic digits right lead", "مرحبا 123", one, 6, true, 10, "6* 7 8 9 5 4 3 2 1 0 0"},
		{"arabic end left", "مرحبا 123", one, 9, false, 3, "9 8 7 6*"},
		// 6 without lead is the other position drawn at the digits' right
		// end: Right from there crosses the space.
		{"arabic digits plain right", "مرحبا 123", one, 6, true, 2, "6 5 4"},
	}
	for _, tc := range cases {
		lay := text.Shape(tc.s, st, tc.opt)
		start, lead := tc.start, false
		if tc.name == "arabic digits right lead" {
			lead = true
		}
		if got := h3Walk(lay, start, lead, tc.right, tc.steps); got != tc.want {
			t.Errorf("%s: %q got %q, want %q", tc.name, tc.s, got, tc.want)
		}
	}
}

func TestH3VisualStopsCaretX(t *testing.T) {
	// Every visual step moves the drawn caret strictly in its direction.
	h3NoFonts(t)
	for _, s := range []string{"abc אבג def", "مرحبا 123", "אבג abc 12 דהו"} {
		lay := text.Shape(s, text.Style{Size: 14}, text.Options{NoWrap: true})
		for _, right := range []bool{true, false} {
			e := &editState{lay: lay}
			// Start at the visual edge the walk leaves from.
			g := editLineStops(lay, 0)
			edge := g[0][0]
			if !right {
				edge = g[len(g)-1][0]
			}
			e.cur, e.lead = edge.pos, edge.lead
			x := editCaretX(lay, e.cur, e.lead)
			for k := 0; k < len(g)-1; k++ {
				p, l, ok := e.visualStep(len(lay.Runes), e.lead, right)
				if !ok {
					t.Fatalf("%q: logical fallback on a mixed line", s)
				}
				e.cur, e.lead = p, l
				nx := editCaretX(lay, p, l)
				if right && !(nx > x) || !right && !(nx < x) {
					t.Fatalf("%q right=%v step %d: caret x %v -> %v", s, right, k, x, nx)
				}
				x = nx
			}
		}
	}
}

func TestH3VisualLineTransitions(t *testing.T) {
	h3NoFonts(t)
	st := text.Style{Size: 14}
	// LTR paragraph wrapped into "abc אבג " / "def ghi": Right past the
	// visual right end goes to the next line's start; the second line is
	// pure LTR and moves logically, except Left from its start, which goes
	// to the previous line's visual end edge (4*/7* share that x; 7 is
	// logically nearer the line's end).
	lay := text.Shape("abc אבג def ghi", st, text.Options{MaxWidth: 70})
	if len(lay.Lines) != 2 || lay.Lines[1].Start != 8 {
		t.Fatalf("wrap: %+v", lay.Lines)
	}
	if got, want := h3Walk(lay, 0, false, true, 9), "0 1 2 3 4 6 5 4* 8 logical"; got != want {
		t.Errorf("wrapped LTR right: %q want %q", got, want)
	}
	if got, want := h3Walk(lay, 8, false, false, 8), "8 7* 5 6 7 3 2 1 0"; got != want {
		t.Errorf("wrapped LTR left: %q want %q", got, want)
	}
	// RTL paragraph wrapped into "אבג abc " / "דהו": Left is forward and
	// crosses to the second line's start (its right edge); Right walks
	// back.
	lay = text.Shape("אבג abc דהו", st, text.Options{MaxWidth: 70})
	if len(lay.Lines) != 2 || !lay.Lines[0].RTL || lay.Lines[1].Start != 8 {
		t.Fatalf("RTL wrap: %+v", lay.Lines)
	}
	if got, want := h3Walk(lay, 0, false, false, 12), "0 1 2 3 4 6 5 4* 8 9 10 11 11"; got != want {
		t.Errorf("wrapped RTL left: %q want %q", got, want)
	}
	if got, want := h3Walk(lay, 11, false, true, 12), "11 10 9 8 7* 5 6 7 3 2 1 0 0"; got != want {
		t.Errorf("wrapped RTL right: %q want %q", got, want)
	}
	// Hard break: Left from the start of "xyz" goes to the visual right
	// end of "abc אבג" (4*), not to its logical end 7 (drawn mid-line).
	lay = text.Shape("abc אבג\nxyz", st, text.Options{})
	if got, want := h3Walk(lay, 8, false, false, 3), "8 4* 5 6"; got != want {
		t.Errorf("hard break left: %q want %q", got, want)
	}
}

func TestH3PureLTRStaysLogical(t *testing.T) {
	h3NoFonts(t)
	for _, s := range []string{"hello world", "你好 abc", "", "line one\nline two"} {
		lay := text.Shape(s, text.Style{Size: 14}, text.Options{MaxWidth: 60})
		for n := range lay.Lines {
			if editLineMixed(lay, n) {
				t.Fatalf("%q line %d reported mixed", s, n)
			}
		}
		for _, i := range []int{0, len([]rune(s)) / 2} {
			e := &editState{lay: lay, cur: i}
			if _, _, ok := e.visualStep(len(lay.Runes), false, true); ok {
				t.Fatalf("%q: visual step on pure LTR text", s)
			}
		}
	}
}

// h3Input renders a 300px Input (no direction provider) and focuses it.
func h3Input(t *testing.T, val *string) *Driver {
	t.Helper()
	var id ID
	d := NewDriver(func(c *Context) {
		id = Input(c, val).Layout(layout.W(300)).ID()
	}, DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Frame()
	d.Click(d.r.c.rects[id].Center())
	d.Frame()
	return d
}

func h3CaretX(t *testing.T, d *Driver) float32 {
	t.Helper()
	d.Frame()
	on, r := d.TextInputActive()
	if !on {
		t.Fatal("text input not active")
	}
	return r.X
}

func TestH3InputVisualArrows(t *testing.T) {
	h3NoFonts(t)
	val := "abc אבג def"
	d := h3Input(t, &val)
	d.Key(input.KeyHome, 0)
	x := h3CaretX(t, d)
	// 11 Right presses cross the 11 runes on screen, left to right.
	for k := 0; k < 11; k++ {
		d.Key(input.KeyRight, 0)
		nx := h3CaretX(t, d)
		if !(nx > x) {
			t.Fatalf("Right #%d: caret x %v -> %v", k+1, x, nx)
		}
		x = nx
	}
	// Back to the right end of the Hebrew word (4 Left: 11 10 9 8 7*);
	// typing there inserts after the space following it.
	for range 4 {
		d.Key(input.KeyLeft, 0)
	}
	d.Frame()
	d.Type("X")
	d.Frame()
	if want := "abc אבגX def"; val != want {
		t.Fatalf("after visual Left: %q want %q", val, want)
	}

	// Arabic with digits (RTL paragraph): Left from the logical start
	// crosses "مرحبا " then enters the digits at their right end (6).
	val = "مرحبا 123"
	d = h3Input(t, &val)
	d.Key(input.KeyHome, 0)
	x = h3CaretX(t, d)
	for k := 0; k < 9; k++ {
		d.Key(input.KeyLeft, 0)
		nx := h3CaretX(t, d)
		if !(nx < x) {
			t.Fatalf("Arabic Left #%d: caret x %v -> %v", k+1, x, nx)
		}
		x = nx
	}
	// 9 Left: 1 2 3 4 5 6 8 7 6*; 3 Right: 7 8 9.
	for range 3 {
		d.Key(input.KeyRight, 0)
	}
	d.Frame()
	d.Type("X")
	d.Frame()
	if want := "مرحبا 123X"; val != want {
		t.Fatalf("Arabic after visual moves: %q want %q", val, want)
	}
}

func TestH3InputVisualSelection(t *testing.T) {
	h3NoFonts(t)
	val := "abc אבג def"
	d := h3Input(t, &val)
	// Shift+Right extends visually; the anchor stays at logical 0. Five
	// steps: 1 2 3 4 6, so the selection is [0, 6).
	d.Key(input.KeyHome, 0)
	for range 5 {
		d.Key(input.KeyRight, input.ModShift)
	}
	d.Frame()
	d.Type("X")
	d.Frame()
	if want := "Xג def"; val != want {
		t.Fatalf("Shift+Right x5 then type: %q want %q", val, want)
	}
	val = "abc אבג def"
	d = h3Input(t, &val)
	// From the end, Shift+Left x4 reaches 7* (the right end of the Hebrew
	// word): selection [7, 11).
	d.Key(input.KeyEnd, 0)
	for range 4 {
		d.Key(input.KeyLeft, input.ModShift)
	}
	d.Frame()
	d.Type("X")
	d.Frame()
	if want := "abc אבגX"; val != want {
		t.Fatalf("Shift+Left x4 then type: %q want %q", val, want)
	}
	// Word movement stays logical: Alt+Left from the end is the start of
	// "def".
	val = "abc אבג def"
	d = h3Input(t, &val)
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyLeft, input.ModAlt)
	d.Frame()
	d.Type("X")
	d.Frame()
	if want := "abc אבג Xdef"; val != want {
		t.Fatalf("Alt+Left then type: %q want %q", val, want)
	}
}

func TestH3TextAreaHardBreakLeft(t *testing.T) {
	h3NoFonts(t)
	v := "abc אבג\nxyz"
	var w *TextAreaW
	d := NewDriver(func(c *Context) { w = TextArea(c, &v).Rows(4).Layout(layout.W(300)) }, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[w.box.id]
	d.Click(geom.Pt(r.X+40, r.Bottom()-8)) // caret at the end
	d.Key(input.KeyHome, 0)                // start of "xyz"
	d.Key(input.KeyLeft, 0)                // visual end of line 0: 4*
	x := h3CaretX(t, d)
	lay := w.st.edit.lay
	right := w.st.edit.origin.X + w.st.edit.shiftOf(0) + lay.Lines[0].Width
	if abs32(x-right) > 1.5 {
		t.Fatalf("caret x %v, want the right end of line 0 at %v", x, right)
	}
	d.Type("X")
	d.Frame()
	if want := "abc Xאבג\nxyz"; v != want {
		t.Fatalf("typed at the visual line end: %q want %q", v, want)
	}
}

func TestH3TextAreaRTLLinesRightAligned(t *testing.T) {
	// Multi-line editors align each right-to-left paragraph to the right
	// and keep left-to-right ones at the left.
	h3NoFonts(t)
	v := "אבג\nabc\nمرحبا"
	var w *TextAreaW
	d := NewDriver(func(c *Context) { w = TextArea(c, &v).Rows(4).Layout(layout.W(300)) }, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	d.Frame()
	// Line i is drawn (paintAlignedGlyphs) and hit-tested at shift[i].
	ed := d.r.c.rects[w.ed.id]
	lay := w.st.edit.lay
	if lay == nil || len(lay.Lines) != 3 {
		t.Fatalf("layout %v", lay)
	}
	for i, ln := range lay.Lines {
		right := w.st.edit.shiftOf(i) + ln.Width
		if ln.RTL != (i != 1) {
			t.Fatalf("line %d RTL=%v", i, ln.RTL)
		}
		if ln.RTL && abs32(right-(ed.W-1)) > 0.01 {
			t.Fatalf("RTL line %d ends at %v, want %v", i, right, ed.W-1)
		}
	}
	if sh := w.st.edit.shift; len(sh) != 3 || sh[0] <= 0 || sh[1] != 0 || sh[2] <= 0 {
		t.Fatalf("line shifts %v", sh)
	}
}

func TestH3PlaceholderFollowsInputToken(t *testing.T) {
	// System fonts: without fonts no glyph op is emitted.
	pale := theme.Hex("#12ab34")
	render := func(scoped bool) (*Driver, *TextAreaW) {
		v := ""
		var w *TextAreaW
		d := NewDriver(func(c *Context) {
			body := func() { w = TextArea(c, &v).Placeholder("hint").Layout(layout.W(300)) }
			if scoped {
				ConfigProvider(c).Components(theme.Components{Input: theme.InputToken{PlaceholderColor: pale}}).Body(body)
			} else {
				body()
			}
		}, DriverOptions{Width: 400, Height: 300})
		d.Frame()
		d.Frame()
		return d, w
	}
	phColor := func(d *Driver, w *TextAreaW) (theme.Color, bool) {
		ed := d.r.c.rects[w.ed.id]
		for _, op := range d.r.List().Ops {
			if tx, ok := op.(paint.Text); ok && len(tx.Glyphs) == 4 && tx.Origin.Y >= ed.Y-1 && tx.Origin.Y <= ed.Bottom() {
				return tx.Color, true
			}
		}
		return theme.Color{}, false
	}
	// Default: the token equals TextQuaternary, so LTR pixels are unchanged.
	d, w := render(false)
	th := d.r.c.Theme
	if th.Components.Input.PlaceholderColor != th.TextQuaternary {
		t.Fatalf("Input.PlaceholderColor %v != TextQuaternary %v", th.Components.Input.PlaceholderColor, th.TextQuaternary)
	}
	if col, ok := phColor(d, w); !ok || col != th.TextQuaternary {
		t.Fatalf("default placeholder color %v (found %v)", col, ok)
	}
	// A scoped Input token override recolors it.
	d, w = render(true)
	if col, ok := phColor(d, w); !ok || col != pale {
		t.Fatalf("scoped placeholder color %v (found %v), want %v", col, ok, pale)
	}
}
