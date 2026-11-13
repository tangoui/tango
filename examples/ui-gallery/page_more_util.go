package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// morePage registers a demo of the ui/more_*.go components twice: Ant
// Design (Name) and Element Plus (Name-Element). reset restores the demo
// state before each screenshot so the variants start alike.
func morePage(p gallery.Page, reset func()) {
	setup := p.Setup
	base := p
	base.Setup = func(d *ui.Driver) {
		if reset != nil {
			reset()
			d.Settle()
		}
		if setup != nil {
			setup(d)
		}
	}
	base.Interact = base.Setup
	gallery.Register(base)
	el := p
	el.Variant = true
	el.Name, el.Title = p.Name+"-Element", p.Title+"（Element Plus）"
	el.Setup = func(d *ui.Driver) {
		if reset != nil {
			reset()
		}
		t := theme.ElementPlus()
		t.BgLayout = t.BgContainer
		d.SetTheme(t)
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(el)
}

// moreRow lays out demo controls in a wrapping row.
func moreRow(c *ui.Context, fn func()) {
	ui.Row(c, fn, layout.Gap(8), layout.WrapItems(), layout.Middle())
}
