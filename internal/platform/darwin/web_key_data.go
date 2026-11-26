package darwin

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var macWebKeyCodes = func() map[input.Key]uint16 {
	codes := make(map[input.Key]uint16)
	for code, key := range macKeyCodes {
		codes[key] = code
	}
	for code, key := range macPhysicalKeys {
		codes[key] = code
	}
	for code, modifier := range macModifierKeys {
		codes[modifier.key] = code
	}
	// kVK_ANSI_A through kVK_ANSI_Z, in alphabetical order.
	for i, code := range [...]uint16{0, 11, 8, 2, 14, 3, 5, 4, 34, 38, 40, 37, 46, 45, 31, 35, 12, 15, 1, 17, 32, 9, 13, 7, 16, 6} {
		codes[input.KeyForRune('a'+rune(i))] = code
	}
	codes[input.KeyCapsLock] = macCapsLockCode
	return codes
}()

func macWebFlags(mods input.Modifiers) uint {
	var flags uint
	for _, m := range [...]struct {
		mod  input.Modifiers
		mask uint
	}{{input.ModShift, nsShiftKeyMask}, {input.ModCtrl, nsControlKeyMask}, {input.ModAlt, nsAlternateKeyMask}, {input.ModMeta, nsCommandKeyMask}} {
		if mods&m.mod != 0 {
			flags |= m.mask
		}
	}
	return flags
}

// macOptionChars is the Option layer of the U.S. (ABC) layout, matching
// the kVK_ANSI key codes sent: unshifted and Shift+Option characters per
// base key. A zero rune is a dead key, which produces no characters on its
// own.
var macOptionChars = map[rune][2]rune{
	'`': {0, '`'}, '1': {'¡', '⁄'}, '2': {'™', '€'}, '3': {'£', '‹'}, '4': {'¢', '›'},
	'5': {'∞', 'ﬁ'}, '6': {'§', 'ﬂ'}, '7': {'¶', '‡'}, '8': {'•', '°'}, '9': {'ª', '·'},
	'0': {'º', '‚'}, '-': {'–', '—'}, '=': {'≠', '±'},
	'q': {'œ', 'Œ'}, 'w': {'∑', '„'}, 'e': {0, '´'}, 'r': {'®', '‰'}, 't': {'†', 'ˇ'},
	'y': {'¥', 'Á'}, 'u': {0, '¨'}, 'i': {0, 'ˆ'}, 'o': {'ø', 'Ø'}, 'p': {'π', '∏'},
	'[': {'“', '”'}, ']': {'‘', '’'}, '\\': {'«', '»'},
	'a': {'å', 'Å'}, 's': {'ß', 'Í'}, 'd': {'∂', 'Î'}, 'f': {'ƒ', 'Ï'}, 'g': {'©', '˝'},
	'h': {'˙', 'Ó'}, 'j': {'∆', 'Ô'}, 'k': {'˚', '\uf8ff'}, 'l': {'¬', 'Ò'}, ';': {'…', 'Ú'}, '\'': {'æ', 'Æ'},
	'z': {'Ω', '¸'}, 'x': {'≈', '˛'}, 'c': {'ç', 'Ç'}, 'v': {'√', '◊'}, 'b': {'∫', 'ı'},
	'n': {0, '˜'}, 'm': {'µ', 'Â'}, ',': {'≤', '¯'}, '.': {'≥', '˘'}, '/': {'÷', '¿'},
	' ': {'\u00a0', '\u00a0'},
}

// macWebKey returns the key code, characters, characters ignoring
// modifiers other than Shift (what key equivalents match), and flags of
// the AppKit events for key.
func macWebKey(key input.Key, mods input.Modifiers) (uint16, string, string, uint, bool) {
	code, ok := macWebKeyCodes[key]
	flags := macWebFlags(mods)
	var r rune
	const (
		numericPad = 1 << 21
		function   = 1 << 23 // NSEventModifierFlagFunction
	)
	switch {
	case key >= input.KeyLeft && key <= input.KeyDown:
		flags |= numericPad | function
		r = [...]rune{0xf702, 0xf703, 0xf700, 0xf701}[key-input.KeyLeft]
	case key >= input.KeyF1 && key <= input.KeyF12:
		flags |= function
		r = 0xf704 + rune(key-input.KeyF1)
	case key >= input.KeyNumpad0 && key <= input.KeyNumpadEnter:
		flags |= numericPad
		r = key.Rune()
		if key == input.KeyNumpadEnter {
			r = 0x03
		}
	default:
		switch key {
		case input.KeyEnter:
			r = '\r'
		case input.KeyTab:
			r = '\t'
			if mods&input.ModShift != 0 {
				r = 0x19 // NSBackTabCharacter
			}
		case input.KeyEscape:
			r = 0x1b
		case input.KeyBackspace:
			r = 0x7f // NSDeleteCharacter (backward delete)
		case input.KeyDelete:
			r = 0xf728
		case input.KeyInsert:
			r = 0xf727
		case input.KeyHome:
			r = 0xf729
		case input.KeyEnd:
			r = 0xf72b
		case input.KeyPageUp:
			r = 0xf72c
		case input.KeyPageDown:
			r = 0xf72d
		default:
			r = platform.WebKeyRune(key, mods)
		}
		if r >= 0xf727 && r <= 0xf72d {
			flags |= function
		}
	}
	if r == 0 {
		return code, "", "", flags, ok
	}
	ignoring, chars := string(r), string(r)
	if mods&input.ModAlt != 0 {
		if option, found := macOptionChars[key.Rune()]; found {
			chars = ""
			if r := option[0]; mods&input.ModShift != 0 {
				chars = string(option[1])
			} else if r != 0 {
				chars = string(r)
			}
		}
	}
	return code, chars, ignoring, flags, ok
}
