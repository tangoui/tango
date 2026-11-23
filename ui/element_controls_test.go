package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestElementChoiceGeometry(t *testing.T) {
	for _, tt := range []struct {
		name         string
		th           *theme.Theme
		height, font float32
	}{
		{"opt in", theme.ElementPlus(theme.ElementPlusControls()), 32, 14},
		{"legacy", theme.ElementPlus(), 22, 12},
		{"Ant", theme.AntDesign(theme.ElementPlusControls()), 22, 12},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var value string
			var segment *SegmentedW
			d := NewDriver(func(c *Context) {
				RadioGroup(c, &value, Opts("Alpha", "Beta")).Vertical()
				segment = Segmented(c, &value, Opts("One", "Two")).Size(Small)
			}, DriverOptions{Width: 260, Height: 180, Theme: tt.th})
			d.Frame()
			a, _ := d.Find("Alpha")
			b, _ := d.Find("Beta")
			if a.H != tt.height || b.H != tt.height || (tt.height == 32 && b.Y != a.Bottom()) {
				t.Fatalf("radio rows: %v %v", a, b)
			}
			if got := segment.font(); got != tt.font {
				t.Fatalf("small segment font: %v, want %v", got, tt.font)
			}
		})
	}
}

func BenchmarkElementChoiceLayout(b *testing.B) {
	var value string
	var checked, switched bool
	opts := Opts("Alpha", "Beta", "Gamma")
	d := NewDriver(func(c *Context) {
		for range 8 {
			Row(c, func() {
				RadioGroup(c, &value, opts)
				Segmented(c, &value, opts)
				Checkbox(c, "Checked", &checked)
				Switch(c, &switched)
			}, layout.Gap(12))
		}
	}, DriverOptions{Theme: theme.ElementPlus(theme.ElementPlusControls())})
	d.Frame()
	b.ResetTimer()
	for range b.N {
		_ = d.r.Frame()
	}
}
