package uihost

// The system Back action (docs/specs/mobile-canvas.md §2.2): the runner
// tells the window whether the UI has something Back should close
// (ui.BackWindow, ui.Context.WantBack) and the window passes it on to
// canvases that implement platform.CanvasBackHandler (Android). While on,
// the platform delivers Back as KeyEscape, which closes the top-most
// overlay. Desktop canvases do not implement it; the call does nothing.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
)

var _ ui.BackWindow = (*window)(nil)

// SetBackHandling implements ui.BackWindow.
func (w *window) SetBackHandling(on bool) {
	if bh, ok := w.cv.(platform.CanvasBackHandler); ok {
		bh.SetBackHandling(on)
	}
}
