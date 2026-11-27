package windows

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

func tipKeyEvent(key proto.Key) platform.IMEKey {
	result := platform.IMEKey{
		Shift: key.Mods&proto.ModShift != 0, Ctrl: key.Mods&proto.ModCtrl != 0,
		Alt: key.Mods&proto.ModAlt != 0, Meta: key.Mods&proto.ModMeta != 0,
		CapsLock: key.Mods&proto.ModCapsLock != 0, Repeat: key.Flags&proto.KeyRepeat != 0,
	}
	switch vk := key.VK; {
	case vk >= 'A' && vk <= 'Z':
		result.Code = "Key" + string(rune(vk))
	case vk >= '0' && vk <= '9':
		result.Code = "Digit" + string(rune(vk))
	case vk == 0x0d && key.Flags&proto.KeyExtended != 0:
		result.Code = "NumpadEnter"
	default:
		result.Code = tipKeyCodes[vk]
	}
	if r, n := utf8.DecodeRuneInString(key.Text); n == len(key.Text) && unicode.IsPrint(r) {
		result.Text = key.Text
	}
	return result
}

func tipModifierKey(k proto.Key) (platform.IMEModifierKey, bool) {
	var code string
	right := k.Flags&proto.KeyExtended != 0
	switch k.VK {
	case 0x10, 0xa0, 0xa1: // VK_SHIFT, VK_LSHIFT, VK_RSHIFT
		code, right = "Shift", k.VK == 0xa1 || k.Scan == 0x36
	case 0x11, 0xa2, 0xa3:
		code, right = "Control", k.VK == 0xa3 || right
	case 0x12, 0xa4, 0xa5:
		code, right = "Alt", k.VK == 0xa5 || right
	case 0x5b, 0x5c:
		code, right = "Meta", k.VK == 0x5c
	case 0x14:
		code = "CapsLock"
	default:
		return platform.IMEModifierKey{}, false
	}
	if code != "CapsLock" {
		if right {
			code += "Right"
		} else {
			code += "Left"
		}
	}
	down := k.Flags&proto.KeyDown != 0
	key := tipKeyEvent(k)
	// GetKeyboardState can still reflect the state before the event.
	switch code {
	case "ShiftLeft", "ShiftRight":
		key.Shift = key.Shift || down
	case "ControlLeft", "ControlRight":
		key.Ctrl = key.Ctrl || down
	case "AltLeft", "AltRight":
		key.Alt = key.Alt || down
	case "MetaLeft", "MetaRight":
		key.Meta = key.Meta || down
	}
	return platform.IMEModifierKey{
		Code: code, Down: down, CapsLock: key.CapsLock,
		Shift: key.Shift, Ctrl: key.Ctrl, Alt: key.Alt, Meta: key.Meta,
	}, true
}

var tipKeyCodes = func() map[uint16]string {
	codes := map[uint16]string{
		0x08: "Backspace", 0x09: "Tab", 0x0d: "Enter", 0x1b: "Escape", 0x20: "Space",
		0x21: "PageUp", 0x22: "PageDown", 0x23: "End", 0x24: "Home",
		0x25: "ArrowLeft", 0x26: "ArrowUp", 0x27: "ArrowRight", 0x28: "ArrowDown",
		0x2d: "Insert", 0x2e: "Delete",
		0x6a: "NumpadMultiply", 0x6b: "NumpadAdd", 0x6d: "NumpadSubtract", 0x6e: "NumpadDecimal", 0x6f: "NumpadDivide",
		// VK_OEM_*, named as on a U.S. keyboard like Code on the other platforms.
		0xba: "Semicolon", 0xbb: "Equal", 0xbc: "Comma", 0xbd: "Minus", 0xbe: "Period", 0xbf: "Slash",
		0xc0: "Backquote", 0xdb: "BracketLeft", 0xdc: "Backslash", 0xdd: "BracketRight", 0xde: "Quote",
	}
	for i := range uint16(10) {
		codes[0x60+i] = "Numpad" + string(rune('0'+i))
	}
	for i := range uint16(12) {
		codes[0x70+i] = "F" + strconv.Itoa(int(i)+1)
	}
	return codes
}()
