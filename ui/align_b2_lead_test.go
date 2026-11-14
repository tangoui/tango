package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// TestLinkElementDefaultRegular: Element's el-link defaults to type
// "default" — regular text at rest, color-primary on hover
// (element-plus theme-chalk/src/common/var.scss $link, 2026-11-14) — while
// Ant and shadcn links keep the link color, and other link-colored text
// (Typography.LinkColor) stays link-colored under Element.
func TestLinkElementDefaultRegular(t *testing.T) {
	for _, tc := range []struct {
		name               string
		th                 *theme.Theme
		rest, hover, press func(th *theme.Theme) theme.Color
	}{
		{"element", theme.ElementPlus(),
			func(th *theme.Theme) theme.Color { return th.TextSecondary },
			func(th *theme.Theme) theme.Color { return th.Primary.Base },
			func(th *theme.Theme) theme.Color { return th.Primary.Base }},
		{"antd", theme.AntDesign(),
			func(th *theme.Theme) theme.Color { return th.TextLink },
			func(th *theme.Theme) theme.Color { return th.TextLinkHover },
			func(th *theme.Theme) theme.Color { return th.TextLinkActive }},
		{"shadcn", theme.Shadcn(),
			func(th *theme.Theme) theme.Color { return th.TextLink },
			func(th *theme.Theme) theme.Color { return th.TextLinkHover },
			func(th *theme.Theme) theme.Color { return th.TextLinkActive }},
	} {
		var lw *LinkW
		NewDriver(func(c *Context) { lw = Link(c, "a") }, DriverOptions{Theme: tc.th}).Frame()
		tk := &tc.th.Components.Typography
		base, hover, press := lw.colors(tc.th, tk)
		if base != tc.rest(tc.th) || hover != tc.hover(tc.th) || press != tc.press(tc.th) {
			t.Errorf("%s: link colors %v / %v / %v", tc.name, base, hover, press)
		}
		if tk.LinkColor != tc.th.TextLink {
			t.Errorf("%s: Typography.LinkColor %v, want the link color %v", tc.name, tk.LinkColor, tc.th.TextLink)
		}
	}
}

// TestRadioGroupBlockKeepsHeight: a Block button group in a column takes
// the column's width, not its height (batch 1 review).
func TestRadioGroupBlockKeepsHeight(t *testing.T) {
	v := "a"
	var g *RadioGroupW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			g = RadioGroup(c, &v, Opts("a", "b", "c")).Buttons().Block()
			Text(c, "after")
		}, layout.W(480), layout.H(400))
	}, DriverOptions{Width: 600, Height: 500, Theme: theme.AntDesign()})
	d.Settle()
	gr := d.r.c.rects[g.n.id]
	if gr.W < 470 || gr.H > 40 {
		t.Fatalf("block group rect %v, want full width and one control tall", gr)
	}
}

// TestOrientationIgnoresUnknown: unknown orientation values keep the
// current direction (like Flex, Space, Segmented, Divider).
func TestOrientationIgnoresUnknown(t *testing.T) {
	v := "a"
	NewDriver(func(c *Context) {
		g := RadioGroup(c, &v, Opts("a", "b")).Vertical().Orientation("diagonal")
		if !g.vertical {
			t.Error("RadioGroup: unknown orientation undid Vertical")
		}
		cw := Compact(c).Vertical().Orientation("x")
		if !cw.vertical {
			t.Error("Compact: unknown orientation undid Vertical")
		}
	}, DriverOptions{}).Frame()
}

// TestLinkColorOverrideFallsBack: with LinkText* unset (Ant, shadcn), a
// scoped Typography.LinkColor override still recolors Link.
func TestLinkColorOverrideFallsBack(t *testing.T) {
	mark := theme.Hex("#123456")
	th := theme.AntDesign()
	var lw *LinkW
	var tk *theme.TypographyToken
	NewDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Typography: theme.TypographyToken{LinkColor: mark}}).Body(func() {
			lw = Link(c, "a")
			tk = &c.ThemeFor(theme.CompTypography).Components.Typography
		})
	}, DriverOptions{Theme: th}).Frame()
	if base, _, _ := lw.colors(th, tk); base != mark {
		t.Fatalf("link rest color %v, want the overridden LinkColor %v", base, mark)
	}
}

// TestPlacementDefaultsOneParser: Select-family and picker components read
// the "placement" Defaults key with the same parser (batch 1 review).
func TestPlacementDefaultsOneParser(t *testing.T) {
	for _, name := range []string{"top", "topRight", "TopRight", "bottom-end"} {
		val := ""
		var date time.Time
		var s *SelectW
		var dp *DatePickerW
		NewDriver(func(c *Context) {
			ConfigProvider(c).Defaults("Select", Props{"placement": name}).Defaults("DatePicker", Props{"placement": name}).Body(func() {
				s = Select(c, &val, Opts("a"))
				dp = DatePicker(c, &date)
			})
		}, DriverOptions{Width: 500, Height: 400}).Settle()
		if s.place != dp.place {
			t.Errorf("%q: Select %v, DatePicker %v", name, s.place, dp.place)
		}
	}
}

// TestSpinnerPrefixAfterButton: in Ant spinner mode the prefix starts
// right after the − button (its token width, not the field height), where
// the editor's room for it begins (batch 1 review).
func TestSpinnerPrefixAfterButton(t *testing.T) {
	for _, size := range []ControlSize{Small, Middle, Large} {
		v := 5.0
		var w *InputNumberW
		d := NewDriver(func(c *Context) {
			w = InputNumber(c, &v).Mode("spinner").Size(size).PrefixText("¥")
		}, DriverOptions{Width: 400, Height: 200, Theme: theme.AntDesign()})
		d.Settle()
		box := d.r.c.rects[w.box.id]
		want := box.X + w.sideW(box.H) + 8
		found := false
		for _, op := range d.r.List().Ops {
			if o, ok := op.(paint.Text); ok && o.Origin.X > box.X+1 && o.Origin.X < want+0.5 && o.Origin.X > want-0.5 {
				found = true
			}
		}
		if !found {
			t.Errorf("size %v: no prefix text at x=%.1f", size, want)
		}
	}
}

// TestSliderDisabledMidDragNoComplete: a drag cut short by disabling the
// slider reports no ChangeComplete once it is enabled again (batch 1
// review).
func TestSliderDisabledMidDragNoComplete(t *testing.T) {
	v := 10.0
	off := false
	var w *SliderW
	completes := 0
	d := NewDriver(func(c *Context) {
		w = Slider(c, &v, 0, 100).Disabled(off).A11yLabel("s").Layout(layout.W(400))
		if w.ChangeComplete() {
			completes++
		}
	}, DriverOptions{Width: 600, Height: 200, Theme: theme.AntDesign()})
	d.Settle()
	h := d.r.c.rects[w.thumbs[0].id]
	tr := d.r.c.rects[w.track.id]
	d.Inject(input.Pointer{Kind: input.Move, Pos: h.Center()})
	d.Inject(input.Pointer{Kind: input.Press, Pos: h.Center()})
	d.Frame()
	d.Move(geom.Pt(tr.X+tr.W*0.5, h.Center().Y))
	off = true
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(tr.X+tr.W*0.5, h.Center().Y)})
	d.Settle()
	off = false
	d.Settle()
	if completes != 0 {
		t.Fatalf("%d ChangeComplete after a drag cut short by Disabled", completes)
	}
}

// TestTagPlainDefaultReadable: a plain default tag keeps a readable label
// in every theme (shadcn's default Badge label is primary-foreground,
// which vanishes on the container background).
func TestTagPlainDefaultReadable(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		var tg *TagW
		NewDriver(func(c *Context) { tg = Tag(c, "plain").Effect("plain") }, DriverOptions{Theme: th}).Frame()
		bg, _, fg := tg.colors()
		if r := theme.ContrastRatio(fg, theme.Over(bg, th.BgContainer)); r < 3 {
			t.Errorf("%v dark=%v: plain default tag label contrast %.2f", th.Flavor, th.Dark, r)
		}
	}
}
