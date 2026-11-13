package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var rateDemo = struct {
	a, b, c, d, e, f float64
	seg1, seg2, seg3 string
	seg4, seg5       string
}{a: 3, b: 2.5, c: 4, d: 3, e: 2, f: 3.5, seg1: "每周", seg2: "列表", seg3: "中", seg4: "Kanban", seg5: "每月"}

func init() {
	entryPage(gallery.Page{Name: "Rate", Title: "Rate 评分", Order: 20, Render: func(c *ui.Context) {
		d := &rateDemo
		gallery.Demo(c, "基本用法", "点击评分，再次点击当前分值可清除。", func() {
			ui.Rate(c, &d.a)
		})
		gallery.Demo(c, "半星与文字", "AllowHalf 支持半星；Texts 显示辅助文字，ShowScore 显示分数。", func() {
			entryCol(c, func() {
				ui.Rate(c, &d.b).AllowHalf().ShowScore()
				ui.Rate(c, &d.c).Texts("极差", "失望", "一般", "满意", "惊喜")
			})
		})
		gallery.Demo(c, "自定义字符、颜色与尺寸", "", func() {
			entryCol(c, func() {
				ui.Rate(c, &d.d).Character("好").Size(18)
				ui.Rate(c, &d.f).AllowHalf().Character("A").Size(24)
				ui.Rate(c, &d.e).Count(10).Color(theme.Hex("#ff4d4f")).Size(16)
			})
		})
		gallery.Demo(c, "只读 / 禁用", "", func() {
			ui.Rate(c, &d.b).AllowHalf().Disabled(true)
		})
	}})

	entryPage(gallery.Page{Name: "Segmented", Title: "Segmented 分段控制器", Order: 21, Render: func(c *ui.Context) {
		d := &rateDemo
		gallery.Demo(c, "基本用法", "", func() {
			ui.Segmented(c, &d.seg1, ui.Opts("每日", "每周", "每月", "每季度", "每年"))
		})
		gallery.Demo(c, "Block 与禁用项", "Block 撑满容器，选项可单独禁用。", func() {
			entryCol(c, func() {
				ui.Segmented(c, &d.seg5, ui.Opts("每日", "每周", "每月")).Block()
				ui.Segmented(c, &d.seg4, []ui.Option{{Label: "Map", Value: "Map"}, {Label: "Transit", Value: "Transit", Disabled: true}, {Label: "Kanban", Value: "Kanban"}})
				ui.Segmented(c, &d.seg1, ui.Opts("每日", "每周", "每月")).Disabled(true)
			})
		})
		gallery.Demo(c, "尺寸与图标", "", func() {
			entryCol(c, func() {
				ui.Segmented(c, &d.seg3, ui.Opts("小", "中", "大")).Size(ui.Small)
				ui.Segmented(c, &d.seg3, ui.Opts("小", "中", "大"))
				ui.Segmented(c, &d.seg3, ui.Opts("小", "中", "大")).Size(ui.Large)
				ui.Segmented(c, &d.seg2, ui.Opts("列表", "看板")).Icons(icon.Table, icon.Appstore)
				ui.Segmented(c, &d.seg2, []ui.Option{{Value: "列表"}, {Value: "看板"}}).Icons(icon.Table, icon.Appstore)
			}, layout.AlignItems(layout.AlignStart))
		})
	}})
}
