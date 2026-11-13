//go:build ios

package ios

// Pointer self-test for the simulator, where simctl can neither move a
// pointer nor scroll (docs/specs/mobile-canvas.md §7.3). With
// TANGO_CANVAS_POINTER_SELFTEST set (simctl launch:
// SIMCTL_CHILD_TANGO_CANVAS_POINTER_SELFTEST=...) the first canvas — a
// canvas page or an attached one — a few seconds after it opens runs what
// canvas_pointer.m's gesture handlers run for a hover (enter, move,
// leave), a trackpad scroll and a wheel notch, sets pointer shapes and
// asks the pointer interaction for its style, hit tests the window at
// given points (pass-through: a transparent pixel of an overlay above the
// web view must hit the web view) and posts a memory warning. Every
// pointer event the sink receives and the AppMemoryWarning emission are
// logged with the prefix "canvas pointer selftest:".
//
// The value is "1" or a list of points in canvas view coordinates,
// "x,y;x,y" (default 100,100;120,110): the hover runs over the first two
// and the hit test over all of them.

/*
#include <stdlib.h>
#include "canvas_pointer.h"
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var pointerTest struct {
	on      bool
	started bool
	pts     []geom.Point
	step    int
}

func init() {
	v := os.Getenv("TANGO_CANVAS_POINTER_SELFTEST")
	if v == "" {
		return
	}
	pointerTest.on = true
	for _, p := range strings.Split(v, ";") {
		xy := strings.Split(p, ",")
		if len(xy) != 2 {
			continue
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(xy[0]), 32)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(xy[1]), 32)
		if err1 == nil && err2 == nil {
			pointerTest.pts = append(pointerTest.pts, geom.Point{X: float32(x), Y: float32(y)})
		}
	}
	for len(pointerTest.pts) < 2 {
		pointerTest.pts = append(pointerTest.pts, []geom.Point{{X: 100, Y: 100}, {X: 120, Y: 110}}[len(pointerTest.pts)])
	}
}

// startPointerSelftest schedules the self-test on the first canvas.
func startPointerSelftest(c *canvas) {
	if !pointerTest.on || pointerTest.started {
		return
	}
	pointerTest.started = true
	pointerLogf("start on canvas %d (attached %v), points %v", c.h, c.ov != nil, pointerTest.pts)
	C.tgp_after(4, c.handle())
}

func pointerLogf(format string, args ...any) {
	if pointerTest.on {
		log.Printf("canvas pointer selftest: "+format, args...)
	}
}

// tracePointer logs an event on its way to the sink.
func tracePointer(ev input.Event) {
	if !pointerTest.on {
		return
	}
	switch e := ev.(type) {
	case input.Pointer:
		kind := map[input.PointerKind]string{input.Move: "Move", input.Leave: "Leave"}[e.Kind]
		pointerLogf("sink <- Pointer{%s at %.0f,%.0f source=%d mods=%d}", kind, e.Pos.X, e.Pos.Y, e.Source, e.Mods)
	case input.Scroll:
		pointerLogf("sink <- Scroll{at %.0f,%.0f DX=%g DY=%g}", e.Pos.X, e.Pos.Y, e.DX, e.DY)
	default:
		pointerLogf("sink <- %T%+v", ev, ev)
	}
}

func (c *canvas) pointerOp(op, a int, p geom.Point, tx, ty float64) string {
	r := C.tgp_selftest(c.handle(), C.int(op), C.int(a), C.double(p.X), C.double(p.Y), C.double(tx), C.double(ty))
	if r == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(r))
	return C.GoString(r)
}

//export tangoIOSPointerSelftest
func tangoIOSPointerSelftest(h C.uintptr_t) {
	c := lookup(h)
	if c == nil || !pointerTest.on {
		return
	}
	p0, p1 := pointerTest.pts[0], pointerTest.pts[1]
	steps := []struct {
		what string
		run  func()
	}{
		{fmt.Sprintf("hover enter at %v", p0), func() { c.pointerOp(C.TGP_TEST_HOVER, C.TGP_HOVER_ENTER, p0, 0, 0) }},
		{fmt.Sprintf("hover move to %v", p1), func() { c.pointerOp(C.TGP_TEST_HOVER, C.TGP_HOVER_MOVE, p1, 0, 0) }},
		{"hover leave", func() { c.pointerOp(C.TGP_TEST_HOVER, C.TGP_HOVER_LEAVE, p1, 0, 0) }},
		{"trackpad scroll, two fingers up 24pt", func() { c.pointerOp(C.TGP_TEST_SCROLL, 0, p0, 0, -24) }},
		{"wheel notch down", func() { c.pointerOp(C.TGP_TEST_SCROLL, 1, p0, 0, -10) }},
		{"cursor shapes", func() {
			for _, cur := range []input.Cursor{input.CursorText, input.CursorResizeEW, input.CursorResizeNS, input.CursorPointer, input.CursorDefault} {
				c.SetCursor(cur)
				pointerLogf("SetCursor(%d) -> %s", cur, c.pointerOp(C.TGP_TEST_STYLE, 0, geom.Point{}, 0, 0))
			}
		}},
		{"hit tests", func() {
			for _, p := range pointerTest.pts {
				opaque := "n/a"
				if c.ov != nil {
					opaque = fmt.Sprint(c.ov.takesPoint(float64(p.X), float64(p.Y)))
				}
				pointerLogf("hit %.0f,%.0f: takes point %s, window hit %s", p.X, p.Y, opaque, c.pointerOp(C.TGP_TEST_HIT, 0, p, 0, 0))
			}
		}},
		{"memory warning", func() { c.pointerOp(C.TGP_TEST_MEMORY, 0, geom.Point{}, 0, 0) }},
	}
	i := pointerTest.step
	pointerTest.step++
	if i >= len(steps) {
		pointerLogf("done")
		return
	}
	pointerLogf("step %s", steps[i].what)
	steps[i].run()
	C.tgp_after(0.5, h)
}
