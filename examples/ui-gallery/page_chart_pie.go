package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
)

var pieView chart.View

var channelShare = []chart.Row{
	{"type": "搜索引擎", "value": 1048}, {"type": "直接访问", "value": 735}, {"type": "邮件营销", "value": 580},
	{"type": "联盟广告", "value": 484}, {"type": "视频广告", "value": 300}, {"type": "其他", "value": 120},
}

func init() {
	chartPage(gallery.Page{Name: "ChartPie", Title: "Pie 饼图与环形图", Order: 203,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "饼图（标签引导线）", func() {
						pieView = chart.Pie(c, channelShare).Angle("value").Color("type").Legend(chart.LegendNone).Show()
					})
				},
				func() {
					chartCard(c, "环形图（中心统计）", func() {
						chart.Donut(c, channelShare).Angle("value").Color("type").NoLabel().Show()
					})
				},
				func() {
					chartCard(c, "内部百分比", func() {
						chart.Pie(c, channelShare).Angle("value").Color("type").InnerLabel().Show()
					})
				},
				func() {
					chartCard(c, "环形图 + 标签", func() {
						chart.Donut(c, channelShare).Angle("value").Color("type").Center("访问量", "3,267").Legend(chart.LegendNone).Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &pieView, 0.6, 0.35) },
	})
}
