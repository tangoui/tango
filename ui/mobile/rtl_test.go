package mobile

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// rtlPhone is phone with the page under ConfigProvider.Direction(dir).
func rtlPhone(dir ui.Direction, page func(c *ui.Context), anim bool) *ui.Driver {
	return phone(func(c *ui.Context) {
		ui.ConfigProvider(c).Direction(dir).Body(func() { page(c) })
	}, anim)
}

func TestSpanMirror(t *testing.T) {
	r := geom.Rect{X: 10, Y: 0, W: 100, H: 20}
	ltr, rtl := spanOf(r, false), spanOf(r, true)
	if ltr.X(20, 30) != 20 || ltr.P(15) != 15 {
		t.Fatal("LTR span must be the identity")
	}
	// 20 from the left edge, 30 wide → 20 from the right edge.
	if got := rtl.X(30, 30); got != 10+100-20-30 {
		t.Fatalf("X: %v", got)
	}
	if got := rtl.P(rtl.P(37)); got != 37 {
		t.Fatalf("P is an involution: %v", got)
	}
	rr := rtl.Radii(geom.Radii{TL: 1, BL: 2})
	if rr != (geom.Radii{TR: 1, BR: 2}) {
		t.Fatalf("radii %+v", rr)
	}
	for _, k := range []input.Key{input.KeyLeft, input.KeyRight} {
		lp, ln := inlineKey(false, k)
		rp, rn := inlineKey(true, k)
		if lp != rn || ln != rp {
			t.Fatalf("inlineKey %v not swapped", k)
		}
	}
}

func TestNavBarRTL(t *testing.T) {
	for _, dir := range []ui.Direction{ui.LTR, ui.RTL} {
		var w *NavBarW
		back := 0
		d := rtlPhone(dir, func(c *ui.Context) {
			w = NavBar(c, "标题").Back("返回").Right("更多")
			if w.BackClicked() {
				back++
			}
		}, false)
		d.Settle()
		b, r := w.st.back, w.st.right
		if b.Empty() || r.Empty() {
			t.Fatalf("%v: zones %v %v", dir, b, r)
		}
		mid := float32(390) / 2
		if dir == ui.RTL {
			if b.X < mid || b.Right() != 390 || r.X != 0 {
				t.Fatalf("RTL: back %v should hug the right edge, action %v the left", b, r)
			}
		} else if b.X != 0 || r.Right() != 390 {
			t.Fatalf("LTR: back %v right %v", b, r)
		}
		d.Tap(geom.Pt(b.X+b.W/2, b.Y+b.H/2))
		d.Settle()
		if back != 1 {
			t.Fatalf("%v: back taps %d", dir, back)
		}
	}
}

func TestSwipeCellRTL(t *testing.T) {
	var w *SwipeCellW
	action := ""
	d := rtlPhone(ui.RTL, func(c *ui.Context) {
		ScrollView(c).Layout(layout.H(600)).Body(func() {
			w = SwipeCell(c).Right(SwipeAction{Key: "del", Text: "删除", Color: "danger"}).Body(func() {
				Cell(c, "可滑动的行")
			})
			if k := w.ActionKey(); k != "" {
				action = k
			}
		})
	}, true)
	d.Frame()
	d.Frame()
	f := newFinger(t, d)
	// A swipe to the left does not reveal the end actions in RTL.
	f.down(geom.Pt(90, 25))
	f.move(geom.Pt(30, 25), 6, 30*time.Millisecond)
	f.hold(100 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if w.IsOpen() {
		t.Fatalf("swipe left opened the cell: %v", w.st.sc.Pos)
	}
	// A swipe to the right does.
	f.down(geom.Pt(90, 25))
	f.move(geom.Pt(150, 25), 6, 30*time.Millisecond)
	f.hold(100 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if !w.IsOpen() || w.st.sc.Pos != w.st.sc.Max {
		t.Fatalf("open: %v %v", w.st.sc.Pos, w.st.sc.Max)
	}
	// The revealed action is at the left edge.
	f.down(geom.Pt(float32(w.st.sc.Max)/2, 25))
	f.up()
	run(d, time.Second)
	if action != "del" || w.IsOpen() {
		t.Fatalf("action %q open=%v", action, w.IsOpen())
	}
}

func TestIndexBarRTL(t *testing.T) {
	for _, dir := range []ui.Direction{ui.LTR, ui.RTL} {
		var rail geom.Rect
		var bubble geom.Rect
		d := rtlPhone(dir, func(c *ui.Context) {
			w := IndexBar(c, []string{"A", "B", "C"}).Layout(layout.H(600)).Body(func(i int) {
				Cell(c, "row")
			})
			rail, _ = c.RectOf(w.rail)
			fb := FloatingBubble(c, nil, AxisBoth, true)
			bubble, _ = c.RectOf(fb.b.ID())
		}, false)
		d.Settle()
		d.Settle()
		if dir == ui.RTL {
			if rail.X != 0 || bubble.X > 390/2 {
				t.Fatalf("RTL: rail %v and bubble %v belong on the left", rail, bubble)
			}
		} else if rail.Right() != 390 || bubble.X < 390/2 {
			t.Fatalf("LTR: rail %v bubble %v", rail, bubble)
		}
	}
}

func TestTabsRTL(t *testing.T) {
	active := 0
	var tabs *TabsW
	d := rtlPhone(ui.RTL, func(c *ui.Context) {
		tabs = Tabs(c, []string{"一", "二", "三"}, &active).Header()
	}, false)
	d.Settle()
	// Titles stretch over the width: the first is at the right.
	d.Tap(geom.Pt(30, 22))
	d.Settle()
	if active != 2 {
		t.Fatalf("tap on the left title: active %d, want 2", active)
	}
	d.Tap(geom.Pt(360, 22))
	d.Settle()
	if active != 0 {
		t.Fatalf("tap on the right title: active %d, want 0", active)
	}
	// Left moves to the next title (on the left) in RTL.
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if active != 1 {
		t.Fatalf("Left: active %d, want 1", active)
	}
	_ = tabs
}

// TestTabsPanesRTL checks that the active pane fills the viewport when the
// layout lays the swipeable panes out right to left.
func TestTabsPanesRTL(t *testing.T) {
	cols := []paintColor{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	for act := 0; act < 3; act++ {
		active := act
		d := rtlPhone(ui.RTL, func(c *ui.Context) {
			Tabs(c, []string{"一", "二", "三"}, &active).Swipeable().Body(func(i int) {
				col := cols[i]
				ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 200} },
					func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, col) })
			})
		}, false)
		d.Settle()
		img := d.Settle()
		for _, x := range []int{10, 380} {
			r, g, b, _ := img.At(x, 150).RGBA()
			want := cols[act]
			if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B {
				t.Fatalf("pane %d at x=%d: got %v", act, x, img.At(x, 150))
			}
		}
	}
}

func TestPopupPositionLogical(t *testing.T) {
	for _, c := range []struct {
		p    Position
		rtl  bool
		want Position
	}{
		{PosStart, false, PosLeft}, {PosStart, true, PosRight},
		{PosEnd, false, PosRight}, {PosEnd, true, PosLeft},
		{PosLeft, true, PosLeft}, {PosRight, true, PosRight}, {PosBottom, true, PosBottom},
	} {
		if got := c.p.resolve(c.rtl); got != c.want {
			t.Errorf("%d rtl=%v: %d want %d", c.p, c.rtl, got, c.want)
		}
	}
}
