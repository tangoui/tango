package input

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
)

// TouchPhase is the stage of one finger's contact.
//
// Append-only: never reorder or insert; the zero value stays TouchBegan.
type TouchPhase uint8

const (
	TouchBegan     TouchPhase = iota // finger went down
	TouchMoved                       // finger moved (or its pressure changed)
	TouchEnded                       // finger lifted
	TouchCancelled                   // the system took the touch away (incoming call, OS gesture)
)

func (p TouchPhase) String() string {
	switch p {
	case TouchBegan:
		return "began"
	case TouchMoved:
		return "moved"
	case TouchEnded:
		return "ended"
	case TouchCancelled:
		return "cancelled"
	}
	return "TouchPhase(" + itoa(int(p)) + ")"
}

// TouchEvent is one change of one finger on a touch screen. A frame may
// carry several (one per finger that changed); hosts send them in the
// order the platform reported them.
//
// ID identifies the finger from TouchBegan to TouchEnded / TouchCancelled
// and must not be reused while the finger is down (UITouch pointer,
// Android pointer ID, PointerEvent.pointerId). Pos is in logical pixels
// relative to the content area, like Pointer.Pos. Pressure is normalized
// to 0..1 (1 = a typical firm press); 0 means the device does not report
// it. Time is the platform timestamp of the change; zero means unknown
// and the runner uses the frame time.
//
// The runner synthesizes Pointer events (Move + Press, Move, Release) from
// the first finger of a gesture so widgets without touch handling keep
// working; see Runner.SetTouchPointerEmulation. Hosts must therefore not
// send their own emulated mouse events for touches they report here.
type TouchEvent struct {
	ID       int64
	Phase    TouchPhase
	Pos      geom.Point
	Pressure float32
	Time     time.Time
}

func (TouchEvent) isEvent() {}

// Done reports whether the finger left the screen with this event.
func (e TouchEvent) Done() bool { return e.Phase == TouchEnded || e.Phase == TouchCancelled }
