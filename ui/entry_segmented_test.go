package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEntrySegmentedVerticalRound(t *testing.T) {
	val := "a"
	opts := []Option{{Label: "a", Value: "a"}, {Label: "bbbbbbbb", Value: "b"}, {Label: "c", Value: "c"}}
	var w *SegmentedW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Segmented(c, &val, opts).Vertical().Shape(SegmentedShapeRound)
		}, layout.Pad(20))
	})
	d.Frame()
	a, b, cc := mustFind(t, d, "a"), mustFind(t, d, "bbbbbbbb"), mustFind(t, d, "c")
	if !(a.Y < b.Y && b.Y < cc.Y) || a.X != b.X || a.W != b.W {
		t.Fatalf("not stacked with equal widths: %v %v %v", a, b, cc)
	}
	if w.n.aState&a11y.Vertical == 0 || w.n.aState&a11y.Horizontal != 0 {
		t.Fatalf("orientation state %v", w.n.aState)
	}
	// Up / down move the selection.
	d.r.c.SetFocus(w.n.id)
	d.Key(input.KeyDown, 0)
	if val != "b" {
		t.Fatalf("down: %q", val)
	}
	d.Key(input.KeyUp, 0)
	if val != "a" {
		t.Fatalf("up: %q", val)
	}
	d.Click(cc.Center())
	if val != "c" {
		t.Fatalf("click: %q", val)
	}
	// Round: pill radii.
	outer, inner := w.radii()
	if inner != w.itemH()/2 || outer != inner+w.trackPad() {
		t.Fatalf("round radii %v %v", outer, inner)
	}
}

func TestEntrySegmentedClickKeysDisabled(t *testing.T) {
	val := "日"
	opts := []Option{{Label: "日", Value: "日"}, {Label: "周", Value: "周"}, {Label: "月", Value: "月", Disabled: true}, {Label: "年", Value: "年"}}
	changed := 0
	all := false
	var block bool
	var w *SegmentedW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Segmented(c, &val, opts).Disabled(all)
			if block {
				w.Block()
			}
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(mustFind(t, d, "周").Center())
	if val != "周" || changed != 1 {
		t.Fatalf("click: %q %d", val, changed)
	}
	// Disabled option ignores clicks.
	d.Click(mustFind(t, d, "月").Center())
	if val != "周" {
		t.Fatalf("disabled option picked: %q", val)
	}
	// Arrow keys skip the disabled option and wrap.
	d.r.c.SetFocus(w.n.id)
	d.Key(input.KeyRight, 0)
	if val != "年" {
		t.Fatalf("right: %q", val)
	}
	d.Key(input.KeyRight, 0)
	if val != "日" {
		t.Fatalf("wrap: %q", val)
	}
	d.Key(input.KeyLeft, 0)
	if val != "年" {
		t.Fatalf("left: %q", val)
	}
	// Block items share the width.
	block = true
	d.Frame()
	d.Frame()
	a, b := mustFind(t, d, "日"), mustFind(t, d, "年")
	if a.W != b.W || a.W < 100 {
		t.Fatalf("block widths %v %v", a.W, b.W)
	}
	// Whole control disabled.
	all = true
	d.Frame()
	d.Click(mustFind(t, d, "周").Center())
	if val != "年" {
		t.Fatalf("disabled control changed: %q", val)
	}
}
