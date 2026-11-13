package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// TestConfigProviderSizeShrinksControls checks that ConfigProvider.Size
// reaches the controls that have a size (Ant componentSize).
func TestConfigProviderSizeShrinksControls(t *testing.T) {
	var (
		s     string
		ss    []string
		on    bool
		tm    time.Time
		rng   [2]time.Time
		col   = theme.Hex("#1677ff")
		page  = 1
		opts  = Opts("甲", "乙")
		casOp = []CascaderOption{{Value: "a", Label: "A"}}
		tree  = []TreeSelectNode{{Value: "a", Label: "A"}}
	)
	controls := []struct {
		name  string
		build func(c *Context)
	}{
		{"Button", func(c *Context) { Button(c, "按钮") }},
		{"Input", func(c *Context) { Input(c, &s) }},
		{"Select", func(c *Context) { Select(c, &s, opts) }},
		{"RadioButtons", func(c *Context) { RadioGroup(c, &s, opts).Buttons() }},
		{"Switch", func(c *Context) { Switch(c, &on) }},
		{"Segmented", func(c *Context) { Segmented(c, &s, opts) }},
		{"DatePicker", func(c *Context) { DatePicker(c, &tm) }},
		{"TimePicker", func(c *Context) { TimePicker(c, &tm) }},
		{"RangePicker", func(c *Context) { RangePicker(c, &rng) }},
		{"Cascader", func(c *Context) { Cascader(c, &ss, casOp) }},
		{"MultiSelect", func(c *Context) { MultiSelect(c, &ss, opts) }},
		{"InputTag", func(c *Context) { InputTag(c, &ss) }},
		{"TreeSelect", func(c *Context) { TreeSelect(c, &s, tree) }},
		{"Mentions", func(c *Context) { Mentions(c, &s, opts) }},
		{"ColorPicker", func(c *Context) { ColorPicker(c, &col) }},
		{"Pagination", func(c *Context) { Pagination(c, &page, 50, 10).Show() }},
	}
	boxes := map[bool][]*Box{}
	d := NewDriver(func(c *Context) {
		build := func(small bool) {
			boxes[small] = boxes[small][:0]
			for _, ctl := range controls {
				ctl := ctl
				b := Row(c, func() { ctl.build(c) }, layout.AlignItems(layout.AlignStart))
				boxes[small] = append(boxes[small], b)
			}
		}
		Row(c, func() {
			Column(c, func() { build(false) }, layout.Grow(1))
			Column(c, func() { ConfigProvider(c).Size(Small).Body(func() { build(true) }) }, layout.Grow(1))
		})
	}, DriverOptions{Width: 1400, Height: 1400})
	d.Frame()
	d.Frame()
	for i, ctl := range controls {
		mid, sm := boxes[false][i].Rect(), boxes[true][i].Rect()
		if !(sm.H < mid.H) {
			t.Errorf("%s: small height %v, middle %v", ctl.name, sm.H, mid.H)
		}
	}
}
