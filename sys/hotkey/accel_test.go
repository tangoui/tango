package hotkey

import "testing"

func TestParseAccel(t *testing.T) {
	cases := []struct {
		in, goos string
		want     accel
	}{
		{"CmdOrCtrl+Shift+K", "darwin", accel{Meta: true, Shift: true, Code: "KeyK", Key: "k"}},
		{"CmdOrCtrl+Shift+K", "windows", accel{Ctrl: true, Shift: true, Code: "KeyK", Key: "k"}},
		{"ctrl+alt+delete", "linux", accel{Ctrl: true, Alt: true, Code: "Delete", Key: "delete"}},
		{"Super+Space", "linux", accel{Meta: true, Code: "Space", Key: " "}},
		{"Option+F12", "darwin", accel{Alt: true, Code: "F12", Key: "f12"}},
		{"Ctrl++", "windows", accel{Ctrl: true, Code: "Equal", Key: "+"}},
		{"Ctrl+Plus", "windows", accel{Ctrl: true, Code: "Equal", Key: "+"}},
		{" Ctrl + 1 ", "windows", accel{Ctrl: true, Code: "Digit1", Key: "1"}},
		{"Esc", "darwin", accel{Code: "Escape", Key: "escape"}},
		{"Alt+Up", "linux", accel{Alt: true, Code: "ArrowUp", Key: "arrowup"}},
		{"Ctrl+num5", "linux", accel{Ctrl: true, Code: "Numpad5"}},
		{"Shift+[", "linux", accel{Shift: true, Code: "BracketLeft", Key: "["}},
	}
	for _, c := range cases {
		got, err := parseAccel(c.in, c.goos)
		if err != nil || got != c.want {
			t.Errorf("parseAccel(%q, %s) = %+v, %v; want %+v", c.in, c.goos, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Hyper+K", "Ctrl+F99", "Ctrl+Foo", "K+Ctrl"} {
		if _, err := parseAccel(bad, "linux"); err == nil {
			t.Errorf("parseAccel(%q) accepted", bad)
		}
	}
}
