package d3d11

import (
	"image"
	"image/color"
	"math"
)

// presentDirtyRects returns the DXGI_PRESENT_PARAMETERS dirty rectangles
// for presenting the device's last frame on a swap chain of the given
// swap effect, or nil to present without the hint.
//
// Only DXGI_SWAP_EFFECT_FLIP_SEQUENTIAL takes them: FLIP_DISCARD (the
// swap chain on Windows 10 and later) "cannot be used with multisampling
// and partial presentation", and Present1 with dirty rectangles may then
// fail with DXGI_ERROR_INVALID_CALL. On FLIP_DISCARD every present is a
// full one, which is what it shows anyway.
//
// The hint tells the compositor which pixels changed since the last
// presented frame; it never changes what is shown here, because present
// always redraws the whole back buffer from the offscreen target. It is
// only correct, so only passed, when the frame presented last is exactly
// the frame before this one (seq is presented+1; presented is 0 after an
// occluded present, whose frame was not shown), this frame was partial
// (changed holds its Dirty rectangles and its Copies' destinations), the
// back buffer was not resized, it has the target's size (a larger buffer
// repeats the target's last row and column, which a changed edge pixel
// would change outside the rectangles) and every rectangle is inside it.
func presentDirtyRects(effect uint32, seq, presented int, full, resized bool, changed []image.Rectangle, target, buf image.Point) []rect {
	if effect != swapEffectFlipSequential {
		return nil
	}
	if presented <= 0 || seq != presented+1 || full || resized || target != buf || len(changed) == 0 {
		return nil
	}
	bounds := image.Rectangle{Max: buf}
	out := make([]rect, 0, len(changed))
	for _, r := range changed {
		if r.Empty() || !r.In(bounds) {
			return nil
		}
		out = append(out, rect{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)})
	}
	return out
}

// presentSize is the back buffer size for a present of size: clamped to
// the device's texture limit.
func presentSize(size image.Point, limit int) image.Point {
	return image.Pt(min(size.X, limit), min(size.Y, limit))
}

// backgroundRGBA is the swap chain background color (DXGI_RGBA) for a
// canvas cleared to c: the opaque color the window shows for c (its
// premultiplied bytes, as over black, since the swap chain ignores alpha),
// converted from sRGB to the linear values SetBackgroundColor expects.
func backgroundRGBA(c color.NRGBA) dxgiRGBA {
	cc := clearColor(c) // premultiplied, sRGB-encoded bytes / 255
	return dxgiRGBA{R: srgbToLinear(cc[0]), G: srgbToLinear(cc[1]), B: srgbToLinear(cc[2]), A: 1}
}

// srgbToLinear decodes one sRGB-encoded channel in [0, 1] (IEC 61966-2-1).
func srgbToLinear(v float32) float32 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow((float64(v)+0.055)/1.055, 2.4))
}
