//go:build windows && (amd64 || arm64)

package windows

// Input of the canvas child window: mouse, wheel, keyboard, WM_CHAR text,
// IMM32 composition, cursor and focus. Events go to the sink in logical
// pixels relative to the canvas's top-left corner.
//
// Keyboard. WM_KEYDOWN/WM_KEYUP are reported as input.KeyEvent (modifier
// keys as their left/right Key, Mods already including the modifier on
// press and excluding it on release, as on macOS; see canvasKeyFromMsg
// for the key mapping). Keys the
// IME consumes arrive as VK_PROCESSKEY and are not reported; a release is
// only reported for a reported press. WM_SYSKEYDOWN/WM_SYSKEYUP (Alt held,
// F10) are reported and then passed to DefWindowProc, and WM_SYSCHAR is not
// handled, so Alt+F4, Alt+Space and menu mnemonics keep working. Menu
// accelerators are translated by the backend's message loop before the
// canvas sees the key (as the menu sees Cmd shortcuts first on macOS).
//
// AltGr. On layouts with a third level the right Alt key arrives as a
// left Ctrl the system synthesizes, then the real right Alt with the same
// message time (press and release alike). The synthesized Ctrl is
// recognised by peeking at the next queued key message (canvasIsAltGrCtrl)
// and not reported; AltGr is reported as KeyAltRight, and while it is held
// Mods do not include Ctrl (unless the right Ctrl key is down).
//
// Text. While text input is on (SetTextInput), WM_CHAR becomes
// input.TextInput (surrogate pairs joined, control characters dropped) and
// the window has an input context: WM_IME_COMPOSITION's GCS_COMPSTR becomes
// input.Composition (caret from GCS_CURSORPOS) and GCS_RESULTSTR becomes
// input.TextInput. The composition string is drawn by the UI, so the
// IME's own composition window is suppressed (ISC_SHOWUICOMPOSITIONWINDOW
// cleared in WM_IME_SETCONTEXT, WM_IME_STARTCOMPOSITION not passed on);
// the candidate window is placed at the caret with ImmSetCandidateWindow
// and IMR_QUERYCHARPOSITION. While text input is off the window has no
// input context, so the IME stays out of the way, and WM_CHAR is ignored.

import (
	"unicode/utf16"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() {
	canvasVKToScan = func(vk uint16) uint16 {
		const mapVKToVSC = 0
		scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), mapVKToVSC)
		return uint16(scan)
	}
}

const (
	canvasWmSetFocus       = 0x0007
	canvasWmCancelMode     = 0x001F
	canvasWmKillFocus      = 0x0008
	canvasWmSetCursor      = 0x0020
	canvasWmKeyDown        = 0x0100
	canvasWmKeyUp          = 0x0101
	canvasWmChar           = 0x0102
	canvasWmSysKeyDown     = 0x0104
	canvasWmSysKeyUp       = 0x0105
	canvasWmImeStartComp   = 0x010D
	canvasWmImeEndComp     = 0x010E
	canvasWmImeComposition = 0x010F
	canvasWmMouseMove      = 0x0200
	canvasWmLButtonDown    = 0x0201
	canvasWmLButtonUp      = 0x0202
	canvasWmRButtonDown    = 0x0204
	canvasWmRButtonUp      = 0x0205
	canvasWmMButtonDown    = 0x0207
	canvasWmMButtonUp      = 0x0208
	canvasWmMouseWheel     = 0x020A
	canvasWmMouseHWheel    = 0x020E
	canvasWmCaptureChanged = 0x0215
	canvasWmImeSetContext  = 0x0281
	canvasWmImeRequest     = 0x0288
	canvasWmMouseLeave     = 0x02A3

	canvasHTClient = 1

	canvasPMNoRemove = 0x0000 // PeekMessage PM_NOREMOVE

	canvasTMELeave = 0x00000002

	canvasSMCXDoubleClk = 36
	canvasSMCYDoubleClk = 37

	canvasSPIGetWheelScrollLines = 0x0068
	canvasSPIGetWheelScrollChars = 0x006C

	canvasGCSCompStr   = 0x0008
	canvasGCSCursorPos = 0x0080
	canvasGCSResultStr = 0x0800

	canvasIACEDefault          = 0x0010
	canvasISCShowUIComposition = 0x80000000
	canvasNICompositionStr     = 0x0015
	canvasCPSCancel            = 0x0004
	canvasCFSPoint             = 0x0002
	canvasCFSExclude           = 0x0080
	canvasIMRQueryCharPos      = 0x0006
)

var (
	canvasSetCapture           = user32.NewProc("SetCapture")
	canvasReleaseCapture       = user32.NewProc("ReleaseCapture")
	canvasTrackMouseEvent      = user32.NewProc("TrackMouseEvent")
	canvasSetCursorProc        = user32.NewProc("SetCursor")
	canvasScreenToClient       = user32.NewProc("ScreenToClient")
	canvasGetCursorPos         = user32.NewProc("GetCursorPos")
	canvasClientToScreen       = user32.NewProc("ClientToScreen")
	canvasGetMessageTime       = user32.NewProc("GetMessageTime")
	canvasGetDoubleClickTime   = user32.NewProc("GetDoubleClickTime")
	canvasGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	canvasSystemParametersInfo = user32.NewProc("SystemParametersInfoW")

	canvasImmGetContext           = canvasIMM32.NewProc("ImmGetContext")
	canvasImmReleaseContext       = canvasIMM32.NewProc("ImmReleaseContext")
	canvasImmGetCompositionString = canvasIMM32.NewProc("ImmGetCompositionStringW")
	canvasImmSetCompositionWindow = canvasIMM32.NewProc("ImmSetCompositionWindow")
	canvasImmSetCandidateWindow   = canvasIMM32.NewProc("ImmSetCandidateWindow")
	canvasImmAssociateContextEx   = canvasIMM32.NewProc("ImmAssociateContextEx")
	canvasImmNotifyIME            = canvasIMM32.NewProc("ImmNotifyIME")
)

// canvasTrackMouse is TRACKMOUSEEVENT.
type canvasTrackMouse struct {
	Size      uint32
	Flags     uint32
	Track     uintptr
	HoverTime uint32
}

// canvasCompositionForm is COMPOSITIONFORM.
type canvasCompositionForm struct {
	Style uint32
	Pos   point
	Area  rect
}

// canvasCandidateForm is CANDIDATEFORM.
type canvasCandidateForm struct {
	Index uint32
	Style uint32
	Pos   point
	Area  rect
}

// canvasIMECharPosition is IMECHARPOSITION.
type canvasIMECharPosition struct {
	Size       uint32
	CharPos    uint32
	Pt         point
	LineHeight uint32
	Document   rect
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(canvasCompositionForm{})-28]
	_ = [1]struct{}{}[unsafe.Sizeof(canvasCandidateForm{})-32]
	_ = [1]struct{}{}[unsafe.Sizeof(canvasIMECharPosition{})-36]
)

// Shared system cursors by IDC_* id (LoadCursor handles need no freeing).
var canvasCursors = map[uintptr]uintptr{}

// inputMessage handles input, cursor, focus and IME messages.
func (c *canvas) inputMessage(m uint32, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case canvasWmMouseMove, canvasWmLButtonDown, canvasWmRButtonDown, canvasWmMButtonDown,
		canvasWmLButtonUp, canvasWmRButtonUp, canvasWmMButtonUp:
		if c.touchMouse() { // synthesized from touch, reported as TouchEvent (canvas_touch_windows.go)
			return 0, true
		}
	}
	switch m {
	case canvasWmPointerDown, canvasWmPointerUpdate, canvasWmPointerUp, canvasWmPointerCaptureChanged:
		return 0, c.pointerMessage(m, wp, lp)
	case canvasWmCancelMode:
		c.cancelTouches()
		return 0, false
	case canvasWmMouseMove:
		c.mouseMove(wp, lp)
		return 0, true
	case canvasWmMouseLeave:
		c.mouseLeave()
		return 0, true
	case canvasWmLButtonDown, canvasWmRButtonDown, canvasWmMButtonDown:
		c.button(canvasButtonOf(m), true, wp, lp)
		return 0, true
	case canvasWmLButtonUp, canvasWmRButtonUp, canvasWmMButtonUp:
		c.button(canvasButtonOf(m), false, wp, lp)
		return 0, true
	case canvasWmCaptureChanged:
		c.captureLost(lp)
		return 0, true
	case canvasWmMouseWheel, canvasWmMouseHWheel:
		c.wheel(m == canvasWmMouseHWheel, wp, lp)
		return 0, true
	case canvasWmSetCursor:
		if wp == c.hwnd && loword(lp) == canvasHTClient {
			canvasSetCursorProc.Call(c.hcursor)
			return 1, true
		}
		return 0, false

	case canvasWmKeyDown, canvasWmSysKeyDown:
		if c.skipAltGrCtrl(m, wp, lp) {
			return 0, m == canvasWmKeyDown
		}
		c.key(uint16(wp), lp, true, canvasKeyRepeat(lp))
		return 0, m == canvasWmKeyDown // system keys continue to DefWindowProc
	case canvasWmKeyUp, canvasWmSysKeyUp:
		if c.skipAltGrCtrl(m, wp, lp) {
			return 0, m == canvasWmKeyUp
		}
		c.key(uint16(wp), lp, false, false)
		return 0, m == canvasWmKeyUp
	case canvasWmChar:
		if c.textOn {
			if s, ok := c.chars.feed(uint16(wp)); ok {
				c.input(input.TextInput{Text: s})
			}
		}
		return 0, true

	case canvasWmSetFocus:
		c.hasFocus = true
		c.reportFocus()
		c.placeIME()
		return 0, true
	case canvasWmKillFocus:
		c.hasFocus = false
		c.altGr = false
		c.chars = canvasCharDecoder{}
		c.reportFocus()
		return 0, true

	case canvasWmImeSetContext:
		if wp != 0 {
			lp &^= canvasISCShowUIComposition // the UI draws the composition
		}
		r, _, _ := procDefWindowProcW.Call(c.hwnd, uintptr(m), wp, lp)
		return r, true
	case canvasWmImeStartComp:
		c.placeIME()
		return 0, true // no system composition window
	case canvasWmImeComposition:
		c.composition(lp)
		return 0, true // not DefWindowProc: it would send the result again as WM_IME_CHAR
	case canvasWmImeEndComp:
		if c.marked != "" {
			c.marked = ""
			c.inputText(input.Composition{})
		}
		return 0, true
	case canvasWmImeRequest:
		if wp == canvasIMRQueryCharPos && lp != 0 && c.textOn {
			c.queryCharPosition((*canvasIMECharPosition)(native(lp)))
			return 1, true
		}
		return 0, false
	}
	return 0, false
}

// input delivers ev to the sink.
func (c *canvas) input(ev input.Event) {
	if c.ready && !c.closed {
		c.sinkDepth++
		c.sink.CanvasInput(ev)
		c.sinkDepth--
	}
}

// inputText delivers text and composition events only while text input
// is on: turning it off discards the composition silently, as on macOS.
func (c *canvas) inputText(ev input.Event) {
	if c.textOn {
		c.input(ev)
	}
}

// reportFocus tells the sink when keyboard focus changed.
func (c *canvas) reportFocus() {
	if !c.ready || c.closed || c.hasFocus == c.focused {
		return
	}
	c.focused = c.hasFocus
	c.sinkDepth++
	c.sink.CanvasFocus(c.focused)
	c.sinkDepth--
}

// ---- mouse ----

func canvasButtonOf(m uint32) input.Button {
	switch m {
	case canvasWmRButtonDown, canvasWmRButtonUp:
		return input.ButtonRight
	case canvasWmMButtonDown, canvasWmMButtonUp:
		return input.ButtonMiddle
	}
	return input.ButtonLeft
}

// lparamPoint is the signed client position in a mouse message.
func lparamPoint(lp uintptr) (int32, int32) {
	return int32(int16(loword(lp))), int32(int16(hiword(lp)))
}

func (c *canvas) mouseMods(wp uintptr) input.Modifiers {
	return c.altGrMods(canvasMouseMods(wp, keyDown(canvasVKMenu), keyDown(canvasVKLWin) || keyDown(canvasVKRWin)))
}

func (c *canvas) keyMods() input.Modifiers {
	return c.altGrMods(canvasMods(keyDown(canvasVKShift), keyDown(canvasVKControl), keyDown(canvasVKMenu),
		keyDown(canvasVKLWin) || keyDown(canvasVKRWin)))
}

// altGrMods drops the Ctrl of the synthesized left Ctrl while AltGr is
// held (see skipAltGrCtrl).
func (c *canvas) altGrMods(m input.Modifiers) input.Modifiers {
	return canvasAltGrMods(m, c.altGr && keyDown(canvasVKRMenu), keyDown(canvasVKRControl))
}

// skipAltGrCtrl reports whether a VK_CONTROL key message is the left Ctrl
// Windows synthesizes in front of AltGr (canvasIsAltGrCtrl), looking at
// the next queued keyboard message without removing it. Such a message is
// not reported: AltGr is reported as KeyAltRight alone, and while it is
// held Mods do not include Ctrl.
func (c *canvas) skipAltGrCtrl(m uint32, wp, lp uintptr) bool {
	if uint16(wp) != canvasVKControl {
		return false
	}
	var next msg
	got, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&next)), c.hwnd, canvasWmKeyDown, canvasWmSysKeyUp, canvasPMNoRemove)
	t, _, _ := canvasGetMessageTime.Call()
	cur := canvasKeyMsg{Msg: m, VK: uint16(wp), LP: lp, Time: uint32(t)}
	nx := canvasKeyMsg{Msg: next.Message, VK: uint16(next.WParam), LP: next.LParam, Time: next.Time}
	if !canvasIsAltGrCtrl(cur, nx, got != 0) {
		return false
	}
	c.altGr = m == canvasWmKeyDown || m == canvasWmSysKeyDown
	return true
}

func (c *canvas) over(x, y int32) bool { return x >= 0 && y >= 0 && x < c.pxW && y < c.pxH }

// enter starts tracking the pointer over the client area.
func (c *canvas) enter() {
	c.inside, c.leavePend = true, false
	tme := canvasTrackMouse{Flags: canvasTMELeave, Track: c.hwnd}
	tme.Size = uint32(unsafe.Sizeof(tme))
	canvasTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
	canvasSetCursorProc.Call(c.hcursor)
}

// dragButton is the button reported with Move: the first one held.
func (c *canvas) dragButton() input.Button {
	for b := input.ButtonLeft; b <= input.ButtonMiddle; b++ {
		if c.buttons&(1<<b) != 0 {
			return b
		}
	}
	return input.ButtonLeft
}

func (c *canvas) mouseMove(wp, lp uintptr) {
	x, y := lparamPoint(lp)
	if !c.inside && c.over(x, y) {
		c.enter()
	}
	c.input(input.Pointer{Kind: input.Move, Pos: c.toLogical(x, y), Button: c.dragButton(), Mods: c.mouseMods(wp)})
}

// mouseLeave handles WM_MOUSELEAVE. While a button is held (capture) the
// drag continues outside and Leave is reported on release instead.
func (c *canvas) mouseLeave() {
	if !c.inside {
		return
	}
	c.inside = false
	if c.buttons != 0 {
		c.leavePend = true
		return
	}
	c.input(input.Pointer{Kind: input.Leave, Pos: c.toLogical(c.lastPointer()), Mods: c.keyMods()})
}

// lastPointer is the current cursor position in client pixels.
func (c *canvas) lastPointer() (int32, int32) {
	var p point
	canvasGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	canvasScreenToClient.Call(c.hwnd, uintptr(unsafe.Pointer(&p)))
	return p.X, p.Y
}

func (c *canvas) button(b input.Button, down bool, wp, lp uintptr) {
	x, y := lparamPoint(lp)
	p := input.Pointer{Pos: c.toLogical(x, y), Button: b, Mods: c.mouseMods(wp)}
	bit := uint8(1) << b
	if down {
		if f, _, _ := canvasGetFocus.Call(); f != c.hwnd {
			canvasSetFocus.Call(c.hwnd)
		}
		if c.buttons == 0 {
			canvasSetCapture.Call(c.hwnd) // keep receiving the drag outside the window
		}
		c.buttons |= bit
		if !c.inside && c.over(x, y) {
			c.enter()
		}
		t, _, _ := canvasGetMessageTime.Call()
		maxMS, _, _ := canvasGetDoubleClickTime.Call()
		cx, _, _ := canvasGetSystemMetrics.Call(canvasSMCXDoubleClk)
		cy, _, _ := canvasGetSystemMetrics.Call(canvasSMCYDoubleClk)
		p.Kind = input.Press
		p.Clicks = c.clicks.press(b, uint32(t), x, y, uint32(maxMS), int32(cx), int32(cy))
		c.lastClicks[b] = p.Clicks
		c.input(p)
		return
	}
	if c.buttons&bit == 0 {
		return // pressed elsewhere (or capture was lost): no Release without Press
	}
	c.buttons &^= bit
	if c.buttons == 0 {
		canvasReleaseCapture.Call()
	}
	p.Kind = input.Release
	p.Clicks = c.lastClicks[b]
	c.input(p)
	if c.buttons == 0 && !c.closed && !c.over(x, y) && (c.inside || c.leavePend) {
		c.inside, c.leavePend = false, false
		c.input(input.Pointer{Kind: input.Leave, Pos: c.toLogical(x, y), Mods: c.mouseMods(wp)})
	}
}

// captureLost handles WM_CAPTURECHANGED: another window took the capture
// in the middle of a drag (a menu, a system dialog, Alt+Tab).
func (c *canvas) captureLost(newOwner uintptr) {
	if newOwner == c.hwnd || c.buttons == 0 {
		return
	}
	b := c.dragButton()
	c.buttons = 0
	c.clicks.reset()
	x, y := c.lastPointer()
	c.input(input.Pointer{Kind: input.Cancel, Pos: c.toLogical(x, y), Button: b, Mods: c.keyMods()})
	if !c.closed && c.leavePend {
		c.leavePend = false
		c.input(input.Pointer{Kind: input.Leave, Pos: c.toLogical(x, y), Mods: c.keyMods()})
	}
}

// wheel handles WM_MOUSEWHEEL / WM_MOUSEHWHEEL (position in screen
// coordinates).
func (c *canvas) wheel(horizontal bool, wp, lp uintptr) {
	delta := int16(hiword(wp))
	if delta == 0 {
		return
	}
	pt := point{X: int32(int16(loword(lp))), Y: int32(int16(hiword(lp)))}
	canvasScreenToClient.Call(c.hwnd, uintptr(unsafe.Pointer(&pt)))
	var units uint32 = 3
	spi := uintptr(canvasSPIGetWheelScrollLines)
	if horizontal {
		spi = canvasSPIGetWheelScrollChars
	}
	canvasSystemParametersInfo.Call(spi, 0, uintptr(unsafe.Pointer(&units)), 0)
	page := float32(c.h)
	if horizontal {
		page = float32(c.w)
	}
	dx, dy := canvasScroll(delta, horizontal, units, page)
	if dx == 0 && dy == 0 {
		return
	}
	c.input(input.Scroll{Pos: c.toLogical(pt.X, pt.Y), DX: dx, DY: dy, Mods: c.mouseMods(uintptr(loword(wp)))})
}

// ---- keyboard ----

// key reports a key message. downKeys holds canvasHeldID records of the
// reported presses: a release (and an auto-repeat) is reported with the
// Key and Code of the press, even when NumLock was toggled while a keypad
// key was held and the message now carries the other form.
func (c *canvas) key(vk uint16, lp uintptr, down, repeat bool) {
	k, code := canvasKeyFromMsg(vk, lp)
	if k == input.KeyUnknown && code == 0 {
		return
	}
	mods := c.keyMods()
	if k.Modifier() != 0 {
		if repeat {
			return // a held modifier auto-repeats; reported once
		}
		mods = canvasModifierEventMods(k, down, mods, keyDown(canvasOtherSideVK(k)))
	}
	id, held := canvasHeldFind(c.downKeys, k, code)
	if down {
		if held {
			k, code = canvasHeldKey(id)
		} else {
			c.downKeys[canvasHeldID(k, code)] = true
		}
		c.input(input.KeyEvent{Key: k, Code: code, Down: true, Repeat: repeat, Mods: mods})
		return
	}
	if held {
		delete(c.downKeys, id)
		k, code = canvasHeldKey(id)
		c.input(input.KeyEvent{Key: k, Code: code, Down: false, Mods: mods})
	}
}

// ---- cursor ----

// SetCursor implements platform.Canvas.
func (c *canvas) SetCursor(cur input.Cursor) {
	if c.closed {
		return
	}
	c.cursor = cur
	id := canvasCursorID(cur)
	h, ok := canvasCursors[id]
	if !ok {
		h, _, _ = procLoadCursorW.Call(0, id)
		canvasCursors[id] = h
	}
	c.hcursor = h
	if c.inside {
		canvasSetCursorProc.Call(h)
	}
}

// ---- text input ----

// SetTextInput implements platform.Canvas.
func (c *canvas) SetTextInput(enabled bool, caret geom.Rect) {
	if c.closed {
		return
	}
	was, old := c.textOn, c.caret
	c.caret = caret
	switch {
	case enabled && !was:
		c.textOn = true
		canvasImmAssociateContextEx.Call(c.hwnd, 0, canvasIACEDefault)
	case !enabled && was:
		c.textOn = false // composition messages from the cancel are not reported
		if himc, _, _ := canvasImmGetContext.Call(c.hwnd); himc != 0 {
			canvasImmNotifyIME.Call(himc, canvasNICompositionStr, canvasCPSCancel, 0)
			canvasImmReleaseContext.Call(c.hwnd, himc)
		}
		c.disableIME()
		c.marked = ""
		c.chars = canvasCharDecoder{}
	}
	if enabled && (!was || old != caret) {
		c.placeIME()
	}
}

// disableIME removes the window's input context, so the IME does not
// engage over non-text content.
func (c *canvas) disableIME() { canvasImmAssociateContextEx.Call(c.hwnd, 0, 0) }

// caretRect is the caret in physical client pixels (at least 1 px wide).
func (c *canvas) caretRect() rect {
	x, y := c.toPhysical(c.caret.X), c.toPhysical(c.caret.Y)
	w, h := max(c.toPhysical(c.caret.W), 1), max(c.toPhysical(c.caret.H), 1)
	return rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

// placeIME positions the composition and candidate windows at the caret.
func (c *canvas) placeIME() {
	if !c.textOn || c.closed {
		return
	}
	himc, _, _ := canvasImmGetContext.Call(c.hwnd)
	if himc == 0 {
		return
	}
	defer canvasImmReleaseContext.Call(c.hwnd, himc)
	r := c.caretRect()
	cf := canvasCompositionForm{Style: canvasCFSPoint, Pos: point{r.Left, r.Top}}
	canvasImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&cf)))
	cand := canvasCandidateForm{Index: 0, Style: canvasCFSExclude, Pos: point{r.Left, r.Bottom}, Area: r}
	canvasImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&cand)))
}

// queryCharPosition answers IMR_QUERYCHARPOSITION, which IMEs built on
// TSF use to place the candidate window: every character is at the caret.
func (c *canvas) queryCharPosition(q *canvasIMECharPosition) {
	r := c.caretRect()
	pt := point{r.Left, r.Top}
	canvasClientToScreen.Call(c.hwnd, uintptr(unsafe.Pointer(&pt)))
	q.Pt = pt
	q.LineHeight = uint32(r.Bottom - r.Top)
	tl, br := point{0, 0}, point{c.pxW, c.pxH}
	canvasClientToScreen.Call(c.hwnd, uintptr(unsafe.Pointer(&tl)))
	canvasClientToScreen.Call(c.hwnd, uintptr(unsafe.Pointer(&br)))
	q.Document = rect{Left: tl.X, Top: tl.Y, Right: br.X, Bottom: br.Y}
}

// composition handles WM_IME_COMPOSITION: the result (committed text)
// first, then the new composition string.
func (c *canvas) composition(flags uintptr) {
	himc, _, _ := canvasImmGetContext.Call(c.hwnd)
	if himc == 0 {
		return
	}
	defer canvasImmReleaseContext.Call(c.hwnd, himc)
	if flags&canvasGCSResultStr != 0 {
		text := canvasCleanText(immString(himc, canvasGCSResultStr))
		had := c.marked != ""
		c.marked = ""
		switch {
		case text != "":
			c.inputText(input.TextInput{Text: text})
		case had:
			c.inputText(input.Composition{})
		}
	}
	if c.closed {
		return
	}
	if flags&canvasGCSCompStr != 0 || (flags == 0 && c.marked != "") {
		text := ""
		if flags&canvasGCSCompStr != 0 {
			text = immString(himc, canvasGCSCompStr)
		}
		if text == c.marked && text == "" {
			return
		}
		cur := len([]rune(text))
		if flags&canvasGCSCursorPos != 0 {
			pos, _, _ := canvasImmGetCompositionString.Call(himc, canvasGCSCursorPos, 0, 0)
			if p := int32(pos); p >= 0 {
				cur = canvasRuneIndex(text, int(p))
			}
		}
		c.marked = text
		if text == "" {
			c.inputText(input.Composition{})
			return
		}
		c.inputText(input.Composition{Text: text, Cursor: cur})
	}
}

// immString reads a composition string (GCS_COMPSTR, GCS_RESULTSTR).
func immString(himc uintptr, kind uintptr) string {
	n, _, _ := canvasImmGetCompositionString.Call(himc, kind, 0, 0)
	size := int32(n) // bytes; negative is an IMM_ERROR_* code
	if size <= 0 {
		return ""
	}
	buf := make([]uint16, size/2)
	canvasImmGetCompositionString.Call(himc, kind, uintptr(unsafe.Pointer(&buf[0])), uintptr(size))
	return string(utf16.Decode(buf))
}
