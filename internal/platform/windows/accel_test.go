package windows

import "testing"

func TestParseAccelerator(t *testing.T) {
	cases := []struct {
		in    string
		vk    uint16
		mods  modifiers
		text  string
		fVirt uint8
	}{
		{"CmdOrCtrl+Shift+N", 'N', modCtrl | modShift, "Ctrl+Shift+N", fVirtKey | fControl | fShift},
		{"Ctrl+n", 'N', modCtrl, "Ctrl+N", fVirtKey | fControl},
		{"Command+Q", 'Q', modCtrl, "Ctrl+Q", fVirtKey | fControl},
		{"Alt+F4", 0x73, modAlt, "Alt+F4", fVirtKey | fAlt},
		{"Option+Shift+Ctrl+1", '1', modCtrl | modShift | modAlt, "Ctrl+Alt+Shift+1", fVirtKey | fControl | fShift | fAlt},
		{"F5", 0x74, 0, "F5", fVirtKey},
		{"F24", 0x87, 0, "F24", fVirtKey},
		{"CmdOrCtrl+Plus", 0xBB, modCtrl, "Ctrl++", fVirtKey | fControl},
		{"Ctrl++", 0xBB, modCtrl, "Ctrl++", fVirtKey | fControl},
		{"Ctrl+=", 0xBB, modCtrl, "Ctrl+=", fVirtKey | fControl},
		{"Ctrl+-", 0xBD, modCtrl, "Ctrl+-", fVirtKey | fControl},
		{"Ctrl+,", 0xBC, modCtrl, "Ctrl+,", fVirtKey | fControl},
		{"Shift+Delete", 0x2E, modShift, "Shift+Del", fVirtKey | fShift},
		{"Ctrl+Enter", 0x0D, modCtrl, "Ctrl+Enter", fVirtKey | fControl},
		{"Esc", 0x1B, 0, "Esc", fVirtKey},
		{"Ctrl+Space", 0x20, modCtrl, "Ctrl+Space", fVirtKey | fControl},
		{"Ctrl+PageDown", 0x22, modCtrl, "Ctrl+PgDn", fVirtKey | fControl},
		{"Alt+Up", 0x26, modAlt, "Alt+Up", fVirtKey | fAlt},
		{"Ctrl+Num5", 0x65, modCtrl, "Ctrl+Num 5", fVirtKey | fControl},
		{" ctrl + shift + z ", 'Z', modCtrl | modShift, "Ctrl+Shift+Z", fVirtKey | fControl | fShift},
	}
	for _, c := range cases {
		a, err := parseAccelerator(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if a.VK != c.vk || a.Mods != c.mods {
			t.Errorf("%q: vk %#x mods %b, want %#x %b", c.in, a.VK, a.Mods, c.vk, c.mods)
		}
		if a.String() != c.text {
			t.Errorf("%q: text %q, want %q", c.in, a.String(), c.text)
		}
		if a.fVirt() != c.fVirt {
			t.Errorf("%q: fVirt %#x, want %#x", c.in, a.fVirt(), c.fVirt)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Super+K", "Meta+K", "Hyper+K", "Ctrl+F25", "Ctrl+F0", "Ctrl+Banana"} {
		if _, err := parseAccelerator(bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}

func TestMatchAccel(t *testing.T) {
	n, _ := parseAccelerator("Ctrl+N")
	sn, _ := parseAccelerator("Ctrl+Shift+N")
	table := []accelEntry{{n, 1}, {sn, 2}}
	if cmd, ok := matchAccel(table, 'N', modCtrl); !ok || cmd != 1 {
		t.Errorf("Ctrl+N = %d %v", cmd, ok)
	}
	if cmd, ok := matchAccel(table, 'N', modCtrl|modShift); !ok || cmd != 2 {
		t.Errorf("Ctrl+Shift+N = %d %v", cmd, ok)
	}
	if _, ok := matchAccel(table, 'N', 0); ok {
		t.Error("bare N must not match")
	}
	if _, ok := matchAccel(table, 'N', modCtrl|modAlt); ok {
		t.Error("extra modifier must not match")
	}
}
