package uihost

import (
	"runtime"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestParseShortcut(t *testing.T) {
	cmd := input.ModCtrl
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		cmd = input.ModMeta
	}
	cases := []struct {
		in   string
		key  input.Key
		mods input.Modifiers
	}{
		{"CmdOrCtrl+N", input.KeyN, cmd},
		{"Shift+CmdOrCtrl+Z", input.KeyZ, cmd | input.ModShift},
		{"Alt+F4", input.KeyF4, input.ModAlt},
		{"F5", input.KeyF5, 0},
		{"Escape", input.KeyEscape, 0},
		{"Ctrl+Up", input.KeyUp, input.ModCtrl},
		{"Cmd+7", input.Key7, input.ModMeta},
		{"Ctrl+num5", input.KeyNumpad5, input.ModCtrl},
		{"Ctrl+[", input.KeyBracketLeft, input.ModCtrl},
		{"Space", input.KeySpace, 0},
	}
	for _, c := range cases {
		b, err := parseShortcut(c.in)
		if err != nil || b.key != c.key || b.mods != c.mods {
			t.Errorf("parseShortcut(%q) = %+v, %v; want key %v mods %v", c.in, b, err, c.key, c.mods)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Hyper+K", "F13", "K+Ctrl"} {
		if _, err := parseShortcut(bad); err == nil {
			t.Errorf("parseShortcut(%q) accepted", bad)
		}
	}
}
