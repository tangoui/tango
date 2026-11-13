package paint

import (
	"bytes"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestDecodeImage(t *testing.T) {
	g := quadImage()
	var buf bytes.Buffer
	if err := png.Encode(&buf, g); err != nil {
		t.Fatal(err)
	}
	img, err := DecodeImage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 2 || ToRGBA(img).RGBAAt(1, 1) != g.RGBAAt(1, 1) {
		t.Fatal("png round trip mismatch")
	}
	buf.Reset()
	if err := jpeg.Encode(&buf, g, nil); err != nil {
		t.Fatal(err)
	}
	if img, err := DecodeImage(&buf); err != nil || img.Bounds().Dy() != 2 {
		t.Fatalf("jpeg: %v", err)
	}
	if _, err := DecodeImage(bytes.NewReader([]byte("GIF89a...."))); err != ErrImageFormat {
		t.Fatalf("want ErrImageFormat, got %v", err)
	}
}
