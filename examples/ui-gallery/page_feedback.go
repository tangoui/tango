package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	gallery.Register(gallery.Page{Name: "Alert", Title: "Alert 警告提示", Group: gallery.Feedback, Order: 1, Render: func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Alert(c, "success", "成功提示的文案").Show()
			ui.Alert(c, "info", "消息提示的文案").Show()
			ui.Alert(c, "warning", "警告提示的文案").Closable().Show()
			ui.Alert(c, "error", "错误提示的文案").Show()
			ui.Alert(c, "success", "成功提示的文案").Description("成功提示的辅助性文字介绍，成功提示的辅助性文字介绍。").Show()
			ui.Alert(c, "error", "错误").Description("错误提示的辅助性文字介绍。").Show()
		}, layout.Gap(16))
	}})
	gallery.Register(gallery.Page{Name: "Popconfirm", Title: "Popconfirm 气泡确认框", Group: gallery.Feedback, Order: 5, Height: 300,
		Render: func(c *ui.Context) {
			ui.Row(c, func() {
				ui.Popconfirm(c, "删除任务", "确定要删除这个任务吗？", func() { ui.Button(c, "删除").Danger() })
			}, layout.PadVH(120, 80))
		},
		Setup: func(d *ui.Driver) { d.ClickLabel("删除") }})
}
