package mobile

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// binding connects picker wheels to an application value.
type binding struct {
	names []string                      // accessible names of the columns (date pickers)
	cols  func(sel []int) [][]ui.Option // columns for the current selection (cascades depend on it)
	read  func() []int                  // selection indices of the bound value
	write func(sel []int, cols [][]ui.Option)
}

// PickerViewW is an inline set of scroll wheels.
type PickerViewW struct {
	c       *ui.Context
	th      *Theme
	bind    binding
	changed bool
	shown   bool
	visible int
	b       *ui.Box
	label   string   // accessible name of every column without its own
	names   []string // accessible names per column
}

func newPickerView(c *ui.Context, b binding) *PickerViewW {
	th := ThemeOf(c)
	w := &PickerViewW{c: c, th: th, bind: b, visible: th.PickerVisible}
	if v, ok := cfgNum(c, "PickerView", "visibleItemCount"); ok {
		w.Visible(int(v))
	}
	return w
}

// PickerView shows one wheel per column; value holds the selected
// option Value of each column. The shadcn style marks the selected row
// with a muted rounded-md band instead of hairlines.
//
// ConfigProvider defaults ("mobile.PickerView", also read by the views of
// Picker, CascadePickerView and DatePickerView): "visibleItemCount" (int,
// at least 3).
func PickerView(c *ui.Context, cols [][]ui.Option, value *[]string) *PickerViewW {
	return newPickerView(c, columnsBinding(cols, value))
}

func columnsBinding(cols [][]ui.Option, value *[]string) binding {
	return binding{
		cols: func([]int) [][]ui.Option { return cols },
		read: func() []int {
			sel := make([]int, len(cols))
			for i, col := range cols {
				if i < len(*value) {
					sel[i] = indexOf(col, (*value)[i])
				}
			}
			return sel
		},
		write: func(sel []int, cols [][]ui.Option) { *value = valuesOf(sel, cols) },
	}
}

func indexOf(col []ui.Option, v string) int {
	for j, o := range col {
		if o.Value == v {
			return j
		}
	}
	return 0
}

func valuesOf(sel []int, cols [][]ui.Option) []string {
	out := make([]string, len(cols))
	for i, col := range cols {
		if i < len(sel) && sel[i] < len(col) {
			out[i] = col[sel[i]].Value
		}
	}
	return out
}

// CascadePickerView shows linked wheels for a tree of options (province /
// city / district); value holds the Value path.
func CascadePickerView(c *ui.Context, opts []ui.CascaderOption, value *[]string) *PickerViewW {
	return newPickerView(c, cascadeBinding(opts, value))
}

func cascadeBinding(opts []ui.CascaderOption, value *[]string) binding {
	cols := func(sel []int) [][]ui.Option {
		var out [][]ui.Option
		level := opts
		for i := 0; len(level) > 0; i++ {
			col := make([]ui.Option, len(level))
			for j, o := range level {
				col[j] = ui.Option{Label: o.Label, Value: o.Value, Disabled: o.Disabled}
			}
			out = append(out, col)
			k := 0
			if i < len(sel) {
				k = max(0, min(sel[i], len(level)-1))
			}
			level = level[k].Children
		}
		return out
	}
	return binding{
		cols: cols,
		read: func() []int {
			var sel []int
			level := opts
			for i := 0; len(level) > 0; i++ {
				k := 0
				if i < len(*value) {
					for j, o := range level {
						if o.Value == (*value)[i] {
							k = j
						}
					}
				}
				sel = append(sel, k)
				level = level[k].Children
			}
			return sel
		},
		write: func(sel []int, cols [][]ui.Option) { *value = valuesOf(sel, cols) },
	}
}

// DatePrecision is the finest column of a date picker.
type DatePrecision uint8

const (
	PrecisionYear DatePrecision = iota
	PrecisionMonth
	PrecisionDay
	PrecisionHour
	PrecisionMinute
)

// DatePickerView shows year / month / day (… hour / minute) wheels bound
// to *value. Optional bounds are the earliest and latest selectable time
// (default: ten years either side of the value).
func DatePickerView(c *ui.Context, value *time.Time, p DatePrecision, bounds ...time.Time) *PickerViewW {
	var lo, hi time.Time
	if len(bounds) > 0 {
		lo = bounds[0]
	}
	if len(bounds) > 1 {
		hi = bounds[1]
	}
	return newPickerView(c, dateBinding(c, value, p, lo, hi))
}

func dateBinding(c *ui.Context, value *time.Time, p DatePrecision, lo, hi time.Time) binding {
	if value.IsZero() {
		*value = c.Now()
	}
	if lo.IsZero() {
		lo = time.Date(value.Year()-10, 1, 1, 0, 0, 0, 0, value.Location())
	}
	if hi.IsZero() {
		hi = time.Date(value.Year()+10, 12, 31, 23, 59, 0, 0, value.Location())
	}
	loc := c.Locale()
	n := int(p) + 1
	names := []string{loc.Calendar.Year, loc.Calendar.Month, loc.A11y.Day, loc.A11y.Hour, loc.A11y.Minute}[:n]
	dc := dateColumnsFor(c, dateColsKey{lo: lo.Year(), hi: hi.Year(), n: n, loc: loc,
		dayFmt: loc.Mobile.DayFmt, hourFmt: loc.Mobile.HourFmt, minFmt: loc.Mobile.MinuteFmt})
	cols := func(sel []int) [][]ui.Option {
		if n < 3 {
			return dc.cols[0]
		}
		y := lo.Year() + idx(sel, 0)
		m := time.Month(idx(sel, 1) + 1)
		days := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return dc.cols[days-28]
	}
	return binding{
		names: names,
		cols:  cols,
		read: func() []int {
			t := *value
			sel := []int{t.Year() - lo.Year(), int(t.Month()) - 1, t.Day() - 1, t.Hour(), t.Minute()}
			return sel[:n]
		},
		write: func(sel []int, _ [][]ui.Option) {
			t := *value
			y, m, d, h, mi := t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()
			y = lo.Year() + idx(sel, 0)
			if n > 1 {
				m = time.Month(idx(sel, 1) + 1)
			}
			if n > 2 {
				d = idx(sel, 2) + 1
			}
			if last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day(); d > last {
				d = last
			}
			if n > 3 {
				h = idx(sel, 3)
			}
			if n > 4 {
				mi = idx(sel, 4)
			}
			nt := time.Date(y, m, d, h, mi, 0, 0, t.Location())
			if nt.Before(lo) {
				nt = lo
			}
			if nt.After(hi) {
				nt = hi
			}
			*value = nt
		},
	}
}

// dateColsKey identifies the options of a date picker's wheels: the year
// range, the number of columns and the locale (with its formats, in case
// an application edits a registered pack).
type dateColsKey struct {
	lo, hi, n               int
	loc                     *ui.Locale
	dayFmt, hourFmt, minFmt string
}

// dateCols holds the ready-made columns of a date picker, one set per
// month length (28–31 days; index days-28). The sets share their option
// slices; callers only read them.
type dateCols struct {
	cols [4][][]ui.Option
}

type dateColsCache struct {
	m map[dateColsKey]*dateCols
}

var keyDateCols = ui.ID(0x6d6f62696c650010)

// dateColumnsFor returns the cached columns for k, building them once per
// key: an open date picker formats its years, months, days, hours and
// minutes only when the range, locale or precision changes, not every
// frame.
func dateColumnsFor(c *ui.Context, k dateColsKey) *dateCols {
	cache := ui.State(c, keyDateCols, func() dateColsCache { return dateColsCache{m: map[dateColsKey]*dateCols{}} })
	if dc, ok := cache.m[k]; ok {
		return dc
	}
	if len(cache.m) >= 8 {
		clear(cache.m)
	}
	dc := buildDateCols(k)
	cache.m[k] = dc
	return dc
}

func buildDateCols(k dateColsKey) *dateCols {
	loc := k.loc
	years := make([]ui.Option, max(0, k.hi-k.lo+1))
	for i := range years {
		years[i] = ui.Option{Label: loc.Year(k.lo + i), Value: strconv.Itoa(k.lo + i)}
	}
	base := [][]ui.Option{years}
	if k.n > 1 {
		ms := make([]ui.Option, 12)
		for i := range ms {
			ms[i] = ui.Option{Label: loc.Month(i + 1), Value: strconv.Itoa(i + 1)}
		}
		base = append(base, ms)
	}
	var ds, hs, mm []ui.Option
	if k.n > 2 {
		ds = make([]ui.Option, 31)
		for i := range ds {
			ds[i] = ui.Option{Label: fmt.Sprintf(k.dayFmt, i+1), Value: strconv.Itoa(i + 1)}
		}
	}
	if k.n > 3 {
		hs = make([]ui.Option, 24)
		for i := range hs {
			hs[i] = ui.Option{Label: fmt.Sprintf(k.hourFmt, i), Value: strconv.Itoa(i)}
		}
	}
	if k.n > 4 {
		mm = make([]ui.Option, 60)
		for i := range mm {
			mm[i] = ui.Option{Label: fmt.Sprintf(k.minFmt, i), Value: strconv.Itoa(i)}
		}
	}
	dc := &dateCols{}
	for j := range dc.cols {
		out := append(make([][]ui.Option, 0, k.n), base...)
		if k.n > 2 {
			out = append(out, ds[:28+j])
		}
		if k.n > 3 {
			out = append(out, hs)
		}
		if k.n > 4 {
			out = append(out, mm)
		}
		dc.cols[j] = out
	}
	return dc
}

func idx(sel []int, i int) int {
	if i < len(sel) {
		return sel[i]
	}
	return 0
}

// Visible sets how many rows each wheel shows (odd numbers centre best).
func (w *PickerViewW) Visible(n int) *PickerViewW { w.visible = max(3, n); return w }

// A11yLabel names the wheels for screen readers (each wheel is an
// adjustable slider whose value is the selected option); A11yLabels names
// them one by one. Date pickers name their columns year, month, day ….
func (w *PickerViewW) A11yLabel(s string) *PickerViewW { w.label = s; return w }

// A11yLabels sets the accessible name of each wheel.
func (w *PickerViewW) A11yLabels(names ...string) *PickerViewW { w.names = names; return w }

// columnName returns the accessible name of column i.
func (w *PickerViewW) columnName(i int) string {
	if i < len(w.names) && w.names[i] != "" {
		return w.names[i]
	}
	if i < len(w.bind.names) && w.bind.names[i] != "" {
		return w.bind.names[i]
	}
	return or(w.label, w.c.Locale().SelectPlaceholder)
}

// Show renders the wheels.
func (w *PickerViewW) Show() *PickerViewW {
	if w.shown {
		return w
	}
	w.shown = true
	c, th := w.c, w.th
	ih := th.PickerItemHeight
	h := ih * float32(w.visible)
	w.b = ui.Row(c, func() {
		sel := w.bind.read()
		cols := w.bind.cols(sel)
		sel = clampSel(sel, cols)
		for i := range cols {
			i := i
			ui.Key(c, i, func() {
				if k, ok := wheel(c, th, cols[i], sel[i], ih, h, w.columnName(i)); ok {
					sel[i] = k
					cols = w.bind.cols(sel)
					sel = clampSel(sel, cols)
					w.bind.write(sel, cols)
					w.changed = true
					c.Invalidate()
				}
			})
		}
	}, layout.H(h))
	w.b.Bg(th.Box)
	if th.shad() {
		// The selection band under the wheels' text.
		w.b.Paint(func(l *paint.List, r geom.Rect) {
			mid := geom.Rect{X: r.X + 8, Y: r.Y + (r.H-ih)/2, W: r.W - 16, H: ih}
			l.Fill(mid, geom.Rad(th.Radius), th.Fill)
		})
	}
	w.b.PaintOver(func(l *paint.List, r geom.Rect) {
		// Fade the rows towards the edges and frame the selected row.
		fade := (r.H - ih) / 2
		bg := th.Box
		clear := fadeCol(bg, 0)
		l.LinearStops(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: fade}, geom.Radii{}, geom.Pt(0, r.Y), geom.Pt(0, r.Y+fade),
			paint.Stop{Offset: 0, Color: fadeCol(bg, 0.95)}, paint.Stop{Offset: 1, Color: fadeCol(clear, 0)})
		l.LinearStops(geom.Rect{X: r.X, Y: r.Bottom() - fade, W: r.W, H: fade}, geom.Radii{}, geom.Pt(0, r.Bottom()-fade), geom.Pt(0, r.Bottom()),
			paint.Stop{Offset: 0, Color: fadeCol(clear, 0)}, paint.Stop{Offset: 1, Color: fadeCol(bg, 0.95)})
		if th.shad() {
			return
		}
		mid := geom.Rect{X: r.X, Y: r.Y + fade, W: r.W, H: ih}
		if th.Style == Vant {
			mid.X, mid.W = r.X+th.PadH, r.W-2*th.PadH
		}
		hairlineTop(l, mid, mid.X, th.Border)
		hairline(l, mid, mid.X, th.Border)
	})
	return w
}

func clampSel(sel []int, cols [][]ui.Option) []int {
	out := make([]int, len(cols))
	for i, col := range cols {
		if i < len(sel) {
			out[i] = max(0, min(sel[i], len(col)-1))
		}
	}
	return out
}

// Changed reports a wheel settling on a new value this frame.
func (w *PickerViewW) Changed() bool { w.Show(); return w.changed }

// ID returns the wheel row's ID.
func (w *PickerViewW) ID() ui.ID { w.Show(); return w.b.ID() }

type wheelState struct {
	sc  Scroller
	gen uint64
}

// wheel draws one picker column and reports the index it settled on.
// For screen readers and the keyboard it is a vertical slider named name:
// Up / Down (Left / Right, Increment / Decrement) move one row, Page Up /
// Page Down five, Home / End to the ends; SetValue takes an option label,
// value or index.
func wheel(c *ui.Context, th *Theme, opts []ui.Option, sel int, ih, h float32, name string) (int, bool) {
	b := ui.Div(c, layout.Grow(1), layout.H(h)).Clip()
	id := b.ID()
	st := ui.State[wheelState](c, id, nil)
	sc := &st.sc
	n := len(opts)
	sc.Min, sc.Max, sc.Snap, sc.Dim = 0, float64(max(0, n-1))*float64(ih), float64(ih), float64(h)
	now := c.Now()
	if sc.Mode == Idle {
		sc.Pos = float64(sel) * float64(ih)
	}
	g := Area(c, b, AxisVertical, nil)
	ia := b.Focusable(input.CursorDefault)
	b.Semantics(a11y.RoleSlider, name).A11yState(a11y.Vertical)
	if c.A11yEnabled() && n > 0 {
		cur := max(0, min(sel, n-1))
		label := opts[cur].Label
		b.A11y(func(s *a11y.Node) {
			s.HasRange, s.ValueNow, s.ValueMin, s.ValueMax, s.Step = true, float64(cur), 0, float64(n-1), 1
			s.Value = label
			stepSem(s)
			s.Actions |= a11y.SetValue
		})
	}
	// Keyboard and screen reader steps.
	if n > 0 {
		base := sel
		if sc.Active() {
			base = int(math.Round(sc.Pos / float64(ih)))
		}
		to := -1
		step := func(d int) {
			if d == 0 {
				return
			}
			t := max(0, min(base+d, n-1))
			for t > 0 && t < n-1 && opts[t].Disabled {
				t += sign(float32(d))
			}
			if !opts[t].Disabled {
				to, base = t, t
			}
		}
		if act, arg, ok := c.A11yAction(id); ok {
			switch act {
			case a11y.Increment:
				step(1)
			case a11y.Decrement:
				step(-1)
			case a11y.SetValue:
				if k := optionIndex(opts, arg); k >= 0 {
					step(k - base)
				}
			}
			keyPresses(c, id) // the action's synthesized keys
		} else if ia.Focused {
			for _, k := range keyPresses(c, id) {
				switch k {
				case input.KeyDown, input.KeyRight:
					step(1)
				case input.KeyUp, input.KeyLeft:
					step(-1)
				case input.KeyPageDown:
					step(5)
				case input.KeyPageUp:
					step(-5)
				case input.KeyHome:
					step(-n)
				case input.KeyEnd:
					step(n)
				}
			}
		}
		if to >= 0 && sc.Mode != Dragging {
			sc.SnapTo(float64(to)*float64(ih), now)
			st.gen++
		}
	}
	if g.TouchStart && sc.Active() {
		sc.Stop()
	}
	if g.Pan.Began {
		sc.BeginDrag()
	}
	if g.Pan.Active {
		sc.DragBy(-float64(g.Pan.Delta.Y))
	}
	if g.Pan.Ended {
		sc.Release(-float64(g.Pan.Velocity.Y), now)
		st.gen++
	}
	if g.Tap && !sc.Active() {
		// Tap a row above or below the centre to move it there.
		if r, ok := c.RectOf(id); ok {
			k := int(math.Floor(float64((g.TapPos.Y - (r.Y + r.H/2 - ih/2)) / ih)))
			if t := sel + k; k != 0 && t >= 0 && t < n {
				sc.SnapTo(float64(t)*float64(ih), now)
				st.gen++
			}
		}
	}
	if _, dy, ok := c.WheelFor(id); ok && sc.Mode != Dragging {
		t := max(0, min(float64(n-1), math.Round(sc.Pos/float64(ih))+float64(sign(dy))))
		sc.SnapTo(t*float64(ih), now)
		st.gen++
	}
	if sc.Active() {
		if pump(c, id, st.gen) {
			sc.Step(now)
		} else {
			sc.Settle()
		}
	}
	pos := float32(sc.Pos)
	fs := th.Font(16, text.Regular)
	b.Paint(func(l *paint.List, r geom.Rect) {
		// A box's own painter is not clipped by Clip (only children are).
		l.Clip(r, geom.Radii{})
		defer l.Pop()
		cy := r.Y + r.H/2
		first := max(0, int((pos-r.H/2)/ih)-1)
		for i := first; i < n; i++ {
			y := cy - ih/2 + float32(i)*ih - pos
			if y > r.Bottom() {
				break
			}
			col := th.Text
			if opts[i].Disabled {
				col = th.TextLight
			}
			lay := text.Shape(opts[i].Label, fs, text.Options{MaxWidth: r.W - 8, MaxLines: 1, Ellipsis: true})
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, y+(ih-lay.Height)/2), lay.Glyphs, col)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r)
		}
	})
	if sc.Mode == Idle && n > 0 {
		k := int(math.Round(sc.Pos / float64(ih)))
		k = nearestEnabled(opts, max(0, min(k, n-1)))
		if k >= 0 && k != sel {
			return k, true
		}
	}
	return sel, false
}

// nearestEnabled returns the enabled option closest to k (the later one
// on a tie), or -1 when every option is disabled. A wheel that comes to
// rest on a disabled row settles here instead.
func nearestEnabled(opts []ui.Option, k int) int {
	n := len(opts)
	for d := 0; d < n; d++ {
		if t := k + d; t >= 0 && t < n && !opts[t].Disabled {
			return t
		}
		if t := k - d; t >= 0 && t < n && !opts[t].Disabled {
			return t
		}
	}
	return -1
}

// optionIndex finds an option by label, value or (numeric) index.
func optionIndex(opts []ui.Option, s string) int {
	s = strings.TrimSpace(s)
	for i, o := range opts {
		if o.Label == s || o.Value == s {
			return i
		}
	}
	if k, err := strconv.Atoi(s); err == nil && k >= 0 && k < len(opts) {
		return k
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if k := int(math.Round(f)); k >= 0 && k < len(opts) {
			return k
		}
	}
	return -1
}

func sign(v float32) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Picker (popup)

type pickerState struct {
	wasOpen bool
	strs    []string
	t       time.Time
}

// PickerW is a bottom popup with a toolbar (cancel / title / confirm) and
// wheels; the bound value only changes on confirm.
type PickerW struct {
	c         *ui.Context
	th        *Theme
	open      *bool
	title     string
	a         *ui.Box
	st        *pickerState
	view      func(st *pickerState) *PickerViewW
	commit    func(st *pickerState)
	confirmed bool
	cancelled bool
	shown     bool
}

// Picker creates a popup picker bound to *open; choose the wheels with
// Columns, Cascade or Date, then call Show. The shadcn style shows it as
// a Drawer (grab handle, semibold title).
//
// ConfigProvider defaults ("mobile.Picker"): "closeOnMaskClick" (bool);
// the wheels read "mobile.PickerView".
func Picker(c *ui.Context, open *bool, title string) *PickerW {
	w := &PickerW{c: c, th: ThemeOf(c), open: open, title: title, a: anchor(c)}
	w.st = ui.State[pickerState](c, w.a.ID(), nil)
	return w
}

func (w *PickerW) syncStrings(value *[]string) {
	if *w.open && !w.st.wasOpen {
		w.st.strs = append([]string(nil), (*value)...)
	}
	w.commit = func(st *pickerState) { *value = append([]string(nil), st.strs...) }
}

// Columns shows independent wheels; value receives the Values on confirm.
func (w *PickerW) Columns(cols [][]ui.Option, value *[]string) *PickerW {
	w.syncStrings(value)
	w.view = func(st *pickerState) *PickerViewW { return PickerView(w.c, cols, &st.strs) }
	return w
}

// Cascade shows linked wheels for a tree of options.
func (w *PickerW) Cascade(opts []ui.CascaderOption, value *[]string) *PickerW {
	w.syncStrings(value)
	w.view = func(st *pickerState) *PickerViewW { return CascadePickerView(w.c, opts, &st.strs) }
	return w
}

// Date shows date / time wheels down to precision p (optional bounds as
// in DatePickerView).
func (w *PickerW) Date(value *time.Time, p DatePrecision, bounds ...time.Time) *PickerW {
	if *w.open && !w.st.wasOpen {
		w.st.t = *value
	}
	w.commit = func(st *pickerState) { *value = st.t }
	w.view = func(st *pickerState) *PickerViewW { return DatePickerView(w.c, &st.t, p, bounds...) }
	return w
}

// Show renders the popup.
func (w *PickerW) Show() *PickerW {
	if w.shown {
		return w
	}
	w.shown = true
	c, th := w.c, w.th
	st := w.st
	p := newPopup(c, w.open).A11yLabel(w.title)
	p.ownBottom = true
	if v, ok := cfgBool(c, "Picker", "closeOnMaskClick"); ok {
		p.CloseOnMask(v)
	}
	p.Body(func() {
		w.toolbar()
		if w.view != nil {
			w.view(st).A11yLabel(w.title).Show()
		}
		bottomInset(c)
	})
	if p.Closed() {
		w.cancelled = true
	}
	if w.confirmed && w.commit != nil {
		w.commit(st)
	}
	if w.confirmed || w.cancelled {
		*w.open = false
		c.Invalidate()
	}
	st.wasOpen = *w.open
	_ = th
	return w
}

func (w *PickerW) toolbar() {
	c, th := w.c, w.th
	vant := th.Style == Vant
	loc := c.Locale()
	cancel, ok := loc.Cancel, loc.Confirm
	h := float32(42)
	bf := th.Font(th.FontMain, text.Regular)
	tf := th.Font(th.FontMain, text.Regular)
	if vant {
		h = 44
		bf = th.Font(14, text.Regular)
		tf = th.Font(16, text.Semibold)
	} else if th.shad() {
		// Drawer header: semibold title, muted cancel, foreground confirm.
		h = 44
		bf = th.Font(th.FontMain, text.Medium)
		tf = th.Font(th.FontBody, text.Semibold)
	}
	ui.Row(c, func() {
		bc := ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: textW(cancel, bf) + 2*th.PadH, H: h} }, nop)
		bc.Semantics(a11y.RoleButton, cancel)
		prc := Touchable(c, bc)
		bc.Paint(func(l *paint.List, r geom.Rect) {
			col := th.Primary
			if vant || th.shad() {
				col = th.TextWeak
			}
			if prc.Pressed {
				col = fadeCol(col, 0.6)
			}
			drawTextCenter(l, cancel, bf, r, col)
		})
		title := w.title
		ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: 0, H: h} }, func(l *paint.List, r geom.Rect) {
			lay := text.Shape(title, tf, text.Options{MaxWidth: r.W, MaxLines: 1, Ellipsis: true})
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.Text)
		}, layout.Grow(1)).Semantics(a11y.RoleHeading, title)
		bo := ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: textW(ok, bf) + 2*th.PadH, H: h} }, nop)
		bo.Semantics(a11y.RoleButton, ok)
		pro := Touchable(c, bo)
		bo.Paint(func(l *paint.List, r geom.Rect) {
			col := th.Primary
			if pro.Pressed {
				col = fadeCol(col, 0.6)
			}
			drawTextCenter(l, ok, bf, r, col)
		})
		if prc.Tap {
			w.cancelled = true
		}
		if pro.Tap {
			w.confirmed = true
		}
	}).Paint(func(l *paint.List, r geom.Rect) {
		if !vant && !th.shad() {
			hairline(l, r, r.X, th.Border)
		}
	})
}

// Confirmed reports a confirm this frame (the bound value was updated).
func (w *PickerW) Confirmed() bool { w.Show(); return w.confirmed }

// Cancelled reports a cancel this frame.
func (w *PickerW) Cancelled() bool { w.Show(); return w.cancelled }
