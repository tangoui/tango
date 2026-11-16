//go:build darwin && !ios

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasShortcutCapture = (*canvas)(nil)

// SetShortcutCapture opts out of native menu equivalents while this exact
// canvas is the key window's first responder. It does not enable the IME.
func (c *canvas) SetShortcutCapture(enabled bool) {
	if !c.closed {
		c.shortcutCapture = enabled
	}
}

func (c *canvas) capturesShortcutEvent(ev id) bool {
	if c.closed || c.view == 0 || (!c.shortcutCapture && !(sendBool(ev, "isARepeat") && c.shortcutHeld[uint16(send(ev, "keyCode"))])) {
		return false
	}
	win := send(c.view, "window")
	return win != 0 && sendBool(win, "isKeyWindow") && send(win, "firstResponder") == c.view
}

var shortcutReleaseMonitor objc.Block

// AppKit drops Command-modified keyups before NSWindow sees them. A local
// (not global) monitor returns captured releases to their focused canvas.
// Without this, releasing Cmd+K would leave the repeat latch stuck forever.
func installShortcutReleaseMonitor() {
	shortcutReleaseMonitor = objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		ev := id(event)
		win := send(theBackend.app, "keyWindow")
		c := canvasOf(send(win, "firstResponder"))
		if c != nil && c.shortcutHeld[uint16(send(ev, "keyCode"))] {
			send(c.view, "keyUp:", uintptr(ev))
			return 0
		}
		return event
	})
	send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", 1<<11, uintptr(shortcutReleaseMonitor))
}

var shortcutSuperOnce sync.Once
var msgShortcutSuper func(*objcSuper, objc.SEL, id) bool

// The explicit defining class matters for inherited methods on overlays:
// using the runtime class would recurse back into this implementation.
func shortcutEquivalentMethod(definingClass string) objc.MethodDef {
	shortcutSuperOnce.Do(func() { purego.RegisterFunc(&msgShortcutSuper, mustDlsym(libObjC, "objc_msgSendSuper2")) })
	return method("performKeyEquivalent:", func(self id, _ objc.SEL, ev id) bool {
		view := self
		if definingClass != "TangoCanvasView" {
			view = send(self, "firstResponder")
		}
		if c := canvasOf(view); c != nil && c.capturesShortcutEvent(ev) {
			c.keyDown(ev)
			return true
		}
		return msgShortcutSuper(&objcSuper{receiver: self, class: class(definingClass)}, sel("performKeyEquivalent:"), ev)
	})
}
