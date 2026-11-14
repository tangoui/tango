package ui

// VirtualSelect: a select whose dropdown only builds the visible options,
// so lists of 100k entries open and scroll at full speed (Element Plus
// el-select-v2, Ant Design Select virtual). The trigger is the shared
// selector shell (selector.go) and option rows use entPaintOption, so it
// looks exactly like Select / MultiSelect in every design system.

import (
	"github.com/tangoui/tango/ui/a11y"
	"strings"
	"time"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// VirtualOptionGroup is a titled group of options (Element's grouped
// options, Ant's OptGroup).
type VirtualOptionGroup struct {
	Label   string
	Options []Option
}

// VirtualOptionState tells a custom option renderer how the row is shown.
type VirtualOptionState struct {
	Active   bool // keyboard / hover highlight
	Selected bool
	Disabled bool // disabled option, or the selection limit is reached
	Created  bool // the "create new entry" row of AllowCreate
}

// fillVSData identifies the option data a cache was built from.
type fillVSData struct {
	opts, groups unsafe.Pointer
	nOpts, nGrp  int
	created      int
	version      int
}

// fillVSRow is a dropdown row: a group title (opt -1), an option (index
// into the flat list) or the create row (opt -2).
type fillVSRow struct {
	opt   int32
	group int32
	pos   int32 // 1-based position of an option in its group (or the list)
}

const (
	fillVSHeader = -1
	fillVSCreate = -2
)

type fillVSState struct {
	pop       entPopup
	active    int // highlighted row
	shown     int // highlight at the last build
	listShown bool
	ptr       geom.Point
	created   []Option          // entries added with AllowCreate
	labels    map[string]string // remembered labels of picked values (remote data changes)

	// Caches (rebuilt when the data, query or version changes).
	data      fillVSData
	flat      []Option
	grp       []int32 // group of each flat option (-1 none)
	lower     []string
	index     map[string]int
	rowsData  fillVSData
	rowsQuery string
	rows      []fillVSRow
	exact     bool     // an option label equals the query
	glabels   []string // titles of the groups (explicit or from Option.Group)
	gsize     []int32  // filtered options per group (last entry: ungrouped)

	remote remoteSearch // debounce of Remote calls

	list, activeID ID      // listbox and highlighted option of the last build
	shownH         float32 // row height at the last build
}

// VirtualSelectW is a single or multiple select with a virtualized
// dropdown. Build it with VirtualSelect and chain modifiers; the dropdown
// and value handling run when a query method is called or at the end of
// the build.
type VirtualSelectW struct {
	_ [0]func()

	c        *Context
	box      *selector
	val      *string
	vals     *[]string
	multiple bool
	opts     []Option
	groups   []VirtualOptionGroup
	st       *fillVSState

	filterable  bool
	filterFn    func(query string, o Option) bool
	remote      func(query string)
	debounce    time.Duration
	loading     bool
	allowCreate bool
	render      func(o Option, s VirtualOptionState)
	maxCount    int
	itemH       float32
	listH       float32
	version     int
	forced      bool

	changed bool
	created string
	isNew   bool
	z       *lazy

	// Host of the dropdown: the selector shell of VirtualSelect /
	// MultiSelect, or Select's own trigger (box == nil).
	qp       *string // search text
	anchor   ID      // node the dropdown attaches to
	focusID  ID      // focus target after a multiple pick (0: keep)
	hostSrch bool    // Select's trigger accepts text (box == nil)
	listName string  // listbox name when box == nil
}

// VirtualSelect adds a select bound to *val over opts whose dropdown only
// builds the visible rows (100k options stay smooth). Call Multiple to
// bind a list of values instead.
//
//	ui.VirtualSelect(c, &city, cities).Filterable().AllowClear()
//	ui.VirtualSelect(c, nil, opts).Multiple(&tags).MaxCount(3)
func VirtualSelect(c *Context, val *string, opts []Option) *VirtualSelectW {
	w := &VirtualSelectW{c: c, val: val, opts: opts, debounce: remoteDebounce}
	w.box = newSelector(c, "virtualselect")
	w.box.placeholder = c.Locale().SelectPlaceholder
	w.st = fillVSStateOf(c, w.box.n.id)
	w.qp, w.anchor = &w.box.st.query, w.box.n.id
	p2bSelectDefaults(c, w.box, &w.listH, &w.debounce, &w.filterable)
	w.z = c.lazy(w.box.n, w.finish)
	return w
}

// fillVSStateOf returns the retained dropdown state of the select owning id.
func fillVSStateOf(c *Context, id ID) *fillVSState {
	return State(c, hashID(id, "fill-vs"), func() fillVSState { return fillVSState{labels: map[string]string{}} })
}

// query returns the current search text.
func (w *VirtualSelectW) query() string {
	if w.qp == nil {
		return ""
	}
	return *w.qp
}

func (w *VirtualSelectW) setQuery(s string) {
	if w.qp != nil {
		*w.qp = s
	}
}

// searchOn reports whether the host accepts typed text.
func (w *VirtualSelectW) searchOn() bool {
	if w.box != nil {
		return w.box.search
	}
	return w.hostSrch
}

// Multiple switches to multiple selection bound to *vals (selected values
// shown as closable tags).
func (w *VirtualSelectW) Multiple(vals *[]string) *VirtualSelectW {
	w.multiple, w.vals = true, vals
	return w
}

// Groups replaces the options with titled groups (group titles are not
// selectable and are skipped by the keyboard).
func (w *VirtualSelectW) Groups(groups []VirtualOptionGroup) *VirtualSelectW {
	w.groups = groups
	return w
}

// Placeholder sets the text shown without a value.
func (w *VirtualSelectW) Placeholder(s string) *VirtualSelectW { w.box.placeholder = s; return w }

// Filterable lets the user type to filter options by label (case-insensitive).
func (w *VirtualSelectW) Filterable() *VirtualSelectW {
	w.filterable = true
	w.box.search = true
	return w
}

// FilterMethod replaces the label match used by Filterable.
func (w *VirtualSelectW) FilterMethod(fn func(query string, o Option) bool) *VirtualSelectW {
	w.filterFn = fn
	return w.Filterable()
}

// Remote turns on server-side search: fn receives the typed query after
// Debounce (default 300ms) and should replace the options; local filtering
// is off. Show progress with Loading.
func (w *VirtualSelectW) Remote(fn func(query string)) *VirtualSelectW {
	w.remote = fn
	return w.Filterable()
}

// Debounce sets the delay of Remote calls.
func (w *VirtualSelectW) Debounce(d time.Duration) *VirtualSelectW { w.debounce = d; return w }

// Loading shows the loading text instead of the options.
func (w *VirtualSelectW) Loading(on bool) *VirtualSelectW { w.loading = on; return w }

// AllowCreate offers the typed text as a new option when no label matches
// it exactly (Element allow-create, Ant mode="tags"); read it with Created.
func (w *VirtualSelectW) AllowCreate() *VirtualSelectW { w.allowCreate = true; return w.Filterable() }

// OptionRender builds each option row's content with fn instead of the
// plain label (the row background, highlight and check mark stay).
func (w *VirtualSelectW) OptionRender(fn func(o Option, s VirtualOptionState)) *VirtualSelectW {
	w.render = fn
	return w
}

// MaxCount limits how many values a multiple select may hold (Element
// multiple-limit, Ant maxCount); other options turn disabled at the limit.
func (w *VirtualSelectW) MaxCount(n int) *VirtualSelectW { w.maxCount = n; return w }

// MaxTagCount collapses tags after n into "+ N ...".
func (w *VirtualSelectW) MaxTagCount(n int) *VirtualSelectW { w.box.maxCount = n; return w }

// ItemHeight sets the option row height (Element 34, Ant 32).
func (w *VirtualSelectW) ItemHeight(px float32) *VirtualSelectW { w.itemH = px; return w }

// ListHeight sets the maximum dropdown height (Element 274, Ant 256).
func (w *VirtualSelectW) ListHeight(px float32) *VirtualSelectW { w.listH = px; return w }

// DataVersion tells the select that the option slices were modified in
// place (caches are keyed by slice identity and this number).
func (w *VirtualSelectW) DataVersion(v int) *VirtualSelectW { w.version = v; return w }

// AllowClear shows a clear button on hover.
func (w *VirtualSelectW) AllowClear() *VirtualSelectW { w.box.allowClear = true; return w }

// Status sets the validation status ("error", "warning").
func (w *VirtualSelectW) Status(s string) *VirtualSelectW { w.box.status = s; return w }

// Size sets the control size.
func (w *VirtualSelectW) Size(s ControlSize) *VirtualSelectW { w.box.size = s; return w }

// Width fixes the control width.
func (w *VirtualSelectW) Width(px float32) *VirtualSelectW { w.box.n.ln.Width = px; return w }

// Layout applies layout options to the control.
func (w *VirtualSelectW) Layout(o ...layout.Option) *VirtualSelectW {
	flexbox.Apply(&w.box.n.ln.Style, o...)
	return w
}

// Disabled turns the control off.
func (w *VirtualSelectW) Disabled(on bool) *VirtualSelectW {
	if on {
		w.box.disabled = true
	}
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *VirtualSelectW) Open() *VirtualSelectW { w.st.pop.open, w.forced = true, true; return w }

// IsOpen reports whether the dropdown is shown.
func (w *VirtualSelectW) IsOpen() bool { w.z.run(); return w.st.pop.open }

// Changed reports a selection change this frame.
func (w *VirtualSelectW) Changed() bool { w.z.run(); return w.changed }

// Created returns the value of an entry created with AllowCreate this frame.
func (w *VirtualSelectW) Created() (string, bool) { w.z.run(); return w.created, w.isNew }

// Query returns the current search text.
func (w *VirtualSelectW) Query() string { return w.query() }

// ID returns the control's ID.
func (w *VirtualSelectW) ID() ID { return w.box.n.id }

// ---------------------------------------------------------------------------
// Data caches

func (w *VirtualSelectW) dataKey() fillVSData {
	k := fillVSData{nOpts: len(w.opts), nGrp: len(w.groups), created: len(w.st.created), version: w.version}
	if len(w.opts) > 0 {
		k.opts = unsafe.Pointer(&w.opts[0])
	}
	if len(w.groups) > 0 {
		k.groups = unsafe.Pointer(&w.groups[0])
		k.nOpts = 0
		for i := range w.groups {
			k.nOpts += len(w.groups[i].Options)
		}
	}
	return k
}

// prepare rebuilds the flat option list when the data changed. Options
// are grouped by explicit Groups, else by Option.Group (groups in order of
// first appearance; options without a group form an untitled group).
func (w *VirtualSelectW) prepare() {
	st := w.st
	k := w.dataKey()
	if k == st.data && st.flat != nil {
		return
	}
	st.data = k
	st.index = nil
	n := len(st.created) + k.nOpts
	st.flat = make([]Option, 0, n)
	st.grp = make([]int32, 0, n)
	st.glabels = st.glabels[:0]
	for _, o := range st.created {
		st.flat = append(st.flat, o)
		st.grp = append(st.grp, -1)
	}
	switch {
	case len(w.groups) > 0:
		for gi := range w.groups {
			st.glabels = append(st.glabels, w.groups[gi].Label)
			for _, o := range w.groups[gi].Options {
				st.flat = append(st.flat, o)
				st.grp = append(st.grp, int32(gi))
			}
		}
	case p2bHasGroups(w.opts):
		idx := map[string]int32{}
		var order [][]int
		for i, o := range w.opts {
			g, ok := idx[o.Group]
			if !ok {
				g = int32(len(order))
				idx[o.Group] = g
				order = append(order, nil)
				st.glabels = append(st.glabels, o.Group)
			}
			order[g] = append(order[g], i)
		}
		for g, ids := range order {
			for _, i := range ids {
				st.flat = append(st.flat, w.opts[i])
				st.grp = append(st.grp, int32(g))
			}
		}
	default:
		for _, o := range w.opts {
			st.flat = append(st.flat, o)
			st.grp = append(st.grp, -1)
		}
	}
	st.lower = nil
	st.rowsData = fillVSData{version: -1}
}

// groupTitle returns the title of group g ("" for none / untitled).
func (w *VirtualSelectW) groupTitle(g int32) string {
	if g < 0 || int(g) >= len(w.st.glabels) {
		return ""
	}
	return w.st.glabels[g]
}

// labelOf returns the label of value v.
func (w *VirtualSelectW) labelOf(v string) string {
	st := w.st
	if st.index == nil {
		st.index = make(map[string]int, len(st.flat))
		for i := len(st.flat) - 1; i >= 0; i-- {
			st.index[st.flat[i].Value] = i
		}
	}
	if i, ok := st.index[v]; ok {
		return st.flat[i].Label
	}
	if l, ok := st.labels[v]; ok {
		return l
	}
	return v
}

// rows returns the filtered dropdown rows for the current query.
func (w *VirtualSelectW) rows() []fillVSRow {
	st := w.st
	q := w.query() // the create row offers the typed text
	fq := q        // local filter (remote results are already filtered)
	if w.remote != nil {
		fq = ""
	}
	if st.rowsData == st.data && st.rowsQuery == q && st.rows != nil && w.filterFn == nil {
		return st.rows
	}
	st.rowsData, st.rowsQuery = st.data, q
	ql := strings.ToLower(fq)
	if ql != "" && w.filterFn == nil && st.lower == nil {
		st.lower = make([]string, len(st.flat))
		for i, o := range st.flat {
			st.lower[i] = strings.ToLower(o.Label)
		}
	}
	rows := st.rows[:0]
	if cap(rows) > 4*len(st.flat)+64 {
		rows = nil
	}
	st.exact = false
	ng := len(st.glabels)
	if cap(st.gsize) >= ng+1 {
		st.gsize = st.gsize[:ng+1]
		clear(st.gsize)
	} else {
		st.gsize = make([]int32, ng+1)
	}
	slot := func(g int32) int {
		if g < 0 {
			return ng
		}
		return int(g)
	}
	lastGrp := int32(-1)
	for i := range st.flat {
		if fq != "" {
			var ok bool
			if w.filterFn != nil {
				ok = w.filterFn(fq, st.flat[i])
			} else {
				ok = strings.Contains(st.lower[i], ql)
			}
			if !ok {
				continue
			}
		}
		if q != "" && st.flat[i].Label == q {
			st.exact = true
		}
		g := st.grp[i]
		if g >= 0 && g != lastGrp {
			if w.groupTitle(g) != "" {
				rows = append(rows, fillVSRow{opt: fillVSHeader, group: g})
			}
			lastGrp = g
		}
		st.gsize[slot(g)]++
		rows = append(rows, fillVSRow{opt: int32(i), group: g, pos: st.gsize[slot(g)]})
	}
	if w.allowCreate && q != "" && !st.exact {
		st.gsize[ng]++
		rows = append([]fillVSRow{{opt: fillVSCreate, group: -1, pos: st.gsize[ng]}}, rows...)
	}
	st.rows = rows
	return rows
}

// setSize returns the number of options in the set of row r.
func (w *VirtualSelectW) setSize(r fillVSRow) int {
	g := len(w.st.gsize) - 1
	if r.group >= 0 {
		g = int(r.group)
	}
	if g < 0 || g >= len(w.st.gsize) {
		return 0
	}
	return int(w.st.gsize[g])
}

// ---------------------------------------------------------------------------
// Selection

func (w *VirtualSelectW) selectedSet() map[string]bool {
	m := map[string]bool{}
	if w.multiple {
		if w.vals != nil {
			for _, v := range *w.vals {
				m[v] = true
			}
		}
	} else if w.val != nil && *w.val != "" {
		m[*w.val] = true
	}
	return m
}

func (w *VirtualSelectW) atLimit() bool {
	return w.multiple && w.maxCount > 0 && w.vals != nil && len(*w.vals) >= w.maxCount
}

// rowOption returns the option of row r (the create row yields the query).
func (w *VirtualSelectW) rowOption(r fillVSRow) (Option, bool) {
	switch r.opt {
	case fillVSHeader:
		return Option{}, false
	case fillVSCreate:
		q := w.query()
		return Option{Label: q, Value: q}, true
	}
	return w.st.flat[r.opt], true
}

// rowDisabled reports whether row r cannot be picked.
func (w *VirtualSelectW) rowDisabled(r fillVSRow, sel map[string]bool) bool {
	o, ok := w.rowOption(r)
	if !ok || o.Disabled {
		return true
	}
	return w.atLimit() && !sel[o.Value]
}

// pick applies a click / Enter on row r.
func (w *VirtualSelectW) pick(r fillVSRow) {
	c, st := w.c, w.st
	o, ok := w.rowOption(r)
	if !ok {
		return
	}
	if r.opt == fillVSCreate {
		st.created = append(st.created, o)
		w.created, w.isNew = o.Value, true
	}
	st.labels[o.Value] = o.Label
	if w.multiple {
		if w.vals == nil {
			return
		}
		if contains(*w.vals, o.Value) {
			*w.vals = msWithout(*w.vals, o.Value)
		} else if !w.atLimit() {
			*w.vals = append(append([]string(nil), *w.vals...), o.Value)
		} else {
			return
		}
		w.setQuery("")
		if w.focusID != 0 {
			c.SetFocus(w.focusID)
		}
	} else {
		if w.val != nil && *w.val != o.Value {
			*w.val = o.Value
		}
		st.pop.open = false
		w.setQuery("")
	}
	w.changed = true
	c.invalid = true
}

// step moves the highlight to the next selectable row.
func (w *VirtualSelectW) step(rows []fillVSRow, i, dir int) int {
	n := len(rows)
	if n == 0 {
		return 0
	}
	for range n {
		i = (i + dir + n) % n
		if rows[i].opt != fillVSHeader {
			return i
		}
	}
	return i
}

// fillVSFirst returns the first non-header row at or after i.
func fillVSFirst(rows []fillVSRow, i int) int {
	for ; i < len(rows); i++ {
		if rows[i].opt != fillVSHeader {
			return i
		}
	}
	return 0
}

// openAt highlights the selected value (or the first option) on open.
func (w *VirtualSelectW) openAt(rows []fillVSRow) {
	st := w.st
	st.active = fillVSFirst(rows, 0)
	cur := ""
	if w.multiple {
		if w.vals != nil && len(*w.vals) > 0 {
			cur = (*w.vals)[0]
		}
	} else if w.val != nil {
		cur = *w.val
	}
	if cur == "" {
		return
	}
	for i, r := range rows {
		if r.opt >= 0 && st.flat[r.opt].Value == cur {
			st.active = i
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Build

func (w *VirtualSelectW) finish() {
	c, b, st := w.c, w.box, w.st
	w.prepare()
	var labels []string
	if w.multiple {
		if w.vals != nil {
			labels = make([]string, len(*w.vals))
			for i, v := range *w.vals {
				labels[i] = w.labelOf(v)
			}
		}
	} else if w.val != nil && *w.val != "" {
		b.single = w.labelOf(*w.val)
	}
	b.open = st.pop.open
	b.build(labels)
	w.focusID = b.ed.id
	if b.disabled {
		st.pop.open, b.open = false, false
		return
	}
	rows := w.rows()
	switch {
	case w.multiple && b.removed >= 0 && w.vals != nil && b.removed < len(*w.vals):
		*w.vals = msWithout(*w.vals, (*w.vals)[b.removed])
		w.changed = true
	case b.clearClicked():
		if w.multiple && w.vals != nil {
			*w.vals = nil
		} else if w.val != nil {
			*w.val = ""
		}
		b.st.query = ""
		w.changed = true
		st.pop.open = false
	case b.clicked():
		if !st.pop.open || !b.search {
			st.pop.open = !st.pop.open
			if st.pop.open {
				w.openAt(rows)
				st.listShown = false
			}
		}
		c.SetFocus(b.ed.id)
		c.invalid = true
	}
	if !w.forced {
		st.pop.dismiss(c, b.n.id)
	}
	if st.pop.open && c.focus == 0 {
		c.SetFocus(b.ed.id)
	}
	w.keys(b.takeKeys(w.wantKey), rows)
	if b.edit().changed {
		w.queryEdited()
	}
	w.tickRemote()
	if !b.focused() && !st.pop.open {
		b.st.query = ""
	}
	b.open = st.pop.open
	w.show()
	if st.pop.open && b.open {
		b.listID = st.list
		b.activeID = st.activeID
	}
}

// wantKey reports the keys the dropdown consumes from the focused trigger
// (the rest edit the search text).
func (w *VirtualSelectW) wantKey(k input.KeyEvent) bool {
	switch k.Key {
	case input.KeyUp, input.KeyDown, input.KeyEnter, input.KeyPageUp, input.KeyPageDown:
		return true
	case input.KeyHome, input.KeyEnd:
		return w.st.pop.open && w.query() == ""
	case input.KeySpace:
		return !w.searchOn()
	case input.KeyBackspace:
		return w.multiple && w.box != nil && w.query() == ""
	}
	return false
}

// keys applies the trigger's navigation keys: the first one opens a closed
// list; Up / Down / PageUp / PageDown / Home / End move the highlight over
// selectable rows; Enter / Space pick; Backspace drops the last tag.
func (w *VirtualSelectW) keys(keys []input.KeyEvent, rows []fillVSRow) {
	st := w.st
	sel := w.selectedSet()
	page := max(1, int(w.listHeight()/w.rowH())-1)
	for _, k := range keys {
		if !st.pop.open && k.Key != input.KeyBackspace {
			st.pop.open = true
			w.openAt(rows)
			st.listShown = false
			continue
		}
		switch k.Key {
		case input.KeyDown:
			st.active = w.step(rows, st.active, 1)
		case input.KeyUp:
			st.active = w.step(rows, st.active, -1)
		case input.KeyPageDown:
			st.active = fillVSFirst(rows, min(len(rows)-1, st.active+page))
		case input.KeyPageUp:
			st.active = fillVSFirst(rows, max(0, st.active-page))
		case input.KeyHome:
			st.active = fillVSFirst(rows, 0)
		case input.KeyEnd:
			st.active = w.step(rows, 0, -1)
		case input.KeyEnter, input.KeySpace:
			if st.active >= 0 && st.active < len(rows) && !w.rowDisabled(rows[st.active], sel) {
				w.pick(rows[st.active])
				sel = w.selectedSet()
			}
		case input.KeyBackspace:
			if w.vals != nil && len(*w.vals) > 0 {
				*w.vals = append([]string(nil), (*w.vals)[:len(*w.vals)-1]...)
				w.changed = true
			}
		}
	}
}

// queryEdited reacts to typing: opens the list, schedules a remote
// search and highlights the first match.
func (w *VirtualSelectW) queryEdited() {
	c, st := w.c, w.st
	st.pop.open = true
	if w.remote != nil {
		st.remote.schedule(c, w.debounce)
	}
	st.active = fillVSFirst(w.rows(), 0)
	st.listShown = false
}

// tickRemote fires a debounced remote search when its time has come.
func (w *VirtualSelectW) tickRemote() {
	w.st.remote.tick(w.c, func() {
		if w.remote != nil {
			w.remote(w.query())
		}
	})
}

// show builds the dropdown while open.
func (w *VirtualSelectW) show() {
	c, st := w.c, w.st
	if st.pop.open {
		if w.changed || w.isNew {
			w.prepare()
		}
		w.dropdown(w.rows(), w.selectedSet())
	} else {
		st.listShown = false
		st.list, st.activeID = 0, 0
	}
	st.ptr = c.pointer
}

func (w *VirtualSelectW) rowH() float32 {
	if w.itemH > 0 {
		return w.itemH
	}
	return entOptionH(w.c) // Select token OptionHeight
}

func (w *VirtualSelectW) listHeight() float32 {
	if w.listH > 0 {
		return w.listH
	}
	if w.c.ThemeFor(theme.CompSelect).Flavor == theme.Element {
		return 274
	}
	return 256
}

// popupName names the listbox.
func (w *VirtualSelectW) popupName() string {
	if w.box != nil {
		return w.box.popupName()
	}
	if w.listName != "" {
		return w.listName
	}
	return w.c.Locale().SelectPlaceholder
}

func (w *VirtualSelectW) dropdown(rows []fillVSRow, sel map[string]bool) {
	c, st := w.c, w.st
	loc := c.Locale()
	rh := w.rowH()
	st.list, st.activeID = 0, 0
	ov := entPanel(c, w.anchor, &st.pop, true, func() {
		switch {
		case w.loading && (len(rows) == 0 || w.remote != nil):
			entLoadingRow(c, loc.Loading, rh)
			return
		case len(rows) == 0:
			msg := loc.NoData
			if w.query() != "" && loc.NotFound != "" {
				msg = loc.NotFound
			}
			Empty(c).Simple().Description(msg)
			return
		}
		total := float64(len(rows)) * float64(rh)
		h := min(float64(w.listHeight()), total)
		sb := fillViewport(c, "fill-vs-list", []layout.Option{layout.H(float32(h))})
		fillSem(c, sb.n, a11y.RoleListbox, w.popupName(), fillIf(w.multiple, a11y.Multiselectable))
		st.list = sb.n.id
		sb.s.extent.Y = total
		view := fillViewH(sb, float32(h))
		fillClampOff(sb, total, view)
		follow := !st.listShown || st.active != st.shown || st.shownH != rh // row height changes with the theme
		if follow && st.active >= 0 && st.active < len(rows) {
			fillReveal(c, sb, float64(st.active)*float64(rh), float64(rh), view)
		}
		st.shownH = rh
		moved := c.pointer != st.ptr
		first, end := fillSpan(sb.s.off.Y, view, float64(rh), len(rows), 3)
		sb.s.base.Y = float64(first) * float64(rh)
		sb.Body(func() {
			for i := first; i < end; {
				g := rows[i].group
				if g < 0 || w.groupTitle(g) == "" {
					w.row(i, rows[i], sel, moved, rh)
					i++
					continue
				}
				// A titled group: its rows in a named group container.
				j := i
				for j < end && rows[j].group == g {
					j++
				}
				from, to := i, j
				title := w.groupTitle(g)
				Key(c, "fill-vs-g\x00"+title, func() {
					gb := c.box("fill-vs-grp", layout.Column, nil)
					gb.n.setRole(a11y.RoleGroup).setName(title)
					gb.Body(func() {
						for k := from; k < to; k++ {
							w.row(k, rows[k], sel, moved, rh)
						}
					})
				})
				i = j
			}
		})
		st.shown, st.listShown = st.active, true
	})
	ov.Style(layout.Padding(entListPad(c)))
}

func (w *VirtualSelectW) row(i int, r fillVSRow, sel map[string]bool, moved bool, rh float32) {
	c, th, st := w.c, w.c.ThemeFor(theme.CompSelect), w.st
	tk := &th.Components.Select
	el := th.Flavor == theme.Element
	if r.opt == fillVSHeader {
		title := w.groupTitle(r.group)
		Key(c, "fill-vs-group\x00"+title, func() {
			n := c.add("fill-vs-group", title)
			n.aHidden = true // names the enclosing group
			n.ln.Height = rh
			n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{H: rh} }
			padL := tk.OptionPadding // aligned with the option labels
			n.paint = func(l *paint.List, rr geom.Rect) {
				col := th.TextTertiary
				if el {
					col = th.TextQuaternary
				}
				entTextIn(c, l, title, padL, rr, th.FontSizeSM, text.Regular, col, rr.W-padL-12)
			}
		})
		return
	}
	o, _ := w.rowOption(r)
	key := o.Value
	if r.opt == fillVSCreate {
		key = "fill-vs-create\x00"
	}
	Key(c, key, func() {
		dis := w.rowDisabled(r, sel)
		on := sel[o.Value]
		var padL float32
		if r.group >= 0 && w.groupTitle(r.group) != "" && th.Flavor == theme.Ant {
			padL = 24
		}
		look := func() entOptState {
			return entOptState{selected: on, active: st.active == i, disabled: dis, check: w.multiple, padL: padL}
		}
		var n *node
		if w.render != nil {
			bx := c.box("fill-vs-option", layout.Row, []layout.Option{layout.H(rh), layout.Middle()})
			n = bx.n
			n.label = o.Label
			pl := padL
			if pl == 0 {
				pl = 12
				if el {
					pl = 20
				}
			}
			pr := float32(12)
			if w.multiple {
				pr += 18
			}
			n.ln.Padding = geom.Insets{Left: pl, Right: pr}
			bx.Body(func() {
				w.render(o, VirtualOptionState{Active: st.active == i, Selected: on, Disabled: dis, Created: r.opt == fillVSCreate})
			})
			n.paint = func(l *paint.List, rr geom.Rect) { entPaintOption(c, l, rr, "", look()) }
		} else {
			n = c.add("fill-vs-option", o.Label)
			n.ln.Height = rh
			n.ln.Measure = func(_, _ float32) geom.Size {
				return geom.Size{W: entTextW(c, o.Label, th.FontSize, text.Semibold) + 24 + 18, H: rh}
			}
			n.paint = func(l *paint.List, rr geom.Rect) { entPaintOption(c, l, rr, o.Label, look()) }
		}
		cur := input.CursorPointer
		if dis {
			n.disabled = true
			cur = input.CursorNotAllowed
		}
		ia := c.interact(n, false, cur)
		fillSem(c, n, a11y.RoleOption, o.Label, a11y.Selectable|fillIf(on, a11y.Selected)|fillIf(dis, a11y.Disabled)|fillIf(st.active == i, a11y.Current))
		if c.a11yOn {
			pos, size := int(r.pos), w.setSize(r)
			n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = pos, size }))
		}
		if st.active == i {
			st.activeID = n.id
		}
		fillKeyboard(c, n)
		if ia.Hovered && moved && !dis {
			st.active = i
		}
		if ia.Clicked && !dis {
			w.pick(r)
		}
	})
}
