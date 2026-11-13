package flyout

import "testing"

func TestPlace(t *testing.T) {
	mac := Rect{X: 0, Y: 25, Width: 1440, Height: 875} // under the menu bar
	win := Rect{X: 0, Y: 0, Width: 1920, Height: 1040} // above a bottom taskbar
	for _, c := range []struct {
		name         string
		anchor       Rect
		hasAnchor    bool
		work         Rect
		hasWork      bool
		bottom       bool
		wantX, wantY int
	}{
		{"menu bar", Rect{X: 1000, Y: 0, Width: 22, Height: 24}, true, mac, true, false, 1011 - 160, 31},
		{"menu bar right edge clamps", Rect{X: 1420, Y: 0, Width: 20, Height: 24}, true, mac, true, false, 1440 - 320, 31},
		{"bottom taskbar", Rect{X: 1700, Y: 1048, Width: 24, Height: 24}, true, win, true, false, 1712 - 160, 1040 - 400 - 6},
		{"left taskbar", Rect{X: 8, Y: 900, Width: 24, Height: 24}, true, Rect{X: 48, Y: 0, Width: 1872, Height: 1080}, true, false, 48 + 6, 1080 - 400},
		{"right taskbar", Rect{X: 1880, Y: 500, Width: 24, Height: 24}, true, Rect{X: 0, Y: 0, Width: 1872, Height: 1080}, true, false, 1872 - 320 - 6, 512 - 200},
		{"no anchor top-right", Rect{}, false, mac, true, false, 1440 - 320 - 6, 25 + 6},
		{"no anchor bottom-right", Rect{}, false, win, true, true, 1920 - 320 - 6, 1040 - 400 - 6},
		{"no work area", Rect{X: 100, Y: 0, Width: 20, Height: 20}, true, Rect{}, false, false, 110 - 160, 26},
		{"nothing", Rect{}, false, Rect{}, false, false, 0, 0},
		{"secondary display left of primary", Rect{X: -400, Y: 0, Width: 22, Height: 24}, true, Rect{X: -1920, Y: 25, Width: 1920, Height: 1055}, true, false, -389 - 160, 31},
	} {
		x, y := Place(c.anchor, c.hasAnchor, c.work, c.hasWork, 320, 400, 6, c.bottom)
		if x != c.wantX || y != c.wantY {
			t.Errorf("%s: (%d,%d), want (%d,%d)", c.name, x, y, c.wantX, c.wantY)
		}
	}
}
