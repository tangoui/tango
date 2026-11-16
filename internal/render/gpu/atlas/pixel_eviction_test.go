package atlas

import (
	"bytes"
	"image"
	"testing"
)

func TestPixelEvictionReuploadMatchesOriginal(t *testing.T) {
	a := New(8, 2)
	a.SetBudgetForTest(1, 0)
	k := key(9)
	m := image.NewAlpha(image.Rect(0, 0, 7, 7))
	for i := range m.Pix {
		m.Pix[i] = uint8((i * 17) % 251)
	}
	a.BeginFrame(1)
	if _, err := a.Insert(key(0), m, image.Point{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Insert(k, m, image.Point{}); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), a.Pixels().Pix...)
	a.EndFrame()
	a.BeginFrame(2)
	if got := a.Evict(2); got != 1 {
		t.Fatalf("evicted=%d", got)
	}
	a.BeginFrame(3)
	if _, err := a.Insert(k, m, image.Point{}); err != nil {
		t.Fatal(err)
	}
	second := append([]byte(nil), a.Pixels().Pix...)
	if !bytes.Equal(first, second) {
		t.Fatal("re-uploaded atlas pixels differ")
	}
}
