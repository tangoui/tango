package android

import (
	"errors"
	"image"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func maskImage(w, h int, alpha func(x, y int) uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Pix[img.PixOffset(x, y)+3] = alpha(x, y)
		}
	}
	return img
}

func TestHitMask(t *testing.T) {
	var m hitMask
	if m.hit(0, 0, 1) {
		t.Fatal("empty mask hits")
	}
	// Left half opaque, right half a soft 20.
	img := maskImage(10, 4, func(x, _ int) uint8 {
		if x < 5 {
			return 255
		}
		return 20
	})
	m.update(img, nil)
	for _, c := range []struct {
		x, y float32
		th   uint8
		want bool
	}{
		{0, 0, 1, true},
		{4.9, 3.9, 1, true},
		{5, 0, 1, true},
		{5, 0, 32, false},
		{9.5, 2, 20, true},
		{10, 0, 1, false},
		{0, 4, 1, false},
		{-0.5, 0, 1, false},
	} {
		if got := m.hit(c.x, c.y, c.th); got != c.want {
			t.Errorf("hit(%v, %v, %d) = %v, want %v", c.x, c.y, c.th, got, c.want)
		}
	}

	// A partial update changes only the region.
	none := maskImage(10, 4, func(int, int) uint8 { return 0 })
	r := image.Rect(0, 0, 2, 2)
	m.update(none, &r)
	if m.hit(1, 1, 1) || !m.hit(3, 1, 1) || !m.hit(1, 3, 1) {
		t.Error("partial update touched the wrong pixels")
	}

	// Another size reads the whole image even with a region.
	big := maskImage(20, 2, func(x, _ int) uint8 { return uint8(x) })
	m.update(big, &r)
	if !m.hit(19, 1, 19) || m.hit(18, 1, 19) || m.hit(0, 2, 0) {
		t.Error("resize did not take the whole frame")
	}

	// Sub-images: offsets of the image bounds are honoured.
	sub := maskImage(6, 6, func(x, y int) uint8 { return uint8(10*y + x) }).SubImage(image.Rect(2, 2, 6, 6)).(*image.RGBA)
	m.update(sub, nil)
	if !m.hit(0, 0, 22) || m.hit(0, 0, 23) || !m.hit(3, 3, 55) {
		t.Error("sub-image offsets")
	}

	m.reset()
	if m.hit(0, 0, 0) {
		t.Error("reset mask hits")
	}
}

func TestOverlayRectFill(t *testing.T) {
	if _, ok := overlayRect(platform.CanvasLayer{}, 2, 100, 100, 200, 200); ok {
		t.Fatal("zero bounds placed")
	}
}

func TestOverlayRectAnchors(t *testing.T) {
	b := platform.Rect{X: 10, Y: 20, Width: 30, Height: 40}
	const d = 2
	// Bounds given for 200×300 px, container now 260×400 px (dx 60, dy 100).
	for _, c := range []struct {
		a    platform.CanvasAnchor
		want image.Rectangle
	}{
		{platform.AnchorTopLeft, image.Rect(20, 40, 80, 120)},
		{platform.AnchorTopRight, image.Rect(80, 40, 140, 120)},
		{platform.AnchorBottomLeft, image.Rect(20, 140, 80, 220)},
		{platform.AnchorBottomRight, image.Rect(80, 140, 140, 220)},
		{platform.AnchorCenter, image.Rect(50, 90, 110, 170)},
	} {
		l := platform.CanvasLayer{Bounds: b, Anchor: c.a}
		got, ok := overlayRect(l, d, 200, 300, 260, 400)
		if !ok || got != c.want {
			t.Errorf("anchor %d: %v, want %v", c.a, got, c.want)
		}
		// Unknown ref size: the bounds are for the current size.
		got, _ = overlayRect(l, d, 0, 0, 260, 400)
		if want := image.Rect(20, 40, 80, 120); got != want {
			t.Errorf("anchor %d without ref: %v, want %v", c.a, got, want)
		}
	}
}

func TestOverlayRectMatchesAnchoredBounds(t *testing.T) {
	// At density 1 the result is AnchoredBounds itself, for any sizes.
	for a := platform.AnchorTopLeft; a <= platform.AnchorCenter; a++ {
		l := platform.CanvasLayer{Bounds: platform.Rect{X: 7, Y: 9, Width: 31, Height: 17}, Anchor: a}
		for _, sz := range [][4]int{{100, 100, 101, 99}, {320, 640, 640, 320}, {50, 50, 49, 51}} {
			got, _ := overlayRect(l, 1, sz[0], sz[1], sz[2], sz[3])
			ab := l.AnchoredBounds(sz[0], sz[1], sz[2], sz[3])
			if want := image.Rect(ab.X, ab.Y, ab.X+ab.Width, ab.Y+ab.Height); got != want {
				t.Errorf("anchor %d %v: %v, want %v", a, sz, got, want)
			}
		}
	}
}

func TestDpToPx(t *testing.T) {
	for _, c := range []struct {
		v    int
		d    float32
		want int
	}{{10, 2.625, 26}, {4, 2.625, 11}, {0, 3, 0}, {-4, 2.625, -10}, {5, 0, 5}} {
		if got := dpToPx(c.v, c.d); got != c.want {
			t.Errorf("dpToPx(%d, %v) = %d, want %d", c.v, c.d, got, c.want)
		}
	}
}

func TestPackPoint(t *testing.T) {
	for _, p := range []image.Point{{0, 0}, {12, 34}, {-5, 7}, {9, -3}, {-1, -1}} {
		v := packPoint(p)
		if x, y := int(int32(v>>32)), int(int32(uint32(v))); x != p.X || y != p.Y {
			t.Errorf("packPoint(%v) unpacks to (%d, %d)", p, x, y)
		}
	}
}

// TestHitMaskPresented: the mask follows only frames that reached the
// screen. A failed presentation drops it (gestures pass through until a
// frame is shown), and a partial update of a mask of another size, or of
// none, waits for a full frame instead of mixing two frames.
func TestHitMaskPresented(t *testing.T) {
	opaque := maskImage(4, 4, func(int, int) uint8 { return 255 })
	var m hitMask
	m.presented(opaque, nil, nil)
	if !m.hit(1, 1, 1) {
		t.Fatal("presented frame does not hit")
	}
	m.presented(opaque, nil, errors.New("present failed"))
	if m.hit(1, 1, 1) {
		t.Error("failed presentation kept the stale mask")
	}
	r := image.Rect(0, 0, 2, 2)
	m.presented(opaque, &r, nil)
	if m.hit(1, 1, 1) {
		t.Error("partial frame built a mask from nothing")
	}
	m.presented(opaque, nil, nil)
	big := maskImage(8, 8, func(int, int) uint8 { return 255 })
	m.presented(big, &r, nil)
	if m.hit(1, 1, 1) || m.hit(6, 6, 1) {
		t.Error("partial frame of another size kept or mixed the old mask")
	}
	m.presented(big, nil, nil)
	if !m.hit(6, 6, 1) {
		t.Error("full frame after a resize does not hit")
	}
	m.presented(nil, nil, nil)
	if m.hit(6, 6, 1) {
		t.Error("nil image kept the mask")
	}
}
