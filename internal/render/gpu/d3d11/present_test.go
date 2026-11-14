package d3d11

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestPresentDirtyRects(t *testing.T) {
	sz := image.Pt(100, 50)
	changed := []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(20, 30, 100, 50)}
	got := presentDirtyRects(swapEffectFlipSequential, 5, 4, false, false, changed, sz, sz)
	if len(got) != 2 || got[1] != (rect{20, 30, 100, 50}) {
		t.Fatalf("partial frame after the presented one: %v", got)
	}
	for name, r := range map[string][]rect{
		"first present":       presentDirtyRects(swapEffectFlipSequential, 1, 0, false, false, changed, sz, sz),
		"frame skipped":       presentDirtyRects(swapEffectFlipSequential, 6, 4, false, false, changed, sz, sz),
		"presented twice":     presentDirtyRects(swapEffectFlipSequential, 4, 4, false, false, changed, sz, sz),
		"full frame":          presentDirtyRects(swapEffectFlipSequential, 5, 4, true, false, changed, sz, sz),
		"resized":             presentDirtyRects(swapEffectFlipSequential, 5, 4, false, true, changed, sz, sz),
		"buffer larger":       presentDirtyRects(swapEffectFlipSequential, 5, 4, false, false, changed, sz, image.Pt(101, 50)),
		"rect outside buffer": presentDirtyRects(swapEffectFlipSequential, 5, 4, false, false, []image.Rectangle{image.Rect(90, 0, 110, 5)}, image.Pt(120, 50), image.Pt(120, 4)),
		"empty rect":          presentDirtyRects(swapEffectFlipSequential, 5, 4, false, false, []image.Rectangle{{}}, sz, sz),
		"nothing changed":     presentDirtyRects(swapEffectFlipSequential, 5, 4, false, false, nil, sz, sz),
		"flip discard":        presentDirtyRects(swapEffectFlipDiscard, 5, 4, false, false, changed, sz, sz),
		"occluded before":     presentDirtyRects(swapEffectFlipSequential, 5, 0, false, false, changed, sz, sz),
	} {
		if r != nil {
			t.Errorf("%s: %v, want no hint", name, r)
		}
	}
}

func TestPresentSize(t *testing.T) {
	if got := presentSize(image.Pt(20000, 300), 16384); got != image.Pt(16384, 300) {
		t.Errorf("presentSize = %v", got)
	}
	if got := presentSize(image.Pt(800, 9000), 8192); got != image.Pt(800, 8192) {
		t.Errorf("presentSize = %v", got)
	}
}

func TestBackgroundRGBA(t *testing.T) {
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }
	for _, c := range []struct {
		in   color.NRGBA
		want dxgiRGBA
	}{
		{color.NRGBA{0, 0, 0, 255}, dxgiRGBA{0, 0, 0, 1}},
		{color.NRGBA{255, 255, 255, 255}, dxgiRGBA{1, 1, 1, 1}},
		{color.NRGBA{128, 0, 10, 255}, dxgiRGBA{0.2158605, 0, 0.0030353, 1}},
		// Alpha is ignored by the swap chain: the premultiplied color.
		{color.NRGBA{255, 255, 255, 0}, dxgiRGBA{0, 0, 0, 1}},
	} {
		got := backgroundRGBA(c.in)
		if !near(got.R, c.want.R) || !near(got.G, c.want.G) || !near(got.B, c.want.B) || got.A != 1 {
			t.Errorf("backgroundRGBA(%v) = %+v, want %+v", c.in, got, c.want)
		}
	}
}
