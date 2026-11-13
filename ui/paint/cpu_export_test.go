package paint_test

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// TestGlyphPlacementMatchesCPU composes a real-font text run by hand from
// GlyphPlacement + GlyphMask and expects CPU.Render's pixels exactly.
func TestGlyphPlacementMatchesCPU(t *testing.T) {
	if len(text.Families()) == 0 {
		t.Skip("no system fonts")
	}
	lay := text.Shape("Tango 你好", text.Style{Size: 15}, text.Options{})
	if len(lay.Glyphs) < 7 {
		t.Fatalf("glyphs = %d", len(lay.Glyphs))
	}
	ink := color.NRGBA{0x20, 0x30, 0xc0, 230}
	for _, scale := range []float32{1, 1.5, 2} {
		origin := geom.Pt(3.3, 2.7)
		l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
		l.Glyphs(origin, lay.Glyphs, ink)
		size := geom.Size{W: lay.Width + 8, H: lay.Height + 6}
		ref := paint.NewCPU()
		if err := ref.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		want := ref.Image()

		got := image.NewRGBA(want.Rect)
		draw.Draw(got, got.Rect, image.NewUniform(l.Background), image.Point{}, draw.Src)
		r := paint.NewCPU()
		drawn := 0
		for _, g := range lay.Glyphs {
			ix, iy, sub := paint.GlyphPlacement(origin, g, scale)
			mask, off := r.GlyphMask(g, g.Size*scale, sub)
			if mask == nil {
				continue
			}
			drawn++
			at := image.Pt(ix+off.X, iy+off.Y)
			dst := image.Rectangle{Min: at, Max: at.Add(mask.Rect.Size())}.Intersect(got.Rect)
			draw.DrawMask(got, dst, image.NewUniform(ink), image.Point{}, mask, dst.Min.Sub(at), draw.Over)
		}
		if drawn < 6 {
			t.Fatalf("scale %v: only %d glyph masks", scale, drawn)
		}
		if n := diffPixels(got, want); n != 0 {
			t.Fatalf("scale %v: %d pixels differ", scale, n)
		}
	}
}

// TestRenderRegion draws a StrokeRect into its bounding box and expects the
// same pixels as the matching region of a full CPU.Render.
func TestRenderRegion(t *testing.T) {
	op := paint.StrokeRect{Rect: geom.R(5.3, 4.6, 40.2, 23.9), Radii: geom.Rad(6), Width: 1.5,
		Color: color.NRGBA{0x16, 0x77, 0xff, 200}}
	for _, scale := range []float32{1, 2} {
		l := &paint.List{}
		l.Add(op)
		ref := paint.NewCPU()
		if err := ref.Render(l, geom.Size{W: 60, H: 40}, scale); err != nil {
			t.Fatal(err)
		}
		s := float64(scale)
		region := image.Rect(int(math.Floor(5.3*s))-1, int(math.Floor(4.6*s))-1,
			int(math.Ceil(45.5*s))+1, int(math.Ceil(28.5*s))+1)
		got := paint.NewCPU().RenderRegion([]paint.Op{op}, region, scale)
		if got.Bounds() != region {
			t.Fatalf("bounds %v, want %v", got.Bounds(), region)
		}
		want := ref.Image().SubImage(region).(*image.RGBA)
		if n := diffPixels(got, want); n != 0 {
			t.Fatalf("scale %v: %d pixels differ", scale, n)
		}
		// Something was actually drawn.
		ink := 0
		for i := 3; i < len(got.Pix); i += 4 {
			if got.Pix[i] != 0 {
				ink++
			}
		}
		if ink < 100 {
			t.Fatalf("scale %v: stroke missing (%d inked pixels)", scale, ink)
		}
	}
}

func diffPixels(a, b *image.RGBA) int {
	if a.Rect != b.Rect {
		return a.Rect.Dx() * a.Rect.Dy()
	}
	n := 0
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				n++
			}
		}
	}
	return n
}
