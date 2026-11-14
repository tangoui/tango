package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Tests of the date picker gap fill: MultiDatePicker (Ant multiple,
// Element dates / months / years), NeedConfirm, DateTimePicker
// DisabledDate / Presets / DisabledTime, RangePicker DisabledTime and
// Calendar Range. Each runs in the Ant and Element themes.

// dmNode returns the last node labelled label (overlays after the page),
// disabled ones included.
func dmNode(d *Driver, label string) *node {
	var out *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == label && !n.abs.Empty() {
			out = n
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	for _, ov := range d.r.c.overlays {
		walk(ov.root)
	}
	return out
}

func dmDays(ds ...int) []time.Time {
	out := make([]time.Time, len(ds))
	for i, x := range ds {
		out[i] = p2aDay(2026, 1, x)
	}
	return out
}

func dmEqual(a, b []time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func TestDateMultiplePicksSortedAndToggles(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var vals []time.Time
		changed := 0
		var w *MultiDatePickerW
		d := p2aDriver(th, func(c *Context) {
			w = MultiDatePicker(c, &vals).DisabledDate(dtWeekendDay)
			if w.Changed() {
				changed++
			}
		})
		p2aOpen(d, w.box.n.id)
		if !w.IsOpen() {
			t.Fatal("not open")
		}
		p2aClick(t, d, "2026-01-20")
		p2aClick(t, d, "2026-01-07")
		p2aClick(t, d, "2026-01-15")
		if !dmEqual(vals, dmDays(7, 15, 20)) || changed != 3 {
			t.Fatalf("vals = %v changed %d", vals, changed)
		}
		if !w.IsOpen() {
			t.Fatal("multiple closed after a pick")
		}
		// Disabled days do not toggle.
		if n := dmNode(d, "2026-01-10"); n == nil || !n.disabled {
			t.Fatal("weekend not disabled")
		}
		// Clicking a selected day removes it.
		p2aClick(t, d, "2026-01-15")
		if !dmEqual(vals, dmDays(7, 20)) {
			t.Fatalf("toggle off: %v", vals)
		}
		// Tags: one per date, in the trigger.
		if len(w.box.labels) != 2 || w.box.labels[0] != "2026-01-07" || w.box.labels[1] != "2026-01-20" {
			t.Fatalf("tags = %v", w.box.labels)
		}
		// Keyboard: the cursor stays on the last clicked day; Right + Enter
		// toggles the next day, Backspace removes the last date.
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !dmEqual(vals, dmDays(7, 16, 20)) {
			t.Fatalf("keyboard toggle: %v", vals)
		}
		d.Key(input.KeyBackspace, 0)
		d.Settle()
		if !dmEqual(vals, dmDays(7, 16)) {
			t.Fatalf("backspace: %v", vals)
		}
		// Escape closes the panel.
		d.Key(input.KeyEscape, 0)
		d.Settle()
		if w.IsOpen() {
			t.Fatal("Escape kept it open")
		}
	})
}

// dtWeekendDay disables Saturdays and Sundays.
func dtWeekendDay(d time.Time) bool {
	return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
}

func TestDateMultipleMonthsYears(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var months, years []time.Time
		var mw, yw *MultiDatePickerW
		d := p2aDriver(th, func(c *Context) {
			mw = MultiDatePicker(c, &months).Picker(PickerMonth)
			yw = MultiDatePicker(c, &years).Picker(PickerYear)
		})
		loc := ZhCN
		if mw.box.placeholder != loc.DatePicker.SelectMonth {
			t.Fatalf("placeholder %q", mw.box.placeholder)
		}
		p2aOpen(d, mw.box.n.id)
		p2aClick(t, d, loc.Months[4])
		p2aClick(t, d, loc.Months[1])
		if !dmEqual(months, []time.Time{p2aDay(2026, 2, 1), p2aDay(2026, 5, 1)}) {
			t.Fatalf("months = %v", months)
		}
		if len(mw.box.labels) != 2 || mw.box.labels[0] != "2026-02" {
			t.Fatalf("month tags %v", mw.box.labels)
		}
		d.Key(input.KeyEscape, 0)
		d.Settle()
		p2aOpen(d, yw.box.n.id)
		p2aClick(t, d, "2024")
		p2aClick(t, d, "2027")
		if !dmEqual(years, []time.Time{p2aDay(2024, 1, 1), p2aDay(2027, 1, 1)}) {
			t.Fatalf("years = %v", years)
		}
	})
}

func TestDateMultipleNeedConfirm(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		vals := dmDays(5)
		var w *MultiDatePickerW
		d := p2aDriver(th, func(c *Context) {
			w = MultiDatePicker(c, &vals).NeedConfirm(true)
		})
		p2aOpen(d, w.box.n.id)
		p2aClick(t, d, "2026-01-12")
		p2aClick(t, d, "2026-01-05")
		if !dmEqual(vals, dmDays(5)) {
			t.Fatalf("committed before OK: %v", vals)
		}
		if !dmEqual(w.shown(), dmDays(12)) {
			t.Fatalf("draft %v", w.shown())
		}
		// Escape drops the draft.
		d.Key(input.KeyEscape, 0)
		d.Settle()
		if w.IsOpen() || !dmEqual(vals, dmDays(5)) {
			t.Fatalf("escape: open=%v %v", w.IsOpen(), vals)
		}
		p2aOpen(d, w.box.n.id)
		p2aClick(t, d, "2026-01-12")
		p2aClick(t, d, ZhCN.OK)
		if w.IsOpen() || !dmEqual(vals, dmDays(5, 12)) {
			t.Fatalf("OK: open=%v %v", w.IsOpen(), vals)
		}
	})
}

func TestDatePickerNeedConfirm(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		v := p2aDay(2026, 1, 5)
		changed := 0
		var w *DatePickerW
		d := p2aDriver(th, func(c *Context) {
			w = DatePicker(c, &v).NeedConfirm(true)
			if w.Changed() {
				changed++
			}
		})
		p2aOpen(d, w.n.id)
		p2aClick(t, d, "2026-01-14")
		if !v.Equal(p2aDay(2026, 1, 5)) || !w.IsOpen() {
			t.Fatalf("pick committed: %v open=%v", v, w.IsOpen())
		}
		if w.display() != "2026-01-14" {
			t.Fatalf("draft display %q", w.display())
		}
		// Closing drops the draft.
		d.Key(input.KeyEscape, 0)
		d.Settle()
		if w.IsOpen() || !v.Equal(p2aDay(2026, 1, 5)) || w.display() != "2026-01-05" {
			t.Fatalf("escape: open=%v v=%v %q", w.IsOpen(), v, w.display())
		}
		// OK commits.
		p2aOpen(d, w.n.id)
		p2aClick(t, d, "2026-01-14")
		p2aClick(t, d, ZhCN.OK)
		if !v.Equal(p2aDay(2026, 1, 14)) || w.IsOpen() || changed != 1 {
			t.Fatalf("OK: v=%v open=%v changed=%d", v, w.IsOpen(), changed)
		}
		// Keyboard: Enter drafts the cursor day, Enter again confirms.
		p2aOpen(d, w.n.id)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !v.Equal(p2aDay(2026, 1, 14)) || !w.IsOpen() {
			t.Fatalf("first Enter committed: %v", v)
		}
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !v.Equal(p2aDay(2026, 1, 15)) || w.IsOpen() {
			t.Fatalf("second Enter: v=%v open=%v", v, w.IsOpen())
		}
	})
}

func TestDateTimePickerDisabledDateAndPresets(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var v time.Time
		var w *DateTimePickerW
		preset := time.Date(2026, 1, 9, 8, 30, 0, 0, time.UTC)
		d := p2aDriver(th, func(c *Context) {
			w = DateTimePicker(c, &v).DisabledDate(dtWeekendDay).
				Presets(DatePreset{Label: "周五上午", Value: preset})
		})
		p2aOpen(d, w.f.n.id)
		if n := dmNode(d, "2026-01-10"); n == nil || !n.disabled {
			t.Fatal("weekend day not disabled")
		}
		// Typing a disabled day does not commit.
		d.Type("2026-01-11 10:00:00")
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !v.IsZero() {
			t.Fatalf("disabled day committed: %v", v)
		}
		p2aClick(t, d, "周五上午")
		if !v.Equal(preset) || w.IsOpen() {
			t.Fatalf("preset: %v open=%v", v, w.IsOpen())
		}
	})
}

func TestDateTimePickerDisabledTime(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		v := time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC)
		var w *DateTimePickerW
		var days []time.Time
		d := p2aDriver(th, func(c *Context) {
			w = DateTimePicker(c, &v).DisabledTime(func(day time.Time) DisabledTimes {
				days = append(days, day)
				if day.Day() == 15 {
					// The 15th: nothing before 12:00, no odd minutes at noon.
					return DisabledTimes{Hours: func(h int) bool { return h < 12 },
						Minutes: func(h, m int) bool { return h == 12 && m%2 == 1 }}
				}
				return DisabledTimes{}
			})
		})
		p2aOpen(d, w.f.n.id)
		if n := dmNode(d, "h03"); n == nil || n.disabled {
			t.Fatal("hour disabled on the 14th")
		}
		// Picking the 15th keeps 10:00, now disabled: OK is refused.
		p2aClick(t, d, "2026-01-15")
		if len(days) == 0 || !days[len(days)-1].Equal(p2aDay(2026, 1, 15)) {
			t.Fatalf("callback days %v", days)
		}
		if n := dmNode(d, "h03"); n == nil || !n.disabled {
			t.Fatal("hour not disabled on the 15th")
		}
		p2aClick(t, d, ZhCN.OK)
		if !v.Equal(time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC)) {
			t.Fatalf("disabled clock committed: %v", v)
		}
		p2aClick(t, d, "h12")
		if n := dmNode(d, "m01"); n == nil || !n.disabled {
			t.Fatal("odd minute at noon not disabled")
		}
		p2aClick(t, d, ZhCN.OK)
		if !v.Equal(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)) {
			t.Fatalf("v = %v", v)
		}
	})
}

func TestRangePickerDisabledTime(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var r [2]time.Time
		var w *RangePickerW
		var ends []bool
		d := p2aDriver(th, func(c *Context) {
			w = RangePicker(c, &r).ShowTime().DisabledTime(func(day time.Time, end bool) DisabledTimes {
				ends = append(ends, end)
				if end {
					return DisabledTimes{Hours: func(h int) bool { return h >= 18 }}
				}
				return DisabledTimes{Hours: func(h int) bool { return h < 9 }}
			})
		})
		p2aOpen(d, w.n.id)
		p2aClick(t, d, "2026-01-05")
		p2aClick(t, d, "2026-01-08")
		// Start column: hours before 9 disabled; end column: 18 and later.
		var startH, endH []*node
		var walk func(n *node)
		walk = func(n *node) {
			if n.label == "h08" {
				startH = append(startH, n)
			}
			if n.label == "h20" {
				endH = append(endH, n)
			}
			for _, k := range n.kids {
				walk(k)
			}
		}
		for _, ov := range d.r.c.overlays {
			walk(ov.root)
		}
		if len(startH) != 2 || len(endH) != 2 {
			t.Fatalf("columns %d %d", len(startH), len(endH))
		}
		if !startH[0].disabled || startH[1].disabled || endH[0].disabled || !endH[1].disabled {
			t.Fatalf("disabled: start h08 %v/%v end h20 %v/%v", startH[0].disabled, startH[1].disabled, endH[0].disabled, endH[1].disabled)
		}
		if !w.draftBlocked() {
			t.Fatal("00:00 start not blocked")
		}
		if n := dmNode(d, ZhCN.OK); n == nil || n.focus { // a disabled Button is not focusable
			t.Fatal("OK enabled for a blocked range")
		}
		w.commitDraft()
		if !r[0].IsZero() {
			t.Fatalf("blocked range committed: %v", r)
		}
		w.st.draft[0] = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
		d.Settle()
		p2aClick(t, d, ZhCN.OK)
		if !r[0].Equal(time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)) || !r[1].Equal(p2aDay(2026, 1, 8)) {
			t.Fatalf("range = %v", r)
		}
		seen := map[bool]bool{}
		for _, e := range ends {
			seen[e] = true
		}
		if !seen[false] || !seen[true] {
			t.Fatalf("ends %v", ends)
		}
	})
}

func TestCalendarRange(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *theme.Theme
	}{{"ant", theme.AntDesign()}, {"element", theme.ElementPlus()}, {"shadcn", theme.Shadcn()}} {
		t.Run(tc.name, func(t *testing.T) {
			v := time.Time{}
			changed := 0
			d := p2aDriver(tc.th, func(c *Context) {
				// Sunday-first weeks: Mar 1 (Sun) to Apr 11 (Sat), six weeks.
				if Calendar(c, &v).Range(p2aDay(2026, 4, 11), p2aDay(2026, 3, 1)).Changed() {
					changed++
				}
			})
			if dmNode(d, "2026-02-28") != nil || dmNode(d, "2026-04-12") != nil {
				t.Fatal("days outside the weeks of the range shown")
			}
			if dmNode(d, "2026-03-01") == nil || dmNode(d, "2026-04-11") == nil {
				t.Fatal("range ends missing")
			}
			p2aClick(t, d, "2026-04-02")
			if !v.Equal(p2aDay(2026, 4, 2)) || changed != 1 {
				t.Fatalf("pick %v changed %d", v, changed)
			}
		})
	}
	// Ranges not on week bounds: padded days are blocked.
	v := time.Time{}
	d := p2aDriver(theme.ElementPlus(), func(c *Context) {
		Calendar(c, &v).Range(p2aDay(2026, 3, 4), p2aDay(2026, 3, 20))
	})
	if dmNode(d, "2026-03-01") == nil {
		t.Fatal("grid does not start with the range's week")
	}
	if dmNode(d, "2026-03-21") == nil || dmNode(d, "2026-03-22") != nil {
		t.Fatal("grid does not end with the range's week")
	}
	p2aClick(t, d, "2026-03-03")
	if !v.IsZero() {
		t.Fatalf("padding day picked: %v", v)
	}
}
