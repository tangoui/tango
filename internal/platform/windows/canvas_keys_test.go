package windows

import (
	"bytes"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

// lp builds a WM_KEYDOWN lParam: repeat count 1, scan code, extended bit.
func lp(scan uint16, ext bool) uintptr {
	v := uintptr(1) | uintptr(scan)<<16
	if ext {
		v |= 1 << 24
	}
	return v
}

func TestCanvasKeyFromMsg(t *testing.T) {
	for _, tc := range []struct {
		vk   uint16
		lp   uintptr
		want input.Key
	}{
		{0x0D, lp(0x1C, false), input.KeyEnter},
		{0x08, lp(0x0E, false), input.KeyBackspace},
		{0x09, lp(0x0F, false), input.KeyTab},
		{0x1B, lp(0x01, false), input.KeyEscape},
		{0x20, lp(0x39, false), input.KeySpace},
		{0x2E, lp(0x53, true), input.KeyDelete},
		{0x25, lp(0x4B, true), input.KeyLeft},
		{0x26, lp(0x48, true), input.KeyUp},
		{0x27, lp(0x4D, true), input.KeyRight},
		{0x28, lp(0x50, true), input.KeyDown},
		{0x24, lp(0x47, true), input.KeyHome},
		{0x23, lp(0x4F, true), input.KeyEnd},
		{0x21, lp(0x49, true), input.KeyPageUp},
		{0x22, lp(0x51, true), input.KeyPageDown},
		{0x25, 1, input.KeyLeft}, // synthesized without a scan code
		{'A', lp(0x1E, false), input.KeyA},
		{'C', lp(0x2E, false), input.KeyC},
		{'Z', lp(0x2C, false), input.KeyZ},
		{'0', lp(0x0B, false), input.Key0},
		{'9', lp(0x0A, false), input.Key9},
		{0xE5, lp(0x1E, false), input.KeyUnknown}, // VK_PROCESSKEY

		// Function keys.
		{0x70, lp(0x3B, false), input.KeyF1}, {0x71, lp(0x3C, false), input.KeyF2},
		{0x72, lp(0x3D, false), input.KeyF3}, {0x73, lp(0x3E, false), input.KeyF4},
		{0x74, lp(0x3F, false), input.KeyF5}, {0x75, lp(0x40, false), input.KeyF6},
		{0x76, lp(0x41, false), input.KeyF7}, {0x77, lp(0x42, false), input.KeyF8},
		{0x78, lp(0x43, false), input.KeyF9}, {0x79, lp(0x44, false), input.KeyF10},
		{0x7A, lp(0x57, false), input.KeyF11}, {0x7B, lp(0x58, false), input.KeyF12},
		{0x7C, lp(0x64, false), input.KeyUnknown}, // VK_F13

		// Modifiers: generic codes split by scan code / extended bit.
		{0x10, lp(0x2A, false), input.KeyShiftLeft},
		{0x10, lp(0x36, false), input.KeyShiftRight},
		{0x10, lp(0x2A, true), input.KeyUnknown}, // fake shift around Shift+keypad
		{0x11, lp(0x1D, false), input.KeyCtrlLeft},
		{0x11, lp(0x1D, true), input.KeyCtrlRight},
		{0x12, lp(0x38, false), input.KeyAltLeft},
		{0x12, lp(0x38, true), input.KeyAltRight},
		{0x5B, lp(0x5B, true), input.KeyMetaLeft},
		{0x5C, lp(0x5C, true), input.KeyMetaRight},
		{0xA0, 0, input.KeyShiftLeft}, {0xA1, 0, input.KeyShiftRight},
		{0xA2, 0, input.KeyCtrlLeft}, {0xA3, 0, input.KeyCtrlRight},
		{0xA4, 0, input.KeyAltLeft}, {0xA5, 0, input.KeyAltRight},

		// Punctuation by scan code, whatever VK_OEM_* the layout sends.
		{0xBD, lp(0x0C, false), input.KeyMinus},
		{0xBB, lp(0x0D, false), input.KeyEqual},
		{0xDB, lp(0x1A, false), input.KeyBracketLeft},
		{0xDD, lp(0x1B, false), input.KeyBracketRight},
		{0xDC, lp(0x2B, false), input.KeyBackslash},
		{0xBA, lp(0x27, false), input.KeySemicolon},
		{0xDE, lp(0x28, false), input.KeyQuote},
		{0xC0, lp(0x29, false), input.KeyBackquote},
		{0xBC, lp(0x33, false), input.KeyComma},
		{0xBE, lp(0x34, false), input.KeyPeriod},
		{0xBF, lp(0x35, false), input.KeySlash},
		{0xBD, lp(0x35, false), input.KeySlash},   // German: '-' on the US slash key
		{'M', lp(0x27, false), input.KeyM},        // AZERTY: a letter VK wins
		{0xBC, lp(0x32, false), input.KeyUnknown}, // AZERTY: ',' on the US M key
		{0xE2, lp(0x56, false), input.KeyUnknown}, // VK_OEM_102 (ISO key): no Key

		// Keypad, NumLock on.
		{0x60, lp(0x52, false), input.KeyNumpad0}, {0x61, lp(0x4F, false), input.KeyNumpad1},
		{0x62, lp(0x50, false), input.KeyNumpad2}, {0x63, lp(0x51, false), input.KeyNumpad3},
		{0x64, lp(0x4B, false), input.KeyNumpad4}, {0x65, lp(0x4C, false), input.KeyNumpad5},
		{0x66, lp(0x4D, false), input.KeyNumpad6}, {0x67, lp(0x47, false), input.KeyNumpad7},
		{0x68, lp(0x48, false), input.KeyNumpad8}, {0x69, lp(0x49, false), input.KeyNumpad9},
		{0x6B, lp(0x4E, false), input.KeyNumpadAdd},
		{0x6D, lp(0x4A, false), input.KeyNumpadSubtract},
		{0x6A, lp(0x37, false), input.KeyNumpadMultiply},
		{0x6F, lp(0x35, true), input.KeyNumpadDivide},
		{0x6E, lp(0x53, false), input.KeyNumpadDecimal},
		{0x0D, lp(0x1C, true), input.KeyNumpadEnter},
		// Keypad, NumLock off: see TestCanvasKeyNumLock.
		{0x0C, lp(0x4C, true), input.KeyUnknown}, // VK_CLEAR elsewhere

		// Others.
		{0x2D, lp(0x52, true), input.KeyInsert},
		{0x14, lp(0x3A, false), input.KeyCapsLock},
		{0x5D, lp(0x5D, true), input.KeyContextMenu},
	} {
		// Everything here, the navigation block included, has no Code.
		if got, code := canvasKeyFromMsg(tc.vk, tc.lp); got != tc.want || code != 0 {
			t.Errorf("canvasKeyFromMsg(%#x, %#x) = %v, %v; want %v, 0", tc.vk, tc.lp, got, code, tc.want)
		}
	}
}

// TestCanvasKeyFromMsgNoScan: input injected without a scan code (wScan 0)
// names a punctuation key through the layout's scan code for its VK_OEM_*
// code; without that mapping it stays unknown.
func TestCanvasKeyFromMsgNoScan(t *testing.T) {
	saved := canvasVKToScan
	t.Cleanup(func() { canvasVKToScan = saved })
	canvasVKToScan = func(vk uint16) uint16 { return map[uint16]uint16{0xBF: 0x35, 0xBD: 0x0C}[vk] }
	for vk, want := range map[uint16]input.Key{0xBF: input.KeySlash, 0xBD: input.KeyMinus, 0xBA: input.KeyUnknown} {
		if got, _ := canvasKeyFromMsg(vk, lp(0, false)); got != want {
			t.Errorf("vk %#x, scan 0: %v, want %v", vk, got, want)
		}
	}
	// A real scan code wins over the mapping.
	if got, _ := canvasKeyFromMsg(0xBF, lp(0x0C, false)); got != input.KeyMinus {
		t.Errorf("vk 0xBF, scan 0x0C: %v, want Minus", got)
	}
	canvasVKToScan = nil
	if got, _ := canvasKeyFromMsg(0xBF, lp(0, false)); got != input.KeyUnknown {
		t.Errorf("no mapping: %v, want Unknown", got)
	}
}

// TestCanvasKeyNumLock covers every keypad key with NumLock on and off:
// on, the Numpad key with no Code; off, the navigation key with the
// Numpad key as Code. Enter and the operators ignore NumLock.
func TestCanvasKeyNumLock(t *testing.T) {
	for _, tc := range []struct {
		scan          uint16
		onVK, offVK   uint16
		onExt, offExt bool
		numpad, nav   input.Key
	}{
		{0x52, 0x60, 0x2D, false, false, input.KeyNumpad0, input.KeyInsert},
		{0x4F, 0x61, 0x23, false, false, input.KeyNumpad1, input.KeyEnd},
		{0x50, 0x62, 0x28, false, false, input.KeyNumpad2, input.KeyDown},
		{0x51, 0x63, 0x22, false, false, input.KeyNumpad3, input.KeyPageDown},
		{0x4B, 0x64, 0x25, false, false, input.KeyNumpad4, input.KeyLeft},
		{0x4C, 0x65, 0x0C, false, false, input.KeyNumpad5, input.KeyUnknown}, // VK_CLEAR: no navigation key
		{0x4D, 0x66, 0x27, false, false, input.KeyNumpad6, input.KeyRight},
		{0x47, 0x67, 0x24, false, false, input.KeyNumpad7, input.KeyHome},
		{0x48, 0x68, 0x26, false, false, input.KeyNumpad8, input.KeyUp},
		{0x49, 0x69, 0x21, false, false, input.KeyNumpad9, input.KeyPageUp},
		{0x53, 0x6E, 0x2E, false, false, input.KeyNumpadDecimal, input.KeyDelete},
		// Not affected by NumLock: the same message either way.
		{0x1C, 0x0D, 0x0D, true, true, input.KeyNumpadEnter, input.KeyNumpadEnter},
		{0x4E, 0x6B, 0x6B, false, false, input.KeyNumpadAdd, input.KeyNumpadAdd},
		{0x4A, 0x6D, 0x6D, false, false, input.KeyNumpadSubtract, input.KeyNumpadSubtract},
		{0x37, 0x6A, 0x6A, false, false, input.KeyNumpadMultiply, input.KeyNumpadMultiply},
		{0x35, 0x6F, 0x6F, true, true, input.KeyNumpadDivide, input.KeyNumpadDivide},
	} {
		if k, code := canvasKeyFromMsg(tc.onVK, lp(tc.scan, tc.onExt)); k != tc.numpad || code != 0 {
			t.Errorf("NumLock on: %#x scan %#x = %v, %v; want %v, 0", tc.onVK, tc.scan, k, code, tc.numpad)
		}
		wantCode := tc.numpad
		if tc.nav == tc.numpad {
			wantCode = 0
		}
		if k, code := canvasKeyFromMsg(tc.offVK, lp(tc.scan, tc.offExt)); k != tc.nav || code != wantCode {
			t.Errorf("NumLock off: %#x scan %#x = %v, %v; want %v, %v", tc.offVK, tc.scan, k, code, tc.nav, wantCode)
		}
		if nav, ok := canvasNumpadNav(tc.numpad); ok != (wantCode != 0) || ok && nav != tc.nav {
			t.Errorf("canvasNumpadNav(%v) = %v, %v", tc.numpad, nav, ok)
		}
	}
}

// TestCanvasHeldKeys checks that a release is reported with the Key and
// Code of its press, also when NumLock changed while the key was held,
// and that the navigation block and keypad records do not mix.
func TestCanvasHeldKeys(t *testing.T) {
	type ev struct{ key, code input.Key }
	held := map[input.Key]bool{}
	press := func(key, code input.Key) ev {
		if id, ok := canvasHeldFind(held, key, code); ok {
			k, c := canvasHeldKey(id)
			return ev{k, c}
		}
		held[canvasHeldID(key, code)] = true
		return ev{key, code}
	}
	release := func(key, code input.Key) (ev, bool) {
		id, ok := canvasHeldFind(held, key, code)
		if !ok {
			return ev{}, false
		}
		delete(held, id)
		k, c := canvasHeldKey(id)
		return ev{k, c}, true
	}
	for _, tc := range []struct {
		name             string
		down, up         ev // the message's mapping at press and release
		wantDown, wantUp ev
	}{
		{"numlock on", ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}},
		{"numlock off", ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}},
		{"off then on", ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyNumpad8, 0}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}},
		{"on then off", ev{input.KeyNumpad0, 0}, ev{input.KeyInsert, input.KeyNumpad0}, ev{input.KeyNumpad0, 0}, ev{input.KeyNumpad0, 0}},
		{"numpad 5 off", ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}},
		{"decimal off then on", ev{input.KeyDelete, input.KeyNumpadDecimal}, ev{input.KeyNumpadDecimal, 0}, ev{input.KeyDelete, input.KeyNumpadDecimal}, ev{input.KeyDelete, input.KeyNumpadDecimal}},
		{"main Up", ev{input.KeyUp, 0}, ev{input.KeyUp, 0}, ev{input.KeyUp, 0}, ev{input.KeyUp, 0}},
	} {
		if got := press(tc.down.key, tc.down.code); got != tc.wantDown {
			t.Errorf("%s: press = %+v, want %+v", tc.name, got, tc.wantDown)
		}
		if got := press(tc.down.key, tc.down.code); got != tc.wantDown { // auto-repeat
			t.Errorf("%s: repeat = %+v, want %+v", tc.name, got, tc.wantDown)
		}
		if got, ok := release(tc.up.key, tc.up.code); !ok || got != tc.wantUp {
			t.Errorf("%s: release = %+v, %v; want %+v", tc.name, got, ok, tc.wantUp)
		}
		if len(held) != 0 {
			t.Errorf("%s: still held: %v", tc.name, held)
		}
	}
	// The main Up and keypad 8 (NumLock off) held together are two keys.
	press(input.KeyUp, 0)
	press(input.KeyUp, input.KeyNumpad8)
	if got, ok := release(input.KeyUp, input.KeyNumpad8); !ok || got != (ev{input.KeyUp, input.KeyNumpad8}) {
		t.Errorf("keypad 8 release = %+v, %v", got, ok)
	}
	if got, ok := release(input.KeyUp, 0); !ok || got != (ev{input.KeyUp, 0}) {
		t.Errorf("main Up release = %+v, %v", got, ok)
	}
	if _, ok := release(input.KeyUp, 0); ok {
		t.Error("release without press reported")
	}
}

func TestCanvasModifierEventMods(t *testing.T) {
	// Press: the modifier is in Mods; release: not, unless the other side is held.
	if m := canvasModifierEventMods(input.KeyShiftLeft, true, 0, false); m != input.ModShift {
		t.Fatalf("shift down = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyShiftLeft, false, input.ModShift, false); m != 0 {
		t.Fatalf("shift up = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyShiftLeft, false, input.ModShift, true); m != input.ModShift {
		t.Fatalf("shift up, other held = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyMetaRight, true, input.ModCtrl, false); m != input.ModCtrl|input.ModMeta {
		t.Fatalf("meta down with ctrl = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyAltRight, false, input.ModAlt|input.ModCtrl, false); m != input.ModCtrl {
		t.Fatalf("alt up with ctrl = %v", m)
	}
	for k, vk := range map[input.Key]int{
		input.KeyShiftLeft: 0xA1, input.KeyShiftRight: 0xA0, input.KeyCtrlLeft: 0xA3, input.KeyCtrlRight: 0xA2,
		input.KeyAltLeft: 0xA5, input.KeyAltRight: 0xA4, input.KeyMetaLeft: 0x5C, input.KeyMetaRight: 0x5B,
		input.KeyA: 0,
	} {
		if got := canvasOtherSideVK(k); got != vk {
			t.Errorf("canvasOtherSideVK(%v) = %#x, want %#x", k, got, vk)
		}
	}
}

func TestCanvasModifiers(t *testing.T) {
	if m := canvasMods(true, false, true, true); m != input.ModShift|input.ModAlt|input.ModMeta {
		t.Fatalf("mods = %v", m)
	}
	// MK_LBUTTON (1) | MK_SHIFT | MK_CONTROL
	if m := canvasMouseMods(0x1|0x4|0x8, false, false); m != input.ModShift|input.ModCtrl {
		t.Fatalf("mouse mods = %v", m)
	}
	if m := canvasMouseMods(0x1, true, false); m != input.ModAlt {
		t.Fatalf("mouse alt = %v", m)
	}
	if !canvasKeyRepeat(1<<30|1) || canvasKeyRepeat(1) {
		t.Fatal("repeat bit")
	}
}

func TestCanvasCursorID(t *testing.T) {
	want := map[input.Cursor]uintptr{
		input.CursorDefault: 32512, input.CursorPointer: 32649, input.CursorText: 32513,
		input.CursorNotAllowed: 32648, input.CursorResizeEW: 32644, input.CursorResizeNS: 32645,
		input.CursorGrab: 32649, input.CursorGrabbing: 32646, input.CursorMove: 32646,
		input.Cursor(200): 32512,
	}
	for c, id := range want {
		if got := canvasCursorID(c); got != id {
			t.Errorf("canvasCursorID(%d) = %d, want %d", c, got, id)
		}
	}
}

func TestCanvasScroll(t *testing.T) {
	// One notch away from the user, 3 lines: reveal content above.
	if dx, dy := canvasScroll(120, false, 3, 600); dx != 0 || dy != -3*canvasScrollLine {
		t.Fatalf("notch up = %v %v", dx, dy)
	}
	// Towards the user: reveal content below.
	if _, dy := canvasScroll(-120, false, 3, 600); dy != 3*canvasScrollLine {
		t.Fatalf("notch down = %v", dy)
	}
	// High-resolution wheel / touchpad: a fraction of a notch.
	if _, dy := canvasScroll(-30, false, 3, 600); dy != 30 {
		t.Fatalf("fraction = %v", dy)
	}
	// Page scrolling.
	if _, dy := canvasScroll(-120, false, canvasWheelPage, 600); dy != 600 {
		t.Fatalf("page = %v", dy)
	}
	// Tilt right reveals content to the right.
	if dx, dy := canvasScroll(120, true, 3, 800); dx != 3*canvasScrollLine || dy != 0 {
		t.Fatalf("tilt = %v %v", dx, dy)
	}
	if _, dy := canvasScroll(120, false, 0, 600); dy != 0 {
		t.Fatalf("zero lines = %v", dy)
	}
}

func TestCanvasClickCounter(t *testing.T) {
	var cc canvasClickCounter
	const maxMS, cx, cy = 500, 4, 4
	if n := cc.press(input.ButtonLeft, 1000, 10, 10, maxMS, cx, cy); n != 1 {
		t.Fatalf("first = %d", n)
	}
	if n := cc.press(input.ButtonLeft, 1200, 11, 9, maxMS, cx, cy); n != 2 {
		t.Fatalf("double = %d", n)
	}
	if n := cc.press(input.ButtonLeft, 1400, 12, 10, maxMS, cx, cy); n != 3 {
		t.Fatalf("triple = %d", n)
	}
	if n := cc.press(input.ButtonLeft, 2000, 12, 10, maxMS, cx, cy); n != 1 {
		t.Fatalf("too slow = %d", n)
	}
	if n := cc.press(input.ButtonLeft, 2100, 20, 10, maxMS, cx, cy); n != 1 {
		t.Fatalf("too far = %d", n)
	}
	if n := cc.press(input.ButtonRight, 2200, 20, 10, maxMS, cx, cy); n != 1 {
		t.Fatalf("other button = %d", n)
	}
	// GetMessageTime wraps around.
	cc.reset()
	cc.press(input.ButtonLeft, 0xFFFFFF00, 0, 0, maxMS, cx, cy)
	if n := cc.press(input.ButtonLeft, 0x10, 0, 0, maxMS, cx, cy); n != 2 {
		t.Fatalf("wrap = %d", n)
	}
	cc.reset()
	if n := cc.press(input.ButtonLeft, 0x20, 0, 0, maxMS, cx, cy); n != 1 {
		t.Fatalf("after reset = %d", n)
	}
}

func TestCanvasCharDecoder(t *testing.T) {
	var d canvasCharDecoder
	feed := func(units ...uint16) string {
		var out string
		for _, u := range units {
			if s, ok := d.feed(u); ok {
				out += s
			}
		}
		return out
	}
	if got := feed('a', 0x4F60 /* 你 */, 0xD83D, 0xDE00 /* 😀 */, 'b'); got != "a你😀b" {
		t.Fatalf("decode = %q", got)
	}
	// Control characters: Backspace, Tab, Enter, Esc, Ctrl+C, DEL.
	if got := feed(0x08, 0x09, 0x0D, 0x1B, 0x03, 0x7F); got != "" {
		t.Fatalf("controls = %q", got)
	}
	// Lone surrogates are dropped.
	if got := feed(0xDE00, 'x', 0xD83D, 'y'); got != "xy" {
		t.Fatalf("lone = %q", got)
	}
}

func TestCanvasText(t *testing.T) {
	s := "a你😀b" // 1 + 1 + 2 + 1 UTF-16 units
	for u16, want := range map[int]int{0: 0, 1: 1, 2: 2, 4: 3, 5: 4, 99: 4} {
		if got := canvasRuneIndex(s, u16); got != want {
			t.Errorf("canvasRuneIndex(%d) = %d, want %d", u16, got, want)
		}
	}
	if got := canvasCleanText("a\r\nb\x7fc"); got != "abc" {
		t.Fatalf("clean = %q", got)
	}
}

func TestCanvasRGBAToBGRA(t *testing.T) {
	// 2×2 source with a padded stride of 12 bytes.
	src := []byte{
		1, 2, 3, 4, 5, 6, 7, 8, 0xEE, 0xEE, 0xEE, 0xEE,
		9, 10, 11, 12, 13, 14, 15, 16, 0xEE, 0xEE, 0xEE, 0xEE,
	}
	dst := make([]byte, 16)
	canvasRGBAToBGRA(dst, 8, src, 12, 2, 2)
	want := []byte{3, 2, 1, 4, 7, 6, 5, 8, 11, 10, 9, 12, 15, 14, 13, 16}
	if !bytes.Equal(dst, want) {
		t.Fatalf("swizzle = %v", dst)
	}
	if c := canvasColorRef(0x11, 0x22, 0x33); c != 0x00332211 {
		t.Fatalf("colorref = %#x", c)
	}
}

func TestCanvasRefreshPeriod(t *testing.T) {
	if p := canvasRefreshPeriod(0); p != time.Second/60 {
		t.Fatalf("default = %v", p)
	}
	if p := canvasRefreshPeriod(1); p != time.Second/60 {
		t.Fatalf("hardware default = %v", p)
	}
	if p := canvasRefreshPeriod(144); p != time.Second/144 {
		t.Fatalf("144 Hz = %v", p)
	}
}

func TestCanvasRoleKey(t *testing.T) {
	for role, want := range map[string]input.Key{
		"undo": input.KeyZ, "redo": input.KeyY, "cut": input.KeyX,
		"copy": input.KeyC, "paste": input.KeyV, "selectAll": input.KeyA,
	} {
		if k, m, ok := canvasRoleKey(role); !ok || k != want || m != input.ModCtrl {
			t.Errorf("canvasRoleKey(%q) = %v %v %v", role, k, m, ok)
		}
	}
	if _, _, ok := canvasRoleKey("quit"); ok {
		t.Fatal("quit mapped")
	}
}

func TestCanvasAltGr(t *testing.T) {
	const ctrlScan, altScan = 0x1D, 0x38
	fakeCtrl := canvasKeyMsg{Msg: canvasMsgKeyDown, VK: canvasVKControl, LP: lp(ctrlScan, false), Time: 1000}
	rAlt := canvasKeyMsg{Msg: canvasMsgKeyDown, VK: canvasVKMenu, LP: lp(altScan, true), Time: 1000}
	if !canvasIsAltGrCtrl(fakeCtrl, rAlt, true) {
		t.Error("AltGr press: fake LCtrl not recognised")
	}
	// Release: WM_KEYUP VK_CONTROL then WM_SYSKEYUP / WM_KEYUP VK_MENU.
	upCtrl, upAlt := fakeCtrl, rAlt
	upCtrl.Msg, upAlt.Msg = canvasMsgKeyUp, canvasMsgSysKeyUp
	upCtrl.Time, upAlt.Time = 2000, 2000
	if !canvasIsAltGrCtrl(upCtrl, upAlt, true) {
		t.Error("AltGr release: fake LCtrl up not recognised")
	}
	for name, c := range map[string]struct {
		cur, next canvasKeyMsg
		has       bool
	}{
		"no next message":    {fakeCtrl, canvasKeyMsg{}, false},
		"different time":     {fakeCtrl, canvasKeyMsg{rAlt.Msg, rAlt.VK, rAlt.LP, 1016}, true},
		"left alt":           {fakeCtrl, canvasKeyMsg{rAlt.Msg, rAlt.VK, lp(altScan, false), 1000}, true},
		"right ctrl":         {canvasKeyMsg{fakeCtrl.Msg, canvasVKControl, lp(ctrlScan, true), 1000}, rAlt, true},
		"next is a letter":   {fakeCtrl, canvasKeyMsg{canvasMsgKeyDown, 'Q', lp(0x10, false), 1000}, true},
		"down then up":       {fakeCtrl, upAlt, true},
		"not a key message":  {canvasKeyMsg{0x0102, canvasVKControl, lp(ctrlScan, false), 1000}, rAlt, true},
		"alt is current msg": {rAlt, rAlt, true},
	} {
		if canvasIsAltGrCtrl(c.cur, c.next, c.has) {
			t.Errorf("%s: recognised as AltGr", name)
		}
	}
	// AltGr reports KeyAltRight (extended VK_MENU).
	if k, _ := canvasKeyFromMsg(canvasVKMenu, lp(altScan, true)); k != input.KeyAltRight {
		t.Errorf("AltGr key = %v, want KeyAltRight", k)
	}
	all := input.ModCtrl | input.ModAlt | input.ModShift
	if got := canvasAltGrMods(all, true, false); got != input.ModAlt|input.ModShift {
		t.Errorf("AltGr held mods = %v", got)
	}
	if got := canvasAltGrMods(all, true, true); got != all {
		t.Errorf("AltGr + right Ctrl mods = %v", got)
	}
	if got := canvasAltGrMods(all, false, false); got != all {
		t.Errorf("no AltGr mods = %v", got)
	}
}

func TestCanvasDropEffect(t *testing.T) {
	for allowed, want := range map[uint32]uint32{0: 0, 1: 1, 2: 0, 3: 1, 4: 4, 6: 4, 7: 1} {
		if got := canvasDropEffect(allowed); got != want {
			t.Errorf("canvasDropEffect(%d) = %d, want %d", allowed, got, want)
		}
	}
}

func TestCanvasSeeThrough(t *testing.T) {
	for _, a := range []platform.Appearance{
		{}, {Transparent: true}, {Material: platform.MaterialBlur}, {Material: platform.MaterialMica},
		{Background: 0x112233FF}, {TitleBar: platform.TitleBarNone}, {TitleBar: platform.TitleBarNone, Transparent: true},
	} {
		// See-through exactly where the frame covers the whole client area.
		glass := frameMargins(a) == [4]int32{-1, -1, -1, -1}
		if got := canvasSeeThrough(a); got != glass {
			t.Errorf("canvasSeeThrough(%+v) = %v, frame margins %v", a, got, frameMargins(a))
		}
	}
}
