package ui

import (
	"math"
	"strconv"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Transposed tables: records become columns and fields become rows, for
// side-by-side comparison of a few records (plans, products, versions).
//
// The data model does not change: cols still describes the fields and rows
// counts the records, and every Text / Render callback still receives a
// record index. Show builds a derived table whose rows are the fields and
// whose columns are a header column (the field titles) followed by one
// column per record, then runs the normal table pipeline on it; the cell
// builder asks tblTranspose which field column and record a body cell
// shows (tpCell).

// tblTranspose is the transposition setup of a table. Modifiers create it
// lazily; Transposed switches it on.
type tblTranspose struct {
	on      bool
	view    bool // set on the derived table Show builds
	header  func(row int) string
	colW    float32 // record column width (default tpColW)
	headW   float32 // header column width (0: fit the widest title)
	unfixed bool    // header column scrolls with the records

	// Derived table only.
	recs   int           // record count of the source table
	fields []TableColumn // the source's leaf fields, display copies
}

// tpColW is the default record column width.
const tpColW = 160

func (t *TableW) tpSetup() *tblTranspose {
	if t.tp == nil {
		t.tp = &tblTranspose{}
	}
	return t.tp
}

// Transposed swaps the axes for comparing records side by side: every
// TableColumn becomes a row and every record a column. The data model is
// unchanged (cols are the fields, rows the record count, callbacks get the
// record index).
//
// The first column is the header column: it shows each field's Title in
// header style (Table HeaderBg / HeaderColor / HeaderFontWeight tokens), is
// a row header for assistive technology and is pinned to the inline start
// (the right edge in RTL) unless TransposedHeaderFixed(false). Each record
// column is at least TransposedColumnWidth wide (default 160; record
// columns share the width of a wider table equally) and shows the field's Text or Render for that record,
// aligned by the field's Align. The header row shows TransposedHeader's
// titles; without TransposedHeader there is no header row. Hidden fields
// are skipped and header groups (Children) are flattened to their leaves.
//
// Transposition turns off, without error, every feature that addresses
// records as rows or fields as columns: sorting and filtering (Sort,
// MultiSort, Filters, column Sortable / Filters), row selection (Selection,
// SelectedKeys, Selections), expandable rows, Tree, Span, DragSort,
// editing (column Editor, EditableRows), Resizable, ColumnState, Summary,
// Pagination / ServerPagination, RowClass, HighlightCurrentRow, Sticky /
// StickySummary and the Wrap of field columns (cells stay one line; Ellipsis
// still works). The change queries (SortChanged, ...) report false.
// RowClicked / RowDoubleClicked return the record whose column was clicked
// (or -1 on the header column) and DisplayedRows lists every record.
//
// ScrollX scrolls many records sideways (the header column stays pinned);
// ScrollY bounds the body height. Virtual is ignored: there are as many
// rows as fields (usually few) and record columns are not virtualized.
func (t *TableW) Transposed() *TableW { t.tpSetup().on = true; return t }

// TransposedHeader titles the record columns of a Transposed table: title
// returns the column title of record row. Without it the transposed table
// has no header row.
func (t *TableW) TransposedHeader(title func(row int) string) *TableW {
	t.tpSetup().header = title
	return t
}

// TransposedColumnWidth sets the width of every record column of a
// Transposed table (default 160). It is a minimum: in a wider table the
// record columns share the free width equally (the header column keeps
// its width); when they do not fit, the body scrolls sideways.
func (t *TableW) TransposedColumnWidth(px float32) *TableW { t.tpSetup().colW = px; return t }

// TransposedHeaderWidth sets the width of the header column of a
// Transposed table (default: the widest field title plus cell padding,
// at least 80).
func (t *TableW) TransposedHeaderWidth(px float32) *TableW { t.tpSetup().headW = px; return t }

// TransposedHeaderFixed pins the header column of a Transposed table to
// the inline start while the records scroll sideways (default on; the right
// edge in RTL).
func (t *TableW) TransposedHeaderFixed(on bool) *TableW { t.tpSetup().unfixed = !on; return t }

// tpFields lists the displayed leaf fields of cols: hidden columns are
// skipped and header groups flattened. The copies drop what a transposed
// cell does not support (editing, wrapping, sorting, filtering, pinning).
func tpFields(cols []TableColumn, out []TableColumn) []TableColumn {
	for _, col := range cols {
		if col.Hidden {
			continue
		}
		if len(col.Children) > 0 {
			out = tpFields(col.Children, out)
			continue
		}
		col.Editor, col.Wrap, col.Fixed = nil, false, ""
		col.Sortable, col.Compare, col.SortMultiple, col.DefaultSort = false, nil, 0, SortNone
		col.Filters, col.OnFilter, col.FilterPanel = nil, nil, nil
		out = append(out, col)
	}
	return out
}

// showTransposed builds the derived table and maps its results back.
func (t *TableW) showTransposed() *TableW {
	c, src := t.c, t.tp
	fields := tpFields(t.cols, nil)
	tp := &tblTranspose{on: true, view: true, header: src.header, colW: src.colW, headW: src.headW, unfixed: src.unfixed, recs: t.rows, fields: fields}
	if tp.colW <= 0 {
		tp.colW = tpColW
	}
	v := &TableW{c: c, rows: len(fields), clickRow: -1, dblRow: -1, indent: t.indent, tp: tp,
		bordered: t.bordered, striped: t.striped, size: t.size, sizeSet: t.sizeSet,
		maxH: t.maxH, scrollW: t.scrollW, rowH: t.rowH, loading: t.loading, noHover: t.noHover,
		emptyText: t.emptyText, a11yName: t.a11yName, revision: t.revision}
	if t.rows == 0 {
		v.rows = 0 // no records: the empty state
	}
	v.rowKey = func(fi int) string {
		if k := fields[fi].Key; k != "" {
			return k
		}
		return strconv.Itoa(fi)
	}
	if tp.headW <= 0 {
		tp.headW = tp.fitHeader(v)
	}
	v.cols = tp.columns()
	v.Show()
	t.id, t.st, t.fr = v.id, v.st, v.fr
	t.clickRow, t.dblRow = tp.record(v, v.clickRow), tp.record(v, v.dblRow)
	t.vis = make([]int, t.rows)
	for i := range t.vis {
		t.vis[i] = i
	}
	return t
}

// fitHeader is the default header column width: the widest field title in
// the header font plus the cell padding.
func (tp *tblTranspose) fitHeader(v *TableW) float32 {
	th := v.c.ThemeFor(theme.CompTable)
	f := &tblFrame{t: v, c: v.c, th: th, tk: &th.Components.Table}
	f.metrics()
	st := f.font(themeWeight(f.tk.HeaderFontWeight))
	w := float32(0)
	for i := range tp.fields {
		w = max(w, text.Width(tp.fields[i].Title, st))
	}
	return max(80, float32(math.Ceil(float64(w+2*f.ph+1))))
}

// columns builds the derived table's columns: the header column, then one
// column per record. Their Text callbacks (keyed by field index) serve
// copying; body cells resolve the field column through tpCell.
func (tp *tblTranspose) columns() []TableColumn {
	fields := tp.fields
	cols := make([]TableColumn, 0, tp.recs+1)
	head := TableColumn{Key: "__tp_head", Width: tp.headW, Text: func(fi int) string { return fields[fi].Title }}
	if !tp.unfixed {
		head.Fixed = FixedLeft
	}
	cols = append(cols, head)
	for r := 0; r < tp.recs; r++ {
		// Record columns share the free width equally, never below colW
		// (wider than the table, the body scrolls); the header column
		// keeps its width.
		col := TableColumn{Key: "__tp_" + strconv.Itoa(r), MinWidth: tp.colW, Grow: 1, Text: func(fi int) string {
			if fields[fi].Text != nil {
				return fields[fi].Text(r)
			}
			return ""
		}}
		if tp.header != nil {
			col.Title = tp.header(r)
		}
		cols = append(cols, col)
	}
	return cols
}

// tpCell resolves a body cell of the derived table: the column whose
// callbacks it calls, the argument (record index, or the field index for
// the header column) and whether it is a header-column cell.
func (tp *tblTranspose) tpCell(lf *tblLeaf, field int) (*TableColumn, int, bool) {
	if lf.di == 0 {
		return lf.col, field, true
	}
	return &tp.fields[field], lf.di - 1, false
}

// record maps a row click of the derived table to the record of the
// active (clicked) column, or -1.
func (tp *tblTranspose) record(v *TableW, clicked int) int {
	if clicked < 0 || v.fr == nil || v.st == nil || !v.st.actOn {
		return -1
	}
	li := v.st.actCol
	if li < 0 || li >= len(v.fr.leaves) || v.fr.leaves[li].di < 1 {
		return -1
	}
	return v.fr.leaves[li].di - 1
}

// tpHeadPaint paints a header-column cell: the header background (over the
// container color, as it may be pinned), the row line and the separator
// on its end edge.
func (f *tblFrame) tpHeadPaint(l *paint.List, r geom.Rect, pos int) {
	var rad geom.Radii
	if hr := f.tk.HeaderBorderRadius; hr > 0 && f.depth == 0 && pos == 0 && f.cx(0)-f.sx <= 0.5 {
		// No header row: the cell takes the table's rounded start corner.
		if f.rtl {
			rad.TR = hr
		} else {
			rad.TL = hr
		}
	}
	l.Fill(r, rad, f.th.BgContainer)
	l.Fill(r, rad, f.headerBg())
	l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, f.spl)
	if !f.t.bordered {
		// Bordered tables draw every cell's end edge already.
		l.Fill(geom.Rect{X: f.inl(r).X(r.W-1, 1), Y: r.Y, W: 1, H: r.H}, geom.Radii{}, f.spl)
	}
}
