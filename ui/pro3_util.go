package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// Shared helpers of the second group of Pro components (ProDescriptions,
// ProList, LightFilter, LoginForm, ProFormFieldSet).

// pro3IconBtn is a small square icon button labelled for Driver lookups
// and screen readers. size is the hit box, the glyph takes 14px.
func pro3IconBtn(c *Context, label string, ic *icon.Icon, size float32) bool {
	th := c.Theme
	n := c.add("pro3-icon", label)
	n.ln.Width, n.ln.Height = size, size
	n.role = a11y.RoleButton
	n.setName(label)
	ia := c.interact(n, true, input.CursorPointer)
	n.paint = func(l *paint.List, r geom.Rect) {
		ink := th.TextTertiary
		if ia.Hovered {
			ink = th.Primary.Hover
			l.Fill(r, geom.Rad(th.RadiusSM), th.FillTertiary)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
		g := float32(14)
		c.paintIcon(l, ic, geom.Rect{X: r.X + (r.W-g)/2, Y: r.Y + (r.H-g)/2, W: g, H: g}, ink, 0) // mirrors directional icons in RTL
	}
	return ia.Clicked
}

// pro3EnumLabel maps a choice value to its enum label.
func pro3EnumLabel(es []ProEnum, v string) string {
	for _, e := range es {
		if e.Value == v {
			return e.Label
		}
	}
	return v
}

// pro3ValueText formats a value of any ProField / ProColumn value type as
// display text ("-" when empty).
func pro3ValueText(c *Context, valueType string, enum []ProEnum, v any) string {
	if v == nil {
		return "-"
	}
	if s, ok := p2eValueText(c, valueType, enum, v); ok {
		return s
	}
	switch valueType {
	case ValueMoney:
		if f, ok := proNumber(v); ok {
			return FormatMoney(c.Locale(), f, "")
		}
	case ValueSelect, ValueRadio, ValueRadioButton, ValueSegmented:
		s := fmt.Sprint(v)
		if s == "" {
			return "-"
		}
		return pro3EnumLabel(enum, s)
	case ValueMultiSelect, ValueCheckbox:
		ss, _ := v.([]string)
		if len(ss) == 0 {
			return "-"
		}
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = pro3EnumLabel(enum, s)
		}
		return strings.Join(out, ", ")
	case ValueSwitch:
		if b, _ := v.(bool); b {
			return c.Locale().ProField.On
		}
		return c.Locale().ProField.Off
	case ValuePassword:
		if s, _ := v.(string); s != "" {
			return "******"
		}
		return "-"
	case ValueDateRange:
		r, ok := v.([2]time.Time)
		if !ok || (r[0].IsZero() && r[1].IsZero()) {
			return "-"
		}
		return r[0].Format("2006-01-02") + " " + c.Locale().RangeSeparator + " " + r[1].Format("2006-01-02")
	case ValueRate, ValueSlider:
		if f, ok := proNumber(v); ok {
			return ProFormat(ValueDigit, nil, f)
		}
	}
	return ProFormatLocale(c.Locale(), valueType, enum, v)
}

// pro3ValueView renders a read-only value by its value type: badges for
// status, tags, a progress bar, a rate, otherwise the formatted text.
func pro3ValueView(c *Context, valueType string, enum []ProEnum, v any) {
	switch valueType {
	case ValueStatus:
		if v == nil || fmt.Sprint(v) == "" {
			Text(c, "-")
			return
		}
		e := proEnum(enum, v)
		Badge(c).Status(e.Status, e.Label)
		return
	case ValueTag:
		if v == nil || fmt.Sprint(v) == "" {
			Text(c, "-")
			return
		}
		e := proEnum(enum, v)
		tg := Tag(c, e.Label)
		if e.Color != "" {
			tg.Color(e.Color)
		} else if e.Status != "" && e.Status != "default" {
			tg.Color(e.Status)
		}
		return
	case ValueProgress:
		f, _ := proNumber(v)
		pr := Progress(c, f).StrokeWidth(6).Layout(layout.Grow(1), layout.MinW(0), layout.MaxW(200))
		if f >= 100 {
			pr.Status("success")
		}
		return
	case ValueRate:
		f, _ := proNumber(v)
		Rate(c, &f).Disabled(true)
		return
	case ValueColor:
		p2eColorView(c, v)
		return
	}
	Text(c, pro3ValueText(c, valueType, enum, v))
}

// pro3Check runs the synchronous rules over s and returns the first
// error ("" when all pass).
func pro3Check(rules []Rule, s string) string {
	for _, r := range rules {
		if r.check == nil {
			continue
		}
		if msg := r.check(s); msg != "" {
			return msg
		}
	}
	return ""
}

// pro3Async delivers results of goroutines to the UI thread.
type pro3Async[T any] struct {
	ch chan T
}

func (a *pro3Async[T]) init() {
	if a.ch == nil {
		a.ch = make(chan T, 16)
	}
}

// poll returns a delivered result, if any.
func (a *pro3Async[T]) poll() (T, bool) {
	a.init()
	select {
	case v := <-a.ch:
		return v, true
	default:
		var zero T
		return zero, false
	}
}
