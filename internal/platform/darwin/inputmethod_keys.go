package darwin

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

const (
	nsEventTypeKeyDown      = 10
	nsEventTypeFlagsChanged = 12
	imeRecognizedEventsMask = 1<<nsEventTypeKeyDown | 1<<nsEventTypeFlagsChanged
	nsCapsLockKeyMask       = 1 << 16
)

func imeModifierFromFlags(code uint16, flags uint) (platform.IMEModifierKey, bool) {
	caps := flags&nsCapsLockKeyMask != 0
	if code == macCapsLockCode {
		return platform.IMEModifierKey{Code: "CapsLock", Down: caps, CapsLock: caps}, true
	}
	key, down, _, ok := modifierKeyFromMac(code, flags)
	if !ok {
		return platform.IMEModifierKey{}, false
	}
	name := key.String()
	switch key {
	case input.KeyCtrlLeft:
		name = "ControlLeft"
	case input.KeyCtrlRight:
		name = "ControlRight"
	}
	return platform.IMEModifierKey{Code: name, Down: down, CapsLock: caps}, true
}

func imeKey(code uint16, chars, charsIgnoringMods string, flags uint, repeat bool) platform.IMEKey {
	key := keyFromMac(code, charsIgnoringMods)
	out := platform.IMEKey{
		Shift:    flags&nsShiftKeyMask != 0,
		Ctrl:     flags&nsControlKeyMask != 0,
		Alt:      flags&nsAlternateKeyMask != 0,
		Meta:     flags&nsCommandKeyMask != 0,
		CapsLock: flags&nsCapsLockKeyMask != 0,
		Repeat:   repeat,
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

func imeModeSourceID(bundleID, mode string) string { return bundleID + "." + mode }

func imeModeID(bundleID, sourceID string) string {
	if mode, ok := strings.CutPrefix(sourceID, bundleID+"."); ok {
		return mode
	}
	return ""
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
