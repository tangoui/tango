package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// G7 RTL tests: editor line alignment and arrow keys, form labels,
// rich text alignment and the mirror checker.

const g7Arabic = "مرحبا"

// g7Input renders one 300px Input in the given direction (DirAuto: no
// provider) and returns the driver and a getter for the editor rect.
func g7Input(dir Direction, val *string) (*Driver, func() geom.Rect) {
	var id ID
	page := func(c *Context) {
		body := func() {
			id = Input(c, val).Layout(layout.W(300)).ID()
		}
		if dir == DirAuto {
			body()
			return
		}
		ConfigProvider(c).Direction(dir).Body(body)
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Frame()
	return d, func() geom.Rect { return d.r.c.rects[id] }
}

func g7Caret(t *testing.T, d *Driver) geom.Rect {
	t.Helper()
	d.Frame()
	on, caret := d.TextInputActive()
	if !on {
		t.Fatal("text input not active")
	}
	return caret
}

func TestG7EditorEmptyRTLCaretAtRight(t *testing.T) {
	for _, dir := range []Direction{LTR, RTL} {
		val := ""
		d, rect := g7Input(dir, &val)
		r := rect()
		d.Click(r.Center())
		c := g7Caret(t, d)
		if dir == RTL && c.X < r.Right()-3 {
			t.Fatalf("RTL empty caret %+v not at the right of %+v", c, r)
		}
		if dir == LTR && c.X > r.X+1 {
			t.Fatalf("LTR empty caret %+v not at the left of %+v", c, r)
		}
	}
}

func TestG7EditorRTLLineRightAligned(t *testing.T) {
	// Arabic content in an LTR scope (DirAuto) is right-aligned: Home puts
	// the caret at the right edge, End at the left end of the text.
	val := g7Arabic
	d, rect := g7Input(DirAuto, &val)
	r := rect()
	d.Click(geom.Pt(r.Right()-2, r.Center().Y))
	d.Key(input.KeyHome, 0)
	home := g7Caret(t, d)
	d.Key(input.KeyEnd, 0)
	end := g7Caret(t, d)
	if home.X < r.Right()-3 {
		t.Fatalf("caret at the logical start %+v not at the right edge of %+v", home, r)
	}
	if !(end.X < home.X-10) || end.X < r.X+r.W/2 {
		t.Fatalf("caret at the logical end %+v; start %+v (box %+v)", end, home, r)
	}
	// The glyphs sit inside [end, home].
	for _, op := range d.r.List().Ops {
		if tx, ok := op.(paint.Text); ok && len(tx.Glyphs) > 0 && tx.Origin.Y >= r.Y-2 && tx.Origin.Y <= r.Bottom() {
			if tx.Origin.X < end.X-1 {
				t.Fatalf("text origin %v left of the line start %v", tx.Origin.X, end.X)
			}
		}
	}
}

func TestG7EditorRTLHitTest(t *testing.T) {
	// Clicking the empty middle of the box (left of right-aligned RTL text)
	// puts the caret at the logical end, so typing appends.
	val := g7Arabic
	d, rect := g7Input(DirAuto, &val)
	r := rect()
	d.Click(geom.Pt(r.X+r.W/2, r.Center().Y))
	d.Frame()
	d.Type("X")
	d.Frame()
	if val != g7Arabic+"X" {
		t.Fatalf("click left of RTL text then type: %q", val)
	}
	// A click at the right edge is the logical start.
	d.Click(geom.Pt(r.Right()-1, r.Center().Y))
	d.Frame()
	d.Type("Y")
	d.Frame()
	if !strings.HasPrefix(val, "Y") {
		t.Fatalf("click at the right edge then type: %q", val)
	}
}

func TestG7EditorRTLArrowKeysLogical(t *testing.T) {
	// In an RTL paragraph Left is the logical next character.
	val := g7Arabic
	d, rect := g7Input(DirAuto, &val)
	d.Click(rect().Center())
	d.Key(input.KeyHome, 0)
	d.Key(input.KeyLeft, 0)
	d.Frame()
	d.Type("X")
	d.Frame()
	rs := []rune(g7Arabic)
	if want := string(rs[:1]) + "X" + string(rs[1:]); val != want {
		t.Fatalf("Home, Left, X in RTL: %q want %q", val, want)
	}
	// Right is the previous character: twice from after X reaches 0.
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyRight, 0)
	d.Frame()
	d.Type("Y")
	d.Frame()
	if !strings.HasPrefix(val, "Y") {
		t.Fatalf("Right twice then Y: %q", val)
	}
	// LTR text keeps the physical mapping.
	lv := "abc"
	d2, r2 := g7Input(RTL, &lv)
	d2.Click(r2().Center())
	d2.Key(input.KeyHome, 0)
	d2.Key(input.KeyRight, 0)
	d2.Frame()
	d2.Type("X")
	d2.Frame()
	if lv != "aXbc" {
		t.Fatalf("LTR text in RTL scope: %q", lv)
	}
}

// g7FormOps renders a horizontal form with one required item and returns
// the label's star / text / colon Text ops and the label and control rects.
func g7FormOps(t *testing.T, dir Direction) (star, label, colon paint.Text, lr, cr geom.Rect) {
	t.Helper()
	val := ""
	var lid, cid ID
	page := func(c *Context) {
		ConfigProvider(c).Direction(dir).Body(func() {
			f := Form(c)
			f.Body(func() {
				it := f.Item("Name").Required()
				it.Body(func() { cid = Input(c, &val).ID() })
				lid = it.labelID
			})
		})
	}
	d := NewDriver(page, DriverOptions{Width: 500, Height: 200})
	d.Frame()
	d.Frame()
	th := d.r.c.Theme
	lr, cr = d.r.c.rects[lid], d.r.c.rects[cid]
	for _, op := range d.r.List().Ops {
		tx, ok := op.(paint.Text)
		if !ok || tx.Origin.Y < lr.Y || tx.Origin.Y > lr.Bottom() || tx.Origin.X < lr.X-1 || tx.Origin.X > lr.Right() {
			continue
		}
		switch {
		case tx.Color == th.Error.Base:
			star = tx
		case len(tx.Glyphs) == 4:
			label = tx
		case len(tx.Glyphs) == 1:
			colon = tx
		}
	}
	if len(star.Glyphs) == 0 || len(label.Glyphs) == 0 {
		t.Fatalf("%v: label ops not found (star %v, label %v)", dir, star, label)
	}
	return
}

func TestG7FormLabelRTL(t *testing.T) {
	sL, lL, cL, lrL, crL := g7FormOps(t, LTR)
	if !(sL.Origin.X < lL.Origin.X) || !(lrL.X < crL.X) {
		t.Fatalf("LTR: star %v label %v, label box %+v control %+v", sL.Origin, lL.Origin, lrL, crL)
	}
	s, l, col, lr, cr := g7FormOps(t, RTL)
	if !(lr.X > cr.X) {
		t.Fatalf("RTL: label column %+v not right of the control %+v", lr, cr)
	}
	if !(s.Origin.X > l.Origin.X) {
		t.Fatalf("RTL: required mark %v not on the start (right) side of the text %v", s.Origin, l.Origin)
	}
	if len(cL.Glyphs) == 1 && len(col.Glyphs) == 1 && !(col.Origin.X < l.Origin.X) {
		t.Fatalf("RTL: colon %v not on the end (left) side of the text %v", col.Origin, l.Origin)
	}
	// Ant's labelAlign right is logical: the label hugs the control, so in
	// RTL its leftmost glyph is near the label column's left edge.
	left := l.Origin.X
	if len(col.Glyphs) == 1 {
		left = col.Origin.X
	}
	if left-lr.X > 16 {
		t.Fatalf("RTL: label group starts %v px from the column's end edge", left-lr.X)
	}
}

func TestG7RichTextAlignRTL(t *testing.T) {
	cases := []struct {
		a     TextAlign
		right bool
	}{
		{AlignStart, true}, {AlignEnd, false}, {AlignPhysicalLeft, false}, {AlignPhysicalRight, true},
	}
	for _, tc := range cases {
		var w *RichTextW
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Direction(RTL).Body(func() {
				w = RichText(c, Span{Text: "hello"}).Align(tc.a)
			})
		}, DriverOptions{Width: 400, Height: 100})
		d.Frame()
		d.Frame()
		r := d.r.c.rects[w.n.id]
		var x float32 = -1
		for _, op := range d.r.List().Ops {
			if tx, ok := op.(paint.Text); ok && len(tx.Glyphs) == 5 {
				x = tx.Origin.X
			}
		}
		if x < 0 {
			t.Fatalf("align %v: no text op", tc.a)
		}
		if got := x > r.X+r.W/2; got != tc.right {
			t.Fatalf("align %v in RTL: text at %v in %+v (want right=%v)", tc.a, x, r, tc.right)
		}
	}
}

func TestG7MirrorCheck(t *testing.T) {
	// A row of three boxes mirrors cleanly; a box placed by hand with a
	// physical offset in RTL does not.
	var bad ID
	page := func(c *Context) {
		Row(c, func() {
			Row(c, func() {}, layout.W(50), layout.H(20))
			Row(c, func() {}, layout.W(80), layout.H(20))
			Row(c, func() {}, layout.W(30), layout.H(20), layout.Physical())
		}, layout.Gap(10), layout.W(300), layout.H(40))
		Column(c, func() {
			x := float32(10)
			if c.IsRTL() {
				x = 200 - 10 - 20 // "fixes" the mirror by hand: stays at the physical left
			}
			b := Row(c, func() {}, layout.W(20), layout.H(20), layout.Abs(x, 5, layout.Unset, layout.Unset))
			bad = b.n.id
		}, layout.W(200), layout.H(30))
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 200})
	rep := d.MirrorCheck(page)
	if rep.Checked == 0 {
		t.Fatal("nothing checked")
	}
	if len(rep.Violations) != 1 || rep.Violations[0].ID != bad {
		t.Fatalf("want exactly the hand-placed box, got:\n%s", rep)
	}
	v := rep.Violations[0]
	// The root mirrors too (a root provider's direction is the window's),
	// so the column moves; the hand-placed box stays far from its mirror.
	if abs32(v.Want.X-v.RTL.X) < 100 {
		t.Fatalf("violation %v", v)
	}
	if rep.Skipped == 0 {
		t.Fatalf("Physical() box not skipped:\n%s", rep)
	}
	// Without the deliberate bug the page passes.
	clean := func(c *Context) {
		Row(c, func() {
			Row(c, func() {}, layout.W(50), layout.H(20))
			Row(c, func() {}, layout.W(80), layout.H(20))
		}, layout.Gap(10), layout.W(300), layout.H(40))
	}
	if rep := NewDriver(clean, DriverOptions{Width: 400, Height: 200}).MirrorCheck(clean); !rep.OK() {
		t.Fatalf("clean page:\n%s", rep)
	}
}

func TestG7MirrorCheckSkipsLTRScope(t *testing.T) {
	// A subtree kept LTR on purpose (code, charts) is not reported.
	page := func(c *Context) {
		Row(c, func() {
			ConfigProvider(c).Direction(LTR).Body(func() {
				Row(c, func() {
					Row(c, func() {}, layout.W(40), layout.H(20))
					Row(c, func() {}, layout.W(40), layout.H(20))
				}, layout.W(200), layout.H(30))
			})
		}, layout.W(300), layout.H(40))
	}
	rep := NewDriver(page, DriverOptions{Width: 400, Height: 200}).MirrorCheck(page)
	if !rep.OK() || rep.Skipped < 2 {
		t.Fatalf("LTR scope:\n%s", rep)
	}
}
