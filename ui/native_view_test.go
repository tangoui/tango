package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestNativeViewRegion(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Div(c, layout.H(40))
			NativeView(c, "web", layout.W(300), layout.H(200))
		}, layout.Pad(10))
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	regs := d.Runner().NativeRegions()
	if len(regs) != 1 {
		t.Fatalf("regions = %d, want 1", len(regs))
	}
	r := regs[0]
	want := geom.Rect{X: 10, Y: 50, W: 300, H: 200}
	if r.Rect != want || r.Visible != want || r.Covered || r.Focused {
		t.Fatalf("region = %+v, want rect %v fully visible", r, want)
	}
}

func TestNativeViewGoneWhenNotBuilt(t *testing.T) {
	show := true
	d := NewDriver(func(c *Context) {
		if show {
			NativeView(c, "web", layout.W(100), layout.H(100))
		}
	}, DriverOptions{Width: 200, Height: 200})
	d.Frame()
	if n := len(d.Runner().NativeRegions()); n != 1 {
		t.Fatalf("regions = %d, want 1", n)
	}
	show = false
	d.Frame()
	if n := len(d.Runner().NativeRegions()); n != 0 {
		t.Fatalf("regions after removal = %d, want 0", n)
	}
}

func TestNativeViewClippedByScroll(t *testing.T) {
	var sb *ScrollBox
	d := NewDriver(func(c *Context) {
		sb = Scroll(c, func() {
			Div(c, layout.H(150))
			NativeView(c, "web", layout.H(200))
			Div(c, layout.H(600))
		}, layout.H(300))
	}, DriverOptions{Width: 300, Height: 400})
	d.Frame()
	r := d.Runner().NativeRegions()[0]
	if r.Rect.Y != 150 || r.Rect.H != 200 {
		t.Fatalf("rect = %v", r.Rect)
	}
	if r.Visible.Y != 150 || r.Visible.Bottom() != 300 {
		t.Fatalf("visible = %v, want y 150..300 (clipped by the scroll box)", r.Visible)
	}
	sb.ScrollTo(0, 100)
	d.Frame()
	d.Frame()
	r = d.Runner().NativeRegions()[0]
	if r.Rect.Y != 50 || r.Visible.Y != 50 || r.Visible.Bottom() != 250 {
		t.Fatalf("after scrolling 100: rect %v visible %v", r.Rect, r.Visible)
	}
	sb.ScrollTo(0, 500)
	d.Frame()
	d.Frame()
	r = d.Runner().NativeRegions()[0]
	if !r.Visible.Empty() {
		t.Fatalf("scrolled out: visible = %v, want empty", r.Visible)
	}
}

func TestNativeViewCoveredByOverlay(t *testing.T) {
	open := false
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			b := Div(c, layout.W(100), layout.H(30))
			NativeView(c, "web", layout.W(300), layout.H(200))
			if open {
				Overlay(c, OverlayOptions{Anchor: b.ID(), Place: BottomStart}, func() {
					Div(c, layout.W(80), layout.H(80))
				})
			}
		})
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	if d.Runner().NativeRegions()[0].Covered {
		t.Fatal("covered without an overlay")
	}
	open = true
	d.Frame()
	if !d.Runner().NativeRegions()[0].Covered {
		t.Fatal("not covered by the dropdown overlapping it")
	}
}

func TestNativeViewTabFocus(t *testing.T) {
	var nv *NativeViewW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Button(c, "before")
			nv = NativeView(c, "web", layout.W(100), layout.H(100))
			Button(c, "after")
		})
	}, DriverOptions{Width: 300, Height: 300})
	d.Frame()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	d.Frame()
	if !nv.Focused() || !d.Runner().NativeRegions()[0].Focused {
		t.Fatal("Tab did not reach the native view")
	}
	// Enter stays with the native view: no click.
	d.Key(input.KeyEnter, 0)
	d.Frame()
	if nv.Clicked() {
		t.Fatal("Enter clicked the native view")
	}
}
