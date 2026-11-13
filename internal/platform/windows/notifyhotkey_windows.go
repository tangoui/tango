//go:build windows && (amd64 || arm64)

package windows

// Round-2 system integration: global hotkeys (RegisterHotKey), balloon
// notifications (Shell_NotifyIconW) and rich clipboard formats (PNG,
// CF_DIBV5 / CF_DIB, CF_HDROP).
//
// Hotkeys and the notification icon talk to a hidden window of their own
// (class TangoSysWindow) owned by the main thread, so the backend's
// application window procedure stays untouched; its messages are
// dispatched by the backend's message loop like any other.
//
// Notifications: Notify shows a WinRT toast when Windows shows toasts for
// the app's AppUserModelID (sysx_toast_windows.go) and falls back to a
// balloon otherwise. Balloons work for any exe; Windows 10/11 renders them
// as toasts in the Action Center. Only the most recent balloon is
// clickable; the icon is removed once its balloon is gone.

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.Hotkeys       = (*Backend)(nil)
	_ platform.Notifier      = (*Backend)(nil)
	_ platform.ClipboardRich = (*Backend)(nil)
)

var (
	procRegisterHotKey           = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
	procShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	procDragQueryFileW           = shell32.NewProc("DragQueryFileW")
)

const (
	wmHotkey       = 0x0312
	wmSysNotifyMsg = wmApp + 0x40 // notification icon callback

	ninBalloonHide      = 0x0403
	ninBalloonTimeout   = 0x0404
	ninBalloonUserClick = 0x0405

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	notifyIconVersion4 = 4
	notifyIconUID      = 0x7A90

	cfDIB   = 8
	cfHDROP = 15
	cfDIBV5 = 17

	wsPopup     = 0x80000000
	idiApp      = 32512
	maxHotkeyID = 0xBFFF
)

// sys is the main-thread state of this file (one backend per process).
var sys = struct {
	once    sync.Once
	hwnd    uintptr
	err     error
	hotkeys map[uint32]sysHotkey // by native id
	byCore  map[uint32]uint32    // core id -> native id
	nextHK  uint32

	iconAdded bool
	icon      uintptr
	balloon   func() // click handler of the balloon showing
}{hotkeys: map[uint32]sysHotkey{}, byCore: map[uint32]uint32{}}

type sysHotkey struct {
	core uint32
	fire func()
}

var sysWndProcCallback uintptr

func (b *Backend) sysWindow() (uintptr, error) {
	sys.once.Do(func() {
		const class = "TangoSysWindow"
		sysWndProcCallback = syscall.NewCallback(sysWndProc)
		wc := wndClassEx{WndProc: sysWndProcCallback, Instance: instance(), ClassName: u16(class)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			sys.err = fmt.Errorf("tango: RegisterClassExW(%s): %v", class, err)
			return
		}
		hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16(class))), 0,
			wsPopup, 0, 0, 0, 0, 0, 0, instance(), 0)
		if hwnd == 0 {
			sys.err = fmt.Errorf("tango: cannot create the system window: %v", err)
			return
		}
		sys.hwnd = hwnd
		icon, _, _ := procLoadIconW.Call(instance(), 1) // first icon resource, if embedded
		if icon == 0 {
			icon, _, _ = procLoadIconW.Call(0, idiApp)
		}
		sys.icon = icon
	})
	return sys.hwnd, sys.err
}

func sysWndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch uint32(m) {
	case wmHotkey:
		if h, ok := sys.hotkeys[uint32(wp)]; ok && h.fire != nil {
			h.fire()
		}
		return 0
	case wmSysNotifyMsg:
		switch uint32(lp) & 0xFFFF { // NOTIFYICON_VERSION_4: LOWORD is the event
		case ninBalloonUserClick:
			fn := sys.balloon
			removeNotifyIcon()
			if fn != nil {
				fn()
			}
		case ninBalloonTimeout, ninBalloonHide:
			removeNotifyIcon()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// ---- Hotkeys ----

func (b *Backend) RegisterHotkey(id uint32, accelerator string, fire func()) error {
	mods, vk, err := hotkeyPlan(accelerator)
	if err != nil {
		return err
	}
	hwnd, err := b.sysWindow()
	if err != nil {
		return err
	}
	b.UnregisterHotkey(id)
	native := uint32(0)
	for range maxHotkeyID {
		sys.nextHK = sys.nextHK%maxHotkeyID + 1
		if _, used := sys.hotkeys[sys.nextHK]; !used {
			native = sys.nextHK
			break
		}
	}
	if native == 0 {
		return errors.New("tango: too many hotkeys")
	}
	if r, _, e := procRegisterHotKey.Call(hwnd, uintptr(native), uintptr(mods), uintptr(vk)); r == 0 {
		return fmt.Errorf("tango: cannot register hotkey %q (taken by another app or the system?): %v", accelerator, e)
	}
	sys.hotkeys[native] = sysHotkey{id, fire}
	sys.byCore[id] = native
	return nil
}

func (b *Backend) UnregisterHotkey(id uint32) {
	native, ok := sys.byCore[id]
	if !ok {
		return
	}
	procUnregisterHotKey.Call(sys.hwnd, uintptr(native))
	delete(sys.byCore, id)
	delete(sys.hotkeys, native)
}

// ---- Notifications ----

func (b *Backend) RequestNotifyPermission(done func(granted bool, err error)) {
	// Balloons need no permission; Focus Assist may still hide them.
	b.runDeferred(func() { done(true, nil) })
}

func sysShellNotify(op uintptr, d *notifyIconData) bool {
	r, _, _ := procShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(&d[0])))
	return r != 0
}

func removeNotifyIcon() {
	if !sys.iconAdded {
		return
	}
	sysShellNotify(nimDelete, newNotifyIconData(sys.hwnd, notifyIconUID, 0, 0, 0))
	sys.iconAdded, sys.balloon = false, nil
}

func (b *Backend) Notify(n platform.Notification, onClick func()) error {
	// A WinRT toast when the system shows them for this app; otherwise the
	// balloon below (sysx_toast_windows.go).
	if b.sxToast(n, onClick) == nil {
		return nil
	}
	hwnd, err := b.sysWindow()
	if err != nil {
		return err
	}
	if !sys.iconAdded {
		d := newNotifyIconData(hwnd, notifyIconUID, nifMessage|nifIcon|nifTip|nifShowTip, wmSysNotifyMsg, sys.icon)
		d.setTip(b.opts.Name)
		if !sysShellNotify(nimAdd, d) {
			return errors.New("tango: Shell_NotifyIconW(NIM_ADD) failed")
		}
		d.setVersion(notifyIconVersion4)
		sysShellNotify(nimSetVersion, d)
		sys.iconAdded = true
	}
	title := n.Title
	if title == "" {
		title = b.opts.Name
	}
	flags := uint32(niifUser)
	if n.Silent {
		flags |= niifNoSound
	}
	d := newNotifyIconData(hwnd, notifyIconUID, 0, 0, 0)
	d.setBalloon(title, balloonText(n.Subtitle, n.Body), flags)
	*(*uint64)(unsafe.Pointer(&d[nidOffBalloonIco])) = uint64(sys.icon)
	if !sysShellNotify(nimModify, d) {
		return errors.New("tango: Shell_NotifyIconW(NIM_MODIFY) failed")
	}
	sys.balloon = onClick
	return nil
}

// ---- Rich clipboard ----

var (
	pngFormatOnce sync.Once
	pngFormat     uintptr
)

func clipboardPNGFormat() uintptr {
	pngFormatOnce.Do(func() {
		pngFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(u16("PNG"))))
	})
	return pngFormat
}

func clipboardAvailable(format uintptr) bool {
	if format == 0 {
		return false
	}
	r, _, _ := procIsClipboardFormatAvailable.Call(format)
	return r != 0
}

// clipboardBytes copies the clipboard's data in format (clipboard open).
func clipboardBytes(format uintptr) ([]byte, error) {
	h, _, err := procGetClipboardData.Call(format)
	if h == 0 {
		return nil, fmt.Errorf("tango: GetClipboardData: %v", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return nil, fmt.Errorf("tango: GlobalLock: %v", err)
	}
	defer procGlobalUnlock.Call(h)
	size, _, _ := procGlobalSize.Call(h)
	return append([]byte(nil), unsafe.Slice((*byte)(native(p)), int(size))...), nil
}

func globalCopy(b []byte) (uintptr, error) {
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(b)))
	if h == 0 {
		return 0, fmt.Errorf("tango: GlobalAlloc: %v", err)
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return 0, fmt.Errorf("tango: GlobalLock: %v", err)
	}
	copy(unsafe.Slice((*byte)(native(p)), len(b)), b)
	procGlobalUnlock.Call(h)
	return h, nil
}

func (b *Backend) ClipboardReadImage() ([]byte, error) {
	if err := b.openClipboard(); err != nil {
		return nil, err
	}
	defer procCloseClipboard.Call()
	if f := clipboardPNGFormat(); clipboardAvailable(f) {
		if data, err := clipboardBytes(f); err == nil && len(data) > 8 {
			return data, nil
		}
	}
	for _, f := range []uintptr{cfDIBV5, cfDIB} {
		if !clipboardAvailable(f) {
			continue
		}
		dib, err := clipboardBytes(f)
		if err != nil {
			return nil, err
		}
		return dibToPNG(dib)
	}
	return nil, nil
}

func (b *Backend) ClipboardWriteImage(png []byte) error {
	dib, err := pngToDIBV5(png)
	if err != nil {
		return fmt.Errorf("tango: cannot decode the PNG image: %w", err)
	}
	hDIB, err := globalCopy(dib)
	if err != nil {
		return err
	}
	hPNG, err := globalCopy(png)
	if err != nil {
		procGlobalFree.Call(hDIB)
		return err
	}
	if err := b.openClipboard(); err != nil {
		procGlobalFree.Call(hDIB)
		procGlobalFree.Call(hPNG)
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if r, _, e := procSetClipboardData.Call(cfDIBV5, hDIB); r == 0 {
		procGlobalFree.Call(hDIB)
		procGlobalFree.Call(hPNG)
		return fmt.Errorf("tango: SetClipboardData(CF_DIBV5): %v", e)
	}
	if f := clipboardPNGFormat(); f != 0 {
		if r, _, _ := procSetClipboardData.Call(f, hPNG); r == 0 {
			procGlobalFree.Call(hPNG)
		}
	} else {
		procGlobalFree.Call(hPNG)
	}
	return nil
}

func (b *Backend) ClipboardReadFiles() ([]string, error) {
	if err := b.openClipboard(); err != nil {
		return nil, err
	}
	defer procCloseClipboard.Call()
	if !clipboardAvailable(cfHDROP) {
		return nil, nil
	}
	hdrop, _, err := procGetClipboardData.Call(cfHDROP)
	if hdrop == 0 {
		return nil, fmt.Errorf("tango: GetClipboardData(CF_HDROP): %v", err)
	}
	n, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	var paths []string
	for i := uintptr(0); i < n; i++ {
		l, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		buf := make([]uint16, l+1)
		procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), l+1)
		if p := utf16Units(buf); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

var (
	dropEffectFormat     uintptr
	dropEffectFormatOnce sync.Once
)

// ClipboardWriteFiles puts paths on the clipboard as CF_HDROP, marked
// "Preferred DropEffect" = DROPEFFECT_COPY so Explorer pastes a copy. An
// empty list just empties the clipboard.
func (b *Backend) ClipboardWriteFiles(paths []string) error {
	var hDrop, hEffect uintptr
	if len(paths) > 0 {
		data, err := dropFiles(paths)
		if err != nil {
			return err
		}
		if hDrop, err = globalCopy(data); err != nil {
			return err
		}
		if hEffect, err = globalCopy([]byte{1, 0, 0, 0}); err != nil { // DROPEFFECT_COPY
			procGlobalFree.Call(hDrop)
			return err
		}
	}
	free := func() {
		if hDrop != 0 {
			procGlobalFree.Call(hDrop)
			procGlobalFree.Call(hEffect)
		}
	}
	if err := b.openClipboard(); err != nil {
		free()
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if hDrop == 0 {
		return nil
	}
	if r, _, e := procSetClipboardData.Call(cfHDROP, hDrop); r == 0 {
		free()
		return fmt.Errorf("tango: SetClipboardData(CF_HDROP): %v", e)
	}
	dropEffectFormatOnce.Do(func() {
		dropEffectFormat, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(u16("Preferred DropEffect"))))
	})
	if dropEffectFormat == 0 {
		procGlobalFree.Call(hEffect)
	} else if r, _, _ := procSetClipboardData.Call(dropEffectFormat, hEffect); r == 0 {
		procGlobalFree.Call(hEffect)
	}
	return nil
}
