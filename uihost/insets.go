package uihost

// Safe-area and soft-keyboard insets of mobile canvases
// (docs/specs/mobile-canvas.md §2.4, §3.1). The window forwards them from
// the canvas when it implements platform.CanvasInsets; the runner polls
// them every frame (ui.SafeAreaWindow, ui.KeyboardInsetWindow). Desktop
// canvases do not implement it, so both stay zero there.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

var (
	_ ui.SafeAreaWindow      = (*window)(nil)
	_ ui.KeyboardInsetWindow = (*window)(nil)
)

// SafeAreaInsets implements ui.SafeAreaWindow.
func (w *window) SafeAreaInsets() geom.Insets {
	if ci, ok := w.cv.(platform.CanvasInsets); ok {
		return ci.SafeAreaInsets()
	}
	return geom.Insets{}
}

// KeyboardInset implements ui.KeyboardInsetWindow.
func (w *window) KeyboardInset() geom.Insets {
	if ci, ok := w.cv.(platform.CanvasInsets); ok {
		return ci.KeyboardInset()
	}
	return geom.Insets{}
}
