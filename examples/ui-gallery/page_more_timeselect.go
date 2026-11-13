package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var moreTimeDemo struct {
	a, start, end, dis string
	openEnd            bool
}

func moreTimeReset() {
	moreTimeDemo.a, moreTimeDemo.start, moreTimeDemo.end, moreTimeDemo.dis = "", "", "", "10:00"
	moreTimeDemo.openEnd = false
}

func init() {
	moreTimeReset()
	morePage(gallery.Page{Name: "TimeSelect", Title: "TimeSelect 时间选择", Group: gallery.Form, Order: 45, Height: 820,
		Render: func(c *ui.Context) {
			s := &moreTimeDemo
			gallery.Demo(c, "固定时间点", "Start / End / Step 生成选项（08:30–18:30，每 15 分钟），值为 \"HH:MM\"。", func() {
				ui.TimeSelect(c, &s.a).Start("08:30").End("18:30").Step("00:15").Placeholder("选择时间")
			})
			gallery.Demo(c, "时间范围", "MinTime / MaxTime 禁用范围外的时间：结束时间必须晚于开始时间。", func() {
				ui.Row(c, func() {
					ui.TimeSelect(c, &s.start).Start("08:30").End("18:30").Step("00:15").MaxTime(s.end).Placeholder("开始时间").Width(200)
					end := ui.TimeSelect(c, &s.end).Start("08:30").End("18:30").Step("00:15").MinTime(s.start).Placeholder("结束时间").Width(200)
					if s.openEnd {
						end.Open()
						s.openEnd = false
					}
				}, layout.Gap(16))
			})
			gallery.Demo(c, "禁用", "", func() {
				ui.TimeSelect(c, &s.dis).Disabled(true)
			})
		},
		Setup: func(d *ui.Driver) {
			s := &moreTimeDemo
			s.a = "09:45"
			s.start = "10:00"
			s.openEnd = true
			d.Settle()
		},
	}, moreTimeReset)
}
