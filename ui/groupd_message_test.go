package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

// gdMessages opens the messages built by open once and settles.
func gdMessages(th *theme.Theme, open func(c *Context)) *Driver {
	shown := false
	d := NewDriver(func(c *Context) {
		if !shown {
			shown = true
			open(c)
		}
	}, DriverOptions{Width: 800, Height: 600, Theme: th})
	d.Settle()
	return d
}

func cxOf(r geom.Rect) float32 { return r.X + r.W/2 }

func TestMessagePlacementAndOffset(t *testing.T) {
	d := gdMessages(nil, func(c *Context) {
		Message(c, "info", "TopDefault").Duration(0)
		Message(c, "info", "TopOffset").Placement(MessageTopStart).Offset(60).Duration(0)
		Message(c, "info", "BottomEnd").Placement(MessageBottomEnd).Duration(0)
		Message(c, "info", "BottomCenter").Placement(MessageBottom).Offset(40).Duration(0)
	})
	top, ok1 := d.FindText("TopDefault")
	ts, ok2 := d.FindText("TopOffset")
	be, ok3 := d.FindText("BottomEnd")
	bc, ok4 := d.FindText("BottomCenter")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		t.Fatal("messages missing")
	}
	if top.Y > 40 || cxOf(top) < 300 || cxOf(top) > 500 {
		t.Errorf("default message not at the top center: %v", top)
	}
	if ts.X > 120 || ts.Y < 60 || ts.Y > 100 {
		t.Errorf("top-start message with offset 60 at %v", ts)
	}
	if be.X < 500 || be.Y < 500 {
		t.Errorf("bottom-end message at %v", be)
	}
	if bc.Y > 600-40 || bc.Y < 450 || cxOf(bc) < 300 || cxOf(bc) > 500 {
		t.Errorf("bottom message with offset 40 at %v", bc)
	}
	// RTL: the end side is the left.
	rtl := gdMessages(nil, func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Message(c, "info", "RtlEnd").Placement(MessageTopEnd).Duration(0)
		})
	})
	if r, ok := rtl.FindText("RtlEnd"); !ok || r.X > 200 {
		t.Errorf("RTL top-end message at %v", r)
	}
	// shadcn: Sonner stacks follow the placement too.
	sh := gdMessages(theme.Shadcn(), func(c *Context) {
		Message(c, "info", "SonnerDefault").Duration(0)
		Message(c, "info", "SonnerTop").Placement(MessageTop).Duration(0)
	})
	sd, _ := sh.FindText("SonnerDefault")
	st, _ := sh.FindText("SonnerTop")
	if sd.Y < 400 || st.Y > 200 {
		t.Errorf("shadcn placements: default %v, top %v", sd, st)
	}
}

func TestMessageShowClose(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := gdMessages(th, func(c *Context) {
			Message(c, "success", "Closable").Duration(0).ShowClose()
		})
		if len(d.r.c.toasts().msgs) != 1 {
			t.Fatal("message not queued")
		}
		// Hover first: a Sonner stack expands under the pointer.
		r, ok := d.Find(ZhCN.Close)
		if !ok {
			t.Fatalf("%s: no close button", th.Name)
		}
		d.Move(r.Center())
		d.Settle()
		if err := d.ClickLabel(ZhCN.Close); err != nil {
			t.Fatalf("%s: %v", th.Name, err)
		}
		d.Advance(time.Second)
		d.Settle()
		if n := len(d.r.c.toasts().msgs); n != 0 {
			t.Fatalf("%s: close button did not dismiss: %d", th.Name, n)
		}
	}
}

func TestMessagePlain(t *testing.T) {
	th := theme.ElementPlus()
	tk := th.Components.Message
	d := gdMessages(th, func(c *Context) { Message(c, "success", "Plain").Duration(0).Plain() })
	if t2bCount(d, tk.PlainBg) == 0 {
		t.Fatal("plain message does not use PlainBg")
	}
	if t2bCount(d, th.Success.Bg) != 0 {
		t.Fatal("plain message still tinted")
	}
	r, _ := d.FindText("Plain")
	if col, ok := gdTextColor(d, r.Expand(2)); !ok || col != tk.PlainColor {
		t.Fatalf("plain text color %v", col)
	}
}
