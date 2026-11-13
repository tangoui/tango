package windows

// Platform-independent part of partial frames (canvas_damage_windows.go).

import "image"

// canvasDamageBlits lists the rectangles WM_PAINT copies from the DIB
// (w×h) for a partially presented frame: the dirty rectangles and the
// update rectangle the system has asked for (pending, empty when none),
// clipped to the DIB, empty ones dropped.
func canvasDamageBlits(dirty []image.Rectangle, pending image.Rectangle, w, h int) []image.Rectangle {
	bounds := image.Rect(0, 0, w, h)
	out := make([]image.Rectangle, 0, len(dirty)+1)
	for _, r := range append(dirty[:len(dirty):len(dirty)], pending) {
		if r = r.Intersect(bounds); !r.Empty() {
			out = append(out, r)
		}
	}
	return out
}
