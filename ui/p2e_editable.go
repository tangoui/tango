package ui

import (
	"slices"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// EditableProTableW is a table over app-owned rows edited in place (Ant
// Design Pro EditableProTable): columns with an Editor show the ProField
// control of their value type while a row is edited; an action column
// offers edit / save / cancel / delete; "add a row" creates a draft row
// that joins the data when saved. Rows are validated (Required, Rules,
// Validate, AsyncValidate of every editor) before they are written back.
//
//	ui.EditableProTable(c, cols, &rows).RowKey(func(r Row) string { return r.ID }).
//		NewRow(func() Row { return Row{ID: newID()} }).
//		OnSave(func(r Row, i int, done func(error)) { go save(r, done) }).
//		Show()
type EditableProTableW[T any] struct {
	_ [0]func()

	c        *Context
	cols     []ProColumn[T]
	rows     *[]T
	title    string
	toolbar  func()
	rowKey   func(T) string
	newRow   func() T
	top      bool
	maxLen   int
	multiple bool
	keys     *[]string
	canEdit  func(row T) bool
	onSave   func(row T, index int, done func(error))
	onDelete func(row T, index int)
	onSort   func(from, to int)
	sortable bool
	bordered bool
	size     ControlSize
	sizeSet  bool
	pageSize int
	scrollX  float32
	scrollY  float32
	virtual  bool
	noDelete bool

	id    ID
	st    *p2eEditState[T]
	table *TableW
	view  []p2eEditRow[T] // displayed rows, copied per Show (default form)
	vw    p2eView         // shape of the displayed rows (bounded form)
	lazy  bool            // bounded form (ScrollY): vw, read live
	snap  p2eSnap[T]      // bounded form: rows the table builds this frame

	saved, deleted, moved bool

	// CellEditorTable / RowEditorTable presets (table_pro_editor.go).
	mode      p2eMode
	noActions bool        // no automatic action column
	edNodes   []p2eEdNode // editor cells built this frame
	disabled  bool        // the table is inside a disabled subtree
}

// p2eView is the shape of the displayed rows of a bounded (ScrollY) Show:
// the keys of the new rows not saved yet (shown at the top or the bottom)
// and the number of data rows. The rows are read by index (viewAt), so
// nothing of *rows is copied for rows the table does not build.
type p2eView struct {
	news []string
	n    int
}

// p2eSnap is the bounded form's frame-local copy of the rows the table
// builds (its window [first, first+len(rows)) and the kept edited rows),
// taken right before it builds them (TableW.rowsHook): a row deleted or a
// new row dropped while the body is being built leaves the rest of that
// build as consistent as the default form's per-Show copy. on is false
// until the hook ran in this Show (rows are then read live).
type p2eSnap[T any] struct {
	on    bool
	first int
	rows  []p2eEditRow[T]
	ok    []bool
	keep  map[int]p2eEditRow[T]
}

type p2eEditRow[T any] struct {
	row   T
	index int // into *rows; -1: a new row not saved yet
	key   string
}

type p2eEditState[T any] struct {
	keys     []string             // editing rows (when not bound)
	drafts   map[string]ProValues // draft values per editing row
	errs     map[string]map[string]string
	pending  map[string]T // new rows not saved yet
	newOrder []string
	seq      int
	gen      map[string]int  // validation run per row
	waiting  map[string]int  // async validations in flight per row
	saving   map[string]bool // OnSave in flight
	saveErr  map[string]string
	async    pro3Async[p2eAsyncResult]
	saveRes  pro3Async[p2eSaveResult]
	page     int
	pageSz   int

	// Presets: the edited column (cell mode), focus the editor on the
	// next frame, the editor had focus (blur saves).
	cellCol  string
	focusCol string
	fresh    bool
	hadFocus bool
}

type p2eAsyncResult struct {
	key, col string
	gen      int
	msg      string
}

type p2eSaveResult struct {
	key string
	gen int
	err error
}

// p2eSaveFailed keys the row-level save error in p2eEditState.errs.
const p2eSaveFailed = "\x00save"

// EditableProTable starts an editable table over *rows; chain options,
// then Show.
func EditableProTable[T any](c *Context, cols []ProColumn[T], rows *[]T) *EditableProTableW[T] {
	// ConfigProvider.Defaults("EditableProTable", ...) keys: "size",
	// "bordered" (bool), "pagination" (page size or Props{"pageSize": n}),
	// "maxLength" (number).
	e := &EditableProTableW[T]{c: c, cols: cols, rows: rows}
	if sz, ok := proPropSize(c, "EditableProTable"); ok {
		e.size, e.sizeSet = sz, true
	}
	e.bordered = Prop(c, "EditableProTable", "bordered", false)
	if _, ok := c.ConfigProp("EditableProTable", "pagination"); ok {
		if n, on := proPropPageSize(c, "EditableProTable", 0); on {
			e.pageSize = max(1, n)
		}
	}
	if n, ok := proPropInt(c, "EditableProTable", "maxLength"); ok {
		e.maxLen = n
	}
	return e
}

// Title sets the toolbar title.
func (e *EditableProTableW[T]) Title(s string) *EditableProTableW[T] { e.title = s; return e }

// ToolBar adds actions at the right of the toolbar.
func (e *EditableProTableW[T]) ToolBar(fn func()) *EditableProTableW[T] { e.toolbar = fn; return e }

// RowKey gives rows stable keys (needed when rows are inserted, deleted or
// reordered while one is edited; default: the row index).
func (e *EditableProTableW[T]) RowKey(f func(T) string) *EditableProTableW[T] { e.rowKey = f; return e }

// NewRow shows the "add a row" button; fn creates the draft row (set its
// key there). The new row opens in edit mode and joins *rows when saved.
func (e *EditableProTableW[T]) NewRow(fn func() T) *EditableProTableW[T] { e.newRow = fn; return e }

// AddAtTop puts new rows (and the add button) above the data.
func (e *EditableProTableW[T]) AddAtTop() *EditableProTableW[T] { e.top = true; return e }

// MaxLength hides the add button once there are n rows.
func (e *EditableProTableW[T]) MaxLength(n int) *EditableProTableW[T] { e.maxLen = n; return e }

// Multiple lets several rows be edited at once (default: one; the other
// rows' edit actions are disabled meanwhile).
func (e *EditableProTableW[T]) Multiple() *EditableProTableW[T] { e.multiple = true; return e }

// EditableKeys binds the keys of the rows being edited (controlled).
func (e *EditableProTableW[T]) EditableKeys(keys *[]string) *EditableProTableW[T] {
	e.keys = keys
	return e
}

// RowEditable limits which rows can be edited.
func (e *EditableProTableW[T]) RowEditable(fn func(row T) bool) *EditableProTableW[T] {
	e.canEdit = fn
	return e
}

// OnSave receives a validated row (index -1 for a new row) before it is
// written back; call done from any goroutine. A nil error writes the row
// into *rows, an error keeps the row in edit mode and shows the message.
func (e *EditableProTableW[T]) OnSave(fn func(row T, index int, done func(error))) *EditableProTableW[T] {
	e.onSave = fn
	return e
}

// OnDelete is called after a confirmed delete instead of removing the row
// from *rows (the app removes it, e.g. after a server call).
func (e *EditableProTableW[T]) OnDelete(fn func(row T, index int)) *EditableProTableW[T] {
	e.onDelete = fn
	return e
}

// NoDelete hides the delete action.
func (e *EditableProTableW[T]) NoDelete() *EditableProTableW[T] { e.noDelete = true; return e }

// DragSort adds a drag handle column that reorders *rows; fn (may be nil)
// is told the move (indices into *rows) after it happened.
func (e *EditableProTableW[T]) DragSort(fn func(from, to int)) *EditableProTableW[T] {
	e.sortable, e.onSort = true, fn
	return e
}

// Bordered draws cell borders.
func (e *EditableProTableW[T]) Bordered() *EditableProTableW[T] { e.bordered = true; return e }

// Size sets the density.
func (e *EditableProTableW[T]) Size(s ControlSize) *EditableProTableW[T] {
	e.size, e.sizeSet = s, true
	return e
}

// Pagination pages the rows locally, n per page.
func (e *EditableProTableW[T]) Pagination(n int) *EditableProTableW[T] {
	e.pageSize = max(1, n)
	return e
}

// ScrollX sets the table's scroll width.
func (e *EditableProTableW[T]) ScrollX(w float32) *EditableProTableW[T] { e.scrollX = w; return e }

// ScrollY bounds the table body to maxH (TableW.ScrollY): the header stays
// in place and only the visible rows are built, so many thousand rows stay
// fast. Rows being edited are always built, even scrolled out of view, so
// their drafts and focus survive; starting to edit a row (Edit, AddRow),
// focus moving into an edited row (Tab), typing in one and a failed
// validation scroll it into view. An edited key not found among the rows
// is not looked up again every frame: a key that reappears in place (a
// row re-keyed at the same row count) is picked up once the row count
// changes or a new rows slice is passed. The toolbar and the add button
// stay outside the scrolling body. Default 0: unbounded, every row is
// built.
func (e *EditableProTableW[T]) ScrollY(maxH float32) *EditableProTableW[T] {
	e.scrollY = maxH
	return e
}

// Virtual is TableW.Virtual for the inner table: with ScrollY the body is
// windowed either way, Virtual states the intent (and takes the theme's
// Table V2 row height where it has one, as TableW.Virtual). Otherwise
// rows keep the height of the density (Size).
func (e *EditableProTableW[T]) Virtual() *EditableProTableW[T] { e.virtual = true; return e }

// Saved reports a row written back in the last Show.
func (e *EditableProTableW[T]) Saved() bool { return e.saved }

// Deleted reports a deleted row in the last Show.
func (e *EditableProTableW[T]) Deleted() bool { return e.deleted }

// Moved reports a drag-sort move in the last Show.
func (e *EditableProTableW[T]) Moved() bool { return e.moved }

// Changed reports any change of *rows in the last Show.
func (e *EditableProTableW[T]) Changed() bool { return e.saved || e.deleted || e.moved }

// Table returns the inner table of the last Show.
func (e *EditableProTableW[T]) Table() *TableW { return e.table }

// ID returns the widget ID (valid after Show).
func (e *EditableProTableW[T]) ID() ID { return e.id }

// Editing reports whether the row with key k is in edit mode.
func (e *EditableProTableW[T]) Editing(k string) bool {
	return e.st != nil && slices.Contains(*e.editKeys(), k)
}

// Errors returns the validation messages of the row with key k by column
// key (nil when valid).
func (e *EditableProTableW[T]) Errors(k string) map[string]string {
	if e.st == nil {
		return nil
	}
	return e.st.errs[k]
}

// Validating reports async validations or a save in flight for row k.
func (e *EditableProTableW[T]) Validating(k string) bool {
	return e.st != nil && (e.st.waiting[k] > 0 || e.st.saving[k])
}

// Edit puts the row with key k into edit mode (valid after Show).
func (e *EditableProTableW[T]) Edit(k string) {
	if e.st == nil {
		return
	}
	if r, ok := e.find(k); ok {
		e.startEdit(r)
	}
}

// Save validates the edited row k and writes it back (as its save action
// does; valid after Show).
func (e *EditableProTableW[T]) Save(k string) {
	if e.st != nil && slices.Contains(*e.editKeys(), k) {
		e.save(k)
	}
}

// Cancel leaves edit mode of row k without saving (a new row is dropped).
func (e *EditableProTableW[T]) Cancel(k string) {
	if e.st != nil {
		e.stopEdit(k)
	}
}

// AddRow adds a new draft row in edit mode (as the add button does) and
// returns its key ("" when NewRow is not set or another row is being
// edited in single mode).
func (e *EditableProTableW[T]) AddRow() string {
	if e.st == nil || e.newRow == nil {
		return ""
	}
	st := e.st
	if !e.multiple {
		if ks := *e.editKeys(); len(ks) > 0 {
			return ""
		}
	}
	st.seq++
	k := "new:" + strconv.Itoa(st.seq)
	row := e.newRow()
	st.pending[k] = row
	if e.top {
		st.newOrder = append([]string{k}, st.newOrder...)
	} else {
		st.newOrder = append(st.newOrder, k)
	}
	e.startEdit(p2eEditRow[T]{row: row, index: -1, key: k})
	if e.mode != p2eModeNone {
		st.focusCol, st.fresh, st.hadFocus = "", true, false
	}
	if e.pageSize > 0 && !e.top {
		st.page = max(1, (len(*e.rows)+len(st.newOrder)+e.pageSize-1)/e.pageSize)
	} else if e.top {
		st.page = 1
	}
	return k
}

func (e *EditableProTableW[T]) editKeys() *[]string {
	if e.keys != nil {
		return e.keys
	}
	return &e.st.keys
}

func (e *EditableProTableW[T]) key(row T, i int) string {
	if e.rowKey != nil {
		return e.rowKey(row)
	}
	return strconv.Itoa(i)
}

// field returns the editor of a column with its defaults filled in.
func (e *EditableProTableW[T]) field(pc *ProColumn[T]) ProField {
	fd := *pc.Editor
	if fd.Key == "" {
		fd.Key = pc.Key
	}
	if fd.Label == "" {
		fd.Label = pc.Title
	}
	if fd.ValueType == "" {
		switch pc.ValueType {
		case ValueStatus, ValueTag:
			fd.ValueType = ValueSelect
		case ValueLink, ValueOption, ValueProgress:
			fd.ValueType = ValueText
			if pc.ValueType == ValueProgress {
				fd.ValueType = ValueDigit
			}
		default:
			fd.ValueType = pc.ValueType
		}
	}
	if fd.ValueEnum == nil {
		fd.ValueEnum = pc.ValueEnum
	}
	return fd
}

func (e *EditableProTableW[T]) editable(pc *ProColumn[T]) bool {
	if e.mode == p2eModeCell && (e.st == nil || pc.Key != e.st.cellCol) {
		return false // cell mode edits (and validates) one column only
	}
	return pc.Editor != nil && pc.Set != nil && !pc.HideInTable
}

// startEdit opens row r in edit mode with a draft of its values.
func (e *EditableProTableW[T]) startEdit(r p2eEditRow[T]) {
	st := e.st
	d := ProValues{}
	for i := range e.cols {
		pc := &e.cols[i]
		if !e.editable(pc) || pc.Value == nil {
			continue
		}
		v := pc.Value(r.row)
		switch x := v.(type) {
		case nil:
			continue
		case []string:
			v = append([]string(nil), x...)
		case []UploadFile:
			v = append([]UploadFile(nil), x...)
		}
		d[pc.Key] = v
	}
	st.drafts[r.key] = d
	delete(st.errs, r.key)
	keys := e.editKeys()
	if !slices.Contains(*keys, r.key) {
		*keys = append(*keys, r.key)
	}
	e.table.revealRow(r.key) // bounded table: show the row (Edit, AddRow)
	e.c.invalid = true
}

// stopEdit leaves edit mode (a new row is dropped).
func (e *EditableProTableW[T]) stopEdit(k string) {
	st := e.st
	keys := e.editKeys()
	if i := slices.Index(*keys, k); i >= 0 {
		*keys = slices.Delete(*keys, i, i+1)
	}
	delete(st.drafts, k)
	delete(st.errs, k)
	delete(st.waiting, k)
	delete(st.saving, k)
	st.gen[k]++ // late async results are ignored
	if e.mode != p2eModeNone {
		st.cellCol, st.focusCol, st.fresh, st.hadFocus = "", "", false, false
	}
	if _, ok := st.pending[k]; ok {
		delete(st.pending, k)
		if i := slices.Index(st.newOrder, k); i >= 0 {
			st.newOrder = slices.Delete(st.newOrder, i, i+1)
		}
	}
	e.c.invalid = true
}

// Show builds the table.
func (e *EditableProTableW[T]) Show() *EditableProTableW[T] {
	c, th := e.c, e.c.Theme
	root := c.box("editable-protable", layout.Column, nil)
	e.id = root.n.id
	e.disabled = root.n.disabled
	st := State[p2eEditState[T]](c, e.id, func() p2eEditState[T] {
		return p2eEditState[T]{drafts: map[string]ProValues{}, errs: map[string]map[string]string{},
			pending: map[string]T{}, gen: map[string]int{}, waiting: map[string]int{}, saving: map[string]bool{},
			saveErr: map[string]string{}, page: 1, pageSz: max(1, e.pageSize)}
	})
	e.st = st
	if e.pageSize > 0 {
		st.pageSz = e.pageSize
	}
	e.lazy = e.scrollY > 0
	e.buildView()
	e.poll()
	e.buildView() // results may have changed the rows
	root.Body(func() {
		if e.title != "" || e.toolbar != nil {
			Row(c, func() {
				if e.title != "" {
					Text(c, e.title).Size(th.FontSizeLG).Weight(text.Medium).Color(th.TextHeading)
				}
				Spacer(c)
				if e.toolbar != nil {
					Row(c, e.toolbar, layout.Gap(8), layout.Middle())
				}
			}, layout.Middle(), layout.MinH(56))
		}
		if e.top {
			e.addButton()
		}
		e.tableView()
		if !e.top {
			e.addButton()
		}
	})
	return e
}

// buildView lists the displayed rows: new rows (top or bottom) and data.
// The bounded form takes only their shape (rows are read by index).
func (e *EditableProTableW[T]) buildView() {
	st := e.st
	if e.lazy {
		e.vw.news = append(e.vw.news[:0], st.newOrder...)
		e.vw.n = len(*e.rows)
		return
	}
	e.view = e.view[:0]
	news := make([]p2eEditRow[T], 0, len(st.newOrder))
	for _, k := range st.newOrder {
		news = append(news, p2eEditRow[T]{row: st.pending[k], index: -1, key: k})
	}
	if e.top {
		e.view = append(e.view, news...)
	}
	for i, r := range *e.rows {
		e.view = append(e.view, p2eEditRow[T]{row: r, index: i, key: e.key(r, i)})
	}
	if !e.top {
		e.view = append(e.view, news...)
	}
}

// viewLen is the number of displayed rows.
func (e *EditableProTableW[T]) viewLen() int {
	if e.lazy {
		return len(e.vw.news) + e.vw.n
	}
	return len(e.view)
}

// viewRow returns displayed row i of the bounded form and its index into
// *rows (-1: a new row), read live; ok is false when the row went away
// since buildView (deleted or saved during this build).
func (e *EditableProTableW[T]) viewRow(i int) (row T, index int, key string, ok bool) {
	nn := len(e.vw.news)
	if e.top {
		if i >= 0 && i < nn {
			return e.pendingRow(e.vw.news[i])
		}
		i -= nn
	} else if i >= e.vw.n {
		if j := i - e.vw.n; j < nn {
			return e.pendingRow(e.vw.news[j])
		}
		return row, -1, "", false
	}
	if i < 0 || i >= e.vw.n || i >= len(*e.rows) {
		return row, -1, "", false
	}
	return (*e.rows)[i], i, "", true
}

func (e *EditableProTableW[T]) pendingRow(k string) (T, int, string, bool) {
	row, ok := e.st.pending[k]
	return row, -1, k, ok
}

// viewAt returns displayed row i with its key: the default form's copy,
// or the bounded form's row read live (see viewRow).
func (e *EditableProTableW[T]) viewAt(i int) (p2eEditRow[T], bool) {
	if !e.lazy {
		if i < 0 || i >= len(e.view) {
			return p2eEditRow[T]{}, false
		}
		return e.view[i], true
	}
	row, idx, k, ok := e.viewRow(i)
	if ok && idx >= 0 {
		k = e.key(row, idx)
	}
	return p2eEditRow[T]{row: row, index: idx, key: k}, ok
}

// poll applies async validation and save results.
func (e *EditableProTableW[T]) poll() {
	st := e.st
	for {
		r, ok := st.async.poll()
		if !ok {
			break
		}
		if r.gen != st.gen[r.key] {
			continue
		}
		if r.msg != "" {
			e.setErr(r.key, r.col, r.msg)
		}
		st.waiting[r.key]--
		if st.waiting[r.key] <= 0 {
			delete(st.waiting, r.key)
			if len(st.errs[r.key]) == 0 {
				e.commit(r.key)
			} else {
				e.table.revealRow(r.key)
			}
		}
		e.c.invalid = true
	}
	for {
		r, ok := st.saveRes.poll()
		if !ok {
			break
		}
		if r.gen != st.gen[r.key] || !st.saving[r.key] {
			continue
		}
		delete(st.saving, r.key)
		if r.err != nil {
			e.setErr(r.key, p2eSaveFailed, r.err.Error())
		} else {
			e.write(r.key)
		}
		e.c.invalid = true
	}
}

func (e *EditableProTableW[T]) setErr(k, col, msg string) {
	m := e.st.errs[k]
	if m == nil {
		m = map[string]string{}
		e.st.errs[k] = m
	}
	m[col] = msg
}

func (e *EditableProTableW[T]) find(k string) (p2eEditRow[T], bool) {
	for i, n := 0, e.viewLen(); i < n; i++ {
		if r, ok := e.viewAt(i); ok && r.key == k {
			return r, true
		}
	}
	return p2eEditRow[T]{}, false
}

// save validates the draft of row k and commits it.
func (e *EditableProTableW[T]) save(k string) {
	c, st := e.c, e.st
	if st.waiting[k] > 0 || st.saving[k] {
		return
	}
	draft := st.drafts[k]
	if draft == nil {
		draft = ProValues{}
		st.drafts[k] = draft
	}
	delete(st.errs, k)
	st.gen[k]++
	gen := st.gen[k]
	var async []ProField
	for i := range e.cols {
		pc := &e.cols[i]
		if !e.editable(pc) {
			continue
		}
		fd := e.field(pc)
		fd.Key = pc.Key
		if msg := pro3Check(proFieldRules(c, &fd, draft), proValueText(draft[pc.Key])); msg != "" {
			e.setErr(k, pc.Key, msg)
			continue
		}
		if fd.AsyncValidate != nil {
			async = append(async, fd)
		}
	}
	c.invalid = true
	if len(st.errs[k]) > 0 {
		e.table.revealRow(k) // bounded table: show the failing row
		return
	}
	if len(async) == 0 {
		e.commit(k)
		return
	}
	st.waiting[k] = len(async)
	ch, inv := st.async.chInit(), c.Invalidator()
	for _, fd := range async {
		col := fd.Key
		fd.AsyncValidate(draft[col], func(msg string) {
			ch <- p2eAsyncResult{key: k, col: col, gen: gen, msg: msg}
			inv()
		})
	}
	e.poll() // synchronous validators have already answered
}

// edited returns row k with the draft applied.
func (e *EditableProTableW[T]) edited(k string) (T, int, bool) {
	r, ok := e.find(k)
	if !ok {
		var zero T
		return zero, -1, false
	}
	row := r.row
	draft := e.st.drafts[k]
	for i := range e.cols {
		pc := &e.cols[i]
		if e.editable(pc) {
			pc.Set(&row, draft[pc.Key])
		}
	}
	return row, r.index, true
}

// commit hands a validated row to OnSave (or writes it right away).
func (e *EditableProTableW[T]) commit(k string) {
	st := e.st
	row, idx, ok := e.edited(k)
	if !ok {
		e.stopEdit(k)
		return
	}
	if e.onSave == nil {
		e.write(k)
		return
	}
	st.saving[k] = true
	gen := st.gen[k]
	ch, inv := st.saveRes.chInit(), e.c.Invalidator()
	e.onSave(row, idx, func(err error) {
		ch <- p2eSaveResult{key: k, gen: gen, err: err}
		inv()
	})
	e.poll() // a synchronous save has already delivered
}

// write puts the edited row into *rows and leaves edit mode.
func (e *EditableProTableW[T]) write(k string) {
	row, idx, ok := e.edited(k)
	if !ok {
		e.stopEdit(k)
		return
	}
	rows := e.rows
	if idx >= 0 && idx < len(*rows) {
		(*rows)[idx] = row
	} else if e.top {
		*rows = append([]T{row}, *rows...)
	} else {
		*rows = append(*rows, row)
	}
	e.stopEdit(k)
	e.saved = true
	e.buildView()
}

// remove deletes row r.
func (e *EditableProTableW[T]) remove(r p2eEditRow[T]) {
	e.stopEdit(r.key)
	if r.index < 0 {
		return
	}
	if e.onDelete != nil {
		e.onDelete(r.row, r.index)
	} else if r.index < len(*e.rows) {
		*e.rows = slices.Delete(*e.rows, r.index, r.index+1)
	}
	e.deleted = true
	e.c.invalid = true
	e.buildView()
}

// addButton is the dashed full-width "add a row" button.
func (e *EditableProTableW[T]) addButton() {
	c := e.c
	if e.newRow == nil || (e.maxLen > 0 && e.viewLen() >= e.maxLen) {
		return
	}
	busy := !e.multiple && len(*e.editKeys()) > 0
	m := geom.Insets{Top: 8}
	if e.top {
		m = geom.Insets{Bottom: 8}
	}
	Column(c, func() {
		b := Button(c, c.Locale().Form.AddRow).Dashed().Icon(icon.Plus).Block().Disabled(busy)
		if b.Clicked() {
			e.AddRow()
		}
	}, layout.Margin(m))
}

// tableView builds the inner Table.
func (e *EditableProTableW[T]) tableView() {
	c, st := e.c, e.st
	loc := c.Locale()
	// The default form copies the rows per Show (closures see this
	// Show's copy, as before batch 3); the bounded form reads the rows the
	// table builds from its frame-local snapshot (snapAt).
	var at func(i int) (p2eEditRow[T], bool)
	var rowAt func(i int) (T, bool)
	if e.lazy {
		e.snap.on = false
		at = e.snapAt
		rowAt = func(i int) (T, bool) {
			r, ok := e.snapAt(i)
			return r.row, ok
		}
	} else {
		view := e.view
		rowsT := make([]T, len(view))
		for i, r := range view {
			rowsT[i] = r.row
		}
		at = func(i int) (p2eEditRow[T], bool) { return view[i], true }
		rowAt = func(i int) (T, bool) { return rowsT[i], true }
	}
	keyAt := func(i int) string {
		r, _ := at(i)
		return r.key
	}
	pt := &ProTableW[T]{c: c}
	var cols []TableColumn
	actions := e.noActions
	e.edNodes = e.edNodes[:0]
	for i := range e.cols {
		pc := &e.cols[i]
		if pc.HideInTable {
			continue
		}
		if pc.ValueType == ValueOption && pc.Render == nil {
			actions = true
			cols = append(cols, e.actionColumn(pc.Key, pc.Title, pc.Width, at))
			continue
		}
		col := pt.tableColumn(*pc, rowAt)
		if e.editable(pc) {
			fd := e.field(pc)
			fd.Key = pc.Key
			key := pc.Key
			col.Editor = &TableEditor{
				Render: func(i int) {
					if r, ok := at(i); ok {
						e.cell(r.key, &fd)
					}
				},
				Error: func(i int) string { return st.errs[keyAt(i)][key] },
			}
		}
		cols = append(cols, col)
	}
	if !actions {
		cols = append(cols, e.actionColumn("__actions", loc.EditableTable.Actions, 0, at))
	}
	t := Table(c, cols, e.viewLen()).
		RowKey(keyAt).
		EditableRows(e.editKeys())
	if e.title != "" {
		t.A11yLabel(e.title)
	}
	if e.sizeSet {
		t.Size(e.size)
	}
	if e.bordered {
		t.Bordered()
	}
	if e.scrollX > 0 {
		t.ScrollX(e.scrollX)
	}
	if e.lazy {
		t.ScrollY(e.scrollY)
		t.keepEdit = true
		t.rowsHook = e.snapRows
		if len(*e.rows) > 0 {
			t.keepData = &(*e.rows)[0] // a new rows slice looks edited keys up again
		}
	}
	if e.virtual {
		t.Virtual()
	}
	if e.pageSize > 0 {
		// EditableProTable is a ProTable: the total shows by default
		// (pro-components table/utils mergePagination showTotal).
		t.Pagination(&st.page, &st.pageSz).PaginationConfig(func(pg *PaginationW) { pg.ShowTotal() })
	}
	if e.sortable {
		t.DragSort(func(from, to int) {
			fr, fok := at(from)
			tr, tok := at(to)
			fi, ti := fr.index, tr.index
			if !fok || !tok || fi < 0 || ti < 0 {
				return // new rows stay where they are until saved
			}
			*e.rows = tblMoveSlice(*e.rows, fi, ti)
			e.moved = true
			if e.onSort != nil {
				e.onSort(fi, ti)
			}
		})
	}
	enter := e.mode != p2eModeNone && p2eEnterDown(c)
	t.Show()
	e.table = t
	e.preset(t, enter)
}

// snapRows is the bounded table's rowsHook: it copies the rows the table
// is about to build (window [first, last] and the kept rows) for snapAt.
func (e *EditableProTableW[T]) snapRows(first, last int, keeps []int) {
	sn := &e.snap
	sn.on, sn.first = true, first
	n := max(0, last-first+1)
	sn.rows = slices.Grow(sn.rows[:0], n)[:n]
	sn.ok = slices.Grow(sn.ok[:0], n)[:n]
	for j := range n {
		sn.rows[j], sn.ok[j] = e.viewAt(first + j)
	}
	clear(sn.keep)
	for _, k := range keeps {
		if r, ok := e.viewAt(k); ok {
			if sn.keep == nil {
				sn.keep = map[int]p2eEditRow[T]{}
			}
			sn.keep[k] = r
		}
	}
}

// snapAt returns displayed row i of the bounded form: from this frame's
// snapshot once the table took it, else read live (viewAt).
func (e *EditableProTableW[T]) snapAt(i int) (p2eEditRow[T], bool) {
	if sn := &e.snap; sn.on {
		if j := i - sn.first; j >= 0 && j < len(sn.rows) {
			return sn.rows[j], sn.ok[j]
		}
		if r, ok := sn.keep[i]; ok {
			return r, true
		}
	}
	return e.viewAt(i)
}

// tblMoveSlice moves element from to the position of element to.
func tblMoveSlice[T any](s []T, from, to int) []T {
	if from == to || from < 0 || to < 0 || from >= len(s) || to >= len(s) {
		return s
	}
	v := s[from]
	s = slices.Delete(s, from, from+1)
	return slices.Insert(s, to, v)
}

// cell is the editor of one cell of an edited row.
func (e *EditableProTableW[T]) cell(k string, fd *ProField) {
	c, st := e.c, e.st
	draft := st.drafts[k]
	if draft == nil {
		draft = ProValues{}
		st.drafts[k] = draft
	}
	prev := c.itemStatus
	if st.errs[k][fd.Key] != "" {
		c.itemStatus = "error"
	}
	b := Row(c, func() {
		Disabled(c, st.waiting[k] > 0 || st.saving[k], func() { proFieldControl(c, fd, draft) })
	}, layout.Grow(1), layout.MinW(0), layout.Middle())
	c.itemStatus = prev
	if e.mode != p2eModeNone {
		e.edNodes = append(e.edNodes, p2eEdNode{col: fd.Key, valueType: fd.ValueType, n: b.n})
	}
	// Name the control after the column, with Required / Invalid like a
	// FormItem.
	b.n.fill(p2eCellField{label: fd.Label, required: fd.Required, err: st.errs[k][fd.Key]})
}

// p2eCellField labels the editor in a cell like a FormItem does.
type p2eCellField struct {
	label    string
	required bool
	err      string
}

func (f p2eCellField) field() (string, ID, bool, string) { return f.label, 0, f.required, f.err }
func (f p2eCellField) semantics(*a11y.Node)              {}

// actionColumn builds the edit / save / cancel / delete links.
func (e *EditableProTableW[T]) actionColumn(key, title string, width float32, at func(int) (p2eEditRow[T], bool)) TableColumn {
	c := e.c
	if width == 0 {
		width = 150
	}
	return TableColumn{Key: key, Title: title, Width: width, Render: func(i int) {
		r, ok := at(i)
		if !ok {
			return
		}
		st := e.st
		loc := c.Locale()
		if slices.Contains(*e.editKeys(), r.key) {
			busy := st.waiting[r.key] > 0 || st.saving[r.key]
			if busy {
				Icon(c, icon.Loading).Spin().Size(14).Color(c.Theme.Primary.Base)
			}
			Disabled(c, busy, func() {
				if Link(c, loc.Save).Clicked() {
					e.save(r.key)
				}
			})
			if Link(c, loc.Cancel).Clicked() {
				e.stopEdit(r.key)
			}
			if msg := st.errs[r.key][p2eSaveFailed]; msg != "" {
				Tooltip(c, msg, func() { Icon(c, icon.CloseCircle).Size(14).Color(c.Theme.Error.Base) })
			}
			return
		}
		allowed := e.canEdit == nil || e.canEdit(r.row)
		other := !e.multiple && len(*e.editKeys()) > 0
		Disabled(c, !allowed || other, func() {
			if Link(c, loc.Edit).Clicked() {
				e.startEdit(r)
			}
		})
		if !e.noDelete {
			p := Popconfirm(c, loc.EditableTable.DeleteConfirm, "", func() {
				Link(c, loc.Delete)
			})
			if p.Confirmed() {
				e.remove(r)
			}
		}
	}}
}
