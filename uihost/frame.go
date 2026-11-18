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
	if w.t == nil {
		s.OnStateChanged(func(tango.WindowState) { w.Invalidate() })
		return
	}
	refresh := func() {
		info := readFrameInfo(s)
		w.t.frameInfo.Store(&info)
	}
	changed := func() {
		refresh()
		w.Invalidate()
	}
	refresh() // before handing the window to its UI thread
	s.OnStateChanged(func(tango.WindowState) { changed() })
	corehook.WatchAppearance(s, changed)
}

func (w *window) FrameInfo() ui.WindowFrameInfo {
	if w.s == nil {
		return ui.WindowFrameInfo{}
	}
	if w.t != nil {
		return *w.t.frameInfo.Load()
	}
	return readFrameInfo(w.s)
}

// readFrameInfo reads the native window on the main thread.
func readFrameInfo(s *tango.Surface) ui.WindowFrameInfo {
	ap, state := s.Appearance(), s.State()
	var st ui.WindowState
	switch state {
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
	s := w.s
	w.onMain(func() {
		switch st {
		case ui.WindowMinimized:
			_ = s.Minimize()
		case ui.WindowMaximized:
			_ = s.Maximize()
		case ui.WindowFullscreen:
			_ = s.Fullscreen()
		default:
			_ = s.Restore()
		}
	})
}

func (w *window) CloseWindow() {
	if s := w.s; s != nil {
		w.onMain(s.Close)
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
