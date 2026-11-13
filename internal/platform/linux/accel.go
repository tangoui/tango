package linux

import (
	"fmt"
	"strings"
)

// gtkKeyNames maps Tango/Electron-style key names to GDK keyval names (the
// part of a GTK accelerator after the modifiers, see gdk/gdkkeysyms.h).
var gtkKeyNames = map[string]string{
	"plus": "plus", "space": "space", "tab": "Tab", "enter": "Return", "return": "Return",
	"esc": "Escape", "escape": "Escape", "backspace": "BackSpace", "delete": "Delete",
	"del": "Delete", "insert": "Insert", "ins": "Insert", "home": "Home", "end": "End",
	"pageup": "Page_Up", "pagedown": "Page_Down", "up": "Up", "down": "Down",
	"left": "Left", "right": "Right", "printscreen": "Print",
	"capslock": "Caps_Lock", "numlock": "Num_Lock", "scrolllock": "Scroll_Lock",
	"volumeup": "XF86AudioRaiseVolume", "volumedown": "XF86AudioLowerVolume",
	"volumemute": "XF86AudioMute", "medianexttrack": "XF86AudioNext",
	"mediaprevioustrack": "XF86AudioPrev", "mediastop": "XF86AudioStop",
	"mediaplaypause": "XF86AudioPlay",
	"numadd":         "KP_Add", "numsub": "KP_Subtract", "nummult": "KP_Multiply",
	"numdiv": "KP_Divide", "numdec": "KP_Decimal",
	"+": "plus", "-": "minus", "=": "equal", ",": "comma", ".": "period", "/": "slash",
	"\\": "backslash", ";": "semicolon", "'": "apostrophe", "`": "grave",
	"[": "bracketleft", "]": "bracketright",
}

// gtkModifiers maps modifier names to GTK accelerator modifier tokens.
// Linux has no Command key: CmdOrCtrl means Control, Cmd/Command/Super/Meta
// mean the Super (Windows/logo) key, as in Electron on Linux.
var gtkModifiers = map[string]string{
	"cmdorctrl": "<Control>", "commandorcontrol": "<Control>",
	"ctrl": "<Control>", "control": "<Control>",
	"cmd": "<Super>", "command": "<Super>", "super": "<Super>", "meta": "<Super>",
	"alt": "<Alt>", "option": "<Alt>", "altgr": "<Mod5>",
	"shift": "<Shift>",
}

// gtkModifierOrder is the order modifiers appear in the output, so equal
// accelerators produce equal strings.
var gtkModifierOrder = []string{"<Control>", "<Shift>", "<Alt>", "<Super>", "<Mod5>"}

// gtkAccelerator converts an accelerator such as "CmdOrCtrl+Shift+N" to the
// format gtk_accelerator_parse reads ("<Control><Shift>n"). Modifier and key
// names are case-insensitive. "" converts to "".
func gtkAccelerator(acc string) (string, error) {
	acc = strings.TrimSpace(acc)
	if acc == "" {
		return "", nil
	}
	parts := strings.Split(acc, "+")
	// "Ctrl++" splits into [..., "", ""]: the key is "+".
	if n := len(parts); n >= 2 && parts[n-1] == "" && parts[n-2] == "" {
		parts = append(parts[:n-2], "+")
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if key == "" {
		return "", fmt.Errorf("tango/linux: accelerator %q has no key", acc)
	}
	mods := map[string]bool{}
	for _, m := range parts[:len(parts)-1] {
		tok, ok := gtkModifiers[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return "", fmt.Errorf("tango/linux: accelerator %q: unknown modifier %q", acc, m)
		}
		mods[tok] = true
	}
	name, err := gtkKeyName(key)
	if err != nil {
		return "", fmt.Errorf("tango/linux: accelerator %q: %w", acc, err)
	}
	var b strings.Builder
	for _, tok := range gtkModifierOrder {
		if mods[tok] {
			b.WriteString(tok)
		}
	}
	b.WriteString(name)
	return b.String(), nil
}

func gtkKeyName(key string) (string, error) {
	if n, ok := gtkKeyNames[strings.ToLower(key)]; ok {
		return n, nil
	}
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			return key, nil
		case c >= 'A' && c <= 'Z':
			return strings.ToLower(key), nil // GTK lowercases letter keyvals
		}
	}
	lk := strings.ToLower(key)
	if len(lk) >= 2 && lk[0] == 'f' {
		n := 0
		for _, c := range lk[1:] {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= 35 {
			return fmt.Sprintf("F%d", n), nil
		}
	}
	if len(lk) == 4 && strings.HasPrefix(lk, "num") && lk[3] >= '0' && lk[3] <= '9' {
		return "KP_" + lk[3:], nil
	}
	return "", fmt.Errorf("unknown key %q", key)
}
