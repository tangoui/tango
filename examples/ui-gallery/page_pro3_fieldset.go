package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

func init() {
	pro3Page("ProFormFieldSet", "ProFormFieldSet 组合字段", gallery.Form, 39, 960, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		vals := ui.ProValues{"phone": []any{"86", nil}}
		var pf *ui.ProFormW
		return func(c *ui.Context) {
				tbDemo(c, "多个控件组合为一个值", "值为 []any，每段一个元素；统一标签与校验（必填要求每段都有值，Validate 做跨段校验）。", func() {
					fields := []ui.ProField{
						{Key: "name", Label: "项目名称", Required: true},
						ui.ProFormFieldSet(c, ui.ProFieldSet{Key: "budget", Label: "预算区间", Separator: "~", Required: true,
							Parts: []ui.ProField{{ValueType: ui.ValueMoney, Placeholder: "最低"}, {ValueType: ui.ValueMoney, Placeholder: "最高"}},
							Validate: func(p []any, all ui.ProValues) string {
								a, _ := p[0].(float64)
								b, _ := p[1].(float64)
								if p[0] != nil && p[1] != nil && a > b {
									return "最低预算不能大于最高预算"
								}
								return ""
							}}),
						ui.ProFormFieldSet(c, ui.ProFieldSet{Key: "phone", Label: "联系电话", Required: true, Help: "区号 + 号码",
							Parts: []ui.ProField{{ValueType: ui.ValueSelect, Width: 100, ValueEnum: []ui.ProEnum{{Value: "86", Label: "+86"}, {Value: "852", Label: "+852"}, {Value: "1", Label: "+1"}}},
								{Placeholder: "手机号码"}}}),
						ui.ProFormFieldSet(c, ui.ProFieldSet{Key: "period", Label: "执行周期",
							Parts: []ui.ProField{{ValueType: ui.ValueDate}, {ValueType: ui.ValueSelect, Width: 120, ValueEnum: []ui.ProEnum{{Value: "d", Label: "每天"}, {Value: "w", Label: "每周"}}}}}),
					}
					pf = ui.ProForm(c, fields).Values(&vals).Horizontal().Show()
				})
			}, func(d *ui.Driver) {
				if pf != nil {
					pf.Submit()
					d.Settle()
				}
			}
	})
}
