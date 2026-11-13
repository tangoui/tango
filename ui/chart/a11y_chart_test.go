package chart

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

func a11yCities() []Row {
	var rows []Row
	for i, m := range []string{"Jan", "Feb", "Mar", "Apr"} {
		rows = append(rows, Row{"m": m, "city": "Tokyo", "t": 10 + i}, Row{"m": m, "city": "London", "t": 5 + i})
	}
	return rows
}

func find(t *testing.T, d *ui.Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %s %q in:\n%s", role, name, d.AccessibilityTree().Dump())
	}
	return n
}

func TestA11yChartFigureAndLegend(t *testing.T) {
	var v View
	d := ui.NewDriver(func(c *ui.Context) {
		ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() {
			v = Line(c, a11yCities()).X("m").Y("t").Series("city").Show()
			Column(c, a11yCities()).X("m").Y("t").Series("city").A11yLabel("Sales").Show()
		})
	}, ui.DriverOptions{Width: 600, Height: 800})
	d.EnableAccessibility()
	fig := find(t, d, a11y.RoleFigure, ui.EnUS.Chart.Line)
	want := []string{"Line chart, 2 series, 8 data points", "from Jan to Apr", "from 5 to 13"}
	for _, w := range want {
		if !strings.Contains(fig.Description, w) {
			t.Errorf("summary %q lacks %q", fig.Description, w)
		}
	}
	if fig := find(t, d, a11y.RoleFigure, "Sales"); !strings.HasPrefix(fig.Description, "Column chart") {
		t.Errorf("column summary %q", fig.Description)
	}
	lg := find(t, d, a11y.RoleButton, "London")
	if !lg.States.Has(a11y.Toggleable|a11y.Pressed|a11y.Focusable) || !lg.Actions.Has(a11y.Press) {
		t.Fatalf("legend item %s %s", lg, lg.Actions)
	}
	if err := d.PerformAction(lg.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if !v.Hidden("London") {
		t.Fatal("press did not hide the series")
	}
	if lg = find(t, d, a11y.RoleButton, "London"); lg.States.Has(a11y.Pressed) {
		t.Errorf("hidden series still pressed: %s", lg)
	}
	// Keyboard: focus the entry and press Space to show it again.
	if err := d.PerformAction(lg.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeySpace, 0)
	d.Settle()
	if v.Hidden("London") {
		t.Fatal("space did not toggle the series")
	}
	if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() > 0 || rep.Count(ui.AuditMissingName) > 0 || rep.Count(ui.AuditNotKeyboard) > 0 {
		t.Errorf("audit:\n%s", rep)
	}
}

func TestA11yChartKinds(t *testing.T) {
	d := ui.NewDriver(func(c *ui.Context) {
		ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() {
			Pie(c, []Row{{"k": "A", "v": 3}, {"k": "B", "v": 1}}).Angle("v").Color("k").Height(200).Show()
			Gauge(c, 0.72).Title("Done").Height(200).Show()
			Liquid(c, 0.4).Height(200).Show()
			TinyArea(c, []float64{3, 5, 2, 8}).Show()
			TinyProgress(c, 0.3).Show()
			Line(c, nil).X("m").Y("t").Height(120).Show()
		})
	}, ui.DriverOptions{Width: 600, Height: 1400})
	d.EnableAccessibility()
	if p := find(t, d, a11y.RoleFigure, ui.EnUS.Chart.Pie); !strings.Contains(p.Description, "2 data points") {
		t.Errorf("pie %q", p.Description)
	}
	g := find(t, d, a11y.RoleMeter, "Done")
	if !g.HasRange || g.ValueNow != 0.72 || g.ValueMax != 1 || g.Value != "72%" {
		t.Errorf("gauge %+v", g)
	}
	if l := find(t, d, a11y.RoleMeter, ui.EnUS.Chart.Liquid); l.Value != "40.00%" {
		t.Errorf("liquid value %q", l.Value)
	}
	sp := d.A11yFindAll(a11y.RoleImage, "")
	found := false
	for _, n := range sp {
		if strings.HasPrefix(n.Name, "Sparkline, 1 series, 4 data points") {
			found = true
		}
	}
	if !found {
		t.Errorf("sparkline not named by its summary:\n%s", d.AccessibilityTree().Dump())
	}
	if pr := find(t, d, a11y.RoleProgressBar, ui.EnUS.A11y.Progress); pr.ValueNow != 30 || pr.Value != "30%" {
		t.Errorf("progress %+v", pr)
	}
	if e := find(t, d, a11y.RoleFigure, ui.EnUS.Chart.Line); e.Description != ui.EnUS.NoData {
		t.Errorf("empty chart description %q", e.Description)
	}
}

func TestLiquidReduceMotion(t *testing.T) {
	moves := func(reduce bool) bool {
		d := ui.NewDriver(func(c *ui.Context) { Liquid(c, 0.5).Show() }, ui.DriverOptions{Width: 300, Height: 300, ReduceMotion: reduce})
		a := append([]byte(nil), d.Frame().Pix...)
		d.Advance(500 * time.Millisecond)
		return string(a) != string(d.Frame().Pix)
	}
	if !moves(false) {
		t.Fatal("wave does not move normally")
	}
	if moves(true) {
		t.Fatal("liquid wave moves with reduced motion")
	}
}

func TestDataTable(t *testing.T) {
	d := ui.NewDriver(func(c *ui.Context) {
		DataTable(c, a11yCities(), "m", "city", "t").A11yLabel("Temperatures").Show()
	}, ui.DriverOptions{Width: 600, Height: 600})
	d.EnableAccessibility()
	g := find(t, d, a11y.RoleGrid, "Temperatures")
	if g.RowCount != 9 || g.ColCount != 3 {
		t.Fatalf("table %d x %d", g.RowCount, g.ColCount)
	}
	find(t, d, a11y.RoleColumnHeader, "city")
	find(t, d, a11y.RoleCell, "London")
	find(t, d, a11y.RoleCell, "13")
}
