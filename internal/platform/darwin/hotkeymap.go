package darwin

// Pure logic for global hotkeys (no build tag, so it is tested on every
// host): accelerator -> Carbon virtual key code + modifier mask.
//
// Key codes are the kVK_* constants of HIToolbox/Events.h. They name
// physical key positions; the ANSI block is encoded below as one string
// whose byte offset is the key code.

import (
	"fmt"
	"strings"
)

// Modifier bits of RegisterEventHotKey (Events.h: cmdKey, shiftKey,
// optionKey, controlKey).
const (
	hkCmd     uint32 = 0x0100
	hkShift   uint32 = 0x0200
	hkOption  uint32 = 0x0800
	hkControl uint32 = 0x1000
)

// ansiLayout[i] is the character on the key whose code is i (kVK_ANSI_A
// = 0x00 ... kVK_ANSI_Grave = 0x32); '_' marks codes that are not ANSI
// character keys (ISO section, Return, Tab, Space).
const ansiLayout = "asdfhgzxcv_bqweryt123465=97-80]ou[ip_lj'k;\\,/nm.__`"

// Function and editing keys (Events.h).
var specialKeyCodes = map[string]uint32{
	"enter": 0x24, "return": 0x24, "tab": 0x30, "space": 0x31,
	"backspace": 0x33, "escape": 0x35, "esc": 0x35,
	"delete": 0x75, "del": 0x75, "insert": 0x72, "ins": 0x72, // kVK_ForwardDelete, kVK_Help
	"home": 0x73, "end": 0x77, "pageup": 0x74, "pagedown": 0x79,
	"left": 0x7B, "right": 0x7C, "down": 0x7D, "up": 0x7E,
}

// functionKeyCodes[n-1] is kVK_Fn.
var functionKeyCodes = [...]uint32{
	0x7A, 0x78, 0x63, 0x76, 0x60, 0x61, 0x62, 0x64, 0x65, 0x6D,
	0x67, 0x6F, 0x69, 0x6B, 0x71, 0x6A, 0x40, 0x4F, 0x50, 0x5A,
}

// keypadDigitCodes[d] is kVK_ANSI_Keypad<d>.
var keypadDigitCodes = [...]uint32{0x52, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5B, 0x5C}

func carbonKeyCode(key string) (uint32, bool) {
	if len(key) == 1 && key[0] != '_' {
		if i := strings.IndexByte(ansiLayout, key[0]); i >= 0 {
			return uint32(i), true
		}
	}
	if c, ok := specialKeyCodes[key]; ok {
		return c, true
	}
	var n int
	if _, err := fmt.Sscanf(key, "f%d", &n); err == nil && fmt.Sprintf("f%d", n) == key && n >= 1 && n <= len(functionKeyCodes) {
		return functionKeyCodes[n-1], true
	}
	if len(key) == 4 && strings.HasPrefix(key, "num") && key[3] >= '0' && key[3] <= '9' {
		return keypadDigitCodes[key[3]-'0'], true
	}
	return 0, false
}

// carbonHotkey converts "CmdOrCtrl+Shift+K" to a key code and modifiers.
// "Plus" (and a trailing "++") is Shift+= on the ANSI layout.
func carbonHotkey(acc string) (code, mods uint32, err error) {
	s := strings.TrimSpace(acc)
	var tokens []string
	if strings.HasSuffix(s, "++") {
		tokens = append(strings.Split(strings.TrimSuffix(s, "++"), "+"), "plus")
	} else {
		tokens = strings.Split(s, "+")
	}
	last := len(tokens) - 1
	for _, t := range tokens[:last] {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "cmdorctrl", "commandorcontrol", "cmd", "command", "super", "meta":
			mods |= hkCmd
		case "ctrl", "control":
			mods |= hkControl
		case "alt", "option", "opt":
			mods |= hkOption
		case "shift":
			mods |= hkShift
		default:
			return 0, 0, fmt.Errorf("tango/darwin: hotkey %q: unknown modifier %q", acc, t)
		}
	}
	key := strings.ToLower(strings.TrimSpace(tokens[last]))
	switch key {
	case "plus", "+":
		key, mods = "=", mods|hkShift
	case "minus":
		key = "-"
	case "equal":
		key = "="
	}
	c, ok := carbonKeyCode(key)
	if !ok {
		return 0, 0, fmt.Errorf("tango/darwin: hotkey %q: %q cannot be a global shortcut key", acc, key)
	}
	return c, mods, nil
}
