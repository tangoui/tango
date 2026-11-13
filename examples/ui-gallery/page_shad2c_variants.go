package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// Element Plus and dark variants of the navigation / feedback pages that
// had none (ui-coverage.md 5.2). The base page is looked up by name when
// rendered, so the registration order of the files does not matter.

func shad2cBase(name string) (gallery.Page, bool) {
	for _, p := range gallery.Pages() {
		if p.Name == name {
			return p, true
		}
	}
	return gallery.Page{}, false
}

func shad2cVariant(base, suffix, title string, mk func() *theme.Theme) {
	gallery.Register(gallery.Page{
		Name: base + suffix, Title: base + title, Group: gallery.Navigation, Order: 900, Variant: true, Parent: base,
		Height: shad2cHeights[base], // the base page's fixed screenshot height (if any)
		Render: func(c *ui.Context) {
			if p, ok := shad2cBase(base); ok {
				p.Render(c)
			}
		},
		Setup: func(d *ui.Driver) {
			t := mk()
			t.BgLayout = t.BgContainer
			d.SetTheme(t)
			d.Settle()
			if p, ok := shad2cBase(base); ok {
				setup := p.Interact
				if setup == nil {
					setup = p.Setup
				}
				if setup != nil {
					setup(d)
				}
			}
		},
	})
}

var shad2cHeights = map[string]float32{"Pagination": 300, "Popconfirm": 300, "Command": 520, "Confirm": 640, "MessageBox": 640, "Prompt": 640}

func init() {
	el := func() *theme.Theme { return theme.ElementPlus() }
	dark := func() *theme.Theme { return theme.AntDesign(theme.Dark()) }
	elDark := func() *theme.Theme { return theme.ElementPlus(theme.Dark()) }
	for _, n := range []string{"Alert", "Breadcrumb", "Pagination", "Popconfirm", "Tabs", "Command", "Toggle"} {
		shad2cVariant(n, "-Element", "（Element Plus）", el)
		shad2cVariant(n, "-Dark", "（暗色）", dark)
	}
	for _, n := range []string{"Confirm", "MessageBox", "Prompt"} {
		shad2cVariant(n, "-Dark", "（暗色）", dark)
		shad2cVariant(n, "-Element-Dark", "（Element 暗色）", elDark)
	}
}
