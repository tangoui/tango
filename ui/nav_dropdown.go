package ui

import (
	"time"
	"unicode"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Trigger selects how a popup opens.
type Trigger uint8

const (
	TriggerHover       Trigger = iota // pointer rests on the trigger (default)
	TriggerClick                      // click toggles
	TriggerContextMenu                // secondary click / long press, opens at the pointer
)

// ---------------------------------------------------------------------------
// Popup menu panels shared by Dropdown and Menu (pop-out submenus).
//
// Keyboard: Up/Down move the highlight within a panel, Right (or Enter on a
// submenu) opens the submenu and moves into it, Left closes the current
// submenu and returns to its parent item, Enter/Space choose a leaf. While
// the keyboard drives the panels, hovering is ignored until the pointer
// moves.

type navPopupState struct {
	path   []string // open submenu key per level
	focus  int      // >0: focus the first item of level focus-1 once built
	kbd    bool     // keyboard navigation active
	ptr    geom.Point
	ids    []ID // item IDs of all panels (this build)
	layer  ID   // overlay of the deepest open panel (routes Escape)
	reveal ID   // item moved to by the keyboard; scrolled into view once
	// bare opens the panels with no item highlighted: the root panel
	// itself takes keyboard focus (not a Tab stop) until Up/Down, Home/End
	// or typeahead reach an item; Enter/Space on it do nothing.
	bare      bool
	focusRoot bool // bare: focus the root panel once built
	root      ID   // bare: the root panel (receives the keys while focused)
}

// escape reports an Escape press routed to the open panels.
func (s *navPopupState) escape(c *Context) bool { return s.layer != 0 && c.EscapeAt(s.layer) }

// keyboard marks keyboard navigation; it ends when the pointer moves.
func (s *navPopupState) keyboard(c *Context) { s.kbd, s.ptr = true, c.pointer }

type navMenuPopup struct {
	c        *Context
	th       *theme.Theme         // panel theme (nil: c.Theme, see look)
	tk       *theme.DropdownToken // panel token (nil: th's Dropdown token)
	menu     *theme.MenuToken     // a Menu's pop-out submenus: its Dark* colors
	st       *navPopupState
	dark     bool
	selected string
	clicked  string
	panels   []ID
	minW     float32
	maxH     float32 // panel height limit (0: the window height)
	opacity  float32
	element  bool
	// side handles Left on the root panel (dir -1) and Right on a root
	// item without children (dir +1); nil ignores them.
	side func(dir int)
	deep int // level of the deepest panel built (for st.layer)
	// edge places a root panel opened at a point like a native context
	// menu: its inline-start top corner at the point (top-right in RTL),
	// flipped to the other side of the point when it does not fit.
	edge bool
}

// navEntry is a keyboard-navigable item recorded while building a list.
type navEntry struct {
	id       ID
	key      string
	level    int
	kids     bool
	open     bool
	disabled bool
	parent   int    // index of the parent entry, -1 at the root
	label    string // for typeahead (popup panels)
}

// navStep returns the next enabled entry from i in direction d (wrapping),
// or -1. filter limits the candidates (nil: all).
func navStep(ents []navEntry, i, d int, filter func(e navEntry) bool) int {
	n := len(ents)
	for k := 1; k <= n; k++ {
		j := ((i+d*k)%n + n) % n
		if e := ents[j]; !e.disabled && (filter == nil || filter(e)) {
			return j
		}
	}
	return -1
}

// navFocus moves keyboard focus to id and shows the focus highlight.
func navFocus(c *Context, id ID) {
	c.SetFocus(id)
	c.focusVisible = true
	c.invalid = true
}

// navKeys returns the key presses delivered to the focused widget id.
func navKeys(c *Context, id ID) []input.Key {
	var out []input.Key
	for _, k := range c.KeysFor(id) {
		if k.Down {
			out = append(out, k.Key)
		}
	}
	return out
}

// look returns the theme and Dropdown-shaped token the panels paint with:
// the ones a Dropdown captured, or for the pop-out submenus of a Menu the
// panel token built from the Menu's own Popup* fields (menuPopupToken).
// The scope's Dropdown token is only a fallback.
func (p *navMenuPopup) look() (*theme.Theme, *theme.DropdownToken) {
	if p.th == nil {
		p.th = p.c.Theme
	}
	if p.tk == nil {
		p.tk = &p.th.Components.Dropdown
	}
	return p.th, p.tk
}

// darkSider reports a dark Menu's pop-out panels, which take the sider
// colors (shadcn has a single sidebar look and ignores dark).
func (p *navMenuPopup) darkSider() bool {
	th, _ := p.look()
	return p.dark && th.Flavor != theme.StyleShadcn
}

func (p *navMenuPopup) colors() (bg, fg, hoverBg, hoverFg, selBg, selFg theme.Color) {
	th, tk := p.look()
	if p.darkSider() {
		if m := p.menu; m != nil {
			if p.element {
				// Element keeps the item color under the pointer.
				return m.DarkItemBg, m.DarkItemColor, m.DarkItemHoverBg, m.DarkItemColor, m.DarkItemSelectedBg, m.DarkItemSelectedColor
			}
			return m.DarkItemBg, m.DarkItemColor, m.DarkItemHoverBg, m.DarkItemHoverColor, m.DarkItemSelectedBg, m.DarkItemSelectedColor
		}
		if p.element {
			return th.SiderBg, th.SiderText, th.SiderItemHoverBg, th.SiderText, theme.Color{}, th.SiderTextActive
		}
		return th.SiderBg, th.SiderText, theme.Color{}, theme.Hex("#ffffff"), th.Primary.Base, theme.Hex("#ffffff")
	}
	return tk.PopupBg, tk.ItemColor, tk.ItemHoverBg, tk.ItemHoverColor, tk.ItemSelectedBg, tk.ItemSelectedColor
}

func (p *navMenuPopup) style(ov *Box) {
	_, tk := p.look()
	bg, _, _, _, _, _ := p.colors()
	op := p.opacity
	if op == 0 {
		op = 1
	}
	ov.n.ln.Padding = geom.VH(tk.PaddingBlock, tk.PaddingInline)
	ov.Bg(bg).Radius(tk.PopupRadius).Shadow(tk.PopupShadow).Opacity(op)
	if tk.PopupBorderColor.A > 0 && !p.darkSider() {
		ov.Border(1, tk.PopupBorderColor)
	}
}

// panel builds one menu level. at != nil places it at a window point.
func (p *navMenuPopup) panel(items []MenuItem, level int, anchor ID, place Placement, offset float32, at *geom.Point) ID {
	c := p.c
	if level == 0 {
		p.st.ids = p.st.ids[:0]
		if c.pointer != p.st.ptr {
			p.st.ptr = c.pointer
			p.st.kbd = false
		}
	}
	var ov, layer *Box
	body := func() { p.items(items, level, anchor) }
	// Long menus scroll inside a height-limited panel.
	_, tk := p.look()
	pad := 2 * tk.PaddingBlock
	limit := p.maxH
	if limit <= 0 {
		limit = c.size.H - 16
	}
	limit -= pad
	if limit > 0 && p.estimate(items) > limit {
		inner := body
		body = func() {
			sb := Scroll(c, inner, layout.H(limit))
			if r := p.st.reveal; r != 0 && navHasNode(sb.n, r) && c.ScrollIntoView(sb.n.id, r) {
				p.st.reveal = 0
			}
		}
	}
	if at != nil {
		var inner *Box
		holder := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
			inner = Column(c, body)
			inner.n.ln.Absolute = true
			inner.n.ln.Pos = layout.Pos{Left: at.X, Top: at.Y, Right: layout.Unset, Bottom: layout.Unset}
			inner.n.hit, inner.n.blocks = true, true
		})
		holder.n.hit, holder.n.blocks = false, false
		pt := *at
		edge, rtl := p.edge, c.IsRTL()
		holder.n.paint = func(_ *paint.List, r geom.Rect) {
			ln := &inner.n.ln
			x, y := pt.X, pt.Y
			if edge {
				x, y = navEdgePlace(pt, ln.Rect.W, ln.Rect.H, r.W, r.H, rtl)
			}
			ln.Rect.X = max(4, min(x, r.W-ln.Rect.W-4))
			ln.Rect.Y = max(4, min(y, r.H-ln.Rect.H-4))
		}
		ov, layer = inner, holder
	} else {
		ov = Overlay(c, OverlayOptions{Anchor: anchor, Place: place, Offset: offset, Flip: true}, body)
		layer = ov
	}
	// Submenus are built inside their parent's body, so they finish first;
	// Escape goes to the deepest one.
	if level >= p.deep {
		p.deep = level
		p.st.layer = layer.n.id
	}
	p.style(ov)
	ov.n.setRole(a11y.RoleMenu).setState(a11y.Vertical)
	if level == 0 && p.st.bare {
		// The panel holds focus while nothing is highlighted; it is
		// reachable by keyboard through the items, so Tab skips it.
		ov.n.hit, ov.n.focus, ov.n.noTab = true, true, true
		ov.n.setName(c.Locale().A11y.Menu)
		p.st.root = ov.n.id
		if p.st.focusRoot {
			p.st.focusRoot = false
			c.SetFocus(ov.n.id)
		}
	}
	if p.minW > 0 && level == 0 {
		ov.n.ln.MinW = p.minW
	}
	p.panels = append(p.panels, ov.n.id)
	return ov.n.id
}

// navEdgePlace returns the top-left corner of a w×h context menu opened
// at pt in a vw×vh window: the menu hangs down and towards the inline end
// from the pointer (top-left corner at pt, top-right in RTL) and opens on
// the other side of pt on an axis where it does not fit (4px margin);
// the caller clamps the result into the window.
func navEdgePlace(pt geom.Point, w, h, vw, vh float32, rtl bool) (x, y float32) {
	x, y = pt.X, pt.Y
	if rtl {
		x = pt.X - w
		if x < 4 && pt.X+w <= vw-4 {
			x = pt.X
		}
	} else if x+w > vw-4 && pt.X-w >= 4 {
		x = pt.X - w
	}
	if y+h > vh-4 && pt.Y-h >= 4 {
		y = pt.Y - h
	}
	return x, y
}

// estimate returns the height of a panel's items (without padding).
func (p *navMenuPopup) estimate(items []MenuItem) float32 {
	_, tk := p.look()
	var h float32
	for _, it := range items {
		switch {
		case it.Group:
			th, _, _, _, _ := p.groupTitle()
			h += th + p.estimate(it.Children)
			if p.groupSeps() {
				h += 9
			}
		case it.Divider && p.element:
			h += 7
		case it.Divider:
			h += 9
		case p.dark && p.element:
			h += 50
		case p.dark:
			h += 44
		default:
			h += tk.ItemHeight
		}
	}
	return h
}

// groupTitle returns the height, font size, weight, color and left inset
// of a group title in a popup panel: shadcn DropdownMenuLabel (px-2
// py-1.5, medium foreground), Ant's group title (5px 12px, description
// color), Element's item-group title (12px secondary text).
func (p *navMenuPopup) groupTitle() (h, fs float32, w text.Weight, col theme.Color, x float32) {
	th, tk := p.look()
	_, fg, _, _, _, _ := p.colors()
	switch {
	case p.darkSider() && p.element:
		return 30, th.FontSizeSM, text.Regular, theme.Fade(fg, .7), 20
	case p.darkSider():
		return 32, th.FontSize, text.Regular, theme.Fade(fg, .65), 16
	}
	return tk.GroupTitleHeight, tk.GroupTitleFontSize, themeWeight(tk.GroupTitleFontWeight), tk.GroupTitleColor, tk.ItemPaddingInline
}

// groupSeps reports whether groups are set apart by separators (shadcn
// DropdownMenuSeparator between a labelled group and its neighbours; Ant
// and Element only show the title).
func (p *navMenuPopup) groupSeps() bool {
	th, _ := p.look()
	return th.Flavor == theme.StyleShadcn
}

// divider adds a separator line to a popup panel.
func (p *navMenuPopup) divider() {
	c := p.c
	th, tk := p.look()
	d := c.add("menu-divider", "")
	d.ln.Height = 1
	d.ln.Margin = geom.Insets{Top: 4, Bottom: 4}
	if p.element {
		d.ln.Margin = geom.Insets{Top: 6, Bottom: 0}
	}
	if th.Flavor == theme.StyleShadcn {
		d.ln.Margin = geom.Insets{Top: 4, Bottom: 4, Left: -4, Right: -4}
	}
	d.setRole(a11y.RoleSeparator).setState(a11y.Horizontal)
	d.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, tk.DividerColor) }
}

// group builds a titled group of a popup panel (MenuItem.Group): a
// non-focusable title, then the children at the same level (keyboard
// navigation passes over the title), inside a named group landmark.
func (p *navMenuPopup) group(it MenuItem, i int, items []MenuItem, add func(MenuItem)) {
	c := p.c
	seps := p.groupSeps()
	if seps && i > 0 && !items[i-1].Divider && !items[i-1].Group {
		p.divider()
	}
	h, fs, w, col, x := p.groupTitle()
	g := Column(c, func() {
		if it.Label != "" {
			t := c.add("menu-group-title", it.Label)
			t.aHidden = true // the group carries the name
			t.ln.Height = h
			t.ln.Measure = func(_, _ float32) geom.Size {
				return geom.Size{W: text.Width(it.Label, c.style(fs, w)) + 2*x, H: h}
			}
			t.paint = func(l *paint.List, r geom.Rect) {
				lay := text.Shape(it.Label, c.style(fs, w), c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, r.W-2*x)}))
				l.Glyphs(geom.Pt(c.inline(r).X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			}
		}
		for j, ch := range it.Children {
			if ch.Group {
				p.group(ch, j, it.Children, add)
				continue
			}
			add(ch)
		}
	})
	g.n.setRole(a11y.RoleGroup).setName(it.Label)
	if seps && i+1 < len(items) && !items[i+1].Divider {
		p.divider()
	}
}

// hasFocus reports whether keyboard focus is on one of the panel items.
func (p *navMenuPopup) hasFocus() bool {
	for _, id := range p.st.ids {
		if id == p.c.focus {
			return true
		}
	}
	return false
}

func (p *navMenuPopup) items(items []MenuItem, level int, parent ID) {
	c := p.c
	th, tk := p.look()
	el := p.element
	shad := th.Flavor == theme.StyleShadcn
	_, fg, hoverBg, hoverFg, selBg, selFg := p.colors()
	var ents []navEntry
	act, actKey, actMods := -1, input.Key(0), input.Modifiers(0)
	var add func(it MenuItem)
	add = func(it MenuItem) {
		if it.Divider {
			p.divider()
			return
		}
		hasKids := len(it.Children) > 0
		n := c.add("menu-pop-item", it.Label)
		st := c.style(th.FontSize, text.Regular)
		h := tk.ItemHeight
		if p.dark && !el && !shad {
			h = 40
		}
		if p.dark && el {
			h = 50
		}
		n.ln.Measure = func(_, _ float32) geom.Size {
			w := text.Width(it.Label, st) + 2*tk.ItemPaddingInline
			if it.Icon != nil {
				w += 22
			}
			if hasKids {
				w += 24
			}
			if it.Shortcut != "" {
				w += p.shortcutGap() + p.shortcutWidth(it.Shortcut)
			}
			minW := float32(120)
			if el {
				minW = 100
			}
			if p.dark && !shad {
				minW = 160
			}
			if shad {
				minW = 128
			}
			return geom.Size{W: max(minW, w), H: h}
		}
		if p.dark && !el && !shad {
			n.ln.Margin = geom.Insets{Top: 2, Bottom: 2}
		}
		ia := c.interact(n, true, input.CursorPointer)
		if it.Disabled {
			n.disabled = true
			n.cursor = input.CursorNotAllowed
			n.focus = false
		}
		p.st.ids = append(p.st.ids, n.id)
		ents = append(ents, navEntry{id: n.id, key: it.Key, level: level, kids: hasKids, disabled: it.Disabled, parent: -1, label: it.Label})
		if !it.Disabled {
			for _, k := range c.KeysFor(n.id) {
				if k.Down {
					act, actKey, actMods = len(ents)-1, k.Key, k.Mods
				}
			}
		}
		open := level < len(p.st.path) && p.st.path[level] == it.Key && hasKids
		n.setRole(a11y.RoleMenuItem)
		if it.Shortcut != "" && c.a11yOn {
			sc := it.Shortcut
			n.fill(semFunc(func(s *a11y.Node) { s.Shortcut = sc }))
		}
		if hasKids {
			n.setState(a11y.HasPopup|a11y.Expandable).stateIf(open, a11y.Expanded)
		}
		if ia.Hovered && !it.Disabled && !p.st.kbd {
			want := append([]string(nil), p.st.path[:min(level, len(p.st.path))]...)
			if hasKids {
				want = append(want, it.Key)
			}
			if !navEqualPath(want, p.st.path) {
				p.st.path = want
				c.invalid = true
			}
		}
		if ia.Clicked && !it.Disabled {
			if hasKids {
				if open {
					p.st.path = p.st.path[:level]
				} else {
					p.st.path = append(p.st.path[:min(level, len(p.st.path))], it.Key)
				}
				c.invalid = true
			} else {
				p.clicked = it.Key
			}
		}
		selected := it.Key != "" && (it.Key == p.selected || (hasKids && containsKey(it.Children, p.selected)))
		n.stateIf(selected && !hasKids, a11y.Selected)
		id := n.id
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(tk.ItemBorderRadius)
			if p.dark && !el && !shad {
				rad = geom.Rad(th.RadiusLG)
			}
			hov := c.Anim(id, "hover", b2f((ia.Hovered || open || ia.FocusVisible) && !it.Disabled), th.DurationFast)
			col := fg
			if selected {
				col = selFg
				if selBg.A > 0 && !hasKids {
					l.Fill(r, rad, selBg)
				}
			}
			if hov > 0 {
				if it.Danger && tk.DangerHoverBg.A > 0 {
					l.Fill(r, rad, theme.Fade(tk.DangerHoverBg, float64(hov)))
					col = tk.DangerHoverColor
				} else {
					if hoverBg.A > 0 && !(selected && selBg.A > 0 && !hasKids) {
						l.Fill(r, rad, theme.Fade(hoverBg, float64(hov)))
					}
					if !selected || p.dark {
						col = theme.Lerp(col, hoverFg, hov)
					}
				}
			}
			if it.Danger && hov == 0 {
				col = tk.DangerColor
			}
			if it.Disabled {
				col = th.TextDisabled
			}
			// x is the logical offset from the start edge.
			s := c.inline(r)
			x := tk.ItemPaddingInline
			if p.dark && !shad {
				x = 16
			}
			if it.Icon != nil {
				is, ic := tk.IconSize, col
				if tk.ItemIconColor.A > 0 && !it.Danger && !it.Disabled {
					ic = tk.ItemIconColor // shadcn: muted-foreground icons
				}
				c.drawIconAt(l, it.Icon, s.X(x, is), r.Y+(r.H-is)/2, is, ic)
				x += is + 8
			}
			lay := navLabel(c, it.Label, th.FontSize, text.Regular)
			l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			if it.Shortcut != "" {
				// The shortcut column ends at the item's end padding (before
				// the chevron of a submenu item).
				end := tk.ItemPaddingInline
				if p.dark && !shad {
					end = 16
				}
				if hasKids {
					end = 28
				}
				sc := tk.ShortcutColor
				switch {
				case it.Disabled:
					sc = th.TextDisabled
				case p.darkSider():
					sc = theme.Fade(fg, .65)
				case hov > 0 && it.Danger && tk.DangerHoverBg.A > 0:
					sc = col
				}
				p.drawShortcut(l, it.Shortcut, r, end, sc)
			}
			if hasKids {
				ac := tk.SubmenuArrowColor
				if p.dark || it.Disabled {
					ac = col
				}
				// The submenu chevron points towards the end (mirrored in RTL).
				c.paintIcon(l, icon.Right, s.Rect(r.W-22, r.Y+(r.H-10)/2, 10, 10), ac, 0)
			}
		}
		if open {
			off := float32(4)
			if el {
				off = 6
			}
			sub := p.panel(it.Children, level+1, n.id, InlineEndTop, off, nil)
			if c.a11yOn {
				n.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(sub) }))
			}
		}
	}
	for i, it := range items {
		if !it.Group {
			add(it)
			continue
		}
		p.group(it, i, items, add)
	}
	if p.st.focus == level+1 {
		p.st.focus = 0
		if j := navStep(ents, -1, 1, nil); j >= 0 {
			navFocus(c, ents[j].id)
		}
	}
	if level == 0 && p.st.root != 0 {
		for _, k := range c.KeysFor(p.st.root) {
			if k.Down {
				p.rootKey(ents, k.Key, k.Mods)
			}
		}
	}
	if act >= 0 {
		// Arrow keys are visual: submenus open towards the end, so in RTL
		// Left opens one and Right closes it. key works on the logical keys.
		if prev, next := c.inlineKey(actKey); prev {
			actKey = input.KeyLeft
		} else if next {
			actKey = input.KeyRight
		}
		p.key(ents, act, actKey, actMods, level, parent)
	}
}

// shortcutGap is the least space between an item's label and its
// shortcut.
func (p *navMenuPopup) shortcutGap() float32 {
	_, tk := p.look()
	return tk.ShortcutGap
}

// shortcutLayout shapes a shortcut text. Shortcuts keep their own
// direction (key names read left to right even in RTL menus).
func (p *navMenuPopup) shortcutLayout(s string) *text.Layout {
	th, tk := p.look()
	fs := tk.ShortcutFontSize
	if fs <= 0 {
		fs = th.FontSize
	}
	return text.Shape(s, p.c.style(fs, text.Regular), text.Options{NoWrap: true})
}

// shortcutWidth returns the advance of a shortcut including its letter
// spacing (added after every glyph, as CSS letter-spacing does).
func (p *navMenuPopup) shortcutWidth(s string) float32 {
	_, tk := p.look()
	lay := p.shortcutLayout(s)
	return lay.Width + tk.ShortcutLetterSpacing*float32(len(lay.Glyphs))
}

// drawShortcut draws the shortcut column of an item in r, its logical end
// inset end from r's inline end edge.
func (p *navMenuPopup) drawShortcut(l *paint.List, s string, r geom.Rect, end float32, col theme.Color) {
	_, tk := p.look()
	lay := p.shortcutLayout(s)
	ls := tk.ShortcutLetterSpacing
	w := lay.Width + ls*float32(len(lay.Glyphs))
	glyphs := lay.Glyphs
	if ls != 0 {
		glyphs = append([]paint.Glyph(nil), lay.Glyphs...)
		for i := range glyphs {
			glyphs[i].X += ls * float32(i)
		}
	}
	x := p.c.inline(r).X(r.W-end-w, w)
	l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), glyphs, col)
}

// key applies one navigation key pressed on ents[i].
func (p *navMenuPopup) key(ents []navEntry, i int, k input.Key, mods input.Modifiers, level int, parent ID) {
	c, e := p.c, ents[i]
	switch k {
	case input.KeyDown, input.KeyUp:
		d := 1
		if k == input.KeyUp {
			d = -1
		}
		if j := navStep(ents, i, d, nil); j >= 0 {
			navFocus(c, ents[j].id)
		}
	case input.KeyHome:
		if j := navStep(ents, -1, 1, nil); j >= 0 {
			navFocus(c, ents[j].id)
		}
	case input.KeyEnd:
		if j := navStep(ents, 0, -1, nil); j >= 0 {
			navFocus(c, ents[j].id)
		}
	case input.KeyRight:
		// (Arrow keys are mapped to logical Left/Right by the caller.)
		if e.kids {
			p.st.path = append(p.st.path[:min(level, len(p.st.path))], e.key)
			p.st.focus = level + 2
		} else if level == 0 && p.side != nil {
			p.side(1)
		}
	case input.KeyEnter, input.KeySpace:
		// The implicit click already toggled the submenu; move into it.
		if e.kids && level < len(p.st.path) && p.st.path[level] == e.key {
			p.st.focus = level + 2
		}
	case input.KeyLeft:
		if level > 0 {
			p.st.path = p.st.path[:min(level-1, len(p.st.path))]
			navFocus(c, parent)
		} else if p.side != nil {
			p.side(-1)
		}
	default:
		// Typeahead: a letter or digit moves to the next item of this
		// panel whose label starts with it.
		r := k.Rune()
		if mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 || !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return
		}
		j := navStep(ents, i, 1, func(e navEntry) bool { return e.level == level && navStartsWith(e.label, r) })
		if j < 0 {
			return
		}
		navFocus(c, ents[j].id)
	}
	p.st.keyboard(c)
	p.st.reveal = c.focus
	c.invalid = true
}

// rootKey applies one key pressed while the root panel of bare panels
// has focus (no item highlighted): Down/Home reach the first enabled
// item, Up/End the last, a letter or digit the first item starting with
// it; other keys (Enter, Space, Left, Right) do nothing.
func (p *navMenuPopup) rootKey(ents []navEntry, k input.Key, mods input.Modifiers) {
	j := -1
	switch k {
	case input.KeyDown, input.KeyHome:
		j = navStep(ents, -1, 1, nil)
	case input.KeyUp, input.KeyEnd:
		j = navStep(ents, 0, -1, nil)
	default:
		r := k.Rune()
		if mods&(input.ModCtrl|input.ModAlt|input.ModMeta) != 0 || !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return
		}
		j = navStep(ents, -1, 1, func(e navEntry) bool { return e.level == 0 && navStartsWith(e.label, r) })
	}
	if j < 0 {
		return
	}
	navFocus(p.c, ents[j].id)
	p.st.keyboard(p.c)
	p.st.reveal = p.c.focus
	p.c.invalid = true
}

// navStartsWith reports whether label starts with the letter or digit r
// (ignoring case).
func navStartsWith(label string, r rune) bool {
	for _, f := range label {
		return unicode.ToLower(f) == r
	}
	return false
}

func navEqualPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// navHasNode reports whether id is n or one of its descendants.
func navHasNode(n *node, id ID) bool {
	if id == 0 {
		return false
	}
	if n.id == id {
		return true
	}
	for _, k := range n.kids {
		if navHasNode(k, id) {
			return true
		}
	}
	return false
}

// popupSem completes the node of a popup trigger: the expanded state (as
// of the end of the frame) and the panel it controls.
func popupSem(s *a11y.Node, open bool, panels []ID) {
	if !open {
		return
	}
	s.States |= a11y.Expanded
	if s.Actions.Has(a11y.Expand) {
		s.Actions = s.Actions&^a11y.Expand | a11y.Collapse
	}
	if len(panels) > 0 {
		s.Controls = a11y.NodeID(panels[0])
	}
}

// navFirstFocusable returns the first focusable node in n's subtree.
func navFirstFocusable(n *node) ID {
	if f := navFirstFocusableNode(n); f != nil {
		return f.id
	}
	return 0
}

// navFirstFocusableNode is navFirstFocusable returning the node.
func navFirstFocusableNode(n *node) *node {
	if n.focus {
		return n
	}
	for _, k := range n.kids {
		if f := navFirstFocusableNode(k); f != nil {
			return f
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dropdown

type dropdownState struct {
	open      bool
	at        geom.Point
	atPointer bool
	panels    []ID
	pressAt   time.Time
	popup     navPopupState
}

// DropdownW is a menu attached to a trigger: hover / click / context-menu
// triggers, nested submenus, placements, arrow, selected key, keyboard
// navigation, and the split-button form (Button).
type DropdownW struct {
	c        *Context
	th       *theme.Theme         // ThemeFor(CompDropdown)
	tk       *theme.DropdownToken // &th.Components.Dropdown
	items    []MenuItem
	trigger  Trigger
	place    Placement
	placeSet bool
	arrow    bool
	selected string
	disabled bool
	minW     float32
	maxH     float32
	st       *dropdownState
	id       ID
	clicked  string
	changed  bool
	openPtr  *bool

	// Button form.
	primary, danger bool
	size            ControlSize
	sizeSet         bool
	ic              *icon.Icon
	btnClicked      bool
}

// Dropdown starts a dropdown menu; chain options, then Body with the
// trigger content, or Button for a split button (Ant Dropdown.Button,
// Element split-button):
//
//	if k := ui.Dropdown(c, items).Trigger(ui.TriggerClick).Body(func() { ui.Button(c, "更多") }).Clicked(); k != "" { ... }
//	dd := ui.Dropdown(c, items).Primary().Button("提交")
func Dropdown(c *Context, items []MenuItem) *DropdownW {
	d := &DropdownW{c: c, items: items, place: BottomStart}
	d.th = c.ThemeFor(theme.CompDropdown)
	d.tk = &d.th.Components.Dropdown
	d.defaults() // ConfigProvider.Defaults("Dropdown", ...): trigger, placement, arrow, disabled
	return d
}

// Trigger selects hover (default), click or context-menu activation.
func (d *DropdownW) Trigger(t Trigger) *DropdownW { d.trigger = t; return d }

// Placement positions the menu (BottomStart, Bottom, BottomEnd, TopStart, Top, TopEnd).
func (d *DropdownW) Placement(p Placement) *DropdownW { d.place, d.placeSet = p, true; return d }

// Arrow draws a pointer towards the trigger.
func (d *DropdownW) Arrow() *DropdownW { d.arrow = true; return d }

// Selected highlights an item.
func (d *DropdownW) Selected(key string) *DropdownW { d.selected = key; return d }

// Disabled prevents opening (and disables the Button form).
func (d *DropdownW) Disabled(on bool) *DropdownW { d.disabled = on; return d }

// MinWidth sets the menu's minimum width.
func (d *DropdownW) MinWidth(px float32) *DropdownW { d.minW = px; return d }

// MaxHeight limits the menu panels' height (default: the window height);
// longer menus scroll and keyboard moves keep the active item visible.
func (d *DropdownW) MaxHeight(px float32) *DropdownW { d.maxH = px; return d }

// Open binds the open state.
func (d *DropdownW) Open(open *bool) *DropdownW { d.openPtr = open; return d }

// Primary styles the Button form as primary.
func (d *DropdownW) Primary() *DropdownW { d.primary = true; return d }

// Danger styles the Button form as dangerous.
func (d *DropdownW) Danger() *DropdownW { d.danger = true; return d }

// Size sets the control size of the Button form.
func (d *DropdownW) Size(s ControlSize) *DropdownW { d.size, d.sizeSet = s, true; return d }

// MenuIcon replaces the menu button icon of the Button form.
func (d *DropdownW) MenuIcon(ic *icon.Icon) *DropdownW { d.ic = ic; return d }

// Clicked returns the key chosen this frame ("" if none).
func (d *DropdownW) Clicked() string { return d.clicked }

// ButtonClicked reports a click on the main part of the Button form.
func (d *DropdownW) ButtonClicked() bool { return d.btnClicked }

// IsOpen reports whether the menu is shown.
func (d *DropdownW) IsOpen() bool { return d.st != nil && d.st.open }

// OpenChanged reports that the menu opened or closed this frame.
func (d *DropdownW) OpenChanged() bool { return d.changed }

// ID returns the trigger box ID.
func (d *DropdownW) ID() ID { return d.id }

// OpenAt opens the menu at a window point (context menus).
func (d *DropdownW) OpenAt(p geom.Point) *DropdownW {
	if d.st != nil {
		d.setOpen(true)
		d.st.at, d.st.atPointer = p, true
	}
	return d
}

func (d *DropdownW) setOpen(v bool) {
	if d.st.open != v {
		d.st.open = v
		d.changed = true
		d.c.invalid = true
		if !v {
			d.st.popup.path = nil
			d.st.popup.kbd = false
			d.st.atPointer = false
			SetHoverOpen(d.c, d.id, false)
		}
	}
	if d.openPtr != nil {
		*d.openPtr = v
	}
}

// Body builds the trigger and, when open, the menu.
func (d *DropdownW) Body(trigger func()) *DropdownW {
	c, th := d.c, d.th
	el := navEl(c)
	b := c.box("dropdown", layout.Row, nil)
	b.n.ln.NoStretch = true
	b.n.ln.Align = layout.AlignCenter
	b.Body(trigger)
	d.id = b.n.id
	d.st = State[dropdownState](c, b.n.id, nil)
	if d.openPtr != nil && *d.openPtr != d.st.open {
		d.st.open = *d.openPtr
	}
	disabled := d.disabled || b.n.disabled
	// The trigger is announced as a popup button: a focusable control
	// inside it (Button, Link) carries the popup states; without one the
	// box itself becomes a focusable button named by its content.
	fn := navFirstFocusableNode(b.n)
	var ia Interaction
	if fn == nil {
		ia = c.interact(b.n, !disabled, input.CursorPointer)
		b.ia = &ia
		b.n.setRole(a11y.RoleButton).stateIf(disabled, a11y.Disabled)
		fn = b.n
		prev := b.n.over
		b.n.over = func(l *paint.List, r geom.Rect) {
			if prev != nil {
				prev(l, r)
			}
			if ia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			}
		}
	} else {
		ia = b.Interact(input.CursorPointer)
		b.n.cursor = input.CursorDefault // the control inside is the pointer target
	}
	if d.trigger == TriggerContextMenu {
		b.n.cursor = input.CursorDefault
	}
	fn.setState(a11y.HasPopup | a11y.Expandable)
	if c.a11yOn {
		st := d.st
		fn.fill(semFunc(func(s *a11y.Node) { popupSem(s, st.open, st.panels) }))
	}
	inside := func(p geom.Point) bool {
		if r, ok := c.rects[b.n.id]; ok && r.Contains(p) {
			return true
		}
		return false
	}
	// Keyboard on the trigger: Down, Enter or Space open the menu and
	// enter it; a context-menu area opens with the Menu key, Shift+F10 or
	// (when the area itself has focus) Enter / Space.
	kbdOpen := false
	if !disabled && navHasNode(b.n, c.focus) {
		for _, k := range c.KeysFor(c.focus) {
			if !k.Down {
				continue
			}
			if d.trigger == TriggerContextMenu {
				if k.Key == input.KeyContextMenu || k.Key == input.KeyF10 && k.Mods&input.ModShift != 0 ||
					fn == b.n && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
					if r, ok := c.rects[b.n.id]; ok {
						d.OpenAt(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
					} else {
						d.OpenAt(c.pointer)
					}
					d.st.popup.focus = 1
					d.st.popup.keyboard(c)
				}
				continue
			}
			switch k.Key {
			case input.KeyDown:
				d.setOpen(true)
				SetHoverOpen(c, b.n.id, true)
				d.st.popup.focus = 1
				d.st.popup.keyboard(c)
			case input.KeyEnter, input.KeySpace:
				kbdOpen = true
			}
		}
	}
	// Enter / Space on a box trigger are its implicit click already.
	kbdClick := kbdOpen && fn != b.n
	if kbdOpen && !d.st.open {
		d.st.popup.focus = 1
		d.st.popup.keyboard(c)
	}
	popFocus := func() bool {
		for _, id := range d.st.popup.ids {
			if id == c.focus {
				return true
			}
		}
		return false
	}
	if !disabled {
		switch d.trigger {
		case TriggerClick:
			if ia.Clicked || c.clickedWithin(b.n) || kbdClick {
				d.setOpen(!d.st.open)
			}
		case TriggerHover:
			over := c.pointerOver(append([]ID{b.n.id}, d.st.panels...)...)
			want := HoverDelay(c, b.n.id, over, 150*time.Millisecond, 100*time.Millisecond)
			if !want && d.st.open && d.st.popup.kbd {
				want = true // keyboard navigation keeps it open until the pointer moves
			}
			if want != d.st.open {
				d.setOpen(want)
			}
			if (ia.Clicked || kbdClick) && !d.st.open {
				d.setOpen(true)
				SetHoverOpen(c, b.n.id, true)
			}
		case TriggerContextMenu:
			if p, ok := c.ContextMenuFor(b.n.id); ok {
				d.OpenAt(p)
			} else if ContextMenuSource != nil {
				if p, ok := ContextMenuSource(c); ok && inside(p) {
					d.OpenAt(p)
				}
			}
			if ia.PressedNow {
				d.st.pressAt = c.now
			}
			if c.Dragging(b.n.id) && !d.st.pressAt.IsZero() {
				if c.now.Sub(d.st.pressAt) >= 500*time.Millisecond {
					d.st.pressAt = time.Time{}
					d.OpenAt(c.pointer)
				} else {
					c.WakeAt(d.st.pressAt.Add(500 * time.Millisecond))
				}
			} else {
				d.st.pressAt = time.Time{}
			}
		}
	}
	if d.st.open {
		ids := append([]ID(nil), d.st.panels...)
		if d.trigger != TriggerContextMenu {
			ids = append(ids, b.n.id)
		}
		if c.PressedOutside(ids...) {
			d.setOpen(false)
		} else if d.st.popup.escape(c) {
			if popFocus() {
				c.SetFocus(navFirstFocusable(b.n))
			}
			d.setOpen(false)
		}
	}
	if !d.st.open || disabled {
		d.st.panels = nil
		d.st.popup.ids = d.st.popup.ids[:0]
		d.st.popup.layer = 0
		return d
	}
	a := c.AnimFrom(b.n.id, "open", 0, 1, th.DurationMid)
	pop := &navMenuPopup{c: c, th: th, tk: d.tk, st: &d.st.popup, selected: d.selected, element: el, opacity: a, minW: d.minW, maxH: d.maxH}
	off := float32(4)
	if el {
		off = 8
	}
	if d.arrow {
		off += 6
	}
	var root ID
	if d.trigger == TriggerContextMenu || d.st.atPointer {
		at := d.st.at
		root = pop.panel(d.items, 0, 0, 0, 0, &at)
	} else {
		root = pop.panel(d.items, 0, b.n.id, d.place, off, nil)
		if d.arrow || el {
			d.drawArrow(root)
		}
	}
	d.st.panels = pop.panels
	if pop.clicked != "" {
		d.clicked = pop.clicked
		if pop.hasFocus() {
			c.SetFocus(navFirstFocusable(b.n))
		}
		d.setOpen(false)
	}
	return d
}

// drawArrow adds the pointer triangle to the root panel.
func (d *DropdownW) drawArrow(panel ID) {
	c, tk := d.c, d.tk
	ov := c.overlays[len(c.overlays)-1]
	for _, o := range c.overlays {
		if o.root.id == panel {
			ov = o
		}
	}
	anchor := d.id
	place := d.place.Resolve(c.IsRTL()) // BottomStart aligns to the right edge in RTL
	prev := ov.root.over
	ov.root.over = func(l *paint.List, r geom.Rect) {
		if prev != nil {
			prev(l, r)
		}
		ar, ok := c.anchorRect(anchor)
		if !ok {
			return
		}
		cx := ar.X + min(ar.W/2, 24)
		switch place {
		case Bottom, Top:
			cx = ar.X + ar.W/2
		case BottomEnd, TopEnd:
			cx = ar.Right() - min(ar.W/2, 24)
		}
		cx = max(r.X+12, min(cx, r.Right()-12))
		p := paint.NewPath()
		up := r.Y >= ar.Bottom()-1
		if up {
			p.MoveTo(cx-6, r.Y+0.5).LineTo(cx, r.Y-6).LineTo(cx+6, r.Y+0.5).Close()
		} else {
			p.MoveTo(cx-6, r.Bottom()-0.5).LineTo(cx, r.Bottom()+6).LineTo(cx+6, r.Bottom()-0.5).Close()
		}
		l.Path(p, tk.PopupBg)
		if navEl(c) {
			q := paint.NewPath()
			if up {
				q.MoveTo(cx-6, r.Y+0.5).LineTo(cx, r.Y-6).LineTo(cx+6, r.Y+0.5)
			} else {
				q.MoveTo(cx-6, r.Bottom()-0.5).LineTo(cx, r.Bottom()+6).LineTo(cx+6, r.Bottom()-0.5)
			}
			l.StrokePath(q, paint.StrokeStyle{Width: 1}, tk.PopupBorderColor)
		}
	}
}

// Button builds the split-button form: a main button labelled label joined
// with a menu button (Ant Dropdown.Button, Element split-button). Check
// ButtonClicked for the main part and Clicked for the chosen item.
func (d *DropdownW) Button(label string) *DropdownW {
	c := d.c
	size := c.navSize(d.size, d.sizeSet)
	ic := d.ic
	if ic == nil {
		ic = icon.More
		if navEl(c) {
			ic = icon.Down
		}
	}
	if !d.placeSet {
		d.place = BottomEnd
	}
	style := func(x *ButtonW) {
		x.Size(size).Disabled(d.disabled)
		if d.primary {
			x.Primary()
		}
		if d.danger {
			x.Danger()
		}
	}
	Compact(c).Body(func() {
		main := Button(c, label)
		style(main)
		if main.Clicked() {
			d.btnClicked = true
		}
		d.Body(func() {
			m := Button(c, "").Icon(ic)
			style(m)
			m.n.label = "dropdown-more"
			m.n.setName(c.Locale().A11y.MoreActions)
			pad := c.controlHeight(size)
			m.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: pad, H: pad} }
		})
	})
	return d
}
