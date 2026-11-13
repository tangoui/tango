package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

const collText = "A dog is a type of domesticated animal. Known for its loyalty and faithfulness, it can be found as a welcome guest in many households across the world."

var (
	collOpen1 = []string{"1"}
	collOpen2 = []string{"2"}
	collOpen3 = []string{"1"}
)

func collItems(c *ui.Context, extra bool) []ui.CollapseItem {
	body := func() { ui.Text(c, collText) }
	items := []ui.CollapseItem{
		{Key: "1", Label: "This is panel header 1", Body: body},
		{Key: "2", Label: "This is panel header 2", Body: body},
		{Key: "3", Label: "This is panel header 3", Body: body, Disabled: true},
	}
	if extra {
		items[0].Extra = func() { ui.Icon(c, icon.Setting).Size(14) }
		items[1].Extra = func() { ui.Icon(c, icon.Setting).Size(14) }
	}
	return items
}

func init() {
	dataPage(gallery.Page{Name: "Collapse", Title: "Collapse 折叠面板", Order: 21, Render: func(c *ui.Context) {
		gallery.Demo(c, "折叠面板", "可以同时展开多个面板；第三个面板禁用。", func() {
			ui.Collapse(c, &collOpen1, collItems(c, false)).Show()
		})
		gallery.Demo(c, "手风琴 + 额外节点", "Accordion 每次只展开一个；Extra 在标题右侧放置操作。", func() {
			ui.Collapse(c, &collOpen2, collItems(c, true)).Accordion().Show()
		})
		gallery.Demo(c, "无边框 / 小尺寸", "Borderless 与 Size(Small)。", func() {
			ui.Collapse(c, &collOpen3, collItems(c, false)).Borderless().Size(ui.Small).Show()
		})
	}})
}
