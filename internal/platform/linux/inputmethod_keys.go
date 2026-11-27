package linux

import (
	"unicode"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func ibusKey(keyval, hardware, state uint32, char rune) platform.IMEKey {
	key, _ := canvasKey(keyval, uint16(hardware+8)) // IBus uses evdev; GDK adds eight.
	event := platform.IMEKey{
		Shift: state&1 != 0,
		Ctrl:  state&4 != 0,
		Alt:   state&8 != 0,
		Meta:  state&(1<<6|1<<26|1<<28) != 0, // X Mod4, Super or Meta
	}
	name := key.String()
	switch {
	case key == input.KeyUnknown:
	case key == input.KeyUp || key == input.KeyDown || key == input.KeyLeft || key == input.KeyRight:
		event.Code = "Arrow" + name
	case len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z':
		event.Code = "Key" + name
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		event.Code = "Digit" + name
	default:
		event.Code = name
	}
	if unicode.IsPrint(char) {
		event.Text = string(char)
	}
	return event
}
