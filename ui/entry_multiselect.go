package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// msTakeKeys removes the key-down events of focused id accepted by want
// from the frame's queue and returns them; other keys stay for editors.
func msTakeKeys(c *Context, id ID, want func(k input.KeyEvent) bool) []input.KeyEvent {
	if c.focus != id {
		return nil
	}
	var out []input.KeyEvent
	rest := c.keys[:0:0]
	for _, k := range c.keys {
		if k.Down && want(k) {
			out = append(out, k)
		} else {
			rest = append(rest, k)
		}
	}
	c.keys = rest
	return out
}

// ---------------------------------------------------------------------------
// MultiSelect

type msState struct {
	pop    entPopup
	active int
}

// MultiSelectW is a select with multiple values shown as closable tags.
type MultiSelectW struct {
	_ [0]func()

	c       *Context
	box     *selector
	val     *[]string
	opts    []Option
	st      *msState
	changed bool
	forced  bool
	z       *lazy
	optIDs  []ID // option nodes of the open list

	rx      p2bRich         // groups, remote, create, render, limit, virtual (p2b_select.go)
	eng     *VirtualSelectW // dropdown engine while rx is in use
	created string
	isNew   bool
}

// MultiSelect adds a multiple-choice select bound to *val (selected values).
func MultiSelect(c *Context, val *[]string, opts []Option) *MultiSelectW {
	w := &MultiSelectW{c: c, val: val, opts: opts}
	w.box = newSelector(c, "multiselect")
	w.st = State[msState](c, hashID(w.box.n.id, "ms"), nil)
	w.rx.defaults(c)
	p2bSelectDefaults(c, w.box, nil, nil, nil)
	w.z = c.lazy(w.box.n, w.finish)
	return w
}

func (w *MultiSelectW) Placeholder(s string) *MultiSelectW { w.box.placeholder = s; return w }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *MultiSelectW) A11yLabel(s string) *MultiSelectW { w.box.n.setName(s); return w }

// Searchable filters options by typing into the selector.
func (w *MultiSelectW) Searchable() *MultiSelectW { w.box.search = true; return w }

// AllowClear shows a clear-all button on hover.
func (w *MultiSelectW) AllowClear() *MultiSelectW        { w.box.allowClear = true; return w }
func (w *MultiSelectW) Status(s string) *MultiSelectW    { w.box.status = s; return w }
func (w *MultiSelectW) Size(s ControlSize) *MultiSelectW { w.box.size = s; return w }
func (w *MultiSelectW) Width(px float32) *MultiSelectW   { w.box.n.ln.Width = px; return w }
func (w *MultiSelectW) Layout(o ...layout.Option) *MultiSelectW {
	flexbox.Apply(&w.box.n.ln.Style, o...)
	return w
}

// MaxTagCount collapses tags after n into a "+ N ..." tag.
func (w *MultiSelectW) MaxTagCount(n int) *MultiSelectW { w.box.maxCount = n; return w }

func (w *MultiSelectW) Disabled(on bool) *MultiSelectW {
	if on {
		w.box.disabled = true
	}
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *MultiSelectW) Open() *MultiSelectW { w.st.pop.open, w.forced = true, true; return w }

// IsOpen reports whether the dropdown is shown.
func (w *MultiSelectW) IsOpen() bool { w.z.run(); return w.st.pop.open }

// Changed reports a selection change this frame.
func (w *MultiSelectW) Changed() bool { w.z.run(); return w.changed }

func (w *MultiSelectW) labelOf(v string) string {
	if w.eng != nil {
		return w.eng.labelOf(v)
	}
	for _, o := range w.opts {
		if o.Value == v {
			return o.Label
		}
	}
	return v
}

func (w *MultiSelectW) filtered() []Option {
	return filterOptions(w.opts, w.box.st.query, w.rx.filter)
}

func (w *MultiSelectW) toggle(v string) {
	if contains(*w.val, v) {
		*w.val = msWithout(*w.val, v)
	} else {
		*w.val = append(append([]string(nil), *w.val...), v)
	}
	w.changed = true
	w.c.invalid = true
}

// msWithout returns a copy of s without v.
func msWithout(s []string, v string) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func (w *MultiSelectW) finish() {
	if w.rx.on(w.opts) {
		w.finishRich()
		return
	}
	c, b, st := w.c, w.box, w.st
	labels := make([]string, len(*w.val))
	for i, v := range *w.val {
		labels[i] = w.labelOf(v)
	}
	b.open = st.pop.open
	b.build(labels)
	if b.disabled {
		st.pop.open = false
		b.open = false
		return
	}
	if b.removed >= 0 && b.removed < len(*w.val) {
		*w.val = msWithout(*w.val, (*w.val)[b.removed])
		w.changed = true
	} else if b.clearClicked() {
		*w.val = nil
		b.st.query = ""
		w.changed = true
		st.pop.open = false
	} else if b.clicked() {
		if !st.pop.open || !b.search {
			st.pop.open = !st.pop.open
		}
		st.active = 0
		c.SetFocus(b.ed.id)
		c.invalid = true
	}
	if !w.forced {
		st.pop.dismiss(c, b.n.id)
	}
	if st.pop.open && c.focus == 0 { // presses inside the panel keep focus
		c.SetFocus(b.ed.id)
	}
	opts := w.filtered()
	keys := b.takeKeys(func(k input.KeyEvent) bool {
		switch k.Key {
		case input.KeyUp, input.KeyDown, input.KeyEnter:
			return true
		case input.KeySpace:
			return !b.search
		case input.KeyBackspace:
			return b.st.query == ""
		}
		return false
	})
	for _, k := range keys {
		switch k.Key {
		case input.KeyDown, input.KeyUp:
			if !st.pop.open {
				st.pop.open = true
				st.active = 0
				continue
			}
			st.active = msStep(opts, st.active, k.Key == input.KeyDown)
		case input.KeyEnter, input.KeySpace:
			if !st.pop.open {
				st.pop.open = true
				st.active = 0
			} else if st.active >= 0 && st.active < len(opts) && !opts[st.active].Disabled {
				w.toggle(opts[st.active].Value)
				b.st.query = ""
			}
		case input.KeyBackspace:
			if n := len(*w.val); n > 0 {
				*w.val = append([]string(nil), (*w.val)[:n-1]...)
				w.changed = true
			}
		}
	}
	if b.edit().changed {
		st.pop.open = true
		st.active = 0
	}
	if !b.focused() && !st.pop.open {
		b.st.query = ""
	}
	b.open = st.pop.open
	if st.pop.open {
		w.dropdown(w.filtered())
	}
}

// msStep moves an option index to the next enabled option.
func msStep(opts []Option, i int, down bool) int {
	n := len(opts)
	if n == 0 {
		return 0
	}
	for range n {
		if down {
			i = (i + 1 + n) % n
		} else {
			i = (i - 1 + n) % n
		}
		if !opts[i].Disabled {
			return i
		}
	}
	return i
}

// msOptionList renders an option list (scrolling when long) into the current
// overlay. paintRow draws row i; onClick handles a click. Rows are options
// (sel reports the selected ones, nil: none); their IDs are appended to
// *ids (nil: not collected) for the trigger's ActiveDescendant. The caller
// marks the overlay box as the listbox.
func msOptionList(c *Context, n int, labels func(i int) (key, label string, disabled bool), active *int, sel func(i int) bool, ids *[]ID, row func(i int, l *paint.List, r geom.Rect), onClick func(i int)) {
	th := c.Theme
	if ids != nil {
		*ids = (*ids)[:0]
	}
	build := func() {
		for i := range n {
			key, label, dis := labels(i)
			Key(c, key, func() {
				it := c.add("ms-option", label)
				it.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(sel != nil && sel(i), a11y.Selected)
				if ids != nil {
					*ids = append(*ids, it.id)
				}
				it.aKbd = true // the trigger's arrow keys move the highlight
				it.ln.Height = entOptionH(c)
				it.ln.Measure = func(_, _ float32) geom.Size {
					return geom.Size{W: entTextW(c, label, th.FontSize, text.Semibold) + 2*20 + 18, H: entOptionH(c)}
				}
				cur := input.CursorPointer
				if dis {
					it.disabled = true
					cur = input.CursorNotAllowed
				}
				ia := c.interact(it, false, cur)
				if ia.Hovered && !dis {
					*active = i
				}
				if ia.Clicked && !dis {
					onClick(i)
				}
				it.paint = func(l *paint.List, r geom.Rect) { row(i, l, r) }
			})
		}
	}
	if n > 8 {
		idx := *active
		sb := Scroll(c, build, layout.MaxH(entOptionH(c)*8))
		followRow(c, sb, idx, *active, 0, entOptionH(c)) // keep the keyboard row in view
	} else {
		build()
	}
}

func (w *MultiSelectW) dropdown(opts []Option) {
	c, st := w.c, w.st
	ov := entPanel(c, w.box.n.id, &st.pop, true, func() {
		if len(opts) == 0 {
			Empty(c).Simple().Description(c.Locale().NoData)
			return
		}
		msOptionList(c, len(opts), func(i int) (string, string, bool) {
			return opts[i].Value, opts[i].Label, opts[i].Disabled
		}, &st.active, func(i int) bool { return contains(*w.val, opts[i].Value) }, &w.optIDs, func(i int, l *paint.List, r geom.Rect) {
			o := opts[i]
			entPaintOption(c, l, r, o.Label, entOptState{selected: contains(*w.val, o.Value), active: st.active == i, disabled: o.Disabled, check: true})
		}, func(i int) {
			w.toggle(opts[i].Value)
			w.box.st.query = ""
			c.SetFocus(w.box.ed.id)
		})
	})
	ov.Style(layout.Padding(entListPad(c)))
	if len(opts) > 0 {
		ov.n.setRole(a11y.RoleListbox).setState(a11y.Multiselectable)
		ov.n.setName(w.box.popupName())
		w.box.listID = ov.n.id
		w.box.activeID = 0
		if st.active >= 0 && st.active < len(w.optIDs) {
			w.box.activeID = w.optIDs[st.active]
		}
	}
}
