//go:build darwin && !ios && tango_dev

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var webKeyEvent struct {
	sync.Once
	new func(id, objc.SEL, uint, nsPoint, uint, float64, int, id, id, id, bool, uint16) id
}

func (s *surface) DispatchWebKey(key input.Key, mods input.Modifiers, done func(error)) {
	code, chars, flags, ok := macWebKey(key, mods)
	if !ok || s.web == 0 || s.closed {
		done(platform.ErrUnsupported)
		return
	}
	webKeyEvent.Do(func() { purego.RegisterFunc(&webKeyEvent.new, msgSendAddr) })
	var err error
	withPool(func() {
		web := s.web
		win := send(web, "window")
		if win == 0 {
			err = platform.ErrUnsupported
			return
		}
		target := send(win, "firstResponder")
		if target != web && !(sendBool(target, "isKindOfClass:", uintptr(class("NSView"))) &&
			sendBool(target, "isDescendantOf:", uintptr(web))) {
			if !sendBool(win, "makeFirstResponder:", uintptr(web)) {
				err = platform.ErrUnsupported
				return
			}
			target = send(win, "firstResponder")
		}
		number := sendInt(win, "windowNumber")
		makeEvent := func(kind uint, flags uint) id {
			return webKeyEvent.new(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
				kind, nsPoint{}, flags, 0, number, 0, nsString(chars), nsString(chars), false, code)
		}
		if modifier, ok := macModifierKeys[code]; ok {
			const flagsChanged = 12
			mod := [...]uint{nsShiftKeyMask, nsControlKeyMask, nsAlternateKeyMask, nsCommandKeyMask}[(modifier.key-input.KeyShiftLeft)/2]
			for _, state := range []uint{flags | mod | modifier.side, flags &^ mod} {
				ev := makeEvent(flagsChanged, state)
				if ev == 0 {
					err = platform.ErrUnsupported
					return
				}
				send(target, "flagsChanged:", uintptr(ev))
			}
			return
		}
		down, up := makeEvent(10, flags), makeEvent(11, flags) // NSEventTypeKeyDown / NSEventTypeKeyUp
		if down == 0 || up == 0 {
			err = platform.ErrUnsupported
			return
		}
		if !s.performWebKeyEquivalent(win, target, down) {
			send(target, "keyDown:", uintptr(down))
		}
		if !s.closed {
			send(target, "keyUp:", uintptr(up))
		}
	})
	done(err)
}

// AppKit normally looks up responder-chain actions in the key window.
// Bind those menu items to the selected window's responders for the call,
// so Edit actions also work when this window is behind another app.
func (s *surface) performWebKeyEquivalent(win, responder, event id) bool {
	menu := retain(send(s.b.app, "mainMenu"))
	defer release(menu)
	if sendBool(win, "isKeyWindow") {
		return sendBool(menu, "performKeyEquivalent:", uintptr(event))
	}
	var bound []id
	var bindTargets func(id)
	bindTargets = func(m id) {
		for i, n := 0, sendInt(m, "numberOfItems"); i < n; i++ {
			item := send(m, "itemAtIndex:", uintptr(i))
			if sub := send(item, "submenu"); sub != 0 {
				bindTargets(sub)
				continue
			}
			action := send(item, "action")
			if action == 0 || send(item, "target") != 0 {
				continue
			}
			for target := responder; target != 0; target = send(target, "nextResponder") {
				if sendBool(target, "respondsToSelector:", uintptr(action)) {
					send(item, "setTarget:", uintptr(target))
					bound = append(bound, retain(item))
					break
				}
				if target == win {
					break
				}
			}
		}
	}
	bindTargets(menu)
	defer func() {
		for _, item := range bound {
			send(item, "setTarget:", 0)
			release(item)
		}
	}()
	return sendBool(menu, "performKeyEquivalent:", uintptr(event))
}
