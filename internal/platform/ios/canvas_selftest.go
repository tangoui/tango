//go:build ios

package ios

// Text-input self-test for the simulator, where simctl can neither tap
// nor type (docs/specs/mobile-canvas.md §7.3). With
// TANGO_CANVAS_IME_SELFTEST set (simctl launch:
// SIMCTL_CHILD_TANGO_CANVAS_IME_SELFTEST=...) the first canvas, a few
// seconds after it opens, taps the listed points with a synthetic finger
// (through the same touch path as UIKit's touches), so a text field takes
// focus, then calls the view's UITextInput methods as the keyboard and an
// input method would: a composition committed as a Chinese character,
// typing, Backspace and an autocorrection. Each step logs the document
// the view's mirror holds (with a widget that reports TextInputState,
// the field's own text), the keyboard inset and whether the soft
// keyboard is up.
//
// The value is "1" (tap nothing: focus a field some other way) or a list
// of points in view coordinates, "x,y;x,y", tapped in order a second
// apart. Without the variable nothing here runs except the two trace
// functions' flag check.

/*
#include <stdlib.h>
#include "canvas_text.h"
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var selftest struct {
	on      bool
	started bool
	taps    []geom.Point
	step    int
}

func init() {
	v := os.Getenv("TANGO_CANVAS_IME_SELFTEST")
	if v == "" {
		return
	}
	selftest.on = true
	for _, p := range strings.Split(v, ";") {
		xy := strings.Split(p, ",")
		if len(xy) != 2 {
			continue
		}
		x, err1 := strconv.ParseFloat(strings.TrimSpace(xy[0]), 32)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(xy[1]), 32)
		if err1 == nil && err2 == nil {
			selftest.taps = append(selftest.taps, geom.Point{X: float32(x), Y: float32(y)})
		}
	}
}

// startSelftest schedules the self-test on the first canvas.
func startSelftest(c *canvas) {
	if !selftest.on || selftest.started {
		return
	}
	selftest.started = true
	log.Printf("canvas ime selftest: start, taps %v", selftest.taps)
	C.tgc_after(4, c.handle())
}

func traceIME(c *canvas, op int, text string, r mobilecanvas.Range, evs []input.Event) {
	if !selftest.on {
		return
	}
	names := []string{"setMarkedText", "insertText", "unmarkText", "deleteBackward", "replaceRange", "setSelectedTextRange"}
	name := fmt.Sprintf("op%d", op)
	if op >= 0 && op < len(names) {
		name = names[op]
	}
	log.Printf("canvas ime selftest: %s(%q, %d+%d) -> %s", name, text, r.Start, r.Len, eventsString(evs))
}

func traceKeyboard(c *canvas) {
	if selftest.on {
		log.Printf("canvas ime selftest: keyboard inset bottom %.1f", c.kbd.Bottom)
	}
}

func eventsString(evs []input.Event) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, ev := range evs {
		if i > 0 {
			b.WriteString(", ")
		}
		switch e := ev.(type) {
		case input.KeyEvent:
			fmt.Fprintf(&b, "Key{%v down=%v}", e.Key, e.Down)
		default:
			fmt.Fprintf(&b, "%T%+v", ev, ev)
		}
	}
	b.WriteByte(']')
	return b.String()
}

func (c *canvas) selftestReport(what string) {
	m := c.doc.mirror()
	log.Printf("canvas ime selftest: %s: textOn=%v keyboard=%v inset=%.1f state=%v hints=%+v text=%q sel=%d+%d mark=%d+%d",
		what, c.textOn, C.tgc_text_active(c.handle()) != 0, c.kbd.Bottom, c.doc.state, c.doc.hints,
		m.Text, m.Sel.Start, m.Sel.Len, m.Mark.Start, m.Mark.Len)
}

func (c *canvas) selftestTap(p geom.Point) {
	now := time.Now()
	s := mobilecanvas.Sample{Pos: p, Time: now}
	const key = ^uintptr(0) // no UITouch has this address
	c.deliver(c.touches.Begin(nil, key, s))
	s.Time = now.Add(50 * time.Millisecond)
	c.deliver(c.touches.End(nil, key, s))
}

func (c *canvas) selftestOp(op int, text string, a, b int) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.tgc_text_selftest(c.handle(), C.int(op), cs, C.int(a), C.int(b))
}

//export tangoIOSCanvasSelftest
func tangoIOSCanvasSelftest(h C.uintptr_t) {
	c := lookup(h)
	if c == nil || !selftest.on {
		return
	}
	step := selftest.step
	selftest.step++
	next := func(sec float64) { C.tgc_after(C.double(sec), h) }
	if step < len(selftest.taps) {
		p := selftest.taps[step]
		log.Printf("canvas ime selftest: tap %.0f,%.0f", p.X, p.Y)
		c.selftestTap(p)
		next(1.2)
		return
	}
	step -= len(selftest.taps)
	ops := []struct {
		what string
		run  func()
	}{
		{"focused", func() {}},
		{"compose zhong", func() { c.selftestOp(C.TGC_TEXT_MARK, "zhong", 5, 0) }},
		{"commit 中", func() { c.selftestOp(C.TGC_TEXT_INSERT, "中", 0, 0) }},
		{"type abc", func() { c.selftestOp(C.TGC_TEXT_INSERT, "abc", 0, 0) }},
		{"backspace", func() { c.selftestOp(C.TGC_TEXT_DELETE, "", 0, 0) }},
		{"type ' teh'", func() { c.selftestOp(C.TGC_TEXT_INSERT, " teh", 0, 0) }},
		{"autocorrect teh -> the", func() {
			// The word before the caret, as UIKit's autocorrection replaces it.
			m := c.doc.mirror()
			end := m.Sel.Start
			c.selftestOp(C.TGC_TEXT_REPLACE, "the", max(end-3, 0), min(3, end))
		}},
		{"move caret to start", func() { c.selftestOp(C.TGC_TEXT_SELECT, "", 0, 0) }},
		{"type >", func() { c.selftestOp(C.TGC_TEXT_INSERT, ">", 0, 0) }},
	}
	if step >= len(ops) {
		c.selftestReport("done")
		return
	}
	if step > 0 {
		c.selftestReport("after " + ops[step-1].what)
	}
	o := ops[step]
	log.Printf("canvas ime selftest: step %s", o.what)
	o.run()
	if step == len(ops)-1 {
		C.tgc_after(1, h)
		selftest.step = len(selftest.taps) + len(ops) // report "done" next
		return
	}
	next(1)
}
