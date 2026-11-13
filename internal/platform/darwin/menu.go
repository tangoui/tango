//go:build darwin && !ios

package darwin

// Application menu bar (NSApp.mainMenu).
//
// The platform contract describes a menu as a tree of MenuItem values. Items
// with a Role map onto AppKit's standard actions, which travel the responder
// chain (or go to NSApp) exactly like in a nib-built menu; items with an ID
// call back into the core through a single TangoMenuTarget, identified by the
// NSMenuItem tag.

import (
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

const nsControlStateValueOn = 1 // NSControlStateValueOn

// appTargetRoles are the role actions NSApplication itself implements; they
// get NSApp as an explicit target instead of relying on the responder chain.
var appTargetRoles = map[string]bool{
	"about": true, "hide": true, "hideOthers": true, "unhide": true, "quit": true,
}

const menuClickSelector = "tangoMenuClicked:"

func registerMenuTarget() {
	clicked := func(_ id, _ objc.SEL, sender id) {
		tag := uint32(sendInt(sender, "tag"))
		if routeMenuClick(tag) { // popup / tray menus (menuext.go)
			return
		}
		if b := theBackend; b != nil && b.menuClick != nil {
			b.menuClick(tag)
		}
	}
	// NSMenuItemValidation: menus that contain role items auto-enable, so
	// our own items must answer with the state the app assigned.
	validate := func(_ id, _ objc.SEL, sender id) bool {
		b := theBackend
		if b == nil {
			return true
		}
		tag := uint32(sendInt(sender, "tag"))
		if on, known := extraMenuEnabled(tag); known {
			return on
		}
		on, known := b.menuEnabled[tag]
		return on || !known
	}
	classDef("TangoMenuTarget", "NSObject", nil, []objc.MethodDef{
		method(menuClickSelector, clicked),
		method("validateMenuItem:", validate),
	})
}

func (b *backend) appName() string {
	if name := b.opts.Name; name != "" {
		return name
	}
	info := send(class("NSProcessInfo"), "processInfo")
	return goString(send(info, "processName"))
}

// installDefaultMenu gives the app the standard App/Edit/Window menus so
// Cmd+Q, Cmd+C/V and friends work before the app sets its own. Pool required.
func (b *backend) installDefaultMenu() {
	b.setMenu([]platform.MenuItem{{Role: "appMenu"}, {Role: "editMenu"}, {Role: "windowMenu"}})
}

func (b *backend) SetAppMenu(parent platform.SurfaceID, items []platform.MenuItem, onClick func(id uint32)) {
	b.menuClick = onClick
	withPool(func() { b.setMenu(items) })
}

// setMenu replaces the whole menu bar. Pool required.
func (b *backend) setMenu(items []platform.MenuItem) {
	clear(b.menuEnabled)
	mb := menuBuilder{b: b}
	bar := newMenu("")
	var windowMenu id
	for _, top := range items {
		if top.Separator {
			continue // the menu bar has no separators
		}
		title, entries := mb.expand(top)
		sub := mb.menu(title, entries)
		head := autorelease(alloc("NSMenuItem"))
		send(head, "setTitle:", uintptr(nsString(title)))
		send(head, "setSubmenu:", uintptr(sub))
		send(bar, "addItem:", uintptr(head))
		if top.Role == "windowMenu" {
			windowMenu = sub
		}
	}
	send(b.app, "setMainMenu:", uintptr(bar))
	if windowMenu != 0 {
		// Lets AppKit list open windows there.
		send(b.app, "setWindowsMenu:", uintptr(windowMenu))
	}
}

// buildMenu builds an autoreleased NSMenu from items; popup and tray menus
// (menuext.go) use it too.
func (b *backend) buildMenu(title string, items []platform.MenuItem) id {
	mb := menuBuilder{b: b}
	return mb.menu(title, items)
}

// newMenu returns an autoreleased, empty NSMenu.
func newMenu(title string) id {
	m := send(class("NSMenu"), "alloc")
	return autorelease(send(m, "initWithTitle:", uintptr(nsString(title))))
}

// menuBuilder turns MenuItem trees into NSMenu objects for one setMenu call;
// it looks the app name up at most once.
type menuBuilder struct {
	b    *backend
	name string
}

func (mb *menuBuilder) appName() string {
	if mb.name == "" {
		mb.name = mb.b.appName()
	}
	return mb.name
}

// expand works out the title and entries of a submenu. A role submenu
// without explicit children gets its standard contents; a plain leaf at the
// top level is wrapped in a submenu of its own, since the menu bar can only
// hold submenus.
func (mb *menuBuilder) expand(it platform.MenuItem) (string, []platform.MenuItem) {
	title, entries := it.Label, it.Submenu
	if std, ok := roleMenus[it.Role]; ok {
		if title == "" {
			title = roleLabel(std.title, mb.appName())
		}
		if len(entries) == 0 {
			entries = make([]platform.MenuItem, 0, len(std.items))
			for _, role := range std.items {
				entries = append(entries, platform.MenuItem{Role: role, Separator: role == ""})
			}
		}
	}
	if len(entries) == 0 && it.Role == "" {
		entries = []platform.MenuItem{it}
	}
	return title, entries
}

// menu builds an autoreleased NSMenu. Auto-enabling is only switched on when
// a standard role is present, because those depend on the responder chain;
// otherwise the Disabled flags are applied verbatim.
func (mb *menuBuilder) menu(title string, entries []platform.MenuItem) id {
	m := newMenu(title)
	send(m, "setAutoenablesItems:", boolArg(hasStandardRole(entries)))
	for _, e := range entries {
		send(m, "addItem:", uintptr(mb.item(e)))
	}
	return m
}

func hasStandardRole(entries []platform.MenuItem) bool {
	for _, e := range entries {
		if _, ok := menuRoles[e.Role]; ok {
			return true
		}
	}
	return false
}

// item builds one autoreleased NSMenuItem (or the shared separator).
func (mb *menuBuilder) item(it platform.MenuItem) id {
	if it.Separator {
		return send(class("NSMenuItem"), "separatorItem")
	}
	mi := autorelease(alloc("NSMenuItem"))
	label, shortcut := it.Label, it.Accelerator
	if std, isRole := menuRoles[it.Role]; isRole {
		if label == "" {
			label = roleLabel(std.label, mb.appName())
		}
		if shortcut == "" {
			shortcut = std.accel
		}
		send(mi, "setAction:", uintptr(sel(std.selector)))
		if appTargetRoles[it.Role] {
			send(mi, "setTarget:", uintptr(mb.b.app))
		}
	} else if it.ID != 0 && len(it.Submenu) == 0 {
		mb.b.menuEnabled[it.ID] = !it.Disabled
		send(mi, "setTag:", uintptr(it.ID))
		send(mi, "setTarget:", uintptr(mb.b.menuTarget))
		send(mi, "setAction:", uintptr(sel(menuClickSelector)))
	}
	send(mi, "setTitle:", uintptr(nsString(label)))
	applyShortcut(mi, shortcut)
	if it.Checked {
		send(mi, "setState:", nsControlStateValueOn)
	}
	if it.Disabled {
		send(mi, "setEnabled:", boolArg(false))
	}
	setItemIcon(mi, it.Icon) // menuext.go
	if len(it.Submenu) > 0 || roleMenus[it.Role].title != "" {
		title, entries := mb.expand(platform.MenuItem{Label: label, Role: it.Role, Submenu: it.Submenu})
		send(mi, "setSubmenu:", uintptr(mb.menu(title, entries)))
	}
	return mi
}

// applyShortcut sets the key equivalent described by an accelerator string
// such as "Cmd+Shift+Z"; unparsable or empty accelerators are ignored.
func applyShortcut(mi id, accel string) {
	key, mods, err := parseAccelerator(accel)
	if err != nil || key == "" {
		return
	}
	send(mi, "setKeyEquivalent:", uintptr(nsString(key)))
	send(mi, "setKeyEquivalentModifierMask:", uintptr(mods))
}
