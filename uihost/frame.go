package uihost

import (
	"runtime"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/ui"
)

// ui.WindowFrame on the window's surface: a self-drawn ui.TitleBar moves,
// minimizes, maximizes and closes the window through it.

// watchFrame keeps the runner's view of the window state current: a state
// change (by the title bar, the system or the user) redraws the page, so
// the maximize / restore button follows.
func (w *window) watchFrame(s *tango.Surface) {
	w.s = s
	s.OnStateChanged(func(tango.WindowState) { w.cv.Invalidate() })
}

func (w *window) FrameInfo() ui.WindowFrameInfo {
	if w.s == nil {
		return ui.WindowFrameInfo{}
	}
	ap := w.s.Appearance()
	var st ui.WindowState
	switch w.s.State() {
	case tango.StateMinimized:
		st = ui.WindowMinimized
	case tango.StateMaximized:
		st = ui.WindowMaximized
	case tango.StateFullscreen:
		st = ui.WindowFullscreen
	}
	return ui.WindowFrameInfo{
		State:          st,
		SystemTitleBar: ap.TitleBar == tango.TitleBarDefault,
		// macOS keeps the traffic lights over a hidden title bar.
		SystemButtons: ap.TitleBar == tango.TitleBarHidden && (runtime.GOOS == "darwin" || runtime.GOOS == "ios"),
		NoMinimize:    ap.NoMinimize, NoMaximize: ap.NoMaximize, NoClose: ap.NoClose,
	}
}

func (w *window) SetWindowState(st ui.WindowState) {
	if w.s == nil {
		return
	}
	switch st {
	case ui.WindowMinimized:
		_ = w.s.Minimize()
	case ui.WindowMaximized:
		_ = w.s.Maximize()
	case ui.WindowFullscreen:
		_ = w.s.Fullscreen()
	default:
		_ = w.s.Restore()
	}
}

func (w *window) CloseWindow() {
	if w.s != nil {
		w.s.Close()
	}
}

// StartWindowMove posts the move instead of starting it inside the frame:
// a platform's move loop (macOS performWindowDragWithEvent:) may not
// return until the button is released. The press is still held when the
// posted call runs.
func (w *window) StartWindowMove() {
	if w.s == nil || corehook.StartWindowMove == nil {
		return
	}
	s := w.s
	w.app.Post(func() { corehook.StartWindowMove(s) })
}
