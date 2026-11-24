package darwin

// Platform-independent helpers of the canvas view (canvas_view.go): key,
// modifier, selector, cursor and scroll mapping and UTF-16 offsets. Kept
// free of build tags so they are tested everywhere.

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/input"
)

// NSEventModifierFlags bits.
const (
	nsShiftKeyMask     = 1 << 17
	nsControlKeyMask   = 1 << 18
	nsAlternateKeyMask = 1 << 19
	nsCommandKeyMask   = 1 << 20
)

// modsFromMac converts NSEvent.modifierFlags.
func modsFromMac(flags uint) input.Modifiers {
	var m input.Modifiers
	if flags&nsShiftKeyMask != 0 {
		m |= input.ModShift
	}
	if flags&nsControlKeyMask != 0 {
		m |= input.ModCtrl
	}
	if flags&nsAlternateKeyMask != 0 {
		m |= input.ModAlt
	}
	if flags&nsCommandKeyMask != 0 {
		m |= input.ModMeta
	}
	return m
}

// Device-dependent modifier bits of NSEvent.modifierFlags (IOKit
// IOLLEvent.h NX_DEVICE*KEYMASK): which side of a modifier is held.
const (
	nxDeviceLCtlKeyMask   = 0x0001
	nxDeviceLShiftKeyMask = 0x0002
	nxDeviceRShiftKeyMask = 0x0004
	nxDeviceLCmdKeyMask   = 0x0008
	nxDeviceRCmdKeyMask   = 0x0010
	nxDeviceLAltKeyMask   = 0x0020
	nxDeviceRAltKeyMask   = 0x0040
	nxDeviceRCtlKeyMask   = 0x2000
)

// Virtual key codes (Carbon kVK_*) of keys whose meaning does not depend
// on the keyboard layout: editing / navigation, function keys, the
// keypad. The keypad always reports Numpad keys (Mac keypads have no
// NumLock; kVK_ANSI_KeypadClear and KeypadEquals have no Key).
var macKeyCodes = map[uint16]input.Key{
	0x24: input.KeyEnter,     // kVK_Return
	0x30: input.KeyTab,       // kVK_Tab
	0x31: input.KeySpace,     // kVK_Space
	0x33: input.KeyBackspace, // kVK_Delete
	0x35: input.KeyEscape,    // kVK_Escape
	0x75: input.KeyDelete,    // kVK_ForwardDelete
	0x72: input.KeyInsert,    // kVK_Help: the Insert key of PC keyboards
	0x6E: input.KeyContextMenu,
	0x7B: input.KeyLeft,
	0x7C: input.KeyRight,
	0x7D: input.KeyDown,
	0x7E: input.KeyUp,
	0x73: input.KeyHome,
	0x77: input.KeyEnd,
	0x74: input.KeyPageUp,
	0x79: input.KeyPageDown,

	0x7A: input.KeyF1,
	0x78: input.KeyF2,
	0x63: input.KeyF3,
	0x76: input.KeyF4,
	0x60: input.KeyF5,
	0x61: input.KeyF6,
	0x62: input.KeyF7,
	0x64: input.KeyF8,
	0x65: input.KeyF9,
	0x6D: input.KeyF10,
	0x67: input.KeyF11,
	0x6F: input.KeyF12,

	0x52: input.KeyNumpad0,
	0x53: input.KeyNumpad1,
	0x54: input.KeyNumpad2,
	0x55: input.KeyNumpad3,
	0x56: input.KeyNumpad4,
	0x57: input.KeyNumpad5,
	0x58: input.KeyNumpad6,
	0x59: input.KeyNumpad7,
	0x5B: input.KeyNumpad8,
	0x5C: input.KeyNumpad9,
	0x45: input.KeyNumpadAdd,
	0x4E: input.KeyNumpadSubtract,
	0x43: input.KeyNumpadMultiply,
	0x4B: input.KeyNumpadDivide,
	0x41: input.KeyNumpadDecimal,
	0x4C: input.KeyNumpadEnter, // kVK_ANSI_KeypadEnter
}

// Virtual key codes of the main-block digit and punctuation keys, by
// their position (kVK_ANSI_* names the US layout key). Used when the
// character the key produces is not a letter or digit (see keyFromMac).
var macPhysicalKeys = map[uint16]input.Key{
	0x1D: input.Key0,
	0x12: input.Key1,
	0x13: input.Key2,
	0x14: input.Key3,
	0x15: input.Key4,
	0x17: input.Key5,
	0x16: input.Key6,
	0x1A: input.Key7,
	0x1C: input.Key8,
	0x19: input.Key9,

	0x1B: input.KeyMinus,
	0x18: input.KeyEqual,
	0x21: input.KeyBracketLeft,
	0x1E: input.KeyBracketRight,
	0x2A: input.KeyBackslash,
	0x29: input.KeySemicolon,
	0x27: input.KeyQuote,
	0x2B: input.KeyComma,
	0x2F: input.KeyPeriod,
	0x2C: input.KeySlash,
	0x32: input.KeyBackquote, // kVK_ANSI_Grave
}

// keyFromMac maps a keyDown:/keyUp: event. Layout-independent keys come
// from the virtual key code. Letters and digits come from
// charactersIgnoringModifiers, so shortcuts follow the active keyboard
// layout (Cmd+C on Dvorak is the key labelled C). Other keys of the main
// block (punctuation, and digits whose unshifted character is not a digit,
// e.g. on AZERTY) are named by their physical position on the US layout.
func keyFromMac(code uint16, charsIgnoringMods string) input.Key {
	if k, ok := macKeyCodes[code]; ok {
		return k
	}
	if r, n := utf8.DecodeRuneInString(charsIgnoringMods); n > 0 && n == len(charsIgnoringMods) {
		if k := input.KeyForRune(r); k != input.KeyUnknown {
			return k
		}
	}
	return macPhysicalKeys[code]
}

// macModifier describes a modifier key: its Key, the device-dependent
// bit of its own side and of the other side.
type macModifier struct {
	key         input.Key
	side, other uint
}

// Virtual key codes of the modifier keys (flagsChanged:).
var macModifierKeys = map[uint16]macModifier{
	0x38: {input.KeyShiftLeft, nxDeviceLShiftKeyMask, nxDeviceRShiftKeyMask},
	0x3C: {input.KeyShiftRight, nxDeviceRShiftKeyMask, nxDeviceLShiftKeyMask},
	0x3B: {input.KeyCtrlLeft, nxDeviceLCtlKeyMask, nxDeviceRCtlKeyMask},
	0x3E: {input.KeyCtrlRight, nxDeviceRCtlKeyMask, nxDeviceLCtlKeyMask},
	0x3A: {input.KeyAltLeft, nxDeviceLAltKeyMask, nxDeviceRAltKeyMask},
	0x3D: {input.KeyAltRight, nxDeviceRAltKeyMask, nxDeviceLAltKeyMask},
	0x37: {input.KeyMetaLeft, nxDeviceLCmdKeyMask, nxDeviceRCmdKeyMask},
	0x36: {input.KeyMetaRight, nxDeviceRCmdKeyMask, nxDeviceLCmdKeyMask},
}

const macCapsLockCode = 0x39 // kVK_CapsLock

// modifierKeyFromMac interprets a flagsChanged: event of a Shift, Control,
// Option or Command key: which key, whether it went down, and the modifier
// set after the event. flagsChanged: only carries the new flags, so the
// side's device bit tells press from release; events without device bits
// (some synthesized ones) fall back to the generic bit, unless the other
// side's device bit says that one is held. ok is false for other keys
// (Caps Lock, Fn, unknown codes).
func modifierKeyFromMac(code uint16, flags uint) (k input.Key, down bool, mods input.Modifiers, ok bool) {
	mk, ok := macModifierKeys[code]
	if !ok {
		return input.KeyUnknown, false, 0, false
	}
	mods = modsFromMac(flags)
	held := mods&mk.key.Modifier() != 0
	down = flags&mk.side != 0 || held && flags&mk.other == 0
	return mk.key, down, modifierEventMods(mk.key, down, mods, flags&mk.other != 0), true
}

// modifierEventMods is the Mods of a modifier key's own event, the same
// rule on every desktop backend: a press includes the key's modifier, a
// release excludes it unless the other side's key is still held.
func modifierEventMods(k input.Key, down bool, mods input.Modifiers, otherHeld bool) input.Modifiers {
	bit := k.Modifier()
	if down || otherHeld {
		return mods | bit
	}
	return mods &^ bit
}

type selKey struct {
	key  input.Key
	mods input.Modifiers
}

// Standard key bindings (NSStandardKeyBindingResponding) that the text
// system sends through doCommandBySelector:, mapped to keys.
var selectorKeys = map[string]selKey{
	"insertNewline:":                                {input.KeyEnter, 0},
	"insertNewlineIgnoringFieldEditor:":             {input.KeyEnter, 0},
	"insertLineBreak:":                              {input.KeyEnter, input.ModShift},
	"insertTab:":                                    {input.KeyTab, 0},
	"insertTabIgnoringFieldEditor:":                 {input.KeyTab, 0},
	"insertBacktab:":                                {input.KeyTab, input.ModShift},
	"cancelOperation:":                              {input.KeyEscape, 0},
	"deleteBackward:":                               {input.KeyBackspace, 0},
	"deleteBackwardByDecomposingPreviousCharacter:": {input.KeyBackspace, 0},
	"deleteForward:":                                {input.KeyDelete, 0},
	"deleteWordBackward:":                           {input.KeyBackspace, input.ModAlt},
	"deleteWordForward:":                            {input.KeyDelete, input.ModAlt},
	"moveLeft:":                                     {input.KeyLeft, 0},
	"moveRight:":                                    {input.KeyRight, 0},
	"moveUp:":                                       {input.KeyUp, 0},
	"moveDown:":                                     {input.KeyDown, 0},
	"moveBackward:":                                 {input.KeyLeft, 0},
	"moveForward:":                                  {input.KeyRight, 0},
	"moveLeftAndModifySelection:":                   {input.KeyLeft, input.ModShift},
	"moveRightAndModifySelection:":                  {input.KeyRight, input.ModShift},
	"moveUpAndModifySelection:":                     {input.KeyUp, input.ModShift},
	"moveDownAndModifySelection:":                   {input.KeyDown, input.ModShift},
	"moveWordLeft:":                                 {input.KeyLeft, input.ModAlt},
	"moveWordRight:":                                {input.KeyRight, input.ModAlt},
	"moveWordLeftAndModifySelection:":               {input.KeyLeft, input.ModAlt | input.ModShift},
	"moveWordRightAndModifySelection:":              {input.KeyRight, input.ModAlt | input.ModShift},
	"moveToBeginningOfLine:":                        {input.KeyHome, 0},
	"moveToEndOfLine:":                              {input.KeyEnd, 0},
	"moveToLeftEndOfLine:":                          {input.KeyHome, 0},
	"moveToRightEndOfLine:":                         {input.KeyEnd, 0},
	"moveToBeginningOfParagraph:":                   {input.KeyHome, 0},
	"moveToEndOfParagraph:":                         {input.KeyEnd, 0},
	"moveToBeginningOfLineAndModifySelection:":      {input.KeyHome, input.ModShift},
	"moveToEndOfLineAndModifySelection:":            {input.KeyEnd, input.ModShift},
	"moveToLeftEndOfLineAndModifySelection:":        {input.KeyHome, input.ModShift},
	"moveToRightEndOfLineAndModifySelection:":       {input.KeyEnd, input.ModShift},
	"moveToBeginningOfDocument:":                    {input.KeyHome, input.ModMeta},
	"moveToEndOfDocument:":                          {input.KeyEnd, input.ModMeta},
	"scrollToBeginningOfDocument:":                  {input.KeyHome, 0},
	"scrollToEndOfDocument:":                        {input.KeyEnd, 0},
	"pageUp:":                                       {input.KeyPageUp, 0},
	"pageDown:":                                     {input.KeyPageDown, 0},
	"scrollPageUp:":                                 {input.KeyPageUp, 0},
	"scrollPageDown:":                               {input.KeyPageDown, 0},
	"pageUpAndModifySelection:":                     {input.KeyPageUp, input.ModShift},
	"pageDownAndModifySelection:":                   {input.KeyPageDown, input.ModShift},
	"selectAll:":                                    {input.KeyA, input.ModMeta},
}

// keyFromSelector maps a doCommandBySelector: selector.
func keyFromSelector(name string) (input.Key, input.Modifiers, bool) {
	k, ok := selectorKeys[name]
	return k.key, k.mods, ok
}

// commandKey picks the key reported for doCommandBySelector: during
// keyDown. The physical key wins when it is a known key (so Shift+Left is
// Left with the real modifiers); otherwise a known selector supplies it
// (Ctrl+A → moveToBeginningOfLine: → Home). A keypad key whose main-block
// twin the selector names (keypad Enter → insertNewline:) stays the keypad
// key.
func commandKey(raw input.Key, rawMods input.Modifiers, selector string) (input.Key, input.Modifiers, bool) {
	sk, smods, sok := keyFromSelector(selector)
	if raw != input.KeyUnknown && (!sok || sk == raw || sk == raw.Base()) {
		return raw, rawMods, true
	}
	if sok {
		return sk, smods, true
	}
	return input.KeyUnknown, 0, false
}

// cursorSelector is the NSCursor class method for c.
func cursorSelector(c input.Cursor) string {
	switch c {
	case input.CursorPointer:
		return "pointingHandCursor"
	case input.CursorText:
		return "IBeamCursor"
	case input.CursorNotAllowed:
		return "operationNotAllowedCursor"
	case input.CursorResizeEW:
		return "resizeLeftRightCursor"
	case input.CursorResizeNS:
		return "resizeUpDownCursor"
	case input.CursorGrab, input.CursorMove:
		return "openHandCursor"
	case input.CursorGrabbing:
		return "closedHandCursor"
	case input.CursorCopy:
		return "dragCopyCursor"
	}
	return "arrowCursor"
}

// scrollLine is the logical pixels per wheel line for mice without
// precise deltas.
const scrollLine = 40

// scrollDelta converts NSEvent scrollingDeltaX/Y to input.Scroll deltas:
// positive DY reveals content below. AppKit's deltas are positive when the
// content should move down/right (already adjusted for "natural"
// scrolling), so they are negated.
func scrollDelta(dx, dy float64, precise bool) (float32, float32) {
	if !precise {
		dx, dy = dx*scrollLine, dy*scrollLine
	}
	return float32(0 - dx), float32(0 - dy) // 0-x: no negative zero
}

// utf16Len is the length of s in UTF-16 code units (NSString length).
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// runeIndex converts a UTF-16 offset into s to a rune offset, clamped.
func runeIndex(s string, u16 int) int {
	n, i := 0, 0
	for _, r := range s {
		if n >= u16 {
			return i
		}
		n += utf16.RuneLen(r)
		i++
	}
	return i
}

// cleanInsert drops the control characters some input sources insert
// (function-key private-use characters, DEL); text input carries text only.
func cleanInsert(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0xF700 && r <= 0xF8FF) {
			return -1
		}
		return r
	}, s)
}

// macHIDKeys maps virtual key codes (Carbon kVK_*) to USB HID usages on
// the Keyboard/Keypad page, by physical position. kVK_Function (Fn) has no
// keyboard usage and is absent.
var macHIDKeys = map[uint16]uint16{
	// Letters.
	0x00: 0x04, 0x0B: 0x05, 0x08: 0x06, 0x02: 0x07, 0x0E: 0x08, 0x03: 0x09, // A B C D E F
	0x05: 0x0A, 0x04: 0x0B, 0x22: 0x0C, 0x26: 0x0D, 0x28: 0x0E, 0x25: 0x0F, // G H I J K L
	0x2E: 0x10, 0x2D: 0x11, 0x1F: 0x12, 0x23: 0x13, 0x0C: 0x14, 0x0F: 0x15, // M N O P Q R
	0x01: 0x16, 0x11: 0x17, 0x20: 0x18, 0x09: 0x19, 0x0D: 0x1A, 0x07: 0x1B, // S T U V W X
	0x10: 0x1C, 0x06: 0x1D, // Y Z
	// Digits 1-9, 0.
	0x12: 0x1E, 0x13: 0x1F, 0x14: 0x20, 0x15: 0x21, 0x17: 0x22,
	0x16: 0x23, 0x1A: 0x24, 0x1C: 0x25, 0x19: 0x26, 0x1D: 0x27,
	// Editing and punctuation.
	0x24: 0x28, // Return
	0x35: 0x29, // Escape
	0x33: 0x2A, // Delete (Backspace)
	0x30: 0x2B, // Tab
	0x31: 0x2C, // Space
	0x1B: 0x2D, // Minus
	0x18: 0x2E, // Equal
	0x21: 0x2F, // LeftBracket
	0x1E: 0x30, // RightBracket
	0x2A: 0x31, // Backslash
	0x29: 0x33, // Semicolon
	0x27: 0x34, // Quote
	0x32: 0x35, // Grave
	0x2B: 0x36, // Comma
	0x2F: 0x37, // Period
	0x2C: 0x38, // Slash
	0x39: 0x39, // CapsLock
	0x0A: 0x64, // ISO_Section: the key next to left Shift on ISO keyboards
	// Function keys.
	0x7A: 0x3A, 0x78: 0x3B, 0x63: 0x3C, 0x76: 0x3D, 0x60: 0x3E, 0x61: 0x3F, // F1-F6
	0x62: 0x40, 0x64: 0x41, 0x65: 0x42, 0x6D: 0x43, 0x67: 0x44, 0x6F: 0x45, // F7-F12
	0x69: 0x68, 0x6B: 0x69, 0x71: 0x6A, 0x6A: 0x6B, 0x40: 0x6C, 0x4F: 0x6D, // F13-F18
	0x50: 0x6E, 0x5A: 0x6F, // F19 F20
	// Navigation.
	0x72: 0x49, // Help: the Insert key of PC keyboards
	0x73: 0x4A, // Home
	0x74: 0x4B, // PageUp
	0x75: 0x4C, // ForwardDelete
	0x77: 0x4D, // End
	0x79: 0x4E, // PageDown
	0x7C: 0x4F, // Right
	0x7B: 0x50, // Left
	0x7D: 0x51, // Down
	0x7E: 0x52, // Up
	0x6E: 0x65, // ContextMenu (Application)
	// Keypad.
	0x47: 0x53,                                                 // KeypadClear: NumLock's position on PC keypads
	0x4B: 0x54, 0x43: 0x55, 0x4E: 0x56, 0x45: 0x57, 0x4C: 0x58, // / * - + Enter
	0x53: 0x59, 0x54: 0x5A, 0x55: 0x5B, 0x56: 0x5C, 0x57: 0x5D, // 1-5
	0x58: 0x5E, 0x59: 0x5F, 0x5B: 0x60, 0x5C: 0x61, 0x52: 0x62, // 6-9 0
	0x41: 0x63, // Decimal
	0x51: 0x67, // Equals
	0x5F: 0x85, // JIS_KeypadComma
	// Media keys that report key codes.
	0x4A: 0x7F, 0x48: 0x80, 0x49: 0x81, // Mute VolumeUp VolumeDown
	// JIS.
	0x5E: 0x87, // JIS_Underscore (International1, Ro)
	0x5D: 0x89, // JIS_Yen (International3)
	0x68: 0x90, // JIS_Kana (LANG1)
	0x66: 0x91, // JIS_Eisu (LANG2)
	// Modifiers.
	0x3B: 0xE0, 0x38: 0xE1, 0x3A: 0xE2, 0x37: 0xE3, // left Control Shift Option Command
	0x3E: 0xE4, 0x3C: 0xE5, 0x3D: 0xE6, 0x36: 0xE7, // right Control Shift Option Command
}

// hidFromMac returns the USB HID usage of a virtual key code, 0 if none.
func hidFromMac(code uint16) uint16 { return macHIDKeys[code] }
