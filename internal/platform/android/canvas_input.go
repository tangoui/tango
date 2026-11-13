package android

import (
	"time"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Motion events of the Android canvas (docs/specs/mobile-canvas.md §2.1).
//
// CanvasView.java packs a touch-screen MotionEvent into one float array so
// a whole event, historical points included, crosses JNI in one call:
//
//	[0] actionMasked  [1] actionIndex  [2] pointer count n  [3] history size h
//	then n × (pointer id, x, y, pressure)                 x, y in device pixels
//	then h × (time − event time in ms, then n × (x, y, pressure))
//
// Mouse, hover and wheel events (TOOL_TYPE_MOUSE, ACTION_HOVER_*,
// ACTION_SCROLL) come one at a time through nativePointer instead.

// MotionEvent.ACTION_* values.
const (
	amDown        = 0
	amUp          = 1
	amMove        = 2
	amCancel      = 3
	amPointerDown = 5
	amPointerUp   = 6
)

const (
	motionHeader    = 4
	motionPerFinger = 4 // id, x, y, pressure
	motionPerSample = 3 // x, y, pressure
)

// motionDecoder turns packed motion events into input.TouchEvent values.
// Main thread.
type motionDecoder struct {
	touches mobilecanvas.Touches[int32]
	clock   mobilecanvas.Clock

	hist []mobilecanvas.Sample // scratch
	evs  []input.TouchEvent    // scratch
}

// decode appends the events of one packed MotionEvent to dst. eventMs is
// MotionEvent.getEventTime() (uptime milliseconds); density converts device
// pixels to logical pixels. Malformed input is ignored.
func (d *motionDecoder) decode(dst []input.Event, eventMs int64, data []float32, density float32) []input.Event {
	if len(data) < motionHeader {
		return dst
	}
	action, index, n, h := int(data[0]), int(data[1]), int(data[2]), int(data[3])
	if n < 0 || h < 0 || len(data) < motionHeader+n*motionPerFinger+h*(1+n*motionPerSample) {
		return dst
	}
	at := d.clock.Time(mobilecanvas.Millis(eventMs))
	finger := func(i int) (int32, mobilecanvas.Sample) {
		o := motionHeader + i*motionPerFinger
		return int32(data[o]), mobilecanvas.Sample{
			Pos:      geom.Point{X: mobilecanvas.Logical(data[o+1], density), Y: mobilecanvas.Logical(data[o+2], density)},
			Pressure: mobilecanvas.Pressure(data[o+3], 1),
			Time:     at,
		}
	}
	evs := d.evs[:0]
	switch action {
	case amDown, amPointerDown:
		if index >= 0 && index < n {
			id, s := finger(index)
			evs = d.touches.Begin(evs, id, s)
		}
	case amUp, amPointerUp:
		if index >= 0 && index < n {
			id, s := finger(index)
			evs = d.touches.End(evs, id, s)
		}
	case amMove:
		histBase := motionHeader + n*motionPerFinger
		stride := 1 + n*motionPerSample
		for i := range n {
			id, cur := finger(i)
			d.hist = d.hist[:0]
			for k := range h {
				o := histBase + k*stride
				so := o + 1 + i*motionPerSample
				d.hist = append(d.hist, mobilecanvas.Sample{
					Pos:      geom.Point{X: mobilecanvas.Logical(data[so], density), Y: mobilecanvas.Logical(data[so+1], density)},
					Pressure: mobilecanvas.Pressure(data[so+2], 1),
					Time:     at.Add(mobilecanvas.Millis(int64(data[o]))),
				})
			}
			evs = d.touches.Move(evs, id, d.hist, cur)
		}
	case amCancel:
		evs = d.touches.CancelAll(evs, at)
	}
	for _, e := range evs {
		dst = append(dst, e)
	}
	d.evs = evs[:0]
	return dst
}

// cancelAll cancels every finger still down (the window lost focus or
// the surface went away).
func (d *motionDecoder) cancelAll(dst []input.Event) []input.Event {
	evs := d.touches.CancelAll(d.evs[:0], time.Time{}) // time unknown: the runner uses the frame time
	for _, e := range evs {
		dst = append(dst, e)
	}
	d.evs = evs[:0]
	return dst
}

// Pointer kinds of nativePointer (CanvasView.java).
const (
	ptrMove    = 0
	ptrPress   = 1
	ptrRelease = 2
	ptrLeave   = 3
	ptrCancel  = 4
	ptrScroll  = 5
)

// MotionEvent.BUTTON_* values.
const (
	abPrimary   = 1
	abSecondary = 2
	abTertiary  = 4
)

// pointerEvent converts one mouse, hover or wheel report. x, y, dx, dy are
// device pixels (the host scales wheel steps by the system scroll factor).
func pointerEvent(kind int, x, y float32, button, meta int, dx, dy, density float32) (input.Event, bool) {
	pos := geom.Point{X: mobilecanvas.Logical(x, density), Y: mobilecanvas.Logical(y, density)}
	mods := canvasMods(meta)
	switch kind {
	case ptrScroll:
		return input.Scroll{Pos: pos, DX: mobilecanvas.Logical(dx, density), DY: mobilecanvas.Logical(dy, density), Mods: mods}, true
	case ptrMove:
		return input.Pointer{Kind: input.Move, Pos: pos, Mods: mods}, true
	case ptrLeave:
		return input.Pointer{Kind: input.Leave, Pos: pos, Mods: mods}, true
	case ptrCancel:
		return input.Pointer{Kind: input.Cancel, Pos: pos, Mods: mods}, true
	case ptrPress, ptrRelease:
		b := input.ButtonLeft
		switch button {
		case abSecondary:
			b = input.ButtonRight
		case abTertiary:
			b = input.ButtonMiddle
		}
		k := input.Press
		if kind == ptrRelease {
			k = input.Release
		}
		return input.Pointer{Kind: k, Pos: pos, Button: b, Mods: mods}, true
	}
	return nil, false
}

// Scheduler actions go back to Java (CanvasView.apply) as one long: bits
// 0-1 the link operation (mobilecanvas.LinkOp: keep, resume, pause), bit 2
// "set the deadline timer", bits 4 and up the timer's uptime in
// milliseconds, rounded up (0 cancels it).
func schedAction(a mobilecanvas.Action, uptimeMs int64, now time.Time) int64 {
	v := int64(a.Link) & 3
	if a.Arm {
		v |= 4
		if !a.At.IsZero() {
			d := a.At.Sub(now)
			at := uptimeMs + d.Milliseconds()
			if d%time.Millisecond > 0 {
				at++
			}
			v |= max(at, 1) << 4
		}
	}
	return v
}

// mergeActions combines two scheduler actions taken in sequence: the later
// link operation and timer setting win.
func mergeActions(a, b mobilecanvas.Action) mobilecanvas.Action {
	if b.Link != mobilecanvas.LinkKeep {
		a.Link = b.Link
	}
	if b.Arm {
		a.Arm, a.At = true, b.At
	}
	a.Frame = a.Frame || b.Frame
	return a
}
