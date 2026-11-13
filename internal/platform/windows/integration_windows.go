//go:build windows && (amd64 || arm64)

package windows

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Desktop integration (contract round 2): displays, system theme, shell,
// taskbar control, and the WM_SETTINGCHANGE / WM_DISPLAYCHANGE handling
// behind AppThemeChanged and AppDisplaysChanged. These broadcasts reach the
// hidden top-level application window (it is not a message-only window,
// which would not receive them). Identifiers carry an "ix" prefix so they
// cannot collide with other files of the package.

var (
	_ platform.Displays    = (*Backend)(nil)
	_ platform.SystemTheme = (*Backend)(nil)
	_ platform.Shell       = (*Backend)(nil)
	_ platform.AppControl  = (*Backend)(nil)
	_ platform.WebViewInfo = (*Backend)(nil)
)

var (
	ixDwmapi = systemDLL("dwmapi.dll")

	ixEnumDisplayMonitors       = user32.NewProc("EnumDisplayMonitors")
	ixEnumDisplayDevicesW       = user32.NewProc("EnumDisplayDevicesW")
	ixIsWindowVisible           = user32.NewProc("IsWindowVisible")
	ixFlashWindowEx             = user32.NewProc("FlashWindowEx")
	ixRegGetValueW              = advapi32.NewProc("RegGetValueW")
	ixShellExecuteW             = shell32.NewProc("ShellExecuteW")
	ixILCreateFromPathW         = shell32.NewProc("ILCreateFromPathW")
	ixILFree                    = shell32.NewProc("ILFree")
	ixSHOpenFolderAndSelect     = shell32.NewProc("SHOpenFolderAndSelectItems")
	ixDwmGetColorizationColor   = ixDwmapi.NewProc("DwmGetColorizationColor")
	ixEnumMonitorsCallback      uintptr
	ixEnumMonitorsCallbackReady bool

	clsidTaskbarList  = guid("56FDF344-FD6D-11d0-958A-006097C9A090")
	iidITaskbarList3  = guid("ea1afb91-9e28-4b86-90e9-9e9f8a5eefaf")
	errShellExecute   = errors.New("tango: ShellExecuteW failed")
	errNoTaskbarList3 = errors.New("tango: ITaskbarList3 is unavailable")
)

const (
	wmSettingChange               = 0x001A
	wmDisplayChange               = 0x007E
	wmDwmColorizationColorChanged = 0x0320

	ixMonitorInfoFPrimary = 1
	ixRRFRtRegDword       = 0x00000010

	ixFlashwTray      = 0x2
	ixFlashwAll       = 0x3
	ixFlashwTimerNoFG = 0xC

	// ITaskbarList3 vtable slots (IUnknown 0-2, ITaskbarList 3-7,
	// ITaskbarList2 8).
	ixTblHrInit           = 3
	ixTblSetProgressValue = 9
	ixTblSetProgressState = 10
	ixTblSetOverlayIcon   = 18

	ixTBPFNoProgress = 0
	ixTBPFNormal     = 2

	ixProgressTotal = 10000
)

type ixMonitorInfoEx struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
	Device  [32]uint16
}

type ixDisplayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type ixFlashInfo struct {
	Size    uint32
	HWnd    uintptr
	Flags   uint32
	Count   uint32
	Timeout uint32
}

// ixState is main-thread state; one backend per process.
var ixState struct {
	inited       bool
	lastTheme    platform.Theme
	lastDisplays []platform.Display
	monitors     []uintptr // filled by the EnumDisplayMonitors callback

	taskbar  uintptr // ITaskbarList3, created on first use
	progress float64 // negative: none
	badge    string
	overlay  uintptr // HICON currently set as overlay
	// buttonMsg is the registered "TaskbarButtonCreated" message: a
	// window's taskbar button exists (again) and takes progress / overlay.
	buttonMsg uint32
	hidden    []platform.SurfaceID
}

// initSystemState records the starting theme and displays so the first
// WM_SETTINGCHANGE does not report a change that did not happen. Called
// from Init.
func (b *Backend) initSystemState() {
	ixState.progress = -1
	if r, _, _ := mtRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarButtonCreated")))); r != 0 {
		ixState.buttonMsg = uint32(r)
	}
	ixState.lastTheme = b.SystemTheme()
	ixState.lastDisplays = b.Displays()
	initMotion()   // motion_windows.go
	initContrast() // contrast_windows.go
	ixState.inited = true
}

// systemMessage handles the broadcasts that may mean a theme or display
// change. Both checks compare with the last state, so redundant messages
// (WM_SETTINGCHANGE comes for many reasons) report nothing.
func (b *Backend) systemMessage(m uint32, wp, lp uintptr) {
	if !ixState.inited {
		return
	}
	switch m {
	case wmSettingChange:
		// "ImmersiveColorSet" announces light/dark and accent changes;
		// SPI_SETWORKAREA (wp 0x2F) a taskbar move or resize.
		if lp != 0 && wstr(lp) == "ImmersiveColorSet" {
			b.themeMaybeChanged()
		}
		if lp != 0 {
			localeSettingChanged(wstr(lp)) // locale_watch_windows.go: "intl"
		}
		b.motionMaybeChanged()   // motion_windows.go: reduce motion
		b.contrastMaybeChanged() // contrast_windows.go: high contrast
		b.displaysMaybeChanged()
	case wmDwmColorizationColorChanged:
		b.themeMaybeChanged()
	case wmDisplayChange:
		b.displaysMaybeChanged()
	}
}

func (b *Backend) themeMaybeChanged() {
	t := b.SystemTheme()
	if t == ixState.lastTheme {
		return
	}
	ixState.lastTheme = t
	if hx, ok := b.h.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppThemeChanged, Theme: t})
	}
}

func (b *Backend) displaysMaybeChanged() {
	ds := b.Displays()
	if slices.Equal(ds, ixState.lastDisplays) {
		return
	}
	ixState.lastDisplays = ds
	if hx, ok := b.h.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppDisplaysChanged})
	}
}

// ---- platform.Displays ----

// Displays enumerates monitors. Each monitor's rectangles are converted to
// logical pixels with that monitor's DPI, as Geometry converts window
// positions with the window's DPI; with mixed scales the logical layout
// can therefore have gaps or overlaps, which At-style lookups tolerate.
func (b *Backend) Displays() []platform.Display {
	if !ixEnumMonitorsCallbackReady {
		ixEnumMonitorsCallback = syscall.NewCallback(func(mon, hdc, r, lp uintptr) uintptr {
			ixState.monitors = append(ixState.monitors, mon)
			return 1
		})
		ixEnumMonitorsCallbackReady = true
	}
	ixState.monitors = ixState.monitors[:0]
	ixEnumDisplayMonitors.Call(0, 0, ixEnumMonitorsCallback, 0)
	var out []platform.Display
	for _, mon := range ixState.monitors {
		mi := ixMonitorInfoEx{}
		mi.Size = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
			continue
		}
		dpi := baseDPI
		if has(mtGetDpiForMonitor) {
			var dx, dy uint32
			if hr, _, _ := mtGetDpiForMonitor.Call(mon, mtMDTEffectiveDPI, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); hr == sOK && dx > 0 {
				dpi = int(dx)
			}
		} else {
			dpi = dpiOf(0)
		}
		dev := syscall.UTF16ToString(mi.Device[:])
		out = append(out, platform.Display{
			ID:       dev,
			Name:     ixMonitorName(dev),
			Bounds:   ixLogical(mi.Monitor, dpi),
			WorkArea: ixLogical(mi.Work, dpi),
			Scale:    float64(dpi) / baseDPI,
			Primary:  mi.Flags&ixMonitorInfoFPrimary != 0,
		})
	}
	return out
}

func ixLogical(r rect, dpi int) platform.Rect {
	x, y := pxToDIP(int(r.Left), dpi), pxToDIP(int(r.Top), dpi)
	return platform.Rect{X: x, Y: y, Width: pxToDIP(int(r.Right), dpi) - x, Height: pxToDIP(int(r.Bottom), dpi) - y}
}

// ixMonitorName returns the monitor's description ("Generic PnP Monitor",
// or the model when the driver provides it).
func ixMonitorName(device string) string {
	dd := ixDisplayDevice{}
	dd.Cb = uint32(unsafe.Sizeof(dd))
	if r, _, _ := ixEnumDisplayDevicesW.Call(uintptr(unsafe.Pointer(u16(device))), 0, uintptr(unsafe.Pointer(&dd)), 0); r == 0 {
		return device
	}
	return syscall.UTF16ToString(dd.DeviceString[:])
}

// ---- platform.SystemTheme ----

func ixRegDword(path, name string) (uint32, bool) {
	var v, size uint32 = 0, 4
	r, _, _ := ixRegGetValueW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(u16(name))),
		ixRRFRtRegDword, 0, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&size)))
	return v, r == 0
}

// SystemTheme reads the "apps" light/dark choice (Settings > Personalization
// > Colors) and the accent color.
func (b *Backend) SystemTheme() platform.Theme {
	var t platform.Theme
	if v, ok := ixRegDword(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme"); ok {
		t.Dark = v == 0
	}
	if v, ok := ixRegDword(`Software\Microsoft\Windows\DWM`, "AccentColor"); ok {
		t.Accent = accentFromDWM(v)
	} else if has(ixDwmGetColorizationColor) {
		var c uint32
		var opaque int32
		if hr, _, _ := ixDwmGetColorizationColor.Call(uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&opaque))); hr == sOK {
			t.Accent = accentFromColorization(c)
		}
	}
	return t
}

// ---- platform.Shell ----

func ixShellExecute(verb, file, params string) error {
	var p *uint16
	if params != "" {
		p = u16(params)
	}
	r, _, _ := ixShellExecuteW.Call(b2h(), uintptr(unsafe.Pointer(u16(verb))), uintptr(unsafe.Pointer(u16(file))), uintptr(unsafe.Pointer(p)), 0, swShowNormal)
	runtime.KeepAlive(p)
	if r <= 32 { // HINSTANCE > 32 means success
		return errShellExecute
	}
	return nil
}

// b2h is the owner window for shell UI (error dialogs); 0 if none.
func b2h() uintptr {
	if b := theBackend; b != nil {
		return b.appHwnd.Load()
	}
	return 0
}

func (b *Backend) OpenURL(u string) error     { return ixShellExecute("open", u, "") }
func (b *Backend) OpenPath(path string) error { return ixShellExecute("open", path, "") }

// RevealPath selects path in an Explorer window with
// SHOpenFolderAndSelectItems; if that fails it starts explorer.exe
// /select, which does the same less reliably.
func (b *Backend) RevealPath(path string) error {
	path = filepath.Clean(path)
	if has(ixILCreateFromPathW) && has(ixSHOpenFolderAndSelect) {
		pidl, _, _ := ixILCreateFromPathW.Call(uintptr(unsafe.Pointer(u16(path))))
		if pidl != 0 {
			hr, _, _ := ixSHOpenFolderAndSelect.Call(pidl, 0, 0, 0)
			ixILFree.Call(pidl)
			if !failed(hr) {
				return nil
			}
		}
	}
	return ixShellExecute("open", filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), `/select,"`+strings.ReplaceAll(path, `"`, "")+`"`)
}

// ---- platform.AppControl ----

func (b *Backend) liveSurfaces() []*surface {
	out := make([]*surface, 0, len(b.byID))
	for _, s := range b.byID {
		if !s.closed && s.hwnd != 0 {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(x, y *surface) int { return int(x.id) - int(y.id) })
	return out
}

func (b *Backend) HideApp() {
	ixState.hidden = ixState.hidden[:0]
	for _, s := range b.liveSurfaces() {
		if v, _, _ := ixIsWindowVisible.Call(s.hwnd); v != 0 {
			ixState.hidden = append(ixState.hidden, s.id)
			s.Hide()
		}
	}
}

func (b *Backend) ShowApp() {
	var first *surface
	for _, id := range ixState.hidden {
		if s := b.byID[id]; s != nil && !s.closed {
			s.Show()
			if first == nil {
				first = s
			}
		}
	}
	ixState.hidden = ixState.hidden[:0]
	if first == nil {
		if ss := b.liveSurfaces(); len(ss) > 0 {
			first = ss[0]
		}
	}
	if first != nil {
		procSetForegroundWindow.Call(first.hwnd)
	}
}

// AppControlOps reports what the taskbar can do: no dock to hide, and
// badges are counts drawn as an overlay icon.
func (b *Backend) AppControlOps() platform.AppControlOp {
	return platform.CtlAll &^ (platform.CtlDock | platform.CtlBadgeText)
}

func (b *Backend) SetDockVisible(bool) {}

// taskbarList returns the shared ITaskbarList3, creating it once.
func (b *Backend) taskbarList() (uintptr, error) {
	if ixState.taskbar != 0 {
		return ixState.taskbar, nil
	}
	var tl uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidTaskbarList)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidITaskbarList3)), uintptr(unsafe.Pointer(&tl)))
	if failed(hr) || tl == 0 {
		return 0, errNoTaskbarList3
	}
	if failed(comCall(tl, ixTblHrInit)) {
		release(tl)
		return 0, errNoTaskbarList3
	}
	ixState.taskbar = tl
	return tl, nil
}

// SetProgress sets the taskbar progress of every window (each has its own
// taskbar button unless grouped). Windows created later get it when their
// button appears (taskbarButtonCreated).
func (b *Backend) SetProgress(p float64) {
	ixState.progress = p
	tl, err := b.taskbarList()
	if err != nil {
		return
	}
	for _, s := range b.liveSurfaces() {
		ixSetProgress(tl, s.hwnd, p)
	}
}

func ixSetProgress(tl, hwnd uintptr, p float64) {
	if p < 0 {
		comCall(tl, ixTblSetProgressState, hwnd, ixTBPFNoProgress)
		return
	}
	comCall(tl, ixTblSetProgressState, hwnd, ixTBPFNormal)
	comCall(tl, ixTblSetProgressValue, hwnd, uintptr(min(p, 1)*ixProgressTotal), ixProgressTotal)
}

// taskbarButtonCreated re-applies the current progress and badge to hwnd
// once the taskbar has made its button (ITaskbarList3 calls made before
// that are lost): windows created after SetBadge / SetProgress, or all of
// them again after Explorer restarts.
func (b *Backend) taskbarButtonCreated(hwnd uintptr) {
	if ixState.progress < 0 && ixState.overlay == 0 {
		return
	}
	tl, err := b.taskbarList()
	if err != nil {
		return
	}
	if ixState.progress >= 0 {
		ixSetProgress(tl, hwnd, ixState.progress)
	}
	if ixState.overlay != 0 {
		desc := u16(ixState.badge)
		comCall(tl, ixTblSetOverlayIcon, hwnd, ixState.overlay, uintptr(unsafe.Pointer(desc)))
		runtime.KeepAlive(desc)
	}
}

// SetBadge draws a count on a red disc and sets it as the overlay icon of
// every window's taskbar button (windows created later get it from
// taskbarButtonCreated). Non-count text is refused by package appctl
// (AppControlOps has no CtlBadgeText); here it clears the badge.
func (b *Backend) SetBadge(text string) {
	text = badgeText(text)
	ixState.badge = text
	tl, err := b.taskbarList()
	if err != nil {
		return
	}
	var icon uintptr
	if text != "" {
		size := smallIconSize(dpiOf(b2h()))
		pix := badgePixels(text, size)
		if color := createDIB32(pix, size, size); color != 0 {
			maskBits := iconMask(pix, size, size)
			mask, _, _ := mtCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
			if mask != 0 {
				ii := mtIconInfo{FIcon: 1, HbmMask: mask, HbmColor: color}
				icon, _, _ = mtCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
				mtDeleteObject.Call(mask)
			}
			mtDeleteObject.Call(color)
		}
	}
	var desc *uint16
	if icon != 0 {
		desc = u16(text) // accessibility description
	}
	for _, s := range b.liveSurfaces() {
		comCall(tl, ixTblSetOverlayIcon, s.hwnd, icon, uintptr(unsafe.Pointer(desc)))
	}
	runtime.KeepAlive(desc)
	// The taskbar keeps its own copy of the icon.
	if ixState.overlay != 0 {
		mtDestroyIcon.Call(ixState.overlay)
	}
	ixState.overlay = icon
}

// RequestAttention flashes the taskbar button of the app's first visible
// window: three times, or until the app comes to the front if critical.
func (b *Backend) RequestAttention(critical bool) {
	for _, s := range b.liveSurfaces() {
		if v, _, _ := ixIsWindowVisible.Call(s.hwnd); v == 0 {
			continue
		}
		fi := ixFlashInfo{HWnd: s.hwnd, Flags: ixFlashwTray, Count: 3}
		if critical {
			fi.Flags, fi.Count = ixFlashwAll|ixFlashwTimerNoFG, 0
		}
		fi.Size = uint32(unsafe.Sizeof(fi))
		ixFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
		return
	}
}

// WebViewVersion reports the WebView2 Runtime the loader would pick
// (platform.WebViewInfo).
func (b *Backend) WebViewVersion() (engine, version string) {
	dir, _, err := findWebView2(os.Getenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER"), runtime.GOARCH, regString)
	if err == nil {
		version = filepath.Base(dir)
	}
	return "WebView2", version
}
