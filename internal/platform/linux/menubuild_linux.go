//go:build linux && !android

package linux

import (
	"sync"
)

// Native symbols for context menus, tray menus and check/radio items. Kept
// in their own struct and bound lazily with a private binder (like
// ffi_ext_linux.go), so a missing symbol degrades the feature instead of
// failing Init.
var mx struct {
	once sync.Once
	gdk  ptr

	setDrawAsRadio      func(item ptr, on bool)                                                          // optional
	popupAtRect         func(menu, gdkWin ptr, rect *gdkRect, rectAnchor, menuAnchor int32, trigger ptr) // GTK >= 3.22
	attachToWidget      func(menu, widget, detacher ptr)
	widgetGetWindow     func(w ptr) ptr
	widgetGetHasWindow  func(w ptr) bool
	widgetGetAllocation func(w ptr, a *gdkRect)
	widgetGetVisible    func(w ptr) bool
	getCurrentEvent     func() ptr  // optional
	eventFree           func(e ptr) // optional
	objectRefSink       func(o ptr) ptr

	popupOK bool // every symbol PopupMenu needs is bound
}

// gdkRect is GdkRectangle / GtkAllocation.
type gdkRect struct{ X, Y, Width, Height int32 }

const gdkGravityNorthWest = 1

func bindMenuExt() {
	mx.once.Do(func() {
		mx.gdk, _, _ = open("libgdk-3.so.0")
		var bd binder
		t, d, o := libGTK, mx.gdk, libGObject
		bd.opt(&mx.setDrawAsRadio, "gtk_check_menu_item_set_draw_as_radio", t)
		bd.opt(&mx.getCurrentEvent, "gtk_get_current_event", t)
		bd.opt(&mx.eventFree, "gdk_event_free", d, t)
		bd.req(&mx.objectRefSink, "g_object_ref_sink", o)
		bd.req(&mx.popupAtRect, "gtk_menu_popup_at_rect", t)
		bd.req(&mx.attachToWidget, "gtk_menu_attach_to_widget", t)
		bd.req(&mx.widgetGetWindow, "gtk_widget_get_window", t)
		bd.req(&mx.widgetGetHasWindow, "gtk_widget_get_has_window", t)
		bd.req(&mx.widgetGetAllocation, "gtk_widget_get_allocation", t)
		bd.req(&mx.widgetGetVisible, "gtk_widget_get_visible", t)
		mx.popupOK = len(bd.missing) == 0
	})
}

// menuBuilder turns resolved menu entries into GTK menu items. The app menu
// bar, context menus and tray menus share it; only what activating an
// entry does differs.
type menuBuilder struct {
	// accel binds entry accelerators; 0 (popup / tray menus) only shows
	// them next to the label.
	accel ptr
	// activate runs on "activate" for entries whose Action is not actNone.
	activate func(e menuEntry)
}

// fill appends entries to a GtkMenuShell and shows them.
func (mb *menuBuilder) fill(shell ptr, entries []menuEntry) {
	for _, e := range entries {
		item := mb.item(e)
		gtkMenuShellAppend(shell, item)
		gtkWidgetShow(item)
	}
}

func (mb *menuBuilder) item(e menuEntry) ptr {
	if e.Separator {
		return gtkSeparatorMenuItemNew()
	}
	bindMenuExt()
	var item ptr
	if e.Check {
		item = gtkCheckMenuItemNewWithMnemonic(e.Label)
		if e.Radio && mx.setDrawAsRadio != nil {
			mx.setDrawAsRadio(item, true)
		}
		// Before connecting: set_active emits "toggled" and "activate".
		// GTK flips the state itself when clicked; the core owns it and
		// re-sends the menu.
		if e.Checked {
			gtkCheckMenuItemSetActive(item, true)
		}
	} else {
		item = gtkMenuItemNewWithMnemonic(e.Label)
	}
	if e.Disabled {
		gtkWidgetSetSensitive(item, false)
	}
	if len(e.Children) > 0 {
		sub := gtkMenuNew()
		mb.fill(sub, e.Children)
		gtkMenuItemSetSubmenu(item, sub)
		return item
	}
	if e.Accel != "" {
		var key, mods uint32
		gtkAcceleratorParse(e.Accel, &key, &mods)
		if key != 0 {
			if e.AccelShow || mb.accel == 0 {
				if label := gtkBinGetChild(item); label != 0 {
					gtkAccelLabelSetAccel(label, key, mods)
				}
			} else {
				gtkWidgetAddAccelerator(item, "activate", mb.accel, key, mods, gtkAccelVisible)
			}
		}
	}
	if e.Action != actNone && mb.activate != nil {
		e, act := e, mb.activate
		connect(item, "activate", 0, func(_, _ ptr) ptr {
			act(e)
			return 0
		})
	}
	return item
}
