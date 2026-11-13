package windows

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestCanvasIsTouchMouse(t *testing.T) {
	for _, c := range []struct {
		extra uintptr
		want  bool
	}{
		{0, false},
		{0xFF515780, true},  // touch
		{0xFF515781, true},  // touch, other low bits
		{0xFF515700, false}, // pen
		{0xFF515600 | 0x80, false},
		{0x12345680, false},
	} {
		if got := canvasIsTouchMouse(c.extra); got != c.want {
			t.Errorf("%#x: %v, want %v", c.extra, got, c.want)
		}
	}
}

func TestCanvasTouchesLifecycle(t *testing.T) {
	var tr canvasTouches
	p := func(x float32) geom.Point { return geom.Pt(x, x) }
	type step struct {
		m     uint32
		id    uint32
		flags uint16
		at    geom.Point
		ph    input.TouchPhase
		pos   geom.Point
		ok    bool
	}
	steps := []step{
		{canvasWmPointerUpdate, 1, canvasPointerFlagInContact, p(1), 0, p(1), false}, // not down here
		{canvasWmPointerDown, 1, canvasPointerFlagInContact, p(2), input.TouchBegan, p(2), true},
		{canvasWmPointerUpdate, 1, 0, p(3), 0, p(3), false}, // no contact
		{canvasWmPointerUpdate, 1, canvasPointerFlagInContact, p(4), input.TouchMoved, p(4), true},
		{canvasWmPointerDown, 2, canvasPointerFlagInContact, p(5), input.TouchBegan, p(5), true},
		{canvasWmPointerUp, 1, 0, p(6), input.TouchEnded, p(6), true},
		{canvasWmPointerUp, 1, 0, p(6), 0, p(6), false}, // already up
		{canvasWmPointerCaptureChanged, 2, 0, p(99), input.TouchCancelled, p(5), true},
		{canvasWmPointerCaptureChanged, 2, 0, p(99), 0, p(99), false},
		{canvasWmPointerDown, 3, canvasPointerFlagInContact, p(7), input.TouchBegan, p(7), true},
		{canvasWmPointerUp, 3, canvasPointerFlagCanceled, p(8), input.TouchCancelled, p(8), true},
	}
	for i, s := range steps {
		ph, pos, ok := tr.update(s.m, s.id, s.flags, s.at)
		if ok != s.ok || (ok && (ph != s.ph || pos != s.pos)) {
			t.Fatalf("step %d: %v %v %v, want %v %v %v", i, ph, pos, ok, s.ph, s.pos, s.ok)
		}
	}
	if len(tr.pos) != 0 {
		t.Fatalf("fingers left: %v", tr.pos)
	}
	tr.update(canvasWmPointerDown, 9, canvasPointerFlagInContact, p(1))
	tr.update(canvasWmPointerDown, 4, canvasPointerFlagInContact, p(2))
	now := time.Unix(100, 0)
	evs := tr.cancelAll(now)
	if len(evs) != 2 || evs[0].ID != 4 || evs[1].ID != 9 || evs[0].Phase != input.TouchCancelled ||
		evs[1].Pos != p(1) || !evs[0].Time.Equal(now) || len(tr.pos) != 0 {
		t.Fatalf("cancelAll %+v", evs)
	}
}

func TestCanvasTouchPressure(t *testing.T) {
	if got := canvasTouchPressure(512, 0); got != 0 {
		t.Fatalf("unreported pressure %v", got)
	}
	if got := canvasTouchPressure(256, canvasTouchMaskPressure); got != 0.5 {
		t.Fatalf("256: %v", got)
	}
	if got := canvasTouchPressure(1024, canvasTouchMaskPressure); got != 1 {
		t.Fatalf("1024: %v", got)
	}
}

func TestCanvasMessageTime(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := canvasMessageTime(now, 5000, 4990); !got.Equal(now.Add(-10 * time.Millisecond)) {
		t.Fatalf("plain: %v", got)
	}
	if got := canvasMessageTime(now, 5, 0xFFFFFFFB); !got.Equal(now.Add(-10 * time.Millisecond)) {
		t.Fatalf("wrap: %v", got)
	}
	if got := canvasMessageTime(now, 100, 200); !got.Equal(now) { // in the future
		t.Fatalf("future: %v", got)
	}
}

func TestCanvasPhysToLogical(t *testing.T) {
	if got := canvasPhysToLogical(150, 75, 1.5); got != geom.Pt(100, 50) {
		t.Fatalf("got %v", got)
	}
}
