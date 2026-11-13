//go:build darwin && !ios

package darwin

// platform.AppShortcuts on macOS: the Dock menu, returned by the app
// delegate's applicationDockMenu: (appevents.go lists the method). AppKit
// asks for it each time the user opens the Dock menu, so replacing the
// menu takes effect on the next open.

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.AppShortcuts = (*backend)(nil)

var dock struct {
	once    sync.Once
	menu    id // NSMenu, retained, or 0
	target  id // TangoDockTarget
	ids     []string
	onClick func(string)
}

func registerDockTarget() {
	dock.once.Do(func() {
		classDef("TangoDockTarget", "NSObject", nil, []objc.MethodDef{
			method("dockItem:", func(self id, _ objc.SEL, item id) {
				i := sendInt(item, "tag")
				if i < 0 || i >= len(dock.ids) || dock.onClick == nil {
					return
				}
				dock.onClick(dock.ids[i])
			}),
		})
		withPool(func() { dock.target = alloc("TangoDockTarget") })
	})
}

// dockMenuMethod is the app delegate's applicationDockMenu:.
func dockMenuMethod() objc.MethodDef {
	return method("applicationDockMenu:", func(self id, _ objc.SEL, app id) id { return dock.menu })
}

func (b *backend) SetAppShortcuts(items []platform.ShortcutItem, onClick func(string)) error {
	registerDockTarget()
	withPool(func() {
		old := dock.menu
		dock.menu, dock.ids, dock.onClick = 0, nil, onClick
		if len(items) > 0 {
			m := alloc("NSMenu")
			send(m, "setAutoenablesItems:", 0)
			for _, it := range items {
				if it.Separator {
					send(m, "addItem:", uintptr(send(class("NSMenuItem"), "separatorItem")))
					continue
				}
				mi := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
					uintptr(nsString(it.Label)), uintptr(sel("dockItem:")), uintptr(nsString(""))))
				send(mi, "setTarget:", uintptr(dock.target))
				send(mi, "setTag:", uintptr(len(dock.ids)))
				send(mi, "setEnabled:", boolArg(!it.Disabled))
				if it.Description != "" {
					send(mi, "setToolTip:", uintptr(nsString(it.Description)))
				}
				send(m, "addItem:", uintptr(mi))
				dock.ids = append(dock.ids, it.ID)
			}
			dock.menu = m
		}
		release(old)
	})
	return nil
}

// SmokeDockMenu returns the Dock menu's titles ("-" for separators) and
// SmokeDockChoose performs item index's action. For the sysx smoke program.
func (b *backend) SmokeDockMenu() []string {
	var out []string
	withPool(func() {
		m := send(b.delegate, "applicationDockMenu:", uintptr(b.app))
		for _, it := range arrayItems(send(m, "itemArray")) {
			if sendBool(it, "isSeparatorItem") {
				out = append(out, "-")
				continue
			}
			out = append(out, goString(send(it, "title")))
		}
	})
	return out
}

func (b *backend) SmokeDockChoose(index int) bool {
	ok := false
	withPool(func() {
		m := send(b.delegate, "applicationDockMenu:", uintptr(b.app))
		if m != 0 && index < sendInt(m, "numberOfItems") {
			send(m, "performActionForItemAtIndex:", uintptr(index))
			ok = true
		}
	})
	return ok
}
