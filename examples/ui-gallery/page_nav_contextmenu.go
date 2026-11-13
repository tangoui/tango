package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Context menus without a trigger widget (ui.ContextMenuAt) and the
// shortcut column of menu items (MenuItem.Shortcut).

var ctxMenuDemo struct {
	picked string
	bare   bool // ContextMenuOptions.NoAutoSelect
}

func ctxMenuReset() { ctxMenuDemo.picked, ctxMenuDemo.bare = "", false }

var ctxMenuItems = []ui.MenuItem{
	{Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"},
	{Key: "redo", Label: "重做", Shortcut: "Ctrl+Shift+Z", Disabled: true},
	{Divider: true},
	{Key: "cut", Label: "剪切", Shortcut: "Ctrl+X"},
	{Key: "copy", Label: "复制", Icon: icon.Copy, Shortcut: "Ctrl+C"},
	{Key: "paste", Label: "粘贴", Shortcut: "Ctrl+V"},
	{Divider: true},
	{Key: "find", Label: "查找", Icon: icon.Search, Children: []ui.MenuItem{
		{Key: "find-next", Label: "查找下一个", Shortcut: "F3"},
		{Key: "replace", Label: "替换", Shortcut: "Ctrl+H"},
	}},
	{Key: "delete", Label: "删除行", Icon: icon.Delete, Shortcut: "Ctrl+Shift+K", Danger: true},
}

func ctxMenuPage(c *ui.Context) {
	navDemo(c, "右键菜单 ContextMenuAt", "没有触发器控件：在区域里右键（或聚焦后按菜单键 / Shift+F10）时调用 ui.ContextMenuAt 在指针处打开；焦点进入第一项（勾选“不预选”后焦点在菜单本身、无高亮，↓/↑ 才选中首/末项），Esc、选中或点外部关闭后焦点回到区域。", func() {
		ui.Row(c, func() {
			area := ui.Column(c, func() { ui.Text(c, "在此区域右键").Tertiary() }, layout.Size(320, 160), layout.Center()).
				DashedBorder(1, c.Theme.Border).Radius(c.Theme.RadiusLG).Semantics(a11y.RoleGroup, "编辑区")
			ia := area.Focusable(input.CursorText)
			id := area.ID()
			open := func(p geom.Point) {
				ui.ContextMenuAtWith(c, id, p, ctxMenuItems, func(k string) { ctxMenuDemo.picked = k }, ui.ContextMenuOptions{NoAutoSelect: ctxMenuDemo.bare})
			}
			if p, ok := c.ContextMenuFor(id); ok {
				open(p)
			}
			if ia.Focused {
				for _, k := range c.KeysFor(id) {
					if k.Down && (k.Key == input.KeyContextMenu || k.Key == input.KeyF10 && k.Mods&input.ModShift != 0) {
						r := area.Rect()
						open(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
					}
				}
			}
			ui.Text(c, "选中："+ctxMenuDemo.picked).Tertiary()
			ui.Checkbox(c, "不预选 NoAutoSelect", &ctxMenuDemo.bare)
		}, layout.Gap(16), layout.Middle())
	})
	navDemo(c, "快捷键列 MenuItem.Shortcut", "快捷键显示在菜单项末端（RTL 下在左侧），颜色较淡；可访问名仍是标签，快捷键作为节点的键盘快捷键。", func() {
		ui.Dropdown(c, ctxMenuItems).Trigger(ui.TriggerClick).Body(func() {
			ui.Button(c, "编辑").IconRight(icon.Down)
		})
	})
	ui.Gap(c, 200)
}

func init() {
	navPage(gallery.Page{Name: "DropdownContextMenu", Title: "右键菜单 / ContextMenuAt", Group: gallery.Navigation, Order: 5, Parent: "Dropdown", Render: ctxMenuPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.FindText("在此区域右键"); ok {
				d.RightClick(geom.Pt(r.X+20, r.Y-30))
			}
		}}, ctxMenuReset)
}
