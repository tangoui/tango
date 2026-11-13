package darwin

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func pngOf(w, h int) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h)))
	return buf.Bytes()
}

func TestIconPoints(t *testing.T) {
	for _, c := range []struct {
		w, h   int
		target float64
		ww, wh float64
	}{
		{32, 32, 16, 16, 16},
		{44, 36, 18, 22, 18},
		{64, 16, 16, 64, 16},
	} {
		w, h, ok := iconPoints(pngOf(c.w, c.h), c.target)
		if !ok || w != c.ww || h != c.wh {
			t.Errorf("%dx%d@%v: %v %v %v", c.w, c.h, c.target, w, h, ok)
		}
	}
	if _, _, ok := iconPoints([]byte("GIF89a"), 16); ok {
		t.Error("non-PNG accepted")
	}
	if _, _, ok := iconPoints(nil, 16); ok {
		t.Error("empty accepted")
	}
}
