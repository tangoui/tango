package main

import (
	"errors"
	"time"

	"github.com/tangoui/tango/ui"
)

var fpStatus = []ui.ProEnum{{Value: "draft", Label: "草稿"}, {Value: "online", Label: "已上线"}, {Value: "closed", Label: "已关闭"}}
var fpTags = []ui.ProEnum{{Value: "core", Label: "核心"}, {Value: "beta", Label: "实验"}, {Value: "gray", Label: "灰度"}}
var fpOwners = []ui.ProEnum{{Value: "qll", Label: "曲丽丽"}, {Value: "zs", Label: "张三"}, {Value: "ls", Label: "李四"}}

// fpRuleFields are the fields of the ProForm demos.
func fpRuleFields() []ui.ProField {
	return []ui.ProField{
		{Key: "name", Label: "规则名称", Required: true, Placeholder: "请输入规则名称", Help: "名称在项目内唯一",
			AsyncValidate: func(v any, done func(string)) {
				if v == "TradeCode 99" {
					done("规则名称已存在")
					return
				}
				done("")
			}},
		{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: fpOwners, Required: true},
		{Key: "amount", Label: "预算金额", ValueType: ui.ValueMoney, Min: 0, Max: 1e7},
		{Key: "deadline", Label: "截止日期", ValueType: ui.ValueDate},
		{Key: "status", Label: "状态", ValueType: ui.ValueRadioButton, ValueEnum: fpStatus, Initial: "draft"},
		{Key: "tags", Label: "标签", ValueType: ui.ValueCheckbox, ValueEnum: fpTags},
		{Key: "province", Label: "省份", ValueType: ui.ValueSelect, ValueEnum: fpProvinces},
		{Key: "city", Label: "城市", ValueType: ui.ValueSelect, Dependencies: []string{"province"},
			Options: func(v ui.ProValues) []ui.ProEnum { return fpCities[v.String("province")] },
			Disable: func(v ui.ProValues) bool { return v.String("province") == "" }},
		{Key: "budgetMax", Label: "预算上限", ValueType: ui.ValueDigit, Dependencies: []string{"amount"},
			Validate: func(v any, all ui.ProValues) string {
				if _, ok := v.(float64); ok && all.Float("budgetMax") < all.Float("amount") {
					return "预算上限不能小于预算金额"
				}
				return ""
			}},
		{Key: "notify", Label: "上线通知", ValueType: ui.ValueSwitch},
		{Key: "desc", Label: "规则描述", ValueType: ui.ValueTextArea, Span: 24, Hide: func(v ui.ProValues) bool { return v.String("status") == "closed" }},
	}
}

func init() {
	fpPage("ProForm", "ProForm 高级表单", 30, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var vals ui.ProValues
		var saved int
		return func(c *ui.Context) {
				tbDemo(c, "根据字段定义渲染", "值类型与 ProTable 共用一套（text/select/money/date/radioButton/checkbox/switch/textarea…）；两列栅格；Request 异步初始值；OnFinish 异步提交时按钮转圈；提交失败自动滚动到第一个错误字段。", func() {
					pf := ui.ProForm(c, fpRuleFields()).Columns(2).Values(&vals).
						Request(func(done func(ui.ProValues, error)) {
							done(ui.ProValues{"name": "TradeCode 99", "owner": "qll", "amount": 12800.0, "province": "zj", "city": "hz",
								"deadline": time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "tags": []string{"core"}, "budgetMax": 8000.0}, nil)
						}).
						OnFinish(func(v ui.ProValues, done func(error)) {
							saved++
							if saved > 1 {
								done(errors.New("服务器繁忙，请稍后再试"))
								return
							}
							done(nil)
						})
					pf.Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("提交")
			}
	})
}
