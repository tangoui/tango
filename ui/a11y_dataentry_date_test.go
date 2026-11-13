package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// Accessibility of the date / time pickers and the ColorPicker.

func dtA11yNode(t *testing.T, d *Driver, id a11y.NodeID) *a11y.Node {
	t.Helper()
	n := d.AccessibilityTree().Node(id)
	if n == nil {
		t.Fatalf("node %d not in tree:\n%s", id, d.AccessibilityTree().Dump())
	}
	return n
}

func TestA11yDatePickerGrid(t *testing.T) {
	loc := ZhCN
	var val time.Time
	d := NewDriver(func(c *Context) { DatePicker(c, &val) }, DriverOptions{Height: 600})
	d.EnableAccessibility()
	now := d.Now()
	val = time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, time.UTC)
	d.Settle()
	cb := mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if !cb.States.Has(a11y.Expandable|a11y.HasPopup|a11y.Focusable) || cb.States.Has(a11y.Expanded) {
		t.Fatalf("closed trigger states %s", cb.States)
	}
	if cb.Value != val.Format("2006-01-02") {
		t.Fatalf("value %q", cb.Value)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if !cb.States.Has(a11y.Expanded) {
		t.Fatalf("Enter did not open: %s", cb.States)
	}
	grid := dtA11yNode(t, d, cb.Controls)
	if grid.Role != a11y.RoleGrid || grid.Name != loc.YearMonth(now.Year(), int(now.Month())) {
		t.Fatalf("controls %s %q", grid.Role, grid.Name)
	}
	if hs := d.AccessibilityTree().FindAll(a11y.RoleColumnHeader, ""); len(hs) != 7 {
		t.Fatalf("%d weekday headers", len(hs))
	}
	act := dtA11yNode(t, d, cb.ActiveDescendant)
	if act.Role != a11y.RoleCell || act.Name != loc.Date(now.Year(), int(now.Month()), 10) || !act.States.Has(a11y.Selected) {
		t.Fatalf("active cell %s %q %s", act.Role, act.Name, act.States)
	}
	today := mustNode(t, d, a11y.RoleCell, loc.Date(now.Year(), int(now.Month()), now.Day()))
	if !today.States.Has(a11y.Current) {
		t.Fatalf("today states %s", today.States)
	}
	// Right moves the active day, PageDown the month; Enter picks.
	d.Key(input.KeyRight, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if n := dtA11yNode(t, d, cb.ActiveDescendant); n.Name != loc.Date(now.Year(), int(now.Month()), 11) {
		t.Fatalf("after Right active %q", n.Name)
	}
	d.Key(input.KeyPageDown, 0)
	d.Settle()
	next := time.Date(now.Year(), now.Month(), 11, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if g := dtA11yNode(t, d, cb.Controls); g.Name != loc.YearMonth(next.Year(), int(next.Month())) {
		t.Fatalf("after PageDown grid %q", g.Name)
	}
	if n := dtA11yNode(t, d, cb.ActiveDescendant); n.Name != loc.Date(next.Year(), int(next.Month()), next.Day()) {
		t.Fatalf("after PageDown active %q", n.Name)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if !val.Equal(next) {
		t.Fatalf("picked %v, want %v", val, next)
	}
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if cb.States.Has(a11y.Expanded) || !cb.States.Has(a11y.Focused) {
		t.Fatalf("after pick %s", cb.States)
	}
	// Escape closes and keeps focus on the trigger.
	d.Key(input.KeyEnter, 0)
	d.Settle()
	d.Key(input.KeyEscape, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectDate)
	if cb.States.Has(a11y.Expanded) || !cb.States.Has(a11y.Focused) {
		t.Fatalf("after Escape %s", cb.States)
	}
	if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Fatalf("audit:\n%s", rep)
	}
}

func TestA11yDatePickerNavButtonsAndDisabled(t *testing.T) {
	loc := EnUS
	var val time.Time
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			DatePicker(c, &val).DisabledDate(func(t time.Time) bool { return t.Day() == 1 }).Open()
		})
	}, DriverOptions{Height: 600})
	d.EnableAccessibility()
	for _, s := range []string{loc.PrevYear, loc.PrevMonth, loc.NextMonth, loc.NextYear} {
		mustNode(t, d, a11y.RoleButton, s)
	}
	now := d.Now()
	first := mustNode(t, d, a11y.RoleCell, loc.Date(now.Year(), int(now.Month()), 1))
	if !first.States.Has(a11y.Disabled) {
		t.Fatalf("disabled day states %s", first.States)
	}
	if got := loc.Date(2026, 11, 12); got != "Nov 12, 2026" {
		t.Fatalf("EnUS date %q", got)
	}
	if err := d.A11yPerform(a11y.RoleButton, loc.NextMonth, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	nm := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	mustNode(t, d, a11y.RoleGrid, loc.YearMonth(nm.Year(), int(nm.Month())))
}

func TestA11yRangePicker(t *testing.T) {
	loc := ZhCN
	var val [2]time.Time
	d := NewDriver(func(c *Context) {
		RangePicker(c, &val).Presets(DatePreset{Label: "P7", Range: [2]time.Time{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)}})
	}, DriverOptions{Width: 1000, Height: 600})
	d.EnableAccessibility()
	name := loc.StartDate + " " + loc.RangeSeparator + " " + loc.EndDate
	mustNode(t, d, a11y.RoleCombobox, name)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	cb := mustNode(t, d, a11y.RoleCombobox, name)
	if !cb.States.Has(a11y.Expanded) || cb.Controls == 0 || cb.ActiveDescendant == 0 {
		t.Fatalf("open range trigger %s controls=%d active=%d", cb.States, cb.Controls, cb.ActiveDescendant)
	}
	if n := len(d.AccessibilityTree().FindAll(a11y.RoleGrid, "")); n != 2 {
		t.Fatalf("%d grids", n)
	}
	now := d.Now()
	if n := dtA11yNode(t, d, cb.ActiveDescendant); n.Name != loc.Date(now.Year(), int(now.Month()), now.Day()) {
		t.Fatalf("active %q", n.Name)
	}
	// Enter picks the start, Right + Enter the end.
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !dtSameDay(val[0], day) || !dtSameDay(val[1], day.AddDate(0, 0, 1)) {
		t.Fatalf("range %v", val)
	}
	cb = mustNode(t, d, a11y.RoleCombobox, name)
	if cb.Value != dtFormat(val[0], "2006-01-02")+" "+loc.RangeSeparator+" "+dtFormat(val[1], "2006-01-02") {
		t.Fatalf("value %q", cb.Value)
	}
	// Presets are buttons.
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if err := d.A11yPerform(a11y.RoleButton, "P7", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if val[0].Day() != 1 || val[1].Day() != 7 {
		t.Fatalf("preset range %v", val)
	}
}

func TestA11yTimePickerColumns(t *testing.T) {
	loc := ZhCN
	val := time.Date(2026, 11, 12, 9, 30, 15, 0, time.UTC)
	d := NewDriver(func(c *Context) { TimePicker(c, &val) }, DriverOptions{Height: 600})
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, loc.SelectTime)
	if !cb.States.Has(a11y.Editable|a11y.Expandable|a11y.Focusable) || cb.Value != "09:30:15" {
		t.Fatalf("time combobox %s %q", cb.States, cb.Value)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectTime)
	if !cb.States.Has(a11y.Expanded | a11y.Focused) {
		t.Fatalf("open states %s", cb.States)
	}
	hours := dtA11yNode(t, d, cb.Controls)
	if hours.Role != a11y.RoleListbox || hours.Name != loc.A11y.Hour {
		t.Fatalf("controls %s %q", hours.Role, hours.Name)
	}
	mustNode(t, d, a11y.RoleListbox, loc.A11y.Minute)
	mustNode(t, d, a11y.RoleListbox, loc.A11y.Second)
	if a := dtA11yNode(t, d, cb.ActiveDescendant); a.Role != a11y.RoleOption || a.Name != "09" || !a.States.Has(a11y.Selected) {
		t.Fatalf("active %s %q %s", a.Role, a.Name, a.States)
	}
	// Right switches to minutes, Down steps them.
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectTime)
	if l := dtA11yNode(t, d, cb.Controls); l.Name != loc.A11y.Minute {
		t.Fatalf("controls after Right %q", l.Name)
	}
	if a := dtA11yNode(t, d, cb.ActiveDescendant); a.Name != "31" {
		t.Fatalf("active minute %q", a.Name)
	}
	// Collapse from assistive technology closes it.
	if err := d.PerformAction(cb.ID, a11y.Collapse, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectTime); cb.States.Has(a11y.Expanded) {
		t.Fatalf("still expanded")
	}
	if err := d.PerformAction(cb.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectTime); !cb.States.Has(a11y.Expanded) {
		t.Fatalf("Expand did not open")
	}
	if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Fatalf("audit:\n%s", rep)
	}
}

func TestA11yDateTimePickerAndTimePanel(t *testing.T) {
	loc := ZhCN
	val := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	tp := time.Date(2026, 1, 1, 7, 5, 0, 0, time.UTC)
	d := NewDriver(func(c *Context) {
		DateTimePicker(c, &val).A11yLabel("Start").Open()
		TimePanel(c, &tp).Format("15:04")
	}, DriverOptions{Width: 1000, Height: 700})
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, "Start")
	if !cb.States.Has(a11y.Expanded) || cb.Value != "2026-11-12 08:00:00" {
		t.Fatalf("datetime %s %q", cb.States, cb.Value)
	}
	mustNode(t, d, a11y.RoleGrid, loc.YearMonth(2026, 11))
	if c := mustNode(t, d, a11y.RoleCell, loc.Date(2026, 11, 12)); !c.States.Has(a11y.Selected) {
		t.Fatalf("selected day %s", c.States)
	}
	g := mustNode(t, d, a11y.RoleGroup, loc.SelectTime)
	if g.Value != "07:05" || !g.States.Has(a11y.Focusable) {
		t.Fatalf("time panel %q %s", g.Value, g.States)
	}
	if a := dtA11yNode(t, d, g.ActiveDescendant); a.Name != "07" {
		t.Fatalf("time panel active %q", a.Name)
	}
}

func TestA11yTimeSelect(t *testing.T) {
	loc := ZhCN
	val := "10:00"
	d := NewDriver(func(c *Context) { TimeSelect(c, &val) }, DriverOptions{Height: 600})
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, loc.SelectTime)
	if cb.Value != "10:00" {
		t.Fatalf("value %q", cb.Value)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.SelectTime)
	if l := dtA11yNode(t, d, cb.Controls); l.Role != a11y.RoleListbox || l.Name != loc.SelectTime {
		t.Fatalf("list %s %q", l.Role, l.Name)
	}
	if a := dtA11yNode(t, d, cb.ActiveDescendant); a.Name != "10:30" {
		t.Fatalf("active %q", a.Name)
	}
	if o := mustNode(t, d, a11y.RoleOption, "10:00"); !o.States.Has(a11y.Selected) {
		t.Fatalf("selected option %s", o.States)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if val != "10:30" {
		t.Fatalf("picked %q", val)
	}
}

func TestA11yColorPicker(t *testing.T) {
	loc := ZhCN
	val := theme.Hex("#1677ff")
	presets := []theme.Color{theme.Hex("#ff0000"), theme.Hex("#00ff00"), theme.Hex("#0000ff")}
	d := NewDriver(func(c *Context) { ColorPicker(c, &val).Presets(presets...).AllowClear() }, DriverOptions{Height: 700})
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, loc.A11y.Color)
	if cb.Value != "#1677FF" {
		t.Fatalf("value %q", cb.Value)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, loc.A11y.Color)
	if !cb.States.Has(a11y.Expanded) {
		t.Fatalf("not open: %s", cb.States)
	}
	if p := dtA11yNode(t, d, cb.Controls); p.Role != a11y.RoleDialog || p.Name != loc.A11y.Color {
		t.Fatalf("panel %s %q", p.Role, p.Name)
	}
	hue := mustNode(t, d, a11y.RoleSlider, loc.ColorPicker.Hue)
	if !hue.HasRange || hue.ValueMax != 360 || !hue.Actions.Has(a11y.Increment) {
		t.Fatalf("hue %+v", hue)
	}
	alpha := mustNode(t, d, a11y.RoleSlider, loc.ColorPicker.Alpha)
	if alpha.ValueNow != 100 || alpha.Value != "100%" {
		t.Fatalf("alpha %v %q", alpha.ValueNow, alpha.Value)
	}
	sv := mustNode(t, d, a11y.RoleSlider, loc.ColorPicker.Saturation)
	if !sv.HasRange || sv.ValueMax != 100 {
		t.Fatalf("saturation %+v", sv)
	}
	mustNode(t, d, a11y.RoleButton, loc.ColorPicker.ClearColor)
	mustNode(t, d, a11y.RoleCombobox, loc.ColorPicker.Format)
	mustNode(t, d, a11y.RoleTextbox, "HEX")
	// SetValue on the alpha slider.
	if err := d.PerformAction(alpha.ID, a11y.SetValue, "50"); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if val.A < 126 || val.A > 129 {
		t.Fatalf("alpha after SetValue %d", val.A)
	}
	// Presets: a radio group, arrow keys move and pick.
	g := mustNode(t, d, a11y.RoleRadioGroup, loc.ColorPicker.Presets)
	if len(g.Children) != 3 {
		t.Fatalf("%d swatches", len(g.Children))
	}
	if err := d.A11yPerform(a11y.RoleRadio, "#FF0000", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if r := mustNode(t, d, a11y.RoleRadio, "#FF0000"); !r.States.Has(a11y.Checked) {
		t.Fatalf("red not checked: %s", r.States)
	}
	if err := d.A11yPerform(a11y.RoleRadio, "#FF0000", a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	d.Key(input.KeyRight, 0)
	d.Settle()
	if val != presets[1] {
		t.Fatalf("Right picked %v", val)
	}
	if f := d.A11yFocused(); f == nil || f.Name != "#00FF00" {
		t.Fatalf("focus %v", f)
	}
	if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Fatalf("audit:\n%s", rep)
	}
}
