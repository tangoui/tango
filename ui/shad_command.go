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
	list    ID      // listbox node (0 when nothing matches)
	optIDs  fillIDs // option nodes by visible index (ActiveDescendant)
	virt    int     // Virtual: 0 auto (long lists), 1 always, -1 never

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

	// Long (virtual) lists: the filtered items and the row layout, kept
	// while the items slice (identity and length) and the search stay.
	fkey cmdFilterKey
	fok  bool
	vis  []int
	lay  cmdLayout
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

// Virtual forces the virtual list on or off; by default lists showing
// more than 100 items are virtual: only the rows in view and the active
// item are built (groups only for their visible part), the list keeps the
// width, height and scroll range of the full list and follows the active
// item when it moves with the keyboard, but items out of view are not in
// the accessibility tree. Without a Filter function the matches and the
// width are reused while the items slice (identity and length) and the
// search stay the same; Virtual(false) builds every row.
func (w *CommandW) Virtual(on bool) *CommandW { w.virt = p2bVirtualMode(on); return w }

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

// cmdFilterKey identifies the input of a cached filter.
type cmdFilterKey struct {
	items    *CommandItem
	n        int
	query    string
	noFilter bool
}

// filter sets visible, reusing the last result for a long list (see
// Virtual) while the items and the search are unchanged.
func (w *CommandW) filter() {
	if w.filterFn != nil || len(w.items) == 0 || !p2bVirtualOn(w.virt, len(w.items)) {
		w.filterItems()
		return
	}
	k := cmdFilterKey{items: &w.items[0], n: len(w.items), query: w.st.query, noFilter: w.noFilter}
	if w.st.fok && w.st.fkey == k {
		w.visible = w.st.vis
		return
	}
	w.visible = nil // a new slice: the cached one is never written
	w.filterItems()
	w.st.fkey, w.st.vis, w.st.fok = k, w.visible, true
}

func (w *CommandW) filterItems() {
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
	w.optIDs.reset()
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
				w.heading(g)
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
	if p2bVirtualOn(w.virt, len(w.visible)) {
		w.buildVirtual(pad)
		return
	}
	Scroll(c, func() {
		lb := Column(c, func() {
			from := 0
			for vi := 1; vi <= len(w.visible); vi++ {
				if vi < len(w.visible) && w.items[w.visible[vi]].Group == w.items[w.visible[from]].Group {
					continue
				}
				if from > 0 {
					w.separator(pad)
				}
				groupRun(from, vi)
				from = vi
			}
		})
		lb.Semantics(a11y.RoleListbox, w.place)
		w.list = lb.n.id
	}, layout.MaxH(w.maxH), layout.Pad(pad))
}

// heading adds the title row of group g (hidden: it names the group).
func (w *CommandW) heading(g string) *Box {
	c, tk := w.c, w.tk
	hs := c.style(tk.GroupTitleFontSize, themeWeight(tk.GroupTitleFontWeight))
	px := tk.GroupTitlePadding
	return Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: text.Width(g, hs) + 2*px, H: tk.GroupTitleHeight} }, func(l *paint.List, r geom.Rect) {
		lay := text.Shape(g, hs, c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(c.inline(r).X(px, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, tk.GroupTitleColor)
	}).A11yHidden() // the group's name
}

// separator adds the line between two groups (pad: the list padding).
func (w *CommandW) separator(pad float32) {
	tk := w.tk
	Custom(w.c, func(_, _ float32) geom.Size { return geom.Size{H: 2*pad + 1} }, func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X - pad, Y: r.Y + pad, W: r.W + 2*pad, H: 1}, geom.Radii{}, tk.SeparatorColor)
	}).A11yHidden()
}

// ---------------------------------------------------------------------------
// Virtual list

// Row kinds of the virtual list.
const (
	cmdRowItem uint8 = iota
	cmdRowTitle
	cmdRowSep
)

// cmdRow is a row of the virtual list: item visible[vi], the heading of
// group run run, or the separator before it.
type cmdRow struct {
	kind uint8
	run  int32
	vi   int32
}

// cmdRun is a group's items visible[from:to]; their rows start at item.
type cmdRun struct {
	from, to, item int
	g              string
}

// cmdLayoutKey identifies the input of a row layout.
type cmdLayoutKey struct {
	vis                 *int
	n                   int
	itemH, titleH, sepH float32
}

// cmdLayout is the row layout of the virtual list: one row per item,
// heading and separator in the order of the full list, with the prefix
// sums of their heights.
type cmdLayout struct {
	key  cmdLayoutKey
	ok   bool
	rows []cmdRow
	runs []cmdRun
	offs []float64
}

// rowOf returns the row of item visible[vi] (-1 when out of range).
func (l *cmdLayout) rowOf(vi int) int {
	k := sort.Search(len(l.runs), func(k int) bool { return l.runs[k].to > vi })
	if vi < 0 || k >= len(l.runs) {
		return -1
	}
	return l.runs[k].item + vi - l.runs[k].from
}

// layout returns the row layout of visible (rebuilt when visible or the
// row heights changed).
func (w *CommandW) layout(pad float32) *cmdLayout {
	tk := w.tk
	k := cmdLayoutKey{vis: &w.visible[0], n: len(w.visible), itemH: tk.ItemHeight, titleH: tk.GroupTitleHeight, sepH: 2*pad + 1}
	l := &w.st.lay
	if l.ok && l.key == k {
		return l
	}
	l.key, l.ok = k, true
	l.rows, l.runs = l.rows[:0], l.runs[:0]
	group := func(vi int) string { return w.items[w.visible[vi]].Group }
	from := 0
	for vi := 1; vi <= len(w.visible); vi++ {
		if vi < len(w.visible) && group(vi) == group(from) {
			continue
		}
		run := int32(len(l.runs))
		if from > 0 {
			l.rows = append(l.rows, cmdRow{kind: cmdRowSep, run: run})
		}
		g := group(from)
		if g != "" {
			l.rows = append(l.rows, cmdRow{kind: cmdRowTitle, run: run})
		}
		l.runs = append(l.runs, cmdRun{from: from, to: vi, item: len(l.rows), g: g})
		for j := from; j < vi; j++ {
			l.rows = append(l.rows, cmdRow{kind: cmdRowItem, run: run, vi: int32(j)})
		}
		from = vi
	}
	l.offs = append(l.offs[:0], 0)
	y := 0.0
	for _, r := range l.rows {
		switch r.kind {
		case cmdRowItem:
			y += float64(k.itemH)
		case cmdRowTitle:
			y += float64(k.titleH)
		default:
			y += float64(k.sepH)
		}
		l.offs = append(l.offs, y)
	}
	return l
}

// rowW is the width row r asks for (as its node's Measure does).
func (w *CommandW) rowW(l *cmdLayout, r int) float32 {
	c, th, tk := w.c, w.th, w.tk
	row := l.rows[r]
	switch row.kind {
	case cmdRowItem:
		return text.Width(w.items[w.visible[row.vi]].Label, c.style(th.FontSize, text.Regular)) + 2*tk.ItemPaddingInline
	case cmdRowTitle:
		hs := c.style(tk.GroupTitleFontSize, themeWeight(tk.GroupTitleFontWeight))
		return text.Width(l.runs[row.run].g, hs) + 2*tk.GroupTitlePadding
	}
	return 0
}

// buildVirtual builds a long list: only the rows in view (inside their
// group for the visible part of a titled group) and the active item, with
// the width of the widest row and the active row kept in view while it
// moves with the keyboard.
func (w *CommandW) buildVirtual(pad float32) {
	c := w.c
	l := w.layout(pad)
	sb := fillViewport(c, "scroll", []layout.Option{layout.MaxH(w.maxH), layout.Pad(pad)})
	wd := fillWidest(c, sb.n.id, cmdLayoutKey{vis: &w.visible[0], n: len(w.visible)}, len(l.rows), func(r int) float32 { return w.rowW(l, r) })
	idx := w.st.active
	keep := l.rowOf(idx)
	win := fillWin{offs: l.offs}
	win.span(sb, len(l.rows))
	item := func(r int) *node {
		vi := int(l.rows[r].vi)
		it := w.items[w.visible[vi]]
		var n *node
		Key(c, it.Value+"\x00"+it.Label, func() { n = w.item(it, vi) })
		return n
	}
	sb.Body(func() {
		lb := Column(c, func() {
			if keep >= 0 && keep < win.first {
				win.place(item(keep), keep, geom.Insets{})
			}
			for r := win.first; r < win.end; {
				row := l.rows[r]
				if row.kind == cmdRowSep {
					w.separator(pad)
					r++
					continue
				}
				e := r + 1
				for e < win.end && l.rows[e].run == row.run {
					e++
				}
				from, to := r, e
				seg := func() {
					for k := from; k < to; k++ {
						if l.rows[k].kind == cmdRowTitle {
							w.heading(l.runs[row.run].g)
						} else {
							item(k)
						}
					}
				}
				run := l.runs[row.run]
				if run.g == "" {
					seg()
				} else {
					// The group node exists for its visible part only; it
					// reports the size of the whole group.
					Key(c, row.run, func() {
						grp := Column(c, seg).Semantics(a11y.RoleGroup, run.g)
						if c.a11yOn {
							size := run.to - run.from
							grp.n.fill(semFunc(func(s *a11y.Node) { s.SetSize = size }))
						}
					})
				}
				r = e
			}
			if keep >= win.end {
				win.place(item(keep), keep, geom.Insets{})
			}
			fillStrut(c, wd)
		})
		lb.Semantics(a11y.RoleListbox, w.place)
		w.list = lb.n.id
	})
	followRowBy(c, sb, idx, w.st.active, func(vi int) (float64, float64) {
		r := l.rowOf(vi)
		if r < 0 {
			return 0, 0
		}
		return float64(pad) + l.offs[r], l.offs[r+1] - l.offs[r]
	})
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
	if id := w.optIDs.at(w.st.active); id != 0 {
		s.ActiveDescendant = a11y.NodeID(id)
	}
}

func (w *CommandW) item(it CommandItem, vi int) *node {
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
	w.optIDs.add(vi, n.id)
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
	return n
}
