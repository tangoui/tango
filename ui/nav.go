package ui

import (
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// MenuItem is an entry of Menu / Dropdown.
type MenuItem struct {
	Key      string
	Label    string
	Icon     *icon.Icon
	Children []MenuItem
	Disabled bool
	Danger   bool
	Divider  bool // render a separator instead of an item
	// Group makes the entry a titled group of Children (Ant type="group",
	// Element el-menu-item-group): Label is a non-clickable title and the
	// children follow at the same level. Dropdowns and pop-out submenus
	// show it as a titled section (shadcn: a DropdownMenuLabel set apart
	// by separators); keyboard navigation skips the title.
	Group bool
	// Shortcut is the item's keyboard shortcut text ("Ctrl+C", "⌘K"),
	// shown in a dimmer column at the item's inline end of Dropdown panels,
	// Menu pop-out submenus and ContextMenuAt. It is display only (the app
	// handles the key itself); the label stays the accessible name and the
	// shortcut is exposed as the node's keyboard shortcut.
	Shortcut string
}

// menuFlatGroups returns items with groups replaced by their children
// (separated by dividers), for the root items of a horizontal menu.
func menuFlatGroups(items []MenuItem) []MenuItem {
	has := false
	for _, it := range items {
		if it.Group || len(it.Children) > 0 {
			has = true
			break
		}
	}
	if !has {
		return items
	}
	out := make([]MenuItem, 0, len(items))
	afterGroup := false
	for _, it := range items {
		if it.Group || afterGroup && !it.Divider {
			if len(out) > 0 && !out[len(out)-1].Divider {
				out = append(out, MenuItem{Divider: true})
			}
			afterGroup = false
		}
		if it.Group {
			out = append(out, menuFlatGroups(it.Children)...)
			afterGroup = true
			continue
		}
		if len(it.Children) > 0 {
			it.Children = menuFlatGroups(it.Children)
		}
		if it.Divider && len(out) > 0 && out[len(out)-1].Divider {
			continue
		}
		out = append(out, it)
	}
	return out
}

func containsKey(items []MenuItem, key string) bool {
	for _, it := range items {
		if it.Key == key || containsKey(it.Children, key) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tabs

// TabItem is a tab.
type TabItem struct {
	_ [0]func()

	Key, Label string
	Icon       *icon.Icon
	Disabled   bool
	NoClose    bool // no close button on this tab of a Closable bar
}

// TabsW is a tab bar: line or card style, editable cards (close and add
// buttons), top / bottom / left / right placement, centered tabs and a
// scrolling tab list with a "more" dropdown for the tabs that do not fit.
type TabsW struct {
	_ [0]func()

	c        *Context
	th       *theme.Theme // Tabs tokens (c.ThemeFor(theme.CompTabs))
	items    []TabItem
	active   *string
	card     bool
	size     ControlSize
	changed  bool
	extra    func()
	tabIDs   []ID
	panel    ID // tab panel built by Panel (0 if the app renders its own)
	closable bool
	onAdd    func()
	added    bool
	closed   string
	place    Placement
	centered bool
	noMargin bool
	barOpts  []layout.Option
	wrap     *Box // holds bar and panel for bottom / left / right placement
	loc      *Locale

	cardSet    bool // Card or type="card" given (shadcn: the outline list)
	borderCard bool
	extraLeft  func()
	height     float32

	gutter      float32 // TabBarGutter (0 with gutterSet false: the theme's)
	gutterSet   bool
	indSize     float32 // Indicator size (0: the tab's extent)
	indAlign    string  // Indicator align: "start" | "center" | "end"
	beforeLeave func(newKey, oldKey string) bool
	clicked     string // key of the tab clicked this frame
	stretch     bool   // Stretch: tabs share the bar width
	variant     string // shadcn TabsList variant: "" / "default" | "line"
	addIcon     *icon.Icon
	removeIcon  *icon.Icon
	moreIcon    *icon.Icon
	// legacy keeps the pre-batch-2 look and skips ConfigProvider.Defaults
	// and the unknown-key fallback (Dock's tab strip, dockTabs).
	legacy bool

	drag tabsDrag // Draggable / OnTearOff (nav_tabs_drag.go)
}

type tabsState struct {
	idx    int    // index of the active tab in the last frame (fallback when its key goes away)
	more   bool   // the tab list overflowed: the more button is shown
	reveal bool   // scroll the active tab into view once it is laid out
	last   string // active key of the last frame
	woke   int    // frames requested before the bar was first laid out
	tries  int    // laid-out reveal attempts that still missed the viewport
	seen   geom.Rect
}

// wake asks for one more frame while the bar has not been laid out yet
// (overflow and scrolling need its geometry); bounded so a bar that is
// never painted does not keep the loop busy.
func (st *tabsState) wake(c *Context) {
	if st.woke < 2 {
		st.woke++
		c.WakeAt(c.now)
	}
}

// Tabs adds a tab bar bound to the active key; render the panel yourself
// based on *active (or with Panel). Call Show after options.
//
// ConfigProvider.Defaults("Tabs", ...) keys: "type" ("line" | "card" |
// "editable-card" | "border-card"), "size" (ControlSize), "centered"
// (bool), "tabPlacement" (Placement or Ant v6 "top" | "bottom" | "start" |
// "end"; the deprecated keys "placement" and "tabPosition" take "top" |
// "bottom" | "left" | "right" too), "stretch" (bool), "tabBarGutter"
// (px), "variant" ("default" | "line", the shadcn TabsList variant).
//
// active may be nil (the Tabs keep the key themselves). When *active is
// not a key of items, the tab at the last active index (clamped; the
// first tab at first) becomes active, like Ant's Tabs when the active key
// goes away; Changed does not report that fallback.
//
// Under shadcn/ui every combination is drawn as the segmented TabsList
// (muted track, background pill with shadow-sm): vertical placements
// stack the triggers, Closable adds a small x to each trigger, OnAdd a
// ghost + button after the list, Card switches to an outline track and
// BorderCard puts the list and panel in a bordered card.
func Tabs(c *Context, active *string, items []TabItem) *TabsW {
	if active == nil {
		active = &State[tabsOwnKey](c, c.newID("tabs-key", ""), nil).key
	}
	t := newTabs(c, active, items)
	switch Prop(c, "Tabs", "type", "") {
	case "card":
		t.card, t.cardSet = true, true
	case "editable-card":
		t.Closable()
	case "border-card":
		t.BorderCard()
	}
	if v, ok := c.ConfigProp("Tabs", "size"); ok {
		if s, ok := v.(ControlSize); ok {
			t.size = s
		}
	}
	t.centered = Prop(c, "Tabs", "centered", false)
	v, ok := c.ConfigProp("Tabs", "tabPlacement")
	if !ok {
		v, ok = c.ConfigProp("Tabs", "placement")
	}
	if !ok {
		v, ok = c.ConfigProp("Tabs", "tabPosition")
	}
	if ok {
		switch p := v.(type) {
		case Placement:
			t.Placement(p)
		case string:
			t.TabPlacement(p)
		}
	}
	t.stretch = Prop(c, "Tabs", "stretch", false)
	t.variant = Prop(c, "Tabs", "variant", "")
	if v, ok := p2bNum(c, "Tabs", "tabBarGutter"); ok {
		t.TabBarGutter(float32(v))
	}
	return t
}

// newTabs builds a TabsW without reading ConfigProvider.Defaults.
func newTabs(c *Context, active *string, items []TabItem) *TabsW {
	return &TabsW{c: c, th: c.ThemeFor(theme.CompTabs), items: items, active: active, size: c.defaultSize(), place: Top, loc: c.Locale()}
}

// dockTabs builds the tab strip of a Dock group: no Defaults("Tabs", ...)
// (an app's tab defaults must not restyle the workbench), no fallback of
// an unknown active key (the layout owns it), and the card size and the
// shadcn active trigger the strip had before batch 2 (CardHeight and a
// 16px padding at every size).
func dockTabs(c *Context, active *string, items []TabItem) *TabsW {
	t := newTabs(c, active, items)
	t.legacy = true
	return t
}

// tabsOwnKey holds the active key of Tabs created with a nil pointer.
type tabsOwnKey struct{ key string }

// TabPlacement puts the bar by Ant v6 name: "top" (default), "bottom",
// "start" or "end" (logical sides: mirrored in RTL); "left" and "right"
// are accepted as their deprecated aliases. Same as Placement.
func (t *TabsW) TabPlacement(p string) *TabsW {
	return t.Placement(map[string]Placement{"bottom": Bottom, "start": Left, "left": Left, "end": Right, "right": Right}[p])
}

// TabBarGutter sets the space between two tabs (Ant tabBarGutter; the
// default comes from the theme: Ant 32 for line tabs, 2 for cards).
func (t *TabsW) TabBarGutter(px float32) *TabsW { t.gutter, t.gutterSet = max(0, px), true; return t }

// Indicator sets the ink bar of line tabs (Ant v6 indicator): size is its
// length in px (0: the tab's extent; Element: the label's), align places
// it along the tab: "start", "center" (default) or "end".
func (t *TabsW) Indicator(size float32, align string) *TabsW {
	t.indSize, t.indAlign = max(0, size), align
	return t
}

// BeforeLeave is asked before the active tab changes (Element
// before-leave): returning false keeps the current tab.
func (t *TabsW) BeforeLeave(fn func(newKey, oldKey string) bool) *TabsW { t.beforeLeave = fn; return t }

// TabClicked returns the key of the tab clicked this frame, also when it
// is already active ("" when none; Ant onTabClick, Element tab-click).
func (t *TabsW) TabClicked() string { return t.clicked }

// Stretch makes the tabs of a horizontal bar share its width (Element
// stretch); the bar no longer scrolls.
func (t *TabsW) Stretch() *TabsW { t.stretch = true; return t }

// Variant picks the list look in every theme: "default" or "line" (line
// tabs: a transparent list with an underline under the active tab).
// Where the default bar is a segmented track (TabsToken.SegmentedList:
// the shadcn TabsList) "line" switches to line tabs; where the default is
// line tabs already (Ant, Element) it is the same look. Card and
// BorderCard pick their own types.
func (t *TabsW) Variant(v string) *TabsW { t.variant = v; return t }

// AddIcon replaces the + of the add button (Ant addIcon).
func (t *TabsW) AddIcon(ic *icon.Icon) *TabsW { t.addIcon = ic; return t }

// RemoveIcon replaces the x of the close buttons (Ant removeIcon).
func (t *TabsW) RemoveIcon(ic *icon.Icon) *TabsW { t.removeIcon = ic; return t }

// MoreIcon replaces the icon of the more button (Ant more.icon).
func (t *TabsW) MoreIcon(ic *icon.Icon) *TabsW { t.moreIcon = ic; return t }

// activate makes key the active tab unless BeforeLeave refuses; it
// reports whether the tab is (now) active.
func (t *TabsW) activate(key string) bool {
	if *t.active == key {
		return true
	}
	if t.beforeLeave != nil && !t.beforeLeave(key, *t.active) {
		return false
	}
	*t.active = key
	t.changed = true
	t.c.invalid = true
	return true
}

// shadLine reports the line variant of a segmented list (theme token
// SegmentedList; other themes draw line tabs by default).
func (t *TabsW) shadLine() bool {
	return t.variant == "line" && t.th.Components.Tabs.SegmentedList
}

// fallback makes a tab active when *active is not a key of the items:
// the one at the last active index (clamped), skipping disabled tabs.
func (t *TabsW) fallback(st *tabsState) {
	n := len(t.items)
	if n == 0 || t.legacy {
		return
	}
	for i, it := range t.items {
		if it.Key == *t.active {
			st.idx = i
			return
		}
	}
	i := max(0, min(st.idx, n-1))
	for k := 0; k < n; k++ {
		if j := (i + k) % n; !t.items[j].Disabled {
			i = j
			break
		}
	}
	st.idx = i
	*t.active = t.items[i].Key
}

func (t *TabsW) Card() *TabsW              { t.card, t.cardSet = true, true; return t }
func (t *TabsW) Size(s ControlSize) *TabsW { t.size = s; return t }
func (t *TabsW) Extra(fn func()) *TabsW    { t.extra = fn; return t }
func (t *TabsW) Changed() bool             { return t.changed }

// Closable makes an editable card bar (Ant type="editable-card", Element
// closable): every tab without NoClose gets a close button; read Closed.
// Delete or Backspace on a focused tab closes it too.
func (t *TabsW) Closable() *TabsW { t.closable, t.card = true, true; return t }

// OnAdd shows the add button of an editable card bar; fn runs when it is
// clicked (Added reports it as well).
func (t *TabsW) OnAdd(fn func()) *TabsW {
	t.card = true
	if fn == nil {
		fn = func() {}
	}
	t.onAdd = fn
	return t
}

// Closed returns the key of the tab closed this frame ("" when none). The
// app removes it from the items (and picks a new active key).
func (t *TabsW) Closed() string { return t.closed }

// Added reports a click on the add button this frame.
func (t *TabsW) Added() bool { return t.added }

// Placement puts the bar at the Top (default), Bottom, Left or Right of
// the panel built by Panel. Left and Right are logical: in a right-to-left
// scope a Left bar sits on the right (the start side), as the layout
// mirrors the row of bar and panel. This is Ant v6 tabPlacement
// (TabPlacement takes its string names; Left = "start", Right = "end").
func (t *TabsW) Placement(p Placement) *TabsW {
	switch p {
	case Bottom, Left, Right:
		t.place = p
	default:
		t.place = Top
	}
	return t
}

// BorderCard puts the bar and the panel built by Panel in a bordered card
// whose header strip holds the tabs (Element type="border-card").
func (t *TabsW) BorderCard() *TabsW { t.borderCard, t.card = true, true; return t }

// ExtraLeft builds content at the start of the bar, before the tabs
// (Ant tabBarExtraContent.left); Extra is the right one.
func (t *TabsW) ExtraLeft(fn func()) *TabsW { t.extraLeft = fn; return t }

// Height fixes the height of a Left / Right Tabs (bar and panel); tabs
// that do not fit scroll and are listed in the more menu.
func (t *TabsW) Height(px float32) *TabsW { t.height = px; return t }

// Centered centers the tabs in the bar.
func (t *TabsW) Centered() *TabsW { t.centered = true; return t }

// NoMargin drops the gap between the bar and the content (for a bar that
// sits at the bottom edge of a header, as in PageContainer).
func (t *TabsW) NoMargin() *TabsW { t.noMargin = true; return t }

// TabBarStyle applies layout options to the bar (padding, margin ...;
// Ant tabBarStyle). They apply after the theme's defaults.
func (t *TabsW) TabBarStyle(opts ...layout.Option) *TabsW {
	t.barOpts = append(t.barOpts, opts...)
	return t
}

func (t *TabsW) vertical() bool { return t.place == Left || t.place == Right }

// physPlace is the physical side of the bar: Left and Right swap in a
// right-to-left scope (paint code draws edges and corners with it).
func (t *TabsW) physPlace() Placement {
	if t.c.IsRTL() {
		switch t.place {
		case Left:
			return Right
		case Right:
			return Left
		}
	}
	return t.place
}

// Panel builds the content of the active tab in a tab panel (exposed to
// screen readers as the tabpanel labelled by the active tab). Call it
// after Show; rendering the panel without it works too. With Bottom, Left
// or Right placement the panel is placed next to the bar.
func (t *TabsW) Panel(fn func()) *Box {
	c := t.c
	label := ""
	for _, it := range t.items {
		if it.Key == *t.active {
			label = it.Label
		}
	}
	var b *Box
	build := func() {
		b = Div(c)
		b.n.setRole(a11y.RoleTabPanel).setName(label)
		t.panel = b.n.id
		Key(c, *t.active, func() { b.Body(fn) })
	}
	if t.wrap == nil {
		build()
		return b
	}
	c.body(t.wrap.n, build)
	shad := t.th.Flavor == theme.StyleShadcn
	if t.borderCard {
		pad := float32(16)
		if navEl(c) {
			pad = 15
		}
		b.n.ln.Padding = geom.All(pad)
		if shad {
			b.n.ln.Padding.Top = 10
		}
	}
	if t.vertical() {
		b.n.ln.Grow, b.n.ln.MinW = 1, 0
		pad := float32(24)
		switch {
		case t.borderCard:
			pad = b.n.ln.Padding.Left
		case navEl(c):
			pad = 0
		case shad:
			pad = 16
		}
		if t.place == Left {
			b.n.ln.Padding.Left = pad
		} else {
			b.n.ln.Padding.Right = pad
		}
	}
	if t.place == Right || t.place == Bottom {
		// The panel comes first (visually and in the reading order).
		kids := t.wrap.n.kids
		if k := len(kids); k > 1 && kids[k-1] == b.n {
			copy(kids[1:], kids[:k-1])
			kids[0] = b.n
		}
	}
	return b
}

// keys moves between tabs with the arrow keys (selecting on focus, like
// the WAI-ARIA tabs pattern) and makes the active tab the only Tab stop.
func (t *TabsW) keys(nodes []*node, active int) {
	c := t.c
	rovingTabStop(nodes, active)
	ids := make([]ID, len(nodes))
	for i, n := range nodes {
		ids[i] = n.id
	}
	t.tabIDs = ids
	axis := rovingHorizontal
	if t.vertical() {
		axis = rovingVertical
	}
	// Delete / Backspace close the focused tab of an editable bar.
	if t.closable {
		for i, id := range ids {
			if id != c.focus || t.items[i].NoClose || t.items[i].Disabled {
				continue
			}
			for _, k := range c.keys {
				if k.Down && (k.Key == input.KeyDelete || k.Key == input.KeyBackspace) {
					t.closed = t.items[i].Key
					c.invalid = true
				}
			}
		}
	}
	to := rovingStep(c, ids, func(i int) bool { return !t.items[i].Disabled }, axis)
	if to < 0 {
		return
	}
	if t.activate(t.items[to].Key) {
		navFocus(c, ids[to])
	}
}

// tabSem marks a tab node; the panel relation is filled in once Panel ran.
func (t *TabsW) tabSem(n *node, it TabItem) {
	n.setRole(a11y.RoleTab).setState(a11y.Selectable).stateIf(it.Key == *t.active, a11y.Selected)
	if it.Key == *t.active && t.c.a11yOn {
		n.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(t.panel) }))
	}
}

// Show builds the tab bar.
func (t *TabsW) Show() *TabsW {
	c := t.c
	switch {
	case t.place == Bottom || t.borderCard && t.place == Top:
		t.wrap = c.box("tabs", layout.Column, nil)
	case t.vertical():
		t.wrap = c.box("tabs", layout.Row, nil)
	}
	if t.wrap == nil {
		t.bar()
		return t
	}
	if t.height > 0 {
		t.wrap.n.ln.Height = t.height
	}
	if t.borderCard {
		th := t.th
		bc := th.BorderSecondary
		rad := th.RadiusLG
		switch th.Flavor {
		case theme.Element:
			bc, rad = th.Border, 0
		case theme.StyleShadcn:
			bc = th.Border
		}
		t.wrap.Bg(th.BgElevated).Border(1, bc).Radius(rad).Clip()
		t.wrap.n.ln.Padding = geom.All(1) // children stay inside the border
	}
	c.body(t.wrap.n, t.bar)
	return t
}

// tabMetrics returns the font, label style and vertical padding of a tab.
func (t *TabsW) tabMetrics() (fs float32, st text.Style, padV float32) {
	c, th := t.c, t.th
	tk := &th.Components.Tabs
	fs, padV = tk.TitleFontSize, tk.HorizontalItemPadding
	switch t.size {
	case Large:
		fs, padV = tk.TitleFontSizeLG, tk.HorizontalItemPaddingLG
	case Small:
		fs, padV = tk.TitleFontSizeSM, tk.HorizontalItemPaddingSM
	}
	st = c.style(fs, text.Regular)
	if tk.TitleFontWeight > 0 {
		st.Weight = themeWeight(tk.TitleFontWeight)
	}
	if th.Flavor == theme.StyleShadcn {
		st = c.style(fs, text.Medium)
	}
	return
}

// cardH is the height of a card tab (and of the add button) at the size.
func (t *TabsW) cardH() float32 {
	tk := &t.th.Components.Tabs
	if t.legacy {
		return tk.CardHeight
	}
	switch t.size {
	case Small:
		return tk.CardHeightSM
	case Large:
		return tk.CardHeightLG
	}
	return tk.CardHeight
}

// cardPadH is the inline padding of a card tab at the size.
func (t *TabsW) cardPadH() float32 {
	tk := &t.th.Components.Tabs
	if t.legacy {
		return 16
	}
	if t.size == Small {
		return tk.CardPaddingHSM
	}
	return tk.CardPaddingH
}

// shadHeight is the height of a shadcn tab list (h-9; triggers are 6px
// shorter inside the 3px track padding).
func (t *TabsW) shadHeight() float32 {
	th := t.th
	switch t.size {
	case Small:
		return th.ControlHeightSM
	case Large:
		return th.ControlHeightLG
	}
	return th.ControlHeight
}

// shadTrack styles the shadcn TabsList around the scrolling tabs: a muted
// rounded track, or for Card a bordered (outline) track.
func (t *TabsW) shadTrack(b *Box) {
	th := t.th
	b.Radius(th.RadiusLG)
	if t.shadLine() {
		b.Radius(0) // data-[variant=line]:rounded-none bg-transparent
		b.n.ln.NoStretch = true
		return
	}
	if t.cardSet {
		b.Border(1, th.Border)
	} else {
		b.Bg(th.SegmentedBg)
	}
	b.n.ln.NoStretch = true
}

// bar builds the tab bar (inside t.wrap for non-top placements).
func (t *TabsW) bar() {
	c, th := t.c, t.th
	tk := &th.Components.Tabs
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	vert := t.vertical()
	dir := layout.Row
	if vert {
		dir = layout.Column
	}
	outer := c.box("tabs-bar", dir, nil)
	st := State[tabsState](c, outer.n.id, func() tabsState { return tabsState{reveal: true} })
	t.fallback(st)
	if t.active != nil && st.last != *t.active {
		st.last, st.reveal = *t.active, true
	}
	var nodes []*node
	activeIdx := 0
	var list *Box
	var sb *ScrollBox
	tabs := func() {
		list = c.box("tabs-list", dir, nil)
		c.body(list.n, func() {
			for i, it := range t.items {
				if it.Key == *t.active {
					activeIdx = i
				}
				nodes = append(nodes, t.tab(i, it))
			}
		})
		switch {
		case vert:
			list.n.ln.Align = layout.AlignStretch
		case shad:
			list.n.ln.Align = layout.AlignCenter
		case !t.card:
			list.n.ln.Align = layout.AlignEnd
		}
	}
	// scroller builds the scrolling viewport of the tabs and, while they
	// overflow it, the more button listing the hidden ones.
	scroller := func() {
		wrapID := c.cur.id
		if vert && t.height == 0 && len(t.barOpts) == 0 {
			// A vertical bar without a height limit grows with its tabs:
			// no viewport (it would clip the card edges over the baseline).
			tabs()
			return
		}
		if t.stretch && !vert {
			// Stretched tabs share the bar: nothing to scroll.
			tabs()
			list.n.ln.Grow, list.n.ln.MinW = 1, 0
			return
		}
		if vert {
			sb = Scroll(c, tabs, layout.MinH(0), layout.Shrink(1))
		} else {
			sb = ScrollH(c, tabs, layout.MinW(0), layout.Shrink(1))
			if _, dy, ok := c.WheelFor(wrapID); ok && dy != 0 {
				ox, _ := sb.Offset()
				sb.ScrollTo(max(0, ox+float64(dy)), 0)
			}
		}
		sb.n.over = nil // no scrollbar: wheel, keyboard and the more menu scroll the list
		t.overflow(st, sb, nodes)
	}
	extraLeft := func() {
		if t.extraLeft == nil {
			return
		}
		ex := Row(c, t.extraLeft, layout.Middle(), layout.Gap(8))
		ex.n.ln.AlignSelf = layout.AlignCenter
		if vert {
			ex.n.ln.Margin.Bottom = 16
		} else {
			ex.n.ln.Margin.Right = 16
		}
	}
	c.body(outer.n, func() {
		if vert {
			extraLeft()
			if shad {
				tr := Column(c, scroller, layout.Pad(3), layout.MinH(0), layout.Shrink(1))
				t.shadTrack(tr)
				t.shadUnderline(tr, func() *node { return tabsNodeAt(nodes, activeIdx) })
			} else {
				scroller()
			}
			if t.onAdd != nil {
				t.addButton()
			}
			if t.extra != nil {
				Column(c, t.extra, layout.Margin(geom.Insets{Top: 16}))
			}
			return
		}
		extraLeft()
		wrap := Row(c, func() {
			if shad {
				tr := Row(c, scroller, layout.Pad(3), layout.MinW(0), layout.Shrink(1), layout.Middle())
				if t.stretch {
					tr.n.ln.Grow = 1
				}
				t.shadTrack(tr)
				t.shadUnderline(tr, func() *node { return tabsNodeAt(nodes, activeIdx) })
			} else {
				scroller()
			}
			if t.onAdd != nil {
				t.addButton()
			}
		}, layout.Grow(1), layout.MinW(0))
		if t.centered {
			wrap.n.ln.Justify = layout.JustifyCenter
		}
		switch {
		case shad:
			wrap.n.ln.Align = layout.AlignCenter
		case !t.card:
			wrap.n.ln.Align = layout.AlignEnd
		}
		wrap.Interact(input.CursorDefault)
		if t.extra != nil {
			if t.centered {
				Gap(c, 16)
			}
			ex := Row(c, t.extra, layout.Middle(), layout.Gap(8))
			ex.n.ln.AlignSelf = layout.AlignCenter
		}
	})
	outer.n.ln.Align = layout.AlignCenter
	if !t.card && !vert && !shad {
		outer.n.ln.Align = layout.AlignEnd
	}
	if vert {
		outer.n.ln.Align = layout.AlignStretch
	}
	list.n.setRole(a11y.RoleTabList)
	if vert {
		list.n.setState(a11y.Vertical)
	} else {
		list.n.setState(a11y.Horizontal)
	}
	t.dragKeys(nodes)
	t.keys(nodes, activeIdx)
	st.idx = activeIdx
	if sb != nil && st.reveal && activeIdx < len(nodes) {
		if c.ScrollIntoView(sb.n.id, nodes[activeIdx].id) {
			// Done once the last layout shows the tab inside the viewport;
			// the viewport may still change (the more button appearing), so
			// check again on the next frames, a bounded number of times.
			vr, ir := c.rects[sb.n.id], c.rects[nodes[activeIdx].id]
			in := ir.X >= vr.X-0.5 && ir.Right() <= vr.Right()+0.5
			if vert {
				in = ir.Y >= vr.Y-0.5 && ir.Bottom() <= vr.Bottom()+0.5
			}
			if ir != st.seen { // a new layout since the last attempt
				st.seen = ir
				st.tries++
			}
			if in || st.tries > 3 {
				st.reveal, st.tries = false, 0
			} else {
				c.WakeAt(c.now)
			}
		} else {
			st.wake(c) // laid out next frame
		}
	}
	place := t.physPlace()
	if t.borderCard && !shad {
		// Header strip of a border card: filled, split from the panel by a
		// line the active tab covers.
		outer.Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(r, geom.Radii{}, tk.HeaderBg)
			switch place {
			case Bottom:
				navHLine(l, r.X, r.Y, r.W, th.Split)
			case Left:
				navVLine(l, r.Right()-1, r.Y, r.H, th.Split)
			case Right:
				navVLine(l, r.X, r.Y, r.H, th.Split)
			default:
				navHLine(l, r.X, r.Bottom()-1, r.W, th.Split)
			}
		})
	}
	if !shad && !t.borderCard {
		// Baseline (the split line across the bar) and the ink bar.
		paintBaseline := outer.PaintOver
		if tk.LineTrimOuter && !t.card {
			paintBaseline = outer.Paint
		}
		paintBaseline(func(l *paint.List, r geom.Rect) {
			width := max(1, tk.LineBorderWidth)
			switch place {
			case Bottom:
				l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: width}, geom.Radii{}, th.Split)
			case Left:
				l.Fill(geom.Rect{X: r.Right() - width, Y: r.Y, W: width, H: r.H}, geom.Radii{}, th.Split)
			case Right:
				l.Fill(geom.Rect{X: r.X, Y: r.Y, W: width, H: r.H}, geom.Radii{}, th.Split)
			default:
				l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - width, W: r.W, H: width}, geom.Radii{}, th.Split)
			}
		})
	}
	id := list.n.id
	card := t.card
	if !shad {
		list.PaintOver(func(l *paint.List, r geom.Rect) {
			if card || len(nodes) == 0 {
				return
			}
			an := nodes[activeIdx]
			if vert {
				ay := c.Anim(id, "inky", an.abs.Y-r.Y, th.DurationSlow)
				ah := c.Anim(id, "inkh", an.abs.H, th.DurationSlow)
				ay, ah = t.indicator(ay, ah)
				x := r.Right() - 2
				if place == Right {
					x = r.X
				}
				l.Fill(geom.Rect{X: x, Y: r.Y + ay, W: 2, H: ah}, geom.Radii{}, tk.InkBarColor)
				return
			}
			// The ink bar animates its logical offset from the start edge,
			// so a bar that changes width in RTL does not slide.
			s := c.inline(r)
			off := s.Off(an.abs.X)
			if s.rtl {
				off = s.Off(an.abs.Right())
			}
			ax := c.Anim(id, "inkx", off, th.DurationSlow)
			aw := c.Anim(id, "inkw", an.abs.W, th.DurationSlow)
			if el && !tk.LineTrimOuter {
				ax += 20
				aw -= 40
			}
			ax, aw = t.indicator(ax, aw)
			y := r.Bottom() - 2
			if place == Bottom {
				y = r.Y
			}
			l.Fill(geom.Rect{X: s.X(ax, aw), Y: y, W: aw, H: 2}, geom.Radii{}, tk.InkBarColor)
		})
	}
	// Gap between bar and content.
	gap := float32(16)
	switch {
	case el:
		gap = 15
	case shad:
		gap = 8 // gap-2
	}
	switch t.place {
	case Bottom:
		outer.n.ln.Margin.Top = gap
	case Left:
		if el {
			outer.n.ln.Margin.Right = 10
		}
	case Right:
		if el {
			outer.n.ln.Margin.Left = 10
		}
	default:
		outer.n.ln.Margin.Bottom = gap
	}
	if t.borderCard {
		outer.n.ln.Margin = geom.Insets{}
		if shad {
			outer.n.ln.Padding = geom.All(6)
		}
	}
	if t.noMargin {
		outer.n.ln.Margin = geom.Insets{}
	}
	flexbox.Apply(&outer.n.ln.Style, t.barOpts...)
	t.dragTabs(outer, nodes)
}

// indicator applies Indicator to an ink bar at off with length n along
// the bar (offsets are logical: start is the inline start).
func (t *TabsW) indicator(off, n float32) (float32, float32) {
	if t.indSize <= 0 || t.indSize >= n {
		return off, n
	}
	switch t.indAlign {
	case "start":
	case "end":
		off += n - t.indSize
	default:
		off += (n - t.indSize) / 2
	}
	return off, t.indSize
}

// tabsNodeAt returns nodes[i] (nil when out of range).
func tabsNodeAt(nodes []*node, i int) *node {
	if i < 0 || i >= len(nodes) {
		return nil
	}
	return nodes[i]
}

// shadUnderline draws the line variant's indicator of the shadcn list:
// the active trigger's after:h-0.5 bg-foreground, 5px under the trigger
// (on the track's bottom edge), or 4px past its end in a vertical list.
func (t *TabsW) shadUnderline(tr *Box, active func() *node) {
	if !t.shadLine() {
		return
	}
	c, th := t.c, t.th
	vert := t.vertical()
	place := t.physPlace()
	id := tr.n.id
	tr.PaintOver(func(l *paint.List, r geom.Rect) {
		an := active()
		if an == nil {
			return
		}
		col := th.Components.Tabs.InkBarColor
		if vert {
			ay := c.Anim(id, "inky", an.abs.Y-r.Y, th.DurationSlow)
			ah := c.Anim(id, "inkh", an.abs.H, th.DurationSlow)
			ay, ah = t.indicator(ay, ah)
			x := r.Right() - 1
			if place == Right {
				x = r.X - 1
			}
			l.Fill(geom.Rect{X: x, Y: r.Y + ay, W: 2, H: ah}, geom.Radii{}, col)
			return
		}
		s := c.inline(r)
		off := s.Off(an.abs.X)
		if s.rtl {
			off = s.Off(an.abs.Right())
		}
		ax := c.Anim(id, "inkx", off, th.DurationSlow)
		aw := c.Anim(id, "inkw", an.abs.W, th.DurationSlow)
		ax, aw = t.indicator(ax, aw)
		l.Fill(geom.Rect{X: s.X(ax, aw), Y: r.Bottom() - 1, W: aw, H: 2}, geom.Radii{}, col)
	})
}

// overflow shows the more button (a dropdown of the tabs scrolled out of
// view) while the tab list is longer than its viewport (wider for a
// horizontal bar, taller for a vertical one).
func (t *TabsW) overflow(st *tabsState, sb *ScrollBox, nodes []*node) {
	c := t.c
	s := sb.s
	vert := t.vertical()
	over := s.view.W > 0 && s.content.X > float64(s.view.W)+0.5
	if vert {
		over = s.view.H > 0 && s.content.Y > float64(s.view.H)+0.5
	}
	if over != st.more {
		st.more = over
		st.reveal, st.tries = true, 0 // the more button changes the viewport: reveal again once laid out
		c.invalid = true
	}
	if _, ok := c.rects[sb.n.id]; !ok {
		st.wake(c) // measure the list first
	}
	if !st.more {
		return
	}
	vr := c.rects[sb.n.id]
	hidden := func() []MenuItem {
		var out []MenuItem
		for i, n := range nodes {
			r, ok := c.rects[n.id]
			off := !ok || r.X < vr.X-0.5 || r.Right() > vr.Right()+0.5
			if vert {
				off = !ok || r.Y < vr.Y-0.5 || r.Bottom() > vr.Bottom()+0.5
			}
			if off {
				it := t.items[i]
				out = append(out, MenuItem{Key: it.Key, Label: it.Label, Icon: it.Icon, Disabled: it.Disabled})
			}
		}
		return out
	}
	var dd *DropdownW
	box := Row(c, func() { dd = t.moreMenu(hidden) })
	box.n.ln.AlignSelf = layout.AlignCenter
	if k := dd.Clicked(); k != "" && k != *t.active && t.activate(k) {
		st.reveal = true
	}
}

// tabsMoreEager builds the more menu's items on every frame like before
// the lazy list. Test-only (tests compare both); don't mutate in parallel
// tests.
var tabsMoreEager = false

// moreMenu builds the more button and its dropdown of hidden tabs. The
// hidden tabs are listed only while the menu shows: a closed Dropdown
// never reads its items (O(tabs) MenuItems per frame saved). They are
// read from the same rects (last frame's layout) either way.
func (t *TabsW) moreMenu(hidden func() []MenuItem) *DropdownW {
	c := t.c
	dd := Dropdown(c, nil).Selected(*t.active)
	if tabsMoreEager {
		dd.items = hidden()
	} else {
		dd.lazy = hidden
	}
	if t.vertical() {
		// Towards the panel: a Left (start) bar opens its menu at the end.
		dd.Placement(map[bool]Placement{true: InlineEndTop, false: InlineStartTop}[t.place == Left])
	}
	return dd.Body(func() {
		h := float32(32)
		if navEl(c) {
			h = 40
		}
		shad := t.th.Flavor == theme.StyleShadcn
		if shad {
			h = t.shadHeight() - 6
		}
		mi := icon.More
		if t.moreIcon != nil {
			mi = t.moreIcon
		}
		ib := IconButton(c, mi, t.loc.Tabs.More).Size(h, 16).NoHoverBg()
		switch {
		case t.vertical() && shad:
			ib.n.ln.Margin = geom.Insets{Top: 2}
		case t.vertical():
			ib.n.ln.Margin = geom.Insets{Top: 4, Bottom: 4}
		case shad:
			ib.n.ln.Margin = geom.Insets{Left: 2}
		default:
			ib.n.ln.Margin = geom.Insets{Left: 8, Right: 8}
			if t.card {
				ib.n.ln.Margin.Left = 2
			}
		}
	})
}

// addButton adds the "+" of an editable card bar.
func (t *TabsW) addButton() {
	c, th := t.c, t.th
	tk := &th.Components.Tabs
	el := navEl(c)
	shad := th.Flavor == theme.StyleShadcn
	n := c.add("tab-add", t.loc.Tabs.Add)
	n.setRole(a11y.RoleButton)
	n.ln.NoStretch = true
	switch {
	case shad:
		// A ghost icon button (size-9) after the list.
		h := t.shadHeight()
		n.ln.Width, n.ln.Height = h, h
		n.ln.AlignSelf = layout.AlignCenter
		if t.vertical() {
			n.ln.Margin = geom.Insets{Top: 4}
		} else {
			n.ln.Margin.Left = 4
		}
	case el:
		n.ln.Width, n.ln.Height = 20, 20
		n.ln.Margin = geom.Insets{Left: 10, Right: 4}
		n.ln.AlignSelf = layout.AlignCenter
	default:
		h := t.cardH()
		n.ln.Width, n.ln.Height = h, h
		n.ln.Margin.Left = 2
		if t.vertical() {
			n.ln.Margin = geom.Insets{Top: 2}
		}
	}
	ia := c.interact(n, true, input.CursorPointer)
	plus := icon.Plus
	if t.addIcon != nil {
		plus = t.addIcon
	}
	if ia.Clicked {
		t.added = true
		c.invalid = true
		if t.onAdd != nil {
			t.onAdd()
		}
	}
	id := n.id
	vert := t.vertical()
	place := t.physPlace()
	n.paint = func(l *paint.List, r geom.Rect) {
		hov := c.Anim(id, "hover", b2f(ia.Hovered), th.DurationMid)
		col := theme.Lerp(tk.ItemColor, tk.ItemHoverColor, hov)
		switch {
		case shad:
			if hov > 0 {
				l.Fill(r, geom.Rad(th.Radius), theme.Fade(th.ItemHoverBg, float64(hov)))
			}
			col = theme.Lerp(th.TextTertiary, th.Text, hov)
			drawIcon(l, plus, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, col)
			if ia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.Radius))
			}
			return
		case el:
			col = theme.Lerp(th.TextSecondary, th.Primary.Base, hov)
			rad := geom.Rad(3)
			l.Stroke(r, rad, 1, theme.Lerp(th.Border, th.Primary.Base, hov))
			drawIcon(l, plus, r.X+(r.W-12)/2, r.Y+(r.H-12)/2, 12, col)
		default:
			rad := geom.Radii{TL: th.RadiusLG, TR: th.RadiusLG}
			if vert {
				rad = geom.Radii{TL: th.RadiusLG, BL: th.RadiusLG}
				if place == Right {
					rad = geom.Radii{TR: th.RadiusLG, BR: th.RadiusLG}
				}
			}
			l.Fill(r, rad, tk.CardBg)
			l.Stroke(r, rad, 1, th.BorderSecondary)
			drawIcon(l, plus, r.X+(r.W-14)/2, r.Y+(r.H-14)/2, 14, col)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
	}
}

// tab builds tab i: a leaf, or a row holding the label and the close
// button on a Closable bar.
func (t *TabsW) tab(i int, it TabItem) *node {
	c, th := t.c, t.th
	tk := &th.Components.Tabs
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	bc := t.borderCard && !shad
	vert := t.vertical()
	fs, st, padV := t.tabMetrics()
	icGap := float32(8)
	if shad {
		icGap = 6
	}
	labelW := func() float32 {
		w := text.Width(it.Label, st)
		if it.Icon != nil {
			w += fs + icGap
		}
		return w
	}
	n := c.add("tab", it.Label)
	t.tabSem(n, it)
	ia := c.interact(n, true, input.CursorPointer)
	if it.Disabled {
		n.disabled = true
	}
	if ia.Clicked && !it.Disabled {
		t.clicked = it.Key
		t.activate(it.Key)
	}
	active := it.Key == *t.active
	closeBtn := t.closable && !it.NoClose
	shadH := t.shadHeight() - 6
	// Size of the tab box (without a close button).
	size := func() geom.Size {
		w := labelW()
		switch {
		case shad:
			return geom.Size{W: w + 16, H: shadH}
		case bc:
			return geom.Size{W: w + 40, H: tk.CardHeight}
		case vert && t.card:
			return geom.Size{W: w + 2*t.cardPadH(), H: t.cardH()}
		case vert && el:
			return geom.Size{W: w + 40, H: 40}
		case vert:
			return geom.Size{W: w + 48, H: st.LineHeight + 16}
		case t.card:
			return geom.Size{W: w + 2*t.cardPadH(), H: t.cardH()}
		case el && tk.LineTrimOuter:
			return geom.Size{W: w, H: 40}
		case el:
			return geom.Size{W: w + 40, H: 40}
		}
		return geom.Size{W: w, H: st.LineHeight + 2*padV}
	}
	if t.stretch && !vert {
		n.ln.Grow = 1
	}
	switch {
	case t.gutterSet && i > 0:
		// antd tabBarGutter applies to every type (also the segmented
		// list and the border card).
		if vert {
			n.ln.Margin.Top = t.gutter
		} else {
			n.ln.Margin.Left = t.gutter
		}
	case shad && t.shadLine() && i > 0:
		if vert {
			n.ln.Margin.Top = 4 // gap-1
		} else {
			n.ln.Margin.Left = 4
		}
	case shad, bc:
	case vert && t.card && i > 0:
		n.ln.Margin.Top = 2
	case vert && !el && i > 0:
		n.ln.Margin.Top = 16
	case !vert && !t.card && (!el || tk.LineTrimOuter) && i > 0:
		n.ln.Margin.Left = tk.HorizontalItemGutter
	case !vert && t.card && i > 0:
		n.ln.Margin.Left = 2
	}
	// Label alignment: centered; Element's vertical tabs align toward the
	// panel, shadcn's vertical triggers start at the left.
	align := 0
	inset := float32(20)
	if vert && el {
		align = map[bool]int{true: 1, false: -1}[t.place == Left]
	}
	if vert && shad {
		align, inset = -1, 8
	}
	labelColor := func() theme.Color {
		hov := c.Anim(n.id, "hover", b2f(ia.Hovered), th.DurationMid)
		var col theme.Color
		switch {
		case shad:
			// The trigger brightens on hover and when active.
			if a := c.Anim(n.id, "on", b2f(active), th.DurationFast); a >= hov {
				col = theme.Lerp(tk.ItemColor, tk.ItemSelectedColor, a)
			} else {
				col = theme.Lerp(tk.ItemColor, tk.ItemHoverColor, hov)
			}
		case bc && el:
			col = theme.Lerp(th.TextTertiary, tk.ItemHoverColor, hov)
		default:
			col = theme.Lerp(tk.ItemColor, tk.ItemHoverColor, hov)
		}
		if active && !shad {
			col = tk.ItemSelectedColor
		}
		if it.Disabled {
			col = th.TextDisabled
		}
		return col
	}
	// drawLabel paints the icon and label; align -1 / 1 puts them at the
	// start / end (logical, mirrored in RTL), 0 centers them.
	drawLabel := func(l *paint.List, r geom.Rect, col theme.Color, align int) {
		lay := text.Shape(it.Label, st, c.textOpts(text.Options{NoWrap: true}))
		w := lay.Width
		if it.Icon != nil {
			w += fs + icGap
		}
		s := c.inline(r)
		x := (r.W - w) / 2
		switch align {
		case 1:
			x = r.W - inset - w
		case -1:
			x = inset
		}
		if it.Icon != nil {
			c.drawIconAt(l, it.Icon, s.X(x, fs), r.Y+(r.H-fs)/2, fs, col)
			x += fs + icGap
		}
		l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
	place := t.physPlace()
	card := t.card
	cardSet := t.cardSet
	shadLine := t.shadLine()
	legacy := t.legacy
	// drawShad paints a shadcn trigger: the active one is a background
	// pill with shadow-sm (Card: the outline track's muted pill).
	drawShad := func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(th.Radius)
		a := c.Anim(n.id, "on", b2f(active), th.DurationFast)
		if a > 0 && !shadLine {
			l.Opacity(a)
			if cardSet {
				l.Fill(r, rad, th.SegmentedBg)
			} else {
				l.Shadows(r, rad, th.ShadowTertiary)
				bg, border := tk.ItemSelectedBg, tk.ItemSelectedBorder
				if legacy {
					bg, border = th.SegmentedSelectedBg, theme.Color{}
				}
				l.Fill(r, rad, bg)
				if border.A > 0 {
					l.Stroke(r, rad, 1, border)
				}
			}
			l.Pop()
		}
	}
	// drawBorderCard paints a tab of a border card: the active one takes
	// the panel color and opens into it between two border lines.
	drawBorderCard := func(l *paint.List, r geom.Rect) {
		if !active {
			return
		}
		line := th.Border
		if !el {
			line = th.BorderSecondary
		}
		switch place {
		case Left, Right:
			l.Fill(r, geom.Radii{}, th.BgElevated)
			navHLine(l, r.X, r.Y, r.W, line)
			navHLine(l, r.X, r.Bottom()-1, r.W, line)
		default:
			l.Fill(r, geom.Radii{}, th.BgElevated)
			s := c.inline(r)
			if i > 0 {
				navVLine(l, s.X(0, 1), r.Y, r.H, line)
			}
			navVLine(l, s.X(r.W-1, 1), r.Y, r.H, line)
		}
	}
	drawCard := func(l *paint.List, r geom.Rect) {
		rad := geom.Radii{TL: th.RadiusLG, TR: th.RadiusLG}
		ext := geom.Insets{Bottom: -1}
		switch place {
		case Bottom:
			rad, ext = geom.Radii{BL: th.RadiusLG, BR: th.RadiusLG}, geom.Insets{Top: -1}
		case Left:
			rad, ext = geom.Radii{TL: th.RadiusLG, BL: th.RadiusLG}, geom.Insets{Right: -1}
		case Right:
			rad, ext = geom.Radii{TR: th.RadiusLG, BR: th.RadiusLG}, geom.Insets{Left: -1}
		}
		bg := tk.CardBg
		if active {
			bg = th.BgContainer
		}
		l.Fill(r, rad, bg)
		l.Stroke(r.Inset(ext), rad, 1, th.BorderSecondary)
		if active {
			// Open the edge toward the panel.
			switch place {
			case Bottom:
				l.Fill(geom.Rect{X: r.X + 1, Y: r.Y, W: r.W - 2, H: 1}, geom.Radii{}, th.BgContainer)
			case Left:
				l.Fill(geom.Rect{X: r.Right() - 1, Y: r.Y + 1, W: 1, H: r.H - 2}, geom.Radii{}, th.BgContainer)
			case Right:
				l.Fill(geom.Rect{X: r.X, Y: r.Y + 1, W: 1, H: r.H - 2}, geom.Radii{}, th.BgContainer)
			default:
				l.Fill(geom.Rect{X: r.X + 1, Y: r.Bottom() - 1, W: r.W - 2, H: 1}, geom.Radii{}, th.BgContainer)
			}
		}
	}
	drawBg := func(l *paint.List, r geom.Rect) {
		switch {
		case shad:
			drawShad(l, r)
		case bc:
			drawBorderCard(l, r)
		case card:
			drawCard(l, r)
		}
	}
	drawFocus := func(l *paint.List, r geom.Rect) {
		if !ia.FocusVisible {
			return
		}
		if shad {
			drawFocusRing(l, th, r, geom.Rad(th.Radius))
			return
		}
		l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.FocusRing)
	}
	if !closeBtn {
		n.ln.Measure = func(_, _ float32) geom.Size { return size() }
		n.paint = func(l *paint.List, r geom.Rect) {
			drawBg(l, r)
			drawLabel(l, r, labelColor(), align)
			drawFocus(l, r)
		}
		return n
	}
	// Editable card: [label][close].
	sz := size()
	padH := t.cardPadH()
	gap := float32(8)
	switch {
	case shad:
		padH, gap = 8, 4
	case el || bc:
		padH, gap = 20, 6
	}
	n.ln.Dir = layout.Row
	n.ln.Align = layout.AlignCenter
	n.ln.Height = sz.H
	n.ln.Padding = geom.Insets{Left: padH, Right: padH - 2}
	if shad {
		n.ln.Padding.Right = 4
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		drawBg(l, r)
		drawFocus(l, r)
	}
	c.body(n, func() {
		lb := c.add("tab-label", "")
		lb.aHidden = true
		lb.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: labelW(), H: st.LineHeight} }
		lb.ln.Grow = 1
		lb.paint = func(l *paint.List, r geom.Rect) { drawLabel(l, r, labelColor(), 0) }
		x := c.add("tab-close", t.loc.RemoveItem(it.Label))
		x.setRole(a11y.RoleButton)
		x.aKbd = true // Delete on the tab does the same
		x.ln.Width, x.ln.Height = 16, 16
		x.ln.Margin.Left = gap
		xia := c.interact(x, false, input.CursorPointer)
		if it.Disabled {
			x.disabled = true
		}
		if xia.Clicked && !it.Disabled {
			t.closed = it.Key
			c.invalid = true
		}
		xid := x.id
		xic := icon.Close
		if t.removeIcon != nil {
			xic = t.removeIcon
		}
		x.paint = func(l *paint.List, r geom.Rect) {
			hov := c.Anim(xid, "hover", b2f(xia.Hovered), th.DurationFast)
			if shad {
				// A small x (size-3.5) that darkens on a rounded hover chip.
				if hov > 0 {
					l.Fill(r, geom.Rad(th.RadiusXS+1), theme.Fade(th.BgTextActive, float64(hov)))
				}
				col := theme.Lerp(th.TextTertiary, th.Text, hov)
				if it.Disabled {
					col = th.TextDisabled
				}
				drawIcon(l, xic, r.X+2, r.Y+2, 12, col)
				return
			}
			if el {
				// Element: shown on the active or hovered tab, a grey disc on hover.
				if !active && !ia.Hovered && !xia.Hovered {
					return
				}
				if hov > 0 {
					l.Path(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, 7), theme.Fade(th.TextTertiary, float64(hov)))
				}
				col := theme.Lerp(labelColor(), th.TextOnSolid, hov)
				drawIcon(l, xic, r.X+3, r.Y+3, 10, col)
				return
			}
			col := theme.Lerp(th.TextTertiary, th.TextHeading, hov)
			if it.Disabled {
				col = th.TextDisabled
			}
			drawIcon(l, xic, r.X+2, r.Y+2, 12, col)
		}
	})
	return n
}

// ---------------------------------------------------------------------------
// Breadcrumb

// BreadcrumbItem is a structured breadcrumb entry.
type BreadcrumbItem struct {
	Label   string
	Icon    *icon.Icon // shown before the label (alone when Label is "")
	Href    string     // reported by ClickedHref; the app navigates
	OnClick func()     // runs when the item is clicked
	Menu    []MenuItem // dropdown menu on the item (read MenuClicked)
	// Separator replaces the separator after this item (antd items[]
	// {type: "separator", separator}); "" keeps the breadcrumb's.
	Separator string
}

// BreadcrumbW is a breadcrumb with structured items: icons, dropdown
// menus, click handlers and a custom separator.
type BreadcrumbW struct {
	_ [0]func()

	c       *Context
	items   []BreadcrumbItem
	sep     string
	sepIcon *icon.Icon
	ddIcon  *icon.Icon // DropdownIcon
	clicked int
	menuKey string
	th      *theme.Theme           // c.ThemeFor(theme.CompBreadcrumb), set by Show
	tk      *theme.BreadcrumbToken // &th.Components.Breadcrumb
}

// Breadcrumb shows a path of plain labels; Clicked on the result returns
// the index of the item clicked this frame, or -1. BreadcrumbItems builds
// the structured form.
func Breadcrumb(c *Context, items ...string) *BreadcrumbW {
	its := make([]BreadcrumbItem, len(items))
	for i, s := range items {
		its[i].Label = s
	}
	return BreadcrumbItems(c, its).Show()
}

// BreadcrumbItems starts a breadcrumb of structured items; chain options,
// then Show.
//
// ConfigProvider.Defaults("Breadcrumb", ...) keys: "separator" (string).
func BreadcrumbItems(c *Context, items []BreadcrumbItem) *BreadcrumbW {
	return &BreadcrumbW{c: c, items: items, clicked: -1, sep: Prop(c, "Breadcrumb", "separator", "")}
}

// Separator replaces the "/" between items (Ant separator, Element
// separator).
func (b *BreadcrumbW) Separator(s string) *BreadcrumbW { b.sep = s; return b }

// SeparatorIcon uses an icon between items (Element separator-icon).
func (b *BreadcrumbW) SeparatorIcon(ic *icon.Icon) *BreadcrumbW { b.sepIcon = ic; return b }

// DropdownIcon replaces the down arrow of items with a Menu (antd
// dropdownIcon).
func (b *BreadcrumbW) DropdownIcon(ic *icon.Icon) *BreadcrumbW { b.ddIcon = ic; return b }

// Clicked returns the index of the item clicked this frame, or -1.
func (b *BreadcrumbW) Clicked() int { return b.clicked }

// ClickedHref returns the Href of the item clicked this frame ("" when
// none was clicked or it has no Href).
func (b *BreadcrumbW) ClickedHref() string {
	if b.clicked < 0 {
		return ""
	}
	return b.items[b.clicked].Href
}

// MenuClicked returns the key chosen in an item's dropdown menu this frame.
func (b *BreadcrumbW) MenuClicked() string { return b.menuKey }

// Show builds the breadcrumb.
func (b *BreadcrumbW) Show() *BreadcrumbW {
	c := b.c
	b.th = c.ThemeFor(theme.CompBreadcrumb)
	b.tk = &b.th.Components.Breadcrumb
	gap := b.tk.ItemGap
	Row(c, func() {
		for i, it := range b.items {
			last := i == len(b.items)-1
			b.item(i, it, last)
			if !last {
				// A plain current page has no link padding: give the
				// separator before it the padding a link would have
				// (antd: separatorMargin 8 around the text).
				next := b.items[i+1]
				b.separator(it.Separator, i+1 == len(b.items)-1 && len(next.Menu) == 0)
			}
		}
	}, layout.Middle(), layout.Gap(gap)).Semantics(a11y.RoleNavigation, c.Locale().A11y.Breadcrumb)
	return b
}

// separator adds the separator after an item.
func (b *BreadcrumbW) separator(own string, beforePlain bool) {
	c, th, tk := b.c, b.th, b.tk
	shad := th.Flavor == theme.StyleShadcn
	ic := b.sepIcon
	if ic == nil && b.sep == "" && shad {
		ic = icon.Right
	}
	if own != "" {
		ic = nil
	}
	endPad := float32(0)
	if beforePlain {
		endPad = tk.LinkPaddingInline
	}
	if ic != nil {
		sep := c.add("crumbsep", "")
		sep.aHidden = true
		sz := tk.IconFontSize
		sep.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: sz, H: sz} }
		sep.ln.Margin = geom.Insets{Left: tk.IconSeparatorMargin, Right: tk.IconSeparatorMargin + endPad}
		col := tk.SeparatorColor
		// A directional icon (the default chevron) mirrors in RTL; a text
		// separator such as "/" does not.
		sep.paint = func(l *paint.List, r geom.Rect) { c.drawIconAt(l, ic, r.X, r.Y, sz, col) }
		return
	}
	s := b.sep
	if own != "" {
		s = own
	}
	if s == "" {
		s = "/"
	}
	t := Text(c, s).Size(tk.FontSize).Color(tk.SeparatorColor).NoWrap()
	t.Layout(layout.Margin(geom.Insets{Left: tk.SeparatorMargin, Right: tk.SeparatorMargin + endPad}))
	t.n.aHidden = true
}

// item adds item i: the current page (last) as text, the others as links.
func (b *BreadcrumbW) item(i int, it BreadcrumbItem, last bool) {
	c, th, tk := b.c, b.th, b.tk
	shad := th.Flavor == theme.StyleShadcn
	s := it.Label
	isz := tk.IconFontSize
	if last && len(it.Menu) == 0 {
		if it.Icon == nil {
			t := Text(c, s).Size(tk.FontSize).Color(tk.LastItemColor).NoWrap()
			t.n.setState(a11y.Current)
			return
		}
		col := tk.LastItemColor
		Row(c, func() {
			Icon(c, it.Icon).Size(isz).Color(col)
			if s != "" {
				Text(c, s).Size(tk.FontSize).NoWrap().Color(col)
			}
		}, layout.Middle(), layout.Gap(4)).Semantics(a11y.RoleText, s).A11yState(a11y.Current)
		return
	}
	link := func() {
		n := c.add("crumb", s)
		n.setRole(a11y.RoleLink)
		if last {
			n.setState(a11y.Current)
		}
		st := c.style(tk.FontSize, text.Regular)
		if !last && tk.LinkFontWeight != 0 {
			st.Weight = text.Weight(tk.LinkFontWeight)
		}
		menu := len(it.Menu) > 0
		extra := float32(0)
		if it.Icon != nil {
			extra += isz
			if s != "" {
				extra += 4
			}
		}
		if menu {
			extra += 4 + 12
		}
		pad := tk.LinkPaddingInline
		h := tk.ItemHeight
		if h == 0 {
			h = st.LineHeight
		}
		n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: text.Width(s, st) + extra + 2*pad, H: h} }
		ia := c.interact(n, true, input.CursorPointer)
		if ia.Clicked && !menu {
			b.clicked = i
			if it.OnClick != nil {
				it.OnClick()
			}
		}
		id := n.id
		n.paint = func(l *paint.List, r geom.Rect) {
			col := tk.ItemColor
			if last {
				col = tk.LastItemColor
			}
			if shad {
				// shadcn: muted links ease to the foreground on hover.
				col = theme.Lerp(tk.ItemColor, tk.LinkHoverColor, c.Anim(id, "hover", b2f(ia.Hovered || last), th.DurationFast))
			} else if ia.Hovered {
				col = tk.LinkHoverColor
				if tk.LinkHoverBg.A > 0 {
					l.Fill(r, geom.Rad(tk.Radius), tk.LinkHoverBg)
				}
			}
			sp := c.inline(r)
			x := pad
			if it.Icon != nil {
				c.drawIconAt(l, it.Icon, sp.X(x, isz), r.Y+(r.H-isz)/2, isz, col)
				x += isz
				if s != "" {
					x += 4
				}
			}
			lay := text.Shape(s, st, c.textOpts(text.Options{NoWrap: true}))
			ty := r.Y
			if shad {
				ty = r.Y + (r.H-lay.Height)/2
			}
			l.Glyphs(geom.Pt(sp.X(x, lay.Width), ty), lay.Glyphs, col)
			if menu {
				dic := icon.Down
				if b.ddIcon != nil {
					dic = b.ddIcon
				}
				drawIcon(l, dic, sp.X(x+lay.Width+4, 12), r.Y+(r.H-12)/2, 12, col)
			}
			if ia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			}
		}
	}
	if len(it.Menu) == 0 {
		link()
		return
	}
	dd := Dropdown(c, it.Menu).Body(link)
	if k := dd.Clicked(); k != "" {
		b.menuKey = k
	}
}

// ---------------------------------------------------------------------------
// Pagination

// PaginationW is a page navigator: page buttons with ellipsis jumps, total
// text, size changer, quick jumper, simple mode, hide on a single page,
// disabled state and Element's background (filled) style.
type PaginationW struct {
	_ [0]func()

	c          *Context
	th         *theme.Theme // Pagination tokens (c.ThemeFor(theme.CompPagination))
	page       *int
	total      int
	pageSize   int
	showTotal  bool
	small      bool
	changed    bool
	sizeOpts   bool
	ps         *int
	jumper     bool
	simple     bool
	hideSingle bool
	disabled   bool
	bg         bool
	bgSet      bool // Background given (modifier or Defaults)
	readOnly   bool
	parts      []PaginationPart // Layout; nil: the flags decide
	pagerCount int              // PagerCount; 0: Ant's window
	large      bool             // Size(Large)
	align      string           // Align: "" | "start" | "center" | "end"
	less       bool             // ShowLessItems
	totalFn    func(total, from, to int) string
	goButton   bool   // ShowQuickJumper go button
	goText     string // its label ("": Locale)
	sizeList   []int  // PageSizeOptions
	prevText   string // Element prev-text
	nextText   string
	prevIcon   *icon.Icon
	nextIcon   *icon.Icon
}

// PaginationPart is one part of a Pagination arranged by
// PaginationW.Layout (Element's layout).
type PaginationPart string

const (
	PaginationPrev   PaginationPart = "prev"   // previous-page button
	PaginationPager  PaginationPart = "pager"  // page buttons (the editable page in Simple)
	PaginationNext   PaginationPart = "next"   // next-page button
	PaginationJumper PaginationPart = "jumper" // "go to [ ] page"
	PaginationSizes  PaginationPart = "sizes"  // page-size select (needs SizeChanger)
	PaginationTotal  PaginationPart = "total"  // "total N items"
	PaginationSpacer PaginationPart = "->"     // pushes the following parts to the end
)

type pagerState struct {
	jump    string // quick jumper / simple pager input
	focused bool   // the input had focus last frame (jump on blur)
	dirty   bool   // edited since the last jump
	shown   int    // page shown in the simple pager input
}

// Pagination adds a pager bound to *page (1-based).
//
// ConfigProvider.Defaults("Pagination", ...) keys: "showQuickJumper",
// "showTotal", "simple", "hideOnSinglePage", "background", "disabled",
// "showLessItems" (bool), "size" (ControlSize or "small" | "default" |
// "large"), "align" ("start" | "center" | "end"), "pageSizeOptions"
// ([]int), "pagerCount" (int).
//
// The page buttons follow @rc-component/pagination: every page up to 7
// pages (5 with ShowLessItems), else the first, the last, the pages
// around the current one and ••• jumps of 5 (3) pages. The Element theme
// uses PagerCount(7) by default (el-pagination pager-count), Background
// on, and page sizes 10/20/30/40/50/100 (PaginationToken). With total 0
// a single disabled page 1 shows, like antd.
//
// Under shadcn/ui page links are ghost buttons, the current page an
// outline button and Previous / Next carry their label next to the
// chevron (icon only in the small and simple pagers).
func Pagination(c *Context, page *int, total, pageSize int) *PaginationW {
	p := &PaginationW{c: c, th: c.ThemeFor(theme.CompPagination), page: page, total: max(0, total), pageSize: max(1, pageSize)}
	tk := &p.th.Components.Pagination
	p.Size(c.defaultSize())
	p.bg = tk.Background
	if v, ok := c.ConfigProp("Pagination", "size"); ok {
		switch s := v.(type) {
		case ControlSize:
			p.Size(s)
		case string:
			p.Size(map[string]ControlSize{"small": Small, "large": Large}[s])
		}
	}
	p.align = Prop(c, "Pagination", "align", "")
	p.less = Prop(c, "Pagination", "showLessItems", false)
	def := []int{10, 20, 50, 100} // @rc-component/pagination Options.js
	if tk.PageSizesByTen {
		def = []int{10, 20, 30, 40, 50, 100}
	}
	p.sizeList = Prop(c, "Pagination", "pageSizeOptions", def)
	if n := Prop(c, "Pagination", "pagerCount", tk.PagerCount); n > 0 {
		p.PagerCount(n)
	}
	p.disabled = Prop(c, "Pagination", "disabled", false)
	p.jumper = Prop(c, "Pagination", "showQuickJumper", false)
	p.showTotal = Prop(c, "Pagination", "showTotal", false)
	p.simple = Prop(c, "Pagination", "simple", false)
	p.hideSingle = Prop(c, "Pagination", "hideOnSinglePage", false)
	if v, ok := p2bBool(c, "Pagination", "background"); ok {
		p.bg, p.bgSet = v, true
	}
	return p
}

func (p *PaginationW) ShowTotal() *PaginationW { p.showTotal = true; return p }
func (p *PaginationW) Small() *PaginationW     { p.small, p.large = true, false; return p }

// Size picks the small pager (Small), the default one or the large one
// (Large: 40px buttons, antd 6 size="large", Element size="large"); Small
// with Simple is the narrow pager for list and transfer footers.
func (p *PaginationW) Size(s ControlSize) *PaginationW {
	p.small, p.large = s == Small, s == Large
	return p
}

// ShowTotalFunc shows the text fn returns before the pager (antd
// showTotal(total, range)): from and to are the 1-based items of the
// current page (0, 0 when total is 0).
func (p *PaginationW) ShowTotalFunc(fn func(total, from, to int) string) *PaginationW {
	p.totalFn, p.showTotal = fn, fn != nil
	return p
}

// Align places the pager in its row: "start", "center" or "end" (antd
// align); "" leaves it at its natural width.
func (p *PaginationW) Align(a string) *PaginationW { p.align = a; return p }

// ShowLessItems shows fewer page buttons: one page on each side of the
// current one and ••• jumps of 3 pages (antd showLessItems).
func (p *PaginationW) ShowLessItems() *PaginationW { p.less = true; return p }

// GoButton adds a confirm button after the quick jumper input (antd
// showQuickJumper.goButton) and turns the jumper on; label "" uses
// Locale().Pagination.Confirm.
func (p *PaginationW) GoButton(label string) *PaginationW {
	p.jumper, p.goButton, p.goText = true, true, label
	return p
}

// PageSizeOptions sets the choices of the size changer (antd
// pageSizeOptions, default 10/20/50/100; Element page-sizes, default
// 10/20/30/40/50/100).
func (p *PaginationW) PageSizeOptions(sizes ...int) *PaginationW {
	p.sizeList = append([]int(nil), sizes...)
	return p
}

// PrevText / NextText show a text instead of the chevron in the previous /
// next buttons (Element prev-text / next-text).
func (p *PaginationW) PrevText(s string) *PaginationW { p.prevText = s; return p }
func (p *PaginationW) NextText(s string) *PaginationW { p.nextText = s; return p }

// PrevIcon / NextIcon replace the chevrons (Element prev-icon / next-icon,
// antd itemRender for prev / next).
func (p *PaginationW) PrevIcon(ic *icon.Icon) *PaginationW { p.prevIcon = ic; return p }
func (p *PaginationW) NextIcon(ic *icon.Icon) *PaginationW { p.nextIcon = ic; return p }

// ReadOnly shows the current page of a Simple pager as text instead of an
// input ("< 2 / 5 >", Ant simple.readOnly): the narrowest pager, for
// embedding at the bottom of a list.
func (p *PaginationW) ReadOnly() *PaginationW { p.readOnly = true; return p }

// ShowQuickJumper adds the "go to [ ] page" input (Enter or leaving the
// input jumps).
func (p *PaginationW) ShowQuickJumper() *PaginationW { p.jumper = true; return p }

// Simple shows only prev, an editable current page, "/ pages" and next.
func (p *PaginationW) Simple() *PaginationW { p.simple = true; return p }

// HideOnSinglePage hides the pager when there is at most one page.
func (p *PaginationW) HideOnSinglePage() *PaginationW { p.hideSingle = true; return p }

// Disabled disables every part of the pager.
func (p *PaginationW) Disabled(on bool) *PaginationW { p.disabled = on; return p }

// Background toggles the filled page buttons (Element background; the
// default of the Element theme, off by default in Ant). A Simple pager is
// plain unless Background(true) is given, so it sits lightly at the bottom
// of a list or transfer panel.
func (p *PaginationW) Background(on bool) *PaginationW { p.bg, p.bgSet = on, true; return p }

// SizeChanger shows a page-size select bound to *size.
func (p *PaginationW) SizeChanger(size *int) *PaginationW {
	p.sizeOpts, p.ps = true, size
	p.pageSize = *size
	return p
}
func (p *PaginationW) Changed() bool { return p.changed }

// Layout arranges the pager from parts in the given order (Element's
// layout "total, prev, pager, next, sizes, jumper"); only the listed parts
// show, whatever ShowTotal / ShowQuickJumper say. PaginationSizes needs
// SizeChanger. PaginationSpacer pushes the parts after it to the end of
// the row.
func (p *PaginationW) Layout(parts ...PaginationPart) *PaginationW {
	p.parts = append(p.parts[:0], parts...)
	if p.parts == nil {
		p.parts = []PaginationPart{}
	}
	return p
}

// PagerCount sets how many page-number buttons the pager shows at most
// (Element pager-count: an odd number from 5 to 21, others are rounded up
// to the next odd number in range). The first and last pages always show;
// the rest are a window around the current page with ••• jumps. Without
// it the pager follows Ant: every page up to 9 pages, else the first, the
// last and five around the current page.
func (p *PaginationW) PagerCount(n int) *PaginationW {
	n = max(5, min(n, 21))
	if n%2 == 0 {
		n++
	}
	p.pagerCount = n
	return p
}

// goTo moves to page n (clamped).
func (p *PaginationW) goTo(n, pages int) {
	n = max(1, min(n, pages))
	if n != *p.page {
		*p.page = n
		p.changed = true
		p.c.invalid = true
	}
}

// Show builds the pager.
func (p *PaginationW) Show() *PaginationW {
	c, th := p.c, p.th
	tk := &th.Components.Pagination
	pages := max(1, (p.total+p.pageSize-1)/p.pageSize)
	empty := p.total == 0 // antd: one disabled page "1"
	*p.page = max(1, min(*p.page, pages))
	if p.hideSingle && pages <= 1 {
		return p
	}
	sz, gap := tk.ItemSize, tk.ItemSpacing
	switch {
	case p.small:
		sz, gap = tk.ItemSizeSM, tk.ItemSpacingSM
	case p.large:
		sz, gap = tk.ItemSizeLG, tk.ItemSpacingLG
	}
	shad := th.Flavor == theme.StyleShadcn
	loc := c.Locale()
	allOff := p.disabled || c.disabledDepth > 0 // ConfigProvider.ComponentDisabled
	filled := p.bg && !shad && (!p.simple || p.bgSet)
	item := func(label, name string, ic *icon.Icon, target int, active, disabled bool) {
		disabled = disabled || allOff
		n := c.add("page", label)
		n.setRole(a11y.RoleButton).setName(name).stateIf(active, a11y.Current)
		st := c.style(th.FontSize, text.Regular)
		if active {
			st.Weight = text.Semibold
		}
		if shad {
			st.Weight = text.Medium
		}
		jump := label == "•••"
		prevBtn := name == loc.PrevPage
		labelled := shad && ic != nil && !jump && !p.small && !p.simple // shadcn PaginationPrevious / Next
		n.ln.Measure = func(_, _ float32) geom.Size {
			if labelled {
				return geom.Size{W: 16 + 4 + text.Width(label, st) + 20, H: sz}
			}
			if ic != nil {
				return geom.Size{W: sz, H: sz}
			}
			return geom.Size{W: max(sz, text.Width(label, st)+12), H: sz}
		}
		ia := c.interact(n, !disabled, input.CursorPointer)
		if disabled {
			n.disabled = true
			n.cursor = input.CursorNotAllowed
		}
		if ia.Clicked && !disabled && target != *p.page {
			*p.page = target
			p.changed = true
			c.invalid = true
		}
		n.paint = func(l *paint.List, r geom.Rect) {
			rad := geom.Rad(th.Radius)
			col := th.Text
			if shad {
				// shadcn: ghost icon buttons, the current page as an outline button.
				hov := c.Anim(n.id, "hover", b2f(ia.Hovered && !disabled), th.DurationFast)
				col = tk.ItemColor
				if active {
					col = tk.ItemActiveColor
					l.Shadows(r, rad, th.ButtonShadow)
					l.Fill(r, rad, theme.Lerp(tk.ItemActiveBg, th.ItemHoverBg, hov))
					if tk.ItemActiveBorderColor.A > 0 {
						l.Stroke(r, rad, 1, tk.ItemActiveBorderColor)
					}
				} else {
					l.Fill(r, rad, theme.Lerp(tk.ItemBg, th.ItemHoverBg, hov))
				}
				if disabled {
					l.Opacity(0.5)
					defer l.Pop()
				}
				if labelled {
					// Previous: chevron then label; Next: label then
					// chevron (logical order, mirrored in RTL).
					lay := text.Shape(label, st, c.textOpts(text.Options{NoWrap: true}))
					sp := c.inline(r)
					x := float32(10)
					if prevBtn {
						c.drawIconAt(l, ic, sp.X(x, 16), r.Y+(r.H-16)/2, 16, col)
						l.Glyphs(geom.Pt(sp.X(x+20, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
					} else {
						l.Glyphs(geom.Pt(sp.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
						c.drawIconAt(l, ic, sp.X(x+lay.Width+4, 16), r.Y+(r.H-16)/2, 16, col)
					}
				} else if jump {
					drawIcon(l, icon.More, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, tk.EllipsisColor)
				} else if ic != nil {
					c.drawIconAt(l, ic, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, col)
				} else {
					lay := text.Shape(label, st, text.Options{NoWrap: true})
					l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
				}
				if ia.FocusVisible {
					drawFocusRing(l, th, r, rad)
				}
				return
			}
			if ia.FocusVisible {
				defer drawFocusRing(l, th, r, rad)
			}
			switch {
			case filled:
				// Background style (Element's default): filled squares.
				col = tk.ItemFilledColor
				bg := tk.ItemFilledBg
				if active {
					bg, col = tk.ItemFilledActiveBg, tk.ItemFilledActiveColor
					if allOff {
						bg = th.Fill
					}
				} else if ia.Hovered && !disabled {
					col = th.Primary.Base
				}
				if allOff {
					bg = theme.Lerp(bg, th.BgContainerDisabled, 0.5)
				}
				l.Fill(r, geom.Rad(2), bg)
			default:
				// Plain style: Ant outlines the current page, Element
				// only colors it.
				col = tk.ItemColor
				if active {
					if allOff {
						if tk.ItemActiveBgDisabled.A > 0 {
							// antd: itemActiveBgDisabled with a colorBorder outline.
							l.Fill(r, rad, tk.ItemActiveBgDisabled)
							l.Stroke(r, rad, 1, th.Border)
						}
					} else {
						hov := ia.Hovered
						if tk.ItemActiveBg.A > 0 {
							l.Fill(r, rad, tk.ItemActiveBg)
						}
						if bc := tk.ItemActiveBorderColor; bc.A > 0 {
							if hov && tk.ItemActiveBorderHover.A > 0 {
								bc = tk.ItemActiveBorderHover
							}
							l.Stroke(r, rad, 1, bc)
						}
						col = tk.ItemActiveColor
						if hov && tk.ItemActiveColorHover.A > 0 {
							col = tk.ItemActiveColorHover
						}
					}
				} else {
					if tk.ItemBg.A > 0 {
						l.Fill(r, rad, tk.ItemBg)
					}
					if ia.Hovered && !disabled {
						if tk.ItemHoverBg.A > 0 {
							l.Fill(r, rad, tk.ItemHoverBg)
						}
						col = tk.ItemHoverColor
					}
				}
			}
			if disabled {
				col = th.TextDisabled
			}
			if jump {
				// antd jump-prev / jump-next: the ellipsis, swapped for a
				// double chevron while hovered or focused.
				if tk.JumpHoverArrow && !disabled && (ia.Hovered || ia.FocusVisible) {
					c.drawIconAt(l, ic, r.X+(r.W-12)/2, r.Y+(r.H-12)/2, 12, tk.JumpArrowColor)
					return
				}
				ec := tk.EllipsisColor
				if disabled {
					ec = th.TextDisabled
				}
				drawIcon(l, icon.More, r.X+(r.W-16)/2, r.Y+(r.H-16)/2, 16, ec)
				return
			}
			if ic != nil {
				// Previous / next chevrons mirror in RTL (the row order
				// mirrors through the layout).
				c.drawIconAt(l, ic, r.X+(r.W-12)/2, r.Y+(r.H-12)/2, 12, col)
				return
			}
			lay := text.Shape(label, st, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
		}
	}
	custom := p.parts != nil
	total := func() {
		s := loc.Total(p.total)
		if p.totalFn != nil {
			from, to := (*p.page-1)*p.pageSize+1, min(p.total, *p.page*p.pageSize)
			if empty {
				from, to = 0, 0
			}
			s = p.totalFn(p.total, from, to)
		}
		t := Text(c, s).NoWrap()
		if !custom && tk.TotalMarginEnd > 0 {
			t.Layout(layout.Margin(geom.Insets{Right: tk.TotalMarginEnd}))
		}
		if allOff {
			t.Color(th.TextDisabled)
		}
	}
	pager := func() {
		if p.simple {
			p.simplePager(pages)
			return
		}
		if empty {
			item("1", loc.Page(1), nil, 1, false, true)
			return
		}
		list := pageList(*p.page, pages, p.less)
		jump, prevName, nextName := 5, loc.Prev5, loc.Next5
		if p.less {
			jump, prevName, nextName = 3, loc.Pagination.Prev3, loc.Pagination.Next3
		}
		if p.pagerCount > 0 {
			list = pageListN(*p.page, pages, p.pagerCount)
			jump, prevName, nextName = max(5, p.pagerCount-2), loc.Prev5, loc.Next5
		}
		for _, pg := range list {
			if pg < 0 {
				target, name, ic := max(1, *p.page-jump), prevName, icon.DoubleLeft
				if pg == -2 {
					target, name, ic = min(pages, *p.page+jump), nextName, icon.DoubleRight
				}
				item("•••", name, ic, target, false, false)
				continue
			}
			item(strconv.Itoa(pg), loc.Page(pg), nil, pg, pg == *p.page, false)
		}
	}
	sizes := func() {
		if p.ps == nil || p.simple {
			return
		}
		v := strconv.Itoa(*p.ps)
		var opts []Option
		selW := float32(110)
		list := p.sizeList
		if len(list) == 0 {
			list = []int{10, 20, 50, 100}
		}
		for _, n := range list {
			lb := loc.PageSizeLabel(n)
			opts = append(opts, Option{Label: lb, Value: strconv.Itoa(n)})
			selW = max(selW, entTextW(c, lb, c.Theme.FontSize, text.Regular)+48) // longer translations
		}
		sel := Select(c, &v, opts).Width(selW).Size(p.ctlSize()).A11yLabel(loc.A11y.PageSize)
		if allOff {
			sel.Disabled(true)
		}
		if sel.Changed() {
			n, _ := strconv.Atoi(v)
			*p.ps = n
			*p.page = 1
			p.changed = true
		}
	}
	parts := p.parts
	if !custom {
		parts = make([]PaginationPart, 0, 6)
		if p.showTotal {
			parts = append(parts, PaginationTotal)
		}
		parts = append(parts, PaginationPrev, PaginationPager, PaginationNext)
		if p.sizeOpts {
			parts = append(parts, PaginationSizes)
		}
		if p.jumper {
			parts = append(parts, PaginationJumper)
		}
	}
	nav := Row(c, func() {
		for _, part := range parts {
			switch part {
			case PaginationTotal:
				total()
			case PaginationPrev:
				lb, ic := loc.PrevPage, p.prevIcon
				if ic == nil {
					ic = icon.Left
				}
				if p.prevText != "" {
					lb, ic = p.prevText, nil
				}
				item(lb, loc.PrevPage, ic, *p.page-1, false, *p.page <= 1)
			case PaginationPager:
				pager()
			case PaginationNext:
				lb, ic := loc.NextPage, p.nextIcon
				if ic == nil {
					ic = icon.Right
				}
				if p.nextText != "" {
					lb, ic = p.nextText, nil
				}
				item(lb, loc.NextPage, ic, *p.page+1, false, *p.page >= pages)
			case PaginationSizes:
				sizes()
			case PaginationJumper:
				if !p.simple {
					p.quickJumper(pages)
				}
			case PaginationSpacer:
				Spacer(c)
			}
		}
	}, p.rowOpts(gap)...)
	nav.Semantics(a11y.RoleNavigation, loc.A11y.Pagination)
	return p
}

// rowOpts lays out the pager row; Align stretches it across a column and
// places the parts (antd justify-content start / center / end).
func (p *PaginationW) rowOpts(gap float32) []layout.Option {
	opts := []layout.Option{layout.Gap(gap), layout.Middle()}
	switch p.align {
	case "start":
		opts = append(opts, layout.Self(layout.AlignStretch))
	case "center":
		opts = append(opts, layout.Self(layout.AlignStretch), layout.JustifyContent(layout.JustifyCenter))
	case "end":
		opts = append(opts, layout.Self(layout.AlignStretch), layout.End())
	}
	return opts
}

// ctlSize is the size of the inputs, select and button inside the pager.
func (p *PaginationW) ctlSize() ControlSize {
	switch {
	case p.small:
		return Small
	case p.large:
		return Large
	}
	return Middle
}

// pagerInput is the page number box of the quick jumper and the simple
// pager: Enter (or leaving it) jumps to the typed page.
func (p *PaginationW) pagerInput(st *pagerState, pages int, width float32, name string) bool {
	c := p.c
	in := Input(c, &st.jump).Width(width).A11yLabel(name).Size(p.ctlSize())
	m := float32(8)
	if p.small {
		m = 4
	}
	if p.disabled {
		in.Disabled(true)
	}
	in.Layout(layout.Margin(geom.Insets{Left: m, Right: m}))
	focused := in.Focused()
	if in.Changed() {
		st.dirty = true
	}
	// Enter jumps; leaving the box jumps only when it was edited.
	jump := in.Submitted() || (st.focused && !focused && st.dirty)
	st.focused = focused
	if !jump {
		return false
	}
	st.dirty = false
	if n, err := strconv.Atoi(strings.TrimSpace(st.jump)); err == nil {
		p.goTo(n, pages)
	}
	return true
}

// quickJumper adds "go to [ ] page".
func (p *PaginationW) quickJumper(pages int) {
	c, th := p.c, p.th
	loc := c.Locale()
	Row(c, func() {
		st := State[pagerState](c, hashID(c.cur.id, "pager-jump"), nil)
		col := th.Text
		if p.disabled {
			col = th.TextDisabled
		}
		Text(c, loc.JumpTo).NoWrap().Color(col)
		tk := &th.Components.Pagination
		w := tk.QuickJumperInputWidth
		if p.small {
			w = tk.QuickJumperInputWidthSM
		}
		if p.pagerInput(st, pages, w, c.Locale().Pagination.Jump) {
			st.jump = "" // the box clears after jumping
		}
		Text(c, loc.PageSuffix).NoWrap().Color(col)
		if p.goButton {
			// antd goButton: a confirm button that jumps to the typed page.
			lb := p.goText
			if lb == "" {
				lb = loc.Pagination.Confirm
			}
			b := Button(c, lb).Size(p.ctlSize()).Layout(layout.Margin(geom.Insets{Left: 8}))
			if p.disabled || c.disabledDepth > 0 {
				b.Disabled(true)
			}
			if b.Clicked() {
				if n, err := strconv.Atoi(strings.TrimSpace(st.jump)); err == nil {
					p.goTo(n, pages)
				}
				st.jump, st.dirty = "", false
			}
		}
	}, layout.Middle(), layout.Margin(geom.Insets{Left: 8}))
}

// simplePager shows "[current] / pages" between the arrows.
func (p *PaginationW) simplePager(pages int) {
	c, th := p.c, p.th
	Row(c, func() {
		col := th.Text
		if p.disabled {
			col = th.TextDisabled
		}
		if p.readOnly {
			// "2 / 5" as text: the current page is announced with the pager.
			m := map[bool]float32{true: 4, false: 8}[p.small]
			Text(c, strconv.Itoa(*p.page)+" / "+strconv.Itoa(pages)).NoWrap().Color(col).
				Layout(layout.Margin(geom.Insets{Left: m, Right: m}))
			return
		}
		st := State[pagerState](c, hashID(c.cur.id, "pager-simple"), nil)
		if !st.focused && (st.shown != *p.page || st.jump == "") {
			st.jump, st.shown = strconv.Itoa(*p.page), *p.page
		}
		tk := &th.Components.Pagination
		w := tk.QuickJumperInputWidth // antd simple-pager input: quickJumperInputWidth
		if p.small {
			w = tk.QuickJumperInputWidthSM
		}
		if p.pagerInput(st, pages, w, c.Locale().Pagination.Current) || !st.focused {
			st.jump, st.shown = strconv.Itoa(*p.page), *p.page
		}
		Text(c, "/").NoWrap().Color(col).Layout(layout.Margin(geom.Insets{Right: map[bool]float32{true: 4, false: 8}[p.small]})).n.aHidden = true
		Text(c, strconv.Itoa(pages)).NoWrap().Color(col)
	}, layout.Middle())
}

// pageListN returns at most count page numbers (odd, >= 5) with -1 / -2
// for the left / right ellipsis: the first and last pages and a window
// around cur (Element's pager-count).
func pageListN(cur, pages, count int) []int {
	if pages <= count {
		out := make([]int, pages)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	half := (count - 1) / 2
	prevMore := cur > count-half
	nextMore := cur < pages-half
	lo, hi := 2, count-1
	switch {
	case prevMore && !nextMore:
		lo, hi = pages-(count-2), pages-1
	case prevMore && nextMore:
		off := count/2 - 1
		lo, hi = cur-off, cur+off
	}
	out := []int{1}
	if prevMore {
		out = append(out, -1)
	}
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	if nextMore {
		out = append(out, -2)
	}
	return append(out, pages)
}

// pageList returns page numbers with -1 / -2 for the left / right •••
// jump, ported from @rc-component/pagination 1.7 Pagination.js: every page
// up to 3+2*buffer pages, else the first and last pages and buffer pages
// on each side of cur (one fewer next to a jump unless less); buffer is 2,
// 1 with showLessItems.
func pageList(cur, pages int, less bool) []int {
	buf := 2
	if less {
		buf = 1
	}
	if pages <= 3+buf*2 {
		out := make([]int, pages)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	left, right := max(1, cur-buf), min(cur+buf, pages)
	if cur-1 <= buf {
		right = 1 + buf*2
	}
	if pages-cur <= buf {
		left = pages - buf*2
	}
	jumpPrev := cur-1 >= buf*2 && cur != 1+2
	jumpNext := pages-cur >= buf*2 && cur != pages-2
	if !less && jumpPrev && right != pages {
		left++
	}
	if !less && jumpNext && left != 1 {
		right--
	}
	var out []int
	if left != 1 {
		out = append(out, 1)
	}
	if jumpPrev {
		out = append(out, -1)
	}
	for i := left; i <= right; i++ {
		out = append(out, i)
	}
	if jumpNext {
		out = append(out, -2)
	}
	if right != pages {
		out = append(out, pages)
	}
	return out
}
