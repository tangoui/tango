package linux

import (
	"strings"
	"unicode"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func ibusKey(keyval, hardware, state uint32, char rune) platform.IMEKey {
	key, _ := canvasKey(keyval, uint16(hardware+8)) // IBus uses evdev; GDK adds eight.
	event := platform.IMEKey{
		Shift:    state&1 != 0,
		CapsLock: state&2 != 0,
		Ctrl:     state&4 != 0,
		Alt:      state&8 != 0,
		Meta:     state&(1<<6|1<<26|1<<28) != 0, // X Mod4, Super or Meta
	}
	name := key.String()
	switch {
	case key == input.KeyUnknown:
	case key == input.KeyCtrlLeft || key == input.KeyCtrlRight:
		event.Code = "Control" + strings.TrimPrefix(name, "Ctrl")
	case key == input.KeyUp || key == input.KeyDown || key == input.KeyLeft || key == input.KeyRight:
		event.Code = "Arrow" + name
	case len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z':
		event.Code = "Key" + name
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		event.Code = "Digit" + name
	default:
		event.Code = name
	}
	// keyval ignores Control; the client would type nothing.
	if unicode.IsPrint(char) && !event.Ctrl {
		event.Text = string(char)
	}
	return event
}

const ibusReleaseMask = 1 << 30

func ibusModifier(keyval, state uint32) (platform.IMEModifierKey, bool) {
	var code string
	var mask uint32
	switch keyval {
	case 0xffe1:
		code, mask = "ShiftLeft", 1
	case 0xffe2:
		code, mask = "ShiftRight", 1
	case 0xffe3:
		code, mask = "ControlLeft", 4
	case 0xffe4:
		code, mask = "ControlRight", 4
	case 0xffe5:
		code = "CapsLock"
	case 0xffe9:
		code, mask = "AltLeft", 8
	case 0xffea, 0xfe03:
		code, mask = "AltRight", 8
	case 0xffe7, 0xffeb:
		code, mask = "MetaLeft", 1<<6|1<<26|1<<28
	case 0xffe8, 0xffec:
		code, mask = "MetaRight", 1<<6|1<<26|1<<28
	default:
		return platform.IMEModifierKey{}, false
	}
	down := state&ibusReleaseMask == 0
	if down {
		state |= mask
	} else {
		state &^= mask
	}
	key := ibusKey(0, 0, state, 0)
	return platform.IMEModifierKey{Code: code, Down: down, Shift: key.Shift, Ctrl: key.Ctrl,
		Alt: key.Alt, Meta: key.Meta, CapsLock: key.CapsLock}, true
}

type ibusHeldKeys map[uint32]bool

func (keys ibusHeldKeys) event(keyval, hardware, state uint32) bool {
	id := hardware
	if id == 0 {
		id = keyval | 1<<31
	}
	if state&ibusReleaseMask != 0 {
		delete(keys, id)
		return false
	}
	repeat := keys[id]
	keys[id] = true
	return repeat
}
