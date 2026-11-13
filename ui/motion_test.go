package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// motionShot saves the driver's frame when TANGO_MOTION_SHOTS names a
// directory (manual inspection of transition frames).
func motionShot(t *testing.T, d *Driver, name string) {
	dir := os.Getenv("TANGO_MOTION_SHOTS")
	if dir == "" {
		return
	}
	if err := d.SavePNG(filepath.Join(dir, name+".png")); err != nil {
		t.Fatal(err)
	}
}

func rectNear(a, b geom.Rect, eps float32) bool {
	d := func(x, y float32) bool { return x-y < eps && y-x < eps }
	return d(a.X, b.X) && d(a.Y, b.Y) && d(a.W, b.W) && d(a.H, b.H)
}

func TestBoxTransformHitTest(t *testing.T) {
	var box *Box
	clicks := 0
	page := func(c *Context) {
		box = Div(c, layout.W(200), layout.H(100)).Bg(c.Theme.Primary.Base)
		box.Scale(0.5) // drawn as 100×50 about the centre (50..150, 25..75)
		if box.Clicked() {
			clicks++
		}
	}
	d := NewDriver(page, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	if r := box.Rect(); !rectNear(r, geom.R(50, 25, 100, 50), 0.01) {
		t.Fatalf("transformed rect %v", r)
	}
	d.Click(geom.Pt(20, 10)) // inside the layout box, outside the drawn one
	if clicks != 0 {
		t.Fatal("click outside the transformed box hit it")
	}
	d.Click(geom.Pt(60, 30))
	if clicks != 1 {
		t.Fatal("click inside the transformed box missed")
	}
	// The pixel at the layout corner stays background; the centre is filled.
	img := d.Frame()
	if c := img.RGBAAt(10, 10); c.B > 200 && c.R < 100 {
		t.Fatalf("corner painted %v", c)
	}
	if c := img.RGBAAt(100, 50); !(c.B > 200 && c.R < 100) {
		t.Fatalf("centre not painted %v", c)
	}
}

func TestBoxTransformIdentityIsExact(t *testing.T) {
	var box *Box
	page := func(c *Context) {
		box = Div(c, layout.W(120), layout.H(40)).Transform(geom.Identity())
	}
	d := NewDriver(page, DriverOptions{Width: 200, Height: 100})
	d.Frame()
	if box.n.xf != nil || box.Rect() != geom.R(0, 0, 120, 40) {
		t.Fatalf("identity transform should be a no-op: %v", box.Rect())
	}
}

// modalRect returns the dialog panel's screen rect (the overlay above the
// mask) from the last frame.
func modalRect(d *Driver) (geom.Rect, bool) {
	c := d.r.c
	if len(c.overlays) < 2 {
		return geom.Rect{}, false
	}
	r, ok := c.rects[c.overlays[len(c.overlays)-1].root.id]
	return r, ok
}

func TestModalZoomFromClick(t *testing.T) {
	for _, flavor := range []string{"ant", "element"} {
		open := false
		page := func(c *Context) {
			Column(c, func() {
				Gap(c, 300)
				if Button(c, "打开").Clicked() {
					open = true
				}
			}, layout.Pad(24))
			Modal(c, &open, "缩放标题").Body(func() { Text(c, "对话框正文：圆角、阴影与文字一起缩放。") })
		}
		th := theme.AntDesign()
		if flavor == "element" {
			th = theme.ElementPlus()
		}
		d := NewDriver(page, DriverOptions{Width: 800, Height: 520, Animations: true, Theme: th})
		d.Frame()
		btn, ok := d.Find("打开")
		if !ok {
			t.Fatal("no button")
		}
		p := btn.Center()
		d.Inject(input.Pointer{Kind: input.Move, Pos: p})
		d.Inject(input.Pointer{Kind: input.Press, Pos: p})
		d.Inject(input.Pointer{Kind: input.Release, Pos: p})
		d.Frame()
		d.Advance(time.Duration(th.DurationSlow) / 15)
		d.Frame()
		motionShot(t, d, "Modal-early-"+flavor)
		d.Advance(time.Duration(th.DurationSlow)/3 - time.Duration(th.DurationSlow)/15)
		d.Frame()
		mid, ok := modalRect(d)
		if !ok {
			t.Fatal("modal not shown")
		}
		motionShot(t, d, "Modal-mid-"+flavor)
		d.Settle()
		full, _ := modalRect(d)
		motionShot(t, d, "Modal-open-"+flavor)
		if flavor == "ant" {
			if !(mid.W < full.W*0.95 && mid.W > full.W*0.2) {
				t.Fatalf("mid-zoom width %v vs %v", mid.W, full.W)
			}
			// Zooming about the click: the click point divides the panel in
			// the same proportions in the middle frame as at rest.
			fx := (p.X - full.X) / full.W
			mx := (p.X - mid.X) / mid.W
			if mx-fx > 0.02 || fx-mx > 0.02 {
				t.Fatalf("zoom origin not at click: %.3f vs %.3f", mx, fx)
			}
		} else if !(mid.W == full.W && mid.Y < full.Y) {
			t.Fatalf("element: mid %v full %v", mid, full)
		}
		// Static state is exact: the panel is 520 (Ant) wide, no transform left.
		if c := d.r.c; c.overlays[len(c.overlays)-1].root.xf != nil {
			t.Fatal("transform left after the enter transition")
		}
		// Leave: still drawn mid-way, gone afterwards.
		d.Inject(input.KeyEvent{Key: input.KeyEscape, Down: true})
		d.Frame()
		d.Advance(time.Duration(th.DurationSlow) / 3)
		d.Frame()
		if open {
			t.Fatal("escape did not close")
		}
		if _, ok := modalRect(d); !ok {
			t.Fatal("modal vanished without a leave transition")
		}
		motionShot(t, d, "Modal-leave-"+flavor)
		d.Settle()
		if len(d.r.c.overlays) != 0 {
			t.Fatalf("overlays left after leave: %d", len(d.r.c.overlays))
		}
	}
}

func TestDrawerSlidesByTransform(t *testing.T) {
	open := false
	var hits int
	page := func(c *Context) {
		if Button(c, "抽屉").Clicked() {
			open = true
		}
		Drawer(c, &open, "抽屉标题").Body(func() {
			if Button(c, "里面").Clicked() {
				hits++
			}
		})
	}
	d := NewDriver(page, DriverOptions{Width: 800, Height: 500, Animations: true})
	d.Frame()
	d.ClickLabel("抽屉") // settles
	panel := func() geom.Rect {
		r, _ := d.r.c.RectOf(d.r.c.overlays[len(d.r.c.overlays)-1].root.kids[0].id)
		return r
	}
	if r := panel(); r != geom.R(800-378, 0, 378, 500) {
		t.Fatalf("open drawer rect %v", r)
	}
	d.ClickLabel("里面")
	if hits != 1 {
		t.Fatal("drawer content not clickable at rest")
	}
	open = false
	d.Frame()
	d.Advance(150 * time.Millisecond)
	d.Frame()
	motionShot(t, d, "Drawer-leave")
	if r := panel(); !(r.X > 800-378 && r.X < 800) || r.W < 377.9 || r.W > 378.1 {
		t.Fatalf("mid-leave rect %v", r)
	}
	d.Settle()
	if len(d.r.c.overlays) != 0 {
		t.Fatal("drawer still shown")
	}
}
