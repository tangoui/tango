package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Extra sections of the navigation components: editable / placed /
// overflowing Tabs, Pagination options, structured Breadcrumb, Menu
// groups and overflow, navigation Steps.

var p2cNav = struct {
	edit, left, right, bottom, center, many string
	editItems                               []ui.TabItem
	added                                   int
	page, simple, bg, dis                   int
	crumbMenu                               string
	menuSel, menuH                          string
	step                                    int
}{}

func p2cNavReset() {
	p2cNav.edit, p2cNav.left, p2cNav.right, p2cNav.bottom, p2cNav.center, p2cNav.many = "1", "a", "a", "a", "a", "t3"
	p2cNav.editItems = []ui.TabItem{{Key: "1", Label: "标签一"}, {Key: "2", Label: "标签二"}, {Key: "3", Label: "固定标签", NoClose: true}}
	p2cNav.added = 0
	p2cNav.page, p2cNav.simple, p2cNav.bg, p2cNav.dis = 6, 3, 2, 2
	p2cNav.crumbMenu = ""
	p2cNav.menuSel, p2cNav.menuH = "o2", "h1"
	p2cNav.step = 1
}

func init() {
	p2cNavReset()
	navPage(gallery.Page{Name: "TabsMore", Title: "Tabs 可编辑、位置与溢出", Group: gallery.Navigation, Order: 2, Parent: "Tabs", Render: p2cTabsPage}, p2cNavReset)
	navPage(gallery.Page{Name: "PaginationMore", Title: "Pagination 跳页、简洁与背景", Group: gallery.Navigation, Order: 4, Parent: "Pagination", Render: p2cPaginationPage}, p2cNavReset)
	navPage(gallery.Page{Name: "BreadcrumbMore", Title: "Breadcrumb 结构化项", Group: gallery.Navigation, Order: 3, Parent: "Breadcrumb", Render: p2cBreadcrumbPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("应用中心"); ok {
				d.Move(r.Center())
				d.Advance(400 * time.Millisecond)
				d.Settle()
			}
		}}, p2cNavReset)
	navPage(gallery.Page{Name: "MenuMore", Title: "Menu 分组与溢出折叠", Group: gallery.Navigation, Order: 6, Parent: "Menu", Render: p2cMenuPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find(ui.ZhCN.More); ok {
				d.Move(r.Center())
				d.Advance(400 * time.Millisecond)
				d.Settle()
				d.Settle()
			}
		}}, p2cNavReset)
	navPage(gallery.Page{Name: "StepsMore", Title: "Steps 导航型与进度", Group: gallery.Navigation, Order: 7, Parent: "Steps", Render: p2cStepsPage}, p2cNavReset)
}

func p2cTabsPage(c *ui.Context) {
	items := []ui.TabItem{{Key: "a", Label: "选项一"}, {Key: "b", Label: "选项二"}, {Key: "c", Label: "选项三"}}
	navDemo(c, "可编辑卡片 Closable / OnAdd", "关闭按钮名为「移除 xxx」；聚焦标签按 Delete 关闭；+ 新增标签。", func() {
		tb := ui.Tabs(c, &p2cNav.edit, p2cNav.editItems).Closable().OnAdd(func() {
			p2cNav.added++
			k := fmt.Sprint("n", p2cNav.added)
			p2cNav.editItems = append(p2cNav.editItems, ui.TabItem{Key: k, Label: fmt.Sprint("新标签 ", p2cNav.added)})
			p2cNav.edit = k
		}).Show()
		if k := tb.Closed(); k != "" {
			for i, it := range p2cNav.editItems {
				if it.Key == k {
					p2cNav.editItems = append(p2cNav.editItems[:i:i], p2cNav.editItems[i+1:]...)
					if p2cNav.edit == k && len(p2cNav.editItems) > 0 {
						p2cNav.edit = p2cNav.editItems[max(0, i-1)].Key
					}
					break
				}
			}
		}
		ui.Text(c, "当前："+p2cNav.edit).Tertiary()
	})
	navDemo(c, "标签位置 Placement(Left / Right / Bottom)", "竖排时 ↑/↓ 切换；Panel 自动放到标签栏另一侧。", func() {
		navCols(c, func() {
			tb := ui.Tabs(c, &p2cNav.left, items).Placement(ui.Left).Show()
			tb.Panel(func() { ui.Text(c, "左侧标签的内容 "+p2cNav.left) })
		}, func() {
			tb := ui.Tabs(c, &p2cNav.right, items).Placement(ui.Right).Card().Show()
			tb.Panel(func() { ui.Text(c, "右侧卡片标签的内容 "+p2cNav.right) })
		}, func() {
			tb := ui.Tabs(c, &p2cNav.bottom, items).Placement(ui.Bottom).Show()
			tb.Panel(func() { ui.Text(c, "底部标签的内容 "+p2cNav.bottom) })
		})
	})
	navDemo(c, "居中 Centered", "", func() {
		ui.Tabs(c, &p2cNav.center, items).Centered().Show()
	})
	navDemo(c, "溢出滚动与「更多」", "标签放不下时可滚轮横向滚动，右侧 … 下拉列出被遮住的标签。", func() {
		var many []ui.TabItem
		for i := 1; i <= 16; i++ {
			many = append(many, ui.TabItem{Key: fmt.Sprint("t", i), Label: fmt.Sprint("标签页 ", i)})
		}
		ui.Column(c, func() {
			ui.Tabs(c, &p2cNav.many, many).Extra(func() { ui.Button(c, "额外操作").Small() }).Show()
		}, layout.W(560))
	})
}

func p2cPaginationPage(c *ui.Context) {
	navDemo(c, "跳页 ShowQuickJumper", "输入页码后回车（或离开输入框）跳转。", func() {
		ui.Pagination(c, &p2cNav.page, 500, 10).ShowTotal().ShowQuickJumper().Show()
	})
	navDemo(c, "简洁模式 Simple", "", func() {
		ui.Pagination(c, &p2cNav.simple, 500, 10).Simple().Show()
	})
	navDemo(c, "背景色 Background / 禁用 Disabled / 单页隐藏 HideOnSinglePage", "Element 主题默认带背景，Background(false) 为朴素样式；下方单页分页被隐藏。", func() {
		ui.Column(c, func() {
			ui.Pagination(c, &p2cNav.bg, 100, 10).Background(true).Show()
			ui.Pagination(c, &p2cNav.bg, 100, 10).Background(false).Show()
			ui.Pagination(c, &p2cNav.dis, 100, 10).Disabled(true).ShowQuickJumper().Show()
			one := 1
			ui.Pagination(c, &one, 5, 10).HideOnSinglePage().Show()
		}, layout.Gap(16))
	})
}

func p2cBreadcrumbPage(c *ui.Context) {
	navDemo(c, "图标、链接、下拉菜单 BreadcrumbItems", "BreadcrumbItem{Label, Icon, Href, OnClick, Menu}；ClickedHref / MenuClicked 读取结果。", func() {
		b := ui.BreadcrumbItems(c, []ui.BreadcrumbItem{
			{Icon: icon.Home, Label: "首页", Href: "/"},
			{Icon: icon.User, Label: "应用中心", Menu: []ui.MenuItem{{Key: "a", Label: "应用列表"}, {Key: "b", Label: "应用分组"}, {Key: "c", Label: "应用设置"}}},
			{Label: "应用列表", Href: "/apps"},
			{Label: "某应用"},
		}).Show()
		if k := b.MenuClicked(); k != "" {
			p2cNav.crumbMenu = k
		}
	})
	navDemo(c, "自定义分隔符 Separator / SeparatorIcon", "", func() {
		ui.Column(c, func() {
			ui.BreadcrumbItems(c, []ui.BreadcrumbItem{{Label: "首页"}, {Label: "活动管理"}, {Label: "活动列表"}}).Separator(">").Show()
			ui.BreadcrumbItems(c, []ui.BreadcrumbItem{{Label: "首页"}, {Label: "活动管理"}, {Label: "活动详情"}}).SeparatorIcon(icon.Right).Show()
		}, layout.Gap(12))
	})
	ui.Gap(c, 120)
}

var p2cMenuItems = []ui.MenuItem{
	{Group: true, Label: "常用功能", Children: []ui.MenuItem{
		{Key: "o1", Label: "工作台", Icon: icon.Dashboard},
		{Key: "o2", Label: "数据分析", Icon: icon.BarChart},
	}},
	{Group: true, Label: "系统管理", Children: []ui.MenuItem{
		{Key: "o3", Label: "用户管理", Icon: icon.User},
		{Key: "sub", Label: "权限设置", Icon: icon.Lock, Children: []ui.MenuItem{
			{Group: true, Label: "角色", Children: []ui.MenuItem{{Key: "r1", Label: "管理员"}, {Key: "r2", Label: "访客"}}},
			{Key: "r3", Label: "权限点"},
		}},
	}},
}

func p2cMenuPage(c *ui.Context) {
	navDemo(c, "分组 MenuItem{Group: true}", "分组标题不可点击，组内条目与标题同级；弹出子菜单里分组以分隔线呈现。", func() {
		ui.Row(c, func() {
			ui.Menu(c, p2cMenuItems, &p2cNav.menuSel).Width(240).DefaultOpen("sub").Show()
			ui.Column(c, func() { ui.Menu(c, p2cMenuItems, &p2cNav.menuSel).Dark().Width(240).Show() }).Bg(c.Theme.SiderBg)
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
	})
	navDemo(c, "水平菜单溢出折叠", "放不下的条目折叠进「…」子菜单（宽度变化后自动展开 / 收起）。", func() {
		var items []ui.MenuItem
		for i, s := range []string{"首页", "产品中心", "解决方案", "客户案例", "开发者", "合作伙伴", "关于我们", "联系我们"} {
			items = append(items, ui.MenuItem{Key: fmt.Sprint("h", i+1), Label: s})
		}
		ui.Column(c, func() { ui.Menu(c, items, &p2cNav.menuH).Mode(ui.MenuHorizontal).Show() }, layout.W(520))
	})
	ui.Gap(c, 200)
}

func p2cStepsPage(c *ui.Context) {
	items := []ui.StepItem{{Title: "填写信息", SubTitle: "00:00:05"}, {Title: "确认订单", Description: "核对后提交"}, {Title: "完成"}}
	navDemo(c, "导航型 Type(\"navigation\")", "可点击切换；Element 主题为 simple 简洁步骤条。", func() {
		ui.Steps(c, items).Type("navigation").Clickable(&p2cNav.step).Show()
	})
	navDemo(c, "进度 Percent", "当前步骤图标外圈显示进度。", func() {
		ui.Steps(c, []ui.StepItem{{Title: "上传"}, {Title: "校验", Description: "已完成 60%"}, {Title: "发布"}}).Current(1).Percent(60).Show()
	})
}
