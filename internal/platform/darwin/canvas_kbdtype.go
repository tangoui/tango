//go:build darwin && !ios

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
)

// kKeyboardISO, the PhysicalKeyboardLayoutType of ISO keyboards
// (HIToolbox Keyboards.h, the four-character code 'ISO ').
const kbdLayoutISO = 0x49534F20

var kbdType struct {
	once   sync.Once
	last   func() uint8           // LMGetKbdType: the keyboard of the last event
	layout func(kbd int16) uint32 // KBGetLayoutType
}

// macISOKeyboard reports whether the keyboard that sent the current event
// has the ISO layout. False when Carbon is unavailable.
func macISOKeyboard() bool {
	kbdType.once.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&kbdType.last, lib, "LMGetKbdType")
		purego.RegisterLibFunc(&kbdType.layout, lib, "KBGetLayoutType")
	})
	if kbdType.last == nil || kbdType.layout == nil {
		return false
	}
	return kbdType.layout(int16(kbdType.last())) == kbdLayoutISO
}
