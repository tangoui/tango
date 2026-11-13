package uihost

// Input-method context of mobile canvases (docs/specs/mobile-canvas.md
// §2.3, M3b). The runner reports the focused field's text, selection and
// hints through ui.TextInputStateWindow; the window forwards them to
// canvases implementing platform.CanvasTextInputState. Other canvases keep
// their context-free input method, so the call is dropped there.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
)

var _ ui.TextInputStateWindow = (*window)(nil)

// SetTextInputState implements ui.TextInputStateWindow.
func (w *window) SetTextInputState(st input.TextInputState) {
	if ts, ok := w.cv.(platform.CanvasTextInputState); ok {
		ts.SetTextInputState(st)
	}
}
