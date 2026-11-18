package ui

// The Height form of Conversations: a fixed-height scrolling list, and
// with Virtual only the rows in view are built. Rows and group headers
// have fixed heights, so the row table is a prefix sum computed once per
// list length, first element and day (the time groups move at midnight).

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// convRowGap is the gap between rows (the list's Column gap).
const convRowGap = 2

// convLayout is the row table of a Conversations list: rows are items and
// the group headers above them. Only the structure is kept; titles, keys
// and group labels are read from the items when a row is built.
type convLayout struct {
	key  convLayoutKey
	rows []int32   // row r: item i (>= 0), or ^i for the group header above item i
	at   []int32   // item i → its row
	offs []float64 // row tops, len rows+1 (the end of the last row, no gap after it)

	active, renaming convFind
	ids              fillIDs // item rows built by the last frame
}

type convLayoutKey struct {
	n                    int
	first                *Conversation
	day                  int
	tz                   *time.Location
	loc                  *Locale
	noGroups             bool
	itemH, headH, headH0 float32
}

// convFind remembers where a key was found.
type convFind struct {
	key   string
	n     int
	first *Conversation
	idx   int
}

// find returns the index of the item with key (-1: none or key ""),
// checking the last answer first.
func (f *convFind) find(items []Conversation, key string) int {
	if key == "" || len(items) == 0 {
		return -1
	}
	if f.idx >= 0 && f.idx < len(items) && items[f.idx].Key == key {
		return f.idx
	}
	if f.idx < 0 && f.key == key && f.n == len(items) && f.first == &items[0] {
		return -1 // missed before on the same data
	}
	f.key, f.n, f.first, f.idx = key, len(items), &items[0], -1
	for i := range items {
		if items[i].Key == key {
			f.idx = i
			break
		}
	}
	return f.idx
}

// groupOf is the group label of item i ("" none).
func (w *ConversationsW) groupOf(i int) string {
	it := &w.items[i]
	if it.Group != "" {
		return it.Group
	}
	return convGroup(w.loc, w.c.now, it.Time)
}

// layout returns the row table, rebuilt when its key changed.
func (w *ConversationsW) layout(th *theme.Theme, tk *theme.ChatToken) *convLayout {
	if w.st.lay == nil {
		w.st.lay = &convLayout{active: convFind{idx: -1}, renaming: convFind{idx: -1}}
	}
	L := w.st.lay
	now := w.c.now
	y, m, d := now.Date()
	// The header is a line of FontSizeSM text padded 12 above (0 for the
	// first row) and 4 below (header).
	lh := th.LineHeightFor(th.FontSizeSM)
	k := convLayoutKey{n: len(w.items), day: y*10000 + int(m)*100 + d, tz: now.Location(), loc: w.loc, noGroups: w.noGroups,
		itemH: tk.ConvItemHeight, headH: lh + 16, headH0: lh + 4}
	if len(w.items) > 0 {
		k.first = &w.items[0]
	}
	if k == L.key && L.offs != nil {
		return L
	}
	L.key = k
	L.rows, L.at = L.rows[:0], L.at[:0]
	group := ""
	for i := range w.items {
		if !w.noGroups {
			if g := w.groupOf(i); g != group && g != "" {
				group = g
				L.rows = append(L.rows, ^int32(i))
			}
		}
		L.at = append(L.at, int32(len(L.rows)))
		L.rows = append(L.rows, int32(i))
	}
	L.offs = L.offs[:0]
	top := 0.0
	for r, v := range L.rows {
		L.offs = append(L.offs, top)
		h := k.itemH
		if v < 0 {
			h = k.headH
			if v == ^int32(0) {
				h = k.headH0
			}
		}
		top += float64(h)
		if r < len(L.rows)-1 {
			top += convRowGap
		}
	}
	L.offs = append(L.offs, top)
	return L
}

// rowOf is the row of item i (-1 for -1).
func (L *convLayout) rowOf(i int) int {
	if i < 0 || i >= len(L.at) {
		return -1
	}
	return int(L.at[i])
}

// scroll applies a ScrollToKey request to sb.
func (w *ConversationsW) scroll(sb *ScrollBox, L *convLayout, view float64) {
	if !w.req.set {
		return
	}
	key, _ := w.req.key.(string)
	var f convFind
	f.idx = -1
	r := L.rowOf(f.find(w.items, key))
	if r < 0 {
		return
	}
	listScrollTo(w.c, sb, w.req, L.offs[r], float64(L.key.itemH), L.offs[len(L.offs)-1], view)
}

// buildView builds the list in a viewport of the given Height.
func (w *ConversationsW) buildView(th *theme.Theme, tk *theme.ChatToken) {
	c := w.c
	opts := []layout.Option{layout.H(w.height)}
	if !w.virtual {
		sb := Scroll(c, nil, opts...)
		sb.n.ln.Gap = convRowGap
		if w.req.set {
			w.scroll(sb, w.layout(th, tk), fillViewH(sb, w.height))
		}
		sb.Body(func() { w.buildAll(th, tk) })
		return
	}
	sb := fillViewport(c, "conversations-view", opts)
	sb.n.ln.Gap = convRowGap
	w.buildWindow(sb, th, tk)
}

func convNavKey(k input.KeyEvent) bool {
	switch k.Key {
	case input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd:
		return true
	}
	return false
}

// buildWindow builds the rows in view into sb, with the keyboard moving
// over every conversation as rovingStep does over the built ones.
func (w *ConversationsW) buildWindow(sb *ScrollBox, th *theme.Theme, tk *theme.ChatToken) {
	c, st := w.c, w.st
	L := w.layout(th, tk)
	n := len(w.items)
	renaming := ""
	if !w.noRename {
		renaming = st.renaming
	}
	enabled := func(i int) bool { it := &w.items[i]; return !it.Disabled && (renaming == "" || it.Key != renaming) }
	act := -1
	if w.active != nil {
		act = L.active.find(w.items, *w.active)
	}
	ren := L.renaming.find(w.items, renaming)
	// The Tab stop: the selected row when it takes focus, else the first
	// that does (rovingTabStop).
	tab := act
	if tab < 0 || !enabled(tab) {
		tab = -1
		for i := 0; i < n; i++ {
			if enabled(i) {
				tab = i
				break
			}
		}
	}
	// The focused row of the last frame and the arrow keys sent to it.
	focused := -1
	if c.focus != 0 {
		for k, id := range L.ids.ids {
			if id == c.focus {
				focused = L.ids.rows[k]
				break
			}
		}
	}
	to := -1
	if focused >= 0 {
		step := func(from, d int) int {
			for k := 1; k <= n; k++ {
				if j := ((from+d*k)%n + n) % n; enabled(j) {
					return j
				}
			}
			return -1
		}
		cur := focused
		for _, k := range msTakeKeys(c, c.focus, convNavKey) {
			t := -1
			switch k.Key {
			case input.KeyUp:
				t = step(cur, -1)
			case input.KeyDown:
				t = step(cur, 1)
			case input.KeyHome:
				t = step(-1, 1)
			case input.KeyEnd:
				t = step(n, -1)
			}
			if k.Key == input.KeyHome || k.Key == input.KeyEnd {
				to, cur = t, t
			} else if t >= 0 {
				to, cur = t, t
			}
		}
	}
	view := fillViewH(sb, w.height)
	w.scroll(sb, L, view)
	if r := L.rowOf(to); r >= 0 {
		fillReveal(c, sb, L.offs[r], float64(L.key.itemH), view)
	}
	win := fillWin{offs: L.offs}
	var nodes []*node
	var idxs []int
	L.ids.reset()
	keeps := [...]int{L.rowOf(act), L.rowOf(ren), L.rowOf(tab), L.rowOf(focused), L.rowOf(to)}
	win.buildKeeps(c, sb, len(L.rows), keeps[:], func(r int) *node {
		before := len(c.cur.kids)
		if v := L.rows[r]; v < 0 {
			i := int(^v)
			w.header(th, tk, w.groupOf(i), i)
		} else {
			i := int(v)
			it := w.items[i]
			Key(c, it.Key, func() {
				nd := w.item(th, tk, it)
				if nd == nil {
					return
				}
				nodes, idxs = append(nodes, nd), append(idxs, i)
				L.ids.add(i, nd.id)
				if c.a11yOn {
					nd.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = i+1, n }))
				}
			})
		}
		if len(c.cur.kids) > before {
			return c.cur.kids[before]
		}
		return nil
	})
	if c.a11yOn {
		w.b.n.fill(semFunc(func(s *a11y.Node) { s.RowCount = n }))
	}
	cur := -1
	for k, i := range idxs {
		if i == tab {
			cur = k
			break
		}
	}
	rovingTabStop(nodes, cur)
	if to >= 0 {
		if id := L.ids.at(to); id != 0 {
			navFocus(c, id)
		}
	}
	// As rovingStep: the focused row's other keys stop here.
	for _, nd := range nodes {
		if c.focus != 0 && nd.id == c.focus {
			c.KeysFor(c.focus)
			break
		}
	}
}
