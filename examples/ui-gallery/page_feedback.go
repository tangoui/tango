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
			// Element effect="dark" 与 center
			ui.Alert(c, "success", "深色主题的成功提示").Effect(ui.AlertEffectDark).Show()
			ui.Alert(c, "info", "深色主题的消息提示").Effect(ui.AlertEffectDark).Closable().Show()
			ui.Alert(c, "warning", "深色、居中的警告提示").Effect(ui.AlertEffectDark).Center().Show()
			ui.Alert(c, "error", "深色错误").Effect(ui.AlertEffectDark).Description("深色背景上的辅助性文字介绍。").Show()
			ui.Alert(c, "info", "居中的消息提示").Center().Closable().Show()
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
