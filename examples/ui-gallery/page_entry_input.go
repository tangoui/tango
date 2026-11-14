package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var inx = struct {
	col1, col2, col3, col4, colEmpty theme.Color
	colHSB, colGroups                theme.Color
	url, site, amount, city, num     string
	sel                              string
	numVal                           float64
	q1, q2, q3, q4, q5               string
	ta1, ta2, ta3, ta4, ta5          string
	otp1, otp2, otp3, otp4, otp5     string
	grad1, grad2                     theme.Gradient
	colHover                         theme.Color
}{
	grad1: theme.Gradient{Angle: 90, Stops: []theme.GradientStop{
		{Offset: 0, Color: theme.Hex("#1677ff")}, {Offset: 1, Color: theme.Hex("#722ed1")}}},
	grad2: theme.Gradient{Angle: 135, Stops: []theme.GradientStop{
		{Offset: 0, Color: theme.Hex("#fa8c16")}, {Offset: 0.5, Color: theme.Hex("#f5222d")}, {Offset: 1, Color: theme.RGBA(114, 46, 209, .6)}}},
	colHover: theme.Hex("#13c2c2"),
	col1:     theme.Hex("#1677ff"), col2: theme.RGBA(245, 34, 45, .6), col3: theme.Hex("#52c41a"), col4: theme.Hex("#722ed1"),
	colHSB: theme.Hex("#3366cc"), colGroups: theme.Hex("#13c2c2"),
	url: "tango-ui", site: "mysite", sel: "https://", city: "杭州", numVal: 42,
	q2: "Tango", q4: "关键字",
	ta2:  "Tango 是一个纯 Go 实现的应用框架，自绘 GUI 层提供与 Ant Design、Element Plus 看齐的组件。",
	ta3:  "超出最大长度时计数标红并显示错误态。这一段文字已经超过二十个字符了",
	ta4:  "第一行\n第二行\n第三行\n第四行\n第五行\n第六行（超出最大行数后可滚动）",
	otp2: "1234", otp3: "882", otp4: "520131",
}

var inxPresetGroups = []ui.ColorPresetGroup{
	{Label: "主色", Colors: []theme.Color{theme.Hex("#1677ff"), theme.Hex("#13c2c2"), theme.Hex("#52c41a"), theme.Hex("#fadb14"), theme.Hex("#fa8c16"), theme.Hex("#f5222d")}},
	{Label: "中性色", Colors: []theme.Color{theme.Hex("#000000"), theme.Hex("#595959"), theme.Hex("#bfbfbf"), theme.Hex("#f0f0f0"), theme.Hex("#ffffff")}},
	{Label: "最近使用", Colors: []theme.Color{theme.Hex("#722ed1"), theme.Hex("#eb2f96")}, Closed: true},
}

var inxPresets = []theme.Color{
	theme.Hex("#f5222d"), theme.Hex("#fa8c16"), theme.Hex("#fadb14"), theme.Hex("#52c41a"),
	theme.Hex("#13c2c2"), theme.Hex("#1677ff"), theme.Hex("#722ed1"), theme.Hex("#eb2f96"),
	theme.RGBA(0, 0, 0, .45), theme.RGBA(22, 119, 255, .5),
}

func inxLabel(c *ui.Context, s string) {
	ui.Text(c, s).Tertiary().Size(12).Layout(layout.Margin(geom.Insets{Bottom: -4}))
}

func init() {
	entryPage(gallery.Page{Name: "ColorPicker", Title: "ColorPicker 颜色选择器", Order: 80, Height: 2300,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "基本用法与状态", "触发器：色块 / 带文字 / 透明色 / 空值 / 校验 / 禁用", func() {
				entryRow(c, func() {
					ui.ColorPicker(c, &inx.col1)
					ui.ColorPicker(c, &inx.col1).ShowText()
					ui.ColorPicker(c, &inx.col2).ShowText().Format("rgb")
					ui.ColorPicker(c, &inx.colEmpty).ShowText()
					ui.ColorPicker(c, &inx.col3).Status("error")
					ui.ColorPicker(c, &inx.col4).Disabled(true)
					ui.ColorPicker(c, &inx.col3).Size(ui.Small)
					ui.ColorPicker(c, &inx.col4).Size(ui.Large)
				})
			})
			gallery.Demo(c, "HSB 格式与多组预设", "Format(\"hsb\") 显示 hsb(色相, 饱和度%, 亮度%) 并按 H/S/B 编辑；PresetGroups 分组展示预设，Ant 风格的组标题可折叠", func() {
				entryRow(c, func() {
					ui.ColorPicker(c, &inx.colHSB).ShowText().Format("hsb")
					ui.ColorPickerPanel(c, &inx.colGroups).Format("hsb").PresetGroups(inxPresetGroups...)
				})
			})
			gallery.Demo(c, "渐变色与悬停触发", "ColorPickerGradient 绑定 theme.Gradient（线性渐变）：色标条上点击添加、拖动移动、拖离删除，下方编辑选中色标的颜色；Trigger(TriggerHover) 悬停即弹出", func() {
				entryRow(c, func() {
					ui.ColorPickerGradient(c, &inx.grad1).ShowText()
					ui.ColorPickerGradient(c, &inx.grad2)
					ui.ColorPicker(c, &inx.colHover).Trigger(ui.TriggerHover).ShowText()
				})
			})
			gallery.Demo(c, "展开的面板", "饱和度面板、色相 / 透明度条、HEX 输入与预设色", func() {
				ui.ColorPicker(c, &inx.col2).ShowText().Presets(inxPresets...).AllowClear().Open()
			})
			ui.Gap(c, 420)
			gallery.Demo(c, "展开的渐变面板", "色标条（选中色标带描边）与角度输入在上，其余与单色面板相同", func() {
				ui.ColorPickerGradient(c, &inx.grad2).ShowText().Open()
			})
			ui.Gap(c, 480)
		}})

	entryPage(gallery.Page{Name: "Compact", Title: "Compact 紧凑组合（Space.Compact / Input.Group）", Order: 81,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "前置 / 后置标签", "Addon 与 Input 贴合（Ant addonBefore/After、Element prepend/append）", func() {
				entryCol(c, func() {
					ui.Compact(c).Body(func() {
						ui.Addon(c, "https://")
						ui.Input(c, &inx.url).Layout(layout.W(220)).A11yLabel("网址")
						ui.Addon(c, ".com")
					})
					ui.Compact(c).Body(func() {
						ui.Select(c, &inx.sel, ui.Opts("https://", "http://")).Width(100).A11yLabel("协议")
						ui.Input(c, &inx.site).Placeholder("站点").Layout(layout.W(200))
						ui.Button(c, "访问").Primary()
					})
					ui.Compact(c).Body(func() {
						ui.Addon(c, "¥")
						ui.InputNumber(c, &inx.numVal).Width(120).A11yLabel("金额")
						ui.Addon(c, "元")
					})
				})
			})
			gallery.Demo(c, "紧凑组合", "Space.Compact：输入框、选择器、按钮组合", func() {
				entryCol(c, func() {
					ui.Compact(c).Body(func() {
						ui.Input(c, &inx.amount).Placeholder("金额").Layout(layout.W(160))
						ui.Input(c, &inx.num).Placeholder("备注").Layout(layout.W(160))
						ui.Button(c, "提交")
					})
					ui.Compact(c).Body(func() {
						ui.Button(c, "上一步")
						ui.Button(c, "下一步")
						ui.Button(c, "完成").Primary()
					})
					ui.Compact(c).Body(func() {
						ui.Select(c, &inx.city, ui.Opts("北京", "上海", "杭州")).Width(100).A11yLabel("城市")
						ui.Input(c, &inx.amount).Placeholder("错误态").Status("error").Layout(layout.W(180))
					})
					ui.Compact(c).Body(func() {
						ui.Addon(c, "禁用").Disabled(true)
						ui.Input(c, &inx.url).Disabled(true).Layout(layout.W(200)).A11yLabel("网址（禁用）")
					})
				})
			})
			gallery.Demo(c, "按钮组 / 竖向", "Compact().Vertical() 上下贴合；包在 Dropdown 等单子节点容器里的控件同样生效", func() {
				ui.Row(c, func() {
					ui.Compact(c).Body(func() {
						ui.Button(c, "").Icon(icon.Plus).A11yLabel("添加")
						ui.Button(c, "").Icon(icon.Minus).A11yLabel("减少")
						ui.Button(c, "").Icon(icon.Delete).A11yLabel("删除")
					})
					ui.Compact(c).Vertical().Body(func() {
						ui.Button(c, "竖向一")
						ui.Button(c, "竖向二")
						ui.Button(c, "竖向三")
					})
				}, layout.Gap(32), layout.AlignItems(layout.AlignStart))
			})
		}})

	entryPage(gallery.Page{Name: "Search", Title: "Input.Search 搜索框", Order: 82,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "搜索框", "图标按钮 / 文字主按钮 / 可清除 / 加载中 / 校验 / 禁用", func() {
				entryCol(c, func() {
					ui.Search(c, &inx.q1).Placeholder("请输入搜索内容")
					ui.Search(c, &inx.q2).Placeholder("请输入搜索内容").AllowClear()
					ui.Search(c, &inx.q3).Placeholder("带主按钮").EnterButton("搜索").Width(300)
					ui.Search(c, &inx.q4).Loading(true)
					ui.Search(c, &inx.q4).EnterButton("搜索").Loading(true).Size(ui.Large).Width(320)
					ui.Search(c, &inx.q5).Placeholder("错误状态").Status("error")
					ui.Search(c, &inx.q5).Placeholder("禁用").Disabled(true)
				})
			})
		},
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("搜索"); ok {
				d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
			}
		}})

	entryPage(gallery.Page{Name: "TextArea", Title: "TextArea 文本域", Order: 83,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "字数统计", "ShowCount + MaxLength；Exceed 允许超出并标红", func() {
				entryCol(c, func() {
					ui.TextArea(c, &inx.ta1).Placeholder("请输入内容").ShowCount().MaxLength(100)
					ui.TextArea(c, &inx.ta2).ShowCount().MaxLength(100).AllowClear().A11yLabel("简介")
					ui.TextArea(c, &inx.ta3).ShowCount().MaxLength(20).Exceed().Rows(2).A11yLabel("摘要")
				}, layout.MaxW(460))
			})
			gallery.Demo(c, "自适应高度与滚动", "AutoSize(2, 4)：随内容增高，超过 4 行后滚轮滚动", func() {
				entryCol(c, func() {
					ui.TextArea(c, &inx.ta5).Placeholder("AutoSize(2, 4)").AutoSize(2, 4)
					ui.TextArea(c, &inx.ta4).AutoSize(2, 4).A11yLabel("多行内容")
					ui.TextArea(c, &inx.ta2).Rows(2).Status("warning").A11yLabel("简介（警告）")
					ui.TextArea(c, &inx.ta2).Rows(2).Disabled(true).A11yLabel("简介（禁用）")
				}, layout.MaxW(460))
			})
		}})

	entryPage(gallery.Page{Name: "OTP", Title: "Input.OTP 验证码输入", Order: 84,
		Render: func(c *ui.Context) {
			entryCol(c, func() {
				inxLabel(c, "默认（聚焦第一个空格）")
				o := ui.OTP(c, &inx.otp1, 6).Numeric()
				if inx.otp1 == "" && c.Focused() == 0 {
					o.Focus()
				}
				inxLabel(c, "已输入部分")
				ui.OTP(c, &inx.otp2, 6)
				inxLabel(c, "掩码")
				ui.OTP(c, &inx.otp4, 6).Mask("•")
				inxLabel(c, "小尺寸 / 大尺寸")
				ui.OTP(c, &inx.otp3, 4).Size(ui.Small)
				ui.OTP(c, &inx.otp3, 4).Size(ui.Large)
				inxLabel(c, "错误 / 禁用")
				ui.OTP(c, &inx.otp2, 6).Status("error")
				ui.OTP(c, &inx.otp2, 6).Disabled(true)
			})
		}})
}
