package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestFooterToolbarUsesCurrentContainerBounds(t *testing.T) {
	width := float32(300)
	var bar *FooterToolbarW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			bar = FooterToolbar(c).Body(func() { Button(c, "save") })
		}, layout.W(width), layout.Pad(12), layout.Margin(geom.Insets{Left: 80}))
	}, DriverOptions{Width: 800, Height: 500})
	check := func(w, bottom float32) {
		t.Helper()
		r := bar.box.Rect()
		if absf(r.X-92) > .5 || absf(r.W-w) > .5 || absf(r.Bottom()-bottom) > .5 {
			t.Fatalf("toolbar bounds = %v; want x=92, width=%v, bottom=%v", r, w, bottom)
		}
	}
	d.Frame()
	check(276, 500)
	width = 420
	d.Resize(900, 600)
	d.Frame()
	check(396, 600)
}
