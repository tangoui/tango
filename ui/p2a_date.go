package ui

// Shared helpers of the date / time picker extensions: period labels of
// the week and quarter pickers, 12-hour clock text, ConfigProvider
// defaults and the two-segment range trigger used by RangePicker and
// TimeRangePicker. Texts come from Locale.DatePicker / Locale.TimePicker.

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Period labels

// p2aThursday returns the Thursday of the 7-day week starting at the week
// start of t (the week number of a grid row is the ISO week of it).
func p2aThursday(t time.Time, ws time.Weekday) time.Time {
	s := fillPeriodStart(t, PickerWeek, ws)
	for i := 0; i < 7; i++ {
		if d := s.AddDate(0, 0, i); d.Weekday() == time.Thursday {
			return d
		}
	}
	return s
}

// p2aOrdinal is the English ordinal suffix of n ("st", "nd", "rd", "th").
func p2aOrdinal(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// p2aWeekLabel formats the week whose Thursday is thu ("2026-46th";
// args: week-based year, week number, English ordinal suffix).
func p2aWeekLabel(loc *Locale, thu time.Time) string {
	y, wn := thu.ISOWeek()
	return locFmt(loc.DatePicker.WeekFmt, y, wn, p2aOrdinal(wn))
}

// p2aQuarterLabel formats the quarter of t ("2026-Q4"; args: year, quarter).
func p2aQuarterLabel(loc *Locale, t time.Time) string {
	return locFmt(loc.DatePicker.QuarterFmt, t.Year(), (int(t.Month())-1)/3+1)
}

// p2aPickerLayout is the default Go layout of a picker granularity.
func p2aPickerLayout(p PickerType) string {
	switch p {
	case PickerMonth:
		return "2006-01"
	case PickerYear:
		return "2006"
	}
	return "2006-01-02"
}

// p2aFormatPeriod formats a value of picker p: week and quarter use the
// locale labels, the other granularities the Go layout.
func p2aFormatPeriod(loc *Locale, t time.Time, p PickerType, layout string, ws time.Weekday) string {
	if t.IsZero() {
		return ""
	}
	switch p {
	case PickerWeek:
		return p2aWeekLabel(loc, p2aThursday(t, ws))
	case PickerQuarter:
		return p2aQuarterLabel(loc, t)
	}
	return t.Format(layout)
}

// p2aParsePeriod parses typed text of picker p back to a period start
// (UTC midnight): the Go layout for days, months and years; for weeks and
// quarters the locale label ("2026-46th", "2026-Q4"), checked by formatting
// the candidate again, or any full date written with the default layout.
func p2aParsePeriod(loc *Locale, s string, p PickerType, layout string, ws time.Weekday) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if p != PickerWeek && p != PickerQuarter {
		t, ok := dtParse(s, layout, time.UTC)
		if !ok {
			return time.Time{}, false
		}
		return fillPeriodStart(t, p, ws), true
	}
	if t, ok := dtParse(s, "2006-01-02", time.UTC); ok {
		return fillPeriodStart(t, p, ws), true
	}
	nums := p2aInts(s)
	for i := 0; i < len(nums); i++ {
		for j := 0; j < len(nums); j++ {
			y, n := nums[i], nums[j]
			if i == j || y < 1 || y > 9999 || n < 1 || n > 53 {
				continue
			}
			var cand time.Time
			if p == PickerQuarter {
				if n > 4 {
					continue
				}
				cand = time.Date(y, time.Month((n-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
			} else {
				// A day of ISO week n of y: the Monday of the week of Jan 4,
				// plus n-1 weeks, then its Thursday.
				jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
				mon := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
				cand = fillPeriodStart(mon.AddDate(0, 0, (n-1)*7+3), PickerWeek, ws)
			}
			if strings.EqualFold(p2aFormatPeriod(loc, cand, p, layout, ws), s) {
				return cand, true
			}
		}
	}
	return time.Time{}, false
}

// p2aInts returns the decimal numbers written in s.
func p2aInts(s string) []int {
	var out []int
	n, in := 0, false
	for _, r := range s + " " {
		if r >= '0' && r <= '9' {
			n, in = n*10+int(r-'0'), true
			if n > 1e6 {
				n = 1e6
			}
			continue
		}
		if in {
			out = append(out, n)
		}
		n, in = 0, false
	}
	return out
}

// p2aPickerPlaceholder is the default placeholder of a granularity.
func p2aPickerPlaceholder(loc *Locale, p PickerType) string {
	switch p {
	case PickerWeek:
		return loc.DatePicker.SelectWeek
	case PickerMonth:
		return loc.DatePicker.SelectMonth
	case PickerQuarter:
		return loc.DatePicker.SelectQuarter
	case PickerYear:
		return loc.DatePicker.SelectYear
	}
	return loc.SelectDate
}

// p2aRangePlaceholders are the default RangePicker placeholders (start,
// end) of a granularity.
func p2aRangePlaceholders(loc *Locale, p PickerType) [2]string {
	d := &loc.DatePicker
	switch p {
	case PickerWeek:
		return [2]string{d.StartWeek, d.EndWeek}
	case PickerMonth:
		return [2]string{d.StartMonth, d.EndMonth}
	case PickerQuarter:
		return [2]string{d.StartQuarter, d.EndQuarter}
	case PickerYear:
		return [2]string{d.StartYear, d.EndYear}
	}
	return [2]string{loc.StartDate, loc.EndDate}
}

// ---------------------------------------------------------------------------
// 12-hour clock

// p2aMeridiem returns the AM / PM words of loc.
func p2aMeridiem(loc *Locale) (am, pm string) {
	return loc.TimePicker.AM, loc.TimePicker.PM
}

// p2aFormatClock formats t with layout and swaps Go's AM / PM for the
// locale words.
func p2aFormatClock(loc *Locale, t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	s := t.Format(layout)
	if !strings.Contains(layout, "PM") && !strings.Contains(layout, "pm") {
		return s
	}
	am, pm := p2aMeridiem(loc)
	return p2aReplaceMeridiem(s, [2]string{"AM", "PM"}, [2]string{"am", "pm"}, am, pm)
}

// p2aParseClock parses a 12-hour text written with the locale words.
func p2aParseClock(loc *Locale, s, layout string, l *time.Location) (time.Time, bool) {
	if strings.Contains(layout, "PM") || strings.Contains(layout, "pm") {
		am, pm := p2aMeridiem(loc)
		up, lo := "AM", "PM"
		if strings.Contains(layout, "pm") {
			up, lo = "am", "pm"
		}
		if am != up {
			s = strings.Replace(s, am, up, 1)
		}
		if pm != lo {
			s = strings.Replace(s, pm, lo, 1)
		}
	}
	return dtParse(s, layout, l)
}

func p2aReplaceMeridiem(s string, upper, lower [2]string, am, pm string) string {
	for _, pair := range [][2]string{upper, lower} {
		s = strings.Replace(s, pair[0], am, 1)
		s = strings.Replace(s, pair[1], pm, 1)
	}
	return s
}

// ---------------------------------------------------------------------------
// ConfigProvider defaults

// p2aProps reads ConfigProvider.Defaults of one component. Keys that are
// absent or of an unexpected type leave the destination unchanged.
type p2aProps struct {
	c    *Context
	comp string
}

func p2aDefaults(c *Context, comp string) p2aProps { return p2aProps{c, comp} }

func (p p2aProps) str(key string, dst *string) bool {
	if v, ok := p.c.ConfigProp(p.comp, key); ok {
		if s, ok := v.(string); ok {
			*dst = s
			return true
		}
	}
	return false
}

func (p p2aProps) flag(key string, dst *bool) bool {
	if v, ok := p.c.ConfigProp(p.comp, key); ok {
		if b, ok := v.(bool); ok {
			*dst = b
			return true
		}
	}
	return false
}

func (p p2aProps) num(key string) (float64, bool) {
	v, ok := p.c.ConfigProp(p.comp, key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func (p p2aProps) px(key string, dst *float32) {
	if n, ok := p.num(key); ok {
		*dst = float32(n)
	}
}

func (p p2aProps) integer(key string, dst *int) {
	if n, ok := p.num(key); ok {
		*dst = int(n)
	}
}

func (p p2aProps) size(dst *ControlSize) {
	if v, ok := p.c.ConfigProp(p.comp, "size"); ok {
		if s, ok := v.(ControlSize); ok {
			*dst = s
		}
	}
}

// ---------------------------------------------------------------------------
// Two-segment range trigger (RangePicker, TimeRangePicker)

// p2aRangeField is what the shared range trigger paints.
type p2aRangeField struct {
	id                      ID
	size                    ControlSize
	status                  string
	variant                 string    // frame look (see InputW.Variant)
	ty                      *dtTyping // text typed into the active end (nil: not typeable)
	text                    [2]string // formatted ends ("" = placeholder)
	ph                      [2]string
	sep                     string
	ic                      *icon.Icon
	active                  int
	open, hover, focus, dis bool
	focusVisible, showClear bool
	clearR                  *geom.Rect // receives the clear button rect
}

// p2aPaintRangeField draws a range input: two ends around a separator
// (Element: text, Ant: arrow), the active-end bar (Ant), icon and clear.
func p2aPaintRangeField(c *Context, l *paint.List, r geom.Rect, f p2aRangeField) {
	th := c.Theme
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	rad := geom.Rad(c.controlRadius(f.size))
	if f.variant == "underlined" {
		rad = geom.Radii{}
	}
	c.paintFrame(l, r, rad, f.id, frameState{hover: f.hover, focus: f.open || f.focus, disabled: f.dis, status: f.status, variant: f.variant})
	fs := c.controlFont(f.size)
	is := float32(14)
	iy := r.Y + (r.H-is)/2
	iconCol := th.TextQuaternary
	if f.dis {
		iconCol = th.TextDisabled
	}
	// Laid out in LTR terms and mirrored in RTL (mx / mr): the start end
	// sits at the inline start, icon and clear at the inline end.
	rtl := c.IsRTL()
	mx := func(x, w float32) float32 { return dtMirror(rtl, r, x, w) }
	mr := func(rr geom.Rect) geom.Rect { return dtMirrorRect(rtl, rr, r) }
	*f.clearR = mr(geom.Rect{X: r.Right() - 11 - is, Y: iy, W: is, H: is})
	left := r.X + 11
	if el {
		drawIcon(l, f.ic, mx(left, is), iy, is, iconCol)
		left += is + 8
	}
	right := r.Right() - 11 - is - 8
	if shad {
		// As DatePicker's shadcn field (date.go): px-3 insets and a 16px
		// end icon with 6px before it.
		pad := th.ControlPaddingH
		*f.clearR = mr(geom.Rect{X: r.Right() - pad - 16, Y: r.Y + (r.H-16)/2, W: 16, H: 16})
		left, right = r.X+pad, r.Right()-pad-16-6
	}
	sepW := entTextW(c, f.sep, fs, text.Regular) + 16
	if !el {
		sepW = is + 16
	}
	segW := (right - left - sepW) / 2
	if shad && (f.open || f.focus) && !f.dis {
		// shadcn: the active end sits on an accent chip instead of Ant's
		// underline bar.
		bx := c.Anim(f.id, "dt-bar", left+float32(f.active)*(segW+sepW), th.DurationMid)
		h := r.H - 12
		l.Fill(mr(geom.Rect{X: bx - 6, Y: r.Y + (r.H-h)/2, W: segW + 12, H: h}), geom.Rad(th.RadiusSM), th.ItemHoverBg)
	}
	for i := 0; i < 2; i++ {
		s, col := f.text[i], th.Text
		if s == "" {
			s, col = f.ph[i], th.TextQuaternary
		}
		if f.dis {
			col = th.TextDisabled
		}
		x := left + float32(i)*(segW+sepW)
		if f.ty != nil && i == f.active {
			lh := th.LineHeightFor(fs)
			tr := mr(geom.Rect{X: x, Y: r.Y + (r.H-lh)/2, W: max(segW, 0), H: lh})
			tc := th.Text
			if f.dis {
				tc = th.TextDisabled
			}
			if dtPaintTyping(c, l, tr, f.id, f.ty, c.style(fs, text.Regular), tc, f.ph[i]) {
				continue
			}
		}
		if el {
			tw := min(entTextW(c, s, fs, text.Regular), segW)
			entText(c, l, s, mx(x+(segW-tw)/2, tw), r, fs, text.Regular, col, segW)
		} else if rtl {
			tw := min(entTextW(c, s, fs, text.Regular), segW)
			entText(c, l, s, mx(x, tw), r, fs, text.Regular, col, segW)
		} else {
			entText(c, l, s, x, r, fs, text.Regular, col, segW)
		}
	}
	sx := left + segW
	if el {
		col := th.Text
		if f.dis {
			col = th.TextDisabled
		}
		entText(c, l, f.sep, mx(sx+8, entTextW(c, f.sep, fs, text.Regular)), r, fs, text.Regular, col, 0)
	} else {
		c.paintIcon(l, icon.ArrowRight, mr(geom.Rect{X: sx + 8, Y: iy, W: is, H: is}), iconCol, 0)
		if (f.open || f.focus) && !shad {
			bx := c.Anim(f.id, "dt-bar", left+float32(f.active)*(segW+sepW), th.DurationMid)
			l.Fill(mr(geom.Rect{X: bx, Y: r.Bottom() - 2 - 1, W: segW, H: 2}), geom.Radii{}, th.Primary.Base)
		}
	}
	if f.showClear {
		if el {
			icon.Draw(l, icon.CloseCircle, *f.clearR, th.TextQuaternary, 0)
		} else {
			entClearIcon(c, l, *f.clearR)
		}
		return
	}
	switch {
	case shad:
		drawIcon(l, f.ic, f.clearR.X, f.clearR.Y, 16, theme.Fade(th.Text, .5))
	case !el:
		drawIcon(l, f.ic, f.clearR.X, iy, is, iconCol)
	}
	if f.focusVisible && !f.open {
		entFocusRing(c, l, r, rad.TL)
	}
}
