//go:build linux && !android

package linux

import "github.com/tangoui/tango/internal/platform"

var _ platform.CanvasShortcutCapture = (*canvas)(nil)

// SetShortcutCapture is independent of text input and only takes effect
// while this widget holds actual native keyboard focus. Main thread.
func (c *canvas) SetShortcutCapture(enabled bool) {
	if !c.closed {
		c.shortcutCapture = enabled
	}
}

func (c *canvas) capturesShortcutEvent(ev ptr) bool {
	return c != nil && !c.closed && (c.shortcutCapture || c.shortcutHeld[canvasEvU16(ev, canvasEvKeyHWCode)]) && canvasFFI.widgetHasFocus(c.widget)
}

// GtkWindow's RUN_LAST default key handler tries menu accelerators before
// forwarding to its focus widget. A normal (not CONNECT_AFTER) window
// handler must claim capture events before that default handler runs.
func (s *surface) captureShortcut(ev ptr, press bool) ptr {
	if s.closed {
		return 0
	}
	c := s.canvas
	if !c.capturesShortcutEvent(ev) {
		c = nil
		if host := attachHosts[s]; host != nil {
			for _, attached := range host.list {
				if attached.canvas.capturesShortcutEvent(ev) {
					c = attached.canvas
					break
				}
			}
		}
	}
	if c == nil {
		return 0
	}
	hw := canvasEvU16(ev, canvasEvKeyHWCode)
	if press {
		if c.shortcutHeld == nil {
			c.shortcutHeld = map[uint16]bool{}
		}
		c.shortcutHeld[hw] = true
	}
	c.onKey(ev, press)
	if !press {
		delete(c.shortcutHeld, hw)
	}
	// Claim modifiers/unknown keys too; onKey intentionally returns false
	// for modifiers outside capture so GTK normally retains its defaults.
	return 1
}
