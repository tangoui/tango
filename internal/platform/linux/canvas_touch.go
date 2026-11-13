package linux

// Platform-independent part of touch input on the canvas widget
// (canvas_touch_linux.go): GDK touch event types and layout, and which
// pointer events are emulated from touch.

import "github.com/tangoui/tango/ui/input"

// GDK_TOUCH_MASK (gdktypes.h).
const canvasGdkTouchMask = 1 << 22

// GdkEventType values of touch events (gdkevents.h).
const (
	canvasGdkTouchBegin  = 37
	canvasGdkTouchUpdate = 38
	canvasGdkTouchEnd    = 39
	canvasGdkTouchCancel = 40
)

// GDK_SOURCE_TOUCHSCREEN (gdkdevice.h).
const canvasGdkSourceTouchscreen = 5

// Byte offsets in GdkEventTouch { any; guint32 time; gdouble x, y;
// gdouble *axes; guint state; GdkEventSequence *sequence; gboolean
// emulating_pointer; GdkDevice *device; gdouble x_root, y_root; } on
// LP64 targets (checked in canvas_touch_test.go).
const (
	canvasEvTouchTime     = 20
	canvasEvTouchX        = 24
	canvasEvTouchY        = 32
	canvasEvTouchSequence = 56
)

// canvasTouchPhase maps a GDK touch event type to a TouchEvent phase.
func canvasTouchPhase(evType uint32) (input.TouchPhase, bool) {
	switch evType {
	case canvasGdkTouchBegin:
		return input.TouchBegan, true
	case canvasGdkTouchUpdate:
		return input.TouchMoved, true
	case canvasGdkTouchEnd:
		return input.TouchEnded, true
	case canvasGdkTouchCancel:
		return input.TouchCancelled, true
	}
	return 0, false
}

// canvasTouchPointer reports whether a pointer event (button, motion,
// crossing) is GDK's emulation of a touch, which the canvas drops because
// the touch itself is reported (the runner emulates the pointer). emulated
// is gdk_event_get_pointer_emulated (false when unknown); source is the
// GdkInputSource of the event's source device, or -1 when unknown.
func canvasTouchPointer(emulated bool, source int32) bool {
	return emulated || source == canvasGdkSourceTouchscreen
}
