package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestPlacementResolve(t *testing.T) {
	cases := []struct{ in, ltr, rtl Placement }{
		{BottomStart, BottomStart, BottomEnd},
		{BottomEnd, BottomEnd, BottomStart},
		{TopStart, TopStart, TopEnd},
		{TopEnd, TopEnd, TopStart},
		{Bottom, Bottom, Bottom},
		{InlineEnd, Right, Left},
		{InlineEndTop, RightStart, LeftStart},
		{InlineEndBottom, RightEnd, LeftEnd},
		{InlineStart, Left, Right},
		{InlineStartTop, LeftStart, RightStart},
		{InlineStartBottom, LeftEnd, RightEnd},
		{ScreenEnd, ScreenRight, ScreenLeft},
		{ScreenStart, ScreenLeft, ScreenRight},
		{ScreenTopEnd, ScreenTopRight, ScreenTopLeft},
		{ScreenTopStart, ScreenTopLeft, ScreenTopRight},
		// Physical placements never mirror.
		{Right, Right, Right},
		{RightStart, RightStart, RightStart},
		{Left, Left, Left},
		{ScreenRight, ScreenRight, ScreenRight},
		{ScreenTopRight, ScreenTopRight, ScreenTopRight},
		{ScreenFill, ScreenFill, ScreenFill},
	}
	for _, c := range cases {
		if got := c.in.Resolve(false); got != c.ltr {
			t.Errorf("%d LTR: got %d want %d", c.in, got, c.ltr)
		}
		if got := c.in.Resolve(true); got != c.rtl {
			t.Errorf("%d RTL: got %d want %d", c.in, got, c.rtl)
		}
	}
}

func overlayRect(t *testing.T, rtl bool, place Placement) (anchor, ov geom.Rect) {
	t.Helper()
	var aid, oid ID
	d := NewDriver(func(c *Context) {
		c.dir.rtl = rtl
		Column(c, func() {
			aid = Row(c, func() {}, layout.W(200), layout.H(30)).ID()
		}, layout.Pad(150))
		oid = Overlay(c, OverlayOptions{Anchor: aid, Place: place, Offset: 4, Margin: 10}, func() {
			Row(c, func() {}, layout.W(80), layout.H(40))
		}).ID()
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Frame()
	return d.r.c.rects[aid], d.r.c.rects[oid]
}

func TestOverlayLogicalPlacement(t *testing.T) {
	a, o := overlayRect(t, false, BottomStart)
	if o.X != a.X || o.Y != a.Bottom()+4 {
		t.Fatalf("LTR BottomStart: %+v anchor %+v", o, a)
	}
	a, o = overlayRect(t, true, BottomStart)
	if o.Right() != a.Right() || o.Y != a.Bottom()+4 {
		t.Fatalf("RTL BottomStart: %+v anchor %+v", o, a)
	}
	a, o = overlayRect(t, true, TopEnd)
	if o.X != a.X || o.Bottom() != a.Y-4 {
		t.Fatalf("RTL TopEnd: %+v anchor %+v", o, a)
	}
	a, o = overlayRect(t, false, InlineEndTop)
	if o.X != a.Right()+4 || o.Y != a.Y {
		t.Fatalf("LTR InlineEndTop: %+v anchor %+v", o, a)
	}
	a, o = overlayRect(t, true, InlineEndTop)
	if o.Right() != a.X-4 || o.Y != a.Y {
		t.Fatalf("RTL InlineEndTop: %+v anchor %+v", o, a)
	}
	// Physical stays physical.
	a, o = overlayRect(t, true, RightStart)
	if o.X != a.Right()+4 {
		t.Fatalf("RTL RightStart: %+v anchor %+v", o, a)
	}
	_, o = overlayRect(t, true, ScreenTopEnd)
	if o.X != 10 || o.Y != 10 {
		t.Fatalf("RTL ScreenTopEnd: %+v", o)
	}
	_, o = overlayRect(t, false, ScreenTopEnd)
	if o.Right() != 790 || o.Y != 10 {
		t.Fatalf("LTR ScreenTopEnd: %+v", o)
	}
	_, o = overlayRect(t, true, ScreenEnd)
	if o.X != 0 || o.H != 600 {
		t.Fatalf("RTL ScreenEnd: %+v", o)
	}
	_, o = overlayRect(t, true, ScreenRight)
	if o.Right() != 800 {
		t.Fatalf("RTL ScreenRight: %+v", o)
	}
}
