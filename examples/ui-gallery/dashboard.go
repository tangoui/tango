package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var proMenu = []ui.MenuItem{
	{Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []ui.MenuItem{
		{Key: "analysis", Label: "分析页"}, {Key: "monitor", Label: "监控页"}, {Key: "workplace", Label: "工作台"},
	}},
	{Key: "form", Label: "表单页", Icon: icon.Form},
	{Key: "list", Label: "列表页", Icon: icon.Table, Children: []ui.MenuItem{
		{Key: "table-list", Label: "查询表格"}, {Key: "basic-list", Label: "标准列表"},
	}},
	{Key: "profile", Label: "详情页", Icon: icon.Profile},
	{Key: "result", Label: "结果页", Icon: icon.CheckCircle},
	{Key: "exception", Label: "异常页", Icon: icon.Warning},
	{Key: "account", Label: "个人中心", Icon: icon.User},
	{Key: "settings", Label: "系统设置", Icon: icon.Setting},
}

type rule struct {
	name, desc string
	calls      int
	status     string
	time       string
}

var rules = func() []rule {
	descs := []string{"这是一段描述", "Ant Design 规则说明", "高可用调度策略", "按天统计的服务调用", "灰度发布规则"}
	sts := []string{"default", "processing", "success", "error"}
	var out []rule
	for i := 0; i < 46; i++ {
		out = append(out, rule{
			name:   fmt.Sprintf("TradeCode %d", 99-i),
			desc:   descs[i%len(descs)],
			calls:  (i*7919 + 113) % 1000 * 10,
			status: sts[(i*3+1)%len(sts)],
			time:   fmt.Sprintf("2026-11-%02d %02d:%02d:00", 12-i%10, 9+i%9, (i*13)%60),
		})
	}
	return out
}()

var statusText = map[string]string{"default": "关闭", "processing": "运行中", "success": "已上线", "error": "异常"}

type dashState struct {
	selected  string
	collapsed bool
	page      int
	pageSize  int
	sel       []int
	sort      ui.SortState
	name      string
	status    string
}

var dash = newDash()

func newDash() *dashState {
	return &dashState{selected: "analysis", page: 1, pageSize: 10, sel: []int{1, 3}}
}

func dashboard(c *ui.Context) {
	th := c.Theme
	st := dash
	ui.ProLayout(c, ui.ProLayoutConfig{
		Title:      "Tango Admin",
		Menu:       proMenu,
		Selected:   &st.selected,
		Collapsed:  &st.collapsed,
		Breadcrumb: []string{"首页", "仪表盘", "分析页"},
		UserName:   "曲丽丽",
		Actions: func() {
			for _, ic := range []*icon.Icon{icon.Search, icon.Question} {
				ui.Div(c, layout.Size(32, 32), layout.Center()).Body(func() { ui.Icon(c, ic).Size(16).Color(th.TextSecondary) })
			}
			ui.Div(c, layout.Size(32, 32), layout.Center()).Body(func() {
				ui.Badge(c).Count(11).Body(func() { ui.Icon(c, icon.Bell).Size(16).Color(th.TextSecondary) })
			})
		},
		Footer: "Tango UI ©2026 · Go 自绘组件库",
	}, func() {
		ui.Row(c, func() {
			statCard(c, "总销售额", "126,560", "¥", func() {
				trend(c, "周同比", "12%", true)
				trend(c, "日同比", "11%", false)
			}, "日销售额", "￥12,423")
			statCard(c, "访问量", "8,846", "", func() {
				ui.Progress(c, 64).HideInfo().StrokeWidth(6).Layout(layout.Grow(1))
			}, "日访问量", "1,234")
			statCard(c, "支付笔数", "6,560", "", func() {
				ui.Progress(c, 42).HideInfo().StrokeWidth(6).Layout(layout.Grow(1))
			}, "转化率", "60%")
			statCard(c, "运营活动效果", "78%", "", func() {
				ui.Progress(c, 78).StrokeWidth(6).Layout(layout.Grow(1))
			}, "活动参与", "2,860 人")
		}, layout.Gap(24), layout.Margin(geom.Insets{Bottom: 24}))
		ui.Card(c, "查询表格").Extra(func() {
			ui.Row(c, func() {
				ui.Button(c, "导出").Icon(icon.Download)
				ui.Button(c, "新建").Primary().Icon(icon.Plus)
			}, layout.Gap(8))
		}).Body(func() {
			ui.Row(c, func() {
				ui.Row(c, func() {
					ui.Text(c, "规则名称：").NoWrap()
					ui.Input(c, &st.name).Placeholder("请输入").Width(200)
				}, layout.Middle())
				ui.Row(c, func() {
					ui.Text(c, "使用状态：").NoWrap()
					ui.Select(c, &st.status, []ui.Option{{Label: "关闭", Value: "default"}, {Label: "运行中", Value: "processing"}, {Label: "已上线", Value: "success"}, {Label: "异常", Value: "error"}}).Placeholder("请选择").Width(200)
				}, layout.Middle())
				ui.Spacer(c)
				ui.Button(c, "重置")
				ui.Button(c, "查询").Primary()
			}, layout.Gap(16), layout.Middle(), layout.Margin(geom.Insets{Bottom: 24}))
			start := (st.page - 1) * st.pageSize
			n := min(st.pageSize, len(rules)-start)
			row := func(i int) rule { return rules[start+i] }
			ui.Table(c, []ui.TableColumn{
				{Key: "name", Title: "规则名称", Width: 160, Render: func(i int) { ui.Link(c, row(i).name) }},
				{Key: "desc", Title: "描述", Text: func(i int) string { return row(i).desc }},
				{Key: "calls", Title: "服务调用次数", Width: 150, Align: ui.AlignRight, Sortable: true, Text: func(i int) string { return ui.FormatNumber(float64(row(i).calls), 0) + " 万" }},
				{Key: "status", Title: "状态", Width: 120, Render: func(i int) { ui.Badge(c).Status(row(i).status, statusText[row(i).status]) }},
				{Key: "time", Title: "上次调度时间", Width: 190, Sortable: true, Text: func(i int) string { return row(i).time }},
				{Key: "op", Title: "操作", Width: 150, Render: func(i int) {
					ui.Link(c, "配置")
					ui.VDivider(c)
					ui.Link(c, "订阅警报")
				}},
			}, n).Selection(&st.sel).Sort(&st.sort).Size(ui.Middle).Show()
			ui.Row(c, func() {
				ui.Pagination(c, &st.page, len(rules), st.pageSize).ShowTotal().SizeChanger(&st.pageSize).Show()
			}, layout.End(), layout.Margin(geom.Insets{Top: 16}))
		})
	})
	_ = theme.Ant
}

func statCard(c *ui.Context, title, value, prefix string, body func(), footLabel, footValue string) {
	th := c.Theme
	ui.Card(c, "").Layout(layout.Grow(1), layout.MinW(0)).Padding(geom.Insets{Top: 20, Right: 24, Bottom: 8, Left: 24}).Body(func() {
		ui.Row(c, func() {
			ui.Text(c, title).Tertiary().Layout(layout.Grow(1))
			ui.Tooltip(c, "指标说明", func() { ui.Icon(c, icon.Info).Color(th.TextTertiary) })
		}, layout.Middle())
		ui.Statistic(c, "", 0, 0).Value(value).Prefix(prefix).ValueSize(30)
		ui.Row(c, body, layout.H(38), layout.Gap(16), layout.Middle())
		ui.Divider(c).Style(layout.Margin(geom.Insets{Top: 8, Bottom: 8}))
		ui.Row(c, func() {
			ui.Text(c, footLabel).NoWrap()
			ui.Text(c, footValue).NoWrap()
		}, layout.Gap(8), layout.Margin(geom.Insets{Bottom: 4}))
	})
}

func trend(c *ui.Context, label, v string, up bool) {
	th := c.Theme
	ui.Row(c, func() {
		ui.Text(c, label).NoWrap()
		ui.Text(c, v).NoWrap()
		if up {
			ui.Icon(c, icon.CaretUp).Size(12).Color(th.Error.Base)
		} else {
			ui.Icon(c, icon.CaretDown).Size(12).Color(th.Success.Base)
		}
	}, layout.Gap(6), layout.Middle())
}
