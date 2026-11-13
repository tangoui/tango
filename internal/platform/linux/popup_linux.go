//go:build linux && !android

package linux

import "github.com/tangoui/tango/internal/platform"

var _ platform.PopupMenu = (*backend)(nil)

// PopupMenu shows items as a GtkMenu at (x, y) in the surface's web view.
// It returns at once; onClick (if an item is chosen) and then onClose run
// later on the main loop. onClose is called exactly once, also when the
// menu cannot be shown.
func (b *backend) PopupMenu(sid platform.SurfaceID, x, y int, items []platform.MenuItem, onClick func(id uint32), onClose func()) {
	finished := false
	finish := func() {
		if finished {
			return
		}
		finished = true
		if onClose != nil {
			onClose()
		}
	}
	s := b.surfaces[sid]
	if !b.loaded.Load() || s == nil || s.closed {
		later(0, finish)
		return
	}
	bindMenuExt()
	entries := buildMenuEntries(items, b.opts.Name)
	if !mx.popupOK || len(entries) == 0 {
		later(0, finish)
		return
	}
	// The content widget: the web view, or a canvas surface's canvas.
	win := mx.widgetGetWindow(s.content)
	if win == 0 { // not realized yet
		later(0, finish)
		return
	}

	m := &appMenu{items: items, onClick: onClick}
	menu := gtkMenuNew()
	mb := menuBuilder{activate: func(e menuEntry) { s.menuActivate(e, m) }}
	mb.fill(menu, entries)
	// Attached menus are destroyed with the content widget (window closed while
	// the menu is open); "destroy" then reports the close.
	mx.attachToWidget(menu, s.content, 0)

	gone := false
	connect(menu, "destroy", 0, func(_, _ ptr) ptr {
		gone = true
		later(0, finish)
		return 0
	})
	// GTK emits "deactivate" before the chosen item's "activate" (both
	// within one gtk_menu_shell_activate_item call), so the menu is torn
	// down and onClose reported from an idle callback that runs after
	// onClick. Destroying the menu inside "deactivate" would also free it
	// under GTK's feet.
	connect(menu, "deactivate", 0, func(_, _ ptr) ptr {
		later(0, func() {
			if !gone {
				gone = true
				gtkWidgetDestroy(menu)
			}
			finish()
		})
		return 0
	})

	// The rectangle is relative to win: the content widget's own GdkWindow, or
	// its parent's when it has none (then offset by its allocation).
	r := gdkRect{X: int32(x), Y: int32(y), Width: 1, Height: 1}
	if !mx.widgetGetHasWindow(s.content) {
		var a gdkRect
		mx.widgetGetAllocation(s.content, &a)
		r.X += a.X
		r.Y += a.Y
	}
	// GTK falls back to gtk_get_current_event itself; passing it
	// explicitly keeps that working where the symbol exists. Popups
	// requested from an idle callback (the usual case: the page asked over
	// IPC) have no current event.
	var trigger ptr
	if mx.getCurrentEvent != nil && mx.eventFree != nil {
		trigger = mx.getCurrentEvent()
	}
	mx.popupAtRect(menu, win, &r, gdkGravityNorthWest, gdkGravityNorthWest, trigger)
	if trigger != 0 {
		mx.eventFree(trigger)
	}
	// Without a grab (e.g. Wayland refusing a popup with no input event)
	// GTK warns and does not show the menu; no "deactivate" would follow.
	if !gone && !mx.widgetGetVisible(menu) {
		gone = true
		gtkWidgetDestroy(menu)
		later(0, finish)
	}
}
