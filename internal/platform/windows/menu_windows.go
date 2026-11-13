//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

type menuSpec struct {
	plan    *menuPlan
	onClick func(id uint32)
	empty   bool
}

// SetAppMenu sets the window menu bar of parent, or of every surface
// without its own menu (now and later) when parent is 0. An empty items
// list removes the menu.
func (b *Backend) SetAppMenu(parent platform.SurfaceID, items []platform.MenuItem, onClick func(id uint32)) {
	spec := &menuSpec{plan: buildMenuPlan(items, b.opts.Name), onClick: onClick, empty: len(items) == 0}
	if parent == 0 {
		b.defaultMenu = spec
		if spec.empty {
			b.defaultMenu = nil
		}
		for _, s := range b.byID {
			if !s.explicitMenu {
				s.applyMenu(spec)
			}
		}
		return
	}
	if s := b.byID[parent]; s != nil {
		s.explicitMenu = true
		s.applyMenu(spec)
	}
}

func (s *surface) applyMenu(spec *menuSpec) {
	if s.closed {
		return
	}
	var hmenu, haccel uintptr
	if !spec.empty {
		hmenu, _, _ = procCreateMenu.Call()
		appendMenuEntries(hmenu, hmenu, spec.plan.Entries, dpiOf(s.hwnd))
		haccel = createAccelTable(spec.plan.Accels)
	}
	old, oldAccel := s.menu, s.accel
	procSetMenu.Call(s.hwnd, hmenu)
	destroyMenu(old) // also frees its item bitmaps
	if oldAccel != 0 {
		procDestroyAcceleratorTable.Call(oldAccel)
	}
	s.menu, s.accel, s.spec = hmenu, haccel, spec
	if spec.empty {
		s.spec = nil
	}
	procDrawMenuBar.Call(s.hwnd)
}

// appendEntries fills hmenu (a fresh, empty menu) at the system DPI.
func appendEntries(hmenu uintptr, entries []menuEntry) {
	appendMenuEntries(hmenu, hmenu, entries, dpiOf(0))
}

// appendMenuEntries fills hmenu, a fresh menu or a submenu of root. Item
// bitmaps (icons) are recorded under root and freed by destroyMenu or
// releaseMenuBitmaps(root); icons are sized for dpi.
func appendMenuEntries(root, hmenu uintptr, entries []menuEntry, dpi int) {
	for i, e := range entries {
		switch e.Kind {
		case entrySeparator:
			procAppendMenuW.Call(hmenu, mfSeparator, 0, 0)
		case entryPopup:
			sub, _, _ := procCreatePopupMenu.Call()
			appendMenuEntries(root, sub, e.Children, dpi)
			flags := uintptr(mfPopup | mfString)
			if e.Disabled {
				flags |= mfGrayed
			}
			procAppendMenuW.Call(hmenu, flags, sub, uintptr(unsafe.Pointer(u16(e.Label))))
			setItemExtras(root, hmenu, i, e, dpi)
		default:
			flags := uintptr(mfString)
			if e.Disabled {
				flags |= mfGrayed
			}
			if e.Checked {
				flags |= mfChecked
			}
			procAppendMenuW.Call(hmenu, flags, uintptr(e.Cmd), uintptr(unsafe.Pointer(u16(e.Label))))
			// Entries map one-to-one to items, so i is the item position.
			setItemExtras(root, hmenu, i, e, dpi)
		}
	}
}

func createAccelTable(entries []accelEntry) uintptr {
	if len(entries) == 0 {
		return 0
	}
	table := make([]accel, len(entries))
	for i, e := range entries {
		table[i] = accel{FVirt: e.Accel.fVirt(), Key: e.Accel.VK, Cmd: e.Cmd}
	}
	h, _, _ := procCreateAcceleratorTableW.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(len(table)))
	return h
}

func (s *surface) runCommand(cmd uint16) {
	if s.spec == nil {
		return
	}
	act, ok := s.spec.plan.Actions[cmd]
	if !ok {
		return
	}
	if act.Role != "" {
		s.runRole(act.Role)
		return
	}
	if act.ID != 0 && s.spec.onClick != nil {
		s.spec.onClick(act.ID)
	}
}

func (s *surface) runRole(role string) {
	if cmd, ok := roleEditCommand(role); ok {
		if s.canvas != nil {
			s.canvas.editRole(role) // the matching Ctrl+key
			return
		}
		// Chromium editing commands through a synthetic key, as mygo does:
		// unlike execCommand from script, paste works this way.
		s.focusWebView()
		s.devtools("Input.dispatchKeyEvent", `{"type":"rawKeyDown","key":"Unidentified","commands":["`+cmd+`"]}`)
		s.devtools("Input.dispatchKeyEvent", `{"type":"keyUp","key":"Unidentified"}`)
		return
	}
	switch role {
	case "quit":
		s.b.userQuit() // asks HostEx.ShouldQuit; appevents_windows.go
	case "close":
		postMessage(s.hwnd, wmClose, 0, 0)
	case "minimize", "hide":
		procShowWindow.Call(s.hwnd, swMinimize)
	case "about":
		name := s.b.opts.Name
		if name == "" {
			name = "Tango"
		}
		hwnd := s.hwnd
		s.b.runDeferred(func() {
			procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(u16(name))),
				uintptr(unsafe.Pointer(u16(rolenames.Format("win.aboutApp", name)))), mbIconInformation)
		})
	}
}
