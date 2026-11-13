package windows

// Platform-independent part of touch input on the canvas window
// (canvas_touch_windows.go): WM_POINTER decoding for touch pointers, the
// per-finger state, pressure and time conversion, and recognising the
// mouse messages Windows synthesizes from touch.

import (
	"slices"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// WM_POINTER messages and flags (winuser.h).
const (
	canvasWmPointerUpdate         = 0x0245
	canvasWmPointerDown           = 0x0246
	canvasWmPointerUp             = 0x0247
	canvasWmPointerCaptureChanged = 0x024C

	canvasPTTouch = 2 // POINTER_INPUT_TYPE PT_TOUCH

	// POINTER_MESSAGE_FLAG_* in the high word of wParam.
	canvasPointerFlagInContact = 0x0004
	canvasPointerFlagCanceled  = 0x8000

	canvasTouchMaskPressure = 0x00000004 // TOUCH_MASK_PRESSURE

	// GetMessageExtraInfo of mouse messages synthesized from pen or touch:
	// MI_WP_SIGNATURE in the bits of SIGNATURE_MASK, and 0x80 set for touch.
	canvasMIWPSignature   = 0xFF515700
	canvasMISignatureMask = 0xFFFFFF00
	canvasMITouchBit      = 0x80
)

// canvasIsTouchMouse reports whether a mouse message with this
// GetMessageExtraInfo value was synthesized from touch. Pen-generated
// mouse messages (no touch bit) are real pointer input and kept.
func canvasIsTouchMouse(extra uintptr) bool {
	e := uint32(extra)
	return e&canvasMISignatureMask == canvasMIWPSignature && e&canvasMITouchBit != 0
}

// canvasTouches tracks the touch pointers that went down on the canvas.
// UI thread.
type canvasTouches struct {
	pos map[uint32]geom.Point // active pointer id -> last logical position
}

// update turns a WM_POINTER message of a touch pointer (id and the
// POINTER_MESSAGE_FLAG_* bits from wParam) at logical position p into a
// TouchEvent phase. ok is false when nothing is reported: an update or up
// of a pointer that did not go down here, a hover update without contact,
// or a capture change of a pointer that is not active. For
// WM_POINTERCAPTURECHANGED p is ignored and the finger's last position
// returned.
func (t *canvasTouches) update(m uint32, id uint32, flags uint16, p geom.Point) (ph input.TouchPhase, pos geom.Point, ok bool) {
	if t.pos == nil {
		t.pos = map[uint32]geom.Point{}
	}
	last, active := t.pos[id]
	switch m {
	case canvasWmPointerDown:
		t.pos[id] = p
		if active { // a lost up: the pointer is still the same finger
			return input.TouchMoved, p, true
		}
		return input.TouchBegan, p, true
	case canvasWmPointerUpdate:
		if !active || flags&canvasPointerFlagInContact == 0 {
			return 0, p, false
		}
		t.pos[id] = p
		return input.TouchMoved, p, true
	case canvasWmPointerUp:
		if !active {
			return 0, p, false
		}
		delete(t.pos, id)
		if flags&canvasPointerFlagCanceled != 0 {
			return input.TouchCancelled, p, true
		}
		return input.TouchEnded, p, true
	case canvasWmPointerCaptureChanged:
		if !active {
			return 0, p, false
		}
		delete(t.pos, id)
		return input.TouchCancelled, last, true
	}
	return 0, p, false
}

// cancelAll forgets every active finger and returns TouchCancelled events
// for them, in id order.
func (t *canvasTouches) cancelAll(now time.Time) []input.TouchEvent {
	ids := make([]uint32, 0, len(t.pos))
	for id := range t.pos {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	evs := make([]input.TouchEvent, 0, len(ids))
	for _, id := range ids {
		evs = append(evs, input.TouchEvent{ID: int64(id), Phase: input.TouchCancelled, Pos: t.pos[id], Time: now})
		delete(t.pos, id)
	}
	return evs
}

// canvasTouchPressure normalizes POINTER_TOUCH_INFO.pressure (0..1024,
// valid when touchMask has TOUCH_MASK_PRESSURE) so that a normal press
// (512) is 1, as TouchEvent.Pressure wants; 0 when not reported.
func canvasTouchPressure(pressure, mask uint32) float32 {
	if mask&canvasTouchMaskPressure == 0 {
		return 0
	}
	return min(float32(pressure)/512, 1)
}

// canvasMessageTime converts a message time (GetMessageTime, milliseconds
// of the GetTickCount clock, wrapping at 2^32) to wall time, given the
// tick count now. Times in the future or more than a minute old (a stale
// or bogus value) become now.
func canvasMessageTime(now time.Time, tickNow, msgTick uint32) time.Time {
	age := tickNow - msgTick // modular: correct across the wrap
	if age > 60_000 {
		return now
	}
	return now.Add(-time.Duration(age) * time.Millisecond)
}

// canvasPhysToLogical converts a client position in physical pixels to
// logical pixels at scale (the canvas's toLogical).
func canvasPhysToLogical(x, y int32, scale float64) geom.Point {
	return geom.Pt(float32(float64(x)/scale), float32(float64(y)/scale))
}
