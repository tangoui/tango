package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// focusedLabel returns the label of the focused node ("" if none).
func focusedLabel(d *Driver) string {
	c := d.Runner().Context()
	var found string
	var walk func(n *node)
	walk = func(n *node) {
		if n.id == c.focus {
			found = n.label
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return found
}

// findText returns the rect of any node (page or overlay) labelled label.
func findText(d *Driver, label string) (geom.Rect, bool) {
	c := d.Runner().Context()
	var r geom.Rect
	ok := false
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == label {
			r, ok = c.rects[n.id], true
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return r, ok
}

func press(d *Driver, k input.Key) {
	d.Key(k, 0)
	d.Frame()
}

func wantFocus(t *testing.T, d *Driver, label string) {
	t.Helper()
	if got := focusedLabel(d); got != label {
		t.Fatalf("focus on %q, want %q", got, label)
	}
}

var keyMenuItems = []MenuItem{
	{Key: "home", Label: "首页"},
	{Key: "sub", Label: "子菜单", Children: []MenuItem{{Key: "a", Label: "甲"}, {Key: "b", Label: "乙"}}},
	{Key: "off", Label: "禁用", Disabled: true},
	{Key: "about", Label: "关于"},
}

func TestMenuKeyboardInline(t *testing.T) {
	sel := ""
	d := navDriver(func(c *Context) { Menu(c, keyMenuItems, &sel).Width(240).Show() })
	d.Frame()
	press(d, input.KeyTab)
	wantFocus(t, d, "首页")
	press(d, input.KeyDown)
	wantFocus(t, d, "子菜单")
	press(d, input.KeyRight) // opens
	if _, ok := d.Find("甲"); !ok {
		t.Fatal("Right did not open the submenu")
	}
	press(d, input.KeyRight) // enters
	wantFocus(t, d, "甲")
	press(d, input.KeyDown)
	wantFocus(t, d, "乙")
	press(d, input.KeyEnter)
	if sel != "b" {
		t.Fatalf("Enter selected %q", sel)
	}
	press(d, input.KeyLeft) // back to the parent
	wantFocus(t, d, "子菜单")
	press(d, input.KeyLeft) // closes
	if _, ok := d.Find("甲"); ok {
		t.Fatal("Left did not close the submenu")
	}
	press(d, input.KeyDown) // skips the disabled item
	wantFocus(t, d, "关于")
	press(d, input.KeyDown) // wraps
	wantFocus(t, d, "首页")
	press(d, input.KeyUp)
	wantFocus(t, d, "关于")
}

func TestMenuKeyboardHorizontal(t *testing.T) {
	sel := ""
	var clicked string
	d := navDriver(func(c *Context) {
		if k := Menu(c, keyMenuItems, &sel).Mode(MenuHorizontal).Show().Clicked(); k != "" {
			clicked = k
		}
	})
	d.Frame()
	d.Move(geom.Pt(900, 500)) // pointer away from the bar
	press(d, input.KeyTab)
	wantFocus(t, d, "首页")
	press(d, input.KeyRight)
	wantFocus(t, d, "子菜单")
	press(d, input.KeyDown) // opens the dropdown and enters it
	d.Frame()
	wantFocus(t, d, "甲")
	press(d, input.KeyDown)
	wantFocus(t, d, "乙")
	press(d, input.KeyUp)
	wantFocus(t, d, "甲")
	press(d, input.KeyEscape)
	if _, ok := d.Find("甲"); ok {
		t.Fatal("Escape did not close the dropdown")
	}
	wantFocus(t, d, "子菜单")
	press(d, input.KeyEnter)
	d.Frame()
	wantFocus(t, d, "甲")
	press(d, input.KeyRight) // a leaf: move along the bar
	d.Frame()
	if _, ok := d.Find("甲"); ok {
		t.Fatal("Right on a leaf kept the dropdown open")
	}
	wantFocus(t, d, "关于")
	press(d, input.KeyLeft) // skips the disabled item
	wantFocus(t, d, "子菜单")
	press(d, input.KeyDown)
	d.Frame()
	press(d, input.KeyEnter)
	if sel != "a" || clicked != "a" {
		t.Fatalf("sel=%q clicked=%q", sel, clicked)
	}
	if _, ok := d.Find("乙"); ok {
		t.Fatal("dropdown stayed open after choosing")
	}
}

func TestMenuKeyboardVerticalPopup(t *testing.T) {
	sel := ""
	d := navDriver(func(c *Context) { Menu(c, keyMenuItems, &sel).Mode(MenuVertical).Width(200).Show() })
	d.Frame()
	d.Move(geom.Pt(900, 500))
	press(d, input.KeyTab)
	press(d, input.KeyDown)
	wantFocus(t, d, "子菜单")
	press(d, input.KeyRight)
	d.Frame()
	wantFocus(t, d, "甲")
	press(d, input.KeyLeft) // closes the pop-out and returns
	if _, ok := d.Find("甲"); ok {
		t.Fatal("Left did not close the pop-out")
	}
	wantFocus(t, d, "子菜单")
	press(d, input.KeyRight)
	d.Frame()
	press(d, input.KeyDown)
	press(d, input.KeyEnter)
	if sel != "b" {
		t.Fatalf("sel=%q", sel)
	}
}

func TestDropdownKeyboard(t *testing.T) {
	items := []MenuItem{{Key: "a", Label: "甲"}, {Key: "sub", Label: "子菜单", Children: []MenuItem{{Key: "x", Label: "叉"}, {Key: "y", Label: "乙"}}}}
	var got string
	d := navDriver(func(c *Context) {
		if k := Dropdown(c, items).Trigger(TriggerClick).Body(func() { Button(c, "打开") }).Clicked(); k != "" {
			got = k
		}
	})
	d.Frame()
	press(d, input.KeyTab)
	wantFocus(t, d, "打开")
	press(d, input.KeyDown) // opens and enters
	d.Frame()
	wantFocus(t, d, "甲")
	press(d, input.KeyDown)
	wantFocus(t, d, "子菜单")
	press(d, input.KeyRight)
	d.Frame()
	wantFocus(t, d, "叉")
	press(d, input.KeyLeft)
	wantFocus(t, d, "子菜单")
	if _, ok := d.Find("叉"); ok {
		t.Fatal("Left did not close the submenu")
	}
	press(d, input.KeyEnter) // Enter on a submenu opens it and enters
	d.Frame()
	wantFocus(t, d, "叉")
	press(d, input.KeyDown)
	press(d, input.KeyEnter)
	if got != "y" {
		t.Fatalf("chose %q", got)
	}
	d.Frame()
	if _, ok := d.Find("甲"); ok {
		t.Fatal("menu stayed open")
	}
	wantFocus(t, d, "打开")
	// Escape closes and returns focus to the trigger.
	press(d, input.KeyDown)
	d.Frame()
	press(d, input.KeyEscape)
	d.Frame()
	if _, ok := d.Find("甲"); ok {
		t.Fatal("Escape did not close")
	}
	wantFocus(t, d, "打开")
}

func TestDropdownButton(t *testing.T) {
	items := []MenuItem{{Key: "a", Label: "甲"}}
	var main bool
	var got string
	d := navDriver(func(c *Context) {
		dd := Dropdown(c, items).Trigger(TriggerClick).Primary().Button("提交")
		main = main || dd.ButtonClicked()
		if k := dd.Clicked(); k != "" {
			got = k
		}
	})
	d.Frame()
	d.ClickLabel("提交")
	if !main {
		t.Fatal("main part not clicked")
	}
	d.ClickLabel("dropdown-more")
	d.Frame()
	d.ClickLabel("甲")
	if got != "a" {
		t.Fatalf("menu chose %q", got)
	}
}

func TestMessageRefKeyAndDuration(t *testing.T) {
	var c0 *Context
	d := NewDriver(func(c *Context) { c0 = c; Text(c, "page") }, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	Message(c0, "loading", "保存中").Key("k").Duration(0)
	d.Frame()
	d.Advance(10 * time.Second)
	d.Frame()
	if _, ok := findText(d, "保存中"); !ok {
		t.Fatal("Duration(0) message expired")
	}
	Message(c0, "success", "已保存").Key("k")
	d.Frame()
	if n := len(c0.toasts().msgs); n != 1 {
		t.Fatalf("messages=%d", n)
	}
	if _, ok := findText(d, "已保存"); !ok {
		t.Fatal("message not updated")
	}
	d.Advance(MessageDuration + time.Second)
	d.Frame()
	d.Frame()
	if len(c0.toasts().msgs) != 0 {
		t.Fatal("updated message did not expire")
	}
}

func TestNotificationRefKey(t *testing.T) {
	var c0 *Context
	d := NewDriver(func(c *Context) { c0 = c; Text(c, "page") }, DriverOptions{Width: 960, Height: 600})
	d.Frame()
	Notification(c0, "info", "旧标题").Key("n").Duration(0)
	d.Frame()
	Notification(c0, "success", "新标题").Description("描述").Key("n").Placement(NotifyBottomLeft)
	d.Frame()
	if n := len(c0.navNotes().list); n != 1 {
		t.Fatalf("notes=%d", n)
	}
	if r, ok := findText(d, "新标题"); !ok || r.X > 300 || r.Y < 300 {
		t.Fatalf("not bottom-left: %v", r)
	}
	NotificationDestroy(c0, "n")
	d.Frame()
	d.Frame()
	if len(c0.navNotes().list) != 0 {
		t.Fatal("not destroyed")
	}
}

func TestSpinWrapAndLoading(t *testing.T) {
	spinning := true
	d := navDriver(func(c *Context) {
		Spin(c).Spinning(spinning).Tip("转圈").Wrap(func() { Button(c, "内容") })
		Loading(c, true).Tip("加载").Body(func() { Button(c, "区域") })
	})
	d.Frame()
	if _, ok := navFindNode(d, "转圈"); !ok {
		t.Fatal("nested spin indicator missing")
	}
	if _, ok := navFindNode(d, "加载"); !ok {
		t.Fatal("loading indicator missing")
	}
	// The masks block the content.
	clicked := false
	d2 := navDriver(func(c *Context) {
		Spin(c).Wrap(func() { clicked = clicked || Button(c, "内容").Clicked() })
	})
	d2.Frame()
	d2.ClickLabel("内容")
	if clicked {
		t.Fatal("content clickable while spinning")
	}
	spinning = false
	d.Frame()
	d.Settle()
	if _, ok := navFindNode(d, "转圈"); ok {
		t.Fatal("indicator shown after spinning stopped")
	}
}

// fakeWin counts Invalidate calls from any goroutine.
type fakeWin struct {
	Window
	mu sync.Mutex
	n  int
}

func (w *fakeWin) Invalidate() {
	w.mu.Lock()
	w.n++
	w.mu.Unlock()
}

func TestConfirmCloseFromGoroutine(t *testing.T) {
	var c0 *Context
	d := NewDriver(func(c *Context) { c0 = c; Text(c, "page") }, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	cancelled := 0
	ref := Confirm(c0, ConfirmOptions{Title: "确认", Content: "内容", OnCancel: func() { cancelled++ }})
	w := &fakeWin{Window: ref.win}
	ref.win = w
	d.Frame()
	if _, ok := findText(d, "内容"); !ok {
		t.Fatal("dialog not shown")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref.Loading(true)
			ref.Close()
		}()
	}
	// Frames run concurrently with the closing goroutines; the UI side
	// may also cancel through Escape at the same time.
	d.Key(input.KeyEscape, 0)
	for i := 0; i < 5; i++ {
		d.Frame()
		_ = ref.IsOpen()
	}
	wg.Wait()
	d.Frame()
	if ref.IsOpen() || OpenDialogs(c0) != 0 {
		t.Fatal("dialog still open")
	}
	if _, ok := findText(d, "内容"); ok {
		t.Fatal("dialog still drawn")
	}
	if cancelled > 1 {
		t.Fatalf("OnCancel ran %d times", cancelled)
	}
	w.mu.Lock()
	n := w.n
	w.mu.Unlock()
	if n < 4 {
		t.Fatalf("Invalidate called %d times", n)
	}
}
