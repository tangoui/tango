package ui

import (
	"strings"
	"time"
	"unicode"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type mtState struct {
	edit   editState
	pop    entPopup
	active int
	skip   int // trigger position dismissed with Escape / outside click (-1 none)
	init   bool
	remote remoteSearch // debounce of Remote calls
}

// MentionsW is a multi-line input that suggests options after a trigger
// character such as '@'.
type MentionsW struct {
	c           *Context
	box, ed     *node
	val         *string
	opts        []Option
	prefixes    []string
	remote      func(prefix, query string)
	debounce    time.Duration
	loading     bool
	st          *mtState
	rows        int
	placeholder string
	status      string
	size        ControlSize
	disabled    bool
	forced      bool
	changed     bool
	selected    string
	boxIA, edIA Interaction
	z           *lazy
	optIDs      []ID // suggestion nodes (ActiveDescendant)
	listID      ID
}

// Mentions adds a mention input bound to *val with candidate opts.
func Mentions(c *Context, val *string, opts []Option) *MentionsW {
	w := &MentionsW{c: c, val: val, opts: opts, prefixes: []string{"@"}, debounce: remoteDebounce, rows: 1, status: c.itemStatus, size: c.defaultSize()}
	w.box = c.add("mentions", "")
	w.st = State[mtState](c, hashID(w.box.id, "mt"), nil)
	if !w.st.init {
		w.st.skip, w.st.init = -1, true
	}
	w.ed = &node{id: hashID(w.box.id, "editor"), parent: w.box, disabled: w.box.disabled}
	w.ed.ln.Ref = w.ed
	w.ed.ln.Grow = 1
	w.ed.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: 200, H: 22} }
	w.box.kids = append(w.box.kids, w.ed)
	w.box.paint = w.paintBox
	w.ed.paint = w.paintText
	w.ed.ownKeys = true
	w.edIA = c.interact(w.ed, true, input.CursorText)
	w.boxIA = c.interact(w.box, false, input.CursorText)
	w.ed.setRole(a11y.RoleTextbox).setState(a11y.Multiline | a11y.HasPopup).fill(w)
	w.configDefaults()
	w.z = c.lazy(w.box, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *MentionsW) A11yLabel(s string) *MentionsW { w.ed.setName(s); return w }

func (w *MentionsW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = *w.val, w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	if w.status == "error" {
		s.States |= a11y.Invalid
	}
	if w.st.pop.open && w.listID != 0 {
		s.Controls = a11y.NodeID(w.listID)
		if a := w.st.active; a >= 0 && a < len(w.optIDs) {
			s.ActiveDescendant = a11y.NodeID(w.optIDs[a])
		}
	}
}

func (w *MentionsW) Placeholder(s string) *MentionsW      { w.placeholder = s; return w }
func (w *MentionsW) Status(s string) *MentionsW           { w.status = s; return w }
func (w *MentionsW) Size(s ControlSize) *MentionsW        { w.size = s; return w }
func (w *MentionsW) Rows(n int) *MentionsW                { w.rows = max(1, n); return w }
func (w *MentionsW) Width(px float32) *MentionsW          { w.box.ln.Width = px; return w }
func (w *MentionsW) Layout(o ...layout.Option) *MentionsW { w.box.ln.Style.Apply(o...); return w }

// Prefix sets the trigger characters (default "@").
func (w *MentionsW) Prefix(p ...string) *MentionsW { w.prefixes = p; return w }

// Remote turns on server-side suggestions: after an edit at an active
// mention, fn receives its trigger prefix and the text typed after it once
// Debounce (300ms by default) has passed, on the UI thread. Start the
// request in the background, pass Loading(true) while it runs, then
// replace the options given to Mentions (shown as-is, not filtered by the
// query) and wake the window with Invalidate.
//
//	ui.Mentions(c, &msg, users).Remote(func(p, q string) { go lookup(p, q) }).Loading(busy)
func (w *MentionsW) Remote(fn func(prefix, query string)) *MentionsW { w.remote = fn; return w }

// Debounce sets the delay before a Remote call.
func (w *MentionsW) Debounce(d time.Duration) *MentionsW { w.debounce = d; return w }

// Loading shows a spinner and the loading text in the suggestions (in
// place of the options with Remote, or when there are none).
func (w *MentionsW) Loading(on bool) *MentionsW { w.loading = on; return w }

func (w *MentionsW) Disabled(on bool) *MentionsW {
	if on {
		w.disabled = true
		w.box.disabled, w.ed.disabled, w.ed.focus = true, true, false
		w.box.cursor, w.ed.cursor = input.CursorNotAllowed, input.CursorNotAllowed
	}
	return w
}

// Open forces the suggestions open when a trigger is at the caret (tests,
// screenshots).
func (w *MentionsW) Open() *MentionsW {
	w.forced = true
	if w.c.focus != w.ed.id {
		n := len([]rune(*w.val))
		w.st.edit.cur, w.st.edit.anc = n, n
	}
	return w
}

// IsOpen reports whether suggestions are shown.
func (w *MentionsW) IsOpen() bool { w.z.run(); return w.st.pop.open }

// Changed reports an edit this frame.
func (w *MentionsW) Changed() bool { w.z.run(); return w.changed }

// Selected returns the option value inserted this frame ("" if none).
func (w *MentionsW) Selected() string { w.z.run(); return w.selected }

// Query returns the mention being typed at the caret: its trigger prefix
// and the text after it; ok is false when the caret is not in a mention.
// Read it after the build (or from a Remote callback) to drive
// suggestions yourself.
func (w *MentionsW) Query() (prefix, query string, ok bool) {
	w.z.run()
	pos, p, q := w.trigger()
	return p, q, pos >= 0
}

// trigger finds the active mention before the caret: the index of the
// prefix rune, the prefix and the typed query.
func (w *MentionsW) trigger() (int, string, string) {
	rs := []rune(*w.val)
	cur := clampIdx(w.st.edit.cur, len(rs))
	if w.st.edit.hasSel() {
		return -1, "", ""
	}
	for i := cur - 1; i >= 0; i-- {
		r := rs[i]
		for _, p := range w.prefixes {
			if string(r) == p && (i == 0 || unicode.IsSpace(rs[i-1])) {
				return i, p, string(rs[i+1 : cur])
			}
		}
		if unicode.IsSpace(r) {
			break
		}
	}
	return -1, "", ""
}

func (w *MentionsW) filtered(q string) []Option {
	if w.remote != nil {
		return w.opts // already filtered by the server
	}
	q = strings.ToLower(q)
	var out []Option
	for _, o := range w.opts {
		if q == "" || strings.Contains(strings.ToLower(o.Label), q) || strings.Contains(strings.ToLower(o.Value), q) {
			out = append(out, o)
		}
	}
	return out
}

func (w *MentionsW) font() text.Style { return w.c.style(w.c.controlFont(w.size), text.Regular) }

func (w *MentionsW) finish() {
	c, st := w.c, w.st
	lh := w.c.Theme.LineHeightFor(c.controlFont(w.size))
	pv := (c.controlHeight(w.size) - lh) / 2
	w.box.ln.Padding = geom.VH(pv, 11)
	if w.size == Small {
		w.box.ln.Padding = geom.VH(pv, 7)
	}
	w.box.ln.Height = float32(w.rows)*lh + 2*pv
	if w.disabled || w.box.disabled {
		st.pop.open = false
		return
	}
	if w.boxIA.PressedNow && !w.edIA.PressedNow {
		c.SetFocus(w.ed.id)
	}
	pos, _, q := w.trigger()
	opts := w.pickable(w.filtered(q))
	if st.pop.open {
		keys := msTakeKeys(c, w.ed.id, func(k input.KeyEvent) bool {
			return k.Key == input.KeyUp || k.Key == input.KeyDown || k.Key == input.KeyEnter
		})
		for _, k := range keys {
			switch k.Key {
			case input.KeyUp, input.KeyDown:
				st.active = msStep(opts, st.active, k.Key == input.KeyDown)
			case input.KeyEnter:
				if st.active >= 0 && st.active < len(opts) {
					w.insert(opts[st.active])
				}
			}
		}
	}
	res := c.processEdit(w.ed.id, &st.edit, w.val, editConfig{multiline: true}, w.edIA)
	prevPos := pos
	pos, _, q = w.trigger()
	if res.changed {
		w.changed = true
		if w.remote != nil && pos >= 0 {
			st.remote.schedule(c, w.debounce)
		}
	}
	st.remote.tick(c, func() {
		// The mention at the caret when the debounce ends (none: skip).
		if p, pf, pq := w.trigger(); p >= 0 && w.remote != nil {
			w.remote(pf, pq)
		}
	})
	if pos != prevPos {
		st.active = 0
	}
	if st.pop.open && c.Dismissed(st.pop.panel, w.box.id) {
		st.skip = pos
		c.invalid = true
	}
	if pos < 0 {
		st.skip = -1
	}
	all := w.filtered(q)
	busy := w.busy(all)
	opts = w.pickable(all)
	focused := c.focus == w.ed.id || (c.focus == 0 && st.pop.open)
	st.pop.open = pos >= 0 && pos != st.skip && (focused || w.forced) && (len(opts) > 0 || busy)
	if st.active >= len(opts) {
		st.active = 0
	}
	if st.pop.open {
		w.dropdown(pos, opts, busy)
	}
}

// busy reports whether the loading row replaces the suggestions.
func (w *MentionsW) busy(opts []Option) bool {
	return w.loading && (len(opts) == 0 || w.remote != nil)
}

// pickable returns the suggestions that can be chosen: none while the
// loading row replaces them.
func (w *MentionsW) pickable(opts []Option) []Option {
	if w.busy(opts) {
		return nil
	}
	return opts
}

func (w *MentionsW) insert(o Option) {
	pos, p, _ := w.trigger()
	if pos < 0 {
		return
	}
	rs := []rune(*w.val)
	cur := clampIdx(w.st.edit.cur, len(rs))
	ins := []rune(p + o.Value + " ")
	nr := append(append(append([]rune(nil), rs[:pos]...), ins...), rs[cur:]...)
	*w.val = string(nr)
	w.st.edit.cur = pos + len(ins)
	w.st.edit.anc = w.st.edit.cur
	w.st.pop.open = false
	w.changed = true
	w.selected = o.Value
	w.c.SetFocus(w.ed.id)
	w.c.invalid = true
}

// caretAt returns the absolute position below rune index i of the editor
// (from the last frame's geometry).
func (w *MentionsW) caretAt(i int) (geom.Point, bool) {
	r, ok := w.c.rects[w.ed.id]
	if !ok {
		return geom.Point{}, false
	}
	lay := text.Shape(*w.val, w.font(), text.Options{MaxWidth: r.W})
	x, top, h := lay.Caret(clampIdx(i, len([]rune(*w.val))))
	return geom.Pt(r.X-w.st.edit.scroll.X+x, r.Y-w.st.edit.scroll.Y+top+h), true
}

func (w *MentionsW) dropdown(pos int, opts []Option, busy bool) {
	c, st := w.c, w.st
	loc := c.Locale()
	th := c.Theme
	a := c.AnimFrom(w.box.id, "ent-open", 0, 1, th.DurationMid)
	var sx, sy float32
	if p, ok := w.caretAt(pos); ok {
		er := c.rects[w.ed.id]
		sx, sy = p.X-er.X, p.Y-er.Bottom()
		if c.IsRTL() {
			sx = p.X - er.Right() // BottomStart aligns the right edges in RTL
		}
		sy = max(sy, c.rects[w.box.id].Bottom()-er.Bottom())
	} else {
		c.animating = true // no geometry yet: ask for one more frame
	}
	ov := Overlay(c, OverlayOptions{Anchor: w.ed.id, Place: BottomStart, Offset: 4, ShiftX: sx, ShiftY: sy}, func() {
		if busy {
			entLoadingRow(c, loc.Loading, entOptionH(c))
			return
		}
		msOptionList(c, len(opts), func(i int) (string, string, bool) {
			return opts[i].Value, opts[i].Label, opts[i].Disabled
		}, &st.active, nil, &w.optIDs, func(i int, l *paint.List, r geom.Rect) {
			entPaintOption(c, l, r, opts[i].Label, entOptState{active: st.active == i, disabled: opts[i].Disabled})
		}, func(i int) { w.insert(opts[i]) })
	}, layout.MinW(120))
	entStylePanel(c, ov)
	ov.Style(layout.Padding(entListPad(c)))
	ov.Opacity(a)
	st.pop.panel = ov.n.id
	if !busy { // the loading row is a status, not a list
		ov.n.setRole(a11y.RoleListbox).setName(popupName(c, w.ed, w.placeholder))
		w.listID = ov.n.id
	}
}

func (w *MentionsW) paintBox(l *paint.List, r geom.Rect) {
	c := w.c
	dis := w.disabled || w.box.disabled
	c.paintFrame(l, r, geom.Rad(controlRadiusOf(c.ThemeFor(theme.CompInput), w.size)), w.box.id, frameState{
		hover: w.boxIA.Hovered || w.edIA.Hovered, focus: c.focus == w.ed.id, disabled: dis, status: w.status})
}

func (w *MentionsW) paintText(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.Theme
	col := th.Text
	if w.disabled || w.box.disabled {
		col = th.TextDisabled
	}
	l.Clip(r.Expand(1), geom.Radii{})
	c.paintEditor(l, r, w.ed.id, &w.st.edit, *w.val, w.font(), col, w.placeholder, false, true)
	l.Pop()
}
