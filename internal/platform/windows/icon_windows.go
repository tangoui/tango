//go:build windows && (amd64 || arm64)

package windows

import (
	"errors"
	"log"
	"unsafe"
)

// Native support for menu item icons, radio items, popup menus and
// notification-area icons. Procs and constants here carry an "mt" prefix
// (menu/tray) so they cannot collide with ones other files add to
// win32_windows.go.

var (
	mtGDI32  = systemDLL("gdi32.dll")
	mtShcore = systemDLL("shcore.dll")

	mtCreateDIBSection       = mtGDI32.NewProc("CreateDIBSection")
	mtCreateBitmap           = mtGDI32.NewProc("CreateBitmap")
	mtDeleteObject           = mtGDI32.NewProc("DeleteObject")
	mtCreateIconIndirect     = user32.NewProc("CreateIconIndirect")
	mtDestroyIcon            = user32.NewProc("DestroyIcon")
	mtSetMenuItemInfoW       = user32.NewProc("SetMenuItemInfoW")
	mtTrackPopupMenuEx       = user32.NewProc("TrackPopupMenuEx")
	mtClientToScreen         = user32.NewProc("ClientToScreen")
	mtGetCursorPos           = user32.NewProc("GetCursorPos")
	mtGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	mtGetSystemMetricsForDpi = user32.NewProc("GetSystemMetricsForDpi")
	mtMonitorFromRect        = user32.NewProc("MonitorFromRect")
	mtGetDpiForMonitor       = mtShcore.NewProc("GetDpiForMonitor")
)

const (
	mtBIRGB        = 0
	mtDIBRGBColors = 0

	mtMIIMFType  = 0x00000100
	mtMIIMBitmap = 0x00000080
	mtMFTRadio   = 0x00000200 // MFT_RADIOCHECK

	mtTPMRightButton = 0x0002
	mtTPMNoNotify    = 0x0080
	mtTPMReturnCmd   = 0x0100

	mtWMNull = 0x0000

	mtSMCXSMIcon = 49 // SM_CXSMICON

	mtMDTEffectiveDPI = 0 // MDT_EFFECTIVE_DPI

	mtMenuIconDIP = 16 // menu icons are 16×16 logical px
)

type mtBitmapInfo struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	Colors        [1]uint32 // BITMAPINFO.bmiColors, unused for BI_RGB 32 bpp
}

type mtIconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// mtMenuItemInfo is MENUITEMINFOW.
type mtMenuItemInfo struct {
	Size         uint32
	Mask         uint32
	Type         uint32
	State        uint32
	ID           uint32
	SubMenu      uintptr
	BmpChecked   uintptr
	BmpUnchecked uintptr
	ItemData     uintptr
	TypeData     *uint16
	Cch          uint32
	BmpItem      uintptr
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(mtBitmapInfo{})-44]
	_ = [1]struct{}{}[unsafe.Sizeof(mtIconInfo{})-32]
	_ = [1]struct{}{}[unsafe.Sizeof(mtMenuItemInfo{})-80]
)

// createDIB32 returns a top-down 32-bit DIB section holding pix (BGRA).
func createDIB32(pix []byte, w, h int) uintptr {
	bi := mtBitmapInfo{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32, Compression: mtBIRGB}
	bi.Size = 40 // BITMAPINFOHEADER only
	var bits uintptr
	hbm, _, _ := mtCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), mtDIBRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		if hbm != 0 {
			mtDeleteObject.Call(hbm)
		}
		return 0
	}
	copy(unsafe.Slice((*byte)(native(bits)), w*h*4), pix)
	return hbm
}

// menuBitmap converts a PNG into a premultiplied 32-bit bitmap for
// MENUITEMINFOW.hbmpItem, size×size physical px. 0 on failure.
func menuBitmap(png []byte, size int) uintptr {
	pix, err := iconPixels(png, size, size, true)
	if err != nil {
		log.Printf("tango: menu icon: %v", err)
		return 0
	}
	return createDIB32(pix, size, size)
}

var errIconCreate = errors.New("tango: cannot create the icon")

// createIcon converts a PNG into an HICON of size×size physical px.
func createIcon(png []byte, size int) (uintptr, error) {
	pix, err := iconPixels(png, size, size, false)
	if err != nil {
		return 0, err
	}
	color := createDIB32(pix, size, size)
	if color == 0 {
		return 0, errIconCreate
	}
	defer mtDeleteObject.Call(color)
	maskBits := iconMask(pix, size, size)
	mask, _, _ := mtCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0, errIconCreate
	}
	defer mtDeleteObject.Call(mask)
	ii := mtIconInfo{FIcon: 1, HbmMask: mask, HbmColor: color}
	h, _, _ := mtCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if h == 0 {
		return 0, errIconCreate
	}
	return h, nil
}

// smallIconSize is SM_CXSMICON at dpi (16 at 96 DPI).
func smallIconSize(dpi int) int {
	if has(mtGetSystemMetricsForDpi) {
		if n, _, _ := mtGetSystemMetricsForDpi.Call(mtSMCXSMIcon, uintptr(dpi)); int32(n) > 0 {
			return int(int32(n))
		}
	}
	if n, _, _ := mtGetSystemMetrics.Call(mtSMCXSMIcon); int32(n) > 0 {
		return int(int32(n))
	}
	return dipToPx(16, dpi)
}

// menuBitmaps holds the item bitmaps of each top-level HMENU (including
// its submenus): DestroyMenu does not free hbmpItem bitmaps.
var menuBitmaps = map[uintptr][]uintptr{}

// releaseMenuBitmaps frees the bitmaps of a menu that is gone or about to
// go (a window menu is destroyed together with its window).
func releaseMenuBitmaps(hmenu uintptr) {
	for _, b := range menuBitmaps[hmenu] {
		mtDeleteObject.Call(b)
	}
	delete(menuBitmaps, hmenu)
}

// destroyMenu destroys a menu tree and its item bitmaps.
func destroyMenu(hmenu uintptr) {
	if hmenu == 0 {
		return
	}
	procDestroyMenu.Call(hmenu)
	releaseMenuBitmaps(hmenu)
}

// setItemExtras applies the radio style and icon of the item at pos in
// hmenu; root owns any bitmap created.
func setItemExtras(root, hmenu uintptr, pos int, e menuEntry, dpi int) {
	if !e.Radio && len(e.Icon) == 0 {
		return
	}
	mii := mtMenuItemInfo{}
	mii.Size = uint32(unsafe.Sizeof(mii))
	if e.Radio {
		mii.Mask |= mtMIIMFType
		mii.Type = mtMFTRadio // MFT_STRING is 0
	}
	if len(e.Icon) > 0 {
		if bmp := menuBitmap(e.Icon, dipToPx(mtMenuIconDIP, dpi)); bmp != 0 {
			menuBitmaps[root] = append(menuBitmaps[root], bmp)
			mii.Mask |= mtMIIMBitmap
			mii.BmpItem = bmp
		}
	}
	if mii.Mask != 0 {
		mtSetMenuItemInfoW.Call(hmenu, uintptr(pos), 1, uintptr(unsafe.Pointer(&mii)))
	}
}

// menuTracking is set while a popup menu (context or tray) is open:
// TrackPopupMenuEx cannot nest.
var menuTracking bool

// trackMenu shows hmenu at screen point (x, y) for owner and returns the
// chosen command (0 if none). It does not send WM_COMMAND (TPM_RETURNCMD),
// so these menus never collide with window-menu command numbers.
func trackMenu(hmenu, owner uintptr, x, y int32) uint16 {
	menuTracking = true
	defer func() { menuTracking = false }()
	// The owner must be foreground or the menu does not close when the
	// user clicks elsewhere; WM_NULL afterwards makes the next click on
	// the notification icon work (KB135788).
	procSetForegroundWindow.Call(owner)
	cmd, _, _ := mtTrackPopupMenuEx.Call(hmenu, mtTPMRightButton|mtTPMNoNotify|mtTPMReturnCmd,
		uintptr(x), uintptr(y), owner, 0)
	postMessage(owner, mtWMNull, 0, 0)
	return uint16(cmd)
}
