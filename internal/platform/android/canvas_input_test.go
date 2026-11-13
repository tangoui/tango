package android

import (
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// motion packs a MotionEvent the way CanvasView.pack does. fingers are
// (id, x, y, pressure); hist is (time offset, then x, y, pressure per
// finger).
func motion(action, index int, fingers [][4]float32, hist [][]float32) []float32 {
	d := []float32{float32(action), float32(index), float32(len(fingers)), float32(len(hist))}
	for _, f := range fingers {
		d = append(d, f[:]...)
	}
	for _, h := range hist {
		d = append(d, h...)
	}
	return d
}

func touches(t *testing.T, evs []input.Event) []input.TouchEvent {
	t.Helper()
	var out []input.TouchEvent
	for _, e := range evs {
		te, ok := e.(input.TouchEvent)
		if !ok {
			t.Fatalf("event %T, want TouchEvent", e)
		}
		out = append(out, te)
	}
	return out
}

func TestMotionDecode(t *testing.T) {
	base := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	d := &motionDecoder{}
	d.clock.Now = func() time.Time { return base }
	const density = 2

	// First finger down at (100, 200) px = (50, 100) dp.
	evs := touches(t, d.decode(nil, 1000, motion(amDown, 0, [][4]float32{{7, 100, 200, 0.5}}, nil), density))
	if len(evs) != 1 || evs[0].Phase != input.TouchBegan || evs[0].ID != 1 || evs[0].Pos != (geom.Point{X: 50, Y: 100}) || evs[0].Pressure != 0.5 || !evs[0].Time.Equal(base) {
		t.Fatalf("down = %+v", evs)
	}
	// Second finger.
	evs = touches(t, d.decode(nil, 1010, motion(amPointerDown, 1, [][4]float32{{7, 100, 200, 0.5}, {9, 300, 400, 2}}, nil), density))
	if len(evs) != 1 || evs[0].ID != 2 || evs[0].Phase != input.TouchBegan || evs[0].Pressure != 1 {
		t.Fatalf("pointer down = %+v", evs)
	}
	if !evs[0].Time.Equal(base.Add(10 * time.Millisecond)) {
		t.Errorf("pointer down time = %v, want +10ms", evs[0].Time)
	}
	// A move with two historical samples per finger: history first, oldest
	// first, then the current sample, finger by finger.
	hist := [][]float32{
		{-8, 102, 202, 0.5, 302, 402, 0.5},
		{-4, 104, 204, 0.5, 304, 404, 0.5},
	}
	evs = touches(t, d.decode(nil, 1020, motion(amMove, 0, [][4]float32{{7, 106, 206, 0.5}, {9, 306, 406, 0.5}}, hist), density))
	if len(evs) != 6 {
		t.Fatalf("move: %d events, want 6: %+v", len(evs), evs)
	}
	wantX := []float32{51, 52, 53, 151, 152, 153}
	for i, e := range evs {
		if e.Phase != input.TouchMoved || e.Pos.X != wantX[i] {
			t.Errorf("move event %d = %+v, want x %v", i, e, wantX[i])
		}
	}
	if !evs[0].Time.Equal(base.Add(12*time.Millisecond)) || evs[0].ID != 1 || evs[3].ID != 2 {
		t.Errorf("move: first sample %+v (want +12ms, finger 1), fourth %+v", evs[0], evs[3])
	}
	// The first finger lifts; then the system cancels the rest.
	evs = touches(t, d.decode(nil, 1030, motion(amPointerUp, 0, [][4]float32{{7, 106, 206, 0}, {9, 306, 406, 0.5}}, nil), density))
	if len(evs) != 1 || evs[0].ID != 1 || evs[0].Phase != input.TouchEnded {
		t.Fatalf("pointer up = %+v", evs)
	}
	evs = touches(t, d.decode(nil, 1040, motion(amCancel, 0, [][4]float32{{9, 306, 406, 0.5}}, nil), density))
	if len(evs) != 1 || evs[0].ID != 2 || evs[0].Phase != input.TouchCancelled {
		t.Fatalf("cancel = %+v", evs)
	}
	if d.touches.Len() != 0 {
		t.Errorf("%d fingers still down", d.touches.Len())
	}
}

func TestMotionDecodeMalformed(t *testing.T) {
	d := &motionDecoder{}
	for _, data := range [][]float32{
		nil,
		{amDown, 0},
		{amDown, 0, 2, 0, 1, 2, 3, 4},  // says two fingers, has one
		{amMove, 0, 1, 3, 1, 2, 3, 4},  // says three history samples, has none
		{amDown, 5, 1, 0, 1, 2, 3, 4},  // action index out of range
		{amDown, 0, -1, 0, 1, 2, 3, 4}, // negative count
	} {
		if evs := d.decode(nil, 0, data, 1); len(evs) != 0 {
			t.Errorf("decode(%v) = %+v, want nothing", data, evs)
		}
	}
}

func TestMotionCancelAll(t *testing.T) {
	d := &motionDecoder{}
	d.decode(nil, 0, motion(amDown, 0, [][4]float32{{1, 10, 10, 1}}, nil), 1)
	evs := touches(t, d.cancelAll(nil))
	if len(evs) != 1 || evs[0].Phase != input.TouchCancelled || evs[0].Pos != (geom.Point{X: 10, Y: 10}) {
		t.Errorf("cancelAll = %+v", evs)
	}
	if evs := d.cancelAll(nil); len(evs) != 0 {
		t.Errorf("second cancelAll = %+v", evs)
	}
}

func TestPointerEvent(t *testing.T) {
	ev, ok := pointerEvent(ptrPress, 20, 40, abSecondary, akMetaShiftOn, 0, 0, 2)
	if p, _ := ev.(input.Pointer); !ok || p.Kind != input.Press || p.Button != input.ButtonRight || p.Pos != (geom.Point{X: 10, Y: 20}) || p.Mods != input.ModShift {
		t.Errorf("press = %+v", ev)
	}
	ev, _ = pointerEvent(ptrRelease, 0, 0, abTertiary, 0, 0, 0, 1)
	if p := ev.(input.Pointer); p.Kind != input.Release || p.Button != input.ButtonMiddle {
		t.Errorf("release = %+v", p)
	}
	ev, _ = pointerEvent(ptrScroll, 10, 10, 0, 0, 0, 96, 2)
	if s, _ := ev.(input.Scroll); s.DY != 48 || s.DX != 0 || s.Pos != (geom.Point{X: 5, Y: 5}) {
		t.Errorf("scroll = %+v", ev)
	}
	for k, want := range map[int]input.PointerKind{ptrMove: input.Move, ptrLeave: input.Leave, ptrCancel: input.Cancel} {
		if ev, _ := pointerEvent(k, 0, 0, 0, 0, 0, 0, 1); ev.(input.Pointer).Kind != want {
			t.Errorf("kind %d = %+v", k, ev)
		}
	}
	if _, ok := pointerEvent(99, 0, 0, 0, 0, 0, 0, 1); ok {
		t.Error("unknown kind delivered")
	}
}

func TestSchedAction(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		a    mobilecanvas.Action
		want int64
	}{
		{mobilecanvas.Action{}, 0},
		{mobilecanvas.Action{Link: mobilecanvas.LinkResume}, 1},
		{mobilecanvas.Action{Link: mobilecanvas.LinkPause}, 2},
		{mobilecanvas.Action{Arm: true}, 4}, // cancel the timer
		{mobilecanvas.Action{Link: mobilecanvas.LinkResume, Arm: true, At: now.Add(250 * time.Millisecond)}, 1 | 4 | (5250 << 4)},
		{mobilecanvas.Action{Arm: true, At: now.Add(1500 * time.Microsecond)}, 4 | (5002 << 4)}, // rounded up
		{mobilecanvas.Action{Arm: true, At: now.Add(-time.Hour)}, 4 | (1 << 4)},                 // past: as soon as possible
	} {
		if got := schedAction(tc.a, 5000, now); got != tc.want {
			t.Errorf("schedAction(%+v) = %#x, want %#x", tc.a, got, tc.want)
		}
	}
}

func TestMergeActions(t *testing.T) {
	at := time.Unix(100, 0)
	a := mobilecanvas.Action{Link: mobilecanvas.LinkResume, Arm: true, At: at}
	b := mobilecanvas.Action{Frame: true}
	if m := mergeActions(a, b); m.Link != mobilecanvas.LinkResume || !m.Arm || !m.At.Equal(at) || !m.Frame {
		t.Errorf("merge keep = %+v", m)
	}
	c := mobilecanvas.Action{Link: mobilecanvas.LinkPause, Arm: true}
	if m := mergeActions(a, c); m.Link != mobilecanvas.LinkPause || !m.Arm || !m.At.IsZero() {
		t.Errorf("merge override = %+v", m)
	}
}
