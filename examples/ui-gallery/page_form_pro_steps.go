package main

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	fpPage("StepsForm", "StepsForm 分步表单", 33, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var vals ui.ProValues
		steps := []ui.StepsFormStep{
			{Title: "填写转账信息", Description: "付款与收款账户", Fields: []ui.ProField{
				{Key: "from", Label: "付款账户", ValueType: ui.ValueSelect, Required: true, ValueEnum: []ui.ProEnum{{Value: "a", Label: "ant-design@alipay.com"}, {Value: "b", Label: "tango@example.com"}}, Initial: "a"},
				{Key: "to", Label: "收款账户", Required: true, Placeholder: "请输入收款人邮箱", Rules: []ui.Rule{ui.Email("邮箱格式不正确")}},
				{Key: "payee", Label: "收款人姓名", Required: true},
				{Key: "amount", Label: "转账金额", ValueType: ui.ValueMoney, Required: true, Min: 0.01},
			}},
			{Title: "确认转账信息", Description: "输入支付密码", Fields: []ui.ProField{
				{Key: "pwd", Label: "支付密码", ValueType: ui.ValuePassword, Required: true, Rules: []ui.Rule{ui.MinLen(6, "至少 6 位")}},
				{Key: "remark", Label: "备注", ValueType: ui.ValueTextArea},
			}},
			{Title: "完成", Description: "转账结果", Fields: []ui.ProField{
				{Key: "notify", Label: "短信通知收款人", ValueType: ui.ValueSwitch, Initial: true},
			}},
		}
		return func(c *ui.Context) {
				tbDemo(c, "分步表单", "每一步的字段先校验再进入下一步；上一步保留已填的值；最后一步提交全部步骤的值。", func() {
					ui.StepsForm(c, steps).Values(&vals).Width(560).
						OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
				})
				_ = layout.Grow
			}, func(d *ui.Driver) {
				if r, ok := d.Find("收款账户"); ok {
					_ = r
				}
				// Fill the first step and go on.
				vals["to"], vals["payee"], vals["amount"] = "tango@example.com", "曲丽丽", 500.0
				d.Settle()
				d.ClickLabel("下一步")
				d.Settle()
				d.Key(input.KeyTab, 0)
			}
	})
	fpPage("ModalForm", "ModalForm / DrawerForm 弹窗与抽屉表单", 34, 1080, 640, func() (func(*ui.Context), func(*ui.Driver)) {
		open, openD := false, false
		var vals ui.ProValues
		fields := []ui.ProField{
			{Key: "name", Label: "规则名称", Required: true, Placeholder: "请输入名称"},
			{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: fpOwners, Required: true},
			{Key: "status", Label: "状态", ValueType: ui.ValueRadio, ValueEnum: fpStatus, Initial: "draft"},
			{Key: "desc", Label: "描述", ValueType: ui.ValueTextArea},
		}
		return func(c *ui.Context) {
				tbDemo(c, "弹窗表单", "ModalForm：确认按钮提交（校验通过、OnFinish 完成后自动关闭，提交中按钮转圈）；DrawerForm 同理。", func() {
					ui.Row(c, func() {
						if ui.Button(c, "新建规则").Primary().Clicked() {
							open = true
						}
						if ui.Button(c, "抽屉表单").Clicked() {
							openD = true
						}
					}, layout.Gap(8))
					ui.ModalForm(c, &open, "新建规则", fields).Values(&vals).OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
					ui.DrawerForm(c, &openD, "编辑规则", fields).Width(480).OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("新建规则")
				d.Settle()
				vals["name"] = "TradeCode 100"
				d.Settle()
				d.ClickLabel("确定")
			}
	})
	fpPage("DrawerForm", "DrawerForm 抽屉表单", 35, 1080, 640, func() (func(*ui.Context), func(*ui.Driver)) {
		open := false
		fields := fpRuleFields()[:6]
		return func(c *ui.Context) {
				tbDemo(c, "抽屉表单", "DrawerForm 从右侧滑出，底部 取消 / 确定。", func() {
					if ui.Button(c, "打开抽屉").Primary().Clicked() {
						open = true
					}
					ui.DrawerForm(c, &open, "编辑规则", fields).Width(520).OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("打开抽屉")
				d.Settle()
			}
	})
	fpPage("QueryFilter", "QueryFilter 查询表单", 36, 1200, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var q1, q2 ui.ProValues
		fields := []ui.ProField{
			{Key: "name", Label: "规则名称"},
			{Key: "status", Label: "状态", ValueType: ui.ValueSelect, ValueEnum: fpStatus},
			{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: fpOwners},
			{Key: "date", Label: "创建日期", ValueType: ui.ValueDate},
			{Key: "range", Label: "更新区间", ValueType: ui.ValueDateRange},
			{Key: "calls", Label: "调用次数", ValueType: ui.ValueDigit},
		}
		var last string
		return func(c *ui.Context) {
				tbDemo(c, "收起状态", "按宽度每行 1–4 个字段，按钮固定在最后一格；超过一行时折叠，「展开」显示全部。ProTable 顶部查询表单即复用此组件。", func() {
					q := ui.QueryFilter(c, fields).Values(&q1).Show()
					if q.Searched() {
						last = ""
						for k, v := range q.Params() {
							last += k + "=" + v + " "
						}
					}
				})
				tbDemo(c, "展开状态", "", func() {
					ui.QueryFilter(c, fields).Values(&q2).DefaultExpanded().Show()
				})
			}, func(d *ui.Driver) {
				q1 = ui.ProValues{"name": "TradeCode", "status": "online"}
				d.Settle()
			}
	})
}
