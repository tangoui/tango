package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
)

var lineView chart.View

func init() {
	chartPage(gallery.Page{Name: "ChartLine", Title: "Line 折线图", Order: 201,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "基础折线（悬停）", func() {
						lineView = chart.Line(c, cityTemps).X("month").Y("temp").Series("city").Format(chart.Unit("°C", 1)).Show()
					})
				},
				func() {
					chartCard(c, "平滑曲线", func() {
						chart.Line(c, cityTemps).X("month").Y("temp").Series("city").Smooth().Show()
					})
				},
				func() {
					chartCard(c, "阶梯折线", func() {
						chart.Line(c, cityTemps).X("month").Y("temp").Series("city").Step().Show()
					})
				},
				func() {
					chartCard(c, "面积图（时间轴，跨年）", func() {
						chart.Area(c, dailyTraffic).X("date").Y("pv").Smooth().Show()
					})
				},
				func() {
					chartCard(c, "堆叠面积", func() {
						chart.Area(c, salesByChannel).X("month").Y("value").Series("channel").Stack().Smooth().Show()
					})
				},
				func() {
					chartCard(c, "10 000 点（LTTB 降采样）", func() {
						chart.Line(c, bigSeries).X("t").Y("v").NoZero().Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &lineView, 0.55, 0.5) },
	})
}
