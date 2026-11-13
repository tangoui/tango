package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var pro2CardDemo struct {
	folded bool
	tab    string
}

func pro2CardReset() {
	pro2CardDemo.folded = false
	pro2CardDemo.tab = "t1"
}

func pro2CardPage(c *ui.Context) {
	s := &pro2CardDemo
	gallery.Demo(c, "基础卡片", "标题、副标题、提示、操作区；bordered / headerBordered / ghost。", func() {
		pro2Canvas(c, func() {
			ui.Row(c, func() {
				ui.ProCard(c, "默认尺寸").SubTitle("副标题").Tooltip("这是提示").
					Extra(func() { ui.Button(c, "操作").Link() }).Layout(layout.Grow(1), layout.Basis(0)).
					Body(func() { ui.Text(c, "内容") })
				ui.ProCard(c, "带边框").Bordered().HeaderBordered().Layout(layout.Grow(1), layout.Basis(0)).
					Extra(func() { ui.Text(c, "2019-11-17").Tertiary() }).
					Body(func() { ui.Text(c, "内容") })
				ui.ProCard(c, "加载中").Loading(true).Layout(layout.Grow(1), layout.Basis(0)).Body(nil)
			}, layout.Gap(16))
		})
	})
	gallery.Demo(c, "分栏与栅格", "split 竖向分栏，colSpan 按 24 栅格分配；嵌套横向分栏。", func() {
		pro2Canvas(c, func() {
			ui.ProCard(c, "左右分栏").Bordered().HeaderBordered().Split(ui.SplitVertical).Body(func() {
				ui.ProCard(c, "左侧详情").ColSpan(8).Body(func() { ui.Text(c, "左侧内容") })
				ui.ProCard(c, "").Split(ui.SplitHorizontal).Body(func() {
					ui.ProCard(c, "").Split(ui.SplitVertical).Body(func() {
						ui.ProCard(c, "昨日全部流量").Body(func() { ui.Text(c, "123").Size(24) })
						ui.ProCard(c, "本月累计流量").Body(func() { ui.Text(c, "234").Size(24) })
						ui.ProCard(c, "今年累计流量").Body(func() { ui.Text(c, "345").Size(24) })
					})
					ui.ProCard(c, "流量趋势").Body(func() { ui.Text(c, "右下内容").Tertiary() })
				})
			})
			ui.ProCard(c, "").Ghost().Gutter(8).Body(func() {
				for i, sp := range []int{4, 4, 4, 12} {
					ui.ProCard(c, "").ColSpan(sp).Bordered().Layout(layout.H(56)).Centered().Body(func() {
						ui.Text(c, map[bool]string{true: "colSpan-12", false: "colSpan-4"}[i == 3])
					})
				}
			})
		})
	})
	gallery.Demo(c, "可折叠与标签页", "点击标题栏折叠；tabs 卡片切换内容。", func() {
		pro2Canvas(c, func() {
			ui.Row(c, func() {
				ui.ProCard(c, "可折叠").Collapsed(&s.folded).Bordered().
					Extra(func() { ui.Button(c, "提交").Small() }).
					Layout(layout.Grow(1), layout.Basis(0)).
					Body(func() { ui.Text(c, "折叠后只保留标题栏。") })
				ui.ProCard(c, "").Tabs(&s.tab, []ui.ProCardTab{
					{Key: "t1", Label: "产品一", Body: func() { ui.Text(c, "内容一") }},
					{Key: "t2", Label: "产品二", Body: func() { ui.Text(c, "内容二") }},
					{Key: "t3", Label: "产品三", Body: func() { ui.Text(c, "内容三") }},
				}).Layout(layout.Grow(1), layout.Basis(0)).Body(nil)
			}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
		})
	})
}

var pro2Sales = []float64{264, 417, 438, 887, 309, 397, 550, 575, 563, 430, 525, 592, 492, 467, 513, 546, 983, 340, 539, 243, 226, 192}

func pro2StatisticCardPage(c *ui.Context) {
	th := c.Theme
	gallery.Demo(c, "基础与趋势", "数值、前后缀、提示、状态点；description 中的同比环比带上升（红）/下降（绿）三角。", func() {
		pro2Canvas(c, func() {
			ui.Row(c, func() {
				ui.StatisticCard(c, "总销售额").Tooltip("指标说明").Value(126560, 0).Prefix("¥").
					Desc("周同比", "12%", "up").Desc("日同比", "11%", "down").
					Footer(func() {
						ui.Row(c, func() { ui.Text(c, "日销售额"); ui.Text(c, "¥12,423") }, layout.Gap(8))
					}).Layout(layout.Grow(1), layout.Basis(0)).Show()
				ui.StatisticCard(c, "访问量").Value(8846, 0).Status("processing").
					Chart(func() { chart.TinyArea(c, pro2Sales).Smooth().Show() }).
					Footer(func() {
						ui.Row(c, func() { ui.Text(c, "日访问量"); ui.Text(c, "1,234") }, layout.Gap(8))
					}).Layout(layout.Grow(1), layout.Basis(0)).Show()
				ui.StatisticCard(c, "支付笔数").Value(6560, 0).Status("success").
					Chart(func() { chart.TinyColumn(c, pro2Sales[:14]).Show() }).
					Footer(func() {
						ui.Row(c, func() { ui.Text(c, "转化率"); ui.Text(c, "60%") }, layout.Gap(8))
					}).Layout(layout.Grow(1), layout.Basis(0)).Show()
				ui.StatisticCard(c, "运营活动效果").Value(78, 0).Suffix("%").Trend("up").
					Chart(func() { chart.TinyProgress(c, 0.78).Target(0.8).Show() }).
					Footer(func() {
						ui.Row(c, func() {
							ui.Text(c, "周同比 12%")
							ui.Text(c, "日同比 11%")
						}, layout.Gap(16))
					}).Layout(layout.Grow(1), layout.Basis(0)).Show()
			}, layout.Gap(16))
		})
	})
	gallery.Demo(c, "指标组与左右图表", "StatisticCardGroup 竖线分隔；图表放在右侧、图标在左侧。", func() {
		pro2Canvas(c, func() {
			ui.StatisticCardGroup(c).Body(func() {
				ui.StatisticCard(c, "今日UV").Value(79, 0).Desc("较昨日", "8.04%", "up").Show()
				ui.StatisticCard(c, "冻结金额").Value(112893, 2).Prefix("¥").Desc("较昨日", "2.1%", "down").Show()
				ui.StatisticCard(c, "信息完整度").Value(92.1, 1).Suffix("%").Show()
				ui.StatisticCard(c, "订单数").Value(3701928, 0).Status("warning").Show()
			})
			ui.Row(c, func() {
				ui.StatisticCard(c, "支付金额").Icon(icon.Shop).Value(2176, 0).Prefix("¥").
					Layout(layout.Grow(1), layout.Basis(0)).Show()
				ui.StatisticCard(c, "访客数").CardTitle("核心指标").Value(475, 0).ValueColor(th.Primary.Base).
					ChartPlace(ui.ChartRight).
					Chart(func() { ui.Column(c, func() { chart.TinyLine(c, pro2Sales).Smooth().Show() }, layout.W(180)) }).
					Layout(layout.Grow(1), layout.Basis(0)).Show()
			}, layout.Gap(16))
		})
	})
	_ = geom.Pt
}

func init() {
	pro2CardReset()
	pro2Page(gallery.Page{Name: "ProCard", Title: "ProCard 高级卡片", Group: gallery.Display, Order: 60, Width: 1100,
		Render: pro2CardPage}, pro2CardReset)
	pro2Page(gallery.Page{Name: "StatisticCard", Title: "StatisticCard 指标卡", Group: gallery.Display, Order: 61, Width: 1200,
		Render: pro2StatisticCardPage}, nil)
}
