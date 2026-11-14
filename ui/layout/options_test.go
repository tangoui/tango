package layout

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestOptions(t *testing.T) {
	var s style
	for _, o := range []Option{{}, Grow(2), W(10), H(20), PadVH(1, 2), Gap(3), Middle(), Physical(), ScrollY(), NoStretch()} {
		if o.apply != nil {
			o.apply(&s)
		}
	}
	if s.Grow != 2 || s.Width != 10 || s.Height != 20 || s.Padding != geom.VH(1, 2) || s.Gap != 3 || s.LineGap != 3 ||
		s.Align != AlignCenter || !s.NoMirror || !s.ScrollY || s.ScrollX || !s.NoStretch {
		t.Fatalf("%+v", s)
	}
	Stretch().apply(&s)
	if s.NoStretch || s.AlignSelf != AlignStretch {
		t.Fatalf("Stretch: %+v", s)
	}
	Abs(1, Unset, 3, Unset).apply(&s)
	if !s.Absolute || s.Pos.Left != 1 || !math.IsNaN(float64(s.Pos.Top)) || s.Pos.Right != 3 {
		t.Fatalf("Abs: %+v", s.Pos)
	}
}
