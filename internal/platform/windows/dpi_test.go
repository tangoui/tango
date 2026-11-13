package windows

import "testing"

func TestDIPConversion(t *testing.T) {
	cases := []struct{ dip, dpi, px int }{
		{1024, 96, 1024},
		{1024, 120, 1280},
		{1024, 144, 1536},
		{1024, 192, 2048},
		{800, 168, 1400}, // 175%
		{1, 144, 2},      // 1.5 rounds half up
		{0, 144, 0},
		{-10, 144, -15},
		{100, 0, 100}, // unknown DPI means 96
	}
	for _, c := range cases {
		if got := dipToPx(c.dip, c.dpi); got != c.px {
			t.Errorf("dipToPx(%d, %d) = %d, want %d", c.dip, c.dpi, got, c.px)
		}
	}
	for _, dpi := range []int{96, 120, 144, 168, 192, 240, 288} {
		for _, v := range []int{0, 1, 99, 640, 768, 1024, 1920} {
			if got := pxToDIP(dipToPx(v, dpi), dpi); got != v {
				t.Errorf("round trip %d at %d DPI = %d", v, dpi, got)
			}
		}
	}
	if got := pxToDIP(1281, 120); got != 1025 {
		t.Errorf("pxToDIP(1281,120) = %d", got)
	}
}

func TestDefaultSizeAndCenter(t *testing.T) {
	if w, h := defaultSurfaceSize(0, 0); w != 1024 || h != 768 {
		t.Errorf("default = %dx%d", w, h)
	}
	if w, h := defaultSurfaceSize(300, 0); w != 300 || h != 768 {
		t.Errorf("partial default = %dx%d", w, h)
	}
	if x, y := centerIn(0, 0, 1920, 1040, 1000, 600); x != 460 || y != 220 {
		t.Errorf("center = %d,%d", x, y)
	}
	if x, y := centerIn(-1920, 0, 0, 1080, 4000, 2000); x != -1920 || y != 0 {
		t.Errorf("oversized = %d,%d", x, y)
	}
}
