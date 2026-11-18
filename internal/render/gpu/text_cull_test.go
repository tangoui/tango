package gpu

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// TestPartialFrameSkipsTextOutsideDirty: in a partial frame a text op
// whose glyphs all lie far from the dirty region is dropped as a whole,
// without placing its glyphs one by one; the frame still matches a full
// one pixel for pixel.
func TestPartialFrameSkipsTextOutsideDirty(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			if err := r.Render(listRows(t, 0, nil), listSize, scale); err != nil {
				t.Fatal(err)
			}
			r.SetVerify(false)
			l := listRows(t, 0, nil)
			row := geom.R(0, listViewport.Y+3*20, listSize.W, 20) // row 3
			l.Damage(row)
			r.glyphsPlaced = 0
			if err := r.Render(l, listSize, scale); err != nil {
				t.Fatal(err)
			}
			if len(dev.last.Dirty) == 0 {
				t.Fatal("not a partial frame")
			}
			// Only rows within a glyph's reach of row 3 (a few above and
			// below) are placed glyph by glyph, not all 20 rows, header and
			// footer.
			if all := r.glyphsPlacedFull(t, l, scale); r.glyphsPlaced*3 > all {
				t.Fatalf("placed %d of %d glyphs for one dirty row", r.glyphsPlaced, all)
			}
			mustSame(t, "partial", dev.img, fullRender(t, l, listSize, scale))
		})
	}
}

// glyphsPlacedFull returns how many glyphs a full frame of l places.
func (r *Renderer) glyphsPlacedFull(t *testing.T, l *paint.List, scale float32) int {
	t.Helper()
	fr := NewRenderer(&cpuDevice{})
	if err := fr.Render(l, listSize, scale); err != nil {
		t.Fatal(err)
	}
	return fr.glyphsPlaced
}

// TestPartialFrameTextCullUnderTransform: text drawn under a translate and
// scale is culled by its transformed positions: the rows next to a dirty
// row inside the transform still draw, matching a full frame.
func TestPartialFrameTextCullUnderTransform(t *testing.T) {
	m := geom.Translate(30, 40).Mul(geom.Scale(1.5, 1.5))
	mk := func(c color.NRGBA) *paint.List {
		l := &paint.List{Background: white}
		l.Transform(m)
		for i := range 12 {
			y := float32(i) * 20
			if i == 3 || i == 10 {
				l.Fill(geom.R(0, y, 200, 20), geom.Radii{}, c)
			}
			l.Glyphs(geom.Pt(8, y+15), shape(t, fmt.Sprintf("第 %d 行 row", i), 12), ink)
		}
		l.Pop()
		return l
	}
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			if err := r.Render(mk(blue), listSize, scale); err != nil {
				t.Fatal(err)
			}
			r.SetVerify(false)
			l := mk(red)
			// The damage is in the transform's coordinates (inside it): two
			// rows far apart, so the frame has two dirty rectangles and
			// text is culled by them, not only by the scissor.
			l.Ops = append(l.Ops[:len(l.Ops)-1], paint.Damage{Rect: geom.R(0, 60, 200, 20)}, paint.Damage{Rect: geom.R(0, 200, 200, 20)}, paint.Pop{})
			if err := r.Render(l, listSize, scale); err != nil {
				t.Fatal(err)
			}
			if len(dev.last.Dirty) < 2 {
				t.Fatalf("dirty %v, want two rectangles", dev.last.Dirty)
			}
			mustSame(t, "transformed partial", dev.img, fullRender(t, l, listSize, scale))
		})
	}
}
