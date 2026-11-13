package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func dataCol(c *ui.Context, w float32, fn func()) {
	ui.Column(c, fn, layout.W(w), layout.Gap(12))
}

func init() {
	dataPage(gallery.Page{Name: "Result", Title: "Result 结果", Order: 60, Width: 1100, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			for _, s := range []struct{ st, title, sub string }{
				{"success", "提交成功", "订单号：2017182818828182881，预计 1-5 分钟完成"},
				{"error", "提交失败", "请核对并修改以下信息后再重新提交"},
				{"info", "操作已受理", "后台正在处理，可稍后在列表中查看"},
				{"warning", "存在风险", "当前操作可能影响线上数据"},
			} {
				ui.Column(c, func() {
					ui.Result(c, s.st, s.title).SubTitle(s.sub).Extra(func() {
						ui.Button(c, "返回列表").Primary()
						ui.Button(c, "再来一次")
					}).Show()
				}, layout.Grow(1), layout.Basis(0))
			}
		}, layout.Gap(8))
		ui.Row(c, func() {
			for _, s := range []struct{ st, title, sub string }{
				{"403", "403", "抱歉，你无权访问该页面"},
				{"404", "404", "抱歉，你访问的页面不存在"},
				{"500", "500", "抱歉，服务器出错了"},
			} {
				ui.Column(c, func() {
					ui.Result(c, s.st, s.title).SubTitle(s.sub).Extra(func() { ui.Button(c, "返回首页").Primary() }).Show()
				}, layout.Grow(1), layout.Basis(0))
			}
		}, layout.Gap(8))
	}})

	dataPage(gallery.Page{Name: "Empty", Title: "Empty 空状态", Order: 8, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			ui.Empty(c)
			ui.Empty(c).Simple()
			ui.Empty(c).Description("暂无订单").Body(func() { ui.Button(c, "立即创建").Primary() })
			ui.Empty(c).ImageSize(60).Description("自定义尺寸")
			ui.Empty(c).Description("")
		}, layout.Gap(48), layout.AlignItems(layout.AlignStart))
	}})

	dataPage(gallery.Page{Name: "Progress", Title: "Progress 进度条", Order: 7, Render: func(c *ui.Context) {
		th := c.Theme
		ui.Row(c, func() {
			dataCol(c, 420, func() {
				ui.Progress(c, 30)
				ui.Progress(c, 50).Status("active")
				ui.Progress(c, 70).Status("exception")
				ui.Progress(c, 100)
				ui.Progress(c, 60).Success(30)
				ui.Progress(c, 80).Gradient(theme.Hex("#108ee9"), theme.Hex("#87d068"))
				ui.Progress(c, 45).Small()
			})
			dataCol(c, 420, func() {
				ui.Progress(c, 70).TextInside()
				ui.Progress(c, 50).Striped(true).StrokeWidth(16)
				ui.Progress(c, 60).Status("warning")
				ui.Progress(c, 40).Indeterminate()
				ui.Progress(c, 50).Steps(5)
				ui.Progress(c, 100).Steps(5).Color(th.Success.Base)
				ui.Progress(c, 60).Steps(10).Small().Format(func(p float64) string { return "6/10" })
			})
		}, layout.Gap(48), layout.Margin(geom.Insets{Bottom: 24}))
		ui.Row(c, func() {
			ui.Progress(c, 75).Circle(120)
			ui.Progress(c, 70).Circle(120).Status("exception")
			ui.Progress(c, 100).Circle(120)
			ui.Progress(c, 75).Dashboard(120)
			ui.Progress(c, 60).Circle(120).Steps(8)
			ui.Progress(c, 90).Circle(120).Gradient(theme.Hex("#108ee9"), theme.Hex("#87d068"))
		}, layout.Gap(24), layout.WrapItems())
	}})

	dataPage(gallery.Page{Name: "Skeleton", Title: "Skeleton 骨架屏", Order: 62, Render: func(c *ui.Context) {
		ui.Skeleton(c).Show()
		ui.Column(c, nil, layout.H(24))
		ui.Skeleton(c).Avatar(false).Rows(3).Active().Show()
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.SkeletonButton(c, ui.Middle, "default", true)
			ui.SkeletonButton(c, ui.Middle, "round", true)
			ui.SkeletonButton(c, ui.Middle, "circle", true)
			ui.SkeletonAvatar(c, 32, false, true)
			ui.SkeletonInput(c, ui.Middle, true)
		}, layout.Gap(12), layout.Middle())
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.SkeletonImage(c, 96, 96, false)
			ui.Column(c, func() { ui.Skeleton(c).Round().Rows(2).Show() }, layout.Grow(1))
		}, layout.Gap(16))
	}})

	deadline := time.Date(2026, 1, 3, 13, 30, 12, 0, time.UTC)
	dataPage(gallery.Page{Name: "Statistic", Title: "Statistic 统计数值 / Countdown 倒计时", Order: 6, Render: func(c *ui.Context) {
		th := c.Theme
		ui.Row(c, func() {
			ui.Statistic(c, "活跃用户", 112893, 0)
			ui.Statistic(c, "账户余额 (CNY)", 112893, 2).Prefix("¥")
			ui.Statistic(c, "增长率", 11.28, 2).Suffix("%").PrefixIcon(icon.ArrowUp).Color(th.Success.Base)
			ui.Statistic(c, "流失率", 9.3, 2).Suffix("%").PrefixIcon(icon.ArrowDown).Color(th.Error.Base)
		}, layout.Gap(64), layout.Margin(geom.Insets{Bottom: 32}))
		ui.Row(c, func() {
			ui.Countdown(c, "倒计时", deadline)
			ui.Countdown(c, "距离活动结束", deadline).Format("D 天 HH 时 mm 分 ss 秒")
			ui.Countdown(c, "毫秒级", deadline).Format("HH:mm:ss:SSS").Color(th.Error.Base)
			ui.Countdown(c, "已结束", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		}, layout.Gap(56), layout.Margin(geom.Insets{Bottom: 32}))
		ui.Row(c, func() {
			ui.Statistic(c, "活跃用户", 0, 0).CountUp(112893, 0)
			ui.Statistic(c, "加载中", 0, 0).Loading(true)
			ui.Statistic(c, "反馈", 1128, 0).PrefixIcon(icon.Message)
		}, layout.Gap(56))
	}})

	dataPage(gallery.Page{Name: "Badge", Title: "Badge 徽标 / Avatar 头像", Order: 3, Render: func(c *ui.Context) {
		box := func() { ui.Avatar(c, "").Square().Size(40) }
		dataRow(c, func() {
			ui.Badge(c).Count(128).Body(box)
			ui.Badge(c).Dot().Body(func() { ui.Icon(c, icon.Bell).Size(20) })
			ui.Badge(c).Count(5).Body(box)
			ui.Badge(c).Count(0).ShowZero().Body(box)
			ui.Badge(c).Count(1000).Max(999).Body(box)
			ui.Badge(c).Dot().Body(box)
			ui.Badge(c).Count(5).Small().Body(box)
			ui.Badge(c).Text("new").Body(func() { ui.Button(c, "评论") })
			ui.Badge(c).Count(12).Color("primary").Body(func() { ui.Button(c, "回复") })
			ui.Badge(c).Count(3).Color("success").Offset(-6, 6).Body(box)
			ui.Badge(c).Count(25)
			ui.Badge(c).Count(4).Color("#faad14")
		}, 28)
		dataRow(c, func() {
			ui.Badge(c).Status("success", "Success")
			ui.Badge(c).Status("error", "Error")
			ui.Badge(c).Status("default", "Default")
			ui.Badge(c).Status("processing", "Processing")
			ui.Badge(c).Status("warning", "Warning")
			for _, p := range []string{"pink", "cyan", "geekblue", "volcano"} {
				ui.Badge(c).Color(p).Status("custom", p)
			}
		}, 20)
		dataRow(c, func() {
			ui.Avatar(c, "").Size(40)
			ui.Avatar(c, "U")
			ui.Avatar(c, "张").Bg(theme.Hex("#f56a00"))
			ui.Avatar(c, "李").Bg(theme.Hex("#7265e6")).Size(24)
			ui.Avatar(c, "Go").Bg(theme.Hex("#00a2ae")).Square()
		}, 20)
		dataRow(c, func() {
			ui.AvatarGroup(c).Body(func() {
				ui.Avatar(c, "A").Bg(theme.Hex("#f56a00"))
				ui.Avatar(c, "K").Bg(theme.Hex("#87d068"))
				ui.Avatar(c, "").Bg(theme.Hex("#1677ff"))
			})
			ui.AvatarGroup(c).Max(2).Body(func() {
				ui.Avatar(c, "U").Bg(theme.Hex("#7265e6"))
				ui.Avatar(c, "张").Bg(theme.Hex("#ffbf00"))
				ui.Avatar(c, "李").Bg(theme.Hex("#00a2ae"))
				ui.Avatar(c, "王")
			})
			ui.AvatarGroup(c).Max(3).Size(40).Body(func() {
				for _, s := range []string{"A", "B", "C", "D", "E"} {
					ui.Avatar(c, s).Size(40).Bg(theme.Hex("#1677ff"))
				}
			})
		}, 40)
		ui.Row(c, func() {
			ui.BadgeRibbon(c, "推荐", "", "end", func() {
				ui.Card(c, "Pushes open the window").Small().Layout(layout.W(300)).Body(func() { ui.Text(c, "and raises the spyglass.") })
			})
			ui.BadgeRibbon(c, "热卖", "error", "start", func() {
				ui.Card(c, "左侧缎带").Small().Layout(layout.W(300)).Body(func() { ui.Text(c, "placement = start") })
			})
			ui.BadgeRibbon(c, "Volcano", "volcano", "end", func() {
				ui.Card(c, "自定义颜色").Small().Layout(layout.W(240)).Body(func() { ui.Text(c, "预设色 volcano") })
			})
		}, layout.Gap(32), layout.Margin(geom.Insets{Top: 12, Left: 8}))
	}})

	dataPage(gallery.Page{Name: "Card", Title: "Card 卡片", Order: 4, Width: 1100, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			ui.Card(c, "默认卡片").Extra(func() { ui.Link(c, "更多") }).Layout(layout.Grow(1)).Body(func() {
				ui.Text(c, "卡片内容")
				ui.Text(c, "卡片内容")
			})
			ui.Card(c, "小尺寸").Small().Layout(layout.Grow(1)).Body(func() { ui.Text(c, "Small 卡片内容") })
			ui.Card(c, "可悬浮").Hoverable().Layout(layout.Grow(1)).Body(func() { ui.Text(c, "鼠标移入出现阴影") })
			ui.Card(c, "无边框").Borderless().Shadow("always").Layout(layout.Grow(1)).Body(func() { ui.Text(c, "Borderless + Shadow") })
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart), layout.Margin(geom.Insets{Bottom: 24}))
		ui.Row(c, func() {
			ui.Card(c, "").Hoverable().Layout(layout.W(260)).Cover(func() { dataCover(c, 0) }).Body(func() {
				ui.CardMeta(c, nil, "Europe Street beat", "www.instagram.com")
			})
			ui.Card(c, "").Layout(layout.W(300)).Cover(func() { dataCover(c, 1) }).Actions(
				func() { ui.CardAction(c, icon.Setting, "setting") },
				func() { ui.CardAction(c, icon.Edit, "edit") },
				func() { ui.CardAction(c, icon.More, "more") },
			).Body(func() {
				ui.CardMeta(c, func() { ui.Avatar(c, "T").Bg(theme.Hex("#1677ff")) }, "Card title", "This is the description")
			})
			ui.Card(c, "加载中").Loading(true).Layout(layout.W(300)).Extra(func() { ui.Link(c, "更多") }).Actions(
				func() { ui.CardAction(c, icon.Like, "like") },
				func() { ui.CardAction(c, icon.More, "more2") },
			).Body(nil)
			ui.Column(c, func() {
				ui.Card(c, "带页脚").Shadow("always").Footer(func() { ui.Text(c, "Footer content").Tertiary() }).Body(func() { ui.Text(c, "Element 卡片支持 footer 插槽") })
				ui.Card(c, "外层卡片").Small().Body(func() {
					ui.Card(c, "内部卡片").Inner().Small().Extra(func() { ui.Link(c, "More") }).Body(func() { ui.Text(c, "Inner Card content") })
				})
			}, layout.Grow(1), layout.Gap(16))
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
	}})

	dataPage(gallery.Page{Name: "Descriptions", Title: "Descriptions 描述列表", Order: 5, Width: 1100, Render: func(c *ui.Context) {
		simple := []ui.DescItem{{Label: "用户名", Value: "曲丽丽"}, {Label: "手机号", Value: "1810000000"}, {Label: "居住地", Value: "杭州市西湖区"},
			{Label: "备注", Value: "无"}, {Label: "联系地址", Value: "浙江省杭州市西湖区万塘路18号", Span: 2}}
		ui.Descriptions(c, simple).Title("用户信息")
		ui.Column(c, nil, layout.H(16))
		items := []ui.DescItem{{Label: "产品", Value: "云数据库"}, {Label: "计费模式", Value: "预付费"}, {Label: "自动续费", Value: "是"},
			{Label: "下单时间", Value: "2018-04-24 18:00:00"}, {Label: "使用时间", Value: "2019-04-24 18:00:00", Span: 2},
			{Label: "状态", Render: func() { ui.Badge(c).Status("processing", "运行中") }, Span: 3},
			{Label: "配置信息", Value: "数据库版本：MySQL 8.0；规格：dds.mongo.mid；存储空间：10 GB；复制因子：3；地域：华东 1"}}
		ui.Descriptions(c, items).Title("带边框").Extra(func() { ui.Button(c, "编辑").Primary() }).Bordered()
		ui.Column(c, nil, layout.H(24))
		ui.Descriptions(c, items[:6]).Title("中尺寸 · 无边框").Size(ui.Middle)
		ui.Column(c, nil, layout.H(8))
		ui.Descriptions(c, items[:4]).Title("垂直布局").Bordered().Vertical().Size(ui.Small).Column(4)
	}})
}

// dataCover paints a procedural cover picture (no image files needed).
func dataCover(c *ui.Context, variant int) {
	ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 160} }, func(l *paint.List, r geom.Rect) {
		dataPaintScene(l, r, variant)
	})
}

func dataPaintScene(l *paint.List, r geom.Rect, variant int) {
	skies := [][2]string{{"#89c4f4", "#e0f0ff"}, {"#f6b48f", "#fde6c8"}, {"#7f8fd8", "#d6dcf7"}, {"#4fb7a4", "#d4f1ea"}}
	sk := skies[variant%len(skies)]
	l.Add(paint.LinearGradient{Rect: r, P0: geom.Pt(0, r.Y), P1: geom.Pt(0, r.Bottom()), From: theme.Hex(sk[0]), To: theme.Hex(sk[1])})
	l.Path(paint.NewPath().Circle(r.X+r.W*0.75, r.Y+r.H*0.3, r.H*0.12), theme.RGBA(255, 255, 255, .85))
	far := paint.NewPath().MoveTo(r.X, r.Bottom())
	for i := 0; i <= 8; i++ {
		x := r.X + r.W*float32(i)/8
		y := r.Y + r.H*(0.45+0.12*float32((i*37+variant*11)%5)/4)
		far.LineTo(x, y)
	}
	far.LineTo(r.Right(), r.Bottom()).Close()
	l.Path(far, theme.RGBA(60, 90, 140, .35))
	near := paint.NewPath().MoveTo(r.X, r.Bottom()).LineTo(r.X, r.Y+r.H*0.75).
		CubeTo(r.X+r.W*0.3, r.Y+r.H*0.55, r.X+r.W*0.6, r.Y+r.H*0.95, r.Right(), r.Y+r.H*0.7).
		LineTo(r.Right(), r.Bottom()).Close()
	l.Path(near, theme.RGBA(30, 70, 60, .55))
}
