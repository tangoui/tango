//go:build windows && (amd64 || arm64)

package windows

// Touch input of the canvas window (docs/specs/touch-input.md). Windows 8
// and later send WM_POINTERDOWN / UPDATE / UP for touch and pen contacts
// to every window (mouse input keeps arriving as WM_MOUSE*, since
// EnableMouseInPointer is not called). For touch pointers (PT_TOUCH) the
// canvas reports input.TouchEvent and returns 0 without DefWindowProc:
// that is what stops Windows from synthesizing mouse messages and
// gestures for the contact, so the runner's own pointer emulation
// (first finger -> input.Pointer) is the only one. Pen pointers go to
// DefWindowProc and arrive as mouse messages, as before. Mouse messages
// that still carry the touch signature in GetMessageExtraInfo (WM_TOUCH
// legacy paths, press-and-hold) are dropped while touch forwarding works.
//
// Positions come in screen pixels (per-monitor DPI aware), are mapped to
// the client area with ScreenToClient and divided by the scale. Pressure
// comes from GetPointerTouchInfo; time from GetMessageTime. A finger
// whose capture is lost (WM_POINTERCAPTURECHANGED) or that is cancelled
// by the system (POINTER_MESSAGE_FLAG_CANCELED on up) is reported as
// TouchCancelled; WM_CANCELMODE (the system cancelling modes, e.g. a
// dialog or menu taking over) cancels every finger still down.

import (
	"time"
	"unsafe"

	"github.com/tangoui/tango/ui/input"
)

var (
	canvasGetPointerType      = user32.NewProc("GetPointerType")      // Windows 8+
	canvasGetPointerTouchInfo = user32.NewProc("GetPointerTouchInfo") // Windows 8+
	canvasGetMessageExtraInfo = user32.NewProc("GetMessageExtraInfo")
	canvasGetTickCount        = kernel32.NewProc("GetTickCount")
)

// canvasPointerTouchInfo is POINTER_TOUCH_INFO (POINTER_INFO first) on
// 64-bit Windows.
type canvasPointerTouchInfo struct {
	PointerType        uint32
	PointerID          uint32
	FrameID            uint32
	PointerFlags       uint32
	SourceDevice       uintptr
	HwndTarget         uintptr
	PtPixelLocation    point
	PtHimetricLocation point
	PtPixelLocationRaw point
	PtHimetricRaw      point
	Time               uint32
	HistoryCount       uint32
	InputData          int32
	KeyStates          uint32
	PerformanceCount   uint64
	ButtonChangeType   int32
	_                  int32
	TouchFlags         uint32
	TouchMask          uint32
	RcContact          rect
	RcContactRaw       rect
	Orientation        uint32
	Pressure           uint32
}

var _ = [1]struct{}{}[unsafe.Sizeof(canvasPointerTouchInfo{})-144]

// touchSupported reports whether the WM_POINTER functions exist.
func touchSupported() bool { return canvasGetPointerType.Available() }

// touchMouse reports whether the mouse message being handled was
// synthesized from touch and must be dropped.
func (c *canvas) touchMouse() bool {
	if !touchSupported() {
		return false
	}
	extra, _, _ := canvasGetMessageExtraInfo.Call()
	return canvasIsTouchMouse(extra)
}

// pointerMessage handles WM_POINTERDOWN / UPDATE / UP / CAPTURECHANGED.
// It returns true when the message belongs to a touch pointer (handled:
// no DefWindowProc, so no synthesized mouse input).
func (c *canvas) pointerMessage(m uint32, wp, lp uintptr) bool {
	if !touchSupported() {
		return false
	}
	id := uint32(loword(wp))
	flags := uint16(hiword(wp))
	if m != canvasWmPointerCaptureChanged {
		var typ uint32
		if r, _, _ := canvasGetPointerType.Call(uintptr(id), uintptr(unsafe.Pointer(&typ))); r == 0 || typ != canvasPTTouch {
			return false
		}
	}
	pt := point{X: int32(int16(loword(lp))), Y: int32(int16(hiword(lp)))}
	canvasScreenToClient.Call(c.hwnd, uintptr(unsafe.Pointer(&pt)))
	ph, pos, ok := c.touches.update(m, id, flags, c.toLogical(pt.X, pt.Y))
	if !ok {
		return m != canvasWmPointerCaptureChanged // a touch message we ignore is still ours
	}
	if ph == input.TouchBegan {
		if f, _, _ := canvasGetFocus.Call(); f != c.hwnd {
			canvasSetFocus.Call(c.hwnd)
		}
	}
	ev := input.TouchEvent{ID: int64(id), Phase: ph, Pos: pos, Time: c.messageTime()}
	if ph == input.TouchBegan || ph == input.TouchMoved {
		var info canvasPointerTouchInfo
		if r, _, _ := canvasGetPointerTouchInfo.Call(uintptr(id), uintptr(unsafe.Pointer(&info))); r != 0 {
			ev.Pressure = canvasTouchPressure(info.Pressure, info.TouchMask)
		}
	}
	c.input(ev)
	return true
}

// messageTime is the wall time of the message being handled.
func (c *canvas) messageTime() time.Time {
	t, _, _ := canvasGetMessageTime.Call()
	now, _, _ := canvasGetTickCount.Call()
	return canvasMessageTime(time.Now(), uint32(now), uint32(t))
}

// cancelTouches reports TouchCancelled for every finger still down.
func (c *canvas) cancelTouches() {
	for _, ev := range c.touches.cancelAll(time.Now()) {
		c.input(ev)
	}
}
