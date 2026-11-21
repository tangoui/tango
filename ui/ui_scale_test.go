package ui

import (
	"image"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestUIScaleWindowBoundaries(t *testing.T) {
	var size geom.Size
	var pixels float32
	var clicks int
	var value string
	d := NewDriver(func(c *Context) {
		size, pixels = c.Size(), c.PixelScale()
		Column(c, func() {
			if Button(c, "Action").Clicked() {
				clicks++
			}
			Input(c, &value).A11yLabel("Query")
			NativeView(c, "web", layout.H(20)).A11yLabel("Web page")
		}, layout.Pad(40))
	}, DriverOptions{Width: 401, Height: 301, Scale: 1.5, Accessibility: true})
	r := d.Runner()
	d.Frame()
	button, _ := d.Find("Action")
	view := r.NativeRegions()[0]
	query, ok := d.A11yFind(a11y.RoleTextbox, "Query")
	if !ok {
		t.Fatal("missing query")
	}
	d.Click(geom.Pt(query.Bounds.X+10, query.Bounds.Y+10))
	on, caret := d.TextInputActive()
	if !on || caret.Empty() {
		t.Fatalf("query didn't report its caret: %v %v", on, caret)
	}

	r.SetUIScale(1.25)
	frame := d.Frame()
	if size != (geom.Size{W: 320.8, H: 240.8}) {
		t.Errorf("layout viewport = %v", size)
	}
	if pixels != 1.875 {
		t.Errorf("pixels per UI unit = %v", pixels)
	}
	if got := frame.Bounds().Size(); got != image.Pt(602, 452) {
		t.Errorf("framebuffer size = %v", got)
	}
	if got := r.NativeRegions()[0].Rect; got.Y != view.Rect.Y*1.25 || got.H != 25 {
		t.Errorf("native placement = %v, original %v", got, view.Rect)
	}
	scaled, ok := d.A11yFind(a11y.RoleTextbox, "Query")
	if !ok || scaled.Bounds.Y != query.Bounds.Y*1.25 || scaled.Bounds.H != query.Bounds.H*1.25 {
		t.Errorf("query bounds = %+v, original %v", scaled, query.Bounds)
	}
	if on, got := d.TextInputActive(); !on || got.Y != caret.Y*1.25 || got.H != caret.H*1.25 {
		t.Errorf("IME caret = %v, original %v", got, caret)
	}

	d.Click(geom.Pt(51, button.Center().Y*1.25))
	if clicks != 1 {
		t.Errorf("physical button click count = %d", clicks)
	}

	r.SetUIScale(1)
	// The previous frame is still visible until d.Click advances the runner.
	d.Click(geom.Pt(51, button.Center().Y*1.25))
	if clicks != 2 || size != (geom.Size{W: 401, H: 301}) {
		t.Errorf("click during scale transition: clicks %d, viewport %v", clicks, size)
	}
}

func TestUIScaleOverlayAndTouch(t *testing.T) {
	var opened bool
	var picks int
	d := NewDriver(func(c *Context) {
		b := Button(c, "Open")
		if b.Clicked() {
			opened = true
		}
		if opened {
			Overlay(c, OverlayOptions{Anchor: b.ID(), Place: BottomStart, Offset: 4}, func() {
				if Button(c, "Pick").Clicked() {
					picks++
				}
			})
		}
	}, DriverOptions{Width: 320, Height: 200})
	d.Runner().SetUIScale(1.25)
	d.Frame()
	anchor, ok := d.Find("Open")
	if !ok {
		t.Fatal("missing trigger")
	}
	d.Tap(anchor.Center())
	popup, ok := d.Find("Pick")
	if !ok || popup.Y < anchor.Bottom()+5 {
		t.Fatalf("popup %v, anchor %v", popup, anchor)
	}
	d.Tap(popup.Center())
	if picks != 1 {
		t.Errorf("touch selected %d popup items", picks)
	}
}

func TestUIScaleKeepsIntegerFramebufferExtent(t *testing.T) {
	d := NewDriver(func(*Context) {}, DriverOptions{Width: 1412.8, Height: 32, Scale: 1.25})
	d.Runner().SetUIScale(1.66)
	if got := d.Frame().Bounds().Size(); got != image.Pt(1766, 40) {
		t.Errorf("fractional UI scale changed framebuffer size: %v", got)
	}
}
