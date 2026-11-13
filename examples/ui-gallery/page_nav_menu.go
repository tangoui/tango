package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var ddItems = []ui.MenuItem{
	{Key: "profile", Label: "个人中心", Icon: icon.User},
	{Key: "share", Label: "分享到", Icon: icon.Link, Children: []ui.MenuItem{
		{Key: "mail", Label: "邮件", Icon: icon.Mail},
		{Key: "more", Label: "更多渠道", Children: []ui.MenuItem{{Key: "wx", Label: "微信"}, {Key: "wb", Label: "微博"}}},
	}},
	{Key: "setting", Label: "系统设置", Icon: icon.Setting},
	{Key: "dis", Label: "禁用项", Disabled: true},
	{Divider: true},
	{Key: "logout", Label: "退出登录", Icon: icon.Logout, Danger: true},
}

var menuState = struct {
	sel, hsel string
	collapsed bool
}{sel: "monitor", hsel: "form", collapsed: true}

func dropdownPage(c *ui.Context) {
	navDemo(c, "触发方式与多级子菜单", "悬停（默认）、点击、右键（长按 500ms 或后端上报右键）三种触发；子菜单向右弹出；方向键导航。", func() {
		ui.Row(c, func() {
			ui.Dropdown(c, ddItems).Body(func() { ui.Link(c, "悬停展开 ▾") })
			ui.Dropdown(c, ddItems).Trigger(ui.TriggerClick).Arrow().Selected("setting").Body(func() {
				ui.Button(c, "点击展开").IconRight(icon.Down)
			})
			ui.Dropdown(c, ddItems).Trigger(ui.TriggerContextMenu).Body(func() {
				ui.Column(c, func() { ui.Text(c, "在此区域右键 / 长按").Tertiary() }, layout.Size(220, 80), layout.Center()).
					DashedBorder(1, c.Theme.Border).Radius(c.Theme.RadiusLG)
			})
		}, layout.Gap(32), layout.Middle())
	})
	navDemo(c, "按钮下拉 Dropdown(...).Button(label)", "左侧为主按钮（ButtonClicked），右侧展开菜单（Clicked）。", func() {
		ui.Row(c, func() {
			ui.Dropdown(c, ddItems[:3]).Button("操作")
			ui.Dropdown(c, ddItems[:3]).Primary().Button("提交")
			ui.Dropdown(c, ddItems[:3]).Danger().Button("危险")
			ui.Dropdown(c, ddItems[:3]).Size(ui.Small).Button("小号")
		}, layout.Gap(16), layout.Middle())
	})
	ui.Gap(c, 220)
}

func menuPage(c *ui.Context) {
	items := proMenu[:6]
	navDemo(c, "水平菜单（下拉子菜单）", "Mode(ui.MenuHorizontal)；←/→ 切换，↓ 或回车展开下拉。", func() {
		ui.Menu(c, items, &menuState.hsel).Mode(ui.MenuHorizontal).Show()
	})
	navDemo(c, "内嵌模式（手风琴）/ 暗色", "↑/↓ 移动，→ 展开并进入子菜单，← 收起或返回上级，回车选中。", func() {
		ui.Row(c, func() {
			ui.Menu(c, items, &menuState.sel).Accordion().Width(240).Show()
			ui.Column(c, func() { ui.Menu(c, items, &menuState.sel).Dark().Width(240).Show() }).Bg(c.Theme.SiderBg)
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
	})
	navDemo(c, "垂直弹出 / 折叠图标模式", "子菜单从右侧弹出，折叠后叶子项显示提示；Esc 关闭弹出的子菜单。", func() {
		ui.Row(c, func() {
			ui.Menu(c, items, &menuState.sel).Mode(ui.MenuVertical).Width(200).Show()
			ui.Gap(c, 40)
			ui.Column(c, func() {
				ui.Menu(c, items, &menuState.sel).Collapsed(true).Dark().Show()
			}).Bg(c.Theme.SiderBg)
			ui.Gap(c, 40)
			ui.Menu(c, items, &menuState.sel).Collapsed(menuState.collapsed).Show()
		}, layout.AlignItems(layout.AlignStart), layout.H(330))
	})
}

func init() {
	navPage(gallery.Page{Name: "Dropdown", Title: "Dropdown 下拉菜单", Group: gallery.Navigation, Order: 5, Render: dropdownPage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("点击展开")
			d.Settle()
			if r, ok := d.Find("分享到"); ok {
				d.Move(r.Center())
				d.Settle()
			}
			if r, ok := d.Find("更多渠道"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		}}, nil)
	navPage(gallery.Page{Name: "Menu", Title: "Menu 导航菜单", Group: gallery.Navigation, Order: 1, Render: menuPage,
		Setup: func(d *ui.Driver) {
			// Hover the last collapsed menu's first item (Find returns the last match).
			if r, ok := d.Find("仪表盘"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		}}, func() { menuState.sel, menuState.hsel = "monitor", "form" })
}
