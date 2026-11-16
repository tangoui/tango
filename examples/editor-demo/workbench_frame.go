package main

import (
	"runtime"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
)

func editorWindowAppearance() tango.Appearance {
	titlebar := tango.TitleBarNone
	if runtime.GOOS == "darwin" {
		titlebar = tango.TitleBarHidden
	}
	return tango.Appearance{TitleBar: titlebar}
}

// WindowFrame.CloseWindow closes directly in uihost. Keep custom chrome under
// the same unsaved-change guard as the native close button and Cmd+Q.
type guardedEditorFrame struct{ ui.WindowFrame }

func (f guardedEditorFrame) CloseWindow() {
	if confirmDiscard() {
		f.WindowFrame.CloseWindow()
	}
}

func workbenchHeaderHeight(c *ui.Context) float32 {
	return c.Theme.ControlHeight + c.Theme.PaddingXS
}
