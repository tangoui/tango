package main

// Spec alignment batch 3, group I: demos of the capabilities added to the
// menu components (docs/specs/align/3-menu.md).

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b3iSel      = "home"
	b3iSideSel  = "inbox"
	b3iOpenKeys = []string{"team"}
	b3iBold     = true
	b3iAlign    = "left"
	b3iCmdVal   = ""
	b3iMenuSel  = "docs"
)

var b3iNavItems = []ui.MenuItem{
	{Key: "home", Label: "Home"},
	{Key: "docs", Label: "Docs", Children: []ui.MenuItem{{Key: "intro", Label: "Introduction"}, {Key: "install", Label: "Installation"}}},
	{Key: "blog", Label: "Blog"},
	{Key: "off", Label: "Disabled", Disabled: true},
}

var b3iSideItems = []ui.MenuItem{
	{Key: "g", Label: "Platform", Group: true, Children: []ui.MenuItem{
		{Key: "inbox", Label: "Inbox", Icon: icon.Mail, Shortcut: "12"},
		{Key: "team", Label: "Team", Icon: icon.User, Children: []ui.MenuItem{{Key: "members", Label: "Members"}, {Key: "roles", Label: "Roles"}}},
		{Key: "trash", Label: "Trash", Icon: icon.Delete, Danger: true},
	}},
}

func init() {
	gallery.Register(gallery.Page{Name: "MenuAlign", Parent: "Menu", Title: "Menu 受控展开、危险项与 NavigationMenu 外观", Group: gallery.Navigation, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "侧栏（shadcn Sidebar 外观）", "OpenKeys 受控展开；Danger 危险项；Shortcut 显示为 antd item extra；shadcn 下子项 h-7 挂在左侧导线上，分组标签 text-xs font-medium /70。", func() {
				ui.Menu(c, b3iSideItems, &b3iSideSel).Width(256).OpenKeys(&b3iOpenKeys).Show()
			})
			gallery.Demo(c, "横向（shadcn NavigationMenu 外观）", "shadcn：h-9 px-4 font-medium 触发器、gap-1、hover bg-accent、chevron 打开时翻转；ItemPanel 放任意面板内容（NavigationMenuContent）。", func() {
				ui.Menu(c, b3iNavItems, &b3iMenuSel).Mode(ui.MenuHorizontal).
					ItemPanel("blog", func() {
						ui.Column(c, func() {
							ui.Text(c, "Latest posts").Strong()
							ui.Text(c, "Release notes and guides.").Secondary()
						}, layout.Gap(4), layout.W(220))
					}).Show()
			})
			gallery.Demo(c, "点击展开、收起无提示", "TriggerSubMenuAction(TriggerClick)；Collapsed + Tooltip(false)；InlineIndent(16)。", func() {
				row(c, func() {
					ui.Menu(c, b3iNavItems, &b3iSel).Mode(ui.MenuVertical).Width(200).TriggerSubMenuAction(ui.TriggerClick).Show()
					ui.Menu(c, b3iSideItems, &b3iSideSel).Collapsed(true).Tooltip(false).Show()
					ui.Menu(c, b3iNavItems, &b3iSel).Width(200).InlineIndent(16).DefaultOpen("docs").Show()
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "DropdownAlign", Parent: "Dropdown", Title: "Dropdown 勾选项、PopupRender 与箭头", Group: gallery.Navigation, Order: 5, Height: 520,
		Render: func(c *ui.Context) {
			items := []ui.MenuItem{
				{Key: "bold", Label: "Bold", Shortcut: "Ctrl+B"},
				{Key: "align", Label: "Align", Group: true, Children: []ui.MenuItem{{Key: "left", Label: "Left"}, {Key: "right", Label: "Right"}}},
				{Divider: true},
				{Key: "more", Label: "More", Icon: icon.Setting, Children: []ui.MenuItem{{Key: "x", Label: "Export"}}},
				{Key: "del", Label: "Delete", Icon: icon.Delete, Danger: true},
			}
			gallery.Demo(c, "勾选与单选项（shadcn CheckboxItem / RadioGroup）", "Check(key, &on)、Radio(&value, keys...)；HideOnClick(false) 选后不关；PopupRender 在菜单下方加内容。", func() {
				row(c, func() {
					ui.Dropdown(c, items).Trigger(ui.TriggerClick).Check("bold", &b3iBold).Radio(&b3iAlign, "left", "right").HideOnClick(false).
						PopupRender(func(menu func()) {
							menu()
							ui.Row(c, func() { ui.Text(c, "Formatting").Secondary() }, layout.PadVH(5, 12))
						}).Body(func() { ui.Button(c, "Format") })
					ui.Dropdown(c, items).Placement(ui.Right).ArrowPointAtCenter(true).Trigger(ui.TriggerClick).Body(func() { ui.Button(c, "Right") })
				})
			})
		},
		Setup: func(d *ui.Driver) { d.ClickLabel("Format") },
	})

	gallery.Register(gallery.Page{Name: "CommandAlign", Parent: "Command", Title: "Command 排序、受控值与键盘", Group: gallery.Navigation, Order: 30, Height: 420,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "cmdk 行为", "字母按顺序出现即匹配，整词/前缀优先，分组按最佳项排序；Loop(true) 首尾循环；Home/End、Ctrl+N/P；Value 受控当前项。", func() {
				ui.Command(c, []ui.CommandItem{
					{Value: "cal", Label: "Calendar", Group: "Suggestions", Icon: icon.Calendar},
					{Value: "emoji", Label: "Search Emoji", Group: "Suggestions", Icon: icon.Smile},
					{Value: "calc", Label: "Calculator", Group: "Suggestions", Disabled: true},
					{Value: "prof", Label: "Profile", Group: "Settings", Icon: icon.User, Shortcut: "⌘P"},
					{Value: "bill", Label: "Billing", Group: "Settings", Shortcut: "⌘B"},
				}).Loop(true).Value(&b3iCmdVal).Layout(layout.W(420))
			})
		}})
}
