package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
)

var columnView chart.View

var rankData = func() []chart.Row {
	names := []string{"工专路 0 号店", "工专路 1 号店", "工专路 2 号店", "工专路 3 号店", "工专路 4 号店", "工专路 5 号店", "工专路 6 号店"}
	vals := []float64{323234, 293200, 251500, 198300, 156880, 120300, 98200}
	var out []chart.Row
	for i, n := range names {
		out = append(out, chart.Row{"shop": n, "sales": vals[i]})
	}
	return out
}()

func init() {
	chartPage(gallery.Page{Name: "ChartColumn", Title: "Column / Bar 柱状图与条形图", Order: 202,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "基础柱状图（悬停）", func() {
						columnView = chart.Column(c, filterSeries(salesByChannel, "搜索引擎")).X("month").Y("value").Label().Show()
					})
				},
				func() {
					chartCard(c, "分组柱状图", func() {
						chart.Column(c, salesByChannel).X("month").Y("value").Series("channel").Show()
					})
				},
				func() {
					chartCard(c, "堆叠柱状图", func() {
						chart.Column(c, salesByChannel).X("month").Y("value").Series("channel").Stack().Show()
					})
				},
				func() {
					chartCard(c, "百分比堆叠", func() {
						chart.Column(c, salesByChannel).X("month").Y("value").Series("channel").Percent().Label().Show()
					})
				},
				func() {
					chartCard(c, "条形图（排行）", func() {
						chart.Bar(c, rankData).X("shop").Y("sales").Label().Format(chart.Thousands(0)).Show()
					})
				},
				func() {
					chartCard(c, "堆叠条形图", func() {
						chart.Bar(c, salesByChannel).X("month").Y("value").Series("channel").Stack().Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &columnView, 0.62, 0.6) },
	})
}

func filterSeries(rows []chart.Row, name string) []chart.Row {
	var out []chart.Row
	for _, r := range rows {
		if r["channel"] == name {
			out = append(out, r)
		}
	}
	return out
}
