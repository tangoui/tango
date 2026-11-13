//go:build darwin && !ios

package darwin

// TangoCanvasView: the NSView of a canvas surface. Layer-backed and
// flipped (origin top-left, like the ui package); it turns AppKit events
// into input events for the CanvasSink and implements NSTextInputClient so
// input methods (Pinyin, Kotoeri, dead keys, dictation, the emoji panel)
// work.
//
// Keyboard routing. keyDown: with text input off reports the key as an
// input.KeyEvent. With text input on (and Command not held) it goes
// through interpretKeyEvents:, so an input method sees it first:
// committed text arrives in insertText:replacementRange: (input.TextInput),
// pre-edit text in setMarkedText:... (input.Composition), and keys the
// input method does not use come back as doCommandBySelector:, reported as
// KeyEvents. Command shortcuts are never interpreted: performKeyEquivalent:
// is not overridden, so the menu sees them first; Edit menu actions sent to
// the view (copy:, paste:, ...) are reported as the matching Cmd+key.

import (
	"math"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// nsRange mirrors NSRange (two NSUInteger).
type nsRange struct{ Location, Length uint }

const nsNotFound = math.MaxInt64 // NSNotFound (NSIntegerMax)

var (
	selGetName func(s objc.SEL) string
	viewOnce   sync.Once
)

func canvasOf(self id) *canvas {
	if c := canvasByView[self]; c != nil && !c.closed {
		return c
	}
	return nil
}

func registerCanvasView() {
	viewOnce.Do(func() {
		purego.RegisterLibFunc(&selGetName, libObjC, "sel_getName")
		yes := func(self id, _ objc.SEL) bool { return true }

		methods := []objc.MethodDef{
			method("isFlipped", yes),
			method("wantsUpdateLayer", yes),
			method("acceptsFirstResponder", yes),
			method("acceptsFirstMouse:", func(self id, _ objc.SEL, ev id) bool { return true }),
			method("updateLayer", func(self id, _ objc.SEL) {
				if c := canvasOf(self); c != nil {
					c.frame()
				}
			}),
			method("setFrameSize:", func(self id, _ objc.SEL, sz nsSize) {
				msgSuperSize(superOf(self), sel("setFrameSize:"), sz)
				if c := canvasOf(self); c != nil && c.checkGeometry() {
					c.setNeedsDisplay()
				}
			}),
			method("viewDidChangeBackingProperties", func(self id, _ objc.SEL) {
				msgSuper(superOf(self), sel("viewDidChangeBackingProperties"))
				if c := canvasOf(self); c != nil && c.checkGeometry() {
					c.setNeedsDisplay()
				}
			}),
			method("becomeFirstResponder", func(self id, _ objc.SEL) bool {
				ok := byte(msgSuper(superOf(self), sel("becomeFirstResponder"))) != 0
				if c := canvasOf(self); c != nil && ok {
					c.firstResp = true
					c.focusMaybeChanged()
				}
				return ok
			}),
			method("resignFirstResponder", func(self id, _ objc.SEL) bool {
				ok := byte(msgSuper(superOf(self), sel("resignFirstResponder"))) != 0
				if c := canvasOf(self); c != nil && ok {
					c.firstResp = false
					c.focusMaybeChanged()
				}
				return ok
			}),
			method("tangoDisplayTick:", func(self id, _ objc.SEL, link id) {
				if c := canvasOf(self); c != nil {
					c.displayTick()
				}
			}),

			// Mouse.
			method("mouseDown:", pointerHandler(input.Press, input.ButtonLeft)),
			method("mouseUp:", pointerHandler(input.Release, input.ButtonLeft)),
			method("mouseDragged:", pointerHandler(input.Move, input.ButtonLeft)),
			method("mouseMoved:", pointerHandler(input.Move, input.ButtonLeft)),
			method("rightMouseDown:", pointerHandler(input.Press, input.ButtonRight)),
			method("rightMouseUp:", pointerHandler(input.Release, input.ButtonRight)),
			method("rightMouseDragged:", pointerHandler(input.Move, input.ButtonRight)),
			method("otherMouseDown:", pointerHandler(input.Press, input.ButtonMiddle)),
			method("otherMouseUp:", pointerHandler(input.Release, input.ButtonMiddle)),
			method("otherMouseDragged:", pointerHandler(input.Move, input.ButtonMiddle)),
			method("mouseEntered:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					c.inside = true
					c.applyCursor()
					c.input(input.Pointer{Kind: input.Move, Pos: c.loc(ev), Mods: eventMods(ev)})
				}
			}),
			method("mouseExited:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					c.inside = false
					c.input(input.Pointer{Kind: input.Leave, Pos: c.loc(ev), Mods: eventMods(ev)})
				}
			}),
			method("cursorUpdate:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					c.applyCursor()
				}
			}),
			method("scrollWheel:", func(self id, _ objc.SEL, ev id) {
				c := canvasOf(self)
				if c == nil {
					return
				}
				dx, dy := scrollDelta(msgGetFloat(ev, sel("scrollingDeltaX")), msgGetFloat(ev, sel("scrollingDeltaY")),
					sendBool(ev, "hasPreciseScrollingDeltas"))
				if dx == 0 && dy == 0 {
					return
				}
				c.input(input.Scroll{Pos: c.loc(ev), DX: dx, DY: dy, Mods: eventMods(ev)})
			}),

			// Keyboard.
			method("keyDown:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					c.keyDown(ev)
				}
			}),
			method("keyUp:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					k := eventKey(ev)
					if c.downKeys[k] {
						delete(c.downKeys, k)
						c.input(input.KeyEvent{Key: k, Down: false, Mods: eventMods(ev)})
					}
				}
			}),
			method("flagsChanged:", func(self id, _ objc.SEL, ev id) {
				if c := canvasOf(self); c != nil {
					c.flagsChanged(ev)
				}
			}),
			method("copy:", menuKey(input.KeyC, input.ModMeta)),
			method("cut:", menuKey(input.KeyX, input.ModMeta)),
			method("paste:", menuKey(input.KeyV, input.ModMeta)),
			method("selectAll:", menuKey(input.KeyA, input.ModMeta)),
			method("undo:", menuKey(input.KeyZ, input.ModMeta)),
			method("redo:", menuKey(input.KeyZ, input.ModMeta|input.ModShift)),

			// The input context exists only while text input is on, so
			// input methods do not engage over non-text content.
			method("inputContext", func(self id, _ objc.SEL) id {
				if c := canvasOf(self); c == nil || !c.textOn {
					return 0
				}
				return msgSuper(superOf(self), sel("inputContext"))
			}),
		}
		methods = append(methods, textInputClientMethods()...)
		methods = append(methods, canvasDropMethods()...)     // canvas_drop.go
		methods = append(methods, canvasValidateMethods()...) // canvas_edit.go
		methods = append(methods, canvasA11yMethods()...)     // canvas_a11y.go
		classDef("TangoCanvasView", "NSView", []string{"NSTextInputClient"}, methods)
	})
}

func pointerHandler(kind input.PointerKind, button input.Button) func(self id, _ objc.SEL, ev id) {
	return func(self id, _ objc.SEL, ev id) {
		c := canvasOf(self)
		if c == nil {
			return
		}
		p := input.Pointer{Kind: kind, Pos: c.loc(ev), Button: button, Mods: eventMods(ev)}
		if button == input.ButtonMiddle && sendInt(ev, "buttonNumber") != 2 {
			return // back / forward and other extra buttons
		}
		switch kind {
		case input.Press:
			p.Clicks = sendInt(ev, "clickCount")
			if kind == input.Press && send(c.s.win, "firstResponder") != c.view {
				send(c.s.win, "makeFirstResponder:", uintptr(c.view))
			}
		case input.Release:
			p.Clicks = sendInt(ev, "clickCount")
		case input.Move:
			if c.inside {
				c.applyCursor()
			}
		}
		c.input(p)
	}
}

func menuKey(k input.Key, m input.Modifiers) func(self id, _ objc.SEL, sender id) {
	return func(self id, _ objc.SEL, sender id) {
		if c := canvasOf(self); c != nil {
			c.input(input.KeyEvent{Key: k, Down: true, Mods: m})
			c.input(input.KeyEvent{Key: k, Down: false, Mods: m})
		}
	}
}

// input delivers ev to the sink.
func (c *canvas) input(ev input.Event) {
	if c.ready && !c.closed {
		c.sink.CanvasInput(ev)
	}
}

// loc is the event's position in view coordinates (logical, top-left).
func (c *canvas) loc(ev id) geom.Point {
	p := msgConvertPoint(c.view, sel("convertPoint:fromView:"), msgGetPoint(ev, sel("locationInWindow")), 0)
	return geom.Pt(float32(p.X), float32(p.Y))
}

func eventMods(ev id) input.Modifiers { return modsFromMac(uint(send(ev, "modifierFlags"))) }

func eventKey(ev id) input.Key {
	var chars string
	withPool(func() { chars = goString(send(ev, "charactersIgnoringModifiers")) })
	return keyFromMac(uint16(send(ev, "keyCode")), chars)
}

func (c *canvas) keyDown(ev id) {
	k := input.KeyEvent{Key: eventKey(ev), Down: true, Repeat: sendBool(ev, "isARepeat"), Mods: eventMods(ev)}
	c.curKey = k
	if c.textOn && k.Mods&input.ModMeta == 0 {
		c.inKeyDown = true
		withPool(func() { send(c.view, "interpretKeyEvents:", uintptr(nsArray(ev))) })
		c.inKeyDown = false
		return
	}
	c.pressKey(k)
}

// flagsChanged reports a modifier key (Shift, Control, Option, Command,
// left and right) as a press or release of that key; a release is only
// reported for a reported press. Caps Lock only sends flagsChanged: when
// its state toggles (once per press, nothing on release), so each toggle
// is reported as a press immediately followed by a release.
func (c *canvas) flagsChanged(ev id) {
	code := uint16(send(ev, "keyCode"))
	flags := uint(send(ev, "modifierFlags"))
	c.flags = modsFromMac(flags)
	if code == macCapsLockCode {
		c.input(input.KeyEvent{Key: input.KeyCapsLock, Down: true, Mods: c.flags})
		c.input(input.KeyEvent{Key: input.KeyCapsLock, Down: false, Mods: c.flags})
		return
	}
	k, down, mods, ok := modifierKeyFromMac(code, flags)
	if !ok || down == c.downKeys[k] {
		return
	}
	if down {
		c.downKeys[k] = true
	} else {
		delete(c.downKeys, k)
	}
	c.input(input.KeyEvent{Key: k, Down: down, Mods: mods})
}

// pressKey reports a key press (and remembers it for keyUp:).
func (c *canvas) pressKey(k input.KeyEvent) {
	if k.Key == input.KeyUnknown {
		return
	}
	c.downKeys[k.Key] = true
	c.input(k)
}

// ---- NSTextInputClient ----

// nsStringArg returns the text of an NSString or NSAttributedString.
func nsStringArg(str id) string {
	if str == 0 {
		return ""
	}
	var s string
	withPool(func() {
		if sendBool(str, "isKindOfClass:", uintptr(class("NSAttributedString"))) {
			str = send(str, "string")
		}
		s = goString(str)
	})
	return s
}

func textInputClientMethods() []objc.MethodDef {
	return []objc.MethodDef{
		method("hasMarkedText", func(self id, _ objc.SEL) bool {
			c := canvasOf(self)
			return c != nil && c.marked != ""
		}),
		method("markedRange", func(self id, _ objc.SEL) nsRange {
			if c := canvasOf(self); c != nil && c.marked != "" {
				return nsRange{0, uint(utf16Len(c.marked))}
			}
			return nsRange{nsNotFound, 0}
		}),
		method("selectedRange", func(self id, _ objc.SEL) nsRange {
			// The view does not know the document; offsets are relative
			// to the marked text, the only text the input method sees.
			if c := canvasOf(self); c != nil && c.marked != "" {
				return nsRange{uint(c.markedSel), 0}
			}
			return nsRange{0, 0}
		}),
		method("setMarkedText:selectedRange:replacementRange:", func(self id, _ objc.SEL, str id, selected, replace nsRange) {
			c := canvasOf(self)
			if c == nil {
				return
			}
			text := nsStringArg(str)
			c.marked = text
			c.markedSel = min(int(selected.Location), utf16Len(text))
			if text == "" {
				c.markedSel = 0
			}
			c.input(input.Composition{Text: text, Cursor: runeIndex(text, c.markedSel)})
		}),
		method("unmarkText", func(self id, _ objc.SEL) {
			// Accept the marked text as if it had been inserted.
			c := canvasOf(self)
			if c == nil || c.marked == "" {
				return
			}
			text := c.marked
			c.marked, c.markedSel = "", 0
			c.input(input.TextInput{Text: text})
		}),
		method("validAttributesForMarkedText", func(self id, _ objc.SEL) id {
			return send(class("NSArray"), "array")
		}),
		method("attributedSubstringForProposedRange:actualRange:", func(self id, _ objc.SEL, r nsRange, actual unsafe.Pointer) id {
			return 0
		}),
		method("insertText:replacementRange:", func(self id, _ objc.SEL, str id, replace nsRange) {
			c := canvasOf(self)
			if c == nil {
				return
			}
			raw := nsStringArg(str)
			text := cleanInsert(raw)
			had := c.marked != ""
			c.marked, c.markedSel = "", 0
			switch {
			case text != "":
				c.input(input.TextInput{Text: text})
			case had:
				c.input(input.Composition{})
			case c.inKeyDown && raw != "":
				// A key without text (F5: a private-use function-key
				// character) the input method passed on: report the key.
				c.pressKey(c.curKey)
			}
		}),
		method("characterIndexForPoint:", func(self id, _ objc.SEL, p nsPoint) uint {
			return nsNotFound
		}),
		method("firstRectForCharacterRange:actualRange:", func(self id, _ objc.SEL, r nsRange, actual *nsRange) nsRect {
			c := canvasOf(self)
			if c == nil {
				return nsRect{}
			}
			if actual != nil {
				*actual = r
			}
			return c.caretScreenRect()
		}),
		method("doCommandBySelector:", func(self id, _ objc.SEL, cmd objc.SEL) {
			c := canvasOf(self)
			if c == nil {
				return
			}
			name := selGetName(cmd)
			if c.inKeyDown {
				if k, m, ok := commandKey(c.curKey.Key, c.curKey.Mods, name); ok {
					c.pressKey(input.KeyEvent{Key: k, Down: true, Repeat: c.curKey.Repeat, Mods: m})
				}
				return
			}
			if k, m, ok := keyFromSelector(name); ok {
				c.input(input.KeyEvent{Key: k, Down: true, Mods: m})
				c.input(input.KeyEvent{Key: k, Down: false, Mods: m})
			}
		}),
	}
}

// caretScreenRect converts the caret from SetTextInput to screen
// coordinates (AppKit's, bottom-left origin) for the candidate window.
func (c *canvas) caretScreenRect() nsRect {
	r := nsRect{Origin: nsPoint{float64(c.caret.X), float64(c.caret.Y)}, Size: nsSize{float64(c.caret.W), float64(c.caret.H)}}
	r.Size.Width = max(r.Size.Width, 1)
	inWin := msgConvertRect(c.view, sel("convertRect:toView:"), r, 0)
	return msgContentRectForRect(c.s.win, sel("convertRectToScreen:"), inWin)
}
