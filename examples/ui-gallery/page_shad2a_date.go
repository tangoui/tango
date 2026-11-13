package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Date / time picker additions: range granularity, typed input, variants,
// hideDisabledOptions; and the dark variants missing from the date panels
// and TimeSelect (docs/specs/ui-coverage.md §5).

var shad2aDateDemo struct {
	months, quarters, years, weeks [2]time.Time
	open                           [2]time.Time
	day                            time.Time
	clock                          time.Time
	span                           [2]time.Time
	filled, borderless, underlined time.Time
	slot                           string
}

func shad2aDateReset() {
	d := &shad2aDateDemo
	utc := func(y, m, dd int) time.Time { return time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC) }
	d.months = [2]time.Time{utc(2026, 2, 1), utc(2026, 6, 1)}
	d.quarters = [2]time.Time{utc(2026, 1, 1), utc(2026, 7, 1)}
	d.years = [2]time.Time{utc(2024, 1, 1), utc(2027, 1, 1)}
	d.weeks = [2]time.Time{utc(2026, 1, 5), utc(2026, 1, 26)}
	d.open = [2]time.Time{utc(2026, 2, 1), utc(2026, 4, 1)}
	d.day = utc(2026, 1, 15)
	d.clock = time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	d.span = [2]time.Time{}
	d.filled, d.borderless, d.underlined = utc(2026, 1, 15), utc(2026, 1, 15), utc(2026, 1, 15)
	d.slot = "10:30"
}

func init() {
	shad2aDateReset()
	entryPage(gallery.Page{Name: "RangePickerPicker", Title: "RangePicker 粒度、键盘输入与变体", Parent: "RangePicker", Order: 52, Width: 1000, Height: 1440,
		Render: func(c *ui.Context) {
			d := &shad2aDateDemo
			gallery.Demo(c, "范围粒度", "Picker(PickerWeek / PickerMonth / PickerQuarter / PickerYear)：两端都是周期的第一天。", func() {
				entryRow(c, func() {
					ui.RangePicker(c, &d.weeks).Picker(ui.PickerWeek).Width(300)
					ui.RangePicker(c, &d.months).Picker(ui.PickerMonth)
					ui.RangePicker(c, &d.quarters).Picker(ui.PickerQuarter)
					ui.RangePicker(c, &d.years).Picker(ui.PickerYear)
				})
			})
			gallery.Demo(c, "键盘输入", "聚焦后直接打字（如 2026-03-09），面板跟随合法日期；Enter 或离开输入框时提交，非法或禁用的值不提交。", func() {
				entryRow(c, func() {
					ui.DatePicker(c, &d.day)
					ui.TimeRangePicker(c, &d.span).Format("15:04")
				})
			})
			gallery.Demo(c, "变体与隐藏禁用项", "Variant(\"filled\" / \"borderless\" / \"underlined\")；HideDisabledOptions(true) 不显示 8 点前与 18 点后的小时。", func() {
				entryRow(c, func() {
					ui.DatePicker(c, &d.filled).Variant("filled")
					ui.DatePicker(c, &d.borderless).Variant("borderless")
					ui.DatePicker(c, &d.underlined).Variant("underlined")
					ui.TimeSelect(c, &d.slot).Variant("filled").Width(160)
				})
				ui.TimePanel(c, &d.clock).Format("15:04").Height(180).
					DisabledHours(func(h int) bool { return h < 8 || h > 18 }).HideDisabledOptions(true)
			})
			gallery.Demo(c, "展开：月份范围", "已选起始月份 2026-02，第二次点击确定结束月份。", func() {
				ui.Row(c, func() {
					ui.RangePicker(c, &d.open).Picker(ui.PickerMonth).Open()
				}, layout.Margin(geom.Insets{Bottom: 300}))
			})
		},
		Setup: func(d *ui.Driver) { shad2aDateReset(); d.Settle() },
	})
	// Dark variants of pages registered elsewhere (fill_panels, fill_misc,
	// more_timeselect sort before this file, so their pages exist here).
	for _, name := range []string{"DatePickerPanel", "TimePanel", "TimeSelect"} {
		p, ok := gallery.Find(name)
		if !ok {
			continue
		}
		run := p.Interact
		if run == nil {
			run = p.Setup
		}
		dk := p
		dk.Variant, dk.Interact = true, nil
		dk.Name, dk.Title = p.Name+"-Dark", p.Title+"（暗色）"
		dk.Setup = func(d *ui.Driver) {
			d.SetTheme(theme.AntDesign(theme.Dark()))
			d.Settle()
			if run != nil {
				run(d)
			}
		}
		gallery.Register(dk)
		ed := dk
		ed.Name, ed.Title = p.Name+"-Element-Dark", p.Title+"（Element Plus 暗色）"
		ed.Setup = func(d *ui.Driver) {
			d.SetTheme(theme.ElementPlus(theme.Dark()))
			d.Settle()
			if run != nil {
				run(d)
			}
		}
		gallery.Register(ed)
	}
}
