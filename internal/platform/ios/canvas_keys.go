package ios

// Hardware keyboard mapping of the canvas view (canvas.m pressesBegan:,
// docs/specs/mobile-canvas.md §2.2): UIKey key codes (USB HID usages,
// UIKeyboardHIDUsage) and modifier flags to input.KeyEvent, and the
// decision which presses the canvas takes. Kept free of build tags so it
// is tested on any host.

import (
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/input"
)

// UIKeyModifierFlags bits.
const (
	uiKeyModAlphaShift = 1 << 16 // Caps Lock
	uiKeyModShift      = 1 << 17
	uiKeyModControl    = 1 << 18
	uiKeyModAlternate  = 1 << 19
	uiKeyModCommand    = 1 << 20
	uiKeyModNumericPad = 1 << 21
)

// modsFromUIKit converts UIKey.modifierFlags.
func modsFromUIKit(flags int64) input.Modifiers {
	var m input.Modifiers
	if flags&uiKeyModShift != 0 {
		m |= input.ModShift
	}
	if flags&uiKeyModControl != 0 {
		m |= input.ModCtrl
	}
	if flags&uiKeyModAlternate != 0 {
		m |= input.ModAlt
	}
	if flags&uiKeyModCommand != 0 {
		m |= input.ModMeta
	}
	return m
}

// hidKeys maps the HID usages of keys whose meaning does not depend on the
// keyboard layout: editing and navigation, function keys, the keypad and
// the modifiers.
var hidKeys = map[uint16]input.Key{
	0x28: input.KeyEnter,     // Return
	0x29: input.KeyEscape,    // Escape
	0x2A: input.KeyBackspace, // Delete (Backspace)
	0x2B: input.KeyTab,
	0x2C: input.KeySpace,
	0x39: input.KeyCapsLock,

	0x3A: input.KeyF1,
	0x3B: input.KeyF2,
	0x3C: input.KeyF3,
	0x3D: input.KeyF4,
	0x3E: input.KeyF5,
	0x3F: input.KeyF6,
	0x40: input.KeyF7,
	0x41: input.KeyF8,
	0x42: input.KeyF9,
	0x43: input.KeyF10,
	0x44: input.KeyF11,
	0x45: input.KeyF12,

	0x49: input.KeyInsert,
	0x4A: input.KeyHome,
	0x4B: input.KeyPageUp,
	0x4C: input.KeyDelete, // Delete Forward
	0x4D: input.KeyEnd,
	0x4E: input.KeyPageDown,
	0x4F: input.KeyRight,
	0x50: input.KeyLeft,
	0x51: input.KeyDown,
	0x52: input.KeyUp,

	0x54: input.KeyNumpadDivide,
	0x55: input.KeyNumpadMultiply,
	0x56: input.KeyNumpadSubtract,
	0x57: input.KeyNumpadAdd,
	0x58: input.KeyNumpadEnter,
	0x59: input.KeyNumpad1,
	0x5A: input.KeyNumpad2,
	0x5B: input.KeyNumpad3,
	0x5C: input.KeyNumpad4,
	0x5D: input.KeyNumpad5,
	0x5E: input.KeyNumpad6,
	0x5F: input.KeyNumpad7,
	0x60: input.KeyNumpad8,
	0x61: input.KeyNumpad9,
	0x62: input.KeyNumpad0,
	0x63: input.KeyNumpadDecimal,
	0x65: input.KeyContextMenu, // Application

	0xE0: input.KeyCtrlLeft,
	0xE1: input.KeyShiftLeft,
	0xE2: input.KeyAltLeft,
	0xE3: input.KeyMetaLeft, // Left GUI (Command)
	0xE4: input.KeyCtrlRight,
	0xE5: input.KeyShiftRight,
	0xE6: input.KeyAltRight,
	0xE7: input.KeyMetaRight,
}

// hidPhysical names the main-block letter, digit and punctuation keys by
// their position on the US layout.
var hidPhysical = func() map[uint16]input.Key {
	m := map[uint16]input.Key{
		0x27: input.Key0,
		0x2D: input.KeyMinus,
		0x2E: input.KeyEqual,
		0x2F: input.KeyBracketLeft,
		0x30: input.KeyBracketRight,
		0x31: input.KeyBackslash,
		0x33: input.KeySemicolon,
		0x34: input.KeyQuote,
		0x35: input.KeyBackquote, // Grave Accent and Tilde
		0x36: input.KeyComma,
		0x37: input.KeyPeriod,
		0x38: input.KeySlash,
	}
	for i := 0; i < 26; i++ { // 0x04 a ... 0x1D z
		m[uint16(0x04+i)] = input.KeyForRune(rune('a' + i))
	}
	for i := 1; i <= 9; i++ { // 0x1E 1 ... 0x26 9
		m[uint16(0x1D+i)] = input.KeyForRune(rune('0' + i))
	}
	return m
}()

// keyFromHID maps a key press. Layout-independent keys come from the
// usage. Letters and digits come from charactersIgnoringModifiers, so
// shortcuts follow the active layout (Cmd+C on Dvorak is the key labelled
// C); other main-block keys are named by their US position. code is the
// physical key (the US-position name), or KeyUnknown.
func keyFromHID(usage uint16, charsIgnoringMods string) (key, code input.Key) {
	if k, ok := hidKeys[usage]; ok {
		return k, k
	}
	code = hidPhysical[usage]
	if r, n := utf8.DecodeRuneInString(charsIgnoringMods); n > 0 && n == len(charsIgnoringMods) {
		if k := input.KeyForRune(r); k != input.KeyUnknown {
			return k, code
		}
	}
	return code, code
}

// keyRoute says what the canvas does with a key press.
type keyRoute uint8

const (
	// routeSystem: not the canvas's; UIKit gets it (super). Keys the
	// canvas cannot name and that type nothing: the Globe key, media
	// and brightness keys.
	routeSystem keyRoute = iota
	// routeKey: report the KeyEvent and keep the press from UIKit.
	// Function and navigation keys, modifiers, and anything with
	// Command or Control (shortcuts) go this way, so a text field never
	// sees Backspace or Return twice (once as a key, once through the
	// text system as deleteBackward or insertText "\n"). Keys that type
	// go this way while text input is off.
	routeKey
	// routeText: a key for the text system while text input is on: the
	// press goes to UIKit (super), which types it through the view's
	// UITextInput (insertText:, or setMarkedText: for an input method
	// on a hardware keyboard), so no KeyEvent is reported. During a
	// composition the editing keys (Backspace, Return, arrows, Escape)
	// go this way too: they edit the marked text, not the field.
	routeText
)

func (r keyRoute) String() string {
	switch r {
	case routeSystem:
		return "system"
	case routeKey:
		return "key"
	case routeText:
		return "text"
	}
	return "keyRoute(?)"
}

// functionKey reports keys that never type a character.
func functionKey(k input.Key) bool {
	switch {
	case k >= input.KeyF1 && k <= input.KeyF12:
		return true
	case k.Modifier() != 0:
		return true
	}
	switch k {
	case input.KeyEnter, input.KeyNumpadEnter, input.KeyTab, input.KeyEscape, input.KeyBackspace,
		input.KeyDelete, input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown, input.KeyHome,
		input.KeyEnd, input.KeyPageUp, input.KeyPageDown, input.KeyInsert, input.KeyCapsLock,
		input.KeyContextMenu:
		return true
	}
	return false
}

// pressRoute decides the route of a key going down (§2.2). text is the
// cleaned characters the press types (see typedText); composing reports
// marked text in the view's UITextInput.
func pressRoute(k input.Key, mods input.Modifiers, text string, textOn, composing bool) keyRoute {
	shortcut := mods&(input.ModCtrl|input.ModMeta) != 0
	if textOn && composing && !shortcut {
		return routeText
	}
	if k == input.KeyUnknown {
		if textOn && text != "" && !shortcut {
			return routeText // a key of a non-US layout (e.g. §) that types
		}
		return routeSystem
	}
	if functionKey(k) || shortcut {
		return routeKey
	}
	if textOn && text != "" {
		return routeText
	}
	return routeKey
}

// typedText drops what is not text from UIKey.characters: control
// characters, DEL and the private-use characters and "UIKeyInput..."
// names UIKit reports for function keys.
func typedText(s string) string {
	if strings.HasPrefix(s, "UIKeyInput") {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0xF700 && r <= 0xF8FF) {
			return -1
		}
		return r
	}, s)
}

// keyPress is the decoded press of one key going down.
type keyPress struct {
	ev    input.KeyEvent
	route keyRoute
}

// decodePress decodes a key going down (UIKey keyCode, modifierFlags,
// characters, charactersIgnoringModifiers) while text input is textOn
// and the view has marked text (composing).
func decodePress(usage uint16, flags int64, chars, charsIgnoringMods string, textOn, composing bool) keyPress {
	k, code := keyFromHID(usage, charsIgnoringMods)
	mods := modsFromUIKit(flags)
	if bit := k.Modifier(); bit != 0 {
		mods |= bit // a modifier's own press includes it
	}
	p := keyPress{route: pressRoute(k, mods, typedText(chars), textOn, composing)}
	p.ev = input.KeyEvent{Key: k, Down: true, Mods: mods}
	if code != k && code != input.KeyUnknown {
		p.ev.Code = code
	}
	return p
}

// releaseEvent is the KeyEvent of a key going up that went down as down.
// flags are the modifiers at the release.
func releaseEvent(down input.KeyEvent, flags int64) input.KeyEvent {
	mods := modsFromUIKit(flags)
	if bit := down.Key.Modifier(); bit != 0 {
		mods &^= bit // a modifier's own release excludes it
	}
	return input.KeyEvent{Key: down.Key, Code: down.Code, Down: false, Mods: mods}
}
