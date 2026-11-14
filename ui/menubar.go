package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// MenubarItemKind is the behaviour of a Menubar item. Open set: new kinds
// are added as new constants.
type MenubarItemKind string

const (
	MenubarItemNormal   MenubarItemKind = ""         // runs a command
	MenubarItemCheckbox MenubarItemKind = "checkbox" // toggles *Checked
	MenubarItemRadio    MenubarItemKind = "radio"    // sets *Radio to its value
)

// MenubarItem is an entry of a Menubar menu. Key identifies the item
// (Label is used when Key is empty) and must be unique within its menu.
type MenubarItem struct {
	Key      string
	Label    string
	Icon     *icon.Icon
	Shortcut string // display only, like MenuItem.Shortcut
	Disabled bool
	Danger   bool
	Divider  bool // a separator instead of an item
	// Group makes the entry a titled group of Children shown inline (a
	// radio group under a label, for example), like MenuItem.Group.
	Group bool
	Kind  MenubarItemKind
	// Checked is the value a checkbox item shows and toggles.
	Checked *bool
	// Radio is the value of the radio group the item belongs to: the item
	// is checked when *Radio equals Value (Key when Value is empty) and
	// choosing it stores that value.
	Radio *string
	Value string
	// Children open as a submenu (or, with Group, form the group).
	Children []MenubarItem
}

// MenubarMenu is a top-level menu of a Menubar ("File", "Edit" …).
type MenubarMenu struct {
	Key      string // identifies the menu (Label when empty)
	Label    string
	Disabled bool
	Items    []MenubarItem
}

type menubarState struct {
	open     string // key of the open menu ("": none)
	cur      int    // trigger that is the Tab stop
	popup    navPopupState
	panels   []ID
	altArmed bool // Alt went down with no other key since
	back     ID   // focus before Alt / F10 moved it into the bar
}

// MenubarW is an application menu bar inside the window (shadcn/ui
// Menubar; the menu bar of desktop applications): a row of top-level
// menus that open drop-down panels (the Dropdown panels) on click, or
// with Alt / F10 and the arrow keys.
type MenubarW struct {
	_       [0]func()
	c       *Context
	th      *theme.Theme
	tk      *theme.MenubarToken
	n       *node
	menus   []MenubarMenu
	st      *menubarState
	z       *lazy
	trig    []*node
	clicked string
	changed bool
	hot     bool // hotkey pressed this frame (Alt / F10)
}

// Menubar adds a menu bar with the given top-level menus:
//
//	mb := ui.Menubar(c, []ui.MenubarMenu{
//		{Label: "文件", Items: []ui.MenubarItem{{Key: "new", Label: "新建", Shortcut: "Ctrl+N"}}},
//		{Label: "视图", Items: []ui.MenubarItem{{Label: "状态栏", Kind: ui.MenubarItemCheckbox, Checked: &status}}},
//	})
//	switch mb.Clicked() { case "new": ... }
//
// A click on a trigger opens its menu; while a menu is open, moving the
// pointer onto another trigger switches to it. Alt (pressed and released
// alone) or F10 moves focus to the bar and back. Keyboard: Left/Right
// move between the menus (mirrored in RTL), Down/Enter/Space open one,
// Up/Down move in it, Right/Left open and close submenus, or at the edge
// switch to the neighbouring menu, Enter chooses, Escape closes, a letter
// jumps to the next item starting with it.
func Menubar(c *Context, menus []MenubarMenu) *MenubarW {
	m := &MenubarW{c: c, menus: menus}
	m.th = c.ThemeFor(theme.CompMenubar)
	m.tk = &m.th.Components.Menubar
	m.n = c.box("menubar", layout.Row, []layout.Option{layout.Middle(), layout.H(m.tk.Height),
		layout.PadVH(0, m.tk.PaddingInline), layout.Gap(m.tk.Gap)}).n
	if m.th.Flavor == theme.StyleShadcn {
		m.n.ln.NoStretch = true // shadcn: an inline bar (w-fit), not a strip
	}
	m.n.setRole(a11y.RoleMenuBar).setState(a11y.Horizontal)
	m.n.paint = m.paintBar
	m.st = State[menubarState](c, m.n.id, nil)
	// The bar reads the hotkeys before the page's focused widget takes
	// the keys of this frame (the triggers are built later, lazily).
	m.hotkeys()
	m.z = c.lazy(m.n, m.build)
	return m
}

// A11yLabel names the bar for assistive technology.
func (m *MenubarW) A11yLabel(s string) *MenubarW { m.n.setName(s); return m }

// Clicked returns the key of the item chosen this frame ("" if none).
// Checkbox and radio items have already updated their bound value.
func (m *MenubarW) Clicked() string { m.z.run(); return m.clicked }

// Changed reports that a checkbox or radio item changed its value this
// frame.
func (m *MenubarW) Changed() bool { m.z.run(); return m.changed }

// OpenMenu returns the key of the open menu ("" when all are closed).
func (m *MenubarW) OpenMenu() string { m.z.run(); return m.st.open }

// ID returns the bar's ID.
func (m *MenubarW) ID() ID { return m.n.id }

// menuKey returns the key of top-level menu i.
func (m *MenubarW) menuKey(i int) string {
	if k := m.menus[i].Key; k != "" {
		return k
	}
	return m.menus[i].Label
}

// hotkeys handles Alt (alone) and F10: they move focus into the bar, or
// out of it again (closing an open menu).
func (m *MenubarW) hotkeys() {
	c, st := m.c, m.st
	var rest []input.KeyEvent
	taken := false
	for _, k := range c.keys {
		hit := false
		switch {
		case k.Key == input.KeyAltLeft || k.Key == input.KeyAltRight:
			if k.Down {
				st.altArmed = true
			} else if st.altArmed {
				st.altArmed = false
				hit = true
			}
		case k.Key == input.KeyF10 && k.Mods&(input.ModShift|input.ModCtrl|input.ModAlt|input.ModMeta) == 0:
			st.altArmed = false
			if k.Down {
				m.hot = !m.hot
			}
			taken = true
			continue // F10 belongs to the bar
		default:
			if k.Down {
				st.altArmed = false
			}
		}
		if hit {
			m.hot = !m.hot
		}
		rest = append(rest, k)
	}
	if taken {
		c.keys = rest
	}
	if m.hot {
		c.invalid = true
	}
}

// inBar reports whether keyboard focus is on a trigger or in a panel.
func (m *MenubarW) inBar() bool {
	c := m.c
	for _, t := range m.trig {
		if t.id == c.focus {
			return true
		}
	}
	for _, id := range m.st.popup.ids {
		if id == c.focus {
			return true
		}
	}
	return false
}

func (m *MenubarW) build() {
	c, th, tk, st := m.c, m.th, m.tk, m.st
	m.trig = m.trig[:0]
	if st.cur >= len(m.menus) {
		st.cur = 0
	}
	if st.open != "" && m.indexOf(st.open) < 0 {
		m.close(false)
	}
	// Keyboard navigation suspends hover switching until the pointer
	// moves (panel() ends it then).
	hoverOK := !st.popup.kbd || c.pointer != st.popup.ptr
	font := c.style(tk.TriggerFontSize, themeWeight(tk.TriggerFontWeight))
	shad := th.Flavor == theme.StyleShadcn
	var ias []Interaction
	for i, mn := range m.menus {
		key := m.menuKey(i)
		t := c.add("menubar-trigger", mn.Label)
		label := mn.Label
		t.ln.NoStretch = true
		t.ln.Measure = func(_, _ float32) geom.Size {
			return geom.Size{W: text.Width(label, font) + 2*tk.TriggerPaddingInline, H: tk.TriggerHeight}
		}
		disabled := mn.Disabled || t.disabled
		ia := c.interact(t, !disabled, input.CursorPointer)
		if disabled {
			t.disabled, t.focus, t.cursor = true, false, input.CursorNotAllowed
		}
		open := st.open == key
		t.setRole(a11y.RoleMenuItem).setState(a11y.HasPopup|a11y.Expandable).stateIf(open, a11y.Expanded).stateIf(disabled, a11y.Disabled)
		if open && c.a11yOn {
			sem := st
			t.fill(semFunc(func(s *a11y.Node) { popupSem(s, true, sem.panels) }))
		}
		if ia.Focused {
			st.cur = i
		}
		id := t.id
		t.paint = func(l *paint.List, r geom.Rect) {
			o := c.Anim(id, "open", b2f(open), th.DurationFast)
			h := c.Anim(id, "hover", b2f((ia.Hovered || shad && ia.FocusVisible) && !disabled), th.DurationFast)
			rad := geom.Rad(tk.TriggerRadius)
			bg := theme.Fade(tk.TriggerHoverBg, float64(h))
			bg = theme.Lerp(bg, tk.TriggerOpenBg, o)
			l.Fill(r, rad, bg)
			col := theme.Lerp(tk.TriggerColor, tk.TriggerHoverColor, h)
			col = theme.Lerp(col, tk.TriggerOpenColor, o)
			if disabled {
				col = th.TextDisabled
			}
			lay := text.Shape(label, font, c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(c.inline(r).X(tk.TriggerPaddingInline, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			if ia.FocusVisible && !shad && !disabled {
				drawFocusRing(l, th, r, rad)
			}
		}
		m.trig = append(m.trig, t)
		ias = append(ias, ia)
	}
	rovingTabStop(m.trig, st.cur)

	if m.hot {
		if m.inBar() || st.open != "" {
			back := st.back
			m.close(false)
			if back != 0 {
				c.SetFocus(back)
			} else {
				c.SetFocus(0)
			}
		} else if j := m.step(-1, 1); j >= 0 {
			st.back = c.focus
			navFocus(c, m.trig[j].id)
		}
	}

	for i := range m.menus {
		key := m.menuKey(i)
		ia := ias[i]
		t := m.trig[i]
		if t.disabled {
			continue
		}
		// While a menu is open the pointer switches menus by hovering.
		if st.open != "" && st.open != key && ia.Hovered && hoverOK {
			m.openMenu(i, false)
		}
		if ia.Clicked {
			if navNonPointer(c, t.id) {
				m.openMenu(i, true)
			} else if st.open == key {
				m.close(false)
			} else {
				m.openMenu(i, false)
			}
		}
		m.triggerKeys(i)
	}
	m.popups()
}

// indexOf returns the index of the menu keyed key, or -1.
func (m *MenubarW) indexOf(key string) int {
	for i := range m.menus {
		if m.menuKey(i) == key {
			return i
		}
	}
	return -1
}

// step returns the next enabled menu from i in direction d (wrapping), or
// -1.
func (m *MenubarW) step(i, d int) int {
	n := len(m.menus)
	for k := 1; k <= n; k++ {
		j := ((i+d*k)%n + n) % n
		if !m.menus[j].Disabled && j < len(m.trig) && !m.trig[j].disabled {
			return j
		}
	}
	return -1
}

// openMenu opens menu i; kbd moves focus to its first item.
func (m *MenubarW) openMenu(i int, kbd bool) {
	c, st := m.c, m.st
	key := m.menuKey(i)
	if st.open != key {
		st.open = key
		st.popup.path = nil
		st.panels = nil
	}
	st.cur = i
	if kbd {
		st.popup.focus = 1
		st.popup.keyboard(c)
	} else {
		st.popup.kbd = false
	}
	c.invalid = true
}

// close closes the open menu; refocus moves focus back to its trigger
// when it was in the panels.
func (m *MenubarW) close(refocus bool) {
	c, st := m.c, m.st
	if refocus {
		if i := m.indexOf(st.open); i >= 0 && i < len(m.trig) {
			navFocus(c, m.trig[i].id)
		}
	}
	st.open = ""
	st.popup.path = nil
	st.popup.kbd = false
	st.popup.layer = 0
	st.popup.ids = st.popup.ids[:0]
	st.panels = nil
	c.invalid = true
}

// triggerKeys applies the keys pressed on trigger i.
func (m *MenubarW) triggerKeys(i int) {
	c, st := m.c, m.st
	t := m.trig[i]
	if c.focus != t.id {
		return
	}
	if st.open == "" && st.back != 0 && c.Escape() {
		back := st.back
		st.back = 0
		c.SetFocus(back)
		return
	}
	for _, k := range c.KeysFor(t.id) {
		if !k.Down {
			continue
		}
		key := k.Key
		if prev, next := c.inlineKey(key); prev || next {
			d := 1
			if prev {
				d = -1
			}
			if j := m.step(i, d); j >= 0 {
				navFocus(c, m.trig[j].id)
				st.cur = j
				if st.open != "" {
					m.openMenu(j, false)
				}
			}
			continue
		}
		switch key {
		case input.KeyHome:
			if j := m.step(-1, 1); j >= 0 {
				navFocus(c, m.trig[j].id)
				st.cur = j
			}
		case input.KeyEnd:
			if j := m.step(0, -1); j >= 0 {
				navFocus(c, m.trig[j].id)
				st.cur = j
			}
		case input.KeyDown, input.KeyUp:
			m.openMenu(i, true)
		case input.KeyEnter, input.KeySpace:
			// The implicit click opened the menu (navNonPointer).
		default:
			r := key.Rune()
			if k.Mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 || !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				continue
			}
			n := len(m.menus)
			for d := 1; d <= n; d++ {
				j := (i + d) % n
				if !m.trig[j].disabled && navStartsWith(m.menus[j].Label, r) {
					navFocus(c, m.trig[j].id)
					st.cur = j
					break
				}
			}
		}
	}
}

// popups builds the panel of the open menu.
func (m *MenubarW) popups() {
	c, th, st := m.c, m.th, m.st
	i := m.indexOf(st.open)
	if i < 0 || i >= len(m.trig) || m.trig[i].disabled {
		if st.open != "" {
			m.close(false)
		}
		return
	}
	if st.popup.escape(c) {
		m.close(m.inBar())
		return
	}
	if c.PressedOutside(append([]ID{m.n.id}, st.panels...)...) {
		m.close(false)
		return
	}
	idx := map[string]*MenubarItem{}
	items := menubarItems(m.menus[i].Items, idx)
	tk := &th.Components.Menubar
	marks := &navMarks{color: tk.IndicatorColor, size: tk.IndicatorSize, width: tk.IndicatorWidth,
		of: func(it MenuItem) (navMarkKind, bool) {
			src := idx[it.Key]
			if src == nil {
				return navMarkNone, false
			}
			switch src.Kind {
			case MenubarItemCheckbox:
				return navMarkCheck, src.Checked != nil && *src.Checked
			case MenubarItemRadio:
				return navMarkRadio, src.Radio != nil && *src.Radio == menubarValue(src)
			}
			return navMarkNone, false
		}}
	el := navEl(c)
	a := c.AnimFrom(hashID(m.n.id, "pop", st.open), "open", 0, 1, th.DurationMid)
	pop := &navMenuPopup{c: c, th: th, tk: &th.Components.Dropdown, st: &st.popup, element: el, opacity: a, marks: marks, minW: 180}
	pop.side = func(dir int) {
		if j := m.step(i, dir); j >= 0 && j != i {
			m.openMenu(j, true)
		}
	}
	off := float32(4)
	if el {
		off = 2
	}
	pop.panel(items, 0, m.trig[i].id, BottomStart, off, nil)
	st.panels = pop.panels
	if pop.clicked != "" {
		if src := idx[pop.clicked]; src != nil {
			switch src.Kind {
			case MenubarItemCheckbox:
				if src.Checked != nil {
					*src.Checked = !*src.Checked
					m.changed = true
				}
			case MenubarItemRadio:
				if v := menubarValue(src); src.Radio != nil && *src.Radio != v {
					*src.Radio = v
					m.changed = true
				}
			}
		}
		m.clicked = pop.clicked
		m.close(pop.hasFocus())
	}
}

// menubarValue is the value a radio item stores.
func menubarValue(it *MenubarItem) string {
	if it.Value != "" {
		return it.Value
	}
	return menubarKey(it)
}

// menubarKey is an item's key (its label when Key is empty).
func menubarKey(it *MenubarItem) string {
	if it.Key != "" {
		return it.Key
	}
	return it.Label
}

// menubarItems converts items to the popup panels' MenuItem form,
// indexing the source of every item by key.
func menubarItems(items []MenubarItem, idx map[string]*MenubarItem) []MenuItem {
	out := make([]MenuItem, 0, len(items))
	for i := range items {
		it := &items[i]
		mi := MenuItem{Label: it.Label, Icon: it.Icon, Shortcut: it.Shortcut, Disabled: it.Disabled,
			Danger: it.Danger, Divider: it.Divider, Group: it.Group}
		if !it.Divider {
			mi.Key = menubarKey(it)
			idx[mi.Key] = it
		}
		if len(it.Children) > 0 {
			mi.Children = menubarItems(it.Children, idx)
		}
		out = append(out, mi)
	}
	return out
}

func (m *MenubarW) paintBar(l *paint.List, r geom.Rect) {
	tk := m.tk
	rad := geom.Rad(tk.Radius)
	if len(tk.Shadow) > 0 {
		l.Shadows(r, rad, tk.Shadow)
	}
	l.Fill(r, rad, tk.Bg)
	if tk.BorderColor.A > 0 {
		l.Stroke(r, rad, 1, tk.BorderColor)
	}
	if tk.DividerColor.A > 0 {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.DividerColor)
	}
}
