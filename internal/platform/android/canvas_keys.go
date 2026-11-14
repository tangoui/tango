package android

// Platform-independent helpers of the Android canvas (canvas_android.go):
// KeyEvent key codes and meta state to input.Key / input.Modifiers, the
// cursor table and how motion events the host packs into a float array
// become input events. Kept free of build tags so they are unit tested on
// every OS.
//
// Numbers are the public constants of android.view.KeyEvent,
// android.view.MotionEvent and android.view.PointerIcon (API 24+).

import (
	"github.com/tangoui/tango/ui/input"
)

// KeyEvent.getMetaState bits.
const (
	akMetaShiftOn      = 0x1
	akMetaAltOn        = 0x2
	akMetaAltLeftOn    = 0x10
	akMetaAltRightOn   = 0x20
	akMetaShiftLeftOn  = 0x40
	akMetaShiftRightOn = 0x80
	akMetaCtrlOn       = 0x1000
	akMetaCtrlLeftOn   = 0x2000
	akMetaCtrlRightOn  = 0x4000
	akMetaMetaOn       = 0x10000
	akMetaMetaLeftOn   = 0x20000
	akMetaMetaRightOn  = 0x40000
	akMetaNumLockOn    = 0x200000
)

// KeyEvent.KEYCODE_* values used below.
const (
	akDpadUp       = 19
	akDpadDown     = 20
	akDpadLeft     = 21
	akDpadRight    = 22
	akDpadCenter   = 23
	akComma        = 55
	akPeriod       = 56
	akAltLeft      = 57
	akAltRight     = 58
	akShiftLeft    = 59
	akShiftRight   = 60
	akTab          = 61
	akSpace        = 62
	akEnter        = 66
	akDel          = 67 // Backspace
	akGrave        = 68
	akMinus        = 69
	akEquals       = 70
	akLeftBracket  = 71
	akRightBracket = 72
	akBackslash    = 73
	akSemicolon    = 74
	akApostrophe   = 75
	akSlash        = 76
	akMenu         = 82
	akPageUp       = 92
	akPageDown     = 93
	akEscape       = 111
	akForwardDel   = 112
	akCtrlLeft     = 113
	akCtrlRight    = 114
	akCapsLock     = 115
	akMetaLeft     = 117
	akMetaRight    = 118
	akMoveHome     = 122
	akMoveEnd      = 123
	akInsert       = 124
	akF1           = 131 // ... akF1+11 = F12
	akNumpad0      = 144 // ... akNumpad0+9 = Numpad 9
	akNumpadDivide = 154
	akNumpadMul    = 155
	akNumpadSub    = 156
	akNumpadAdd    = 157
	akNumpadDot    = 158
	akNumpadEnter  = 160

	akA    = 29 // ... akA+25 = Z
	akNum0 = 7  // ... akNum0+9 = 9
	akBack = 4
)

// akKeys maps key codes whose meaning does not depend on the layout.
var akKeys = map[int]input.Key{
	akDpadUp: input.KeyUp, akDpadDown: input.KeyDown, akDpadLeft: input.KeyLeft, akDpadRight: input.KeyRight,
	akDpadCenter: input.KeyEnter,
	akComma:      input.KeyComma, akPeriod: input.KeyPeriod,
	akAltLeft: input.KeyAltLeft, akAltRight: input.KeyAltRight,
	akShiftLeft: input.KeyShiftLeft, akShiftRight: input.KeyShiftRight,
	akTab: input.KeyTab, akSpace: input.KeySpace, akEnter: input.KeyEnter, akDel: input.KeyBackspace,
	akGrave: input.KeyBackquote, akMinus: input.KeyMinus, akEquals: input.KeyEqual,
	akLeftBracket: input.KeyBracketLeft, akRightBracket: input.KeyBracketRight, akBackslash: input.KeyBackslash,
	akSemicolon: input.KeySemicolon, akApostrophe: input.KeyQuote, akSlash: input.KeySlash,
	akMenu: input.KeyContextMenu, akPageUp: input.KeyPageUp, akPageDown: input.KeyPageDown,
	akEscape: input.KeyEscape, akForwardDel: input.KeyDelete,
	akCtrlLeft: input.KeyCtrlLeft, akCtrlRight: input.KeyCtrlRight, akCapsLock: input.KeyCapsLock,
	akMetaLeft: input.KeyMetaLeft, akMetaRight: input.KeyMetaRight,
	akMoveHome: input.KeyHome, akMoveEnd: input.KeyEnd, akInsert: input.KeyInsert,
	akNumpadDivide: input.KeyNumpadDivide, akNumpadMul: input.KeyNumpadMultiply,
	akNumpadSub: input.KeyNumpadSubtract, akNumpadAdd: input.KeyNumpadAdd,
	akNumpadDot: input.KeyNumpadDecimal, akNumpadEnter: input.KeyNumpadEnter,
}

// akNumpadNav is what keypad keys stand for with NumLock off (the
// input.KeyEvent convention); keypad 5 is KeyUnknown.
var akNumpadNav = [10]input.Key{
	input.KeyInsert, input.KeyEnd, input.KeyDown, input.KeyPageDown, input.KeyLeft,
	input.KeyUnknown, input.KeyRight, input.KeyHome, input.KeyUp, input.KeyPageUp,
}

// canvasMods converts a meta state to modifiers.
func canvasMods(meta int) input.Modifiers {
	var m input.Modifiers
	if meta&akMetaShiftOn != 0 {
		m |= input.ModShift
	}
	if meta&akMetaCtrlOn != 0 {
		m |= input.ModCtrl
	}
	if meta&akMetaAltOn != 0 {
		m |= input.ModAlt
	}
	if meta&akMetaMetaOn != 0 {
		m |= input.ModMeta
	}
	return m
}

// modifierOf returns the modifier a modifier key sets and the meta bit of
// the same modifier on the other side; 0 for other keys.
func modifierOf(code int) (input.Modifiers, int) {
	switch code {
	case akShiftLeft:
		return input.ModShift, akMetaShiftRightOn
	case akShiftRight:
		return input.ModShift, akMetaShiftLeftOn
	case akCtrlLeft:
		return input.ModCtrl, akMetaCtrlRightOn
	case akCtrlRight:
		return input.ModCtrl, akMetaCtrlLeftOn
	case akAltLeft:
		return input.ModAlt, akMetaAltRightOn
	case akAltRight:
		return input.ModAlt, akMetaAltLeftOn
	case akMetaLeft:
		return input.ModMeta, akMetaMetaRightOn
	case akMetaRight:
		return input.ModMeta, akMetaMetaLeftOn
	}
	return 0, 0
}

// canvasKey maps a key code to the reported Key and the physical Code (0:
// same as Key). base is the character the key produces in the current
// layout without modifiers (KeyEvent.getUnicodeChar(0)), which decides
// letters and digits as input.KeyEvent asks.
func canvasKey(code, base, meta int) (key, phys input.Key) {
	switch {
	case code >= akNumpad0 && code <= akNumpad0+9:
		n := code - akNumpad0
		if meta&akMetaNumLockOn != 0 {
			return input.KeyNumpad0 + input.Key(n), 0
		}
		return akNumpadNav[n], input.KeyNumpad0 + input.Key(n)
	case code == akNumpadDot && meta&akMetaNumLockOn == 0:
		return input.KeyDelete, input.KeyNumpadDecimal
	}
	if k := input.KeyForRune(rune(base)); k != input.KeyUnknown {
		return k, 0
	}
	switch {
	case code >= akA && code <= akA+25:
		return input.KeyForRune(rune('a' + code - akA)), 0
	case code >= akNum0 && code <= akNum0+9:
		return input.Key0 + input.Key(code-akNum0), 0
	case code >= akF1 && code <= akF1+11:
		return input.KeyF1 + input.Key(code-akF1), 0
	}
	return akKeys[code], 0
}

// canvasKeys turns KeyEvents into input.KeyEvent values. A release reports
// the Key/Code of its press even if NumLock or the layout changed in
// between. Main thread.
type canvasKeys struct {
	down map[int][2]input.Key // key code -> Key, Code reported at the press
	back bool                 // Back is delivered as Escape (SetBackHandling)
}

// event converts one KeyEvent. ok is false for keys the canvas leaves to
// the system (Back while back handling is off, volume, unknown codes).
// While back handling is on, Back (delivered by the Java host's
// deliverBack) is KeyEscape with Code KeyBack.
func (s *canvasKeys) event(down bool, code, base, meta, repeat int) (input.KeyEvent, bool) {
	prev, held := s.down[code]
	if code == akBack && !s.back && !held {
		return input.KeyEvent{}, false
	}
	var key, phys input.Key
	switch {
	case held:
		key, phys = prev[0], prev[1]
	case code == akBack:
		key, phys = input.KeyEscape, input.KeyBack
	default:
		key, phys = canvasKey(code, base, meta)
	}
	if key == input.KeyUnknown && phys == input.KeyUnknown {
		return input.KeyEvent{}, false
	}
	if down {
		if s.down == nil {
			s.down = map[int][2]input.Key{}
		}
		s.down[code] = [2]input.Key{key, phys}
	} else {
		delete(s.down, code)
	}
	mods := canvasMods(meta)
	if m, other := modifierOf(code); m != 0 {
		if down {
			mods |= m
		} else if meta&other == 0 {
			mods &^= m
		}
	}
	return input.KeyEvent{Key: key, Code: phys, Down: down, Repeat: down && repeat > 0, Mods: mods}, true
}

// reset forgets the pressed keys (focus lost: their releases go elsewhere).
func (s *canvasKeys) reset() { clear(s.down) }

// canvasText reports whether a key press should also deliver its
// character as input.TextInput while text input is on: a printable
// character and no Ctrl, Alt or Meta (those are shortcuts). unicode is
// KeyEvent.getUnicodeChar() with the event's meta state.
func canvasText(unicode, meta int) (string, bool) {
	if unicode <= 0 || unicode&0x80000000 != 0 || meta&(akMetaCtrlOn|akMetaMetaOn|akMetaAltOn) != 0 {
		return "", false
	}
	r := rune(unicode)
	if r < 0x20 || r == 0x7f {
		return "", false
	}
	return string(r), true
}

// PointerIcon.TYPE_* values.
const (
	apiArrow       = 1000
	apiHand        = 1002
	apiText        = 1008
	apiAllScroll   = 1013
	apiCopy        = 1011
	apiNoDrop      = 1012
	apiHorizontal  = 1014
	apiVertical    = 1015
	apiGrab        = 1020
	apiGrabbing    = 1021
	apiDefaultType = apiArrow
)

// canvasPointerIcon maps a cursor to a PointerIcon type (mouse pointers on
// tablets, ChromeOS and desktop mode).
func canvasPointerIcon(c input.Cursor) int {
	switch c {
	case input.CursorPointer:
		return apiHand
	case input.CursorText:
		return apiText
	case input.CursorNotAllowed:
		return apiNoDrop
	case input.CursorCopy:
		return apiCopy
	case input.CursorResizeEW:
		return apiHorizontal
	case input.CursorResizeNS:
		return apiVertical
	case input.CursorGrab:
		return apiGrab
	case input.CursorGrabbing:
		return apiGrabbing
	case input.CursorMove:
		return apiAllScroll
	}
	return apiDefaultType
}
