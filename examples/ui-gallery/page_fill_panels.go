package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

var fillDPDemo struct {
	day, week, month, quarter, year time.Time
	from, to                        time.Time
	mfrom, mto                      time.Time
	picker                          string
}

func fillDPReset() {
	d := &fillDPDemo
	d.day = time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	d.week = time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	d.month = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	d.quarter = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	d.year = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	d.from = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	d.to = time.Date(2026, 12, 8, 0, 0, 0, 0, time.UTC)
	d.mfrom = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	d.mto = time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC)
	d.picker = "date"
}

// fillMondayLocale is English with weeks starting on Monday.
var fillMondayLocale = func() ui.Locale { l := *ui.EnUS; l.WeekStart = time.Monday; return l }()

func fillWeekend(t time.Time) bool { return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday }

func init() {
	fillDPReset()
	morePage(gallery.Page{Name: "DatePickerPanel", Title: "DatePickerPanel 日期面板", Group: gallery.Form, Order: 49, Width: 1180,
		Render: func(c *ui.Context) {
			d := &fillDPDemo
			gallery.Demo(c, "五种粒度", "可直接内嵌的面板；点击标题在 年 / 月 / 日 视图间跳转；周起始日随 Locale.WeekStart。", func() {
				ui.Row(c, func() {
					ui.DatePickerPanel(c, &d.day).DisabledDate(fillWeekend)
					ui.DatePickerPanel(c, &d.week).Picker(ui.PickerWeek)
					ui.Column(c, func() {
						ui.DatePickerPanel(c, &d.quarter).Picker(ui.PickerQuarter)
						ui.DatePickerPanel(c, &d.year).Picker(ui.PickerYear)
					}, layout.Gap(16))
				}, layout.Gap(16), layout.WrapItems())
			})
			gallery.Demo(c, "范围选择", "两次点击确定起止；第一次点击后悬停预览范围。", func() {
				ui.Column(c, func() {
					ui.DatePickerPanel(c, &d.from).Range(&d.to)
					ui.DatePickerPanel(c, &d.mfrom).Range(&d.mto).Picker(ui.PickerMonth)
				}, layout.Gap(16))
			})
			gallery.Demo(c, "English、周一开始的语言包与禁用", "", func() {
				ui.Row(c, func() {
					ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() {
						ui.DatePickerPanel(c, &d.day)
					})
					ui.ConfigProvider(c).Locale(&fillMondayLocale).Body(func() {
						ui.DatePickerPanel(c, &d.week).Picker(ui.PickerWeek)
					})
					ui.DatePickerPanel(c, &d.month).Picker(ui.PickerMonth).Disabled(true)
				}, layout.Gap(16))
			})
		},
	}, fillDPReset)
}

var fillCPDemo struct {
	a, b, c theme.Color
	g       theme.Gradient
}

func fillCPReset() {
	fillCPDemo.a = theme.Hex("#1677ff")
	fillCPDemo.b = theme.RGBA(64, 158, 255, 0.6)
	fillCPDemo.c = theme.Hex("#67c23a")
	fillCPDemo.g = theme.Gradient{Angle: 90, Stops: []theme.GradientStop{
		{Offset: 0, Color: theme.Hex("#409eff")}, {Offset: 0.6, Color: theme.Hex("#67c23a")}, {Offset: 1, Color: theme.Hex("#e6a23c")}}}
}

func init() {
	fillCPReset()
	presets := []theme.Color{theme.Hex("#ff4500"), theme.Hex("#ff8c00"), theme.Hex("#ffd700"), theme.Hex("#90ee90"), theme.Hex("#00ced1"), theme.Hex("#1e90ff"), theme.Hex("#c71585"), theme.RGBA(255, 69, 0, 0.68)}
	morePage(gallery.Page{Name: "ColorPickerPanel", Title: "ColorPickerPanel 取色面板", Group: gallery.Form, Order: 61,
		Render: func(c *ui.Context) {
			d := &fillCPDemo
			gallery.Demo(c, "内嵌取色面板", "饱和度/明度区、色相条、透明度条、输入框与预设色；修改即时生效。", func() {
				ui.Row(c, func() {
					ui.ColorPickerPanel(c, &d.a).NoAlpha()
					ui.ColorPickerPanel(c, &d.b).Presets(presets...)
					ui.Column(c, func() {
						ui.ColorPickerPanel(c, &d.c).Border(false).Disabled(true).NoAlpha()
						ui.Row(c, func() {
							for _, col := range []theme.Color{d.a, d.b, d.c} {
								ui.Div(c, layout.Size(28, 28)).Bg(col).Radius(4)
							}
						}, layout.Gap(8))
					}, layout.Gap(12))
				}, layout.Gap(16), layout.WrapItems())
			})
			gallery.Demo(c, "渐变面板", "ColorPickerPanelGradient 绑定 theme.Gradient：色标条与角度在上，编辑选中色标的颜色；ColorPickerPanel 单色用法不变。", func() {
				ui.Row(c, func() {
					ui.ColorPickerPanelGradient(c, &d.g)
					ui.Column(c, func() {
						ui.Div(c, layout.Size(160, 48)).Radius(6).Paint(func(l *paint.List, r geom.Rect) {
							// Preview: the gradient painted left to right.
							s := d.g.Sorted().Stops
							stops := make([]paint.Stop, len(s))
							for i, st := range s {
								stops[i] = paint.Stop{Offset: float32(st.Offset), Color: st.Color}
							}
							l.LinearStops(r, geom.Rad(6), geom.Pt(r.X, r.Y), geom.Pt(r.Right(), r.Y), stops...)
						})
						ui.Text(c, d.g.CSS()).Secondary().Size(12).Layout(layout.W(220))
					}, layout.Gap(8))
				}, layout.Gap(16), layout.WrapItems())
			})
		},
	}, fillCPReset)
}
