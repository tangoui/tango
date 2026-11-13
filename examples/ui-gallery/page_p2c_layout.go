package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// Extra sections of the layout components: ProLayout top / mix layouts
// with a PageContainer filling the content area, and the collapsible
// Sider with its trigger.

var p2cLay = struct {
	sel, sel2, tab string
	collapsed      bool
	mixCollapsed   bool
}{}

func p2cLayReset() {
	p2cLay.sel, p2cLay.sel2, p2cLay.tab = "list", "list", "all"
	p2cLay.collapsed, p2cLay.mixCollapsed = false, false
}

func init() {
	p2cLayReset()
	navPage(gallery.Page{Name: "ProLayoutTop", Title: "ProLayout 顶部导航与混合布局", Group: gallery.Layout, Order: 30, Parent: "Layout", Render: p2cProLayoutPage}, p2cLayReset)
	navPage(gallery.Page{Name: "SiderTrigger", Title: "Sider 折叠触发器与断点", Group: gallery.Layout, Order: 31, Parent: "Layout", Render: p2cSiderPage}, p2cLayReset)
}

func p2cPage(c *ui.Context) {
	ui.PageContainer(c, "查询表格").Breadcrumb("首页", "列表页", "查询表格").SubTitle("NoPadding 铺满内容区").
		Tabs(&p2cLay.tab, []ui.TabItem{{Key: "all", Label: "全部"}, {Key: "on", Label: "进行中"}, {Key: "done", Label: "已完成"}}).
		Extra(func() { ui.Button(c, "新建").Primary() }).
		Body(func() {
			ui.Card(c, "规则列表").Body(func() { ui.Text(c, "页头背景横跨整个内容区，内容在页内留白。").Tertiary() })
		})
}

func p2cProLayoutPage(c *ui.Context) {
	frame := func(fn func()) {
		ui.Column(c, fn, layout.H(360)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.Radius).Clip()
	}
	navDemo(c, "顶部导航 Layout: \"top\"", "Logo、水平菜单与用户信息在顶栏；PageContainer 放入后内容区自动 NoPadding。", func() {
		frame(func() {
			ui.ProLayout(c, ui.ProLayoutConfig{Title: "Tango Pro", Menu: proMenu[:5], Selected: &p2cLay.sel, UserName: "曲丽丽", Layout: "top"}, func() { p2cPage(c) })
		})
	})
	navDemo(c, "混合布局 Layout: \"mix\"", "全宽顶栏 + 侧栏菜单，侧栏底部为折叠触发器；Breakpoint 可按窗口宽度自动折叠。", func() {
		frame(func() {
			ui.ProLayout(c, ui.ProLayoutConfig{Title: "Tango Pro", Menu: proMenu[:5], Selected: &p2cLay.sel2, Collapsed: &p2cLay.mixCollapsed,
				UserName: "曲丽丽", Layout: "mix", Breakpoint: ui.MD}, func() { p2cPage(c) })
		})
	})
}

func p2cSiderPage(c *ui.Context) {
	navDemo(c, "CollapsibleSider", "底部触发器切换折叠；Breakpoint(ui.LG) 在窄窗口自动折叠；Light() 亮色。", func() {
		ui.Column(c, func() {
			ui.Layout(c, func() {
				ui.CollapsibleSider(c, 200, &p2cLay.collapsed).Breakpoint(ui.SM).Body(func() {
					ui.Menu(c, proMenu[:5], &p2cLay.sel).Dark().Collapsed(p2cLay.collapsed).Show()
				})
				ui.Content(c, func() {
					ui.Column(c, func() { ui.Text(c, "Content") }, layout.Grow(1), layout.Center()).Bg(c.Theme.BgContainer).Radius(c.Theme.RadiusLG)
				}, layout.Pad(24))
			})
		}, layout.H(320)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.Radius).Clip()
	})
}
