package ui

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

func ctxTestItems() []MenuItem {
	return []MenuItem{
		{Key: "cut", Label: "Cut", Shortcut: "Ctrl+X", Disabled: true},
		{Key: "copy", Label: "Copy", Shortcut: "Ctrl+C"},
		{Key: "paste", Label: "Paste", Shortcut: "Ctrl+V"},
		{Divider: true},
		{Key: "more", Label: "More", Children: []MenuItem{{Key: "sel", Label: "Select All", Shortcut: "Ctrl+A"}}},
	}
}

// ctxShortcutOps returns the text runs drawn in the shortcut color.
func ctxShortcutOps(d *Driver, col theme.Color) []paint.Text {
	var out []paint.Text
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && t.Color == col {
			out = append(out, t)
		}
	}
	return out
}

func ctxNear(a, b float32) bool { return math.Abs(float64(a-b)) < 0.51 }

// TestMenuShortcutColumn: Dropdown items draw the shortcut at the inline
// end (right in LTR, left in RTL) in the shortcut color, the panel is wide
// enough for it, Find still locates the item by its label and the
// shortcut is the node's keyboard shortcut, not part of its name.
func TestMenuShortcutColumn(t *testing.T) {
	for _, flavor := range []string{"antd", "shadcn"} {
		for _, rtl := range []bool{false, true} {
			th := theme.AntDesign()
			if flavor == "shadcn" {
				th = theme.Shadcn()
			}
			tk := th.Components.Dropdown
			items := []MenuItem{{Key: "copy", Label: "Copy", Shortcut: "Ctrl+Shift+C"}, {Key: "x", Label: "X"}}
			d := NewDriver(func(c *Context) {
				dir := LTR
				if rtl {
					dir = RTL
				}
				ConfigProvider(c).Direction(dir).Body(func() {
					Dropdown(c, items).Trigger(TriggerClick).Body(func() { Button(c, "Open") })
				})
			}, DriverOptions{Width: 600, Height: 400, Theme: th, Accessibility: true})
			d.Settle()
			d.ClickLabel("Open")
			r := mustFind(t, d, "Copy")
			ops := ctxShortcutOps(d, tk.ShortcutColor)
			if len(ops) != 1 {
				t.Fatalf("%s rtl=%v: %d shortcut runs", flavor, rtl, len(ops))
			}
			lay := text.Shape("Ctrl+Shift+C", text.Style{Size: tk.ShortcutFontSize, LineHeight: th.LineHeightFor(tk.ShortcutFontSize)}, text.Options{NoWrap: true})
			n := float32(len(lay.Glyphs))
			w := lay.Width + tk.ShortcutLetterSpacing*n
			o := ops[0]
			if rtl {
				if !ctxNear(o.Origin.X, r.X+tk.ItemPaddingInline) {
					t.Errorf("%s RTL: shortcut starts at %v, item %v", flavor, o.Origin.X, r)
				}
			} else if !ctxNear(o.Origin.X+w, r.Right()-tk.ItemPaddingInline) {
				t.Errorf("%s LTR: shortcut ends at %v, item %v", flavor, o.Origin.X+w, r)
			}
			if tk.ShortcutLetterSpacing > 0 && !ctxNear(o.Glyphs[len(o.Glyphs)-1].X-lay.Glyphs[len(lay.Glyphs)-1].X, tk.ShortcutLetterSpacing*(n-1)) {
				t.Errorf("%s: letter spacing not applied", flavor)
			}
			st := text.Style{Size: th.FontSize, LineHeight: th.LineHeightFor(th.FontSize)}
			if need := text.Width("Copy", st) + tk.ShortcutGap + w + 2*tk.ItemPaddingInline; r.W+0.5 < need {
				t.Errorf("%s: item width %v < %v", flavor, r.W, need)
			}
			node := mustNode(t, d, a11y.RoleMenuItem, "Copy")
			if node.Shortcut != "Ctrl+Shift+C" {
				t.Errorf("%s: a11y shortcut %q", flavor, node.Shortcut)
			}
			if x := mustNode(t, d, a11y.RoleMenuItem, "X"); x.Shortcut != "" {
				t.Errorf("shortcut leaked to %v", x)
			}
		}
	}
}

// TestMenuShortcutSubmenu: a Menu's pop-out submenu draws shortcuts with
// its own Popup* token.
func TestMenuShortcutSubmenu(t *testing.T) {
	th := theme.AntDesign()
	th.Components.Menu.PopupShortcutColor = theme.Hex("#123456")
	sel := ""
	items := []MenuItem{{Key: "f", Label: "File", Children: []MenuItem{{Key: "s", Label: "Save", Shortcut: "Ctrl+S"}}}}
	d := NewDriver(func(c *Context) { Menu(c, items, &sel).Mode(MenuVertical).Width(160).Show() },
		DriverOptions{Width: 600, Height: 400, Theme: th})
	d.Settle()
	d.Move(mustFind(t, d, "File").Center())
	d.Settle()
	r := mustFind(t, d, "Save")
	ops := ctxShortcutOps(d, theme.Hex("#123456"))
	if len(ops) != 1 || ops[0].Origin.X < r.X+r.W/2 {
		t.Fatalf("submenu shortcut runs %v in %v", ops, r)
	}
}

// TestMenuTypeahead: a letter moves to the next item starting with it.
func TestMenuTypeahead(t *testing.T) {
	items := []MenuItem{{Key: "a", Label: "Apple"}, {Key: "b", Label: "Banana"}, {Key: "c", Label: "blue"}}
	var picked string
	d := NewDriver(func(c *Context) {
		if k := Dropdown(c, items).Trigger(TriggerClick).Body(func() { Button(c, "Fruit") }).Clicked(); k != "" {
			picked = k
		}
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyB, 0)
	d.Key(input.KeyB, 0)
	d.Key(input.KeyB, input.ModCtrl) // shortcuts are not typeahead
	d.Key(input.KeyEnter, 0)
	if picked != "c" {
		t.Fatalf("picked %q", picked)
	}
}

type ctxPage struct {
	rtl    bool
	opts   ContextMenuOptions
	picked string
	area   ID
}

func (p *ctxPage) build(c *Context, items []MenuItem) {
	dir := LTR
	if p.rtl {
		dir = RTL
	}
	ConfigProvider(c).Direction(dir).Body(func() {
		Button(c, "Before")
		a := Column(c, func() {}, layout.Size(500, 300)).Semantics(a11y.RoleGroup, "Editor")
		a.Focusable(input.CursorText)
		p.area = a.ID()
		if pt, ok := c.ContextMenuFor(a.ID()); ok {
			ContextMenuAtWith(c, a.ID(), pt, items, func(k string) { p.picked = k }, p.opts)
		}
	})
}

func ctxDriver(p *ctxPage, items []MenuItem) *Driver {
	return NewDriver(func(c *Context) { p.build(c, items) }, DriverOptions{Width: 800, Height: 600, Accessibility: true})
}

// ctxPanel returns the rectangle of the open context menu panel.
func ctxPanel(t *testing.T, d *Driver) geom.Rect {
	t.Helper()
	st := State[contextMenuState](d.r.c, contextMenuKey, nil)
	if !st.open || len(st.panels) == 0 {
		t.Fatal("context menu not open")
	}
	return d.r.c.rects[st.panels[0]]
}

func TestContextMenuAt(t *testing.T) {
	p := &ctxPage{}
	d := ctxDriver(p, ctxTestItems())
	d.Settle()
	area := d.r.c.rects[p.area]
	at := geom.Pt(area.X+40, area.Y+30)
	d.Click(at) // focus the owner
	d.RightClick(at)
	if r := ctxPanel(t, d); r.X != at.X || r.Y != at.Y {
		t.Fatalf("panel %v, want top-left at %v", r, at)
	}
	if !ContextMenuOpen(d.r.c, p.area) {
		t.Fatal("ContextMenuOpen false")
	}
	// Focus lands on the first enabled item (Cut is disabled).
	if f := d.A11yFocused(); f == nil || f.Name != "Copy" || f.Role != a11y.RoleMenuItem {
		t.Fatalf("focus on open: %v", f)
	}
	menu := mustNode(t, d, a11y.RoleMenu, "")
	_ = menu
	// No trigger node: the only focusable nodes are the Before button, the
	// owner and the enabled menu items; no button besides Before.
	var focusable []string
	for _, n := range d.AccessibilityTree().Nodes() {
		if n.States.Has(a11y.Focusable) {
			focusable = append(focusable, n.Name)
		}
		if n.Role == a11y.RoleButton && n.Name != "Before" {
			t.Errorf("extra button %v", n)
		}
	}
	if len(focusable) != 5 { // Before, Editor, Copy, Paste, More
		t.Errorf("focusable nodes %q", focusable)
	}
	if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 || rep.Count(AuditNoRole) != 0 {
		t.Errorf("audit: %+v", rep.Issues)
	}
	c := mustNode(t, d, a11y.RoleMenuItem, "Copy")
	if c.Shortcut != "Ctrl+C" {
		t.Errorf("shortcut %q", c.Shortcut)
	}
	// Keyboard: End, Home, Down; Right enters the submenu, Left leaves it.
	d.Key(input.KeyEnd, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("End: %v", f)
	}
	d.Key(input.KeyRight, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Select All" {
		t.Fatalf("Right: %v", f)
	}
	d.Key(input.KeyLeft, 0)
	d.Key(input.KeyHome, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Copy" {
		t.Fatalf("Home: %v", f)
	}
	d.Key(input.KeyP, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Paste" {
		t.Fatalf("typeahead: %v", f)
	}
	// Escape closes and returns focus to the owner.
	d.Key(input.KeyEscape, 0)
	if ContextMenuOpen(d.r.c, p.area) {
		t.Fatal("Escape kept the menu open")
	}
	if d.r.c.focus != p.area {
		t.Fatalf("focus after Escape: %v", d.A11yFocused())
	}
	if _, ok := d.A11yFind(a11y.RoleMenu, ""); ok {
		t.Fatal("menu still in tree")
	}
	// Select by pointer: onSelect runs and focus returns to the owner.
	d.RightClick(at)
	d.Click(mustFind(t, d, "Paste").Center())
	if p.picked != "paste" || ContextMenuOpen(d.r.c, p.area) || d.r.c.focus != p.area {
		t.Fatalf("pointer select: picked=%q open=%v", p.picked, ContextMenuOpen(d.r.c, p.area))
	}
	// Select by keyboard in a submenu.
	d.RightClick(at)
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0)
	if p.picked != "sel" || d.r.c.focus != p.area {
		t.Fatalf("keyboard select: picked=%q", p.picked)
	}
	// Disabled items are not chosen.
	d.RightClick(at)
	d.Click(mustFind(t, d, "Cut").Center())
	if !ContextMenuOpen(d.r.c, p.area) || p.picked != "sel" {
		t.Fatal("disabled item chosen")
	}
	// A right press elsewhere on the owner moves the menu there.
	at2 := geom.Pt(area.X+200, area.Y+100)
	d.RightClick(at2)
	if r := ctxPanel(t, d); r.X != at2.X || r.Y != at2.Y {
		t.Fatalf("reopened at %v, want %v", r, at2)
	}
	// An outside press closes it; focus moved by the press stays.
	d.Click(mustFind(t, d, "Before").Center())
	if ContextMenuOpen(d.r.c, p.area) {
		t.Fatal("outside press kept the menu open")
	}
	// CloseContextMenu.
	d.RightClick(at)
	CloseContextMenu(d.r.c, p.area)
	d.Settle()
	if _, ok := d.Find("Copy"); ok {
		t.Fatal("CloseContextMenu kept the menu")
	}
}

// TestContextMenuAtRTL: in RTL the panel's top-right corner is at the
// point and the shortcut column is on the left.
func TestContextMenuAtRTL(t *testing.T) {
	p := &ctxPage{rtl: true}
	d := ctxDriver(p, ctxTestItems())
	d.Settle()
	area := d.r.c.rects[p.area]
	at := geom.Pt(area.X+300, area.Y+30)
	d.RightClick(at)
	r := ctxPanel(t, d)
	if r.Right() != at.X || r.Y != at.Y {
		t.Fatalf("RTL panel %v, want top-right at %v", r, at)
	}
	tk := d.r.c.Theme.Components.Dropdown
	item := mustFind(t, d, "Copy")
	ops := ctxShortcutOps(d, tk.ShortcutColor)
	found := false
	for _, o := range ops {
		if o.Origin.Y > item.Y && o.Origin.Y < item.Bottom() && ctxNear(o.Origin.X, item.X+tk.ItemPaddingInline) {
			found = true
		}
	}
	if !found {
		t.Fatalf("RTL shortcut not at the item's left: %v %v", item, ops)
	}
}

// TestContextMenuAtEdges: near the window's right / bottom edges the menu
// opens to the left of / above the point.
func TestContextMenuAtEdges(t *testing.T) {
	var owner ID
	open := true
	d := NewDriver(func(c *Context) {
		a := Column(c, func() {}, layout.Size(800, 600))
		owner = a.ID()
		if open {
			open = false
			ContextMenuAt(c, owner, geom.Pt(790, 590), ctxTestItems(), nil)
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Settle()
	r := ctxPanel(t, d)
	if r.Right() != 790 || r.Bottom() != 590 {
		t.Fatalf("edge panel %v", r)
	}
}

// TestContextMenuAtNoAutoSelect: with NoAutoSelect the menu opens with
// focus on the (named, non-Tab-stop) menu panel and no item highlighted;
// Enter does nothing, Down/Up/Home/End and typeahead reach items, and
// Escape returns focus to the owner.
func TestContextMenuAtNoAutoSelect(t *testing.T) {
	p := &ctxPage{opts: ContextMenuOptions{NoAutoSelect: true}}
	d := ctxDriver(p, ctxTestItems())
	d.Settle()
	area := d.r.c.rects[p.area]
	at := geom.Pt(area.X+40, area.Y+30)
	open := func() {
		t.Helper()
		d.Click(at)
		d.RightClick(at)
		if !ContextMenuOpen(d.r.c, p.area) {
			t.Fatal("menu not open")
		}
		f := d.A11yFocused()
		if f == nil || f.Role != a11y.RoleMenu || f.Name == "" {
			t.Fatalf("focus on open: %v", f)
		}
	}
	open()
	if r := ctxPanel(t, d); r.X != at.X || r.Y != at.Y {
		t.Fatalf("panel %v, want top-left at %v", r, at)
	}
	if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 || rep.Count(AuditNoRole) != 0 {
		t.Errorf("audit: %+v", rep.Issues)
	}
	// No item highlighted, and the panel is not a Tab stop.
	st := State[contextMenuState](d.r.c, contextMenuKey, nil)
	for _, id := range st.popup.ids {
		if d.r.c.focus == id {
			t.Fatal("an item is focused on open")
		}
	}
	if st.popup.root == 0 || d.r.c.focus != st.popup.root {
		t.Fatal("menu panel not focused")
	}
	for _, e := range d.r.c.focusList {
		if e.id == st.popup.root && e.tab {
			t.Fatal("menu panel is a Tab stop")
		}
	}
	// Enter and Space with nothing highlighted do nothing.
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeySpace, 0)
	if p.picked != "" || !ContextMenuOpen(d.r.c, p.area) {
		t.Fatalf("Enter with no highlight: picked=%q open=%v", p.picked, ContextMenuOpen(d.r.c, p.area))
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleMenu {
		t.Fatalf("focus after Enter: %v", f)
	}
	// Down highlights the first enabled item (Cut is disabled).
	d.Key(input.KeyDown, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Copy" {
		t.Fatalf("Down: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	if p.picked != "copy" || d.r.c.focus != p.area {
		t.Fatalf("Enter after Down: picked=%q", p.picked)
	}
	// Up highlights the last enabled item.
	open()
	d.Key(input.KeyUp, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("Up: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	// Home / End.
	open()
	d.Key(input.KeyEnd, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("End: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	open()
	d.Key(input.KeyHome, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Copy" {
		t.Fatalf("Home: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	// Typeahead from the panel.
	open()
	d.Key(input.KeyP, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Paste" {
		t.Fatalf("typeahead: %v", f)
	}
	d.Key(input.KeyM, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("typeahead from item: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	// Escape with nothing highlighted closes and returns focus to the owner.
	open()
	d.Key(input.KeyEscape, 0)
	if ContextMenuOpen(d.r.c, p.area) || d.r.c.focus != p.area {
		t.Fatalf("Escape: open=%v focus=%v", ContextMenuOpen(d.r.c, p.area), d.A11yFocused())
	}
	// Tab moves on from the panel's place like from an item: to the
	// first item, Shift+Tab back to the owner.
	open()
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Copy" {
		t.Fatalf("Tab: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	open()
	d.Key(input.KeyTab, input.ModShift)
	if d.r.c.focus != p.area {
		t.Fatalf("Shift+Tab: %v", d.A11yFocused())
	}
	d.Key(input.KeyEscape, 0)
	// Hover still highlights, and a click chooses.
	open()
	d.Move(mustFind(t, d, "Paste").Center())
	d.Settle()
	d.Click(mustFind(t, d, "Paste").Center())
	if p.picked != "paste" || d.r.c.focus != p.area {
		t.Fatalf("pointer select: picked=%q", p.picked)
	}
}
