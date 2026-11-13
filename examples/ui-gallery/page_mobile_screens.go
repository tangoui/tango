package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Mobile screens: each returns a page body (built inside a phone frame)
// with its own state. overlays opens the popups shown in phone shots; the
// inline gallery pages keep them closed.

type mobileScreen struct {
	name, title string
	make        func(overlays bool) func(c *ui.Context)
}

var mobileScreens = []mobileScreen{
	{"nav", "导航：NavBar / SearchBar / Tabs / NoticeBar / Grid / TabBar", mobileNavScreen},
	{"list", "列表：List / Cell / SwipeCell / Badge / Tag", mobileListScreen},
	{"form", "表单：Form / Field / Stepper / Button", mobileFormScreen},
	{"display", "展示：Swiper / Skeleton / Empty / Result / Toast", mobileDisplayScreen},
	{"pull", "PullToRefresh / InfiniteList", mobilePullScreen},
	{"picker", "Picker 滚轮选择器（日期时间）", mobilePickerScreen},
	{"cascade", "PickerView / CascadePickerView", mobileCascadeScreen},
	{"popup", "Popup / Dialog", mobileDialogScreen},
	{"actionsheet", "ActionSheet 动作面板", mobileActionSheetScreen},
	{"indexbar", "IndexBar / FloatingBubble / SafeArea", mobileIndexScreen},
	{"app", "移动端示例应用", mobileAppScreen},
}

func mobileTabs() []mobile.TabItem {
	return []mobile.TabItem{
		{Key: "home", Title: "首页", Icon: icon.Home},
		{Key: "msg", Title: "消息", Icon: icon.Message, Badge: "5"},
		{Key: "todo", Title: "待办", Icon: icon.Calendar, Badge: "dot"},
		{Key: "me", Title: "我的", Icon: icon.User},
	}
}

// pageCol is the full-height page column below the status bar.
func pageCol(c *ui.Context, fn func()) {
	ui.Column(c, fn, layout.Grow(1))
}

func mobileNavScreen(bool) func(c *ui.Context) {
	q := ""
	tab, active := "home", 0
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "发现").Back("").RightIcon(icon.More).RightLabel("更多")
			mobile.SearchBar(c, &q, "搜索商品、店铺", "")
			mobile.Tabs(c, []string{"推荐", "关注", "热门", "附近"}, &active).Header()
			mobile.NoticeBar(c, "系统将于今晚 23:00 至次日 01:00 进行升级维护，期间部分功能不可用。").Closeable()
			ui.Div(c, layout.H(12))
			mobile.Grid(c, 4, []mobile.GridItem{
				{Text: "扫一扫", Icon: icon.Fullscreen}, {Text: "付款", Icon: icon.Shop}, {Text: "出行", Icon: icon.Global, Badge: "dot"}, {Text: "卡包", Icon: icon.Inbox},
				{Text: "消息", Icon: icon.Message, Badge: "12"}, {Text: "日程", Icon: icon.Calendar}, {Text: "文件", Icon: icon.Folder}, {Text: "设置", Icon: icon.Setting},
			})
			ui.Div(c, layout.H(12))
			mobile.Tabs(c, []string{"全部", "待付款", "待发货", "待收货", "待评价", "退款/售后"}, &active).Header()
			ui.Spacer(c)
			mobile.TabBar(c, mobileTabs(), &tab)
		})
	}
}

func mobileListScreen(bool) func(c *ui.Context) {
	wifi, bt := true, false
	first := true
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "设置").Back("")
			mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
				mobile.List(c, "网络").Body(func() {
					mobile.Cell(c, "无线局域网").Icon(icon.Global).Switch(&wifi)
					mobile.Cell(c, "蓝牙").Icon(icon.Link).Switch(&bt)
					mobile.Cell(c, "蜂窝网络").Icon(icon.Bell).Extra("已开启").Arrow()
				})
				mobile.List(c, "账号").Body(func() {
					mobile.Cell(c, "个人资料").Desc("头像、昵称、简介").Arrow()
					mobile.Cell(c, "通知").Extra("3 条未读").Arrow()
					sw := mobile.SwipeCell(c).
						Right(mobile.SwipeAction{Text: "置顶", Color: "primary"}, mobile.SwipeAction{Text: "删除", Color: "danger"})
					if first {
						sw.Open(1)
						first = false
					}
					sw.Body(func() { mobile.Cell(c, "向左滑动这一行").Desc("显示操作按钮") })
				})
				mobile.List(c, "卡片模式").Card().Body(func() {
					mobile.Cell(c, "账单").Extra("¥ 128.00").Arrow()
					mobile.Cell(c, "优惠券").Extra("2 张可用").Arrow()
				})
				ui.Row(c, func() {
					box := func() { ui.Div(c, layout.Size(44, 44)).Radius(8).Bg(mobile.ThemeOf(c).TextLight) }
					mobile.Badge(c, "5", box)
					mobile.Badge(c, "dot", box)
					mobile.Badge(c, "99+", box)
				}, layout.Gap(24), layout.PadVH(20, 16), layout.Middle())
				ui.Row(c, func() {
					mobile.Tag(c, "默认")
					mobile.Tag(c, "主要").Color("primary")
					mobile.Tag(c, "成功").Color("success").Light()
					mobile.Tag(c, "警告").Color("warning").Outline()
					mobile.Tag(c, "危险").Color("danger").Round()
				}, layout.Gap(10), layout.PadVH(0, 16), layout.Middle())
			})
			mobile.SafeArea(c, mobile.Bottom)
		})
	}
}

func mobileFormScreen(bool) func(c *ui.Context) {
	name, phone, note := "张三", "", ""
	qty := 2.0
	agree := true
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "填写资料").Back("")
			mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
				mobile.Form(c).Header("标签在左").Body(func() {
					mobile.FormItem(c, "姓名").Required().Body(func() { mobile.Field(c, &name).Placeholder("请输入姓名") })
					mobile.FormItem(c, "手机号").Required().Error("请输入正确的手机号").Body(func() { mobile.Field(c, &phone).Placeholder("请输入手机号") })
					mobile.FormItem(c, "生日").Arrow().Body(func() { ui.Text(c, "1990-06-01").Color(mobile.ThemeOf(c).Text) })
					mobile.FormItem(c, "数量").Body(func() { mobile.Stepper(c, &qty).Min(1).Max(10) })
				})
				mobile.Form(c).Vertical().Header("标签在上").Body(func() {
					mobile.FormItem(c, "备注").Help("最多 100 字").Body(func() { mobile.Field(c, &note).Placeholder("选填") })
				})
				mobile.List(c, "").Body(func() { mobile.Cell(c, "同意用户协议").Switch(&agree) })
				ui.Column(c, func() {
					mobile.Button(c, "提交").Primary().Block().Size(mobile.Large)
					ui.Row(c, func() {
						mobile.Button(c, "默认")
						mobile.Button(c, "主要").Primary()
						mobile.Button(c, "成功").Color("success")
						mobile.Button(c, "危险").Danger().Fill("outline")
						mobile.Button(c, "加载").Primary().Loading(true)
					}, layout.Gap(8), layout.WrapItems())
					ui.Row(c, func() {
						mobile.Button(c, "迷你").Size(mobile.Mini).Primary()
						mobile.Button(c, "小号").Size(mobile.Small).Primary().Fill("outline")
						mobile.Button(c, "圆角").Primary().Round()
						mobile.Button(c, "禁用").Disabled(true)
					}, layout.Gap(8), layout.Middle())
				}, layout.Pad(16), layout.Gap(12))
			})
			mobile.SafeArea(c, mobile.Bottom)
		})
	}
}

func mobileDisplayScreen(overlays bool) func(c *ui.Context) {
	cur := 0
	toast := overlays
	return func(c *ui.Context) {
		th := mobile.ThemeOf(c)
		pageCol(c, func() {
			mobile.NavBar(c, "展示").Back("")
			mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
				ui.Column(c, func() {
					colors := []string{"#4b7bec", "#20bf6b", "#fa8231", "#a55eea"}
					mobile.Swiper(c, &cur).Height(140).Radius(8).Loop().Slides(4, func(i int) {
						ui.Div(c, layout.Grow(1), layout.Center()).Bg(hexColor(colors[i])).Body(func() {
							ui.Text(c, fmt.Sprint("轮播 ", i+1)).Size(20).Color(th.OnSolid)
						})
					})
				}, layout.Pad(12))
				ui.Div(c).Bg(th.Box).Body(func() { mobile.Skeleton(c).Title().Paragraph(3).Avatar().Animated(false) })
				ui.Div(c, layout.H(12))
				mobile.Result(c, "success", "操作成功").Description("内容详情可折行，建议不超过两行。")
				ui.Div(c, layout.H(12))
				ui.Div(c).Bg(th.Box).Body(func() { mobile.Empty(c) })
			})
			mobile.SafeArea(c, mobile.Bottom)
		})
		mobile.Toast(c, &toast, "保存成功").Icon("success").Duration(0).Show()
	}
}

func hexColor(s string) paintColor { return themeHex(s) }

func mobilePullScreen(bool) func(c *ui.Context) {
	refreshing := true
	loading := true
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "下拉刷新").Back("")
			mobile.PullToRefresh(c, &refreshing).Layout(layout.H(300)).Bg(mobile.ThemeOf(c).PageBg).Body(func() {
				mobile.List(c, "").Body(func() {
					for i := 0; i < 4; i++ {
						ui.Key(c, i, func() { mobile.Cell(c, fmt.Sprint("刷新列表第 ", i+1, " 项")) })
					}
				})
			})
			ui.Div(c, layout.H(12))
			mobile.InfiniteList(c, &loading).Layout(layout.Grow(1)).Body(func() {
				mobile.List(c, "上拉加载").Body(func() {
					for i := 0; i < 3; i++ {
						ui.Key(c, i, func() {
							mobile.Cell(c, fmt.Sprint("加载列表第 ", i+1, " 项")).Desc("滚动到底部自动加载更多")
						})
					}
				})
			})
			mobile.SafeArea(c, mobile.Bottom)
		})
	}
}

var pickerDate = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)

func mobilePickerScreen(overlays bool) func(c *ui.Context) {
	open := overlays
	t := pickerDate
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "选择器").Back("")
			mobile.List(c, "").Body(func() {
				if mobile.Cell(c, "预约时间").Extra(t.Format("2006-01-02 15:04")).Arrow().Clicked() {
					open = true
				}
			})
		})
		mobile.Picker(c, &open, "预约时间").Date(&t, mobile.PrecisionMinute).Show()
	}
}

var cascadeOpts = []ui.CascaderOption{
	{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{
		{Label: "杭州", Value: "hz", Children: []ui.CascaderOption{{Label: "西湖区", Value: "xh"}, {Label: "余杭区", Value: "yh"}, {Label: "滨江区", Value: "bj"}}},
		{Label: "宁波", Value: "nb", Children: []ui.CascaderOption{{Label: "海曙区", Value: "hs"}, {Label: "鄞州区", Value: "yz"}}},
	}},
	{Label: "江苏", Value: "js", Children: []ui.CascaderOption{
		{Label: "南京", Value: "nj", Children: []ui.CascaderOption{{Label: "玄武区", Value: "xw"}, {Label: "鼓楼区", Value: "gl"}}},
		{Label: "苏州", Value: "sz", Children: []ui.CascaderOption{{Label: "姑苏区", Value: "gs"}}},
	}},
	{Label: "广东", Value: "gd", Children: []ui.CascaderOption{
		{Label: "广州", Value: "gz", Children: []ui.CascaderOption{{Label: "天河区", Value: "th"}}},
		{Label: "深圳", Value: "szx", Children: []ui.CascaderOption{{Label: "南山区", Value: "ns"}, {Label: "福田区", Value: "ft"}}},
	}},
}

func mobileCascadeScreen(bool) func(c *ui.Context) {
	area := []string{"zj", "hz", "bj"}
	week := []string{"周三", "下午"}
	d := pickerDate
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "滚轮选择").Back("")
			mobile.List(c, "级联（省 / 市 / 区）").Body(func() { mobile.CascadePickerView(c, cascadeOpts, &area).Show() })
			mobile.List(c, "多列").Body(func() {
				mobile.PickerView(c, [][]ui.Option{ui.Opts("周一", "周二", "周三", "周四", "周五"), ui.Opts("上午", "下午", "晚上")}, &week).Visible(5).Show()
			})
			mobile.List(c, "日期").Body(func() { mobile.DatePickerView(c, &d, mobile.PrecisionDay).Visible(5).Show() })
		})
	}
}

func mobileDialogScreen(overlays bool) func(c *ui.Context) {
	open := overlays
	pop := false
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "弹出层").Back("")
			mobile.List(c, "").Body(func() {
				if mobile.Cell(c, "底部弹出").Arrow().Clicked() {
					pop = true
				}
				if mobile.Cell(c, "确认对话框").Arrow().Clicked() {
					open = true
				}
			})
		})
		mobile.Popup(c, &pop).A11yLabel("弹出层").Body(func() {
			ui.Column(c, func() { ui.Text(c, "弹出层内容") }, layout.H(240), layout.Pad(16))
		})
		mobile.Dialog(c, &open, "删除确认").Content("删除后数据将无法恢复，确定要删除这条记录吗？").Confirm("删除", "").Show()
	}
}

func mobileActionSheetScreen(overlays bool) func(c *ui.Context) {
	open := overlays
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "动作面板").Back("")
			mobile.List(c, "").Body(func() {
				if mobile.Cell(c, "打开动作面板").Arrow().Clicked() {
					open = true
				}
			})
		})
		mobile.ActionSheet(c, &open, []mobile.Action{
			{Text: "复制链接"}, {Text: "修改", Description: "修改后需要重新审核"}, {Text: "禁用操作", Disabled: true}, {Text: "删除", Danger: true, Bold: true},
		}).Extra("请选择你要进行的操作").Cancel("").Show()
	}
}

func mobileIndexScreen(bool) func(c *ui.Context) {
	groups := []string{"A", "B", "C", "D", "F", "G", "H", "J", "K", "L", "N", "S", "W", "X", "Y", "Z"}
	names := map[string][]string{
		"A": {"安庆", "鞍山", "安阳"}, "B": {"北京", "保定", "包头", "蚌埠"}, "C": {"成都", "重庆", "长沙", "常州"},
		"D": {"大连", "东莞"}, "F": {"福州", "佛山"}, "G": {"广州", "贵阳"}, "H": {"杭州", "合肥", "哈尔滨"},
		"J": {"济南", "嘉兴"}, "K": {"昆明"}, "L": {"兰州", "洛阳"}, "N": {"南京", "宁波", "南昌"},
		"S": {"上海", "深圳", "苏州"}, "W": {"武汉", "无锡"}, "X": {"西安", "厦门"}, "Y": {"烟台"}, "Z": {"郑州", "珠海"},
	}
	return func(c *ui.Context) {
		pageCol(c, func() {
			mobile.NavBar(c, "选择城市").Back("")
			mobile.IndexBar(c, groups).Layout(layout.Grow(1)).Body(func(i int) {
				for _, n := range names[groups[i]] {
					ui.Key(c, n, func() { mobile.Cell(c, n).Clickable() })
				}
			})
			mobile.SafeArea(c, mobile.Bottom)
		})
		mobile.FloatingBubble(c, icon.Message, mobile.AxisBoth, true).A11yLabel("客服")
	}
}

func mobileAppScreen(overlays bool) func(c *ui.Context) {
	refreshing := false
	open := overlays
	tab := "msg"
	first := true
	type chat struct{ name, msg, at string }
	chats := []chat{
		{"产品讨论群", "小王：新版原型已更新，大家看一下", "09:41"},
		{"李雷", "好的，明天上午十点见", "09:12"},
		{"韩梅梅", "[图片]", "昨天"},
		{"项目周报", "本周完成 12 项，延期 1 项", "昨天"},
		{"设计团队", "图标规范 v2 已发布", "周二"},
		{"王老师", "作业已批改", "周一"},
		{"快递助手", "您的包裹已到达驿站", "周一"},
		{"运维告警", "磁盘使用率超过 80%", "11/08"},
	}
	return func(c *ui.Context) {
		th := mobile.ThemeOf(c)
		pageCol(c, func() {
			mobile.NavBar(c, "消息").RightIcon(icon.Plus).RightLabel("新建")
			mobile.PullToRefresh(c, &refreshing).Layout(layout.Grow(1)).Bg(th.PageBg).Body(func() {
				mobile.List(c, "").Body(func() {
					for i, ch := range chats {
						i, ch := i, ch
						ui.Key(c, ch.name, func() {
							sw := mobile.SwipeCell(c).Right(
								mobile.SwipeAction{Text: "标为已读", Color: "primary"},
								mobile.SwipeAction{Text: "删除", Color: "danger"})
							if i == 1 && first {
								sw.Open(1)
								first = false
							}
							sw.Body(func() { chatRow(c, th, ch.name, ch.msg, ch.at, i) })
							if sw.ActionKey() == "删除" {
								open = true
							}
						})
					}
				})
			})
			mobile.TabBar(c, mobileTabs(), &tab)
		})
		mobile.ActionSheet(c, &open, []mobile.Action{{Text: "删除聊天记录", Danger: true}, {Text: "仅删除会话"}}).
			Extra("删除后，将清空该聊天的消息记录").Cancel("").Show()
	}
}

func chatRow(c *ui.Context, th *mobile.Theme, name, msg, at string, i int) {
	palette := []string{"#4b7bec", "#20bf6b", "#fa8231", "#a55eea", "#eb3b5a", "#0fb9b1", "#3867d6", "#f7b731"}
	col := themeHex(palette[i%len(palette)])
	unread := map[int]string{0: "3", 3: "dot", 6: "1"}[i]
	ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 72} }, func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Radii{}, th.Box)
		x := r.X + th.PadH
		av := geom.Rect{X: x, Y: r.Y + 14, W: 44, H: 44}
		l.Fill(av, geom.Rad(8), col)
		first := []rune(name)[:1]
		lay := text.Shape(string(first), th.Font(18, text.Medium), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(av.X+(av.W-lay.Width)/2, av.Y+(av.H-lay.Height)/2), lay.Glyphs, th.OnSolid)
		if unread != "" {
			mobile.DrawBadge(l, th, unread, av.Right(), av.Y)
		}
		tx := av.Right() + 12
		if i > 0 {
			l.Fill(geom.Rect{X: tx, Y: r.Y, W: r.Right() - tx, H: 0.5}, geom.Radii{}, th.Border)
		}
		nf := th.Font(th.FontBody, text.Regular)
		mf := th.Font(th.FontSmall+1, text.Regular)
		tf := th.Font(12, text.Regular)
		at := text.Shape(at, tf, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.Right()-th.PadH-at.Width, r.Y+16), at.Glyphs, th.TextLight)
		nl := text.Shape(name, nf, text.Options{MaxWidth: r.Right() - tx - 80, MaxLines: 1, Ellipsis: true})
		l.Glyphs(geom.Pt(tx, r.Y+13), nl.Glyphs, th.Text)
		ml := text.Shape(msg, mf, text.Options{MaxWidth: r.Right() - tx - th.PadH, MaxLines: 1, Ellipsis: true})
		l.Glyphs(geom.Pt(tx, r.Y+13+nf.LineHeight+2), ml.Glyphs, th.TextWeak)
	})
}
