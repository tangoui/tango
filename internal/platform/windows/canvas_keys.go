package windows

// Platform-independent helpers of the canvas child window
// (canvas_windows.go, canvas_input_windows.go): virtual-key, modifier,
// cursor and wheel mapping, click counting, UTF-16 decoding of WM_CHAR,
// pixel swizzling and refresh pacing. Kept free of build tags so they are
// tested on every OS.

import (
	"encoding/binary"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

// Virtual-key codes (winuser.h).
const (
	canvasVKBack     = 0x08
	canvasVKTab      = 0x09
	canvasVKClear    = 0x0C // keypad 5 with NumLock off
	canvasVKReturn   = 0x0D
	canvasVKShift    = 0x10
	canvasVKControl  = 0x11
	canvasVKMenu     = 0x12
	canvasVKCapital  = 0x14
	canvasVKEscape   = 0x1B
	canvasVKSpace    = 0x20
	canvasVKPrior    = 0x21
	canvasVKNext     = 0x22
	canvasVKEnd      = 0x23
	canvasVKHome     = 0x24
	canvasVKLeft     = 0x25
	canvasVKUp       = 0x26
	canvasVKRight    = 0x27
	canvasVKDown     = 0x28
	canvasVKInsert   = 0x2D
	canvasVKDelete   = 0x2E
	canvasVKLWin     = 0x5B
	canvasVKRWin     = 0x5C
	canvasVKApps     = 0x5D
	canvasVKNumpad0  = 0x60
	canvasVKNumpad9  = 0x69
	canvasVKMultiply = 0x6A
	canvasVKAdd      = 0x6B
	canvasVKSubtract = 0x6D
	canvasVKDecimal  = 0x6E
	canvasVKDivide   = 0x6F
	canvasVKF1       = 0x70
	canvasVKF12      = 0x7B
	canvasVKLShift   = 0xA0
	canvasVKRShift   = 0xA1
	canvasVKLControl = 0xA2
	canvasVKRControl = 0xA3
	canvasVKLMenu    = 0xA4
	canvasVKRMenu    = 0xA5
	canvasVKProcess  = 0xE5 // the IME is processing the key
	canvasVKKeyFirst = 0x30 // '0'
)

// Keys whose virtual-key code does not depend on the keyboard layout.
var canvasVKKeys = map[uint16]input.Key{
	canvasVKBack:     input.KeyBackspace,
	canvasVKTab:      input.KeyTab,
	canvasVKReturn:   input.KeyEnter, // keypad Enter: extended bit, see canvasKeyFromMsg
	canvasVKEscape:   input.KeyEscape,
	canvasVKSpace:    input.KeySpace,
	canvasVKPrior:    input.KeyPageUp,
	canvasVKNext:     input.KeyPageDown,
	canvasVKEnd:      input.KeyEnd,
	canvasVKHome:     input.KeyHome,
	canvasVKLeft:     input.KeyLeft,
	canvasVKUp:       input.KeyUp,
	canvasVKRight:    input.KeyRight,
	canvasVKDown:     input.KeyDown,
	canvasVKInsert:   input.KeyInsert,
	canvasVKDelete:   input.KeyDelete,
	canvasVKCapital:  input.KeyCapsLock,
	canvasVKApps:     input.KeyContextMenu,
	canvasVKMultiply: input.KeyNumpadMultiply,
	canvasVKAdd:      input.KeyNumpadAdd,
	canvasVKSubtract: input.KeyNumpadSubtract,
	canvasVKDecimal:  input.KeyNumpadDecimal,
	canvasVKDivide:   input.KeyNumpadDivide,
	canvasVKLShift:   input.KeyShiftLeft,
	canvasVKRShift:   input.KeyShiftRight,
	canvasVKLControl: input.KeyCtrlLeft,
	canvasVKRControl: input.KeyCtrlRight,
	canvasVKLMenu:    input.KeyAltLeft,
	canvasVKRMenu:    input.KeyAltRight,
	canvasVKLWin:     input.KeyMetaLeft,
	canvasVKRWin:     input.KeyMetaRight,
}

// Scan codes (set 1, lParam bits 16-23) of the keypad keys that send a
// navigation virtual key with NumLock off (or with Shift held), when the
// extended bit is clear: the physical keypad key, reported as Code. The
// navigation block keys with the same codes carry the extended bit.
var canvasNumpadScanKeys = map[uint16]input.Key{
	0x52: input.KeyNumpad0,       // VK_INSERT
	0x4F: input.KeyNumpad1,       // VK_END
	0x50: input.KeyNumpad2,       // VK_DOWN
	0x51: input.KeyNumpad3,       // VK_NEXT
	0x4B: input.KeyNumpad4,       // VK_LEFT
	0x4C: input.KeyNumpad5,       // VK_CLEAR
	0x4D: input.KeyNumpad6,       // VK_RIGHT
	0x47: input.KeyNumpad7,       // VK_HOME
	0x48: input.KeyNumpad8,       // VK_UP
	0x49: input.KeyNumpad9,       // VK_PRIOR
	0x53: input.KeyNumpadDecimal, // VK_DELETE
}

// Scan codes of the main-block punctuation keys, by their position on the
// US layout. The VK_OEM_* codes these keys send move with the layout, the
// scan code does not.
var canvasPunctScanKeys = map[uint16]input.Key{
	0x0C: input.KeyMinus,
	0x0D: input.KeyEqual,
	0x1A: input.KeyBracketLeft,
	0x1B: input.KeyBracketRight,
	0x2B: input.KeyBackslash,
	0x27: input.KeySemicolon,
	0x28: input.KeyQuote,
	0x29: input.KeyBackquote,
	0x33: input.KeyComma,
	0x34: input.KeyPeriod,
	0x35: input.KeySlash,
}

// canvasKeyLParam splits a WM_KEYDOWN/WM_KEYUP lParam into the scan code
// (bits 16-23) and the extended-key flag (bit 24).
func canvasKeyLParam(lp uintptr) (scan uint16, extended bool) {
	return uint16(lp>>16) & 0xFF, lp&(1<<24) != 0
}

// canvasKeyFromMsg maps the virtual-key code and lParam of
// WM_KEYDOWN/WM_KEYUP to the reported Key and, when it differs, the
// physical key (KeyEvent.Code; 0 otherwise). Letter and digit virtual keys
// follow the active keyboard layout (the key labelled C on Dvorak sends
// 'C'), so shortcuts are layout aware like on macOS; VK_OEM_* punctuation
// keys are named by their physical position (scan code). The generic
// VK_SHIFT, VK_CONTROL and VK_MENU become the left or right key: right
// Shift has scan code 0x36, right Ctrl and Alt the extended bit; the
// extended VK_SHIFT the system fakes around Shift+keypad and Print Screen
// is dropped. Keypad digits and the decimal key follow NumLock: with it on
// they send VK_NUMPAD0-9/VK_DECIMAL and are Numpad keys; with it off (or
// Shift held) they send navigation keys without the extended bit, reported
// as the navigation Key (keypad 5, VK_CLEAR, as KeyUnknown) with the
// Numpad key as code. Keypad Enter and operators ignore NumLock.
// VK_PROCESSKEY (the IME owns the key) and unmapped keys are KeyUnknown
// with code 0.
func canvasKeyFromMsg(vk uint16, lp uintptr) (key, code input.Key) {
	scan, ext := canvasKeyLParam(lp)
	switch vk {
	case canvasVKShift:
		switch {
		case ext:
			return input.KeyUnknown, 0 // fake shift (E0 2A / E0 AA)
		case scan == 0x36:
			return input.KeyShiftRight, 0
		}
		return input.KeyShiftLeft, 0
	case canvasVKControl:
		if ext {
			return input.KeyCtrlRight, 0
		}
		return input.KeyCtrlLeft, 0
	case canvasVKMenu:
		if ext {
			return input.KeyAltRight, 0
		}
		return input.KeyAltLeft, 0
	case canvasVKReturn:
		if ext {
			return input.KeyNumpadEnter, 0
		}
		return input.KeyEnter, 0
	case canvasVKInsert, canvasVKDelete, canvasVKHome, canvasVKEnd, canvasVKPrior, canvasVKNext,
		canvasVKLeft, canvasVKUp, canvasVKRight, canvasVKDown, canvasVKClear:
		if p, ok := canvasNumpadScanKeys[scan]; ok && !ext {
			nav, _ := canvasNumpadNav(p)
			return nav, p
		}
	}
	if k, ok := canvasVKKeys[vk]; ok {
		return k, 0
	}
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= canvasVKKeyFirst && vk <= '9':
		return input.KeyForRune(rune(vk)), 0
	case vk >= canvasVKNumpad0 && vk <= canvasVKNumpad9:
		return input.KeyNumpad0 + input.Key(vk-canvasVKNumpad0), 0
	case vk >= canvasVKF1 && vk <= canvasVKF12:
		return input.KeyF1 + input.Key(vk-canvasVKF1), 0
	case canvasIsOEMVK(vk) && !ext:
		if scan == 0 && canvasVKToScan != nil {
			scan = canvasVKToScan(vk)
		}
		return canvasPunctScanKeys[scan], 0
	}
	return input.KeyUnknown, 0
}

// canvasVKToScan maps a virtual-key code to its scan code on the active
// keyboard layout (MapVirtualKeyW; nil off Windows). Input injected without
// a scan code (SendInput or keybd_event with wScan 0, as .NET SendKeys,
// some remote-desktop and automation tools do) still names the punctuation
// key that types the character, so Ctrl+/ and the like keep working.
var canvasVKToScan func(vk uint16) uint16

// canvasNumpadNav is the navigation key a keypad key stands for with
// NumLock off: Numpad8 is Up, Numpad0 Insert, NumpadDecimal Delete ...
// Numpad5 has no navigation meaning and is KeyUnknown. ok is false for
// keys that do not change with NumLock.
func canvasNumpadNav(k input.Key) (nav input.Key, ok bool) {
	switch k {
	case input.KeyNumpad0:
		return input.KeyInsert, true
	case input.KeyNumpad1:
		return input.KeyEnd, true
	case input.KeyNumpad2:
		return input.KeyDown, true
	case input.KeyNumpad3:
		return input.KeyPageDown, true
	case input.KeyNumpad4:
		return input.KeyLeft, true
	case input.KeyNumpad5:
		return input.KeyUnknown, true
	case input.KeyNumpad6:
		return input.KeyRight, true
	case input.KeyNumpad7:
		return input.KeyHome, true
	case input.KeyNumpad8:
		return input.KeyUp, true
	case input.KeyNumpad9:
		return input.KeyPageUp, true
	case input.KeyNumpadDecimal:
		return input.KeyDelete, true
	}
	return input.KeyUnknown, false
}

// canvasHeldNav marks a held-key record of a keypad key pressed in its
// navigation form (see canvasHeldID). input.Key values stay far below it.
const canvasHeldNav input.Key = 0x8000

// canvasHeldID is the record of a reported press in the canvas's held-key
// set, from which the release is reported with the same Key and Code
// even when NumLock changed in between: the key itself, or for a keypad
// key reported as a navigation key its Numpad code marked canvasHeldNav.
func canvasHeldID(key, code input.Key) input.Key {
	if code != 0 {
		return code | canvasHeldNav
	}
	return key
}

// canvasHeldKey is the Key and Code a held-key record was pressed with.
func canvasHeldKey(id input.Key) (key, code input.Key) {
	if id&canvasHeldNav != 0 {
		code = id &^ canvasHeldNav
		key, _ = canvasNumpadNav(code)
		return key, code
	}
	return id, 0
}

// canvasHeldFind finds the record of the physical key of a key message
// in held, whichever NumLock form it was pressed in.
func canvasHeldFind(held map[input.Key]bool, key, code input.Key) (input.Key, bool) {
	phys := code
	if phys == 0 {
		phys = key
	}
	ids := []input.Key{canvasHeldID(key, code)}
	if _, ok := canvasNumpadNav(phys); ok {
		ids = []input.Key{phys, phys | canvasHeldNav}
	}
	for _, id := range ids {
		if held[id] {
			return id, true
		}
	}
	return 0, false
}

// canvasIsOEMVK reports the layout-dependent VK_OEM_* punctuation codes
// (VK_OEM_1..VK_OEM_3, VK_OEM_PLUS..VK_OEM_PERIOD, VK_OEM_4..VK_OEM_8,
// VK_OEM_102).
func canvasIsOEMVK(vk uint16) bool {
	return vk >= 0xBA && vk <= 0xC0 || vk >= 0xDB && vk <= 0xDF || vk == 0xE2
}

// canvasOtherSideVK is the virtual key of the other side of a modifier
// key (VK_RSHIFT for KeyShiftLeft ...), 0 for other keys.
func canvasOtherSideVK(k input.Key) int {
	switch k {
	case input.KeyShiftLeft:
		return canvasVKRShift
	case input.KeyShiftRight:
		return canvasVKLShift
	case input.KeyCtrlLeft:
		return canvasVKRControl
	case input.KeyCtrlRight:
		return canvasVKLControl
	case input.KeyAltLeft:
		return canvasVKRMenu
	case input.KeyAltRight:
		return canvasVKLMenu
	case input.KeyMetaLeft:
		return canvasVKRWin
	case input.KeyMetaRight:
		return canvasVKLWin
	}
	return 0
}

// canvasModifierEventMods is the Mods of a modifier key's own event, the
// same rule on every desktop backend: a press includes the key's
// modifier, a release excludes it unless the other side's key is still
// held.
func canvasModifierEventMods(k input.Key, down bool, mods input.Modifiers, otherHeld bool) input.Modifiers {
	bit := k.Modifier()
	if down || otherHeld {
		return mods | bit
	}
	return mods &^ bit
}

// canvasMods builds a modifier set; the Windows key is ModMeta.
func canvasMods(shift, ctrl, alt, win bool) input.Modifiers {
	var m input.Modifiers
	if shift {
		m |= input.ModShift
	}
	if ctrl {
		m |= input.ModCtrl
	}
	if alt {
		m |= input.ModAlt
	}
	if win {
		m |= input.ModMeta
	}
	return m
}

// Mouse message wParam key-state bits.
const (
	canvasMKShift   = 0x0004
	canvasMKControl = 0x0008
)

// canvasMouseMods reads Shift/Ctrl from a mouse message's wParam; Alt and
// the Windows key are not in it and come from GetKeyState.
func canvasMouseMods(wp uintptr, alt, win bool) input.Modifiers {
	return canvasMods(wp&canvasMKShift != 0, wp&canvasMKControl != 0, alt, win)
}

// canvasKeyRepeat reports bit 30 of a WM_KEYDOWN lParam: the key was
// already down (auto-repeat).
func canvasKeyRepeat(lp uintptr) bool { return lp&(1<<30) != 0 }

// Standard cursor resource IDs (IDC_*).
const (
	canvasIDCArrow   = 32512
	canvasIDCIBeam   = 32513
	canvasIDCSizeWE  = 32644
	canvasIDCSizeNS  = 32645
	canvasIDCSizeAll = 32646
	canvasIDCNo      = 32648
	canvasIDCHand    = 32649
)

// canvasCursorID is the system cursor for c. Windows has no open/closed
// hand cursors: Grab (something can be dragged) uses the hand, Grabbing
// (dragging) and Move use the four-way arrow, which is what Windows itself
// shows while moving things.
func canvasCursorID(c input.Cursor) uintptr {
	switch c {
	case input.CursorPointer, input.CursorGrab:
		return canvasIDCHand
	case input.CursorText:
		return canvasIDCIBeam
	case input.CursorNotAllowed:
		return canvasIDCNo
	case input.CursorResizeEW:
		return canvasIDCSizeWE
	case input.CursorResizeNS:
		return canvasIDCSizeNS
	case input.CursorGrabbing, input.CursorMove:
		return canvasIDCSizeAll
	}
	// CursorCopy: Windows has no stock copy cursor (OLE drag-and-drop draws
	// its own); the arrow is the closest system shape.
	return canvasIDCArrow
}

const (
	// canvasScrollLine is the logical pixels per wheel line, the same line
	// height the macOS canvas uses for non-precise wheels.
	canvasScrollLine = 40
	canvasWheelDelta = 120        // WHEEL_DELTA: one notch
	canvasWheelPage  = 0xFFFFFFFF // WHEEL_PAGESCROLL from SPI_GETWHEELSCROLLLINES
)

// canvasScroll converts a WM_MOUSEWHEEL / WM_MOUSEHWHEEL delta into
// input.Scroll deltas in logical pixels. units is the user's lines (or
// characters, horizontally) per notch, canvasWheelPage for a page; page is
// the page size in logical pixels. A positive vertical delta means the
// wheel rotated away from the user, which scrolls up (reveals content
// above): DY is negative. A positive horizontal delta tilts right and
// reveals content to the right: DX is positive. High-resolution wheels
// and touchpads send fractions of a notch, kept proportionally.
func canvasScroll(delta int16, horizontal bool, units uint32, page float32) (dx, dy float32) {
	per := float32(units) * canvasScrollLine
	if units == canvasWheelPage {
		per = page
	}
	v := float32(delta) / canvasWheelDelta * per
	if horizontal {
		return v, 0
	}
	return 0, 0 - v // 0-v: no negative zero
}

// canvasClickCounter counts consecutive presses of one button within the
// double-click time and rectangle (GetDoubleClickTime,
// SM_CXDOUBLECLK/SM_CYDOUBLECLK), like the system does for CS_DBLCLKS but
// without stopping at two.
type canvasClickCounter struct {
	button input.Button
	time   uint32 // GetMessageTime of the last press, ms (wraps)
	x, y   int32  // physical client pixels
	count  int
}

// press records a press and returns its click count.
func (cc *canvasClickCounter) press(b input.Button, t uint32, x, y int32, maxMS uint32, cx, cy int32) int {
	same := cc.count > 0 && b == cc.button && t-cc.time <= maxMS &&
		canvasAbs(x-cc.x)*2 <= cx && canvasAbs(y-cc.y)*2 <= cy
	if same {
		cc.count++
	} else {
		cc.count = 1
		cc.x, cc.y = x, y // the rectangle is centred on the first click
	}
	cc.button, cc.time = b, t
	return cc.count
}

// reset forgets the last press, so the next one counts as a single
// click (used when capture is lost).
func (cc *canvasClickCounter) reset() { cc.count = 0 }

func canvasAbs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// canvasCharDecoder joins the UTF-16 code units of successive WM_CHAR
// messages; characters outside the BMP arrive as a surrogate pair in two
// messages.
type canvasCharDecoder struct{ hi uint16 }

// feed adds one code unit and returns the completed text, if any. Control
// characters (Ctrl+letter, Backspace, Tab, Enter, Esc, DEL) are dropped:
// they are reported as key events. A lone surrogate is dropped.
func (d *canvasCharDecoder) feed(u uint16) (string, bool) {
	switch {
	case utf16.IsSurrogate(rune(u)) && u < 0xDC00: // high
		d.hi = u
		return "", false
	case utf16.IsSurrogate(rune(u)): // low
		hi := d.hi
		d.hi = 0
		if hi == 0 {
			return "", false
		}
		r := utf16.DecodeRune(rune(hi), rune(u))
		return string(r), true
	}
	d.hi = 0
	if u < 0x20 || u == 0x7f {
		return "", false
	}
	return string(rune(u)), true
}

// canvasRuneIndex converts a UTF-16 offset into s (GCS_CURSORPOS) to a
// rune offset, clamped to the length of s.
func canvasRuneIndex(s string, u16 int) int {
	n, i := 0, 0
	for _, r := range s {
		if n >= u16 {
			return i
		}
		n += utf16.RuneLen(r)
		i++
	}
	return i
}

// canvasCleanText drops control characters from IME result strings.
func canvasCleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// canvasRGBAToBGRA copies a w×h premultiplied RGBA image into a 32-bit
// DIB (BGRA in memory, premultiplied as AlphaBlend expects), swapping the
// red and blue channels.
func canvasRGBAToBGRA(dst []byte, dstStride int, src []byte, srcStride, w, h int) {
	for y := 0; y < h; y++ {
		s := src[y*srcStride : y*srcStride+w*4]
		d := dst[y*dstStride : y*dstStride+w*4]
		for i := 0; i < len(s); i += 4 {
			v := binary.LittleEndian.Uint32(s[i:])
			binary.LittleEndian.PutUint32(d[i:], v&0xFF00FF00|(v&0xFF)<<16|(v>>16)&0xFF)
		}
	}
}

// canvasColorRef turns the RGB of a premultiplied RGBA pixel into a
// COLORREF (0x00BBGGRR).
func canvasColorRef(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

// canvasRefreshPeriod is the frame interval for a display refresh rate in
// Hz as reported by EnumDisplaySettings (0 and 1 mean "hardware default").
func canvasRefreshPeriod(hz uint32) time.Duration {
	if hz <= 1 || hz > 1000 {
		hz = 60
	}
	return time.Second / time.Duration(hz)
}

// canvasRoleKey is the shortcut of an Edit menu role on Windows.
func canvasRoleKey(role string) (input.Key, input.Modifiers, bool) {
	switch role {
	case "undo":
		return input.KeyZ, input.ModCtrl, true
	case "redo":
		return input.KeyY, input.ModCtrl, true
	case "cut":
		return input.KeyX, input.ModCtrl, true
	case "copy":
		return input.KeyC, input.ModCtrl, true
	case "paste":
		return input.KeyV, input.ModCtrl, true
	case "selectAll":
		return input.KeyA, input.ModCtrl, true
	}
	return input.KeyUnknown, 0, false
}

// canvasKeyMsg is the part of a queued keyboard message the AltGr check
// looks at.
type canvasKeyMsg struct {
	Msg  uint32 // WM_KEYDOWN, WM_KEYUP, WM_SYSKEYDOWN or WM_SYSKEYUP
	VK   uint16
	LP   uintptr
	Time uint32 // GetMessageTime / MSG.time
}

const (
	canvasMsgKeyDown    = 0x0100
	canvasMsgKeyUp      = 0x0101
	canvasMsgSysKeyDown = 0x0104
	canvasMsgSysKeyUp   = 0x0105
)

func canvasKeyMsgDown(m uint32) (down, ok bool) {
	switch m {
	case canvasMsgKeyDown, canvasMsgSysKeyDown:
		return true, true
	case canvasMsgKeyUp, canvasMsgSysKeyUp:
		return false, true
	}
	return false, false
}

// canvasIsAltGrCtrl reports whether cur is the left Ctrl that Windows
// synthesizes for AltGr (the right Alt key on layouts with a third level:
// German, French, Polish ...). The system sends it as VK_CONTROL without
// the extended bit, immediately followed by the real right Alt (VK_MENU
// with the extended bit) with the same message time, on press and on
// release. next is the message after cur in the queue (PeekMessage with
// PM_NOREMOVE); hasNext is false when the queue has none. A real left
// Ctrl pressed just before right Alt has a different time.
func canvasIsAltGrCtrl(cur, next canvasKeyMsg, hasNext bool) bool {
	if !hasNext || cur.VK != canvasVKControl || next.VK != canvasVKMenu {
		return false
	}
	if _, ext := canvasKeyLParam(cur.LP); ext {
		return false // right Ctrl
	}
	if _, ext := canvasKeyLParam(next.LP); !ext {
		return false // left Alt: Ctrl+Alt typed by hand
	}
	cd, ok1 := canvasKeyMsgDown(cur.Msg)
	nd, ok2 := canvasKeyMsgDown(next.Msg)
	return ok1 && ok2 && cd == nd && cur.Time == next.Time
}

// canvasAltGrMods removes the Ctrl that GetKeyState / MK_CONTROL report
// for the synthesized left Ctrl while AltGr is held, unless the right
// Ctrl key is really down. Alt (the AltGr key itself) stays.
func canvasAltGrMods(m input.Modifiers, altGr, rightCtrlHeld bool) input.Modifiers {
	if altGr && !rightCtrlHeld {
		return m &^ input.ModCtrl
	}
	return m
}

// canvasDropEffect is the IDropTarget answer for files dropped on a canvas
// given the effects the source allows: copy, else link, else none (never
// move: the files stay where they are).
func canvasDropEffect(allowed uint32) uint32 {
	const copyEffect, linkEffect = 1, 4 // DROPEFFECT_COPY, DROPEFFECT_LINK
	switch {
	case allowed&copyEffect != 0:
		return copyEffect
	case allowed&linkEffect != 0:
		return linkEffect
	}
	return 0
}

// canvasSeeThrough reports whether a canvas surface with appearance a
// shows the desktop or a backdrop through transparent pixels: the
// appearances for which the surface extends the DWM frame over the whole
// client area (see frameMargins).
func canvasSeeThrough(a platform.Appearance) bool {
	return a.Transparent || a.Material != platform.MaterialNone
}

// winScanHID maps Set 1 scan codes without the extended flag to USB HID
// usages on the Keyboard/Keypad page; winExtScanHID those sent with it
// (E0 prefix).
var winScanHID = map[uint16]uint16{
	0x01: 0x29,                                                 // Escape
	0x02: 0x1E, 0x03: 0x1F, 0x04: 0x20, 0x05: 0x21, 0x06: 0x22, // 1-5
	0x07: 0x23, 0x08: 0x24, 0x09: 0x25, 0x0A: 0x26, 0x0B: 0x27, // 6-9 0
	0x0C: 0x2D, 0x0D: 0x2E, 0x0E: 0x2A, 0x0F: 0x2B, // - = Backspace Tab
	0x10: 0x14, 0x11: 0x1A, 0x12: 0x08, 0x13: 0x15, 0x14: 0x17, // Q W E R T
	0x15: 0x1C, 0x16: 0x18, 0x17: 0x0C, 0x18: 0x12, 0x19: 0x13, // Y U I O P
	0x1A: 0x2F, 0x1B: 0x30, 0x1C: 0x28, 0x1D: 0xE0, // [ ] Enter LeftCtrl
	0x1E: 0x04, 0x1F: 0x16, 0x20: 0x07, 0x21: 0x09, 0x22: 0x0A, // A S D F G
	0x23: 0x0B, 0x24: 0x0D, 0x25: 0x0E, 0x26: 0x0F, // H J K L
	0x27: 0x33, 0x28: 0x34, 0x29: 0x35, 0x2A: 0xE1, 0x2B: 0x31, // ; ' ` LeftShift \
	0x2C: 0x1D, 0x2D: 0x1B, 0x2E: 0x06, 0x2F: 0x19, 0x30: 0x05, // Z X C V B
	0x31: 0x11, 0x32: 0x10, 0x33: 0x36, 0x34: 0x37, 0x35: 0x38, // N M , . /
	0x36: 0xE5, 0x37: 0x55, 0x38: 0xE2, 0x39: 0x2C, 0x3A: 0x39, // RightShift KP* LeftAlt Space CapsLock
	0x3B: 0x3A, 0x3C: 0x3B, 0x3D: 0x3C, 0x3E: 0x3D, 0x3F: 0x3E, // F1-F5
	0x40: 0x3F, 0x41: 0x40, 0x42: 0x41, 0x43: 0x42, 0x44: 0x43, // F6-F10
	0x45: 0x48,                                     // Pause (NumLock sends 0x45 with the extended flag)
	0x46: 0x47,                                     // ScrollLock
	0x47: 0x5F, 0x48: 0x60, 0x49: 0x61, 0x4A: 0x56, // KP7 KP8 KP9 KP-
	0x4B: 0x5C, 0x4C: 0x5D, 0x4D: 0x5E, 0x4E: 0x57, // KP4 KP5 KP6 KP+
	0x4F: 0x59, 0x50: 0x5A, 0x51: 0x5B, 0x52: 0x62, 0x53: 0x63, // KP1 KP2 KP3 KP0 KP.
	0x54: 0x46,             // SysRq (Alt+Print Screen)
	0x56: 0x64,             // the key next to left Shift on ISO keyboards
	0x57: 0x44, 0x58: 0x45, // F11 F12
	0x59: 0x67,                                                             // KP=
	0x64: 0x68, 0x65: 0x69, 0x66: 0x6A, 0x67: 0x6B, 0x68: 0x6C, 0x69: 0x6D, // F13-F18
	0x6A: 0x6E, 0x6B: 0x6F, 0x6C: 0x70, 0x6D: 0x71, 0x6E: 0x72, 0x76: 0x73, // F19-F24
	0x70: 0x88, // Kana (International2)
	0x71: 0x91, // Hanja (LANG2)
	0x72: 0x90, // Hangul (LANG1)
	0x73: 0x87, // Ro (International1)
	0x79: 0x8A, // Henkan (International4)
	0x7B: 0x8B, // Muhenkan (International5)
	0x7D: 0x89, // Yen (International3)
	0x7E: 0x85, // keypad comma (Brazilian)
	0xF1: 0x91, // Hanja on keyboards that send F1
	0xF2: 0x90, // Hangul on keyboards that send F2
}

var winExtScanHID = map[uint16]uint16{
	0x1C: 0x58,                         // keypad Enter
	0x1D: 0xE4,                         // RightCtrl
	0x20: 0x7F,                         // Mute
	0x2E: 0x81,                         // VolumeDown
	0x30: 0x80,                         // VolumeUp
	0x35: 0x54,                         // KP/
	0x37: 0x46,                         // Print Screen
	0x38: 0xE6,                         // RightAlt
	0x45: 0x53,                         // NumLock
	0x46: 0x48,                         // Ctrl+Pause (Break)
	0x47: 0x4A, 0x48: 0x52, 0x49: 0x4B, // Home Up PageUp
	0x4B: 0x50, 0x4D: 0x4F, // Left Right
	0x4F: 0x4D, 0x50: 0x51, 0x51: 0x4E, // End Down PageDown
	0x52: 0x49, 0x53: 0x4C, // Insert Delete
	0x5B: 0xE3, 0x5C: 0xE7, 0x5D: 0x65, // LeftWin RightWin Apps
	0x5E: 0x66, // Power
}

const canvasVKPause = 0x13

// hidFromWinScan returns the USB HID usage and the KeyEvent.Native code of
// a key message: its scan code, with an 0xE000 prefix for extended keys;
// Pause, whose real sequence is E1 1D 45, is 0xE11D. The fake Shift the
// system sends around Print Screen and Shift+keypad keys has neither
// (0, 0). Unknown scan codes give 0, 0.
func hidFromWinScan(vk, scan uint16, extended bool) (hid uint16, native uint32) {
	switch {
	case vk == canvasVKPause:
		return 0x48, 0xE11D
	case vk == canvasVKShift && extended:
		return 0, 0
	case extended:
		if h, ok := winExtScanHID[scan]; ok {
			return h, 0xE000 | uint32(scan)
		}
	default:
		if h, ok := winScanHID[scan]; ok {
			return h, uint32(scan)
		}
	}
	return 0, 0
}
