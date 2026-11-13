package darwin

import "testing"

func TestCarbonHotkey(t *testing.T) {
	cases := []struct {
		in         string
		code, mods uint32
	}{
		{"CmdOrCtrl+Shift+K", 0x28, hkCmd | hkShift}, // kVK_ANSI_K
		{"Ctrl+Alt+Space", 0x31, hkControl | hkOption},
		{"A", 0x00, 0},
		{"Cmd+B", 0x0B, hkCmd},
		{"Cmd+M", 0x2E, hkCmd},
		{"Cmd+`", 0x32, hkCmd},
		{"Cmd+5", 0x17, hkCmd},
		{"Cmd+6", 0x16, hkCmd},
		{"F5", 0x60, 0},
		{"F20", 0x5A, 0},
		{"Cmd++", 0x18, hkCmd | hkShift},
		{"cmd+plus", 0x18, hkCmd | hkShift},
		{"Option+Up", 0x7E, hkOption},
		{" Command + 0 ", 0x1D, hkCmd},
		{"Ctrl+num8", 0x5B, hkControl},
	}
	for _, c := range cases {
		code, mods, err := carbonHotkey(c.in)
		if err != nil || code != c.code || mods != c.mods {
			t.Errorf("%q: %#x %#x %v", c.in, code, mods, err)
		}
	}
	for _, bad := range []string{"", "Hyper+K", "Ctrl+Nope", "Ctrl+_", "F21", "f05"} {
		if _, _, err := carbonHotkey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if len(ansiLayout) != 0x33 {
		t.Fatalf("layout length %#x", len(ansiLayout))
	}
}
