package linux

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Go mirrors of the GTK 3 event structs (gdkevents.h), laid out by the Go
// compiler with the same natural-alignment rules a C compiler uses on LP64
// targets. The offset constants in canvas_keys.go must match them.
type (
	canvasCGdkEventAny struct {
		typ       int32
		window    uintptr
		sendEvent int8
	}
	canvasCGdkEventButton struct {
		typ          int32
		window       uintptr
		sendEvent    int8
		time         uint32
		x, y         float64
		axes         uintptr
		state        uint32
		button       uint32
		device       uintptr
		xRoot, yRoot float64
	}
	canvasCGdkEventMotion struct {
		typ          int32
		window       uintptr
		sendEvent    int8
		time         uint32
		x, y         float64
		axes         uintptr
		state        uint32
		isHint       int16
		device       uintptr
		xRoot, yRoot float64
	}
	canvasCGdkEventCrossing struct {
		typ          int32
		window       uintptr
		sendEvent    int8
		subwindow    uintptr
		time         uint32
		x, y         float64
		xRoot, yRoot float64
		mode         int32
		detail       int32
		focus        int32
		state        uint32
	}
	canvasCGdkEventScroll struct {
		typ            int32
		window         uintptr
		sendEvent      int8
		time           uint32
		x, y           float64
		state          uint32
		direction      int32
		device         uintptr
		xRoot, yRoot   float64
		deltaX, deltaY float64
		isStop         uint32
	}
	canvasCGdkEventKey struct {
		typ        int32
		window     uintptr
		sendEvent  int8
		time       uint32
		state      uint32
		keyval     uint32
		length     int32
		str        uintptr
		hwKeycode  uint16
		group      uint8
		isModifier uint32
	}
)

func TestCanvasEventOffsets(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("offsets are for LP64 targets")
	}
	var (
		b canvasCGdkEventButton
		m canvasCGdkEventMotion
		c canvasCGdkEventCrossing
		s canvasCGdkEventScroll
		k canvasCGdkEventKey
		a canvasCGdkEventAny
	)
	for _, tc := range []struct {
		name      string
		got, want uintptr
	}{
		{"any.type", unsafe.Offsetof(a.typ), canvasEvTypeOffset},
		{"button.time", unsafe.Offsetof(b.time), canvasEvButtonTime},
		{"button.x", unsafe.Offsetof(b.x), canvasEvButtonX},
		{"button.y", unsafe.Offsetof(b.y), canvasEvButtonY},
		{"button.state", unsafe.Offsetof(b.state), canvasEvButtonState},
		{"button.button", unsafe.Offsetof(b.button), canvasEvButtonButton},
		{"motion.x", unsafe.Offsetof(m.x), canvasEvMotionX},
		{"motion.y", unsafe.Offsetof(m.y), canvasEvMotionY},
		{"motion.state", unsafe.Offsetof(m.state), canvasEvMotionState},
		{"crossing.x", unsafe.Offsetof(c.x), canvasEvCrossingX},
		{"crossing.y", unsafe.Offsetof(c.y), canvasEvCrossingY},
		{"crossing.state", unsafe.Offsetof(c.state), canvasEvCrossingState},
		{"scroll.x", unsafe.Offsetof(s.x), canvasEvScrollX},
		{"scroll.y", unsafe.Offsetof(s.y), canvasEvScrollY},
		{"scroll.state", unsafe.Offsetof(s.state), canvasEvScrollState},
		{"scroll.direction", unsafe.Offsetof(s.direction), canvasEvScrollDirection},
		{"scroll.delta_x", unsafe.Offsetof(s.deltaX), canvasEvScrollDeltaX},
		{"scroll.delta_y", unsafe.Offsetof(s.deltaY), canvasEvScrollDeltaY},
		{"key.state", unsafe.Offsetof(k.state), canvasEvKeyState},
		{"key.keyval", unsafe.Offsetof(k.keyval), canvasEvKeyKeyval},
		{"key.hardware_keycode", unsafe.Offsetof(k.hwKeycode), canvasEvKeyHWCode},
		{"key.group", unsafe.Offsetof(k.group), canvasEvKeyGroup},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: offset %d, constant %d", tc.name, tc.got, tc.want)
		}
	}
	// Struct sizes as documented by sizeof() on GTK 3 / LP64.
	for _, tc := range []struct {
		name      string
		got, want uintptr
	}{
		{"GdkEventButton", unsafe.Sizeof(b), 80},
		{"GdkEventMotion", unsafe.Sizeof(m), 80},
		{"GdkEventCrossing", unsafe.Sizeof(c), 88},
		{"GdkEventScroll", unsafe.Sizeof(s), 96},
		{"GdkEventKey", unsafe.Sizeof(k), 56},
	} {
		if tc.got != tc.want {
			t.Errorf("sizeof(%s) = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestCanvasModsFromGDK(t *testing.T) {
	if got := canvasMods(canvasGdkShiftMask | canvasGdkControlMask | canvasGdkLockMask | 1<<4); got != input.ModShift|input.ModCtrl {
		t.Fatalf("shift+ctrl (+caps, numlock) = %v", got)
	}
	if got := canvasMods(canvasGdkMod1Mask | canvasGdkMod4Mask); got != input.ModAlt|input.ModMeta {
		t.Fatalf("alt+mod4 = %v", got)
	}
	if got := canvasMods(canvasGdkSuperMask | 1<<28 /* META */); got != input.ModMeta {
		t.Fatalf("super(+meta) = %v", got)
	}
}

func TestCanvasKeyFromGDK(t *testing.T) {
	const hwA = 38 // the US 'a' key: not in canvasHWKeys
	for _, tc := range []struct {
		keyval uint32
		hw     uint16
		want   input.Key
	}{
		{0xff0d, 36, input.KeyEnter},
		{0xff09, 23, input.KeyTab}, {0xfe20, 23, input.KeyTab},
		{0xff1b, 9, input.KeyEscape}, {0xff08, 22, input.KeyBackspace},
		{0xffff, 119, input.KeyDelete},
		{0xff51, 113, input.KeyLeft}, {0xff52, 111, input.KeyUp}, {0xff53, 114, input.KeyRight}, {0xff54, 116, input.KeyDown},
		{0xff50, 110, input.KeyHome}, {0xff57, 115, input.KeyEnd},
		{0xff55, 112, input.KeyPageUp}, {0xff56, 117, input.KeyPageDown},
		{0x20, 65, input.KeySpace},
		{'a', hwA, input.KeyA}, {'C', 54, input.KeyC}, {'z', 52, input.KeyZ}, {'7', 16, input.Key7},
		{0x6c1, hwA, input.KeyUnknown}, // Cyrillic_a (re-translated by latinKey)
		{'-', hwA, input.KeyUnknown},   // punctuation on an unknown position

		// Function keys.
		{0xffbe, 67, input.KeyF1}, {0xffbf, 68, input.KeyF2}, {0xffc0, 69, input.KeyF3},
		{0xffc1, 70, input.KeyF4}, {0xffc2, 71, input.KeyF5}, {0xffc3, 72, input.KeyF6},
		{0xffc4, 73, input.KeyF7}, {0xffc5, 74, input.KeyF8}, {0xffc6, 75, input.KeyF9},
		{0xffc7, 76, input.KeyF10}, {0xffc8, 95, input.KeyF11}, {0xffc9, 96, input.KeyF12},
		{0xffca, 191, input.KeyUnknown}, // F13

		// Modifiers.
		{0xffe1, 50, input.KeyShiftLeft}, {0xffe2, 62, input.KeyShiftRight},
		{0xffe3, 37, input.KeyCtrlLeft}, {0xffe4, 105, input.KeyCtrlRight},
		{0xffe9, 64, input.KeyAltLeft}, {0xffea, 108, input.KeyAltRight},
		{0xffe7, 64, input.KeyAltLeft}, {0xffe8, 108, input.KeyAltRight}, // Meta_L/R: Mod1
		{0xffeb, 133, input.KeyMetaLeft}, {0xffec, 134, input.KeyMetaRight},

		// Punctuation by hardware keycode, whatever the keyval.
		{'-', 20, input.KeyMinus}, {'=', 21, input.KeyEqual},
		{'[', 34, input.KeyBracketLeft}, {']', 35, input.KeyBracketRight},
		{'\\', 51, input.KeyBackslash}, {';', 47, input.KeySemicolon},
		{'\'', 48, input.KeyQuote}, {'`', 49, input.KeyBackquote},
		{',', 59, input.KeyComma}, {'.', 60, input.KeyPeriod}, {'/', 61, input.KeySlash},
		{'?', 61, input.KeySlash},        // Shift held
		{'-', 61, input.KeySlash},        // German: '-' on the US slash key
		{0xfc, 34, input.KeyBracketLeft}, // German udiaeresis on the US '[' key
		{0x6c6, 47, input.KeySemicolon},  // Cyrillic_zhe on the US ';' key
		{'m', 47, input.KeyM},            // AZERTY: a letter wins (layout-aware shortcuts)
		{',', 58, input.KeyUnknown},      // AZERTY: ',' on the US M key is not KeyComma
		// Digits: by character, by position when the character is not one.
		{'&', 10, input.Key1}, {0xe0, 19, input.Key0}, {'!', 10, input.Key1},

		// Keypad: NumLock on and off, see TestCanvasKeyNumLock.
		{0xffac, 91, input.KeyNumpadDecimal}, // KP_Separator
		{0xffbd, 125, input.KeyUnknown},      // KP_Equal
		// Keypad by keycode when the keyval is unusual.
		{0xffffff, 87, input.KeyNumpad1}, {0xffffff, 104, input.KeyNumpadEnter},

		// Others.
		{0xff63, 118, input.KeyInsert}, {0xffe5, 66, input.KeyCapsLock}, {0xff67, 135, input.KeyContextMenu},
	} {
		// Everything here, the navigation block included, has no Code.
		if got, code := canvasKey(tc.keyval, tc.hw); got != tc.want || code != 0 {
			t.Errorf("canvasKey(%#x, %d) = %v, %v; want %v, 0", tc.keyval, tc.hw, got, code, tc.want)
		}
	}
	for keyval, want := range map[uint32]bool{
		'a': false, 0xe9: false, 0xff0d: false, 0x6c1: true, 0x7e1: true, 0x010003b1: true, 0xffbe: false,
	} {
		if got := canvasNeedsLatinFallback(keyval); got != want {
			t.Errorf("canvasNeedsLatinFallback(%#x) = %v", keyval, got)
		}
	}
}

// TestCanvasKeyNumLock covers every keypad key with NumLock on and off:
// on (KP_0 ...), the Numpad key with no Code; off (KP_Insert ...), the
// navigation key with the Numpad key of the hardware keycode as Code.
// Enter and the operators ignore NumLock.
func TestCanvasKeyNumLock(t *testing.T) {
	for _, tc := range []struct {
		hw          uint16
		onKV, offKV uint32
		numpad, nav input.Key
	}{
		{90, 0xffb0, 0xff9e, input.KeyNumpad0, input.KeyInsert},
		{87, 0xffb1, 0xff9c, input.KeyNumpad1, input.KeyEnd},
		{88, 0xffb2, 0xff99, input.KeyNumpad2, input.KeyDown},
		{89, 0xffb3, 0xff9b, input.KeyNumpad3, input.KeyPageDown},
		{83, 0xffb4, 0xff96, input.KeyNumpad4, input.KeyLeft},
		{84, 0xffb5, 0xff9d, input.KeyNumpad5, input.KeyUnknown}, // KP_Begin: no navigation key
		{85, 0xffb6, 0xff98, input.KeyNumpad6, input.KeyRight},
		{79, 0xffb7, 0xff95, input.KeyNumpad7, input.KeyHome},
		{80, 0xffb8, 0xff97, input.KeyNumpad8, input.KeyUp},
		{81, 0xffb9, 0xff9a, input.KeyNumpad9, input.KeyPageUp},
		{91, 0xffae, 0xff9f, input.KeyNumpadDecimal, input.KeyDelete},
		// Not affected by NumLock: the same keyval either way.
		{104, 0xff8d, 0xff8d, input.KeyNumpadEnter, input.KeyNumpadEnter},
		{86, 0xffab, 0xffab, input.KeyNumpadAdd, input.KeyNumpadAdd},
		{82, 0xffad, 0xffad, input.KeyNumpadSubtract, input.KeyNumpadSubtract},
		{63, 0xffaa, 0xffaa, input.KeyNumpadMultiply, input.KeyNumpadMultiply},
		{106, 0xffaf, 0xffaf, input.KeyNumpadDivide, input.KeyNumpadDivide},
	} {
		if k, code := canvasKey(tc.onKV, tc.hw); k != tc.numpad || code != 0 {
			t.Errorf("NumLock on: canvasKey(%#x, %d) = %v, %v; want %v, 0", tc.onKV, tc.hw, k, code, tc.numpad)
		}
		wantCode := tc.numpad
		if tc.nav == tc.numpad {
			wantCode = 0
		}
		if k, code := canvasKey(tc.offKV, tc.hw); k != tc.nav || code != wantCode {
			t.Errorf("NumLock off: canvasKey(%#x, %d) = %v, %v; want %v, %v", tc.offKV, tc.hw, k, code, tc.nav, wantCode)
		}
		if wantCode != 0 {
			// Unknown keycode: the Numpad key the keyval belongs to.
			if k, code := canvasKey(tc.offKV, 0); k != tc.nav || code != tc.numpad {
				t.Errorf("NumLock off, no keycode: canvasKey(%#x, 0) = %v, %v", tc.offKV, k, code)
			}
		}
	}
	// The hardware keycode names the physical key.
	if k, code := canvasKey(0xff97, 88); k != input.KeyDown || code != input.KeyNumpad2 {
		t.Errorf("KP_Up on the keypad 2 keycode = %v, %v; want Down, Numpad2", k, code)
	}
}

// TestCanvasHeldKeys checks that a repeat and the release are reported
// with the Key and Code of the press, also when NumLock changed while the
// key was held, and that the navigation block and keypad records do not
// mix.
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
		down, up         ev // the event's mapping at press and release
		wantDown, wantUp ev
	}{
		{"numlock on", ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}, ev{input.KeyNumpad8, 0}},
		{"numlock off", ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}},
		{"off then on", ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyNumpad8, 0}, ev{input.KeyUp, input.KeyNumpad8}, ev{input.KeyUp, input.KeyNumpad8}},
		{"on then off", ev{input.KeyNumpad0, 0}, ev{input.KeyInsert, input.KeyNumpad0}, ev{input.KeyNumpad0, 0}, ev{input.KeyNumpad0, 0}},
		{"keypad 5 off", ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}, ev{input.KeyUnknown, input.KeyNumpad5}},
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

func TestCanvasModifierKeys(t *testing.T) {
	if m, ok := canvasModifierKeyMods(0xffe1, 0, true); !ok || m != input.ModShift {
		t.Fatalf("shift down = %v %v", m, ok)
	}
	if m, ok := canvasModifierKeyMods(0xffe3, canvasGdkControlMask|canvasGdkShiftMask, false); !ok || m != input.ModShift {
		t.Fatalf("ctrl up with shift = %v %v", m, ok)
	}
	if m, ok := canvasModifierKeyMods(0xffeb, canvasGdkMod4Mask, false); !ok || m != 0 {
		t.Fatalf("super up (X11 mod4 in state) = %v %v", m, ok)
	}
	if m, ok := canvasModifierKeyMods(0xffe9, 0, true); !ok || m != input.ModAlt {
		t.Fatalf("alt down = %v %v", m, ok)
	}
	if _, ok := canvasModifierKeyMods('a', 0, true); ok {
		t.Fatal("letter treated as modifier")
	}
	if _, ok := canvasModifierKeyMods(0xffe5, 0, true); ok {
		t.Fatal("Caps_Lock treated as modifier")
	}
	// Mods of the modifier key's own event.
	if m := canvasModifierEventMods(input.KeyShiftLeft, true, input.ModShift, false); m != input.ModShift {
		t.Fatalf("shift down = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyShiftLeft, false, 0, false); m != 0 {
		t.Fatalf("shift up = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyShiftLeft, false, 0, true); m != input.ModShift {
		t.Fatalf("shift up, right still held = %v", m)
	}
	if m := canvasModifierEventMods(input.KeyMetaLeft, true, input.ModCtrl, false); m != input.ModCtrl|input.ModMeta {
		t.Fatalf("super down with ctrl = %v", m)
	}
	for l, r := range map[input.Key]input.Key{
		input.KeyShiftLeft: input.KeyShiftRight, input.KeyCtrlLeft: input.KeyCtrlRight,
		input.KeyAltLeft: input.KeyAltRight, input.KeyMetaLeft: input.KeyMetaRight,
	} {
		if canvasOtherSide(l) != r || canvasOtherSide(r) != l || l.Modifier() != r.Modifier() || l.Modifier() == 0 {
			t.Errorf("canvasOtherSide(%v) = %v, (%v) = %v", l, canvasOtherSide(l), r, canvasOtherSide(r))
		}
	}
	if canvasOtherSide(input.KeyA) != input.KeyUnknown {
		t.Fatal("canvasOtherSide(A)")
	}
}

func TestCanvasButtons(t *testing.T) {
	for n, want := range map[uint32]input.Button{1: input.ButtonLeft, 2: input.ButtonMiddle, 3: input.ButtonRight} {
		if b, ok := canvasButton(n); !ok || b != want {
			t.Errorf("button %d = %v %v", n, b, ok)
		}
	}
	if _, ok := canvasButton(8); ok {
		t.Error("back button reported")
	}
	if canvasHeldButton(0) != input.ButtonLeft || canvasHeldButton(canvasGdkButton3Mask) != input.ButtonRight ||
		canvasHeldButton(canvasGdkButton2Mask) != input.ButtonMiddle || canvasHeldButton(canvasGdkButton1Mask|canvasGdkButton3Mask) != input.ButtonLeft {
		t.Error("canvasHeldButton")
	}
}

func TestCanvasClickCounter(t *testing.T) {
	var cc canvasClickCounter
	const ms, dist = 400, 5
	steps := []struct {
		b    input.Button
		t    uint32
		x, y float64
		want int
	}{
		{input.ButtonLeft, 1000, 10, 10, 1},
		{input.ButtonLeft, 1200, 12, 11, 2},
		{input.ButtonLeft, 1500, 12, 11, 3},
		{input.ButtonLeft, 1600, 12, 11, 1},  // a fourth click starts over
		{input.ButtonLeft, 2100, 12, 11, 1},  // too slow
		{input.ButtonRight, 2200, 12, 11, 1}, // other button
		{input.ButtonRight, 2300, 30, 11, 1}, // too far
		{input.ButtonRight, 2400, 30, 11, 2},
		{input.ButtonRight, 0xffffffff, 0, 0, 1},
		{input.ButtonRight, 100, 0, 0, 2}, // time wrapped around
	}
	for i, s := range steps {
		if got := cc.press(s.b, s.t, s.x, s.y, ms, dist); got != s.want {
			t.Fatalf("step %d: clicks = %d, want %d", i, got, s.want)
		}
	}
	if cc.release(input.ButtonRight) != 2 || cc.release(input.ButtonLeft) != 1 {
		t.Fatal("release count")
	}
	if canvasGdkPressCount(canvasGdk2ButtonPress) != 2 || canvasGdkPressCount(canvasGdk3ButtonPress) != 3 || canvasGdkPressCount(canvasGdkButtonPress) != 1 {
		t.Fatal("canvasGdkPressCount")
	}
}

func TestCanvasCursorNames(t *testing.T) {
	want := map[input.Cursor]string{
		input.CursorDefault: "default", input.CursorPointer: "pointer", input.CursorText: "text",
		input.CursorNotAllowed: "not-allowed", input.CursorResizeEW: "ew-resize", input.CursorResizeNS: "ns-resize",
		input.CursorGrab: "grab", input.CursorGrabbing: "grabbing", input.CursorMove: "move",
		input.Cursor(200): "default",
	}
	for c, name := range want {
		if got := canvasCursorNames(c); len(got) == 0 || got[0] != name {
			t.Errorf("canvasCursorNames(%d) = %v, want %s first", c, got, name)
		}
	}
}

func TestCanvasScrollDelta(t *testing.T) {
	for _, tc := range []struct {
		dir    uint32
		dx, dy float64
		finger bool
		wx, wy float32
		wantOK bool
	}{
		{canvasGdkScrollDown, 0, 0, false, 0, canvasScrollLine, true}, // wheel down reveals content below
		{canvasGdkScrollUp, 0, 0, false, 0, -canvasScrollLine, true},
		{canvasGdkScrollLeft, 0, 0, false, -canvasScrollLine, 0, true},
		{canvasGdkScrollRight, 0, 0, false, canvasScrollLine, 0, true},
		{canvasGdkScrollSmooth, 0, 1, false, 0, canvasScrollLine, true},
		{canvasGdkScrollSmooth, -0.5, 2.5, true, -0.5 * canvasScrollTouchpadUnit, 2.5 * canvasScrollTouchpadUnit, true},
		{9, 0, 0, false, 0, 0, false},
	} {
		x, y, ok := canvasScrollDelta(tc.dir, tc.dx, tc.dy, tc.finger)
		if x != tc.wx || y != tc.wy || ok != tc.wantOK {
			t.Errorf("canvasScrollDelta(%d, %v, %v, %v) = %v %v %v", tc.dir, tc.dx, tc.dy, tc.finger, x, y, ok)
		}
	}
}

func TestCanvasFrameSched(t *testing.T) {
	var f canvasFrameSched
	if !f.request() {
		t.Fatal("first request must install the tick callback")
	}
	if f.request() {
		t.Fatal("second request reinstalled the tick callback")
	}
	if draw, keep := f.tick(); !draw || !keep {
		t.Fatal("tick with a request must draw")
	}
	for i := 1; i < canvasIdleTicks; i++ {
		if draw, keep := f.tick(); draw || !keep {
			t.Fatalf("idle tick %d: draw=%v keep=%v", i, draw, keep)
		}
	}
	if f.request() {
		t.Fatal("request while still ticking reinstalled")
	}
	f.tick() // consumes it
	for i := 1; i < canvasIdleTicks; i++ {
		f.tick()
	}
	if draw, keep := f.tick(); draw || keep {
		t.Fatal("idle callback must remove itself")
	}
	if !f.request() {
		t.Fatal("request after removal must reinstall")
	}
	f.drew()
	if draw, _ := f.tick(); draw {
		t.Fatal("drew() must satisfy the pending request")
	}
	f.stopped()
	if !f.request() {
		t.Fatal("request after stopped must reinstall")
	}
}

func TestCanvasTimeoutMS(t *testing.T) {
	now := time.Unix(100, 0)
	for d, want := range map[time.Duration]uint32{
		-time.Second: 0, 0: 0, time.Microsecond: 1, time.Millisecond: 1,
		1500 * time.Microsecond: 2, time.Second: 1000,
	} {
		if got := canvasTimeoutMS(now, now.Add(d)); got != want {
			t.Errorf("canvasTimeoutMS(%v) = %d, want %d", d, got, want)
		}
	}
}

func TestCanvasRGBAToARGB32(t *testing.T) {
	// 2x2 premultiplied RGBA with a 12-byte source stride and a 16-byte
	// destination stride (padding must stay untouched).
	src := []byte{
		0x10, 0x20, 0x30, 0x40, 0xff, 0x00, 0x80, 0xff, 0xee, 0xee, 0xee, 0xee,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0xee, 0xee, 0xee, 0xee,
	}
	dst := bytes.Repeat([]byte{0xaa}, 32)
	canvasRGBAToARGB32(dst, 16, src, 12, 2, 2)
	px := func(x, y int) uint32 { return binary.NativeEndian.Uint32(dst[y*16+x*4:]) }
	for _, tc := range []struct {
		x, y int
		want uint32
	}{
		{0, 0, 0x40102030}, {1, 0, 0xffff0080}, {0, 1, 0}, {1, 1, 0x04010203},
	} {
		if got := px(tc.x, tc.y); got != tc.want {
			t.Errorf("pixel (%d,%d) = %#08x, want %#08x", tc.x, tc.y, got, tc.want)
		}
	}
	for _, i := range []int{8, 9, 10, 11, 12, 13, 14, 15, 24, 31} {
		if dst[i] != 0xaa {
			t.Fatalf("padding byte %d overwritten", i)
		}
	}
	if binary.NativeEndian.Uint16([]byte{1, 0}) == 1 { // little-endian: bytes B, G, R, A
		if !bytes.Equal(dst[0:4], []byte{0x30, 0x20, 0x10, 0x40}) {
			t.Fatalf("little-endian byte order = % x", dst[0:4])
		}
	}
}

func TestCanvasTextHelpers(t *testing.T) {
	if canvasClampRunes("你好", 5) != 2 || canvasClampRunes("你好", -1) != 0 || canvasClampRunes("ab", 1) != 1 {
		t.Fatal("canvasClampRunes")
	}
	if got := canvasCleanCommit("a\tb\r\x7fc你"); got != "abc你" {
		t.Fatalf("canvasCleanCommit = %q", got)
	}
	if canvasEventMask&(canvasGdkSmoothScrollMask|canvasGdkKeyPressMask|canvasGdkFocusChangeMask) == 0 {
		t.Fatal("event mask")
	}
}

func TestCanvasCaretRect(t *testing.T) {
	for _, tc := range []struct {
		in   geom.Rect
		want canvasRect
	}{
		{geom.Rect{X: 10, Y: 20, W: 1, H: 16}, canvasRect{10, 20, 1, 16}},
		{geom.Rect{X: 10.5, Y: 20.25, W: 0, H: 15.5}, canvasRect{10, 20, 1, 16}},
		{geom.Rect{X: -1.5, Y: -0.5, W: 2, H: 1}, canvasRect{-2, -1, 3, 2}},
	} {
		if got := canvasCaretRect(tc.in); got != tc.want {
			t.Errorf("canvasCaretRect(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestCanvasEditKey(t *testing.T) {
	for role, def := range roleDefs {
		if def.action != actEdit {
			continue
		}
		if _, _, ok := canvasEditKey(def.command); !ok {
			t.Errorf("edit role %q (command %q) has no canvas key", role, def.command)
		}
	}
	if k, m, ok := canvasEditKey("Redo"); !ok || k != input.KeyZ || m != input.ModCtrl|input.ModShift {
		t.Fatal("Redo")
	}
	if _, _, ok := canvasEditKey("Bogus"); ok {
		t.Fatal("unknown command mapped")
	}
}

func TestCanvasAltGr(t *testing.T) {
	// ISO_Level3_Shift (AltGr) is the right Alt key.
	if k, code := canvasKey(canvasKeyvalISOLevel3Shift, 108); k != input.KeyAltRight || code != 0 {
		t.Errorf("ISO_Level3_Shift = %v/%v, want KeyAltRight", k, code)
	}
	// Its own event: Alt on press, nothing on release, never Ctrl.
	m, ok := canvasModifierKeyMods(canvasKeyvalISOLevel3Shift, 0, true)
	if !ok || m&input.ModCtrl != 0 {
		t.Fatalf("AltGr press mods = %v, %v", m, ok)
	}
	if got := canvasModifierEventMods(input.KeyAltRight, true, m, false); got != input.ModAlt {
		t.Errorf("AltGr press event mods = %v, want ModAlt", got)
	}
	m, ok = canvasModifierKeyMods(canvasKeyvalISOLevel3Shift, canvasGdkMod5Mask, false)
	if got := canvasModifierEventMods(input.KeyAltRight, false, m, false); !ok || got != 0 {
		t.Errorf("AltGr release event mods = %v (%v), want 0", got, ok)
	}
	// A key typed with AltGr held (state Mod5) carries no Ctrl or Alt.
	if got := canvasMods(canvasGdkMod5Mask | canvasGdkShiftMask); got != input.ModShift {
		t.Errorf("mods with AltGr+Shift held = %v, want ModShift", got)
	}
}
