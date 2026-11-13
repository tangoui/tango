package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

var (
	listPage  = 1
	listCount = 3
	listNames = []string{"Ant Design Title 1", "Ant Design Title 2", "Ant Design Title 3", "Ant Design Title 4", "Ant Design Title 5", "Ant Design Title 6", "Ant Design Title 7", "Ant Design Title 8"}
	listColor = []string{"#f56a00", "#7265e6", "#ffbf00", "#00a2ae", "#87d068", "#1677ff", "#eb2f96", "#fa8c16"}
)

const listDesc = "Ant Design, a design language for background applications, is refined by Ant UED Team"

func init() {
	dataPage(gallery.Page{Name: "List", Title: "List 列表", Order: 23, Render: func(c *ui.Context) {
		gallery.Demo(c, "头像元信息 + 操作 + 分页", "ListItemMeta、ListActions 与 Pagination。", func() {
			ui.List(c, len(listNames)).Pagination(&listPage, 3).Item(func(i int) {
				ui.ListItemMeta(c, func() { ui.Avatar(c, listNames[i][:1]).Bg(theme.Hex(listColor[i])) }, listNames[i], listDesc)
				ui.ListActions(c, func() { ui.Link(c, "edit"); ui.Link(c, "more") })
			})
		})
		gallery.Demo(c, "加载更多", "LoadMore 在列表底部显示按钮。", func() {
			l := ui.List(c, listCount).LoadMore("loading more", false).Item(func(i int) {
				ui.ListItemMeta(c, func() { ui.Avatar(c, "U") }, listNames[i], listDesc)
				ui.Text(c, "content").Tertiary()
			})
			if l.LoadMoreClicked() {
				listCount = min(len(listNames), listCount+2)
			}
		})
		gallery.Demo(c, "带边框 / 页头页脚", "Bordered 与 Header/Footer。", func() {
			ui.List(c, 3).Bordered().Header(func() { ui.Text(c, "Header") }).Footer(func() { ui.Text(c, "Footer") }).Item(func(i int) {
				ui.Text(c, fmt.Sprintf("Racing car sprays burning fuel into crowd. (%d)", i+1))
			})
		})
	}})
}
