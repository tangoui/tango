package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

type formDemo struct {
	a, b, c, d, e, pwd, area string
	sel, sel2                string
	num                      float64
	ck1, ck2, ck3            bool
	group                    []string
	radio, radio2            string
	sw1, sw2                 bool
	slider                   float64
	date                     time.Time
	user, email, intro       string
	validated                bool
}

var fd = &formDemo{a: "Tango", d: "无效输入", pwd: "secret123", num: 3, ck2: true, group: []string{"Go"}, radio: "B", radio2: "上海", sw1: true, slider: 40, sel2: "杭州",
	date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)}

var (
	selLong     string
	selLongOpen bool
)

var selLongOpts = func() []ui.Option {
	out := make([]ui.Option, 40)
	for i := range out {
		out[i] = ui.Option{Value: fmt.Sprint(i + 1), Label: fmt.Sprintf("选项 %d", i+1)}
	}
	return out
}()

func init() {
	gallery.Register(gallery.Page{Name: "Input", Title: "Input 输入框", Group: gallery.Form, Order: 1, Render: func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Input(c, &fd.a).Placeholder("基本输入框")
			ui.Input(c, &fd.b).Placeholder("带前缀图标").Prefix(icon.User).AllowClear()
			ui.Input(c, &fd.c).Placeholder("请输入金额").PrefixText("¥").SuffixText("RMB")
			ui.Input(c, &fd.pwd).Password().A11yLabel("密码")
			ui.Input(c, &fd.d).Status("error").A11yLabel("错误状态")
			ui.Input(c, &fd.e).Placeholder("警告状态").Status("warning")
			ui.Input(c, &fd.e).Placeholder("禁用状态").Disabled(true)
			ui.TextArea(c, &fd.area).Placeholder("多行文本输入").Rows(2).ShowCount().MaxLength(100)
		}, layout.Gap(12))
	}})
	gallery.Register(gallery.Page{Name: "InputNumber", Title: "InputNumber 数字输入框", Group: gallery.Form, Order: 2, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			ui.InputNumber(c, &fd.num).Min(1).Max(10).A11yLabel("数量")
			ui.InputNumber(c, &fd.slider).Width(120).Precision(1).Step(0.5).A11yLabel("比例")
			ui.InputNumber(c, &fd.num).Disabled(true).A11yLabel("数量（禁用）")
		}, layout.Gap(12), layout.Middle())
	}})
	gallery.Register(gallery.Page{Name: "Select", Title: "Select 选择器", Group: gallery.Form, Order: 3, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			ui.Select(c, &fd.sel, ui.Opts("北京", "上海", "杭州", "深圳")).Placeholder("请选择城市").Width(180)
			ui.Select(c, &fd.sel2, ui.Opts("北京", "上海", "杭州", "深圳")).Searchable().AllowClear().Width(180).A11yLabel("城市（可搜索）")
			ui.Select(c, &fd.sel, ui.Opts("A")).Placeholder("禁用").Disabled(true).Width(140)
			long := ui.Select(c, &selLong, selLongOpts).Placeholder("长列表（40 项）").Width(180)
			if selLongOpen {
				long.Open()
			}
		}, layout.Gap(12), layout.WrapItems())
	}, Height: 420, Setup: func(d *ui.Driver) {
		// Open the long list with a value far down: the list scrolls inside
		// its 256px height to show the selected option.
		selLong, selLongOpen = "16", true
		d.Settle()
	}})
	gallery.Register(gallery.Page{Name: "Checkbox", Title: "Checkbox 多选框 / Radio 单选框", Group: gallery.Form, Order: 4, Render: func(c *ui.Context) {
		row(c, func() {
			ui.Checkbox(c, "未选中", &fd.ck1)
			ui.Checkbox(c, "已选中", &fd.ck2)
			ui.Checkbox(c, "半选", &fd.ck3).Indeterminate(true)
			ui.Checkbox(c, "禁用", &fd.ck2).Disabled(true)
		})
		ui.CheckboxGroup(c, &fd.group, ui.Opts("Go", "Rust", "TypeScript", "Swift"))
		ui.Column(c, func() {}, layout.H(12))
		ui.RadioGroup(c, &fd.radio, []ui.Option{{Label: "选项 A", Value: "A"}, {Label: "选项 B", Value: "B"}, {Label: "选项 C", Value: "C"}, {Label: "禁用", Value: "D", Disabled: true}})
		ui.Column(c, func() {}, layout.H(12))
		row(c, func() {
			ui.RadioGroup(c, &fd.radio2, ui.Opts("北京", "上海", "广州", "深圳")).Buttons()
			ui.RadioGroup(c, &fd.radio2, ui.Opts("北京", "上海", "广州")).Solid()
		})
	}})
	gallery.Register(gallery.Page{Name: "Switch", Title: "Switch 开关 / Slider 滑动输入条", Group: gallery.Form, Order: 5, Render: func(c *ui.Context) {
		row(c, func() {
			ui.Switch(c, &fd.sw1).A11yLabel("通知")
			ui.Switch(c, &fd.sw2).A11yLabel("自动更新")
			ui.Switch(c, &fd.sw1).Texts("开", "关").A11yLabel("通知（带文字）")
			ui.Switch(c, &fd.sw1).Small().A11yLabel("通知（小号）")
			ui.Switch(c, &fd.sw1).Loading(true).A11yLabel("同步")
			ui.Switch(c, &fd.sw2).Disabled(true).A11yLabel("自动更新（禁用）")
		})
		ui.Slider(c, &fd.slider, 0, 100).Layout(layout.MaxW(360)).A11yLabel("音量")
	}})
	gallery.Register(gallery.Page{Name: "Form", Title: "Form 表单", Group: gallery.Form, Order: 7, Render: func(c *ui.Context) {
		f := ui.Form(c).LabelWidth(90)
		f.Body(func() {
			f.Item("用户名").Rules(&fd.user, ui.Required("请输入用户名"), ui.MinLen(3, "用户名至少 3 个字符")).Body(func() {
				ui.Input(c, &fd.user).Placeholder("请输入用户名")
			})
			f.Item("邮箱").Rules(&fd.email, ui.Required("请输入邮箱"), ui.Email("邮箱格式不正确")).Body(func() {
				ui.Input(c, &fd.email).Placeholder("name@example.com")
			})
			f.Item("城市").Body(func() { ui.Select(c, &fd.sel2, ui.Opts("北京", "上海", "杭州")) })
			f.Item("简介").Help("不超过 100 字").Body(func() { ui.TextArea(c, &fd.intro).Rows(2) })
			f.Item("").NoLabel().Body(func() {
				ui.Row(c, func() {
					if ui.Button(c, "提交").Primary().Clicked() {
						f.Validate()
					}
					if ui.Button(c, "重置").Clicked() {
						f.ResetValidation()
					}
				}, layout.Gap(8))
			})
			if !fd.validated {
				fd.validated = true
				f.Validate()
			}
		})
	}})
}
