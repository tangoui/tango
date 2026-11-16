package ui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// CommandItem is one entry of a Command palette.
type CommandItem struct {
	Value    string
	Label    string
	Group    string     // heading the item is listed under ("" = none)
	Icon     *icon.Icon // optional leading icon
	Shortcut string     // right-aligned hint such as "⌘P"
	Keywords []string   // extra search terms
	Disabled bool
}

// CommandW is a command palette under construction (shadcn/ui Command):
// a search field over a filtered, grouped list navigated with the arrow
// keys. It renders in every design system with the active theme's tokens.
type CommandW struct {
	_ [0]func()

	c       *Context
	box     *Box
	items   []CommandItem
	st      *commandState
	in      *InputW
	keys    []input.KeyEvent
	place   string
	empty   string
	maxH    float32
	picked  *CommandItem
	visible []int
	z       *lazy
	list    ID   // listbox node (0 when nothing matches)
	optIDs  []ID // option nodes, parallel to visible (ActiveDescendant)

	th         *theme.Theme // ThemeFor(CompCommand)
	tk         *theme.CommandToken
	loop       bool
	vim        bool
	noFilter   bool
	noPointer  bool
	filterFn   func(value, search string, keywords []string) float64
	valuePtr   *string
	searchPtr  *string
	valChanged bool
}

type commandState struct {
	query  string
	active int
	last   string
	ptr    geom.Point
	moved  bool
	bound  string // search text last written to Search
	val    string // value of the active item at the end of the last build
}

// Command adds a command palette over items. Typing filters and ranks the
// list like cmdk: an item matches when the search letters appear in order
// in its label, value or keywords (case-insensitive); whole-word and
// prefix matches rank first and groups follow their best item. Up/Down
// (and Ctrl+N/J, Ctrl+P/K) move the active row, Home/End jump to the first
// / last, Enter or a click picks it (Selected).
//
// ConfigProvider.Defaults("Command", ...) keys: "placeholder", "emptyText"
// (string), "loop" (bool, default false), "shouldFilter" (bool, default
// true), "vimBindings" (bool, default true).
func Command(c *Context, items []CommandItem) *CommandW {
	loc := c.Locale()
	w := &CommandW{c: c, items: items, place: loc.Command.Placeholder, empty: loc.Command.Empty}
	w.th = c.ThemeFor(theme.CompCommand)
	w.tk = &w.th.Components.Command
	w.maxH = w.tk.ListMaxHeight
	w.configDefaults() // Defaults("Command", ...): placeholder, emptyText
	w.loop = Prop(c, "Command", "loop", false)
	w.noFilter = !Prop(c, "Command", "shouldFilter", true)
	w.vim = Prop(c, "Command", "vimBindings", true)
	tk := w.tk
	w.box = Div(c, layout.MinW(0))
	w.box.Bg(tk.Bg).Radius(tk.Radius).Clip()
	if tk.BorderColor.A > 0 {
		w.box.Border(1, tk.BorderColor)
	}
	w.st = State[commandState](c, hashID(w.box.n.id, "cmd"), nil)
	w.z = c.lazy(w.box.n, w.build)
	return w
}

// Placeholder replaces the search placeholder.
func (w *CommandW) Placeholder(s string) *CommandW { w.place = s; return w }

// Empty replaces the "no results" text.
func (w *CommandW) Empty(s string) *CommandW { w.empty = s; return w }

// MaxHeight limits the list height (default 300).
func (w *CommandW) MaxHeight(h float32) *CommandW { w.maxH = h; return w }

// Shadow gives the palette a floating shadow (as in a dialog).
func (w *CommandW) Shadow() *CommandW { w.box.Shadow(w.tk.Shadow); return w }

// Loop lets the arrow keys wrap from the last item to the first and back
// (cmdk loop; default false).
func (w *CommandW) Loop(on bool) *CommandW { w.loop = on; return w }

// VimBindings(false) turns off Ctrl+N/J (next) and Ctrl+P/K (previous)
// (cmdk vimBindings; default true).
func (w *CommandW) VimBindings(on bool) *CommandW { w.vim = on; return w }

// ShouldFilter(false) lists every item whatever the search (cmdk
// shouldFilter): the app filters items itself, for example from a server,
// reading Query.
func (w *CommandW) ShouldFilter(on bool) *CommandW { w.noFilter = !on; return w }

// Filter replaces the matching and ranking (cmdk filter): fn returns 0 to
// hide an item, else its rank (higher first) for the item's value (Label
// when Value is empty), the trimmed search and its keywords.
func (w *CommandW) Filter(fn func(value, search string, keywords []string) float64) *CommandW {
	w.filterFn = fn
	return w
}

// Value binds the value of the active item (cmdk value / onValueChange):
// the palette highlights the item whose Value (Label when empty) equals
// *v and writes the active item's value back as it moves. A value that is
// not listed leaves the first item active.
func (w *CommandW) Value(v *string) *CommandW { w.valuePtr = v; return w }

// ValueChanged reports that the active item changed this frame.
func (w *CommandW) ValueChanged() bool { w.z.run(); return w.valChanged }

// Search binds the search text (cmdk CommandInput value /
// onValueChange).
func (w *CommandW) Search(q *string) *CommandW { w.searchPtr = q; return w }

// DisablePointerSelection stops hovering from moving the active item
// (cmdk disablePointerSelection); clicks still pick.
func (w *CommandW) DisablePointerSelection(on bool) *CommandW { w.noPointer = on; return w }

// Layout applies layout options to the palette box.
func (w *CommandW) Layout(opts ...layout.Option) *CommandW { w.box.Style(opts...); return w }

// Query returns the current search text.
func (w *CommandW) Query() string { return w.st.query }

// Selected returns the item picked this frame.
func (w *CommandW) Selected() (CommandItem, bool) {
	w.z.run()
	if w.picked == nil {
		return CommandItem{}, false
	}
	return *w.picked, true
}

// ID returns the palette box ID.
func (w *CommandW) ID() ID { return w.box.n.id }

// itemValue is the value an item is matched and bound by.
func commandValue(it CommandItem) string {
	if it.Value != "" {
		return it.Value
	}
	return it.Label
}

// score ranks an item for the lower-case search q (0: hidden).
func (w *CommandW) score(it CommandItem, q string) float64 {
	if w.filterFn != nil {
		return w.filterFn(commandValue(it), q, it.Keywords)
	}
	if q == "" {
		return 1
	}
	best := max(commandMatch(it.Label, q), commandMatch(it.Value, q))
	for _, k := range it.Keywords {
		best = max(best, commandMatch(k, q)*0.95)
	}
	return best
}

// commandMatch scores how well s matches the lower-case search q: 1 for
// the whole text, then a prefix, a word start, any substring, and lowest
// the search letters in order with gaps (fewer gaps rank higher); 0 when
// the letters do not all appear in order.
func commandMatch(s, q string) float64 {
	t := strings.ToLower(s)
	switch {
	case q == "" || t == "":
		return 0
	case t == q:
		return 1
	case strings.HasPrefix(t, q):
		return 0.9
	}
	if i := strings.Index(t, q); i >= 0 {
		r, _ := utf8.DecodeLastRuneInString(t[:i])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return 0.8 // starts a word
		}
		return 0.7
	}
	// Letters in order: score by how tightly they sit.
	qr := []rune(q)
	k, first, last := 0, -1, -1
	for i, r := range []rune(t) {
		if k < len(qr) && r == qr[k] {
			if first < 0 {
				first = i
			}
			last = i
			k++
		}
	}
	if k < len(qr) {
		return 0
	}
	span := float64(last - first + 1)
	return 0.1 + 0.5*float64(len(qr))/span
}

func (w *CommandW) filter() {
	q := strings.ToLower(strings.TrimSpace(w.st.query))
	if w.filterFn != nil {
		q = strings.TrimSpace(w.st.query)
	}
	w.visible = w.visible[:0]
	// Keep groups together in order of first appearance; with a search,
	// rank items within a group and groups by their best item.
	type entry struct {
		i     int
		score float64
	}
	var groups []string
	byGroup := map[string][]entry{}
	for i, it := range w.items {
		sc := 1.0
		if !w.noFilter {
			sc = w.score(it, q)
		}
		if _, ok := byGroup[it.Group]; !ok {
			groups = append(groups, it.Group)
			byGroup[it.Group] = nil
		}
		if sc > 0 {
			byGroup[it.Group] = append(byGroup[it.Group], entry{i, sc})
		}
	}
	rank := q != "" && !w.noFilter
	if rank {
		for _, g := range groups {
			es := byGroup[g]
			sort.SliceStable(es, func(a, b int) bool { return es[a].score > es[b].score })
		}
		sort.SliceStable(groups, func(a, b int) bool {
			ga, gb := byGroup[groups[a]], byGroup[groups[b]]
			if len(ga) == 0 || len(gb) == 0 {
				return len(gb) == 0 && len(ga) > 0
			}
			return ga[0].score > gb[0].score
		})
	}
	for _, g := range groups {
		for _, e := range byGroup[g] {
			w.visible = append(w.visible, e.i)
		}
	}
}

// step moves the active item to the next enabled one (cmdk: wraps only
// with Loop).
func (w *CommandW) step(down bool) {
	n := len(w.visible)
	if n == 0 {
		return
	}
	d := 1
	if !down {
		d = -1
	}
	for j := w.st.active + d; ; j += d {
		if w.loop {
			j = (j + n) % n
		}
		if j < 0 || j >= n || j == w.st.active {
			return
		}
		if !w.items[w.visible[j]].Disabled {
			w.st.active = j
			return
		}
	}
}

// edge moves the active item to the first (or last) enabled one.
func (w *CommandW) edge(last bool) {
	n := len(w.visible)
	for k := range n {
		j := k
		if last {
			j = n - 1 - k
		}
		if !w.items[w.visible[j]].Disabled {
			w.st.active = j
			return
		}
	}
}

func (w *CommandW) build() {
	c, tk := w.c, w.tk
	if w.searchPtr != nil && *w.searchPtr != w.st.bound {
		w.st.query = *w.searchPtr // changed by the app
	}
	// Search row: icon + borderless input, separated by a bottom border.
	row := Row(c, func() {
		Icon(c, icon.Search).Size(tk.InputIconSize).Color(tk.InputIconColor)
		w.in = Input(c, &w.st.query).Borderless().Placeholder(w.place).Layout(layout.Grow(1))
		// The search field is a combobox driving the always-open listbox:
		// focus stays in it, the active option is its ActiveDescendant.
		w.in.ed.setRole(a11y.RoleCombobox).setState(a11y.Editable | a11y.Expandable | a11y.HasPopup).fill(w)
	}, layout.H(tk.InputHeight), layout.PadVH(0, tk.InputPaddingInline), layout.Gap(0), layout.Middle())
	row.Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.InputBorderColor)
	})
	vimKey := func(k input.KeyEvent) bool {
		return w.vim && k.Mods&input.ModCtrl != 0 && (k.Key == input.KeyN || k.Key == input.KeyJ || k.Key == input.KeyP || k.Key == input.KeyK)
	}
	w.keys = msTakeKeys(c, w.in.ed.id, func(k input.KeyEvent) bool {
		switch k.Key {
		case input.KeyUp, input.KeyDown, input.KeyEnter, input.KeyHome, input.KeyEnd:
			return true
		}
		return vimKey(k)
	})
	if w.searchPtr != nil {
		*w.searchPtr = w.st.query
		w.st.bound = w.st.query
	}
	w.st.moved = c.pointer != w.st.ptr
	w.st.ptr = c.pointer
	queryChanged := w.st.query != w.st.last
	if queryChanged {
		w.st.last, w.st.active = w.st.query, 0
	}
	w.filter()
	if w.valuePtr != nil && !queryChanged {
		// The bound value picks the active item (unknown: keep it).
		for vi, i := range w.visible {
			if commandValue(w.items[i]) == *w.valuePtr {
				w.st.active = vi
				break
			}
		}
	}
	if w.st.active >= len(w.visible) {
		w.st.active = 0
	}
	if len(w.visible) > 0 && w.items[w.visible[w.st.active]].Disabled {
		w.edge(false)
	}
	for _, k := range w.keys {
		if !k.Down {
			continue
		}
		switch {
		case k.Key == input.KeyDown, vimKey(k) && (k.Key == input.KeyN || k.Key == input.KeyJ):
			w.step(true)
		case k.Key == input.KeyUp, vimKey(k):
			w.step(false)
		case k.Key == input.KeyHome:
			w.edge(false)
		case k.Key == input.KeyEnd:
			w.edge(true)
		case k.Key == input.KeyEnter:
			if w.st.active < len(w.visible) {
				if it := w.items[w.visible[w.st.active]]; !it.Disabled {
					w.picked = &it
				}
			}
		}
		c.invalid = true
	}
	w.optIDs = w.optIDs[:0]
	w.list = 0
	defer w.syncValue()
	if len(w.visible) == 0 {
		Row(c, func() { Text(c, w.empty).Color(tk.EmptyColor) }, layout.PadVH(tk.EmptyPaddingBlock, 8), layout.Center()).Semantics(a11y.RoleStatus, w.empty)
		return
	}
	// groupRun builds the items visible[from:to] of one group, inside a
	// named group when the group has a heading.
	groupRun := func(from, to int) {
		g := w.items[w.visible[from]].Group
		body := func() {
			if g != "" {
				hs := c.style(tk.GroupTitleFontSize, themeWeight(tk.GroupTitleFontWeight))
				px := tk.GroupTitlePadding
				Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: text.Width(g, hs) + 2*px, H: tk.GroupTitleHeight} }, func(l *paint.List, r geom.Rect) {
					lay := text.Shape(g, hs, c.textOpts(text.Options{NoWrap: true}))
					l.Glyphs(geom.Pt(c.inline(r).X(px, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, tk.GroupTitleColor)
				}).A11yHidden() // the group's name
			}
			for vi := from; vi < to; vi++ {
				it, vi := w.items[w.visible[vi]], vi
				Key(c, it.Value+"\x00"+it.Label, func() { w.item(it, vi) })
			}
		}
		if g == "" {
			body()
			return
		}
		Column(c, body).Semantics(a11y.RoleGroup, g)
	}
	pad := tk.ListPadding
	Scroll(c, func() {
		lb := Column(c, func() {
			from := 0
			for vi := 1; vi <= len(w.visible); vi++ {
				if vi < len(w.visible) && w.items[w.visible[vi]].Group == w.items[w.visible[from]].Group {
					continue
				}
				if from > 0 {
					Custom(c, func(_, _ float32) geom.Size { return geom.Size{H: 2*pad + 1} }, func(l *paint.List, r geom.Rect) {
						l.Fill(geom.Rect{X: r.X - pad, Y: r.Y + pad, W: r.W + 2*pad, H: 1}, geom.Radii{}, tk.SeparatorColor)
					}).A11yHidden()
				}
				groupRun(from, vi)
				from = vi
			}
		})
		lb.Semantics(a11y.RoleListbox, w.place)
		w.list = lb.n.id
	}, layout.MaxH(w.maxH), layout.Pad(pad))
}

// syncValue reports a change of the active item since the last build and
// writes its value to the bound Value.
func (w *CommandW) syncValue() {
	cur := ""
	if w.st.active < len(w.visible) {
		cur = commandValue(w.items[w.visible[w.st.active]])
	}
	if cur != w.st.val {
		w.valChanged = w.st.val != "" || w.valuePtr != nil && *w.valuePtr != cur
		w.st.val = cur
	}
	if w.valuePtr != nil && cur != "" {
		*w.valuePtr = cur
	}
}

// semantics completes the search combobox: the listbox it controls and
// the active option.
func (w *CommandW) semantics(s *a11y.Node) {
	if w.list == 0 {
		return
	}
	s.States |= a11y.Expanded
	s.Controls = a11y.NodeID(w.list)
	if w.st.active < len(w.optIDs) {
		s.ActiveDescendant = a11y.NodeID(w.optIDs[w.st.active])
	}
}

func (w *CommandW) item(it CommandItem, vi int) {
	c, th, tk := w.c, w.th, w.tk
	n := c.add("commanditem", it.Label)
	st := c.style(th.FontSize, text.Regular)
	small := c.style(tk.ShortcutFontSize, text.Regular)
	n.ln.Measure = func(maxW, _ float32) geom.Size {
		return geom.Size{W: text.Width(it.Label, st) + 2*tk.ItemPaddingInline, H: tk.ItemHeight}
	}
	disabled := it.Disabled || n.disabled
	ia := c.interact(n, false, input.CursorPointer)
	if disabled {
		n.disabled = true
		n.cursor = input.CursorNotAllowed
		n.setState(a11y.Disabled)
	}
	// Options are reached with the arrow keys of the search field.
	n.aKbd = true
	n.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(vi == w.st.active, a11y.Selected)
	w.optIDs = append(w.optIDs, n.id)
	if c.a11yOn {
		pos, size, sc := vi+1, len(w.visible), it.Shortcut
		n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize, s.Shortcut = pos, size, sc }))
	}
	if ia.Hovered && !disabled && w.st.moved && !w.noPointer {
		w.st.active = vi
	}
	if ia.Clicked && !disabled {
		w.st.active = vi
		w.picked = &it
		c.invalid = true
	}
	active := vi == w.st.active
	n.paint = func(l *paint.List, r geom.Rect) {
		if active && !disabled {
			l.Fill(r, geom.Rad(tk.ItemRadius), tk.ItemActiveBg)
		}
		fg := tk.ItemColor
		if active {
			fg = tk.ItemActiveColor
		}
		if disabled {
			l.Opacity(tk.DisabledOpacity)
			defer l.Pop()
		}
		// Icon and label from the start, the shortcut at the end.
		s := c.inline(r)
		x := tk.ItemPaddingInline
		if it.Icon != nil {
			is := tk.ItemIconSize
			c.drawIconAt(l, it.Icon, s.X(x, is), r.Y+(r.H-is)/2, is, tk.ItemIconColor)
			x += is + tk.ItemGap
		}
		lay := text.Shape(it.Label, st, c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
		if it.Shortcut != "" {
			// Shortcuts read left to right; letter spacing after every
			// glyph (shadcn tracking-widest).
			sl := text.Shape(it.Shortcut, small, text.Options{NoWrap: true})
			ls := tk.ShortcutLetterSpacing
			glyphs := sl.Glyphs
			if ls != 0 {
				glyphs = append([]paint.Glyph(nil), sl.Glyphs...)
				for i := range glyphs {
					glyphs[i].X += ls * float32(i)
				}
			}
			sw := sl.Width + ls*float32(len(glyphs))
			l.Glyphs(geom.Pt(s.X(r.W-tk.ItemPaddingInline-sw, sw), r.Y+(r.H-sl.Height)/2), glyphs, tk.ShortcutColor)
		}
	}
}
