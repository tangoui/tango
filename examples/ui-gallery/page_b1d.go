package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// Spec alignment batch 1, group D: the date / time / color picker options
// added to match antd 6.7 and Element Plus 2.14.8 (MinDate / MaxDate,
// ShowWeek, ShowNow, Placement, Order, NeedConfirm, DefaultTime,
// IncludeEndTime, DisabledFormat). See docs/specs/align/1-datetime.md.

type b1dDemo struct {
	bounded, week, top    time.Time
	order                 [2]time.Time
	clock, dt             time.Time
	shift                 [2]time.Time
	slot                  string
	color                 theme.Color
	boundedRange          [2]time.Time
	monthBounded, noToday time.Time
}

var b1d = &b1dDemo{
	bounded:      time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	week:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	top:          time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	noToday:      time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	monthBounded: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	order:        [2]time.Time{time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)},
	clock:        time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC),
	shift:        [2]time.Time{time.Date(2026, 1, 1, 22, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)},
	slot:         "17:45",
	color:        theme.Hex("#1677ff"),
}

func init() {
	entryPage(gallery.Page{Name: "DatePickerAlign", Title: "DatePicker 可选范围、周数与弹出位置", Parent: "DatePicker", Order: 49, Width: 1000, Height: 860, Render: func(c *ui.Context) {
		gallery.Demo(c, "可选范围（展开）", "MinDate / MaxDate：范围外的日期不可选，翻页箭头停在边界月份。", func() {
			ui.DatePicker(c, &b1d.bounded).MinDate(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)).MaxDate(time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)).ShowWeek(true).Open()
		})
		ui.Gap(c, 360)
		gallery.Demo(c, "其他选项", "ShowNow(false) 去掉“今天”；Picker(PickerMonth).MinDate 按月比较；Placement(ui.TopStart) 向上弹出。", func() {
			entryRow(c, func() {
				ui.DatePicker(c, &b1d.noToday).ShowNow(false)
				ui.DatePicker(c, &b1d.monthBounded).Picker(ui.PickerMonth).MinDate(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))
				ui.DatePicker(c, &b1d.top).Placement(ui.TopStart)
			})
		})
	}})
	entryPage(gallery.Page{Name: "RangePickerAlign", Title: "RangePicker 保持选择顺序与可选范围", Parent: "RangePicker", Order: 52, Width: 1000, Height: 900, Render: func(c *ui.Context) {
		gallery.Demo(c, "Order(false) 与 Separator", "先选的为开始，结束可以早于开始；分隔符自定义。", func() {
			ui.RangePicker(c, &b1d.order).Order(false).Separator("~")
		})
		gallery.Demo(c, "可选范围（展开）", "MinDate / MaxDate 同样作用于两个月面板。", func() {
			ui.RangePicker(c, &b1d.boundedRange).MinDate(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)).MaxDate(time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)).Open()
		})
	}})
	entryPage(gallery.Page{Name: "TimePickerAlign", Title: "TimePicker 免确认、跨夜范围与时间点", Parent: "TimePicker", Order: 50, Width: 1000, Height: 780, Render: func(c *ui.Context) {
		gallery.Demo(c, "NeedConfirm(false)（展开）", "点选即生效，没有确定按钮；ShowNow(false) 去掉“此刻”。", func() {
			ui.TimePicker(c, &b1d.clock).NeedConfirm(false).ShowNow(false).Open()
		})
		ui.Gap(c, 250)
		gallery.Demo(c, "跨夜范围与结束时间", "TimeRangePicker.Order(false) 保留 22:00 – 06:00；DateTimePicker.DefaultTime；TimeSelect.IncludeEndTime 把 17:45 加入选项。", func() {
			entryRow(c, func() {
				ui.TimeRangePicker(c, &b1d.shift).Format("15:04").Order(false)
				ui.DateTimePicker(c, &b1d.dt).DefaultTime(time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC))
				ui.TimeSelect(c, &b1d.slot).Start("09:00").End("17:45").Step("00:30").IncludeEndTime(true)
			})
		})
	}})
	entryPage(gallery.Page{Name: "ColorPickerAlign", Title: "ColorPicker 禁用格式切换", Parent: "ColorPicker", Order: 60, Height: 620, Render: func(c *ui.Context) {
		gallery.Demo(c, "DisabledFormat（展开）", "不显示格式下拉，只按当前格式编辑（Ant disabledFormat）。", func() {
			ui.ColorPicker(c, &b1d.color).ShowText().DisabledFormat(true).Open()
		})
	}})
}
