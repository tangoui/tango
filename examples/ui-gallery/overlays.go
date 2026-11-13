package main

import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

type overlayState struct {
	modal    bool
	name     string
	kind     string
	owner    string
	date     time.Time
	desc     string
	shown    bool
	page     int
	selected string
	open     bool
}

var ov = newOverlayState()

func newOverlayState() *overlayState {
	return &overlayState{modal: true, name: "TradeCode 100", page: 1, selected: "table-list"}
}

// overlays shows a list page with an open Modal containing an open Select
// dropdown, plus a global Message.
func overlays(c *ui.Context) {
	st := ov
	if !st.shown {
		st.shown = true
		ui.Message(c, "success", "规则已保存")
	}
	ui.ProLayout(c, ui.ProLayoutConfig{
		Title: "Tango Admin", Menu: proMenu, Selected: &st.selected,
		Breadcrumb: []string{"首页", "列表页", "查询表格"}, UserName: "曲丽丽",
	}, func() {
		ui.Card(c, "规则列表").Extra(func() {
			if ui.Button(c, "新建规则").Primary().Icon(icon.Plus).Clicked() {
				st.modal = true
			}
		}).Body(func() {
			ui.Table(c, []ui.TableColumn{
				{Title: "规则名称", Text: func(i int) string { return rules[i].name }},
				{Title: "描述", Text: func(i int) string { return rules[i].desc }},
				{Title: "状态", Width: 140, Render: func(i int) { ui.Badge(c).Status(rules[i].status, statusText[rules[i].status]) }},
			}, 6).Show()
		})
	})
	m := ui.Modal(c, &st.modal, "新建规则").Width(560)
	m.Body(func() {
		form := ui.Form(c).LabelWidth(96)
		form.Layout(layout.Margin(geom.Insets{Top: 16}))
		form.Body(func() {
			form.Item("规则名称").Required().Body(func() {
				ui.Input(c, &st.name).Placeholder("请输入规则名称").AllowClear()
			})
			form.Item("规则类型").Required().Body(func() {
				s := ui.Select(c, &st.kind, ui.Opts("定时调度", "事件触发", "手动执行", "接口回调", "数据同步")).Placeholder("请选择规则类型")
				if !ov.open {
					s.Open()
					ov.open = true
				}
			})
			form.Item("负责人").Body(func() {
				ui.Input(c, &st.owner).Placeholder("请输入负责人").Prefix(icon.User)
			})
			form.Item("描述").Error("请输入至少五个字符的规则描述！").Body(func() {
				ui.TextArea(c, &st.desc).Placeholder("请输入").Rows(2).Status("error")
			})
		})
	})
}

func overlaysSetup(d *ui.Driver) { d.Advance(400 * time.Millisecond) }
