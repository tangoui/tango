package main

import (
	"fmt"
	"math"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var (
	dashTrendView chart.View
	dashRange     = "本月"
)

var dashVisits = []float64{7, 5, 4, 2, 4, 7, 5, 6, 5, 9, 6, 3, 1, 5, 3, 6, 5, 8, 6, 7, 9, 11, 10, 8}

var dashTrend = func() []chart.Row {
	var out []chart.Row
	for i := 0; i < 30; i++ {
		day := fmt.Sprintf("11-%02d", i+1)
		a := 4200 + 1400*math.Sin(float64(i)/4.5) + float64((i*7919)%600)
		b := 2600 + 900*math.Sin(float64(i)/4.5+1.2) + float64((i*104729)%400)
		out = append(out, chart.Row{"day": day, "type": "本期", "value": math.Round(a)}, chart.Row{"day": day, "type": "上期", "value": math.Round(b)})
	}
	return out
}()

var dashRank = func() []chart.Row {
	names := []string{"杭州西湖店", "上海静安店", "北京朝阳店", "深圳南山店", "成都高新店", "广州天河店"}
	vals := []float64{323234, 293200, 251500, 198300, 156880, 120300}
	var out []chart.Row
	for i, n := range names {
		out = append(out, chart.Row{"shop": n, "sales": vals[i]})
	}
	return out
}()

func init() {
	base := gallery.Page{Name: "ChartDashboard", Title: "分析仪表盘", Group: gallery.Display, Order: 220, Width: 1440, Render: analysisDashboard}
	for _, v := range []struct {
		suffix, title string
		th            func() *theme.Theme
	}{
		{"", "", func() *theme.Theme { return theme.AntDesign() }},
		{"-Element", "（Element Plus / ECharts）", func() *theme.Theme { return theme.ElementPlus() }},
		{"-Dark", "（暗色）", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	} {
		p := base
		p.Name, p.Title = base.Name+v.suffix, base.Title+v.title
		mk := v.th
		p.Setup = func(d *ui.Driver) {
			d.SetTheme(mk())
			d.Settle()
			hoverAt(d, &dashTrendView, 0.58, 0.45)
		}
		p.Variant = v.suffix != ""
		p.Interact = func(d *ui.Driver) { hoverAt(d, &dashTrendView, 0.58, 0.45) }
		gallery.Register(p)
	}
}

// analysisDashboard is an Ant Design Pro style analysis page built from
// chart components.
func analysisDashboard(c *ui.Context) {
	th := c.Theme
	ui.Div(c, layout.Pad(16), layout.Gap(16)).Bg(th.BgLayout).Body(func() {
		// Statistic cards with sparklines.
		ui.Row(c, func() {
			chartStatCard(c, "总销售额", "¥ 126,560", func() {
				ui.Row(c, func() {
					trend(c, "周同比", "12%", true)
					trend(c, "日同比", "11%", false)
				}, layout.Gap(16), layout.Middle(), layout.H(46), layout.AlignItems(layout.AlignEnd))
			}, "日销售额", "¥ 12,423")
			chartStatCard(c, "访问量", "8,846", func() {
				chart.TinyArea(c, dashVisits).Smooth().Names(dayNames(len(dashVisits))...).Show()
			}, "日访问量", "1,234")
			chartStatCard(c, "支付笔数", "6,560", func() {
				chart.TinyColumn(c, dashVisits).Names(dayNames(len(dashVisits))...).Show()
			}, "转化率", "60%")
			chartStatCard(c, "运营活动效果", "78%", func() {
				ui.Column(c, func() { chart.TinyProgress(c, 0.78).Target(0.85).Show() }, layout.H(46), layout.JustifyContent(layout.JustifyCenter))
			}, "周同比 12%", "日同比 11%")
		}, layout.Gap(16))
		// Main trend.
		ui.Card(c, "销售趋势").Extra(func() {
			ui.Segmented(c, &dashRange, ui.Opts("今日", "本周", "本月", "全年")).Size(ui.Small)
		}).Body(func() {
			dashTrendView = chart.Area(c, dashTrend).X("day").Y("value").Series("type").Smooth().Format(chart.Thousands(0)).Height(300).Show()
		})
		ui.Row(c, func() {
			ui.Column(c, func() {
				ui.Card(c, "销售渠道占比").Body(func() {
					chart.Donut(c, channelShare).Angle("value").Color("type").NoLabel().Center("销售额", "¥ 3,267").Height(300).Show()
				})
			}, layout.Grow(1), layout.Basis(0))
			ui.Column(c, func() {
				ui.Card(c, "门店销售额排名").Body(func() {
					chart.Bar(c, dashRank).X("shop").Y("sales").Format(chart.Compact(c.Locale().Code)).Label().MaxWidth(18).Radius(2).Height(300).Show()
				})
			}, layout.Grow(1), layout.Basis(0))
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
		ui.Card(c, "访问时段热力").Body(func() {
			chart.Heatmap(c, weekHeat).X("hour").Y("day").Value("count").Height(260).Show()
		})
	})
}

func dayNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("11-%02d", i+1)
	}
	return out
}

func chartStatCard(c *ui.Context, title, value string, body func(), footLabel, footValue string) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Card(c, "").Padding(geom.Insets{Top: 20, Left: 24, Right: 24, Bottom: 8}).Body(func() {
			ui.Text(c, title).Tertiary()
			ui.Text(c, value).Size(30).LineHeight(38).Color(th.TextHeading)
			ui.Column(c, body, layout.H(46), layout.Margin(geom.Insets{Top: 4, Bottom: 12}))
			ui.Div(c, layout.H(1)).Bg(th.Split)
			ui.Row(c, func() {
				ui.Text(c, footLabel).Secondary()
				ui.Text(c, footValue)
			}, layout.Gap(8), layout.PadVH(9, 0))
		})
	}, layout.Grow(1), layout.Basis(0))
}

func chartTrend(c *ui.Context, label, v string, up bool) {
	th := c.Theme
	col, arrow := th.Success.Base, "▼"
	if up {
		col, arrow = th.Error.Base, "▲"
	}
	ui.Row(c, func() {
		ui.Text(c, label).Secondary()
		ui.Text(c, v)
		ui.Text(c, arrow).Size(10).Color(col)
	}, layout.Gap(6), layout.Middle())
}
