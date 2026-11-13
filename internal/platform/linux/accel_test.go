package linux

import "testing"

func TestGTKAccelerator(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"CmdOrCtrl+Shift+N":    "<Control><Shift>n",
		"Shift+CmdOrCtrl+n":    "<Control><Shift>n",
		"CommandOrControl+Q":   "<Control>q",
		"Ctrl+Alt+Delete":      "<Control><Alt>Delete",
		"Cmd+K":                "<Super>k",
		"Super+Space":          "<Super>space",
		"Option+F12":           "<Alt>F12",
		"F1":                   "F1",
		"Ctrl++":               "<Control>plus",
		"Ctrl+Plus":            "<Control>plus",
		"Ctrl+-":               "<Control>minus",
		"Ctrl+=":               "<Control>equal",
		"Ctrl+,":               "<Control>comma",
		"Ctrl+/":               "<Control>slash",
		"Ctrl+[":               "<Control>bracketleft",
		"Alt+Enter":            "<Alt>Return",
		"Esc":                  "Escape",
		"Ctrl+PageDown":        "<Control>Page_Down",
		"Ctrl+Up":              "<Control>Up",
		"Ctrl+1":               "<Control>1",
		"Ctrl+num5":            "<Control>KP_5",
		"ctrl+shift+backspace": "<Control><Shift>BackSpace",
		" Ctrl + S ":           "<Control>s",
	}
	for in, want := range cases {
		got, err := gtkAccelerator(in)
		if err != nil || got != want {
			t.Errorf("gtkAccelerator(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"Ctrl+", "Hyper+A", "Ctrl+Foo", "F0", "F99", "Ctrl+é"} {
		if got, err := gtkAccelerator(bad); err == nil {
			t.Errorf("gtkAccelerator(%q) = %q, want error", bad, got)
		}
	}
}
