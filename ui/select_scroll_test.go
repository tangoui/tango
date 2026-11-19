package ui

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestSelectLongListScrolls(t *testing.T) {
	var opts []Option
	for i := 0; i < 40; i++ {
		s := fmt.Sprintf("选项 %02d", i)
		opts = append(opts, Option{Label: s, Value: s})
	}
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		val := "选项 30"
		var w *SelectW
		d := NewDriver(func(c *Context) {
			Column(c, func() { w = Select(c, &val, opts).Width(200) }, layout.Pad(20))
		}, DriverOptions{Width: 600, Height: 700, Theme: th})
		d.Frame()
		d.Click(d.r.c.rects[w.n.id].Center())
		d.Frame()
		pr := d.r.c.rects[w.st.panel]
		if pr.H > 256+14 {
			t.Fatalf("%s: panel %v not bounded", th.Name, pr)
		}
		r := mustFind(t, d, "选项 30")
		if r.Y < pr.Y || r.Bottom() > pr.Bottom() {
			t.Fatalf("%s: selected %v outside panel %v", th.Name, r, pr)
		}
		// Keyboard moves keep the highlight visible.
		for i := 0; i < 8; i++ {
			d.Key(input.KeyDown, 0)
		}
		hl := mustFind(t, d, opts[w.st.active].Label)
		if w.st.active != 38 || hl.Y < pr.Y || hl.Bottom() > pr.Bottom()+0.5 {
			t.Fatalf("%s: active %d at %v panel %v", th.Name, w.st.active, hl, pr)
		}
		d.Key(input.KeyEnter, 0)
		if val != "选项 38" {
			t.Fatalf("%s: picked %q", th.Name, val)
		}
	}
}

func TestSelectAndTreeSelectListHeight(t *testing.T) {
	options := v1Opts(30)
	nodes := make([]TreeSelectNode, len(options))
	for i, o := range options {
		nodes[i] = TreeSelectNode{Value: o.Value, Label: o.Label}
	}
	for _, name := range []string{"Select", "TreeSelect"} {
		for _, virtual := range []bool{true, false} {
			for _, tc := range []struct {
				name  string
				n     int
				limit float32
				want  float32
			}{
				{"custom-limit", len(options), 80, 80},
				{"default-limit", len(options), 0, 256},
				{"short-list", 1, 80, 0}, // natural height of one row
			} {
				t.Run(name+"/virtual="+strconv.FormatBool(virtual)+"/"+tc.name, func(t *testing.T) {
					var value string
					want := tc.want
					d := newPerfDriver(func(c *Context) {
						if name == "Select" {
							w := Select(c, &value, options[:tc.n]).Virtual(virtual).ListHeight(tc.limit).Open()
							if tc.n == 1 {
								want = w.th.Components.Select.OptionHeight
							}
						} else {
							w := TreeSelect(c, &value, nodes[:tc.n]).Virtual(virtual).ListHeight(tc.limit).Open()
							if tc.n == 1 {
								want = w.rowH()
							}
						}
					}, false)
					d.Settle()
					scrolls := v1Scrolls(d)
					if len(scrolls) != 1 {
						t.Fatalf("want one dropdown scroll state, got %d", len(scrolls))
					}
					if got := scrolls[0].view.H; got != want {
						t.Errorf("dropdown viewport = %gpx, want %gpx", got, want)
					}
				})
			}
		}
	}
}
