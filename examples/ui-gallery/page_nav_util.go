package main

import (
	"path/filepath"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// navPage registers a page that is also rendered with the Element Plus and
// dark themes (<Name>-element.png, <Name>-dark.png, <Name>-element-dark.png)
// next to the default Ant Design screenshot. reset restores the page's
// app state before every variant so Setup interactions start fresh.
func navPage(p gallery.Page, reset func()) {
	orig := p.Setup
	p.Interact = func(d *ui.Driver) {
		if reset != nil {
			reset()
			d.Settle()
		}
		if orig != nil {
			orig(d)
		}
	}
	p.Setup = func(d *ui.Driver) {
		w := p.Width
		if w == 0 {
			w = 960
		}
		page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
		flat := func(t *theme.Theme) *theme.Theme { t.BgLayout = t.BgContainer; return t }
		variants := []struct {
			suffix string
			th     *theme.Theme
		}{
			{"-element", flat(theme.ElementPlus())},
			{"-dark", flat(theme.AntDesign(theme.Dark()))},
			{"-element-dark", flat(theme.ElementPlus(theme.Dark()))},
		}
		for _, v := range variants {
			if reset != nil {
				reset()
			}
			if err := shotOnce(filepath.Join("components", p.Name+v.suffix), w, p.Height, v.th, page, orig); err != nil {
				panic(err)
			}
		}
		if reset != nil {
			reset()
		}
		if orig != nil {
			orig(d)
		}
	}
	gallery.Register(p)
}

// navDemo is a titled demo block.
func navDemo(c *ui.Context, title, desc string, fn func()) { gallery.Demo(c, title, desc, fn) }

// navCols lays out demos side by side.
func navCols(c *ui.Context, fns ...func()) {
	ui.Row(c, func() {
		for _, f := range fns {
			ui.Column(c, f, layout.Grow(1), layout.MinW(0))
		}
	}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
}

func navPad(v float32) geom.Insets { return geom.Insets{Bottom: v} }

func pointerPress(p geom.Point) input.Event {
	return input.Pointer{Kind: input.Press, Pos: p, Button: input.ButtonLeft}
}

func pointerRelease(p geom.Point) input.Event {
	return input.Pointer{Kind: input.Release, Pos: p, Button: input.ButtonLeft}
}
