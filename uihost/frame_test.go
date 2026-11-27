package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/apphook"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
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
	// StartWindowMove posts to App's queue, not fake's native-event queue.
	// FlushFrame returning only proves the frame ran, not the posted move.
	tango.Await(a, func() bool { return true })
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

type overlayHostBackend struct{ *fake.Backend }

func (overlayHostBackend) SupportsOverlay() bool { return true }

func TestOverlayWindowFrame(t *testing.T) {
	fb := fake.New()
	a := apphook.NewApp(overlayHostBackend{fb}, tango.Options{}).(*tango.App)
	faketest.Start(t, a, fb)
	type result struct {
		info ui.WindowFrameInfo
		err  error
	}
	got := tango.Await(a, func() result {
		r, s, err := uihost.Open(a, uihost.Options{Surface: tango.SurfaceOptions{
			Kind: tango.KindOverlay,
		}}, func(*ui.Context) {}, ui.RunnerOptions{})
		if err != nil {
			return result{err: err}
		}
		if err := s.SetAppearance(tango.Appearance{TitleBar: tango.TitleBarHidden, NoClose: true}); err != nil {
			return result{err: err}
		}
		return result{info: r.Context().Window().(ui.WindowFrame).FrameInfo()}
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.info.SystemTitleBar || got.info.SystemButtons || !got.info.NoMinimize || !got.info.NoMaximize || got.info.NoClose {
		t.Fatalf("overlay frame info = %+v", got.info)
	}
}
