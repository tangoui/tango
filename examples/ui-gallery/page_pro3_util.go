package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// pro3Page registers a demo of the second Pro component group in three
// themes (Ant Design, Element Plus, Ant Design dark). mk builds a fresh
// page for every variant; the components' texts come from the built-in
// zh-CN pack (the gallery's default locale).
func pro3Page(name, title, group string, order int, width, height float32, mk func() (func(c *ui.Context), func(d *ui.Driver))) {
	variants := []struct {
		suffix string
		th     func() *theme.Theme
	}{
		{"", nil},
		{"-element", func() *theme.Theme { return theme.ElementPlus() }},
		{"-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	}
	for _, v := range variants {
		render, setup := mk()
		r := render
		p := gallery.Page{Name: name + v.suffix, Title: title, Group: group, Order: order, Width: width, Height: height,
			Render: func(c *ui.Context) { r(c) }}
		p.Variant = v.th != nil
		p.Interact = setup
		thf := v.th
		p.Setup = func(d *ui.Driver) {
			if thf != nil {
				t := thf()
				t.BgLayout = t.BgContainer
				d.SetTheme(t)
				d.Settle()
			}
			if setup != nil {
				setup(d)
			}
		}
		gallery.Register(p)
	}
}
