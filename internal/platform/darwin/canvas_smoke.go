//go:build darwin && !ios

package darwin

// Test hooks for internal/platform/darwin/internal/smokecanvas: synthetic
// AppKit events and NSTextInputClient calls sent straight to a canvas
// view, so the smoke test exercises the event translation without a real
// mouse, keyboard or input method.

import (
	"errors"
	"fmt"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
)

var (
	smokeOnce sync.Once

	msgMouseEvent func(cls id, s objc.SEL, typ uint, loc nsPoint, flags uint, ts float64, wn int, ctx id, evNum, clicks int, pressure float32) id
	msgSetMarked  func(obj id, s objc.SEL, str id, selected, replace nsRange)
	msgInsertText func(obj id, s objc.SEL, str id, replace nsRange)
	msgGetRange   func(obj id, s objc.SEL) nsRange
	msgFirstRect  func(obj id, s objc.SEL, r nsRange, actual *nsRange) nsRect

	cgEventCreateKeyboard     func(src uintptr, code uint16, down bool) uintptr
	cgEventSetFlags           func(ev uintptr, flags uint64)
	cgEventKeyboardSetUnicode func(ev uintptr, n uint, chars *uint16)
	cgEventCreateScroll2      func(src uintptr, units uint32, count uint32, w1, w2, w3 int32) uintptr
)

func loadSmoke() {
	smokeOnce.Do(func() {
		purego.RegisterFunc(&msgMouseEvent, msgSendAddr)
		purego.RegisterFunc(&msgSetMarked, msgSendAddr)
		purego.RegisterFunc(&msgInsertText, msgSendAddr)
		purego.RegisterFunc(&msgGetRange, msgSendAddr)
		stret := msgSendAddr
		if isAMD64 {
			stret = mustDlsym(libObjC, "objc_msgSend_stret")
		}
		purego.RegisterFunc(&msgFirstRect, stret)
		libCG := mustDlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		purego.RegisterLibFunc(&cgEventCreateKeyboard, libCG, "CGEventCreateKeyboardEvent")
		purego.RegisterLibFunc(&cgEventSetFlags, libCG, "CGEventSetFlags")
		purego.RegisterLibFunc(&cgEventKeyboardSetUnicode, libCG, "CGEventKeyboardSetUnicodeString")
		purego.RegisterLibFunc(&cgEventCreateScroll2, libCG, "CGEventCreateScrollWheelEvent2") // macOS 13+
	})
}

// SmokeCanvasInput sends synthetic input to cv (from NewCanvasSurface) on
// the main thread. Events reach the canvas's sink synchronously; the
// returned error covers what the hook checks itself (NSTextInputClient
// return values, the candidate window rectangle). For the internal smoke
// test only.
func (b *backend) SmokeCanvasInput(cv platform.Canvas) (err error) {
	c, ok := cv.(*canvas)
	if !ok || c.closed {
		return errors.New("not an open darwin canvas")
	}
	loadSmoke()
	var errs []error
	check := func(cond bool, format string, args ...any) {
		if !cond {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	withPool(func() {
		v := c.view
		wn := sendInt(c.s.win, "windowNumber")
		mouse := func(sel string, typ uint, x, y float64, clicks int, flags uint) {
			p := msgConvertPoint(v, objc.RegisterName("convertPoint:toView:"), nsPoint{x, y}, 0)
			ev := msgMouseEvent(class("NSEvent"), objc.RegisterName("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
				typ, p, flags, 0, wn, 0, 0, clicks, 1)
			check(ev != 0, "mouseEventWithType %s returned nil", sel)
			if ev != 0 {
				send(v, sel, uintptr(ev))
			}
		}
		// Pointer: double click with Shift at (100, 50), a move, a right click.
		const leftDown, leftUp, rightDown, rightUp, moved = 1, 2, 3, 4, 5
		mouse("mouseMoved:", moved, 30, 20, 0, 0)
		mouse("mouseDown:", leftDown, 100, 50, 2, nsShiftKeyMask)
		mouse("mouseUp:", leftUp, 100, 50, 2, nsShiftKeyMask)
		mouse("rightMouseDown:", rightDown, 10, 10, 1, 0)
		mouse("rightMouseUp:", rightUp, 10, 10, 1, 0)

		// Scroll: 30 px and 1 line "up" on the wheel = reveal content below.
		for _, u := range []struct {
			units uint32
			delta int32
		}{{0, -30}, {1, -1}} {
			if cg := cgEventCreateScroll2(0, u.units, 1, u.delta, 0, 0); cg != 0 {
				ev := send(class("NSEvent"), "eventWithCGEvent:", cg)
				send(v, "scrollWheel:", uintptr(ev))
				cfRelease(cg)
			}
		}

		key := func(sel string, code uint16, down bool, ch string, flags uint64) {
			cg := cgEventCreateKeyboard(0, code, down)
			if cg == 0 {
				check(false, "CGEventCreateKeyboardEvent failed")
				return
			}
			cgEventSetFlags(cg, flags)
			if ch != "" {
				u := utf16.Encode([]rune(ch))
				cgEventKeyboardSetUnicode(cg, uint(len(u)), &u[0])
			}
			ev := send(class("NSEvent"), "eventWithCGEvent:", cg)
			send(v, sel, uintptr(ev))
			cfRelease(cg)
		}
		// Keys with text input off: Left, then Cmd+C.
		c.SetTextInput(false, geom.Rect{})
		check(send(v, "inputContext") == 0, "inputContext not nil with text input off")
		key("keyDown:", 0x7B, true, "", 0)
		key("keyUp:", 0x7B, false, "", 0)
		key("keyDown:", 0x08, true, "c", nsCommandKeyMask)
		key("keyUp:", 0x08, false, "c", nsCommandKeyMask)
		// New keys: F5, Left Shift on its own (flagsChanged:), keypad 1,
		// slash.
		key("keyDown:", 0x60, true, "\uF708", 0)
		key("keyUp:", 0x60, false, "\uF708", 0)
		key("flagsChanged:", 0x38, true, "", nsShiftKeyMask|nxDeviceLShiftKeyMask)
		key("flagsChanged:", 0x38, false, "", 0)
		key("keyDown:", 0x53, true, "1", 0)
		key("keyUp:", 0x53, false, "1", 0)
		key("keyDown:", 0x2C, true, "/", 0)
		key("keyUp:", 0x2C, false, "/", 0)

		// Text input on: the NSTextInputClient protocol.
		c.SetTextInput(true, geom.R(30, 40, 2, 18))
		check(sendBool(v, "conformsToProtocol:", uintptr(unsafe.Pointer(objc.GetProtocol("NSTextInputClient")))), "view does not conform to NSTextInputClient")
		check(send(v, "inputContext") != 0, "inputContext nil with text input on")
		notFound := nsRange{nsNotFound, 0}
		msgSetMarked(v, objc.RegisterName("setMarkedText:selectedRange:replacementRange:"), nsString("ni hao"), nsRange{6, 0}, notFound)
		check(sendBool(v, "hasMarkedText"), "hasMarkedText false after setMarkedText")
		if r := msgGetRange(v, objc.RegisterName("markedRange")); r != (nsRange{0, 6}) {
			check(false, "markedRange = %v, want {0 6}", r)
		}
		if r := msgGetRange(v, objc.RegisterName("selectedRange")); r != (nsRange{6, 0}) {
			check(false, "selectedRange = %v, want {6 0}", r)
		}
		var actual nsRange
		got := msgFirstRect(v, objc.RegisterName("firstRectForCharacterRange:actualRange:"), nsRange{0, 6}, &actual)
		frame := msgRect(c.s.win, objc.RegisterName("frame"))
		content := msgContentRectForRect(c.s.win, objc.RegisterName("contentRectForFrameRect:"), frame)
		want := nsRect{Origin: nsPoint{content.Origin.X + 30, content.Origin.Y + content.Size.Height - 40 - 18}, Size: nsSize{2, 18}}
		near := func(a, b float64) bool { return a-b < 0.5 && b-a < 0.5 }
		check(near(got.Origin.X, want.Origin.X) && near(got.Origin.Y, want.Origin.Y) && near(got.Size.Width, 2) && near(got.Size.Height, 18),
			"firstRectForCharacterRange = %+v, want %+v", got, want)
		check(actual == (nsRange{0, 6}), "actualRange = %v, want {0 6}", actual)
		msgInsertText(v, objc.RegisterName("insertText:replacementRange:"), nsString("你好"), notFound)
		check(!sendBool(v, "hasMarkedText"), "hasMarkedText true after insertText")
		send(v, "doCommandBySelector:", uintptr(objc.RegisterName("insertNewline:")))
		// F6 with text input on goes through interpretKeyEvents: and comes
		// back as a private-use insertText:, reported as the key.
		key("keyDown:", 0x61, true, "\uF709", 0)
		key("keyUp:", 0x61, false, "\uF709", 0)
		c.SetTextInput(false, geom.Rect{})
	})
	return errors.Join(errs...)
}
