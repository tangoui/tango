package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

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
