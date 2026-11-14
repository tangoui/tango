package tango

import (
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// Window moves for self-drawn title bars (ui.TitleBar in uihost windows):
// the page asks through corehook.StartWindowMove instead of the WebView
// frame shim's "move" message.

func init() {
	corehook.StartWindowMove = func(surface any) {
		s, ok := surface.(*Surface)
		if !ok {
			return
		}
		s.do(func(ns platform.Surface) {
			if d, ok := ns.(platform.Dragger); ok {
				d.StartMove()
			}
		})
	}
}
