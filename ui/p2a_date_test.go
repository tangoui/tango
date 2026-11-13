package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Interaction tests of the date / time picker extensions (Picker
// granularity, single-date presets / clear / status / width, RangePicker
// ShowTime, TimeRangePicker, 12-hour clock, disabled times, steps and
// ConfigProvider defaults), each in the Ant and Element themes.

func p2aThemes(t *testing.T, fn func(t *testing.T, th *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{{"ant", func() *theme.Theme { return theme.AntDesign() }}, {"element", func() *theme.Theme { return theme.ElementPlus() }}} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th()) })
	}
}

func p2aDriver(th *theme.Theme, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) {
		Column(c, func() { page(c) }, layout.Pad(20), layout.Gap(12))
	}, DriverOptions{Width: 1100, Height: 800, Theme: th})
	d.Settle()
	return d
}

func p2aDay(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

// p2aFindAll returns the rects of every node labelled label (disabled
// ones included), page first, then overlays in creation order.
func p2aFindAll(d *Driver, label string) []geom.Rect {
	var out []geom.Rect
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == label && !n.abs.Empty() {
			out = append(out, n.abs)
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

func p2aClick(t *testing.T, d *Driver, label string) {
	t.Helper()
	if err := d.ClickLabel(label); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
	d.Settle()
}

func p2aOpen(d *Driver, id ID) {
	d.Click(d.r.c.rects[id].Center())
	d.Settle()
}

func TestP2ADatePickerGranularity(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var week, mon, q, yr time.Time
		var ws, ms, qs, ys *DatePickerW
		d := p2aDriver(th, func(c *Context) {
			ws = DatePicker(c, &week).Picker(PickerWeek)
			ms = DatePicker(c, &mon).Picker(PickerMonth)
			qs = DatePicker(c, &q).Picker(PickerQuarter)
			ys = DatePicker(c, &yr).Picker(PickerYear)
		})
		loc := ZhCN
		if ws.ph() != loc.DatePicker.SelectWeek || qs.ph() != loc.DatePicker.SelectQuarter {
			t.Fatalf("placeholders %q %q", ws.ph(), qs.ph())
		}
		// Week: any day picks its week (Sunday start), shown as year-week.
		p2aOpen(d, ws.n.id)
		p2aClick(t, d, "2026-01-14")
		if !week.Equal(p2aDay(2026, 1, 11)) || ws.IsOpen() {
			t.Fatalf("week = %v open=%v", week, ws.IsOpen())
		}
		if got := ws.display(); got != locFmt(ZhCN.DatePicker.WeekFmt, 2026, 3, "rd") {
			t.Fatalf("week display %q", got)
		}
		// Month.
		p2aOpen(d, ms.n.id)
		p2aClick(t, d, loc.Months[4])
		if !mon.Equal(p2aDay(2026, 5, 1)) || ms.display() != "2026-05" {
			t.Fatalf("month = %v %q", mon, ms.display())
		}
		// Quarter.
		p2aOpen(d, qs.n.id)
		p2aClick(t, d, "Q3")
		if !q.Equal(p2aDay(2026, 7, 1)) || qs.display() != "2026-Q3" {
			t.Fatalf("quarter = %v %q", q, qs.display())
		}
		// Year.
		p2aOpen(d, ys.n.id)
		p2aClick(t, d, "2024")
		if !yr.Equal(p2aDay(2024, 1, 1)) || ys.display() != "2024" {
			t.Fatalf("year = %v %q", yr, ys.display())
		}
		// Week labels follow the locale formats.
		ja := *EnUS
		ja.DatePicker.WeekFmt, ja.DatePicker.QuarterFmt = "%[1]d/W%[2]d", "Q%[2]d %[1]d"
		if s := p2aFormatPeriod(&ja, p2aDay(2026, 11, 12), PickerWeek, "", time.Sunday); s != "2026/W46" {
			t.Fatalf("custom week %q", s)
		}
		if s := p2aFormatPeriod(&ja, p2aDay(2026, 11, 12), PickerQuarter, "", time.Sunday); s != "Q4 2026" {
			t.Fatalf("custom quarter %q", s)
		}
	})
}

func TestP2ADatePickerTitleViews(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		v := p2aDay(2026, 1, 15)
		var w *DatePickerW
		d := p2aDriver(th, func(c *Context) { w = DatePicker(c, &v) })
		p2aOpen(d, w.n.id)
		// Day view: year title -> decade, year -> months, month -> days.
		p2aClick(t, d, ZhCN.Year(2026))
		if w.st.fp.mode != PickerYear {
			t.Fatalf("mode %v", w.st.fp.mode)
		}
		p2aClick(t, d, "2027")
		if w.st.fp.mode != PickerMonth {
			t.Fatalf("mode after year %v", w.st.fp.mode)
		}
		p2aClick(t, d, ZhCN.Months[2])
		if w.st.fp.mode != PickerDate || !w.IsOpen() {
			t.Fatalf("mode after month %v open %v", w.st.fp.mode, w.IsOpen())
		}
		p2aClick(t, d, "2027-03-09")
		if !v.Equal(p2aDay(2027, 3, 9)) || w.IsOpen() {
			t.Fatalf("picked %v open %v", v, w.IsOpen())
		}
		// Month title -> month grid; reopening starts at the day view again.
		p2aOpen(d, w.n.id)
		p2aClick(t, d, ZhCN.Month(3))
		if w.st.fp.mode != PickerMonth {
			t.Fatalf("month title mode %v", w.st.fp.mode)
		}
		d.Key(input.KeyEscape, 0)
		d.Settle()
		p2aOpen(d, w.n.id)
		if w.st.fp.mode != PickerDate {
			t.Fatalf("reopen mode %v", w.st.fp.mode)
		}
		// Keyboard: Cmd/Ctrl+Up zooms out, arrows move, Enter drills down.
		d.Key(input.KeyUp, input.ModCtrl|input.ModMeta)
		d.Settle()
		if w.st.fp.mode != PickerMonth {
			t.Fatalf("zoom out mode %v", w.st.fp.mode)
		}
		d.Key(input.KeyRight, 0) // March -> April
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if w.st.fp.mode != PickerDate || !w.IsOpen() {
			t.Fatalf("drill mode %v", w.st.fp.mode)
		}
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if !v.Equal(p2aDay(2027, 4, 2)) {
			t.Fatalf("keyboard pick %v", v)
		}
	})
}

func TestP2ADatePickerPresetsClearStatusWidth(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		v := p2aDay(2026, 1, 15)
		changed := 0
		var w *DatePickerW
		d := p2aDriver(th, func(c *Context) {
			w = DatePicker(c, &v).Status("error").Width(240).
				Presets(DatePreset{Label: "P-yesterday", Value: p2aDay(2025, 12, 31)}, DatePreset{Label: "P-week", Value: p2aDay(2026, 1, 8)})
			if w.Changed() {
				changed++
			}
		})
		if r := d.r.c.rects[w.n.id]; r.W != 240 {
			t.Fatalf("width %v", r.W)
		}
		if w.status != "error" {
			t.Fatal("status")
		}
		p2aOpen(d, w.n.id)
		p2aClick(t, d, "P-week")
		if !v.Equal(p2aDay(2026, 1, 8)) || w.IsOpen() || changed != 1 {
			t.Fatalf("preset %v open %v changed %d", v, w.IsOpen(), changed)
		}
		// Hover shows the clear button; clicking it empties the value.
		r := d.r.c.rects[w.n.id]
		d.Move(r.Center())
		d.Settle()
		d.Click(w.st.clearR.Center())
		d.Settle()
		if !v.IsZero() || w.IsOpen() || changed != 2 {
			t.Fatalf("clear: %v open %v changed %d", v, w.IsOpen(), changed)
		}
	})
	// AllowClear(false): the icon area opens the panel instead.
	v := p2aDay(2026, 1, 15)
	var w *DatePickerW
	d := p2aDriver(theme.AntDesign(), func(c *Context) { w = DatePicker(c, &v).AllowClear(false) })
	d.Move(d.r.c.rects[w.n.id].Center())
	d.Settle()
	d.Click(w.st.clearR.Center())
	d.Settle()
	if v.IsZero() || !w.IsOpen() {
		t.Fatalf("AllowClear(false) cleared: %v open %v", v, w.IsOpen())
	}
}

func TestP2ARangePickerShowTime(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var v [2]time.Time
		changed := 0
		var w *RangePickerW
		d := p2aDriver(th, func(c *Context) {
			w = RangePicker(c, &v).ShowTime()
			if w.Changed() {
				changed++
			}
		})
		p2aOpen(d, w.n.id)
		p2aClick(t, d, "2026-01-05")
		p2aClick(t, d, "2026-01-12")
		if changed != 0 || !w.IsOpen() || !w.st.draft[0].Equal(p2aDay(2026, 1, 5)) || !w.st.draft[1].Equal(p2aDay(2026, 1, 12)) {
			t.Fatalf("days are drafts: %v open %v", w.st.draft, w.IsOpen())
		}
		// Start column set first, end column set second.
		hs := p2aFindAll(d, "h02")
		if len(hs) != 2 {
			t.Fatalf("%d hour-2 options", len(hs))
		}
		d.Click(hs[0].Center())
		d.Settle()
		d.Click(p2aFindAll(d, "h04")[1].Center())
		d.Settle()
		d.Click(p2aFindAll(d, "m03")[1].Center())
		d.Settle()
		p2aClick(t, d, ZhCN.OK)
		want := [2]time.Time{time.Date(2026, 1, 5, 2, 0, 0, 0, time.UTC), time.Date(2026, 1, 12, 4, 3, 0, 0, time.UTC)}
		if v != want || changed != 1 || w.IsOpen() {
			t.Fatalf("committed %v changed %d open %v", v, changed, w.IsOpen())
		}
		if s := w.layout(); s != "2006-01-02 15:04:05" {
			t.Fatalf("layout %q", s)
		}
		// Keyboard: the end day picked again with Enter confirms.
		p2aOpen(d, w.n.id)
		d.Key(input.KeyRight, 0) // cursor Jan 6
		d.Key(input.KeyEnter, 0) // start Jan 6 (clock kept)
		d.Key(input.KeyDown, 0)  // Jan 13
		d.Key(input.KeyEnter, 0) // end Jan 13
		d.Key(input.KeyEnter, 0) // confirm
		d.Settle()
		if w.IsOpen() || !v[0].Equal(time.Date(2026, 1, 6, 2, 0, 0, 0, time.UTC)) || !v[1].Equal(time.Date(2026, 1, 13, 4, 3, 0, 0, time.UTC)) {
			t.Fatalf("keyboard %v open %v", v, w.IsOpen())
		}
	})
}

func TestP2ATimeRangePicker(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var v [2]time.Time
		changed := 0
		var w *TimeRangePickerW
		d := p2aDriver(th, func(c *Context) {
			w = TimeRangePicker(c, &v).Format("15:04").MinuteStep(15).DisabledHours(func(h int) bool { return h < 2 })
			if w.Changed() {
				changed++
			}
		})
		p2aOpen(d, w.n.id)
		if len(p2aFindAll(d, "m10")) != 0 || len(p2aFindAll(d, "m15")) != 2 {
			t.Fatal("minute step 15 not applied")
		}
		// Disabled hour does nothing.
		d.Click(p2aFindAll(d, "h01")[0].Center())
		d.Settle()
		if !w.st.draft[0].IsZero() {
			t.Fatalf("disabled hour picked: %v", w.st.draft[0])
		}
		d.Click(p2aFindAll(d, "h04")[0].Center())
		d.Settle()
		d.Click(p2aFindAll(d, "h03")[1].Center())
		d.Settle()
		d.Click(p2aFindAll(d, "m45")[1].Center())
		d.Settle()
		p2aClick(t, d, ZhCN.OK)
		if changed != 1 || w.IsOpen() || v[0].Hour() != 3 || v[0].Minute() != 45 || v[1].Hour() != 4 {
			t.Fatalf("range %v changed %d (ordered by clock)", v, changed)
		}
		// Keyboard: Right crosses from the start minutes to the end hours.
		p2aOpen(d, w.n.id)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyRight, 0)
		if w.st.active != 1 || w.st.ts[1].col != 0 {
			t.Fatalf("active %d col %d", w.st.active, w.st.ts[1].col)
		}
		d.Key(input.KeyDown, 0) // 4 -> 5
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if v[1].Hour() != 5 || w.IsOpen() || changed != 2 {
			t.Fatalf("keyboard range %v", v)
		}
		if got := w.text(v[0]); got != "03:45" {
			t.Fatalf("text %q", got)
		}
	})
}

func TestP2ATimePickerDisabled12HoursSteps(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		v := time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
		var w *TimePickerW
		loc := *EnUS
		loc.TimePicker.AM, loc.TimePicker.PM = "a.m.", "p.m."
		d := p2aDriver(th, func(c *Context) {
			ConfigProvider(c).Locale(&loc).Body(func() {
				w = TimePicker(c, &v).Use12Hours().MinuteStep(10).
					DisabledHours(func(h int) bool { return h == 22 }).
					DisabledMinutes(func(h, m int) bool { return h == 21 && m == 40 })
			})
		})
		if got := w.f.display(); got != "09:30:00 a.m." {
			t.Fatalf("12h display %q", got)
		}
		p2aOpen(d, w.f.n.id)
		if len(p2aFindAll(d, "h00")) != 1 { // the "12" option
			t.Fatal("12 o'clock option missing")
		}
		if len(p2aFindAll(d, "h13")) != 0 {
			t.Fatal("24-hour options in 12-hour mode")
		}
		if len(p2aFindAll(d, "m05")) != 0 {
			t.Fatal("minute step not applied")
		}
		p2aClick(t, d, "ap01") // PM
		if w.st.draft.Hour() != 21 {
			t.Fatalf("PM draft %v", w.st.draft)
		}
		p2aClick(t, d, "h10") // 22h is disabled
		if w.st.draft.Hour() != 21 {
			t.Fatalf("disabled hour picked %v", w.st.draft)
		}
		p2aClick(t, d, "m40") // 21:40 is disabled
		if w.st.draft.Minute() != 30 {
			t.Fatalf("disabled minute picked %v", w.st.draft)
		}
		// Keyboard skips the disabled 22h: 21 -> 23 (Left x3: AM/PM -> hours).
		for i := 0; i < 3; i++ {
			d.Key(input.KeyLeft, 0)
		}
		d.Key(input.KeyDown, 0)
		d.Settle()
		if w.st.draft.Hour() != 23 {
			t.Fatalf("keyboard hour %v", w.st.draft)
		}
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if v.Hour() != 23 || w.f.display() != "11:30:00 p.m." {
			t.Fatalf("commit %v %q", v, w.f.display())
		}
		// Typing a disabled time does not commit; the locale words parse.
		p2aOpen(d, w.f.n.id)
		d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
		d.Type("10:00:00 p.m.")
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if v.Hour() != 23 {
			t.Fatalf("disabled typed time committed: %v", v)
		}
		d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
		d.Type("08:10:00 p.m.")
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if v.Hour() != 20 || v.Minute() != 10 {
			t.Fatalf("typed 12h time: %v", v)
		}
	})
}

func TestP2AConfigDefaults(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var dv, tv, dtv time.Time
		var rv, trv [2]time.Time
		var dp *DatePickerW
		var tp *TimePickerW
		var rp *RangePickerW
		var trp *TimeRangePickerW
		var dtp *DateTimePickerW
		var explicit *DatePickerW
		p2aDriver(th, func(c *Context) {
			ConfigProvider(c).
				Defaults("DatePicker", Props{"picker": PickerMonth, "width": 240, "allowClear": false, "size": Small}).
				Defaults("TimePicker", Props{"use12Hours": true, "minuteStep": 5, "format": "03:04 PM"}).
				Defaults("RangePicker", Props{"showTime": true, "separator": "~"}).
				Defaults("TimeRangePicker", Props{"format": "15:04", "width": float32(280)}).
				Defaults("DateTimePicker", Props{"format": "2006/01/02 15:04", "placeholder": "When"}).
				Body(func() {
					dp = DatePicker(c, &dv)
					explicit = DatePicker(c, &dv).Picker(PickerYear).Width(200)
					tp = TimePicker(c, &tv)
					rp = RangePicker(c, &rv)
					trp = TimeRangePicker(c, &trv)
					dtp = DateTimePicker(c, &dtv)
				})
		})
		if dp.picker != PickerMonth || dp.n.ln.Width != 240 || dp.allowClear || dp.size != Small {
			t.Fatalf("DatePicker defaults: picker %v width %v clear %v size %v", dp.picker, dp.n.ln.Width, dp.allowClear, dp.size)
		}
		if explicit.picker != PickerYear || explicit.n.ln.Width != 200 {
			t.Fatal("explicit modifiers must win over defaults")
		}
		if !tp.clock.use12 || tp.steps[1] != 5 || tp.layout() != "03:04 PM" {
			t.Fatalf("TimePicker defaults %+v %v %q", tp.clock, tp.steps, tp.layout())
		}
		if !rp.showTime || rp.sep != "~" {
			t.Fatal("RangePicker defaults")
		}
		if trp.layout() != "15:04" || trp.n.ln.Width != 280 {
			t.Fatal("TimeRangePicker defaults")
		}
		if dtp.format != "2006/01/02 15:04" || dtp.f.ph != "When" {
			t.Fatal("DateTimePicker defaults")
		}
	})
}

func TestP2AAccessibility(t *testing.T) {
	p2aThemes(t, func(t *testing.T, th *theme.Theme) {
		var mon, tv time.Time
		var rv, trv [2]time.Time
		var mp *DatePickerW
		var tp *TimePickerW
		var rp *RangePickerW
		var trp *TimeRangePickerW
		d := p2aDriver(th, func(c *Context) {
			mp = DatePicker(c, &mon).Picker(PickerMonth).A11yLabel("Month")
			tp = TimePicker(c, &tv).Use12Hours().DisabledHours(func(h int) bool { return h == 3 }).A11yLabel("Clock")
			rp = RangePicker(c, &rv).ShowTime().A11yLabel("Span")
			trp = TimeRangePicker(c, &trv).A11yLabel("Hours")
		})
		d.EnableAccessibility()
		// Month picker: the combobox controls the month grid of the year.
		p2aOpen(d, mp.n.id)
		cb := mustNode(t, d, a11y.RoleCombobox, "Month")
		grid := dtA11yNode(t, d, cb.Controls)
		if grid.Role != a11y.RoleGrid || grid.Name != ZhCN.Year(2026) {
			t.Fatalf("controls %s %q", grid.Role, grid.Name)
		}
		d.Key(input.KeyRight, 0)
		d.Settle()
		cb = mustNode(t, d, a11y.RoleCombobox, "Month")
		if act := dtA11yNode(t, d, cb.ActiveDescendant); act.Role != a11y.RoleCell || act.Name != ZhCN.Months[1] {
			t.Fatalf("active %s %q", act.Role, act.Name)
		}
		if rows := d.AccessibilityTree().FindAll(a11y.RoleRow, ""); len(rows) == 0 {
			t.Fatal("grid rows missing")
		}
		if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
			t.Fatalf("audit (month grid):\n%s", rep)
		}
		d.Key(input.KeyEscape, 0)
		d.Settle()
		// 12-hour TimePicker: an AM / PM listbox and a disabled option.
		p2aOpen(d, tp.f.n.id)
		mustNode(t, d, a11y.RoleListbox, ZhCN.TimePicker.Meridiem)
		three := mustNode(t, d, a11y.RoleOption, "03")
		if !three.States.Has(a11y.Disabled) {
			t.Fatalf("disabled hour states %s", three.States)
		}
		mustNode(t, d, a11y.RoleOption, ZhCN.TimePicker.PM)
		if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
			t.Fatalf("audit (12h):\n%s", rep)
		}
		d.Key(input.KeyEscape, 0)
		d.Settle()
		// Range with time: two named time groups with hour listboxes.
		p2aOpen(d, rp.n.id)
		mustNode(t, d, a11y.RoleGroup, ZhCN.TimePicker.StartTime)
		mustNode(t, d, a11y.RoleGroup, ZhCN.TimePicker.EndTime)
		if n := len(d.AccessibilityTree().FindAll(a11y.RoleListbox, ZhCN.A11y.Hour)); n != 2 {
			t.Fatalf("%d hour listboxes", n)
		}
		if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
			t.Fatalf("audit (range time):\n%s", rep)
		}
		d.Key(input.KeyEscape, 0)
		d.Settle()
		// TimeRangePicker: Controls is the active hour listbox.
		p2aOpen(d, trp.n.id)
		cbt := mustNode(t, d, a11y.RoleCombobox, "Hours")
		if !cbt.States.Has(a11y.Expanded) {
			t.Fatalf("not expanded %s", cbt.States)
		}
		if l := dtA11yNode(t, d, cbt.Controls); l.Role != a11y.RoleListbox || l.Name != ZhCN.A11y.Hour {
			t.Fatalf("controls %s %q", l.Role, l.Name)
		}
		if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
			t.Fatalf("audit (time range):\n%s", rep)
		}
	})
}

// TestP2ARangePlaceholdersByPicker: without explicit placeholders a
// RangePicker names its ends by granularity (Start month …, StartDate for
// dates), also when the granularity comes from ConfigProvider defaults;
// Placeholder and the "placeholder" default win. The accessible name
// follows the shown placeholders.
func TestP2ARangePlaceholdersByPicker(t *testing.T) {
	var r [2]time.Time
	var month, year, def, exp, cfg *RangePickerW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			Column(c, func() {
				month = RangePicker(c, &r).Picker(PickerMonth)
				def = RangePicker(c, &r)
				exp = RangePicker(c, &r).Picker(PickerQuarter).Placeholder("From", "To")
				ConfigProvider(c).Defaults("RangePicker", Props{"picker": PickerYear}).Body(func() {
					year = RangePicker(c, &r)
				})
				ConfigProvider(c).Defaults("RangePicker", Props{"picker": PickerWeek, "placeholder": [2]string{"W0", "W1"}}).Body(func() {
					cfg = RangePicker(c, &r)
				})
			})
		})
	}, DriverOptions{Width: 600, Height: 600})
	d.EnableAccessibility()
	d.Frame()
	for _, tc := range []struct {
		w    *RangePickerW
		want [2]string
	}{
		{month, [2]string{"Start month", "End month"}},
		{year, [2]string{"Start year", "End year"}},
		{def, [2]string{"Start date", "End date"}},
		{exp, [2]string{"From", "To"}},
		{cfg, [2]string{"W0", "W1"}},
	} {
		if got := tc.w.placeholders(); got != tc.want {
			t.Errorf("placeholders %q, want %q", got, tc.want)
		}
	}
	sep := EnUS.RangeSeparator
	if _, ok := d.A11yFind(a11y.RoleCombobox, "Start month "+sep+" End month"); !ok {
		t.Error("month range accessible name does not follow the picker")
	}
}
