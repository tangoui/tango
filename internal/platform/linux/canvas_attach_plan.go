package linux

// Platform-independent parts of canvases attached to a WebView surface
// (canvas_attach_linux.go): layer placement and the pass-through hit
// mask turned into rectangles for an input shape (cairo region). Kept free of build
// tags so they are tested on every OS.

import (
	"math"

	"github.com/tangoui/tango/internal/platform"
)

// attachBounds is where the canvas of layer l sits, in logical pixels, in
// a content area of curW×curH when l was set for a content area of
// baseW×baseH: zero Bounds fill the content area; non-zero Bounds follow
// l.Anchor.
func attachBounds(l platform.CanvasLayer, baseW, baseH, curW, curH int) platform.Rect {
	if l.Bounds.Width <= 0 || l.Bounds.Height <= 0 {
		return platform.Rect{Width: max(curW, 0), Height: max(curH, 0)}
	}
	return l.AnchoredBounds(baseW, baseH, curW, curH)
}

// attachDevice converts a logical rectangle to device pixels at scale.
// Both edges are rounded, so canvases sharing an edge in logical pixels
// share it in device pixels too; the size is at least 1×1.
func attachDevice(r platform.Rect, scale float64) (x, y, w, h int32) {
	if scale <= 0 {
		scale = 1
	}
	rd := func(v int) int32 { return int32(math.Round(float64(v) * scale)) }
	x, y = rd(r.X), rd(r.Y)
	w, h = rd(r.X+r.Width)-x, rd(r.Y+r.Height)-y
	return x, y, max(w, 1), max(h, 1)
}

// attachHitMask writes into mask (len w*h) 1 for pixels of the
// premultiplied RGBA frame pix (stride bytes per row) whose alpha is at
// least thr, else 0, and reports whether mask changed.
func attachHitMask(mask, pix []byte, stride, w, h int, thr uint8) bool {
	changed := false
	for y := range h {
		row := pix[y*stride:]
		m := mask[y*w : (y+1)*w]
		for x := range m {
			v := byte(0)
			if row[x*4+3] >= thr {
				v = 1
			}
			if m[x] != v {
				m[x] = v
				changed = true
			}
		}
	}
	return changed
}

// attachRect is a rectangle in device pixels: x, y, width, height.
type attachRect [4]int32

// attachMaskRects covers the 1 pixels of a w×h mask with rectangles:
// horizontal runs per row, rows with the same runs merged into one band.
// The result is y-x banded (sorted by y, then x), as cairo regions like it.
func attachMaskRects(mask []byte, w, h int) []attachRect {
	var out []attachRect
	var prev, cur [][2]int32 // runs [x0, x1) of the previous and current row
	open := 0                // index in out of the first rectangle of the open band
	for y := 0; y < h; y++ {
		cur = cur[:0]
		row := mask[y*w : (y+1)*w]
		for x := 0; x < w; {
			if row[x] == 0 {
				x++
				continue
			}
			x0 := x
			for x < w && row[x] != 0 {
				x++
			}
			cur = append(cur, [2]int32{int32(x0), int32(x)})
		}
		if y > 0 && sameRuns(prev, cur) {
			for i := open; i < len(out); i++ {
				out[i][3]++
			}
			continue
		}
		open = len(out)
		for _, r := range cur {
			out = append(out, attachRect{r[0], int32(y), r[1] - r[0], 1})
		}
		prev, cur = cur, prev
	}
	return out
}

func sameRuns(a, b [][2]int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
