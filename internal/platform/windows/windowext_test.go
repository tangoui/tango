package windows

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestEdgeHitTest(t *testing.T) {
	want := map[string]uintptr{
		"n": htTop, "s": htBottom, "e": htRight, "w": htLeft,
		"ne": htTopRight, "nw": htTopLeft, "se": htBottomRight, "sw": htBottomLeft,
	}
	for edge, ht := range want {
		if got, ok := edgeHitTest(edge); !ok || got != ht {
			t.Errorf("edgeHitTest(%q) = %d, %v; want %d", edge, got, ok, ht)
		}
	}
	for _, bad := range []string{"", "N", "x", "nse"} {
		if _, ok := edgeHitTest(bad); ok {
			t.Errorf("edgeHitTest(%q) accepted", bad)
		}
	}
}

func TestFileURLToPath(t *testing.T) {
	ok := map[string]string{
		"file:///C:/Users/a%20b/x.txt":         `C:\Users\a b\x.txt`,
		"file:///c:/dir/":                      `c:\dir\`,
		"file:///D:":                           `D:\`,
		"file://localhost/C:/x":                `C:\x`,
		"FILE:///C|/x":                         `C:\x`,
		"file://server/share/dir/f.txt":        `\\server\share\dir\f.txt`,
		"file:///C:/%E4%B8%AD%E6%96%87.png":    `C:\中文.png`,
		"file:///C:/a%23b.txt":                 `C:\a#b.txt`,
		"file:///C:/Program%20Files%20(x86)/q": `C:\Program Files (x86)\q`,
	}
	for in, want := range ok {
		got, err := fileURLToPath(in)
		if err != nil || got != want {
			t.Errorf("fileURLToPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://x/y", "file://server/", "file://", "about:blank"} {
		if got, err := fileURLToPath(bad); err == nil {
			t.Errorf("fileURLToPath(%q) = %q, want error", bad, got)
		}
	}
	if !isFileURL("File:///C:/x") || isFileURL("https://tango.localhost/") || isFileURL("fil") {
		t.Error("isFileURL")
	}
}

func TestStateOf(t *testing.T) {
	cases := []struct {
		min, max, full bool
		want           platform.WindowState
	}{
		{false, false, false, platform.StateNormal},
		{true, false, false, platform.StateMinimized},
		{false, true, false, platform.StateMaximized},
		{false, false, true, platform.StateFullscreen},
		{false, true, true, platform.StateFullscreen},
		{true, false, true, platform.StateMinimized},
	}
	for _, c := range cases {
		if got := stateOf(c.min, c.max, c.full); got != c.want {
			t.Errorf("stateOf(%v,%v,%v) = %v, want %v", c.min, c.max, c.full, got, c.want)
		}
	}
	if sizeState(sizeTypeMaximize, false) != platform.StateMaximized ||
		sizeState(sizeTypeMinimize, true) != platform.StateMinimized ||
		sizeState(sizeTypeRestored, true) != platform.StateFullscreen ||
		sizeState(sizeTypeRestored, false) != platform.StateNormal {
		t.Error("sizeState")
	}
}

func TestFirstShowCmd(t *testing.T) {
	if firstShowCmd(platform.StateNormal, false) != showNormal ||
		firstShowCmd(platform.StateNormal, true) != showNoActivate ||
		firstShowCmd(platform.StateMaximized, false) != showMaximized ||
		firstShowCmd(platform.StateMinimized, false) != showMinimized ||
		firstShowCmd(platform.StateMinimized, true) != showMinNoActive ||
		firstShowCmd(platform.StateFullscreen, false) != showNormal {
		t.Error("firstShowCmd")
	}
}

func TestBoxAndFullscreenStyle(t *testing.T) {
	base := uint32(styleOverlappedWindow | 0x02000000)
	got := boxStyle(base, platform.Appearance{}, true)
	if got != base {
		t.Errorf("all boxes: %#x", got)
	}
	got = boxStyle(base, platform.Appearance{NoMinimize: true, NoMaximize: true}, true)
	if got&(styleMinimizeBox|styleMaximizeBox) != 0 || got&styleThickFrame == 0 {
		t.Errorf("no boxes: %#x", got)
	}
	got = boxStyle(base, platform.Appearance{}, false)
	if got&styleMaximizeBox != 0 {
		t.Errorf("non-resizable got a maximize box: %#x", got)
	}
	fs := fullscreenStyle(base | 0x10000000) // WS_VISIBLE kept
	if fs&styleOverlappedWindow != 0 || fs&stylePopup == 0 || fs&0x10000000 == 0 || fs&0x02000000 == 0 {
		t.Errorf("fullscreenStyle: %#x", fs)
	}
}

func TestFrameMargins(t *testing.T) {
	none := [4]int32{}
	all := [4]int32{-1, -1, -1, -1}
	one := [4]int32{1, 1, 1, 1}
	cases := []struct {
		a    platform.Appearance
		want [4]int32
	}{
		{platform.Appearance{NoShadow: true}, none},
		{platform.Appearance{}, none}, // the shadow is for frameless only
		{platform.Appearance{TitleBar: platform.TitleBarNone, NoShadow: true}, none},
		{platform.Appearance{TitleBar: platform.TitleBarNone}, one},
		{platform.Appearance{TitleBar: platform.TitleBarHidden}, one},
		{platform.Appearance{Transparent: true}, all},
		{platform.Appearance{Material: platform.MaterialMica, TitleBar: platform.TitleBarNone}, all},
	}
	for i, c := range cases {
		if got := frameMargins(c.a); got != c.want {
			t.Errorf("%d: frameMargins = %v, want %v", i, got, c.want)
		}
	}
	if backdropType(platform.MaterialMica) != 2 || backdropType(platform.MaterialBlur) != 3 || backdropType(platform.MaterialNone) != 1 {
		t.Error("backdropType")
	}
}

func TestWebviewColor(t *testing.T) {
	if webviewColor(platform.Appearance{Transparent: true, Background: 0x112233FF}) != 0 {
		t.Error("transparent")
	}
	if got := webviewColor(platform.Appearance{}); got != 0xFFFFFFFF {
		t.Errorf("default %#x", got)
	}
	// 0xRRGGBBAA → bytes {A, R, G, B}, alpha forced opaque.
	if got := webviewColor(platform.Appearance{Background: 0x11223380}); got != 0x332211FF {
		t.Errorf("packed %#x", got)
	}
}

func TestLayeredAlpha(t *testing.T) {
	for _, op := range []float64{0, 1, 1.5, -1} {
		if a, on := layeredAlpha(op); on || a != 255 {
			t.Errorf("layeredAlpha(%v) = %d, %v", op, a, on)
		}
	}
	if a, on := layeredAlpha(0.5); !on || a != 128 {
		t.Errorf("layeredAlpha(0.5) = %d, %v", a, on)
	}
}

func TestMaximizedClient(t *testing.T) {
	l, tp, r, b := maximizedClient(-8, -8, 1928, 1048, true, 8, 8)
	if l != 0 || tp != 0 || r != 1920 || b != 1040 {
		t.Errorf("maximized: %d %d %d %d", l, tp, r, b)
	}
	l, tp, r, b = maximizedClient(100, 100, 900, 700, false, 8, 8)
	if l != 100 || tp != 100 || r != 900 || b != 700 {
		t.Errorf("restored: %d %d %d %d", l, tp, r, b)
	}
}

func TestPlaceCenteredAndClamp(t *testing.T) {
	if !placeCentered(nil) || !placeCentered(&platform.SurfaceExtras{}) ||
		!placeCentered(&platform.SurfaceExtras{Centered: true, Bounds: platform.Rect{X: 5, Y: 5}}) ||
		placeCentered(&platform.SurfaceExtras{Bounds: platform.Rect{Width: 300, Height: 200}}) {
		t.Error("placeCentered")
	}
	if clampSize(50, 100, 0) != 100 || clampSize(500, 0, 400) != 400 || clampSize(300, 100, 400) != 300 || clampSize(300, 0, 0) != 300 {
		t.Error("clampSize")
	}
}
