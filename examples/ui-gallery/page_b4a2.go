package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	gallery.Register(gallery.Page{Name: "StatisticCardAlign", Parent: "StatisticCard", Title: "StatisticCard v3 布局与子部件", Group: gallery.Display, Width: 1100, Render: func(c *ui.Context) {
		gallery.Demo(c, "布局与趋势", "vertical：完整数值；horizontal：两端对齐；inline：紧凑内联。红涨绿跌。", func() {
			ui.StatisticCardGroup(c).Body(func() {
				ui.StatisticCard(c, "Sales").Value(126560, 0).Prefix("¥").Trend("up").Desc("Week", "12%", "up").Desc("Day", "11%", "down").Show()
				ui.StatisticCard(c, "Visits").Value(8846, 0).StatisticLayout(ui.StatisticHorizontal).Show()
				ui.StatisticCard(c, "Conversion").Value(78.6, 1).Suffix("%").StatisticLayout(ui.StatisticInline).Show()
			})
		})
		gallery.Demo(c, "StatisticCard.Statistic", "独立 ProStatistic，默认 inline；description 与 footer 可以组合它。", func() {
			ui.StatisticCard(c, "Balance").Value(112893, 2).Prefix("¥").Status("success").Tooltip("Current account balance").Description(func() {
				ui.ProStatistic(c, "Yesterday").Value(1024, 0).Trend("up").Show()
				ui.ProStatistic(c, "Last week").Value(896, 0).Trend("down").Show()
			}).Footer(func() { ui.ProStatistic(c, "Total orders").Value(8846, 0).Show() }).Layout(layout.W(430)).Show()
		})
		gallery.Demo(c, "Divider 与 Operation", "显式分隔与算式操作符；组的默认 body padding 为 0。", func() {
			ui.StatisticCardGroup(c).Split("").Body(func() {
				ui.StatisticCard(c, "Total").Value(100, 0).Ghost().Show()
				ui.StatisticCardOperation(c, "=")
				ui.StatisticCard(c, "Online").Value(64, 0).Ghost().Show()
				ui.StatisticCardOperation(c, "+")
				ui.StatisticCard(c, "Offline").Value(36, 0).Ghost().Show()
				ui.StatisticCardDivider(c, "vertical")
				ui.StatisticCard(c, "Rate").Value(64, 0).Suffix("%").Ghost().Show()
			})
		})
		gallery.Demo(c, "图表插槽与 loading", "左右内容间距 16，顶部对齐；插槽可以是任意内容。", func() {
			ui.Row(c, func() {
				ui.StatisticCard(c, "Detail").Value(42, 0).ChartPlace(ui.ChartRight).Chart(func() { ui.Text(c, "Chart / content slot").Color(c.Theme.TextSecondary) }).Layout(layout.W(480)).Show()
				ui.StatisticCard(c, "Loading").Loading(true).Layout(layout.W(480)).Show()
			}, layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart))
		})
		ui.StatisticCardDivider(c, "horizontal").Layout(layout.Margin(geom.Insets{Top: 8, Bottom: 8}))
	}})
}
