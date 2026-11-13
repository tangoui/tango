package paint

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// rasterSrc rasterizes a w×h block above the baseline; ok=false for id 0
// (always: one source keeps one path per glyph).
type rasterSrc struct {
	mu    sync.Mutex
	calls int
}

func (s *rasterSrc) GlyphPath(id uint32, size float32) *Path {
	return NewPath().MoveTo(0, 0).LineTo(0, -size).LineTo(size/2, -size).LineTo(size/2, 0).Close()
}

func (s *rasterSrc) RasterGlyph(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	switch id {
	case 0:
		return nil, image.Point{}, false
	case 2: // empty (space)
		return nil, image.Point{}, true
	}
	m := image.NewAlpha(image.Rect(5, 5, 8, 9)) // not at origin: GlyphMask rebases
	for i := range m.Pix {
		m.Pix[i] = 100 + subx
	}
	return m, image.Pt(1, -4), true
}

func TestGlyphRasterizerMaskUsedAsIs(t *testing.T) {
	src := &rasterSrc{}
	c := NewCPU()
	c.TextGamma = 0.5 // must not touch engine masks
	m, off := c.GlyphMask(Glyph{Src: src, ID: 1, Size: 10}, 10, 3)
	// The 3×4 block is inked to its edge: GlyphMask adds the 1px
	// transparent border outline masks have (the GPU atlas relies on it).
	if m == nil || m.Rect != image.Rect(0, 0, 5, 6) || off != image.Pt(0, -5) {
		t.Fatalf("mask %v off %v", m.Rect, off)
	}
	if !clearBorder(m) {
		t.Fatal("no transparent border")
	}
	if m.Pix[m.Stride+1] != 103 {
		t.Fatalf("gamma applied to engine mask: %d", m.Pix[m.Stride+1])
	}
	// Cached: the source is asked once per key.
	c.GlyphMask(Glyph{Src: src, ID: 1, Size: 10}, 10, 3)
	if src.calls != 1 {
		t.Fatalf("calls %d", src.calls)
	}
	if m, _ := c.GlyphMask(Glyph{Src: src, ID: 2, Size: 10}, 10, 0); m != nil {
		t.Fatal("empty glyph")
	}
	// ok=false: the outline is rasterized, with TextGamma.
	m0, _ := c.GlyphMask(Glyph{Src: src, ID: 0, Size: 10}, 10, 0)
	if m0 == nil || m0.Rect.Dx() < 5 {
		t.Fatalf("fallback mask %v", m0)
	}
}

func TestGlyphRasterizerRenderMatchesGlyphMask(t *testing.T) {
	src := &rasterSrc{}
	var l List
	l.Glyphs(geom.Pt(10, 20), []Glyph{{Src: src, ID: 1, Size: 10}}, color.NRGBA{A: 255})
	c := NewCPU()
	if err := c.Render(&l, geom.Size{W: 30, H: 30}, 1); err != nil {
		t.Fatal(err)
	}
	img := c.Image()
	// Pen (10,20) + off (1,-4): a 3×4 block with coverage 100.
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			in := x >= 11 && x < 14 && y >= 16 && y < 20
			a := img.RGBAAt(x, y).A
			if in != (a > 0) {
				t.Fatalf("pixel %d,%d alpha %d in=%v", x, y, a, in)
			}
		}
	}
}

// borderSrc returns a mask that already has a transparent border: it is
// used as is.
type borderSrc struct{ rasterSrc }

func (s *borderSrc) RasterGlyph(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	m := image.NewAlpha(image.Rect(0, 0, 4, 4))
	m.Pix[1*4+1], m.Pix[2*4+2] = 200, 50
	return m, image.Pt(-1, -3), true
}

func TestGlyphRasterizerKeepsBorderedMask(t *testing.T) {
	m, off := NewCPU().GlyphMask(Glyph{Src: &borderSrc{}, ID: 1, Size: 10}, 10, 0)
	if m.Rect != image.Rect(0, 0, 4, 4) || off != image.Pt(-1, -3) || m.Pix[5] != 200 {
		t.Fatalf("mask %v off %v", m.Rect, off)
	}
}
