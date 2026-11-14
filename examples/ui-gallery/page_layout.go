package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func init() {
	navPage(gallery.Page{Name: "Layout", Title: "Layout 布局", Group: gallery.Layout, Order: 1, Render: layoutPage}, nil)
}

func layoutPage(c *ui.Context) {
	th := c.Theme
	hdr := theme.Lerp(th.Primary.Bg, th.Primary.Shades[2], 0.5)
	side := theme.Lerp(th.Primary.Bg, th.Primary.Shades[2], 0.25)
	main := th.Primary.Bg
	label := func(s string) func() { return func() { ui.Text(c, s) } }
	box := func(fn func()) { ui.Column(c, fn).Border(1, th.BorderSecondary).Radius(th.Radius).Clip() }
	navDemo(c, "主题默认样式", "Header / Sider / Content / Footer 使用当前设计语言的默认尺寸与配色。", func() {
		white := th.TextOn(th.SiderBg, theme.Hex("#ffffff")) // shadcn's sider is light
		box(func() {
			ui.Layout(c, func() {
				ui.Header(c, func() { ui.Text(c, "Header").Color(white) }).Bg(th.SiderBg)
				ui.Layout(c, func() {
					ui.Sider(c, 200, false, func() {
						ui.Column(c, func() { ui.Text(c, "Sider").Color(white) }, layout.Grow(1), layout.Center())
					}).Bg(th.SiderBg)
					ui.Content(c, func() {
						ui.Column(c, label("Content"), layout.H(120), layout.Center()).Bg(th.BgContainer).Radius(th.RadiusLG)
					}, layout.Pad(24))
				})
				ui.Footer(c, label("Footer ©2026"))
			})
		})
	})
	navDemo(c, "常见布局", "Layout 直接包含 Sider 时横排，否则竖排；Content 与嵌套 Layout 占满剩余空间。", func() {
		ui.Column(c, func() {
			box(func() {
				ui.Layout(c, func() {
					ui.Header(c, label("Header")).Bg(hdr)
					ui.Content(c, label("Content"), layout.H(100), layout.Pad(20)).Bg(main)
				})
			})
			box(func() {
				ui.Layout(c, func() {
					ui.Header(c, label("Header")).Bg(hdr)
					ui.Layout(c, func() {
						ui.Sider(c, 200, false, label("Sider"), layout.Pad(20)).Bg(side)
						ui.Content(c, label("Content"), layout.H(140), layout.Pad(20)).Bg(main)
					})
					ui.Footer(c, label("Footer")).Bg(hdr)
				})
			})
			box(func() {
				ui.Layout(c, func() {
					ui.Sider(c, 200, false, label("Sider"), layout.Pad(20)).Bg(side)
					ui.Layout(c, func() {
						ui.Header(c, label("Header")).Bg(hdr)
						ui.Content(c, label("Content"), layout.H(100), layout.Pad(20)).Bg(main)
						ui.Footer(c, label("Footer")).Bg(hdr)
					})
				})
			})
		}, layout.Gap(16))
	})
}
