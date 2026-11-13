package paint

import "image"

// GlyphRasterizer is an optional interface of a GlyphSource backed by a
// platform text engine (CoreText, DirectWrite, FreeType) that rasterizes
// glyphs itself, with the platform's hinting and filtering, instead of
// letting paint fill the GlyphPath outline.
//
// When the Src of a glyph implements it, CPU.GlyphMask (and so Render and
// the GPU glyph atlas, which takes its masks from GlyphMask) uses the
// RasterGlyph result; ok=false falls back to rasterizing GlyphPath.
//
// Contract:
//
//   - size is the device pixel size, subx the quarter-pixel x bucket
//     (0..3, see GlyphPlacement) to add to the pen, skew the synthetic
//     italic slant of Glyph.Skew (x += skew * height above the baseline).
//   - mask is the final 8-bit coverage (R8): the engine applies its own
//     gamma / contrast, so GlyphMask does NOT apply CPU.TextGamma to it.
//     off is the mask's top-left relative to the device pen position
//     (x floored, baseline y rounded, as GlyphPlacement places it). A nil
//     mask with ok=true is an empty glyph (space). Like outline masks, the
//     mask keeps a 1px transparent border, the convention outline masks
//     and the GPU atlas share (the atlas samples nearest, clamps the
//     sample point to within half a texel of the entry's edge and keeps
//     a 1-texel gap between entries); GlyphMask adds the border, moving
//     off by -1,-1, when the source returns a mask inked to its edge.
//   - The mask is cached and shared by the caller (CPU glyph cache, GPU
//     atlas): the source must not modify it afterwards.
//   - The result must be a deterministic function of the arguments for a
//     given source. The GPU atlas keys masks by (Src, ID, Size, Sub, Skew)
//     whichever path produced them, and the CPU cache may evict and
//     rebuild a mask while the atlas still holds the old one, so one
//     source must always take the same path for the same key: ok=false
//     only for a glyph the engine really cannot rasterize, and then for
//     that key every time. A source never switches engines at run time;
//     switching engines means new GlyphSource values (a different Src).
//   - It must be safe for concurrent use.
type GlyphRasterizer interface {
	RasterGlyph(id uint32, size float32, subx uint8, skew float32) (mask *image.Alpha, off image.Point, ok bool)
}

// rasterGlyph asks a GlyphRasterizer source for the mask of g; ok=false
// means the outline path applies. The mask is rebased to origin 0,0, as
// the cached masks are.
func rasterGlyph(g Glyph, size float32, sub uint8) (m *image.Alpha, off image.Point, ok bool) {
	rg, is := g.Src.(GlyphRasterizer)
	if !is {
		return nil, image.Point{}, false
	}
	m, off, ok = rg.RasterGlyph(g.ID, size, sub, g.Skew)
	if !ok {
		return nil, image.Point{}, false
	}
	if m == nil || m.Rect.Empty() {
		return nil, image.Point{}, true
	}
	if m.Rect.Min != (image.Point{}) {
		m = &image.Alpha{Pix: m.Pix, Stride: m.Stride, Rect: m.Rect.Sub(m.Rect.Min)}
	}
	if !clearBorder(m) {
		m, off = padMask(m), off.Sub(image.Pt(1, 1))
	}
	return m, off, true
}

// clearBorder reports whether the outermost rows and columns of m are all
// zero: outline masks keep a 1px transparent border, which the GPU atlas
// relies on (bilinear samples at an entry's edge must not pick up its
// neighbour).
func clearBorder(m *image.Alpha) bool {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	for x := 0; x < w; x++ {
		if m.Pix[x] != 0 || m.Pix[(h-1)*m.Stride+x] != 0 {
			return false
		}
	}
	for y := 0; y < h; y++ {
		if m.Pix[y*m.Stride] != 0 || m.Pix[y*m.Stride+w-1] != 0 {
			return false
		}
	}
	return true
}

// padMask returns m with a 1px transparent border added.
func padMask(m *image.Alpha) *image.Alpha {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	out := image.NewAlpha(image.Rect(0, 0, w+2, h+2))
	for y := 0; y < h; y++ {
		copy(out.Pix[(y+1)*out.Stride+1:], m.Pix[y*m.Stride:y*m.Stride+w])
	}
	return out
}
