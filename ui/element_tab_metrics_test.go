package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

func TestElementTabItemHeight(t *testing.T) {
	var active string
	d := NewDriver(func(c *Context) {
		Tabs(c, &active, []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}).
			TabItemHeight(30).ActiveTabFontWeight(600).IndicatorThickness(1).Show()
	}, DriverOptions{Theme: theme.ElementPlus(theme.ElementPlusControls())})
	d.Frame()
	for _, name := range []string{"Alpha", "Beta"} {
		r, ok := d.Find(name)
		if !ok || r.H != 30 {
			t.Fatalf("%s geometry: %v, %v", name, r, ok)
		}
	}
}

func TestCodeBlockVisibleStartByteOnSoftWrap(t *testing.T) {
	source := strings.Repeat("abcdef", 90)
	var offset int
	var ok bool
	d := NewDriver(func(c *Context) {
		offset, ok = CodeBlock(c, source).Wrap(true).Height(64).VisibleStartByte()
	}, DriverOptions{Width: 180, Height: 120})
	d.Frame()
	if ok {
		t.Fatal("reported geometry before the first paint")
	}
	d.Frame()
	if !ok || offset != 0 {
		t.Fatalf("initial offset = %d, %v", offset, ok)
	}
	d.Scroll(geom.Pt(60, 40), 96)
	d.Frame()
	if !ok || offset <= 0 || offset >= len(source) {
		t.Fatalf("scrolled offset = %d, %v", offset, ok)
	}
	source = "replacement"
	d.Frame()
	if ok {
		t.Fatal("reported the previous document's geometry")
	}
}
