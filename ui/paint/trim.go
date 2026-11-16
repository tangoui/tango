package paint

import "image"

// Trim releases rebuildable glyph, transformed-image and shadow caches. The
// currently rendered image, scale and text gamma are preserved, so the window
// can continue presenting it until the next frame. Call between frames on the
// rendering goroutine. Masks are recomputed with identical pixels on demand.
func (r *CPU) Trim() {
	r.glyphs = map[glyphKey]*glyphMask{}
	r.imgs = map[imgCacheKey]*imgCacheEntry{}
	r.shadows = map[shadowKey]*image.Alpha{}
}
