//go:build ios

package ios

// iPad pointer and trackpad input on canvases (docs/specs/mobile-canvas.md
// §2.1). The UIKit half is canvas_pointer.m: hover and scroll gesture
// recognizers and a UIPointerInteraction on every canvas view. Clicks
// stay touches (the runner synthesizes the pointer press from the first
// finger), so this only adds what touches cannot carry: the pointer
// moving without a button, scrolling and the pointer shape.

/*
#include "canvas_pointer.h"
*/
import "C"

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// setCursor shows the pointer shape for cur (pointerStyle).
func (c *canvas) setCursor(cur input.Cursor) {
	st := pointerStyle(cur)
	if st == c.cursor {
		return
	}
	c.cursor = st
	C.tgp_set_cursor(c.handle(), C.int(st))
}

// opaqueAt reports whether (x, y) takes pointer input: always, except
// over transparent pixels of a pass-through overlay.
func (c *canvas) opaqueAt(x, y float64) bool {
	return c.ov == nil || c.ov.takesPoint(x, y)
}

//export tangoIOSCanvasHover
func tangoIOSCanvasHover(h C.uintptr_t, phase C.int, x, y C.double, flags C.int64_t) {
	c := lookup(h)
	if c == nil || !c.ready {
		return
	}
	p := input.Pointer{Pos: geom.Point{X: float32(x), Y: float32(y)}, Mods: modsFromUIKit(int64(flags))}
	ev, ok := c.hover.event(int(phase), p, c.opaqueAt(float64(x), float64(y)))
	if !ok {
		return
	}
	tracePointer(ev)
	c.sink.CanvasInput(ev)
}

//export tangoIOSCanvasScroll
func tangoIOSCanvasScroll(h C.uintptr_t, discrete C.int, tx, ty, x, y C.double, flags C.int64_t) {
	c := lookup(h)
	if c == nil || !c.ready || !c.opaqueAt(float64(x), float64(y)) {
		return
	}
	dx, dy := scrollDelta(float64(tx), float64(ty), discrete != 0)
	if dx == 0 && dy == 0 {
		return
	}
	ev := input.Scroll{Pos: geom.Point{X: float32(x), Y: float32(y)}, DX: dx, DY: dy, Mods: modsFromUIKit(int64(flags))}
	tracePointer(ev)
	c.sink.CanvasInput(ev)
}
