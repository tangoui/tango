package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// t2bCount counts the paint ops (fills, strokes, paths, text) drawn in col.
func t2bCount(d *Driver, col theme.Color) int {
	n := 0
	for _, op := range d.r.List().Ops {
		var c theme.Color
		switch o := op.(type) {
		case paint.FillRect:
			c = o.Color
		case paint.StrokeRect:
			c = o.Color
		case paint.FillPath:
			c = o.Color
		case paint.Text:
			c = o.Color
		default:
			continue
		}
		if c == col {
			n++
		}
	}
	return n
}

// t2bRender renders the instance built by fn inside a provider overriding
// cs and, with outside, a second instance after the provider.
func t2bRender(th *theme.Theme, cs theme.Components, outside bool, fn func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Components(cs).Body(func() { fn(c) })
			if outside {
				fn(c)
			}
		}, layout.Gap(40), layout.Pad(40))
	}, DriverOptions{Width: 900, Height: 1200, Theme: th})
	d.Settle()
	return d
}

var t2bMark = theme.Hex("#12ab34")

// TestT2bTokenOverridesAreScoped overrides one color token per component
// in a nested ConfigProvider: the scoped instance paints it, the instance
// outside the provider adds no mark and keeps the derived default.
func TestT2bTokenOverridesAreScoped(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(theme.Dark()), theme.Shadcn()} {
		k := th.Components
		for _, tc := range []struct {
			name string
			cs   theme.Components
			def  theme.Color
			fn   func(c *Context)
		}{
			{"Timeline", theme.Components{Timeline: theme.TimelineToken{DotColor: t2bMark}}, k.Timeline.DotColor,
				func(c *Context) { Timeline(c, []TimelineItem{{Content: "a"}, {Content: "b"}}).Show() }},
			{"Descriptions", theme.Components{Descriptions: theme.DescriptionsToken{LabelBg: t2bMark}}, k.Descriptions.LabelBg,
				func(c *Context) { Descriptions(c, []DescItem{{Label: "L", Value: "v"}}).Bordered() }},
			{"List", theme.Components{List: theme.ListToken{SplitColor: t2bMark}}, k.List.SplitColor,
				func(c *Context) { List(c, 2).Item(func(int) { Text(c, "row") }) }},
			{"Badge", theme.Components{Badge: theme.BadgeToken{CountBg: t2bMark}}, k.Badge.CountBg,
				func(c *Context) { Badge(c).Count(5).Body(func() { Text(c, "content") }) }},
			{"Progress", theme.Components{Progress: theme.ProgressToken{DefaultColor: t2bMark}}, k.Progress.DefaultColor,
				func(c *Context) { Progress(c, 40) }},
			{"Alert", theme.Components{Alert: theme.AlertToken{InfoIconColor: t2bMark}}, k.Alert.InfoIconColor,
				func(c *Context) { Alert(c, "info", "note").Show() }},
			{"Statistic", theme.Components{Statistic: theme.StatisticToken{ContentColor: t2bMark}}, k.Statistic.ContentColor,
				func(c *Context) { Statistic(c, "Users", 42, 0) }},
			{"Divider", theme.Components{Divider: theme.DividerToken{Color: t2bMark}}, k.Divider.Color,
				func(c *Context) { Divider(c) }},
			{"Typography", theme.Components{Typography: theme.TypographyToken{MarkBg: t2bMark}}, k.Typography.MarkBg,
				func(c *Context) { Text(c, "marked").Mark() }},
			{"Link", theme.Components{Typography: theme.TypographyToken{LinkColor: t2bMark}}, k.Typography.LinkColor,
				func(c *Context) { Link(c, "link") }},
			{"Image", theme.Components{Image: theme.ImageToken{PlaceholderBg: t2bMark}}, k.Image.PlaceholderBg,
				func(c *Context) { Image(c, nil).Size(60, 40) }},
			{"Carousel", theme.Components{Carousel: theme.CarouselToken{DotColor: t2bMark}}, k.Carousel.DotColor,
				func(c *Context) { Carousel(c, 3, func(int) { Text(c, "slide") }).Height(80).Show() }},
			{"Result", theme.Components{Result: theme.ResultToken{SuccessIconColor: t2bMark}}, k.Result.SuccessIconColor,
				func(c *Context) { Result(c, "success", "Done").Show() }},
			{"Empty", theme.Components{Empty: theme.EmptyToken{DescriptionColor: t2bMark}}, k.Empty.DescriptionColor,
				func(c *Context) { Empty(c) }},
		} {
			t.Run(t2bThemeName(th)+"/"+tc.name, func(t *testing.T) {
				in := t2bRender(th, tc.cs, false, tc.fn)
				both := t2bRender(th, tc.cs, true, tc.fn)
				n := t2bCount(in, t2bMark)
				if n == 0 {
					t.Fatal("override not painted inside the scope")
				}
				if m := t2bCount(both, t2bMark); m != n {
					t.Errorf("override painted %d times with an instance outside the scope, want %d", m, n)
				}
				if t2bCount(both, tc.def) <= t2bCount(in, tc.def) {
					t.Errorf("default %v not painted outside the scope", tc.def)
				}
			})
		}
	}
}

func t2bThemeName(th *theme.Theme) string {
	s := th.Flavor.String()
	if th.Dark {
		s += "-dark"
	}
	return s
}

// TestT2bTokenDefaults: the derived tokens equal the global values the
// components painted before they had tokens.
func TestT2bTokenDefaults(t *testing.T) {
	for _, th := range theme.BuiltinThemes() {
		k := th.Components
		el := th.Flavor == theme.Element
		shad := th.Flavor == theme.StyleShadcn
		type pair struct {
			name      string
			got, want theme.Color
		}
		tailCol := th.Split
		if shad {
			tailCol = th.Border
		}
		dot := th.Primary.Base
		if el {
			dot = th.Split
		}
		labelBg := th.FillAlter
		if el {
			labelBg = th.FillTertiary
		}
		for _, p := range []pair{
			{"Timeline.TailColor", k.Timeline.TailColor, tailCol},
			{"Timeline.DotColor", k.Timeline.DotColor, dot},
			{"Descriptions.LabelBg", k.Descriptions.LabelBg, labelBg},
			{"List.SplitColor", k.List.SplitColor, th.Split},
			{"Badge.CountBg", k.Badge.CountBg, th.Error.Base},
			{"Badge.TextColor", k.Badge.TextColor, th.TextOnSolid},
			{"Progress.DefaultColor", k.Progress.DefaultColor, th.Primary.Base},
			{"Alert.ErrorIconColor", k.Alert.ErrorIconColor, th.Error.Base},
			{"Statistic.ContentColor", k.Statistic.ContentColor, th.TextHeading},
			{"Divider.Color", k.Divider.Color, th.Split},
			{"Typography.MarkBg", k.Typography.MarkBg, th.MarkBg},
			{"Typography.MarkColor", k.Typography.MarkColor, th.MarkText},
			{"Typography.LinkColor", k.Typography.LinkColor, th.TextLink},
			{"Typography.LinkHoverColor", k.Typography.LinkHoverColor, th.TextLinkHover},
			{"Image.PlaceholderBg", k.Image.PlaceholderBg, th.FillTertiary},
			{"Image.PreviewBg", k.Image.PreviewBg, th.BgMask},
			{"Carousel.DotOutsideActiveColor", k.Carousel.DotOutsideActiveColor, th.Primary.Base},
			{"Result.TitleColor", k.Result.TitleColor, th.TextHeading},
			{"Empty.SimpleLine", k.Empty.SimpleLine, th.Illustration.Line},
		} {
			if p.got != p.want {
				t.Errorf("%s %s = %v, want %v", t2bThemeName(th), p.name, p.got, p.want)
			}
		}
		if k.Progress.RemainingColor.A == 0 != shad {
			t.Errorf("%s: Progress.RemainingColor follows the bar only under shadcn", t2bThemeName(th))
		}
		if k.Alert.InfoBorderColor.A == 0 != el {
			t.Errorf("%s: Alert borders are none only under Element", t2bThemeName(th))
		}
	}
	// The Mark pair stays guarded on the token: HighContrast lifts it.
	for _, th := range theme.BuiltinThemes() {
		hc := theme.HighContrast(th)
		ty := hc.Components.Typography
		fg := ty.MarkColor
		if fg.A == 0 {
			fg = hc.Text
		}
		if r := theme.ContrastRatio(fg, theme.Over(ty.MarkBg, hc.BgContainer)); r < 4.5 {
			t.Errorf("%s high contrast: mark %.2f", t2bThemeName(th), r)
		}
	}
}
