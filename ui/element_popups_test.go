package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestElementSelectPopupGeometry(t *testing.T) {
	for _, tt := range []struct {
		name string
		th   *theme.Theme
		gap  float32
	}{
		{"opt in", theme.ElementPlus(theme.ElementPlusControls()), 12},
		{"legacy", theme.ElementPlus(), 4},
		{"Ant", theme.AntDesign(), 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, bottom := range []bool{false, true} {
				value := "First"
				var sel *SelectW
				y := float32(12)
				if bottom {
					y = 106
				}
				d := NewDriver(func(c *Context) {
					Column(c, func() {
						sel = Select(c, &value, Opts("First", "Second")).Layout(layout.W(120)).Open()
					}, layout.Margin(geom.Insets{Top: y, Left: 16}))
				}, DriverOptions{Width: 240, Height: 150, Theme: tt.th})
				d.Frame()
				anchor := d.r.c.rects[sel.n.id]
				panel := d.r.c.rects[sel.st.panel]
				gap := panel.Y - anchor.Bottom()
				if bottom {
					gap = anchor.Y - panel.Bottom()
				}
				if gap != tt.gap || panel.W < anchor.W {
					t.Fatalf("bottom=%v: anchor=%v panel=%v (gap %v, want %v)", bottom, anchor, panel, gap, tt.gap)
				}
			}
		})
	}
}
