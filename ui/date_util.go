package ui

// Date utilities shared by the date components: calendar arithmetic,
// localized week/month names (Locale), parsing and formatting, and the month panel (single
// date, range with hover preview, disabled dates, keyboard cursor) used by
// DatePicker, DateTimePicker and RangePicker. Calendar (data_calendar.go)
// uses the same arithmetic. Unexported names use the "dt" prefix.

import (
	"fmt"
	"strconv"
	"strings"
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

// ---------------------------------------------------------------------------
// Date arithmetic

// dtDay truncates t to midnight in its location.
func dtDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// dtMonthStart returns the first day of t's month.
func dtMonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// dtAddMonths moves a month start by n months.
func dtAddMonths(month time.Time, n int) time.Time {
	return time.Date(month.Year(), month.Month()+time.Month(n), 1, 0, 0, 0, 0, month.Location())
}

// dtSameDay compares calendar dates (non-zero values only).
func dtSameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// dtBetween reports a <= d <= b by calendar date (a, b in any order).
func dtBetween(d, a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	d, a, b = dtDay(d), dtDay(a), dtDay(b)
	if b.Before(a) {
		a, b = b, a
	}
	return !d.Before(a) && !d.After(b)
}

// dtInSet reports whether the calendar date of d is one of days.
func dtInSet(d time.Time, days []time.Time) bool {
	for _, x := range days {
		if dtSameDay(d, x) {
			return true
		}
	}
	return false
}

// dtWeekStart is the first weekday of the grid (Locale.WeekStart: Sunday
// for every built-in pack; an app can register a Monday-first locale).
func dtWeekStart(c *Context) time.Weekday { return c.Locale().WeekStart }

// dtGridStart returns the first cell of a 6x7 month grid.
func dtGridStart(month time.Time, ws time.Weekday) time.Time {
	off := (int(month.Weekday()) - int(ws) + 7) % 7
	return month.AddDate(0, 0, -off)
}

// dtWeekNames returns the short weekday headers of loc starting at ws.
func dtWeekNames(loc *Locale, ws time.Weekday) []string {
	out := make([]string, 7)
	for i := range out {
		out[i] = loc.Weekdays[(int(ws)+i)%7]
	}
	return out
}

// dtMonthTitle formats the panel header of a month.
func dtMonthTitle(loc *Locale, month time.Time) string {
	return loc.YearMonth(month.Year(), int(month.Month()))
}

// dtWithClock combines the date of d with the clock of t.
func dtWithClock(d, t time.Time) time.Time {
	y, m, dd := d.Date()
	loc := d.Location()
	if !t.IsZero() {
		loc = t.Location()
		return time.Date(y, m, dd, t.Hour(), t.Minute(), t.Second(), 0, loc)
	}
	return time.Date(y, m, dd, 0, 0, 0, 0, loc)
}

// dtFormat formats t, or returns "" for the zero time.
func dtFormat(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(layout)
}

// dtParse parses s with layout in loc; surrounding spaces are ignored.
func dtParse(s, layout string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation(layout, strings.TrimSpace(s), loc)
	return t, err == nil
}

// ---------------------------------------------------------------------------
// Month panel

// dtMonth configures one month grid.
type dtMonth struct {
	view         time.Time // first day of the shown month
	today        time.Time
	selA, selB   time.Time   // selected endpoints (selB zero for single)
	sels         []time.Time // selected days of a multiple picker
	prevA, prevB time.Time   // hover preview range (zero = none)
	cursor       time.Time   // keyboard cursor
	disabled     func(time.Time) bool
	hideOut      bool // leave days of adjacent months blank (range panels)
	navPrev      bool // show « ‹
	navNext      bool // show › »
	onNav        func(months int)
	hover        *time.Time // receives the hovered day
	ids          *dtGridIDs // receives the grid and active cell IDs (semantics)
}

// semExpanded marks an open combobox: Expanded, and Collapse instead of
// Expand (the default actions are derived before semantics fillers run).
func semExpanded(s *a11y.Node) {
	s.States |= a11y.Expanded
	if s.Actions.Has(a11y.Expand) {
		s.Actions = s.Actions&^a11y.Expand | a11y.Collapse
	}
}

// dtGridIDs receives the semantic node IDs of a month panel this frame:
// the first grid (Controls of the trigger) and the cell of the keyboard
// cursor, or of the selected day without one (ActiveDescendant).
type dtGridIDs struct{ grid, active ID }

// dtTheme returns the theme the date components read (DatePicker Alias
// applied) and their tokens.
func dtTheme(c *Context) (*theme.Theme, *theme.DatePickerToken) {
	th := c.ThemeFor(theme.CompDatePicker)
	return th, &th.Components.DatePicker
}

// dtCellSize returns the day cell size (DatePickerToken.CellWidth/Height).
func dtCellSize(th *theme.Theme) (w, h float32) {
	tk := &th.Components.DatePicker
	return tk.CellWidth, tk.CellHeight
}

// dtPanelWidth is the width of one month panel.
func dtPanelWidth(th *theme.Theme) float32 {
	w, _ := dtCellSize(th)
	return 7*w + 2*th.Components.DatePicker.PanelPaddingInline
}

// dtRadius clamps a cell highlight radius to half the highlight height.
func dtRadius(rad, h float32) geom.Radii { return geom.Rad(min(rad, h/2)) }

// dtMonthPanel builds a month grid and returns the clicked day.
func dtMonthPanel(c *Context, m dtMonth) (picked time.Time, ok bool) {
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	cw, ch := dtCellSize(th)
	loc := c.Locale()
	ws := dtWeekStart(c)
	Column(c, func() {
		// Header.
		hdr := Row(c, func() {
			nav := func(ic *icon.Icon, label string, months int, show bool) {
				if shad && (months == 12 || months == -12) {
					return // shadcn Calendar: month arrows only
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
				if b.Clicked() && m.onNav != nil {
					m.onNav(months)
				}
			}
			nav(icon.DoubleLeft, loc.PrevYear, -12, m.navPrev)
			nav(icon.Left, loc.PrevMonth, -1, m.navPrev)
			Spacer(c)
			t := Text(c, dtMonthTitle(loc, m.view)).NoWrap()
			if el {
				t.Size(16).Weight(text.Medium)
			} else if shad {
				t.Weight(text.Medium)
			} else {
				t.Strong()
			}
			Spacer(c)
			nav(icon.Right, loc.NextMonth, 1, m.navNext)
			nav(icon.DoubleRight, loc.NextYear, 12, m.navNext)
		}, layout.H(tk.HeaderHeight), layout.Middle())
		if el {
			hdr.Style(layout.PadVH(0, 12), layout.Margin(geom.Insets{Top: 4}))
		} else if shad {
			hdr.Style(layout.PadVH(0, 12), layout.Gap(4), layout.Margin(geom.Insets{Top: 4}))
		} else {
			hdr.Style(layout.PadVH(0, 8))
		}
		dtHeaderDivider(hdr, tk)
		grid := Column(c, func() {
			wk := Row(c, func() {
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
			first := dtGridStart(m.view, ws)
			for row := 0; row < 6; row++ {
				Row(c, func() {
					for col := 0; col < 7; col++ {
						day := first.AddDate(0, 0, row*7+col)
						inMonth := day.Month() == m.view.Month()
						if m.hideOut && !inMonth {
							b := c.add("dt-blank", "")
							b.ln.Width, b.ln.Height = cw, ch
							continue
						}
						if dtDayCell(c, m, day, inMonth, cw, ch, row*7+col) {
							picked, ok = day, true
						}
					}
				}).n.setRole(a11y.RoleRow)
			}
		}, layout.PadVH(8, tk.PanelPaddingInline))
		// The weekday header and the six weeks form a grid named by the
		// month; the trigger keeps focus and points at the cursor cell.
		grid.n.setRole(a11y.RoleGrid).setName(dtMonthTitle(loc, m.view))
		if m.ids != nil && m.ids.grid == 0 {
			m.ids.grid = grid.n.id
		}
	}, layout.W(dtPanelWidth(th)))
	return picked, ok
}

// dtHeaderDivider draws the divider under a panel header (when the
// token has one).
func dtHeaderDivider(hdr *Box, tk *theme.DatePickerToken) {
	if tk.HeaderBorderColor.A == 0 {
		return
	}
	col := tk.HeaderBorderColor
	hdr.Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, col)
	})
}

// dtWeekLabel adds a weekday column header of a month grid.
func dtWeekLabel(c *Context, th *theme.Theme, tk *theme.DatePickerToken, d string, cw, ch float32) {
	lb := Text(c, d).Align(AlignCenter).Layout(layout.W(cw), layout.H(ch)).LineHeight(ch)
	lb.n.setRole(a11y.RoleColumnHeader)
	if tk.WeekColor.A > 0 {
		lb.Color(tk.WeekColor)
	}
	if tk.WeekFontSize != th.FontSize {
		lb.Size(tk.WeekFontSize)
	}
}

// dtDayCell builds one day cell and reports a click.
func dtDayCell(c *Context, m dtMonth, day time.Time, inMonth bool, cw, ch float32, idx int) bool {
	th, tk := dtTheme(c)
	el := th.Flavor == theme.Element
	cell := c.add("dt-day", day.Format("2006-01-02"))
	cell.ln.Width, cell.ln.Height = cw, ch
	dis := m.disabled != nil && m.disabled(day)
	if dis {
		cell.disabled = true
	}
	cell.setRole(a11y.RoleCell).setState(a11y.Selectable)
	// Keyboard: the picker moves a day cursor with the arrow keys (or the
	// date is typed into the DateTimePicker field).
	cell.aKbd = true
	if c.a11yOn {
		cell.setName(c.Locale().Date(day.Year(), int(day.Month()), day.Day()))
	}
	ia := c.interact(cell, false, input.CursorPointer)
	if dis {
		cell.cursor = input.CursorNotAllowed
	}
	if ia.Hovered && m.hover != nil {
		*m.hover = day
	}
	clicked := ia.Clicked && !dis
	isA, isB := dtSameDay(day, m.selA), dtSameDay(day, m.selB)
	if !isA && dtInSet(day, m.sels) {
		isA = true // a day of a multiple selection looks like a picked end
	}
	inSel := dtBetween(day, m.selA, m.selB)
	inPrev := dtBetween(day, m.prevA, m.prevB)
	isToday := dtSameDay(day, m.today)
	isCur := dtSameDay(day, m.cursor)
	lo, hi := m.selA, m.selB
	if !lo.IsZero() && !hi.IsZero() && hi.Before(lo) {
		lo, hi = hi, lo
	}
	plo, phi := m.prevA, m.prevB
	if !plo.IsZero() && !phi.IsZero() && phi.Before(plo) {
		plo, phi = phi, plo
	}
	cell.stateIf(inMonth && (isA || isB || (inSel && !lo.IsZero() && !hi.IsZero())), a11y.Selected).stateIf(isToday, a11y.Current)
	if m.ids != nil && inMonth && (isCur || (m.cursor.IsZero() && isA)) {
		m.ids.active = cell.id
	}
	weekCol := idx % 7
	label := fmt.Sprint(day.Day())
	if th.Flavor == theme.StyleShadcn {
		selRange := inSel && !dtSameDay(lo, hi)
		prevRange := inPrev && !dtSameDay(plo, phi)
		last := day.AddDate(0, 0, 1).Month() != day.Month()
		look := dayLook{
			inMonth: inMonth, disabled: dis, hovered: ia.Hovered || isCur,
			selected: (isA || isB) && inMonth, today: isToday,
			rangeMid: inMonth && (selRange || prevRange),
			rangeLo:  inMonth && ((selRange && dtSameDay(day, lo)) || (prevRange && dtSameDay(day, plo))),
			rangeHi:  inMonth && ((selRange && dtSameDay(day, hi)) || (prevRange && dtSameDay(day, phi))),
			rowFirst: weekCol == 0 || (m.hideOut && day.Day() == 1),
			rowLast:  weekCol == 6 || (m.hideOut && last),
		}
		cell.paint = func(l *paint.List, r geom.Rect) { dtPaintDayShadcn(c, l, r, label, look) }
		return clicked
	}
	cell.paint = func(l *paint.List, r geom.Rect) {
		is := tk.CellInnerSize
		in := geom.Rect{X: r.X + (r.W-is)/2, Y: r.Y + (r.H-is)/2, W: is, H: is}
		rad := dtRadius(tk.CellRadius, is)
		col := tk.CellColor
		if !inMonth {
			col = tk.CellOutColor
		}
		// Range strip behind the cell (in-month cells only).
		strip := geom.Rect{X: r.X, Y: in.Y, W: r.W, H: is}
		stripCol := tk.CellRangeBg
		if el {
			strip = geom.Rect{X: r.X, Y: r.Y + (r.H-30)/2 + 3, W: r.W, H: 24}
		}
		// The strips are laid out in LTR terms (start half, start edge)
		// and mirrored in RTL, where the week runs right to left.
		rtl := c.IsRTL()
		if inMonth && inSel && !dtSameDay(lo, hi) {
			s := strip
			if dtSameDay(day, lo) {
				s.X, s.W = r.X+r.W/2, r.W/2
			} else if dtSameDay(day, hi) {
				s.W = r.W / 2
			}
			if rtl {
				s = mirrorRect(s, r)
			}
			l.Fill(s, geom.Radii{}, stripCol)
		}
		if inMonth && inPrev && !dtSameDay(plo, phi) {
			s := strip
			if dtSameDay(day, plo) {
				s.X, s.W = in.X, r.Right()-in.X
			} else if dtSameDay(day, phi) {
				s.W = in.Right() - r.X
			}
			if rtl {
				s = mirrorRect(s, r)
			}
			if !inSel && tk.CellRangePreviewBg.A > 0 {
				l.Fill(s, geom.Radii{}, tk.CellRangePreviewBg)
			}
			dash := []float32{3, 3}
			bc := tk.CellRangeHoverBorder
			l.StrokePath(paint.NewPath().MoveTo(s.X, s.Y+0.5).LineTo(s.Right(), s.Y+0.5), paint.StrokeStyle{Width: 1, Dash: dash}, bc)
			l.StrokePath(paint.NewPath().MoveTo(s.X, s.Bottom()-0.5).LineTo(s.Right(), s.Bottom()-0.5), paint.StrokeStyle{Width: 1, Dash: dash}, bc)
			startX, endX := s.X+0.5, s.Right()-0.5
			if rtl {
				startX, endX = endX, startX
			}
			if weekCol == 0 || dtSameDay(day, plo) {
				l.Line(geom.Pt(startX, s.Y), geom.Pt(startX, s.Bottom()), 1, bc)
			}
			if weekCol == 6 || dtSameDay(day, phi) {
				l.Line(geom.Pt(endX, s.Y), geom.Pt(endX, s.Bottom()), 1, bc)
			}
		}
		wt := text.Regular
		switch {
		case dis:
			if tk.CellDisabledBg.A > 0 {
				l.Fill(geom.Rect{X: r.X, Y: in.Y, W: r.W, H: is}, geom.Radii{}, tk.CellDisabledBg)
			}
			col = tk.CellDisabledColor
		case (isA || isB) && inMonth:
			l.Fill(in, rad, tk.CellActiveBg)
			col = tk.CellActiveColor
			wt = dtWeight(tk.CellActiveFontWeight)
		case (ia.Hovered || isCur) && inMonth && !inSel:
			col = dtHover(l, in, rad, tk, col)
		case (ia.Hovered || isCur) && inMonth:
			l.Fill(in, rad, tk.CellHoverWithRangeBg)
		}
		if isToday && !(isA || isB) && !dis {
			col, wt = dtToday(l, in, rad, tk, col, wt, false)
		}
		lay := text.Shape(label, c.style(th.FontSize, wt), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(in.X+(in.W-lay.Width)/2, in.Y+(in.H-lay.Height)/2), lay.Glyphs, col)
	}
	return clicked
}

// dtWeight converts a token font weight (zero: regular).
func dtWeight(w int) text.Weight {
	if w <= 0 {
		return text.Regular
	}
	return text.Weight(w)
}

// dtHover paints a hovered cell highlight and returns its text color:
// CellHoverBg, or with none only the CellHoverColor text.
func dtHover(l *paint.List, in geom.Rect, rad geom.Radii, tk *theme.DatePickerToken, col theme.Color) theme.Color {
	if tk.CellHoverBg.A > 0 {
		l.Fill(in, rad, tk.CellHoverBg)
	} else if tk.CellHoverColor.A > 0 {
		col = tk.CellHoverColor
	}
	return col
}

// dtToday marks the current day / period: outline, fill (skipped while
// hovered, the hover fill shows instead), text color and weight.
func dtToday(l *paint.List, in geom.Rect, rad geom.Radii, tk *theme.DatePickerToken, col theme.Color, wt text.Weight, hovered bool) (theme.Color, text.Weight) {
	if tk.CellTodayBorder.A > 0 {
		l.Stroke(in, rad, 1, tk.CellTodayBorder)
	}
	if tk.CellTodayBg.A > 0 && !hovered {
		l.Fill(in, rad, tk.CellTodayBg)
	}
	if tk.CellTodayColor.A > 0 {
		col = tk.CellTodayColor
	}
	if tk.CellTodayFontWeight > 0 {
		wt = text.Weight(tk.CellTodayFontWeight)
	}
	return col, wt
}

// dtShadcnNav turns a calendar navigation icon button into shadcn's
// 28px outline button.
func dtShadcnNav(c *Context, b *IconButtonW) {
	th := c.Theme
	b.Size(28, 16)
	prev := b.n.paint
	b.n.paint = func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(th.Radius)
		if len(th.ButtonShadow) > 0 {
			l.Shadows(r, rad, th.ButtonShadow)
		}
		l.Fill(r, rad, th.BgContainer)
		l.Stroke(r, rad, 1, th.Border)
		prev(l, r)
	}
}

// dayLook is the state of a shadcn calendar day.
type dayLook struct {
	inMonth, disabled, hovered, selected, today, rangeMid bool
	// rangeLo / rangeHi mark the ends of a multi-day range (selected or
	// previewed); rowFirst / rowLast the cells where a band row starts or
	// ends (rounded like shadcn's first/last selected day of a week).
	rangeLo, rangeHi, rowFirst, rowLast bool
}

// dtPaintDayShadcn draws a day the way shadcn's Calendar does: rounded-md
// buttons, primary fill when selected, accent for today, hover and the
// middle of a range, muted half-opaque days outside the month.
func dtPaintDayShadcn(c *Context, l *paint.List, r geom.Rect, label string, d dayLook) {
	th, tk := dtTheme(c)
	rad := geom.Rad(tk.CellRadius)
	col := tk.CellColor
	if d.rangeMid {
		// The accent band of a range: square in the middle, rounded at
		// the range ends and where a week row starts or ends.
		var br geom.Radii
		if d.rangeLo || d.rowFirst {
			br.TL, br.BL = tk.CellRadius, tk.CellRadius
		}
		if d.rangeHi || d.rowLast {
			br.TR, br.BR = tk.CellRadius, tk.CellRadius
		}
		if c.IsRTL() { // the band runs right to left
			br.TL, br.TR, br.BL, br.BR = br.TR, br.TL, br.BR, br.BL
		}
		l.Fill(r, br, tk.CellRangeBg)
	}
	switch {
	case d.selected:
		l.Fill(r, rad, tk.CellActiveBg)
		col = tk.CellActiveColor
	case d.rangeMid:
		if tk.CellRangeColor.A > 0 {
			col = tk.CellRangeColor
		}
	case d.disabled:
		col = tk.CellDisabledColor
	case d.hovered && d.inMonth:
		l.Fill(r, rad, tk.CellHoverBg)
		if tk.CellHoverColor.A > 0 {
			col = tk.CellHoverColor
		}
	case d.today && d.inMonth:
		if tk.CellTodayBg.A > 0 {
			l.Fill(r, rad, tk.CellTodayBg)
		}
		if tk.CellTodayColor.A > 0 {
			col = tk.CellTodayColor
		}
	case !d.inMonth:
		col = tk.CellOutColor
	}
	wt := text.Regular
	if d.today || d.selected {
		wt = text.Medium
	}
	lay := text.Shape(label, c.style(th.FontSize, wt), text.Options{NoWrap: true})
	l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
}

// dtMirror maps the x of an element of width w, laid out in LTR terms
// inside r, to its physical x: unchanged in LTR, reflected in r in RTL.
// The pickers compute their fields' icons and text in LTR terms (keeping
// LTR arithmetic exact) and place them through it.
func dtMirror(rtl bool, r geom.Rect, x, w float32) float32 {
	if rtl {
		return 2*r.X + r.W - x - w
	}
	return x
}

// dtMirrorRect is dtMirror for a rectangle.
func dtMirrorRect(rtl bool, rr, r geom.Rect) geom.Rect {
	if rtl {
		return mirrorRect(rr, r)
	}
	return rr
}

// dtTextX is the x of a text line of width tw starting at the inline
// start of the field text rect tr.
func dtTextX(rtl bool, tr geom.Rect, tw float32) float32 {
	if rtl {
		return tr.Right() - tw
	}
	return tr.X
}

// dtStartText draws a single line off from the inline start of r (entText
// at r.X+off in LTR, right-aligned at r.Right()-off in RTL).
func dtStartText(c *Context, l *paint.List, s string, off float32, r geom.Rect, size float32, wt text.Weight, col theme.Color, maxW float32) {
	if !c.IsRTL() {
		entText(c, l, s, r.X+off, r, size, wt, col, maxW)
		return
	}
	if s == "" {
		return
	}
	opt := text.Options{NoWrap: true}
	if maxW > 0 {
		opt.Ellipsis, opt.MaxWidth = true, maxW
	}
	lay := text.Shape(s, c.style(size, wt), opt)
	l.Glyphs(geom.Pt(r.Right()-off-lay.Width, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
}

// dtEndDivider draws the 1px divider on the inline-end edge of r (preset
// lists beside a panel).
func dtEndDivider(c *Context, l *paint.List, r geom.Rect, th *theme.Theme) {
	x := r.Right() - 1
	if c.IsRTL() {
		x = r.X
	}
	l.Fill(geom.Rect{X: x, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, th.Components.DatePicker.SplitColor)
}

// dtMoveCursor applies a keyboard step to a day cursor; it returns false
// for keys that are not date navigation. Left / Right move visually: in an
// RTL grid (weeks run right to left) Left is the next day.
func dtMoveCursor(cur time.Time, k input.Key, rtl bool) (time.Time, bool) {
	switch k {
	case input.KeyLeft:
		if rtl {
			return cur.AddDate(0, 0, 1), true
		}
		return cur.AddDate(0, 0, -1), true
	case input.KeyRight:
		if rtl {
			return cur.AddDate(0, 0, -1), true
		}
		return cur.AddDate(0, 0, 1), true
	case input.KeyUp:
		return cur.AddDate(0, 0, -7), true
	case input.KeyDown:
		return cur.AddDate(0, 0, 7), true
	case input.KeyPageUp:
		return cur.AddDate(0, -1, 0), true
	case input.KeyPageDown:
		return cur.AddDate(0, 1, 0), true
	}
	return cur, false
}

// dtPanelHeight is the height of one month panel built by dtMonthPanel.
func dtPanelHeight(th *theme.Theme) float32 {
	_, ch := dtCellSize(th)
	hh := th.Components.DatePicker.HeaderHeight
	if th.Flavor == theme.Element {
		// 4 + header, 8 pad, weekday row + 4 margin, 6 rows, 8 pad.
		return 4 + hh + 8 + ch + 4 + 6*ch + 8
	}
	return hh + 8 + 7*ch + 8
}

// dtParseHM parses "H:MM" into minutes after midnight (-1 on error).
func dtParseHM(s string) int {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return -1
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm > 59 {
		return -1
	}
	return hh*60 + mm
}

// dtFormatHM formats minutes after midnight as "HH:MM".
func dtFormatHM(min int) string {
	return fmt.Sprintf("%02d:%02d", min/60%100, min%60)
}

// ---------------------------------------------------------------------------
// Single-date panel (DatePicker, DateTimePicker)

type dtPanelState struct {
	view    time.Time // first day of the shown month
	cursor  time.Time // keyboard cursor (zero = none)
	hasView bool
}

// sync shows the month of val (or today) and resets the cursor.
func (st *dtPanelState) sync(val, now time.Time) {
	base := val
	if base.IsZero() {
		base = now
	}
	st.view = dtMonthUTC(base)
	st.cursor = time.Time{}
	st.hasView = true
}

// key applies a navigation key to the cursor; it returns false for keys
// that are not date navigation.
func (st *dtPanelState) key(k input.Key, val, now time.Time, rtl bool) bool {
	cur := st.cursor
	if cur.IsZero() {
		cur = val
		if cur.IsZero() {
			cur = now
		}
		cur = dtDayUTC(cur)
	}
	next, ok := dtMoveCursor(cur, k, rtl)
	if !ok {
		return false
	}
	st.cursor = next
	st.view = dtMonthStart(next)
	return true
}

// dtDatePanel builds a single-date month panel over st with an optional
// "今天" footer and returns the picked day (UTC midnight).
// ids (optional) receives the semantic IDs of the grid.
func dtDatePanel(c *Context, st *dtPanelState, val time.Time, disabled func(time.Time) bool, today bool, ids *dtGridIDs) (time.Time, bool) {
	if !st.hasView {
		st.sync(val, c.now)
	}
	m := dtMonth{
		view: st.view, today: c.now, selA: val, cursor: st.cursor, disabled: disabled,
		navPrev: true, navNext: true, ids: ids,
		onNav: func(n int) { st.view = dtAddMonths(st.view, n); c.invalid = true },
	}
	if ids != nil {
		*ids = dtGridIDs{}
	}
	var picked time.Time
	var ok bool
	Column(c, func() {
		picked, ok = dtMonthPanel(c, m)
		if today && dtFooterLink(c, c.Locale().Today) {
			if t := dtDayUTC(c.now); disabled == nil || !disabled(t) {
				picked, ok = t, true
			}
		}
	})
	if ok {
		st.view = dtMonthStart(picked)
		st.cursor = picked
		c.invalid = true
	}
	return picked, ok
}

// dtStylePanel applies the DatePicker panel background and shadow to a
// picker dropdown opened with entPanel (which sets the shared radius and
// border).
func dtStylePanel(c *Context, ov *Box) {
	_, tk := dtTheme(c)
	ov.Bg(tk.PanelBg).Shadow(tk.PanelShadow)
}

// dtFooterLink is a centered link in a 40px footer with a top divider.
func dtFooterLink(c *Context, label string) bool {
	_, tk := dtTheme(c)
	clicked := false
	split := tk.SplitColor
	Row(c, func() { clicked = dtLink(c, label) }, layout.H(tk.FooterHeight), layout.Center()).Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, split)
	})
	return clicked
}

// dtLink is a footer action of the pickers ("Today", "Now"): a link, or
// in shadcn a small ghost button (shadcn has no text links in popovers).
func dtLink(c *Context, label string) bool {
	if c.Theme.Flavor == theme.StyleShadcn {
		return Button(c, label).Text().Small().Clicked()
	}
	return Link(c, label).Clicked()
}

// dtMonthUTC returns the first day of t's month at UTC midnight (the
// calendars keep their views and picked days in UTC dates).
func dtMonthUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// dtDayUTC returns t's calendar date at UTC midnight.
func dtDayUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
