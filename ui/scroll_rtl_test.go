package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// rtlScrollH builds a 400px ScrollH whose content is a 100px start box and
// a 1900px filler, laid out right to left.
func rtlScrollH(sb **ScrollBox, first **Box) *Driver {
	return NewDriver(func(c *Context) {
		c.dir.rtl = true
		*sb = ScrollH(c, func() {
			*first = Row(c, func() {}, layout.W(100), layout.H(40))
			Row(c, func() {}, layout.W(1900), layout.H(40))
		}, layout.W(400))
		(*sb).n.ln.RTL = true
	}, DriverOptions{Width: 600, Height: 400})
}

func TestScrollRTLLogicalOffset(t *testing.T) {
	var sb *ScrollBox
	var first *Box
	d := rtlScrollH(&sb, &first)
	d.Frame()
	d.Frame()
	view := d.r.c.rects[sb.ID()]
	fr := d.r.c.rects[first.ID()]
	// Offset 0 is the inline start: the first box sits at the right edge.
	if x, _ := sb.Offset(); x != 0 {
		t.Fatalf("initial offset %v", x)
	}
	if fr.Right() != view.Right() || fr.W != 100 {
		t.Fatalf("first box %+v not at the right edge of %+v", fr, view)
	}
	// ScrollTo(300) moves the content right by 300 (reveals the left side).
	sb.ScrollTo(300, 0)
	d.Frame()
	d.Frame()
	if fr2 := d.r.c.rects[first.ID()]; fr2.X != fr.X+300 {
		t.Fatalf("after ScrollTo(300): %+v want x=%v", fr2, fr.X+300)
	}
	// A physical wheel to the left (dx < 0) scrolls towards the end in RTL.
	d.Inject(input.Scroll{Pos: geom.Pt(view.X+50, view.Y+10), DX: -50})
	d.Settle()
	if x, _ := sb.Offset(); x != 350 {
		t.Fatalf("wheel dx=-50: offset %v want 350", x)
	}
	d.Inject(input.Scroll{Pos: geom.Pt(view.X+50, view.Y+10), DX: 80})
	d.Settle()
	if x, _ := sb.Offset(); x != 270 {
		t.Fatalf("wheel dx=+80: offset %v want 270", x)
	}
}

func TestScrollRTLThumbAndDrag(t *testing.T) {
	var sb *ScrollBox
	var first *Box
	d := rtlScrollH(&sb, &first)
	d.Frame()
	d.Frame()
	r := d.r.c.rects[sb.ID()]
	d.Move(geom.Pt(r.Right()-10, r.Bottom()-4))
	d.Frame()
	// At logical offset 0 the 80px thumb sits at the right end of the track.
	thumb := geom.Rect{}
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.H > 5 && f.Rect.H < 9 && f.Rect.W > 50 && f.Rect.W < 100 {
			thumb = f.Rect
		}
	}
	if thumb.W == 0 || thumb.Right() < r.Right()-4 {
		t.Fatalf("thumb %+v not at the right end of %+v", thumb, r)
	}
	// Dragging the thumb 160px left scrolls 800px towards the end.
	d.Drag(geom.Pt(r.Right()-10, r.Bottom()-4), geom.Pt(r.Right()-10-160, r.Bottom()-4))
	if x, _ := sb.Offset(); x < 790 || x > 810 {
		t.Fatalf("drag offset %v want ~800", x)
	}
}

func TestScrollRTLVerticalBarOnLeft(t *testing.T) {
	var sb *ScrollBox
	d := NewDriver(func(c *Context) {
		c.dir.rtl = true
		sb = Scroll(c, func() { Column(c, func() {}, layout.H(800)) }, layout.H(100), layout.W(200)).Always()
		sb.n.ln.RTL = true
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[sb.ID()]
	d.Move(geom.Pt(500, 350))
	var bar geom.Rect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W >= 5 && f.Rect.W <= 8 && f.Rect.H > 10 {
			bar = f.Rect
		}
	}
	if bar.W == 0 || bar.X > r.X+4 {
		t.Fatalf("vertical thumb %+v not on the left of %+v", bar, r)
	}
	if id, ok := d.r.c.scrollbarAt(geom.Pt(r.X+3, r.Y+20)); !ok || id != sb.ID()^scrollTag {
		t.Fatal("left strip is not the scrollbar in RTL")
	}
	if _, ok := d.r.c.scrollbarAt(geom.Pt(r.Right()-3, r.Y+20)); ok {
		t.Fatal("right strip must not grab the scrollbar in RTL")
	}
	// Dragging the left thumb down scrolls.
	d.Drag(geom.Pt(r.X+3, r.Y+5), geom.Pt(r.X+3, r.Y+40))
	if _, y := sb.Offset(); y <= 0 {
		t.Fatalf("drag on left bar did not scroll: %v", y)
	}
}

func TestScrollStateRTLMath(t *testing.T) {
	ln := &layout.Node{}
	ln.RTL, ln.ScrollX = true, true
	s := &scrollState{ln: ln}
	view := geom.Size{W: 100, H: 50}
	laid := geom.Size{W: 400, H: 50}
	if sx, _ := s.frame(view, laid); sx != 300 {
		t.Fatalf("offset 0: sx=%v want 300", sx)
	}
	s.off.X = 300
	if sx, _ := s.frame(view, laid); sx != 0 {
		t.Fatalf("offset max: sx=%v want 0", sx)
	}
	if sx, _ := s.shift(view, laid); sx != 0 {
		t.Fatalf("shift: %v", sx)
	}
	if s.physOffX() != 0 {
		t.Fatalf("physOffX %v", s.physOffX())
	}
	// Content narrower than the view: no translation.
	s.off.X = 0
	if sx, _ := s.frame(view, geom.Size{W: 60, H: 50}); sx != 0 {
		t.Fatalf("narrow: %v", sx)
	}
	// Overflow without ScrollX: children were mirrored in the box width.
	ln.ScrollX = false
	s.off.X = 40
	if sx, _ := s.frame(view, geom.Size{W: 150, H: 50}); sx != -40 {
		t.Fatalf("no ScrollX: %v", sx)
	}
	// LTR is unchanged.
	ln.RTL, ln.ScrollX = false, true
	s.off.X = 120
	if sx, _ := s.frame(view, laid); sx != 120 {
		t.Fatalf("ltr: %v", sx)
	}
}

func TestScrollRTLRevealX(t *testing.T) {
	ln := &layout.Node{}
	ln.RTL, ln.ScrollX = true, true
	s := &scrollState{ln: ln}
	s.frame(geom.Size{W: 100, H: 50}, geom.Size{W: 400, H: 50})
	// Logical range [250, 270) is off to the left at offset 0.
	if !s.revealX(250, 20) || s.off.X != 170 {
		t.Fatalf("revealX: %v", s.off.X)
	}
}
