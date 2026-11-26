package darwin

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestMacWebKey(t *testing.T) {
	for _, tc := range []struct {
		key   input.Key
		mods  input.Modifiers
		code  uint16
		chars string
		flags uint
	}{
		{input.KeyS, input.ModMeta, 1, "s", nsCommandKeyMask},
		{input.KeyEqual, input.ModShift, 0x18, "+", nsShiftKeyMask},
		{input.KeyA, input.ModCtrl, 0, "a", nsControlKeyMask},
		{input.KeyLeft, 0, 0x7b, "\uf702", 1 << 21},
		{input.KeyNumpadEnter, 0, 0x4c, "\x03", 1 << 21},
	} {
		code, chars, flags, ok := macWebKey(tc.key, tc.mods)
		if !ok || code != tc.code || chars != tc.chars || flags != tc.flags {
			t.Errorf("%v: got %#x %q %#x %v, want %#x %q %#x", tc.key, code, chars, flags, ok, tc.code, tc.chars, tc.flags)
		}
	}
	if _, _, _, ok := macWebKey(input.KeyPaste, 0); ok {
		t.Error("synthetic clipboard key unexpectedly mapped")
	}
}
