package editor

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"image/color"
)

// fixedPaint fingerprints fixed primitives in screen coordinates. The
// cache's pixels (and slider old/new rectangles) never move with rows.
type fixedPaint struct {
	rect geom.Rect
	key  uint64
}

func (w *ViewW) navigationFill(l *paint.List, r geom.Rect, c color.NRGBA) {
	if r.Empty() || c.A == 0 {
		return
	}
	// The order matters for overlapping translucent primitives and owners.
	key := uint64(c.R) | uint64(c.G)<<8 | uint64(c.B)<<16 | uint64(c.A)<<24 | uint64(len(w.vs.navPaint))<<32
	w.vs.navPaint = append(w.vs.navPaint, fixedPaint{r, key})
	l.Fill(r, geom.Radii{}, c)
}
