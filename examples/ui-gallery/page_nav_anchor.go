package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

var anchorItems = []ui.AnchorItem{
	{Key: "basic", Title: "基础用法"},
	{Key: "static", Title: "静态位置", Children: []ui.AnchorItem{{Key: "offset", Title: "偏移量"}}},
	{Key: "api", Title: "API 参考"},
	{Key: "faq", Title: "常见问题"},
}

func anchorPage(c *ui.Context) {
	th := c.Theme
	navDemo(c, "Anchor 锚点 + Affix 固钉 + BackTop 回到顶部", "右侧内容在容器内滚动：锚点随滚动高亮当前章节，点击平滑滚动；工具条滚过顶部后固定在容器顶部；滚动超过 200px 显示回到顶部按钮。", func() {
		var sb *ui.ScrollBox
		ui.Row(c, func() {
			var target ui.ID
			if r := ui.State[ui.ID](c, ui.ID(7001), nil); r != nil {
				target = *r
			}
			ui.Column(c, func() {
				ui.Anchor(c, anchorItems).Target(target).Show()
				ui.Gap(c, 24)
				ui.Text(c, "横向锚点").Tertiary()
				ui.Gap(c, 8)
				ui.Anchor(c, anchorItems[:3]).Horizontal().Target(target).Show()
			}, layout.W(240))
			sb = ui.Scroll(c, func() {
				ui.Affix(c).OffsetTop(0).Target(target).Body(func() {
					ui.Row(c, func() {
						ui.Text(c, "固钉工具条").Strong()
						ui.Spacer(c)
						ui.Button(c, "保存").Primary().Small()
					}, layout.PadVH(8, 16), layout.Middle()).Bg(th.Primary.Bg).Border(1, th.Primary.Border).Radius(th.Radius)
				})
				for _, it := range []struct{ key, title string }{{"basic", "基础用法"}, {"static", "静态位置"}, {"offset", "偏移量"}, {"api", "API 参考"}, {"faq", "常见问题"}} {
					ui.AnchorSection(c, it.key, func() {
						ui.Title(c, it.title, 4)
						for i := 0; i < 3; i++ {
							ui.Paragraph(c, "锚点用于跳转到页面指定位置，并随滚动高亮当前所在的章节。内容区域在容器内滚动，锚点通过 Target 关联到滚动容器。")
						}
					}, layout.Gap(8), layout.Margin(geom.Insets{Top: 16}))
				}
				ui.Gap(c, 200)
			}, layout.H(360), layout.Grow(1), layout.PadVH(0, 16))
			*ui.State[ui.ID](c, ui.ID(7001), nil) = sb.ID()
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
		ui.BackTop(c, sb.ID()).VisibilityHeight(200).Position(80, 60).Show()
	})
}

func init() {
	navPage(gallery.Page{Name: "Anchor", Title: "Anchor 锚点 / Affix 固钉 / BackTop", Group: gallery.Navigation, Order: 20, Height: 560,
		Render: anchorPage,
		Setup: func(d *ui.Driver) {
			r, _ := d.Find("偏移量")
			_ = r
			d.Scroll(geom.Pt(600, 300), 330)
			d.Settle()
			d.Frame()
		}}, nil)
}
