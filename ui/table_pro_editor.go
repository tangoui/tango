package ui

import (
	"slices"

	"github.com/tangoui/tango/ui/input"
)

// p2eMode is the editing preset of an EditableProTableW.
type p2eMode int8

const (
	p2eModeNone p2eMode = iota // EditableProTable: action column links
	p2eModeCell                // CellEditorTable: one cell at a time
	p2eModeRow                 // RowEditorTable: one row at a time
)

// p2eEdNode is an editor cell built this frame (focus tracking).
type p2eEdNode struct {
	col       string
	valueType string
	n         *node
}

// p2eEnterDown reports an Enter press this frame (read before the editors
// consume the keys).
func p2eEnterDown(c *Context) bool {
	for _, k := range c.keys {
		if k.Down && k.Key == input.KeyEnter {
			return true
		}
	}
	return false
}

// tblActivated reports the cell the user activated in the last Show: a
// double click, or Enter on the keyboard-active cell (the table does not
// edit it itself because its editor is a Render editor or none). It
// returns the data row and the column key.
func tblActivated(t *TableW) (int, string, bool) {
	st, fr := t.st, t.fr
	if st == nil || fr == nil || !st.actOn || st.actCol < 0 || st.actCol >= len(fr.leaves) {
		return -1, "", false
	}
	col := fr.leaves[st.actCol].col
	if col == nil {
		return -1, "", false
	}
	switch {
	case t.dblRow >= 0:
		return t.dblRow, col.Key, true
	case t.clickRow >= 0 && st.kbd:
		return t.clickRow, col.Key, true
	}
	return -1, "", false
}

// preset runs the CellEditorTable / RowEditorTable interaction after the
// inner table was built: Enter in an editor saves, Escape cancels, focus
// leaving the edited cell (row) saves, and a double click or Enter on a
// cell starts editing it (its row).
func (e *EditableProTableW[T]) preset(t *TableW, enter bool) {
	if e.mode == p2eModeNone {
		return
	}
	c, st := e.c, e.st
	if ks := *e.editKeys(); len(ks) > 0 {
		k := ks[0]
		within := false
		submit := false
		for _, en := range e.edNodes {
			if c.focusWithin(en.n) {
				within = true
				submit = enter && p2eEnterCommits(en.valueType)
				break
			}
		}
		busy := e.Validating(k)
		switch {
		case st.fresh:
			e.focusEditor()
		case (within || busy && (c.focus == 0 || c.focus == t.fr.focusID())) && c.Escape():
			e.stopEdit(k)
			c.SetFocus(t.fr.focusID())
		case within && submit && !busy:
			e.save(k)
			if !e.Editing(k) {
				c.SetFocus(t.fr.focusID())
			}
		case st.hadFocus && !within && !busy:
			// Blur saves; a failed validation keeps the editor open
			// until the next focus visit.
			st.hadFocus = false
			e.save(k)
		}
		if within && e.Editing(k) {
			st.hadFocus = true
		}
	}
	if e.disabled {
		return
	}
	row, col, ok := tblActivated(t)
	if !ok || row < 0 || row >= len(e.view) {
		return
	}
	r := e.view[row]
	if e.canEdit != nil && !e.canEdit(r.row) {
		return
	}
	if e.mode == p2eModeCell {
		pc := e.column(col)
		if pc == nil || pc.Editor == nil || pc.Set == nil || pc.HideInTable {
			return
		}
		if e.Editing(r.key) && st.cellCol == col {
			return
		}
	} else if e.Editing(r.key) {
		return
	}
	// Leave the open editor first (saved); stay when it cannot be saved.
	for _, k := range slices.Clone(*e.editKeys()) {
		e.save(k)
		if e.Editing(k) {
			return
		}
	}
	if e.mode == p2eModeCell {
		st.cellCol = col
	}
	e.startEdit(r)
	st.focusCol, st.fresh, st.hadFocus = col, true, false
	c.invalid = true
}

// Enter belongs to popup/choice controls (and multiline text), not the
// table. Those controls save on blur or via the explicit Save method.
func p2eEnterCommits(valueType string) bool {
	switch valueType {
	case "", ValueText, ValuePassword, ValueDigit, ValueMoney, ValuePercent, ValueProgress, ValueSecond, ValueIndex:
		return true
	}
	return false
}

// column returns the column with key k.
func (e *EditableProTableW[T]) column(k string) *ProColumn[T] {
	for i := range e.cols {
		if e.cols[i].Key == k {
			return &e.cols[i]
		}
	}
	return nil
}

// focusEditor moves focus into the editor of the activated column (or
// the first editor of the row) once its cells exist.
func (e *EditableProTableW[T]) focusEditor() {
	st := e.st
	var target *node
	for _, en := range e.edNodes {
		if target == nil || en.col == st.focusCol {
			if id := navFirstFocusableNode(en.n); id != nil {
				target = id
				if en.col == st.focusCol {
					break
				}
			}
		}
	}
	if target == nil {
		return // not built yet: retry next frame
	}
	st.fresh = false
	e.c.SetFocus(target.id)
}

// ---------------------------------------------------------------------------
// CellEditorTable

// CellEditorTableW is an editable table that edits one cell at a time
// (Pro Components v3 CellEditorTable): double-click a cell of a column
// with an Editor (or press Enter on the keyboard-active cell) to edit it;
// Enter or moving focus away validates that cell and saves the row, Escape
// cancels and drops the draft. Only the edited column is validated. It is
// an EditableProTable preset (same editors, validation and OnSave flow),
// bordered and without pagination by default.
type CellEditorTableW[T any] struct {
	_ [0]func()
	e *EditableProTableW[T]
}

// CellEditorTable starts a cell-editing table over *rows; chain options,
// then Show.
//
// ConfigProvider.Defaults("CellEditorTable", ...) keys: "bordered" (bool,
// default true), "size", "pagination" (page size or Props{"pageSize": n};
// default none).
func CellEditorTable[T any](c *Context, cols []ProColumn[T], rows *[]T) *CellEditorTableW[T] {
	return &CellEditorTableW[T]{e: p2ePreset(c, cols, rows, p2eModeCell, "CellEditorTable")}
}

// p2ePreset configures an EditableProTableW for a preset: bordered, no
// pagination and no action column unless the preset's Defaults say so.
func p2ePreset[T any](c *Context, cols []ProColumn[T], rows *[]T, mode p2eMode, comp string) *EditableProTableW[T] {
	e := EditableProTable(c, cols, rows)
	e.mode, e.noActions = mode, true
	e.bordered = Prop(c, comp, "bordered", true)
	e.pageSize = 0
	if _, ok := c.ConfigProp(comp, "pagination"); ok {
		if n, on := proPropPageSize(c, comp, 0); on {
			e.pageSize = max(1, n)
		}
	}
	if sz, ok := proPropSize(c, comp); ok {
		e.size, e.sizeSet = sz, true
	}
	return e
}

// Title sets the toolbar title.
func (w *CellEditorTableW[T]) Title(s string) *CellEditorTableW[T] { w.e.Title(s); return w }

// ToolBar adds actions at the right of the toolbar.
func (w *CellEditorTableW[T]) ToolBar(fn func()) *CellEditorTableW[T] { w.e.ToolBar(fn); return w }

// RowKey gives rows stable keys (default: the row index).
func (w *CellEditorTableW[T]) RowKey(f func(T) string) *CellEditorTableW[T] {
	w.e.RowKey(f)
	return w
}

// RowEditable limits which rows can be edited.
func (w *CellEditorTableW[T]) RowEditable(fn func(row T) bool) *CellEditorTableW[T] {
	w.e.RowEditable(fn)
	return w
}

// OnSave receives the row with the validated cell applied before it is
// written back (see EditableProTableW.OnSave).
func (w *CellEditorTableW[T]) OnSave(fn func(row T, index int, done func(error))) *CellEditorTableW[T] {
	w.e.OnSave(fn)
	return w
}

// Bordered turns the cell borders on or off (default on).
func (w *CellEditorTableW[T]) Bordered(on bool) *CellEditorTableW[T] { w.e.bordered = on; return w }

// Size sets the density.
func (w *CellEditorTableW[T]) Size(s ControlSize) *CellEditorTableW[T] { w.e.Size(s); return w }

// Pagination pages the rows locally, n per page (default: no pagination).
func (w *CellEditorTableW[T]) Pagination(n int) *CellEditorTableW[T] { w.e.Pagination(n); return w }

// ScrollX sets the table's scroll width.
func (w *CellEditorTableW[T]) ScrollX(px float32) *CellEditorTableW[T] { w.e.ScrollX(px); return w }

// Show builds the table.
func (w *CellEditorTableW[T]) Show() *CellEditorTableW[T] { w.e.Show(); return w }

// Edit starts editing the cell of row key k in column col (valid after
// Show; ignored for a column without an Editor).
func (w *CellEditorTableW[T]) Edit(k, col string) {
	e := w.e
	if e.st == nil || e.disabled {
		return
	}
	pc := e.column(col)
	r, ok := e.find(k)
	if pc == nil || pc.Editor == nil || pc.Set == nil || pc.HideInTable || !ok || (e.canEdit != nil && !e.canEdit(r.row)) {
		return
	}
	for _, ek := range slices.Clone(*e.editKeys()) {
		e.save(ek)
		if e.Editing(ek) {
			return
		}
	}
	e.st.cellCol = col
	e.startEdit(r)
	e.st.focusCol, e.st.fresh, e.st.hadFocus = col, true, false
}

// Save validates the edited cell and saves its row (as Enter does).
func (w *CellEditorTableW[T]) Save() {
	if k, _ := w.EditingCell(); k != "" {
		w.e.Save(k)
	}
}

// Cancel leaves the edited cell without saving (as Escape does).
func (w *CellEditorTableW[T]) Cancel() {
	if k, _ := w.EditingCell(); k != "" {
		w.e.Cancel(k)
	}
}

// EditingCell returns the row key and column key of the edited cell
// ("" when none).
func (w *CellEditorTableW[T]) EditingCell() (key, col string) {
	e := w.e
	if e.st == nil {
		return "", ""
	}
	if ks := *e.editKeys(); len(ks) > 0 {
		return ks[0], e.st.cellCol
	}
	return "", ""
}

// Errors returns the validation messages of row k by column key.
func (w *CellEditorTableW[T]) Errors(k string) map[string]string { return w.e.Errors(k) }

// Validating reports async validation or a save in flight for row k.
func (w *CellEditorTableW[T]) Validating(k string) bool { return w.e.Validating(k) }

// Saved reports a row written back in the last Show.
func (w *CellEditorTableW[T]) Saved() bool { return w.e.Saved() }

// Changed reports a change of *rows in the last Show.
func (w *CellEditorTableW[T]) Changed() bool { return w.e.Changed() }

// Table returns the inner table of the last Show.
func (w *CellEditorTableW[T]) Table() *TableW { return w.e.Table() }

// ID returns the widget ID (valid after Show).
func (w *CellEditorTableW[T]) ID() ID { return w.e.ID() }

// ---------------------------------------------------------------------------
// RowEditorTable

// RowEditorTableW is an editable table that edits one whole row at a time
// (Pro Components v3 RowEditorTable): double-click a row (or press Enter on
// one of its cells) to edit every editable cell of it; Enter or moving
// focus out of the row validates all of its editors and saves, Escape
// cancels and drops the drafts (a new row is removed). It is an
// EditableProTable preset, bordered and without pagination by default.
type RowEditorTableW[T any] struct {
	_ [0]func()
	e *EditableProTableW[T]
}

// RowEditorTable starts a row-editing table over *rows; chain options,
// then Show.
//
// ConfigProvider.Defaults("RowEditorTable", ...) keys: "bordered" (bool,
// default true), "size", "pagination" (page size or Props{"pageSize": n};
// default none).
func RowEditorTable[T any](c *Context, cols []ProColumn[T], rows *[]T) *RowEditorTableW[T] {
	return &RowEditorTableW[T]{e: p2ePreset(c, cols, rows, p2eModeRow, "RowEditorTable")}
}

// Title sets the toolbar title.
func (w *RowEditorTableW[T]) Title(s string) *RowEditorTableW[T] { w.e.Title(s); return w }

// ToolBar adds actions at the right of the toolbar.
func (w *RowEditorTableW[T]) ToolBar(fn func()) *RowEditorTableW[T] { w.e.ToolBar(fn); return w }

// RowKey gives rows stable keys (default: the row index).
func (w *RowEditorTableW[T]) RowKey(f func(T) string) *RowEditorTableW[T] {
	w.e.RowKey(f)
	return w
}

// RowEditable limits which rows can be edited.
func (w *RowEditorTableW[T]) RowEditable(fn func(row T) bool) *RowEditorTableW[T] {
	w.e.RowEditable(fn)
	return w
}

// NewRow shows the "add a row" button (see EditableProTableW.NewRow); the
// new row opens in edit mode with focus in its first editor.
func (w *RowEditorTableW[T]) NewRow(fn func() T) *RowEditorTableW[T] { w.e.NewRow(fn); return w }

// AddAtTop puts new rows (and the add button) above the data.
func (w *RowEditorTableW[T]) AddAtTop() *RowEditorTableW[T] { w.e.AddAtTop(); return w }

// MaxLength hides the add button once there are n rows.
func (w *RowEditorTableW[T]) MaxLength(n int) *RowEditorTableW[T] { w.e.MaxLength(n); return w }

// OnSave receives a validated row (index -1 for a new row) before it is
// written back (see EditableProTableW.OnSave).
func (w *RowEditorTableW[T]) OnSave(fn func(row T, index int, done func(error))) *RowEditorTableW[T] {
	w.e.OnSave(fn)
	return w
}

// Bordered turns the cell borders on or off (default on).
func (w *RowEditorTableW[T]) Bordered(on bool) *RowEditorTableW[T] { w.e.bordered = on; return w }

// Size sets the density.
func (w *RowEditorTableW[T]) Size(s ControlSize) *RowEditorTableW[T] { w.e.Size(s); return w }

// Pagination pages the rows locally, n per page (default: no pagination).
func (w *RowEditorTableW[T]) Pagination(n int) *RowEditorTableW[T] { w.e.Pagination(n); return w }

// ScrollX sets the table's scroll width.
func (w *RowEditorTableW[T]) ScrollX(px float32) *RowEditorTableW[T] { w.e.ScrollX(px); return w }

// Show builds the table.
func (w *RowEditorTableW[T]) Show() *RowEditorTableW[T] {
	e := w.e
	e.Show()
	return w
}

// Edit starts editing the row with key k (valid after Show).
func (w *RowEditorTableW[T]) Edit(k string) {
	e := w.e
	r, ok := e.find(k)
	if e.st == nil || e.disabled || !ok || e.Editing(k) || (e.canEdit != nil && !e.canEdit(r.row)) {
		return
	}
	for _, ek := range slices.Clone(*e.editKeys()) {
		e.save(ek)
		if e.Editing(ek) {
			return
		}
	}
	e.startEdit(r)
	e.st.focusCol, e.st.fresh, e.st.hadFocus = "", true, false
}

// AddRow adds a new draft row in edit mode and returns its key ("" when
// NewRow is not set or another row is being edited).
func (w *RowEditorTableW[T]) AddRow() string {
	return w.e.AddRow()
}

// Save validates the edited row and saves it (as Enter does).
func (w *RowEditorTableW[T]) Save() {
	if k := w.EditingKey(); k != "" {
		w.e.Save(k)
	}
}

// Cancel leaves row editing without saving (as Escape does).
func (w *RowEditorTableW[T]) Cancel() {
	if k := w.EditingKey(); k != "" {
		w.e.Cancel(k)
	}
}

// EditingKey returns the key of the edited row ("" when none).
func (w *RowEditorTableW[T]) EditingKey() string {
	if w.e.st == nil {
		return ""
	}
	if ks := *w.e.editKeys(); len(ks) > 0 {
		return ks[0]
	}
	return ""
}

// Errors returns the validation messages of row k by column key.
func (w *RowEditorTableW[T]) Errors(k string) map[string]string { return w.e.Errors(k) }

// Validating reports async validation or a save in flight for row k.
func (w *RowEditorTableW[T]) Validating(k string) bool { return w.e.Validating(k) }

// Saved reports a row written back in the last Show.
func (w *RowEditorTableW[T]) Saved() bool { return w.e.Saved() }

// Changed reports a change of *rows in the last Show.
func (w *RowEditorTableW[T]) Changed() bool { return w.e.Changed() }

// Table returns the inner table of the last Show.
func (w *RowEditorTableW[T]) Table() *TableW { return w.e.Table() }

// ID returns the widget ID (valid after Show).
func (w *RowEditorTableW[T]) ID() ID { return w.e.ID() }
