package ui

import "github.com/tangoui/tango/ui/theme"

// This file holds the public API of Table: column and state types plus the
// builder modifiers. The implementation is split across table_*.go:
//
//	table_model.go   pure logic: columns, widths, sorting, filtering, paging,
//	                 tree flattening, spans, virtual ranges (unit tested)
//	table_show.go    frame orchestration and retained state
//	table_header.go  header cells, sorters, filter dropdowns, resizing
//	table_body.go    rows, cells, fixed columns, expand/tree, summary, drag
//	table_edit.go    editable cells and row editing
//	table_keys.go    keyboard navigation and copy
//	table_pro.go     ProTable: query form, toolbar, value types, requests

// SortOrder is a column sort direction.
//
// Append-only: never reorder or insert; the zero value stays SortNone.
type SortOrder uint8

const (
	SortNone SortOrder = iota
	SortAsc
	SortDesc
)

// SortState is a column's sort, owned by the app (controlled) or by the
// table (uncontrolled).
type SortState struct {
	Column string // TableColumn.Key
	Order  SortOrder
}

// Fixed column sides (TableColumn.Fixed). They are logical, like Ant's
// RTL table: "left" is the start side (the right edge in an RTL scope) and
// "right" the end side.
const (
	FixedLeft  = "left"
	FixedRight = "right"
)

// TableColumn describes one column. Only Key/Title and one of Text/Render
// are needed; everything else is optional.
type TableColumn struct {
	Key   string
	Title string
	// Width fixes the width; 0 shares the remaining space by Grow (default
	// 1), never below MinWidth (default 80).
	Width    float32
	Grow     float32
	MinWidth float32
	// Align aligns the cell and header text: AlignLeft / AlignRight
	// (AlignStart / AlignEnd) follow the direction, so an end-aligned
	// numeric column lines up on the left in RTL; AlignPhysicalLeft /
	// AlignPhysicalRight never flip.
	Align TextAlign
	// Fixed pins the column ("left" | "right": start | end, see FixedLeft)
	// while scrolling sideways.
	Fixed string
	// Ellipsis shows the full text in a tooltip when a truncated cell is
	// hovered (text cells stay on one line).
	Ellipsis bool
	// Wrap lets the cell text wrap onto several lines; rows grow to fit
	// their tallest cell (measured, also under virtual scrolling). Render
	// cells of a Wrap column wrap their widgets and are measured after
	// layout. Wrap and Ellipsis are exclusive: Wrap wins.
	Wrap bool
	// Hidden hides the column until it is enabled in the column settings.
	Hidden bool

	// Sortable shows the sorter. With Compare the table sorts locally;
	// without it the app sorts (server-side) using the reported SortState.
	Sortable bool
	Compare  func(a, b int) int
	// SortMultiple > 0 lets this column take part in multi-column sorting;
	// a higher value has a higher priority.
	SortMultiple int
	// DefaultSort is the initial order when the table owns its sort state.
	DefaultSort SortOrder
	// SortDirections lists the orders a sorter click cycles through
	// (Ant Design sortDirections, Element Plus sort-orders); after the
	// last one the column returns to unsorted. Empty: the table's
	// SortDirections, by default ascending, descending.
	SortDirections []SortOrder

	// Filters lists the values of the filter dropdown. OnFilter filters
	// locally (a row passes if it matches any selected value); without it
	// the app filters using the reported filter state.
	Filters      []Option
	FilterSearch bool // search box above the options
	FilterSingle bool // radio instead of checkboxes
	OnFilter     func(row int, value string) bool
	// FilterPanel replaces the option list with a custom panel.
	FilterPanel func(p *TableFilterPanel)
	// DefaultFilter is the initial filter of the column when the table
	// owns its filter state, or when the bound Filters map has no entry
	// for the column on the first frame (Ant Design defaultFilteredValue).
	DefaultFilter []string
	// FilterResetToDefault makes the filter's reset button restore
	// DefaultFilter instead of clearing it (Ant Design
	// filterResetToDefaultFilteredValue).
	FilterResetToDefault bool

	// Text returns the cell text; Render, if set, builds custom content
	// (any widget: tags, badges, buttons, progress, avatars).
	Text   func(row int) string
	Render func(row int)

	// Children makes this a header group (multi-level header).
	Children []TableColumn

	// Editor makes the cells editable.
	Editor *TableEditor
}

// TableEditor makes a column's cells editable in place.
type TableEditor struct {
	Kind      string // "text" (default) | "number" | "select" | "date"
	Options   []Option
	Precision int
	Value     func(row int) string    // current value (default: column Text)
	Set       func(row int, v string) // commit
	// Validate returns an error message for an invalid value.
	Validate func(v string) string
	// Render builds a custom editor for row-edit mode (EditableRows) in
	// place of Kind, bound to the app's own draft; Error returns the
	// message shown in a tip under it ("" when valid). EditableProTable
	// uses it for ProField controls. Without Set the cell is not
	// click-to-edit.
	Render func(row int)
	Error  func(row int) string
}

// TableSpan identifies a body cell for Span: Row is the data index, Pos the
// position among the displayed rows, Col the index of the leaf data column.
type TableSpan struct {
	Row, Pos, Col int
	Key           string // column key
}

// TableColumnState is the user-adjustable column layout: order, visibility
// and resized widths. ProTable's column settings edit it.
type TableColumnState struct {
	Order  []string
	Hidden map[string]bool
	Widths map[string]float32
	// Fixed overrides a top-level column's Fixed side: FixedLeft,
	// FixedRight or "none" (unpinned).
	Fixed map[string]string
}

// FixedNone unpins a column in TableColumnState.Fixed.
const FixedNone = "none"

// TableFilterPanel is passed to custom filter panels.
type TableFilterPanel struct {
	_ [0]func()

	c       *Context
	Values  *[]string // draft values; Confirm applies them
	confirm bool
	reset   bool
	close   bool
}

// Context returns the build context.
func (p *TableFilterPanel) Context() *Context { return p.c }

// Confirm applies the draft values and closes the panel.
func (p *TableFilterPanel) Confirm() { p.confirm = true }

// Reset clears the filter and closes the panel.
func (p *TableFilterPanel) Reset() { p.reset = true }

// Close closes the panel without applying.
func (p *TableFilterPanel) Close() { p.close = true }

// TableW is a data table (Ant Design Table / Element Plus Table + Table V2).
//
//	ui.Table(c, cols, len(data)).Selection(&sel).Sort(&sort).ScrollY(400).Show()
//
// Rows are addressed by data index 0..rows-1 (or the ids given to Tree);
// column callbacks receive that index, so local sorting and filtering never
// change what a callback sees.
type TableW struct {
	_ [0]func()

	c    *Context
	cols []TableColumn
	rows int

	bordered, striped bool
	size              ControlSize
	sizeSet           bool
	maxH              float32
	scrollW           float32
	rowH              float32
	virtual           bool
	loading           bool
	resizable         bool
	noHover           bool
	highlight         bool
	emptyText         string
	rowKey            func(int) string
	revision          uint64

	sort      *SortState
	multiSort *[]SortState
	filters   *map[string][]string

	selection   *[]int
	selKeys     *[]string
	single      bool
	selDisabled func(int) bool
	hideAll     bool
	strict      bool // tree selection without parent/child linkage

	sticky    bool
	stickyTop float32
	stickySum bool
	stickyBot float32
	stickyIn  ID

	expRender  func(row int)
	expKeys    *[]string
	expAllow   func(row int) bool
	treeRoots  []int
	treeKids   func(row int) []int
	treeHas    func(row int) bool
	treeLoad   func(row int)
	treeOpen   *[]string
	indent     float32
	span       func(TableSpan) (rowSpan, colSpan int)
	summary    []func(col string, rows []int) string
	sumFixed   bool
	dragSort   func(from, to int)
	editRows   *[]string
	colState   *TableColumnState
	a11yName   string
	page       *int
	pageSize   *int
	serverPage bool
	total      int
	pageSizes  []int
	rowStyle   func(row int) RowStyle
	selItems   []TableSelection
	selMenu    bool
	tp         *tblTranspose // Transposed (table_transpose.go)

	// Ant Design / Element Plus options added with the spec alignment.
	sortDirs      []SortOrder
	preserveSel   bool
	selColW       float32
	selFixed      bool
	expandByClick bool
	hideExpandCol bool
	expandAll     bool
	noHeader      bool
	headH         float32
	title, footer func()
	pagePlace     []string
	pageCfg       func(p *PaginationW)
	curKey        *string
	showSum       bool
	ellipsisAll   bool
	sortTip       bool
	scrollTop     bool // scroll the body to the first row on page / sort / filter changes

	// Results of the last Show.
	sortedCh, filterCh, selCh, expCh, pageCh, resizedCh, movedCh, editedCh bool
	curCh                                                                  bool
	clickRow                                                               int
	dblRow                                                                 int

	st  *tblState
	fr  *tblFrame
	id  ID
	vis []int // data rows displayed (current page), for callbacks
}

// Table starts a table over rows data rows; chain modifiers, then Show.
func Table(c *Context, cols []TableColumn, rows int) *TableW {
	t := &TableW{c: c, cols: cols, rows: rows, clickRow: -1, dblRow: -1, indent: 15}
	t.size, t.sizeSet = c.ComponentSize()
	// ConfigProvider.Defaults("Table", ...) keys: "size" ("small" |
	// "middle" | "large" or a ControlSize), "bordered" (bool), "striped"
	// / "stripe" (Element, bool).
	if s, ok := dataPropSize(c, "Table"); ok {
		t.size, t.sizeSet = s, true
	}
	t.bordered = Prop(c, "Table", "bordered", false)
	t.striped = Prop(c, "Table", "striped", Prop(c, "Table", "stripe", false))
	t.tableDefaults()
	return t
}

// RowStyle styles one body row (RowClass). Zero fields keep the default
// look; hover, selection and the current-row highlight still win over Bg.
type RowStyle struct {

	// Status tints the row with a palette background ("success",
	// "warning", "error", "info", "processing"), like Element Plus'
	// success-row / warning-row classes.
	Status string
	// Bg sets the row background (wins over Status).
	Bg theme.Color
	// Text colors the text of plain (Text) cells; Render cells style
	// their own widgets.
	Text theme.Color
	// Bold draws plain cell text in medium weight.
	Bold bool
}

// Built-in selection menu items (TableSelection.Key) of Selections.
const (
	SelectionAll    = "all"    // select every selectable row (all pages of local data)
	SelectionPage   = "page"   // select the rows of the current page
	SelectionInvert = "invert" // invert the selection of the current page
	SelectionNone   = "none"   // clear the whole selection
)

// TableSelection is an item of the header selection menu (Ant Design
// rowSelection.selections). Key is one of the Selection* constants or a
// custom key; custom items need Text and OnSelect, which receives the
// displayed data rows of the current page (change the selection there).
type TableSelection struct {
	Key      string
	Text     string
	OnSelect func(pageRows []int)
}

// Bordered draws borders around every cell.
func (t *TableW) Bordered() *TableW { t.bordered = true; return t }

// Striped tints every other row.
func (t *TableW) Striped() *TableW { t.striped = true; return t }

// Size picks the density: Large (default), Middle or Small (Element:
// default, Large, Small).
func (t *TableW) Size(s ControlSize) *TableW { t.size, t.sizeSet = s, true; return t }

// Selection binds a multi-select column to data indices.
func (t *TableW) Selection(sel *[]int) *TableW { t.selection = sel; return t }

// SelectedKeys binds the selection to row keys (RowKey). Keys survive
// paging, sorting and server-side data replacement.
func (t *TableW) SelectedKeys(keys *[]string) *TableW { t.selKeys = keys; return t }

// SingleSelect turns the selection column into radios.
func (t *TableW) SingleSelect() *TableW { t.single = true; return t }

// SelectDisabled disables the selection box of some rows.
func (t *TableW) SelectDisabled(f func(row int) bool) *TableW { t.selDisabled = f; return t }

// RowClass styles rows: fn returns the style of data row i (background,
// status tint, text color, weight). It is the Ant rowClassName / Element
// row-class-name hook without CSS.
func (t *TableW) RowClass(fn func(i int) RowStyle) *TableW { t.rowStyle = fn; return t }

// Selections adds a menu next to the header checkbox (Ant Design
// rowSelection.selections). Without items it offers select all,
// select current page (when paginated), invert current page and clear.
func (t *TableW) Selections(items ...TableSelection) *TableW {
	t.selMenu, t.selItems = true, items
	return t
}

// HideSelectAll hides the header checkbox.
func (t *TableW) HideSelectAll() *TableW { t.hideAll = true; return t }

// CheckStrictly checks tree rows independently, without parent/child
// linkage. It is the default (Ant Design rowSelection.checkStrictly
// true); it overrides a ConfigProvider default of false. See CheckLinked.
func (t *TableW) CheckStrictly() *TableW { t.strict = true; return t }

// Sticky pins the header to the top of the scroll container the table
// scrolls in (the nearest ancestor Scroll, or StickyContainer), offset px
// below its top edge (Ant Design sticky.offsetHeader).
func (t *TableW) Sticky(offsetTop float32) *TableW {
	t.sticky, t.stickyTop = true, offsetTop
	return t
}

// StickySummary pins the summary rows to the bottom of the scroll
// container, offset px above its bottom edge, while the table extends
// below it (Ant Design summary fixed + sticky.offsetSummary).
func (t *TableW) StickySummary(offsetBottom float32) *TableW {
	t.stickySum, t.stickyBot = true, offsetBottom
	return t
}

// StickyContainer names the scroll container (a ScrollBox ID) Sticky and
// StickySummary measure against (default: the nearest scrolling ancestor).
func (t *TableW) StickyContainer(id ID) *TableW { t.stickyIn = id; return t }

// Sort binds single-column sorting (controlled).
func (t *TableW) Sort(s *SortState) *TableW { t.sort = s; return t }

// MultiSort binds multi-column sorting (columns with SortMultiple > 0).
func (t *TableW) MultiSort(s *[]SortState) *TableW { t.multiSort = s; return t }

// Filters binds the filter state: column key → selected values.
func (t *TableW) Filters(f *map[string][]string) *TableW { t.filters = f; return t }

// ScrollY bounds the body height; the header (and a fixed summary) stay in
// place and only the visible rows are built, so huge tables stay fast.
func (t *TableW) ScrollY(maxH float32) *TableW { t.maxH = maxH; return t }

// ScrollX sets the scrollable content width (Ant's scroll.x).
func (t *TableW) ScrollX(totalW float32) *TableW { t.scrollW = totalW; return t }

// Virtual is Table V2 mode: a bounded body (ScrollY) where only visible rows
// and columns are built. Rows are fixed-height unless a column wraps (Wrap):
// then visible rows are measured, the rest estimated, and the heights cached.
// With ScrollY the table always virtualizes; Virtual documents the intent.
func (t *TableW) Virtual() *TableW { t.virtual = true; return t }

// RowHeight overrides the row height derived from the size.
func (t *TableW) RowHeight(h float32) *TableW { t.rowH = h; return t }

// RowKey gives rows stable keys (selection, expansion, editing).
func (t *TableW) RowKey(f func(int) string) *TableW { t.rowKey = f; return t }

// Revision tells the table its data changed, so cached local sorting and
// filtering is recomputed (bump it whenever rows change in place).
func (t *TableW) Revision(v uint64) *TableW { t.revision = v; return t }

// Loading shows a translucent mask with a spinner.
func (t *TableW) Loading(on bool) *TableW { t.loading = on; return t }

// Resizable lets the user drag header edges to resize columns.
func (t *TableW) Resizable() *TableW { t.resizable = true; return t }

// NoHover disables the row hover highlight.
func (t *TableW) NoHover() *TableW { t.noHover = true; return t }

// HighlightCurrentRow keeps the clicked row highlighted (Element Plus).
func (t *TableW) HighlightCurrentRow() *TableW { t.highlight = true; return t }

// EmptyText sets the empty-state description.
func (t *TableW) EmptyText(s string) *TableW { t.emptyText = s; return t }

// A11yLabel names the table (grid) for screen readers, e.g. the title of
// the card it sits in.
func (t *TableW) A11yLabel(s string) *TableW { t.a11yName = s; return t }

// Expandable adds an expand column; render builds a row's detail panel.
func (t *TableW) Expandable(render func(row int)) *TableW { t.expRender = render; return t }

// ExpandedKeys binds the expanded rows (controlled).
func (t *TableW) ExpandedKeys(keys *[]string) *TableW { t.expKeys = keys; return t }

// RowExpandable limits which rows can expand.
func (t *TableW) RowExpandable(f func(row int) bool) *TableW { t.expAllow = f; return t }

// Tree shows hierarchical data: roots are the top-level row ids and
// children lists a row's child rows.
func (t *TableW) Tree(roots []int, children func(row int) []int) *TableW {
	t.treeRoots, t.treeKids = roots, children
	return t
}

// TreeLazy loads children on demand: has reports rows that have children
// not loaded yet; load is called once when such a row is expanded (the app
// fetches, appends children and calls Invalidate). A spinner shows meanwhile.
func (t *TableW) TreeLazy(has func(row int) bool, load func(row int)) *TableW {
	t.treeHas, t.treeLoad = has, load
	return t
}

// TreeOpenKeys binds the expanded tree nodes (controlled).
func (t *TableW) TreeOpenKeys(keys *[]string) *TableW { t.treeOpen = keys; return t }

// Indent sets the per-level tree indent (default TableToken.IndentSize:
// 15, Element Plus 16).
func (t *TableW) Indent(px float32) *TableW { t.indent = px; return t }

// Span merges body cells: return (rowSpan, colSpan); 0 hides a cell that a
// neighbour covers, (1,1) is a normal cell.
func (t *TableW) Span(f func(s TableSpan) (rowSpan, colSpan int)) *TableW { t.span = f; return t }

// Summary adds a summary (totals) row; fn returns the text of each column
// for the displayed rows. Call it again for more rows.
func (t *TableW) Summary(fn func(col string, rows []int) string) *TableW {
	t.summary = append(t.summary, fn)
	return t
}

// SummaryFixed keeps the summary visible under a scrolling body.
func (t *TableW) SummaryFixed() *TableW { t.sumFixed = true; return t }

// DragSort adds a drag handle column; fn(from, to) moves data row from to
// the position of data row to.
func (t *TableW) DragSort(fn func(from, to int)) *TableW { t.dragSort = fn; return t }

// EditableRows binds the keys of rows in row-edit mode (every column with
// an Editor shows an input; use Save/Cancel from an action column).
func (t *TableW) EditableRows(keys *[]string) *TableW { t.editRows = keys; return t }

// ColumnState binds column order, visibility and widths.
func (t *TableW) ColumnState(s *TableColumnState) *TableW { t.colState = s; return t }

// Pagination paginates locally: rows are all data; page is 1-based.
func (t *TableW) Pagination(page, pageSize *int) *TableW {
	t.page, t.pageSize, t.serverPage = page, pageSize, false
	return t
}

// ServerPagination shows a pager for server data: rows are the current
// page's rows and total is the count of all rows.
func (t *TableW) ServerPagination(page, pageSize *int, total int) *TableW {
	t.page, t.pageSize, t.serverPage, t.total = page, pageSize, true, total
	return t
}

// PageSizes sets the page-size options of the pager.
func (t *TableW) PageSizes(sizes ...int) *TableW { t.pageSizes = sizes; return t }

// SortChanged reports a sorter click in the last Show.
func (t *TableW) SortChanged() bool { return t.sortedCh }

// FilterChanged reports a confirmed or reset filter in the last Show.
func (t *TableW) FilterChanged() bool { return t.filterCh }

// SelectionChanged reports a selection change in the last Show.
func (t *TableW) SelectionChanged() bool { return t.selCh }

// ExpandChanged reports an expand or tree toggle in the last Show.
func (t *TableW) ExpandChanged() bool { return t.expCh }

// PageChanged reports a page or page-size change in the last Show.
func (t *TableW) PageChanged() bool { return t.pageCh }

// Resized reports a column resize in the last Show.
func (t *TableW) Resized() bool { return t.resizedCh }

// Moved reports a drag-sort drop in the last Show.
func (t *TableW) Moved() bool { return t.movedCh }

// Edited reports a committed cell or row edit in the last Show.
func (t *TableW) Edited() bool { return t.editedCh }

// RowClicked returns the clicked data row or -1.
func (t *TableW) RowClicked() int { return t.clickRow }

// RowDoubleClicked returns the double-clicked data row or -1.
func (t *TableW) RowDoubleClicked() int { return t.dblRow }

// ID returns the table's widget ID (valid after Show).
func (t *TableW) ID() ID { return t.id }

// DisplayedRows returns the data rows shown on the current page, in order
// (valid after Show).
func (t *TableW) DisplayedRows() []int {
	if t.vis == nil && t.fr != nil {
		t.vis = t.fr.dataRows()
	}
	return t.vis
}

// tableDefaults reads the behaviour defaults of ConfigProvider.Defaults
// ("Table", ...). Keys are Ant Design prop names: "checkStrictly"
// (default true), "preserveSelectedRowKeys", "expandRowByClick",
// "defaultExpandAllRows", "showExpandColumn" (default true), "showHeader"
// (default true), "showSorterTooltip" (default true), "indentSize"
// (default TableToken.IndentSize: 15, Element 16),
// "scrollToFirstRowOnChange" (default true), plus Element Plus'
// "highlightCurrentRow", "showOverflowTooltip", "showSummary" and
// "defaultExpandAll".
func (t *TableW) tableDefaults() {
	c := t.c
	t.strict = Prop(c, "Table", "checkStrictly", true)
	t.preserveSel = Prop(c, "Table", "preserveSelectedRowKeys", false)
	t.expandByClick = Prop(c, "Table", "expandRowByClick", false)
	t.expandAll = Prop(c, "Table", "defaultExpandAllRows", Prop(c, "Table", "defaultExpandAll", false))
	t.hideExpandCol = !Prop(c, "Table", "showExpandColumn", true)
	t.noHeader = !Prop(c, "Table", "showHeader", true)
	t.sortTip = Prop(c, "Table", "showSorterTooltip", true)
	ind := c.ThemeFor(theme.CompTable).Components.Table.IndentSize // antd 15, Element 16
	if ind <= 0 {
		ind = 15
	}
	t.indent = propFloat(c, "Table", "indentSize", ind)
	t.scrollTop = Prop(c, "Table", "scrollToFirstRowOnChange", true)
	t.highlight = Prop(c, "Table", "highlightCurrentRow", false)
	t.ellipsisAll = Prop(c, "Table", "showOverflowTooltip", false)
	t.showSum = Prop(c, "Table", "showSummary", false)
}

// SortDirections sets the orders a sorter click cycles through for every
// column without its own TableColumn.SortDirections (Ant Design
// sortDirections, Element Plus sort-orders). The default is SortAsc,
// SortDesc; after the last order the column returns to unsorted.
// SortDesc, SortAsc starts with descending; a single order toggles
// between it and unsorted.
func (t *TableW) SortDirections(dirs ...SortOrder) *TableW { t.sortDirs = dirs; return t }

// ShowSorterTooltip shows or hides the "click to sort" tooltip of sortable
// headers (Ant Design showSorterTooltip, default true; Element Plus has
// none).
func (t *TableW) ShowSorterTooltip(on bool) *TableW { t.sortTip = on; return t }

// PreserveSelectedRowKeys keeps selected keys whose rows are no longer in
// the data (Ant Design preserveSelectedRowKeys, Element Plus
// reserve-selection). By default a selection change drops them, so with
// ServerPagination the selection covers the current page only.
func (t *TableW) PreserveSelectedRowKeys() *TableW { t.preserveSel = true; return t }

// CheckLinked turns on the parent/child linkage of tree selection
// (Ant Design rowSelection.checkStrictly false, Element Plus' default
// tree-props checkStrictly false): checking a row checks its descendants,
// a parent is checked once all its children are and shows a half state
// when only some are. By default (Ant Design checkStrictly true) rows are
// checked independently.
func (t *TableW) CheckLinked() *TableW { t.strict = false; return t }

// SelectionColumnWidth sets the width of the selection column (Ant Design
// rowSelection.columnWidth).
func (t *TableW) SelectionColumnWidth(px float32) *TableW { t.selColW = px; return t }

// SelectionFixed pins the selection column (and the expand column) to the
// start side (Ant Design rowSelection.fixed). Without it they are pinned
// when the first data column is.
func (t *TableW) SelectionFixed() *TableW { t.selFixed = true; return t }

// ExpandRowByClick toggles a row's detail panel (Expandable) or tree
// children when the row is clicked (Ant Design expandable.expandRowByClick).
func (t *TableW) ExpandRowByClick() *TableW { t.expandByClick = true; return t }

// HideExpandColumn hides the expand column of Expandable (Ant Design
// expandable.showExpandColumn false); rows then expand by ExpandRowByClick,
// Enter or the bound ExpandedKeys.
func (t *TableW) HideExpandColumn() *TableW { t.hideExpandCol = true; return t }

// DefaultExpandAllRows expands every expandable row and tree node on the
// first frame when the table owns the expansion state (Ant Design
// expandable.defaultExpandAllRows, Element Plus default-expand-all).
// Rows of TreeLazy whose children are not loaded stay closed.
func (t *TableW) DefaultExpandAllRows() *TableW { t.expandAll = true; return t }

// HideHeader hides the header row (Ant Design / Element Plus showHeader
// false). Sorting and filters stay bound but have no controls.
func (t *TableW) HideHeader() *TableW { t.noHeader = true; return t }

// HeaderHeight overrides the height of a header row (Element Plus Table V2
// header-height); 0 keeps the height derived from the size.
func (t *TableW) HeaderHeight(px float32) *TableW { t.headH = px; return t }

// Title builds a bar above the table (Ant Design title).
func (t *TableW) Title(fn func()) *TableW { t.title = fn; return t }

// Footer builds a bar below the table body (Ant Design footer).
func (t *TableW) Footer(fn func()) *TableW { t.footer = fn; return t }

// Pagination placements (PaginationPlacement), Ant Design 6
// pagination.placement.
const (
	PageTopStart     = "topStart"
	PageTopCenter    = "topCenter"
	PageTopEnd       = "topEnd"
	PageBottomStart  = "bottomStart"
	PageBottomCenter = "bottomCenter"
	PageBottomEnd    = "bottomEnd" // default
	PageNone         = "none"
)

// PaginationPlacement places the pager: one top and/or one bottom
// placement (PageTopStart … PageBottomEnd); PageNone hides it while
// paging still applies. The default is PageBottomEnd.
func (t *TableW) PaginationPlacement(p ...string) *TableW { t.pagePlace = p; return t }

// PaginationConfig customizes the pager after the table set it up (Ant
// Design's pagination object): fn receives it before Show, e.g.
// p.ShowTotal().ShowQuickJumper(). Like Ant Design, the table's pager
// shows no total by default and is small for Small and Middle tables.
func (t *TableW) PaginationConfig(fn func(p *PaginationW)) *TableW { t.pageCfg = fn; return t }

// CurrentRowKey binds the highlighted current row by key (Element Plus
// current-row-key / setCurrentRow); "" means none. It turns on
// HighlightCurrentRow; CurrentChanged reports clicks that moved it.
func (t *TableW) CurrentRowKey(key *string) *TableW { t.curKey, t.highlight = key, true; return t }

// CurrentChanged reports that the current row moved in the last Show
// (Element Plus current-change).
func (t *TableW) CurrentChanged() bool { return t.curCh }

// ShowSummary adds Element Plus' default summary row (show-summary): the
// first data column shows Locale().Table.SumText, columns whose displayed
// texts are numbers show their sum, others stay empty. Use Summary for a
// custom summary-method.
func (t *TableW) ShowSummary() *TableW { t.showSum = true; return t }

// ShowOverflowTooltip makes every text column ellipsis with a tooltip
// (Element Plus' table-level show-overflow-tooltip).
func (t *TableW) ShowOverflowTooltip() *TableW { t.ellipsisAll = true; return t }

// ScrollToFirstRowOnChange sets whether a page, sort or filter change
// scrolls a bounded body back to the first row (Ant Design
// scroll.scrollToFirstRowOnChange, default true).
func (t *TableW) ScrollToFirstRowOnChange(on bool) *TableW { t.scrollTop = on; return t }
