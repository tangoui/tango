package main

// Spec alignment batch 1, group C (docs/specs/align/1-select.md):
// placement, loading suffix, unknown values, label render, collapsed-tag
// text, InputTag Space trigger, Transfer target order / select-all and
// Cascader separator.

import (
	"strconv"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b1cSel1, b1cSel2, b1cSel3 = "ghost", "sh", ""
	b1cMS1                    = []string{"bj", "sh", "gz", "sz"}
	b1cMS2                    = []string{"bj", "sh"}
	b1cTags                   = []string{"Go", "Rust"}
	b1cCas                    = []string{"zj", "hz", "xh"}
	b1cTarget                 = []string{"3", "1"}
	b1cTarget2                []string
	b1cOpen                   = ""
)

var b1cItems = []ui.TransferItem{
	{Key: "1", Title: "选项一"}, {Key: "2", Title: "选项二"}, {Key: "3", Title: "选项三"}, {Key: "4", Title: "选项四"},
}

func init() {
	gallery.Register(gallery.Page{Name: "SelectAlign", Title: "Select 规范对齐（选择器）", Group: gallery.Form, Order: 3, Parent: "Select",
		Width: 1000, Height: 980,
		Render: func(c *ui.Context) {
			ui.Column(c, func() {
				ui.Text(c, "未知值显示原值 · labelRender · loading").Secondary()
				ui.Row(c, func() {
					ui.Select(c, &b1cSel1, selCities).A11yLabel("未知值").Width(180)
					ui.Select(c, &b1cSel2, selCities).A11yLabel("城市").Width(180).LabelRender(func(o ui.Option) string { return o.Label + "（" + o.Value + "）" })
					ui.Select(c, &b1cSel3, selCities).Width(180).Placeholder("加载中").Loading(true)
				}, layout.Gap(12))
				ui.Text(c, "maxTagPlaceholder · maxTagTextLength").Secondary()
				ui.Row(c, func() {
					ui.MultiSelect(c, &b1cMS1, selCities).A11yLabel("城市").Width(260).MaxTagCount(2).
						MaxTagPlaceholder(func(o []string) string { return "另有 " + strconv.Itoa(len(o)) + " 项" })
					ui.MultiSelect(c, &b1cMS2, []ui.Option{{Label: "北京市朝阳区", Value: "bj"}, {Label: "上海市浦东新区", Value: "sh"}}).A11yLabel("地址").Width(260).MaxTagTextLength(3)
				}, layout.Gap(12))
				ui.Text(c, "InputTag trigger=Space · Cascader separator").Secondary()
				ui.Row(c, func() {
					ui.InputTag(c, &b1cTags).Trigger("Space").Placeholder("空格添加").Width(260)
					ui.Cascader(c, &b1cCas, selRegions).A11yLabel("地区").Separator(" > ").Width(260)
				}, layout.Gap(12))
				ui.Text(c, "Transfer：默认 targetOrder=unshift（右侧按 target 顺序）· showSelectAll=false").Secondary()
				ui.Row(c, func() {
					ui.Transfer(c, b1cItems, &b1cTarget).ListSize(160, 200)
					ui.Transfer(c, b1cItems, &b1cTarget2).ShowSelectAll(false).ListSize(160, 200)
				}, layout.Gap(24))
				ui.Gap(c, 260)
				ui.Row(c, func() {
					ui.Select(c, &b1cOpen, selCities).A11yLabel("向上展开").Width(200).Placement(ui.TopStart).NotFoundContent("没有匹配项").Open()
					ui.Text(c, "placement=topLeft（向上展开）").Secondary()
				}, layout.Gap(12), layout.Middle())
			}, layout.Gap(10), layout.Pad(24))
		}})
}
