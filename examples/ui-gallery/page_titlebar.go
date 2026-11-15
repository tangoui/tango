package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// TitleBar: the self-drawn title bar of a frameless window, with
// WindowControls. The gallery has no native window, so a demo frame keeps
// the window state (a uihost window drives the real one).

type demoFrame struct {
	state ui.WindowState
	moves int
	last  string
}

func (f *demoFrame) FrameInfo() ui.WindowFrameInfo   { return ui.WindowFrameInfo{State: f.state} }
func (f *demoFrame) SetWindowState(s ui.WindowState) { f.state = s }
func (f *demoFrame) CloseWindow()                    { f.last = "close" }
func (f *demoFrame) StartWindowMove()                { f.moves++ }

var titleBarDemo = struct {
	win, mac demoFrame
	query    string
}{}

func titleBarMenus() []ui.MenubarMenu {
	return []ui.MenubarMenu{
		{Key: "file", Label: "文件", Items: []ui.MenubarItem{{Key: "open", Label: "打开…", Shortcut: "Ctrl+O"}}},
		{Key: "edit", Label: "编辑", Items: []ui.MenubarItem{{Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"}}},
		{Key: "view", Label: "视图", Items: []ui.MenubarItem{{Key: "full", Label: "全屏", Shortcut: "F11"}}},
	}
}

func titleBarTools(c *ui.Context) {
	ui.Tooltip(c, "切换侧栏", func() { ui.IconButton(c, icon.PanelLeft, "切换侧栏") }).Placement("bottom")
	ui.Tooltip(c, "设置", func() { ui.IconButton(c, icon.Setting, "设置") }).Placement("bottom")
}

func titleBarPage(c *ui.Context) {
	s := &titleBarDemo
	window := func(fn func()) {
		ui.Column(c, fn, layout.Margin(geom.Insets{Bottom: 12}), layout.Pad(1)).
			Border(1, c.Theme.BorderSecondary).Radius(c.Theme.Radius).Clip()
	}
	state := func(f *demoFrame) string {
		return [...]string{"正常", "最小化", "最大化", "全屏"}[f.state]
	}
	gallery.Demo(c, "Windows / Linux", "无边框窗口的标题栏：图标、菜单、居中的搜索框、工具按钮和右侧窗口按钮。按住空白处拖动窗口，双击最大化 / 还原；关闭按钮悬停变红。", func() {
		window(func() {
			ui.TitleBar(c).Icon(icon.Code).Title("Tango IDE").Frame(&s.win).Controls(ui.WindowControlsCaption).
				Menu(func() { ui.Menubar(c, titleBarMenus()).A11yLabel("应用菜单") }).
				Center(func() { ui.Search(c, &s.query).Placeholder("搜索文件、命令").Size(ui.Small).Width(280) }).
				Tools(func() { titleBarTools(c) })
			ui.Column(c, func() {}, layout.H(48))
		})
		ui.Text(c, "窗口状态："+state(&s.win)).Tertiary()
	})
	gallery.Demo(c, "macOS", "红绿灯在左侧，悬停整组时显示符号；标题居中。系统自己绘制红绿灯时（TitleBarHidden），标题栏只留出位置。", func() {
		window(func() {
			ui.TitleBar(c).Title("main.go — tango").Frame(&s.mac).Controls(ui.WindowControlsTraffic).
				Tools(func() { titleBarTools(c) })
			ui.Column(c, func() {}, layout.H(48))
		})
		ui.Text(c, "窗口状态："+state(&s.mac)).Tertiary()
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "TitleBar", Title: "TitleBar 标题栏", Group: gallery.Layout, Order: 61, Render: titleBarPage,
		Setup: func(d *ui.Driver) {
			titleBarDemo.win, titleBarDemo.mac = demoFrame{state: ui.WindowMaximized}, demoFrame{}
			// Hover the caption close button (right of restore).
			if r, ok := d.Find("向下还原"); ok {
				d.Move(geom.Pt(r.X+r.W*1.5, r.Y+r.H/2))
				d.Settle()
			}
		}})
}
