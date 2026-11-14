package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestTourPerStepOptions(t *testing.T) {
	th := theme.AntDesign()
	tk := th.Components.Tour
	open, cur := true, 0
	var ids [3]ID
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			ids[0] = Button(c, "A").ID()
			ids[1] = Button(c, "B").ID()
			ids[2] = Button(c, "C").ID()
		}, layout.Pad(40), layout.Gap(8))
		Tour(c, &open, &cur, []TourStep{
			{Target: ids[0], Title: "one", NextText: "Go on"},
			{Target: ids[1], Title: "two", Mask: TourMaskOff, Type: TourTypePrimary, HidePrev: true},
			{Target: ids[2], Title: "three", PrevText: "Back"},
		}).Show()
	}, DriverOptions{Width: 800, Height: 600, Theme: th})
	d.Settle()
	d.Settle()
	if t2bCount(d, tk.MaskColor) == 0 {
		t.Fatal("step 1 has no mask")
	}
	if t2bCount(d, tk.PrimaryBg) != 0 && tk.PrimaryBg != th.Primary.Base {
		t.Fatal("step 1 is primary")
	}
	if err := d.ClickLabel("Go on"); err != nil {
		t.Fatal("custom next text: ", err)
	}
	if cur != 1 {
		t.Fatalf("cur %d", cur)
	}
	d.Settle()
	if t2bCount(d, tk.MaskColor) != 0 {
		t.Fatal("step 2 overrides the mask off")
	}
	if t2bCount(d, tk.PrimaryBg) == 0 {
		t.Fatal("step 2 is not the primary panel")
	}
	if _, ok := d.Find(ZhCN.Tour.Prev); ok {
		t.Fatal("step 2 hides the previous button")
	}
	d.ClickLabel(ZhCN.Tour.Next)
	if _, ok := d.Find("Back"); !ok {
		t.Fatal("custom previous text missing")
	}
}

func TestTourRenders(t *testing.T) {
	open, cur := true, 0
	closed := false
	d := NewDriver(func(c *Context) {
		tw := Tour(c, &open, &cur, []TourStep{{Title: "one"}, {Title: "two"}, {Title: "three"}}).
			IndicatorsRender(func(current, total int) {
				Text(c, "step "+itoa64(int64(current+1))+" of "+itoa64(int64(total)))
			}).
			ActionsRender(func(a *TourActions) {
				if Button(c, "Last").Clicked() {
					a.GoTo(a.Total - 1)
				}
				if Button(c, "Quit").Clicked() {
					a.Close()
				}
				a.Default()
			}).Show()
		closed = closed || tw.Closed()
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	if _, ok := d.FindText("step 1 of 3"); !ok {
		t.Fatal("custom indicators missing")
	}
	if _, ok := d.Find(ZhCN.Tour.Next); !ok {
		t.Fatal("Default() did not build the buttons")
	}
	d.ClickLabel("Last")
	if cur != 2 {
		t.Fatalf("GoTo: cur %d", cur)
	}
	if _, ok := d.FindText("step 3 of 3"); !ok {
		t.Fatal("indicators not updated")
	}
	d.ClickLabel("Quit")
	if open || !closed {
		t.Fatal("Close did not close the tour")
	}
}

func TestTourScrollsTargetIntoView(t *testing.T) {
	for _, reveal := range []bool{true, false} {
		open, cur := false, 0
		var target ID
		var sb *ScrollBox
		d := NewDriver(func(c *Context) {
			sb = Scroll(c, func() {
				Column(c, func() {}, layout.H(900))
				target = Button(c, "Far").ID()
				Column(c, func() {}, layout.H(400))
			}, layout.H(300))
			Tour(c, &open, &cur, []TourStep{{Title: "start"}, {Target: target, Title: "far"}}).ScrollIntoView(reveal).Show()
		}, DriverOptions{Width: 600, Height: 400})
		d.Settle()
		if _, y := sb.Offset(); y != 0 {
			t.Fatal("scrolled before the tour")
		}
		open = true
		d.Settle()
		d.ClickLabel(ZhCN.Tour.Next)
		d.Settle()
		d.Settle()
		_, y := sb.Offset()
		if scrolled := y > 600; scrolled != reveal {
			t.Fatalf("reveal %v: scroll offset %v", reveal, y)
		}
		if reveal {
			if r, ok := d.Find("Far"); !ok || r.Y < 0 || r.Bottom() > 300 {
				t.Fatalf("target not visible: %v", r)
			}
		}
	}
}
