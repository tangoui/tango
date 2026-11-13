package hotkey

import (
	"fmt"
	"strings"
)

// accel is a parsed accelerator, resolved for one platform (CmdOrCtrl is
// Meta on macOS, Ctrl elsewhere).
type accel struct {
	Ctrl, Alt, Shift, Meta bool
	// Code is the KeyboardEvent.code of the key ("KeyK", "Digit1", "F5",
	// "ArrowUp"); Key the lower-cased KeyboardEvent.key that also matches
	// ("k", "+"), "" if only Code does.
	Code, Key string
}

var namedCodes = map[string][2]string{ // name -> {code, key}
	"enter": {"Enter", "enter"}, "return": {"Enter", "enter"},
	"tab": {"Tab", "tab"}, "space": {"Space", " "},
	"backspace": {"Backspace", "backspace"},
	"delete":    {"Delete", "delete"}, "del": {"Delete", "delete"},
	"insert": {"Insert", "insert"}, "ins": {"Insert", "insert"},
	"escape": {"Escape", "escape"}, "esc": {"Escape", "escape"},
	"up": {"ArrowUp", "arrowup"}, "down": {"ArrowDown", "arrowdown"},
	"left": {"ArrowLeft", "arrowleft"}, "right": {"ArrowRight", "arrowright"},
	"home": {"Home", "home"}, "end": {"End", "end"},
	"pageup": {"PageUp", "pageup"}, "pagedown": {"PageDown", "pagedown"},
	"plus": {"Equal", "+"}, "+": {"Equal", "+"},
	"=": {"Equal", "="}, "-": {"Minus", "-"}, "minus": {"Minus", "-"},
	",": {"Comma", ","}, ".": {"Period", "."}, "/": {"Slash", "/"},
	";": {"Semicolon", ";"}, "'": {"Quote", "'"}, "`": {"Backquote", "`"},
	"[": {"BracketLeft", "["}, "]": {"BracketRight", "]"}, "\\": {"Backslash", "\\"},
}

// parseAccel parses an Electron-style accelerator ("CmdOrCtrl+Shift+K")
// for goos. Modifier and key names are case-insensitive.
func parseAccel(s, goos string) (accel, error) {
	var a accel
	s = strings.TrimSpace(s)
	if s == "" {
		return a, fmt.Errorf("hotkey: empty accelerator")
	}
	parts := strings.Split(s, "+")
	if n := len(parts); n >= 2 && strings.TrimSpace(parts[n-1]) == "" && strings.TrimSpace(parts[n-2]) == "" {
		parts = append(parts[:n-2], "+") // "Ctrl++"
	}
	mac := goos == "darwin" || goos == "ios"
	for _, m := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "cmdorctrl", "commandorcontrol":
			if mac {
				a.Meta = true
			} else {
				a.Ctrl = true
			}
		case "cmd", "command", "super", "meta", "win":
			a.Meta = true
		case "ctrl", "control":
			a.Ctrl = true
		case "alt", "option", "opt":
			a.Alt = true
		case "shift":
			a.Shift = true
		default:
			return accel{}, fmt.Errorf("hotkey: accelerator %q: unknown modifier %q", s, m)
		}
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	lk := strings.ToLower(key)
	switch {
	case key == "":
		return accel{}, fmt.Errorf("hotkey: accelerator %q: missing key", s)
	case len(lk) == 1 && lk[0] >= 'a' && lk[0] <= 'z':
		a.Code, a.Key = "Key"+strings.ToUpper(lk), lk
	case len(lk) == 1 && lk[0] >= '0' && lk[0] <= '9':
		a.Code, a.Key = "Digit"+lk, lk
	case len(lk) == 4 && strings.HasPrefix(lk, "num") && lk[3] >= '0' && lk[3] <= '9':
		a.Code = "Numpad" + lk[3:]
	case len(lk) >= 2 && lk[0] == 'f' && isNumber(lk[1:]):
		n := 0
		fmt.Sscanf(lk[1:], "%d", &n)
		if n < 1 || n > 24 {
			return accel{}, fmt.Errorf("hotkey: accelerator %q: no such function key", s)
		}
		a.Code, a.Key = fmt.Sprintf("F%d", n), lk
	default:
		nc, ok := namedCodes[lk]
		if !ok {
			return accel{}, fmt.Errorf("hotkey: accelerator %q: unknown key %q", s, key)
		}
		a.Code, a.Key = nc[0], nc[1]
	}
	return a, nil
}

func isNumber(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
