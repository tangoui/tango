package main

import (
	"image/color"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// Third Pro round: ProForm read-only mode, the standalone ProField,
// SchemaForm with layoutType and formList, and the added value types in
// forms, descriptions and tables.

var fpMoreImg = miscThumb(color.NRGBA{22, 119, 255, 255}, color.NRGBA{114, 46, 209, 255})

func fpMoreFields() []ui.ProField {
	return []ui.ProField{
		{Key: "week", Label: "周", ValueType: ui.ValueDateWeek, Span: 12},
		{Key: "month", Label: "月份", ValueType: ui.ValueDateMonth, Span: 12},
		{Key: "quarter", Label: "季度", ValueType: ui.ValueDateQuarter, Span: 12},
		{Key: "year", Label: "年份", ValueType: ui.ValueDateYear, Span: 12},
		{Key: "span", Label: "活动时间", ValueType: ui.ValueDateTimeRange},
		{Key: "cover", Label: "封面", ValueType: ui.ValueImage, Span: 12},
		{Key: "owner", Label: "负责人", ValueType: ui.ValueAvatar, Span: 12},
		{Key: "dur", Label: "耗时（秒）", ValueType: ui.ValueSecond, Span: 12},
		{Key: "seen", Label: "最近访问", ValueType: ui.ValueFromNow, Span: 12},
		{Key: "cmd", Label: "启动命令", ValueType: ui.ValueCode},
		{Key: "cfg", Label: "配置", ValueType: ui.ValueJSONCode},
		{Key: "code", Label: "验证码", ValueType: ui.ValueCaptcha, CaptchaSeconds: 60},
	}
}

func fpMoreValues(now time.Time) ui.ProValues {
	day := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	return ui.ProValues{
		"week": day, "month": day, "quarter": day, "year": day,
		"span":  [2]time.Time{day, day.Add(50 * time.Hour)},
		"cover": fpMoreImg, "owner": "曲丽丽",
		"dur": 3725.0, "seen": now.Add(-5 * time.Minute),
		"cmd": "go run ./examples/ui-gallery -only ProForm",
		"cfg": `{"theme":"dark","columns":2}`,
	}
}

func init() {
	fpPage("ProFormReadonly", "ProForm 只读模式", 31, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		readonly := true
		vals := ui.ProValues{"name": "TradeCode 99", "owner": "qll", "amount": 12800.0, "province": "zj", "city": "hz",
			"deadline": time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "tags": []string{"core", "beta"}, "status": "online",
			"budgetMax": 20000.0, "notify": true, "desc": "灰度期间每日巡检。"}
		return func(c *ui.Context) {
			tbDemo(c, "Readonly 整表只读 · ProField.Readonly 单字段只读", "只读时每个字段按 valueType 显示格式化后的值（金额、日期、选项标签、开关文字…），不做校验、不显示提交按钮；取消勾选回到编辑态，其中“规则名称”设了 Readonly 仍保持只读。", func() {
				ui.Checkbox(c, "只读", &readonly)
				ui.Gap(c, 12)
				fields := fpRuleFields()
				fields[0].Readonly = true
				ui.ProForm(c, fields).Columns(2).Values(&vals).Readonly(readonly).Show()
			})
		}, nil
	})

	fpPage("SchemaForm", "SchemaForm 统一入口（layoutType、formList）", 37, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		lt := string(ui.SchemaLayoutForm)
		open := false
		vals := ui.ProValues{"members": []ui.ProValues{{"name": "曲丽丽", "role": "owner"}}}
		schema := []ui.ProField{
			{Key: "name", Label: "规则名称", Group: "基本信息", Required: true, Span: 12},
			{Key: "status", Label: "状态", Group: "基本信息", ValueType: ui.ValueSelect, ValueEnum: fpStatus, Span: 12},
			{Key: "month", Label: "生效月份", Group: "基本信息", ValueType: ui.ValueDateMonth, Span: 12},
			{Key: "amount", Label: "预算金额", Group: "预算与成员", ValueType: ui.ValueMoney, Span: 12},
			{Key: "members", Label: "成员", Group: "预算与成员", ValueType: ui.ValueFormList, Span: 24, List: &ui.ProFieldList{Max: 3, AddText: "添加成员",
				Fields: []ui.ProField{
					{Key: "name", Label: "姓名", Required: true, Width: 160},
					{Key: "role", Label: "角色", ValueType: ui.ValueSelect, Initial: "dev", Width: 140,
						ValueEnum: []ui.ProEnum{{Value: "owner", Label: "负责人"}, {Value: "dev", Label: "开发"}, {Value: "qa", Label: "测试"}}},
				}}},
		}
		opts := []ui.Option{
			{Label: "Form", Value: string(ui.SchemaLayoutForm)}, {Label: "ModalForm", Value: string(ui.SchemaLayoutModalForm)},
			{Label: "DrawerForm", Value: string(ui.SchemaLayoutDrawerForm)}, {Label: "StepsForm", Value: string(ui.SchemaLayoutStepsForm)},
			{Label: "QueryFilter", Value: string(ui.SchemaLayoutQueryFilter)}, {Label: "LightFilter", Value: string(ui.SchemaLayoutLightFilter)},
		}
		return func(c *ui.Context) {
			tbDemo(c, "LayoutType 切换外壳", "同一份 schema 渲染为 Form / ModalForm / DrawerForm / StepsForm / QueryFilter / LightFilter；StepsForm 里每组 Group 成为一步；valueType 为 formList 的字段用 Form.List 编辑多行（ProField.List 给出每行字段）。", func() {
				ui.Row(c, func() {
					ui.Segmented(c, &lt, opts)
					t := ui.SchemaLayoutType(lt)
					if (t == ui.SchemaLayoutModalForm || t == ui.SchemaLayoutDrawerForm) && ui.Button(c, "打开表单").Primary().Clicked() {
						open = true
					}
				}, layout.Gap(12), layout.Middle(), layout.Margin(geom.Insets{Bottom: 16}))
				ui.SchemaForm(c, schema).LayoutType(ui.SchemaLayoutType(lt)).Columns(2).Values(&vals).
					Open(&open).Title("编辑规则").Width(720).Show()
			})
		}, nil
	})

	p2ePage("ProFieldMore", "ProField 更多值类型与单独渲染", gallery.Form, "ProForm", 36, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var vals, view ui.ProValues
		mode := string(ui.ProFieldRead)
		var price, quarter, seen any = 12800.5, time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), nil
		type job struct {
			name string
			dur  float64
			cmd  string
		}
		jobs := []job{{"构建", 95, "go build ./..."}, {"测试", 1840, "go test ./ui/"}, {"截图", 7260, "go run ./examples/ui-gallery"}}
		return func(c *ui.Context) {
			if vals == nil {
				vals, view = fpMoreValues(c.Now()), fpMoreValues(c.Now())
				seen = c.Now().Add(-3 * time.Hour)
			}
			tbDemo(c, "新增 valueType：编辑与只读", "dateWeek / dateMonth / dateQuarter / dateYear 用对应粒度的 DatePicker；dateTimeRange 带时间；image / avatar 用单图上传卡；second 时长；fromNow 相对时间；code / jsonCode 用多行输入（JSON 自动校验）；captcha 带“获取验证码”倒计时。下方是同一组值在 ProDescriptions 里的只读展示。", func() {
				ui.ProForm(c, fpMoreFields()).Columns(2).Values(&vals).NoReset().Show()
				var items []ui.ProDescItem
				for _, fd := range fpMoreFields() {
					items = append(items, ui.ProDescItem{Key: fd.Key, Label: fd.Label, ValueType: fd.ValueType})
				}
				ui.ProDescriptions(c, items).Column(2).Bordered().DataSource(&view).Show()
			})
			tbDemo(c, "ProFieldOf 单独渲染", "不放进表单也能按 valueType 渲染一个值；Mode 在 read / edit 之间切换。", func() {
				ui.Segmented(c, &mode, []ui.Option{{Label: "只读 read", Value: string(ui.ProFieldRead)}, {Label: "编辑 edit", Value: string(ui.ProFieldEdit)}})
				ui.Gap(c, 12)
				m := ui.ProFieldMode(mode)
				ui.Row(c, func() {
					for _, f := range []struct {
						fd ui.ProField
						v  *any
					}{
						{ui.ProField{Label: "价格", ValueType: ui.ValueMoney}, &price},
						{ui.ProField{Label: "季度", ValueType: ui.ValueDateQuarter}, &quarter},
						{ui.ProField{Label: "最近访问", ValueType: ui.ValueFromNow}, &seen},
					} {
						ui.Key(c, f.fd.Label, func() {
							ui.Column(c, func() {
								ui.Text(c, f.fd.Label).Secondary()
								ui.ProFieldOf(c, f.fd, f.v).Mode(m).Show()
							}, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Gap(6))
						})
					}
				}, layout.Gap(24))
			})
			tbDemo(c, "ProTable 列", "index 显示行号，second 显示时长，code 显示单行代码，image 显示缩略图。", func() {
				ui.ProTable(c, []ui.ProColumn[job]{
					{Key: "idx", Title: "序号", ValueType: ui.ValueIndex, Width: 72},
					{Key: "pic", Title: "图标", ValueType: ui.ValueImage, Width: 80, Value: func(job) any { return fpMoreImg }},
					{Key: "name", Title: "任务", Value: func(j job) any { return j.name }},
					{Key: "dur", Title: "耗时", ValueType: ui.ValueSecond, Value: func(j job) any { return j.dur }},
					{Key: "cmd", Title: "命令", ValueType: ui.ValueCode, Value: func(j job) any { return j.cmd }},
				}).Title("任务耗时").RowKey(func(j job) string { return j.name }).NoSearch().NoOptions().NoPagination().
					Request(func(_ ui.ProRequest, done func(ui.ProResult[job])) {
						done(ui.ProResult[job]{Rows: jobs, Total: len(jobs)})
					}).Show()
			})
		}, nil
	})
}
