package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
)

var mirrorLineView chart.View

// ChartMirror shows Mirror charts next to a default one. Render it with
// -dir rtl to see them in a right-to-left scope: the default plot stays
// left to right while legend, tooltip and labels follow the scope; with
// Mirror the plot itself runs right to left. The page sets no direction
// itself, so the gallery's direction passes and -mirror-check drive it.
func init() {
	chartPage(gallery.Page{Name: "ChartMirror", Title: "RTL 与镜像图表", Order: 211,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "默认（RTL 下绘图区保持 LTR）", func() {
						chart.Line(c, cityTemps).X("month").Y("temp").Series("city").Smooth().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 折线（悬停）", func() {
						mirrorLineView = chart.Line(c, cityTemps).X("month").Y("temp").Series("city").Format(chart.Unit("°C", 1)).Smooth().Mirror().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 柱状图", func() {
						chart.Column(c, salesByChannel).X("month").Y("value").Series("channel").Stack().Mirror().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 条形图", func() {
						chart.Bar(c, rankData).X("shop").Y("sales").Label().Format(chart.Thousands(0)).Radius(4).Mirror().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 环形图（逆时针）", func() {
						chart.Donut(c, channelShare).Angle("value").Color("type").Mirror().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 双轴图", func() {
						chart.DualAxes(c, salesRate).X("month").Column("sales").Line("rate").LineFormat(chart.Percent(0)).Mirror().Show()
					})
				},
				func() {
					chartCard(c, "Mirror 仪表盘", func() { chart.Gauge(c, 0.72).Title("完成率").Mirror().Show() })
				},
				func() {
					chartCard(c, "Mirror 雷达图", func() {
						chart.Radar(c, skills).X("item").Y("score").Series("user").Mirror().Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &mirrorLineView, 0.3, 0.5) },
	})
}
