package linux

import (
	"testing"
	"unsafe"

	"github.com/tangoui/tango/ui/input"
)

// canvasCGdkEventTouch mirrors GdkEventTouch (LP64).
type canvasCGdkEventTouch struct {
	typ              int32
	window           uintptr
	sendEvent        int8
	time             uint32
	x, y             float64
	axes             uintptr
	state            uint32
	sequence         uintptr
	emulatingPointer int32
	device           uintptr
	xRoot, yRoot     float64
}

func TestCanvasTouchOffsets(t *testing.T) {
	var e canvasCGdkEventTouch
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"time", unsafe.Offsetof(e.time), canvasEvTouchTime},
		{"x", unsafe.Offsetof(e.x), canvasEvTouchX},
		{"y", unsafe.Offsetof(e.y), canvasEvTouchY},
		{"sequence", unsafe.Offsetof(e.sequence), canvasEvTouchSequence},
	} {
		if c.got != c.want {
			t.Errorf("touch.%s at %d, constant %d", c.name, c.got, c.want)
		}
	}
}

func TestCanvasTouchPhase(t *testing.T) {
	for _, c := range []struct {
		typ  uint32
		want input.TouchPhase
		ok   bool
	}{
		{canvasGdkTouchBegin, input.TouchBegan, true},
		{canvasGdkTouchUpdate, input.TouchMoved, true},
		{canvasGdkTouchEnd, input.TouchEnded, true},
		{canvasGdkTouchCancel, input.TouchCancelled, true},
		{canvasGdkButtonPress, 0, false},
	} {
		if ph, ok := canvasTouchPhase(c.typ); ok != c.ok || ph != c.want {
			t.Errorf("type %d: %v %v", c.typ, ph, ok)
		}
	}
}

func TestCanvasTouchPointer(t *testing.T) {
	if !canvasTouchPointer(true, -1) || !canvasTouchPointer(false, canvasGdkSourceTouchscreen) {
		t.Fatal("emulated pointer events kept")
	}
	if canvasTouchPointer(false, 0) || canvasTouchPointer(false, canvasGdkSourceTouchpad) || canvasTouchPointer(false, -1) {
		t.Fatal("real pointer events dropped")
	}
}
