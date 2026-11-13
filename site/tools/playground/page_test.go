package main

import (
	"testing"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func demoDriver(t *testing.T, name string) (*demoState, *ui.Driver) {
	t.Helper()
	s := newDemo(name)
	d := ui.NewDriver(s.page, ui.DriverOptions{Width: 680, Height: 340})
	t.Cleanup(d.Runner().Close)
	d.Frame()
	return s, d
}

func TestButtonCountsOnlyEnabledActions(t *testing.T) {
	s, d := demoDriver(t, "button")
	if err := d.ClickLabel("Click me"); err != nil {
		t.Fatal(err)
	}
	if s.clicks != 1 {
		t.Fatalf("clicks=%d", s.clicks)
	}
	if err := d.ClickLabel("Disabled"); err != nil {
		t.Fatal(err)
	}
	if s.clicks != 1 {
		t.Fatal("disabled button changed state")
	}
}

func TestInputCommitsAndEditsText(t *testing.T) {
	s, d := demoDriver(t, "input")
	d.Key(input.KeyTab, 0)
	d.Type("Hello 世界")
	if s.name != "Hello 世界" {
		t.Fatalf("name=%q", s.name)
	}
	d.Key(input.KeyBackspace, 0)
	if s.name != "Hello 世" {
		t.Fatalf("backspace name=%q", s.name)
	}
}

func TestSwitchTogglesFromKeyboard(t *testing.T) {
	s, d := demoDriver(t, "switch")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeySpace, 0)
	if !s.enabled {
		t.Fatal("switch did not toggle")
	}
}

func TestSliderRespondsToArrowKey(t *testing.T) {
	s, d := demoDriver(t, "slider")
	before := s.level
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyRight, 0)
	if s.level <= before {
		t.Fatalf("level=%v before=%v", s.level, before)
	}
}

func TestSelectChoosesOption(t *testing.T) {
	s, d := demoDriver(t, "select")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if s.language == "" {
		t.Fatal("selection remained empty")
	}
}

func TestTabsSwitchPanel(t *testing.T) {
	s, d := demoDriver(t, "tabs")
	if err := d.ClickLabel("Events"); err != nil {
		t.Fatal(err)
	}
	if s.tab != "events" {
		t.Fatalf("tab=%q", s.tab)
	}
	if _, ok := d.FindText("Calls, events and streams"); !ok {
		t.Fatal("events panel not rendered")
	}
}

func TestAllDemosRenderInEachTheme(t *testing.T) {
	for _, n := range []string{"button", "input", "switch", "slider", "select", "tabs"} {
		for _, th := range []*theme.Theme{theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.ElementPlus(), theme.ElementPlus(theme.Dark()), theme.Shadcn(), theme.Shadcn(theme.Dark())} {
			s := newDemo(n)
			d := ui.NewDriver(s.page, ui.DriverOptions{Width: 420, Height: 340, Theme: th})
			if img := d.Frame(); img == nil || img.Bounds().Empty() {
				t.Fatalf("empty image for %s", n)
			}
			d.Runner().Close()
		}
	}
}

func TestUnknownGalleryPageDoesNotRenderButtonFallback(t *testing.T) {
	_, d := demoDriver(t, "gallery:DoesNotExist")
	if _, ok := d.FindText("Click me"); ok {
		t.Fatal("unknown component silently rendered Button")
	}
	if _, ok := d.FindText("Unknown component"); !ok {
		t.Fatal("missing explicit unknown-component message")
	}
}

func TestBareGallerySceneCanScrollToItsLowerContent(t *testing.T) {
	if _, ok := gallery.Find("BareScrollRegression"); !ok {
		gallery.Register(gallery.Page{Name: "BareScrollRegression", Bare: true, Height: 1110, Render: func(c *ui.Context) {
			ui.Column(c, func() { ui.Text(c, "Top of dashboard"); ui.Gap(c, 800); ui.Text(c, "Bottom of dashboard") })
		}})
	}
	_, d := demoDriver(t, "gallery:BareScrollRegression")
	before, ok := d.FindText("Bottom of dashboard")
	if !ok {
		t.Fatal("bottom content missing")
	}
	d.Scroll(geom.Pt(100, 100), 800)
	after, ok := d.FindText("Bottom of dashboard")
	if !ok || after.Y >= before.Y || after.Y > 340 {
		t.Fatalf("bottom did not scroll into view: before=%v after=%v", before, after)
	}
}
