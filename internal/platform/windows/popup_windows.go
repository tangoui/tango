//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.PopupMenu = (*Backend)(nil)

// PopupMenu shows a context menu at (x, y) in the surface's client area
// (logical px; the WebView fills the client area). The menu's modal loop
// runs from a deferred message, outside the caller's stack, like the
// dialogs: the call returns at once and onClick/onClose come later.
func (b *Backend) PopupMenu(id platform.SurfaceID, x, y int, items []platform.MenuItem, onClick func(id uint32), onClose func()) {
	plan := buildMenuPlan(items, b.opts.Name)
	b.runDeferred(func() {
		b.popupMenu(id, x, y, plan, onClick)
		if onClose != nil {
			onClose()
		}
	})
}

func (b *Backend) popupMenu(id platform.SurfaceID, x, y int, plan *menuPlan, onClick func(id uint32)) {
	s := b.byID[id]
	if s == nil || s.closed || menuTracking || len(plan.Entries) == 0 {
		return
	}
	dpi := dpiOf(s.hwnd)
	hmenu, _, _ := procCreatePopupMenu.Call()
	if hmenu == 0 {
		return
	}
	appendMenuEntries(hmenu, hmenu, plan.Entries, dpi)
	pt := point{X: int32(dipToPx(x, dpi)), Y: int32(dipToPx(y, dpi))}
	mtClientToScreen.Call(s.hwnd, uintptr(unsafe.Pointer(&pt)))
	cmd := trackMenu(hmenu, s.hwnd, pt.X, pt.Y)
	destroyMenu(hmenu)
	act, ok := plan.Actions[cmd]
	if cmd == 0 || !ok {
		return
	}
	if act.Role != "" {
		if !s.closed {
			s.runRole(act.Role)
		}
		return
	}
	if act.ID != 0 && onClick != nil {
		onClick(act.ID)
	}
}
