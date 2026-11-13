package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// pro2Page registers a demo of the ui/pro2_*.go components twice: Ant
// Design (Name) and Element Plus (Name-Element). reset restores the demo
// state before each screenshot.
func pro2Page(p gallery.Page, reset func()) {
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
		d.SetTheme(theme.ElementPlus())
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(el)
}

// pro2Canvas paints a grey layout background behind cards, as the Pro
// component demos do.
func pro2Canvas(c *ui.Context, fn func(), opts ...layout.Option) *ui.Box {
	o := append([]layout.Option{layout.Pad(16), layout.Gap(16)}, opts...)
	return ui.Column(c, fn, o...).Bg(c.Theme.FillTertiary).Radius(c.Theme.Radius)
}
