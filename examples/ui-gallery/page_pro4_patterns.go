package main

// Complete Pro v6 page compositions. Reference: ant-design-pro@6.0.3,
// 42f2cb1, src/pages/{user/login,table-list,form/basic-form,profile/basic,
// result/success}, src/app.tsx. Data is local demo data, not a real service.
import (
	"fmt"
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	gallery.Register(gallery.Page{Name: "ProPatternLogin", Parent: "LoginForm", Title: "Pro v6 · 登录页", Group: gallery.Form, Width: 1100, Height: 760, Render: proPatternLogin})
	gallery.Register(gallery.Page{Name: "ProPatternList", Parent: "ProTable", Title: "Pro v6 · 查询列表页", Group: gallery.Display, Width: 1280, Height: 900, Render: proPatternList})
	gallery.Register(gallery.Page{Name: "ProPatternForm", Parent: "ProForm", Title: "Pro v6 · 基础表单页", Group: gallery.Form, Width: 1200, Height: 950, Render: proPatternForm})
	gallery.Register(gallery.Page{Name: "ProPatternDetail", Parent: "ProDescriptions", Title: "Pro v6 · 详情页", Group: gallery.Display, Width: 1280, Height: 1000, Render: proPatternDetail})
	gallery.Register(gallery.Page{Name: "ProPatternResult", Parent: "Result", Title: "Pro v6 · 结果页", Group: gallery.Feedback, Width: 1200, Height: 850, Render: proPatternResult})
	gallery.Register(gallery.Page{Name: "ProPatternSettings", Parent: "SettingDrawer", Title: "Pro v6 · 设置抽屉", Group: gallery.Layout, Width: 1280, Height: 850, Render: proPatternSettings})
}

func proPatternLogin(c *ui.Context) {
	ui.LoginFormPage(c).Logo(func() { p3Logo(c) }).Title("管理工作台").SubTitle("演示环境 · 不连接真实账号服务").
		OnForgot(func() { ui.Message(c, "info", "演示环境无需重置密码") }).
		OnSendCode(func(phone string, done func(error)) { ui.Message(c, "success", "演示验证码：1234"); done(nil) }).
		Actions(func() { p3OtherLogins(c) }).
		OnFinish(func(v ui.ProValues, done func(error)) { ui.Message(c, "success", "演示登录成功"); done(nil) }).Show()
}

func proPatternShell(c *ui.Context, title string, body func()) {
	selected := ui.State(c, ui.ID(0xb401), func() string { return "workspace" })
	collapsed := ui.State(c, ui.ID(0xb402), func() bool { return false })
	ui.ProLayout(c, ui.ProLayoutConfig{Title: "演示工作台", Selected: selected, Collapsed: collapsed, LightSider: true, UserName: "演示用户", NoPadding: true, AutoBreadcrumb: true,
		Menu: []ui.MenuItem{{Key: "workspace", Label: "工作台"}, {Key: "projects", Label: "项目管理"}, {Key: "settings", Label: "设置"}}, Footer: "Tango · 本地演示"}, func() {
		ui.PageContainer(c, title).Breadcrumb("首页", "工作台", title).Body(body)
	})
}

func proPatternList(c *ui.Context) {
	proPatternShell(c, "查询列表", func() {
		rows := ui.State(c, ui.ID(0xb410), func() []tbRule { return tbRules(18) })
		selected := ui.State(c, ui.ID(0xb411), func() []string { return nil })
		cols := []ui.ProColumn[tbRule]{
			{Key: "name", Title: "规则名称", Value: func(r tbRule) any { return r.name }},
			{Key: "owner", Title: "负责人", Value: func(r tbRule) any { return r.owner }},
			{Key: "status", Title: "状态", ValueType: ui.ValueStatus, ValueEnum: p3Status, Value: func(r tbRule) any { return r.status }},
			{Key: "calls", Title: "调用次数", ValueType: ui.ValueDigit, Sortable: true, HideInSearch: true, Value: func(r tbRule) any { return r.calls }},
			{Key: "updated", Title: "更新时间", ValueType: ui.ValueDateTime, HideInSearch: true, Value: func(r tbRule) any { return r.updated }},
		}
		table := ui.ProTable(c, cols).Title("规则管理").PageSize(5).RowKey(func(r tbRule) string { return r.key }).Selection(selected)
		table.ToolBar(func() {
				if ui.Button(c, "新建规则").Primary().Clicked() {
					*rows = append(*rows, tbRule{key: fmt.Sprint("demo-", len(*rows)), name: "新规则", owner: "演示用户", status: "running"})
					table.Reload()
				}
			}).
			Request(func(req ui.ProRequest, done func(ui.ProResult[tbRule])) {
				rs, total, err := tbRuleServer(*rows, req)
				done(ui.ProResult[tbRule]{Rows: rs, Total: total, Err: err})
			}).Show()
		if len(*selected) > 0 {
			ui.FooterToolbar(c).Extra(func() { ui.Text(c, fmt.Sprintf("已选择 %d 项", len(*selected))) }).Body(func() {
				if ui.Button(c, "清空选择").Clicked() {
					*selected = nil
				}
			})
		}
	})
}

func proPatternForm(c *ui.Context) {
	proPatternShell(c, "基础表单", func() {
		values := ui.State(c, ui.ID(0xb420), func() ui.ProValues { return ui.ProValues{"visibility": "team", "weight": 100.0} })
		ui.ProCard(c, "").Body(func() {
			ui.Paragraph(c, "表单页用于收集或验证信息。填写项目目标后提交；公开范围决定可选成员字段。")
			fields := []ui.ProField{
				{Key: "title", Label: "标题", Required: true, Placeholder: "给目标起一个名称"},
				{Key: "period", Label: "起止日期", ValueType: ui.ValueDateRange, Required: true},
				{Key: "goal", Label: "目标描述", ValueType: ui.ValueTextArea, Required: true},
				{Key: "standard", Label: "衡量标准", ValueType: ui.ValueTextArea, Required: true},
				{Key: "client", Label: "客户", Placeholder: "可选"},
				{Key: "reviewer", Label: "评审人", ValueType: ui.ValueSelect, ValueEnum: fpOwners},
				{Key: "weight", Label: "权重", ValueType: ui.ValueDigit, Min: 0, Max: 100, Help: "范围 0–100"},
				{Key: "visibility", Label: "目标公开", ValueType: ui.ValueRadioButton, ValueEnum: []ui.ProEnum{{Value: "all", Label: "全部公开"}, {Value: "team", Label: "部分公开"}, {Value: "private", Label: "不公开"}}},
				{Key: "members", Label: "公开成员", ValueType: ui.ValueSelect, ValueEnum: fpOwners, Dependencies: []string{"visibility"}, Hide: func(v ui.ProValues) bool { return v.String("visibility") != "team" }},
			}
			ui.ProForm(c, fields).Values(values).Columns(1).OnFinish(func(v ui.ProValues, done func(error)) {
				ui.Message(c, "success", "目标已保存（本地演示）")
				done(nil)
			}).Show()
		})
	})
}

func proPatternDetail(c *ui.Context) {
	proPatternShell(c, "基础详情", func() {
		data := ui.State(c, ui.ID(0xb430), func() ui.ProValues {
			return ui.ProValues{"number": "TK-20261115", "state": "待审核", "creator": "演示用户", "date": "2026-11-15", "name": "演示项目", "phone": "13800000000", "remark": "本地示例记录"}
		})
		ui.ProCard(c, "").Body(func() {
			ui.ProDescriptions(c, []ui.ProDescItem{{Key: "number", Label: "申请单号"}, {Key: "state", Label: "状态"}, {Key: "creator", Label: "创建人"}, {Key: "date", Label: "申请日期"}}).Title("退款申请").DataSource(data).Show()
			ui.Divider(c)
			ui.ProDescriptions(c, []ui.ProDescItem{{Key: "name", Label: "用户姓名"}, {Key: "phone", Label: "联系电话"}, {Key: "remark", Label: "备注"}}).Title("用户信息").DataSource(data).Show()
			ui.Divider(c)
			ui.Text(c, "退货商品").Strong()
			ui.ProTable(c, []ui.ProColumn[tbRule]{{Key: "name", Title: "商品名称", Value: func(r tbRule) any { return r.name }}, {Key: "amount", Title: "金额", ValueType: ui.ValueMoney, Value: func(r tbRule) any { return r.amount }}, {Key: "owner", Title: "操作员", Value: func(r tbRule) any { return r.owner }}}).NoSearch().Request(func(req ui.ProRequest, done func(ui.ProResult[tbRule])) {
				done(ui.ProResult[tbRule]{Rows: tbRules(3), Total: 3})
			}).Show()
		})
	})
}

func proPatternResult(c *ui.Context) {
	proPatternShell(c, "操作结果", func() {
		ui.ProCard(c, "").Body(func() {
			ui.Result(c, "success", "提交成功").SubTitle("提交结果页用于反馈任务处理结果。以下记录仅为本地演示，不会向服务器发送数据。").Extra(func() {
				if ui.Button(c, "返回列表").Primary().Clicked() {
					ui.Message(c, "info", "请从目录打开查询列表页")
				}
				if ui.Button(c, "查看项目").Clicked() {
					ui.Message(c, "info", "演示项目 TK-20261115")
				}
			}).Body(func() {
				ui.Descriptions(c, []ui.DescItem{{Label: "项目名称", Value: "演示审批项目"}, {Label: "项目编号", Value: "TK-20261115"}, {Label: "负责人", Value: "演示用户"}, {Label: "创建时间", Value: "2026-11-15"}}).Column(2)
				ui.Gap(c, 24)
				ui.Steps(c, []ui.StepItem{{Title: "创建项目"}, {Title: "部门初审"}, {Title: "财务复核"}, {Title: "完成"}}).Current(1).Show()
			})
		})
	})
}

func proPatternSettings(c *ui.Context) {
	settings := ui.State(c, ui.ID(0xb440), func() ui.ProSettings {
		s := ui.DefaultProSettings()
		if c.Theme.Dark {
			s.NavTheme = "realDark"
		}
		return s
	})
	selected := ui.State(c, ui.ID(0xb441), func() string { return "settings" })
	open := ui.State(c, ui.ID(0xb442), func() bool { return true })
	ui.ProSettingsProvider(c, settings, func() {
		cfg := ui.ProLayoutConfig{Title: "可配置工作台", Selected: selected, LightSider: true, UserName: "演示用户", Menu: []ui.MenuItem{{Key: "overview", Label: "概览"}, {Key: "settings", Label: "设置"}}, NoPadding: true}
		settings.Apply(&cfg)
		ui.ProLayout(c, cfg, func() {
			ui.PageContainer(c, "布局设置").Body(func() {
				ui.ProCard(c, "即时预览").Body(func() {
					ui.Paragraph(c, "右侧设置面板直接作用于本页 ProSettingsProvider 和 ProLayout。切换主题色、导航布局、内容宽度与固定栏位即可查看真实变化。")
					if ui.Button(c, "打开设置").Primary().Clicked() {
						*open = true
					}
					ui.Row(c, func() {
						ui.Tag(c, "当前布局："+settings.Layout)
						ui.Tag(c, "内容宽度："+settings.ContentWidth)
					}, layout.Gap(12))
				})
			})
		})
		ui.SettingDrawer(c, settings).Open(open).NoTrigger().Show()
	})
}
