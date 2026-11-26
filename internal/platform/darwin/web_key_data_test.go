package darwin

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestMacWebKey(t *testing.T) {
	for _, tc := range []struct {
		key             input.Key
		mods            input.Modifiers
		code            uint16
		chars, ignoring string
		flags           uint
	}{
		{input.KeyS, input.ModMeta, 1, "s", "s", nsCommandKeyMask},
		{input.KeyEqual, input.ModShift, 0x18, "+", "+", nsShiftKeyMask},
		{input.KeyA, input.ModCtrl, 0, "a", "a", nsControlKeyMask},
		{input.KeyLeft, 0, 0x7b, "\uf702", "\uf702", 1<<21 | 1<<23},
		{input.KeyF5, 0, 0x60, "\uf708", "\uf708", 1 << 23},
		{input.KeyHome, 0, 0x73, "\uf729", "\uf729", 1 << 23},
		{input.KeyNumpadEnter, 0, 0x4c, "\x03", "\x03", 1 << 21},
		// Option changes the characters, not what key equivalents match.
		{input.KeyA, input.ModAlt, 0, "å", "a", nsAlternateKeyMask},
		{input.KeyA, input.ModAlt | input.ModShift, 0, "Å", "A", nsAlternateKeyMask | nsShiftKeyMask},
		{input.Key2, input.ModAlt, 0x13, "™", "2", nsAlternateKeyMask},
		{input.KeyS, input.ModAlt | input.ModMeta, 1, "ß", "s", nsAlternateKeyMask | nsCommandKeyMask},
		{input.KeyE, input.ModAlt, 14, "", "e", nsAlternateKeyMask}, // dead key
		{input.KeyE, input.ModAlt | input.ModShift, 14, "´", "E", nsAlternateKeyMask | nsShiftKeyMask},
		{input.KeyEnter, input.ModAlt, 0x24, "\r", "\r", nsAlternateKeyMask},
	} {
		code, chars, ignoring, flags, ok := macWebKey(tc.key, tc.mods)
		if !ok || code != tc.code || chars != tc.chars || ignoring != tc.ignoring || flags != tc.flags {
			t.Errorf("%v %v: got %#x %q %q %#x %v, want %#x %q %q %#x", tc.key, tc.mods, code, chars, ignoring, flags, ok, tc.code, tc.chars, tc.ignoring, tc.flags)
		}
	}
	if _, _, _, _, ok := macWebKey(input.KeyPaste, 0); ok {
		t.Error("synthetic clipboard key unexpectedly mapped")
	}
}
