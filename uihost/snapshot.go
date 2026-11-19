package uihost

// Per-frame UI-state snapshots of threaded windows (design §5.7). AppKit
// asks some questions on the main thread that need an answer at once and
// depend on UI state: does this key belong to a window shortcut, does this
// press move the window. The UI thread publishes an immutable snapshot of
// that state at the end of every frame; the main thread answers from the
// latest one, so its decisions lag the UI by at most one frame.

import (
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// uiSnapshot is the UI state of a threaded window as of its latest frame.
// Published by the UI thread, never modified after; read on the main
// thread.
type uiSnapshot struct {
	editableFocus bool        // focus is on editable text (the runner turned text input on)
	composing     bool        // an input method is composing
	capturing     bool        // a shortcut recorder captures keys
	dragRegions   []geom.Rect // drag areas of self-drawn title bars (logical pixels)

	theme *theme.Theme // the runner's theme (FollowingSystemTheme, follow.go)
}

var emptySnapshot uiSnapshot

// snapshot returns the latest published snapshot. Any goroutine.
func (t *threadedHost) snapshot() *uiSnapshot {
	if s := t.snap.Load(); s != nil {
		return s
	}
	return &emptySnapshot
}

// publishSnapshot publishes the window's state after a frame. UI thread.
func (h *host) publishSnapshot() {
	w := h.win
	h.t.snap.Store(&uiSnapshot{
		editableFocus: w.textOn,
		composing:     h.r.Composing(),
		capturing:     w.capturing,
		dragRegions:   h.r.DragRegions(),
		theme:         h.r.Context().Theme,
	})
}

// dragOnMain starts the window move for a primary press inside one of the
// snapshot's drag regions, without the UI thread, and reports whether it
// took the event (which then must not be queued). A second press there
// within the title bar's double-click interval maximizes or restores the
// window, as ui.TitleBar does. Main thread, threaded windows.
func (h *host) dragOnMain(ev input.Event) bool {
	p, ok := ev.(input.Pointer)
	if !ok || p.Kind != input.Press || p.Button != input.ButtonLeft || h.win.s == nil || corehook.StartWindowMove == nil {
		return false // no platform move: the UI handles the press
	}
	in := false
	for _, r := range h.t.snapshot().dragRegions {
		if r.Contains(p.Pos) {
			in = true
			break
		}
	}
	if !in {
		return false
	}
	s, now := h.win.s, time.Now()
	last, lastPos := h.t.dragPress, h.t.dragPos
	if !last.IsZero() && now.Sub(last) < titleBarDoubleClick && abs(p.Pos.X-lastPos.X) < 5 && abs(p.Pos.Y-lastPos.Y) < 5 {
		h.t.dragPress = time.Time{}
		var act ui.WindowAction
		if info := readFrameInfo(s); !info.NoMaximize {
			switch info.State {
			case ui.WindowMaximized:
				act = ui.WindowActionRestore
				_ = s.Restore()
			case ui.WindowNormal:
				act = ui.WindowActionMaximize
				_ = s.Maximize()
			}
		}
		if act != "" { // TitleBarW.Action reports it in the next frame
			pos, t := p.Pos, h.t
			t.post(h.app, func() { h.r.TitleBarActed(pos, act); h.invalidate() })
		}
		return true
	}
	h.t.dragPress, h.t.dragPos = now, p.Pos
	// In the press's own native callback: the platform's move loop (macOS
	// performWindowDragWithEvent:) gets the mouse-down it needs.
	corehook.StartWindowMove(s)
	return true
}

// titleBarDoubleClick is ui.TitleBar's double-click interval.
const titleBarDoubleClick = 450 * time.Millisecond

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
