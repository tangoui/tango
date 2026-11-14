package ui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ProEnum is one value of an enumerated ProTable column (status, tag,
// select). Status is a badge status ("success", "error", "processing",
// "warning", "default"); Color a tag color.
type ProEnum struct {
	Value, Label  string
	Status, Color string
	// Children nests options for the cascader and treeSelect value types.
	Children []ProEnum
}

// Value types of ProColumn.ValueType.
const (
	ValueText     = "text"
	ValueMoney    = "money"
	ValueDigit    = "digit"
	ValuePercent  = "percent"
	ValueDate     = "date"
	ValueDateTime = "dateTime"
	ValueStatus   = "status"
	ValueTag      = "tag"
	ValueProgress = "progress"
	ValueLink     = "link"
	ValueOption   = "option" // action column: Render only, never searched
)

// ProColumn is a ProTable column over rows of type T.
type ProColumn[T any] struct {
	Key, Title string
	Width      float32
	MinWidth   float32
	Fixed      string
	Align      TextAlign
	Ellipsis   bool
	Hidden     bool // hidden until enabled in the column settings

	// ValueType formats Value (see the Value* constants).
	ValueType string
	ValueEnum []ProEnum
	Value     func(row T) any
	// Render overrides the cell (i is the row index in the current page).
	Render func(row T, i int)
	// OnClick makes link cells clickable.
	OnClick func(row T)
	// Currency is the ISO 4217 code ("USD", "JPY") or symbol of money cells
	// (default: the Locale's, "¥" for zh-CN / ja, "$" for en-US, "NT$" for
	// zh-TW). A code also sets the decimals (JPY: none); see FormatMoney.
	Currency string

	// Editor makes the column editable in EditableProTable: the field
	// (Required, Rules, Validate, AsyncValidate, Options, Min / Max ...)
	// picks the control by its ValueType; Key, Label, ValueType and
	// ValueEnum default to the column's. Set writes an edited value back
	// into a copy of the row; without Set the column stays read-only.
	Editor *ProField
	Set    func(row *T, v any)

	Sortable     bool // server-side sort (reported in ProRequest.Sort)
	Filterable   bool // filter dropdown from ValueEnum (ProRequest.Filters)
	HideInSearch bool
	HideInTable  bool
}

// ProRequest is what the request function receives.
type ProRequest struct {
	Page, PageSize int
	Sort           []SortState
	Filters        map[string][]string
	Params         map[string]string // query form values (non-empty only)
	// ExtParams is the value given to ProTableW.Params (Ant params); a
	// map[string]string / map[string]any is also merged into Params
	// (the query form wins on a shared key).
	ExtParams any
}

// ProResult is delivered back by the request function (from any goroutine).
type ProResult[T any] struct {
	Rows  []T
	Total int
	Err   error
}

// ProTableW is a query list page: an auto-generated query form, a toolbar
// (refresh, density, column settings, fullscreen) and a table fed by an
// asynchronous request function.
type ProTableW[T any] struct {
	_ [0]func()

	c         *Context
	cols      []ProColumn[T]
	title     string
	rowKey    func(T) string
	request   func(req ProRequest, done func(ProResult[T]))
	toolbar   func()
	search    bool
	selection *[]string
	pageSize  int
	scrollX   float32
	bordered  bool
	expand    func(row T)
	dragSort  func(from, to int)
	rowStyle  func(row T) RowStyle
	table     *TableW
	st        *proState[T]
	id        ID

	// Functional extras (Ant params, polling, columnsState,
	// tableAlertRender / tableAlertOptionRender, options, pagination).
	ext        any
	extSet     bool
	polling    time.Duration
	colRef     *TableColumnState
	onColState func(TableColumnState)
	persistKey string
	persist    ColumnsStateStore
	alertFn    func(keys []string, clear func())
	alertOpt   func(keys []string, clear func())
	noAlert    bool
	noOptions  bool
	noPager    bool
	density0   ControlSize
	densitySet bool
}

// ColumnsStateStore keeps ProTable column states where the application
// wants them (memory, a settings file, a database): ProTable loads the
// state once by key and saves it on every change. It never touches the
// disk itself.
type ColumnsStateStore interface {
	LoadColumnsState(key string) (TableColumnState, bool)
	SaveColumnsState(key string, s TableColumnState)
}

// ColumnsStateFuncs adapts two functions to ColumnsStateStore.
type ColumnsStateFuncs struct {
	Load func(key string) (TableColumnState, bool)
	Save func(key string, s TableColumnState)
}

// LoadColumnsState calls Load (nil: nothing stored).
func (f ColumnsStateFuncs) LoadColumnsState(key string) (TableColumnState, bool) {
	if f.Load == nil {
		return TableColumnState{}, false
	}
	return f.Load(key)
}

// SaveColumnsState calls Save (nil: dropped).
func (f ColumnsStateFuncs) SaveColumnsState(key string, s TableColumnState) {
	if f.Save != nil {
		f.Save(key, s)
	}
}

type proState[T any] struct {
	page, pageSize int
	sort           []SortState
	filters        map[string][]string
	query          ProValues // query form values (QueryFilter)
	params         map[string]string
	density        ControlSize
	densitySet     bool
	colState       TableColumnState
	full           bool
	sel            []string
	reload         int
	densOpen       bool
	densFocus      bool // move focus into the density menu once
	densPanel      ID
	setOpen        bool
	setPanel       ID
	dragKey        string
	dragOn         bool
	extSig         string    // signature of the last Params value
	pollPaused     bool      // PausePolling
	lastDone       time.Time // delivery of the last result (polling)
	silent         bool      // polling request: keep the rows, no mask
	colSig         string    // signature of the last reported column state
	colLoaded      bool      // persisted column state read
	colInit        bool      // colSig holds the first built state

	mu      sync.Mutex
	seq     int
	result  *ProResult[T]
	rows    []T
	total   int
	err     error
	loading bool
	sig     string
}

// ProTable starts a ProTable; set Request, then Show.
//
// ConfigProvider.Defaults("ProTable", ...) keys: "search", "bordered",
// "options" (bool), "pagination" (false, a page size or
// Props{"pageSize": n}), "size" (density: "small" | "middle" | "large"),
// "headerTitle" (string), "polling" (time.Duration or milliseconds).
func ProTable[T any](c *Context, cols []ProColumn[T]) *ProTableW[T] {
	p := &ProTableW[T]{c: c, cols: cols, search: true, pageSize: 10}
	p.search = Prop(c, "ProTable", "search", true)
	p.bordered = Prop(c, "ProTable", "bordered", false)
	p.noOptions = !Prop(c, "ProTable", "options", true)
	if _, ok := c.ConfigProp("ProTable", "pagination"); ok {
		n, on := proPropPageSize(c, "ProTable", 10)
		p.noPager = !on
		if on {
			p.pageSize = n
		}
	}
	if sz, ok := proPropSize(c, "ProTable"); ok {
		p.density0, p.densitySet = sz, true
	}
	p.title = Prop(c, "ProTable", "headerTitle", "")
	if d, ok := p2bDur(c, "ProTable", "polling"); ok {
		p.polling = d
	}
	return p
}

// Params sets external request parameters (Ant params): a change of the
// value (compared by its printed form) requests page 1 again. The value
// reaches the request as ProRequest.ExtParams; maps are also merged into
// ProRequest.Params.
func (p *ProTableW[T]) Params(v any) *ProTableW[T] { p.ext, p.extSet = v, true; return p }

// Polling requests the current page again every d after the previous
// result arrived (0: off). Polling requests keep the rows on screen
// without the loading mask; PausePolling / ResumePolling control it.
func (p *ProTableW[T]) Polling(d time.Duration) *ProTableW[T] { p.polling = d; return p }

// PausePolling stops polling until ResumePolling (valid after Show).
func (p *ProTableW[T]) PausePolling() {
	if p.st != nil {
		p.st.pollPaused = true
	}
}

// ResumePolling restarts paused polling (valid after Show); the next poll
// is due one interval after the last result.
func (p *ProTableW[T]) ResumePolling() {
	if p.st != nil && p.st.pollPaused {
		p.st.pollPaused = false
		p.c.invalid = true
	}
}

// PollingPaused reports whether polling is paused.
func (p *ProTableW[T]) PollingPaused() bool { return p.st != nil && p.st.pollPaused }

// ColumnsState binds the column state (order, visibility, widths, pinned
// side) to *s, so the application controls it (Ant columnsState.value).
func (p *ProTableW[T]) ColumnsState(s *TableColumnState) *ProTableW[T] { p.colRef = s; return p }

// OnColumnsStateChange is called with the new column state after the user
// changes it in the column settings or by resizing (Ant
// columnsState.onChange).
func (p *ProTableW[T]) OnColumnsStateChange(fn func(TableColumnState)) *ProTableW[T] {
	p.onColState = fn
	return p
}

// PersistColumnsState loads the column state from store under key once and
// saves every change there (Ant columnsState.persistenceKey); the store
// decides where the state lives.
func (p *ProTableW[T]) PersistColumnsState(key string, store ColumnsStateStore) *ProTableW[T] {
	p.persistKey, p.persist = key, store
	return p
}

// TableAlertRender replaces the text of the selection bar (Ant
// tableAlertRender); clear empties the selection.
func (p *ProTableW[T]) TableAlertRender(fn func(keys []string, clear func())) *ProTableW[T] {
	p.alertFn = fn
	return p
}

// TableAlertOptionRender replaces the actions of the selection bar (the
// default clear link), e.g. with batch operations (Ant
// tableAlertOptionRender).
func (p *ProTableW[T]) TableAlertOptionRender(fn func(keys []string, clear func())) *ProTableW[T] {
	p.alertOpt = fn
	return p
}

// NoTableAlert hides the selection bar (Ant tableAlertRender={false}).
func (p *ProTableW[T]) NoTableAlert() *ProTableW[T] { p.noAlert = true; return p }

// NoOptions hides the toolbar setting icons (Ant options={false}).
func (p *ProTableW[T]) NoOptions() *ProTableW[T] { p.noOptions = true; return p }

// NoPagination drops the pager: requests ask for every row (PageSize 0).
func (p *ProTableW[T]) NoPagination() *ProTableW[T] { p.noPager = true; return p }

// colState returns the column state in use (bound or internal).
func (p *ProTableW[T]) colState() *TableColumnState {
	if p.colRef != nil {
		return p.colRef
	}
	return &p.st.colState
}

// clearSelection empties the bound selection.
func (p *ProTableW[T]) clearSelection() {
	if p.selection != nil {
		*p.selection = (*p.selection)[:0]
		p.c.invalid = true
	}
}

// Title sets the toolbar title.
func (p *ProTableW[T]) Title(s string) *ProTableW[T] { p.title = s; return p }

// RowKey gives rows stable keys (selection across pages).
func (p *ProTableW[T]) RowKey(f func(T) string) *ProTableW[T] { p.rowKey = f; return p }

// Request sets the data source. It is called on the UI thread whenever the
// page, sort, filters or query change; call done (from any goroutine) with
// the page rows and total. The table shows a loading mask meanwhile and an
// error banner if Err is set.
func (p *ProTableW[T]) Request(fn func(req ProRequest, done func(ProResult[T]))) *ProTableW[T] {
	p.request = fn
	return p
}

// ToolBar adds actions (e.g. a "新建" button) left of the toolbar icons.
func (p *ProTableW[T]) ToolBar(fn func()) *ProTableW[T] { p.toolbar = fn; return p }

// NoSearch hides the query form.
func (p *ProTableW[T]) NoSearch() *ProTableW[T] { p.search = false; return p }

// Selection binds selected row keys (kept across pages).
func (p *ProTableW[T]) Selection(keys *[]string) *ProTableW[T] { p.selection = keys; return p }

// PageSize sets the initial page size (default 10).
func (p *ProTableW[T]) PageSize(n int) *ProTableW[T] { p.pageSize = n; return p }

// ScrollX sets the table's scroll width.
func (p *ProTableW[T]) ScrollX(w float32) *ProTableW[T] { p.scrollX = w; return p }

// Bordered draws cell borders.
func (p *ProTableW[T]) Bordered() *ProTableW[T] { p.bordered = true; return p }

// Expandable adds detail panels.
func (p *ProTableW[T]) Expandable(fn func(row T)) *ProTableW[T] { p.expand = fn; return p }

// DragSort adds a drag handle column (Ant Design Pro DragSortTable):
// dropping a row calls fn with the indices of the moved row and of the row
// it was dropped on, both in the current page (Rows). Reorder the data
// (locally or on the server) and Reload, or reorder the page in place
// with MovePageRow.
func (p *ProTableW[T]) DragSort(fn func(from, to int)) *ProTableW[T] { p.dragSort = fn; return p }

// RowClass styles rows (see TableW.RowClass).
func (p *ProTableW[T]) RowClass(fn func(row T) RowStyle) *ProTableW[T] { p.rowStyle = fn; return p }

// MovePageRow moves row from to the position of row to in the current
// page without a new request (valid after Show; use it from DragSort).
func (p *ProTableW[T]) MovePageRow(from, to int) {
	if p.st != nil {
		p.st.rows = tblMoveSlice(p.st.rows, from, to)
		p.c.invalid = true
	}
}

// Reload requests the data again (valid after Show).
func (p *ProTableW[T]) Reload() {
	if p.st != nil {
		p.st.reload++
		p.c.invalid = true
	}
}

// Rows returns the rows of the current page.
func (p *ProTableW[T]) Rows() []T {
	if p.st == nil {
		return nil
	}
	return p.st.rows
}

// Table returns the inner table of the last Show.
func (p *ProTableW[T]) Table() *TableW { return p.table }

// Loading reports a pending request.
func (p *ProTableW[T]) Loading() bool { return p.st != nil && p.st.loading }

// proMergeExt merges a map-shaped Params value into the query params
// (form values win).
func proMergeExt(dst map[string]string, ext any) {
	add := func(k string, v any) {
		if _, ok := dst[k]; ok {
			return
		}
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && v != nil {
			dst[k] = s
		}
	}
	switch m := ext.(type) {
	case map[string]string:
		for k, v := range m {
			add(k, v)
		}
	case map[string]any:
		for k, v := range m {
			add(k, v)
		}
	}
}

// proBuildRequest assembles the request parameters.
func proBuildRequest(page, pageSize int, sorts []SortState, filters map[string][]string, params map[string]string) ProRequest {
	req := ProRequest{Page: max(1, page), PageSize: pageSize, Filters: map[string][]string{}, Params: map[string]string{}}
	for _, s := range sorts {
		if s.Order != SortNone && s.Column != "" {
			req.Sort = append(req.Sort, s)
		}
	}
	for k, v := range filters {
		if len(v) > 0 {
			req.Filters[k] = append([]string(nil), v...)
		}
	}
	for k, v := range params {
		if strings.TrimSpace(v) != "" {
			req.Params[k] = strings.TrimSpace(v)
		}
	}
	return req
}

// signature of a request (to detect changes).
func (r ProRequest) signature() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d/%d|", r.Page, r.PageSize)
	for _, s := range r.Sort {
		fmt.Fprintf(&b, "%s:%d,", s.Column, s.Order)
	}
	b.WriteByte('|')
	keys := make([]string, 0, len(r.Filters))
	for k := range r.Filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s;", k, strings.Join(r.Filters[k], ","))
	}
	b.WriteByte('|')
	keys = keys[:0]
	for k := range r.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s;", k, r.Params[k])
	}
	return b.String()
}

// Show builds the ProTable.
func (p *ProTableW[T]) Show() *ProTableW[T] {
	c := p.c
	root := c.box("protable", layout.Column, nil)
	p.id = root.n.id
	st := State[proState[T]](c, p.id, func() proState[T] {
		return proState[T]{page: 1, pageSize: p.pageSize, query: ProValues{}, params: map[string]string{}, filters: map[string][]string{}}
	})
	p.st = st
	p.poll()
	p.extras()
	if st.full {
		root.Body(func() {
			ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
				Scroll(c, func() { p.content() }, layout.Grow(1), layout.Pad(24))
			})
			ov.Bg(c.Theme.BgLayout)
		})
	} else {
		root.Body(func() { p.content() })
	}
	p.maybeRequest()
	p.reportColumns()
	return p
}

// extras applies Params, Polling, persisted column state and the density
// default before the table is built.
func (p *ProTableW[T]) extras() {
	c, st := p.c, p.st
	if p.densitySet && !st.densitySet {
		st.density, st.densitySet = p.density0, true
	}
	if p.persist != nil && p.persistKey != "" && !st.colLoaded {
		st.colLoaded = true
		if cs, ok := p.persist.LoadColumnsState(p.persistKey); ok {
			*p.colState() = cs
		}
		st.colSig = proColSig(*p.colState())
	}
	if p.extSet {
		sig := fmt.Sprintf("%#v", p.ext)
		if st.extSig != "" && sig != st.extSig {
			st.page = 1
		}
		st.extSig = sig
	}
	if p.polling > 0 && p.request != nil && !st.pollPaused && !st.loading && !st.lastDone.IsZero() {
		due := st.lastDone.Add(p.polling)
		if !c.Now().Before(due) {
			st.reload++
			st.silent = true
			st.lastDone = c.Now()
		} else {
			c.WakeAt(due)
		}
	}
}

// proColSig prints a column state for change detection (fmt sorts map
// keys).
func proColSig(s TableColumnState) string {
	return fmt.Sprint(s.Order, s.Hidden, s.Widths, s.Fixed)
}

// reportColumns calls OnColumnsStateChange and saves to the persistence
// store when the column state changed this frame.
func (p *ProTableW[T]) reportColumns() {
	st := p.st
	if p.onColState == nil && p.persist == nil {
		return
	}
	cs := *p.colState()
	sig := proColSig(cs)
	if sig == st.colSig {
		return
	}
	first := !st.colInit
	st.colSig, st.colInit = sig, true
	if first {
		return // the initial (loaded, normalised) state is not a change
	}
	if p.onColState != nil {
		p.onColState(cs)
	}
	if p.persist != nil && p.persistKey != "" {
		p.persist.SaveColumnsState(p.persistKey, cs)
	}
}

// poll applies a delivered result.
func (p *ProTableW[T]) poll() {
	st := p.st
	st.mu.Lock()
	r := st.result
	st.result = nil
	st.mu.Unlock()
	if r == nil {
		return
	}
	st.rows, st.total, st.err, st.loading = r.Rows, r.Total, r.Err, false
	if r.Err != nil {
		st.rows = nil
	}
	st.silent = false
	st.lastDone = p.c.Now()
	p.c.invalid = true
}

// maybeRequest issues a request when its parameters changed.
func (p *ProTableW[T]) maybeRequest() {
	st := p.st
	if p.request == nil {
		return
	}
	size := st.pageSize
	if p.noPager {
		size = 0
	}
	req := proBuildRequest(st.page, size, st.sort, st.filters, st.params)
	if p.noPager {
		req.Page, req.PageSize = 1, 0
	}
	if p.extSet {
		req.ExtParams = p.ext
		proMergeExt(req.Params, p.ext)
	}
	sig := req.signature() + "#" + strconv.Itoa(st.reload) + "#" + st.extSig
	if sig == st.sig {
		return
	}
	st.sig = sig
	st.mu.Lock()
	st.seq++
	seq := st.seq
	st.mu.Unlock()
	st.loading = true // a polling request (silent) hides the mask
	win := p.c.win
	p.request(req, func(r ProResult[T]) {
		st.mu.Lock()
		if seq == st.seq {
			st.result = &r
		}
		st.mu.Unlock()
		if win != nil {
			win.Invalidate()
		}
	})
	p.poll() // a synchronous request has already delivered
}

func (p *ProTableW[T]) content() {
	c, th := p.c, p.c.Theme
	st := p.st
	if shad2d(c) {
		p.shadContent()
		return
	}
	if p.search && p.hasSearch() {
		Column(c, func() { p.queryFilter() }, layout.PadVH(24, 24), layout.Margin(geom.Insets{Bottom: 16})).Bg(th.BgContainer).Radius(th.RadiusLG)
	}
	Column(c, func() {
		p.toolbarRow()
		if p.selection != nil && len(*p.selection) > 0 && !p.noAlert {
			p.alertBar()
		}
		if st.err != nil {
			Row(c, func() {
				a := Alert(c, "error", locFmt(c.Locale().ProTable.LoadErrorFmt, st.err.Error()))
				a.Show()
			}, layout.Margin(geom.Insets{Bottom: 16}))
			Row(c, func() {
				if Button(c, c.Locale().Retry).Small().Icon(icon.Reload).Clicked() {
					p.Reload()
				}
			}, layout.Margin(geom.Insets{Bottom: 16}))
		}
		p.tableView()
	}, layout.PadVH(16, 24), layout.Padding(geom.Insets{Top: 0, Right: 24, Bottom: 24, Left: 24})).Bg(th.BgContainer).Radius(th.RadiusLG)
}

func (p *ProTableW[T]) hasSearch() bool {
	for _, col := range p.cols {
		if p.searchable(col) {
			return true
		}
	}
	return false
}

func (p *ProTableW[T]) searchable(col ProColumn[T]) bool {
	return !col.HideInSearch && col.ValueType != ValueOption && col.Key != ""
}

// queryFilter is the search form: a QueryFilter over the searchable
// columns.
func (p *ProTableW[T]) queryFilter() {
	st := p.st
	var fields []ProField
	for _, col := range p.cols {
		if p.searchable(col) {
			fields = append(fields, proFieldFromColumn(col))
		}
	}
	q := QueryFilter(p.c, fields).Values(&st.query).Loading(st.loading && !st.silent).Show()
	if q.Searched() {
		st.params = q.Params()
		st.page = 1
		st.reload++
		p.c.invalid = true
	}
}

func (p *ProTableW[T]) density() ControlSize {
	if p.st.densitySet {
		return p.st.density
	}
	return Large
}

// toolbarRow is the title + actions + setting icons.
func (p *ProTableW[T]) toolbarRow() {
	c, th, st := p.c, p.c.Theme, p.st
	Row(c, func() {
		if p.title != "" {
			t := Text(c, p.title).Size(th.FontSizeLG).Weight(text.Medium).Color(th.TextHeading)
			if shad2d(c) {
				t.Weight(text.Semibold)
			}
		}
		Spacer(c)
		if p.toolbar != nil {
			Row(c, p.toolbar, layout.Gap(8), layout.Middle())
		}
		if p.noOptions {
			return
		}
		Row(c, func() {
			if ok, _ := p.tool(c.Locale().ProTable.Reload, icon.Reload, false, false); ok {
				p.Reload()
			}
			ok, anchor := p.tool(c.Locale().ProTable.Density, tblIconDensity, st.densOpen, true)
			if ok {
				st.densOpen = !st.densOpen
				st.densFocus = st.densOpen
				c.invalid = true
			}
			if st.densOpen {
				p.densityMenu(anchor)
			}
			ok, anchor = p.settingTool(st.setOpen)
			if ok {
				st.setOpen = !st.setOpen
				c.invalid = true
			}
			if st.setOpen {
				p.columnSettings(anchor)
			}
			ic, label := icon.Fullscreen, c.Locale().Fullscreen
			if st.full {
				ic, label = icon.FullscreenExit, c.Locale().ExitFullscreen
			}
			if ok, _ := p.tool(label, ic, false, false); ok {
				st.full = !st.full
				c.invalid = true
			}
		}, layout.Gap(4), layout.Middle(), layout.Margin(geom.Insets{Left: 12}))
	}, layout.Middle(), layout.MinH(64))
}

// tool is a toolbar option button in the theme's look.
func (p *ProTableW[T]) tool(tip string, ic *icon.Icon, on, popup bool) (bool, ID) {
	if shad2d(p.c) {
		return p.shadTool(tip, "", nil, ic, on, popup)
	}
	return p.toolIcon(tip, ic, on, popup)
}

// settingTool opens the column settings (shadcn: the outline "View"
// button of the data-table toolbar).
func (p *ProTableW[T]) settingTool(on bool) (bool, ID) {
	loc := p.c.Locale()
	if shad2d(p.c) {
		return p.shadTool(loc.ProTable.ColumnSetting, loc.ProTable.ColumnSetting, shad2dIconSliders, nil, on, true)
	}
	return p.toolIcon(loc.ProTable.ColumnSetting, icon.Setting, on, true)
}

// toolIcon is a 28px icon button with a tooltip (its accessible name);
// popup marks the buttons that open a menu or panel (on: open).
func (p *ProTableW[T]) toolIcon(tip string, ic *icon.Icon, on, popup bool) (bool, ID) {
	c, th := p.c, p.c.Theme
	n := c.add("pro-tool", tip)
	n.setRole(a11y.RoleButton)
	if popup {
		n.setState(a11y.Expandable|a11y.HasPopup).stateIf(on, a11y.Expanded)
	}
	n.ln.Width, n.ln.Height = 28, 28
	ia := c.interact(n, true, input.CursorPointer)
	n.paint = func(l *paint.List, r geom.Rect) {
		col := th.Text
		if ia.Hovered || on || ia.FocusVisible {
			l.Fill(r, geom.Rad(th.RadiusSM), th.FillSecondary)
		}
		if ia.FocusVisible {
			l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.Primary.Base)
		}
		ir := geom.Rect{X: r.X + 6, Y: r.Y + 6, W: 16, H: 16}
		icon.Draw(l, ic, ir, col, 0)
	}
	if ia.Hovered && !on {
		tf := &tblFrame{c: c, th: th, el: th.Flavor == theme.Element}
		tf.tip(n.id, tip, false)
		tf.flushTips()
	}
	return ia.Clicked, n.id
}

// densityMenu lists the three densities.
func (p *ProTableW[T]) densityMenu(anchor ID) {
	c, th, st := p.c, p.c.Theme, p.st
	cur := p.density()
	// A menu of radio items: Tab reaches the current one, Up / Down move,
	// Enter picks, Escape closes (focus returns to the toolbar button).
	var items []*node
	var ids []ID
	curIdx := 0
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomEnd, Offset: 4, Flip: true}, func() {
		for i, it := range []struct {
			label string
			size  ControlSize
		}{{c.Locale().ProTable.DensityDefault, Large}, {c.Locale().ProTable.DensityMiddle, Middle}, {c.Locale().ProTable.DensityCompact, Small}} {
			it := it
			b := Row(c, func() { Text(c, it.label) }, layout.PadVH(5, 12), layout.MinW(96), layout.Middle())
			b.Semantics(a11y.RoleMenuItem, it.label).A11yState(a11y.Checkable)
			ia := b.Focusable(input.CursorPointer)
			items, ids = append(items, b.n), append(ids, b.n.id)
			switch {
			case it.size == cur:
				curIdx = i
				b.A11yState(a11y.Checked)
				b.Bg(th.Primary.Bg).Radius(th.RadiusSM)
			case ia.Hovered || ia.FocusVisible:
				b.Bg(th.ItemHoverBg).Radius(th.RadiusSM)
			}
			if ia.Clicked {
				st.density, st.densitySet, st.densOpen = it.size, true, false
				c.invalid = true
			}
		}
	}, layout.Pad(4))
	ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.Shadow)
	ov.Semantics(a11y.RoleMenu, c.Locale().ProTable.Density)
	rovingTabStop(items, curIdx)
	if to := rovingStep(c, ids, nil, rovingVertical); to >= 0 {
		navFocus(c, ids[to])
	}
	if st.densFocus && len(ids) > 0 {
		st.densFocus = false
		c.SetFocus(ids[curIdx])
	}
	st.densPanel = ov.n.id
	if c.Dismissed(st.densPanel, anchor) {
		st.densOpen = false
		c.invalid = true
	}
}

// columnOrder returns the top-level column keys in display order.
func (p *ProTableW[T]) columnOrder() []string {
	var keys []string
	for _, col := range p.cols {
		if !col.HideInTable {
			keys = append(keys, col.Key)
		}
	}
	if len(p.colState().Order) == 0 {
		return keys
	}
	rank := map[string]int{}
	for i, k := range p.colState().Order {
		rank[k] = i
	}
	sort.SliceStable(keys, func(i, j int) bool {
		ri, oki := rank[keys[i]]
		rj, okj := rank[keys[j]]
		return oki && okj && ri < rj
	})
	return keys
}

func (p *ProTableW[T]) colHidden(col ProColumn[T]) bool {
	if h, ok := p.colState().Hidden[col.Key]; ok {
		return h
	}
	return col.Hidden
}

// columnSettings is the column visibility / order / pinning popover.
// Columns are grouped by their fixed side (left, none, right); hovering an
// item shows buttons that pin it to the left or right or unpin it.
func (p *ProTableW[T]) columnSettings(anchor ID) {
	c, th, st := p.c, p.c.Theme, p.st
	loc := c.Locale()
	order := p.columnOrder()
	byKey := map[string]ProColumn[T]{}
	for _, col := range p.cols {
		byKey[col.Key] = col
	}
	if p.colState().Hidden == nil {
		p.colState().Hidden = map[string]bool{}
	}
	if p.colState().Fixed == nil {
		p.colState().Fixed = map[string]string{}
	}
	fixedOf := func(k string) string {
		if fx, ok := p.colState().Fixed[k]; ok {
			if fx == FixedNone {
				return ""
			}
			return fx
		}
		return byKey[k].Fixed
	}
	groups := [3][]string{}
	for _, k := range order {
		switch fixedOf(k) {
		case FixedLeft:
			groups[0] = append(groups[0], k)
		case FixedRight:
			groups[2] = append(groups[2], k)
		default:
			groups[1] = append(groups[1], k)
		}
	}
	titled := len(groups[0]) > 0 || len(groups[2]) > 0
	titles := [3]string{loc.Table.FixedLeft, loc.Table.NoFixed, loc.Table.FixedRight}
	var shown []string // display order (drag targets)
	var rowIDs []ID
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomEnd, Offset: 4, Flip: true}, func() {
		Row(c, func() {
			vis := 0
			for _, k := range order {
				if !p.colHidden(byKey[k]) {
					vis++
				}
			}
			all := vis == len(order)
			v := all
			if Checkbox(c, loc.ProTable.ColumnDisplay, &v).Indeterminate(vis > 0 && !all).Changed() {
				for _, k := range order {
					p.colState().Hidden[k] = !v
				}
			}
			Spacer(c)
			if Link(c, loc.Reset).Clicked() {
				*p.colState() = TableColumnState{Hidden: map[string]bool{}, Fixed: map[string]string{}}
				c.invalid = true
			}
		}, layout.PadVH(8, 12), layout.MinW(240), layout.Middle())
		Div(c, layout.H(1)).Bg(th.Split)
		Column(c, func() {
			for gi, keys := range groups {
				if len(keys) == 0 {
					continue
				}
				if titled {
					Text(c, titles[gi]).Size(th.FontSizeSM).Color(th.TextTertiary).Layout(layout.Padding(geom.Insets{Top: 6, Left: 12, Bottom: 2}))
				}
				for _, k := range keys {
					col := byKey[k]
					shown = append(shown, k)
					Key(c, k, func() { rowIDs = append(rowIDs, p.settingItem(col, gi)) })
				}
			}
		}, layout.PadVH(4, 0))
	})
	ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.Shadow)
	ov.Semantics(a11y.RoleDialog, loc.ProTable.ColumnSetting)
	if c.a11yOn {
		ov.A11y(func(s *a11y.Node) { s.Actions |= a11y.Dismiss })
	}
	st.setPanel = ov.n.id
	// Drag reordering: the target is the item under the pointer.
	if st.dragOn {
		target := -1
		for i, id := range rowIDs {
			if r, ok := c.rects[id]; ok && c.pointer.Y >= r.Y {
				target = i
			}
		}
		target = max(0, target)
		if c.active == 0 {
			from := -1
			for i, k := range shown {
				if k == st.dragKey {
					from = i
				}
			}
			if from >= 0 && from != target && target < len(shown) {
				idx := make([]int, len(shown))
				for i := range idx {
					idx[i] = i
				}
				idx = tblMove(idx, from, target)
				no := make([]string, len(shown))
				for i, j := range idx {
					no[i] = shown[j]
				}
				p.colState().Order = no
			}
			st.dragOn = false
			c.invalid = true
		} else if target < len(rowIDs) {
			if r, ok := c.rects[rowIDs[target]]; ok {
				ov.PaintOver(func(l *paint.List, _ geom.Rect) {
					l.Fill(geom.Rect{X: r.X + 8, Y: r.Y - 1, W: r.W - 16, H: 2}, geom.Radii{}, th.Primary.Base)
				})
			}
		}
	} else if c.Dismissed(st.setPanel, anchor) {
		st.setOpen = false
		c.invalid = true
	}
}

// settingItem is one column of the settings list (group: 0 left-fixed,
// 1 unpinned, 2 right-fixed); it returns the row's ID.
func (p *ProTableW[T]) settingItem(col ProColumn[T], group int) ID {
	c, th, st := p.c, p.c.Theme, p.st
	loc := c.Locale()
	k := col.Key
	const itemH = 32
	b := Row(c, func() {
		h := c.add("col-drag", "drag:"+k)
		h.ln.Width, h.ln.Height = 16, 16
		// Focusable: Up / Down move the column (keyboard drag).
		h.setRole(a11y.RoleButton).setName(loc.A11y.DragHandle)
		h.ownKeys = true
		hia := c.interact(h, true, input.CursorMove)
		if hia.PressedNow {
			st.dragKey, st.dragOn = k, true
		}
		for _, kv := range c.KeysFor(h.id) {
			if kv.Down && (kv.Key == input.KeyUp || kv.Key == input.KeyDown) {
				p.moveColumn(k, kv.Key == input.KeyDown)
			}
		}
		h.paint = func(l *paint.List, r geom.Rect) {
			icon.Draw(l, icon.Holder, r, th.TextTertiary, 0)
			if hia.FocusVisible {
				l.Stroke(r.Expand(1), geom.Rad(th.RadiusSM), 2, th.Primary.Base)
			}
		}
		v := !p.colHidden(col)
		if Checkbox(c, col.Title, &v).Changed() {
			p.colState().Hidden[k] = !v
		}
		Spacer(c)
		hov := false
		if r, ok := c.rects[c.cur.id]; ok && r.Contains(c.pointer) && c.pointerIn {
			hov = true
		}
		pin := func(label, tip string, ic *icon.Icon, fx string) {
			n := c.add("col-pin", label+":"+k)
			n.setRole(a11y.RoleButton).setName(tip)
			n.ln.Width, n.ln.Height = 20, 20
			ia := c.interact(n, true, input.CursorPointer)
			if ia.Clicked {
				p.colState().Fixed[k] = fx
				c.invalid = true
			}
			n.paint = func(l *paint.List, r geom.Rect) {
				if !hov && !ia.Hovered && !ia.FocusVisible {
					return
				}
				col := th.TextTertiary
				if ia.Hovered || ia.FocusVisible {
					col = th.Primary.Base
				}
				icon.Draw(l, ic, r.Inset(geom.All(3)), col, 0)
			}
			if ia.Hovered {
				tf := &tblFrame{c: c, th: th, el: th.Flavor == theme.Element}
				tf.tip(n.id, tip, false)
				tf.flushTips()
			}
		}
		left := loc.Table.PinLeft
		right := loc.Table.PinRight
		unpin := loc.Table.Unpin
		switch group {
		case 0:
			pin("unpin", unpin, tblIconPinMiddle, FixedNone)
			pin("pin-right", right, tblIconPinBottom, FixedRight)
		case 2:
			pin("pin-left", left, tblIconPinTop, FixedLeft)
			pin("unpin", unpin, tblIconPinMiddle, FixedNone)
		default:
			pin("pin-left", left, tblIconPinTop, FixedLeft)
			pin("pin-right", right, tblIconPinBottom, FixedRight)
		}
	}, layout.PadVH(0, 12), layout.H(itemH), layout.Middle(), layout.Gap(8))
	if b.Hovered() {
		b.Bg(th.ItemHoverBg)
	}
	return b.n.id
}

// moveColumn moves column k one place up or down in the display order
// (the keyboard alternative of dragging it in the column settings).
func (p *ProTableW[T]) moveColumn(k string, down bool) {
	order := p.columnOrder()
	for i, o := range order {
		if o != k {
			continue
		}
		j := i - 1
		if down {
			j = i + 1
		}
		if j >= 0 && j < len(order) {
			order[i], order[j] = order[j], order[i]
			p.colState().Order = order
			p.c.invalid = true
		}
		return
	}
}

// alertBar shows the selection count with a clear action.
func (p *ProTableW[T]) alertBar() {
	c, th := p.c, p.c.Theme
	keys := append([]string(nil), *p.selection...)
	Row(c, func() {
		if p.alertFn != nil {
			Row(c, func() { p.alertFn(keys, p.clearSelection) }, layout.Gap(8), layout.Middle())
		} else {
			Text(c, c.Locale().Selected(len(*p.selection)))
		}
		if p.alertOpt != nil {
			Spacer(c)
			Row(c, func() { p.alertOpt(keys, p.clearSelection) }, layout.Gap(8), layout.Middle())
		} else if Link(c, c.Locale().ProTable.ClearSelection).Clicked() {
			*p.selection = (*p.selection)[:0]
			c.invalid = true
		}
	}, layout.PadVH(9, 24), layout.Gap(16), layout.Middle(), layout.Margin(geom.Insets{Bottom: 16})).
		Bg(th.Primary.Bg).Border(1, th.Primary.Border).Radius(th.RadiusLG)
}

// tableView builds the inner table.
func (p *ProTableW[T]) tableView() {
	c, st := p.c, p.st
	rows := st.rows
	var cols []TableColumn
	for _, pc := range p.cols {
		if pc.HideInTable {
			continue
		}
		cols = append(cols, p.tableColumn(pc, rows))
	}
	p.colState().Order = p.columnOrder()
	t := Table(c, cols, len(rows)).
		MultiSort(&st.sort).
		Filters(&st.filters).
		ServerPagination(&st.page, &st.pageSize, st.total).
		Loading(st.loading && !st.silent).
		Size(p.density()).
		ColumnState(p.colState()).
		EmptyText(map[bool]string{true: c.Locale().LoadFailed, false: ""}[st.err != nil])
	if p.title != "" {
		t.A11yLabel(p.title)
	}
	if p.rowKey != nil {
		t.RowKey(func(i int) string { return p.rowKey(rows[i]) })
	}
	if p.selection != nil {
		t.SelectedKeys(p.selection)
	}
	if p.scrollX > 0 {
		t.ScrollX(p.scrollX)
	}
	if p.bordered {
		t.Bordered()
	}
	if p.expand != nil {
		t.Expandable(func(i int) { p.expand(rows[i]) })
	}
	if p.dragSort != nil {
		t.DragSort(p.dragSort)
	}
	if p.rowStyle != nil {
		t.RowClass(func(i int) RowStyle { return p.rowStyle(rows[i]) })
	}
	if p.noPager || shad2d(c) {
		// No table pager: none at all, or the shadcn footer (shadFooter).
		t.ServerPagination(nil, nil, 0)
	}
	t.Show()
	p.table = t
	if t.SortChanged() || t.FilterChanged() {
		st.page = 1
	}
}

// tableColumn converts a ProColumn into a TableColumn with value-type
// formatting.
func (p *ProTableW[T]) tableColumn(pc ProColumn[T], rows []T) TableColumn {
	c := p.c
	col := TableColumn{Key: pc.Key, Title: pc.Title, Width: pc.Width, MinWidth: pc.MinWidth, Fixed: pc.Fixed,
		Align: pc.Align, Ellipsis: pc.Ellipsis, Hidden: pc.Hidden, Sortable: pc.Sortable}
	switch pc.ValueType {
	case ValueMoney, ValueDigit, ValuePercent:
		if pc.Align == AlignStart {
			col.Align = AlignEnd // numbers line up at the end (left in RTL)
		}
	}
	if pc.Filterable && len(pc.ValueEnum) > 0 {
		for _, e := range pc.ValueEnum {
			col.Filters = append(col.Filters, Option{Label: e.Label, Value: e.Value})
		}
	}
	if pc.Render != nil {
		col.Render = func(i int) { pc.Render(rows[i], i) }
		return col
	}
	val := func(i int) any {
		if pc.Value == nil {
			return nil
		}
		return pc.Value(rows[i])
	}
	switch pc.ValueType {
	case ValueStatus:
		col.Render = func(i int) {
			e := proEnum(pc.ValueEnum, val(i))
			Badge(c).Status(e.Status, e.Label)
		}
	case ValueTag:
		col.Render = func(i int) {
			e := proEnum(pc.ValueEnum, val(i))
			tg := Tag(c, e.Label)
			if e.Color != "" {
				tg.Color(e.Color)
			} else if e.Status != "" {
				tg.Color(e.Status)
			}
		}
	case ValueProgress:
		col.Render = func(i int) {
			v, _ := proNumber(val(i))
			pr := Progress(c, v).StrokeWidth(6).Layout(layout.Grow(1), layout.MinW(0), layout.MaxW(200))
			if v >= 100 {
				pr.Status("success")
			}
		}
	case ValueLink:
		col.Render = func(i int) {
			if Link(c, ProFormat(ValueText, nil, val(i))).Clicked() && pc.OnClick != nil {
				pc.OnClick(rows[i])
			}
		}
	case ValueColor:
		col.Render = func(i int) { p2eColorView(c, val(i)) }
	default:
		col.Text = func(i int) string { return pro3ValueText(c, pc.ValueType, pc.ValueEnum, val(i)) }
		if pc.ValueType == ValueMoney {
			loc, cur := c.Locale(), pc.Currency
			col.Text = func(i int) string {
				v := val(i)
				if f, ok := proNumber(v); ok && v != nil {
					return FormatMoney(loc, f, cur)
				}
				return ProFormatLocale(loc, ValueMoney, nil, v)
			}
		}
	}
	return col
}

func proEnum(es []ProEnum, v any) ProEnum {
	s := fmt.Sprint(v)
	for _, e := range es {
		if e.Value == s {
			if e.Status == "" {
				e.Status = "default"
			}
			return e
		}
	}
	return ProEnum{Value: s, Label: s, Status: "default"}
}

func proNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// ProFormat renders a value as text for a value type, formatting money
// like zh-CN (ProFormatLocale(nil, ...)). Code with a Context should call
// ProFormatLocale(c.Locale(), ...) so money follows the active Locale, as
// ProTable, ProDescriptions and the other Pro components do.
func ProFormat(valueType string, enum []ProEnum, v any) string {
	return ProFormatLocale(nil, valueType, enum, v)
}

// ProFormatLocale is ProFormat with money in the currency and number style
// of loc (see FormatMoney).
func ProFormatLocale(loc *Locale, valueType string, enum []ProEnum, v any) string {
	if v == nil {
		return "-"
	}
	switch valueType {
	case ValueMoney:
		if f, ok := proNumber(v); ok {
			return FormatMoney(loc, f, "")
		}
	case ValueDigit:
		if f, ok := proNumber(v); ok {
			if f == math.Trunc(f) {
				return FormatNumber(f, 0)
			}
			return FormatNumber(f, 2)
		}
	case ValuePercent:
		if f, ok := proNumber(v); ok {
			return strconv.FormatFloat(f, 'f', 2, 64) + "%"
		}
	case ValueDate, ValueDateTime:
		layout := "2006-01-02"
		if valueType == ValueDateTime {
			layout = "2006-01-02 15:04:05"
		}
		switch t := v.(type) {
		case time.Time:
			if t.IsZero() {
				return "-"
			}
			return t.Format(layout)
		case int64:
			return time.Unix(t, 0).Format(layout)
		}
	case ValueStatus, ValueTag:
		return proEnum(enum, v).Label
	}
	s := fmt.Sprint(v)
	if s == "" {
		return "-"
	}
	return s
}
