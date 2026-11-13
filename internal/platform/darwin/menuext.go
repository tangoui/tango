//go:build darwin && !ios

package darwin

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// Context menus (NSMenu popUpMenuPositioningItem:atLocation:inView:) and
// tray icons (NSStatusItem), plus the per-item extras of the shared menu
// builder (icons).
//
// Popup and tray menu items use the same TangoMenuTarget as the menu bar;
// their ids are routed here first (routeMenuClick). Ids are allocated by
// the core and unique across all menus, so one table serves every menu.

var (
	_ platform.PopupMenu = (*backend)(nil)
	_ platform.Trays     = (*backend)(nil)
)

const (
	nsVariableStatusItemLength = -1.0
	nsEventMaskLeftMouseUp     = 1 << 2
	nsEventMaskRightMouseUp    = 1 << 4
	nsEventTypeRightMouseUp    = 4
	nsEventModifierFlagControl = 1 << 18
	nsImageLeft                = 2
	nsImageOnly                = 1

	menuIconPoints = 16.0
	trayIconPoints = 18.0
)

var (
	extOnce          sync.Once
	msgPopUpMenu     func(menu id, s objc.SEL, item id, loc nsPoint, view id) bool
	msgStatusItemLen func(bar id, s objc.SEL, length float64) id

	// Main thread only.
	menuRoutes     = map[uint32]func(id uint32){}
	menuRouteState = map[uint32]bool{} // enabled state of routed items
	trayByButton   = map[id]*statusTray{}
	openPopup      id // the NSMenu being tracked by PopupMenu, if any
)

func loadMenuExt() {
	extOnce.Do(func() {
		purego.RegisterFunc(&msgPopUpMenu, msgSendAddr)
		purego.RegisterFunc(&msgStatusItemLen, msgSendAddr)
		classDef("TangoTrayTarget", "NSObject", nil, []objc.MethodDef{
			method("tangoTrayClicked:", func(self id, _ objc.SEL, button id) {
				if t := trayByButton[button]; t != nil {
					t.clicked()
				}
			}),
		})
	})
}

// routeMenuClick runs the handler of a popup or tray item; false if tag is
// not one of theirs.
func routeMenuClick(tag uint32) bool {
	fn := menuRoutes[tag]
	if fn == nil {
		return false
	}
	fn(tag)
	return true
}

func extraMenuEnabled(tag uint32) (enabled, ok bool) {
	enabled, ok = menuRouteState[tag]
	return
}

// addRoutes sends every clickable id in items to fn and returns the ids.
func addRoutes(items []platform.MenuItem, fn func(uint32), ids []uint32) []uint32 {
	for _, it := range items {
		if len(it.Submenu) > 0 {
			ids = addRoutes(it.Submenu, fn, ids)
			continue
		}
		if it.ID != 0 && it.Role == "" && !it.Separator {
			menuRoutes[it.ID] = fn
			menuRouteState[it.ID] = !it.Disabled
			ids = append(ids, it.ID)
		}
	}
	return ids
}

func removeRoutes(ids []uint32) {
	for _, i := range ids {
		delete(menuRoutes, i)
		delete(menuRouteState, i)
	}
}

// nsImagePNG returns an autoreleased NSImage of png sized to height points
// (keeping the aspect ratio), or 0.
func nsImagePNG(png []byte, height float64) id {
	w, h, ok := iconPoints(png, height)
	if !ok {
		return 0
	}
	img := send(send(class("NSImage"), "alloc"), "initWithData:", uintptr(nsData(png)))
	if img == 0 {
		return 0
	}
	autorelease(img)
	msgSetSize(img, sel("setSize:"), nsSize{w, h})
	return img
}

func setItemIcon(item id, png []byte) {
	if len(png) == 0 {
		return
	}
	if img := nsImagePNG(png, menuIconPoints); img != 0 {
		send(item, "setImage:", uintptr(img)) // shown left of the title
	}
}

// ---------------------------------------------------------------------------
// PopupMenu

func (b *backend) PopupMenu(sid platform.SurfaceID, x, y int, items []platform.MenuItem, onClick func(uint32), onClose func()) {
	loadMenuExt()
	s := b.surfaces[sid]
	if s == nil || s.closed || s.view == 0 || openPopup != 0 {
		onClose()
		return
	}
	var chosen uint32
	ids := addRoutes(items, func(i uint32) { chosen = i }, nil)
	withPool(func() {
		menu := retain(b.buildMenu("", items))
		openPopup = menu
		view := s.view
		pt := nsPoint{float64(x), float64(y)}
		if flipped := sendBool(view, "isFlipped"); !flipped {
			pt.Y = msgRect(view, sel("bounds")).Size.Height - pt.Y
		}
		// Blocks in a nested tracking loop until the menu closes; the chosen
		// item's action is sent before it returns.
		msgPopUpMenu(menu, sel("popUpMenuPositioningItem:atLocation:inView:"), 0, pt, view)
		openPopup = 0
		release(menu)
	})
	removeRoutes(ids)
	if chosen != 0 {
		onClick(chosen)
	}
	onClose()
}

// SmokeClosePopup ends the open context menu, first choosing the item at
// index choose if choose >= 0. For the internal smoke test.
func (b *backend) SmokeClosePopup(choose int) bool {
	if openPopup == 0 {
		return false
	}
	if choose >= 0 {
		send(openPopup, "performActionForItemAtIndex:", uintptr(choose))
	}
	send(openPopup, "cancelTracking")
	return true
}

// ---------------------------------------------------------------------------
// Trays

type statusTray struct {
	b          *backend
	h          platform.TrayHandlers
	statusItem id // NSStatusItem, retained
	btn        id // its NSStatusBarButton
	menu       id // NSMenu, retained, or 0
	ids        []uint32
	gone       bool
	menuOn     platform.TrayMenuTrigger
}

func (b *backend) NewTray(o platform.TrayOptions, h platform.TrayHandlers) (platform.Tray, error) {
	loadMenuExt()
	t := &statusTray{b: b, h: h}
	withPool(func() {
		statusBar := send(class("NSStatusBar"), "systemStatusBar")
		t.statusItem = retain(msgStatusItemLen(statusBar, sel("statusItemWithLength:"), nsVariableStatusItemLength))
		t.btn = send(t.statusItem, "button")
		target := alloc("TangoTrayTarget") // lives as long as the tray; released in Remove
		send(t.btn, "setTarget:", uintptr(target))
		send(t.btn, "setAction:", uintptr(sel("tangoTrayClicked:")))
		send(t.btn, "sendActionOn:", nsEventMaskLeftMouseUp|nsEventMaskRightMouseUp)
		trayByButton[t.btn] = t
		t.apply(o)
	})
	return t, nil
}

func (t *statusTray) apply(o platform.TrayOptions) {
	if o.MenuOn != t.menuOn {
		t.menuOn = o.MenuOn
		t.attachMenu()
	}
	img := nsImagePNG(o.Icon, trayIconPoints)
	if img != 0 {
		send(img, "setTemplate:", boolArg(o.Template))
	}
	send(t.btn, "setImage:", uintptr(img)) // 0 clears
	send(t.btn, "setTitle:", uintptr(nsString(o.Title)))
	pos := nsImageLeft
	if o.Title == "" {
		pos = nsImageOnly
	}
	send(t.btn, "setImagePosition:", uintptr(pos))
	if o.Tooltip != "" {
		send(t.btn, "setToolTip:", uintptr(nsString(o.Tooltip)))
	} else {
		send(t.btn, "setToolTip:", 0)
	}
}

func (t *statusTray) SetOptions(o platform.TrayOptions) {
	if t.gone {
		return
	}
	withPool(func() { t.apply(o) })
}

// attachMenu gives the status item its menu, so AppKit opens it on any
// click (the click handlers are then not called) — except with
// TrayMenuSecondary, where clicked opens it for right clicks only.
func (t *statusTray) attachMenu() {
	m := t.menu
	if t.menuOn == platform.TrayMenuSecondary {
		m = 0
	}
	send(t.statusItem, "setMenu:", uintptr(m))
}

// SetMenu sets the menu; see attachMenu.
func (t *statusTray) SetMenu(items []platform.MenuItem) {
	if t.gone {
		return
	}
	removeRoutes(t.ids)
	t.ids = nil
	withPool(func() {
		old := t.menu
		t.menu = 0
		if items != nil {
			t.menu = retain(t.b.buildMenu("", items))
			t.ids = addRoutes(items, func(i uint32) {
				if t.h.OnMenu != nil {
					t.h.OnMenu(i)
				}
			}, nil)
		}
		t.attachMenu()
		release(old)
	})
}

func (t *statusTray) clicked() {
	ev := send(t.b.app, "currentEvent")
	right := false
	clicks := 1
	if ev != 0 {
		right = sendInt(ev, "type") == nsEventTypeRightMouseUp ||
			uint(sendInt(ev, "modifierFlags"))&nsEventModifierFlagControl != 0
		clicks = sendInt(ev, "clickCount")
	}
	switch {
	case right:
		if t.h.OnRightClick != nil {
			t.h.OnRightClick()
		}
		if t.menu != 0 && !t.gone {
			// TrayMenuSecondary: open the menu the way AppKit does for a
			// status item with a menu, then detach it again so primary
			// clicks keep reaching OnClick.
			withPool(func() {
				send(t.statusItem, "setMenu:", uintptr(t.menu))
				send(t.btn, "performClick:", 0)
				if !t.gone {
					send(t.statusItem, "setMenu:", 0)
				}
			})
		}
	case clicks == 2:
		if t.h.OnDoubleClick != nil {
			t.h.OnDoubleClick()
		}
	default:
		if t.h.OnClick != nil {
			t.h.OnClick()
		}
	}
}

func (t *statusTray) Bounds() (platform.Rect, bool) {
	if t.gone {
		return platform.Rect{}, false
	}
	var r platform.Rect
	ok := false
	withPool(func() {
		hostWin := send(t.btn, "window")
		displays := arrayItems(send(class("NSScreen"), "screens"))
		if hostWin == 0 || len(displays) == 0 {
			return
		}
		icon := msgRect(hostWin, sel("frame"))
		mainH := msgRect(displays[0], sel("frame")).Size.Height
		// Cocoa screen space is bottom-up from the primary display.
		r = platform.Rect{
			X:      int(icon.Origin.X),
			Y:      int(mainH - icon.Origin.Y - icon.Size.Height),
			Width:  int(icon.Size.Width),
			Height: int(icon.Size.Height),
		}
		// Before the menu bar has placed the item its window is not
		// visible yet and its frame is meaningless.
		ok = sendBool(hostWin, "isVisible") && icon.Size.Width > 0 && icon.Size.Height > 0
	})
	return r, ok
}

func (t *statusTray) Remove() {
	if t.gone {
		return
	}
	t.gone = true
	removeRoutes(t.ids)
	t.ids = nil
	delete(trayByButton, t.btn)
	withPool(func() {
		target := send(t.btn, "target")
		send(t.btn, "setTarget:", 0)
		statusBar := send(class("NSStatusBar"), "systemStatusBar")
		send(statusBar, "removeStatusItem:", uintptr(t.statusItem))
		release(t.menu)
		release(t.statusItem)
		release(target)
		t.menu, t.statusItem = 0, 0
	})
}

// SmokeTrayMenu returns, for every live tray, its menu items as
// "label|state|enabled". SmokeTrayChoose performs the action of item index
// of tray i's menu. For the internal smoke test.
func (b *backend) SmokeTrayMenu() [][]string {
	var out [][]string
	for _, t := range trayByButton {
		var row []string
		if t.menu != 0 {
			for _, it := range arrayItems(send(t.menu, "itemArray")) {
				st := "off"
				if sendInt(it, "state") == nsControlStateValueOn {
					st = "on"
				}
				en := "enabled"
				if !sendBool(it, "isEnabled") {
					en = "disabled"
				}
				img := ""
				if send(it, "image") != 0 {
					img = "|icon"
				}
				row = append(row, goString(send(it, "title"))+"|"+st+"|"+en+img)
			}
		}
		out = append(out, row)
	}
	return out
}

func (b *backend) SmokeTrayChoose(index int) bool {
	for _, t := range trayByButton {
		if t.menu != 0 {
			send(t.menu, "performActionForItemAtIndex:", uintptr(index))
			return true
		}
	}
	return false
}
