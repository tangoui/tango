//go:build linux && !android && tango_dev

package linux

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var webKeyGTK struct {
	sync.Once
	ok               bool
	activateKey      func(ptr, ptr) bool
	grabFocus        func(ptr)
	isMapped         func(ptr) bool
	keymapForDisplay func(ptr) ptr
	keymapEntries    func(ptr, uint32, *ptr, *int32) bool
}

func bindWebKeyGTK() {
	var bd binder
	k := &webKeyGTK
	k.ok = bd.opt(&k.activateKey, "gtk_window_activate_key", libGTK) &&
		bd.opt(&k.grabFocus, "gtk_widget_grab_focus", libGTK) &&
		bd.opt(&k.isMapped, "gtk_widget_get_mapped", libGTK) &&
		bd.opt(&k.keymapForDisplay, "gdk_keymap_get_for_display", libGDK, libGTK) &&
		bd.opt(&k.keymapEntries, "gdk_keymap_get_entries_for_keyval", libGDK, libGTK)
}

func (s *surface) DispatchWebKey(key input.Key, mods input.Modifiers, done func(error)) {
	done(s.dispatchWebKey(key, mods))
}

func (s *surface) dispatchWebKey(key input.Key, mods input.Modifiers) error {
	if s.closed {
		return platform.ErrWebViewClosed
	}
	k, ok := gtkWebKey(key, mods)
	if !ok || s.web == 0 || s.win == 0 {
		return platform.ErrUnsupported
	}
	f := &webOperations
	f.once.Do(bindWebOperations)
	webKeyGTK.Do(bindWebKeyGTK)
	if !f.inputOK || !webKeyGTK.ok {
		return platform.ErrUnsupported
	}
	win := f.widgetWindow(s.web)
	if win == 0 || !webKeyGTK.isMapped(s.web) {
		return platform.ErrUnsupported
	}
	display := f.windowDisplay(win)
	seat := f.displaySeat(display)
	if seat == 0 {
		return platform.ErrUnsupported
	}
	device := f.seatKeyboard(seat)
	if device == 0 {
		return platform.ErrUnsupported
	}

	var codes ptr
	var n int32
	var code uint16
	var group uint8
	if keymap := webKeyGTK.keymapForDisplay(display); keymap != 0 && webKeyGTK.keymapEntries(keymap, k.keyval, &codes, &n) {
		code, group = gtkWebKeycode(unsafe.Slice((*gdkKeymapEntry)(toPtr(codes)), n), mods&input.ModShift != 0)
		gFree(codes)
	}

	const keyPress, keyRelease = 8, 9 // GDK_KEY_PRESS / GDK_KEY_RELEASE
	webKeyGTK.grabFocus(s.web)
	for _, kind := range []int32{keyPress, keyRelease} {
		event := f.eventNew(kind)
		if event == 0 {
			return errors.New("tango/linux: could not create WebView key event")
		}
		e := (*webGDKKey)(toPtr(event))
		e.window, e.send = wv.objectRef(win), 1
		e.keyval, e.state, e.keycode, e.group = k.keyval, k.state, code, group
		if k.modifier {
			e.modifier = 1
			bit := canvasModifierKeyMask(k.keyval)
			switch key {
			case input.KeyCapsLock:
				bit = canvasGdkLockMask
			case input.KeyMetaLeft, input.KeyMetaRight:
				bit |= canvasGdkMod4Mask | webGdkMetaMask
			}
			if kind == keyPress {
				e.state &^= bit
			} else {
				e.state |= bit
			}
		}
		if k.text != "" {
			e.length, e.text = int32(len(k.text)), f.strdup(k.text)
		}
		f.eventDevice(event, device)
		used := kind == keyPress && webKeyGTK.activateKey(s.win, event)
		if !used && !s.closed {
			f.widgetEvent(s.web, event)
		}
		f.eventFree(event)
		if s.closed {
			break
		}
	}
	return nil
}
