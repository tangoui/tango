package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// entryPage registers one data-entry demo three times: Ant Design (Name),
// Element Plus (Name-Element) and Ant dark (Name-Dark). The gallery runner
// renders pages with the Ant theme; the variants switch theme in Setup.
func entryPage(p gallery.Page) {
	p.Group = gallery.Form
	if p.Order == 0 {
		p.Order = 40
	}
	setup := p.Setup
	gallery.Register(p)
	el := p
	el.Variant = true
	el.Name, el.Title = p.Name+"-Element", p.Title+"（Element Plus）"
	el.Setup = func(d *ui.Driver) {
		t := theme.ElementPlus()
		t.BgLayout = t.BgContainer
		d.SetTheme(t)
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(el)
	dk := p
	dk.Variant = true
	dk.Name, dk.Title = p.Name+"-Dark", p.Title+"（暗色）"
	dk.Setup = func(d *ui.Driver) {
		d.SetTheme(theme.AntDesign(theme.Dark()))
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(dk)
}

// entryRow lays out demo controls in a wrapping row.
func entryRow(c *ui.Context, fn func()) {
	ui.Row(c, fn, layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart))
}

// entryCol stacks demo controls.
func entryCol(c *ui.Context, fn func(), opts ...layout.Option) {
	ui.Column(c, fn, append([]layout.Option{layout.Gap(12)}, opts...)...)
}
