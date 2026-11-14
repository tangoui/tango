package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

type menubarFixture struct {
	status, wrap bool
	zoom         string
	clicked      []string
	changed      int
	open         string
}

func (f *menubarFixture) menus() []MenubarMenu {
	return []MenubarMenu{
		{Label: "文件", Items: []MenubarItem{
			{Key: "new", Label: "新建", Shortcut: "Ctrl+N"},
			{Key: "open", Label: "打开"},
			{Divider: true},
			{Key: "share", Label: "共享", Children: []MenubarItem{{Key: "mail", Label: "邮件"}, {Key: "link", Label: "链接"}}},
			{Key: "print", Label: "打印", Disabled: true},
		}},
		{Label: "编辑", Items: []MenubarItem{
			{Key: "undo", Label: "撤销"},
			{Key: "redo", Label: "重做"},
		}},
		{Label: "格式", Disabled: true, Items: []MenubarItem{{Key: "bold", Label: "加粗"}}},
		{Label: "视图", Items: []MenubarItem{
			{Key: "status", Label: "状态栏", Kind: MenubarItemCheckbox, Checked: &f.status},
			{Key: "wrap", Label: "自动换行", Kind: MenubarItemCheckbox, Checked: &f.wrap},
			{Label: "缩放", Group: true, Children: []MenubarItem{
				{Key: "z100", Label: "100%", Kind: MenubarItemRadio, Radio: &f.zoom, Value: "100"},
				{Key: "z150", Label: "150%", Kind: MenubarItemRadio, Radio: &f.zoom, Value: "150"},
			}},
		}},
	}
}

func menubarDriver(f *menubarFixture) *Driver {
	f.zoom = "100"
	return navDriver(func(c *Context) {
		Column(c, func() {
			mb := Menubar(c, f.menus()).A11yLabel("应用菜单")
			if k := mb.Clicked(); k != "" {
				f.clicked = append(f.clicked, k)
			}
			if mb.Changed() {
				f.changed++
			}
			f.open = mb.OpenMenu()
			Button(c, "正文")
		})
	})
}

func TestMenubarClickAndHoverSwitch(t *testing.T) {
	f := &menubarFixture{}
	d := menubarDriver(f)
	d.Frame()
	if err := d.ClickLabel("文件"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if f.open != "文件" {
		t.Fatalf("open = %q, want 文件", f.open)
	}
	if _, ok := findText(d, "新建"); !ok {
		t.Fatal("file menu panel not shown")
	}
	// With a menu open, hovering a neighbouring trigger switches to it.
	r, _ := d.Find("编辑")
	d.Move(r.Center())
	d.Frame()
	if f.open != "编辑" {
		t.Fatalf("hover switch: open = %q, want 编辑", f.open)
	}
	if _, ok := findText(d, "新建"); ok {
		t.Fatal("file menu still shown after switching")
	}
	// Hovering a disabled trigger does not switch.
	r, _ = d.Find("格式")
	d.Move(r.Center())
	d.Frame()
	if f.open != "编辑" {
		t.Fatalf("disabled trigger opened: %q", f.open)
	}
	// Choosing an item reports it and closes the menu.
	if err := d.ClickLabel("重做"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if len(f.clicked) != 1 || f.clicked[0] != "redo" || f.open != "" {
		t.Fatalf("clicked %v open %q", f.clicked, f.open)
	}
	// A click on the open trigger closes it; outside closes too.
	d.ClickLabel("文件")
	d.Frame()
	d.ClickLabel("文件")
	d.Frame()
	if f.open != "" {
		t.Fatalf("second click did not close: %q", f.open)
	}
	d.ClickLabel("文件")
	d.Frame()
	d.ClickLabel("正文")
	d.Frame()
	if f.open != "" {
		t.Fatal("outside press did not close the menu")
	}
}

func TestMenubarCheckboxAndRadio(t *testing.T) {
	f := &menubarFixture{}
	d := menubarDriver(f)
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("视图")
	d.Frame()
	n, ok := d.A11yFind(a11y.RoleMenuItem, "状态栏")
	if !ok || !n.States.Has(a11y.Checkable) || n.States.Has(a11y.Checked) {
		t.Fatalf("status item: %+v %v", n, ok)
	}
	d.ClickLabel("状态栏")
	d.Frame()
	if !f.status || f.changed != 1 || f.open != "" {
		t.Fatalf("checkbox: status=%v changed=%d open=%q", f.status, f.changed, f.open)
	}
	d.ClickLabel("视图")
	d.Frame()
	if n, _ := d.A11yFind(a11y.RoleMenuItem, "状态栏"); !n.States.Has(a11y.Checked) {
		t.Fatal("checked state not exposed")
	}
	if n, _ := d.A11yFind(a11y.RoleMenuItem, "100%"); !n.States.Has(a11y.Checked) {
		t.Fatal("radio 100% not checked")
	}
	d.ClickLabel("150%")
	d.Frame()
	if f.zoom != "150" || f.changed != 2 {
		t.Fatalf("radio: zoom=%q changed=%d", f.zoom, f.changed)
	}
	// Choosing the checked radio again changes nothing.
	d.ClickLabel("视图")
	d.Frame()
	d.ClickLabel("150%")
	d.Frame()
	if f.zoom != "150" || f.changed != 2 {
		t.Fatalf("re-choosing radio: zoom=%q changed=%d", f.zoom, f.changed)
	}
}

func TestMenubarKeyboard(t *testing.T) {
	f := &menubarFixture{}
	d := menubarDriver(f)
	d.Frame()
	press(d, input.KeyTab)
	wantFocus(t, d, "文件")
	press(d, input.KeyRight)
	wantFocus(t, d, "编辑")
	press(d, input.KeyRight) // skips the disabled 格式
	wantFocus(t, d, "视图")
	press(d, input.KeyRight) // wraps
	wantFocus(t, d, "文件")
	press(d, input.KeyLeft)
	wantFocus(t, d, "视图")
	press(d, input.KeyHome)
	wantFocus(t, d, "文件")
	press(d, input.KeyE) // typeahead on the triggers: 编辑 starts with no latin letter
	wantFocus(t, d, "文件")
	press(d, input.KeyDown)
	if f.open != "文件" {
		t.Fatalf("Down did not open: %q", f.open)
	}
	wantFocus(t, d, "新建")
	press(d, input.KeyDown)
	wantFocus(t, d, "打开")
	press(d, input.KeyDown)
	wantFocus(t, d, "共享")
	press(d, input.KeyRight) // opens the submenu
	wantFocus(t, d, "邮件")
	press(d, input.KeyLeft) // back to the parent item
	wantFocus(t, d, "共享")
	press(d, input.KeyUp)
	press(d, input.KeyRight) // leaf at the root: next menu
	if f.open != "编辑" {
		t.Fatalf("Right on a leaf: open = %q, want 编辑", f.open)
	}
	wantFocus(t, d, "撤销")
	press(d, input.KeyLeft) // root panel: previous menu
	if f.open != "文件" {
		t.Fatalf("Left at root: open = %q", f.open)
	}
	wantFocus(t, d, "新建")
	press(d, input.KeyEnter)
	if len(f.clicked) != 1 || f.clicked[0] != "new" || f.open != "" {
		t.Fatalf("Enter: clicked %v open %q", f.clicked, f.open)
	}
	wantFocus(t, d, "文件")
	press(d, input.KeyEnter)
	if f.open != "文件" {
		t.Fatal("Enter on the trigger did not open")
	}
	wantFocus(t, d, "新建")
	press(d, input.KeyEscape)
	if f.open != "" {
		t.Fatal("Escape did not close")
	}
	wantFocus(t, d, "文件")
}

func TestMenubarHotkeys(t *testing.T) {
	f := &menubarFixture{}
	d := menubarDriver(f)
	d.Frame()
	d.ClickLabel("正文")
	d.Frame()
	wantFocus(t, d, "正文")
	press(d, input.KeyF10)
	wantFocus(t, d, "文件")
	press(d, input.KeyF10)
	wantFocus(t, d, "正文")
	// Alt pressed and released alone.
	press(d, input.KeyAltLeft)
	wantFocus(t, d, "文件")
	press(d, input.KeyEscape) // a closed bar entered by hotkey: back
	wantFocus(t, d, "正文")
	// Alt combined with another key does not toggle.
	d.Inject(input.KeyEvent{Key: input.KeyAltLeft, Down: true})
	d.Inject(input.KeyEvent{Key: input.KeyF, Down: true, Mods: input.ModAlt})
	d.Inject(input.KeyEvent{Key: input.KeyF, Mods: input.ModAlt})
	d.Inject(input.KeyEvent{Key: input.KeyAltLeft})
	d.Settle()
	d.Frame()
	wantFocus(t, d, "正文")
}

func TestMenubarA11yAndRTL(t *testing.T) {
	f := &menubarFixture{}
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Menubar(c, f.menus()).A11yLabel("应用菜单")
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	d.Frame()
	bar, ok := d.A11yFind(a11y.RoleMenuBar, "应用菜单")
	if !ok || !bar.States.Has(a11y.Horizontal) {
		t.Fatalf("menubar node: %+v %v", bar, ok)
	}
	file, ok := d.A11yFind(a11y.RoleMenuItem, "文件")
	if !ok || !file.States.Has(a11y.HasPopup) {
		t.Fatalf("trigger: %+v", file)
	}
	r1, _ := d.Find("文件")
	r2, _ := d.Find("编辑")
	if !(r1.X > r2.X) {
		t.Fatalf("RTL: first menu should be right of the second: %v %v", r1, r2)
	}
	press(d, input.KeyTab)
	wantFocus(t, d, "文件")
	press(d, input.KeyLeft) // RTL: Left is the next menu
	wantFocus(t, d, "编辑")
	d.ClickLabel("文件")
	d.Frame()
	n, _ := d.A11yFind(a11y.RoleMenuItem, "文件")
	if !n.States.Has(a11y.Expanded) || n.Controls == 0 {
		t.Fatalf("open trigger: states %v controls %v", n.States, n.Controls)
	}
	if p := d.AccessibilityTree().Node(n.Controls); p == nil || p.Role != a11y.RoleMenu {
		t.Fatalf("controlled node is not a menu: %+v", p)
	}
	// The panel opens under the trigger, aligned to its inline start
	// (the right edge in RTL).
	tr, _ := d.Find("文件")
	pr, _ := findText(d, "新建")
	if !(pr.Right() > tr.Right()-40 && pr.Y > tr.Bottom()) {
		t.Fatalf("RTL panel placement: trigger %v item %v", tr, pr)
	}
}

func TestMenubarTokens(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		k := th.Components.Menubar
		if k.Height <= 0 || k.TriggerHeight <= 0 || k.TriggerHeight > k.Height || k.IndicatorWidth < k.IndicatorSize {
			t.Errorf("%v: bad menubar token %+v", th.Flavor, k)
		}
		if k.TriggerColor.A == 0 || k.IndicatorColor.A == 0 {
			t.Errorf("%v: missing colors", th.Flavor)
		}
	}
	over := theme.AntDesign(theme.WithComponents(theme.Components{Menubar: theme.MenubarToken{Height: 48}}))
	if over.Components.Menubar.Height != 48 || over.Components.Menubar.TriggerHeight != 28 {
		t.Fatal("override not merged")
	}
}

// TestMenubarHotkeysAfterFocusedWidget: Alt and F10 reach the bar even when
// the focused widget is built before it and consumes the frame's keys
// (a focused button above a menubar placed lower on the page).
func TestMenubarHotkeysAfterFocusedWidget(t *testing.T) {
	f := &menubarFixture{zoom: "100"}
	d := navDriver(func(c *Context) {
		Column(c, func() {
			Button(c, "搜索")
			c.KeysFor(c.Focused()) // like a focused text widget: takes the frame's keys
			mb := Menubar(c, f.menus()).A11yLabel("应用菜单")
			f.open = mb.OpenMenu()
		})
	})
	d.Frame()
	press(d, input.KeyTab) // the button is the first tab stop
	wantFocus(t, d, "搜索")
	press(d, input.KeyF10)
	wantFocus(t, d, "文件")
	press(d, input.KeyF10)
	wantFocus(t, d, "搜索")
	press(d, input.KeyAltLeft)
	wantFocus(t, d, "文件")
}

// TestMenubarUpOpensAtLastItem: Up on a trigger opens its menu with the
// last enabled item focused (Down: the first).
func TestMenubarUpOpensAtLastItem(t *testing.T) {
	f := &menubarFixture{}
	d := menubarDriver(f)
	d.Frame()
	press(d, input.KeyTab)
	wantFocus(t, d, "文件")
	press(d, input.KeyUp)
	if f.open != "文件" {
		t.Fatalf("Up did not open: %q", f.open)
	}
	wantFocus(t, d, "共享") // 打印 is disabled
	press(d, input.KeyEscape)
	wantFocus(t, d, "文件")
	press(d, input.KeyDown)
	wantFocus(t, d, "新建")
}
