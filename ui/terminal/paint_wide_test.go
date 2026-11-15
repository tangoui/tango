package terminal

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/terminal/vt"
)

// Text after a wide character, in the style of the text before it, is
// drawn at its own cells: a segment broken by a wide (or combining)
// character starts again after it.
func TestViewTextAfterWideCharStartsAtItsCell(t *testing.T) {
	for _, tc := range []struct {
		name, out string
		col       int // where "cd" starts
	}{
		{"wide", "ab你cd", 4},
		{"emoji", "ab😀cd", 4},
		{"combining", "abécd", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newViewRig(t, 400, 200, nil)
			g.output(tc.out, func(v *vt.Terminal) bool { x, _, _ := v.Cursor(); return x == tc.col+2 })
			g.d.Frame()
			cw, lh := cellSize()
			r := g.vw.Box().Rect()
			var starts []float64
			for _, op := range g.d.Runner().List().Ops {
				if tx, ok := op.(paint.Text); ok && tx.Origin.Y < r.Y+lh-0.5 && len(tx.Glyphs) == 2 {
					starts = append(starts, float64((tx.Origin.X-r.X)/cw))
				}
			}
			// The two-glyph runs of row 0 are "ab" and "cd".
			if len(starts) != 2 || math.Abs(starts[0]) > 0.01 || math.Abs(starts[1]-float64(tc.col)) > 0.01 {
				t.Fatalf("two-cell runs start at columns %v, want [0 %d]", starts, tc.col)
			}
		})
	}
}
