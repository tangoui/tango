package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var navState = struct {
	tab, tab2   string
	page, page2 int
	size        int
}{tab: "1", tab2: "b", page: 6, page2: 1, size: 10}

func init() {
	gallery.Register(gallery.Page{Name: "Tabs", Title: "Tabs 标签页", Group: gallery.Navigation, Order: 2, Render: func(c *ui.Context) {
		ui.Tabs(c, &navState.tab, []ui.TabItem{{Key: "1", Label: "基本信息"}, {Key: "2", Label: "安全设置", Icon: icon.Lock}, {Key: "3", Label: "消息通知"}, {Key: "4", Label: "禁用", Disabled: true}}).Show()
		ui.Text(c, "标签页内容 "+navState.tab).Layout(layout.Margin(geomBottom(24)))
		ui.Tabs(c, &navState.tab2, []ui.TabItem{{Key: "a", Label: "卡片一"}, {Key: "b", Label: "卡片二"}, {Key: "c", Label: "卡片三"}}).Card().Show()
	}})
	gallery.Register(gallery.Page{Name: "Breadcrumb", Title: "Breadcrumb 面包屑", Group: gallery.Navigation, Order: 3, Render: func(c *ui.Context) {
		ui.Breadcrumb(c, "首页", "应用中心", "应用列表", "某应用")
	}})
	gallery.Register(gallery.Page{Name: "Pagination", Title: "Pagination 分页", Group: gallery.Navigation, Order: 4, Height: 300, Render: func(c *ui.Context) {
		ui.Pagination(c, &navState.page, 500, 10).Show()
		ui.Column(c, func() {}, layout.H(16))
		ui.Pagination(c, &navState.page2, 85, navState.size).ShowTotal().SizeChanger(&navState.size).Show()
		ui.Column(c, func() {}, layout.H(16))
		ui.Pagination(c, &navState.page2, 50, 10).Small().Show()
	}})
}
