package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// g4Dir wraps fn in a nested provider with direction dir (nested so the
// root window theme is left alone).
func g4Dir(c *Context, dir Direction, fn func()) {
	Column(c, func() { ConfigProvider(c).Direction(dir).Body(fn) }, layout.Pad(20))
}

// g4TextIn returns the origin x of the first text run drawn inside r.
func g4TextIn(d *Driver, r geom.Rect) (float32, bool) {
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && r.Contains(geom.Pt(t.Origin.X+1, t.Origin.Y+1)) {
			return t.Origin.X, true
		}
	}
	return 0, false
}

// g4Fills returns the FillRect ops of color col.
func g4Fills(d *Driver, col theme.Color) []geom.Rect {
	var out []geom.Rect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == col {
			out = append(out, f.Rect)
		}
	}
	return out
}

// TestG4TagCloseRTL: the close icon (and its hit area) of a closable tag
// sits at the inline end, the left edge in RTL.
func TestG4TagCloseRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		var tag *TagW
		closed := false
		dir := LTR
		if rtl {
			dir = RTL
		}
		d := NewDriver(func(c *Context) {
			g4Dir(c, dir, func() {
				tag = Tag(c, "Closable tag").Closable()
				if tag.Closed() {
					closed = true
				}
			})
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[tag.n.id]
		x, ok := g4TextIn(d, r)
		if !ok {
			t.Fatal("label not painted")
		}
		// The label runs from the start; the close icon follows at the end:
		// the free space before the label is the 8px start padding.
		if rtl && x < r.X+18 || !rtl && x > r.X+10 {
			t.Errorf("rtl=%v: label at x=%v not at the start of %+v", rtl, x, r)
		}
		// Clicking the start edge does not close; the end edge does.
		start, end := r.X+4, r.Right()-6
		if rtl {
			start, end = r.Right()-4, r.X+6
		}
		d.Click(geom.Pt(start, r.Y+r.H/2))
		d.Settle()
		if closed {
			t.Errorf("rtl=%v: click at the start closed the tag", rtl)
		}
		d.Click(geom.Pt(end, r.Y+r.H/2))
		d.Settle()
		if !closed {
			t.Errorf("rtl=%v: click on the close icon did not close", rtl)
		}
	}
}

// TestG4BadgeTopEndRTL: the count bubble sits on the top-end corner.
func TestG4BadgeTopEndRTL(t *testing.T) {
	var box *Box
	th := theme.AntDesign()
	d := NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() {
			box = Badge(c).Count(5).Body(func() { Avatar(c, "U").Square() })
		})
	}, DriverOptions{Width: 300, Height: 200, Theme: th})
	d.Settle()
	r := d.r.c.rects[box.n.id]
	fills := g4Fills(d, th.Error.Base)
	if len(fills) == 0 {
		t.Fatal("no bubble painted")
	}
	b := fills[0]
	if cx := b.X + b.W/2; cx < r.X-2 || cx > r.X+2 {
		t.Errorf("bubble centre %v, want the left edge %v of %+v", cx, r.X, r)
	}
}

// TestG4ProgressFillsFromStartRTL: a line progress fills from the right
// in RTL, with the percentage on the left.
func TestG4ProgressFillsFromStartRTL(t *testing.T) {
	th := theme.AntDesign()
	var p *ProgressW
	d := NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() { p = Progress(c, 40).Layout(layout.W(300)) })
	}, DriverOptions{Width: 400, Height: 200, Theme: th})
	d.Settle()
	r := d.r.c.rects[p.n.id]
	fills := g4Fills(d, th.Primary.Base)
	if len(fills) != 1 {
		t.Fatalf("fills %v", fills)
	}
	trail := g4Fills(d, th.FillSecondary)
	if len(trail) != 1 || trail[0].Right() != r.Right() {
		t.Fatalf("trail %v not at the right of %+v", trail, r)
	}
	if f := fills[0]; f.Right() != r.Right() || f.W < trail[0].W*0.39 || f.W > trail[0].W*0.41 {
		t.Errorf("fill %+v does not grow from the right edge of %+v", f, r)
	}
	if x, ok := g4TextIn(d, r); !ok || x > trail[0].X {
		t.Errorf("label at x=%v not left of the bar %+v", x, trail[0])
	}
}

// TestG4DrawerSidesRTL: the default (end) drawer opens on the left in RTL;
// DrawerStart on the right; DrawerRight stays physical.
func TestG4DrawerSidesRTL(t *testing.T) {
	for _, tc := range []struct {
		place DrawerPlacement
		set   bool
		left  bool
	}{{0, false, true}, {DrawerEnd, true, true}, {DrawerStart, true, false}, {DrawerRight, true, false}, {DrawerLeft, true, true}} {
		open := true
		d := NewDriver(func(c *Context) {
			g4Dir(c, RTL, func() {
				dr := Drawer(c, &open, "Side")
				if tc.set {
					dr.Placement(tc.place)
				}
				dr.Body(func() { Text(c, "drawer body") })
			})
		}, DriverOptions{Width: 1000, Height: 600})
		d.Settle()
		r, ok := d.FindText("drawer body")
		if !ok {
			t.Fatal("drawer not shown")
		}
		if left := r.X < 500; left != tc.left {
			t.Errorf("placement %v (set %v): body at %+v, want left=%v", tc.place, tc.set, r, tc.left)
		}
	}
}

// TestG4DrawerSlidesTowardsItsEdge: while opening, the RTL default drawer
// slides in from the left edge.
func TestG4DrawerSlidesTowardsItsEdge(t *testing.T) {
	open := false
	d := NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() {
			Drawer(c, &open, "Side").Body(func() { Text(c, "drawer body") })
		})
	}, DriverOptions{Width: 1000, Height: 600, Animations: true})
	d.Settle()
	open = true
	d.Frame()
	d.Advance(60e6)
	d.Frame()
	var mid geom.Affine
	found := false
	for _, op := range d.r.List().Ops {
		if x, ok := op.(paint.PushTransform); ok && x.M.E != 0 && x.M.F == 0 {
			mid, found = x.M, true
		}
	}
	if !found || mid.E >= 0 {
		t.Errorf("opening RTL end drawer: transform %+v (found %v), want a negative x shift", mid, found)
	}
}

// TestG4NotificationEndCornerRTL: the default notification opens in the
// top-left corner in RTL; NotifyTopRight stays physical.
func TestG4NotificationEndCornerRTL(t *testing.T) {
	d := NewDriver(func(c *Context) {
		s := State[bool](c, hashID("g4-fire"), nil)
		g4Dir(c, RTL, func() {
			if !*s {
				*s = true
				Notification(c, "info", "End note").Duration(0)
				Notification(c, "info", "Right note").Placement(NotifyTopRight).Duration(0)
			}
		})
	}, DriverOptions{Width: 1000, Height: 600})
	d.Settle()
	e, ok1 := d.FindText("End note")
	r, ok2 := d.FindText("Right note")
	if !ok1 || !ok2 {
		t.Fatal("notes not shown")
	}
	if e.X > 500 || r.X < 500 {
		t.Errorf("end note %+v should be left, right note %+v right", e, r)
	}
	// The card content mirrors: the status icon sits right of the title.
	ic, ok := d.FindText("info")
	if !ok {
		t.Fatal("status icon not found")
	}
	if ic.X < e.Right() {
		t.Errorf("icon %+v not at the start (right) of title %+v", ic, e)
	}
}

// TestG4MessageKeepsCallerScope: a message fired inside a dark RTL
// provider renders dark and right to left although the global layer is
// built outside the provider.
func TestG4MessageKeepsCallerScope(t *testing.T) {
	light := theme.AntDesign()
	dark := theme.AntDesign(theme.Dark())
	d := NewDriver(func(c *Context) {
		s := State[bool](c, hashID("g4-fire"), nil)
		Column(c, func() {
			ConfigProvider(c).Dark(true).Direction(RTL).Body(func() {
				if !*s {
					*s = true
					Message(c, "success", "Scoped message").Duration(0)
				}
			})
		})
	}, DriverOptions{Width: 600, Height: 300, Theme: light})
	d.Settle()
	txt, ok := d.FindText("Scoped message")
	if !ok {
		t.Fatal("message not shown")
	}
	bg := g4Fills(d, dark.Components.Message.ContentBg)
	inside := false
	for _, f := range bg {
		if f.Contains(geom.Pt(txt.X+txt.W/2, txt.Y+txt.H/2)) {
			inside = true
		}
	}
	if !inside {
		t.Errorf("message not on the dark background %v (fills %v)", dark.Components.Message.ContentBg, bg)
	}
	ic, ok := d.FindText("success")
	if !ok || ic.X < txt.Right() {
		t.Errorf("status icon %+v not right of the text %+v (RTL)", ic, txt)
	}
}

// TestG4PopoverLogicalPlacement: "left" opens at the inline start, on the
// right of the trigger in RTL.
func TestG4PopoverLogicalPlacement(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		dir := LTR
		if rtl {
			dir = RTL
		}
		var anchor *Box
		d := NewDriver(func(c *Context) {
			g4Dir(c, dir, func() {
				Column(c, func() {
					o := true
					anchor = Popover(c).Text("pop content").Placement("left").Trigger("manual").Open(&o).Body(func() { Text(c, "trigger") })
				}, layout.Pad(200))
			})
		}, DriverOptions{Width: 800, Height: 600})
		d.Settle()
		d.Settle()
		a := d.r.c.rects[anchor.n.id]
		p, ok := d.FindText("pop content")
		if !ok {
			t.Fatalf("rtl=%v: popover not shown", rtl)
		}
		if rtl && p.X < a.Right() || !rtl && p.Right() > a.X {
			t.Errorf("rtl=%v: panel text %+v vs anchor %+v", rtl, p, a)
		}
	}
	// The resolved side is reported for the arrow.
	var side string
	NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() { _, side = c.popPlacement("leftTop") })
	}, DriverOptions{}).Frame()
	if side != "right" {
		t.Errorf("leftTop in RTL resolves to %q", side)
	}
}

// TestG4CarouselKeysRTL: with the frame focused, Left shows the next slide
// in RTL.
func TestG4CarouselKeysRTL(t *testing.T) {
	cur := 0
	var box *Box
	d := NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() {
			box = Carousel(c, 3, func(i int) { Text(c, "slide") }).Dots(false).Current(&cur).Show()
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	r := d.r.c.rects[box.n.id]
	d.Click(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
	d.Settle()
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if cur != 1 {
		t.Fatalf("Left in RTL: slide %d, want 1", cur)
	}
	d.Key(input.KeyRight, 0)
	d.Settle()
	if cur != 0 {
		t.Fatalf("Right in RTL: slide %d, want 0", cur)
	}
}

// TestG4TimelineStartRTL: the default timeline puts the axis at the right
// and the content on its left, right-aligned against it.
func TestG4TimelineStartRTL(t *testing.T) {
	var w *TimelineW
	d := NewDriver(func(c *Context) {
		g4Dir(c, RTL, func() {
			w = Timeline(c, []TimelineItem{{Content: "first event"}, {Content: "second"}}).Show().Layout(layout.W(300))
		})
	}, DriverOptions{Width: 500, Height: 300})
	d.Settle()
	r := d.r.c.rects[w.box.n.id]
	txt, ok := d.FindText("first event")
	if !ok {
		t.Fatal("no content")
	}
	// Axis (10px) + gap (16px) at the right edge.
	if txt.Right() < r.Right()-30 || txt.Right() > r.Right()-20 {
		t.Errorf("content %+v not right-aligned next to the axis at the right of %+v", txt, r)
	}
}
