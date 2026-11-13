//go:build ios

package ios

// Text input of the canvas (docs/specs/mobile-canvas.md §2.3, §2.4): the
// Go half of canvas_text.m. The view's UITextInput passes each edit here
// (tangoIOSCanvasText); textDoc (canvas_textdoc.go) turns it into input
// events and the new mirror, which goes back to the view before the
// events go to the sink. SetTextInputState (M3b) replaces the document
// with the focused widget's text and tells UIKit about the change.

/*
#include <stdlib.h>
#include "canvas_text.h"
*/
import "C"

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func cTraits(t textTraits) C.tgc_traits {
	b := func(v bool) C.int {
		if v {
			return 1
		}
		return 0
	}
	return C.tgc_traits{
		keyboard:   C.int(t.Keyboard),
		ret:        C.int(t.Return),
		secure:     b(t.Secure),
		correct:    C.int(t.Correct),
		spell:      C.int(t.SpellCheck),
		capitalize: C.int(t.Capitalize),
	}
}

// setTextInput implements SetTextInput. Turning text input on or off
// starts a fresh M3a document (a widget with state sends it next) and
// shows or hides the keyboard.
func (c *canvas) setTextInput(enabled bool, caret geom.Rect) {
	if caret != c.caret {
		c.caret = caret
		C.tgc_text_caret(c.handle(), C.double(caret.X), C.double(caret.Y), C.double(caret.W), C.double(caret.H))
	}
	if enabled == c.textOn {
		return
	}
	c.textOn = enabled
	c.doc.reset()
	c.traits = traitsFor(input.TextHints{})
	c.pushMirror(true, true)
	C.tgc_text_on(c.handle(), boolInt(enabled), cTraits(c.traits))
}

// SetTextInputState implements platform.CanvasTextInputState: the
// focused widget's text, selection and hints. Called after SetTextInput
// whenever they change; ignored while text input is off.
func (c *canvas) SetTextInputState(st input.TextInputState) {
	if c.closed || !c.textOn {
		return
	}
	tc, sc := c.doc.setState(st)
	if t := traitsFor(st.Hints); t != c.traits {
		c.traits = t
		C.tgc_text_traits(c.handle(), cTraits(t))
	}
	if tc || sc {
		c.pushMirror(tc, sc)
	}
}

// pushMirror sends the document to the view. notifyText and notifySel
// make it tell UIKit's input delegate (changes UIKit did not make).
func (c *canvas) pushMirror(notifyText, notifySel bool) {
	m := c.doc.mirror()
	var p *C.char
	if m.Text != "" {
		p = C.CString(m.Text)
		defer C.free(unsafe.Pointer(p))
	}
	C.tgc_text_mirror(c.handle(), p, C.int(len(m.Text)), C.int(m.Sel.Start), C.int(m.Sel.Len),
		C.int(m.Mark.Start), C.int(m.Mark.Len), boolInt(notifyText), boolInt(notifySel))
}

func boolInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

//export tangoIOSCanvasText
func tangoIOSCanvasText(h C.uintptr_t, op C.int, text *C.char, a, b C.int) {
	c := lookup(h)
	if c == nil || !c.ready || !c.textOn {
		return
	}
	s := C.GoString(text)
	r := mobilecanvas.Range{Start: int(a), Len: int(b)}
	var evs []input.Event
	switch op {
	case C.TGC_TEXT_MARK:
		evs = c.doc.setMarked(s, r)
	case C.TGC_TEXT_INSERT:
		evs = c.doc.insert(s)
	case C.TGC_TEXT_UNMARK:
		evs = c.doc.unmark()
	case C.TGC_TEXT_DELETE:
		evs = c.doc.deleteBackward()
	case C.TGC_TEXT_REPLACE:
		evs = c.doc.replace(r, s)
	case C.TGC_TEXT_SELECT:
		evs = c.doc.selectRange(r)
	default:
		return
	}
	c.pushMirror(false, false) // UIKit made this change itself
	traceIME(c, int(op), s, r, evs)
	for _, ev := range evs {
		if c.closed {
			return
		}
		c.sink.CanvasInput(ev)
	}
}

//export tangoIOSCanvasKeyboard
func tangoIOSCanvasKeyboard(h C.uintptr_t, bottom C.double) {
	c := lookup(h)
	if c == nil {
		return
	}
	in := mobilecanvas.Clamp(geom.Insets{Bottom: float32(bottom)})
	if in == c.kbd {
		return
	}
	c.kbd = in
	traceKeyboard(c)
	if c.ready {
		c.Invalidate() // the runner polls KeyboardInset every frame
	}
}
