package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

func p3FilterFields() []ui.ProField {
	return []ui.ProField{
		{Key: "status", Label: "状态", ValueType: ui.ValueSelect, ValueEnum: p3Status},
		{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: fpOwners},
		{Key: "name", Label: "关键词"},
		{Key: "created", Label: "创建日期", ValueType: ui.ValueDateRange},
		{Key: "amount", Label: "金额", ValueType: ui.ValueMoney},
	}
}

func init() {
	pro3Page("LightFilter", "LightFilter 轻量筛选", gallery.Form, 36, 960, 520, func() (func(*ui.Context), func(*ui.Driver)) {
		vals := ui.ProValues{"status": "running", "created": [2]time.Time{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}}
		var vals2 ui.ProValues
		params := ""
		return func(c *ui.Context) {
				tbDemo(c, "紧凑胶囊", "每个字段显示为“标签: 值”，点击弹出浮层编辑（复用 ProForm 的编辑控件），确认后生效；有值时可一键清除。", func() {
					lf := ui.LightFilter(c, p3FilterFields()).Values(&vals).Show()
					if lf.Changed() {
						params = ""
						for k, v := range lf.Params() {
							params += k + "=" + v + " "
						}
					}
					if params != "" {
						ui.Text(c, "查询参数："+params).Tertiary()
					}
				})
				tbDemo(c, "带边框", "", func() {
					ui.LightFilter(c, p3FilterFields()[:3]).Values(&vals2).Bordered().Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("负责人")
			}
	})
}
