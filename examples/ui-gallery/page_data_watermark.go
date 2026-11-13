package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

func init() {
	dataPage(gallery.Page{Name: "Watermark", Title: "Watermark 水印", Order: 33, Render: func(c *ui.Context) {
		para := "Tango 是一个纯 Go 的应用框架，自绘 GUI 层提供与 Ant Design、Element Plus 视觉一致的组件。水印平铺在内容之上，但不会拦截鼠标事件。"
		gallery.Demo(c, "基础用法", "单行文字水印，默认旋转 -22°、间距 100。", func() {
			ui.Watermark(c, "Tango UI").Body(func() {
				ui.Column(c, func() {
					for i := 0; i < 3; i++ {
						ui.Paragraph(c, para)
					}
					ui.Button(c, "水印下的按钮仍可点击").Primary()
				}, layout.H(330))
			})
		})
		gallery.Demo(c, "多行水印 / 自定义样式", "多行文字、加粗、彩色、自定义旋转与间距。", func() {
			ui.Row(c, func() {
				ui.Watermark(c, "Tango", "内部资料 请勿外传").Weight(text.Semibold).FontSize(14).Layout(layout.Grow(1)).Body(func() {
					ui.Column(c, func() { ui.Paragraph(c, para) }, layout.H(200))
				})
				ui.Watermark(c, "CONFIDENTIAL").Rotate(-45).Gap(60, 60).Color(theme.Fade(c.Theme.Error.Base, 0.2)).Layout(layout.Grow(1)).Body(func() {
					ui.Column(c, func() { ui.Paragraph(c, para) }, layout.H(200))
				})
				ui.Watermark(c).Image(dataImgAvatar, 40, 40).Gap(80, 80).Layout(layout.Grow(1)).Body(func() {
					ui.Column(c, func() { ui.Paragraph(c, para) }, layout.H(200))
				})
			}, layout.Gap(16))
		})
	}})
}
