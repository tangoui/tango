package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Batch 4 layout primitives are deliberately thin public entries backed
// by the same Breadcrumb, Spin and ProLayout header implementations.
func init() {
	gallery.Register(gallery.Page{Name: "ProBreadcrumb", Title: "ProBreadcrumb 路由面包屑", Parent: "ProLayout", Group: gallery.Layout, Order: 44, Width: 1000, Render: func(c *ui.Context) {
		selected := "details"
		ui.ProLayout(c, ui.ProLayoutConfig{Title: "Workspace", Layout: "top", Menu: []ui.MenuItem{{Key: "projects", Label: "Projects", Children: []ui.MenuItem{{Key: "details", Label: "Details"}}}}, Selected: &selected, AutoBreadcrumb: true, UserName: "Lily"}, func() {
			ui.ProCard(c, "Route-aware breadcrumb").Body(func() {
				ui.ProBreadcrumb(c).Show()
				ui.Gap(c, 16)
				ui.Text(c, "The public breadcrumb reads the enclosing RouteContext.").Secondary()
			})
		})
	}})
	gallery.Register(gallery.Page{Name: "PageLoading", Title: "PageLoading 页面加载", Parent: "PageContainer", Group: gallery.Layout, Order: 45, Width: 800, Render: func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Text(c, "Large Spin with page-level spacing").Strong()
			ui.PageLoading(c).Description("Loading page").Show()
		}, layout.H(280))
		ui.Divider(c)
		ui.Text(c, "Spinning(false): no loading indicator").Secondary()
		ui.PageLoading(c).Spinning(false).Show()
	}})
	gallery.Register(gallery.Page{Name: "TopNavHeader", Title: "TopNavHeader 顶部导航", Parent: "ProLayout", Group: gallery.Layout, Order: 46, Width: 1000, Render: func(c *ui.Context) {
		selected := "home"
		menu := []ui.MenuItem{{Key: "home", Label: "Home"}, {Key: "projects", Label: "Projects"}, {Key: "reports", Label: "Reports"}, {Key: "settings", Label: "Settings"}}
		ui.TopNavHeader(c, menu, &selected).Title("Workspace").UserName("Lily").Actions(func() { ui.IconButton(c, icon.Search, "Search").Size(32, 16) }).Show()
		ui.Gap(c, 24)
		ui.Text(c, "Narrow width: horizontal menu overflow remains keyboard accessible.").Secondary()
		ui.Column(c, func() { ui.TopNavHeader(c, menu, &selected).Title("Team").UserName("Lily").Show() }, layout.W(480))
	}})
}
