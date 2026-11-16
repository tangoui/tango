package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Navigation and overlay fixtures (Task 5). Every value a component can
// change lives in the driver's Context (State) and is only changed by the
// component's own events; interactions are real pointer / key / wheel
// input. Popups opened from a trigger (Dropdown, Menubar, context menus,
// NotificationCenter, FloatButtonGroup) build closed like the Select seed;
// overlay surfaces (Drawer, Overlay) and the imperative pop-ups (Message,
// Notification, Confirm, MessageBox, Prompt) build open, close on step 0
// and are reopened on step 1 through their trigger button; Tour builds open
// and steps next / previous with its own buttons.

func perfNavID(key string) ID { return hashID("perf.nav." + key) }

// perfNavFind is the rect of the widget labelled label, else of a node
// whose text is label.
func perfNavFind(r *perfRun, label string) geom.Rect {
	if box, ok := r.D.Find(label); ok {
		return box
	}
	if box, ok := r.D.FindText(label); ok {
		return box
	}
	panic("perf: nav target not found: " + label)
}

// perfNavClick clicks label, then moves the pointer off the fixture so
// the settled screen has no hover.
func perfNavClick(r *perfRun, label string) {
	r.D.Click(perfNavFind(r, label).Center())
	r.D.Move(perfOutside)
}

// perfNavSwitch clicks a on even steps and b on odd ones (select / undo).
func perfNavSwitch(a, b string) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		if i%2 == 0 {
			perfNavClick(r, a)
		} else {
			perfNavClick(r, b)
		}
	}
}

// perfNavPopup opens a trigger-built popup by clicking label on even
// steps and closes it with Escape on odd ones (then blurs the trigger,
// which Escape focuses, with a click on the page padding).
func perfNavPopup(label string) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		if i%2 == 0 {
			perfNavClick(r, label)
			return
		}
		r.D.Key(input.KeyEscape, 0)
		r.D.Click(perfOutside) // Escape returns focus to the trigger: blur it
	}
}

// perfNavReopen closes a build-open overlay with Escape on even steps and
// reopens it by clicking its trigger button on odd ones.
func perfNavReopen(trigger string) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		if i%2 == 0 {
			r.D.Key(input.KeyEscape, 0)
			r.D.Settle()
			return
		}
		perfNavClick(r, trigger)
		r.D.Settle()
	}
}

// perfNavOnce runs open on the first build of a driver's page (the
// imperative pop-ups are called once, like from a Clicked branch, never
// every frame).
func perfNavOnce(c *Context, key string, open func()) {
	State(c, perfNavID(key+".once"), func() bool { open(); return true })
}

var perfNavMenuItems = []MenuItem{
	{Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []MenuItem{
		{Key: "analysis", Label: "分析页"}, {Key: "monitor", Label: "监控页"}, {Key: "workplace", Label: "工作台"},
	}},
	{Key: "form", Label: "表单页", Icon: icon.Form},
	{Key: "list", Label: "列表页", Icon: icon.Table, Children: []MenuItem{
		{Key: "table-list", Label: "查询表格"}, {Key: "basic-list", Label: "标准列表"},
	}},
	{Key: "profile", Label: "详情页", Icon: icon.Profile},
	{Key: "result", Label: "结果页", Icon: icon.CheckCircle},
	{Key: "exception", Label: "异常页", Icon: icon.Warning},
}

var perfNavDropdownItems = []MenuItem{
	{Key: "profile", Label: "个人中心", Icon: icon.User},
	{Key: "share", Label: "分享到", Icon: icon.Link, Children: []MenuItem{
		{Key: "mail", Label: "邮件", Icon: icon.Mail}, {Key: "wx", Label: "微信"},
	}},
	{Key: "setting", Label: "系统设置", Icon: icon.Setting},
	{Key: "dis", Label: "禁用项", Disabled: true},
	{Divider: true},
	{Key: "logout", Label: "退出登录", Icon: icon.Logout, Danger: true},
}

var perfNavContextItems = []MenuItem{
	{Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"},
	{Key: "redo", Label: "重做", Shortcut: "Ctrl+Shift+Z", Disabled: true},
	{Divider: true},
	{Key: "cut", Label: "剪切", Shortcut: "Ctrl+X"},
	{Key: "copy", Label: "复制", Icon: icon.Copy, Shortcut: "Ctrl+C"},
	{Key: "paste", Label: "粘贴", Shortcut: "Ctrl+V"},
	{Divider: true},
	{Key: "delete", Label: "删除行", Icon: icon.Delete, Shortcut: "Ctrl+Shift+K", Danger: true},
}

func perfNavMenubar() []MenubarMenu {
	return []MenubarMenu{
		{Key: "file", Label: "文件", Items: []MenubarItem{
			{Key: "new", Label: "新建窗口", Icon: icon.Plus, Shortcut: "Ctrl+N"},
			{Key: "open", Label: "打开…", Icon: icon.Folder, Shortcut: "Ctrl+O"},
			{Divider: true},
			{Key: "print", Label: "打印…", Shortcut: "Ctrl+P", Disabled: true},
		}},
		{Key: "edit", Label: "编辑", Items: []MenubarItem{
			{Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"},
			{Key: "redo", Label: "重做", Shortcut: "Ctrl+Shift+Z"},
		}},
		{Key: "help", Label: "帮助", Items: []MenubarItem{
			{Key: "docs", Label: "文档", Icon: icon.Question},
			{Key: "about", Label: "关于"},
		}},
	}
}

var perfNavCrumbs = []string{"首页", "应用中心", "应用列表", "某应用"}

// perfNavSections are the anchored chapters of the scroll-position
// fixtures (Anchor, Affix, BackTop).
var perfNavSections = []struct{ key, title string }{
	{"basic", "基础用法"}, {"static", "静态位置"}, {"api", "API 参考"}, {"faq", "常见问题"},
}

func perfNavChapters(c *Context, anchored bool) {
	for _, s := range perfNavSections {
		body := func() {
			Title(c, s.title, 4)
			for i := 0; i < 2; i++ {
				Paragraph(c, "锚点用于跳转到页面指定位置，并随滚动高亮当前所在的章节。Content scrolls inside the container.")
			}
		}
		if anchored {
			AnchorSection(c, s.key, body, layout.Gap(8), layout.Margin(geom.Insets{Top: 16}))
		} else {
			Column(c, body, layout.Gap(8), layout.Margin(geom.Insets{Top: 16}))
		}
	}
	Gap(c, 200)
}

// perfNavScrollBox is the 360px scroll container of the scroll-position
// fixtures; their interaction wheels it at its center.
func perfNavScrollBox(c *Context, fn func()) *ScrollBox {
	return Scroll(c, fn, layout.Size(640, 360), layout.PadVH(0, 16))
}

// perfNavWheel scrolls the scroll container (the fixture's first node)
// down on even steps and back up on odd ones.
func perfNavWheel(r *perfRun, i int) {
	p := geom.Pt(r.Target.X+320, r.Target.Y+180)
	dy := float32(330)
	if i%2 == 1 {
		dy = -dy
	}
	r.D.Scroll(p, dy)
	r.D.Move(perfOutside)
}

// perfNavLargeItems are n flat menu items "菜单项 i" (keys "m<i>").
func perfNavLargeItems(n int) []MenuItem {
	items := make([]MenuItem, n)
	for i := range items {
		s := strconv.Itoa(i + 1)
		items[i] = MenuItem{Key: "m" + s, Label: "菜单项 " + s}
	}
	return items
}

// perfNavLargeTabs are n tabs "标签 i" (keys "t<i>").
func perfNavLargeTabs(n int) []TabItem {
	tabs := make([]TabItem, n)
	for i := range tabs {
		s := strconv.Itoa(i + 1)
		tabs[i] = TabItem{Key: "t" + s, Label: "标签 " + s}
	}
	return tabs
}

func perfNavCommandItems(n int) []CommandItem {
	if n <= 0 {
		return []CommandItem{
			{Value: "calendar", Label: "日历 Calendar", Group: "建议", Icon: icon.Calendar},
			{Value: "search", Label: "搜索 Search", Group: "建议", Icon: icon.Search},
			{Value: "profile", Label: "个人资料 Profile", Group: "设置", Icon: icon.User, Shortcut: "⌘P"},
			{Value: "billing", Label: "账单 Billing", Group: "设置", Shortcut: "⌘B"},
			{Value: "settings", Label: "设置 Settings", Group: "设置", Icon: icon.Setting, Shortcut: "⌘S"},
		}
	}
	items := make([]CommandItem, n)
	for i := range items {
		s := strconv.Itoa(i + 1)
		items[i] = CommandItem{Value: "cmd-" + s, Label: "命令 Command " + s, Group: "命令"}
	}
	return items
}

func perfNavCommand(n int) func(*Context) {
	items := perfNavCommandItems(n)
	return func(c *Context) {
		Command(c, items).Placeholder("输入命令或搜索…").Layout(layout.W(420))
	}
}

func init() {
	registerPerf(
		perfFixture{
			Name: "Menu",
			Build: func(c *Context) {
				sel := State(c, perfNavID("menu"), func() string { return "profile" })
				Menu(c, perfNavMenuItems, sel).Width(240).Show()
			},
			Interact: perfNavSwitch("表单页", "详情页"),
			Large: func(n int) func(*Context) {
				items := perfNavLargeItems(n)
				return func(c *Context) {
					sel := State(c, perfNavID("menu.large"), func() string { return "m1" })
					Scroll(c, func() { Menu(c, items, sel).Width(240).Show() }, layout.Size(280, 760))
				}
			},
		},
		perfFixture{
			Name:     "Menubar",
			Build:    func(c *Context) { Menubar(c, perfNavMenubar()) },
			Interact: perfNavPopup("文件"),
		},
		perfFixture{
			Name: "Tabs",
			Build: func(c *Context) {
				tab := State(c, perfNavID("tabs"), func() string { return "1" })
				Tabs(c, tab, []TabItem{{Key: "1", Label: "基本信息"}, {Key: "2", Label: "安全设置", Icon: icon.Lock}, {Key: "3", Label: "消息通知"}, {Key: "4", Label: "禁用", Disabled: true}}).Show()
			},
			Interact: perfNavSwitch("安全设置", "基本信息"),
			// n tabs in one bar (the ones that do not fit scroll and are
			// listed in the more menu); switching tabs is the interaction.
			Large: func(n int) func(*Context) {
				tabs := perfNavLargeTabs(n)
				return func(c *Context) {
					tab := State(c, perfNavID("tabs.large"), func() string { return "t1" })
					Tabs(c, tab, tabs).Show()
				}
			},
			LargeInteract: perfNavSwitch("标签 2", "标签 1"),
		},
		perfFixture{
			// Clicking a crumb navigates up (the path is cut there); the
			// link in the page content navigates back down.
			Name: "Breadcrumb",
			Build: func(c *Context) {
				path := State(c, perfNavID("crumbs"), func() []string { return append([]string(nil), perfNavCrumbs...) })
				if i := Breadcrumb(c, (*path)...).Clicked(); i >= 0 && i < len(*path)-1 {
					*path = append([]string(nil), (*path)[:i+1]...)
					c.Invalidate()
				}
				if Link(c, "进入某应用").Clicked() {
					*path = append([]string(nil), perfNavCrumbs...)
					c.Invalidate()
				}
			},
			Interact: perfNavSwitch("应用中心", "进入某应用"),
		},
		perfFixture{
			// An item with a dropdown menu, which opens on hover (the
			// breadcrumb's Dropdown keeps the default hover trigger).
			Name: "BreadcrumbItems",
			Build: func(c *Context) {
				BreadcrumbItems(c, []BreadcrumbItem{
					{Label: "首页", Icon: icon.Home, Href: "/"},
					{Label: "应用中心", Menu: []MenuItem{{Key: "a", Label: "应用一"}, {Key: "b", Label: "应用二"}}},
					{Label: "应用列表", Href: "/apps"},
					{Label: "某应用"},
				}).Show()
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Move(perfNavFind(r, "应用中心").Center())
				} else {
					r.D.Move(perfOutside)
				}
				r.D.Settle()
			},
		},
		perfFixture{
			Name: "Pagination",
			Build: func(c *Context) {
				Pagination(c, State(c, perfNavID("page"), func() int { return 1 }), 500, 10).Show()
			},
			Interact: perfNavSwitch("2", "1"),
		},
		perfFixture{
			Name: "Steps",
			Build: func(c *Context) {
				cur := State(c, perfNavID("steps"), func() int { return 1 })
				Steps(c, []StepItem{{Title: "已完成", Description: "这是一段描述"}, {Title: "进行中", SubTitle: "剩余 00:00:08", Description: "这是一段描述"}, {Title: "待处理", Description: "这是一段描述"}}).Clickable(cur).Show()
			},
			Interact: perfNavSwitch("待处理", "进行中"),
		},
		perfFixture{
			// Anchor needs its sections inside the scroll container it
			// follows: wheeling the container moves the highlight.
			Name: "Anchor", Covers: []string{"AnchorSection"},
			Build: func(c *Context) {
				Row(c, func() {
					sb := perfNavScrollBox(c, func() { perfNavChapters(c, true) })
					Anchor(c, []AnchorItem{{Key: "basic", Title: "基础用法"}, {Key: "static", Title: "静态位置"}, {Key: "api", Title: "API 参考"}, {Key: "faq", Title: "常见问题"}}).Target(sb.ID()).Show()
				}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
			},
			Interact: perfNavWheel,
		},
		perfFixture{
			Name: "Affix",
			Build: func(c *Context) {
				// The Affix is built inside the container it pins to, before
				// the container's ID exists: keep the (stable) ID of the
				// last frame, set once, as the gallery does.
				target := State[ID](c, perfNavID("affix.target"), nil)
				sb := perfNavScrollBox(c, func() {
					Affix(c).OffsetTop(0).Target(*target).Body(func() {
						Row(c, func() {
							Text(c, "固钉工具条").Strong()
							Spacer(c)
							Button(c, "保存").Primary().Small()
						}, layout.PadVH(8, 16), layout.Middle()).Bg(c.Theme.Primary.Bg).Border(1, c.Theme.Primary.Border).Radius(c.Theme.Radius)
					})
					perfNavChapters(c, false)
				})
				if *target != sb.ID() {
					*target = sb.ID()
					c.Invalidate()
				}
			},
			Interact: perfNavWheel,
		},
		perfFixture{
			// Wheeling down past VisibilityHeight shows the button;
			// clicking it scrolls the container back to the top.
			Name: "BackTop",
			Build: func(c *Context) {
				sb := perfNavScrollBox(c, func() { perfNavChapters(c, false) })
				BackTop(c, sb.ID()).VisibilityHeight(200).Show()
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfNavWheel(r, 0)
					return
				}
				perfNavClick(r, "回到顶部")
				r.D.Settle()
			},
		},
		perfFixture{
			// The button toggles the help it stands for and shows the
			// state on itself (question mark / close), so the visible
			// change stays at the button, not elsewhere on the page.
			Name: "FloatButton",
			Build: func(c *Context) {
				open := State[bool](c, perfNavID("float"), nil)
				ic, label := icon.Question, "帮助"
				if *open {
					ic, label = icon.Close, "关闭帮助"
				}
				if FloatButton(c).Icon(ic).Primary().A11yLabel(label).Clicked() {
					*open = !*open
					c.Invalidate()
				}
			},
			Interact: func(r *perfRun, _ int) {
				r.D.Click(perfNavFloat(r).Center())
				r.D.Move(perfOutside)
			},
		},
		perfFixture{
			Name: "FloatButtonGroup",
			Build: func(c *Context) {
				open := State[bool](c, perfNavID("floatgroup"), nil)
				FloatButtonGroup(c).Trigger("click").Open(open).Primary().Body(func() {
					FloatButton(c).Icon(icon.Message).A11yLabel("消息")
					FloatButton(c).Icon(icon.Edit).A11yLabel("编辑")
				})
			},
			Interact: func(r *perfRun, i int) {
				r.D.Click(perfNavFloat(r).Center())
				r.D.Move(perfOutside)
			},
		},
		perfFixture{
			Name: "Dropdown",
			Build: func(c *Context) {
				Dropdown(c, perfNavDropdownItems).Trigger(TriggerClick).Body(func() { Button(c, "点击展开").IconRight(icon.Down) })
			},
			Interact: perfNavPopup("点击展开"),
		},
		perfFixture{
			// Right-click opens the menu at the pointer; Escape closes it.
			// ContextMenuOpen drives the status line next to the area.
			Name: "ContextMenuAt", Covers: []string{"ContextMenuOpen"},
			Build: func(c *Context) {
				Row(c, func() {
					area := perfNavContextArea(c, "在此区域右键")
					if p, ok := c.ContextMenuFor(area); ok {
						ContextMenuAt(c, area, p, perfNavContextItems, func(string) {})
					}
					status := "菜单：关闭"
					if ContextMenuOpen(c, area) {
						status = "菜单：打开"
					}
					Text(c, status).Tertiary()
				}, layout.Gap(16), layout.Middle())
			},
			Interact: perfNavContextMenu(false),
		},
		perfFixture{
			// The code-editor pattern: NoAutoSelect, and a wheel over the
			// owner closes the menu through CloseContextMenu.
			Name: "ContextMenuAtWith", Covers: []string{"CloseContextMenu"},
			Build: func(c *Context) {
				area := perfNavContextArea(c, "编辑区右键")
				if _, _, ok := c.WheelFor(area); ok {
					CloseContextMenu(c, area)
				}
				if p, ok := c.ContextMenuFor(area); ok {
					ContextMenuAtWith(c, area, p, perfNavContextItems, func(string) {}, ContextMenuOptions{NoAutoSelect: true})
				}
			},
			Interact: perfNavContextMenu(true),
		},
		perfFixture{
			Name: "SetHoverOpen",
			Skip: "sets a hover-intent flag and builds nothing; hover-triggered popups (Dropdown) call it internally",
		},
		perfFixture{
			Name: "Drawer", Overlay: true,
			Build: func(c *Context) {
				open := State(c, perfNavID("drawer"), func() bool { return true })
				if Button(c, "打开抽屉").Clicked() {
					*open = true
				}
				Drawer(c, open, "基础抽屉").Size(420).Body(func() {
					Text(c, "抽屉内容 Drawer content")
					Text(c, "支持任意组件。")
				})
			},
			Interact: perfNavReopen("打开抽屉"),
		},
		perfFixture{
			// A custom anchored panel on the overlay primitive, dismissed by
			// Escape or an outside press like the built-in popups.
			Name: "Overlay", Overlay: true,
			Build: func(c *Context) {
				open := State(c, perfNavID("overlay"), func() bool { return true })
				btn := Button(c, "自定义浮层")
				if btn.Clicked() {
					*open = !*open
				}
				if !*open {
					return
				}
				ov := Overlay(c, OverlayOptions{Anchor: btn.ID(), Place: BottomStart, Offset: 4}, func() {
					Text(c, "自定义浮层内容 Overlay content")
					Text(c, "Escape 或点击外部关闭").Tertiary()
				}, layout.Pad(12), layout.Gap(4)).Bg(c.Theme.BgElevated).Radius(c.Theme.RadiusLG).Shadow(c.Theme.ShadowSecondary)
				if c.Dismissed(ov.ID(), btn.ID()) {
					*open = false
					c.Invalidate()
				}
			},
			Interact: perfNavReopen("自定义浮层"),
		},
		perfFixture{
			// Message closes through MessageDestroy (a button, as in the
			// gallery) and reopens from its trigger button.
			Name: "Message", Overlay: true, Covers: []string{"MessageDestroy"},
			Build: func(c *Context) {
				open := func() { Message(c, "success", "规则已保存 Rule saved").Key("perf").Duration(0) }
				perfNavOnce(c, "message", open)
				Row(c, func() {
					if Button(c, "显示消息").Clicked() {
						open()
					}
					if Button(c, "关闭消息").Clicked() {
						MessageDestroy(c, "perf")
					}
				}, layout.Gap(8))
			},
			Interact: perfNavSwitch("关闭消息", "显示消息"),
		},
		perfFixture{
			Name: "Notification", Overlay: true, Covers: []string{"NotificationDestroy"},
			Build: func(c *Context) {
				open := func() {
					Notification(c, "info", "版本更新 Update").Description("新版本已下载完成，是否立即重启？").Key("perf").Duration(0)
				}
				perfNavOnce(c, "notification", open)
				Row(c, func() {
					if Button(c, "显示通知").Clicked() {
						open()
					}
					if Button(c, "关闭通知").Clicked() {
						NotificationDestroy(c, "perf")
					}
				}, layout.Gap(8))
			},
			Interact: perfNavSwitch("关闭通知", "显示通知"),
		},
		perfFixture{
			Name: "NotificationCenter",
			Build: func(c *Context) {
				items := State(c, perfNavID("notify.items"), func() []NotificationItem {
					now := c.Now()
					return []NotificationItem{
						{Key: "build", Group: "今天", Level: "error", Title: "构建失败", Body: "go build ./...：未定义的标识符 frame。", Time: now.Add(-2 * time.Minute)},
						{Key: "update", Group: "今天", Level: "info", Title: "有可用更新", Body: "Tango 0.4.0 已发布，重启后安装。", Time: now.Add(-40 * time.Minute)},
						{Key: "push", Group: "更早", Level: "success", Title: "推送完成", Body: "已推送 3 个提交到 origin/main。", Time: now.Add(-3 * time.Hour), Read: true},
					}
				})
				NotificationCenter(c, items).Placement("bottomLeft")
			},
			Interact: perfNavPopup("通知"),
		},
		perfFixture{
			// Escape cancels the dialog; the delete button opens it again.
			// OpenDialogs drives the counter line.
			Name: "Confirm", Overlay: true, Covers: []string{"OpenDialogs"},
			Build: func(c *Context) {
				open := func() {
					Confirm(c, ConfirmOptions{Title: "确定删除这条规则吗？", Content: "删除后不可恢复 This cannot be undone.", Danger: true})
				}
				perfNavOnce(c, "confirm", open)
				if Button(c, "删除规则").Danger().Clicked() {
					open()
				}
				Text(c, "打开的对话框："+strconv.Itoa(OpenDialogs(c))).Tertiary()
			},
			Interact: perfNavReopen("删除规则"),
		},
		perfFixture{
			// OK dismisses the alert; its trigger opens it again.
			Name: "MessageBox", Overlay: true,
			Build: func(c *Context) {
				open := func() {
					MessageBox(c, ConfirmOptions{Title: "发布成功", Content: "版本 1.2 已上线 Version 1.2 is live.", Status: "success", OkText: "知道了"})
				}
				perfNavOnce(c, "messagebox", open)
				if Button(c, "查看结果").Clicked() {
					open()
				}
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					perfNavClick(r, "知道了")
				} else {
					perfNavClick(r, "查看结果")
				}
				r.D.Settle()
			},
		},
		perfFixture{
			Name: "Prompt", Overlay: true,
			Build: func(c *Context) {
				open := func() {
					Prompt(c, ConfirmOptions{Title: "重命名", Content: "请输入新的规则名称", InputValue: "TradeCode 1", InputPlaceholder: "规则名称"})
				}
				perfNavOnce(c, "prompt", open)
				if Button(c, "重命名").Clicked() {
					open()
				}
			},
			Interact: perfNavReopen("重命名"),
		},
		perfFixture{
			// Next / previous step through the tour's own buttons.
			Name: "Tour", Overlay: true,
			Build: func(c *Context) {
				open := State(c, perfNavID("tour.open"), func() bool { return true })
				cur := State[int](c, perfNavID("tour.cur"), nil)
				var ids [3]ID
				Row(c, func() {
					ids[0] = Button(c, "上传").ID()
					ids[1] = Button(c, "保存").Primary().ID()
					ids[2] = Button(c, "更多操作").ID()
				}, layout.Gap(8))
				Tour(c, open, cur, []TourStep{
					{Target: ids[0], Title: "上传文件", Description: "把文件放在这里。"},
					{Target: ids[1], Title: "保存修改", Description: "保存你的修改，支持快捷键 Ctrl+S。"},
					{Target: ids[2], Title: "其他操作", Description: "更多操作在这里。"},
				}).Show()
			},
			Interact: perfNavSwitch("下一步", "上一步"),
		},
		perfFixture{
			// Typing filters the palette; Backspace restores the list.
			Name: "Command", Build: perfNavCommand(0),
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					// The search field is the 44px row at the top of the palette.
					r.D.Click(geom.Pt(r.Target.X+120, r.Target.Y+22))
					r.D.Type("设")
					return
				}
				r.D.Key(input.KeyBackspace, 0)
				r.D.Click(perfOutside)
			},
			Large: perfNavCommand,
		},
	)
}

// perfNavContextArea is the dashed region a context menu belongs to.
func perfNavContextArea(c *Context, label string) ID {
	area := Column(c, func() { Text(c, label).Tertiary() }, layout.Size(320, 160), layout.Center()).
		DashedBorder(1, c.Theme.Border).Radius(c.Theme.RadiusLG)
	area.Focusable(input.CursorText)
	return area.ID()
}

// perfNavContextMenu right-clicks inside the area on even steps; odd
// steps close the menu with Escape, or with a wheel over the area.
func perfNavContextMenu(wheel bool) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		p := geom.Pt(r.Target.X+40, r.Target.Y+40)
		if i%2 == 0 {
			r.D.RightClick(p)
			return
		}
		if wheel { // over the area, beside the menu opened at p
			r.D.Scroll(geom.Pt(r.Target.X+300, r.Target.Y+140), 40)
		} else {
			r.D.Key(input.KeyEscape, 0)
		}
		r.D.Move(perfOutside)
	}
}

// perfNavFloat is the bottom-most float button: a lone FloatButton, or
// the trigger of a FloatButtonGroup (its menu opens above it). Float
// buttons carry the hit label "float".
func perfNavFloat(r *perfRun) geom.Rect {
	var best geom.Rect
	found := false
	for _, h := range r.D.r.c.hits {
		if h.label == "float" && (!found || h.rect.Y > best.Y) {
			best, found = h.rect, true
		}
	}
	if !found {
		panic("perf: float button not found")
	}
	return best
}

// perfNavInternal returns the retained component states of type T in the
// driver's Context (read only, for assertions).
func perfNavInternal[T any](d *Driver) []*T {
	var out []*T
	for _, e := range d.r.c.states {
		if v, ok := e.v.(*T); ok {
			out = append(out, v)
		}
	}
	return out
}

func perfNavHas(d *Driver, text string) string {
	if _, ok := perfNavShown(d, text); ok {
		return "yes"
	}
	return "no"
}

// perfNavShown is FindText restricted to visible nodes: a node inside a
// fully faded group (a toast leaving under NoAnimations stays in the tree
// at opacity ~0 until its exit time) does not count, and the search goes
// on to the next match in the same layer. Layers are searched top-most
// first, exact labels before containing ones.
func perfNavShown(d *Driver, text string) (geom.Rect, bool) {
	c := d.r.c
	roots := []*node{}
	for i := len(c.overlays) - 1; i >= 0; i-- {
		roots = append(roots, c.overlays[i].root)
	}
	if c.root != nil {
		roots = append(roots, c.root)
	}
	var find func(n *node, exact bool) *node
	find = func(n *node, exact bool) *node {
		if n.opacity > 0 && n.opacity < 0.01 {
			return nil // the whole faded subtree is invisible
		}
		if !n.abs.Empty() && n.label != "" && (n.label == text || !exact && strings.Contains(n.label, text)) {
			return n
		}
		for _, k := range n.kids {
			if m := find(k, exact); m != nil {
				return m
			}
		}
		return nil
	}
	for _, exact := range []bool{true, false} {
		for _, r := range roots {
			if n := find(r, exact); n != nil {
				return n.abs, true
			}
		}
	}
	return geom.Rect{}, false
}

// Post-merge catch-up (task M1): navigation entries added upstream.

var perfNavTopMenu = []MenuItem{{Key: "workplace", Label: "工作台"}, {Key: "analysis", Label: "分析页"}, {Key: "monitor", Label: "监控页"}}

var perfNavTableActions = []MenuItem{{Key: "copy", Label: "复制行"}, {Key: "archive", Label: "归档"}, {Key: "delete", Label: "删除行", Danger: true}}

func init() {
	registerPerf(
		perfFixture{
			// As Breadcrumb: a crumb navigates up, the link back down. The
			// route items are given with Items (no enclosing ProLayout).
			Name: "ProBreadcrumb",
			Build: func(c *Context) {
				path := State(c, perfNavID("procrumbs"), func() []string { return append([]string(nil), perfNavCrumbs...) })
				items := make([]BreadcrumbItem, len(*path))
				for i, s := range *path {
					items[i] = BreadcrumbItem{Label: s}
				}
				if i := ProBreadcrumb(c).Items(items).Show().Clicked(); i >= 0 && i < len(*path)-1 {
					*path = append([]string(nil), (*path)[:i+1]...)
					c.Invalidate()
				}
				if Link(c, "进入某应用").Clicked() {
					*path = append([]string(nil), perfNavCrumbs...)
					c.Invalidate()
				}
			},
			Interact: perfNavSwitch("应用中心", "进入某应用"),
		},
		perfFixture{
			Name: "TopNavHeader",
			Build: func(c *Context) {
				sel := State(c, perfNavID("topnav"), func() string { return "workplace" })
				TopNavHeader(c, perfNavTopMenu, sel).Title("Tango").UserName("管理员").Show()
			},
			Interact: perfNavSwitch("分析页", "工作台"),
		},
		perfFixture{
			// The row-action menu opens on hover (TableDropdown's default
			// trigger) over its ellipsis button, the fixture's only node.
			Name:  "TableDropdown",
			Build: func(c *Context) { TableDropdown(c, perfNavTableActions).Show() },
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Move(r.Center())
				} else {
					r.D.Move(perfOutside)
				}
				r.D.Settle()
			},
		},
	)
}

// TestPerfNavOverlayOpenClose checks every popup / overlay fixture through
// its own Interact: the identifying content is (not) shown at rest, step 0
// flips it, step 1 restores it, and when shown it lies in an overlay's
// visible content (perfOverlayContent).
func TestPerfNavOverlayOpenClose(t *testing.T) {
	tests := []struct {
		name, text string
		openAtRest bool
	}{
		{"Menubar", "新建窗口", false},
		{"BreadcrumbItems", "应用一", false},
		{"Dropdown", "系统设置", false},
		{"TableDropdown", "归档", false},
		{"ContextMenuAt", "复制", false},
		{"ContextMenuAtWith", "复制", false},
		{"NotificationCenter", "构建失败", false},
		{"Drawer", "抽屉内容 Drawer content", true},
		{"Overlay", "自定义浮层内容 Overlay content", true},
		{"Message", "规则已保存 Rule saved", true},
		{"Notification", "新版本已下载完成，是否立即重启？", true},
		{"Confirm", "确定删除这条规则吗？", true},
		{"MessageBox", "版本 1.2 已上线 Version 1.2 is live.", true},
		{"Prompt", "请输入新的规则名称", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			check := func(when string, want bool) {
				t.Helper()
				r, ok := perfNavShown(d, tc.text)
				if ok != want {
					t.Fatalf("%s: %q shown = %v, want %v", when, tc.text, ok, want)
				}
				if !ok {
					return
				}
				// Inside what an overlay shows, not merely inside a
				// transparent full-window carrier layer.
				for _, o := range d.r.c.overlays {
					if perfOverlayContent(o).Intersect(r) == r {
						return
					}
				}
				t.Fatalf("%s: %q is not inside an overlay's visible content", when, tc.text)
			}
			check("at rest", tc.openAtRest)
			r := &perfRun{D: d}
			perfStep(r, fx.Interact, 0)
			check("after step 0", !tc.openAtRest)
			perfStep(r, fx.Interact, 1)
			check("after step 1", tc.openAtRest)
			perfStep(r, fx.Interact, 2)
			check("after step 2", !tc.openAtRest)
		})
	}
}

// TestPerfNavInteractOutcome checks the component-specific result of each
// non-overlay interaction (retained value, highlight, visible text) at
// rest, after step 0 and after step 1.
func TestPerfNavInteractOutcome(t *testing.T) {
	str := func(key string) func(*Driver) string {
		return func(d *Driver) string { return *State[string](d.r.c, perfNavID(key), nil) }
	}
	num := func(key string) func(*Driver) string {
		return func(d *Driver) string { return strconv.Itoa(*State[int](d.r.c, perfNavID(key), nil)) }
	}
	has := func(text string) func(*Driver) string { return func(d *Driver) string { return perfNavHas(d, text) } }
	tests := []struct {
		name  string
		probe func(*Driver) string
		want  [3]string
	}{
		{"Menu", str("menu"), [3]string{"profile", "form", "profile"}},
		{"Tabs", str("tabs"), [3]string{"1", "2", "1"}},
		{"Breadcrumb", func(d *Driver) string {
			return strings.Join(*State[[]string](d.r.c, perfNavID("crumbs"), nil), "/")
		}, [3]string{"首页/应用中心/应用列表/某应用", "首页/应用中心", "首页/应用中心/应用列表/某应用"}},
		{"ProBreadcrumb", func(d *Driver) string {
			return strings.Join(*State[[]string](d.r.c, perfNavID("procrumbs"), nil), "/")
		}, [3]string{"首页/应用中心/应用列表/某应用", "首页/应用中心", "首页/应用中心/应用列表/某应用"}},
		{"TopNavHeader", str("topnav"), [3]string{"workplace", "analysis", "workplace"}},
		{"Pagination", num("page"), [3]string{"1", "2", "1"}},
		{"Steps", num("steps"), [3]string{"1", "2", "1"}},
		{"Anchor", func(d *Driver) string {
			st := perfNavInternal[anchorState](d)
			if len(st) != 1 {
				return "anchors=" + strconv.Itoa(len(st))
			}
			return st[0].active
		}, [3]string{"basic", "static", "basic"}},
		{"Affix", func(d *Driver) string {
			st := perfNavInternal[affixState](d)
			if len(st) != 1 {
				return "affixes=" + strconv.Itoa(len(st))
			}
			return strconv.FormatBool(st[0].affixed)
		}, [3]string{"false", "true", "false"}},
		{"BackTop", func(d *Driver) string {
			_, ok := d.Find("回到顶部")
			return strconv.FormatBool(ok)
		}, [3]string{"false", "true", "false"}},
		{"FloatButton", func(d *Driver) string {
			// The retained flag and the button's label agree. An icon-only
			// float button's label is its accessible name (its hit label is
			// always "float"), so read it from the accessibility tree.
			if d.AccessibilityTree() == nil {
				d.EnableAccessibility()
			}
			_, closeShown := d.A11yFind(a11y.RoleButton, "关闭帮助")
			_, helpShown := d.A11yFind(a11y.RoleButton, "帮助")
			return strconv.FormatBool(*State[bool](d.r.c, perfNavID("float"), nil)) + "," +
				strconv.FormatBool(closeShown) + "," + strconv.FormatBool(helpShown)
		}, [3]string{"false,false,true", "true,true,false", "false,false,true"}},
		{"FloatButtonGroup", func(d *Driver) string {
			n := 0
			for _, h := range d.r.c.hits {
				if h.label == "float" {
					n++
				}
			}
			return strconv.Itoa(n)
		}, [3]string{"1", "3", "1"}},
		{"ContextMenuAt", has("菜单：打开"), [3]string{"no", "yes", "no"}},
		{"Confirm", func(d *Driver) string {
			for _, s := range []string{"打开的对话框：0", "打开的对话框：1"} {
				if _, ok := perfNavShown(d, s); ok {
					return s
				}
			}
			return "?"
		}, [3]string{"打开的对话框：1", "打开的对话框：0", "打开的对话框：1"}},
		{"Tour", func(d *Driver) string {
			for _, s := range []string{"上传文件", "保存修改", "其他操作"} {
				if _, ok := perfNavShown(d, s); ok {
					return s
				}
			}
			return "?"
		}, [3]string{"上传文件", "保存修改", "上传文件"}},
		{"Command", func(d *Driver) string {
			return perfNavHas(d, "日历 Calendar") + "," + perfNavHas(d, "设置 Settings")
		}, [3]string{"yes,yes", "no,yes", "yes,yes"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			r := &perfRun{D: d}
			for i, want := range tc.want {
				if i > 0 {
					perfStep(r, fx.Interact, i-1)
				}
				if got := tc.probe(d); got != want {
					t.Fatalf("after %d steps: got %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestPerfNavCommandLarge checks that the Large palette really lists the
// requested n commands (the first is shown, the list scrolls).
func TestPerfNavCommandLarge(t *testing.T) {
	for _, n := range []int{1000, 100000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			d := newPerfDriver(perfPage(perfRegistry["Command"].Large(n)), false)
			d.Settle()
			if _, ok := d.FindText("命令 Command 1"); !ok {
				t.Fatal("first command not rendered")
			}
			if got := len(perfNavInternal[commandState](d)); got != 1 {
				t.Fatalf("%d command palettes", got)
			}
			h := perfListHash(d)
			r := &perfRun{D: d}
			perfStep(r, perfScroll, 0)
			if perfListHash(d) == h {
				t.Fatal("wheel did not scroll the command list")
			}
		})
	}
}

// TestPerfNavCloseContextMenu shows that the ContextMenuAtWith fixture's
// wheel step closes the menu through CloseContextMenu: the same page
// without the call keeps the menu open after the wheel.
func TestPerfNavCloseContextMenu(t *testing.T) {
	fx := perfRegistry["ContextMenuAtWith"]
	noClose := func(c *Context) {
		area := perfNavContextArea(c, "编辑区右键")
		if p, ok := c.ContextMenuFor(area); ok {
			ContextMenuAtWith(c, area, p, perfNavContextItems, func(string) {}, ContextMenuOptions{NoAutoSelect: true})
		}
	}
	for _, tc := range []struct {
		name  string
		build func(*Context)
		want  string
	}{{"fixture", fx.Build, "no"}, {"without CloseContextMenu", noClose, "yes"}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPerfDriver(perfPage(tc.build), false)
			d.Settle()
			r := &perfRun{D: d}
			perfStep(r, fx.Interact, 0)
			if perfNavHas(d, "复制") != "yes" {
				t.Fatal("right click did not open the menu")
			}
			perfStep(r, fx.Interact, 1)
			if got := perfNavHas(d, "复制"); got != tc.want {
				t.Fatalf("menu shown after the wheel = %s, want %s", got, tc.want)
			}
		})
	}
}
