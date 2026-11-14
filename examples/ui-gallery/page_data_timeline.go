package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	dataPage(gallery.Page{Name: "Timeline", Title: "Timeline 时间轴", Order: 22, Width: 1000, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			gallery.Demo(c, "基本用法 + 进行中", "Pending 显示最后一个进行中的节点。", func() {
				ui.Timeline(c, []ui.TimelineItem{
					{Content: "Create a services site", Time: "2015-09-01"},
					{Content: "Solve initial network problems", Time: "2015-09-01"},
					{Content: "Technical testing", Time: "2015-09-01"},
				}).Pending("Recording...").Show()
			})
			gallery.Demo(c, "颜色与自定义节点", "Color 设置状态色，DotIcon 自定义节点。", func() {
				ui.Timeline(c, []ui.TimelineItem{
					{Content: "Create a services site 2015-09-01", Color: "success", Large: true},
					{Content: "Solve initial network problems", Color: "error", Hollow: true},
					{Content: "Technical testing 2015-09-01", DotIcon: icon.Clock, Color: "error"},
					{Content: "Network problems being solved", Color: "gray"},
				}).Show()
			})
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
		gallery.Demo(c, "交替展现 / 标签", "Mode(\"alternate\") 内容在轴两侧交替；Label 显示在对侧。", func() {
			ui.Row(c, func() {
				ui.Timeline(c, []ui.TimelineItem{
					{Content: "Create a services site", Label: "2015-09-01"},
					{Content: "Solve initial network problems", Label: "2015-09-01 09:12:11"},
					{Content: "Technical testing", Label: "2015-09-01"},
					{Content: "Network problems being solved", Label: "2015-09-01"},
				}).Show().Layout(layout.Grow(1))
				ui.Timeline(c, []ui.TimelineItem{
					{Content: "Create a services site 2015-09-01"},
					{Content: "Solve initial network problems 2015-09-01", Color: "success"},
					{Content: "Technical testing 2015-09-01", DotIcon: icon.Clock},
					{Content: "Network problems being solved 2015-09-01", Color: "error"},
				}).Mode("alternate").Show().Layout(layout.Grow(1))
			}, layout.Gap(24))
		})
		gallery.Demo(c, "横向时间轴", "Horizontal() 沿横轴排列；Mode 仍可用：left 内容在轴下、right 在轴上、alternate 上下交替，Label 在对侧。", func() {
			ui.Timeline(c, []ui.TimelineItem{
				{Content: "Create a services site", Time: "2015-09-01"},
				{Content: "Solve initial network problems", Time: "2015-09-01", Color: "success"},
				{Content: "Technical testing", Time: "2015-09-01", DotIcon: icon.Clock},
				{Content: "Network problems being solved", Time: "2015-09-01", Color: "error"},
			}).Horizontal().Pending("Recording...").Show()
			ui.Gap(c, 24)
			ui.Timeline(c, []ui.TimelineItem{
				{Content: "Create a services site", Label: "2015-09-01"},
				{Content: "Solve initial network problems", Label: "2015-09-01"},
				{Content: "Technical testing", Label: "2015-09-01"},
				{Content: "Network problems being solved", Label: "2015-09-01"},
			}).Horizontal().Show()
			ui.Gap(c, 24)
			ui.Timeline(c, []ui.TimelineItem{
				{Content: "Create a services site"},
				{Content: "Solve initial network problems", Color: "success"},
				{Content: "Technical testing", Color: "warning"},
				{Content: "Network problems being solved", Color: "error"},
				{Content: "Released"},
			}).Horizontal().Mode("alternate").Show()
		})
	}})
}
