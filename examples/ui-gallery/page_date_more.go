package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// Demos of the date gap fill: multiple dates (Ant multiple, Element
// dates / months / years), NeedConfirm, DateTimePicker DisabledDate /
// DisabledTime / Presets, RangePicker DisabledTime and Calendar Range.

type dmDemo struct {
	days, daysOpen, many, months, years []time.Time
	confirm                             time.Time
	dtOpen, dtClosed                    time.Time
	spanOpen                            [2]time.Time
	calRange, calRangeCard              time.Time
}

func dmDay(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

var dmd = &dmDemo{
	days:     []time.Time{dmDay(1, 6), dmDay(1, 8)},
	daysOpen: []time.Time{dmDay(1, 7), dmDay(1, 14), dmDay(1, 21)},
	many:     []time.Time{dmDay(1, 5), dmDay(1, 6), dmDay(1, 7), dmDay(1, 8)},
	months:   []time.Time{dmDay(3, 1), dmDay(6, 1)},
	years:    []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	confirm:  dmDay(1, 9),
	dtOpen:   time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC),
	spanOpen: [2]time.Time{time.Date(2026, 1, 8, 9, 30, 0, 0, time.UTC), time.Date(2026, 1, 22, 17, 0, 0, 0, time.UTC)},
	calRange: dmDay(1, 14),
}

// dmWorkHours disables hours outside 9-18 and, on Fridays, the afternoon.
func dmWorkHours(day time.Time) ui.DisabledTimes {
	return ui.DisabledTimes{
		Hours: func(h int) bool {
			if day.Weekday() == time.Friday && h >= 12 {
				return true
			}
			return h < 9 || h > 18
		},
		Minutes: func(h, m int) bool { return h == 18 && m > 0 },
	}
}

func init() {
	entryPage(gallery.Page{Name: "DatePickerMultiple", Title: "DatePicker 多选日期", Parent: "DatePicker", Order: 49, Width: 1000, Height: 900, Render: func(c *ui.Context) {
		gallery.Demo(c, "多选", "MultiDatePicker(c, &[]time.Time)：每次点击切换一个日期，面板不关闭；选中项以标签显示，Backspace 删除最后一个。Picker(ui.PickerMonth / PickerYear) 即 Element 的 type=\"months\" / \"years\"。", func() {
			entryRow(c, func() {
				ui.MultiDatePicker(c, &dmd.days).A11yLabel("多个日期")
				ui.MultiDatePicker(c, &dmd.months).Picker(ui.PickerMonth).A11yLabel("多个月份")
				ui.MultiDatePicker(c, &dmd.years).Picker(ui.PickerYear).A11yLabel("多个年份")
				ui.MultiDatePicker(c, &dmd.many).MaxTagCount(2).A11yLabel("折叠标签")
			})
		})
		gallery.Demo(c, "多选（展开，周末不可选）", "DisabledDate 同 DatePicker；NeedConfirm(true) 时改动需点“确定”。", func() {
			ui.MultiDatePicker(c, &dmd.daysOpen).DisabledDate(dtWeekend).NeedConfirm(true).Width(320).A11yLabel("值班日期").Open()
		})
	}})
	entryPage(gallery.Page{Name: "DatePickerConfirm", Title: "DatePicker 需要确认", Parent: "DatePicker", Order: 49, Height: 720, Render: func(c *ui.Context) {
		gallery.Demo(c, "NeedConfirm（展开）", "NeedConfirm(true)：选中的日期先作为草稿显示在输入框里，点“确定”或在同一天再按 Enter 才提交；关闭面板则丢弃。", func() {
			ui.DatePicker(c, &dmd.confirm).NeedConfirm(true).Open()
		})
	}, Setup: func(d *ui.Driver) {
		_ = d.ClickLabel("2026-01-16")
	}})
	entryPage(gallery.Page{Name: "DateTimePickerDisabled", Title: "DateTimePicker 禁用日期 / 时间与快捷选项", Parent: "DateTimePicker", Order: 51, Width: 1000, Height: 900, Render: func(c *ui.Context) {
		presets := []ui.DatePreset{
			{Label: "明早", Value: time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)},
			{Label: "下周一", Value: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)},
			{Label: "月底", Value: time.Date(2026, 1, 30, 18, 0, 0, 0, time.UTC)},
		}
		gallery.Demo(c, "基本用法", "DisabledDate(fn)、DisabledTime(func(day) ui.DisabledTimes)、Presets(...)。", func() {
			entryRow(c, func() {
				ui.DateTimePicker(c, &dmd.dtClosed).DisabledDate(dtWeekend).DisabledTime(dmWorkHours).Presets(presets...).Format("2006-01-02 15:04")
			})
		})
		gallery.Demo(c, "展开（周末不可选；9–18 点可选，周五仅上午）", "", func() {
			ui.DateTimePicker(c, &dmd.dtOpen).DisabledDate(dtWeekend).DisabledTime(dmWorkHours).Presets(presets...).Format("2006-01-02 15:04").Open()
		})
	}})
	entryPage(gallery.Page{Name: "RangePickerDisabledTime", Title: "RangePicker 按日期禁用时间", Parent: "RangePicker", Order: 52, Width: 1000, Height: 760, Render: func(c *ui.Context) {
		gallery.Demo(c, "ShowTime + DisabledTime（展开）", "DisabledTime(func(day, end) ui.DisabledTimes)：开始时间不早于 9 点，结束时间不晚于 18 点。", func() {
			ui.RangePicker(c, &dmd.spanOpen).ShowTime().Format("2006-01-02 15:04").DisabledTime(func(day time.Time, end bool) ui.DisabledTimes {
				if end {
					return ui.DisabledTimes{Hours: func(h int) bool { return h > 18 }}
				}
				return ui.DisabledTimes{Hours: func(h int) bool { return h < 9 }}
			}).Open()
		})
	}})
	dataPage(gallery.Page{Name: "CalendarRange", Title: "Calendar 指定日期范围", Parent: "Calendar", Order: 70, Width: 1100, Height: 1300, Render: func(c *ui.Context) {
		gallery.Demo(c, "Range（Element range）", "Range(start, end)：只显示从开始日所在周到结束日所在周，标题显示范围，范围外的日期不可选。", func() {
			ui.Calendar(c, &dmd.calRange).Range(dmDay(1, 4), dmDay(1, 31))
		})
		gallery.Demo(c, "卡片模式", "", func() {
			ui.Column(c, func() {
				ui.Calendar(c, &dmd.calRangeCard).Card().Range(dmDay(1, 7), dmDay(1, 27))
			}, layout.W(320))
		})
	}})
}
