package ui

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// MenuMode selects how Menu shows submenus.
//
// Append-only: never reorder or insert; the zero value stays MenuInline.
type MenuMode uint8

const (
	MenuInline     MenuMode = iota // submenus expand in place (sider)
	MenuVertical                   // submenus pop out to the right
	MenuHorizontal                 // top bar, submenus drop down
)

type menuState struct {
	open   map[string]bool
	pop    string // top-level key whose popup is shown
	popup  navPopupState
	panels []ID
	items  []ID
	kbd    bool // popup opened by keyboard: hover does not close it
	ptr    geom.Point
	woke   bool // asked for a frame to fit a horizontal bar once laid out
}

// MenuW is a navigation menu: inline, vertical and horizontal modes,
// collapsed icon mode with pop-out submenus and tooltips, dark theme,
// accordion behavior and keyboard navigation.
//
// Keyboard (the menu items are in the Tab order): Up/Down move between
// visible items; in inline mode Right opens a submenu (again: enters it)
// and Left closes it or returns to the parent; in vertical / collapsed
// mode Right or Enter opens the pop-out submenu and moves into it; in
// horizontal mode Left/Right move along the bar and Down or Enter opens the
// dropdown. Enter/Space choose a leaf and Escape closes a pop-out submenu.
type MenuW struct {
	_ [0]func()

	c         *Context
	th        *theme.Theme // Menu tokens (c.ThemeFor(theme.CompMenu))
	items     []MenuItem
	sel       *string
	mode      MenuMode
	dark      bool
	collapsed bool
	accordion bool
	defOpen   []string
	width     float32
	st        *menuState
	clicked   string
	id        ID

	ents   []navEntry
	nodes  []*node // item nodes, parallel to ents (roving Tab stop)
	act    int
	actKey input.Key
}

// Menu starts a menu bound to the selected key; chain options, then Show.
//
// ConfigProvider.Defaults("Menu", ...) keys: "mode" (MenuMode), "theme"
// ("dark" | "light"), "accordion", "inlineCollapsed" (bool).
func Menu(c *Context, items []MenuItem, selected *string) *MenuW {
	m := &MenuW{c: c, th: c.ThemeFor(theme.CompMenu), items: items, sel: selected, act: -1}
	m.mode = Prop(c, "Menu", "mode", MenuInline)
	m.dark = Prop(c, "Menu", "theme", "light") == "dark"
	m.accordion = Prop(c, "Menu", "accordion", false)
	m.collapsed = Prop(c, "Menu", "inlineCollapsed", false)
	return m
}

// Mode selects inline (default), vertical or horizontal.
func (m *MenuW) Mode(md MenuMode) *MenuW { m.mode = md; return m }

// Dark uses the dark sider colors (Ant theme="dark", Element's dark
// el-menu). Under shadcn/ui it is ignored by design: shadcn has a single
// Sidebar look built from the --sidebar-* variables, which already follow
// the light / dark theme (theme.Shadcn(theme.Dark()) gives the dark
// sidebar); a dark sidebar on a light shadcn page is not offered.
func (m *MenuW) Dark() *MenuW { m.dark = true; return m }

// Collapsed shows icons only; submenus pop out on hover.
func (m *MenuW) Collapsed(on bool) *MenuW { m.collapsed = on; return m }

// Accordion keeps only one inline submenu open (Element unique-opened).
func (m *MenuW) Accordion() *MenuW { m.accordion = true; return m }

// DefaultOpen opens inline submenus initially.
func (m *MenuW) DefaultOpen(keys ...string) *MenuW { m.defOpen = keys; return m }

// Width sets the expanded width (default: fill).
func (m *MenuW) Width(px float32) *MenuW { m.width = px; return m }

// Clicked returns the leaf key clicked this frame.
func (m *MenuW) Clicked() string { return m.clicked }

func (m *MenuW) el() bool { return navEl(m.c) }

// colors returns the menu colors from the Menu tokens: the light set, or
// the Dark* set for a dark menu. fgActive is the hover / active text color
// (dark items fade to it on hover; horizontal items use it for the hover
// text and the underline). shadcn has a single Sidebar look, so Dark is
// ignored there.
func (m *MenuW) colors() (bg, fg, fgActive, hoverBg, selBg, selFg, subBg theme.Color) {
	tk := &m.th.Components.Menu
	if !m.dark || m.th.Flavor == theme.StyleShadcn {
		return tk.ItemBg, tk.ItemColor, tk.ItemSelectedColor, tk.ItemHoverBg, tk.ItemSelectedBg, tk.ItemSelectedColor, tk.SubMenuItemBg
	}
	return tk.DarkItemBg, tk.DarkItemColor, tk.DarkItemHoverColor, tk.DarkItemHoverBg, tk.DarkItemSelectedBg, tk.DarkItemSelectedColor, tk.DarkSubMenuItemBg
}

// Show builds the menu.
func (m *MenuW) Show() *MenuW {
	c, th := m.c, m.th
	bg, _, _, _, _, _, _ := m.colors()
	dir := layout.Column
	if m.mode == MenuHorizontal {
		dir = layout.Row
	}
	b := c.box("menu", dir, nil)
	m.id = b.n.id
	m.st = State[menuState](c, b.n.id, func() menuState {
		st := menuState{open: map[string]bool{}}
		for _, k := range m.defOpen {
			st.open[k] = true
		}
		for _, it := range m.items {
			if len(it.Children) > 0 && m.sel != nil && containsKey(it.Children, *m.sel) {
				st.open[it.Key] = true
			}
		}
		return st
	})
	if c.pointer != m.st.ptr {
		m.st.ptr = c.pointer
		m.st.kbd = false
	}
	m.st.items = m.st.items[:0]
	if m.mode == MenuHorizontal {
		// Items that do not fit the bar collapse into a "…" submenu.
		w := float32(0)
		if r, ok := c.rects[b.n.id]; ok {
			w = r.W - b.n.ln.Padding.Horizontal()
		} else if !m.st.woke {
			m.st.woke = true
			c.WakeAt(c.now)
		}
		m.items = m.fitHorizontal(w)
		b.n.ln.MinW, b.n.ln.Shrink = 0, 1
		if n := len(m.items); n > 0 && m.items[n-1].Key == menuMoreKey {
			b.n.ln.Grow = 1 // take the free width so the bar can grow back
		}
	}
	b.Bg(bg)
	if m.mode == MenuHorizontal {
		b.n.setRole(a11y.RoleMenuBar).setState(a11y.Horizontal)
	} else {
		b.n.setRole(a11y.RoleMenu).setState(a11y.Vertical)
	}
	if m.mode == MenuHorizontal {
		h := float32(46)
		if m.el() {
			h = 60
		}
		b.n.ln.Height = h
		b.n.ln.Align = layout.AlignStretch
		if th.Flavor == theme.StyleShadcn {
			b.Bg(th.BgContainer)
		} else if !m.dark {
			b.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Bottom()-1, r.W, th.Split) })
		}
	} else {
		w := m.width
		if m.collapsed {
			w = th.SiderCollapsed
			if !m.el() {
				w = 80
			}
		}
		if w > 0 {
			b.n.ln.Width = c.Anim(b.n.id, "w", w, th.DurationMid)
		}
		b.n.ln.Padding = geom.VH(4, 0)
		if m.el() {
			b.n.ln.Padding = geom.Insets{}
		}
		b.n.clip = true
		if !m.dark {
			// The divider sits on the end edge (towards the content).
			b.Paint(func(l *paint.List, r geom.Rect) { navVLine(l, c.inline(r).X(r.W-1, 1), r.Y, r.H, th.Split) })
		}
	}
	b.Body(func() {
		if m.mode == MenuHorizontal {
			m.horizontal()
		} else {
			m.list(m.items, 0, -1)
		}
	})
	m.tabStop()
	m.keyNav()
	m.popups()
	return m
}

// tabStop makes the menu a single Tab stop (the focused, else the
// selected, else the first item); arrow keys move between the items.
func (m *MenuW) tabStop() {
	cur := -1
	for i, e := range m.ents {
		if e.id == m.c.focus {
			cur = i
			break
		}
		if cur < 0 && m.sel != nil && e.key == *m.sel {
			cur = i
		}
	}
	rovingTabStop(m.nodes, cur)
}

func (m *MenuW) popupMode() bool { return m.collapsed || m.mode != MenuInline }

// record adds a keyboard entry and collects the keys pressed on it.
func (m *MenuW) record(n *node, e navEntry) int {
	i := len(m.ents)
	m.ents = append(m.ents, e)
	m.nodes = append(m.nodes, n)
	if !e.disabled {
		for _, k := range navKeys(m.c, e.id) {
			m.act, m.actKey = i, k
		}
	}
	return i
}

func (m *MenuW) list(items []MenuItem, level, parent int) {
	c, th := m.c, m.th
	tk := &th.Components.Menu
	el := m.el()
	_, fg, fgActive, hoverBg, selBg, selFg, subBg := m.colors()
	for _, it := range items {
		it := it
		if it.Divider {
			d := c.add("menu-divider", "")
			d.ln.Height = 1
			d.ln.Margin = geom.Insets{Top: 4, Bottom: 4}
			d.setRole(a11y.RoleSeparator).setState(a11y.Horizontal)
			d.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, th.Split) }
			continue
		}
		if it.Group {
			m.group(it, level, parent)
			continue
		}
		hasKids := len(it.Children) > 0
		open := m.st.open[it.Key] && !m.popupMode()
		selected := m.sel != nil && *m.sel == it.Key
		childSel := hasKids && m.sel != nil && containsKey(it.Children, *m.sel)
		n := c.add("menu-item", it.Label)
		n.setRole(a11y.RoleMenuItem).stateIf(selected, a11y.Selected|a11y.Current)
		if hasKids {
			n.setState(a11y.Expandable).stateIf(m.popupMode(), a11y.HasPopup).
				stateIf(open || m.popupMode() && level == 0 && m.st.pop == it.Key, a11y.Expanded)
		}
		n.ln.Height = tk.ItemHeight
		shad := th.Flavor == theme.StyleShadcn
		if mi := tk.ItemMarginInline; shad {
			n.ln.Margin = geom.Insets{Top: 1, Bottom: 1, Left: mi, Right: mi}
		} else if !el {
			n.ln.Margin = geom.Insets{Top: 4, Bottom: 4, Left: mi, Right: mi}
		}
		ia := c.interact(n, true, input.CursorPointer)
		if it.Disabled {
			n.disabled, n.focus, n.cursor = true, false, input.CursorNotAllowed
		}
		if level == 0 {
			m.st.items = append(m.st.items, n.id)
		}
		idx := m.record(n, navEntry{id: n.id, key: it.Key, level: level, kids: hasKids, open: open, disabled: it.Disabled, parent: parent})
		popOpen := m.popupMode() && level == 0 && m.st.pop == it.Key
		if level == 0 && m.popupMode() && ia.Hovered && !it.Disabled && m.st.pop != it.Key && !m.st.kbd {
			m.st.pop = it.Key
			m.st.popup.path = nil
			c.invalid = true
		}
		if ia.Clicked && !it.Disabled {
			switch {
			case hasKids && !m.popupMode():
				m.toggle(it, !open)
			case hasKids:
				m.a11yPop(n.id, it.Key)
			default:
				if m.sel != nil {
					*m.sel = it.Key
				}
				m.clicked = it.Key
				m.st.pop = ""
			}
			c.invalid = true
		}
		indent := float32(24 + level*24)
		if el {
			indent = float32(20 + level*20)
		}
		if shad {
			indent = 8 // nesting comes from the sub column's margin
		}
		icononly := m.collapsed && level == 0
		id := n.id
		arrow := c.Anim(id, "arrow", b2f(open), th.DurationMid)
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(tk.ItemBorderRadius)
			s := c.inline(r)
			hov := c.Anim(id, "hover", b2f((ia.Hovered || popOpen) && !it.Disabled), th.DurationMid)
			col := fg
			if selected || childSel {
				col = selFg
				if childSel && !m.dark && !el {
					col = fgActive
				}
			}
			if selected && !hasKids && selBg.A > 0 {
				l.Fill(r, rad, selBg)
			} else if childSel && icononly && selBg.A > 0 {
				l.Fill(r, rad, selBg)
			} else if hov > 0 {
				if hoverBg.A > 0 {
					l.Fill(r, rad, theme.Fade(hoverBg, float64(hov)))
				}
				if m.dark && !el {
					col = theme.Lerp(col, fgActive, hov)
				} else if !m.dark && !selected && !childSel && tk.ItemHoverColor != fg {
					col = theme.Lerp(col, tk.ItemHoverColor, hov)
				}
			}
			if it.Disabled {
				col = th.TextDisabled
			}
			if el && selected && !hasKids && !m.dark && icononly {
				col = th.Primary.Base
			}
			// x is the logical offset from the start edge (indent grows
			// from the right in RTL).
			is := tk.IconSize
			x := indent
			if icononly {
				is = 16
				x = (r.W - is) / 2
			}
			if it.Icon != nil {
				c.drawIconAt(l, it.Icon, s.X(x, is), r.Y+(r.H-is)/2, is, col)
				x += is + 10
				if shad {
					x -= 2
				}
			} else if icononly {
				// No icon: show the first character.
				lay := navLabel(c, string([]rune(it.Label)[:1]), th.FontSize, text.Regular)
				l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			}
			if !icononly {
				st := c.style(th.FontSize, text.Regular)
				if shad && selected {
					st.Weight = text.Medium
				}
				end := r.W - 16
				if hasKids {
					end -= 20
				}
				lay := text.Shape(it.Label, st, c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, end-x)}))
				l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
				if hasKids {
					ar := s.Rect(r.W-26, r.Y+(r.H-10)/2, 10, 10)
					if m.popupMode() {
						c.paintIcon(l, icon.Right, ar, col, 0)
					} else {
						icon.Draw(l, icon.Down, ar, col, float64(arrow)*math.Pi)
					}
				}
			}
			if ia.FocusVisible {
				l.Stroke(r, rad, 2, th.FocusRing)
			}
		}
		if hasKids && open && shad {
			// shadcn SidebarMenuSub: children hang off a guide line on the
			// start side.
			sub := Column(c, func() { m.list(it.Children, level+1, idx) })
			m.subSem(n, sub, it.Label)
			sub.n.ln.Margin = geom.Insets{Left: 14}
			sub.Paint(func(l *paint.List, r geom.Rect) { navVLine(l, c.inline(r).X(8, 1), r.Y+2, r.H-4, th.Split) })
			continue
		}
		if hasKids && open {
			sub := Column(c, func() { m.list(it.Children, level+1, idx) })
			m.subSem(n, sub, it.Label)
			if subBg.A > 0 {
				sub.Bg(subBg)
			}
		}
	}
}

// group builds a titled item group (Ant type="group"): the title, then
// the children at the same level, inside a group landmark.
func (m *MenuW) group(it MenuItem, level, parent int) {
	c, th := m.c, m.th
	el := m.el()
	_, fg, _, _, _, _, _ := m.colors()
	g := Column(c, func() {
		if !(m.collapsed && level == 0) {
			t := c.add("menu-group-title", it.Label)
			t.aHidden = true // the group carries the name
			fs := th.FontSize
			h := float32(38)
			col := th.TextTertiary
			x := float32(16 + level*24)
			switch {
			case el:
				fs, h, col, x = th.FontSizeSM, 30, th.TextSecondary, float32(20+level*20)
			case th.Flavor == theme.StyleShadcn:
				fs, h, x = th.FontSizeSM, 32, 16
			}
			if m.dark {
				col = theme.Fade(fg, .7)
			}
			t.ln.Height = h
			t.paint = func(l *paint.List, r geom.Rect) {
				lay := text.Shape(it.Label, c.style(fs, text.Regular), c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, r.W-x-8)}))
				l.Glyphs(geom.Pt(c.inline(r).X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			}
		}
		m.list(it.Children, level, parent)
	})
	g.n.setRole(a11y.RoleGroup).setName(it.Label)
}

// rootItem finds the root-level item with key (looking into root groups).
func menuRootItem(items []MenuItem, key string) (MenuItem, bool) {
	for _, it := range items {
		if it.Group {
			if x, ok := menuRootItem(it.Children, key); ok {
				return x, true
			}
			continue
		}
		if !it.Divider && it.Key == key {
			return it, true
		}
	}
	return MenuItem{}, false
}

// menuMoreKey is the key of the overflow item of a horizontal menu.
const menuMoreKey = "\x00menu-more"

// menuHItemWidth is the width of a horizontal menu item (as measured).
func (m *MenuW) hItemWidth(it MenuItem) float32 {
	c, th := m.c, m.th
	if it.Key == menuMoreKey {
		return 56
	}
	w := text.Width(it.Label, c.style(th.FontSize, text.Regular)) + 40
	if it.Icon != nil {
		w += 24
	}
	if len(it.Children) > 0 && m.el() {
		w += 18
	}
	return w
}

// fitHorizontal returns the root items of a horizontal menu: those that
// fit the bar's width (last frame), then a "…" item holding the rest
// (Ant overflowedIndicator, Element ellipsis).
func (m *MenuW) fitHorizontal(width float32) []MenuItem {
	var items []MenuItem
	for _, it := range m.items {
		switch {
		case it.Divider:
		case it.Group:
			items = append(items, menuFlatGroups(it.Children)...)
		default:
			items = append(items, it)
		}
	}
	if width <= 0 {
		return items
	}
	total := float32(0)
	for _, it := range items {
		if !it.Divider {
			total += m.hItemWidth(it)
		}
	}
	if total <= width+0.5 {
		return items
	}
	avail := width - m.hItemWidth(MenuItem{Key: menuMoreKey})
	used := float32(0)
	k := 0
	for k < len(items) {
		w := float32(0)
		if !items[k].Divider {
			w = m.hItemWidth(items[k])
		}
		if used+w > avail {
			break
		}
		used += w
		k++
	}
	rest := menuFlatGroups(items[k:])
	out := append(append([]MenuItem(nil), items[:k]...), MenuItem{Key: menuMoreKey, Label: m.c.Locale().More, Icon: icon.More, Children: rest})
	return out
}

// subSem marks an expanded inline submenu as a menu controlled by its item.
func (m *MenuW) subSem(item *node, sub *Box, label string) {
	sub.n.setRole(a11y.RoleMenu).setName(label).setState(a11y.Vertical)
	if m.c.a11yOn {
		id := sub.n.id
		item.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(id) }))
	}
}

// a11yPop handles a click on a root item with a pop-out submenu: a screen
// reader's Expand / Press opens it in keyboard mode (it stays open without
// the pointer), Collapse closes it; a pointer click just shows it.
func (m *MenuW) a11yPop(id ID, key string) {
	act, _, ok := m.c.A11yAction(id)
	switch {
	case ok && act == a11y.Collapse:
		m.closePop(true)
	case ok:
		m.openPop(key)
	default:
		m.st.pop = key
	}
}

// toggle opens or closes an inline submenu (accordion closes the others).
func (m *MenuW) toggle(it MenuItem, open bool) {
	if open && m.accordion {
		for k := range m.st.open {
			if k != it.Key && !containsKey(it.Children, k) {
				delete(m.st.open, k)
			}
		}
	}
	m.st.open[it.Key] = open
	m.c.invalid = true
}

func (m *MenuW) horizontal() {
	c, th := m.c, m.th
	el := m.el()
	_, fg, fgActive, _, selBg, selFg, _ := m.colors()
	for _, it := range m.items {
		it := it
		if it.Divider {
			continue
		}
		n := c.add("menu-h", it.Label)
		hasKids := len(it.Children) > 0
		n.setRole(a11y.RoleMenuItem)
		if hasKids {
			n.setState(a11y.Expandable|a11y.HasPopup).stateIf(m.st.pop == it.Key, a11y.Expanded)
		}
		n.ln.Measure = func(_, maxH float32) geom.Size { return geom.Size{W: m.hItemWidth(it), H: 0} }
		ia := c.interact(n, true, input.CursorPointer)
		if it.Disabled {
			n.disabled, n.focus, n.cursor = true, false, input.CursorNotAllowed
		}
		m.st.items = append(m.st.items, n.id)
		m.record(n, navEntry{id: n.id, key: it.Key, kids: hasKids, disabled: it.Disabled, parent: -1})
		if hasKids && ia.Hovered && !it.Disabled && m.st.pop != it.Key && !m.st.kbd {
			m.st.pop = it.Key
			m.st.popup.path = nil
			c.invalid = true
		}
		if !hasKids && ia.Hovered && m.st.pop != "" && !m.st.kbd {
			m.st.pop = ""
		}
		if ia.Clicked && !it.Disabled && !hasKids {
			if m.sel != nil {
				*m.sel = it.Key
			}
			m.clicked = it.Key
			c.invalid = true
		}
		if _, _, ok := c.A11yAction(n.id); ok && ia.Clicked && hasKids && !it.Disabled {
			m.a11yPop(n.id, it.Key)
		}
		selected := m.sel != nil && (*m.sel == it.Key || containsKey(it.Children, *m.sel))
		n.stateIf(selected, a11y.Selected|a11y.Current)
		popOpen := m.st.pop == it.Key
		id := n.id
		n.paint = func(l *paint.List, r geom.Rect) {
			hov := c.Anim(id, "hover", b2f((ia.Hovered || popOpen) && !it.Disabled), th.DurationMid)
			col := theme.Lerp(fg, fgActive, hov)
			if selected {
				col = fgActive
			}
			if th.Flavor == theme.StyleShadcn {
				// NavigationMenu: ghost triggers, accent pill on hover / active.
				pill := r.Inset(geom.Insets{Top: 6, Bottom: 6, Left: 4, Right: 4})
				a := float64(hov)
				if selected {
					a = 1
				}
				if a > 0 {
					l.Fill(pill, geom.Rad(th.Radius), theme.Fade(th.ItemHoverBg, a))
				}
			} else if m.dark && !el {
				if selected {
					l.Fill(r, geom.Radii{}, selBg)
					col = selFg
				}
			} else {
				a := float64(hov)
				if selected {
					a = 1
				}
				if a > 0 {
					bx, bw := r.X+20, r.W-40
					if el {
						bx, bw = r.X, r.W
					}
					l.Fill(geom.Rect{X: bx, Y: r.Bottom() - 2, W: bw, H: 2}, geom.Radii{}, theme.Fade(fgActive, a))
				}
				if el && hov > 0 && !selected {
					l.Fill(r.Inset(geom.Insets{Bottom: 2}), geom.Radii{}, theme.Fade(th.ItemHoverBg, a))
				}
			}
			if it.Disabled {
				col = th.TextDisabled
			}
			if it.Key == menuMoreKey {
				c.drawIconAt(l, icon.More, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, col)
				if ia.FocusVisible {
					l.Stroke(r.Inset(geom.Insets{Top: 2, Bottom: 2}), geom.Rad(th.RadiusSM), 2, th.FocusRing)
				}
				return
			}
			s := c.inline(r)
			x := float32(20)
			if it.Icon != nil {
				c.drawIconAt(l, it.Icon, s.X(x, 14), r.Y+(r.H-14)/2, 14, col)
				x += 24
			}
			lay := navLabel(c, it.Label, th.FontSize, text.Regular)
			l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			if hasKids && el {
				icon.Draw(l, icon.Down, s.Rect(x+lay.Width+6, r.Y+(r.H-12)/2, 12, 12), col, 0)
			}
			if ia.FocusVisible {
				l.Stroke(r.Inset(geom.Insets{Top: 2, Bottom: 2}), geom.Rad(th.RadiusSM), 2, th.FocusRing)
			}
		}
	}
}

// openPop opens the pop-out submenu of a root item from the keyboard and
// moves focus to its first item.
func (m *MenuW) openPop(key string) {
	m.st.pop = key
	m.st.popup.path = nil
	m.st.popup.focus = 1
	m.st.popup.keyboard(m.c)
	m.st.kbd, m.st.ptr = true, m.c.pointer
	m.c.invalid = true
}

// closePop closes the pop-out submenu; focus returns to its root item
// when it was inside.
func (m *MenuW) closePop(refocus bool) {
	if refocus {
		for _, e := range m.ents {
			if e.level == 0 && e.key == m.st.pop {
				navFocus(m.c, e.id)
			}
		}
	}
	m.st.pop = ""
	m.st.popup.path = nil
	m.st.popup.layer = 0
	m.st.kbd = false
	m.st.panels = nil
	m.c.invalid = true
}

// keyNav applies the navigation key pressed on a menu item this frame.
func (m *MenuW) keyNav() {
	if m.act < 0 {
		return
	}
	c, e, k := m.c, m.ents[m.act], m.actKey
	// Arrow keys are visual: map them to the logical Left (towards the
	// start: previous item, close) and Right (towards the end: next item,
	// open), so in RTL Left opens a submenu and Right closes it.
	if prev, next := c.inlineKey(k); prev {
		k = input.KeyLeft
	} else if next {
		k = input.KeyRight
	}
	root := func(x navEntry) bool { return x.level == 0 }
	dir := 1
	if k == input.KeyUp || k == input.KeyLeft {
		dir = -1
	}
	if m.mode == MenuHorizontal {
		switch k {
		case input.KeyLeft, input.KeyRight:
			if j := navStep(m.ents, m.act, dir, root); j >= 0 {
				navFocus(c, m.ents[j].id)
				if m.st.pop != "" {
					m.closePop(false)
				}
			}
		case input.KeyDown, input.KeyEnter, input.KeySpace:
			if e.kids {
				m.openPop(e.key)
			}
		}
		return
	}
	switch k {
	case input.KeyDown, input.KeyUp:
		if j := navStep(m.ents, m.act, dir, nil); j >= 0 {
			navFocus(c, m.ents[j].id)
		}
	case input.KeyHome:
		if j := navStep(m.ents, -1, 1, nil); j >= 0 {
			navFocus(c, m.ents[j].id)
		}
	case input.KeyEnd:
		if j := navStep(m.ents, 0, -1, nil); j >= 0 {
			navFocus(c, m.ents[j].id)
		}
	case input.KeyRight:
		switch {
		case !e.kids:
		case m.popupMode():
			if e.level == 0 {
				m.openPop(e.key)
			}
		case !e.open:
			m.st.open[e.key] = true
			c.invalid = true
		case m.act+1 < len(m.ents) && m.ents[m.act+1].parent == m.act:
			if j := navStep(m.ents, m.act, 1, nil); j >= 0 {
				navFocus(c, m.ents[j].id)
			}
		}
	case input.KeyLeft:
		switch {
		case e.kids && e.open:
			m.st.open[e.key] = false
			c.invalid = true
		case e.parent >= 0:
			navFocus(c, m.ents[e.parent].id)
		}
	case input.KeyEnter, input.KeySpace:
		// The implicit click opened the pop-out submenu; move into it.
		if e.kids && m.popupMode() && e.level == 0 {
			m.openPop(e.key)
		}
	}
}

// popups shows the pop-out submenu (collapsed / vertical / horizontal) and
// the tooltip of a hovered collapsed leaf.
func (m *MenuW) popups() {
	c := m.c
	if !m.popupMode() && m.mode != MenuHorizontal {
		return
	}
	if m.st.pop != "" && m.st.popup.escape(c) {
		m.closePop(true)
		return
	}
	over := c.pointerOver(append(append([]ID{}, m.st.items...), m.st.panels...)...)
	if (!HoverDelay(c, hashID(m.id, "pop"), over, 0, 300*time.Millisecond) && !m.st.kbd) || m.st.pop == "" {
		if m.st.pop != "" && !over {
			m.st.pop = ""
			m.st.popup.path = nil
		}
		m.st.panels = nil
		return
	}
	if c.PressedOutside(append([]ID{m.id}, m.st.panels...)...) {
		m.closePop(false)
		return
	}
	item, found := menuRootItem(m.items, m.st.pop)
	anchor := ID(0)
	for _, e := range m.ents {
		if e.level == 0 && e.key == m.st.pop {
			anchor = e.id
			break
		}
	}
	if !found || anchor == 0 {
		return
	}
	if len(item.Children) == 0 {
		if m.collapsed {
			navTooltipBubble(c, anchor, item.Label, InlineEnd)
		}
		m.st.panels = nil
		return
	}
	a := c.AnimFrom(hashID(m.id, "pop", item.Key), "open", 0, 1, c.Theme.DurationMid)
	mk := &m.th.Components.Menu
	pop := &navMenuPopup{c: c, th: m.th, tk: menuPopupToken(mk), menu: mk, st: &m.st.popup, dark: m.dark, element: m.el(), opacity: a}
	if m.sel != nil {
		pop.selected = *m.sel
	}
	pop.side = func(dir int) {
		if m.mode != MenuHorizontal {
			if dir < 0 {
				m.closePop(true)
			}
			return
		}
		cur := -1
		for i, e := range m.ents {
			if e.level == 0 && e.key == m.st.pop {
				cur = i
			}
		}
		if j := navStep(m.ents, cur, dir, func(e navEntry) bool { return e.level == 0 }); j >= 0 {
			if m.ents[j].kids {
				m.openPop(m.ents[j].key)
			} else {
				m.closePop(false)
				navFocus(c, m.ents[j].id)
			}
		}
	}
	place, off := InlineEndTop, float32(4)
	if m.mode == MenuHorizontal {
		place, off = BottomStart, 4
		if m.el() {
			off = 0
		}
	}
	pop.panel(item.Children, 0, anchor, place, off, nil)
	m.st.panels = pop.panels
	if pop.clicked != "" {
		if m.sel != nil {
			*m.sel = pop.clicked
		}
		m.clicked = pop.clicked
		m.closePop(pop.hasFocus())
	}
}

// menuPopupToken maps the Popup* fields of a Menu token onto the panel
// token the shared pop-out drawing (navMenuPopup) reads, so Menu submenus
// follow Menu overrides and ignore Dropdown ones.
func menuPopupToken(k *theme.MenuToken) *theme.DropdownToken {
	return &theme.DropdownToken{
		Alias:   k.Alias,
		PopupBg: k.PopupBg, PopupBorderColor: k.PopupBorderColor, PopupRadius: k.PopupRadius, PopupShadow: k.PopupShadow,
		PaddingBlock: k.PopupPaddingBlock, PaddingInline: k.PopupPaddingInline,
		ItemHeight: k.PopupItemHeight, ItemPaddingInline: k.PopupItemPaddingInline, ItemBorderRadius: k.PopupItemBorderRadius,
		ItemColor: k.PopupItemColor, ItemHoverBg: k.PopupItemHoverBg, ItemHoverColor: k.PopupItemHoverColor,
		ItemSelectedBg: k.PopupItemSelectedBg, ItemSelectedColor: k.PopupItemSelectedColor,
		ItemIconColor: k.PopupItemIconColor, IconSize: k.PopupIconSize,
		DangerColor: k.PopupDangerColor, DangerHoverBg: k.PopupDangerHoverBg, DangerHoverColor: k.PopupDangerHoverColor,
		SubmenuArrowColor: k.PopupArrowColor,
		GroupTitleHeight:  k.PopupGroupTitleHeight, GroupTitleFontSize: k.PopupGroupTitleFontSize,
		GroupTitleFontWeight: k.PopupGroupTitleFontWeight, GroupTitleColor: k.PopupGroupTitleColor,
		DividerColor:  k.PopupDividerColor,
		ShortcutColor: k.PopupShortcutColor, ShortcutFontSize: k.PopupShortcutFontSize,
		ShortcutLetterSpacing: k.PopupShortcutLetterSpacing, ShortcutGap: k.PopupShortcutGap,
	}
}
