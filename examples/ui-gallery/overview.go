package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// overview stacks the demo pages of the General and Form groups in two
// columns.
func overview(c *ui.Context) {
	var ps []gallery.Page
	for _, p := range gallery.Pages() {
		if p.Group == gallery.General || p.Group == gallery.Form {
			ps = append(ps, p)
		}
	}
	ui.Column(c, func() {
		ui.Title(c, "Tango UI 组件总览", 2)
		ui.Text(c, "Go 自绘组件库 · Ant Design 主题 · 按钮与表单控件的各种状态").Tertiary().Layout(layout.Margin(geom.Insets{Bottom: 24}))
		ui.Row(c, func() {
			for col := 0; col < 2; col++ {
				col := col
				ui.Column(c, func() {
					for i, p := range ps {
						if i%2 != col {
							continue
						}
						ui.Title(c, p.Title, 4).Layout(layout.Margin(geom.Insets{Bottom: 12}))
						ui.Column(c, func() { p.Render(c) }, layout.Margin(geom.Insets{Bottom: 32}))
					}
				}, layout.Grow(1), layout.MinW(0))
			}
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
	}, layout.Pad(32))
}
