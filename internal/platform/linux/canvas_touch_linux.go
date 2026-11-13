//go:build linux && !android

package linux

// Touch input of the canvas widget (docs/specs/touch-input.md). The widget
// selects GDK_TOUCH_MASK, so GTK delivers "touch-event" with
// GDK_TOUCH_BEGIN / UPDATE / END / CANCEL for each finger; each becomes an
// input.TouchEvent. ID is the GdkEventSequence pointer, which is unique
// while the finger is down. Positions are already logical: GTK 3 event
// coordinates are in application pixels (the integer scale factor is
// applied by GDK), relative to the widget's window, like the pointer
// events. Pressure comes from the event's GDK_AXIS_PRESSURE when the
// device reports one (0 otherwise); the time is left zero (GDK event
// times are server milliseconds, not wall time), so the runner uses the
// frame time.
//
// Selecting touch events stops the X server from emulating the pointer
// for this window (XInput 2), but GDK itself may still emulate pointer
// events from the first touch (Wayland). Pointer events marked as emulated
// (gdk_event_get_pointer_emulated, GTK 3.22+) or coming from a touchscreen
// source device are dropped, so the runner's pointer emulation is the only
// one.

import (
	"unsafe"

	"github.com/tangoui/tango/ui/input"
)

// gdkAxisPressure is GDK_AXIS_PRESSURE.
const gdkAxisPressure = 5

// onTouch handles "touch-event".
func (c *canvas) onTouch(_, ev ptr) ptr {
	if c.closed {
		return 0
	}
	ph, ok := canvasTouchPhase(canvasEvU32(ev, canvasEvTypeOffset))
	if !ok {
		return 0
	}
	seq := *(*uintptr)(unsafe.Add(toPtr(ev), canvasEvTouchSequence))
	te := input.TouchEvent{ID: int64(seq), Phase: ph, Pos: canvasEvPoint(ev, canvasEvTouchX, canvasEvTouchY)}
	if f := canvasFFI.eventGetAxis; f != nil && (ph == input.TouchBegan || ph == input.TouchMoved) {
		var v float64
		if f(ev, gdkAxisPressure, &v) {
			te.Pressure = float32(min(max(v, 0), 1))
		}
	}
	if ph == input.TouchBegan && !canvasFFI.widgetHasFocus(c.widget) {
		canvasFFI.widgetGrabFocus(c.widget)
	}
	c.input(te)
	return 1
}

// touchPointer reports whether a pointer event is emulated from touch
// (see the file comment) and must be dropped.
func (c *canvas) touchPointer(ev ptr) bool {
	f := &canvasFFI
	emulated := f.eventGetPointerEmulated != nil && f.eventGetPointerEmulated(ev)
	source := int32(-1)
	if f.deviceGetSource != nil {
		if dev := f.eventGetSourceDevice(ev); dev != 0 {
			source = f.deviceGetSource(dev)
		}
	}
	return canvasTouchPointer(emulated, source)
}
