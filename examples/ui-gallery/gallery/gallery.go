// Package gallery is the page registry of the ui-gallery example. Each
// component registers a demo page from its own file:
//
//	func init() {
//		gallery.Register(gallery.Page{Name: "Button", Group: gallery.General, Render: buttonPage})
//	}
//
// The gallery program renders every page to build/ui-gallery/components/<name>.png
// and composes the overview from the pages of the General and Form groups.
package gallery

import (
	"sort"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// Group names, in display order.
const (
	General    = "通用"
	Layout     = "布局"
	Navigation = "导航"
	Form       = "数据录入"
	Display    = "数据展示"
	Feedback   = "反馈"
)

var groupOrder = map[string]int{General: 0, Layout: 1, Navigation: 2, Form: 3, Display: 4, Feedback: 5}

// Page is one registered demo.
type Page struct {
	Name   string // component name, also the PNG file name
	Title  string // display title, e.g. "Button 按钮"
	Group  string
	Order  int     // sort key inside the group
	Width  float32 // screenshot width (default 960)
	Height float32 // screenshot height (default: fit content up to 1400)
	Render func(c *ui.Context)
	// Status returns a live textual summary for documentation hosts, e.g. an
	// editor caret position and document length. It must not mutate page state.
	Status func() string
	// Parent names the component page this one is an extra section of
	// (Table sections, Image preview). Home shows it under the parent
	// instead of listing it separately; it still gets its own PNG.
	Parent string
	// Setup runs once before the screenshot (e.g. drive hover/open state).
	Setup func(d *ui.Driver)
	// Variant marks a themed copy of another page (Name-Element,
	// Name-Dark …); theme passes such as -theme shadcn skip it.
	Variant bool
	// Interact is Setup without any theme switching (the page's own
	// interactions only); themed passes run it after picking their theme.
	// Nil means Setup is theme-neutral.
	Interact func(d *ui.Driver)
	// Bare renders the page without the titled Frame (full-bleed demos
	// such as dashboards).
	Bare bool
}

var pages []Page

// Register adds a page. Call it from an init function.
func Register(p Page) {
	if p.Title == "" {
		p.Title = p.Name
	}
	pages = append(pages, p)
}

// Pages returns all pages sorted by group, order and name.
func Pages() []Page {
	out := append([]Page(nil), pages...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if groupOrder[a.Group] != groupOrder[b.Group] {
			return groupOrder[a.Group] < groupOrder[b.Group]
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.Name < b.Name
	})
	return out
}

// Demo renders a titled demo block (a card with a short description) used
// inside pages so every page looks alike.
func Demo(c *ui.Context, title, desc string, fn func()) {
	ui.Card(c, title).Layout(layout.Margin(geom.Insets{Bottom: 16})).Body(func() {
		if desc != "" {
			ui.Text(c, desc).Tertiary().Layout(layout.Margin(geom.Insets{Bottom: 16}))
		}
		fn()
	})
}

// Frame wraps a page in the standard padded canvas.
func Frame(c *ui.Context, title string, fn func()) {
	ui.Column(c, func() {
		ui.Title(c, title, 3).Layout(layout.Margin(geom.Insets{Bottom: 16}))
		fn()
	}, layout.Pad(24))
}
