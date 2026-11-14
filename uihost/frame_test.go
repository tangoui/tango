package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// The window implements ui.WindowFrame on its surface: a TitleBar moves,
// maximizes and closes the real window.
func TestWindowFrame(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var info ui.WindowFrameInfo
	var hasFrame bool
	_, s, cv := open(t, a, fb, 400, 200, func(c *ui.Context) {
		f, ok := c.Window().(ui.WindowFrame)
		hasFrame = ok
		if ok {
			info = f.FrameInfo()
		}
		ui.TitleBar(c).Title("App").Controls(ui.WindowControlsCaption)
	})
	for cv.FlushFrame() {
	}
	fb.Do(func() {})
	if !hasFrame {
		t.Fatal("uihost window does not implement ui.WindowFrame")
	}
	if !info.SystemTitleBar || info.State != ui.WindowNormal {
		t.Fatalf("info %+v", info)
	}
	press := func(p geom.Point) {
		cv.Input(input.Pointer{Kind: input.Move, Pos: p})
		cv.Input(input.Pointer{Kind: input.Press, Pos: p, Button: input.ButtonLeft})
		cv.Input(input.Pointer{Kind: input.Release, Pos: p, Button: input.ButtonLeft})
		for cv.FlushFrame() {
		}
	}
	// Dragging the bar starts a native move.
	press(geom.Pt(150, 18))
	if _, _, _, moves := cv.Surface().WindowCounts(); moves != 1 {
		t.Fatalf("moves = %d", moves)
	}
	// The maximize caption button maximizes the surface.
	press(geom.Pt(400-46-23, 18))
	if st := tango.Await(a, s.State); st != tango.StateMaximized {
		t.Fatalf("state = %v", st)
	}
	// A frameless appearance is reported.
	tango.Await(a, func() error { return s.SetAppearance(tango.Appearance{TitleBar: tango.TitleBarNone, NoMinimize: true}) })
	cv.Input(input.Pointer{Kind: input.Move, Pos: geom.Pt(10, 100)}) // a frame reads it
	for cv.FlushFrame() {
	}
	fb.Do(func() {})
	if info.SystemTitleBar || !info.NoMinimize || info.State != ui.WindowMaximized {
		t.Fatalf("info after appearance change %+v", info)
	}
}
