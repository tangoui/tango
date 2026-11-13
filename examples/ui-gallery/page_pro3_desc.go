package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

var p3Status = []ui.ProEnum{{Value: "running", Label: "运行中", Status: "processing"}, {Value: "online", Label: "已上线", Status: "success"}, {Value: "error", Label: "异常", Status: "error"}}

func p3DescItems() []ui.ProDescItem {
	return []ui.ProDescItem{
		{Key: "name", Label: "规则名称", Field: &ui.ProField{Required: true}},
		{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: fpOwners},
		{Key: "status", Label: "状态", ValueType: ui.ValueStatus, ValueEnum: p3Status},
		{Key: "amount", Label: "预算金额", ValueType: ui.ValueMoney},
		{Key: "rate", Label: "转化率", ValueType: ui.ValuePercent},
		{Key: "calls", Label: "调用次数", ValueType: ui.ValueDigit},
		{Key: "created", Label: "创建时间", ValueType: ui.ValueDateTime, ReadOnly: true},
		{Key: "deadline", Label: "截止日期", ValueType: ui.ValueDate},
		{Key: "progress", Label: "进度", ValueType: ui.ValueProgress, ReadOnly: true},
		{Key: "memo", Label: "备注", ValueType: ui.ValueTextArea, Span: 3},
	}
}

func p3DescData() map[string]any {
	return map[string]any{"name": "TradeCode 99", "owner": "qll", "status": "running", "amount": 12800.5, "rate": 23.6,
		"calls": 1288460.0, "created": time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC),
		"deadline": time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "progress": 64.0, "memo": "按值类型渲染；点击铅笔图标可行内编辑单项。"}
}

func init() {
	pro3Page("ProDescriptions", "ProDescriptions 高级描述列表", gallery.Display, 60, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		return func(c *ui.Context) {
				tbDemo(c, "远程数据 + 行内编辑", "Request 在后台加载数据（加载中显示遮罩）；值类型与 ProTable 一致；Editable 开启行内编辑，复用 ProForm 的编辑控件，保存时校验并异步提交。", func() {
					ui.ProDescriptions(c, p3DescItems()).Title("规则详情").Column(3).Bordered().
						Extra(func() { ui.Button(c, "刷新").Small() }).
						Request(func() (map[string]any, error) { return p3DescData(), nil }).
						Editable(func(key string, v any, all ui.ProValues, done func(error)) { done(nil) }).Show()
				})
				tbDemo(c, "无边框 · 中等尺寸", "", func() {
					ui.ProDescriptions(c, p3DescItems()[:6]).Column(3).Size(ui.Middle).
						DataSource(ptrVals(ui.ProValues(p3DescData()))).Show()
				})
			}, func(d *ui.Driver) {
				for i := 0; i < 50; i++ {
					d.Settle()
					if _, ok := d.FindText("TradeCode 99"); ok {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				d.ClickLabel("编辑负责人")
				d.ClickLabel("编辑规则名称")
			}
	})
}

func ptrVals(v ui.ProValues) *ui.ProValues { return &v }
