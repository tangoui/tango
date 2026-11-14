//go:build darwin

package metal

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// BenchmarkMetalFrostedBar measures a FooterToolbar-sized frosted bar
// (a 1920×56 backdrop blur of radius 8 under a 60% fill at the window's
// bottom, as the pinned FooterToolbar draws it) over the full-window
// editor frame, against the same frame with a plain bar: the difference
// is the blur's cost per frame.
func BenchmarkMetalFrostedBar(b *testing.B) {
	bar := geom.R(0, windowSize.H-56, windowSize.W, 56)
	for _, scale := range []float32{1, 2} {
		for _, frosted := range []bool{false, true} {
			name := fmt.Sprintf("scale%v/plain", scale)
			if frosted {
				name = fmt.Sprintf("scale%v/frosted", scale)
			}
			b.Run(name, func(b *testing.B) {
				d := newDevice(b)
				l := editorList(b)
				if frosted {
					l.BackdropBlur(bar, geom.Radii{}, 8)
					l.Fill(bar, geom.Radii{}, color.NRGBA{255, 255, 255, 153})
				} else {
					l.Fill(bar, geom.Radii{}, color.NRGBA{255, 255, 255, 245})
				}
				benchFrames(b, d, l, windowSize, scale, false)
			})
		}
	}
}
