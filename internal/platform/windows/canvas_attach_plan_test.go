package windows

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestAttachBounds(t *testing.T) {
	fill := attachBounds(platform.CanvasLayer{}, 100, 80, 300, 200)
	if fill != (platform.Rect{Width: 300, Height: 200}) {
		t.Errorf("fill: %+v", fill)
	}
	r := platform.Rect{X: 10, Y: 20, Width: 50, Height: 40}
	for _, c := range []struct {
		a    platform.CanvasAnchor
		want platform.Rect
	}{
		{platform.AnchorTopLeft, r},
		{platform.AnchorTopRight, platform.Rect{X: 110, Y: 20, Width: 50, Height: 40}},
		{platform.AnchorBottomLeft, platform.Rect{X: 10, Y: 70, Width: 50, Height: 40}},
		{platform.AnchorBottomRight, platform.Rect{X: 110, Y: 70, Width: 50, Height: 40}},
		{platform.AnchorCenter, platform.Rect{X: 60, Y: 45, Width: 50, Height: 40}},
	} {
		got := attachBounds(platform.CanvasLayer{Bounds: r, Anchor: c.a}, 200, 150, 300, 200)
		if got != c.want {
			t.Errorf("anchor %d: got %+v want %+v", c.a, got, c.want)
		}
	}
}

func TestAttachDevice(t *testing.T) {
	x, y, w, h := attachDevice(platform.Rect{X: 1, Y: 1, Width: 1, Height: 1}, 1.5)
	if x != 2 || y != 2 || w != 1 || h != 1 { // round(1.5)=2, round(3)=3
		t.Errorf("1.5x: %d %d %d %d", x, y, w, h)
	}
	x, y, w, h = attachDevice(platform.Rect{X: 40, Y: 30, Width: 200, Height: 100}, 2)
	if x != 80 || y != 60 || w != 400 || h != 200 {
		t.Errorf("2x: %d %d %d %d", x, y, w, h)
	}
	if _, _, w, h = attachDevice(platform.Rect{}, 1); w != 1 || h != 1 {
		t.Errorf("min size: %d %d", w, h)
	}
}

func TestAttachHitMask(t *testing.T) {
	// 3×2 frame, stride with padding.
	pix := []byte{
		0, 0, 0, 0, 0, 0, 0, 31, 0, 0, 0, 32, 9, 9,
		0, 0, 0, 255, 0, 0, 0, 1, 0, 0, 0, 0, 9, 9,
	}
	mask := make([]byte, 6)
	if !attachHitMask(mask, pix, 14, 3, 2, 32) {
		t.Fatal("first mask: no change reported")
	}
	if want := []byte{0, 0, 1, 1, 0, 0}; string(mask) != string(want) {
		t.Errorf("thr 32: %v", mask)
	}
	if attachHitMask(mask, pix, 14, 3, 2, 32) {
		t.Error("same frame reported a change")
	}
	attachHitMask(mask, pix, 14, 3, 2, 1)
	if want := []byte{0, 1, 1, 1, 1, 0}; string(mask) != string(want) {
		t.Errorf("thr 1: %v", mask)
	}
}

func TestAttachMaskRects(t *testing.T) {
	mask := []byte{
		0, 1, 1, 0, 1,
		0, 1, 1, 0, 1,
		1, 1, 0, 0, 0,
		0, 0, 0, 0, 0,
		0, 1, 1, 0, 1,
	}
	got := attachMaskRects(mask, 5, 5)
	want := []attachRect{
		{1, 0, 2, 2}, {4, 0, 1, 2},
		{0, 2, 2, 1},
		{1, 4, 2, 1}, {4, 4, 1, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// Coverage equals the mask.
	cover := make([]byte, 25)
	for _, r := range got {
		for y := r[1]; y < r[1]+r[3]; y++ {
			for x := r[0]; x < r[0]+r[2]; x++ {
				cover[y*5+x]++
			}
		}
	}
	if string(cover) != string(mask) {
		t.Errorf("coverage %v", cover)
	}
	if len(attachMaskRects(make([]byte, 12), 4, 3)) != 0 {
		t.Error("empty mask gave rectangles")
	}
	if r := attachMaskRects([]byte{1, 1, 1, 1}, 2, 2); len(r) != 1 || r[0] != (attachRect{0, 0, 2, 2}) {
		t.Errorf("full mask: %v", r)
	}
}
