package ui

import (
	"math"
	"slices"
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

	openKeys    *[]string // bound inline open keys (nil: internal state)
	openChanged bool
	selectable  bool
	tooltip     bool
	indent      float32 // per-level inline indent (0: the token)
	openDelay   time.Duration
	closeDelay  time.Duration
	clickSub    bool       // submenus pop out on click (triggerSubMenuAction)
	expandIcon  *icon.Icon // nil: the theme chevron
	moreIcon    *icon.Icon // nil: icon.More
	noEllipsis  bool
	panels      map[string]func()

	ents   []navEntry
	nodes  []*node // item nodes, parallel to ents (roving Tab stop)
	act    int
	actKey input.Key
}

// Menu starts a menu bound to the selected key; chain options, then Show.
//
// ConfigProvider.Defaults("Menu", ...) keys: "mode" (MenuMode), "theme"
// ("dark" | "light"), "accordion", "inlineCollapsed", "selectable",
// "tooltip", "ellipsis" (bool), "inlineIndent" (float32 px),
// "subMenuOpenDelay" / "subMenuCloseDelay" (time.Duration; default the
// Menu token: Ant 100 / 100 ms, Element 300 / 300 ms, shadcn 200 / 150 ms),
// "triggerSubMenuAction" ("hover" | "click").
func Menu(c *Context, items []MenuItem, selected *string) *MenuW {
	m := &MenuW{c: c, th: c.ThemeFor(theme.CompMenu), items: items, sel: selected, act: -1}
	tk := &m.th.Components.Menu
	m.mode = Prop(c, "Menu", "mode", MenuInline)
	m.dark = Prop(c, "Menu", "theme", "light") == "dark"
	m.accordion = Prop(c, "Menu", "accordion", false)
	m.collapsed = Prop(c, "Menu", "inlineCollapsed", false)
	m.selectable = Prop(c, "Menu", "selectable", true)
	m.tooltip = Prop(c, "Menu", "tooltip", true)
	m.noEllipsis = !Prop(c, "Menu", "ellipsis", true)
	m.indent = Prop(c, "Menu", "inlineIndent", float32(0))
	m.openDelay = Prop(c, "Menu", "subMenuOpenDelay", tk.SubMenuOpenDelay)
	m.closeDelay = Prop(c, "Menu", "subMenuCloseDelay", tk.SubMenuCloseDelay)
	m.clickSub = Prop(c, "Menu", "triggerSubMenuAction", "hover") == "click"
	return m
}

// OpenKeys binds the keys of the open inline submenus (antd openKeys /
// onOpenChange, Element default-openeds): the menu reads and writes
// *keys; OpenChanged reports a change. Keys that are not submenus of the
// menu are ignored.
func (m *MenuW) OpenKeys(keys *[]string) *MenuW { m.openKeys = keys; return m }

// OpenChanged reports that the user opened or closed an inline submenu
// this frame.
func (m *MenuW) OpenChanged() bool { return m.openChanged }

// Selectable(false) leaves the selected key alone when a leaf is clicked
// (antd selectable; Clicked still reports it).
func (m *MenuW) Selectable(on bool) *MenuW { m.selectable = on; return m }

// Tooltip(false) turns off the tooltip of a collapsed menu's root items
// (antd tooltip={false}).
func (m *MenuW) Tooltip(on bool) *MenuW { m.tooltip = on; return m }

// InlineIndent sets the indent per inline level (antd inlineIndent 24,
// Element --el-menu-level-padding 20; default the Menu token).
func (m *MenuW) InlineIndent(px float32) *MenuW { m.indent = px; return m }

// SubMenuOpenDelay waits d before a hovered pop-out submenu opens (antd
// subMenuOpenDelay, Element show-timeout).
func (m *MenuW) SubMenuOpenDelay(d time.Duration) *MenuW { m.openDelay = d; return m }

// SubMenuCloseDelay waits d before a pop-out submenu closes once the
// pointer left it (antd subMenuCloseDelay, Element hide-timeout).
func (m *MenuW) SubMenuCloseDelay(d time.Duration) *MenuW { m.closeDelay = d; return m }

// TriggerSubMenuAction selects how pop-out submenus open: TriggerHover
// (default) or TriggerClick (antd triggerSubMenuAction, Element
// menu-trigger); a click popup stays until a choice, Escape or a press
// outside.
func (m *MenuW) TriggerSubMenuAction(t Trigger) *MenuW { m.clickSub = t == TriggerClick; return m }

// ExpandIcon replaces the submenu chevron (antd expandIcon).
func (m *MenuW) ExpandIcon(ic *icon.Icon) *MenuW { m.expandIcon = ic; return m }

// OverflowedIndicator replaces the icon of the "…" item that collects the
// items a horizontal bar cannot fit (antd overflowedIndicator, Element
// ellipsis-icon).
func (m *MenuW) OverflowedIndicator(ic *icon.Icon) *MenuW { m.moreIcon = ic; return m }

// Ellipsis(false) keeps every root item of a horizontal bar instead of
// folding the overflow into "…" (Element ellipsis).
func (m *MenuW) Ellipsis(on bool) *MenuW { m.noEllipsis = !on; return m }

// ItemPanel shows fn in the drop-down panel of the root item keyed key of
// a horizontal menu instead of its children (shadcn NavigationMenuContent):
// hovering or focusing the item opens the panel, Escape or leaving it
// closes it. The item gets a chevron like a submenu.
func (m *MenuW) ItemPanel(key string, fn func()) *MenuW {
	if m.panels == nil {
		m.panels = map[string]func(){}
	}
	m.panels[key] = fn
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

// Accordion keeps only one branch of inline submenus open (Element unique-opened).
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
	if !m.darkLook() {
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
	if m.openKeys != nil {
		// Controlled open keys: the bound slice is the truth each frame.
		clear(m.st.open)
		for _, k := range *m.openKeys {
			m.st.open[k] = true
		}
	}
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
		if m.noEllipsis {
			w = 0
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
	tk := &th.Components.Menu
	if m.mode == MenuHorizontal {
		b.n.ln.Height = tk.HorizontalHeight
		b.n.ln.Align = layout.AlignStretch
		b.n.ln.Gap = tk.HorizontalItemGap
		if hb := tk.HorizontalBg; hb.A > 0 && !m.dark {
			b.Bg(hb) // shadcn NavigationMenu sits on the page (no sidebar colors)
		}
		if bc := tk.HorizontalBorderColor; !m.dark && bc.A > 0 {
			b.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Bottom()-1, r.W, bc) })
		}
	} else {
		w := m.width
		if m.collapsed {
			w = tk.CollapsedWidth
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
		if h := tk.SubMenuItemHeight; level > 0 && !m.popupMode() && h > 0 {
			n.ln.Height = h
		}
		mi, mb := tk.ItemMarginInline, tk.ItemMarginBlock
		n.ln.Margin = geom.Insets{Top: mb, Bottom: mb, Left: mi, Right: mi}
		disabled := it.Disabled || n.disabled
		ia := c.interact(n, true, input.CursorPointer)
		if disabled {
			n.disabled, n.focus, n.cursor = true, false, input.CursorNotAllowed
			n.setState(a11y.Disabled)
		}
		if level == 0 {
			m.st.items = append(m.st.items, n.id)
		}
		idx := m.record(n, navEntry{id: n.id, key: it.Key, level: level, kids: hasKids, open: open, disabled: disabled, parent: parent})
		popOpen := m.popupMode() && level == 0 && m.st.pop == it.Key
		// A click trigger only stops hover from opening submenus; a
		// collapsed leaf still shows its tooltip on hover.
		if level == 0 && m.popupMode() && ia.Hovered && !disabled && m.st.pop != it.Key && !m.st.kbd && (!m.clickOpen() || !hasKids && !m.clickPopOpen()) {
			m.st.pop = it.Key
			m.st.popup.path = nil
			c.invalid = true
		}
		if ia.Clicked && !disabled {
			switch {
			case hasKids && !m.popupMode():
				m.setOpen(it.Key, !open)
			case hasKids:
				m.a11yPop(n.id, it.Key)
			default:
				m.choose(it.Key)
				m.st.pop = ""
			}
			c.invalid = true
		}
		indent := m.levelIndent(level)
		if ci := tk.ItemContentInset; ci > 0 {
			indent = ci // nesting comes from the submenu guide column
		}
		icononly := m.collapsed && level == 0
		id := n.id
		arrow := c.Anim(id, "arrow", b2f(open), th.DurationMid)
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(tk.ItemBorderRadius)
			s := c.inline(r)
			hov := c.Anim(id, "hover", b2f((ia.Hovered || popOpen) && !disabled), th.DurationMid)
			col := fg
			if selected || childSel {
				col = selFg
				if childSel && !m.dark && !el {
					col = fgActive
				}
			}
			danger := it.Danger
			dcol, dsel := tk.DangerItemColor, tk.DangerItemSelectedBg
			if m.darkLook() {
				dcol, dsel = tk.DarkDangerItemColor, tk.DarkDangerItemSelectedBg
			}
			if danger {
				col = dcol
				if selected && m.darkLook() {
					col = selFg // antd darkDangerItemSelectedColor: light solid
				}
			}
			sb := selBg
			if danger && dsel.A > 0 {
				sb = dsel
			}
			if selected && !hasKids && sb.A > 0 {
				l.Fill(r, rad, sb)
			} else if childSel && icononly && sb.A > 0 {
				l.Fill(r, rad, sb)
			} else if hov > 0 {
				if hoverBg.A > 0 {
					l.Fill(r, rad, theme.Fade(hoverBg, float64(hov)))
				}
				if m.dark && !el && !danger {
					col = theme.Lerp(col, fgActive, hov)
				} else if !m.dark && !selected && !childSel && !danger && tk.ItemHoverColor != fg {
					col = theme.Lerp(col, tk.ItemHoverColor, hov)
				}
			}
			if disabled {
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
				is = tk.CollapsedIconSize
				x = (r.W - is) / 2
			}
			if it.Icon != nil {
				c.drawIconAt(l, it.Icon, s.X(x, is), r.Y+(r.H-is)/2, is, col)
				x += is + tk.IconGap
			} else if icononly {
				// No icon: show the first character.
				lay := navLabel(c, string([]rune(it.Label)[:1]), th.FontSize, text.Regular)
				l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			}
			if !icononly {
				st := c.style(th.FontSize, text.Regular)
				if w := tk.ItemSelectedFontWeight; selected && w > 0 {
					st.Weight = themeWeight(w)
				}
				end := r.W - 16
				if hasKids {
					end -= 20
				}
				var ex *text.Layout
				if it.Shortcut != "" && !hasKids {
					// The extra column (antd item extra) at the end.
					ex = text.Shape(it.Shortcut, c.style(tk.ExtraFontSize, text.Regular), text.Options{NoWrap: true})
					ec := tk.ExtraColor
					if disabled {
						ec = th.TextDisabled
					} else if m.dark {
						ec = theme.Fade(fg, .65)
					}
					l.Glyphs(geom.Pt(s.X(end-ex.Width, ex.Width), r.Y+(r.H-ex.Height)/2), ex.Glyphs, ec)
					end -= ex.Width + 16
				}
				lay := text.Shape(it.Label, st, c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, end-x)}))
				l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
				if hasKids {
					ar := s.Rect(r.W-26, r.Y+(r.H-10)/2, 10, 10)
					switch {
					case m.expandIcon != nil:
						c.paintIcon(l, m.expandIcon, ar, col, 0)
					case th.Flavor == theme.Ant && m.popupMode():
						antdSubmenuArrow(l, s, r, th, col, 0, chevronRight)
					case m.popupMode():
						c.paintIcon(l, icon.Right, ar, col, 0)
					case th.Flavor == theme.Ant:
						antdSubmenuArrow(l, s, r, th, col, arrow, chevronDown)
					default:
						icon.Draw(l, icon.Down, ar, col, float64(arrow)*math.Pi)
					}
				}
			}
			if ia.FocusVisible {
				l.Stroke(r, rad, 2, th.FocusRing)
			}
		}
		if gi := tk.SubMenuGuideInset; hasKids && open && gi > 0 {
			// Children hang off a guide line on the start side (shadcn
			// SidebarMenuSub); the column's margin takes back the items'
			// own inline margin so they start SubMenuItemInset from the
			// menu edge.
			sub := Column(c, func() { m.list(it.Children, level+1, idx) })
			m.subSem(n, sub, it.Label)
			in := tk.SubMenuItemInset - mi
			pb := tk.SubMenuPaddingBlock
			sub.n.ln.Margin = geom.Insets{Left: in, Right: in}
			sub.n.ln.Padding = geom.Insets{Top: pb, Bottom: pb}
			guide := tk.SubMenuGuideColor
			sub.Paint(func(l *paint.List, r geom.Rect) {
				navVLine(l, c.inline(r).X(gi-in, 1), r.Y+pb, r.H-2*pb, guide)
			})
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

// levelIndent returns the start inset of an inline item's content at
// level (antd inlineIndent per level, Element base-level + level padding).
func (m *MenuW) levelIndent(level int) float32 {
	step := m.indent
	if step <= 0 {
		step = m.th.Components.Menu.InlineIndent
	}
	return step * float32(level+1)
}

// choose handles a click on leaf key: it selects the key (unless
// Selectable(false)) and reports it.
func (m *MenuW) choose(key string) {
	if m.sel != nil && m.selectable {
		*m.sel = key
	}
	m.clicked = key
}

// group builds a titled item group (Ant type="group"): the title, then
// the children at the same level, inside a group landmark.
func (m *MenuW) group(it MenuItem, level, parent int) {
	c, th := m.c, m.th
	tk := &th.Components.Menu
	_, fg, _, _, _, _, _ := m.colors()
	g := Column(c, func() {
		if !(m.collapsed && level == 0) {
			t := c.add("menu-group-title", it.Label)
			t.aHidden = true // the group carries the name
			fs, h, col := tk.GroupTitleFontSize, tk.GroupTitleHeight, tk.GroupTitleColor
			x := tk.GroupTitlePaddingInline
			if level > 0 && tk.GroupTitleNested {
				x += m.levelIndent(level) - m.levelIndent(0)
			}
			wt := themeWeight(tk.GroupTitleFontWeight)
			if m.darkLook() {
				col = tk.DarkGroupTitleColor
				if col.A == 0 {
					col = theme.Fade(fg, .7)
				}
			}
			t.ln.Height = h
			t.paint = func(l *paint.List, r geom.Rect) {
				lay := text.Shape(it.Label, c.style(fs, wt), c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, r.W-x-8)}))
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
	tk := &th.Components.Menu
	gap := tk.HorizontalItemGap
	if it.Key == menuMoreKey {
		return 56 + gap
	}
	w := text.Width(it.Label, c.style(th.FontSize, themeWeight(tk.HorizontalFontWeight))) + 2*tk.HorizontalItemPaddingInline + gap
	if it.Icon != nil {
		w += 24
	}
	if m.hasHPanel(it) && tk.HorizontalArrowSize > 0 {
		w += tk.HorizontalArrowSize + tk.HorizontalArrowGap
	}
	return w
}

// hasHPanel reports whether a root item of a horizontal menu opens a
// panel (children or an ItemPanel).
func (m *MenuW) hasHPanel(it MenuItem) bool {
	return len(it.Children) > 0 || m.panels[it.Key] != nil
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
	if total <= width+m.th.Components.Menu.HorizontalItemGap+0.5 {
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
	case m.clickSub:
		m.clickToggle(key)
	default:
		m.st.pop = key
	}
}

// menuSubmenuBranch finds the submenu's path and its children.
func menuSubmenuBranch(items []MenuItem, key string) ([]string, []MenuItem) {
	for _, it := range items {
		if !it.Group && !it.Divider && it.Key == key {
			return []string{key}, it.Children
		}
		if path, children := menuSubmenuBranch(it.Children, key); path != nil {
			if !it.Group {
				path = append(path, it.Key)
			}
			return path, children
		}
	}
	return nil, nil
}

// setOpen handles both pointer and keyboard changes to inline submenus.
func (m *MenuW) setOpen(key string, open bool) {
	if open && m.accordion {
		path, children := menuSubmenuBranch(m.items, key)
		for k := range m.st.open {
			if !slices.Contains(path, k) && !containsKey(children, k) {
				delete(m.st.open, k)
			}
		}
	}
	m.st.open[key] = open
	m.syncOpen()
}

// syncOpen reports a change of the open inline submenus and writes them
// to the bound OpenKeys (in item order).
func (m *MenuW) syncOpen() {
	m.openChanged = true
	m.c.invalid = true
	if m.openKeys == nil {
		return
	}
	keys := []string{}
	var walk func(items []MenuItem)
	walk = func(items []MenuItem) {
		for _, it := range items {
			if len(it.Children) == 0 {
				continue
			}
			if !it.Group && m.st.open[it.Key] {
				keys = append(keys, it.Key)
			}
			walk(it.Children)
		}
	}
	walk(m.items)
	*m.openKeys = keys
}

func (m *MenuW) horizontal() {
	c, th := m.c, m.th
	tk := &th.Components.Menu
	el := m.el()
	_, fg, fgActive, _, selBg, selFg, _ := m.colors()
	wt := themeWeight(tk.HorizontalFontWeight)
	for _, it := range m.items {
		it := it
		if it.Divider {
			continue
		}
		n := c.add("menu-h", it.Label)
		hasKids := m.hasHPanel(it)
		n.setRole(a11y.RoleMenuItem)
		if hasKids {
			n.setState(a11y.Expandable|a11y.HasPopup).stateIf(m.st.pop == it.Key, a11y.Expanded)
		}
		n.ln.Measure = func(_, maxH float32) geom.Size { return geom.Size{W: m.hItemWidth(it) - tk.HorizontalItemGap, H: 0} }
		disabled := it.Disabled || n.disabled
		ia := c.interact(n, true, input.CursorPointer)
		if disabled {
			n.disabled, n.focus, n.cursor = true, false, input.CursorNotAllowed
			n.setState(a11y.Disabled)
		}
		m.st.items = append(m.st.items, n.id)
		m.record(n, navEntry{id: n.id, key: it.Key, kids: hasKids, disabled: disabled, parent: -1})
		if hasKids && ia.Hovered && !disabled && m.st.pop != it.Key && !m.st.kbd && !m.clickOpen() {
			m.st.pop = it.Key
			m.st.popup.path = nil
			c.invalid = true
		}
		if !hasKids && ia.Hovered && m.st.pop != "" && !m.st.kbd && !m.clickOpen() {
			m.st.pop = ""
		}
		if ia.Clicked && !disabled && !hasKids {
			m.choose(it.Key)
			c.invalid = true
		}
		if _, _, ok := c.A11yAction(n.id); ok && ia.Clicked && hasKids && !disabled {
			m.a11yPop(n.id, it.Key)
		} else if ia.Clicked && hasKids && !disabled && m.clickSub && !navNonPointer(c, n.id) {
			m.clickToggle(it.Key)
		}
		selected := m.sel != nil && (*m.sel == it.Key || containsKey(it.Children, *m.sel))
		n.stateIf(selected, a11y.Selected|a11y.Current)
		popOpen := m.st.pop == it.Key
		id := n.id
		turn := c.Anim(id, "arrow", b2f(popOpen), th.DurationSlow)
		n.paint = func(l *paint.List, r geom.Rect) {
			hov := c.Anim(id, "hover", b2f((ia.Hovered || popOpen) && !disabled), th.DurationMid)
			col := theme.Lerp(fg, fgActive, hov)
			if selected {
				col = fgActive
			}
			pill := tk.HorizontalItemHoverBg.A > 0 && tk.HorizontalItemRadius > 0
			switch {
			case pill:
				// shadcn NavigationMenuTrigger: ghost pill, accent on hover,
				// accent/50 while its panel is open, accent when active.
				col = fg
				rad := geom.Rad(tk.HorizontalItemRadius)
				switch {
				case ia.Hovered && !disabled:
					l.Fill(r, rad, tk.HorizontalItemHoverBg)
				case selected && tk.HorizontalItemSelectedBg.A > 0:
					l.Fill(r, rad, tk.HorizontalItemSelectedBg)
				case popOpen && tk.HorizontalItemOpenBg.A > 0:
					l.Fill(r, rad, tk.HorizontalItemOpenBg)
				}
			case m.dark && !el:
				if selected {
					l.Fill(r, geom.Radii{}, selBg)
					col = selFg
				}
			default:
				a := float64(hov)
				if selected {
					a = 1
				}
				if bh := tk.HorizontalActiveBarHeight; a > 0 && bh > 0 {
					in := tk.HorizontalActiveBarInset
					l.Fill(geom.Rect{X: r.X + in, Y: r.Bottom() - bh, W: r.W - 2*in, H: bh}, geom.Radii{}, theme.Fade(fgActive, a))
				}
				if hb := tk.HorizontalItemHoverBg; hb.A > 0 && hov > 0 && !selected {
					l.Fill(r.Inset(geom.Insets{Bottom: tk.HorizontalActiveBarHeight}), geom.Radii{}, theme.Fade(hb, a))
				}
			}
			if it.Danger {
				col = tk.DangerItemColor
				if m.darkLook() {
					col = tk.DarkDangerItemColor
				}
			}
			if disabled {
				col = th.TextDisabled
			}
			if it.Key == menuMoreKey {
				ic := m.moreIcon
				if ic == nil {
					ic = icon.More
				}
				c.drawIconAt(l, ic, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, col)
				if ia.FocusVisible {
					menuHFocusRing(l, th, r)
				}
				return
			}
			s := c.inline(r)
			x := tk.HorizontalItemPaddingInline
			if it.Icon != nil {
				c.drawIconAt(l, it.Icon, s.X(x, 14), r.Y+(r.H-14)/2, 14, col)
				x += 24
			}
			lay := navLabel(c, it.Label, th.FontSize, wt)
			l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			if as := tk.HorizontalArrowSize; hasKids && as > 0 {
				ic := m.expandIcon
				if ic == nil {
					ic = icon.Down
				}
				// The chevron turns over while the panel is open (shadcn
				// rotate-180) when the token asks for it.
				rot := 0.0
				if tk.HorizontalArrowTurn {
					rot = float64(turn) * math.Pi
				}
				icon.Draw(l, ic, s.Rect(x+lay.Width+tk.HorizontalArrowGap, r.Y+(r.H-as)/2, as, as), col, rot)
			}
			if ia.FocusVisible {
				menuHFocusRing(l, th, r)
			}
		}
	}
}

// clickOpen reports click-triggered pop-out submenus
// (TriggerSubMenuAction(TriggerClick)): hovering does not open them.
func (m *MenuW) clickOpen() bool { return m.clickSub }

// clickPopOpen reports a click-opened submenu currently shown (a
// collapsed leaf's hover tooltip does not replace it).
func (m *MenuW) clickPopOpen() bool {
	if !m.clickSub || m.st.pop == "" {
		return false
	}
	it, ok := menuRootItem(m.items, m.st.pop)
	return ok && len(it.Children) > 0
}

// darkLook reports the dark sider colors in use (MenuToken.DarkTheme:
// shadcn ignores Dark).
func (m *MenuW) darkLook() bool { return m.dark && m.th.Components.Menu.DarkTheme }

// clickToggle opens or closes the pop-out submenu of root key on a click.
func (m *MenuW) clickToggle(key string) {
	if m.st.pop == key {
		m.st.pop = ""
	} else {
		m.st.pop = key
	}
	m.st.popup.path = nil
	m.c.invalid = true
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
			m.setOpen(e.key, true)
		case m.act+1 < len(m.ents) && m.ents[m.act+1].parent == m.act:
			if j := navStep(m.ents, m.act, 1, nil); j >= 0 {
				navFocus(c, m.ents[j].id)
			}
		}
	case input.KeyLeft:
		switch {
		case e.kids && e.open:
			m.setOpen(e.key, false)
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
	shown := HoverDelay(c, hashID(m.id, "pop"), over, m.openDelay, m.closeDelay) || m.st.kbd || m.clickPopOpen()
	if !shown || m.st.pop == "" {
		if m.st.pop != "" && !over && !m.clickPopOpen() {
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
	mk := &m.th.Components.Menu
	if fn := m.panels[item.Key]; fn != nil && m.mode == MenuHorizontal {
		m.itemPanel(fn, anchor, mk)
		return
	}
	if len(item.Children) == 0 {
		if m.collapsed && m.tooltip {
			navTooltipBubble(c, anchor, item.Label, InlineEnd)
		}
		m.st.panels = nil
		return
	}
	a := c.AnimFrom(hashID(m.id, "pop", item.Key), "open", 0, 1, c.Theme.DurationMid)
	pop := &navMenuPopup{c: c, th: m.th, tk: menuPopupToken(mk), menu: mk, st: &m.st.popup, dark: m.darkLook(), element: m.el(), opacity: a}
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
	place, off := InlineEndTop, mk.PopupOffset
	if m.mode == MenuHorizontal {
		place, off = BottomStart, mk.HorizontalPopupOffset
	}
	pop.panel(item.Children, 0, anchor, place, off, nil)
	m.st.panels = pop.panels
	if pop.clicked != "" {
		m.choose(pop.clicked)
		// Restore focus without turning a pointer selection into keyboard focus.
		visible := c.focusVisible
		m.closePop(pop.hasFocus())
		c.focusVisible = visible
	}
}

// itemPanel shows the ItemPanel content of the open root item: a panel
// under it in the pop-out look (shadcn NavigationMenuContent mt-1.5
// rounded-md border bg-popover shadow p-2 pr-2.5).
func (m *MenuW) itemPanel(fn func(), anchor ID, mk *theme.MenuToken) {
	c := m.c
	a := c.AnimFrom(hashID(m.id, "pop", m.st.pop), "open", 0, 1, c.Theme.DurationMid)
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomStart, Offset: mk.HorizontalPopupOffset, Flip: true}, fn,
		layout.Pad(8))
	ov.Bg(mk.PopupBg).Radius(mk.PopupRadius).Shadow(mk.PopupShadow).Opacity(a)
	if mk.PopupBorderColor.A > 0 {
		ov.Border(1, mk.PopupBorderColor)
	}
	ov.n.setRole(a11y.RoleGroup)
	if it, ok := menuRootItem(m.items, m.st.pop); ok {
		ov.n.setName(it.Label)
	}
	m.st.popup.layer = ov.n.id
	m.st.panels = []ID{ov.n.id}
	if m.st.popup.focus > 0 {
		// Opened from the keyboard: move into the panel.
		m.st.popup.focus = 0
		if f := navFirstFocusable(ov.n); f != 0 {
			navFocus(c, f)
		}
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
		SubmenuArrowColor: k.PopupArrowColor, SubmenuArrowSize: k.PopupArrowSize, SubmenuArrowInset: k.PopupArrowInset,
		GroupTitleHeight: k.PopupGroupTitleHeight, GroupTitleFontSize: k.PopupGroupTitleFontSize,
		GroupTitleFontWeight: k.PopupGroupTitleFontWeight, GroupTitleColor: k.PopupGroupTitleColor,
		DividerColor:  k.PopupDividerColor,
		ShortcutColor: k.PopupShortcutColor, ShortcutFontSize: k.PopupShortcutFontSize,
		ShortcutLetterSpacing: k.PopupShortcutLetterSpacing, ShortcutGap: k.PopupShortcutGap,
	}
}

// menuHFocusRing outlines a focused horizontal menu item. antd puts its
// focus outline (genFocusOutline: outlineOffset 1) on the whole
// .ant-menu-item, active bar included: the ring encloses the bar instead of
// crossing it. An inset ring drew its lower edge across the bar and left
// the bar's ends sticking out below it like a stray underline.
// It keeps the menu's 2px FocusRing (as the inline items) with antd's 1px
// outline offset, so the line sits 1px outside the item.
func menuHFocusRing(l *paint.List, th *theme.Theme, r geom.Rect) {
	const w, offset = 2, 1
	g := float32(offset + w/2)
	l.Stroke(r.Expand(g), geom.Rad(th.RadiusSM).Grow(g), w, th.FocusRing)
}
