//go:build android

package android

/*
#include "jni_android.h"
*/
import "C"

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Soft keyboard and input methods of the Android canvas
// (docs/specs/mobile-canvas.md §2.3, M3). CanvasView is a text editor
// while text input is on: onCreateInputConnection hands the input method a
// CanvasInput, whose edits come here through TangoCanvas.nativeIME, one
// synchronous call each. The session (canvas_ime.go) turns them into input
// events and a new mirror, which goes back to CanvasPage.mirror before the
// call returns, so the input method's next query already sees its edit.

var (
	// CanvasPage (instance).
	mCvTextInput = &jmethod{cls: &canvasPageClass, name: "textInput", sig: "(ZIIFFFFZ)V"}
	mCvMirror    = &jmethod{cls: &canvasPageClass, name: "mirror", sig: "(Ljava/lang/String;IIIII)V"}
)

var _ platform.CanvasTextInputState = (*canvas)(nil)

// nativeIME operations (CanvasInput.java).
const (
	imeOpCompose   = 1 // setComposingText(text, a = newCursorPosition)
	imeOpCommit    = 2 // commitText(text, a = newCursorPosition)
	imeOpFinish    = 3 // finishComposingText
	imeOpDelete    = 4 // deleteSurroundingText(a, b), UTF-16 units
	imeOpDeleteCP  = 5 // deleteSurroundingTextInCodePoints(a, b)
	imeOpRegion    = 6 // setComposingRegion(a, b)
	imeOpSelect    = 7 // setSelection(a, b)
	imeOpBackspace = 8 // sendKeyEvent(KEYCODE_DEL)
	imeOpEnter     = 9 // sendKeyEvent(KEYCODE_ENTER), performEditorAction
	imeOpReset     = 10
)

func jvFloat(f float32) C.jvalue {
	var v C.jvalue
	*(*C.jfloat)(unsafe.Pointer(&v)) = C.jfloat(f)
	return v
}

// SetTextInput turns text input on or off and moves the caret the input
// method's candidate window follows. Turning it on shows the soft keyboard
// with a fresh session; off hides it.
func (c *canvas) SetTextInput(enabled bool, caret geom.Rect) {
	if c.closed {
		return
	}
	c.caret = caret
	if enabled != c.textOn {
		c.textOn = enabled
		c.ime.reset(false)
		c.pushMirror()
	}
	c.pushTextInput(false)
}

// SetTextInputState implements platform.CanvasTextInputState: the focused
// widget's text around the caret, selection, composition and hints.
func (c *canvas) SetTextInputState(st input.TextInputState) {
	if c.closed || !c.textOn {
		return
	}
	evs, restart := c.ime.setState(st)
	c.deliver(evs)
	c.pushMirror()
	if restart {
		if canvasDebug > 0 {
			logf(logInfo, "canvas %d: input method restarts (hints %+v)", c.id, st.Hints)
		}
		c.pushTextInput(true)
	}
}

// pushTextInput sends the text input state, the EditorInfo and the caret
// (device pixels in the view) to CanvasPage.textInput; restart asks for a
// new input connection.
func (c *canvas) pushTextInput(restart bool) {
	typ, opts := editorInfo(c.ime.hints)
	d, r := c.density, c.caret
	withEnv(func(e env) {
		e.callVoid(c.page, mCvTextInput, jvBool(c.textOn), jvInt(typ), jvInt(opts),
			jvFloat(r.X*d), jvFloat(r.Y*d), jvFloat(r.W*d), jvFloat(r.H*d), jvBool(restart))
	})
}

// pushMirror sends the session's document to CanvasPage.mirror.
func (c *canvas) pushMirror() {
	if c.closed {
		return
	}
	text, base, ss, se, cs, ce := c.ime.m.java()
	withEnv(func(e env) {
		e.callVoid(c.page, mCvMirror, jvObj(e.str(text)), jvInt(base), jvInt(ss), jvInt(se), jvInt(cs), jvInt(ce))
	})
}

// nativeIME carries out one edit of the input method.
//
//export Java_org_tango_host_TangoCanvas_nativeIME
func Java_org_tango_host_TangoCanvas_nativeIME(penv *C.JNIEnv, cls C.jclass, id C.jlong, op C.jint, text C.jstring, a, b C.jint) {
	c := canvasFor(id)
	if c == nil || !c.textOn {
		return
	}
	var evs []input.Event
	s := &c.ime
	switch op {
	case imeOpCompose:
		evs = s.composing(env{penv}.gostr(text), int(a))
	case imeOpCommit:
		evs = s.commit(env{penv}.gostr(text), int(a))
	case imeOpFinish:
		evs = s.finish()
	case imeOpDelete:
		evs = s.deleteAround(int(a), int(b), false)
	case imeOpDeleteCP:
		evs = s.deleteAround(int(a), int(b), true)
	case imeOpRegion:
		evs = s.composeRegion(int(a), int(b))
	case imeOpSelect:
		evs = s.selection(int(a), int(b))
	case imeOpBackspace:
		evs = s.backspace()
	case imeOpEnter:
		evs = s.enter()
	case imeOpReset:
		evs = s.reset(true)
	default:
		return
	}
	if canvasDebug > 0 {
		logf(logDebug, "canvas %d: ime op %d (%d, %d) -> %d events", c.id, int(op), int(a), int(b), len(evs))
	}
	c.deliver(evs)
	c.pushMirror()
}

// key delivers a hardware key event (planKey decides how text input sees
// it). code is the Android key code, unicode and meta from the KeyEvent.
func (c *canvas) key(ev input.KeyEvent, code, unicode, meta int) {
	if !ev.Down && c.imeKeys[code] {
		delete(c.imeKeys, code) // the session took the press
		return
	}
	text, printable := "", false
	if ev.Down && c.textOn {
		text, printable = canvasText(unicode, meta)
	}
	switch planKey(c.textOn, c.ime.doc.Marked(), ev.Key, printable) {
	case keyToIME:
		if !ev.Down {
			return
		}
		if c.imeKeys == nil {
			c.imeKeys = map[int]bool{}
		}
		c.imeKeys[code] = true
		if ev.Key == input.KeyBackspace {
			c.deliver(c.ime.backspace())
		} else {
			c.deliver(c.ime.enter())
		}
		c.pushMirror()
	case keyAndText:
		c.evs = append(c.evs[:0], ev)
		c.deliver(c.evs)
		c.deliver(c.ime.typed(text))
		c.pushMirror()
	default:
		c.evs = append(c.evs[:0], ev)
		c.deliver(c.evs)
		if ev.Down && c.textOn && c.ime.state && ev.Mods&(input.ModCtrl|input.ModAlt|input.ModMeta) == 0 {
			c.ime.predictKey(ev.Key)
			c.pushMirror()
		}
	}
}
