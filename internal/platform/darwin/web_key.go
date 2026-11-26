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
	if s.closed {
		done(platform.ErrWebViewClosed)
		return
	}
	code, chars, ignoring, flags, ok := macWebKey(key, mods)
	if !ok || s.web == 0 {
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
		retain(target)
		defer release(target)
		number := sendInt(win, "windowNumber")
		makeEvent := func(kind uint, flags uint) id {
			return webKeyEvent.new(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
				kind, nsPoint{}, flags, 0, number, 0, nsString(chars), nsString(ignoring), false, code)
		}
		if modifier, ok := macModifierKeys[code]; ok {
			const flagsChanged = 12
			mod := macWebFlags(key.Modifier())
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
// A background window needs explicit targets. A private menu copy keeps
// these targets out of nested dispatch and AppKit's normal event loop.
func (s *surface) performWebKeyEquivalent(win, responder, event id) bool {
	menu := send(s.b.app, "mainMenu")
	if sendBool(win, "isKeyWindow") {
		return sendBool(menu, "performKeyEquivalent:", uintptr(event))
	}
	menu = send(menu, "copy")
	defer release(menu)
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
			target := responder
			for target != 0 && !sendBool(target, "respondsToSelector:", uintptr(action)) {
				if target == win {
					target = 0
				} else {
					target = send(target, "nextResponder")
				}
			}
			if target != 0 {
				send(item, "setTarget:", uintptr(target))
			} else {
				send(item, "setKeyEquivalent:", uintptr(nsString("")))
			}
		}
	}
	bindTargets(menu)
	return sendBool(menu, "performKeyEquivalent:", uintptr(event))
}
