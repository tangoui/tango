package main

import (
	"fmt"
	"math"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/layout"
)

var (
	scatterView chart.View
	radarView   chart.View
	dualView    chart.View
	heatView    chart.View
	funnelView  chart.View
)

var heightWeight = func() []chart.Row {
	var out []chart.Row
	seed := uint32(11)
	rnd := func() float64 { seed = seed*1664525 + 1013904223; return float64(seed>>8) / float64(1<<24) }
	for i := 0; i < 120; i++ {
		g, h0, w0 := "女", 160.0, 54.0
		if i%2 == 0 {
			g, h0, w0 = "男", 174.0, 70.0
		}
		h := h0 + (rnd()-0.5)*22
		out = append(out, chart.Row{"gender": g, "height": math.Round(h*10) / 10, "weight": math.Round((w0+(h-h0)*0.7+(rnd()-0.5)*14)*10) / 10})
	}
	return out
}()

var countries = []chart.Row{
	{"country": "中国", "gdp": 17.9, "life": 78.2, "pop": 141},
	{"country": "美国", "gdp": 25.4, "life": 76.4, "pop": 33},
	{"country": "日本", "gdp": 4.2, "life": 84.5, "pop": 12.5},
	{"country": "德国", "gdp": 4.1, "life": 80.9, "pop": 8.4},
	{"country": "印度", "gdp": 3.4, "life": 70.8, "pop": 142},
	{"country": "英国", "gdp": 3.1, "life": 80.7, "pop": 6.7},
	{"country": "法国", "gdp": 2.8, "life": 82.3, "pop": 6.8},
	{"country": "巴西", "gdp": 1.9, "life": 73.4, "pop": 21.5},
	{"country": "印尼", "gdp": 1.3, "life": 71.8, "pop": 27.5},
}

var skills = func() []chart.Row {
	items := []string{"设计", "开发", "市场", "用户", "测试", "语言", "技术"}
	a := []float64{70, 60, 50, 40, 60, 70, 50}
	b := []float64{30, 70, 60, 40, 50, 40, 80}
	var out []chart.Row
	for i, it := range items {
		out = append(out, chart.Row{"item": it, "user": "张三", "score": a[i]}, chart.Row{"item": it, "user": "李四", "score": b[i]})
	}
	return out
}()

var salesRate = func() []chart.Row {
	var out []chart.Row
	for i, m := range months {
		v := 320 + 180*math.Sin(float64(i)/2) + float64(i*20)
		out = append(out, chart.Row{"month": m, "sales": math.Round(v), "orders": math.Round(v * 0.6), "rate": math.Round((0.12+0.05*math.Cos(float64(i)/1.7))*1000) / 1000})
	}
	return out
}()

var weekHeat = func() []chart.Row {
	days := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}
	var out []chart.Row
	for d, day := range days {
		for h := 0; h < 24; h += 2 {
			v := 20 + 60*math.Exp(-math.Pow(float64(h)-14, 2)/30) + 20*math.Exp(-math.Pow(float64(h)-20, 2)/8)
			if d >= 5 {
				v *= 1.3
			}
			v += float64((d*31+h*17)%15) - 7
			out = append(out, chart.Row{"hour": fmt.Sprintf("%02d:00", h), "day": day, "count": math.Round(v)})
		}
	}
	return out
}()

var funnelData = []chart.Row{
	{"stage": "浏览网站", "count": 50000}, {"stage": "放入购物车", "count": 35000}, {"stage": "生成订单", "count": 25000},
	{"stage": "支付订单", "count": 15000}, {"stage": "完成交易", "count": 8000},
}

func init() {
	chartPage(gallery.Page{Name: "ChartScatter", Title: "Scatter 散点图与气泡图", Order: 204,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "散点图（悬停）", func() {
						scatterView = chart.Scatter(c, heightWeight).X("height").Y("weight").Series("gender").Show()
					})
				},
				func() {
					chartCard(c, "气泡图", func() {
						chart.Bubble(c, countries).X("gdp").Y("life").Size("pop").Series("country").Legend(chart.LegendNone).Radius(6, 30).Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) {
			if r := scatterView.Rect(); r.W > 0 {
				// Hover the densest area; nearest point within 8px wins.
				for fx := float32(0.45); fx < 0.6; fx += 0.01 {
					hoverAt(d, &scatterView, fx, 0.55)
					if _, ok := scatterView.Tooltip(); ok {
						break
					}
				}
			}
		},
	})
	chartPage(gallery.Page{Name: "ChartRadar", Title: "Radar 雷达图", Order: 205,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "雷达图（悬停）", func() {
						radarView = chart.Radar(c, skills).X("item").Y("score").Series("user").Show()
					})
				},
				func() {
					chartCard(c, "圆形网格、仅描边", func() {
						chart.Radar(c, skills).X("item").Y("score").Series("user").Circle().NoArea().YMax(100).Show()
					})
				},
			)
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &radarView, 0.62, 0.3) },
	})
	chartPage(gallery.Page{Name: "ChartGauge", Title: "Gauge 仪表盘与水波图", Order: 206,
		Render: func(c *ui.Context) {
			chartGrid(c,
				func() {
					chartCard(c, "仪表盘", func() { chart.Gauge(c, 0.72).Title("完成率").Show() })
				},
				func() {
					chartCard(c, "分段仪表盘", func() {
						th := c.Theme
						chart.Gauge(c, 68).Range(0, 100).Title("健康度").Bands([]float64{40, 70, 100}, th.Error.Base, th.Warning.Base, th.Success.Base).Show()
					})
				},
				func() {
					chartCard(c, "水波图", func() { chart.Liquid(c, 0.3).Title("容量").Still().Show() })
				},
				func() {
					chartCard(c, "水波图（高水位）", func() { chart.Liquid(c, 0.78).Still().Show() })
				},
			)
		},
	})
	chartPage(gallery.Page{Name: "ChartFunnel", Title: "Funnel 漏斗图", Order: 207,
		Render: func(c *ui.Context) {
			chartCard(c, "转化漏斗（悬停）", func() {
				funnelView = chart.Funnel(c, funnelData).X("stage").Y("count").Format(chart.Thousands(0)).Height(320).Show()
			})
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &funnelView, 0.5, 0.62) },
	})
	chartPage(gallery.Page{Name: "ChartHeatmap", Title: "Heatmap 热力图", Order: 208,
		Render: func(c *ui.Context) {
			chartCard(c, "一周访问热力（悬停）", func() {
				heatView = chart.Heatmap(c, weekHeat).X("hour").Y("day").Value("count").Label().Height(340).Show()
			})
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &heatView, 0.6, 0.4) },
	})
	chartPage(gallery.Page{Name: "ChartDualAxes", Title: "DualAxes 双轴图", Order: 209,
		Render: func(c *ui.Context) {
			chartCard(c, "销售额与转化率（悬停）", func() {
				dualView = chart.DualAxes(c, salesRate).X("month").Column("sales", "orders").Line("rate").
					Alias("sales", "销售额").Alias("orders", "订单量").Alias("rate", "转化率").LineFormat(chart.Percent(0)).Smooth().Height(340).Show()
			})
		},
		Setup: func(d *ui.Driver) { hoverAt(d, &dualView, 0.4, 0.5) },
	})
	chartPage(gallery.Page{Name: "ChartTiny", Title: "Tiny 迷你图与状态", Order: 210,
		Render: func(c *ui.Context) {
			vals := []float64{264, 417, 438, 887, 309, 397, 550, 575, 563, 430, 525, 592, 492, 467, 513, 546, 983, 340, 539, 243, 226, 192}
			chartGrid(c,
				func() { chartCard(c, "TinyLine", func() { chart.TinyLine(c, vals).Smooth().Show() }) },
				func() { chartCard(c, "TinyArea", func() { chart.TinyArea(c, vals).Smooth().Show() }) },
				func() { chartCard(c, "TinyColumn", func() { chart.TinyColumn(c, vals).Show() }) },
				func() {
					chartCard(c, "TinyProgress", func() {
						ui.Column(c, func() {
							chart.TinyProgress(c, 0.78).Target(0.85).Show()
						}, layout.PadVH(19, 0))
					})
				},
				func() { chartCard(c, "空状态", func() { chart.Line(c, nil).X("x").Y("y").Height(200).Show() }) },
				func() {
					chartCard(c, "加载态", func() {
						chart.Column(c, salesByChannel).X("month").Y("value").Series("channel").Loading(true).Height(200).Show()
					})
				},
			)
		},
	})
}
