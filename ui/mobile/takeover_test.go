package mobile

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// fieldList is a ScrollView of input rows (ui.Input inside each).
func fieldList(sv **ScrollViewW, vals []string) func(c *ui.Context) {
	return func(c *ui.Context) {
		*sv = ScrollView(c).Layout(layout.H(400))
		(*sv).Body(func() {
			for i := range vals {
				ui.Key(c, i, func() {
					ui.Row(c, func() { Field(c, &vals[i]) }, layout.H(50))
				})
			}
		})
	}
}

func TestScrollViewDragStartingOnInput(t *testing.T) {
	for _, touch := range []bool{false, true} {
		var sv *ScrollViewW
		vals := make([]string, 30)
		d := phone(fieldList(&sv, vals), false)
		d.Frame()
		d.Frame()
		from, to := geom.Pt(150, 125), geom.Pt(150, 25)
		if touch {
			d.TouchDrag(from, to)
		} else {
			d.Drag(from, to)
		}
		if off := sv.ScrollOffset(); off < 50 {
			t.Fatalf("touch=%v: drag from an input scrolled to %v", touch, off)
		}
		// The input lost the press: no focus, no keyboard.
		if on, _ := d.TextInputActive(); on {
			t.Fatalf("touch=%v: input focused by a scrolling drag", touch)
		}
	}
}

func TestScrollViewTapOnInputStillFocuses(t *testing.T) {
	var sv *ScrollViewW
	vals := make([]string, 30)
	d := phone(fieldList(&sv, vals), false)
	d.Frame()
	d.Frame()
	d.Tap(geom.Pt(150, 125))
	if on, _ := d.TextInputActive(); !on {
		t.Fatal("tap on an input inside a ScrollView did not focus it")
	}
	d.Type("hi")
	if vals[2] != "hi" || sv.ScrollOffset() != 0 {
		t.Fatalf("typed %q, offset %v", vals[2], sv.ScrollOffset())
	}
}

func TestSwipeCellDragStartingOnInput(t *testing.T) {
	var sw *SwipeCellW
	var sv *ScrollViewW
	val := ""
	d := phone(func(c *ui.Context) {
		sv = ScrollView(c).Layout(layout.H(600))
		sv.Body(func() {
			sw = SwipeCell(c).Right(SwipeAction{Key: "del", Text: "Delete", Color: "danger"})
			sw.Body(func() {
				ui.Row(c, func() { Field(c, &val) }, layout.H(50))
			})
			for i := 0; i < 30; i++ {
				ui.Key(c, i, func() { Cell(c, "row") })
			}
		})
	}, false)
	d.Frame()
	d.Frame()
	// Horizontal: the swipe cell opens.
	d.TouchDrag(geom.Pt(300, 25), geom.Pt(150, 27))
	if !sw.IsOpen() || sv.ScrollOffset() != 0 {
		t.Fatalf("horizontal drag on the input: open=%v offset=%v", sw.IsOpen(), sv.ScrollOffset())
	}
	sw.Close()
	d.Settle()
	// Vertical on the same input: the list scrolls, the cell stays shut.
	d.TouchDrag(geom.Pt(150, 25), geom.Pt(152, -300))
	if sw.IsOpen() || sv.ScrollOffset() < 50 {
		t.Fatalf("vertical drag on the input: open=%v offset=%v", sw.IsOpen(), sv.ScrollOffset())
	}
}

func TestAreaPinchFromTouches(t *testing.T) {
	var pinches []Pinch
	d := phone(func(c *ui.Context) {
		b := ui.Div(c, layout.Size(390, 600))
		g := Area(c, b, AxisBoth, nil)
		if g.Pinch.Active {
			pinches = append(pinches, g.Pinch)
		}
	}, false)
	d.Frame()
	d.Pinch(geom.Pt(200, 300), 100, 250)
	if len(pinches) == 0 {
		t.Fatal("no pinch reported")
	}
	last := pinches[len(pinches)-1]
	if math.Abs(float64(last.Scale)-2.5) > 0.01 || last.Center != geom.Pt(200, 300) {
		t.Fatalf("pinch %+v", last)
	}
	pinches = nil
	d.TwoFingerPan(geom.Pt(200, 300), geom.Pt(200, 400), 80)
	last = pinches[len(pinches)-1]
	if math.Abs(float64(last.Scale)-1) > 0.01 || last.Pan != geom.Pt(0, 100) {
		t.Fatalf("two-finger pan %+v", last)
	}
	d.Frame()
	pinches = nil
	d.Frame()
	if len(pinches) != 0 {
		t.Fatal("pinch still active after the fingers lifted")
	}
}

func TestSafeAreaFromWindow(t *testing.T) {
	var in geom.Insets
	var bottom ui.ID
	d := phone(func(c *ui.Context) {
		Provider(c, Options{}, func() {
			in = SafeAreaInsets(c)
			bottom = SafeArea(c, Bottom).ID()
		})
	}, false)
	d.SetSafeArea(geom.Insets{Top: 47, Bottom: 34})
	d.Frame()
	d.Frame()
	if in.Top != 47 || in.Bottom != 34 {
		t.Fatalf("insets %v", in)
	}
	if r, ok := d.Runner().Context().RectOf(bottom); !ok || r.H != 34 {
		t.Fatalf("bottom safe area box %v", r)
	}
	// A Provider override still wins.
	over := geom.Insets{Top: 20}
	d2 := phone(func(c *ui.Context) {
		Provider(c, Options{SafeArea: &over}, func() { in = SafeAreaInsets(c) })
	}, false)
	d2.SetSafeArea(geom.Insets{Top: 47})
	d2.Frame()
	if in.Top != 20 {
		t.Fatalf("override %v", in)
	}
}
