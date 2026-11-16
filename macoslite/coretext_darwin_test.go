//go:build darwin

package macoslite

import (
	"bytes"
	"image"
	"testing"
)

func textCanvas(t *testing.T, fc *fontCache) (*Canvas, *image.RGBA) {
	t.Helper()
	if err := loadNative(); err != nil {
		t.Skip("CoreText unavailable:", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 200, 60))
	c := newCanvas(img, 2)
	c.fonts = fc
	return c, img
}

func inkPixels(img *image.RGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			n++
		}
	}
	return n
}

func TestCoreTextLatinDrawsInk(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c, img := textCanvas(t, &fc)
	c.Text("Reset 12.34", Point{4, 20}, 13, Color{A: 255})
	if inkPixels(img) < 50 {
		t.Fatalf("latin text drew %d pixels", inkPixels(img))
	}
	// Ink sits above the baseline (y=20pt → 40px), not at the bottom edge.
	for x := 0; x < 200; x++ {
		if img.RGBAAt(x, 59).A != 0 {
			t.Fatal("text drawn upside down or at the wrong baseline")
		}
	}
}

func TestCoreTextCJKFallback(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c, img := textCanvas(t, &fc)
	c.Text("重置", Point{4, 20}, 13, Color{A: 255})
	if inkPixels(img) < 50 {
		t.Fatalf("CJK text drew %d pixels", inkPixels(img))
	}
}

func TestCoreTextColor(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c, img := textCanvas(t, &fc)
	c.Text("HHHH", Point{4, 20}, 20, Color{R: 255, A: 255})
	var best image.Point
	var bestA uint8
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			if a := img.RGBAAt(x, y).A; a > bestA {
				bestA, best = a, image.Pt(x, y)
			}
		}
	}
	p := img.RGBAAt(best.X, best.Y)
	if p.A < 200 || p.R < p.A-2 || p.G > 2 || p.B > 2 {
		t.Fatalf("opaque glyph pixel = %+v, want premultiplied red", p)
	}
}

func TestCoreTextEmptyAndTransparent(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c, img := textCanvas(t, &fc)
	c.Text("", Point{4, 20}, 13, Color{A: 255})
	c.Text("abc", Point{4, 20}, 13, Color{})
	c.Text("abc", Point{4, 20}, 0, Color{A: 255})
	c.Text("abc", Point{4, 20}, -3, Color{A: 255})
	if inkPixels(img) != 0 {
		t.Fatal("empty, transparent or non-positive-size text must draw nothing")
	}
}

func TestCoreTextInvalidUTF8(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c, _ := textCanvas(t, &fc)
	c.Text("a\xffb", Point{4, 20}, 13, Color{A: 255}) // must not panic
}

func TestCoreTextDeterministic(t *testing.T) {
	var fc fontCache
	defer fc.clear()
	c1, img1 := textCanvas(t, &fc)
	c1.Text("Lite 42 重置", Point{4, 20}, 13, Color{A: 255})
	c2, img2 := textCanvas(t, &fc)
	c2.Text("Lite 42 重置", Point{4, 20}, 13, Color{A: 255})
	if !bytes.Equal(img1.Pix, img2.Pix) {
		t.Fatal("repeated draws differ")
	}
}

func TestFontCacheLRUAndTrim(t *testing.T) {
	if err := loadNative(); err != nil {
		t.Skip(err)
	}
	var released []uintptr
	fc := fontCache{release: func(h uintptr) { released = append(released, h); cfRelease(h) }}
	first := fc.get(10)
	if first == 0 {
		t.Fatal("no font")
	}
	for s := 11.0; s <= 18; s++ {
		fc.get(s)
	}
	if fc.len() != fontCacheSize || len(released) != 1 || released[0] != first {
		t.Fatalf("len=%d released=%v first=%#x", fc.len(), released, first)
	}
	if fc.get(18) == 0 || len(released) != 1 {
		t.Fatal("cache hit must not evict")
	}
	fc.clear()
	if fc.len() != 0 || len(released) != 1+fontCacheSize {
		t.Fatalf("clear released %d", len(released))
	}
	if fc.get(13) == 0 {
		t.Fatal("draw after trim must still get a font")
	}
	fc.clear()
}
