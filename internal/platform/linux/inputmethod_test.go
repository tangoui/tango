package linux

import "testing"

func TestIBusKey(t *testing.T) {
	for _, tc := range []struct {
		keyval, hardware, state uint32
		char                    rune
		code, text              string
		shift, ctrl, meta       bool
	}{
		{'A', 30, 1, 'A', "KeyA", "A", true, false, false},
		{'!', 2, 1, '!', "Digit1", "!", true, false, false},
		{0xff51, 105, 0, 0, "ArrowLeft", "", false, false, false},
		{0xff08, 14, 4 | 1<<26, '\b', "Backspace", "", false, true, true},
		{'c', 46, 4, 'c', "KeyC", "", false, true, false},
		{0xffe3, 29, 0, 0, "ControlLeft", "", false, false, false},
		{0xffe4, 97, 0, 0, "ControlRight", "", false, false, false},
	} {
		got := ibusKey(tc.keyval, tc.hardware, tc.state, tc.char)
		if got.Code != tc.code || got.Text != tc.text || got.Shift != tc.shift || got.Ctrl != tc.ctrl || got.Meta != tc.meta {
			t.Errorf("key %#x = %+v, want %+v", tc.keyval, got, tc)
		}
	}
}
