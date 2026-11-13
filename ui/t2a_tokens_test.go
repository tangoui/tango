package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

var t2aMark = theme.Hex("#12ab34")

// t2aCount counts the fill, stroke, path and text ops painted in col.
func t2aCount(d *Driver, col theme.Color) int {
	n := 0
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			n += b2i(o.Color == col)
		case paint.StrokeRect:
			n += b2i(o.Color == col)
		case paint.FillPath:
			n += b2i(o.Color == col)
		case paint.Text:
			n += b2i(o.Color == col)
		}
	}
	return n
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestT2aTokenOverridesAreScoped overrides one color token of Steps,
// Breadcrumb, Splitter, FloatButton, Anchor and Collapse in a nested
// ConfigProvider: only the scoped instance paints it, the instance outside
// keeps the derived value.
func TestT2aTokenOverridesAreScoped(t *testing.T) {
	th := theme.AntDesign()
	k := th.Components
	for _, tc := range []struct {
		name string
		cs   theme.Components
		def  theme.Color
		fn   func(c *Context, scoped bool)
	}{
		{"Steps", theme.Components{Steps: theme.StepsToken{ProcessIconBg: t2aMark}}, k.Steps.ProcessIconBg,
			func(c *Context, _ bool) { Steps(c, []StepItem{{Title: "a"}, {Title: "b"}}).Show() }},
		{"Breadcrumb", theme.Components{Breadcrumb: theme.BreadcrumbToken{SeparatorColor: t2aMark}}, k.Breadcrumb.SeparatorColor,
			func(c *Context, _ bool) { Breadcrumb(c, "a", "b") }},
		{"Splitter", theme.Components{Splitter: theme.SplitterToken{BarColor: t2aMark}}, k.Splitter.BarColor,
			func(c *Context, _ bool) {
				Splitter(c).Style(layout.W(300), layout.H(60)).Body(func() {
					SplitterPanel(c).Body(func() {})
					SplitterPanel(c).Body(func() {})
				})
			}},
		{"FloatButton", theme.Components{FloatButton: theme.FloatButtonToken{Bg: t2aMark}}, k.FloatButton.Bg,
			func(c *Context, scoped bool) {
				f := FloatButton(c)
				if scoped {
					f.Right(100)
				}
			}},
		{"Anchor", theme.Components{Anchor: theme.AnchorToken{RailColor: t2aMark}}, k.Anchor.RailColor,
			func(c *Context, _ bool) { Anchor(c, []AnchorItem{{Key: "a", Title: "A"}}).Show() }},
		{"Collapse", theme.Components{Collapse: theme.CollapseToken{HeaderBg: t2aMark}}, k.Collapse.HeaderBg,
			func(c *Context, _ bool) { Collapse(c, nil, []CollapseItem{{Key: "a", Label: "A"}}).Show() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(func(c *Context) {
				g4Scoped(c, tc.cs, func(scoped bool) { tc.fn(c, scoped) })
			}, DriverOptions{Width: 600, Height: 600, Theme: th})
			d.Settle()
			if n := t2aCount(d, t2aMark); n != 1 {
				t.Errorf("override painted %d times, want once (inside the scope)", n)
			}
			if n := t2aCount(d, tc.def); n < 1 {
				t.Errorf("default %v not painted outside the scope", tc.def)
			}
		})
	}
}

// TestT2aTourToken: a Tour opened inside a provider paints its panel and
// dots with the provider's tokens; the same tour outside does not.
func TestT2aTourToken(t *testing.T) {
	th := theme.AntDesign()
	run := func(scoped bool) *Driver {
		open, cur := true, 0
		d := NewDriver(func(c *Context) {
			body := func() {
				Tour(c, &open, &cur, []TourStep{{Title: "one"}, {Title: "two"}}).Show()
			}
			if scoped {
				ConfigProvider(c).Components(theme.Components{Tour: theme.TourToken{Bg: t2aMark, IndicatorActiveColor: t2aMark}}).Body(body)
			} else {
				body()
			}
		}, DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Settle()
		return d
	}
	if n := t2aCount(run(true), t2aMark); n != 2 {
		t.Errorf("scoped tour painted the override %d times, want panel + active dot", n)
	}
	d := run(false)
	if n := t2aCount(d, t2aMark); n != 0 {
		t.Errorf("override leaked: %d", n)
	}
	if t2aCount(d, th.Components.Tour.Bg) < 1 || t2aCount(d, th.Components.Tour.IndicatorActiveColor) < 1 {
		t.Error("default tour colors not painted")
	}
}

// TestT2aTokenSizes: size tokens change the layout inside the scope only.
func TestT2aTokenSizes(t *testing.T) {
	cs := theme.Components{
		Splitter: theme.SplitterToken{SplitTriggerSize: 12},
		Anchor:   theme.AnchorToken{LinkPaddingBlock: 10},
		Collapse: theme.CollapseToken{HeaderPaddingBlock: 20},
	}
	page := func(c *Context, scoped bool) {
		sfx := "o"
		if scoped {
			sfx = "s"
		}
		Splitter(c).Style(layout.W(300), layout.H(40)).Body(func() {
			SplitterPanel(c).Body(func() {})
			SplitterPanel(c).Body(func() {})
		})
		Anchor(c, []AnchorItem{{Key: sfx, Title: "anchor-" + sfx}}).Show()
		Collapse(c, nil, []CollapseItem{{Key: sfx, Label: "coll-" + sfx}}).Show()
	}
	d := NewDriver(func(c *Context) {
		g4Scoped(c, cs, func(scoped bool) { page(c, scoped) })
	}, DriverOptions{Width: 600, Height: 800, Theme: theme.AntDesign()})
	d.Settle()
	d.Settle()
	var bars []geom.Rect
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == "splitter-bar-1" && n.drawn == d.r.c.pass {
			bars = append(bars, n.abs)
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	if len(bars) != 2 || !near(bars[0].W, 12) || !near(bars[1].W, 6) {
		t.Errorf("splitter bars %v, want widths 12 (scoped) and 6", bars)
	}
	if s, o := g6Rect(t, d, "anchor-s"), g6Rect(t, d, "anchor-o"); !near(s.H, 42) || !near(o.H, 30) {
		t.Errorf("anchor link heights %v / %v, want 42 / 30", s.H, o.H)
	}
	if s, o := g6Rect(t, d, "coll-s"), g6Rect(t, d, "coll-o"); !near(s.H-o.H, 16) {
		t.Errorf("collapse header heights %v / %v, want 16 apart", s.H, o.H)
	}
}

// TestT2aTokenDefaults: the derived tokens reproduce the values the
// components painted before they read them, in every design system.
func TestT2aTokenDefaults(t *testing.T) {
	for _, th := range theme.BuiltinThemes() {
		k := th.Components
		el, shad := th.Flavor == theme.Element, th.Flavor == theme.StyleShadcn
		name := th.Flavor.String()
		if th.Dark {
			name += "-dark"
		}
		// Steps.
		st := k.Steps
		switch {
		case el:
			if st.IconSize != 24 || st.NavIconSize != 16 || st.TailWidth != 2 || st.ProcessIconBorderColor != th.Text ||
				st.WaitTitleColor != th.TextQuaternary || st.TailColor != th.BorderHover || st.NavBg != th.FillTertiary || st.NavInkBarColor.A != 0 {
				t.Errorf("%s steps: %+v", name, st)
			}
		case shad:
			if st.FinishIconBg != th.Primary.Base || st.WaitIconBorderColor != th.Border || st.HoverColor != th.Text || st.ProcessDescriptionColor != th.TextTertiary {
				t.Errorf("%s steps: %+v", name, st)
			}
		default:
			if st.IconSize != 32 || st.IconSizeSM != 24 || st.FinishIconBg != th.Primary.Bg || st.WaitIconBg != th.FillSecondary ||
				st.TailColor != th.Split || st.HoverColor != th.Primary.Base || st.DotWaitColor != th.TextQuaternary || st.NavBg.A != 0 {
				t.Errorf("%s steps: %+v", name, st)
			}
		}
		// Breadcrumb.
		bc := k.Breadcrumb
		wantItem, wantLast, wantSep := th.TextTertiary, th.Text, th.TextQuaternary
		if el {
			wantItem, wantLast = th.Text, th.TextSecondary
		}
		if shad {
			wantSep = th.TextTertiary
		}
		if bc.ItemColor != wantItem || bc.LastItemColor != wantLast || bc.SeparatorColor != wantSep || bc.FontSize != th.FontSize {
			t.Errorf("%s breadcrumb: %+v", name, bc)
		}
		// Splitter.
		if sp := k.Splitter; sp.SplitTriggerSize != 6 || sp.BarColor != th.Split || sp.HandleActiveColor != th.Primary.Base {
			t.Errorf("%s splitter: %+v", name, sp)
		}
		// FloatButton.
		fb := k.FloatButton
		wantIcon, wantColor := float32(18), th.Text
		if el {
			wantIcon, wantColor = 20, th.Primary.Base
		}
		if fb.Size != 40 || fb.IconSize != wantIcon || fb.Color != wantColor || fb.Bg != th.BgElevated || fb.Radius != th.RadiusLG {
			t.Errorf("%s float button: %+v", name, fb)
		}
		// Anchor.
		an := k.Anchor
		if an.LinkPaddingInlineStart != 16 || an.InkBarColor != th.Primary.Base || an.RailColor != th.Split || (el != (an.LinkColor == th.TextSecondary)) {
			t.Errorf("%s anchor: %+v", name, an)
		}
		// Tour.
		tr := k.Tour
		if tr.Width != 320 || tr.Bg != th.BgElevated || tr.PrimaryBg != th.Primary.Base || tr.MaskColor != th.BgMask ||
			(el && (tr.TitleFontSize != 16 || tr.TitleFontWeight != 0 || tr.Radius != th.Radius)) ||
			(!el && (tr.TitleFontWeight != th.FontWeightStrong || tr.Radius != th.RadiusLG)) {
			t.Errorf("%s tour: %+v", name, tr)
		}
		// Collapse.
		co := k.Collapse
		switch {
		case el:
			if co.HeaderHeight != 48 || co.HeaderFontSize != 13 || co.BorderColor != th.BorderSecondary || co.ContentPaddingBlock != 25 {
				t.Errorf("%s collapse: %+v", name, co)
			}
		case shad:
			if co.HeaderBg.A != 0 || co.ArrowSize != 16 || co.BorderColor != th.Split || co.HeaderPaddingBlock != 16 {
				t.Errorf("%s collapse: %+v", name, co)
			}
		default:
			if co.HeaderBg != th.FillAlter || co.ContentBg != th.BgContainer || co.BorderColor != th.Border || co.Radius != th.RadiusLG ||
				co.HeaderPaddingBlockSM != 8 || co.ContentPaddingInlineLG != 24 {
				t.Errorf("%s collapse: %+v", name, co)
			}
		}
	}
}
