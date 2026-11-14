package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// AspectRatio: content laid out at a fixed width-to-height ratio.

// aspectFill fills a ratio box with a placeholder "photo": a soft
// diagonal gradient of the theme's primary color, a picture icon and the
// ratio caption.
func aspectFill(c *ui.Context, caption string) {
	th := c.Theme
	ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{} }, func(l *paint.List, r geom.Rect) {
		l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: r,
			Start: geom.Pt(r.X, r.Y), End: geom.Pt(r.Right(), r.Bottom()),
			Stops: []paint.Stop{{Offset: 0, Color: th.Primary.Bg}, {Offset: 1, Color: th.Primary.BgHover}}})
	})
	ui.Column(c, func() {
		ui.Icon(c, icon.Picture).Size(32).Color(th.Primary.Base)
		ui.Text(c, caption).Strong().Color(th.TextOn(th.Primary.Bg, th.Text))
	}, layout.Center(), layout.Gap(8))
}

func aspectRatioPage(c *ui.Context) {
	gallery.Demo(c, "固定宽高比", "宽度随父容器，高度按比例计算；内容铺满并按圆角裁剪。", func() {
		ui.Row(c, func() {
			for _, x := range []struct {
				name  string
				ratio float32
			}{{"16 / 9", 16.0 / 9}, {"4 / 3", 4.0 / 3}, {"1 / 1", 1}} {
				ui.Column(c, func() {
					ui.AspectRatio(c, x.ratio, func() { aspectFill(c, x.name) }).Radius(c.Theme.RadiusLG).A11yLabel(x.name)
				}, layout.Grow(1))
			}
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
	})
	gallery.Demo(c, "最大宽度", "MaxWidth 限制宽度，高度一同缩放。", func() {
		ui.AspectRatio(c, 21.0/9, func() { aspectFill(c, "21 / 9") }).Radius(c.Theme.RadiusLG).MaxWidth(420).A11yLabel("21 / 9")
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "AspectRatio", Title: "AspectRatio 宽高比", Group: gallery.Layout, Order: 40, Render: aspectRatioPage})
}
