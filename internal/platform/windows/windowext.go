package windows

// Pure logic behind the round-2 window capabilities (Geometry, Stateful,
// Styled, Dragger, file drops) of the Windows backend. The native side is
// in surface_ext_windows.go.

import (
	"errors"
	"math"
	"net/url"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// Window style bits used here (WS_*); the native files have their own
// copies, these keep this file free of platform build tags.
const (
	styleCaption     = 0x00C00000
	styleSysMenu     = 0x00080000
	styleThickFrame  = 0x00040000
	styleMinimizeBox = 0x00020000
	styleMaximizeBox = 0x00010000
	stylePopup       = 0x80000000

	styleOverlappedWindow = styleCaption | styleSysMenu | styleThickFrame | styleMinimizeBox | styleMaximizeBox
)

// Non-client hit-test codes (HT*).
const (
	htCaption     = 2
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
)

// ShowWindow commands (SW_*).
const (
	showNormal       = 1
	showMinimized    = 2
	showMaximized    = 3
	showNoActivate   = 4
	showMinNoActive  = 7
	showNA           = 8
	showCmdShow      = 5
	showCmdMaximize  = 3
	showCmdMinimize  = 6
	showCmdRestore   = 9
	sizeTypeRestored = 0
	sizeTypeMinimize = 1
	sizeTypeMaximize = 2
)

// edgeHitTest maps a Dragger edge name to the hit-test code that starts
// the matching resize when posted with WM_NCLBUTTONDOWN.
func edgeHitTest(edge string) (uintptr, bool) {
	switch edge {
	case "n":
		return htTop, true
	case "s":
		return htBottom, true
	case "e":
		return htRight, true
	case "w":
		return htLeft, true
	case "ne":
		return htTopRight, true
	case "nw":
		return htTopLeft, true
	case "se":
		return htBottomRight, true
	case "sw":
		return htBottomLeft, true
	}
	return 0, false
}

// isFileURL reports whether u is a file: URL.
func isFileURL(u string) bool {
	return len(u) >= 5 && strings.EqualFold(u[:5], "file:")
}

// fileURLToPath converts a file: URL, as WebView2 navigates to a dropped
// file, into a Windows path:
//
//	file:///C:/a%20b/c.txt          → C:\a b\c.txt
//	file://localhost/C:/x           → C:\x
//	file://server/share/dir/f.txt   → \\server\share\dir\f.txt
func fileURLToPath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(u.Scheme, "file") {
		return "", errors.New("not a file URL")
	}
	p := u.Path
	if p == "" && u.Opaque != "" { // file:C:/x
		if p, err = url.PathUnescape(u.Opaque); err != nil {
			return "", err
		}
	}
	host := u.Host
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	if host != "" {
		if p == "" || p == "/" {
			return "", errors.New("file URL without a share")
		}
		return `\\` + host + strings.ReplaceAll(p, "/", `\`), nil
	}
	// "/C:/x" or "/C|/x" → "C:/x"
	if len(p) >= 3 && p[0] == '/' && isDriveLetter(p[1]) && (p[2] == ':' || p[2] == '|') {
		p = p[1:2] + ":" + p[3:]
	} else if len(p) >= 2 && isDriveLetter(p[0]) && (p[1] == ':' || p[1] == '|') {
		p = p[:1] + ":" + p[2:]
	}
	if p == "" {
		return "", errors.New("file URL without a path")
	}
	p = strings.ReplaceAll(p, "/", `\`)
	if len(p) == 2 && p[1] == ':' {
		p += `\`
	}
	return p, nil
}

func isDriveLetter(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }

// stateOf derives the contract state. Minimized wins over fullscreen (a
// fullscreen window can be minimized and comes back fullscreen), which
// wins over maximized (IsZoomed may stay set under fullscreen).
func stateOf(minimized, maximized, fullscreen bool) platform.WindowState {
	switch {
	case minimized:
		return platform.StateMinimized
	case fullscreen:
		return platform.StateFullscreen
	case maximized:
		return platform.StateMaximized
	}
	return platform.StateNormal
}

// sizeState derives the state from WM_SIZE's wParam.
func sizeState(sizeType uintptr, fullscreen bool) platform.WindowState {
	return stateOf(sizeType == sizeTypeMinimize, sizeType == sizeTypeMaximize, fullscreen)
}

// firstShowCmd is the ShowWindow command that first shows a window that
// should start in state (fullscreen is entered after a normal show).
func firstShowCmd(state platform.WindowState, noActivate bool) int {
	switch state {
	case platform.StateMaximized:
		return showMaximized // always activates
	case platform.StateMinimized:
		if noActivate {
			return showMinNoActive
		}
		return showMinimized
	}
	if noActivate {
		return showNoActivate
	}
	return showNormal
}

// boxStyle sets the minimize/maximize box bits of style from a. A window
// that is not resizable never gets a maximize box (as without extras).
func boxStyle(style uint32, a platform.Appearance, resizable bool) uint32 {
	style &^= styleMinimizeBox | styleMaximizeBox
	if !a.NoMinimize {
		style |= styleMinimizeBox
	}
	if !a.NoMaximize && resizable {
		style |= styleMaximizeBox
	}
	return style
}

// fullscreenStyle is style without the overlapped frame, as a popup.
func fullscreenStyle(style uint32) uint32 {
	return style&^styleOverlappedWindow | stylePopup
}

// frameless reports whether a has no system title bar on Windows: both
// TitleBarHidden and TitleBarNone remove the whole non-client area.
func frameless(a platform.Appearance) bool {
	return a.TitleBar == platform.TitleBarHidden || a.TitleBar == platform.TitleBarNone
}

// frameMargins is the MARGINS (left, right, top, bottom) passed to
// DwmExtendFrameIntoClientArea: the whole window (-1) for transparency or
// a backdrop material, 1px to keep DWM's shadow on a frameless window.
func frameMargins(a platform.Appearance) [4]int32 {
	switch {
	case a.Transparent || a.Material != platform.MaterialNone:
		return [4]int32{-1, -1, -1, -1}
	case frameless(a) && !a.NoShadow:
		return [4]int32{1, 1, 1, 1}
	}
	return [4]int32{}
}

// backdropType is the DWMWA_SYSTEMBACKDROP_TYPE value for m.
func backdropType(m platform.Material) uint32 {
	switch m {
	case platform.MaterialMica:
		return 2 // DWMSBT_MAINWINDOW
	case platform.MaterialBlur:
		return 3 // DWMSBT_TRANSIENTWINDOW (acrylic)
	}
	return 1 // DWMSBT_NONE
}

// webviewColor packs the COREWEBVIEW2_COLOR {A, R, G, B} for a as the
// 4-byte value passed in a register. WebView2 accepts only alpha 0 or 255:
// transparent, or Background made opaque (white when 0).
func webviewColor(a platform.Appearance) uintptr {
	if a.Transparent {
		return 0
	}
	bg := a.Background
	if bg == 0 {
		bg = 0xFFFFFFFF
	}
	r, g, b := (bg>>24)&0xFF, (bg>>16)&0xFF, (bg>>8)&0xFF
	return uintptr(0xFF) | uintptr(r)<<8 | uintptr(g)<<16 | uintptr(b)<<24
}

// layeredAlpha is the SetLayeredWindowAttributes alpha for an opacity,
// and whether the window must be layered at all.
func layeredAlpha(opacity float64) (uint8, bool) {
	if opacity <= 0 || opacity >= 1 || math.IsNaN(opacity) {
		return 255, false
	}
	return uint8(math.Round(opacity * 255)), true
}

// maximizedClient is the client rectangle of a frameless window, whose
// non-client area is removed: a maximized window overhangs its monitor by
// the sizing frame on each side, which is kept out of the client area.
func maximizedClient(l, t, r, b int32, maximized bool, fx, fy int32) (int32, int32, int32, int32) {
	if !maximized {
		return l, t, r, b
	}
	return l + fx, t + fy, r - fx, b - fy
}

// placeCentered reports whether a new window keeps the default centered
// placement for extras ex.
func placeCentered(ex *platform.SurfaceExtras) bool {
	if ex == nil || ex.Centered {
		return true
	}
	return ex.Bounds == platform.Rect{}
}

// clampSize clamps v to [lo, hi], where 0 means no limit.
func clampSize(v, lo, hi int) int {
	if hi > 0 && v > hi {
		v = hi
	}
	if lo > 0 && v < lo {
		v = lo
	}
	return v
}
