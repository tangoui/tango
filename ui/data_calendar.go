package ui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Calendar (Ant Calendar / el-calendar)
//
// The page-level calendar with month and year views and custom cell
// content. The pickers' dropdown panel is dtMonthPanel (date_util.go).

// CalendarW is a calendar.
type CalendarW struct {
	_ [0]func()

	c          *Context
	b          *Box
	z          *lazy
	val        *time.Time
	mode       *string
	full       bool
	cellRender func(d time.Time)
	monthCell  func(m time.Time)
	disabled   func(d time.Time) bool
	header     bool
	changed    bool
	panelCh    bool
	cellH      float32
	th         *theme.Theme // Calendar Alias applied
	tk         *theme.CalendarToken
}

type calViewState struct {
	view  time.Time // first day of the shown month
	mode  string
	init  bool
	yearS string
	monS  string
	// Keyboard: the active (roving) date of the month grid and whether
	// focus must move to it once its cell is built.
	kbd     time.Time
	pending bool
	grid    time.Time // month of the grid being built
}

// Calendar adds a calendar bound to *val (zero means today, nothing
// selected). Modifiers apply until the end of the build; the content is
// built then (or at Changed/PanelChanged).
//
//	ui.Calendar(c, &day).CellRender(func(d time.Time) { ... })
//	ui.Calendar(c, &day).Card().DisabledDate(isWeekend)
func Calendar(c *Context, val *time.Time) *CalendarW {
	// ConfigProvider.Defaults("Calendar", ...) keys: "fullscreen" (bool;
	// false is the Card form).
	w := &CalendarW{c: c, val: val, full: Prop(c, "Calendar", "fullscreen", true), header: true}
	w.th = c.ThemeFor(theme.CompCalendar)
	w.tk = &w.th.Components.Calendar
	w.b = c.box("calendar", layout.Column, nil)
	w.z = dataLazy(c, w.b.n, w.build)
	return w
}

// Card uses Ant's compact card mode (fullscreen=false).
func (w *CalendarW) Card() *CalendarW { w.full = false; return w }

// Mode binds the view mode ("month" or "year").
func (w *CalendarW) Mode(m *string) *CalendarW { w.mode = m; return w }

// CellRender adds content below the date in month view.
func (w *CalendarW) CellRender(fn func(d time.Time)) *CalendarW { w.cellRender = fn; return w }

// MonthCellRender adds content below the month in year view.
func (w *CalendarW) MonthCellRender(fn func(m time.Time)) *CalendarW {
	w.monthCell = fn
	return w
}

// DisabledDate greys out and blocks dates.
func (w *CalendarW) DisabledDate(fn func(d time.Time) bool) *CalendarW {
	w.disabled = fn
	return w
}

// CellHeight overrides the fullscreen cell height.
func (w *CalendarW) CellHeight(px float32) *CalendarW { w.cellH = px; return w }

// NoHeader hides the header.
func (w *CalendarW) NoHeader() *CalendarW { w.header = false; return w }

// Layout applies layout options to the calendar.
func (w *CalendarW) Layout(o ...layout.Option) *CalendarW { w.b.Style(o...); return w }

// Changed reports a date pick.
func (w *CalendarW) Changed() bool { w.z.run(); return w.changed }

// PanelChanged reports a month/year/mode navigation.
func (w *CalendarW) PanelChanged() bool { w.z.run(); return w.panelCh }

func (w *CalendarW) build() {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	outer := w.b
	st := State[calViewState](c, outer.n.id, nil)
	if !st.init {
		base := *w.val
		if base.IsZero() {
			base = c.now
		}
		st.view, st.mode, st.init = dtMonthUTC(base), "month", true
	}
	mode := &st.mode
	if w.mode != nil {
		if *w.mode == "" {
			*w.mode = "month"
		}
		mode = w.mode
	}
	nav := func(t time.Time) {
		st.view = dtMonthUTC(t)
		w.panelCh = true
		c.invalid = true
	}
	pick := func(d time.Time) {
		if w.disabled != nil && w.disabled(d) {
			return
		}
		*w.val = d
		w.changed = true
		if d.Month() != st.view.Month() || d.Year() != st.view.Year() {
			st.view = dtMonthUTC(d)
			w.panelCh = true
		}
		c.invalid = true
	}
	if !w.full {
		outer.Border(1, tk.MiniBorderColor).Radius(tk.MiniRadius)
	}
	outer.Bg(tk.FullBg)
	if w.header {
		if el {
			w.elHeader(st, nav, pick)
		} else {
			w.antHeader(st, mode, nav)
		}
	}
	if *mode == "year" {
		w.yearGrid(st, mode, nav)
	} else {
		w.monthGrid(st, pick)
	}
}

func (w *CalendarW) antHeader(st *calViewState, mode *string, nav func(time.Time)) {
	c, tk := w.c, w.tk
	pad := layout.PadVH(12, 0)
	if !w.full {
		pad = layout.PadVH(12, 8)
	}
	size := Middle
	if !w.full {
		size = Small
	}
	loc := c.Locale()
	b := Row(c, func() {
		var years []Option
		y0 := st.view.Year()
		for y := y0 - 10; y <= y0+10; y++ {
			s := strconv.Itoa(y)
			years = append(years, Option{Label: loc.Year(y), Value: s})
		}
		st.yearS = strconv.Itoa(y0)
		if Select(c, &st.yearS, years).A11yLabel(loc.Calendar.Year).Size(size).Width(map[bool]float32{true: tk.YearControlWidth, false: tk.YearControlWidthSM}[w.full]).Changed() {
			if y, err := strconv.Atoi(st.yearS); err == nil {
				nav(time.Date(y, st.view.Month(), 1, 0, 0, 0, 0, time.UTC))
			}
		}
		if *mode == "month" {
			var months []Option
			for m := 1; m <= 12; m++ {
				months = append(months, Option{Label: loc.Month(m), Value: strconv.Itoa(m)})
			}
			st.monS = strconv.Itoa(int(st.view.Month()))
			if Select(c, &st.monS, months).A11yLabel(loc.Calendar.Month).Size(size).Width(map[bool]float32{true: tk.MonthControlWidth, false: tk.MonthControlWidthSM}[w.full]).Changed() {
				if m, err := strconv.Atoi(st.monS); err == nil {
					nav(time.Date(st.view.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC))
				}
			}
		}
		mv := *mode
		g := RadioGroup(c, &mv, []Option{{Label: loc.Calendar.Month, Value: "month"}, {Label: loc.Calendar.Year, Value: "year"}}).Buttons().Size(size)
		if g.Changed() {
			*mode = mv
			w.panelCh = true
			c.invalid = true
		}
	}, pad, layout.Gap(8), layout.End(), layout.Middle())
	if !w.full {
		b.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Bottom()-1, r.W, tk.HeaderBorderColor) })
	}
}

func (w *CalendarW) elHeader(st *calViewState, nav func(time.Time), pick func(time.Time)) {
	c, th, tk := w.c, w.th, w.tk
	loc := c.Locale()
	Row(c, func() {
		Text(c, dtMonthTitle(loc, st.view)).Color(th.Text).Layout(layout.Grow(1))
		Row(c, func() {
			if Button(c, loc.PrevMonth).Small().Clicked() {
				nav(st.view.AddDate(0, -1, 0))
			}
			if Button(c, loc.Today).Small().Clicked() {
				nav(c.now)
				pick(dtDayUTC(c.now))
			}
			if Button(c, loc.NextMonth).Small().Clicked() {
				nav(st.view.AddDate(0, 1, 0))
			}
		}, layout.Middle())
	}, layout.PadVH(12, 20), layout.Middle()).Paint(func(l *paint.List, r geom.Rect) {
		dataHLine(l, r.X, r.Bottom()-1, r.W, tk.HeaderBorderColor)
	})
}

func (w *CalendarW) monthGrid(st *calViewState, pick func(time.Time)) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element && w.full // Element grid style
	ws := dtWeekStart(c)
	names := dtWeekNames(c.Locale(), ws)
	start := st.view
	gridStart := dtGridStart(start, ws)
	off := (int(start.Weekday()) - int(ws) + 7) % 7
	cellH := w.cellH
	if cellH == 0 {
		if w.full {
			cellH = tk.FullCellHeight
		} else {
			cellH = tk.MiniCellHeight
		}
	}
	rows := 6
	if el {
		// Element shows only the weeks that contain days of the month.
		last := start.AddDate(0, 1, -1)
		rows = (off + last.Day() + 6) / 7
	}
	pad := layout.PadVH(0, 0)
	if !w.full && !el {
		pad = layout.PadVH(8, 8)
	}
	if el {
		pad = layout.PadVH(12, 20)
	}
	// The roving Tab stop: the date last moved to with the keyboard, else
	// the selected date, today or the 1st of the shown month.
	st.grid = start
	inView := func(d time.Time) bool { return d.Year() == start.Year() && d.Month() == start.Month() }
	active := start
	switch {
	case inView(st.kbd):
		active = st.kbd
	case inView(*w.val):
		active = *w.val
	case inView(c.now):
		active = c.now
	}
	grid := Column(c, func() {
		// Weekday header.
		Row(c, func() {
			for _, nm := range names {
				t := Text(c, nm).Layout(layout.Grow(1), layout.Basis(0))
				t.n.setRole(a11y.RoleColumnHeader)
				switch {
				case el:
					t.Color(th.Text).Align(AlignStart).Layout(layout.PadVH(12, 0))
				case w.full:
					t.Align(AlignEnd).Layout(layout.Padding(geom.Insets{Right: 12, Bottom: 5}))
				case th.Flavor == theme.StyleShadcn:
					// text-muted-foreground text-[0.8rem] font-normal
					t.Size(13).Color(th.TextTertiary).Align(AlignCenter).Layout(layout.PadVH(5, 0))
				default:
					t.Align(AlignCenter).Layout(layout.PadVH(5, 0))
				}
			}
		}).n.setRole(a11y.RoleRow)
		for r := 0; r < rows; r++ {
			Row(c, func() {
				for col := 0; col < 7; col++ {
					day := gridStart.AddDate(0, 0, r*7+col)
					Key(c, day.Format("20060102"), func() {
						w.dayCell(day, st, cellH, pick, r, col, rows, dtSameDay(day, active))
					})
				}
			}).n.setRole(a11y.RoleRow)
		}
	}, pad)
	grid.n.setRole(a11y.RoleGrid).setName(dtMonthTitle(c.Locale(), start))
}

// calDayKeys moves the keyboard date of the month grid (WAI-ARIA date
// grid: arrows by day / week, Home / End to the week's ends, Page Up /
// Down by month); it reports the new date.
func calDayKeys(c *Context, id ID, day time.Time, ws time.Weekday) (time.Time, bool) {
	to, moved := day, false
	for _, k := range c.KeysFor(id) {
		if !k.Down {
			continue
		}
		wd := (int(to.Weekday()) - int(ws) + 7) % 7
		// Left / Right move visually (the week runs right to left in RTL).
		prev, next := c.inlineKey(k.Key)
		switch {
		case prev:
			to = to.AddDate(0, 0, -1)
			moved = true
			continue
		case next:
			to = to.AddDate(0, 0, 1)
			moved = true
			continue
		}
		switch k.Key {
		case input.KeyUp:
			to = to.AddDate(0, 0, -7)
		case input.KeyDown:
			to = to.AddDate(0, 0, 7)
		case input.KeyHome:
			to = to.AddDate(0, 0, -wd)
		case input.KeyEnd:
			to = to.AddDate(0, 0, 6-wd)
		case input.KeyPageUp:
			to = to.AddDate(0, -1, 0)
		case input.KeyPageDown:
			to = to.AddDate(0, 1, 0)
		default:
			continue
		}
		moved = true
	}
	return to, moved
}

func (w *CalendarW) dayCell(day time.Time, st *calViewState, cellH float32, pick func(time.Time), row, col, rows int, tabStop bool) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element && w.full // Element grid style
	inMonth := day.Month() == st.view.Month()
	sel := dtSameDay(*w.val, day)
	isToday := dtSameDay(day, c.now)
	dis := w.disabled != nil && w.disabled(day)
	label := strconv.Itoa(day.Day())
	if w.full && !el {
		label = fmt.Sprintf("%02d", day.Day())
	}
	opts := []layout.Option{layout.Grow(1), layout.Basis(0), layout.H(cellH)}
	switch {
	case el:
		opts = append(opts, layout.Pad(8))
	case w.full:
		opts = append(opts, layout.Margin(geom.Insets{Left: 4, Right: 4}), layout.Padding(geom.Insets{Top: 4, Left: 8, Right: 8}))
	default:
		opts = append(opts, layout.Center())
	}
	cell := c.box("cal-cell", layout.Column, opts)
	cell.n.label = day.Format("2006-01-02")
	cur := input.CursorPointer
	if dis {
		cur = input.CursorNotAllowed
	}
	ia := cell.Focusable(cur)
	loc := c.Locale()
	cell.n.setRole(a11y.RoleCell).setName(loc.Date(day.Year(), int(day.Month()), day.Day())).
		setState(a11y.Selectable).stateIf(sel, a11y.Selected).stateIf(isToday, a11y.Current)
	cell.n.aKbd = true // arrow keys reach every date
	if !tabStop {
		cell.n.noTab = true
	}
	if st.pending && dtSameDay(day, st.kbd) && st.view.Equal(st.grid) {
		st.pending = false
		navFocus(c, cell.n.id)
	}
	if ia.Clicked && !dis {
		pick(day)
		st.kbd = day
	}
	if ia.Focused {
		if to, ok := calDayKeys(c, cell.n.id, day, dtWeekStart(c)); ok {
			st.kbd, st.pending = to, true
			if to.Month() != st.view.Month() || to.Year() != st.view.Year() {
				st.view = dtMonthUTC(to)
				w.panelCh = true
			}
			c.invalid = true
		}
	}
	if ia.FocusVisible {
		cell.n.over = func(l *paint.List, r geom.Rect) {
			if !w.full {
				s := shad2bCalDay(th)
				r = geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}
			} else {
				g := th.FocusRingOffset + th.FocusRingWidth
				r = r.Inset(geom.All(g))
			}
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
	}
	hov := c.Anim(cell.n.id, "hover", b2f(ia.Hovered && !dis), th.DurationMid)
	fs := c.style(th.FontSize, text.Regular)
	cell.n.paint = func(l *paint.List, r geom.Rect) {
		rtl := c.IsRTL()
		fg := tk.ItemColor
		if !inMonth {
			fg = tk.ItemOutColor
		}
		if dis {
			fg = th.TextDisabled
		}
		switch {
		case el:
			bg := theme.Color{}
			if sel {
				bg = tk.ItemActiveBg
			} else if hov > 0 {
				bg = theme.Fade(tk.ItemHoverBg, float64(hov))
			}
			if dis {
				bg = tk.ItemDisabledBg
			}
			l.Fill(r, geom.Radii{}, bg)
			// Grid lines: every cell draws its inline-end and bottom edge,
			// the first row/column also the top/inline-start one (the
			// week runs right to left in RTL).
			bc := tk.ItemBorderColor
			endX, startX := r.Right()-1, r.X
			if rtl {
				endX, startX = r.X, r.Right()-1
			}
			dataVLine(l, endX, r.Y, r.H, bc)
			dataHLine(l, r.X, r.Bottom()-1, r.W, bc)
			if row == 0 {
				dataHLine(l, r.X, r.Y, r.W, bc)
			}
			if col == 0 {
				dataVLine(l, startX, r.Y, r.H, bc)
			}
			if isToday || sel {
				fg = tk.ItemActiveColor
			}
			if !inMonth {
				fg = tk.ItemOutColor
			}
			lay := text.Shape(label, fs, text.Options{NoWrap: true})
			x := r.X + 8
			if rtl {
				x = r.Right() - 8 - lay.Width
			}
			l.Glyphs(geom.Pt(x, r.Y+8), lay.Glyphs, fg)
		case w.full:
			bg := theme.Color{}
			if sel {
				bg = tk.ItemActiveBg
			} else if hov > 0 {
				bg = theme.Fade(tk.ItemHoverBg, float64(hov))
			}
			if dis {
				bg = tk.ItemDisabledBg
			}
			l.Fill(r, geom.Radii{}, bg)
			top := tk.ItemBorderColor
			if isToday || sel {
				top = tk.ItemActiveColor
			}
			l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, geom.Radii{}, top)
			if sel || isToday {
				fg = tk.ItemActiveColor
			}
			if !inMonth {
				fg = tk.ItemOutColor
			}
			lay := text.Shape(label, fs, text.Options{NoWrap: true})
			x := r.Right() - 8 - lay.Width // date at the inline end
			if rtl {
				x = r.X + 8
			}
			l.Glyphs(geom.Pt(x, r.Y+4), lay.Glyphs, fg)
		default:
			s := shad2bCalDay(th)
			in := geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}
			rad := geom.Rad(tk.DayRadius)
			switch {
			case sel:
				l.Fill(in, rad, tk.DayActiveBg)
				fg = tk.DayActiveColor
			case isToday && tk.DayTodayBg.A > 0:
				l.Fill(in, rad, tk.DayTodayBg)
			case hov > 0:
				l.Fill(in, rad, theme.Fade(tk.DayHoverBg, float64(hov)))
			}
			if isToday && !sel && tk.DayTodayBg.A == 0 {
				l.Stroke(in, rad, 1, tk.DayTodayBorder)
			}
			if !inMonth && !sel && tk.DayOutColor.A > 0 {
				fg = tk.DayOutColor
			}
			lay := text.Shape(strconv.Itoa(day.Day()), fs, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(in.X+(in.W-lay.Width)/2, in.Y+(in.H-lay.Height)/2), lay.Glyphs, fg)
		}
	}
	if w.cellRender != nil && (w.full || el) {
		cell.Clip()
		cell.Body(func() {
			top := float32(24)
			if el {
				top = 26
			}
			Column(c, func() { w.cellRender(day) }, layout.Margin(geom.Insets{Top: top}), layout.Gap(4), layout.Grow(1))
		})
	}
	_ = rows
}

func (w *CalendarW) yearGrid(st *calViewState, mode *string, nav func(time.Time)) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	cellH := float32(116)
	if !w.full {
		cellH = 48
	}
	pad := layout.PadVH(0, 0)
	if !w.full {
		pad = layout.PadVH(8, 8)
	}
	thisMonth := dtMonthUTC(c.now)
	loc := c.Locale()
	var cells [12]ID
	grid := Column(c, func() {
		for r := 0; r < 4; r++ {
			Row(c, func() {
				for col := 0; col < 3; col++ {
					m := time.Date(st.view.Year(), time.Month(r*3+col+1), 1, 0, 0, 0, 0, time.UTC)
					Key(c, m.Format("200601"), func() {
						opts := []layout.Option{layout.Grow(1), layout.Basis(0), layout.H(cellH)}
						if w.full {
							opts = append(opts, layout.Margin(geom.Insets{Left: 4, Right: 4}), layout.Padding(geom.Insets{Top: 4, Left: 8, Right: 8}))
						} else {
							opts = append(opts, layout.Center())
						}
						cell := c.box("cal-month", layout.Column, opts)
						cell.n.label = m.Format("2006-01")
						ia := cell.Focusable(input.CursorPointer)
						cells[r*3+col] = cell.n.id
						if ia.Clicked {
							nav(m)
							*mode = "month"
						}
						sel := m.Year() == st.view.Year() && m.Month() == st.view.Month()
						cur := m.Equal(thisMonth)
						cell.n.setRole(a11y.RoleCell).setName(loc.YearMonth(m.Year(), int(m.Month()))).
							setState(a11y.Selectable).stateIf(sel, a11y.Selected).stateIf(cur, a11y.Current)
						cell.n.aKbd = true
						if !sel {
							cell.n.noTab = true
						}
						if ia.FocusVisible {
							cell.n.over = func(l *paint.List, rr geom.Rect) {
								g := th.FocusRingOffset + th.FocusRingWidth
								drawFocusRing(l, th, rr.Inset(geom.All(g)), geom.Rad(th.RadiusSM))
							}
						}
						hov := c.Anim(cell.n.id, "hover", b2f(ia.Hovered), th.DurationMid)
						label := loc.Month(int(m.Month()))
						fs := c.style(th.FontSize, text.Regular)
						cell.n.paint = func(l *paint.List, rr geom.Rect) {
							col := tk.ItemColor
							if w.full {
								bg := theme.Color{}
								if sel {
									bg = tk.ItemActiveBg
								} else if hov > 0 {
									bg = theme.Fade(th.ItemHoverBg, float64(hov))
								}
								l.Fill(rr, geom.Radii{}, bg)
								top := th.Split
								if sel || cur {
									top, col = tk.ItemActiveColor, tk.ItemActiveColor
								}
								l.Fill(geom.Rect{X: rr.X, Y: rr.Y, W: rr.W, H: 2}, geom.Radii{}, top)
								lay := text.Shape(label, fs, text.Options{NoWrap: true})
								x := rr.Right() - 8 - lay.Width // month at the inline end
								if c.IsRTL() {
									x = rr.X + 8
								}
								l.Glyphs(geom.Pt(x, rr.Y+4), lay.Glyphs, col)
								return
							}
							lay := text.Shape(label, fs, text.Options{NoWrap: true})
							in := geom.Rect{X: rr.X + (rr.W-lay.Width-16)/2, Y: rr.Y + (rr.H-24)/2, W: lay.Width + 16, H: 24}
							switch {
							case sel:
								l.Fill(in, geom.Rad(th.RadiusSM), tk.DayActiveBg)
								col = tk.DayActiveColor
							case hov > 0:
								l.Fill(in, geom.Rad(th.RadiusSM), theme.Fade(tk.DayHoverBg, float64(hov)))
							}
							if cur && !sel {
								l.Stroke(in, geom.Rad(th.RadiusSM), 1, th.Primary.Base)
							}
							l.Glyphs(geom.Pt(in.X+8, in.Y+(24-lay.Height)/2), lay.Glyphs, col)
						}
						if w.monthCell != nil && w.full {
							cell.Clip().Body(func() {
								Column(c, func() { w.monthCell(m) }, layout.Margin(geom.Insets{Top: 24}), layout.Gap(4))
							})
						}
					})
				}
			}).n.setRole(a11y.RoleRow)
		}
	}, pad)
	grid.n.setRole(a11y.RoleGrid).setName(loc.Year(st.view.Year()))
	// Arrows move by month (Left / Right) or row (Up / Down) inside the year.
	for i, id := range cells {
		if id == 0 || c.focus != id {
			continue
		}
		to := i
		for _, k := range navKeys(c, id) {
			// Left / Right move visually (rows run right to left in RTL).
			if prev, next := c.inlineKey(k); prev {
				to--
			} else if next {
				to++
			}
			switch k {
			case input.KeyUp:
				to -= 3
			case input.KeyDown:
				to += 3
			case input.KeyHome:
				to = 0
			case input.KeyEnd:
				to = 11
			}
			to = min(11, max(0, to))
		}
		if to != i {
			navFocus(c, cells[to])
		}
	}
	_ = el
}
