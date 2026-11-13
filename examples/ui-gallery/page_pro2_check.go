package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var pro2CheckDemo struct {
	single bool
	plain  bool
	dis    bool
	value  string
	values []string
	sizes  [3]bool
}

func pro2CheckReset() {
	s := &pro2CheckDemo
	s.single, s.plain, s.dis = true, false, true
	s.value = "react"
	s.values = []string{"a", "c"}
	s.sizes = [3]bool{false, true, false}
}

var pro2CheckOpts = []ui.CheckCardOption{
	{Value: "react", Title: "React", Description: "用于构建用户界面的 JavaScript 库", Avatar: "R"},
	{Value: "vue", Title: "Vue", Description: "渐进式 JavaScript 框架", Avatar: "V"},
	{Value: "angular", Title: "Angular", Description: "现代 Web 开发平台", Avatar: "A", Disabled: true},
}

var pro2CheckMulti = []ui.CheckCardOption{
	{Value: "a", Title: "仪表盘", Description: "分析页、监控页、工作台", Icon: icon.Dashboard},
	{Value: "b", Title: "表单页", Description: "基础、分步、高级表单", Icon: icon.Form},
	{Value: "c", Title: "列表页", Description: "查询表格、标准列表、卡片列表", Icon: icon.Table},
}

func pro2CheckCardPage(c *ui.Context) {
	s := &pro2CheckDemo
	gallery.Demo(c, "基础用法", "可勾选卡片：头像、标题、描述、右上角操作；选中后边框变主色并显示角标；禁用。", func() {
		ui.Row(c, func() {
			ui.CheckCard(c, "示例标题", &s.single).Avatar("蚂").Description("选择一个由流程编排提供的典型用户案例，可以从中学习到流程编排很多设计理念").
				Extra(func() { ui.Icon(c, icon.More).Size(16).Color(c.Theme.TextTertiary) }).Show()
			ui.CheckCard(c, "未选中", &s.plain).Description("点击卡片切换选中状态").Show()
			ui.CheckCard(c, "禁用并选中", &s.dis).Description("不可操作").Disabled(true).Show()
		}, layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart))
	})
	gallery.Demo(c, "单选组", "CheckCardGroup Value：再次点击已选卡片可取消。当前值："+s.value, func() {
		ui.CheckCardGroup(c, pro2CheckOpts).Value(&s.value).Show()
	})
	gallery.Demo(c, "多选组", "CheckCardGroup Values，图标头像。", func() {
		ui.CheckCardGroup(c, pro2CheckMulti).Values(&s.values).Show()
	})
	gallery.Demo(c, "尺寸", "small 212px / default 320px / large 440px。", func() {
		ui.Row(c, func() {
			ui.CheckCard(c, "Small", &s.sizes[0]).Description("小尺寸").Small().Show()
			ui.CheckCard(c, "Default", &s.sizes[1]).Description("默认尺寸").Show()
			ui.CheckCard(c, "Large", &s.sizes[2]).Description("大尺寸").Large().Show()
		}, layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart))
	})
}

func init() {
	pro2CheckReset()
	pro2Page(gallery.Page{Name: "CheckCard", Title: "CheckCard 多选卡片", Group: gallery.Form, Order: 60, Width: 1100,
		Render: pro2CheckCardPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("未选中"); ok {
				d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
				d.Settle()
			}
		}}, pro2CheckReset)
}
