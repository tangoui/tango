package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func fillDate(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

func TestDatePickerPanelDayAndViews(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := fillDate(2026, 11, 12)
		changed := 0
		var w *DatePickerPanelW
		d := fillDriver(th, func(c *Context) {
			w = DatePickerPanel(c, &val).DisabledDate(func(t time.Time) bool { return t.Day() == 13 })
			if w.Changed() {
				changed++
			}
		})
		d.ClickLabel("2026-11-20")
		if !val.Equal(fillDate(2026, 11, 20)) || changed != 1 {
			t.Fatalf("val=%v changed=%d", val, changed)
		}
		d.ClickLabel("2026-11-13") // disabled
		if !val.Equal(fillDate(2026, 11, 20)) {
			t.Fatalf("disabled day picked: %v", val)
		}
		// Header year -> decade grid -> year -> month grid -> month -> days.
		if err := d.ClickLabel(ZhCN.Year(2026)); err != nil {
			t.Fatal(err)
		}
		if w.View() != PickerYear {
			t.Fatalf("view = %v", w.View())
		}
		d.ClickLabel("2028")
		if w.View() != PickerMonth {
			t.Fatalf("view after year = %v", w.View())
		}
		d.ClickLabel(ZhCN.Months[2])
		if w.View() != PickerDate {
			t.Fatalf("view after month = %v", w.View())
		}
		d.ClickLabel("2028-03-15")
		if !val.Equal(fillDate(2028, 3, 15)) {
			t.Fatalf("val = %v", val)
		}
		// Header month -> month grid.
		d.ClickLabel(ZhCN.Month(3))
		if w.View() != PickerMonth {
			t.Fatalf("month title view = %v", w.View())
		}
		// Keyboard: Tab focuses the panel, arrows move, Enter drills / picks.
		d.ClickLabel(ZhCN.Months[3])
		d.Key(input.KeyTab, 0)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeyEnter, 0)
		if !val.Equal(fillDate(2028, 3, 17)) {
			t.Fatalf("keyboard val = %v", val)
		}
	})
}

func TestDatePickerPanelPickers(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		week := fillDate(2026, 11, 12)
		q := fillDate(2026, 1, 1)
		y := fillDate(2027, 1, 1)
		var mon time.Time
		d := NewDriver(func(c *Context) {
			Row(c, func() {
				DatePickerPanel(c, &week).Picker(PickerWeek)
				DatePickerPanel(c, &q).Picker(PickerQuarter)
				DatePickerPanel(c, &y).Picker(PickerYear)
				Key(c, "en", func() {
					ConfigProvider(c).Locale(EnUS).Body(func() { DatePickerPanel(c, &mon).Picker(PickerMonth) })
				})
			})
		}, DriverOptions{Width: 1600, Height: 700, Theme: th()})
		d.Settle()
		d.ClickLabel("2026-11-19")
		ws := ZhCN.WeekStart
		want := fillPeriodStart(fillDate(2026, 11, 19), PickerWeek, ws)
		if !week.Equal(want) {
			t.Fatalf("week = %v want %v", week, want)
		}
		d.ClickLabel("Q3")
		if !q.Equal(fillDate(2026, 7, 1)) {
			t.Fatalf("quarter = %v", q)
		}
		d.ClickLabel("2024")
		if !y.Equal(fillDate(2024, 1, 1)) {
			t.Fatalf("year = %v", y)
		}
		d.ClickLabel(EnUS.Months[4])
		if mon.Month() != time.May || mon.Day() != 1 {
			t.Fatalf("month = %v", mon)
		}
	})
}

func TestDatePickerPanelRange(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		from, to := fillDate(2026, 11, 1), time.Time{}
		changed := 0
		d := fillDriver(th, func(c *Context) {
			if DatePickerPanel(c, &from).Range(&to).Changed() {
				changed++
			}
		})
		d.ClickLabel("2026-12-10") // right panel
		if changed != 0 {
			t.Fatal("first click must not commit")
		}
		d.Move(mustFind(t, d, "2026-11-20").Center())
		d.ClickLabel("2026-11-20")
		if !from.Equal(fillDate(2026, 11, 20)) || !to.Equal(fillDate(2026, 12, 10)) || changed != 1 {
			t.Fatalf("range %v..%v changed %d", from, to, changed)
		}
	})
}

func TestDatePickerPanelWeekStartFollowsLocale(t *testing.T) {
	mon := *EnUS
	mon.WeekStart = time.Monday
	for _, loc := range []*Locale{EnUS, &mon} {
		val := fillDate(2026, 11, 12)
		d := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
			ConfigProvider(c).Locale(loc).Body(func() { DatePickerPanel(c, &val).Picker(PickerWeek) })
		})
		d.ClickLabel("2026-11-12")
		want := fillPeriodStart(val, PickerWeek, loc.WeekStart)
		if val.Weekday() != loc.WeekStart || !val.Equal(want) {
			t.Fatalf("week start %v: got %v", loc.WeekStart, val)
		}
		// The first weekday header is the locale's week start.
		r1, ok1 := d.FindText(loc.Weekdays[loc.WeekStart])
		r2, ok2 := d.FindText(loc.Weekdays[(loc.WeekStart+1)%7])
		if !ok1 || !ok2 || r1.X >= r2.X {
			t.Fatalf("header order wrong for %v", loc.WeekStart)
		}
	}
}

func TestColorPickerPanel(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		col := theme.Hex("#1677ff")
		preset := theme.Hex("#ff4500")
		changes := 0
		d := fillDriver(th, func(c *Context) {
			if ColorPickerPanel(c, &col).Presets(preset).Changed() {
				changes++
			}
		})
		r := mustFind(t, d, ZhCN.ColorPicker.Saturation)
		d.Click(geom.Pt(r.X+2, r.Bottom()-2)) // near black
		if theme.Luma(col) > 0.1 || changes == 0 {
			t.Fatalf("sv click: %v changes %d", col, changes)
		}
		d.ClickLabel(cpHex(preset))
		if col != preset {
			t.Fatalf("preset: %v", col)
		}
		// The value input commits typed colors.
		before := changes
		d.Drag(geom.Pt(r.X+r.W-2, r.Y+2), geom.Pt(r.X+r.W/2, r.Y+r.H/2))
		if changes == before {
			t.Fatal("drag did not change the color")
		}
	})
}

func TestColorPickerPanelDisabled(t *testing.T) {
	col := theme.Hex("#1677ff")
	d := fillDriver(func() *theme.Theme { return theme.ElementPlus() }, func(c *Context) {
		ColorPickerPanel(c, &col).Disabled(true)
	})
	if r, ok := d.FindText(ZhCN.ColorPicker.Saturation); ok {
		d.Click(geom.Pt(r.X+2, r.Bottom()-2))
	}
	if col != theme.Hex("#1677ff") {
		t.Fatalf("disabled panel changed the color: %v", col)
	}
}
