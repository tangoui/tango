package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// dataPage registers a data-display demo three times: Ant Design (light),
// Element Plus (light) and Ant Design dark. The gallery renders pages with
// the Ant theme; the variants switch theme in Setup before the screenshot.
// Output: components/<Name>.png, <Name>-element.png, <Name>-dark.png.
func dataPage(p gallery.Page) {
	p.Group = gallery.Display
	if p.Order == 0 {
		p.Order = 50
	}
	setup := p.Setup
	gallery.Register(p)
	for _, v := range []struct {
		suffix string
		th     func() *theme.Theme
	}{
		{"-element", func() *theme.Theme { return theme.ElementPlus() }},
		{"-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	} {
		q := p
		q.Variant = true
		q.Name += v.suffix
		mk := v.th
		q.Setup = func(d *ui.Driver) {
			t := mk()
			t.BgLayout = t.BgContainer
			d.SetTheme(t)
			d.Settle()
			if setup != nil {
				setup(d)
			}
		}
		gallery.Register(q)
	}
}

// dataRow lays out demo items in a wrapping row.
func dataRow(c *ui.Context, fn func(), gap ...float32) {
	g := float32(16)
	if len(gap) > 0 {
		g = gap[0]
	}
	ui.Row(c, fn, layout.Gap(g), layout.AlignItems(layout.AlignStart), layout.WrapItems(), layout.Margin(geom.Insets{Bottom: 12}))
}
