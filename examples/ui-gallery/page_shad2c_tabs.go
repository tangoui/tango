package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

// Tabs: vertical overflow, border card, extra content on both sides and
// the card / closable combinations (all three styles, light and dark).

var shad2cTabs struct {
	vl, vr, bc, bcl, ex, card, edit string
}

func shad2cTabsReset() {
	shad2cTabs.vl, shad2cTabs.vr, shad2cTabs.bc, shad2cTabs.bcl = "v9", "v2", "a", "b"
	shad2cTabs.ex, shad2cTabs.card, shad2cTabs.edit = "a", "b", "a"
}

func shad2cTabsPage(c *ui.Context) {
	var many []ui.TabItem
	for i := 1; i <= 12; i++ {
		many = append(many, ui.TabItem{Key: fmt.Sprintf("v%d", i), Label: fmt.Sprintf("选项 %d", i)})
	}
	items := []ui.TabItem{{Key: "a", Label: "用户管理", Icon: icon.User}, {Key: "b", Label: "配置管理"}, {Key: "c", Label: "角色管理"}, {Key: "d", Label: "定时任务", Disabled: true}}
	navDemo(c, "竖向溢出 Height + 「更多」", "Left / Right 固定高度后，放不下的标签可滚动，… 下拉列出被遮住的标签。", func() {
		navCols(c, func() {
			tb := ui.Tabs(c, &shad2cTabs.vl, many).Placement(ui.Left).Height(220).Show()
			tb.Panel(func() { ui.Text(c, "左侧内容 "+shad2cTabs.vl) })
		}, func() {
			tb := ui.Tabs(c, &shad2cTabs.vr, many).Placement(ui.Right).Card().Height(220).Show()
			tb.Panel(func() { ui.Text(c, "右侧卡片内容 "+shad2cTabs.vr) })
		})
	})
	navDemo(c, "边框卡片 BorderCard", "Element type=\"border-card\"：标签栏与内容在同一个带边框的卡片里。", func() {
		navCols(c, func() {
			tb := ui.Tabs(c, &shad2cTabs.bc, items).BorderCard().Show()
			tb.Panel(func() { ui.Text(c, "边框卡片内容 "+shad2cTabs.bc) })
		}, func() {
			tb := ui.Tabs(c, &shad2cTabs.bcl, items[:3]).BorderCard().Placement(ui.Left).Show()
			tb.Panel(func() { ui.Text(c, "左侧边框卡片 "+shad2cTabs.bcl) })
		})
	})
	navDemo(c, "两侧附加内容 ExtraLeft / Extra", "Ant tabBarExtraContent 的 left / right。", func() {
		ui.Tabs(c, &shad2cTabs.ex, items[:3]).
			ExtraLeft(func() { ui.Button(c, "左侧").Small() }).
			Extra(func() { ui.Button(c, "右侧").Small() }).Show()
	})
	navDemo(c, "卡片 Card 与可编辑 Closable + OnAdd", "", func() {
		ui.Tabs(c, &shad2cTabs.card, items[:3]).Card().Show()
		ui.Tabs(c, &shad2cTabs.edit, items[:3]).Closable().OnAdd(nil).Show()
	})
}

func init() {
	shad2cTabsReset()
	navPage(gallery.Page{Name: "TabsLayout", Title: "Tabs 竖向溢出、边框卡片与附加内容", Group: gallery.Navigation, Order: 2, Parent: "Tabs", Render: shad2cTabsPage}, shad2cTabsReset)
}
