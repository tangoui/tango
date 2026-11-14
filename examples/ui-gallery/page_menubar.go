package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Menubar: an application menu bar inside the window (shadcn/ui Menubar).

var menubarDemo struct {
	status, ruler bool
	zoom          string
	last          string
}

func menubarReset() {
	menubarDemo.status, menubarDemo.ruler, menubarDemo.zoom, menubarDemo.last = true, false, "100", "—"
}

func menubarMenus() []ui.MenubarMenu {
	s := &menubarDemo
	return []ui.MenubarMenu{
		{Key: "file", Label: "文件", Items: []ui.MenubarItem{
			{Key: "new", Label: "新建窗口", Icon: icon.Plus, Shortcut: "Ctrl+N"},
			{Key: "open", Label: "打开…", Icon: icon.Folder, Shortcut: "Ctrl+O"},
			{Divider: true},
			{Key: "share", Label: "共享", Children: []ui.MenubarItem{
				{Key: "mail", Label: "邮件链接"},
				{Key: "msg", Label: "消息"},
				{Key: "notes", Label: "备忘录"},
			}},
			{Divider: true},
			{Key: "print", Label: "打印…", Shortcut: "Ctrl+P", Disabled: true},
		}},
		{Key: "edit", Label: "编辑", Items: []ui.MenubarItem{
			{Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"},
			{Key: "redo", Label: "重做", Shortcut: "Ctrl+Shift+Z"},
			{Divider: true},
			{Key: "cut", Label: "剪切", Shortcut: "Ctrl+X"},
			{Key: "copy", Label: "复制", Shortcut: "Ctrl+C"},
			{Key: "paste", Label: "粘贴", Shortcut: "Ctrl+V"},
		}},
		{Key: "view", Label: "视图", Items: []ui.MenubarItem{
			{Key: "status", Label: "状态栏", Kind: ui.MenubarItemCheckbox, Checked: &s.status},
			{Key: "ruler", Label: "标尺", Kind: ui.MenubarItemCheckbox, Checked: &s.ruler, Shortcut: "Ctrl+R"},
			{Label: "缩放", Group: true, Children: []ui.MenubarItem{
				{Key: "z75", Label: "75%", Kind: ui.MenubarItemRadio, Radio: &s.zoom, Value: "75"},
				{Key: "z100", Label: "100%", Kind: ui.MenubarItemRadio, Radio: &s.zoom, Value: "100"},
				{Key: "z150", Label: "150%", Kind: ui.MenubarItemRadio, Radio: &s.zoom, Value: "150"},
			}},
			{Divider: true},
			{Key: "full", Label: "全屏", Shortcut: "F11"},
		}},
		{Key: "window", Label: "窗口", Disabled: true},
		{Key: "help", Label: "帮助", Items: []ui.MenubarItem{
			{Key: "docs", Label: "文档", Icon: icon.Question},
			{Key: "about", Label: "关于"},
		}},
	}
}

func menubarPage(c *ui.Context) {
	gallery.Demo(c, "应用菜单栏", "点击顶层项打开菜单，打开后移到相邻项直接切换；Alt 或 F10 进入菜单栏，←→ 切换、↑↓ 移动、Enter 执行、Esc 关闭、字母键跳转。勾选项、单选组、子菜单、分隔线、快捷键提示与禁用项。", func() {
		mb := ui.Menubar(c, menubarMenus()).A11yLabel("应用菜单")
		if k := mb.Clicked(); k != "" {
			menubarDemo.last = k
		}
		ui.Text(c, "最近执行："+menubarDemo.last+"　缩放："+menubarDemo.zoom+"%").Tertiary().
			Layout(layout.Margin(geom.Insets{Top: 12}))
		ui.Column(c, func() {}, layout.H(320))
	})
}

func init() {
	menubarReset()
	gallery.Register(gallery.Page{Name: "Menubar", Title: "Menubar 菜单栏", Group: gallery.Navigation, Order: 6, Render: menubarPage,
		Setup: func(d *ui.Driver) {
			menubarReset()
			d.ClickLabel("视图")
			d.Settle()
		}})
}
