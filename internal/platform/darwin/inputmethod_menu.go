//go:build darwin && !ios

package darwin

import (
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

func imeMenu(self id, _ objc.SEL) id {
	var menu id
	imeOnMain(func() {
		sink, ok := imeHost.sink.(platform.InputMethodMenus)
		if !ok {
			return
		}
		items := sink.IMEMenu()
		if len(items) == 0 {
			return
		}
		menu = alloc("NSMenu")
		send(menu, "setAutoenablesItems:", 0)
		for _, item := range items {
			if item.Separator {
				send(menu, "addItem:", uintptr(send(class("NSMenuItem"), "separatorItem")))
				continue
			}
			native := send(class("NSMenuItem"), "alloc")
			native = send(native, "initWithTitle:action:keyEquivalent:", uintptr(nsString(item.Title)),
				uintptr(sel("tangoMenuItem:")), uintptr(nsString("")))
			send(native, "setTarget:", uintptr(self))
			send(native, "setRepresentedObject:", uintptr(nsString(item.ID)))
			if item.Checked {
				send(native, "setState:", 1)
			}
			if item.Disabled {
				send(native, "setEnabled:", 0)
			}
			send(menu, "addItem:", uintptr(native))
			release(native)
		}
	})
	// The caller owns the surrounding pool. The menu must survive the pool
	// used to construct it on the main thread.
	return autorelease(menu)
}

func imeMenuItem(_ id, _ objc.SEL, sender id) {
	imeOnMain(func() {
		sink, ok := imeHost.sink.(platform.InputMethodMenus)
		if !ok {
			return
		}
		item := sender
		if sendBool(sender, "isKindOfClass:", uintptr(class("NSDictionary"))) {
			item = send(sender, "objectForKey:", uintptr(imeRuntime.menuItemKey))
		}
		if !sendBool(item, "isKindOfClass:", uintptr(class("NSMenuItem"))) || !sendBool(item, "isEnabled") {
			return
		}
		value := send(item, "representedObject")
		if sendBool(value, "isKindOfClass:", uintptr(class("NSString"))) {
			sink.IMEMenuClicked(goString(value))
		}
	})
}
