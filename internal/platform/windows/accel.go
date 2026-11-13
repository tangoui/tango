package windows

import (
	"fmt"
	"strconv"
	"strings"
)

// modifiers is a set of accelerator modifier keys.
type modifiers uint8

const (
	modCtrl modifiers = 1 << iota
	modShift
	modAlt
)

// ACCEL.fVirt flags (winuser.h).
const (
	fVirtKey = 0x01
	fShift   = 0x04
	fControl = 0x08
	fAlt     = 0x10
)

// accelerator is a parsed key combination.
type accelerator struct {
	Mods modifiers
	VK   uint16 // Win32 virtual-key code
	Name string // key as displayed in menus, e.g. "N", "F5", "Del"
}

// fVirt returns the ACCEL.fVirt flags of a.
func (a accelerator) fVirt() uint8 {
	f := uint8(fVirtKey)
	if a.Mods&modCtrl != 0 {
		f |= fControl
	}
	if a.Mods&modShift != 0 {
		f |= fShift
	}
	if a.Mods&modAlt != 0 {
		f |= fAlt
	}
	return f
}

// String is the Windows menu text of a, e.g. "Ctrl+Shift+N".
func (a accelerator) String() string {
	var b strings.Builder
	if a.Mods&modCtrl != 0 {
		b.WriteString("Ctrl+")
	}
	if a.Mods&modAlt != 0 {
		b.WriteString("Alt+")
	}
	if a.Mods&modShift != 0 {
		b.WriteString("Shift+")
	}
	b.WriteString(a.Name)
	return b.String()
}

type keyInfo struct {
	vk   uint16
	name string
}

// namedKeys maps lower-cased key names to virtual keys (US layout for the
// OEM keys).
var namedKeys = map[string]keyInfo{
	"plus":      {0xBB, "+"},
	"+":         {0xBB, "+"},
	"=":         {0xBB, "="},
	"equal":     {0xBB, "="},
	"-":         {0xBD, "-"},
	"minus":     {0xBD, "-"},
	",":         {0xBC, ","},
	"comma":     {0xBC, ","},
	".":         {0xBE, "."},
	"period":    {0xBE, "."},
	"/":         {0xBF, "/"},
	"slash":     {0xBF, "/"},
	"`":         {0xC0, "`"},
	";":         {0xBA, ";"},
	"'":         {0xDE, "'"},
	"[":         {0xDB, "["},
	"]":         {0xDD, "]"},
	"\\":        {0xDC, "\\"},
	"space":     {0x20, "Space"},
	"tab":       {0x09, "Tab"},
	"backspace": {0x08, "Backspace"},
	"delete":    {0x2E, "Del"},
	"del":       {0x2E, "Del"},
	"insert":    {0x2D, "Ins"},
	"ins":       {0x2D, "Ins"},
	"return":    {0x0D, "Enter"},
	"enter":     {0x0D, "Enter"},
	"escape":    {0x1B, "Esc"},
	"esc":       {0x1B, "Esc"},
	"up":        {0x26, "Up"},
	"down":      {0x28, "Down"},
	"left":      {0x25, "Left"},
	"right":     {0x27, "Right"},
	"home":      {0x24, "Home"},
	"end":       {0x23, "End"},
	"pageup":    {0x21, "PgUp"},
	"pagedown":  {0x22, "PgDn"},
}

// parseAccelerator parses an Electron-style accelerator such as
// "CmdOrCtrl+Shift+N". Cmd/Command/CmdOrCtrl map to Ctrl on Windows.
// The Windows key (Super/Meta) cannot be used in accelerator tables.
func parseAccelerator(s string) (accelerator, error) {
	var a accelerator
	s = strings.TrimSpace(s)
	if s == "" {
		return a, fmt.Errorf("empty accelerator")
	}
	parts := strings.Split(s, "+")
	// "Ctrl++" splits into [..., "", ""]: the key is "+".
	if strings.HasSuffix(s, "++") || s == "+" {
		parts = append(parts[:len(parts)-2], "+")
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	for _, m := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "cmdorctrl", "commandorcontrol", "cmd", "command", "ctrl", "control":
			a.Mods |= modCtrl
		case "shift":
			a.Mods |= modShift
		case "alt", "option", "altgr":
			a.Mods |= modAlt
		case "super", "meta", "win", "windows":
			return accelerator{}, fmt.Errorf("accelerator %q: the Windows key is not supported", s)
		default:
			return accelerator{}, fmt.Errorf("accelerator %q: unknown modifier %q", s, m)
		}
	}
	if key == "" {
		return accelerator{}, fmt.Errorf("accelerator %q: missing key", s)
	}
	lower := strings.ToLower(key)
	switch {
	case len(key) == 1 && (key[0] >= 'a' && key[0] <= 'z' || key[0] >= 'A' && key[0] <= 'Z'):
		up := strings.ToUpper(key)
		a.VK, a.Name = uint16(up[0]), up
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		a.VK, a.Name = uint16(key[0]), key
	case len(lower) >= 2 && lower[0] == 'f' && isDigits(lower[1:]):
		n, _ := strconv.Atoi(lower[1:])
		if n < 1 || n > 24 {
			return accelerator{}, fmt.Errorf("accelerator %q: no such function key", s)
		}
		a.VK, a.Name = uint16(0x70+n-1), "F"+strconv.Itoa(n)
	case strings.HasPrefix(lower, "num") && len(lower) == 4 && lower[3] >= '0' && lower[3] <= '9':
		a.VK, a.Name = uint16(0x60+lower[3]-'0'), "Num "+lower[3:]
	default:
		k, ok := namedKeys[lower]
		if !ok {
			return accelerator{}, fmt.Errorf("accelerator %q: unknown key %q", s, key)
		}
		a.VK, a.Name = k.vk, k.name
	}
	return a, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// accelEntry binds an accelerator to a menu command.
type accelEntry struct {
	Accel accelerator
	Cmd   uint16
}

// matchAccel finds the command bound to vk with exactly mods held. Used for
// keys the WebView reports through AcceleratorKeyPressed, which never pass
// through the host's TranslateAccelerator.
func matchAccel(table []accelEntry, vk uint16, mods modifiers) (uint16, bool) {
	for _, e := range table {
		if e.Accel.VK == vk && e.Accel.Mods == mods {
			return e.Cmd, true
		}
	}
	return 0, false
}
