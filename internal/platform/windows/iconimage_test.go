package windows

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func px(pix []byte, w, x, y int) [4]byte {
	i := (y*w + x) * 4
	return [4]byte{pix[i], pix[i+1], pix[i+2], pix[i+3]}
}

func TestIconPixelsDownscaleAverages(t *testing.T) {
	// 4×4: left half opaque red, right half fully transparent.
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	data := encodePNG(t, img)

	pre, err := iconPixels(data, 2, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := px(pre, 2, 0, 0); got != [4]byte{0, 0, 255, 255} {
		t.Errorf("left = %v, want opaque red BGRA", got)
	}
	if got := px(pre, 2, 1, 1); got != [4]byte{0, 0, 0, 0} {
		t.Errorf("right = %v, want transparent", got)
	}

	// 1×1: half covered. Premultiplied red equals alpha; straight is 255.
	pre, _ = iconPixels(data, 1, 1, true)
	if got := px(pre, 1, 0, 0); got[2] != got[3] || got[3] < 126 || got[3] > 128 {
		t.Errorf("premultiplied = %v", got)
	}
	str, _ := iconPixels(data, 1, 1, false)
	if got := px(str, 1, 0, 0); got[2] != 255 || got[3] < 126 || got[3] > 128 {
		t.Errorf("straight = %v", got)
	}
}

func TestIconPixelsFitsAndCenters(t *testing.T) {
	// 4×2 opaque blue into 4×4: rows 1 and 2 filled, 0 and 3 transparent.
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
		}
	}
	pix, err := iconPixels(encodePNG(t, img), 4, 4, true)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		want := byte(0)
		if y == 1 || y == 2 {
			want = 255
		}
		if a := px(pix, 4, 0, y)[3]; a != want {
			t.Errorf("row %d alpha = %d, want %d", y, a, want)
		}
	}
	if got := px(pix, 4, 3, 1); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("blue = %v", got)
	}
}

func TestIconPixelsUpscaleNearest(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
	pix, err := iconPixels(encodePNG(t, img), 4, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			want := [4]byte{0, 255, 0, 255}
			if x >= 2 {
				want = [4]byte{0, 0, 255, 255}
			}
			if got := px(pix, 4, x, y); got != want {
				t.Errorf("(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestIconPixelsErrors(t *testing.T) {
	if _, err := iconPixels([]byte("not a png"), 16, 16, true); err == nil {
		t.Error("bad PNG accepted")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if _, err := iconPixels(encodePNG(t, img), 0, 16, true); err == nil {
		t.Error("zero size accepted")
	}
}

func TestIconMask(t *testing.T) {
	// 17 pixels wide: rows pad to 4 bytes (two 16-bit words).
	w, h := 17, 2
	pix := make([]byte, w*h*4)
	pix[(0*w+0)*4+3] = 255  // (0,0) opaque
	pix[(1*w+16)*4+3] = 255 // (16,1) opaque
	m := iconMask(pix, w, h)
	if len(m) != 8 {
		t.Fatalf("len = %d, want 8", len(m))
	}
	want := []byte{0x7F, 0xFF, 0x80, 0x00, 0xFF, 0xFF, 0x00, 0x00}
	if !bytes.Equal(m, want) {
		t.Errorf("mask = % X, want % X", m, want)
	}
}
