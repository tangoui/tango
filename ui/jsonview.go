package ui

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

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

// ---------------------------------------------------------------------------
// JSONView: a lazy, virtualized value tree.

// JSONViewLocale holds the JSONView texts.
type JSONViewLocale struct {
	_ [0]func()

	Name                   string // accessible name of the tree
	CopyValue, CopyPath    string
	ExpandAll, CollapseAll string
	ItemsFmt               string // array size, arg: n ("%d 项")
	KeysFmt                string // object size, arg: n ("%d 个键")
	Search                 string // search box placeholder
	PrevMatch, NextMatch   string
	MatchFmt               string // current match and count, args: i, n
	NoMatch                string
	ParseErrorFmt          string // arg: the parser's message
}

// Items formats an array size.
func (l *JSONViewLocale) Items(n int) string { return locFmt(l.ItemsFmt, n) }

// Keys formats an object size.
func (l *JSONViewLocale) Keys(n int) string { return locFmt(l.KeysFmt, n) }

// Match formats the match position.
func (l *JSONViewLocale) Match(i, n int) string { return locFmt(l.MatchFmt, i, n) }

// ParseError formats a parse error.
func (l *JSONViewLocale) ParseError(msg string) string { return locFmt(l.ParseErrorFmt, msg) }

// jvGroup is the size of an expanded range: containers with more members
// show them in groups ("[0 … 99]"), nested by powers of it.
const jvGroup = 100

// jvSearchCap bounds the nodes one search visits.
const jvSearchCap = 200_000

// JSONViewW shows a value tree: a JSON document (JSONView), a Go value
// (JSONViewValue) or any ValueSource such as a debugger's variables
// (JSONViewSource). Nodes load when expanded, large arrays and objects
// open in groups of 100, and only the visible rows are built. Values are
// colored by type; containers show their size. A row's context menu (or
// Cmd/Ctrl+C, Cmd/Ctrl+Shift+C on the current row) copies its value or its
// path ($.a.b[0]); the toolbar searches keys and values and expands or
// collapses everything.
//
// Keyboard (the tree is one Tab stop): Up/Down/Page Up/Page Down/Home/End
// move the current row, Right expands or enters, Left collapses or goes to
// the parent, Enter toggles.
type JSONViewW struct {
	_ [0]func()

	c        *Context
	b        *Box
	src      ValueSource
	err      error
	key      any  // identity of the data (a new one reloads the tree)
	always   bool // the data has no identity: reload every frame
	version  int
	height   float32
	toolbar  bool
	types    bool
	depth    int
	queryP   *string
	label    string
	copied   string
	selected string
	st       *jvState
	sb       *ScrollBox
	tree     *Box
	first    int
	end      int
	focusVis bool
	z        *lazy
}

type jvNode struct {
	ValueNode
	key     string // expansion key: the path, or path#from:to for a group
	path    string
	depth   int
	parent  *jvNode // row parent (a group or a container)
	owner   *jvNode // groups: the container whose members they hold
	group   bool
	from    int
	to      int
	kids    []*jvNode
	loaded  bool
	loading bool
}

type jvMatch struct {
	chain []int32 // child indexes from the roots
	path  string
}

type jvWalk struct {
	node  ValueNode
	idx   int32 // index in the parent
	next  int   // next child to visit
	page  []ValueNode
	pageA int  // index of page[0]
	all   bool // page holds every child (roots, or a Count of -1)
}

type jvState struct {
	key      any
	ver      int
	src      ValueSource
	err      error
	roots    []*jvNode
	expanded map[string]bool
	rows     []*jvNode
	dirty    bool
	cursor   string
	reveal   bool
	pending  bool // some node is loading

	query   string
	matches []jvMatch
	stack   []jvWalk
	visited int
	started bool
	done    bool
	cur     int
	jumped  bool
}

// JSONView adds a viewer of the JSON document data (shown with an error
// message when it is not valid JSON). The same data slice (by address and
// length) keeps the tree's state; replace the slice to show new data.
//
//	ui.JSONView(c, body).Height(400)
func JSONView(c *Context, data []byte, opts ...layout.Option) *JSONViewW {
	w := jvNew(c, opts)
	key := jvBytesKey{n: len(data)}
	if len(data) > 0 {
		key.p = &data[0]
	}
	w.key = key
	st := w.state()
	if st.key != w.key || st.src == nil && st.err == nil {
		w.src, w.err = JSONSource(data)
	} else {
		w.src, w.err = st.src, st.err
	}
	return w
}

type jvBytesKey struct {
	p *byte
	n int
}

// JSONViewValue adds a viewer of a Go value (see GoValueSource). Pass a
// pointer, map or slice to keep the tree's state across frames, and bump
// DataVersion when its contents change.
func JSONViewValue(c *Context, v any, opts ...layout.Option) *JSONViewW {
	w := jvNew(c, opts)
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		w.key = [2]uintptr{rv.Pointer(), 0}
	case reflect.Map, reflect.Slice:
		w.key = [2]uintptr{rv.Pointer(), uintptr(rv.Len())}
	default:
		// A value without identity: reload every frame, keeping what is
		// expanded.
		w.key, w.always = rv.Type(), true
	}
	st := w.state()
	if w.always || st.key != w.key || st.src == nil {
		w.src = GoValueSource(v)
	} else {
		w.src = st.src
	}
	return w
}

// JSONViewSource adds a viewer of a value tree source (a debugger's
// variables, a remote document …). The same source value keeps the tree's
// state; bump DataVersion to reload it.
func JSONViewSource(c *Context, src ValueSource, opts ...layout.Option) *JSONViewW {
	w := jvNew(c, opts)
	w.src = src
	if src != nil && reflect.TypeOf(src).Comparable() {
		w.key = src
	} else {
		w.key, w.always = reflect.TypeOf(src), true
	}
	return w
}

func jvNew(c *Context, opts []layout.Option) *JSONViewW {
	w := &JSONViewW{c: c, toolbar: true, depth: 1}
	w.b = c.box("jsonview", layout.Column, opts)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

func (w *JSONViewW) state() *jvState {
	if w.st == nil {
		w.st = State(w.c, w.b.n.id, func() jvState { return jvState{expanded: map[string]bool{}, cur: -1} })
	}
	return w.st
}

// Height fixes the viewer's height (toolbar included); without it the
// viewer grows in its parent.
func (w *JSONViewW) Height(px float32) *JSONViewW { w.height = px; return w }

// NoToolbar hides the search / expand toolbar.
func (w *JSONViewW) NoToolbar() *JSONViewW { w.toolbar = false; return w }

// ShowTypes shows the nodes' Type labels.
func (w *JSONViewW) ShowTypes() *JSONViewW { w.types = true; return w }

// ExpandDepth sets how many levels open when data is first shown (default
// 1: the roots).
func (w *JSONViewW) ExpandDepth(n int) *JSONViewW { w.depth = max(0, n); return w }

// DataVersion reloads the tree when v changes (keeping what is expanded).
func (w *JSONViewW) DataVersion(v int) *JSONViewW { w.version = v; return w }

// Query binds the search text to *q.
func (w *JSONViewW) Query(q *string) *JSONViewW { w.queryP = q; return w }

// A11yLabel sets the accessible name of the tree.
func (w *JSONViewW) A11yLabel(s string) *JSONViewW { w.label = s; return w }

// Layout applies layout options to the viewer.
func (w *JSONViewW) Layout(opts ...layout.Option) *JSONViewW {
	flexbox.Apply(&w.b.n.ln.Style, opts...)
	return w
}

// Copied returns the text copied this frame ("" if none).
func (w *JSONViewW) Copied() string { w.z.run(); return w.copied }

// Current returns the path of the current row ("" if none).
func (w *JSONViewW) Current() string { w.z.run(); return w.selected }

// Rows returns the number of visible rows (expanded nodes included).
func (w *JSONViewW) Rows() int { w.z.run(); return len(w.st.rows) }

// Matches returns the number of search matches found so far.
func (w *JSONViewW) Matches() int { w.z.run(); return len(w.st.matches) }

// ID returns the viewer's ID.
func (w *JSONViewW) ID() ID { return w.b.n.id }

// reset loads a new tree, keeping the expanded keys when only the version
// changed.
func (w *JSONViewW) reset() {
	st := w.st
	keep := st.key == w.key
	st.key, st.ver, st.src, st.err = w.key, w.version, w.src, w.err
	st.roots, st.rows, st.dirty = nil, nil, true
	st.matches, st.stack, st.started, st.done, st.cur, st.visited = nil, nil, false, false, -1, 0
	if !keep {
		st.expanded = map[string]bool{}
		st.cursor = ""
	}
	if w.src == nil {
		return
	}
	for i, r := range w.src.Roots() {
		n := &jvNode{ValueNode: r, depth: 0}
		n.path = r.Name
		if n.path == "" {
			n.path = "$"
		}
		if len(st.roots) > 0 && r.Name == "" {
			n.path = "$" + strconv.Itoa(i)
		}
		n.key = n.path
		st.roots = append(st.roots, n)
	}
	if !keep {
		budget := 2000
		var open func(ns []*jvNode)
		open = func(ns []*jvNode) {
			for _, n := range ns {
				if budget <= 0 || n.depth >= w.depth || !jvExpandable(n) {
					continue
				}
				budget--
				st.expanded[n.key] = true
				if w.load(n) && !n.group {
					open(n.kids)
				}
			}
		}
		open(st.roots)
	}
}

func jvExpandable(n *jvNode) bool {
	return n.group || ((n.Kind == ValueObject || n.Kind == ValueArray) && n.Count != 0)
}

// jvBucket returns the group size for n members.
func jvBucket(n int) int {
	size := jvGroup
	for (n+size-1)/size > jvGroup {
		size *= jvGroup
	}
	return size
}

// load fills n.kids (groups for large ranges); false while loading.
func (w *JSONViewW) load(n *jvNode) bool {
	if n.loaded {
		return true
	}
	owner, from, to := n, 0, n.Count
	if n.group {
		owner, from, to = n.owner, n.from, n.to
	}
	if to-from > jvGroup && (n.group || n.Count > jvGroup) {
		size := jvBucket(to - from)
		if size >= to-from {
			size = jvGroup
		}
		for a := from; a < to; a += size {
			b := min(to, a+size)
			g := &jvNode{group: true, from: a, to: b, owner: owner, parent: n, depth: n.depth + 1}
			g.path = owner.path
			g.key = owner.path + "#" + strconv.Itoa(a) + ":" + strconv.Itoa(b)
			n.kids = append(n.kids, g)
		}
		n.loaded = true
		return true
	}
	if to < 0 {
		to = -1
	}
	kids, ok := w.src.Children(&owner.ValueNode, from, to)
	if !ok {
		n.loading = true
		w.st.pending = true
		return false
	}
	n.loading = false
	for _, k := range kids {
		kn := &jvNode{ValueNode: k, parent: n, depth: n.depth + 1}
		if k.Index {
			kn.path = owner.path + "[" + k.Name + "]"
		} else {
			kn.path = owner.path + jsonPathKey(k.Name)
		}
		kn.key = kn.path
		n.kids = append(n.kids, kn)
	}
	n.loaded = true
	return true
}

// flatten rebuilds the visible rows.
func (w *JSONViewW) flatten() {
	st := w.st
	st.rows = st.rows[:0]
	st.pending = false
	var walk func(ns []*jvNode)
	walk = func(ns []*jvNode) {
		for _, n := range ns {
			st.rows = append(st.rows, n)
			if st.expanded[n.key] && jvExpandable(n) && w.load(n) {
				walk(n.kids)
			}
		}
	}
	walk(st.roots)
	st.dirty = st.pending
}

func (w *JSONViewW) rowOf(key string) int {
	for i, n := range w.st.rows {
		if n.key == key {
			return i
		}
	}
	return -1
}

func (w *JSONViewW) setExpanded(n *jvNode, on bool) {
	st := w.st
	if on {
		st.expanded[n.key] = true
	} else {
		delete(st.expanded, n.key)
	}
	st.dirty = true
	w.c.invalid = true
}

func (w *JSONViewW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompJSONView)
	tk := &th.Components.JSONView
	loc := &c.Locale().JSONView
	st := w.state()
	if w.always || st.key != w.key || st.ver != w.version || (st.src == nil && st.err == nil && (w.src != nil || w.err != nil)) {
		w.reset()
	}
	if w.height > 0 {
		w.b.n.ln.Height = w.height
	} else if w.b.n.ln.Grow == 0 {
		w.b.n.ln.Grow = 1
	}
	w.b.n.ln.MinH = 80
	if w.err != nil {
		Alert(c, "error", loc.ParseError(w.err.Error())).Show()
		return
	}
	if w.toolbar {
		w.toolbarRow(loc)
	}
	q := st.query
	if w.queryP != nil {
		q = *w.queryP
	}
	if q != st.query {
		st.query = q
		st.matches, st.stack, st.started, st.done, st.cur, st.visited, st.jumped = nil, nil, false, false, -1, 0, false
	}
	w.search()
	if st.cur >= 0 && st.cur < len(st.matches) && !st.jumped {
		st.jumped = true
		w.revealChain(st.matches[st.cur].chain)
	}
	if st.dirty || st.rows == nil {
		w.flatten()
	}
	if st.pending {
		c.WakeAt(c.now.Add(50 * time.Millisecond)) // poll the loading nodes
	}
	w.tree = c.box("jsonview-tree", layout.Column, []layout.Option{layout.Grow(1), layout.MinH(0)})
	ia := c.interact(w.tree.n, true, input.CursorDefault)
	name := w.label
	if name == "" {
		name = loc.Name
	}
	fillSem(c, w.tree.n, a11y.RoleTree, name)
	w.tree.n.ownKeys = true
	w.focusVis = ia.FocusVisible
	if keys := c.KeysFor(w.tree.n.id); len(keys) > 0 {
		w.handleKeys(keys, tk)
	}
	if st.dirty {
		w.flatten()
	}
	w.tree.Body(func() { w.rowsView(tk, loc) })
	if i := w.rowOf(st.cursor); i >= 0 {
		w.selected = st.rows[i].path
	}
}

func (w *JSONViewW) rowsView(tk *theme.JSONViewToken, loc *JSONViewLocale) {
	c, st := w.c, w.st
	rh := float64(tk.RowHeight)
	total := float64(len(st.rows))*rh + 8
	w.sb = fillViewport(c, "jsonview-list", []layout.Option{layout.Grow(1), layout.MinH(0)})
	w.sb.s.extent.Y = total
	view := fillViewH(w.sb, 200)
	if st.reveal {
		if i := w.rowOf(st.cursor); i >= 0 {
			fillReveal(c, w.sb, float64(i)*rh+4, rh, view)
		}
		st.reveal = false
	}
	fillClampOff(w.sb, total, view)
	w.first, w.end = fillSpan(w.sb.s.off.Y-4, view, rh, len(st.rows), 3)
	w.sb.s.base.Y = float64(w.first) * rh
	w.sb.Body(func() {
		Div(c, layout.H(4))
		for r := w.first; r < w.end; r++ {
			w.row(st.rows[r], tk, loc)
		}
	})
}

func (w *JSONViewW) handleKeys(keys []input.KeyEvent, tk *theme.JSONViewToken) {
	c, st := w.c, w.st
	if st.dirty {
		w.flatten()
	}
	rows := st.rows
	if len(rows) == 0 {
		return
	}
	idx := w.rowOf(st.cursor)
	page := 10
	if w.sb != nil && w.sb.s.view.H > 0 {
		page = max(1, int(w.sb.s.view.H/tk.RowHeight)-1)
	}
	for _, k := range keys {
		if !k.Down {
			continue
		}
		if idx < 0 {
			idx = 0
			if k.Key == input.KeyUp || k.Key == input.KeyDown {
				continue
			}
		}
		n := rows[idx]
		if k.Mods.Shortcut() && k.Key == input.KeyC {
			if k.Mods&input.ModShift != 0 {
				w.copy(n.path)
			} else {
				w.copy(w.valueText(n))
			}
			continue
		}
		switch treeArrow(c, k.Key) {
		case input.KeyDown:
			idx = min(idx+1, len(rows)-1)
		case input.KeyUp:
			idx = max(idx-1, 0)
		case input.KeyPageDown:
			idx = min(idx+page, len(rows)-1)
		case input.KeyPageUp:
			idx = max(idx-page, 0)
		case input.KeyHome:
			idx = 0
		case input.KeyEnd:
			idx = len(rows) - 1
		case input.KeyRight:
			if jvExpandable(n) && !st.expanded[n.key] {
				w.setExpanded(n, true)
			} else if st.expanded[n.key] && len(n.kids) > 0 {
				idx = min(idx+1, len(rows)-1)
			}
		case input.KeyLeft:
			if st.expanded[n.key] && jvExpandable(n) {
				w.setExpanded(n, false)
			} else if n.parent != nil {
				if i := w.rowOf(n.parent.key); i >= 0 {
					idx = i
				}
			}
		case input.KeyEnter, input.KeySpace:
			if jvExpandable(n) {
				w.setExpanded(n, !st.expanded[n.key])
			}
		default:
			continue
		}
		if st.dirty {
			w.flatten()
			rows = st.rows
		}
		idx = max(0, min(idx, len(rows)-1))
	}
	if idx >= 0 && idx < len(rows) {
		st.cursor = rows[idx].key
		st.reveal = true
		c.invalid = true
	}
}

// valueText is the text "copy value" copies.
func (w *JSONViewW) valueText(n *jvNode) string {
	if n.group {
		return ""
	}
	if cp, ok := w.src.(ValueCopier); ok {
		return cp.CopyValue(&n.ValueNode)
	}
	return n.Value
}

func (w *JSONViewW) copy(s string) {
	w.copied = s
	if w.c.win != nil {
		w.c.win.WriteClipboard(s)
	}
	w.c.invalid = true
}

// row builds one row: a node with the switcher, key and value painted.
func (w *JSONViewW) row(n *jvNode, tk *theme.JSONViewToken, loc *JSONViewLocale) {
	c, st := w.c, w.st
	th := c.Theme
	Key(c, n.key, func() {
		open := st.expanded[n.key]
		exp := jvExpandable(n)
		cur := st.cursor == n.key
		cursorRow := cur && w.focusVis
		key, value, meta := w.texts(n, loc)
		label := key
		if value != "" {
			label = strings.TrimSpace(key + " " + value)
		}
		rn := c.add("jsonview-row", label)
		rn.ln.Height = tk.RowHeight
		ia := c.interact(rn, false, input.CursorDefault)
		fillSem(c, rn, a11y.RoleTreeItem, label, a11y.Selectable|fillIf(cur, a11y.Selected)|
			fillIf(exp, a11y.Expandable)|fillIf(exp && open, a11y.Expanded)|fillIf(n.loading, a11y.Busy))
		if c.a11yOn {
			lv := n.depth + 1
			rn.fill(semFunc(func(s *a11y.Node) { s.Level = lv }))
		}
		fillKeyboard(c, rn)
		indent := float32(n.depth)*tk.Indent + 6
		sw := float32(16)
		q := st.query
		var curMatch bool
		if st.cur >= 0 && st.cur < len(st.matches) {
			curMatch = st.matches[st.cur].path == n.path && !n.group
		}
		rot := c.Anim(rn.id, "rot", b2f(open), th.DurationMid)
		loading := n.loading
		mono := text.Style{Size: tk.ValueFontSize, LineHeight: tk.RowHeight, Mono: true}
		keyCol := tk.KeyColor
		if n.Index || n.group {
			keyCol = tk.MetaColor
		}
		valCol := w.valueColor(n, tk)
		rn.paint = func(l *paint.List, r geom.Rect) {
			s := c.inline(r)
			switch {
			case cur:
				l.Fill(r, geom.Rad(tk.Radius), tk.RowSelectedBg)
			case ia.Hovered:
				l.Fill(r, geom.Rad(tk.Radius), tk.RowHoverBg)
			}
			if cursorRow {
				l.Stroke(r, geom.Rad(tk.Radius), 1, th.Primary.Border)
			}
			cy := r.Y + r.H/2
			if exp {
				box := s.Rect(indent, cy-sw/2, sw, sw)
				if loading {
					drawSpinner(c, l, box.Expand(-2))
				} else {
					c.paintIcon(l, icon.Right, box.Expand(-3), tk.SwitcherColor, float64(rot)*math.Pi/2)
				}
			}
			x := indent + sw + 4
			put := func(str string, col theme.Color) {
				if str == "" {
					return
				}
				lay := text.Shape(str, mono, text.Options{NoWrap: true})
				pr := s.Rect(x, r.Y, lay.Width, r.H)
				if q != "" {
					if i := logIndexFold(str, q); i >= 0 {
						a, _, _ := lay.Caret(len([]rune(str[:i])))
						b, _, _ := lay.Caret(len([]rune(str[:i+logFoldLen(str[i:], q)])))
						mr := geom.Rect{X: pr.X + a, Y: r.Y + 3, W: b - a, H: r.H - 6}
						if curMatch {
							l.Fill(mr, geom.Rad(2), tk.MatchCurrentBg)
							l.Stroke(mr, geom.Rad(2), 1, tk.MatchCurrentBorder)
						} else {
							l.Fill(mr, geom.Rad(2), tk.MatchBg)
						}
					}
				}
				l.Glyphs(geom.Pt(pr.X, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
				adv, _, _ := lay.Caret(len(lay.Runes)) // trailing spaces included
				x += adv
			}
			if key != "" {
				put(key, keyCol)
				if !n.group {
					put(": ", tk.PunctColor)
				}
			}
			put(value, valCol)
			if meta != "" {
				x += 8
				put(meta, tk.MetaColor)
			}
		}
		switch {
		case ia.Clicked:
			st.cursor = n.key
			c.SetFocus(w.tree.n.id)
			onSwitch := false
			if exp {
				p := c.pointer
				if rr, ok := c.rects[rn.id]; ok {
					sx := c.inline(rr).Rect(indent, rr.Y, sw+4, rr.H)
					onSwitch = sx.Contains(p)
				}
			}
			if exp && (onSwitch || ia.Clicks >= 2) {
				w.setExpanded(n, !open)
			}
			c.invalid = true
		}
		if p, ok := c.ContextMenuFor(rn.id); ok && !n.group {
			st.cursor = n.key
			items := []MenuItem{{Key: "value", Label: loc.CopyValue, Icon: icon.Copy}, {Key: "path", Label: loc.CopyPath}}
			nn := n
			ContextMenuAt(c, w.tree.n.id, p, items, func(k string) {
				switch k {
				case "value":
					w.copy(w.valueText(nn))
				case "path":
					w.copy(nn.path)
				}
			})
		}
	})
}

// texts returns the key, value and meta texts of a row.
func (w *JSONViewW) texts(n *jvNode, loc *JSONViewLocale) (key, value, meta string) {
	if n.group {
		return "[" + strconv.Itoa(n.from) + " … " + strconv.Itoa(n.to-1) + "]", "", ""
	}
	key = n.Name
	if n.Index {
		key = "[" + n.Name + "]"
	}
	switch n.Kind {
	case ValueString:
		value = strconv.Quote(n.Value)
		if len(value) > 2 && !strings.ContainsAny(n.Value, "\"\\\n\t\r") {
			value = "\"" + n.Value + "\"" // keep non-ASCII readable
		}
	case ValueObject:
		value = "{…}"
		if n.Value != "" {
			value = n.Value
		}
		if n.Count >= 0 {
			meta = loc.Keys(n.Count)
		}
	case ValueArray:
		value = "[…]"
		if n.Value != "" {
			value = n.Value
		}
		if n.Count >= 0 {
			meta = loc.Items(n.Count)
		}
	default:
		value = n.Value
	}
	if w.types && n.Type != "" {
		meta = strings.TrimSpace(n.Type + "  " + meta)
	}
	return key, value, meta
}

func (w *JSONViewW) valueColor(n *jvNode, tk *theme.JSONViewToken) theme.Color {
	switch n.Kind {
	case ValueString:
		return tk.StringColor
	case ValueNumber:
		return tk.NumberColor
	case ValueBool:
		return tk.BoolColor
	case ValueNull:
		return tk.NullColor
	case ValueOther:
		return tk.OtherColor
	}
	return tk.PunctColor
}

func (w *JSONViewW) toolbarRow(loc *JSONViewLocale) {
	c, st := w.c, w.st
	th := c.Theme
	Row(c, func() {
		q := &st.query
		if w.queryP != nil {
			q = w.queryP
		}
		in := Input(c, q).Placeholder(loc.Search).Prefix(icon.Search).AllowClear().Size(Small).Width(220).A11yLabel(loc.Search)
		in.Changed()
		if in.Submitted() {
			w.step(1)
		}
		if *q != "" {
			label := loc.NoMatch
			if n := len(st.matches); n > 0 {
				label = loc.Match(st.cur+1, n)
			}
			Text(c, label).Size(th.FontSizeSM).Color(th.TextSecondary)
			if IconButton(c, icon.Up, loc.PrevMatch).Size(24, 14).Clicked() {
				w.step(-1)
			}
			if IconButton(c, icon.Down, loc.NextMatch).Size(24, 14).Clicked() {
				w.step(1)
			}
		}
		Spacer(c)
		Tooltip(c, loc.ExpandAll, func() {
			if IconButton(c, icon.Plus, loc.ExpandAll).Size(24, 14).Clicked() {
				w.expandAll()
			}
		})
		Tooltip(c, loc.CollapseAll, func() {
			if IconButton(c, icon.Minus, loc.CollapseAll).Size(24, 14).Clicked() {
				st.expanded = map[string]bool{}
				st.dirty = true
				c.invalid = true
			}
		})
	}, layout.Middle(), layout.Gap(8), layout.PadVH(6, 8)).PaintOver(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
	})
}

// expandAll opens loaded and loadable nodes breadth first, up to 5000
// nodes (large documents are not opened completely).
func (w *JSONViewW) expandAll() {
	st := w.st
	budget := 5000
	queue := append([]*jvNode(nil), st.roots...)
	for len(queue) > 0 && budget > 0 {
		n := queue[0]
		queue = queue[1:]
		if !jvExpandable(n) {
			continue
		}
		st.expanded[n.key] = true
		budget--
		if w.load(n) {
			queue = append(queue, n.kids...)
		}
	}
	st.dirty = true
	w.c.invalid = true
}

func (w *JSONViewW) step(d int) {
	st := w.st
	if len(st.matches) == 0 {
		return
	}
	st.cur = ((max(st.cur, 0)+d)%len(st.matches) + len(st.matches)) % len(st.matches)
	st.jumped = false
	w.c.invalid = true
}

// search walks the source depth first within a time budget, recording
// nodes whose key or value contains the query.
func (w *JSONViewW) search() {
	c, st := w.c, w.st
	if st.query == "" || st.done || w.src == nil {
		return
	}
	q := st.query
	if !st.started {
		st.started = true
		roots := w.src.Roots()
		st.stack = []jvWalk{{node: ValueNode{Kind: ValueArray, Count: len(roots)}, page: roots, all: true, idx: -1}}
	}
	loc := &c.Locale().JSONView
	deadline := time.Now().Add(6 * time.Millisecond)
	chain := func() []int32 {
		out := make([]int32, 0, len(st.stack))
		for _, f := range st.stack[1:] {
			out = append(out, f.idx)
		}
		return out
	}
	paths := func() string {
		var b strings.Builder
		for k, f := range st.stack[1:] {
			if k == 0 {
				if f.node.Name == "" {
					b.WriteString("$")
				} else {
					b.WriteString(f.node.Name)
				}
				continue
			}
			if f.node.Index {
				b.WriteString("[" + f.node.Name + "]")
			} else {
				b.WriteString(jsonPathKey(f.node.Name))
			}
		}
		return b.String()
	}
	for n := 0; len(st.stack) > 0; n++ {
		if n&255 == 255 && time.Now().After(deadline) {
			c.WakeAt(c.now.Add(time.Millisecond))
			return
		}
		top := &st.stack[len(st.stack)-1]
		if top.next >= top.pageA+len(top.page) {
			if top.all || (top.node.Count >= 0 && top.next >= top.node.Count) {
				st.stack = st.stack[:len(st.stack)-1]
				continue
			}
			to := top.next + jvGroup
			if top.node.Count < 0 {
				to = -1
			}
			kids, ok := w.src.Children(&top.node, top.next, to)
			if !ok {
				c.WakeAt(c.now.Add(50 * time.Millisecond))
				return
			}
			if len(kids) == 0 {
				st.stack = st.stack[:len(st.stack)-1]
				continue
			}
			top.page, top.pageA, top.all = kids, top.next, top.node.Count < 0
		}
		child := top.page[top.next-top.pageA]
		idx := int32(top.next)
		top.next++
		st.visited++
		st.stack = append(st.stack, jvWalk{node: child, idx: idx})
		_, v, _ := w.texts(&jvNode{ValueNode: child}, loc)
		if (child.Name != "" && logIndexFold(child.Name, q) >= 0) || (child.Kind != ValueObject && child.Kind != ValueArray && logIndexFold(v, q) >= 0) {
			st.matches = append(st.matches, jvMatch{chain: chain(), path: paths()})
			if st.cur < 0 {
				st.cur, st.jumped = 0, false
			}
		}
		if !(child.Kind == ValueObject || child.Kind == ValueArray) || child.Count == 0 || st.visited >= jvSearchCap {
			st.stack = st.stack[:len(st.stack)-1]
		}
	}
	st.done = true
}

// revealChain expands the ancestors of the node at chain and makes it the
// current row.
func (w *JSONViewW) revealChain(chain []int32) {
	st := w.st
	if len(chain) == 0 || int(chain[0]) >= len(st.roots) {
		return
	}
	n := st.roots[chain[0]]
	for _, ci := range chain[1:] {
		idx := int(ci)
		st.expanded[n.key] = true
		if !w.load(n) {
			break
		}
		cur := n
		for len(cur.kids) > 0 && cur.kids[0].group {
			var g *jvNode
			for _, k := range cur.kids {
				if idx >= k.from && idx < k.to {
					g = k
					break
				}
			}
			if g == nil {
				return
			}
			st.expanded[g.key] = true
			if !w.load(g) {
				return
			}
			cur = g
		}
		base := 0
		if cur.group {
			base = cur.from
		}
		if idx-base < 0 || idx-base >= len(cur.kids) {
			return
		}
		n = cur.kids[idx-base]
	}
	st.cursor = n.key
	st.reveal = true
	st.dirty = true
	w.c.invalid = true
}
