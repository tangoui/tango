package gallery

import (
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Theme switch of the interactive home: "" follows the window theme until
// the user picks a design system or toggles dark mode.
var (
	homeStyle string
	homeDark  *bool
)

var homeStyles = []ui.Option{{Label: "Ant Design", Value: "antd"}, {Label: "Element Plus", Value: "element-plus"}, {Label: "shadcn/ui", Value: "shadcn"}}

func flavorOf(s string) theme.Flavor {
	switch s {
	case "element-plus":
		return theme.Element
	case "shadcn":
		return theme.StyleShadcn
	}
	return theme.Ant
}

// themeBar is the design-system switch shown above the catalog.
func themeBar(c *ui.Context) {
	cur := c.Theme.Flavor.String()
	if homeStyle != "" {
		cur = homeStyle
	}
	dark := c.Theme.Dark
	ui.Row(c, func() {
		ui.Text(c, "设计风格").Tertiary()
		if ui.Segmented(c, &cur, homeStyles).Changed() {
			homeStyle = cur
		}
		ui.Text(c, "暗色").Tertiary()
		if ui.Switch(c, &dark).Changed() {
			homeDark = &dark
		}
	}, layout.Gap(12), layout.Middle(), layout.Margin(geom.Insets{Bottom: 16}))
}

// Groups returns the group names in display order.
func Groups() []string {
	return []string{General, Layout, Navigation, Form, Display, Feedback}
}

// IsVariant reports whether p is a theme variant screenshot of another page
// (Name-Dark, Name-Element, ...); the home page lists each component once.
func IsVariant(p Page) bool { return p.Variant || strings.Contains(p.Name, "-") }

// ByGroup returns the component pages (no theme variants) of one group in
// display order.
func ByGroup(group string) []Page {
	var out []Page
	for _, p := range Pages() {
		if p.Group == group && !IsVariant(p) && p.Parent == "" {
			out = append(out, p)
		}
	}
	return out
}

// Find returns the page called name.
func Find(name string) (Page, bool) {
	for _, p := range pages {
		if p.Name == name {
			return p, true
		}
	}
	return Page{}, false
}

// Home is the gallery's start page: a side menu listing every component by
// group and, on the right, either the catalog (sel is "") or the selected
// demo page. Clicking a component in the menu or the catalog opens it.
// The catalog header switches the design system (Ant Design, Element Plus,
// shadcn/ui) and dark mode for the whole gallery.
func Home(c *ui.Context, sel *string) {
	if homeStyle == "" && homeDark == nil {
		home(c, sel)
		return
	}
	cp := ui.ConfigProvider(c)
	if homeStyle != "" {
		cp.Flavor(flavorOf(homeStyle))
	}
	if homeDark != nil {
		cp.Dark(*homeDark)
	}
	cp.Body(func() { home(c, sel) })
}

func home(c *ui.Context, sel *string) {
	var items []ui.MenuItem
	open := make([]string, 0, len(Groups()))
	items = append(items, ui.MenuItem{Key: "home", Label: "全部组件"})
	for _, g := range Groups() {
		ps := ByGroup(g)
		if len(ps) == 0 {
			continue
		}
		it := ui.MenuItem{Key: "group:" + g, Label: g + " (" + strconv.Itoa(len(ps)) + ")"}
		for _, p := range ps {
			it.Children = append(it.Children, ui.MenuItem{Key: p.Name, Label: p.Title})
		}
		items = append(items, it)
		open = append(open, it.Key)
	}
	ui.Layout(c, func() {
		ui.Sider(c, 240, false, func() {
			ui.Scroll(c, func() {
				ui.Menu(c, items, sel).DefaultOpen(open...).Width(240).Show()
			}, layout.Grow(1))
		})
		ui.Content(c, func() {
			ui.Scroll(c, func() {
				if p, ok := Find(*sel); ok {
					Frame(c, p.Title, func() {
						p.Render(c)
						for _, s := range Pages() {
							if s.Parent == p.Name && !IsVariant(s) {
								ui.Title(c, s.Title, 4).Layout(layout.Margin(geom.Insets{Top: 24, Bottom: 12}))
								s.Render(c)
							}
						}
					})
					return
				}
				catalog(c, sel)
			}, layout.Grow(1))
		})
	})
}

// catalog lists every component as a clickable card, grouped.
func catalog(c *ui.Context, sel *string) {
	n := 0
	for _, g := range Groups() {
		n += len(ByGroup(g))
	}
	Frame(c, "Tango UI 组件", func() {
		themeBar(c)
		ui.Text(c, "共 "+strconv.Itoa(n)+" 个组件页（另有暗色 / Element 主题变体截图），按分组浏览；点击卡片打开组件页。").Tertiary().
			Layout(layout.Margin(geom.Insets{Bottom: 16}))
		for _, g := range Groups() {
			ps := ByGroup(g)
			if len(ps) == 0 {
				continue
			}
			ui.Title(c, g, 4).Layout(layout.Margin(geom.Insets{Top: 8, Bottom: 12}))
			ui.Row(c, func() {
				for _, p := range ps {
					p := p
					ui.Key(c, p.Name, func() {
						b := ui.Div(c, layout.W(200), layout.PadVH(12, 16), layout.Gap(4)).
							Bg(c.Theme.BgContainer).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.RadiusLG)
						b.Body(func() {
							ui.Text(c, p.Name).Strong()
							ui.Text(c, p.Title).Secondary()
						})
						if b.Clicked() {
							*sel = p.Name
							c.Invalidate()
						}
					})
				}
			}, layout.WrapItems(), layout.Gap(12), layout.Margin(geom.Insets{Bottom: 16}))
		}
	})
}
