package ui

// DatePickerPanel: the date panel of DatePicker as an embeddable widget
// without input or popup (Element Plus el-date-picker-panel, Ant Design
// DatePicker's panel). Day grids reuse date_util.go (dtGridStart,
// dtWeekNames, dtDayCell, dtCellSize, dtMoveCursor); the month, quarter
// and year grids and the clickable header live here.

import (
	"fmt"
	"github.com/tangoui/tango/ui/a11y"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// PickerType is the granularity of a DatePickerPanel (Ant picker,
// Element type).
//
// Append-only: never reorder or insert; the zero value stays PickerDate.
type PickerType uint8

const (
	PickerDate PickerType = iota
	PickerWeek
	PickerMonth
	PickerQuarter
	PickerYear
)

type fillDPState struct {
	mode       PickerType // view level shown: PickerDate (days), Month, Quarter or Year
	view       time.Time  // left panel anchor: month start (days), Jan 1 (months/quarters), decade start (years)
	has        bool
	picker     PickerType
	hover      time.Time // hovered day / period of this build
	lastHover  time.Time // hovered at the previous build (drawn as preview)
	cursor     time.Time
	rangeStart time.Time // first end of a range being picked
}

// DatePickerPanelW is an inline date panel.
type DatePickerPanelW struct {
	_ [0]func()

	c        *Context
	val      *time.Time
	end      *time.Time
	ranged   bool
	picker   PickerType
	disabled func(time.Time) bool
	off      bool
	border   bool
	st       *fillDPState
	n        *node
	ia       Interaction
	changed  bool
	z        *lazy
	// Set when the panel is the popup of a DatePicker (no node of its own):
	// ids receives the grid / active cell semantics, onPick replaces the
	// write to *val of a single pick.
	ids    *dtGridIDs
	onPick func(time.Time)
	// onRange replaces the write to *val / *end of a completed range
	// (the popup of a RangePicker with Picker).
	onRange func(a, b time.Time)
	// multi lists the selected period starts of a MultiDatePicker popup
	// (drawn as selected; picks go to onPick, which toggles them).
	multi []time.Time
}

// DatePickerPanel adds an inline date panel bound to *val. Picked values
// are UTC midnights normalized to the start of their period (week start
// per Locale.WeekStart, first day of the month, quarter or year).
//
//	ui.DatePickerPanel(c, &day)
//	ui.DatePickerPanel(c, &from).Range(&to).Picker(ui.PickerMonth)
func DatePickerPanel(c *Context, val *time.Time) *DatePickerPanelW {
	w := &DatePickerPanelW{c: c, val: val, border: true}
	w.n = c.add("fill-datepanel", "")
	w.n.ln.Dir = layout.Column
	w.n.ln.NoStretch = true
	w.n.ownKeys = true
	w.st = State[fillDPState](c, w.n.id, nil)
	w.ia = c.interact(w.n, true, input.CursorDefault)
	w.z = c.lazy(w.n, w.build)
	return w
}

// Picker sets the granularity: PickerDate (default), PickerWeek,
// PickerMonth, PickerQuarter or PickerYear.
func (w *DatePickerPanelW) Picker(t PickerType) *DatePickerPanelW { w.picker = t; return w }

// Range turns on range selection: *val holds the start and *end the end
// (both set by the second click; two panels side by side).
func (w *DatePickerPanelW) Range(end *time.Time) *DatePickerPanelW {
	w.end, w.ranged = end, end != nil
	return w
}

// DisabledDate marks periods that cannot be picked (called with the first
// day of the period).
func (w *DatePickerPanelW) DisabledDate(fn func(time.Time) bool) *DatePickerPanelW {
	w.disabled = fn
	return w
}

// Border toggles the panel border (default on).
func (w *DatePickerPanelW) Border(on bool) *DatePickerPanelW { w.border = on; return w }

// Disabled turns the whole panel off.
func (w *DatePickerPanelW) Disabled(on bool) *DatePickerPanelW {
	if on {
		w.off = true
		w.n.disabled, w.n.focus = true, false
	}
	return w
}

// Changed reports a new value (both ends in range mode) this frame.
func (w *DatePickerPanelW) Changed() bool { w.z.run(); return w.changed }

// View returns the view level shown (PickerDate means the day grid).
func (w *DatePickerPanelW) View() PickerType { w.z.run(); return w.st.mode }

// ---------------------------------------------------------------------------
// Period arithmetic

// fillPeriodStart normalizes t to the start of its period (UTC midnight).
func fillPeriodStart(t time.Time, p PickerType, ws time.Weekday) time.Time {
	d := dtDayUTC(t)
	switch p {
	case PickerWeek:
		off := (int(d.Weekday()) - int(ws) + 7) % 7
		return d.AddDate(0, 0, -off)
	case PickerMonth:
		return dtMonthUTC(d)
	case PickerQuarter:
		q := (int(d.Month()) - 1) / 3
		return time.Date(d.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, time.UTC)
	case PickerYear:
		return time.Date(d.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return d
}

// fillPeriodLast returns the last day of the period starting at s.
func fillPeriodLast(s time.Time, p PickerType) time.Time {
	switch p {
	case PickerWeek:
		return s.AddDate(0, 0, 6)
	case PickerMonth:
		return s.AddDate(0, 1, -1)
	case PickerQuarter:
		return s.AddDate(0, 3, -1)
	case PickerYear:
		return s.AddDate(1, 0, -1)
	}
	return s
}

// fillViewLevel maps a picker to the grid it is picked in.
func fillViewLevel(p PickerType) PickerType {
	if p == PickerWeek {
		return PickerDate
	}
	return p
}

// fillAnchor returns the panel anchor of level for a date.
func fillAnchor(t time.Time, level PickerType) time.Time {
	switch level {
	case PickerMonth, PickerQuarter:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	case PickerYear:
		return time.Date(t.Year()-t.Year()%10, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return dtMonthUTC(t)
}

// fillAnchorStep moves an anchor by n panels of level.
func fillAnchorStep(a time.Time, level PickerType, n int) time.Time {
	switch level {
	case PickerMonth, PickerQuarter:
		return a.AddDate(n, 0, 0)
	case PickerYear:
		return a.AddDate(10*n, 0, 0)
	}
	return dtAddMonths(a, n)
}

// fillDPOrder orders two period starts.
func fillDPOrder(a, b time.Time) (time.Time, time.Time) {
	if b.Before(a) {
		return b, a
	}
	return a, b
}

// ---------------------------------------------------------------------------
// Build

func (w *DatePickerPanelW) ws() time.Weekday { return dtWeekStart(w.c) }

func (w *DatePickerPanelW) sync() {
	st := w.st
	if st.has && st.picker == w.picker {
		return
	}
	st.has, st.picker = true, w.picker
	st.mode = fillViewLevel(w.picker)
	base := *w.val
	if base.IsZero() {
		base = w.c.now
	}
	st.view = fillAnchor(base, st.mode)
	st.cursor, st.rangeStart = time.Time{}, time.Time{}
}

// selected returns the committed (or in-progress) ends, normalized.
func (w *DatePickerPanelW) selected() (a, b time.Time) {
	ws := w.ws()
	if !w.st.rangeStart.IsZero() {
		return w.st.rangeStart, time.Time{}
	}
	if !w.val.IsZero() {
		a = fillPeriodStart(*w.val, w.picker, ws)
	}
	if w.ranged && !w.end.IsZero() {
		b = fillPeriodStart(*w.end, w.picker, ws)
	}
	return a, b
}

// commit applies a picked period start p.
func (w *DatePickerPanelW) commit(p time.Time) {
	c, st := w.c, w.st
	if !w.ranged && w.onPick != nil {
		st.cursor = p
		c.invalid = true
		w.onPick(p)
		return
	}
	if !w.ranged {
		if !w.val.Equal(p) {
			*w.val = p
		}
		w.changed = true
	} else if st.rangeStart.IsZero() {
		st.rangeStart = p
	} else {
		a, b := fillDPOrder(st.rangeStart, p)
		st.rangeStart = time.Time{}
		if w.onRange != nil {
			st.cursor = p
			c.invalid = true
			w.onRange(a, b)
			return
		}
		*w.val, *w.end = a, b
		w.changed = true
	}
	st.cursor = p
	c.invalid = true
}

// pickAt handles a click on a cell of the current level (period start p).
func (w *DatePickerPanelW) pickAt(p time.Time) {
	st := w.st
	base := fillViewLevel(w.picker)
	if st.mode == base {
		if w.picker == PickerWeek {
			p = fillPeriodStart(p, PickerWeek, w.ws())
		}
		w.commit(p)
		return
	}
	// Drill down: year -> month (or quarter) -> day.
	switch st.mode {
	case PickerYear:
		if base == PickerQuarter {
			st.mode = PickerQuarter
		} else {
			st.mode = PickerMonth
		}
	case PickerMonth, PickerQuarter:
		st.mode = PickerDate
	}
	st.view = fillAnchor(p, st.mode)
	st.cursor = time.Time{}
	w.c.invalid = true
}

func (w *DatePickerPanelW) build() {
	c := w.c
	th, tk := dtTheme(c)
	w.sync()
	el := th.Flavor == theme.Element
	if w.border {
		rad := th.RadiusLG
		if el {
			rad = th.Radius
		}
		w.n.radii = geom.Rad(rad)
		w.n.paint = func(l *paint.List, r geom.Rect) {
			l.Fill(r, w.n.radii, tk.PanelBg)
			l.Stroke(r, w.n.radii, 1, tk.PanelBorderColor)
		}
	}
	if w.ia.FocusVisible {
		w.n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, w.n.radii) }
	}
	fillSem(c, w.n, a11y.RoleGroup, c.Locale().SelectDate, fillIf(w.off, a11y.Disabled))
	if !w.off {
		w.keys()
	}
	c.body(w.n, func() {
		if w.off {
			Disabled(c, true, w.content)
		} else {
			w.content()
		}
	})
	if w.off {
		w.n.opacity = 0.55
	}
}

// reset re-anchors the view at the value on the next build (a DatePicker
// popup that opens again).
func (w *DatePickerPanelW) reset() { w.st.has = false }

// content builds the panels (without the outer node).
func (w *DatePickerPanelW) content() {
	c, st := w.c, w.st
	_, tk := dtTheme(c)
	w.sync()
	if w.ids != nil {
		*w.ids = dtGridIDs{}
	}
	st.lastHover, st.hover = st.hover, time.Time{}
	hover := &st.hover
	panels := 1
	if w.ranged {
		panels = 2
	}
	Row(c, func() {
		for i := 0; i < panels; i++ {
			Key(c, i, func() {
				anchor := fillAnchorStep(st.view, st.mode, i)
				prev := i == 0
				next := i == panels-1
				if st.mode == PickerDate {
					w.dayPanel(anchor, prev, next, hover)
				} else {
					w.gridPanel(anchor, prev, next, hover)
				}
			})
			if i < panels-1 {
				Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 1} }, func(l *paint.List, r geom.Rect) {
					l.Fill(r, geom.Radii{}, tk.SplitColor)
				}, layout.Self(layout.AlignStretch))
			}
		}
	})
}

// keys applies keyboard navigation to the cursor of the current level.
func (w *DatePickerPanelW) keys() {
	for _, k := range w.c.KeysFor(w.n.id) {
		if k.Down {
			w.key(k)
		}
	}
}

// key applies one key press to the cursor of the current level; it
// reports whether the key was panel navigation. Ctrl/Cmd+Up zooms out to
// the next coarser view (the keyboard equivalent of the header titles).
func (w *DatePickerPanelW) key(k input.KeyEvent) bool {
	c, st := w.c, w.st
	w.sync()
	if k.Key == input.KeyUp && k.Mods.Shortcut() {
		return w.zoomOut()
	}
	cur := st.cursor
	if cur.IsZero() {
		cur = *w.val
		if cur.IsZero() {
			cur = c.now
		}
		cur = fillPeriodStart(cur, st.mode, w.ws())
	}
	if k.Key == input.KeyEnter || k.Key == input.KeySpace {
		if w.disabled == nil || !w.disabled(cur) || st.mode != fillViewLevel(w.picker) {
			mode := st.mode
			w.pickAt(cur)
			if st.mode != mode {
				// Drilled down: keep the keyboard on the chosen period.
				st.cursor = fillPeriodStart(cur, st.mode, w.ws())
			}
		}
		return true
	}
	var next time.Time
	ok := true
	if st.mode == PickerDate {
		next, ok = dtMoveCursor(cur, k.Key, c.IsRTL())
	} else {
		cols := w.cols(st.mode)
		step := func(n int) time.Time {
			switch st.mode {
			case PickerQuarter:
				return cur.AddDate(0, 3*n, 0)
			case PickerYear:
				return cur.AddDate(n, 0, 0)
			}
			return cur.AddDate(0, n, 0)
		}
		prev, nxt := c.inlineKey(k.Key) // visual: Left is next in RTL
		switch {
		case prev:
			next = step(-1)
		case nxt:
			next = step(1)
		case k.Key == input.KeyUp:
			next = step(-cols)
		case k.Key == input.KeyDown:
			next = step(cols)
		case k.Key == input.KeyPageUp:
			next = fillAnchorStep(cur, st.mode, -1)
		case k.Key == input.KeyPageDown:
			next = fillAnchorStep(cur, st.mode, 1)
		default:
			ok = false
		}
	}
	if !ok {
		return false
	}
	st.cursor = next
	left := fillAnchor(st.view, st.mode)
	last := fillAnchorStep(left, st.mode, 1)
	if w.ranged {
		last = fillAnchorStep(left, st.mode, 2)
	}
	if a := fillAnchor(next, st.mode); a.Before(left) || !a.Before(last) {
		st.view = a
		if w.ranged && !a.Before(last) {
			st.view = fillAnchorStep(a, st.mode, -1)
		}
	}
	c.invalid = true
	return true
}

// zoomOut shows the next coarser view (days -> months -> decade), keeping
// the cursor's period; it reports false at the decade level.
func (w *DatePickerPanelW) zoomOut() bool {
	c, st := w.c, w.st
	base := st.cursor
	if base.IsZero() {
		base = st.view
	}
	switch st.mode {
	case PickerDate:
		st.mode = PickerMonth
		if w.picker == PickerQuarter {
			st.mode = PickerQuarter
		}
	case PickerMonth, PickerQuarter:
		st.mode = PickerYear
	default:
		return false
	}
	st.view = fillAnchor(base, st.mode)
	st.cursor = fillPeriodStart(base, st.mode, w.ws())
	c.invalid = true
	return true
}

// header builds the panel header: nav arrows around clickable titles.
func (w *DatePickerPanelW) header(anchor time.Time, prev, next bool, titles func()) {
	c, st := w.c, w.st
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	loc := c.Locale()
	nav := func(ic *icon.Icon, label string, n int, show bool) {
		if shad {
			// shadcn Calendar: a single pair of chevrons. The day view
			// steps months (years via the clickable titles); the coarser
			// views step their own page with the same chevrons.
			if st.mode == PickerDate && (n == 12 || n == -12) {
				return
			}
			if ic == icon.DoubleLeft {
				ic = icon.Left
			} else if ic == icon.DoubleRight {
				ic = icon.Right
			}
		}
		if !show {
			if shad {
				Gap(c, 28)
			} else {
				Gap(c, 24)
			}
			return
		}
		b := IconButton(c, ic, label).Size(24, 12).NoHoverBg()
		if el {
			b.Size(24, 14)
		}
		if shad {
			dtShadcnNav(c, b)
		}
		if b.Clicked() {
			if st.mode == PickerDate {
				st.view = dtAddMonths(st.view, n)
			} else {
				st.view = fillAnchorStep(st.view, st.mode, n)
			}
			c.invalid = true
		}
	}
	bigPrev, bigNext := loc.PrevYear, loc.NextYear
	big := 12
	if st.mode != PickerDate {
		big = 1
	}
	if st.mode == PickerYear {
		bigPrev, bigNext = loc.DatePicker.PrevDecade, loc.DatePicker.NextDecade
	}
	hdr := Row(c, func() {
		nav(icon.DoubleLeft, bigPrev, -big, prev)
		if st.mode == PickerDate {
			nav(icon.Left, loc.PrevMonth, -1, prev)
		}
		Spacer(c)
		Row(c, titles, layout.Gap(4), layout.Middle())
		Spacer(c)
		if st.mode == PickerDate {
			nav(icon.Right, loc.NextMonth, 1, next)
		}
		nav(icon.DoubleRight, bigNext, big, next)
	}, layout.H(tk.HeaderHeight), layout.Middle())
	switch {
	case el:
		hdr.Style(layout.PadVH(0, 12), layout.Margin(geom.Insets{Top: 4}))
	case shad:
		hdr.Style(layout.PadVH(0, 12), layout.Gap(4), layout.Margin(geom.Insets{Top: 4}))
	default:
		hdr.Style(layout.PadVH(0, 8))
	}
	dtHeaderDivider(hdr, tk)
}

// headBtn is a clickable header title (year / month); it reports a click.
func (w *DatePickerPanelW) headBtn(label string, active bool) bool {
	c := w.c
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	fs, wt := th.FontSize, text.Semibold
	if el {
		fs, wt = 16, text.Medium
	} else if th.Flavor == theme.StyleShadcn {
		wt = text.Medium
	}
	n := c.add("fill-dp-title", label)
	n.ln.NoStretch = true
	n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: entTextW(c, label, fs, wt) + 2, H: 24} }
	ia := Interaction{}
	if active {
		ia = c.interact(n, false, input.CursorPointer)
		fillSem(c, n, a11y.RoleButton, label)
		fillKeyboard(c, n) // Ctrl/Cmd+Up on the focused panel or picker
	} else {
		fillSem(c, n, a11y.RoleHeading, label)
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		col := tk.HeaderColor
		if ia.Hovered && tk.HeaderHoverBg.A > 0 {
			l.Fill(r.Inset(geom.Insets{Left: -4, Right: -4, Top: -2, Bottom: -2}), geom.Rad(th.RadiusSM), tk.HeaderHoverBg)
		}
		if ia.Hovered && tk.HeaderHoverColor.A > 0 {
			col = tk.HeaderHoverColor
		}
		entText(c, l, label, r.X+1, r, fs, wt, col, 0)
	}
	return ia.Clicked
}

// titles builds the header titles of the level shown at anchor.
func (w *DatePickerPanelW) titles(anchor time.Time) func() {
	c, st := w.c, w.st
	loc := c.Locale()
	return func() {
		switch st.mode {
		case PickerDate:
			y, m := anchor.Year(), int(anchor.Month())
			yearBtn := func() {
				if w.headBtn(loc.Year(y), true) {
					st.mode, st.view = PickerYear, fillAnchor(anchor, PickerYear)
					c.invalid = true
				}
			}
			monthBtn := func() {
				if w.headBtn(loc.Month(m), true) {
					st.mode, st.view = PickerMonth, fillAnchor(anchor, PickerMonth)
					c.invalid = true
				}
			}
			// Follow the locale's year/month order ("2026年11月" / "Nov 2026").
			// The two titles form a group named by the whole month.
			title := loc.YearMonth(y, m)
			g := c.add("fill-dp-titles", title)
			g.ln.Dir, g.ln.Gap, g.ln.Align, g.ln.NoStretch = layout.Row, 4, layout.AlignCenter, true
			fillSem(c, g, a11y.RoleGroup, title)
			c.body(g, func() {
				if strings.HasPrefix(title, fmt.Sprint(y)) {
					yearBtn()
					monthBtn()
				} else {
					monthBtn()
					yearBtn()
				}
			})
		case PickerMonth, PickerQuarter:
			if w.headBtn(loc.Year(anchor.Year()), true) {
				st.mode, st.view = PickerYear, fillAnchor(anchor, PickerYear)
				c.invalid = true
			}
		default:
			y := anchor.Year()
			w.headBtn(fmt.Sprintf("%d-%d", y, y+9), false)
		}
	}
}

// padH is the horizontal padding of the panel body.
func (w *DatePickerPanelW) padH() float32 {
	_, tk := dtTheme(w.c)
	return tk.PanelPaddingInline
}

// dayPanel builds a month of days (date and week pickers).
func (w *DatePickerPanelW) dayPanel(month time.Time, prev, next bool, hover *time.Time) {
	c, st := w.c, w.st
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	cw, ch := dtCellSize(th)
	loc := c.Locale()
	ws := w.ws()
	week := w.picker == PickerWeek && st.mode == PickerDate
	pw := dtPanelWidth(th)
	if week {
		pw += cw
	}
	a, b := w.selected()
	m := dtMonth{view: month, today: c.now, cursor: st.cursor, disabled: w.disabled, hideOut: w.ranged, hover: hover, ids: w.ids}
	if st.mode == fillViewLevel(w.picker) {
		m.selA, m.selB = a, b
		if w.multi != nil {
			m.selA, m.selB = time.Time{}, time.Time{}
			for _, s := range w.multi {
				if week {
					for i := 0; i < 7; i++ {
						m.sels = append(m.sels, s.AddDate(0, 0, i))
					}
				} else {
					m.sels = append(m.sels, s)
				}
			}
			a = time.Time{}
		}
		if week && !a.IsZero() {
			if b.IsZero() {
				m.selB = fillPeriodLast(a, PickerWeek)
			} else {
				lo, hi := fillDPOrder(a, b)
				m.selA, m.selB = lo, fillPeriodLast(hi, PickerWeek)
			}
		}
		hv := st.hoverShown()
		switch {
		case w.ranged && !st.rangeStart.IsZero() && !hv.IsZero():
			pa, pb := st.rangeStart, hv
			if week {
				pb = fillPeriodStart(pb, PickerWeek, ws)
				lo, hi := fillDPOrder(pa, pb)
				pa, pb = lo, fillPeriodLast(hi, PickerWeek)
			}
			m.prevA, m.prevB = pa, pb
		case week && !hv.IsZero():
			pa := fillPeriodStart(hv, PickerWeek, ws)
			m.prevA, m.prevB = pa, fillPeriodLast(pa, PickerWeek)
		}
	}
	Column(c, func() {
		w.header(month, prev, next, w.titles(month))
		grid := Column(c, func() {
			wk := Row(c, func() {
				if week {
					Gap(c, cw)
				}
				for _, d := range dtWeekNames(loc, ws) {
					dtWeekLabel(c, th, tk, d, cw, ch)
				}
			})
			wk.n.setRole(a11y.RoleRow)
			if el {
				wk.Style(layout.Margin(geom.Insets{Bottom: 4}))
				wk.Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.SplitColor)
				})
			}
			first := dtGridStart(month, ws)
			for row := 0; row < 6; row++ {
				Row(c, func() {
					if week {
						// ISO week number of the row's Thursday.
						var thu time.Time
						for i := 0; i < 7; i++ {
							if d := first.AddDate(0, 0, row*7+i); d.Weekday() == time.Thursday {
								thu = d
							}
						}
						_, wn := thu.ISOWeek()
						wl := Text(c, fmt.Sprint(wn)).Align(AlignCenter).Color(tk.WeekNumberColor).Layout(layout.W(cw), layout.H(ch)).LineHeight(ch)
						if c.a11yOn {
							wl.n.setRole(a11y.RoleRowHeader).setName(p2aWeekLabel(loc, thu))
						}
					}
					for col := 0; col < 7; col++ {
						day := first.AddDate(0, 0, row*7+col)
						inMonth := day.Month() == month.Month()
						if m.hideOut && !inMonth {
							bl := c.add("dt-blank", "")
							bl.ln.Width, bl.ln.Height = cw, ch
							continue
						}
						if dtDayCell(c, m, day, inMonth, cw, ch, row*7+col) && !w.off {
							w.pickAt(day)
						}
					}
				}).n.setRole(a11y.RoleRow)
			}
		}, layout.PadVH(8, w.padH()))
		// Weekday headers and six weeks form a grid named by the month.
		grid.n.setRole(a11y.RoleGrid).setName(dtMonthTitle(loc, month))
		if w.ids != nil && w.ids.grid == 0 {
			w.ids.grid = grid.n.id
		}
	}, layout.W(pw))
}

// hoverShown is the day / period hovered in the last frame.
func (st *fillDPState) hoverShown() time.Time { return st.lastHover }

// cols returns the column count of a coarse grid.
func (w *DatePickerPanelW) cols(level PickerType) int {
	el := w.c.Theme.Flavor != theme.Ant
	switch level {
	case PickerQuarter:
		return 4
	case PickerYear, PickerMonth:
		if el {
			return 4
		}
	}
	return 3
}

// gridPanel builds a month / quarter / year grid at anchor.
func (w *DatePickerPanelW) gridPanel(anchor time.Time, prev, next bool, hover *time.Time) {
	c, st := w.c, w.st
	th, _ := dtTheme(c)
	loc := c.Locale()
	level := st.mode
	pw := dtPanelWidth(th)
	_, ch := dtCellSize(th)
	bodyH := 7 * ch
	if th.Flavor == theme.Element {
		bodyH += 4
	}
	// Cells: start dates and labels.
	type cell struct {
		start time.Time
		label string
		out   bool // outside the decade (Ant year grid)
	}
	var cells []cell
	switch level {
	case PickerMonth:
		for m := 1; m <= 12; m++ {
			label := loc.Months[m-1]
			if label == "" {
				label = loc.Month(m)
			}
			cells = append(cells, cell{start: time.Date(anchor.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC), label: label})
		}
	case PickerQuarter:
		for q := 1; q <= 4; q++ {
			cells = append(cells, cell{start: time.Date(anchor.Year(), time.Month(3*q-2), 1, 0, 0, 0, 0, time.UTC), label: locFmt(loc.DatePicker.QuarterCellFmt, q)})
		}
	default:
		y0 := anchor.Year()
		lo, hi := y0, y0+9
		if th.Flavor == theme.Ant {
			lo, hi = y0-1, y0+10
		}
		for y := lo; y <= hi; y++ {
			cells = append(cells, cell{start: time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), label: fmt.Sprint(y), out: y < y0 || y > y0+9})
		}
	}
	cols := w.cols(level)
	rows := (len(cells) + cols - 1) / cols
	if level == PickerQuarter {
		rows = 1
	}
	padH := w.padH()
	cw := (pw - 2*padH) / float32(cols)
	rh := bodyH / float32(max(rows, 3))
	if level == PickerQuarter {
		rh = 56
	}
	base := level == fillViewLevel(w.picker)
	a, b := w.selected()
	var pa, pb time.Time
	hv := st.hoverShown()
	if base && w.ranged && !st.rangeStart.IsZero() && !hv.IsZero() {
		pa, pb = fillDPOrder(st.rangeStart, hv)
	}
	if !a.IsZero() && !b.IsZero() {
		a, b = fillDPOrder(a, b)
	}
	todayStart := fillPeriodStart(c.now, level, w.ws())
	gridName := loc.Year(anchor.Year())
	if level == PickerYear {
		gridName = fmt.Sprintf("%d-%d", anchor.Year(), anchor.Year()+9)
	}
	Column(c, func() {
		w.header(anchor, prev, next, w.titles(anchor))
		grid := Column(c, func() {
			for r := 0; r < rows; r++ {
				Row(c, func() {
					for k := 0; k < cols; k++ {
						i := r*cols + k
						if i >= len(cells) {
							bl := c.add("dt-blank", "")
							bl.ln.Width, bl.ln.Height = cw, rh
							continue
						}
						ce := cells[i]
						look := fillCellLook{
							label: ce.label, out: ce.out,
							current: ce.start.Equal(todayStart),
							cursor:  ce.start.Equal(st.cursor),
							first:   k == 0, last: k == cols-1 || i == len(cells)-1,
						}
						if base && w.multi != nil {
							look.selected = dtInSet(ce.start, w.multi)
						} else if base {
							look.selected = (!a.IsZero() && ce.start.Equal(a)) || (!b.IsZero() && ce.start.Equal(b))
							if !a.IsZero() && !b.IsZero() {
								look.inRange = !ce.start.Before(a) && !ce.start.After(b) && !a.Equal(b)
								look.rangeLo, look.rangeHi = ce.start.Equal(a), ce.start.Equal(b)
							} else if !pa.IsZero() {
								look.inRange = !ce.start.Before(pa) && !ce.start.After(pb) && !pa.Equal(pb)
								look.rangeLo, look.rangeHi = ce.start.Equal(pa), ce.start.Equal(pb)
							}
						} else {
							// Navigating: mark the period containing the value.
							if !w.val.IsZero() {
								look.selected = ce.start.Equal(fillPeriodStart(*w.val, level, w.ws()))
							}
						}
						look.disabled = base && w.disabled != nil && w.disabled(ce.start)
						if w.fillCell(look, ce.start, cw, rh, hover) && !w.off {
							w.pickAt(ce.start)
						}
					}
				}).n.setRole(a11y.RoleRow)
			}
		}, layout.PadVH(8, padH))
		grid.n.setRole(a11y.RoleGrid).setName(gridName)
		if w.ids != nil && w.ids.grid == 0 {
			w.ids.grid = grid.n.id
		}
	}, layout.W(pw))
}

// fillCellLook is the state of a month / quarter / year cell.
type fillCellLook struct {
	label                string
	selected, inRange    bool
	rangeLo, rangeHi     bool
	current, cursor, out bool
	disabled             bool
	first, last          bool // first / last column (range strip ends)
}

// fillCell adds a coarse grid cell and reports a click.
func (w *DatePickerPanelW) fillCell(look fillCellLook, start time.Time, cw, ch float32, hover *time.Time) bool {
	c := w.c
	th, tk := dtTheme(c)
	n := c.add("fill-dp-cell", look.label)
	n.ln.Width, n.ln.Height = cw, ch
	if look.disabled {
		n.disabled = true
	}
	ia := c.interact(n, false, input.CursorPointer)
	if look.disabled {
		n.cursor = input.CursorNotAllowed
	}
	if ia.Hovered && hover != nil {
		*hover = start
	}
	// Grid cells of the month / quarter / decade grid; the focused panel
	// (or the DatePicker trigger) moves the cursor with the arrow keys.
	fillSem(c, n, a11y.RoleCell, look.label, a11y.Selectable|fillIf(look.selected, a11y.Selected)|
		fillIf(look.disabled, a11y.Disabled)|fillIf(look.current, a11y.Current))
	fillKeyboard(c, n)
	if w.ids != nil && (look.cursor || (w.st.cursor.IsZero() && look.selected)) {
		w.ids.active = n.id
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		iw, ih := tk.MonthCellWidth, tk.MonthCellHeight
		rad := dtRadius(tk.CellRadius, ih)
		iw = min(iw, r.W-4)
		in := geom.Rect{X: r.X + (r.W-iw)/2, Y: r.Y + (r.H-ih)/2, W: iw, H: ih}
		if look.inRange {
			s := geom.Rect{X: r.X, Y: in.Y, W: r.W, H: ih}
			if look.rangeLo {
				s.X, s.W = in.X+iw/2, r.Right()-in.X-iw/2
			}
			if look.rangeHi {
				s.W = in.X + iw/2 - s.X
			}
			if c.IsRTL() { // the grid runs right to left
				s = mirrorRect(s, r)
			}
			l.Fill(s, geom.Radii{}, tk.CellRangeBg)
		}
		col, wt := tk.CellColor, text.Regular
		if look.out {
			col = tk.CellOutColor
		}
		switch {
		case look.disabled:
			if tk.CellDisabledBg.A > 0 {
				l.Fill(geom.Rect{X: r.X, Y: in.Y, W: r.W, H: ih}, geom.Radii{}, tk.CellDisabledBg)
			}
			col = tk.CellDisabledColor
		case look.selected:
			l.Fill(in, rad, tk.CellActiveBg)
			col = tk.CellActiveColor
			wt = dtWeight(tk.CellActiveFontWeight)
		case ia.Hovered || look.cursor:
			col = dtHover(l, in, rad, tk, col)
		}
		if look.current && !look.selected && !look.disabled {
			col, wt = dtToday(l, in, rad, tk, col, wt, ia.Hovered || look.cursor)
		}
		lay := text.Shape(look.label, c.style(th.FontSize, wt), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(in.X+(in.W-lay.Width)/2, in.Y+(in.H-lay.Height)/2), lay.Glyphs, col)
	}
	return ia.Clicked && !look.disabled
}
