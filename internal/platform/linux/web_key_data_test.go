package linux

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestGTKWebKey(t *testing.T) {
	for _, tc := range []struct {
		key           input.Key
		mods          input.Modifiers
		keyval, state uint32
		text          string
	}{
		{input.KeyS, input.ModCtrl, 's', canvasGdkControlMask, ""},
		{input.KeyK, input.ModShift, 'K', canvasGdkShiftMask, "K"},
		{input.KeyEqual, input.ModShift, '+', canvasGdkShiftMask, "+"},
		{input.KeyLeft, input.ModAlt, 0xff51, canvasGdkMod1Mask, ""},
		{input.KeyTab, input.ModShift, 0xfe20, canvasGdkShiftMask, ""},
		{input.KeyNumpadEnter, 0, 0xff8d, 0, "\r"},
	} {
		k, ok := gtkWebKey(tc.key, tc.mods)
		if !ok || k.keyval != tc.keyval || k.state != tc.state || k.text != tc.text {
			t.Errorf("%v: %+v %v, want keyval %#x state %#x text %q", tc.key, k, ok, tc.keyval, tc.state, tc.text)
		}
	}
}

func TestGTKWebKeycode(t *testing.T) {
	entries := []gdkKeymapEntry{{31, 1, 1}, {45, 0, 1}, {49, 0, 2}}
	code, group := gtkWebKeycode(entries, true)
	if code != 45 || group != 0 {
		t.Fatalf("hardware code=%d group=%d", code, group)
	}
}
