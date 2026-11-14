package ui

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func tabKeys(items []TabItem) []string {
	var ks []string
	for _, it := range items {
		ks = append(ks, it.Key)
	}
	return ks
}

// dragSteps presses at from, moves to to in steps (one frame each) and
// releases there, like Driver.Drag but leaving the last step to the caller
// (to look at the frame before the release).
func dragTo(d *Driver, from, to geom.Point, release bool) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from})
	d.Frame()
	const steps = 4
	for i := 1; i <= steps; i++ {
		f := float32(i) / steps
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)})
		d.Frame()
	}
	if release {
		d.Inject(input.Pointer{Kind: input.Release, Pos: to})
		d.Settle()
	}
}

func TestTabsDragReorderAndTearOff(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
		active := "a"
		var torn []tearOff
		page := func(c *Context) {
			tb := Tabs(c, &active, items).Closable().Draggable().OnTearOff(func(k string, p geom.Point) { torn = append(torn, tearOff{k, p}) }).Show()
			if it := tb.Reordered(); it != nil {
				items = it
			}
			if key, _, ok := tb.TornOff(); ok {
				items = slices.DeleteFunc(items, func(it TabItem) bool { return it.Key == key })
			}
		}
		d := NewDriver(page, DriverOptions{Width: 600, Height: 300, Theme: th})
		d.Frame()
		ra, _ := d.Find("Alpha")
		rc, _ := d.Find("Gamma")
		// Drag Alpha past the middle of Gamma: it goes last and stays active.
		dragTo(d, geom.Pt(ra.X+10, ra.Center().Y), geom.Pt(rc.Right()-4, rc.Center().Y), true)
		if got := tabKeys(items); !slices.Equal(got, []string{"b", "c", "a"}) || active != "a" {
			t.Fatalf("%v: reorder: %v active %q", th.Flavor, got, active)
		}
		// A short wobble is a click, not a drag.
		rb, _ := d.Find("Beta")
		d.Drag(rb.Center(), rb.Center().Add(geom.Pt(3, 0)))
		if got := tabKeys(items); !slices.Equal(got, []string{"b", "c", "a"}) || active != "b" {
			t.Fatalf("%v: click: %v active %q", th.Flavor, got, active)
		}
		// Released above the window: torn off.
		rg, _ := d.Find("Gamma")
		dragTo(d, rg.Center(), geom.Pt(rg.Center().X+20, -40), true)
		if len(torn) != 1 || torn[0].key != "c" || torn[0].pos.Y != -40 {
			t.Fatalf("%v: tear off: %+v", th.Flavor, torn)
		}
		if got := tabKeys(items); !slices.Equal(got, []string{"b", "a"}) {
			t.Fatalf("%v: after tear off: %v", th.Flavor, got)
		}
		// Escape during a drag cancels it.
		rb, _ = d.Find("Beta")
		ra, _ = d.Find("Alpha")
		dragTo(d, rb.Center(), geom.Pt(ra.Right()-2, ra.Center().Y), false)
		d.Key(input.KeyEscape, 0)
		d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(ra.Right()-2, ra.Center().Y)})
		d.Settle()
		if got := tabKeys(items); !slices.Equal(got, []string{"b", "a"}) {
			t.Fatalf("%v: cancelled drag moved: %v", th.Flavor, got)
		}
	}
}

func TestTabsDragKeyboard(t *testing.T) {
	items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
	active := "a"
	d := NewDriver(func(c *Context) {
		if it := Tabs(c, &active, items).Draggable().Show().Reordered(); it != nil {
			items = it
		}
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	d.ClickLabel("Alpha")
	d.Key(input.KeyRight, input.ModShift|shortcutMod())
	if got := tabKeys(items); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("move right: %v", got)
	}
	d.Key(input.KeyPageDown, input.ModShift|shortcutMod())
	if got := tabKeys(items); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("move to end: %v", got)
	}
	// Plain arrows still move the focus (and the selection).
	d.Key(input.KeyLeft, 0)
	if got := tabKeys(items); !slices.Equal(got, []string{"b", "c", "a"}) || active != "c" {
		t.Fatalf("roving: %v active %q", got, active)
	}
}

func TestTabsDragRTL(t *testing.T) {
	items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
	active := "a"
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			if it := Tabs(c, &active, items).Draggable().Show().Reordered(); it != nil {
				items = it
			}
		})
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	ra, _ := d.Find("Alpha")
	rb, _ := d.Find("Beta")
	if ra.X < rb.X {
		t.Fatal("not right to left")
	}
	// In RTL the logical end is on the left: past Beta's middle (to its left).
	dragTo(d, ra.Center(), geom.Pt(rb.X+4, rb.Center().Y), true)
	if got := tabKeys(items); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("rtl reorder: %v", got)
	}
}
