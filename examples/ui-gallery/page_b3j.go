package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Spec alignment batch 3, group J: the Pagination, Steps, Splitter,
// Breadcrumb and PageHeader options added to match antd 6.7, Pro
// Components v3, Element Plus 2.14.8 and shadcn/ui v4
// (docs/specs/align/3-nav.md).

var b3j = struct {
	p1, p2, p3, p4, size int
	steps, stepsMax      int
}{p1: 10, p2: 3, p3: 1, p4: 6, size: 10, steps: 1, stepsMax: 15}

func b3jSteps(n int) []ui.StepItem {
	out := make([]ui.StepItem, n)
	for i := range out {
		out[i] = ui.StepItem{Title: "步骤 " + string(rune('A'+i)), Content: "说明文字"}
	}
	return out
}

func init() {
	navPage(gallery.Page{Name: "PaginationAlign", Title: "Pagination 大号、对齐、精简页码与确认按钮", Group: gallery.Navigation, Order: 4, Parent: "Pagination", Width: 1000, Height: 820, Render: func(c *ui.Context) {
		gallery.Demo(c, "Size(Large) 与 Align(\"center\")", "大号按钮 40px（antd itemSizeLG），整行居中。", func() {
			ui.Pagination(c, &b3j.p1, 500, 10).Size(ui.Large).Align("center").Show()
		})
		gallery.Demo(c, "ShowLessItems", "当前页两侧各 1 页，••• 每次跳 3 页（antd showLessItems）。", func() {
			ui.Pagination(c, &b3j.p1, 500, 10).ShowLessItems().Show()
		})
		gallery.Demo(c, "GoButton、ShowTotalFunc、PageSizeOptions", "跳页框后带确认按钮；总数文字取当前页范围；自定义每页条数。", func() {
			ui.Pagination(c, &b3j.p2, 85, b3j.size).GoButton("").SizeChanger(&b3j.size).PageSizeOptions(5, 10, 25).
				ShowTotalFunc(func(total, from, to int) string { return fmt.Sprintf("第 %d-%d 条，共 %d 条", from, to, total) }).Show()
		})
		gallery.Demo(c, "PrevText / NextText 与 total 为 0", "Element prev-text / next-text；total 0 时只有禁用的第 1 页。", func() {
			ui.Pagination(c, &b3j.p4, 200, 10).PrevText("上一页").NextText("下一页").Show()
			ui.Gap(c, 8)
			ui.Pagination(c, &b3j.p3, 0, 10).Show()
		})
	}}, func() { b3j.p1, b3j.p2, b3j.p3, b3j.p4, b3j.size = 10, 3, 1, 6, 10 })

	navPage(gallery.Page{Name: "StepsAlign", Title: "Steps 描边变体、起始序号与最大显示数", Group: gallery.Navigation, Order: 21, Parent: "Steps", Width: 1000, Height: 700, Render: func(c *ui.Context) {
		gallery.Demo(c, "Variant(\"outlined\")", "antd 6 variant：图标为状态色描边。", func() {
			ui.Steps(c, b3jSteps(3)).Variant("outlined").Clickable(&b3j.steps).Show()
		})
		gallery.Demo(c, "Initial(10) + MaxCount(5)", "序号从 11 开始；10 步只显示首、尾、当前和相邻步骤，其余折成省略步骤。", func() {
			ui.Steps(c, b3jSteps(10)).Initial(10).MaxCount(5).Clickable(&b3j.stepsMax).Show()
		})
		gallery.Demo(c, "Type(\"dot\") + TitlePlacement", "v6 命名：点状步骤，标题在下方。", func() {
			ui.Steps(c, b3jSteps(4)).Type("dot").Current(2).Show()
		})
	}}, func() { b3j.steps, b3j.stepsMax = 1, 15 })

	navPage(gallery.Page{Name: "SplitterAlign", Title: "Splitter 方向、百分比限制与把手", Group: gallery.Layout, Order: 12, Parent: "Splitter", Width: 1000, Height: 640, Render: func(c *ui.Context) {
		gallery.Demo(c, "WithHandle（shadcn Resizable withHandle）", "分隔条上带把手；左侧面板最大 70%。", func() {
			ui.Splitter(c).WithHandle().Style(layout.H(140)).Body(func() {
				ui.SplitterPanel(c).MaxPercent(0.7).Body(func() { ui.Text(c, "左侧") })
				ui.SplitterPanel(c).Body(func() { ui.Text(c, "右侧") })
			}).Border(1, c.Theme.Border).Radius(c.Theme.Radius)
		})
		gallery.Demo(c, "Orientation(\"vertical\")", "antd 6 orientation；上方面板最小 20%。", func() {
			ui.Splitter(c).Orientation("vertical").Style(layout.H(180)).Body(func() {
				ui.SplitterPanel(c).MinPercent(0.2).Collapsible().Body(func() { ui.Text(c, "上方") })
				ui.SplitterPanel(c).Body(func() { ui.Text(c, "下方") })
			}).Border(1, c.Theme.Border).Radius(c.Theme.Radius)
		})
	}}, nil)

	navPage(gallery.Page{Name: "BreadcrumbAlign", Title: "Breadcrumb 单项分隔符与下拉图标", Group: gallery.Navigation, Order: 3, Parent: "Breadcrumb", Width: 1000, Height: 220, Render: func(c *ui.Context) {
		gallery.Demo(c, "Item.Separator + DropdownIcon", "第一项后用“:”分隔（antd type separator）；下拉项换成实心箭头。", func() {
			ui.BreadcrumbItems(c, []ui.BreadcrumbItem{
				{Label: "位置", Separator: ":"},
				{Label: "应用中心", Menu: []ui.MenuItem{{Key: "a", Label: "应用 A"}, {Key: "b", Label: "应用 B"}}},
				{Label: "应用列表"},
				{Label: "某应用"},
			}).DropdownIcon(icon.CaretDown).Show()
		})
	}}, nil)

	navPage(gallery.Page{Name: "PageHeaderAlign", Title: "PageHeader 头像、返回图标与结构化面包屑", Group: gallery.Navigation, Order: 22, Parent: "PageHeader", Width: 1000, Height: 320, Render: func(c *ui.Context) {
		gallery.Demo(c, "Avatar + BackIcon + BreadcrumbItems", "Pro v3 PageHeader：返回按钮、头像、标题、副标题与标签。", func() {
			ui.PageHeader(c, "订单详情").SubTitle("#20261115").BackIcon(icon.Left).
				Avatar(func() { ui.Avatar(c, "订") }).
				BreadcrumbItems([]ui.BreadcrumbItem{{Label: "首页"}, {Label: "订单"}, {Label: "详情"}}).
				Tags(func() { ui.Tag(c, "处理中").Color("processing") }).
				Extra(func() { ui.Button(c, "编辑"); ui.Button(c, "提交").Primary() }).Bordered().Show()
		})
	}}, nil)
}
