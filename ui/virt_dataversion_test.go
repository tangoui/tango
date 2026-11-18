package ui

// Final review I2: DataVersion on the a-class virtual lists (AutoComplete,
// Mentions, Command, Cascader, CascaderPanel). An in-place edit of the
// option slice (same slice, same length) refreshes the cached matches and
// widths on the next frame once DataVersion changes; without a change the
// cache keeps its documented result.

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/layout"
)

// TestDataVersionAutoComplete: "option-4" matches 111 options (virtual).
// An option renamed in place to match joins only after a DataVersion
// change; a long label renamed in place widens the list only then.
func TestDataVersionAutoComplete(t *testing.T) {
	opts := v1Opts(500)
	val, ver := "", 0
	var w *AutoCompleteW
	d := driver(func(c *Context) {
		Column(c, func() { w = AutoComplete(c, &val, opts).Width(160).DataVersion(ver) }, layout.Pad(20))
	})
	d.Frame()
	d.Click(v1Center(d, w.in.ed.id))
	d.Type("option-4")
	d.Settle()
	if got := v1Count(d, "ms-option"); got == 0 || got > v1Window {
		t.Fatalf("%d rows built, want a window of the 111 matches", got)
	}
	width := d.r.c.rects[w.listID].W
	opts[0].Label = "option-4 joined"
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "option-4 joined") != nil {
		t.Fatal("without a DataVersion change the cached matches changed (documented: they stay)")
	}
	ver++
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "option-4 joined") == nil {
		t.Fatal("DataVersion change: the option renamed in place did not join the matches")
	}
	// Width: a matching label made long in place.
	opts[0].Label = "option-4 " + strings.Repeat("long label ", 4)
	d.Frame()
	d.Settle()
	if got := d.r.c.rects[w.listID].W; got != width {
		t.Fatalf("without a DataVersion change the width changed: %v, was %v", got, width)
	}
	ver++
	d.Frame()
	d.Settle()
	if got := d.r.c.rects[w.listID].W; got <= width {
		t.Fatalf("DataVersion change: width %v, want wider than %v", got, width)
	}
}

func TestDataVersionMentions(t *testing.T) {
	opts := v1Opts(500)
	val, ver := "@option-4", 0
	d := driver(func(c *Context) {
		Column(c, func() { Mentions(c, &val, opts).Width(240).Open().DataVersion(ver) }, layout.Pad(20))
	})
	d.Settle()
	if got := v1Count(d, "ms-option"); got == 0 || got > v1Window {
		t.Fatalf("%d rows built, want a window of the 111 matches", got)
	}
	opts[0].Label = "option-4 joined"
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "option-4 joined") != nil {
		t.Fatal("without a DataVersion change the cached matches changed (documented: they stay)")
	}
	ver++
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "option-4 joined") == nil {
		t.Fatal("DataVersion change: the option renamed in place did not join the matches")
	}
}

// TestDataVersionCommand: "command 4" matches far more than 100 of 1000
// items (virtual); an item renamed in place joins after a DataVersion
// change, and a long label widens the list only then.
func TestDataVersionCommand(t *testing.T) {
	items := v1CommandItems(1000)
	ver := 0
	var w *CommandW
	d := driver(func(c *Context) {
		Row(c, func() { w = Command(c, items).DataVersion(ver) }, layout.Pad(10))
	})
	d.Settle()
	d.Click(v1Center(d, w.in.ed.id))
	d.Type("command 4")
	d.Settle()
	if n := len(w.visible); n <= p2bVirtualMin {
		t.Fatalf("%d matches, want a virtual list", n)
	}
	if slices.Contains(w.visible, 0) {
		t.Fatal("command 1 matches \"command 4\"")
	}
	width := d.r.c.rects[w.list].W
	items[0].Label = "command 4 joined"
	d.Frame()
	d.Settle()
	if slices.Contains(w.visible, 0) {
		t.Fatal("without a DataVersion change the cached matches changed (documented: they stay)")
	}
	ver++
	d.Frame()
	d.Settle()
	if !slices.Contains(w.visible, 0) {
		t.Fatal("DataVersion change: the item renamed in place did not join the matches")
	}
	items[0].Label = "command 4 " + strings.Repeat("long command label ", 3)
	d.Frame()
	d.Settle()
	if got := d.r.c.rects[w.list].W; got != width {
		t.Fatalf("without a DataVersion change the width changed: %v, was %v", got, width)
	}
	ver++
	d.Frame()
	d.Settle()
	if got := d.r.c.rects[w.list].W; got <= width {
		t.Fatalf("DataVersion change: width %v, want wider than %v", got, width)
	}
}

// TestDataVersionCascader: a virtual column (sized by its widest row,
// built or not) widens after a DataVersion change when a label out of view
// is made long in place (Cascader's menu and CascaderPanel).
func TestDataVersionCascader(t *testing.T) {
	for _, panel := range []bool{false, true} {
		opts := v1CasOpts(400)
		val := []string(nil)
		ver := 0
		var pw *CascaderPanelW
		d := driver(func(c *Context) {
			Column(c, func() {
				if panel {
					pw = CascaderPanel(c, &val, opts).DataVersion(ver)
					return
				}
				Cascader(c, &val, opts).Width(240).Open().DataVersion(ver)
			}, layout.Pad(10))
		})
		boxW := func() float32 {
			if panel {
				return d.r.c.rects[pw.ID()].W
			}
			w := float32(0) // the menu's columns are its scroll viewports
			for _, s := range v1Scrolls(d) {
				w = max(w, float32(s.view.W))
			}
			return w
		}
		d.Settle()
		if got := v1Count(d, "cas-option"); got == 0 || got >= 400 {
			t.Fatalf("panel=%v: %d rows built, want a window", panel, got)
		}
		width := boxW()
		opts[350].Label = strings.Repeat("long cascader label ", 3)
		d.Frame()
		d.Settle()
		if v1Built(d, "cas-option", opts[350].Label) != nil {
			t.Fatal("the renamed row should be out of view")
		}
		if got := boxW(); got != width {
			t.Fatalf("panel=%v: without a DataVersion change the width changed: %v, was %v", panel, got, width)
		}
		ver++
		d.Frame()
		d.Settle()
		if got := boxW(); got <= width {
			t.Fatalf("panel=%v: DataVersion change: width %v, want wider than %v", panel, got, width)
		}
	}
}
