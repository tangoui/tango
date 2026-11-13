package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Component token overrides of the navigation group (Menu, Tabs,
// Pagination, Layout) apply inside the ConfigProvider scope only.

// g3Colored returns the rects of the fills and strokes drawn with col.
func g3Colored(d *Driver, col theme.Color) []geom.Rect {
	var out []geom.Rect
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col {
				out = append(out, o.Rect)
			}
		case paint.StrokeRect:
			if o.Color == col {
				out = append(out, o.Rect)
			}
		}
	}
	return out
}

// g3ScopeTest renders build twice side by side, the first copy inside a
// ConfigProvider with comps, and checks that col is painted in the scoped
// copy (at its widget labelled label) and nowhere in the other one.
func g3ScopeTest(t *testing.T, name string, comps theme.Components, col theme.Color, label string, build func(c *Context)) {
	t.Helper()
	var in, out *Box
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			in = Column(c, func() { ConfigProvider(c).Components(comps).Body(func() { build(c) }) }, layout.W(450))
			out = Column(c, func() { build(c) }, layout.W(450))
		}, layout.Gap(20))
	}, DriverOptions{Width: 1000, Height: 500, Theme: theme.AntDesign()})
	d.Settle()
	ir, or := d.r.c.rects[in.ID()], d.r.c.rects[out.ID()]
	hits := g3Colored(d, col)
	inside := false
	for _, r := range hits {
		switch {
		case ir.Contains(r.Center()):
			inside = true
		case or.Contains(r.Center()):
			t.Errorf("%s: override painted outside its scope at %+v", name, r)
		}
	}
	if !inside {
		t.Errorf("%s: override not painted in scope (%d hits)", name, len(hits))
	}
	if label != "" {
		if _, ok := d.Find(label); !ok {
			t.Errorf("%s: %q missing", name, label)
		}
	}
}

func TestG3MenuToken(t *testing.T) {
	col := theme.Hex("#12ab34")
	g3ScopeTest(t, "Menu.ItemSelectedBg", theme.Components{Menu: theme.MenuToken{ItemSelectedBg: col}}, col, "Profile", func(c *Context) {
		sel := "a"
		Menu(c, g3Menu(), &sel).Width(200).Show()
	})
	dark := theme.Hex("#223344")
	g3ScopeTest(t, "Menu.DarkItemBg", theme.Components{Menu: theme.MenuToken{DarkItemBg: dark}}, dark, "", func(c *Context) {
		sel := "home"
		Menu(c, g3Menu(), &sel).Dark().Width(200).Show()
	})
}

func TestG3TabsToken(t *testing.T) {
	col := theme.Hex("#ab1234")
	items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}
	g3ScopeTest(t, "Tabs.InkBarColor", theme.Components{Tabs: theme.TabsToken{InkBarColor: col}}, col, "Alpha", func(c *Context) {
		act := "a"
		Tabs(c, &act, items).Show()
	})
	card := theme.Hex("#fedcba")
	g3ScopeTest(t, "Tabs.CardBg", theme.Components{Tabs: theme.TabsToken{CardBg: card}}, card, "", func(c *Context) {
		act := "a"
		Tabs(c, &act, items).Card().Show()
	})
}

func TestG3PaginationToken(t *testing.T) {
	col := theme.Hex("#0a0b0c")
	g3ScopeTest(t, "Pagination.ItemActiveBorderColor", theme.Components{Pagination: theme.PaginationToken{ItemActiveBorderColor: col}}, col, "", func(c *Context) {
		p := 2
		Pagination(c, &p, 50, 10).Show()
	})
	// ItemSize changes the measured page buttons in scope only.
	var in, out *PaginationW
	d := NewDriver(func(c *Context) {
		p1, p2 := 2, 2
		// (Not at the root: a root provider sets the window theme.)
		Column(c, func() {
			ConfigProvider(c).Components(theme.Components{Pagination: theme.PaginationToken{ItemSize: 40}}).Body(func() {
				in = Pagination(c, &p1, 50, 10).Show()
			})
			out = Pagination(c, &p2, 50, 10).Show()
		})
	}, DriverOptions{Width: 800, Height: 300, Theme: theme.AntDesign()})
	d.Settle()
	_, _ = in, out
	var hs []geom.Rect
	for _, h := range d.r.c.hits {
		if h.label == "3" {
			hs = append(hs, h.rect)
		}
	}
	if len(hs) != 2 || hs[0].H != 40 || hs[1].H != 32 {
		t.Fatalf("page rects %v", hs)
	}
}

func TestG3LayoutToken(t *testing.T) {
	col := theme.Hex("#334455")
	g3ScopeTest(t, "Layout.HeaderBg", theme.Components{Layout: theme.LayoutToken{HeaderBg: col}}, col, "", func(c *Context) {
		Layout(c, func() {
			Header(c, func() { Text(c, "H") })
			Content(c, func() { Text(c, "C") })
		}, layout.H(200))
	})
	tr := theme.Hex("#556677")
	g3ScopeTest(t, "Layout.LightTriggerBg", theme.Components{Layout: theme.LayoutToken{LightTriggerBg: tr}}, tr, "", func(c *Context) {
		coll := false
		Layout(c, func() {
			CollapsibleSider(c, 160, &coll).Light().Body(func() {})
			Content(c, func() {})
		}, layout.H(200))
	})
}
