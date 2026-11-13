package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

// Demos of the date / time picker extensions: granularity, single-date
// presets / clear / status, RangePicker ShowTime, TimeRangePicker and the
// 12-hour TimePicker with disabled options.

type p2aDemo struct {
	week, month, quarter, year, monthOpen, weekOpen time.Time
	preset, status                                  time.Time
	span, spanOpen                                  [2]time.Time
	clock, clockOpen                                time.Time
	hours, hoursOpen                                [2]time.Time
}

var p2a = &p2aDemo{
	week:      time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
	month:     time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	quarter:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	year:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	monthOpen: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	weekOpen:  time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
	preset:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	span:      [2]time.Time{time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC), time.Date(2026, 1, 9, 18, 0, 0, 0, time.UTC)},
	spanOpen:  [2]time.Time{time.Date(2026, 1, 8, 9, 30, 0, 0, time.UTC), time.Date(2026, 1, 22, 18, 0, 0, 0, time.UTC)},
	clock:     time.Date(2026, 1, 1, 14, 30, 0, 0, time.UTC),
	clockOpen: time.Date(2026, 1, 1, 21, 15, 0, 0, time.UTC),
	hours:     [2]time.Time{time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)},
	hoursOpen: [2]time.Time{time.Date(2026, 1, 1, 8, 30, 0, 0, time.UTC), time.Date(2026, 1, 1, 17, 45, 0, 0, time.UTC)},
}

func p2aDay(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

func init() {
	entryPage(gallery.Page{Name: "DatePickerPicker", Title: "DatePicker 粒度（周 / 月 / 季度 / 年）", Parent: "DatePicker", Order: 49, Height: 620, Render: func(c *ui.Context) {
		gallery.Demo(c, "选择器粒度", "Picker(ui.PickerWeek / PickerMonth / PickerQuarter / PickerYear)；点击面板标题在年 / 月 / 日视图间跳转。", func() {
			entryRow(c, func() {
				ui.DatePicker(c, &p2a.week).Picker(ui.PickerWeek)
				ui.DatePicker(c, &p2a.month).Picker(ui.PickerMonth)
				ui.DatePicker(c, &p2a.quarter).Picker(ui.PickerQuarter)
				ui.DatePicker(c, &p2a.year).Picker(ui.PickerYear)
			})
		})
		gallery.Demo(c, "周选择（展开）", "整行高亮，左侧为周数。", func() {
			ui.DatePicker(c, &p2a.weekOpen).Picker(ui.PickerWeek).Open()
		})
	}, Setup: func(d *ui.Driver) {
		if r, ok := d.Find("2026-01-21"); ok {
			d.Move(r.Center())
		}
	}})
	entryPage(gallery.Page{Name: "DatePickerMonth", Title: "DatePicker 月份选择", Parent: "DatePicker", Order: 49, Height: 620, Render: func(c *ui.Context) {
		gallery.Demo(c, "月份选择（展开）", "点击年份标题进入年份视图。", func() {
			ui.DatePicker(c, &p2a.monthOpen).Picker(ui.PickerMonth).Open()
		})
	}})
	entryPage(gallery.Page{Name: "DatePickerPresets", Title: "DatePicker 预设、清除、状态", Parent: "DatePicker", Order: 49, Height: 560, Render: func(c *ui.Context) {
		gallery.Demo(c, "状态与宽度", "Status(\"error\" | \"warning\")、Width(px)、AllowClear(false)。", func() {
			entryRow(c, func() {
				ui.DatePicker(c, &p2a.status).Status("error")
				ui.DatePicker(c, &p2a.status).Status("warning")
				ui.DatePicker(c, &p2a.preset).Width(240).AllowClear(false)
			})
		})
		gallery.Demo(c, "预设日期（展开）", "Presets(ui.DatePreset{Label, Value}...)；悬停输入框显示清除按钮。", func() {
			ui.DatePicker(c, &p2a.preset).Presets(
				ui.DatePreset{Label: "今天", Value: p2aDay(1, 1)},
				ui.DatePreset{Label: "昨天", Value: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)},
				ui.DatePreset{Label: "一周前", Value: time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)},
			).Open()
		})
	}})
	entryPage(gallery.Page{Name: "RangePickerShowTime", Title: "RangePicker 带时间", Parent: "RangePicker", Order: 52, Width: 1000, Height: 760, Render: func(c *ui.Context) {
		gallery.Demo(c, "日期时间范围", "ShowTime()：两个月份面板，下方开始 / 结束时间列，“确定”提交。", func() {
			ui.RangePicker(c, &p2a.span).ShowTime()
		})
		gallery.Demo(c, "展开状态", "", func() {
			ui.RangePicker(c, &p2a.spanOpen).ShowTime().Format("2006-01-02 15:04").Open()
		})
	}})
	entryPage(gallery.Page{Name: "TimeRangePicker", Title: "TimeRangePicker 时间范围", Order: 50, Height: 520, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本用法", "开始 / 结束两组时间列；步长与禁用时间同 TimePicker。", func() {
			entryRow(c, func() {
				ui.TimeRangePicker(c, &p2a.hours)
				ui.TimeRangePicker(c, &p2a.hours).Use12Hours().Format("03:04 PM")
			})
		})
		gallery.Demo(c, "展开（15 分钟步长，0–7 点不可选）", "", func() {
			ui.TimeRangePicker(c, &p2a.hoursOpen).Format("15:04").MinuteStep(15).
				DisabledHours(func(h int) bool { return h < 8 }).Open()
		})
	}})
	entryPage(gallery.Page{Name: "TimePicker12Hours", Title: "TimePicker 12 小时制与禁用时间", Parent: "TimePicker", Order: 50, Height: 480, Render: func(c *ui.Context) {
		gallery.Demo(c, "12 小时制", "Use12Hours()；Steps(1, 5, 10)。", func() {
			entryRow(c, func() {
				ui.TimePicker(c, &p2a.clock).Use12Hours()
				ui.TimePicker(c, &p2a.clock).Steps(1, 5, 10)
			})
		})
		gallery.Demo(c, "禁用时间（展开）", "DisabledHours / DisabledMinutes：22 点后、整点后 40 分以后不可选。", func() {
			ui.TimePicker(c, &p2a.clockOpen).Use12Hours().
				DisabledHours(func(h int) bool { return h >= 22 }).
				DisabledMinutes(func(h, m int) bool { return m > 40 }).Open()
		})
	}})
}
