package ui

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/layout"
)

func TestAspectRatioFollowsWidth(t *testing.T) {
	var r16, r1, rMax, rBad *AspectRatioW
	inner := ID(0)
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			r16 = AspectRatio(c, 16.0/9, func() {
				inner = Div(c).ID()
			})
			r1 = AspectRatio(c, 1, nil).Radius(8)
			rMax = AspectRatio(c, 2, nil).MaxWidth(200)
			rBad = AspectRatio(c, -3, nil)
		}, layout.W(320))
	}, DriverOptions{Width: 600, Height: 1200})
	d.Frame()
	d.Frame()
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 0.6 }
	g := r16.Rect()
	if !near(g.W, 320) || !near(g.H, 180) {
		t.Fatalf("16:9 box = %v, want 320x180", g)
	}
	if ir, _ := d.Runner().Context().RectOf(inner); !near(ir.W, 320) || !near(ir.H, 180) || !near(ir.Y, g.Y) {
		t.Fatalf("content = %v, want to fill %v", ir, g)
	}
	if g := r1.Rect(); !near(g.H, g.W) || !near(g.W, 320) {
		t.Fatalf("1:1 box = %v", g)
	}
	if g := rMax.Rect(); !near(g.H*2, g.W) || g.W > 200.5 {
		t.Fatalf("max width box = %v", g)
	}
	if rBad.Ratio() != 1 {
		t.Fatalf("bad ratio = %v, want 1", rBad.Ratio())
	}
	// Width changes carry the height along.
	d2 := NewDriver(func(c *Context) {
		Column(c, func() { r16 = AspectRatio(c, 4.0/3, nil) }, layout.W(480))
	}, DriverOptions{Width: 600, Height: 600})
	d2.Frame()
	d2.Frame()
	if g := r16.Rect(); !near(g.H, 360) {
		t.Fatalf("4:3 at 480 = %v", g)
	}
}

func TestAspectRatioRTL(t *testing.T) {
	var w *AspectRatioW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Column(c, func() { w = AspectRatio(c, 2, nil).MaxWidth(100) }, layout.W(300))
		})
	}, DriverOptions{Width: 300, Height: 300})
	d.Frame()
	d.Frame()
	g := w.Rect()
	if g.W > 100.5 || g.Right() < 299 {
		t.Fatalf("RTL: a narrower ratio box sits at the inline start (right): %v", g)
	}
}
