package android

// Pure logic of canvases attached to WebView pages (canvas_attach_android.go,
// docs/specs/mobile-canvas.md §1.4): the pass-through hit mask and the
// anchored placement of fixed bounds. Kept free of build tags so it is unit
// tested on every OS.

import (
	"image"
	"math"

	"github.com/tangoui/tango/internal/platform"
)

// hitMask keeps the alpha channel of the last frame an overlay presented,
// so a touch can be tested against it (CanvasView.dispatchTouchEvent asks
// on ACTION_DOWN). Coordinates are device pixels of the frame, which match
// the view's pixels.
type hitMask struct {
	a    []byte
	w, h int
}

// reset forgets the mask: every point misses until the next update.
func (m *hitMask) reset() {
	m.a, m.w, m.h = m.a[:0], 0, 0
}

// update takes the alpha of img inside r (in img's coordinate space, as
// img.Bounds()). When the mask has another size, or r is empty or nil, the
// whole image is read.
func (m *hitMask) update(img *image.RGBA, r *image.Rectangle) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		m.reset()
		return
	}
	area := b
	if r == nil || w != m.w || h != m.h || r.Intersect(b).Empty() {
		if cap(m.a) < w*h {
			m.a = make([]byte, w*h)
		}
		m.a = m.a[:w*h]
		m.w, m.h = w, h
	} else {
		area = r.Intersect(b)
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		row := img.Pix[img.PixOffset(area.Min.X, y):]
		dst := m.a[(y-b.Min.Y)*w+(area.Min.X-b.Min.X):]
		for x := range area.Dx() {
			dst[x] = row[x*4+3]
		}
	}
}

// presented records a presentation of img over the dirty rectangle r (nil:
// the whole frame) that returned err. The mask follows only frames that
// reached the screen: after a failed presentation what is on screen is
// unknown, so the mask is dropped (every point misses, so a gesture goes
// to the page below) until a frame succeeds. A partial update of a mask
// of another size, or of none, cannot be trusted either: then the mask
// waits for a full frame (Present after a resize is full: needFull).
func (m *hitMask) presented(img *image.RGBA, r *image.Rectangle, err error) {
	if err != nil || img == nil {
		m.reset()
		return
	}
	if r != nil {
		b := img.Bounds()
		if m.w != b.Dx() || m.h != b.Dy() {
			m.reset()
			return
		}
	}
	m.update(img, r)
}

// hit reports whether the frame's pixel at (x, y) has an alpha of at least
// threshold. Points outside the frame, or without a frame, miss.
func (m *hitMask) hit(x, y float32, threshold uint8) bool {
	if m.w == 0 || !(x >= 0) || !(y >= 0) {
		return false
	}
	ix, iy := int(x), int(y)
	if ix >= m.w || iy >= m.h {
		return false
	}
	return m.a[iy*m.w+ix] >= threshold
}

// overlayRect is where an overlay with fixed Bounds (logical pixels) sits
// in a container of w×h device pixels: the bounds converted at density,
// moved by CanvasLayer.AnchoredBounds from the container size refW×refH
// they were given for. A zero ref size means the bounds are given for the
// current size. ok is false for zero Bounds (the overlay fills the
// container).
func overlayRect(l platform.CanvasLayer, density float32, refW, refH, w, h int) (r image.Rectangle, ok bool) {
	if l.Bounds.Width <= 0 || l.Bounds.Height <= 0 {
		return image.Rectangle{}, false
	}
	if refW <= 0 || refH <= 0 {
		refW, refH = w, h
	}
	px := l
	px.Bounds = platform.Rect{
		X: dpToPx(l.Bounds.X, density), Y: dpToPx(l.Bounds.Y, density),
		Width: dpToPx(l.Bounds.Width, density), Height: dpToPx(l.Bounds.Height, density),
	}
	a := px.AnchoredBounds(refW, refH, w, h)
	return image.Rect(a.X, a.Y, a.X+a.Width, a.Y+a.Height), true
}

// dpToPx converts logical to device pixels as the Java host does
// (Math.round(dp * density)).
func dpToPx(v int, density float32) int {
	if !(density > 0) {
		density = 1
	}
	return int(math.Floor(float64(float32(v)*density) + 0.5))
}

// packPoint packs a view position for the Java host (TangoCanvas.nativePlace):
// x in the high 32 bits, y in the low ones.
func packPoint(p image.Point) int64 {
	return int64(p.X)<<32 | int64(uint32(int32(p.Y)))
}
