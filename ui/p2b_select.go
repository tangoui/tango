package ui

// Select / MultiSelect: option groups, remote search, entry creation,
// custom option rendering, selection limits and virtual dropdowns. When
// any of these is used (or the list is long) the dropdown is driven by
// the VirtualSelect engine (fill_virtual_select.go), so both selects share
// one implementation of grouping, filtering, debounce and virtualization;
// short plain lists keep the original dropdown.

import "time"

// OptionGroup is a titled group of options for Select / MultiSelect
// Groups (the same type as VirtualOptionGroup).
type OptionGroup = VirtualOptionGroup

// OptionState tells an OptionRender function how the row is shown.
type OptionState = VirtualOptionState

// p2bRich holds the advanced dropdown settings shared by Select and
// MultiSelect.
type p2bRich struct {
	groups      []OptionGroup
	remote      func(query string)
	filter      func(query string, o Option) bool
	debounce    time.Duration
	loading     bool
	allowCreate bool
	render      func(o Option, s OptionState)
	maxCount    int
	virtual     int // 0 auto (long lists), 1 always, -1 never
	listH       float32
	version     int
}

// on reports whether the engine drives the dropdown.
func (r *p2bRich) on(opts []Option) bool {
	if r.groups != nil || r.remote != nil || r.allowCreate || r.render != nil || r.maxCount > 0 || r.virtual > 0 || r.loading {
		return true
	}
	if r.virtual == 0 && len(opts) > p2bVirtualMin {
		return true
	}
	return p2bHasGroups(opts)
}

func (r *p2bRich) defaults(c *Context) {
	r.debounce = remoteDebounce
	if v, ok := p2bBool(c, "Select", "virtual"); ok {
		r.virtual = -1
		if v {
			r.virtual = 0
		}
	}
	if v, ok := p2bNum(c, "Select", "listHeight"); ok {
		r.listH = float32(v)
	}
	if v, ok := p2bDur(c, "Select", "debounce"); ok {
		r.debounce = v
	}
}

// engine returns a VirtualSelect engine over opts with the retained state
// of the select owning id.
func (r *p2bRich) engine(c *Context, id ID, opts []Option, search bool) *VirtualSelectW {
	return &VirtualSelectW{
		c: c, opts: opts, groups: r.groups, st: fillVSStateOf(c, id),
		filterable: search, filterFn: r.filter, remote: r.remote, debounce: r.debounce, loading: r.loading,
		allowCreate: r.allowCreate, render: r.render, maxCount: r.maxCount,
		listH: r.listH, version: r.version,
	}
}

// ---------------------------------------------------------------------------
// Select modifiers

// Groups shows the options under titled groups (Ant OptGroup, Element
// el-option-group); the opts passed to Select are ignored. Options can
// also be grouped with Option.Group.
func (w *SelectW) Groups(groups []OptionGroup) *SelectW { w.rx.groups = groups; return w }

// Remote turns on server-side search: fn receives the typed text after
// Debounce (300ms by default) and should replace the options. Pair it with
// Loading while the request runs. Implies Searchable.
func (w *SelectW) Remote(fn func(query string)) *SelectW {
	w.rx.remote, w.search = fn, true
	return w
}

// FilterMethod replaces the case-insensitive label match of the search
// text with fn (same as VirtualSelect.FilterMethod); it is not used with
// Remote, whose results are already filtered. Implies Searchable.
func (w *SelectW) FilterMethod(fn func(query string, o Option) bool) *SelectW {
	w.rx.filter, w.search = fn, true
	return w
}

// Debounce sets the delay before a Remote call.
func (w *SelectW) Debounce(d time.Duration) *SelectW { w.rx.debounce = d; return w }

// Loading shows a spinner and the loading text in the dropdown.
func (w *SelectW) Loading(on bool) *SelectW { w.rx.loading = on; return w }

// AllowCreate offers the typed text as a new option when no label equals
// it (Element allow-create); read it with Created. Implies Searchable.
func (w *SelectW) AllowCreate() *SelectW { w.rx.allowCreate, w.search = true, true; return w }

// OptionRender builds each option row's content with fn (row background,
// highlight and selection marks stay).
func (w *SelectW) OptionRender(fn func(o Option, s OptionState)) *SelectW {
	w.rx.render = fn
	return w
}

// Virtual forces the virtual dropdown on or off; by default lists longer
// than 100 options are virtual (only the visible rows are built).
func (w *SelectW) Virtual(on bool) *SelectW {
	w.rx.virtual = -1
	if on {
		w.rx.virtual = 1
	}
	return w
}

// ListHeight sets the maximum dropdown height (Ant listHeight, 256).
func (w *SelectW) ListHeight(px float32) *SelectW { w.rx.listH = px; return w }

// DataVersion tells the select that the option slice was modified in
// place (the virtual dropdown caches by slice identity and this number).
func (w *SelectW) DataVersion(v int) *SelectW { w.rx.version = v; return w }

// Created returns the value of an entry created with AllowCreate this frame.
func (w *SelectW) Created() (string, bool) { w.z.run(); return w.created, w.isNew }

// finishRich is Select's finish when the engine drives the dropdown: the
// trigger (value text, search editor, clear button) stays Select's own.
func (w *SelectW) finishRich(ed *node) {
	c, st := w.c, w.st
	e := w.rx.engine(c, w.n.id, w.opts, w.search)
	e.val, e.qp, e.anchor, e.hostSrch = w.val, &st.query, w.n.id, w.search
	e.listName = popupName(c, w.n, w.placeholder)
	w.eng = e
	e.prepare()
	es := e.st
	es.pop.open = st.open
	rows := e.rows()
	if st.open && !es.listShown && !st.listShown { // opened by Open()
		e.openAt(rows)
		st.listShown = true
	}
	if w.ia.Clicked {
		if r := c.rects[w.n.id]; w.allowClear && *w.val != "" && c.inline(r).Off(c.pointer.X) > r.W-30 {
			*w.val = ""
			w.changed = true
			es.pop.open = false
		} else if !es.pop.open || !w.search {
			es.pop.open = !es.pop.open
			if es.pop.open {
				st.query = ""
				e.openAt(rows)
				es.listShown = false
			}
		}
		c.invalid = true
	}
	if es.pop.open && c.PressedOutside(w.n.id, es.pop.panel) {
		es.pop.open = false
		c.invalid = true
	}
	if es.pop.open && c.EscapeAt(es.pop.panel) {
		es.pop.open = false
	}
	if c.focus == w.n.id || c.focus == w.edID {
		e.keys(msTakeKeys(c, c.focus, e.wantKey), rows)
	}
	ed.aHidden = true // the search editor is part of the combobox
	if es.pop.open && w.search {
		c.SetFocus(w.edID)
		ed.hit, ed.focus = true, true
		if c.processEdit(w.edID, &st.edit, &st.query, editConfig{}, Interaction{}).changed {
			e.queryEdited()
		}
	}
	e.tickRemote()
	e.show() // option clicks pick (and close) here
	w.changed = w.changed || e.changed
	w.created, w.isNew = e.created, e.isNew
	st.open = es.pop.open
	st.panel = es.pop.panel
	w.listID = es.list
	if !st.open {
		st.listShown = false
		if c.focus == w.edID {
			c.SetFocus(w.n.id)
		}
	}
}

// ---------------------------------------------------------------------------
// MultiSelect modifiers

// Groups shows the options under titled groups; the opts passed to
// MultiSelect are ignored. Options can also be grouped with Option.Group.
func (w *MultiSelectW) Groups(groups []OptionGroup) *MultiSelectW { w.rx.groups = groups; return w }

// Remote turns on server-side search (see SelectW.Remote). Implies Searchable.
func (w *MultiSelectW) Remote(fn func(query string)) *MultiSelectW {
	w.rx.remote, w.box.search = fn, true
	return w
}

// FilterMethod replaces the label match of the search text with fn (see
// SelectW.FilterMethod). Implies Searchable.
func (w *MultiSelectW) FilterMethod(fn func(query string, o Option) bool) *MultiSelectW {
	w.rx.filter, w.box.search = fn, true
	return w
}

// Debounce sets the delay before a Remote call.
func (w *MultiSelectW) Debounce(d time.Duration) *MultiSelectW { w.rx.debounce = d; return w }

// Loading shows a spinner and the loading text in the dropdown.
func (w *MultiSelectW) Loading(on bool) *MultiSelectW { w.rx.loading = on; return w }

// AllowCreate offers the typed text as a new option (Ant mode="tags",
// Element allow-create); read it with Created. Implies Searchable.
func (w *MultiSelectW) AllowCreate() *MultiSelectW {
	w.rx.allowCreate, w.box.search = true, true
	return w
}

// OptionRender builds each option row's content with fn.
func (w *MultiSelectW) OptionRender(fn func(o Option, s OptionState)) *MultiSelectW {
	w.rx.render = fn
	return w
}

// MaxCount limits how many values may be selected (Ant maxCount, Element
// multiple-limit); the other options turn disabled at the limit.
func (w *MultiSelectW) MaxCount(n int) *MultiSelectW { w.rx.maxCount = n; return w }

// Virtual forces the virtual dropdown on or off (default: lists longer
// than 100 options).
func (w *MultiSelectW) Virtual(on bool) *MultiSelectW {
	w.rx.virtual = -1
	if on {
		w.rx.virtual = 1
	}
	return w
}

// ListHeight sets the maximum dropdown height.
func (w *MultiSelectW) ListHeight(px float32) *MultiSelectW { w.rx.listH = px; return w }

// DataVersion tells the select that the option slice was modified in place.
func (w *MultiSelectW) DataVersion(v int) *MultiSelectW { w.rx.version = v; return w }

// Created returns the value of an entry created with AllowCreate this frame.
func (w *MultiSelectW) Created() (string, bool) { w.z.run(); return w.created, w.isNew }

// finishRich runs the VirtualSelect engine on MultiSelect's selector shell.
func (w *MultiSelectW) finishRich() {
	c := w.c
	e := w.rx.engine(c, w.box.n.id, w.opts, w.box.search)
	e.box, e.multiple, e.vals, e.forced = w.box, true, w.val, w.forced
	e.qp, e.anchor = &w.box.st.query, w.box.n.id
	w.eng = e
	e.st.pop.open = w.st.pop.open
	e.finish()
	w.st.pop.open = e.st.pop.open
	w.changed = w.changed || e.changed
	w.created, w.isNew = e.created, e.isNew
}
