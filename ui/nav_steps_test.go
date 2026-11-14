package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func stepsItems() []StepItem {
	return []StepItem{{Title: "Alpha", Description: "a"}, {Title: "Beta", Description: "b"}, {Title: "Gamma", Description: "c"}}
}

func TestStepsInline(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(theme.Dark())} {
		cur := 1
		rtl := false
		d := NewDriver(func(c *Context) {
			dir := LTR
			if rtl {
				dir = RTL
			}
			ConfigProvider(c).Direction(dir).Body(func() {
				Column(c, func() {
					Steps(c, stepsItems()).Type("inline").Clickable(&cur).Show()
				}, layout.Pad(20))
			})
		}, DriverOptions{Theme: th, Width: 800})
		d.Frame()
		a, b, g := mustFind(t, d, "Alpha"), mustFind(t, d, "Beta"), mustFind(t, d, "Gamma")
		// Adjacent, compact (not stretched over the 760px row).
		if b.X != a.Right() || g.X != b.Right() || g.Right()-a.X > 400 {
			t.Fatalf("%v inline layout: %v %v %v", th.Flavor, a, b, g)
		}
		d.Click(g.Center())
		if cur != 2 {
			t.Fatalf("%v inline click: %d", th.Flavor, cur)
		}
		rtl = true
		d.Frame()
		d.Frame()
		if a, g := mustFind(t, d, "Alpha"), mustFind(t, d, "Gamma"); a.X < g.X {
			t.Fatalf("%v inline rtl order: %v %v", th.Flavor, a, g)
		}
	}
}

func TestStepsPanel(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		cur := 0
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				Steps(c, stepsItems()).Panel().Clickable(&cur).Show()
			}, layout.Pad(20))
		}, DriverOptions{Theme: th, Width: 800})
		d.Frame()
		a, b, g := mustFind(t, d, "Alpha"), mustFind(t, d, "Beta"), mustFind(t, d, "Gamma")
		tk := th.Components.Steps
		// Panels share the row, separated by the gap.
		if b.X-a.Right() != tk.PanelGap || g.Right() < 700 || a.H != b.H {
			t.Fatalf("%v panel layout: %v %v %v", th.Flavor, a, b, g)
		}
		// Keyboard: Tab to the second panel, Enter selects it.
		d.Key(input.KeyTab, 0)
		d.Key(input.KeyTab, 0)
		d.Key(input.KeyEnter, 0)
		if cur != 1 {
			t.Fatalf("%v panel keyboard: %d", th.Flavor, cur)
		}
	}
}

func TestStepsDotRender(t *testing.T) {
	type call struct {
		i  int
		st string
		r  geom.Rect
	}
	var calls []call
	for _, inline := range []bool{false, true} {
		calls = calls[:0]
		d := driver(func(c *Context) {
			s := Steps(c, stepsItems()).Current(1).Status("error")
			if inline {
				s.Inline()
			}
			s.DotRender(func(l *paint.List, d StepDot) {
				calls = append(calls, call{d.Index, d.Status, d.Rect})
				if d.Index == 0 {
					d.Default()
				}
			}).Show()
		})
		d.Frame()
		if len(calls) != 3 {
			t.Fatalf("inline=%v: %d calls", inline, len(calls))
		}
		want := []string{"finish", "error", "wait"}
		for i, cl := range calls {
			if cl.i != i || cl.st != want[i] || cl.r.W <= 0 {
				t.Errorf("inline=%v call %d: %+v", inline, i, cl)
			}
		}
	}
	// DotRender turns the dot style on for the default type.
	s := &StepsW{}
	s.DotRender(func(*paint.List, StepDot) {})
	if !s.dot {
		t.Fatal("DotRender did not enable dots")
	}
}

func TestStepsTypeStrings(t *testing.T) {
	s := &StepsW{}
	if s.Type("panel"); s.kind != "panel" || s.nav {
		t.Fatal("panel")
	}
	if s.Type("navigation"); s.kind != "" || !s.nav {
		t.Fatal("navigation")
	}
	if s.Type("inline"); s.kind != "inline" {
		t.Fatal("inline")
	}
}
