package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Typed input, RangePicker granularity, hideDisabledOptions, variants /
// ConfigProvider defaults and the shadcn calendar of the date / time
// pickers.

func shad2aDateThemes(t *testing.T, fn func(t *testing.T, th *theme.Theme)) {
	p2aThemes(t, fn)
	t.Run("shadcn", func(t *testing.T) { fn(t, theme.Shadcn()) })
}

func TestShad2aDatePickerTyping(t *testing.T) {
	shad2aDateThemes(t, func(t *testing.T, th *theme.Theme) {
		v := p2aDay(2026, 1, 15)
		var m time.Time
		changed := 0
		var w, mw *DatePickerW
		d := p2aDriver(th, func(c *Context) {
			w = DatePicker(c, &v).DisabledDate(func(t time.Time) bool { return t.Weekday() == time.Sunday })
			if w.Changed() {
				changed++
			}
			mw = DatePicker(c, &m).Picker(PickerMonth)
		})
		p2aOpen(d, w.n.id)
		d.Type("2026-03-09")
		if !w.st.ty.on || w.st.ty.buf != "2026-03-09" || !w.IsOpen() {
			t.Fatalf("typing %v %q open %v", w.st.ty.on, w.st.ty.buf, w.IsOpen())
		}
		if w.st.fp.view.Month() != time.March || !w.st.fp.cursor.Equal(p2aDay(2026, 3, 9)) {
			t.Fatalf("panel did not follow: view %v cursor %v", w.st.fp.view, w.st.fp.cursor)
		}
		if changed != 0 || !v.Equal(p2aDay(2026, 1, 15)) {
			t.Fatal("typing must not commit before Enter")
		}
		d.Key(input.KeyEnter, 0)
		if !v.Equal(p2aDay(2026, 3, 9)) || changed != 1 || w.IsOpen() || w.st.ty.on {
			t.Fatalf("Enter: %v changed %d open %v", v, changed, w.IsOpen())
		}
		// Invalid and disabled text is not committed.
		d.Key(input.KeyBackspace, 0) // starts editing from the end: "2026-03-0"
		if w.st.ty.buf != "2026-03-0" || !w.IsOpen() {
			t.Fatalf("backspace %q", w.st.ty.buf)
		}
		d.Key(input.KeyEnter, 0)
		if !v.Equal(p2aDay(2026, 3, 9)) || !w.st.ty.on {
			t.Fatal("invalid text committed")
		}
		d.Type("8") // 2026-03-08 is a Sunday
		d.Key(input.KeyEnter, 0)
		if !v.Equal(p2aDay(2026, 3, 9)) || changed != 1 {
			t.Fatalf("disabled date committed: %v", v)
		}
		// Escape drops invalid text and keeps the value.
		d.Key(input.KeyEscape, 0)
		if w.st.ty.on || w.IsOpen() || !v.Equal(p2aDay(2026, 3, 9)) {
			t.Fatal("escape kept the editor")
		}
		// Month picker: typed with its layout, committed on blur.
		p2aOpen(d, mw.n.id)
		d.Type("2026-05")
		d.Key(input.KeyTab, input.ModShift)
		if !m.Equal(p2aDay(2026, 5, 1)) {
			t.Fatalf("month picker typed %v", m)
		}
	})
}

func TestShad2aParsePeriod(t *testing.T) {
	for _, loc := range []*Locale{ZhCN, EnUS, Ja, ZhTW} {
		for _, ws := range []time.Weekday{time.Sunday, time.Monday} {
			for _, day := range []time.Time{p2aDay(2026, 1, 1), p2aDay(2026, 11, 13), p2aDay(2025, 12, 29), p2aDay(2027, 1, 3)} {
				for _, p := range []PickerType{PickerDate, PickerWeek, PickerMonth, PickerQuarter, PickerYear} {
					want := fillPeriodStart(day, p, ws)
					s := p2aFormatPeriod(loc, want, p, p2aPickerLayout(p), ws)
					got, ok := p2aParsePeriod(loc, s, p, p2aPickerLayout(p), ws)
					if !ok || !got.Equal(want) {
						t.Fatalf("%s ws=%v p=%d %q: got %v %v want %v", loc.Code, ws, p, s, got, ok, want)
					}
				}
			}
		}
	}
	if _, ok := p2aParsePeriod(ZhCN, "2026-Q5", PickerQuarter, "", time.Monday); ok {
		t.Fatal("Q5 parsed")
	}
	if got, ok := p2aParsePeriod(ZhCN, "2026-02-18", PickerWeek, "", time.Monday); !ok || !got.Equal(p2aDay(2026, 2, 16)) {
		t.Fatalf("full date in week picker: %v", got)
	}
}

func TestShad2aRangePickerPicker(t *testing.T) {
	shad2aDateThemes(t, func(t *testing.T, th *theme.Theme) {
		var v, q [2]time.Time
		changed := 0
		var w, qw *RangePickerW
		d := p2aDriver(th, func(c *Context) {
			w = RangePicker(c, &v).Picker(PickerMonth)
			if w.Changed() {
				changed++
			}
			qw = RangePicker(c, &q).Picker(PickerQuarter)
		})
		now := d.Now()
		p2aOpen(d, w.n.id)
		if w.st.fp.mode != PickerMonth {
			t.Fatalf("month grid expected, got %d", w.st.fp.mode)
		}
		d.Key(input.KeyEnter, 0) // this month
		if w.st.active != 1 || w.st.fp.rangeStart.IsZero() || changed != 0 {
			t.Fatalf("first end: active %d start %v", w.st.active, w.st.fp.rangeStart)
		}
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyEnter, 0)
		a := fillPeriodStart(now, PickerMonth, time.Monday)
		if changed != 1 || w.IsOpen() || !v[0].Equal(a) || !v[1].Equal(a.AddDate(0, 2, 0)) {
			t.Fatalf("month range %v changed %d open %v", v, changed, w.IsOpen())
		}
		if s := w.show(v[0]); s != a.Format("2006-01") {
			t.Fatalf("trigger text %q", s)
		}
		// Typed ends.
		p2aOpen(d, qw.n.id)
		d.Type("2026-Q3")
		d.Key(input.KeyEnter, 0)
		if qw.st.active != 1 || !qw.IsOpen() {
			t.Fatalf("typed start: active %d", qw.st.active)
		}
		d.Type("2026-Q1")
		d.Key(input.KeyEnter, 0)
		if q != [2]time.Time{p2aDay(2026, 1, 1), p2aDay(2026, 7, 1)} || qw.IsOpen() {
			t.Fatalf("typed quarters %v", q)
		}
	})
}

func TestShad2aRangePickerTyping(t *testing.T) {
	shad2aDateThemes(t, func(t *testing.T, th *theme.Theme) {
		v := [2]time.Time{p2aDay(2026, 1, 5), p2aDay(2026, 1, 20)}
		var w *RangePickerW
		d := p2aDriver(th, func(c *Context) { w = RangePicker(c, &v) })
		r := d.r.c.rects[w.n.id]
		d.Click(r.Center().Add(geom.Pt(r.W/4, 0))) // end segment
		d.Settle()
		if w.st.active != 1 {
			t.Fatalf("active %d", w.st.active)
		}
		d.Type("2026-02-10")
		if w.st.view.Month() != time.January && w.st.view.Month() != time.February {
			t.Fatalf("view %v", w.st.view)
		}
		d.Key(input.KeyEnter, 0)
		if v[1].Format("2006-01-02") != "2026-02-10" || !v[0].Equal(p2aDay(2026, 1, 5)) || w.IsOpen() {
			t.Fatalf("typed end %v", v)
		}
	})
}

func TestShad2aTimeRangePickerTyping(t *testing.T) {
	shad2aDateThemes(t, func(t *testing.T, th *theme.Theme) {
		var v [2]time.Time
		changed := 0
		var w *TimeRangePickerW
		d := p2aDriver(th, func(c *Context) {
			w = TimeRangePicker(c, &v).Format("15:04").DisabledHours(func(h int) bool { return h < 6 })
			if w.Changed() {
				changed++
			}
		})
		p2aOpen(d, w.n.id)
		d.Type("05:00") // disabled
		d.Key(input.KeyEnter, 0)
		if w.st.active != 0 || !w.st.ty.on {
			t.Fatal("disabled clock accepted")
		}
		d.Key(input.KeyA, input.ModMeta|input.ModCtrl)
		d.Type("17:45")
		d.Key(input.KeyEnter, 0)
		if w.st.active != 1 || changed != 0 {
			t.Fatalf("start typed: active %d", w.st.active)
		}
		d.Type("08:30")
		d.Key(input.KeyEnter, 0)
		if changed != 1 || w.IsOpen() || v[0].Format("15:04") != "08:30" || v[1].Format("15:04") != "17:45" {
			t.Fatalf("typed range %v changed %d", v, changed)
		}
	})
}

func TestShad2aHideDisabledOptions(t *testing.T) {
	shad2aDateThemes(t, func(t *testing.T, th *theme.Theme) {
		v := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
		var tr [2]time.Time
		d := p2aDriver(th, func(c *Context) {
			TimePicker(c, &v).DisabledHours(func(h int) bool { return h < 8 }).HideDisabledOptions(true).Open()
		})
		if len(p2aFindAll(d, "h07")) != 0 || len(p2aFindAll(d, "h08")) == 0 {
			t.Fatal("disabled hours should be hidden")
		}
		d = p2aDriver(th, func(c *Context) {
			TimePicker(c, &v).DisabledHours(func(h int) bool { return h < 8 }).Open()
			TimeRangePicker(c, &tr).Open()
		})
		if len(p2aFindAll(d, "h07")) == 0 {
			t.Fatal("disabled hours are shown greyed by default")
		}
	})
}

func TestShad2aDateDefaults(t *testing.T) {
	var dv, tv time.Time
	var rv, trv [2]time.Time
	ts := ""
	var dw *DatePickerW
	var rw *RangePickerW
	var tw *TimePickerW
	var trw *TimeRangePickerW
	var dtw *DateTimePickerW
	var sw *TimeSelectW
	p2aDriver(theme.Shadcn(), func(c *Context) {
		ConfigProvider(c).
			Defaults("DatePicker", Props{"variant": "filled"}).
			Defaults("RangePicker", Props{"picker": PickerYear, "variant": "borderless", "placeholder": [2]string{"a", "b"}}).
			Defaults("TimePicker", Props{"hideDisabledOptions": true, "variant": "underlined"}).
			Defaults("TimeRangePicker", Props{"hideDisabledOptions": true}).
			Defaults("DateTimePicker", Props{"variant": "filled"}).
			Defaults("TimeSelect", Props{"step": "00:15", "allowClear": false, "variant": "filled"}).
			Body(func() {
				dw = DatePicker(c, &dv)
				rw = RangePicker(c, &rv).Variant("outlined") // explicit wins
				tw = TimePicker(c, &tv)
				trw = TimeRangePicker(c, &trv)
				dtw = DateTimePicker(c, &dv)
				sw = TimeSelect(c, &ts)
			})
	})
	if dw.variant != "filled" || rw.picker != PickerYear || rw.variant != "outlined" || rw.ph != [2]string{"a", "b"} {
		t.Fatalf("date defaults: %q %d %q %v", dw.variant, rw.picker, rw.variant, rw.ph)
	}
	if !tw.clock.hide || tw.f.variant != "underlined" || !trw.clock.hide || dtw.f.variant != "filled" {
		t.Fatal("time defaults not applied")
	}
	if sw.step != "00:15" || sw.clearable || sw.variant != "filled" {
		t.Fatalf("TimeSelect defaults: %q %v %q", sw.step, sw.clearable, sw.variant)
	}
}

func TestShad2aShadcnCalendar(t *testing.T) {
	v := p2aDay(2026, 1, 15)
	rv := [2]time.Time{p2aDay(2026, 1, 5), p2aDay(2026, 1, 20)}
	for _, th := range []*theme.Theme{theme.Shadcn(), theme.Shadcn(theme.Dark()), theme.AntDesign()} {
		shad := th.Flavor == theme.StyleShadcn
		d := p2aDriver(th, func(c *Context) {
			DatePicker(c, &v).Open()
		})
		cells := p2aFindAll(d, "2026-01-15")
		if len(cells) == 0 {
			t.Fatal("no day cell")
		}
		if shad && (cells[0].W != 32 || cells[0].H != 32) {
			t.Fatalf("shadcn day cell %v, want size-8", cells[0])
		}
		// shadcn: month chevrons only, outline size-7.
		if got := len(p2aFindAll(d, ZhCN.PrevYear)); (got == 0) == !shad {
			t.Fatalf("%s: prev-year arrows %d", th.Name, got)
		}
		if shad {
			nav := p2aFindAll(d, ZhCN.PrevMonth)
			if len(nav) == 0 || nav[0].W != 28 {
				t.Fatalf("nav button %v", nav)
			}
		}
		// The footer "Today" is a ghost button in shadcn, a link elsewhere.
		d.EnableAccessibility()
		d.Settle()
		if n, ok := d.A11yFind(a11y.RoleButton, ZhCN.Today); ok != shad {
			t.Fatalf("%s: today button %v", th.Name, n)
		}
		// Range band: ends rounded on their outer side, middle square.
		var lo, mid, hi dayLook
		d = p2aDriver(th, func(c *Context) { RangePicker(c, &rv).Open() })
		_ = d
		m := dtMonth{selA: rv[0], selB: rv[1], hideOut: true}
		look := func(day time.Time, col int) dayLook {
			l := dayLook{}
			lo, hi := m.selA, m.selB
			l.rangeMid = dtBetween(day, lo, hi)
			l.rangeLo, l.rangeHi = dtSameDay(day, lo), dtSameDay(day, hi)
			l.rowFirst, l.rowLast = col == 0, col == 6
			return l
		}
		lo, mid, hi = look(rv[0], 1), look(p2aDay(2026, 1, 12), 1), look(rv[1], 3)
		if !lo.rangeLo || lo.rangeHi || !mid.rangeMid || mid.rangeLo || !hi.rangeHi {
			t.Fatal("range looks")
		}
	}
}

func TestShad2aShadcnRangeBandPaint(t *testing.T) {
	// The band of a range end covers the whole cell under the primary
	// button: sample the pixel right of the start day's button corner.
	rv := [2]time.Time{p2aDay(2026, 1, 6), p2aDay(2026, 1, 21)}
	th := theme.Shadcn()
	var w *RangePickerW
	d := p2aDriver(th, func(c *Context) { w = RangePicker(c, &rv).Open() })
	_ = w
	img := d.Image()
	r := p2aFindAll(d, "2026-01-07") // a middle day
	if len(r) == 0 {
		t.Fatal("no middle cell")
	}
	px := img.RGBAAt(int(r[0].X+2), int(r[0].Y+2))
	want := th.ItemHoverBg
	if diff := int(px.R) - int(want.R); diff < -6 || diff > 6 {
		t.Fatalf("middle day corner %v, want accent %v", px, want)
	}
}
