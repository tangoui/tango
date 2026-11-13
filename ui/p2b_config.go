package ui

// ConfigProvider defaults of the selection components (Select,
// MultiSelect, VirtualSelect, Slider, Cascader, TreeSelect, Transfer) and
// small helpers they share. Keys mirror the Ant Design prop names:
//
//	ui.ConfigProvider(c).Defaults("Select", ui.Props{"allowClear": true, "showSearch": true}).Body(page)
//
// Explicit modifiers always win over these defaults.

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// p2bBool reads a boolean default property.
func p2bBool(c *Context, comp, key string) (bool, bool) {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// p2bNum reads a numeric default property of any Go number type.
func p2bNum(c *Context, comp, key string) (float64, bool) {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case time.Duration:
		return float64(x), true
	}
	return 0, false
}

// p2bStr reads a string default property.
func p2bStr(c *Context, comp, key string) (string, bool) {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// p2bDur reads a duration default (time.Duration, or milliseconds as a
// plain number).
func p2bDur(c *Context, comp, key string) (time.Duration, bool) {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return 0, false
	}
	if d, ok := v.(time.Duration); ok {
		return d, true
	}
	if ms, ok := p2bNum(c, comp, key); ok {
		return time.Duration(ms * float64(time.Millisecond)), true
	}
	return 0, false
}

// p2bSelectDefaults applies the "Select" defaults to a selector shell:
// size, allowClear, showSearch, placeholder, maxTagCount, listHeight, debounce.
func p2bSelectDefaults(c *Context, b *selector, listH *float32, debounce *time.Duration, filterable *bool) {
	shad2aSize(c, "Select", &b.size)
	if v, ok := p2bBool(c, "Select", "allowClear"); ok {
		b.allowClear = v
	}
	if v, ok := p2bBool(c, "Select", "showSearch"); ok {
		b.search = v
		if filterable != nil {
			*filterable = v
		}
	}
	if v, ok := p2bStr(c, "Select", "placeholder"); ok {
		b.placeholder = v
	}
	if v, ok := p2bNum(c, "Select", "maxTagCount"); ok {
		b.maxCount = int(v)
	}
	if v, ok := p2bNum(c, "Select", "listHeight"); ok && listH != nil {
		*listH = float32(v)
	}
	if v, ok := p2bDur(c, "Select", "debounce"); ok && debounce != nil {
		*debounce = v
	}
}

// CheckedStrategy chooses which checked nodes of a tree-shaped multiple
// selection (Cascader, TreeSelect) are shown as tags and returned by
// Checked (Ant showCheckedStrategy).
type CheckedStrategy int

const (
	// ShowChild shows every checked leaf.
	ShowChild CheckedStrategy = iota
	// ShowParent shows a fully checked parent instead of its children.
	ShowParent
	// ShowAll shows every checked node, parents and children.
	ShowAll
)

// p2bStrategy reads a "showCheckedStrategy" default.
func p2bStrategy(c *Context, comp string, def CheckedStrategy) CheckedStrategy {
	v, ok := c.ConfigProp(comp, "showCheckedStrategy")
	if !ok {
		return def
	}
	switch x := v.(type) {
	case CheckedStrategy:
		return x
	case string:
		switch x {
		case "child", "SHOW_CHILD":
			return ShowChild
		case "parent", "SHOW_PARENT":
			return ShowParent
		case "all", "SHOW_ALL":
			return ShowAll
		}
	}
	return def
}

// p2bVirtualMin is the option count above which Select, MultiSelect,
// TreeSelect and Transfer switch to virtual lists (only the visible rows
// are built).
const p2bVirtualMin = 100

// p2bHasGroups reports whether any option names a group.
func p2bHasGroups(opts []Option) bool {
	for i := range opts {
		if opts[i].Group != "" {
			return true
		}
	}
	return false
}

// p2bSpin draws the small rotating loading arc of option lists and tree
// switchers (Ant LoadingOutlined, Element is-loading).
func p2bSpin(c *Context, l *paint.List, r geom.Rect, col theme.Color) {
	ang := spinAngle(c)
	p := paint.NewPath().Arc(r.X+r.W/2, r.Y+r.H/2, r.W/2-1, ang, 1.5*math.Pi)
	l.StrokePath(p, paint.StrokeStyle{Width: 1.5, Cap: paint.CapRound}, col)
}
