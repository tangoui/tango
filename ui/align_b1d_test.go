package ui

// Spec alignment, batch 1 group D (date, time and color pickers): the
// behaviours added to match antd 6.7 / Element Plus 2.14.8. See
// docs/specs/align/1-datetime.md.

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func b1dDay(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

// b1dOpen clicks the trigger id and settles.
func b1dOpen(d *Driver, id ID) {
	d.Click(d.r.c.rects[id].Center())
	d.Settle()
}

// TestDatePickerMinMaxDate: days outside [MinDate, MaxDate] cannot be
// picked and the month arrows stop at the bound months (rc-picker
// useDisabledBoundary, PanelHeader disabledOffsetPrev / Next).
func TestDatePickerMinMaxDate(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).MinDate(dtJan(10)).MaxDate(b1dDay(2, 5))
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	d.Click(mustFind(t, d, "2026-01-09").Center())
	d.Settle()
	if !v.Equal(dtJan(15)) || !w.IsOpen() {
		t.Fatalf("day before MinDate picked: %v open=%v", v, w.IsOpen())
	}
	// The previous-month arrow is disabled at the MinDate month.
	p2aClick(t, d, ZhCN.PrevMonth)
	if _, ok := d.Find("2026-01-31"); !ok {
		t.Fatal("prev arrow left the MinDate month")
	}
	p2aClick(t, d, ZhCN.NextMonth)
	if _, ok := d.Find("2026-02-28"); !ok {
		t.Fatal("next arrow should reach February")
	}
	p2aClick(t, d, ZhCN.NextMonth)
	if _, ok := d.Find("2026-02-28"); !ok {
		t.Fatal("next arrow passed the MaxDate month")
	}
	d.Click(mustFind(t, d, "2026-02-06").Center())
	d.Settle()
	if !v.Equal(dtJan(15)) {
		t.Fatalf("day after MaxDate picked: %v", v)
	}
	d.Click(mustFind(t, d, "2026-02-05").Center())
	d.Settle()
	if !v.Equal(b1dDay(2, 5)) || w.IsOpen() {
		t.Fatalf("MaxDate day not picked: %v", v)
	}
}

// TestDatePickerMinDateMonthPicker: with Picker(PickerMonth) the bound
// compares months: the MinDate month itself stays pickable.
func TestDatePickerMinDateMonthPicker(t *testing.T) {
	var v time.Time
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).Picker(PickerMonth).MinDate(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	p2aClick(t, d, ZhCN.Months[1]) // February: before the MinDate month
	if !v.IsZero() {
		t.Fatalf("month before MinDate picked: %v", v)
	}
	p2aClick(t, d, ZhCN.Months[2]) // March holds MinDate
	if !v.Equal(b1dDay(3, 1)) {
		t.Fatalf("MinDate month not picked: %v", v)
	}
}

// TestDatePickerMinDateOutOfRangeValue: a value outside the bounds is
// still shown (Ant keeps the controlled value) and the panel opens.
func TestDatePickerMinDateOutOfRangeValue(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).MinDate(b1dDay(3, 1))
		}, layout.Pad(20))
	})
	d.Frame()
	if got := w.display(); got != "2026-01-15" {
		t.Fatalf("display %q", got)
	}
	b1dOpen(d, w.n.id)
	if !w.IsOpen() {
		t.Fatal("panel did not open")
	}
	// Keyboard Enter on the out-of-range cursor does not pick.
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if !v.Equal(dtJan(15)) {
		t.Fatalf("out-of-range day picked by keyboard: %v", v)
	}
}

// TestDatePickerNavOffA11y: the arrow stopped by MinDate is disabled in
// the accessibility tree too.
func TestDatePickerNavOffA11y(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).MinDate(dtJan(1))
		}, layout.Pad(20))
	})
	d.EnableAccessibility()
	d.Frame()
	b1dOpen(d, w.n.id)
	prev, ok := d.A11yFind(a11y.RoleButton, ZhCN.PrevMonth)
	if !ok || !prev.States.Has(a11y.Disabled) {
		t.Fatalf("prev month arrow should be a disabled button: %+v", prev)
	}
	next, ok := d.A11yFind(a11y.RoleButton, ZhCN.NextMonth)
	if !ok || next.States.Has(a11y.Disabled) {
		t.Fatal("next month arrow should be enabled")
	}
}

// TestDatePickerShowNowShowWeek: ShowNow(false) drops the Today link,
// ShowWeek adds the ISO week numbers.
func TestDatePickerShowNowShowWeek(t *testing.T) {
	v := dtJan(15)
	var a, b *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			a = DatePicker(c, &v)
			b = DatePicker(c, &v).ShowNow(false).ShowWeek(true)
		}, layout.Pad(20), layout.Gap(400))
	})
	d.Frame()
	b1dOpen(d, a.n.id)
	if _, ok := d.Find(ZhCN.Today); !ok {
		t.Fatal("Today link missing by default")
	}
	if len(p2aFindAll(d, "5")) != 0 {
		t.Fatal("week numbers without ShowWeek")
	}
	d.Key(input.KeyEscape, 0)
	b1dOpen(d, b.n.id)
	if !b.IsOpen() || a.IsOpen() {
		t.Fatalf("open a=%v b=%v", a.IsOpen(), b.IsOpen())
	}
	if _, ok := d.Find(ZhCN.Today); ok {
		t.Fatal("ShowNow(false) still shows Today")
	}
	if len(p2aFindAll(d, "5")) != 1 { // 2026-01-29 is in ISO week 5
		t.Fatal("ShowWeek: week number 5 missing")
	}
}

// TestDatePickerPlacementTop: TopStart opens the panel above the field.
func TestDatePickerPlacementTop(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			Gap(c, 420)
			w = DatePicker(c, &v).Placement(TopStart)
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	field, panel := d.r.c.rects[w.n.id], d.r.c.rects[w.st.panel]
	if panel.Empty() || panel.Bottom() > field.Y {
		t.Fatalf("panel %v not above field %v", panel, field)
	}
}

// TestDatePickerDisabledNewOptions: a disabled picker with the new
// options neither opens nor changes.
func TestDatePickerDisabledNewOptions(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).MinDate(dtJan(1)).ShowWeek(true).Placement(TopEnd).Disabled(true)
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	if w.IsOpen() || w.Changed() {
		t.Fatal("disabled picker opened")
	}
}

// TestDatePickerPanelMinMax: the inline panel greys out cells outside the
// bounds and keeps them unpickable.
func TestDatePickerPanelMinMax(t *testing.T) {
	v := dtJan(15)
	d := dtDriver(func(c *Context) {
		DatePickerPanel(c, &v).MaxDate(dtJan(20))
	})
	d.Settle()
	d.Click(mustFind(t, d, "2026-01-21").Center())
	d.Settle()
	if !v.Equal(dtJan(15)) {
		t.Fatalf("panel picked a day after MaxDate: %v", v)
	}
	d.Click(mustFind(t, d, "2026-01-20").Center())
	d.Settle()
	if !v.Equal(dtJan(20)) {
		t.Fatalf("panel did not pick MaxDate: %v", v)
	}
}

// TestRangePickerOrderFalse: Order(false) keeps the picked order.
func TestRangePickerOrderFalse(t *testing.T) {
	var v [2]time.Time
	var w *RangePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() { w = RangePicker(c, &v).Order(false) }, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	d.Click(mustFind(t, d, "2026-01-20").Center())
	d.Click(mustFind(t, d, "2026-01-10").Center())
	d.Settle()
	if !v[0].Equal(dtJan(20)) || !v[1].Equal(dtJan(10)) {
		t.Fatalf("Order(false) range %v", v)
	}
}

// TestRangePickerMinMaxSeparator: days outside the bounds do not start a
// range; Separator replaces the separator text.
func TestRangePickerMinMaxSeparator(t *testing.T) {
	var v [2]time.Time
	var w *RangePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() { w = RangePicker(c, &v).MinDate(dtJan(10)).Separator("~") }, layout.Pad(20))
	})
	d.EnableAccessibility()
	d.Frame()
	b1dOpen(d, w.n.id)
	d.Click(mustFind(t, d, "2026-01-05").Center())
	d.Settle()
	if w.st.pick != 0 {
		t.Fatal("a day before MinDate started the range")
	}
	d.Click(mustFind(t, d, "2026-01-12").Center())
	d.Click(mustFind(t, d, "2026-01-14").Center())
	d.Settle()
	if !v[0].Equal(dtJan(12)) || !v[1].Equal(dtJan(14)) {
		t.Fatalf("range %v", v)
	}
	n, ok := d.A11yFind(a11y.RoleCombobox, ZhCN.StartDate+" ~ "+ZhCN.EndDate)
	if !ok || n.Value == "" {
		t.Fatalf("separator not used in the accessible name / value: %+v", n)
	}
}

// TestTimePickerNeedConfirmFalse: column picks set the value at once, the
// panel stays open and there is no OK button.
func TestTimePickerNeedConfirmFalse(t *testing.T) {
	var v time.Time
	changed := 0
	var w *TimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = TimePicker(c, &v).NeedConfirm(false)
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.f.n.id)
	if _, ok := d.Find(ZhCN.OK); ok {
		t.Fatal("OK button with NeedConfirm(false)")
	}
	d.Click(mustFind(t, d, "h05").Center())
	d.Settle()
	if v.Hour() != 5 || changed != 1 || !w.IsOpen() {
		t.Fatalf("v=%v changed=%d open=%v", v, changed, w.IsOpen())
	}
	d.Key(input.KeyDown, 0) // keyboard steps apply too
	d.Settle()
	if v.Hour() != 6 {
		t.Fatalf("keyboard step not applied: %v", v)
	}
}

// TestTimePickerShowNowFalseDisabled: ShowNow(false) drops the Now link;
// a disabled picker does not open.
func TestTimePickerShowNowFalseDisabled(t *testing.T) {
	var v, u time.Time
	var w, x *TimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = TimePicker(c, &v).ShowNow(false)
			x = TimePicker(c, &u).NeedConfirm(false).Disabled(true)
		}, layout.Pad(20), layout.Gap(300))
	})
	d.Frame()
	b1dOpen(d, w.f.n.id)
	if _, ok := d.Find(ZhCN.Now); ok {
		t.Fatal("Now link with ShowNow(false)")
	}
	if _, ok := d.Find(ZhCN.OK); !ok {
		t.Fatal("OK button missing")
	}
	d.Key(input.KeyEscape, 0)
	b1dOpen(d, x.f.n.id)
	if x.IsOpen() || !u.IsZero() {
		t.Fatal("disabled time picker opened")
	}
}

// TestTimeRangePickerOrderFalse: Order(false) keeps an overnight range.
func TestTimeRangePickerOrderFalse(t *testing.T) {
	var v [2]time.Time
	var w *TimeRangePickerW
	d := p2aDriver(theme.AntDesign(), func(c *Context) {
		w = TimeRangePicker(c, &v).Format("15:04").Order(false).Separator("-")
	})
	d.EnableAccessibility()
	p2aOpen(d, w.n.id)
	d.Type("22:00")
	d.Key(input.KeyEnter, 0)
	d.Type("06:00")
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if v[0].Hour() != 22 || v[1].Hour() != 6 {
		t.Fatalf("Order(false) range %v", v)
	}
	d.Frame()
	if n, ok := d.A11yFind(a11y.RoleCombobox, ZhCN.TimePicker.StartTime+" - "+ZhCN.TimePicker.EndTime); !ok || n.Value != "22:00 - 06:00" {
		t.Fatalf("separator / value: %+v", n)
	}
}

// TestDateTimePickerDefaultTimeMinDate: a day picked without a time gets
// DefaultTime; days before MinDate are not pickable.
func TestDateTimePickerDefaultTimeMinDate(t *testing.T) {
	var v time.Time
	var w *DateTimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DateTimePicker(c, &v).DefaultTime(time.Date(0, 1, 1, 10, 30, 0, 0, time.UTC)).MinDate(dtJan(10)).ShowNow(false)
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.f.n.id)
	if _, ok := d.Find(ZhCN.Now); ok {
		t.Fatal("Now action with ShowNow(false)")
	}
	d.Click(mustFind(t, d, "2026-01-05").Center())
	d.Settle()
	if !w.st.date.IsZero() {
		t.Fatalf("day before MinDate picked: %v", w.st.date)
	}
	d.Click(mustFind(t, d, "2026-01-12").Center())
	d.Settle()
	p2aClick(t, d, ZhCN.OK)
	if v.Day() != 12 || v.Hour() != 10 || v.Minute() != 30 {
		t.Fatalf("value %v", v)
	}
}

// TestTimeSelectIncludeEndTime: End is appended when the steps skip it.
func TestTimeSelectIncludeEndTime(t *testing.T) {
	var v string
	var a, b *TimeSelectW
	d := dtDriver(func(c *Context) {
		a = TimeSelect(c, &v).Start("09:00").End("17:45").Step("00:30")
		b = TimeSelect(c, &v).Start("09:00").End("17:45").Step("00:30").IncludeEndTime(true).MaxTime("17:40")
	})
	d.Frame()
	oa, ob := a.Options(), b.Options()
	if last := oa[len(oa)-1]; last.Value != "17:30" {
		t.Fatalf("without IncludeEndTime last = %v", last.Value)
	}
	if last := ob[len(ob)-1]; last.Value != "17:45" || !last.Disabled {
		t.Fatalf("IncludeEndTime last = %+v (MaxTime 17:40 disables it)", last)
	}
	if got := moreWithEnd(moreTimeItems("09:00", "10:00", "00:30", "", ""), "10:00", "", ""); len(got) != 3 {
		t.Fatalf("End already reached: %d items", len(got))
	}
}

// TestColorPickerDisabledFormat: the format select is left out.
func TestColorPickerDisabledFormat(t *testing.T) {
	col := theme.Hex("#1677ff")
	var w *ColorPickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() { w = ColorPicker(c, &col).DisabledFormat(true).Open() }, layout.Pad(20))
	})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleCombobox, ZhCN.ColorPicker.Format); ok {
		t.Fatal("DisabledFormat should leave the format select out")
	}
	if _, ok := d.A11yFind(a11y.RoleTextbox, "HEX"); !ok {
		t.Fatal("the hex input should stay")
	}
	if !w.IsOpen() {
		t.Fatal("panel closed")
	}
}

// TestColorPickerPlacementTop: TopStart opens the panel above.
func TestColorPickerPlacementTop(t *testing.T) {
	col := theme.Hex("#1677ff")
	var w *ColorPickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			Gap(c, 500)
			w = ColorPicker(c, &col).Placement(TopStart)
		}, layout.Pad(20))
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	field, panel := d.r.c.rects[w.n.id], d.r.c.rects[w.st.pop.panel]
	if panel.Empty() || panel.Bottom() > field.Y {
		t.Fatalf("panel %v not above field %v", panel, field)
	}
}

// TestB1DPlacementDefaults: Defaults "placement" takes Ant names
// ("topRight": above, end edges aligned), "showWeek" adds week numbers and
// TimePicker "needConfirm": false drops the OK button.
func TestB1DPlacementDefaults(t *testing.T) {
	v := dtJan(15)
	var w *DatePickerW
	var tp *TimePickerW
	var tv time.Time
	d := dtDriver(func(c *Context) {
		ConfigProvider(c).Defaults("DatePicker", Props{"placement": "topRight", "showWeek": true}).
			Defaults("TimePicker", Props{"needConfirm": false}).Body(func() {
			Column(c, func() {
				tp = TimePicker(c, &tv)
				Gap(c, 380)
				Row(c, func() {
					Gap(c, 300)
					w = DatePicker(c, &v)
				})
			}, layout.Pad(20))
		})
	})
	d.Frame()
	b1dOpen(d, w.n.id)
	field, panel := d.r.c.rects[w.n.id], d.r.c.rects[w.st.panel]
	if panel.Empty() || panel.Bottom() > field.Y || panel.Right() != field.Right() {
		t.Fatalf("topRight: panel %v field %v", panel, field)
	}
	if len(p2aFindAll(d, "5")) != 1 {
		t.Fatal("showWeek default: week number missing")
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()
	b1dOpen(d, tp.f.n.id)
	if !tp.IsOpen() {
		t.Fatalf("time picker did not open (date open %v, field %v)", w.IsOpen(), d.r.c.rects[tp.f.n.id])
	}
	if _, ok := d.Find(ZhCN.OK); ok {
		t.Fatal("needConfirm default false: OK button shown")
	}
}

// TestTimePickerNeedConfirmFalseComponentDisabled: under
// ConfigProvider.ComponentDisabled a NeedConfirm(false) picker neither
// opens nor changes.
func TestTimePickerNeedConfirmFalseComponentDisabled(t *testing.T) {
	var v time.Time
	var w *TimePickerW
	d := dtDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			Column(c, func() { w = TimePicker(c, &v).NeedConfirm(false) }, layout.Pad(20))
		})
	})
	d.Frame()
	b1dOpen(d, w.f.n.id)
	d.Key(input.KeyDown, 0)
	d.Settle()
	if w.IsOpen() || !v.IsZero() || w.Changed() {
		t.Fatalf("component-disabled picker reacted: open=%v v=%v", w.IsOpen(), v)
	}
}

// TestRangePickerOrderFalseShowTime: with ShowTime the OK commit keeps
// the picked order too (antd order applies to showTime ranges).
func TestRangePickerOrderFalseShowTime(t *testing.T) {
	var v [2]time.Time
	var w *RangePickerW
	d := p2aDriver(theme.AntDesign(), func(c *Context) {
		w = RangePicker(c, &v).ShowTime().Order(false)
	})
	p2aOpen(d, w.n.id)
	p2aClick(t, d, "2026-01-20")
	p2aClick(t, d, "2026-01-10")
	p2aClick(t, d, ZhCN.OK)
	if !v[0].Equal(dtJan(20)) || !v[1].Equal(dtJan(10)) {
		t.Fatalf("ShowTime Order(false) range %v", v)
	}
}
