package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// p2ePage registers a demo of the Pro / Table additions in three themes
// (Ant Design, Element Plus, Ant Design dark); parent
// files the page under an existing component page ("" for a new one).
func p2ePage(name, title, group, parent string, order int, width, height float32, mk func() (func(c *ui.Context), func(d *ui.Driver))) {
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
		p := gallery.Page{Name: name + v.suffix, Title: title, Group: group, Order: order, Width: width, Height: height, Parent: parent,
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

// p2eHold presses at from and moves to to without releasing (a drag in
// progress for the screenshot).
func p2eHold(d *ui.Driver, from, to geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from})
	d.Frame()
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)})
		d.Frame()
	}
}
