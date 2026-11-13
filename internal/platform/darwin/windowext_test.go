package darwin

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestCoordinateFlip(t *testing.T) {
	// 1080-high primary display; a 600-high frame whose top is 100px below
	// the top of the screen sits at y=380 in AppKit coordinates.
	if x, y := topLeft(50, 380, 600, 1080); x != 50 || y != 100 {
		t.Fatalf("topLeft = %d,%d", x, y)
	}
	if got := bottomLeftY(100, 600, 1080); got != 380 {
		t.Fatalf("bottomLeftY = %v", got)
	}
	// Round trip on a display above the primary (negative top).
	if _, y := topLeft(0, bottomLeftY(-300, 500, 1080), 500, 1080); y != -300 {
		t.Fatal(y)
	}
}

func TestWindowState(t *testing.T) {
	for _, c := range []struct {
		fs, mini, zoom bool
		want           platform.WindowState
	}{
		{false, false, false, platform.StateNormal},
		{false, false, true, platform.StateMaximized},
		{false, true, true, platform.StateMinimized},
		{true, false, true, platform.StateFullscreen},
	} {
		if got := windowState(c.fs, c.mini, c.zoom); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

func TestRGBA(t *testing.T) {
	r, g, b, a := rgbaParts(0xff8000ff)
	if r != 1 || g < 0.5 || g > 0.51 || b != 0 || a != 1 {
		t.Fatal(r, g, b, a)
	}
}
