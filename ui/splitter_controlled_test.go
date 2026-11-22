package ui

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestSplitterControlledSizeAndCollapse(t *testing.T) {
	width, collapsed := float32(140), false
	var resize, folds int
	var split *SplitterW
	d := NewDriver(func(c *Context) {
		split = Splitter(c).Style(layout.Fill())
		split.Body(func() {
			SplitterPanel(c).Size(width).Min(100).Collapsed(collapsed).CollapseToggle("", "").
				OnResize(func(px float32) { width = px; resize++ }).
				OnCollapse(func(on bool) { collapsed = on; folds++ })
			SplitterPanel(c)
		})
	}, DriverOptions{Width: 500, Height: 150})
	assertWidth := func(want float32) {
		t.Helper()
		d.Frame()
		if got := split.PixelSizes()[0]; math.Abs(float64(got-want)) > 0.01 {
			t.Fatalf("width %.2f, want %.2f", got, want)
		}
	}
	assertWidth(140)
	d.Resize(800, 150)
	d.Frame()
	assertWidth(140)
	width, collapsed = 180, true
	assertWidth(0)
	if folds+resize != 0 {
		t.Fatalf("external control emitted events: %d resize, %d folds", resize, folds)
	}
	bar, ok := d.Find("splitter-bar-1")
	if !ok {
		t.Fatal("bar missing")
	}
	d.Click(geom.Pt(bar.Center().X, bar.Y+15))
	d.Key(input.KeyEnter, 0)
	if collapsed || folds != 1 {
		t.Fatalf("Enter did not restore: collapsed=%v, folds=%d", collapsed, folds)
	}
	assertWidth(180)
	d.Key(input.KeyRight, 0)
	if resize != 1 {
		t.Fatalf("expected one resize event, got %d", resize)
	}
	assertWidth(190)
}
