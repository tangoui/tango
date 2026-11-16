package main

// Spec alignment batch 1, group B: demos of the capabilities added to the
// input and form components (docs/specs/align/1-input.md).

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b1bSpin, b1bSpinSm, b1bSpinMax = 3.0, 2.0, 10.0
	b1bNoCtl, b1bRO, b1bStrict     = 8.0, 42.0, 4.0
	b1bOTP1, b1bOTP2, b1bOTP3      = "123", "", "8848"
	b1bRadio, b1bRadio2, b1bRadio3 = "a", "b", "m"
	b1bSw1, b1bSw2                 = true, false
	b1bSl1, b1bSl2, b1bSl3         = 30.0, 60.0, 37.0
	b1bName, b1bNick, b1bNote      string
	b1bFiles                       = []ui.UploadFile{{Name: "avatar.png", Size: 52000, Status: "done"}}
)

func init() {
	gallery.Register(gallery.Page{Name: "InputNumberAlign", Parent: "InputNumber", Title: "InputNumber 模式、控制按钮与只读", Group: gallery.Form, Order: 2,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "spinner 模式", "Mode(\"spinner\")：− 在前、+ 在后，数值居中；到 Max 时 + 禁用（antd 6 mode）。", func() {
				row(c, func() {
					ui.InputNumber(c, &b1bSpin).Mode("spinner").Min(0).Max(10).A11yLabel("数量")
					ui.InputNumber(c, &b1bSpinSm).Mode("spinner").Size(ui.Small).Min(0).Max(10).A11yLabel("数量（小）")
					ui.InputNumber(c, &b1bSpinMax).Mode("spinner").Size(ui.Large).Min(0).Max(10).A11yLabel("数量（大）")
				})
			})
			gallery.Demo(c, "隐藏按钮、只读与严格步长", "Controls(false) 去掉步进按钮；ReadOnly 可选中不可改；StepStrictly 提交时取步长整数倍（Element step-strictly）。", func() {
				row(c, func() {
					ui.InputNumber(c, &b1bNoCtl).Controls(false).A11yLabel("无按钮")
					ui.InputNumber(c, &b1bRO).ReadOnly().A11yLabel("只读")
					ui.InputNumber(c, &b1bStrict).Step(2).StepStrictly().A11yLabel("步长 2")
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "OTPAlign", Parent: "OTP", Title: "OTP 分隔符、变体与只读", Group: gallery.Form, Order: 9,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "分隔符", "Separator(\"-\") 每两格之间一个；SeparatorFunc 只在第 3 格后。", func() {
				ui.Column(c, func() {
					ui.OTP(c, &b1bOTP1, 6).Separator("-").A11yLabel("验证码")
					ui.OTP(c, &b1bOTP2, 6).SeparatorFunc(func(i int) string {
						if i == 2 {
							return "-"
						}
						return ""
					}).A11yLabel("验证码 2")
				}, layout.Gap(12))
			})
			gallery.Demo(c, "变体与只读", "Variant(\"filled\" | \"underlined\")（antd variant、Element type）；ReadOnly。", func() {
				ui.Column(c, func() {
					ui.OTP(c, &b1bOTP3, 4).Variant("filled").A11yLabel("填充")
					ui.OTP(c, &b1bOTP3, 4).Variant("underlined").A11yLabel("下划线")
					ui.OTP(c, &b1bOTP3, 4).ReadOnly().A11yLabel("只读")
				}, layout.Gap(12))
			})
		}})

	gallery.Register(gallery.Page{Name: "RadioAlign", Parent: "Radio", Title: "Radio 竖向与撑满", Group: gallery.Form, Order: 4,
		Render: func(c *ui.Context) {
			opts := ui.Opts("a", "b", "c")
			gallery.Demo(c, "竖向", "Vertical / Orientation(\"vertical\")（antd 6 vertical、orientation）。", func() {
				row(c, func() {
					ui.RadioGroup(c, &b1bRadio, opts).Vertical().A11yLabel("竖向")
					ui.RadioGroup(c, &b1bRadio2, opts).Vertical().Buttons().A11yLabel("竖向按钮")
				})
			})
			gallery.Demo(c, "撑满", "Block 按钮平分整行（antd block）。", func() {
				ui.Column(c, func() {
					ui.RadioGroup(c, &b1bRadio3, ui.Opts("日", "周", "月", "m")).Buttons().Solid().Block().A11yLabel("周期")
				}, layout.W(420))
			})
		}})

	gallery.Register(gallery.Page{Name: "SwitchAlign", Parent: "Switch", Title: "Switch 滑块图标 / Slider 反向与不包含", Group: gallery.Form, Order: 5,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "滑块上的图标", "ActionIcons（Element active-action-icon / inactive-action-icon）。", func() {
				row(c, func() {
					ui.Switch(c, &b1bSw1).ActionIcons(icon.Check, icon.Close).A11yLabel("开")
					ui.Switch(c, &b1bSw2).ActionIcons(icon.Check, icon.Close).A11yLabel("关")
				})
			})
			gallery.Demo(c, "Slider 反向、不包含", "Reverse（antd reverse）；Included(false) 不填充轨道（antd included）；松手时 ChangeComplete。", func() {
				ui.Column(c, func() {
					ui.Slider(c, &b1bSl1, 0, 100).Reverse().A11yLabel("反向").Layout(layout.W(360))
					ui.Slider(c, &b1bSl2, 0, 100).Included(false).Marks(map[float64]string{0: "0", 60: "60", 100: "100"}).A11yLabel("不包含").Layout(layout.W(360))
					ui.Slider(c, &b1bSl3, 0, 100).Disabled(true).A11yLabel("禁用").Layout(layout.W(360))
				}, layout.Gap(16))
			})
		}})

	gallery.Register(gallery.Page{Name: "FormAlign", Parent: "Form", Title: "Form 可选标记 / Upload 单文件替换", Group: gallery.Form, Order: 7,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "可选标记", "RequiredMarkOptional（antd requiredMark=\"optional\"）：必填项不显示星号，非必填项标签后显示“（可选）”。", func() {
				f := ui.Form(c).RequiredMarkOptional().LabelWidth(160).Layout(layout.W(480))
				f.Body(func() {
					f.Item("用户名").Required().Body(func() { ui.Input(c, &b1bName) })
					f.Item("昵称").Tooltip("显示在个人主页").Body(func() { ui.Input(c, &b1bNick) })
					f.Item("备注").Body(func() { ui.Input(c, &b1bNote) })
				})
			})
			gallery.Demo(c, "单文件替换", "MaxCount(1)：再次选择时替换当前文件，按钮保持可用（antd maxCount=1）。", func() {
				ui.Upload(c, &b1bFiles).MaxCount(1)
			})
		}})
}
