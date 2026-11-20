package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

func TestDropdownOpenFadesFromTransparent(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Dropdown(c, []MenuItem{{Key: "profile", Label: "Profile"}}).Trigger(TriggerClick).Button("Actions")
	}, DriverOptions{Width: 400, Height: 300, Animations: true})
	d.Frame()
	r, ok := d.Find("dropdown-more")
	if !ok {
		t.Fatal("missing dropdown trigger")
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: r.Center()})

	d.Inject(input.Pointer{Kind: input.Press, Pos: r.Center()})
	d.Inject(input.Pointer{Kind: input.Release, Pos: r.Center()})
	d.Frame()
	if _, ok := d.Find("Profile"); !ok {
		t.Fatal("click did not open the dropdown")
	}
	alpha := func() float32 {
		for _, op := range d.Runner().list.Ops {
			if fade, ok := op.(paint.PushOpacity); ok {
				return fade.Alpha
			}
		}
		return 1 // no opacity group means fully opaque
	}
	first := alpha()
	d.Advance(16 * time.Millisecond)
	d.Frame()
	next := alpha()
	if first > 0.01 || next <= first || next >= 1 {
		t.Fatalf("menu must fade in without an opaque flash: first alpha=%g, next alpha=%g", first, next)
	}
	d.Settle()
	if a := alpha(); a != 1 {
		t.Fatalf("settled menu alpha=%g, want 1", a)
	}
}
