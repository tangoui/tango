package ui

import (
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// sysBtnWindow is a headless Window that also implements WindowFrame, standing
// in for a host window whose system draws the traffic lights (TitleBarHidden
// on macOS). Present keeps the frame so the test can inspect pixels.
type sysBtnWindow struct {
	systemButtons bool
	img           *image.RGBA
}

func (w *sysBtnWindow) Size() geom.Size                   { return geom.Size{W: 800, H: 200} }
func (w *sysBtnWindow) Scale() float32                    { return 1 }
func (w *sysBtnWindow) Invalidate()                       {}
func (w *sysBtnWindow) InvalidateAt(t time.Time)          {}
func (w *sysBtnWindow) SetCursor(c input.Cursor)          {}
func (w *sysBtnWindow) ReadClipboard() string             { return "" }
func (w *sysBtnWindow) WriteClipboard(s string)           {}
func (w *sysBtnWindow) SetTextInput(on bool, r geom.Rect) {}
func (w *sysBtnWindow) Present(img *image.RGBA) error {
	if img != nil {
		w.img = image.NewRGBA(img.Bounds())
		copy(w.img.Pix, img.Pix)
	}
	return nil
}
func (w *sysBtnWindow) FrameInfo() WindowFrameInfo {
	return WindowFrameInfo{SystemButtons: w.systemButtons}
}
func (w *sysBtnWindow) SetWindowState(s WindowState) {}
func (w *sysBtnWindow) CloseWindow()                 {}
func (w *sysBtnWindow) StartWindowMove()             {}

// hasTrafficRed reports whether the frame contains the macOS close-light fill
// (#ff5f57), i.e. the bar drew its own traffic lights.
func hasTrafficRed(img *image.RGBA) bool {
	if img == nil {
		return false
	}
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
		if a > 0x80 && r > 0xf0 && g > 0x50 && g < 0x70 && b > 0x40 && b < 0x60 {
			return true
		}
	}
	return false
}

// TestTitleBarReservesSystemButtons: when the host window keeps the system
// traffic lights, the self-drawn bar must not draw a second set (the tan dev
// worker used to, because its remote window did not implement WindowFrame).
func TestTitleBarReservesSystemButtons(t *testing.T) {
	withPlatform(t, true) // traffic lights are the macOS window controls
	page := func(c *Context) { TitleBar(c).Menu(func() { Text(c, "menu") }) }
	for _, tc := range []struct {
		system bool
		want   bool
	}{{true, false}, {false, true}} {
		w := &sysBtnWindow{systemButtons: tc.system}
		r := NewRunner(page, RunnerOptions{})
		r.Attach(w)
		if err := r.Frame(); err != nil {
			t.Fatal(err)
		}
		if got := hasTrafficRed(w.img); got != tc.want {
			t.Fatalf("systemButtons=%v: traffic lights drawn=%v, want %v", tc.system, got, tc.want)
		}
	}
}
