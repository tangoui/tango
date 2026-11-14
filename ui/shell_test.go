package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// withPlatform runs fn with the shell components' platform conventions.
func withPlatform(t *testing.T, mac bool) {
	t.Helper()
	old := shellMac
	shellMac = mac
	t.Cleanup(func() { shellMac = old })
}

// shellAudit fails on unnamed controls and controls without a role or
// keyboard access.
func shellAudit(t *testing.T, d *Driver) {
	t.Helper()
	rep := d.AuditNoContrast()
	if rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		t.Fatalf("audit:\n%v", rep)
	}
}

// ---------------------------------------------------------------------------
// StatusBar

func TestStatusBarItems(t *testing.T) {
	clicks, static := 0, 0
	page := func(c *Context) {
		StatusBar(c).Left(func() {
			if StatusBarItem(c, "main").Tooltip("切换分支").Clicked() {
				clicks++
			}
			if StatusBarItem(c, "UTF-8").Static().Clicked() {
				static++
			}
			StatusBarItem(c, "").Badge(120).Tooltip("通知")
		}).Right(func() {
			StatusBarItem(c, "索引").Progress(0.5).Static()
		})
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 100})
	d.Frame()
	main := mustFind(t, d, "main")
	if main.H != c0(d).Theme.Components.StatusBar.Height {
		t.Fatalf("item height %v", main.H)
	}
	d.Click(main.Center())
	if clicks != 1 {
		t.Fatalf("clicks = %d", clicks)
	}
	if _, ok := d.FindText("切换分支"); !ok {
		t.Fatal("tooltip not shown while hovered")
	}
	if r, ok := d.FindText("UTF-8"); ok {
		d.Click(r.Center())
	}
	if static != 0 {
		t.Fatal("a static item reported a click")
	}
	// Keyboard: Tab reaches the clickable item, Enter activates it.
	d.Click(geom.Pt(590, 90))
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if clicks != 2 {
		t.Fatalf("keyboard clicks = %d", clicks-1)
	}
	// The right side ends at the bar's end.
	if r, ok := d.FindText("索引"); !ok || r.Right() < 500 {
		t.Fatalf("right item at %v", r)
	}

	d.EnableAccessibility()
	if _, ok := d.A11yFind(a11y.RoleToolbar, ZhCN.StatusBar.Label); !ok {
		t.Fatal("bar not exposed as a named toolbar")
	}
	n, ok := d.A11yFind(a11y.RoleButton, "通知")
	if !ok {
		t.Fatal("icon-only item not named by its tooltip")
	}
	if !strings.Contains(n.Description, "99+") {
		t.Fatalf("badge not described: %q", n.Description)
	}
	shellAudit(t, d)
}

func TestStatusBarRTL(t *testing.T) {
	page := func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			StatusBar(c).Left(func() { StatusBarItem(c, "start") }).Right(func() { StatusBarItem(c, "end") })
		})
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 100})
	d.Frame()
	s, e := mustFind(t, d, "start"), mustFind(t, d, "end")
	if !(s.X > e.X) {
		t.Fatalf("RTL: start %v should be right of end %v", s, e)
	}
}

// c0 returns the driver's context (theme access in tests).
func c0(d *Driver) *Context { return d.r.c }

// ---------------------------------------------------------------------------
// TitleBar / WindowControls

type testFrame struct {
	info   WindowFrameInfo
	moves  int
	closes int
	sets   []WindowState
}

func (f *testFrame) FrameInfo() WindowFrameInfo { return f.info }
func (f *testFrame) SetWindowState(s WindowState) {
	f.sets = append(f.sets, s)
	f.info.State = s
}
func (f *testFrame) CloseWindow()     { f.closes++ }
func (f *testFrame) StartWindowMove() { f.moves++ }

func TestTitleBarCaption(t *testing.T) {
	withPlatform(t, false)
	f := &testFrame{}
	var act WindowAction
	page := func(c *Context) {
		if a := TitleBar(c).Title("App").Frame(f).Tools(func() { IconButton(c, nil, "tool") }).Action(); a != "" {
			act = a
		}
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	loc := ZhCN
	// Caption buttons at the end, in order.
	minR, maxR, closeR := mustFind(t, d, loc.TitleBar.Minimize), mustFind(t, d, loc.TitleBar.Maximize), mustFind(t, d, loc.Close)
	if !(minR.X < maxR.X && maxR.X < closeR.X && closeR.Right() == 600) {
		t.Fatalf("caption buttons %v %v %v", minR, maxR, closeR)
	}
	// Dragging the empty bar moves the window.
	empty := geom.Pt(300, 18)
	d.Click(empty)
	if f.moves != 1 {
		t.Fatalf("moves = %d", f.moves)
	}
	// A second press soon after at the same place maximizes instead.
	d.Advance(100 * time.Millisecond)
	d.Click(empty)
	if f.moves != 1 || len(f.sets) != 1 || f.sets[0] != WindowMaximized || act != WindowActionMaximize {
		t.Fatalf("double click: moves %d sets %v act %q", f.moves, f.sets, act)
	}
	// Maximized: the middle button restores.
	d.Frame()
	d.Click(mustFind(t, d, loc.TitleBar.Restore).Center())
	if f.info.State != WindowNormal {
		t.Fatalf("restore: state %v", f.info.State)
	}
	d.Click(mustFind(t, d, loc.TitleBar.Minimize).Center())
	if f.info.State != WindowMinimized {
		t.Fatalf("minimize: state %v", f.info.State)
	}
	d.Click(mustFind(t, d, loc.Close).Center())
	if f.closes != 1 || act != WindowActionClose {
		t.Fatalf("close: %d %q", f.closes, act)
	}
	// Controls inside the bar keep their presses.
	moves := f.moves
	d.Click(mustFind(t, d, "tool").Center())
	if f.moves != moves {
		t.Fatal("pressing a tool moved the window")
	}
}

func TestTitleBarFrameInfo(t *testing.T) {
	withPlatform(t, true)
	f := &testFrame{info: WindowFrameInfo{SystemButtons: true}}
	page := func(c *Context) { TitleBar(c).Title("Doc").Frame(f) }
	d := NewDriver(page, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	if _, ok := d.Find(ZhCN.Close); ok {
		t.Fatal("drew buttons over the system's traffic lights")
	}
	// The title is centered.
	if r, ok := d.FindText("Doc"); !ok || r.X < 250 {
		t.Fatalf("title at %v", r)
	}

	// Self-drawn traffic lights; NoMaximize disables the zoom light.
	f2 := &testFrame{info: WindowFrameInfo{NoMaximize: true}}
	act := WindowAction("")
	page = func(c *Context) {
		if a := TitleBar(c).Frame(f2).Manual().Action(); a != "" {
			act = a
		}
	}
	d = NewDriver(page, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	closeR := mustFind(t, d, ZhCN.Close)
	if closeR.X > 40 {
		t.Fatalf("traffic lights not at the start: %v", closeR)
	}
	d.Click(closeR.Center())
	if act != WindowActionClose || f2.closes != 0 {
		t.Fatalf("manual close: act %q closes %d", act, f2.closes)
	}
	act = ""
	if r, ok := d.Find(ZhCN.Fullscreen); ok {
		d.Click(r.Center())
	}
	if act != "" || len(f2.sets) != 0 {
		t.Fatalf("disabled zoom acted: %q %v", act, f2.sets)
	}

	// A window with its system title bar gets no buttons.
	f3 := &testFrame{info: WindowFrameInfo{SystemTitleBar: true}}
	d = NewDriver(func(c *Context) { WindowControls(c).Frame(f3) }, DriverOptions{Width: 300, Height: 100})
	d.Frame()
	if _, ok := d.Find(ZhCN.Close); ok {
		t.Fatal("buttons drawn for a window with a system title bar")
	}
}

func TestWindowControlsA11y(t *testing.T) {
	withPlatform(t, false)
	d := NewDriver(func(c *Context) { TitleBar(c).Title("App").Frame(&testFrame{}) }, DriverOptions{Width: 600, Height: 100})
	d.EnableAccessibility()
	for _, name := range []string{ZhCN.TitleBar.Minimize, ZhCN.TitleBar.Maximize, ZhCN.Close} {
		if _, ok := d.A11yFind(a11y.RoleButton, name); !ok {
			t.Fatalf("button %q missing", name)
		}
	}
	shellAudit(t, d)
}

// ---------------------------------------------------------------------------
// NotificationCenter

func TestNotificationCenter(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	items := []NotificationItem{
		{Key: "a", Title: "构建失败", Body: "详情", Level: "error", Group: "今天", Time: now.Add(-2 * time.Minute),
			Actions: []NotificationAction{{Key: "log", Label: "查看日志"}}},
		{Key: "b", Title: "有更新", Level: "info", Group: "今天", Time: now.Add(-3 * time.Hour)},
		{Key: "c", Title: "已推送", Group: "更早", Read: true},
	}
	var clicked, action string
	changed := 0
	page := func(c *Context) {
		nc := NotificationCenter(c, &items).Placement("bottomLeft")
		if k, ok := nc.Clicked(); ok {
			clicked = k
		}
		if k, a, ok := nc.Action(); ok {
			action = k + ":" + a
		}
		if nc.Changed() {
			changed++
		}
	}
	d := NewDriver(page, DriverOptions{Width: 800, Height: 700})
	d.w.now = now
	d.Frame()
	open := func() {
		d.Click(mustFind(t, d, ZhCN.A11y.Notifications).Center())
	}
	open()
	if _, ok := d.FindText(ZhCN.NotificationCenter.Unread(2)); !ok {
		t.Fatal("unread count missing from the header")
	}
	if _, ok := d.FindText("2 分钟前"); !ok {
		t.Fatal("relative time missing")
	}
	if _, ok := d.FindText("更早"); !ok {
		t.Fatal("group heading missing")
	}
	// Activating an entry marks it read and closes the panel.
	d.Click(mustFind(t, d, "有更新").Center())
	if clicked != "b" || !items[1].Read || changed == 0 {
		t.Fatalf("activate: clicked %q read %v", clicked, items[1].Read)
	}
	if _, ok := d.FindText("构建失败"); ok {
		t.Fatal("panel stayed open")
	}
	open()
	d.Click(mustFind(t, d, "查看日志").Center())
	if action != "a:log" || !items[0].Read {
		t.Fatalf("action %q read %v", action, items[0].Read)
	}
	// Dismiss one, then clear all.
	items[0].Read = false
	open()
	d.Click(mustFind(t, d, ZhCN.NotificationCenter.MarkAllRead).Center())
	if items[0].Read != true {
		t.Fatal("mark all read")
	}
	d.Click(mustFind(t, d, ZhCN.NotificationCenter.Dismiss).Center())
	if len(items) != 2 || items[1].Key != "b" { // the last entry's button is found
		t.Fatalf("dismiss: %v", items)
	}
	d.Click(mustFind(t, d, ZhCN.NotificationCenter.ClearAll).Center())
	if len(items) != 0 {
		t.Fatalf("clear: %d left", len(items))
	}
	if _, ok := d.FindText(ZhCN.NotificationCenter.Empty); !ok {
		t.Fatal("empty state missing")
	}
	// Esc closes the panel (with the pointer away from the header's
	// tooltips, which would take the first Esc).
	d.Move(geom.Pt(790, 690))
	d.Key(input.KeyEscape, 0)
	if _, ok := d.FindText(ZhCN.NotificationCenter.Empty); ok {
		t.Fatal("Esc did not close the panel")
	}
}

func TestNotificationAgo(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{10 * time.Second, "刚刚"}, {5 * time.Minute, "5 分钟前"}, {3 * time.Hour, "3 小时前"}, {50 * time.Hour, "2 天前"}, {10 * 24 * time.Hour, "2026年11月4日"}} {
		if got := ZhCN.notifyAgo(now.Add(-c.d), now); got != c.want {
			t.Errorf("%v ago: %q, want %q", c.d, got, c.want)
		}
	}
	if got := EnUS.notifyAgo(now.Add(-5*time.Minute), now); got != "5m ago" {
		t.Errorf("en: %q", got)
	}
}

func TestNotificationCenterA11y(t *testing.T) {
	items := []NotificationItem{{Key: "a", Title: "构建失败", Level: "error"}}
	d := NewDriver(func(c *Context) { NotificationCenter(c, &items).Placement("bottomLeft") }, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Click(mustFind(t, d, ZhCN.A11y.Notifications).Center())
	d.EnableAccessibility()
	n, ok := d.A11yFind(a11y.RoleButton, "构建失败")
	if !ok || !strings.Contains(n.Description, ZhCN.NotificationCenter.Unread(1)) {
		t.Fatalf("entry %v", n)
	}
	shellAudit(t, d)
}

// ---------------------------------------------------------------------------
// HotkeyInput

func keyDown(d *Driver, k input.Key, m input.Modifiers) {
	d.Inject(input.KeyEvent{Key: k, Down: true, Mods: m})
	d.Settle()
}

func TestHotkeyInputRecord(t *testing.T) {
	withPlatform(t, false)
	v := "CmdOrCtrl+S"
	changed := 0
	var hk *HotkeyInputW
	page := func(c *Context) {
		hk = HotkeyInput(c, &v)
		if hk.Changed() {
			changed++
		}
		Button(c, "next")
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	field := c0(d).rects[hk.ID()]
	d.Click(geom.Pt(field.X+20, field.Y+field.H/2))
	if !hk.Recording() {
		t.Fatal("click did not start recording")
	}
	// Held modifiers show while recording.
	keyDown(d, input.KeyCtrlLeft, input.ModCtrl)
	keyDown(d, input.KeyShiftLeft, input.ModCtrl|input.ModShift)
	if _, ok := d.FindText("Ctrl"); !ok {
		t.Fatal("held Ctrl not shown")
	}
	keyDown(d, input.KeyK, input.ModCtrl|input.ModShift)
	if v != "CmdOrCtrl+Shift+K" || changed != 1 || hk.Recording() {
		t.Fatalf("recorded %q changed %d recording %v", v, changed, hk.Recording())
	}
	// Enter records again; a bare letter is not a shortcut, F5 is.
	d.Key(input.KeyEnter, 0)
	if !hk.Recording() {
		t.Fatal("Enter did not start recording")
	}
	keyDown(d, input.KeyA, 0)
	if v != "CmdOrCtrl+Shift+K" || !hk.Recording() {
		t.Fatalf("bare letter recorded: %q", v)
	}
	keyDown(d, input.KeyF5, 0)
	if v != "F5" {
		t.Fatalf("F5: %q", v)
	}
	// Alt+Numpad1 keeps the keypad key.
	d.Key(input.KeyEnter, 0)
	d.Inject(input.KeyEvent{Key: input.KeyNumpad1, Down: true, Mods: input.ModAlt})
	d.Settle()
	if v != "Alt+num1" {
		t.Fatalf("numpad: %q", v)
	}
	// Backspace clears.
	d.Key(input.KeyBackspace, 0)
	if v != "" {
		t.Fatalf("backspace: %q", v)
	}
	// Tab moves focus and stops recording.
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyTab, 0)
	if hk.Recording() || c0(d).focus == hk.ID() {
		t.Fatal("Tab was recorded")
	}
}

func TestHotkeyInputEscape(t *testing.T) {
	withPlatform(t, true)
	v, open := "CmdOrCtrl+P", true
	var hk *HotkeyInputW
	page := func(c *Context) {
		Modal(c, &open, "快捷键").NoFooter().Body(func() {
			hk = HotkeyInput(c, &v).Literal()
		})
	}
	d := NewDriver(page, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	field := c0(d).rects[hk.ID()]
	d.Click(geom.Pt(field.X+20, field.Y+field.H/2))
	keyDown(d, input.KeyMetaLeft, input.ModMeta)
	d.Key(input.KeyEscape, 0)
	if !open || hk.Recording() || v != "CmdOrCtrl+P" {
		t.Fatalf("Esc while recording: open %v recording %v value %q", open, hk.Recording(), v)
	}
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("second Esc did not reach the dialog")
	}
	// Literal stores Cmd as pressed.
	open = true
	d.Frame()
	d.Click(geom.Pt(field.X+20, field.Y+field.H/2))
	keyDown(d, input.KeyK, input.ModMeta|input.ModAlt)
	if v != "Alt+Cmd+K" {
		t.Fatalf("literal: %q", v)
	}
}

func TestHotkeyInputConflictAndClear(t *testing.T) {
	withPlatform(t, false)
	v := "CmdOrCtrl+S"
	page := func(c *Context) {
		Column(c, func() {
			HotkeyInput(c, &v).A11yLabel("保存").Conflict(func(a string) string {
				if a == "CmdOrCtrl+S" {
					return "已被占用"
				}
				return ""
			})
		}, layout.Pad(20))
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	if _, ok := d.FindText("已被占用"); !ok {
		t.Fatal("conflict message missing")
	}
	d.EnableAccessibility()
	n, ok := d.A11yFind(a11y.RoleTextbox, "保存")
	if !ok || n.Value != "Ctrl+S" || !strings.Contains(n.Description, "已被占用") {
		t.Fatalf("a11y %+v", n)
	}
	shellAudit(t, d)
	d.Click(mustFind(t, d, ZhCN.HotkeyInput.ClearLabel).Center())
	if v != "" {
		t.Fatalf("clear: %q", v)
	}
	if _, ok := d.FindText("已被占用"); ok {
		t.Fatal("conflict message stayed after clearing")
	}
}

func TestShortcutLabels(t *testing.T) {
	withPlatform(t, true)
	for in, want := range map[string]string{
		"CmdOrCtrl+Shift+P": "⇧⌘P", "Ctrl+Alt+Delete": "⌃⌥⌦", "Cmd+Up": "⌘↑", "F5": "F5", "Ctrl++": "⌃+", "Alt+num5": "⌥Num 5",
	} {
		if got := ShortcutLabel(in); got != want {
			t.Errorf("mac %q: %q, want %q", in, got, want)
		}
	}
	shellMac = false
	for in, want := range map[string]string{
		"CmdOrCtrl+Shift+P": "Ctrl+Shift+P", "Alt+F4": "Alt+F4", "Ctrl+esc": "Ctrl+Esc", "Bogus+K": "",
	} {
		if got := ShortcutLabel(in); got != want {
			t.Errorf("pc %q: %q, want %q", in, got, want)
		}
	}
}

// TestShellI18n renders the shell components in the four built-in
// languages (the i18n wiring check of i18n_wiring_test.go).
func TestShellI18n(t *testing.T) {
	withPlatform(t, false)
	var empty []NotificationItem
	hk := ""
	tc := i18nCase{name: "Shell",
		page: func(c *Context, ids i18nIDs) {
			TitleBar(c).Title("App").Frame(&testFrame{})
			ids["nc"] = NotificationCenter(c, &empty).Placement("bottomLeft").z.n.id
			HotkeyInput(c, &hk)
		},
		setup: func(d *Driver, ids i18nIDs) { i18nClick(d, ids, "nc") },
		texts: func(l *Locale) []string {
			return []string{l.TitleBar.Minimize, l.TitleBar.Maximize, l.Close, l.A11y.Notifications,
				l.NotificationCenter.MarkAllRead, l.NotificationCenter.ClearAll, l.NotificationCenter.Empty,
				l.HotkeyInput.Placeholder}
		}}
	for _, l := range i18nLocales {
		d, _ := i18nRender(tc, l)
		labels := i18nLabels(d)
		for _, s := range tc.texts(l) {
			if !labels[s] {
				t.Errorf("%s: %q not rendered", l.Code, s)
			}
		}
		if l == EnUS {
			for s := range labels {
				if strings.IndexFunc(s, isCJK) >= 0 && s != "App" {
					t.Errorf("en-US: CJK label %q", s)
				}
			}
		}
	}
}
