package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// TestScrollShiftScrollbarStripRTL: the scrollbar strip a ScrollShift
// damages is at the inline end edge of the scroll container: the right
// edge in LTR, the left edge in RTL (where paintScrollbars draws the bar).
func TestScrollShiftScrollbarStripRTL(t *testing.T) {
	r := geom.Rect{W: 200, H: 100}
	left := geom.Rect{W: damageGutter, H: 100}
	right := geom.Rect{X: 200 - damageGutter, W: damageGutter, H: 100}
	for _, rtl := range []bool{false, true} {
		d := geom.Point{Y: -10}
		page := func(c *Context) {
			c.dir.rtl = rtl
			sb := Scroll(c, func() {
				for i := 0; i < 30; i++ {
					Div(c, layout.H(20)).Bg(c.Theme.BgContainer)
				}
				c.ScrollShift(r, d)
			}, layout.Size(200, 100))
			sb.n.ln.RTL = rtl
		}
		g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
		g.frame(t)
		g.frame(t)
		dam, sc := hints(g.r)
		if len(sc) != 1 || sc[0].Rect != r {
			t.Fatalf("rtl %v: scroll regions %v", rtl, sc)
		}
		bar, other := right, left
		if rtl {
			bar, other = left, right
		}
		if !covered(dam, bar) {
			t.Errorf("rtl %v: damage %v misses the scrollbar strip %v", rtl, dam, bar)
		}
		if covered(dam, other) {
			t.Errorf("rtl %v: damage %v covers the edge without a scrollbar %v", rtl, dam, other)
		}
	}
}

// TestBoxScrollShiftFromPaint: a widget that scrolls its own painting
// declares the shift from its paint function.
func TestBoxScrollShiftFromPaint(t *testing.T) {
	r := geom.Rect{W: 300, H: 200}
	d := geom.Point{Y: -20}
	page := func(c *Context) {
		var b *Box
		b = Custom(c, func(maxW, maxH float32) geom.Size { return geom.Size{W: 300, H: 200} }, func(l *paint.List, rr geom.Rect) {
			l.Fill(rr, geom.Radii{}, c.Theme.BgContainer)
			b.ScrollShift(r, d)
		})
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	dam, sc := hints(g.r)
	if len(sc) != 1 || sc[0].Rect != r || sc[0].Delta != d {
		t.Fatalf("scroll regions %v", sc)
	}
	if !covered(dam, geom.Rect{Y: 180, W: 300, H: 20}) {
		t.Fatalf("damage %v misses the exposed strip", dam)
	}
	if covered(dam, geom.Rect{Y: 20, W: 300, H: 100}) {
		t.Fatalf("shifted content redrawn: %v", dam)
	}
}

// TestWakeAtDamaged: a WakeAtDamaged timer wakes the host like WakeAt but
// keeps the frame it wakes partial (its widget declares the change), while
// a WakeAt timer reached in the same run still makes its frame full.
func TestWakeAtDamaged(t *testing.T) {
	caret := geom.Rect{X: 10, Y: 10, W: 2, H: 16}
	var plain time.Time
	page := func(c *Context) {
		c.Damage(caret) // the caret widget declares its blink every frame
		c.WakeAtDamaged(c.Now().Add(500 * time.Millisecond))
		if !plain.IsZero() {
			c.WakeAt(plain)
		}
	}
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	want := g.now.Add(500 * time.Millisecond)
	if got := g.r.NextWake(); !got.Equal(want) {
		t.Fatalf("NextWake %v, want %v", got, want)
	}
	g.now = want
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 || d[0] != caret {
		t.Fatalf("WakeAtDamaged frame: damage %v, want only the caret", d)
	}
	// A plain timer, earlier than the next blink: reaching it is a full
	// frame.
	plain = g.now.Add(100 * time.Millisecond)
	g.frame(t)
	if got := g.r.NextWake(); !got.Equal(plain) {
		t.Fatalf("NextWake %v, want the WakeAt timer %v", got, plain)
	}
	g.now = plain
	plain = time.Time{}
	g.frame(t)
	if d, s := hints(g.r); len(d)+len(s) != 0 {
		t.Fatalf("WakeAt timer frame wrote partial hints %v %v", d, s)
	}
	// Later than the next blink: the blink frame stays partial.
	plain = g.now.Add(time.Second)
	g.frame(t)
	g.now = g.now.Add(500 * time.Millisecond)
	g.frame(t)
	if d, _ := hints(g.r); len(d) != 1 {
		t.Fatalf("blink before the WakeAt timer: damage %v", d)
	}
	g.now = g.now.Add(time.Second)
	g.frame(t)
	if d, s := hints(g.r); len(d)+len(s) != 0 {
		t.Fatalf("WakeAt timer reached after a blink: partial hints %v %v", d, s)
	}
}
