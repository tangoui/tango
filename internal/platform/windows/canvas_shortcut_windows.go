//go:build windows && (amd64 || arm64)

package windows

import "github.com/tangoui/tango/internal/platform"

var _ platform.CanvasShortcutCapture = (*canvas)(nil)

// SetShortcutCapture is UI-thread only and never changes the IME state.
func (c *canvas) SetShortcutCapture(enabled bool) {
	if !c.closed {
		c.shortcutCapture = enabled
	}
}

func (c *canvas) capturesShortcutKey(vk uint16) bool {
	if c == nil || c.closed || (!c.shortcutCapture && !c.shortcutHeld[vk]) {
		return false
	}
	// Do not trust hasFocus: accelerator translation precedes dispatch and
	// stale queued messages may still name a canvas after focus moved.
	focus, _, _ := canvasGetFocus.Call()
	return focus == c.hwnd
}

// Captured input is raw keys, never TranslateMessage-generated text/menu
// characters. Check the native focus before either accelerator translation
// or character synthesis in the message loop.
func capturesCanvasShortcut(m *msg) bool {
	switch m.Message {
	case canvasWmKeyDown, canvasWmKeyUp, canvasWmSysKeyDown, canvasWmSysKeyUp:
		if c := canvasByHwnd[m.HWnd]; c != nil {
			return c.capturesShortcutKey(uint16(m.WParam))
		}
	}
	return false
}

func (c *canvas) holdShortcut(vk uint16) {
	if c.shortcutHeld == nil {
		c.shortcutHeld = map[uint16]bool{}
	}
	c.shortcutHeld[vk] = true
}
