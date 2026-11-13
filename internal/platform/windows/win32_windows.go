//go:build windows && (amd64 || arm64)

package windows

import (
	"syscall"
	"unsafe"
)

// Win32 bindings used across the backend: the libraries, the functions
// declared against them (see dll_windows.go for the loader), message and
// style constants, the structures passed by pointer, and small wrappers.
// Names, values and layouts follow the Windows SDK headers named in each
// group.

// kernel32 is resolved by bare name: it is a KnownDLL, always mapped from
// the system directory. Everything else is opened by absolute path inside
// that directory, so a DLL planted next to the executable is never used.
var (
	kernel32 = newSysLib("kernel32.dll")

	procGetSystemDirectoryW = kernel32.export("GetSystemDirectoryW")
	sysDir                  = systemDirectory()

	user32   = newSysLib(inSystemDir("user32.dll"))
	ole32    = newSysLib(inSystemDir("ole32.dll"))
	shell32  = newSysLib(inSystemDir("shell32.dll"))
	shlwapi  = newSysLib(inSystemDir("shlwapi.dll"))
	advapi32 = newSysLib(inSystemDir("advapi32.dll"))
)

// systemDirectory asks Windows where System32 is, falling back to the
// usual location if the call fails or the answer does not fit MAX_PATH.
func systemDirectory() string {
	const maxPath = 260
	var buf [maxPath]uint16
	n, _, _ := procGetSystemDirectoryW.Call(uintptr(unsafe.Pointer(&buf[0])), maxPath)
	if n == 0 || n > maxPath {
		return `C:\Windows\System32`
	}
	return utf16Units(buf[:n])
}

// inSystemDir is the absolute path of a DLL shipped with Windows.
func inSystemDir(file string) string { return sysDir + `\` + file }

// systemDLL opens a further system library the syscall way; files that
// need *syscall.LazyProc values (sysx_network_windows.go) use it.
func systemDLL(name string) *syscall.LazyDLL { return syscall.NewLazyDLL(inSystemDir(name)) }

// Process, memory and module functions (kernel32).
var (
	procGetModuleHandleW = kernel32.export("GetModuleHandleW")
	procLoadLibraryExW   = kernel32.export("LoadLibraryExW")
	procGetProcAddress   = kernel32.export("GetProcAddress")
	procCreateActCtxW    = kernel32.export("CreateActCtxW")
	procActivateActCtx   = kernel32.export("ActivateActCtx")

	procGlobalAlloc  = kernel32.export("GlobalAlloc")
	procGlobalLock   = kernel32.export("GlobalLock")
	procGlobalUnlock = kernel32.export("GlobalUnlock")
	procGlobalSize   = kernel32.export("GlobalSize")
	procGlobalFree   = kernel32.export("GlobalFree")
)

// DPI awareness and scaling (user32; the ...ForDpi and DpiAwarenessContext
// functions are Windows 10 only and checked before use).
var (
	procSetProcessDpiAwarenessContext = user32.export("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.export("SetProcessDPIAware")
	procGetDpiForWindow               = user32.export("GetDpiForWindow")
	procGetDpiForSystem               = user32.export("GetDpiForSystem")
	procAdjustWindowRectExForDpi      = user32.export("AdjustWindowRectExForDpi")
	procAdjustWindowRectEx            = user32.export("AdjustWindowRectEx")
)

// Window classes, windows and their placement (user32).
var (
	procRegisterClassExW    = user32.export("RegisterClassExW")
	procLoadCursorW         = user32.export("LoadCursorW")
	procLoadIconW           = user32.export("LoadIconW")
	procCreateWindowExW     = user32.export("CreateWindowExW")
	procDestroyWindow       = user32.export("DestroyWindow")
	procDefWindowProcW      = user32.export("DefWindowProcW")
	procShowWindow          = user32.export("ShowWindow")
	procUpdateWindow        = user32.export("UpdateWindow")
	procIsIconic            = user32.export("IsIconic")
	procSetWindowPos        = user32.export("SetWindowPos")
	procGetWindowRect       = user32.export("GetWindowRect")
	procGetClientRect       = user32.export("GetClientRect")
	procSetWindowTextW      = user32.export("SetWindowTextW")
	procSetForegroundWindow = user32.export("SetForegroundWindow")
	procGetForegroundWindow = user32.export("GetForegroundWindow")
	procGetActiveWindow     = user32.export("GetActiveWindow")
	procGetAncestor         = user32.export("GetAncestor")
	procMonitorFromWindow   = user32.export("MonitorFromWindow")
	procGetMonitorInfoW     = user32.export("GetMonitorInfoW")
)

// The message queue (user32).
var (
	procGetMessageW      = user32.export("GetMessageW")
	procPeekMessageW     = user32.export("PeekMessageW")
	procTranslateMessage = user32.export("TranslateMessage")
	procDispatchMessageW = user32.export("DispatchMessageW")
	procPostMessageW     = user32.export("PostMessageW")
	procPostQuitMessage  = user32.export("PostQuitMessage")
	procGetKeyState      = user32.export("GetKeyState")
	procMapVirtualKeyW   = user32.export("MapVirtualKeyW")
	procMessageBoxW      = user32.export("MessageBoxW")
)

// Menus and keyboard accelerators (user32).
var (
	procCreateMenu              = user32.export("CreateMenu")
	procCreatePopupMenu         = user32.export("CreatePopupMenu")
	procAppendMenuW             = user32.export("AppendMenuW")
	procSetMenu                 = user32.export("SetMenu")
	procGetMenu                 = user32.export("GetMenu")
	procDrawMenuBar             = user32.export("DrawMenuBar")
	procDestroyMenu             = user32.export("DestroyMenu")
	procCreateAcceleratorTableW = user32.export("CreateAcceleratorTableW")
	procTranslateAcceleratorW   = user32.export("TranslateAcceleratorW")
	procDestroyAcceleratorTable = user32.export("DestroyAcceleratorTable")
)

// The clipboard (user32).
var (
	procOpenClipboard              = user32.export("OpenClipboard")
	procIsClipboardFormatAvailable = user32.export("IsClipboardFormatAvailable")
	procGetClipboardData           = user32.export("GetClipboardData")
	procEmptyClipboard             = user32.export("EmptyClipboard")
	procSetClipboardData           = user32.export("SetClipboardData")
	procCloseClipboard             = user32.export("CloseClipboard")
)

// COM, the shell and the registry.
var (
	procCoInitializeEx   = ole32.export("CoInitializeEx")
	procCoCreateInstance = ole32.export("CoCreateInstance")
	procCoTaskMemFree    = ole32.export("CoTaskMemFree")

	procSHCreateItemFromParsingName = shell32.export("SHCreateItemFromParsingName")
	procSHCreateMemStream           = shlwapi.export("SHCreateMemStream")

	procRegOpenKeyExW    = advapi32.export("RegOpenKeyExW")
	procRegQueryValueExW = advapi32.export("RegQueryValueExW")
	procRegCloseKey      = advapi32.export("RegCloseKey")
)

// Window messages (winuser.h). The wmApp* values are private messages
// posted to the hidden application window.
const (
	wmDestroy         = 0x0002
	wmMove            = 0x0003
	wmSize            = 0x0005
	wmActivate        = 0x0006
	wmSetFocus        = 0x0007
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmQuit            = 0x0012
	wmEndSession      = 0x0016
	wmActivateApp     = 0x001C
	wmGetMinMaxInfo   = 0x0024
	wmCommand         = 0x0111
	wmMoving          = 0x0216
	wmDpiChanged      = 0x02E0
	wmApp             = 0x8000

	wmAppWake     = wmApp + 1 // Backend.Wake asks for Host.Drain
	wmAppStarted  = wmApp + 2 // the loop is running: report LifecycleStarted
	wmAppDeferred = wmApp + 3 // work queued with runDeferred (modal dialogs)
)

// Window styles, ShowWindow commands and SetWindowPos flags (winuser.h).
const (
	wsOverlapped   = 0
	wsMaximizeBox  = 0x00010000
	wsMinimizeBox  = 0x00020000
	wsThickFrame   = 0x00040000
	wsSysMenu      = 0x00080000
	wsCaption      = 0x00C00000
	wsClipChildren = 0x02000000

	// WS_OVERLAPPEDWINDOW: a normal resizable top-level window.
	wsOverlappedWindow = wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsOverlapped

	cwUseDefault = 0x80000000 // CW_USEDEFAULT as an unsigned argument

	swHide       = 0
	swShowNormal = 1
	swShow       = 5
	swMinimize   = 6
	swRestore    = 9

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	sizeMinimized = 1 // WM_SIZE wParam SIZE_MINIMIZED
	waInactive    = 0 // WM_ACTIVATE low word WA_INACTIVE
	gaRoot        = 2 // GetAncestor GA_ROOT

	monitorDefaultToNearest = 2

	pmRemove = 0x0001 // PeekMessage PM_REMOVE
	idcArrow = 32512  // LoadCursor IDC_ARROW
)

// Menus, keys and the clipboard (winuser.h, winbase.h).
const (
	mfString    = 0
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800

	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt

	cfUnicodeText = 13     // CF_UNICODETEXT
	gmemMoveable  = 0x0002 // GMEM_MOVEABLE
)

// COM, DPI and registry values.
const (
	coinitApartmentThreaded = 0x2 // COINIT_APARTMENTTHREADED
	clsctxInprocServer      = 0x1 // CLSCTX_INPROC_SERVER

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the handle value -4.
	dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

	sOK                = 0
	eNoInterface       = 0x80004002 // E_NOINTERFACE
	errorCancelledHRes = 0x800704C7 // HRESULT for ERROR_CANCELLED (1223)

	hkeyCurrentUser  = 0x80000001
	hkeyLocalMachine = 0x80000002
	keyQueryValue    = 0x0001 // KEY_QUERY_VALUE
	keyWow6432Key    = 0x0200 // KEY_WOW64_32KEY
)

// rect is RECT.
type rect struct {
	Left, Top     int32
	Right, Bottom int32
}

// point is POINT.
type point struct {
	X int32
	Y int32
}

// msg is MSG (the trailing lPrivate field included).
type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

// wndClassEx is WNDCLASSEXW.
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// minMaxInfo is MINMAXINFO, the WM_GETMINMAXINFO payload.
type minMaxInfo struct {
	Reserved     point
	MaxSize      point
	MaxPosition  point
	MinTrackSize point
	MaxTrackSize point
}

// monitorInfo is MONITORINFO.
type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

// accel is ACCEL. Its fields add up to 6 bytes with no padding after the
// one-byte flags field beyond the explicit filler; the array types below
// fail to compile if Go ever lays it out differently.
type accel struct {
	FVirt uint8
	_     uint8
	Key   uint16
	Cmd   uint16
}

var (
	_ [unsafe.Sizeof(accel{}) - 6]struct{}
	_ [6 - unsafe.Sizeof(accel{})]struct{}
)

// guid turns one of the package's GUID literals into a GUID. A typo in a
// literal is a programming error, reported at package initialization.
func guid(literal string) GUID {
	if g, ok := parseGUID(literal); ok {
		return g
	}
	panic("tango: bad GUID " + literal)
}

// takeWstr reads a string that a COM method allocated with CoTaskMemAlloc
// and frees the allocation.
func takeWstr(p uintptr) string {
	if p == 0 {
		return ""
	}
	s := wstr(p)
	procCoTaskMemFree.Call(p)
	return s
}

// instance is the HINSTANCE of the executable.
func instance() uintptr {
	module, _, _ := procGetModuleHandleW.Call(0)
	return module
}

// postMessage queues a message for hwnd's thread and reports success.
func postMessage(hwnd uintptr, m uint32, w, l uintptr) bool {
	ok, _, _ := procPostMessageW.Call(hwnd, uintptr(m), w, l)
	return ok != 0
}

// loword and hiword split the low 32 bits of a message parameter.
func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16 { return uint16(v >> 16 & 0xFFFF) }

// dpiOf is the DPI hwnd is displayed at: the window's own DPI where
// Windows 10 reports it, else the system DPI, else 96.
func dpiOf(hwnd uintptr) int {
	if hwnd != 0 && procGetDpiForWindow.Available() {
		if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
			return int(dpi)
		}
	}
	if procGetDpiForSystem.Available() {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
			return int(dpi)
		}
	}
	return baseDPI
}

// adjustWindowRect turns client bounds in r into the bounds of a window
// with the given style (and a menu bar if menu), measured at dpi when the
// system can, else at the system DPI. Extended styles are never used.
func adjustWindowRect(r *rect, style uint32, menu bool, dpi int) {
	var hasMenu uintptr
	if menu {
		hasMenu = 1
	}
	const exStyle = 0
	if !procAdjustWindowRectExForDpi.Available() {
		procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(r)), uintptr(style), hasMenu, exStyle)
		return
	}
	procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(r)), uintptr(style), hasMenu, exStyle, uintptr(dpi))
}

// keyDown reports whether virtual key vk is held, from the state of the
// message being handled (GetKeyState sets the sign bit when down).
func keyDown(vk int) bool {
	state, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(state) < 0
}

// currentModifiers collects the held Ctrl, Shift and Alt keys.
func currentModifiers() modifiers {
	var held modifiers
	for _, k := range [...]struct {
		vk  int
		mod modifiers
	}{{vkControl, modCtrl}, {vkShift, modShift}, {vkMenu, modAlt}} {
		if keyDown(k.vk) {
			held |= k.mod
		}
	}
	return held
}

// regKey is an open registry key handle.
type regKey uintptr

// openRegKey opens path under HKLM (machine) or HKCU for reading, in the
// 32-bit registry view: EdgeUpdate keeps its client records there on
// 64-bit Windows. It returns 0 when the key cannot be opened.
func openRegKey(machine bool, path string) regKey {
	root := uintptr(hkeyCurrentUser)
	if machine {
		root = hkeyLocalMachine
	}
	var h uintptr
	status, _, _ := procRegOpenKeyExW.Call(root, uintptr(unsafe.Pointer(u16(path))), 0,
		keyQueryValue|keyWow6432Key, uintptr(unsafe.Pointer(&h)))
	if status != 0 {
		return 0
	}
	return regKey(h)
}

func (k regKey) close() { procRegCloseKey.Call(uintptr(k)) }

// query runs RegQueryValueExW for value name. With data == nil it only
// learns the size; otherwise it fills data, *size giving its capacity in
// bytes on entry. It returns the Win32 status (0 on success).
func (k regKey) query(name *uint16, valueType, size *uint32, data *uint16) uintptr {
	status, _, _ := procRegQueryValueExW.Call(uintptr(k), uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(valueType)), uintptr(unsafe.Pointer(data)), uintptr(unsafe.Pointer(size)))
	return status
}

// regString reads a string value; "" means missing, empty or unreadable.
// The value is sized first, then read into a buffer one unit larger than
// needed so the result is terminated even if the stored data is not.
func regString(machine bool, path, name string) string {
	key := openRegKey(machine, path)
	if key == 0 {
		return ""
	}
	defer key.close()
	value := u16(name)
	var valueType, size uint32
	if key.query(value, &valueType, &size, nil) != 0 || size == 0 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if key.query(value, &valueType, &size, &buf[0]) != 0 {
		return ""
	}
	return utf16Units(buf)
}
