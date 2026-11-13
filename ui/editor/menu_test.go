package editor

import (
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// menuLabels returns the labels of the open context menu, "-" for a
// separator (nil when it is closed).
func menuLabels(ed *editDriver) []string {
	m := &(*ed.vw).vs.menu
	if !m.open {
		return nil
	}
	var out []string
	for _, it := range m.items {
		if it.Divider {
			out = append(out, "-")
			continue
		}
		l := it.Label
		if it.Shortcut != "" {
			l = withKey(l, it.Shortcut)
		}
		out = append(out, l)
	}
	return out
}

// withKey is how menuLabels writes an item with a Shortcut (the label
// itself is plain; the key is in ui.MenuItem.Shortcut).
func withKey(label, key string) string { return label + " [" + key + "]" }

// menuItem returns the shown item labelled label (searching submenus).
func menuItem(ed *editDriver, label string) (ui.MenuItem, bool) {
	var find func([]ui.MenuItem) (ui.MenuItem, bool)
	find = func(its []ui.MenuItem) (ui.MenuItem, bool) {
		for _, it := range its {
			if it.Label == label {
				return it, true
			}
			if x, ok := find(it.Children); ok {
				return x, true
			}
		}
		return ui.MenuItem{}, false
	}
	return find((*ed.vw).vs.menu.items)
}

// rightClick right-clicks at p and lets the menu open.
func (ed *editDriver) rightClick(p geom.Point) {
	ed.RightClick(p)
	ed.Advance(time.Second)
}

// macLabel is label with its macOS keybinding text.
func macLabel(label, keys string) string { return withKey(label, keys) }

// registerTestCommand registers a command for the test only.
func registerTestCommand(t *testing.T, cmd Command) {
	t.Helper()
	RegisterCommand(cmd)
	t.Cleanup(func() {
		commands.Lock()
		delete(commands.m, cmd.ID)
		commands.Unlock()
	})
}

// The built-in menu of a writable view, macOS keymap.
var builtinMenu = []string{
	macLabel("Select All", "⌘A"),
	macLabel("Toggle Line Comment", "⌘/"),
	"-",
	macLabel("Cut", "⌘X"),
	macLabel("Copy", "⌘C"),
	macLabel("Paste", "⌘V"),
}

func TestContextMenuItemsFiltered(t *testing.T) {
	goText(t)
	st := NewState("package main\n\nfunc main() {}\n", "a.go", theme.Base("vs"))
	ro := false
	extra := []MenuItem{
		// When false: neither the item nor its group's separator.
		{Command: "editor.action.selectAll", Label: "Python only", Group: "5_lang", When: "editorLangId == python"},
		// A command that is not registered.
		{Command: "test.notRegistered", Label: "Missing", Group: "6_missing"},
		// A submenu whose items are all hidden.
		{Label: "Empty submenu", Group: "7_sub", Submenu: []MenuItem{{Command: "test.notRegistered"}}},
		// A When that does not parse hides the item.
		{Command: "editor.action.selectAll", Label: "Bad when", Group: "8_bad", When: "a ==="},
	}
	ed := newEditDriver(st, 500, 300, func(w *ViewW) {
		pinMac(w)
		w.ReadOnly(ro).MenuItems(extra...)
	})
	ed.focus(t)

	ed.rightClick(ed.at(t, st, 0, 3))
	if got := menuLabels(ed); !slices.Equal(got, builtinMenu) {
		t.Fatalf("menu:\n got %q\nwant %q", got, builtinMenu)
	}
	ed.Key(input.KeyEscape, 0)
	if menuLabels(ed) != nil {
		t.Fatal("Esc did not close the menu")
	}
	if !(*ed.vw).Focused() {
		t.Error("the view lost focus after Esc")
	}

	// Read-only: Cut, Paste and Toggle Line Comment are hidden (when
	// "!editorReadonly"), and so is the command palette while it is not
	// registered.
	ro = true
	ed.Frame()
	ed.rightClick(ed.at(t, st, 0, 3))
	want := []string{macLabel("Select All", "⌘A"), "-", macLabel("Copy", "⌘C")}
	if got := menuLabels(ed); !slices.Equal(got, want) {
		t.Fatalf("read-only menu:\n got %q\nwant %q", got, want)
	}
	ed.Key(input.KeyEscape, 0)
	ro = false
	ed.Frame()

	// The command palette item appears once the host registers the
	// command, with its key.
	registerTestCommand(t, Command{ID: "editor.action.quickCommand", Label: "Show All Commands", Run: func(*State, any) bool { return true }})
	km := keymap.New(keymap.Mac, keymap.DefaultRules(keymap.Mac), []keymap.Rule{{Key: "f1", Command: "editor.action.quickCommand"}})
	ed2 := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(km) })
	ed2.focus(t)
	ed2.rightClick(ed2.at(t, st, 0, 3))
	want = append(slices.Clone(builtinMenu), "-", macLabel("Command Palette", "F1"))
	if got := menuLabels(ed2); !slices.Equal(got, want) {
		t.Fatalf("menu with the palette:\n got %q\nwant %q", got, want)
	}
}

func TestContextMenuOrder(t *testing.T) {
	goText(t)
	st := NewState("hello world\n", "a.txt", theme.Base("vs"))
	run := func(*State, any) bool { return true }
	for _, id := range []string{"test.a", "test.b", "test.c", "test.d", "test.e", "test.f", "test.g"} {
		registerTestCommand(t, Command{ID: id, Label: id, Run: run})
	}
	items := []MenuItem{
		{Command: "test.a", Label: "No group"},
		{Command: "test.b", Label: "Zed", Group: "5_mine", Order: 2},
		{Command: "test.c", Label: "Beta", Group: "5_mine", Order: 1},
		{Command: "test.d", Label: "Alpha", Group: "5_mine", Order: 1},
		{Command: "test.e", Label: "Go to", Group: MenuGroupNavigation, Order: 9},
		{Label: "More", Group: "5_mine", Order: 3, Submenu: []MenuItem{
			{Command: "test.g", Label: "Second", Group: "b"},
			{Command: "test.f", Label: "First", Group: "a"},
		}},
	}
	ed := newEditDriver(st, 500, 300, func(w *ViewW) {
		pinMac(w)
		w.MenuItems(items...)
	})
	ed.focus(t)
	ed.rightClick(ed.at(t, st, 0, 3))
	want := []string{
		"Go to", "-",
		macLabel("Select All", "⌘A"), macLabel("Toggle Line Comment", "⌘/"), "-",
		"Alpha", "Beta", "Zed", "More", "-",
		macLabel("Cut", "⌘X"), macLabel("Copy", "⌘C"), macLabel("Paste", "⌘V"), "-",
		"No group",
	}
	if got := menuLabels(ed); !slices.Equal(got, want) {
		t.Fatalf("order:\n got %q\nwant %q", got, want)
	}
	// The submenu is sorted and grouped too.
	var sub []string
	for _, it := range (*ed.vw).vs.menu.items {
		if it.Label == "More" {
			for _, k := range it.Children {
				sub = append(sub, k.Label)
				if k.Divider {
					sub[len(sub)-1] = "-"
				}
			}
		}
	}
	if want := []string{"First", "-", "Second"}; !slices.Equal(sub, want) {
		t.Errorf("submenu: %q, want %q", sub, want)
	}
}

func TestContextMenuRunsCommand(t *testing.T) {
	goText(t)
	doc := "hello world\nsecond line\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)

	// A right click inside the selection keeps it; Copy copies it.
	st.SetSelections([]Selection{sel(6, 11)})
	ed.Frame()
	ed.rightClick(ed.at(t, st, 0, 8))
	if st.Primary() != sel(6, 11) {
		t.Fatalf("right click in the selection moved it: %+v", st.Primary())
	}
	r, ok := ed.Find("Copy")
	if !ok {
		t.Fatalf("no Copy item; menu %q", menuLabels(ed))
	}
	ed.Click(r.Center())
	ed.Advance(time.Second)
	if got := ed.Clipboard(); got != "world" {
		t.Fatalf("clipboard after Copy: %q, want %q", got, "world")
	}
	if menuLabels(ed) != nil {
		t.Error("the menu stayed open after choosing Copy")
	}
	if !(*ed.vw).Focused() {
		t.Error("focus did not return to the view")
	}

	// A right click outside the selections puts a single caret there;
	// Paste (an edit) runs and reports Changed.
	st.SetSelections([]Selection{sel(0, 2), sel(12, 13)})
	ed.Frame()
	l1 := st.Snapshot().LineStart(1)
	ed.rightClick(ed.at(t, st, 1, 6))
	if len(st.Sels) != 1 || st.Primary() != sel(l1+6, l1+6) {
		t.Fatalf("right click outside the selections: %+v", st.Sels)
	}
	changes := ed.changes
	r, _ = ed.Find("Paste")
	ed.Click(r.Center())
	ed.Advance(time.Second)
	if got, want := st.Snapshot().String(), "hello world\nsecondworld line\n"; got != want {
		t.Fatalf("after Paste: %q, want %q", got, want)
	}
	if ed.changes != changes+1 {
		t.Errorf("Changed reported %d times for Paste, want 1", ed.changes-changes)
	}

	// Keyboard: no item is highlighted when the menu opens (Monaco), so
	// Enter runs nothing; Down highlights the first item and Enter runs it
	// (Select All).
	ed.rightClick(ed.at(t, st, 0, 1))
	before := st.Primary()
	ed.Key(input.KeyEnter, 0)
	if st.Primary() != before || menuLabels(ed) == nil {
		t.Fatalf("Enter with nothing highlighted: selection %+v (was %+v), menu %v", st.Primary(), before, menuLabels(ed))
	}
	ed.Key(input.KeyDown, 0)
	ed.Key(input.KeyEnter, 0)
	if n := st.Snapshot().Len(); st.Primary() != sel(0, n) {
		t.Fatalf("Down, Enter: selection %+v, want all", st.Primary())
	}
	if menuLabels(ed) != nil || !(*ed.vw).Focused() {
		t.Error("menu open or view unfocused after choosing with the keyboard")
	}

	// A press outside the menu closes it without running anything.
	ed.rightClick(ed.at(t, st, 0, 1))
	text := st.Snapshot().String()
	g := (*ed.vw).vs
	ed.Click(geom.Pt(g.text.X+g.text.W-4, g.text.Y+g.text.H-4))
	ed.Advance(time.Second)
	if menuLabels(ed) != nil || st.Snapshot().String() != text {
		t.Error("an outside press did not just close the menu")
	}

	// The gutter has no menu; neither has a view with ContextMenu off,
	// where the right click still moves the caret.
	ed.rightClick(geom.Pt(g.gutter.X+2, ed.at(t, st, 0, 0).Y))
	if menuLabels(ed) != nil {
		t.Error("menu opened on the gutter")
	}
	st.Options.ContextMenu = false
	ed.Frame()
	ed.rightClick(ed.at(t, st, 0, 4))
	if menuLabels(ed) != nil || st.Primary() != sel(4, 4) {
		t.Errorf("contextmenu off: menu %q, selection %+v", menuLabels(ed), st.Primary())
	}
	st.Options.ContextMenu = true
}

func TestContextMenuShowsKeybinding(t *testing.T) {
	goText(t)
	st := NewState("hello\n", "a.txt", theme.Base("vs"))
	// The labels follow the view's keymap: Windows keys, and a rebinding
	// of Copy (the user's rule, last, wins).
	km := keymap.New(keymap.Windows, keymap.DefaultRules(keymap.Windows), []keymap.Rule{{Key: "ctrl+shift+c", Command: "editor.action.clipboardCopyAction"}})
	ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(km) })
	ed.focus(t)
	ed.rightClick(ed.at(t, st, 0, 2))
	labels := menuLabels(ed)
	for _, want := range []string{
		withKey("Select All", "Ctrl+A"),
		withKey("Copy", "Ctrl+Shift+C"),
		withKey("Paste", "Ctrl+V"),
	} {
		if !slices.Contains(labels, want) {
			t.Errorf("no item %q in %q", want, labels)
		}
	}
	// A command without a key shows its label alone.
	registerTestCommand(t, Command{ID: "test.nokey", Label: "No Key", Run: func(*State, any) bool { return true }})
	ed.Key(input.KeyEscape, 0)
	ed2 := newEditDriver(st, 500, 300, func(w *ViewW) {
		w.Keymap(km).MenuItems(MenuItem{Command: "test.nokey", Group: "5_x"})
	})
	ed2.focus(t)
	ed2.rightClick(ed2.at(t, st, 0, 2))
	if !slices.Contains(menuLabels(ed2), "No Key") {
		t.Errorf("no plain %q in %q", "No Key", menuLabels(ed2))
	}
}

func TestRegisterMenuItemPanics(t *testing.T) {
	for name, it := range map[string]MenuItem{
		"empty":    {Label: "x"},
		"bad when": {Command: "x", When: "a ==="},
		"bad sub":  {Label: "s", Submenu: []MenuItem{{Label: "y"}}},
		"no label": {Submenu: []MenuItem{{Command: "x"}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			RegisterMenuItem("test.menu", it)
		}()
	}
	menuReg.Lock()
	delete(menuReg.m, "test.menu")
	menuReg.Unlock()
}

// TestContextMenuHostContext checks that the application's context keys
// (ViewW.Context) reach the items' When, the commands' Precondition and
// the keybinding labels.
func TestContextMenuHostContext(t *testing.T) {
	goText(t)
	st := NewState("hello\n", "a.txt", theme.Base("vs"))
	ran := 0
	registerTestCommand(t, Command{ID: "test.host", Label: "Host Only", Precondition: "myKey", Run: func(*State, any) bool { ran++; return true }})
	km := keymap.New(keymap.Mac, keymap.DefaultRules(keymap.Mac), []keymap.Rule{{Key: "f6", Command: "test.host", When: "myKey"}})
	my := true
	ed := newEditDriver(st, 500, 300, func(w *ViewW) {
		w.Keymap(km).Context(func(key string) (any, bool) {
			if key == "myKey" {
				return my, true
			}
			return nil, false
		}).MenuItems(
			MenuItem{Command: "test.host", Group: "5_host", When: "myKey"},
			MenuItem{Command: "test.host", Label: "Hidden", Group: "5_host", When: "!myKey"},
		)
	})
	ed.focus(t)
	ed.rightClick(ed.at(t, st, 0, 2))
	it, ok := menuItem(ed, "Host Only")
	if !ok || it.Disabled || it.Shortcut != "F6" {
		t.Fatalf("host item: %+v (shown %v); menu %q", it, ok, menuLabels(ed))
	}
	if _, ok := menuItem(ed, "Hidden"); ok {
		t.Error("item with When !myKey shown")
	}
	r, _ := ed.Find("Host Only")
	ed.Click(r.Center())
	ed.Advance(time.Second)
	if ran != 1 {
		t.Fatalf("host command ran %d times, want 1", ran)
	}

	// M1: a false Precondition shows the item disabled; clicking it runs
	// nothing and leaves the menu open.
	my = false
	ed.Frame()
	registerTestCommand(t, Command{ID: "test.off", Label: "Off", Precondition: "myKey", Run: func(*State, any) bool { ran++; return true }})
	ed2 := newEditDriver(st, 500, 300, func(w *ViewW) {
		w.Context(func(key string) (any, bool) { return my, key == "myKey" }).MenuItems(MenuItem{Command: "test.off", Group: "5_host"})
	})
	ed2.focus(t)
	ed2.rightClick(ed2.at(t, st, 0, 2))
	if it, ok := menuItem(ed2, "Off"); !ok || !it.Disabled {
		t.Fatalf("disabled item: %+v (shown %v)", it, ok)
	}
	r, _ = ed2.Find("Off")
	ed2.Click(r.Center())
	ed2.Advance(time.Second)
	if ran != 1 {
		t.Errorf("a disabled item ran its command")
	}
}

// TestContextMenuAtPointer checks that the menu opens at the pointer, in
// left-to-right and right-to-left scopes, and closes on the wheel.
func TestContextMenuAtPointer(t *testing.T) {
	goText(t)
	for _, rtl := range []bool{false, true} {
		st := NewState("hello world, a longer line of text\n", "a.txt", theme.Base("vs"))
		var vw *ViewW
		ed := &editDriver{vw: &vw}
		ed.Driver = ui.NewDriver(func(c *ui.Context) {
			body := func() { vw = View(c, st); pinMac(vw); vw.Changed() }
			if rtl {
				ui.ConfigProvider(c).Direction(ui.RTL).Body(body)
			} else {
				body()
			}
		}, ui.DriverOptions{Width: 600, Height: 300})
		ed.Frame()
		ed.Frame()
		ed.focus(t)
		p := ed.at(t, st, 0, 20)
		ed.rightClick(p)
		r, ok := ed.Find("Select All")
		if !ok {
			t.Fatalf("rtl %v: no menu", rtl)
		}
		edge := r.X - p.X // the panel's start edge is at the pointer
		if rtl {
			edge = p.X - r.Right()
		}
		if edge < 0 || edge > 16 || r.Y < p.Y || r.Y > p.Y+16 {
			t.Errorf("rtl %v: first item %+v for a click at %v", rtl, r, p)
		}
		away := geom.Pt(p.X-40, p.Y) // over the text, not the menu
		if rtl {
			away.X = p.X + 40
		}
		ed.Scroll(away, 30)
		ed.Settle()
		if menuLabels(ed) != nil {
			t.Errorf("rtl %v: the wheel did not close the menu", rtl)
		}
	}
}

// TestContextMenuAccessibility checks the open menu's semantics: menu
// items with names and roles, no unnamed control (no trigger node).
func TestContextMenuAccessibility(t *testing.T) {
	goText(t)
	st := NewState("hello world\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.EnableAccessibility()
	ed.Frame()
	ed.focus(t)
	ed.rightClick(ed.at(t, st, 0, 3))
	if menuLabels(ed) == nil {
		t.Fatal("menu not open")
	}
	if _, ok := ed.A11yFind(a11y.RoleMenuItem, "Copy"); !ok {
		t.Error("no menu item Copy in the accessibility tree")
	}
	// The view itself has no semantics yet (an unnamed focusable box,
	// outside this test's scope): only the menu's nodes are checked.
	view := (*ed.vw).vs.rect
	for _, is := range ed.AuditNoContrast().Issues {
		if is.Kind == ui.AuditMissingName && is.Interactive && is.Rect != view {
			t.Errorf("unnamed control: %v", is)
		}
	}
}
