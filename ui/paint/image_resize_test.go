package paint

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func rampImage(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetRGBA(x, y, color.RGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), 0, 255})
		}
	}
	return m
}

// Moved from ui (the image widget's former private copies).
func TestResizeBilinearIdentityAndReduction(t *testing.T) {
	g := rampImage(16, 8)
	if id := ResizeBilinear(g, 16, 8); !bytes.Equal(id.Pix, g.Pix) {
		t.Fatal("identity resize changed pixels")
	}
	sm := ResizeBilinear(rampImage(256, 256), 10, 10)
	prev := -1
	for x := 0; x < 10; x++ {
		v := int(sm.RGBAAt(x, 5).R)
		if v < prev {
			t.Fatalf("not monotonic at %d", x)
		}
		prev = v
	}
	if r := sm.RGBAAt(0, 0).R; r > 30 {
		t.Fatalf("left edge too bright: %d", r)
	}
	if r := sm.RGBAAt(9, 0).R; r < 225 {
		t.Fatalf("right edge too dark: %d", r)
	}
}

func TestOrientBoundsAndFlip(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	if r := Orient(src, 1, false, false); r.Bounds().Dx() != 2 || r.Bounds().Dy() != 3 || r.RGBAAt(1, 0).R != 255 {
		t.Fatalf("rotate 90: %v", r.Bounds())
	}
	if r := Orient(src, -1, false, false); r.RGBAAt(0, 2).R != 255 {
		t.Fatal("rotate -90 misplaced pixel")
	}
	if f := Orient(src, 0, true, false); f.RGBAAt(2, 0).R != 255 {
		t.Fatal("flipX misplaced pixel")
	}
	if f := Orient(src, 0, false, true); f.RGBAAt(0, 1).R != 255 {
		t.Fatal("flipY misplaced pixel")
	}
}
