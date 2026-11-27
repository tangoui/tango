package darwin

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func imeKey(code uint16, chars, charsIgnoringMods string, flags uint) platform.IMEKey {
	key := keyFromMac(code, charsIgnoringMods)
	out := platform.IMEKey{
		Shift: flags&nsShiftKeyMask != 0,
		Ctrl:  flags&nsControlKeyMask != 0,
		Alt:   flags&nsAlternateKeyMask != 0,
		Meta:  flags&nsCommandKeyMask != 0,
	}
	name := key.String()
	switch {
	case key == input.KeyUnknown:
	case len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z':
		out.Code = "Key" + name
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		out.Code = "Digit" + name
	case key == input.KeyUp || key == input.KeyDown || key == input.KeyLeft || key == input.KeyRight:
		out.Code = "Arrow" + name
	default:
		out.Code = name
	}
	if r, n := utf8.DecodeRuneInString(chars); n == len(chars) && n > 0 &&
		(r != utf8.RuneError || n > 1) && unicode.IsPrint(r) && (r < 0xf700 || r > 0xf8ff) {
		out.Text = chars
	}
	return out
}

// utf16Offset translates a rune index to an NSRange offset.
func utf16Offset(s string, runes int) int {
	offset := 0
	for _, r := range s {
		if runes <= 0 {
			break
		}
		offset += utf16.RuneLen(r)
		runes--
	}
	return offset
}
