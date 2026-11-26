package linux

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var gtkWebKeyvals = func() map[input.Key]uint32 {
	keys := make(map[input.Key]uint32)
	for keyval, key := range canvasGdkKeyvals {
		// Prefer the standard keysym to keypad aliases, Meta (Shift+Alt)
		// and the ISO variants. The minimum is stable across map order.
		if old := keys[key]; old == 0 || keyval < old {
			keys[key] = keyval
		}
	}
	keys[input.KeyEnter] = 0xff0d // Return
	keys[input.KeyTab] = 0xff09   // Tab
	keys[input.KeyAltLeft] = 0xffe9
	keys[input.KeyAltRight] = 0xffea
	keys[input.KeyNumpadDecimal] = 0xffae
	return keys
}()

type gtkWebKeyData struct {
	keyval, state uint32
	text          string
	modifier      bool
}

func gtkWebKey(key input.Key, mods input.Modifiers) (gtkWebKeyData, bool) {
	k := gtkWebKeyData{keyval: gtkWebKeyvals[key]}
	for _, m := range [...]struct {
		mod  input.Modifiers
		mask uint32
	}{{input.ModShift, canvasGdkShiftMask}, {input.ModCtrl, canvasGdkControlMask}, {input.ModAlt, canvasGdkMod1Mask}, {input.ModMeta, canvasGdkMod4Mask | canvasGdkSuperMask}} {
		if mods&m.mod != 0 {
			k.state |= m.mask
		}
	}
	r := platform.WebKeyRune(key, mods)
	switch {
	case key >= input.KeyF1 && key <= input.KeyF12:
		k.keyval = 0xffbe + uint32(key-input.KeyF1)
	case key >= input.KeyNumpad0 && key <= input.KeyNumpad9:
		k.keyval = 0xffb0 + uint32(key-input.KeyNumpad0)
	case key == input.KeyTab && mods&input.ModShift != 0:
		k.keyval = 0xfe20 // ISO_Left_Tab
	case k.keyval == 0 && r != 0:
		k.keyval = uint32(r) // ASCII keyvals equal Unicode
	}
	if mods&(input.ModCtrl|input.ModAlt|input.ModMeta) == 0 {
		if key == input.KeyEnter || key == input.KeyNumpadEnter {
			r = '\r'
		}
		if r != 0 {
			k.text = string(r)
		}
	}
	k.modifier = canvasModifierKeyMask(k.keyval) != 0 || key == input.KeyCapsLock
	return k, k.keyval != 0
}

// GdkKeymapKey has three integer fields in the GTK3 ABI.
type gdkKeymapEntry struct {
	code         uint32
	group, level int32
}

func gtkWebKeycode(entries []gdkKeymapEntry, shifted bool) (code uint16, group uint8) {
	if len(entries) == 0 {
		return 0, 0
	}
	var level int32
	if shifted {
		level = 1
	}
	best := entries[0]
	for _, entry := range entries {
		if entry.group < best.group || entry.group == best.group && entry.level == level {
			best = entry
		}
	}
	return uint16(best.code), uint8(best.group)
}
