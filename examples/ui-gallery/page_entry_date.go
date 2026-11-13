package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

type dtDemo struct {
	d1, d2, dOpen              time.Time
	t1, t2, t3, t4, tOpen      time.Time
	dt1, dt2, dtOpen           time.Time
	r1, r2, r3, rOpen, rPreset [2]time.Time
}

var dtd = &dtDemo{
	d1:     time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	dOpen:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	t1:     time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
	t2:     time.Date(2026, 1, 15, 14, 45, 0, 0, time.UTC),
	t4:     time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC),
	tOpen:  time.Date(2026, 1, 15, 10, 25, 36, 0, time.UTC),
	dt1:    time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
	dtOpen: time.Date(2026, 1, 15, 14, 20, 0, 0, time.UTC),
	r1:     [2]time.Time{time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)},
	rOpen:  [2]time.Time{time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 22, 0, 0, 0, 0, time.UTC)},
}

func dtPresets() []ui.DatePreset {
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, time.UTC) }
	return []ui.DatePreset{
		{Label: "最近 7 天", Range: [2]time.Time{d(1, 25), d(1, 31)}},
		{Label: "最近 30 天", Range: [2]time.Time{d(1, 2), d(1, 31)}},
		{Label: "本月", Range: [2]time.Time{d(1, 1), d(1, 31)}},
		{Label: "上个月", Range: [2]time.Time{d(12, 1).AddDate(-1, 0, 0), d(12, 31).AddDate(-1, 0, 0)}},
	}
}

// dtWeekend disables Saturdays and Sundays.
func dtWeekend(d time.Time) bool { return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday }

func init() {
	entryPage(gallery.Page{Name: "DatePicker", Title: "DatePicker 日期选择框", Order: 49, Height: 900, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本用法", "点击或 Enter 打开；方向键移动、PageUp/PageDown 翻月、Enter 选中。", func() {
			entryRow(c, func() {
				ui.DatePicker(c, &dtd.d1)
				ui.DatePicker(c, &dtd.d2)
				ui.DatePicker(c, &dtd.d1).Format("2006年01月02日")
				ui.DatePicker(c, &dtd.d1).Disabled(true)
			})
		})
		gallery.Demo(c, "禁用日期（周末不可选）", "DisabledDate(func(time.Time) bool)；展开状态。", func() {
			ui.DatePicker(c, &dtd.dOpen).DisabledDate(dtWeekend).Open()
		})
	}})
	entryPage(gallery.Page{Name: "TimePicker", Title: "TimePicker 时间选择框", Order: 50, Height: 900, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本用法", "时 / 分 / 秒滚动列；支持直接输入时间、步长、清除。", func() {
			entryRow(c, func() {
				ui.TimePicker(c, &dtd.t1)
				ui.TimePicker(c, &dtd.t2).Format("15:04").MinuteStep(15)
				ui.TimePicker(c, &dtd.t3).Placeholder("请选择时间")
			})
		})
		gallery.Demo(c, "尺寸与状态", "", func() {
			entryRow(c, func() {
				ui.TimePicker(c, &dtd.t1).Size(ui.Small)
				ui.TimePicker(c, &dtd.t1).Size(ui.Large)
				ui.TimePicker(c, &dtd.t3).Status("error").Placeholder("错误状态")
				ui.TimePicker(c, &dtd.t3).Status("warning").Placeholder("警告状态")
				ui.TimePicker(c, &dtd.t4).Disabled(true)
			})
		})
		gallery.Demo(c, "展开状态", "", func() {
			ui.TimePicker(c, &dtd.tOpen).Open()
		})
	}})
	entryPage(gallery.Page{Name: "DateTimePicker", Title: "DateTimePicker 日期时间选择", Order: 51, Height: 820, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本用法", "左侧日期面板，右侧时间列；“确定”提交。", func() {
			entryRow(c, func() {
				ui.DateTimePicker(c, &dtd.dt1)
				ui.DateTimePicker(c, &dtd.dt2).Format("2006-01-02 15:04")
				ui.DateTimePicker(c, &dtd.dt2).Status("error")
				ui.DateTimePicker(c, &dtd.dt1).Disabled(true)
			})
		})
		gallery.Demo(c, "展开状态", "", func() {
			ui.DateTimePicker(c, &dtd.dtOpen).Open()
		})
	}})
	entryPage(gallery.Page{Name: "RangePicker", Title: "RangePicker 日期范围选择", Order: 52, Width: 1000, Height: 900, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本用法", "两个相邻月份面板；第一次点击选开始，悬停预览范围，第二次点击完成。", func() {
			entryRow(c, func() {
				ui.RangePicker(c, &dtd.r1)
				ui.RangePicker(c, &dtd.r2)
			})
		})
		gallery.Demo(c, "状态", "", func() {
			entryRow(c, func() {
				ui.RangePicker(c, &dtd.r3).Status("error")
				ui.RangePicker(c, &dtd.r1).Disabled(true)
			})
		})
		gallery.Demo(c, "展开状态（带预设范围）", "已点选开始日期 1 月 12 日，悬停 1 月 27 日预览范围。", func() {
			ui.RangePicker(c, &dtd.rOpen).Presets(dtPresets()...).Open()
		})
	}, Setup: func(d *ui.Driver) {
		// First click picks the start; hovering another day previews the range.
		_ = d.ClickLabel("2026-01-12")
		if r, ok := d.Find("2026-01-27"); ok {
			d.Move(r.Center())
		}
	}})
}
