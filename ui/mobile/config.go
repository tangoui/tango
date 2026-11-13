package mobile

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui"
)

// ConfigProvider defaults of the mobile components. The component names
// carry a "mobile." prefix so they never collide with the desktop
// components of package ui ("Button", "Tabs" …):
//
//	ui.ConfigProvider(c).Defaults("mobile.Button", ui.Props{"size": "large", "shape": "round"}).Body(page)
//
// Explicit modifiers always win: defaults are applied when the builder is
// created, the chained modifiers run afterwards. The keys follow the
// Ant Design Mobile / Vant prop names; each component documents the keys
// it reads.

func cfgKey(comp string) string { return "mobile." + comp }

// cfgBool reads a boolean default.
func cfgBool(c *ui.Context, comp, key string) (bool, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// cfgNum reads a numeric default of any Go number type.
func cfgNum(c *ui.Context, comp, key string) (float64, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), key)
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

// cfgStr reads a string default (lower-cased for enum keys by the caller).
func cfgStr(c *ui.Context, comp, key string) (string, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// cfgDur reads a duration default (time.Duration, or milliseconds as a
// plain number).
func cfgDur(c *ui.Context, comp, key string) (time.Duration, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), key)
	if !ok {
		return 0, false
	}
	if d, ok := v.(time.Duration); ok {
		return d, true
	}
	if ms, ok := cfgNum(c, comp, key); ok {
		return time.Duration(ms * float64(time.Millisecond)), true
	}
	return 0, false
}

// cfgSize reads a "size" default: a mobile.Size, a ui.ControlSize or one
// of "mini", "small", "middle", "large".
func cfgSize(c *ui.Context, comp string) (Size, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), "size")
	if !ok {
		return Middle, false
	}
	switch x := v.(type) {
	case Size:
		return x, true
	case ui.ControlSize:
		return sizeOfControl(x), true
	case string:
		switch strings.ToLower(x) {
		case "mini":
			return Mini, true
		case "small":
			return Small, true
		case "middle", "medium", "default":
			return Middle, true
		case "large":
			return Large, true
		}
	}
	return Middle, false
}

func sizeOfControl(s ui.ControlSize) Size {
	switch s {
	case ui.Small:
		return Small
	case ui.Large:
		return Large
	}
	return Middle
}

// cfgPosition reads a "position" default: a Position or "bottom", "top",
// "center", "left", "right", "start", "end".
func cfgPosition(c *ui.Context, comp string) (Position, bool) {
	v, ok := c.ConfigProp(cfgKey(comp), "position")
	if !ok {
		return PosBottom, false
	}
	switch x := v.(type) {
	case Position:
		return x, true
	case string:
		switch strings.ToLower(x) {
		case "bottom":
			return PosBottom, true
		case "top":
			return PosTop, true
		case "center", "middle":
			return PosCenter, true
		case "left":
			return PosLeft, true
		case "right":
			return PosRight, true
		case "start":
			return PosStart, true
		case "end":
			return PosEnd, true
		}
	}
	return PosBottom, false
}

// cfgRound reads a "shape" default ("round" / "rounded" → true,
// "default" / "rectangular" → false).
func cfgRound(c *ui.Context, comp string) (bool, bool) {
	s, ok := cfgStr(c, comp, "shape")
	if !ok {
		return false, false
	}
	switch strings.ToLower(s) {
	case "round", "rounded":
		return true, true
	case "default", "rectangular", "square":
		return false, true
	}
	return false, false
}
