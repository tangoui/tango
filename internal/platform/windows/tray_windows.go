//go:build windows && (amd64 || arm64)

package windows

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Notification-area (system tray) icons with Shell_NotifyIconW, version 4
// semantics. All trays share one hidden top-level window of their own class
// (a message-only window would miss the TaskbarCreated broadcast sent when
// Explorer restarts, after which every icon must be added again).
//
// Mouse and keyboard, as Windows users expect:
//   - left click (or Space/Enter on the focused icon): OnClick;
//   - double click: OnDoubleClick (preceded by OnClick for the first click);
//   - right click (or Shift+F10 / the menu key): the menu if one is set,
//     otherwise OnRightClick. OnRightClick is not called when a menu opens.
//
// Version 4 reports a right click as WM_RBUTTONUP followed by
// WM_CONTEXTMENU; only WM_CONTEXTMENU is handled, so it fires once.
//
// TaskbarCreated re-adding and SetForegroundWindow + WM_NULL around the
// menu are the documented requirements (Microsoft Learn: Shell_NotifyIconW,
// "Taskbar Creation Notification"; TrackPopupMenu remarks).

var _ platform.Trays = (*Backend)(nil)

var (
	mtShellNotifyIconW          = shell32.NewProc("Shell_NotifyIconW")
	mtShellNotifyIconGetRect    = shell32.NewProc("Shell_NotifyIconGetRect")
	mtRegisterWindowMessageW    = user32.NewProc("RegisterWindowMessageW")
	mtChangeWindowMessageFilter = user32.NewProc("ChangeWindowMessageFilterEx")
)

const (
	mtNIMAdd        = 0
	mtNIMModify     = 1
	mtNIMDelete     = 2
	mtNIMSetVersion = 4

	mtNIFMessage = 0x01
	mtNIFIcon    = 0x02
	mtNIFTip     = 0x04
	mtNIFShowTip = 0x80

	mtNotifyIconVersion4 = 4

	// Events in LOWORD(lParam) of the callback message (version 4).
	mtEvContextMenu   = 0x007B // WM_CONTEXTMENU
	mtEvLButtonUp     = 0x0202 // WM_LBUTTONUP
	mtEvLButtonDblClk = 0x0203 // WM_LBUTTONDBLCLK
	mtEvKeySelect     = 0x0401 // NIN_KEYSELECT

	mtTrayCallback = wmApp + 0x40 // private to the tray window class
	mtWSPopup      = 0x80000000
	mtMsgFltAllow  = 1
	mtIDIApp       = 32512 // IDI_APPLICATION
	mtMaxTip       = 128

	mtMonitorDefaultToNearest = 2

	trayClass = "TangoTray"
)

// mtNotifyIconData is NOTIFYICONDATAW.
type mtNotifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [mtMaxTip]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32 // union with uTimeout
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        GUID
	BalloonIcon     uintptr
}

// mtNotifyIconID is NOTIFYICONIDENTIFIER.
type mtNotifyIconID struct {
	Size     uint32
	Wnd      uintptr
	ID       uint32
	GUIDItem GUID
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(mtNotifyIconData{})-976]
	_ = [1]struct{}{}[unsafe.Sizeof(mtNotifyIconID{})-40]
)

// trayHost is the shared window and the live trays (main thread only).
var trayHost struct {
	hwnd           uintptr
	taskbarCreated uint32
	byID           map[uint32]*tray
	nextID         uint32
}

var (
	trayClassOnce sync.Once
	trayClassErr  error
)

func ensureTrayWindow() error {
	if trayHost.hwnd != 0 {
		return nil
	}
	trayClassOnce.Do(func() {
		wc := wndClassEx{
			WndProc:   syscall.NewCallback(trayWndProc),
			Instance:  instance(),
			ClassName: u16(trayClass),
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			trayClassErr = fmt.Errorf("tango: RegisterClassExW(%s): %v", trayClass, err)
		}
	})
	if trayClassErr != nil {
		return trayClassErr
	}
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(trayClass))), uintptr(unsafe.Pointer(u16(""))),
		mtWSPopup, 0, 0, 0, 0, 0, 0, instance(), 0)
	if hwnd == 0 {
		return fmt.Errorf("tango: cannot create the tray window: %v", err)
	}
	trayHost.hwnd = hwnd
	trayHost.byID = map[uint32]*tray{}
	r, _, _ := mtRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	trayHost.taskbarCreated = uint32(r)
	// An elevated process does not get the broadcast from a non-elevated
	// Explorer unless it lets the message through.
	if r != 0 && has(mtChangeWindowMessageFilter) {
		mtChangeWindowMessageFilter.Call(hwnd, r, mtMsgFltAllow, 0)
	}
	return nil
}

func trayWndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch msg := uint32(m); {
	case msg == mtTrayCallback:
		if t := trayHost.byID[uint32(hiword(lp))]; t != nil {
			t.notify(loword(lp), wp)
		}
		return 0
	case trayHost.taskbarCreated != 0 && msg == trayHost.taskbarCreated:
		for _, t := range trayHost.byID {
			t.added = false
			t.refreshIcon() // the DPI may have changed too
			t.add()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// removeAllTrays deletes every icon; Windows otherwise leaves them in the
// notification area until the mouse passes over them.
func removeAllTrays() {
	for _, t := range trayHost.byID {
		t.Remove()
	}
}

type tray struct {
	b       *Backend
	id      uint32
	opts    platform.TrayOptions
	h       platform.TrayHandlers
	plan    *menuPlan // nil: no menu
	icon    uintptr
	ownIcon bool // icon was created here and must be destroyed
	added   bool // known to the shell
	removed bool
}

func (b *Backend) NewTray(o platform.TrayOptions, h platform.TrayHandlers) (platform.Tray, error) {
	if b.h == nil {
		return nil, errors.New("tango: NewTray before Init")
	}
	if err := ensureTrayWindow(); err != nil {
		return nil, err
	}
	t := &tray{b: b, opts: o, h: h}
	if err := t.setIcon(o.Icon); err != nil {
		return nil, err
	}
	trayHost.nextID++
	t.id = trayHost.nextID
	trayHost.byID[t.id] = t
	// When the shell is not running yet, NIM_ADD fails; TaskbarCreated
	// adds the icon once it is.
	t.add()
	return t, nil
}

// setIcon replaces the HICON with one made from png (the application
// icon when empty). The old icon is destroyed after the shell has the
// new one (the caller's NIM_MODIFY), see SetOptions.
func (t *tray) setIcon(png []byte) error {
	var icon uintptr
	own := false
	if len(png) > 0 {
		h, err := createIcon(png, smallIconSize(dpiOf(0)))
		if err != nil {
			return fmt.Errorf("tango: tray icon: %w", err)
		}
		icon, own = h, true
	} else {
		icon, _, _ = procLoadIconW.Call(instance(), 1) // shared: never destroyed
		if icon == 0 {
			icon, _, _ = procLoadIconW.Call(0, mtIDIApp)
		}
	}
	t.icon, t.ownIcon = icon, own
	return nil
}

func (t *tray) refreshIcon() {
	if !t.ownIcon {
		return
	}
	old := t.icon
	if err := t.setIcon(t.opts.Icon); err != nil {
		return // keeps the old icon
	}
	mtDestroyIcon.Call(old)
}

func (t *tray) data(flags uint32) mtNotifyIconData {
	nid := mtNotifyIconData{Wnd: trayHost.hwnd, ID: t.id, Flags: flags,
		CallbackMessage: mtTrayCallback, Icon: t.icon}
	nid.Size = uint32(unsafe.Sizeof(nid))
	tip := utf16z(t.opts.Tooltip)
	n := len(tip) - 1
	if n > mtMaxTip-1 {
		n = mtMaxTip - 1
		if tip[n-1] >= 0xD800 && tip[n-1] < 0xDC00 { // no lone high surrogate
			n--
		}
	}
	copy(nid.Tip[:n], tip[:n])
	return nid
}

func shellNotify(op uintptr, nid *mtNotifyIconData) bool {
	r, _, _ := mtShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(nid)))
	return r != 0
}

func (t *tray) add() {
	if t.removed || t.added {
		return
	}
	nid := t.data(mtNIFMessage | mtNIFIcon | mtNIFTip | mtNIFShowTip)
	if !shellNotify(mtNIMAdd, &nid) {
		return
	}
	t.added = true
	nid.Version = mtNotifyIconVersion4
	shellNotify(mtNIMSetVersion, &nid)
}

func (t *tray) update() {
	if !t.added {
		t.add()
		return
	}
	nid := t.data(mtNIFMessage | mtNIFIcon | mtNIFTip | mtNIFShowTip)
	shellNotify(mtNIMModify, &nid)
}

// notify handles a callback-message event; anchor is wParam, the event's
// screen position in physical px (version 4). The secondary click opens
// the menu whatever MenuOn says (TrayMenuDefault and TrayMenuSecondary are
// the same on Windows); TrayMenuPrimary makes the primary click open it
// too, instead of OnClick.
func (t *tray) notify(ev uint16, anchor uintptr) {
	switch ev {
	case mtEvLButtonUp, mtEvKeySelect:
		if t.plan != nil && t.opts.MenuOn == platform.TrayMenuPrimary {
			t.showMenu(int32(int16(loword(anchor))), int32(int16(hiword(anchor))))
		} else if t.h.OnClick != nil {
			t.h.OnClick()
		}
	case mtEvLButtonDblClk:
		if t.h.OnDoubleClick != nil {
			t.h.OnDoubleClick()
		}
	case mtEvContextMenu:
		if t.plan != nil {
			t.showMenu(int32(int16(loword(anchor))), int32(int16(hiword(anchor))))
		} else if t.h.OnRightClick != nil {
			t.h.OnRightClick()
		}
	}
}

func (t *tray) showMenu(x, y int32) {
	if menuTracking {
		return
	}
	if x == 0 && y == 0 {
		var pt point
		if r, _, _ := mtGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r != 0 {
			x, y = pt.X, pt.Y
		}
	}
	// The core may call SetMenu or Remove while the menu is open (Drain
	// runs inside the menu's modal loop): use this plan throughout.
	plan := t.plan
	hmenu, _, _ := procCreatePopupMenu.Call()
	if hmenu == 0 {
		return
	}
	appendMenuEntries(hmenu, hmenu, plan.Entries, dpiOf(0))
	cmd := trackMenu(hmenu, trayHost.hwnd, x, y)
	destroyMenu(hmenu)
	act, ok := plan.Actions[cmd]
	if cmd == 0 || !ok || t.removed {
		return
	}
	switch act.Role {
	case "":
		if act.ID != 0 && t.h.OnMenu != nil {
			t.h.OnMenu(act.ID)
		}
	case "quit":
		t.b.userQuit()
	case "about":
		name := t.b.opts.Name
		if name == "" {
			name = "Tango"
		}
		t.b.runDeferred(func() {
			procMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(name))),
				uintptr(unsafe.Pointer(u16("About "+name))), mbIconInformation)
		})
	}
	// Window and editing roles have no target in a tray menu.
}

// platform.Tray

// SetOptions applies Icon, Tooltip and MenuOn (read at each click);
// Template and Title do not exist on Windows. An icon that fails to decode keeps the previous one.
func (t *tray) SetOptions(o platform.TrayOptions) {
	if t.removed {
		return
	}
	oldIcon, oldOwn := t.icon, t.ownIcon
	iconChanged := !bytes.Equal(o.Icon, t.opts.Icon)
	if iconChanged {
		if err := t.setIcon(o.Icon); err != nil {
			log.Print(err)
			o.Icon = t.opts.Icon
			iconChanged = false
		}
	}
	t.opts = o
	t.update()
	if iconChanged && oldOwn {
		mtDestroyIcon.Call(oldIcon)
	}
}

func (t *tray) SetMenu(items []platform.MenuItem) {
	if len(items) == 0 {
		t.plan = nil
		return
	}
	t.plan = buildMenuPlan(items, t.b.opts.Name)
}

// Bounds is the icon's rectangle in logical px, converted at the DPI of
// the monitor it is on. It fails while the icon is hidden in the overflow
// area on some Windows versions.
func (t *tray) Bounds() (platform.Rect, bool) {
	if t.removed || !t.added || !has(mtShellNotifyIconGetRect) {
		return platform.Rect{}, false
	}
	id := mtNotifyIconID{Wnd: trayHost.hwnd, ID: t.id}
	id.Size = uint32(unsafe.Sizeof(id))
	var r rect
	if hr, _, _ := mtShellNotifyIconGetRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&r))); hr != sOK {
		return platform.Rect{}, false
	}
	dpi := dpiOf(0)
	if has(mtGetDpiForMonitor) {
		if mon, _, _ := mtMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), mtMonitorDefaultToNearest); mon != 0 {
			var dx, dy uint32
			if hr, _, _ := mtGetDpiForMonitor.Call(mon, mtMDTEffectiveDPI, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); hr == sOK && dx != 0 {
				dpi = int(dx)
			}
		}
	}
	return platform.Rect{
		X: pxToDIP(int(r.Left), dpi), Y: pxToDIP(int(r.Top), dpi),
		Width: pxToDIP(int(r.Right-r.Left), dpi), Height: pxToDIP(int(r.Bottom-r.Top), dpi),
	}, true
}

func (t *tray) Remove() {
	if t.removed {
		return
	}
	if t.added {
		nid := t.data(0)
		shellNotify(mtNIMDelete, &nid)
	}
	t.removed, t.added, t.plan = true, false, nil
	if t.ownIcon {
		mtDestroyIcon.Call(t.icon)
	}
	t.icon, t.ownIcon = 0, false
	delete(trayHost.byID, t.id)
}
