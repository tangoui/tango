package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Dropdown / Menu groups: MenuItem.Group renders a titled section in the
// dropdown and in pop-out submenus (shadcn DropdownMenuLabel + separator,
// Ant group title, Element item-group title).

var shad2cDD struct {
	clicked string
	menu    string
}

func shad2cDDReset() { shad2cDD.clicked, shad2cDD.menu = "", "profile" }

var shad2cGroupItems = []ui.MenuItem{
	{Key: "account", Label: "我的账户", Group: true, Children: []ui.MenuItem{
		{Key: "profile", Label: "个人资料", Icon: icon.User},
		{Key: "billing", Label: "账单", Icon: icon.Shop},
		{Key: "settings", Label: "设置", Icon: icon.Setting},
	}},
	{Key: "team", Label: "团队", Group: true, Children: []ui.MenuItem{
		{Key: "members", Label: "成员", Icon: icon.Team},
		{Key: "invite", Label: "邀请", Children: []ui.MenuItem{{Key: "mail", Label: "邮件"}, {Key: "msg", Label: "消息"}}},
	}},
	{Key: "logout", Label: "退出登录", Icon: icon.Logout, Danger: true},
}

func shad2cDropdownPage(c *ui.Context) {
	navDemo(c, "下拉菜单分组 MenuItem.Group", "分组渲染为带标题的分区（不再当成子菜单）；方向键跳过分组标题。", func() {
		ui.Row(c, func() {
			dd := ui.Dropdown(c, shad2cGroupItems).Trigger(ui.TriggerClick).MinWidth(200).Body(func() {
				ui.Button(c, "分组菜单").Icon(icon.Down)
			})
			if k := dd.Clicked(); k != "" {
				shad2cDD.clicked = k
			}
			ui.Text(c, "选中："+shad2cDD.clicked).Tertiary()
		}, layout.Gap(16), layout.Middle())
		ui.Column(c, func() {}, layout.H(330))
	})
	navDemo(c, "弹出子菜单中的分组", "Menu 竖向模式下，子菜单里的分组同样有标题。", func() {
		items := []ui.MenuItem{
			{Key: "home", Label: "首页", Icon: icon.Home},
			{Key: "user", Label: "用户", Icon: icon.User, Children: shad2cGroupItems[:2]},
		}
		ui.Column(c, func() { ui.Menu(c, items, &shad2cDD.menu).Mode(ui.MenuVertical).Width(200).Show() }, layout.W(200))
		ui.Column(c, func() {}, layout.H(260))
	})
}

func init() {
	shad2cDDReset()
	navPage(gallery.Page{Name: "DropdownGroup", Title: "Dropdown / Menu 分组", Group: gallery.Navigation, Order: 5, Parent: "Dropdown", Render: shad2cDropdownPage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("分组菜单")
			d.Key(input.KeyDown, 0) // keyboard highlight lands on the first item, past the group title
			if r, ok := d.FindText("用户"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		}}, shad2cDDReset)
}
