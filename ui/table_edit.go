package ui

import (
	"strconv"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// tblEdit is the single cell being edited (cell edit mode).
type tblEdit struct {
	on       bool
	key, col string
	d        tblDraft
	fresh    bool // focus/open the editor on the next build
	edID     ID
	hadFocus bool
}

// tblDraft is an editor's working value.
type tblDraft struct {
	v    string
	num  float64
	date time.Time
	err  string
	init bool
}

func tblWalkLeaves(cols []TableColumn, fn func(col *TableColumn)) {
	for i := range cols {
		if len(cols[i].Children) > 0 {
			tblWalkLeaves(cols[i].Children, fn)
			continue
		}
		fn(&cols[i])
	}
}

// editValue returns a cell's current value for editing and copying.
func editValue(col *TableColumn, row int) string {
	if col.Editor != nil && col.Editor.Value != nil {
		return col.Editor.Value(row)
	}
	if col.Text != nil {
		return col.Text(row)
	}
	return ""
}

func (d *tblDraft) load(col *TableColumn, row int) {
	d.v = editValue(col, row)
	d.num, _ = strconv.ParseFloat(d.v, 64)
	d.date, _ = dtParse(d.v, "2006-01-02", time.UTC) // zero time when empty / invalid
	d.err = ""
	d.init = true
}

func (f *tblFrame) isEditing(r tblRow, col *TableColumn) bool {
	if f.editSet[r.key] {
		return true
	}
	e := &f.st.edit
	return e.on && e.key == r.key && e.col == col.Key
}

// maybeStartEdit starts editing the cell at leaf li if it is editable.
func (f *tblFrame) maybeStartEdit(r tblRow, li int) bool {
	if li < 0 || li >= len(f.leaves) {
		return false
	}
	col := f.leaves[li].col
	if col == nil || col.Editor == nil || col.Editor.Set == nil || col.Editor.Render != nil || f.editSet[r.key] {
		return false
	}
	e := &f.st.edit
	if e.on && e.key == r.key && e.col == col.Key {
		return true
	}
	*e = tblEdit{on: true, key: r.key, col: col.Key, fresh: true}
	e.d.load(col, r.data)
	f.c.invalid = true
	return true
}

// validate checks a draft; it stores and returns the error.
func validateDraft(col *TableColumn, d *tblDraft) bool {
	d.err = ""
	if col.Editor.Kind == "number" {
		d.v = strconv.FormatFloat(d.num, 'f', -1, 64)
		if col.Editor.Precision > 0 {
			d.v = strconv.FormatFloat(d.num, 'f', col.Editor.Precision, 64)
		}
	}
	if col.Editor.Validate != nil {
		d.err = col.Editor.Validate(d.v)
	}
	return d.err == ""
}

// commitCell validates and applies the cell being edited.
func (f *tblFrame) commitCell(r tblRow, col *TableColumn) bool {
	e := &f.st.edit
	if !validateDraft(col, &e.d) {
		f.c.invalid = true
		return false
	}
	if col.Editor.Set != nil && e.d.v != editValue(col, r.data) {
		col.Editor.Set(r.data, e.d.v)
		f.t.editedCh = true
	}
	e.on = false
	f.c.SetFocus(f.focusID())
	f.c.invalid = true
	return true
}

// rowDraft returns the row-edit draft of a cell.
func (st *tblState) rowDraft(key string, col *TableColumn, row int) *tblDraft {
	if st.rowDrafts == nil {
		st.rowDrafts = map[string]map[string]*tblDraft{}
	}
	m := st.rowDrafts[key]
	if m == nil {
		m = map[string]*tblDraft{}
		st.rowDrafts[key] = m
	}
	d := m[col.Key]
	if d == nil {
		d = &tblDraft{}
		d.load(col, row)
		m[col.Key] = d
	}
	return d
}

// editor builds the in-cell editor.
func (f *tblFrame) editor(r tblRow, col *TableColumn, w, h float32) {
	c, st := f.c, f.st
	rowMode := f.editSet[r.key]
	if ed := col.Editor; ed.Render != nil {
		if !rowMode {
			return
		}
		b := Row(c, func() { ed.Render(r.data) }, layout.Grow(1), layout.MinW(0), layout.Middle())
		if ed.Error != nil {
			if msg := ed.Error(r.data); msg != "" {
				f.tip(b.n.id, msg, true)
			}
		}
		return
	}
	e := &st.edit
	d := &e.d
	if rowMode {
		d = st.rowDraft(r.key, col, r.data)
	}
	size := Middle
	if f.rh < 44 {
		size = Small
	}
	status := ""
	if d.err != "" {
		status = "error"
	}
	fresh := !rowMode && e.fresh
	var edID ID
	submitted := false
	kind := col.Editor.Kind
	switch kind {
	case "number":
		in := InputNumber(c, &d.num).Size(size).Width(w).A11yLabel(col.Title)
		if col.Editor.Precision > 0 {
			in.Precision(col.Editor.Precision)
		}
		edID = in.ed.id
	case "select":
		s := Select(c, &d.v, col.Editor.Options).Size(size).Status(status).A11yLabel(col.Title).Layout(layout.Grow(1), layout.MinW(0))
		if fresh {
			s.Open()
		}
		if s.Changed() && !rowMode {
			submitted = true
		}
		edID = s.n.id
	case "date":
		dp := DatePicker(c, &d.date)
		dp.A11yLabel(col.Title)
		dp.n.ln.Width = w
		if fresh {
			dp.Open()
		}
		if dp.Changed() {
			d.v = dtFormat(d.date, "2006-01-02")
			if !rowMode {
				submitted = true
			}
		}
		edID = dp.n.id
	default:
		in := Input(c, &d.v).Size(size).Status(status).A11yLabel(col.Title).Layout(layout.Grow(1), layout.MinW(0))
		edID = in.ID()
		if fresh {
			// Click-to-edit puts the caret at the end instead of selecting all.
			n := len([]rune(d.v))
			in.e.cur, in.e.anc, in.e.wasFocused = n, n, true
		}
	}
	if d.err != "" {
		f.tip(edID, d.err, true)
	}
	if fresh {
		c.SetFocus(edID)
		e.fresh = false
		e.edID = edID
	}
	if c.focus == edID {
		for _, k := range c.keys {
			if k.Down && k.Key == input.KeyEnter {
				submitted = true
			}
		}
	}
	if rowMode {
		return
	}
	focused := c.focus == edID
	if focused {
		e.hadFocus = true
	}
	switch {
	case submitted:
		f.commitCell(r, col)
	case (focused || kind == "select" || kind == "date") && c.Escape():
		e.on = false
		c.SetFocus(f.focusID())
		c.invalid = true
	case e.hadFocus && !focused && (kind == "" || kind == "text" || kind == "number"):
		// Blur commits (a failed validation keeps the editor open).
		if !f.commitCell(r, col) {
			e.hadFocus = false
		}
	}
}

// ---------------------------------------------------------------------------
// Row editing API (use inside an action column's Render).

func (t *TableW) state() *tblState {
	if t.st == nil {
		return nil
	}
	return t.st
}

// IsEditing reports whether row is in row-edit mode.
func (t *TableW) IsEditing(row int) bool {
	return t.editRows != nil && tblHas(*t.editRows, tblKey(t.rowKey, row))
}

// Edit puts row into row-edit mode.
func (t *TableW) Edit(row int) {
	if t.editRows == nil {
		return
	}
	k := tblKey(t.rowKey, row)
	*t.editRows = tblToggle(*t.editRows, k, true)
	if st := t.state(); st != nil {
		delete(st.rowDrafts, k)
	}
	t.c.invalid = true
}

// Save validates the row's editors and commits them; it returns false and
// shows the errors when a value is invalid.
func (t *TableW) Save(row int) bool {
	st := t.state()
	if st == nil || t.editRows == nil {
		return false
	}
	k := tblKey(t.rowKey, row)
	ok := true
	var cols []*TableColumn
	tblWalkLeaves(t.cols, func(col *TableColumn) {
		if col.Editor == nil {
			return
		}
		cols = append(cols, col)
		if !validateDraft(col, st.rowDraft(k, col, row)) {
			ok = false
		}
	})
	t.c.invalid = true
	if !ok {
		return false
	}
	for _, col := range cols {
		d := st.rowDraft(k, col, row)
		if col.Editor.Set != nil && d.v != editValue(col, row) {
			col.Editor.Set(row, d.v)
		}
	}
	delete(st.rowDrafts, k)
	*t.editRows = tblToggle(*t.editRows, k, false)
	t.editedCh = true
	return true
}

// Cancel leaves row-edit mode without saving.
func (t *TableW) Cancel(row int) {
	if t.editRows == nil {
		return
	}
	k := tblKey(t.rowKey, row)
	*t.editRows = tblToggle(*t.editRows, k, false)
	if st := t.state(); st != nil {
		delete(st.rowDrafts, k)
	}
	t.c.invalid = true
}
