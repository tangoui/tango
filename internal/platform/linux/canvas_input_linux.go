//go:build linux && !android

package linux

// Input of the canvas widget (canvas_linux.go): pointer, scroll, keyboard,
// focus and GtkIMContext. GdkEvent fields are read at the offsets in
// canvas_keys.go (GTK 3 layout on LP64 targets).
//
// Keyboard routing. With text input on, every key event first goes to the
// IM context (gtk_im_context_filter_keypress); when it consumes the event
// nothing else is reported: committed text arrives as input.TextInput
// ("commit") and pre-edit text as input.Composition ("preedit-changed").
// Keys it does not consume (Return, arrows, Ctrl shortcuts with the simple
// context ...) and every key while text input is off are reported as
// input.KeyEvent. Modifier keys are reported as their left/right Key, Mods
// including the modifier on press and excluding it on release, like the
// darwin backend's flagsChanged:. GtkWindow normally activates menu
// accelerators before the focus widget sees a key. Shortcut capture uses
// an earlier window signal handler (canvas_shortcut_linux.go) and bypasses
// the IM context without changing its text-input state.

import (
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func canvasEvU32(ev ptr, off uintptr) uint32 { return *(*uint32)(unsafe.Add(toPtr(ev), off)) }

// canvasEvTime reads a GdkEvent time field (milliseconds, server clock).
func canvasEvTime(ev ptr, off uintptr) time.Duration {
	return time.Duration(canvasEvU32(ev, off)) * time.Millisecond
}

func canvasEvU16(ev ptr, off uintptr) uint16 { return *(*uint16)(unsafe.Add(toPtr(ev), off)) }
func canvasEvU8(ev ptr, off uintptr) uint8   { return *(*uint8)(unsafe.Add(toPtr(ev), off)) }
func canvasEvF64(ev ptr, off uintptr) float64 {
	return *(*float64)(unsafe.Add(toPtr(ev), off))
}

func canvasEvPoint(ev ptr, xOff, yOff uintptr) geom.Point {
	return geom.Pt(float32(canvasEvF64(ev, xOff)), float32(canvasEvF64(ev, yOff)))
}

// connectInput connects the widget's input signals and creates the IM
// context. Called once from newCanvasWidget.
func (c *canvas) connectInput() {
	w := c.widget
	connect(w, "button-press-event", 1, func(_, ev ptr) ptr { return c.onButton(ev, true) })
	connect(w, "button-release-event", 1, func(_, ev ptr) ptr { return c.onButton(ev, false) })
	connect(w, "motion-notify-event", 1, c.onMotion)
	connect(w, "enter-notify-event", 1, c.onEnter)
	connect(w, "leave-notify-event", 1, c.onLeave)
	connect(w, "scroll-event", 1, c.onScroll)
	connect(w, "touch-event", 1, c.onTouch) // canvas_touch_linux.go
	connect(w, "key-press-event", 1, func(_, ev ptr) ptr { return c.onKey(ev, true) })
	connect(w, "key-release-event", 1, func(_, ev ptr) ptr { return c.onKey(ev, false) })
	connect(w, "focus-in-event", 1, c.onFocusChange)
	connect(w, "focus-out-event", 1, c.onFocusChange)

	c.im = canvasFFI.imMulticontextNew()
	if c.im != 0 {
		connect(c.im, "commit", 1, c.onCommit)
		connect(c.im, "preedit-changed", 0, c.onPreedit)
		connect(c.im, "preedit-end", 0, c.onPreedit)
	}
}

// input delivers ev to the sink.
func (c *canvas) input(ev input.Event) {
	if c.ready && !c.closed {
		c.sink.CanvasInput(ev)
	}
}

// ---- pointer ----

func (c *canvas) onButton(ev ptr, press bool) ptr {
	if c.closed {
		return 0
	}
	if c.touchPointer(ev) {
		return 1 // emulated from a touch, which is reported itself
	}
	if press && canvasGdkPressCount(canvasEvU32(ev, canvasEvTypeOffset)) > 1 {
		// GDK_2BUTTON_PRESS / GDK_3BUTTON_PRESS follow the plain press
		// they belong to, which was already reported with its count.
		return 1
	}
	b, ok := canvasButton(canvasEvU32(ev, canvasEvButtonButton))
	if !ok {
		return 0 // back / forward: let GTK have them
	}
	pos := canvasEvPoint(ev, canvasEvButtonX, canvasEvButtonY)
	p := input.Pointer{Kind: input.Release, Pos: pos, Button: b, Mods: canvasMods(canvasEvU32(ev, canvasEvButtonState)),
		Time: canvasEvTime(ev, canvasEvButtonTime)}
	if press {
		p.Kind = input.Press
		p.Clicks = c.clicks.press(b, canvasEvU32(ev, canvasEvButtonTime), float64(pos.X), float64(pos.Y),
			canvasFFI.dblMS, canvasFFI.dblDist)
		if !canvasFFI.widgetHasFocus(c.widget) {
			canvasFFI.widgetGrabFocus(c.widget)
		}
	} else {
		p.Clicks = c.clicks.release(b)
	}
	c.input(p)
	return 1
}

func (c *canvas) onMotion(_, ev ptr) ptr {
	if c.closed || c.touchPointer(ev) {
		return 0
	}
	state := canvasEvU32(ev, canvasEvMotionState)
	c.input(input.Pointer{Kind: input.Move, Pos: canvasEvPoint(ev, canvasEvMotionX, canvasEvMotionY),
		Button: canvasHeldButton(state), Mods: canvasMods(state), Time: canvasEvTime(ev, canvasEvMotionTime)})
	return 1
}

func (c *canvas) onEnter(_, ev ptr) ptr {
	if c.closed || c.touchPointer(ev) {
		return 0
	}
	c.inside = true
	c.applyCursor()
	c.input(input.Pointer{Kind: input.Move, Pos: canvasEvPoint(ev, canvasEvCrossingX, canvasEvCrossingY),
		Mods: canvasMods(canvasEvU32(ev, canvasEvCrossingState))})
	return 0
}

func (c *canvas) onLeave(_, ev ptr) ptr {
	if c.closed || c.touchPointer(ev) {
		return 0
	}
	c.inside = false
	c.input(input.Pointer{Kind: input.Leave, Pos: canvasEvPoint(ev, canvasEvCrossingX, canvasEvCrossingY),
		Mods: canvasMods(canvasEvU32(ev, canvasEvCrossingState))})
	return 0
}

func (c *canvas) onScroll(_, ev ptr) ptr {
	if c.closed {
		return 0
	}
	dir := canvasEvU32(ev, canvasEvScrollDirection)
	finger := false
	if dir == canvasGdkScrollSmooth && canvasFFI.deviceGetSource != nil {
		if dev := canvasFFI.eventGetSourceDevice(ev); dev != 0 {
			src := canvasFFI.deviceGetSource(dev)
			finger = src == canvasGdkSourceTouchpad || src == canvasGdkSourceTrackpoint
		}
	}
	dx, dy, ok := canvasScrollDelta(dir, canvasEvF64(ev, canvasEvScrollDeltaX), canvasEvF64(ev, canvasEvScrollDeltaY), finger)
	if !ok {
		return 0
	}
	if dx != 0 || dy != 0 { // a smooth "is_stop" event carries no motion
		c.input(input.Scroll{Pos: canvasEvPoint(ev, canvasEvScrollX, canvasEvScrollY), DX: dx, DY: dy,
			Mods: canvasMods(canvasEvU32(ev, canvasEvScrollState))})
	}
	return 1
}

// ---- keyboard ----

func (c *canvas) onKey(ev ptr, press bool) ptr {
	if c.closed {
		return 0
	}
	state := canvasEvU32(ev, canvasEvKeyState)
	keyval := canvasEvU32(ev, canvasEvKeyKeyval)
	hw := canvasEvU16(ev, canvasEvKeyHWCode)
	hid := hidFromXKB(hw)
	k, code := canvasKey(keyval, hw)
	if k == input.KeyUnknown && code == 0 && canvasNeedsLatinFallback(keyval) {
		k = c.latinKey(ev, state)
	}
	consumed, consumedPress := c.filterKey(hw, state, k, code, press)
	if consumed {
		id, held := canvasHeldFind(c.downKeys, k, code)
		if !press && held {
			delete(c.downKeys, id)
		}
		return 1
	}
	if !c.shortcutCapture && !c.shortcutHeld[hw] && c.textOn && c.im != 0 && canvasFFI.imFilterKeypress(c.im, ev) {
		return 1 // the input method used it
	}
	if m, ok := canvasModifierKeyMods(keyval, state, press); ok {
		// Modifier keys are reported once per press (no auto-repeat) and
		// not consumed, so GTK keeps tracking them.
		if k.Modifier() == 0 || press == c.downKeys[k] {
			return 0
		}
		m = canvasModifierEventMods(k, press, m, c.downKeys[canvasOtherSide(k)])
		if press {
			c.downKeys[k] = true
		} else {
			delete(c.downKeys, k)
		}
		c.deliverKey(input.KeyEvent{Key: k, Down: press, Mods: m, HID: hid, Native: uint32(hw)}, false)
		return 0
	}
	if k == input.KeyUnknown && code == 0 {
		// A key Key has no name for (F13-F24, JIS and Korean keys) still
		// reaches apps that read physical codes; its press and release
		// are paired by HID usage.
		if hid == 0 {
			return 0
		}
		held := c.downHID[hid]
		switch {
		case press && !held:
			if c.downHID == nil {
				c.downHID = map[uint16]bool{}
			}
			c.downHID[hid] = true
		case !press && held:
			delete(c.downHID, hid)
		case !press:
			return 0
		}
		c.deliverKey(input.KeyEvent{Down: press, Repeat: press && held, Mods: canvasMods(state), HID: hid, Native: uint32(hw)}, false)
		return 1
	}
	// downKeys holds canvasHeldID records of the reported presses: a
	// repeat and the release are reported with the Key and Code of the
	// press, even when NumLock was toggled while a keypad key was held.
	mods := canvasMods(state)
	id, held := canvasHeldFind(c.downKeys, k, code)
	if press {
		// GDK 3 has no repeat flag: a press of a held key is a repeat.
		if held {
			k, code = canvasHeldKey(id)
		} else {
			c.downKeys[canvasHeldID(k, code)] = true
		}
		c.deliverKey(input.KeyEvent{Key: k, Code: code, Down: true, Repeat: held || consumedPress, Mods: mods, HID: hid, Native: uint32(hw)}, false)
		return 1
	}
	if !held && !consumedPress {
		return 0
	}
	if held {
		delete(c.downKeys, id)
		k, code = canvasHeldKey(id)
	}
	c.deliverKey(input.KeyEvent{Key: k, Code: code, Mods: mods, HID: hid, Native: uint32(hw)}, false)
	return 1
}

// filterKey tracks hardware state independently of what GTK's IM context
// forwards to the canvas. A consumed press still has a physical release.
type canvasRawKey struct {
	event    input.KeyEvent
	consumed bool
}

func (c *canvas) filterKey(hw uint16, state uint32, k, code input.Key, press bool) (consumed, consumedPress bool) {
	if !c.ready || c.closed {
		return false, false
	}
	if _, ok := c.sink.(platform.CanvasKeySink); !ok {
		return false, false
	}
	raw, held := c.rawKeys[hw]
	ev := raw.event
	if !held {
		ev = input.KeyEvent{Key: k, Code: code}
	}
	ev.Down, ev.Repeat, ev.Mods = press, press && held, canvasMods(state)
	if ev.Key.Modifier() != 0 {
		if press == held {
			return false, false
		}
		other := canvasOtherSide(ev.Key)
		otherDown := false
		for _, raw := range c.rawKeys {
			otherDown = otherDown || raw.event.Key == other
		}
		ev.Mods = canvasModifierEventMods(ev.Key, press, ev.Mods, otherDown)
	}
	if press {
		if c.rawKeys == nil {
			c.rawKeys = make(map[uint16]canvasRawKey)
		}
		raw.event = ev
		c.rawKeys[hw] = raw
	} else {
		delete(c.rawKeys, hw)
	}
	consumed = platform.FilterCanvasKey(c.sink, ev)
	if consumed && press {
		if _, stillHeld := c.rawKeys[hw]; stillHeld {
			raw.consumed = true
			c.rawKeys[hw] = raw
		}
	}
	return consumed, raw.consumed
}

func (c *canvas) deliverKey(ev input.KeyEvent, synthetic bool) {
	if c.ready && !c.closed {
		platform.DeliverCanvasKey(c.sink, ev, synthetic)
	}
}

// latinKey re-translates a non-Latin key (Cyrillic, Greek ...) in keyboard
// group 0, so Ctrl+C works on the key labelled C with a us,ru layout.
func (c *canvas) latinKey(ev ptr, state uint32) input.Key {
	f := &canvasFFI
	if f.keymapTranslate == nil || canvasEvU8(ev, canvasEvKeyGroup) == 0 {
		return input.KeyUnknown
	}
	km := f.keymapForDisplay(f.widgetGetDisplay(c.widget))
	if km == 0 {
		return input.KeyUnknown
	}
	var keyval, consumed uint32
	var group, level int32
	if !f.keymapTranslate(km, uint32(canvasEvU16(ev, canvasEvKeyHWCode)), state, 0, &keyval, &group, &level, &consumed) {
		return input.KeyUnknown
	}
	k, _ := canvasKey(keyval, canvasEvU16(ev, canvasEvKeyHWCode))
	return k
}

// releaseKeys reports a release for every key still held, when focus
// leaves (the releases go to another widget).
func (c *canvas) releaseKeys() {
	for id := range c.downKeys {
		delete(c.downKeys, id)
		k, code := canvasHeldKey(id)
		c.deliverKey(input.KeyEvent{Key: k, Code: code}, true)
	}
}

// ---- focus ----

func (c *canvas) onFocusChange(_, _ ptr) ptr {
	if !c.closed {
		c.focusMaybeChanged()
	}
	return 0 // let GTK update its focus state too
}

// focusMaybeChanged reports CanvasFocus when the widget's focus (focus
// widget of the active toplevel) changed, and keeps the IM context's
// focus in step.
func (c *canvas) focusMaybeChanged() {
	if c.closed {
		return
	}
	f := canvasFFI.widgetHasFocus(c.widget)
	if !f {
		clear(c.rawKeys)
		clear(c.shortcutHeld)
	}
	c.syncIMFocus(f)
	if f == c.focused {
		return
	}
	if !c.ready {
		return // start queues a focus check
	}
	c.focused = f
	if !f {
		c.releaseKeys()
	}
	c.sink.CanvasFocus(f)
}

// ---- text input ----

// syncIMFocus gives the IM context focus while text input is on and the
// widget has focus.
func (c *canvas) syncIMFocus(focused bool) {
	if c.im == 0 {
		return
	}
	want := c.textOn && focused
	if want == c.imFocused {
		return
	}
	c.imFocused = want
	if want {
		canvasFFI.imFocusIn(c.im)
		c.updateCaret()
	} else {
		canvasFFI.imFocusOut(c.im)
	}
}

// SetTextInput implements platform.Canvas. Main thread.
func (c *canvas) SetTextInput(enabled bool, caret geom.Rect) {
	if c.closed {
		return
	}
	was, old := c.textOn, c.caret
	c.caret = caret
	if was && !enabled {
		// Off: drop any pre-edit silently (textOn is false first, so the
		// preedit-changed that reset emits is ignored).
		c.textOn = false
		if c.im != 0 && c.marked != "" {
			canvasFFI.imReset(c.im)
		}
		c.marked, c.markedCur = "", 0
		c.syncIMFocus(false)
		return
	}
	c.textOn = enabled
	if !enabled {
		return
	}
	if !was {
		c.syncIMFocus(canvasFFI.widgetHasFocus(c.widget))
	} else if old != caret {
		c.updateCaret()
	}
}

// updateCaret positions the candidate window: the caret in the widget's
// GdkWindow coordinates (logical pixels), which is the IM client window.
func (c *canvas) updateCaret() {
	if c.im == 0 || !c.textOn {
		return
	}
	r := canvasCaretRect(c.caret)
	canvasFFI.imSetCursorLocation(c.im, &r)
}

// onCommit handles the IM context's "commit" (context, const gchar *str).
func (c *canvas) onCommit(_, str ptr) ptr {
	if c.closed || !c.textOn {
		return 0
	}
	text := canvasCleanCommit(goStr(str))
	had := c.marked != ""
	c.marked, c.markedCur = "", 0
	switch {
	case text != "":
		c.input(input.TextInput{Text: text})
	case had:
		c.input(input.Composition{})
	}
	return 0
}

// onPreedit handles "preedit-changed" and "preedit-end".
func (c *canvas) onPreedit(_, _ ptr) ptr {
	if c.closed || !c.textOn || c.im == 0 {
		return 0
	}
	var str ptr
	var cur int32
	canvasFFI.imGetPreeditString(c.im, &str, 0, &cur) // attrs: NULL is allowed
	text := takeStr(str)
	pos := canvasClampRunes(text, int(cur)) // cursor_pos is in characters
	if text == c.marked && pos == c.markedCur {
		return 0
	}
	c.marked, c.markedCur = text, pos
	if text == "" {
		pos = 0
	}
	c.input(input.Composition{Text: text, Cursor: pos})
	return 0
}
