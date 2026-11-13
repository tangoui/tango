package platform

import (
	"image"
	"testing"
)

func TestCanvasLayerHitThreshold(t *testing.T) {
	for _, c := range []struct{ in, want uint8 }{{0, 1}, {1, 1}, {32, 32}, {255, 255}} {
		if got := (CanvasLayer{HitAlpha: c.in}).HitThreshold(); got != c.want {
			t.Errorf("HitAlpha %d: threshold %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCanvasLayerAnchoredBounds(t *testing.T) {
	b := Rect{X: 300, Y: 200, Width: 80, Height: 60}
	for _, c := range []struct {
		a    CanvasAnchor
		want Rect
	}{
		{AnchorTopLeft, b},
		{AnchorTopRight, Rect{X: 400, Y: 200, Width: 80, Height: 60}},
		{AnchorBottomLeft, Rect{X: 300, Y: 250, Width: 80, Height: 60}},
		{AnchorBottomRight, Rect{X: 400, Y: 250, Width: 80, Height: 60}},
		{AnchorCenter, Rect{X: 350, Y: 225, Width: 80, Height: 60}},
	} {
		if got := (CanvasLayer{Bounds: b, Anchor: c.a}).AnchoredBounds(400, 300, 500, 350); got != c.want {
			t.Errorf("anchor %d: %+v, want %+v", c.a, got, c.want)
		}
	}
	if got := (CanvasLayer{Anchor: AnchorBottomRight}).AnchoredBounds(400, 300, 500, 350); got != (Rect{}) {
		t.Errorf("fill moved: %+v", got)
	}
}

func TestDamageRects(t *testing.T) {
	b := image.Rect(0, 0, 100, 100)
	rects, full := DamageRects([]image.Rectangle{
		image.Rect(-10, -10, 10, 10), // clipped
		image.Rect(50, 50, 50, 60),   // empty
		image.Rect(90, 90, 120, 95),  // clipped
		image.Rect(200, 200, 210, 210),
	}, b, 8)
	if full || len(rects) != 2 || rects[0] != image.Rect(0, 0, 10, 10) || rects[1] != image.Rect(90, 90, 100, 95) {
		t.Fatalf("rects %v full %v", rects, full)
	}
	if rects, full := DamageRects(nil, b, 8); full || len(rects) != 0 {
		t.Fatalf("empty: rects %v full %v", rects, full)
	}
	if _, full := DamageRects([]image.Rectangle{image.Rect(0, 0, 100, 51)}, b, 8); !full {
		t.Fatal("more than half the frame should be presented whole")
	}
	many := make([]image.Rectangle, 9)
	for i := range many {
		many[i] = image.Rect(i, 0, i+1, 1)
	}
	if _, full := DamageRects(many, b, 8); !full {
		t.Fatal("too many rects should be presented whole")
	}
}

func TestDamageToLogical(t *testing.T) {
	for _, c := range []struct {
		r     image.Rectangle
		scale float64
		want  image.Rectangle
	}{
		{image.Rect(21, 20, 61, 60), 2, image.Rect(10, 10, 31, 30)},
		{image.Rect(3, 3, 4, 4), 1.5, image.Rect(2, 2, 3, 3)},
		{image.Rect(5, 6, 7, 8), 1, image.Rect(5, 6, 7, 8)},
		{image.Rect(5, 6, 7, 8), 0, image.Rect(5, 6, 7, 8)},
	} {
		if got := DamageToLogical(c.r, c.scale); got != c.want {
			t.Errorf("%v @%v: %v, want %v", c.r, c.scale, got, c.want)
		}
	}
}
