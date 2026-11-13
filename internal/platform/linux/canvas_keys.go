package linux

// Platform-independent helpers of the GTK canvas widget (canvas_linux.go,
// canvas_input_linux.go): GDK constants and event-struct offsets, key /
// modifier / button / cursor / scroll mapping, click counting, the frame
// scheduler state machine and the RGBA -> cairo ARGB32 pixel conversion.
// Kept free of build tags so they are unit tested on every OS.
//
// Values come from the GTK 3 / GDK 3 / cairo public headers
// (gdk/gdkevents.h, gdk/gdktypes.h, gdk/gdkkeysyms.h, cairo.h).

import (
	"encoding/binary"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// GdkEventMask bits (gdktypes.h).
const (
	canvasGdkPointerMotionMask = 1 << 2
	canvasGdkButtonPressMask   = 1 << 8
	canvasGdkButtonReleaseMask = 1 << 9
	canvasGdkKeyPressMask      = 1 << 10
	canvasGdkKeyReleaseMask    = 1 << 11
	canvasGdkEnterNotifyMask   = 1 << 12
	canvasGdkLeaveNotifyMask   = 1 << 13
	canvasGdkFocusChangeMask   = 1 << 14
	canvasGdkScrollMask        = 1 << 21
	canvasGdkSmoothScrollMask  = 1 << 23

	canvasEventMask = canvasGdkPointerMotionMask | canvasGdkButtonPressMask | canvasGdkButtonReleaseMask |
		canvasGdkKeyPressMask | canvasGdkKeyReleaseMask | canvasGdkEnterNotifyMask | canvasGdkLeaveNotifyMask |
		canvasGdkFocusChangeMask | canvasGdkScrollMask | canvasGdkSmoothScrollMask | canvasGdkTouchMask
)

// GdkEventType values (gdkevents.h).
const (
	canvasGdkMotionNotify  = 3
	canvasGdkButtonPress   = 4
	canvasGdk2ButtonPress  = 5
	canvasGdk3ButtonPress  = 6
	canvasGdkButtonRelease = 7
	canvasGdkKeyPress      = 8
	canvasGdkKeyRelease    = 9
	canvasGdkEnterNotify   = 10
	canvasGdkLeaveNotify   = 11
	canvasGdkScroll        = 31
)

// GdkModifierType bits (gdktypes.h).
const (
	canvasGdkShiftMask   = 1 << 0
	canvasGdkLockMask    = 1 << 1
	canvasGdkControlMask = 1 << 2
	canvasGdkMod1Mask    = 1 << 3 // Alt on every common keymap
	canvasGdkMod4Mask    = 1 << 6 // Super on every common keymap (X11 real modifier)
	canvasGdkMod5Mask    = 1 << 7 // ISO_Level3_Shift (AltGr) on every common keymap; not reported
	canvasGdkButton1Mask = 1 << 8
	canvasGdkButton2Mask = 1 << 9
	canvasGdkButton3Mask = 1 << 10
	canvasGdkSuperMask   = 1 << 26 // virtual modifier (set by Wayland GDK)
)

// GdkScrollDirection (gdkevents.h).
const (
	canvasGdkScrollUp     = 0
	canvasGdkScrollDown   = 1
	canvasGdkScrollLeft   = 2
	canvasGdkScrollRight  = 3
	canvasGdkScrollSmooth = 4
)

// GdkInputSource (gdkdevice.h) values that report finger-sized scroll
// deltas.
const (
	canvasGdkSourceTouchpad   = 6
	canvasGdkSourceTrackpoint = 7
)

// Byte offsets of the GdkEvent* fields read by the canvas, for the GTK 3
// layouts on LP64 targets (linux/amd64, linux/arm64: 8-byte pointers and
// doubles, 4-byte enums / gint / guint, natural alignment). Each struct
// starts with GdkEventAny { GdkEventType type; GdkWindow *window; gint8
// send_event; }. canvas_keys_test.go checks them against Go mirrors of
// the C structs.
const (
	// GdkEventButton { any; guint32 time; gdouble x, y; gdouble *axes;
	// guint state; guint button; GdkDevice *device; gdouble x_root, y_root; }
	canvasEvButtonTime   = 20
	canvasEvButtonX      = 24
	canvasEvButtonY      = 32
	canvasEvButtonState  = 48
	canvasEvButtonButton = 52

	// GdkEventMotion { any; guint32 time; gdouble x, y; gdouble *axes;
	// guint state; gint16 is_hint; GdkDevice *device; gdouble x_root, y_root; }
	canvasEvMotionX     = 24
	canvasEvMotionY     = 32
	canvasEvMotionState = 48

	// GdkEventCrossing { any; GdkWindow *subwindow; guint32 time; gdouble
	// x, y, x_root, y_root; GdkCrossingMode mode; GdkNotifyType detail;
	// gboolean focus; guint state; }
	canvasEvCrossingX     = 40
	canvasEvCrossingY     = 48
	canvasEvCrossingState = 84

	// GdkEventScroll { any; guint32 time; gdouble x, y; guint state;
	// GdkScrollDirection direction; GdkDevice *device; gdouble x_root,
	// y_root; gdouble delta_x, delta_y; guint is_stop : 1; }
	canvasEvScrollX         = 24
	canvasEvScrollY         = 32
	canvasEvScrollState     = 40
	canvasEvScrollDirection = 44
	canvasEvScrollDeltaX    = 72
	canvasEvScrollDeltaY    = 80

	// GdkEventKey { any; guint32 time; guint state; guint keyval; gint
	// length; gchar *string; guint16 hardware_keycode; guint8 group;
	// guint is_modifier : 1; }
	canvasEvKeyState   = 24
	canvasEvKeyKeyval  = 28
	canvasEvKeyHWCode  = 48
	canvasEvKeyGroup   = 50
	canvasEvTypeOffset = 0
)

// canvasMods converts a GdkModifierType. Meta is the Super ("Windows")
// key; GDK_META_MASK is deliberately ignored because X11 keymaps often
// alias it to Alt.
func canvasMods(state uint32) input.Modifiers {
	var m input.Modifiers
	if state&canvasGdkShiftMask != 0 {
		m |= input.ModShift
	}
	if state&canvasGdkControlMask != 0 {
		m |= input.ModCtrl
	}
	if state&canvasGdkMod1Mask != 0 {
		m |= input.ModAlt
	}
	if state&(canvasGdkMod4Mask|canvasGdkSuperMask) != 0 {
		m |= input.ModMeta
	}
	return m
}

// GDK keyvals (gdkkeysyms.h) of the keys whose keyval does not depend on
// the keyboard layout. Keypad digit keyvals that change with NumLock are
// in canvasGdkKPNav. Meta_L
// and Meta_R are the Alt keys: X11 keymaps produce them for Shift+Alt and
// put them on Mod1, which is ModAlt (see canvasMods).
var canvasGdkKeyvals = map[uint32]input.Key{
	0xff0d: input.KeyEnter, // Return
	0xfe34: input.KeyEnter, // ISO_Enter
	0xff09: input.KeyTab,   // Tab
	0xfe20: input.KeyTab,   // ISO_Left_Tab (Shift+Tab)
	0xff89: input.KeyTab,   // KP_Tab
	0xff1b: input.KeyEscape,
	0xff08: input.KeyBackspace,
	0xffff: input.KeyDelete, // Delete
	0xff51: input.KeyLeft,
	0xff52: input.KeyUp,
	0xff53: input.KeyRight,
	0xff54: input.KeyDown,
	0xff50: input.KeyHome,
	0xff57: input.KeyEnd,
	0xff55: input.KeyPageUp,
	0xff56: input.KeyPageDown,
	0x0020: input.KeySpace,
	0xff80: input.KeySpace, // KP_Space
	0xff63: input.KeyInsert,
	0xffe5: input.KeyCapsLock,
	0xff67: input.KeyContextMenu, // Menu

	0xffe1: input.KeyShiftLeft,  // Shift_L
	0xffe2: input.KeyShiftRight, // Shift_R
	0xffe3: input.KeyCtrlLeft,   // Control_L
	0xffe4: input.KeyCtrlRight,  // Control_R
	0xffe9: input.KeyAltLeft,    // Alt_L
	0xffea: input.KeyAltRight,   // Alt_R
	0xffe7: input.KeyAltLeft,    // Meta_L (Shift+Alt_L on X11)
	0xffe8: input.KeyAltRight,   // Meta_R
	0xfe03: input.KeyAltRight,   // ISO_Level3_Shift (AltGr): Mods carry Alt, never Ctrl
	0xffeb: input.KeyMetaLeft,   // Super_L
	0xffec: input.KeyMetaRight,  // Super_R

	0xff8d: input.KeyNumpadEnter,    // KP_Enter
	0xffab: input.KeyNumpadAdd,      // KP_Add
	0xffad: input.KeyNumpadSubtract, // KP_Subtract
	0xffaa: input.KeyNumpadMultiply, // KP_Multiply
	0xffaf: input.KeyNumpadDivide,   // KP_Divide
	0xffae: input.KeyNumpadDecimal,  // KP_Decimal
	0xffac: input.KeyNumpadDecimal,  // KP_Separator (decimal comma layouts)
}

// Keyvals of the keypad digit and decimal keys with NumLock off (or
// Shift held), by the keypad key that sends them. KP_0..KP_9 and
// KP_Decimal are the same keys with NumLock on.
var canvasGdkKPNav = map[uint32]input.Key{
	0xff9f: input.KeyNumpadDecimal, // KP_Delete
	0xff9e: input.KeyNumpad0,       // KP_Insert
	0xff9c: input.KeyNumpad1,       // KP_End
	0xff99: input.KeyNumpad2,       // KP_Down
	0xff9b: input.KeyNumpad3,       // KP_Page_Down
	0xff96: input.KeyNumpad4,       // KP_Left
	0xff9d: input.KeyNumpad5,       // KP_Begin
	0xff98: input.KeyNumpad6,       // KP_Right
	0xff95: input.KeyNumpad7,       // KP_Home
	0xff97: input.KeyNumpad8,       // KP_Up
	0xff9a: input.KeyNumpad9,       // KP_Page_Up
}

// Hardware keycodes (evdev code + 8, on X11 and Wayland) of the
// main-block digit and punctuation keys, by their position on the US
// layout, and of the keypad. Punctuation keyvals move with the layout and
// with Shift; the keycode does not.
var canvasHWKeys = map[uint16]input.Key{
	10: input.Key1, 11: input.Key2, 12: input.Key3, 13: input.Key4, 14: input.Key5,
	15: input.Key6, 16: input.Key7, 17: input.Key8, 18: input.Key9, 19: input.Key0,

	20: input.KeyMinus,        // KEY_MINUS
	21: input.KeyEqual,        // KEY_EQUAL
	34: input.KeyBracketLeft,  // KEY_LEFTBRACE
	35: input.KeyBracketRight, // KEY_RIGHTBRACE
	47: input.KeySemicolon,    // KEY_SEMICOLON
	48: input.KeyQuote,        // KEY_APOSTROPHE
	49: input.KeyBackquote,    // KEY_GRAVE
	51: input.KeyBackslash,    // KEY_BACKSLASH
	59: input.KeyComma,        // KEY_COMMA
	60: input.KeyPeriod,       // KEY_DOT
	61: input.KeySlash,        // KEY_SLASH

	90: input.KeyNumpad0, 87: input.KeyNumpad1, 88: input.KeyNumpad2, 89: input.KeyNumpad3,
	83: input.KeyNumpad4, 84: input.KeyNumpad5, 85: input.KeyNumpad6, 79: input.KeyNumpad7,
	80: input.KeyNumpad8, 81: input.KeyNumpad9,
	86:  input.KeyNumpadAdd,      // KEY_KPPLUS
	82:  input.KeyNumpadSubtract, // KEY_KPMINUS
	63:  input.KeyNumpadMultiply, // KEY_KPASTERISK
	106: input.KeyNumpadDivide,   // KEY_KPSLASH
	91:  input.KeyNumpadDecimal,  // KEY_KPDOT
	104: input.KeyNumpadEnter,    // KEY_KPENTER
}

// canvasKey maps a key event's keyval and hardware keycode to the
// reported Key and, when it differs, the physical key (KeyEvent.Code; 0
// otherwise). Keys with a layout-independent keyval (editing, function,
// modifier and keypad keys) map by keyval. Latin letters and digits map by
// their character, so shortcuts follow the active (Latin) layout: Ctrl+C
// on Dvorak is the key labelled C. Other main-block keys (punctuation, and
// digit keys whose character is not a digit: Shift+1, AZERTY) are named by
// their physical position on the US layout, from the hardware keycode.
// Keypad digits and the decimal key follow NumLock: KP_0..KP_9 and
// KP_Decimal (on) are Numpad keys; KP_Home, KP_Up ... KP_Delete (off) are
// the navigation Key (KP_Begin, keypad 5, KeyUnknown) with the Numpad key
// of the hardware keycode as code. Keypad Enter and operators ignore
// NumLock.
func canvasKey(keyval uint32, hw uint16) (key, code input.Key) {
	if p, ok := canvasGdkKPNav[keyval]; ok {
		if hk, ok := canvasHWKeys[hw]; ok {
			if _, nav := canvasNumpadNav(hk); nav {
				p = hk // the physical key, should a keymap move the keyvals
			}
		}
		nav, _ := canvasNumpadNav(p)
		return nav, p
	}
	if k, ok := canvasGdkKeyvals[keyval]; ok {
		return k, 0
	}
	switch {
	case keyval >= 0xffb0 && keyval <= 0xffb9: // KP_0 .. KP_9
		return input.KeyNumpad0 + input.Key(keyval-0xffb0), 0
	case keyval >= 0xffbe && keyval <= 0xffc9: // F1 .. F12
		return input.KeyF1 + input.Key(keyval-0xffbe), 0
	case keyval < 0x80:
		if k := input.KeyForRune(rune(keyval)); k != input.KeyUnknown {
			return k, 0
		}
	}
	return canvasHWKeys[hw], 0
}

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

// canvasHeldFind finds the record of the physical key of a key event in
// held, whichever NumLock form it was pressed in.
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

// canvasOtherSide is the other side of a modifier key (KeyShiftRight for
// KeyShiftLeft ...), KeyUnknown for other keys.
func canvasOtherSide(k input.Key) input.Key {
	switch k {
	case input.KeyShiftLeft, input.KeyCtrlLeft, input.KeyAltLeft, input.KeyMetaLeft:
		return k + 1 // input.Key declares each right key after its left key
	case input.KeyShiftRight, input.KeyCtrlRight, input.KeyAltRight, input.KeyMetaRight:
		return k - 1
	}
	return input.KeyUnknown
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

// canvasNeedsLatinFallback reports whether keyval is a character outside Latin-1
// (Cyrillic, Greek, ...), so the key should be re-translated in keyboard
// group 0 to find the Latin letter for shortcuts, as GTK accelerators do.
func canvasNeedsLatinFallback(keyval uint32) bool {
	return keyval >= 0x100 && keyval < 0xfe00 || keyval >= 0x01000100 && keyval <= 0x0110ffff
}

// canvasKeyvalISOLevel3Shift is the keyval of AltGr on layouts with a
// third level (de, fr, pl ...). It is reported as KeyAltRight; the state
// bit it sets (Mod5) is not a Tango modifier, so keys typed with AltGr
// carry neither Ctrl nor Alt (unlike Windows, X11 does not fake a Ctrl).
const canvasKeyvalISOLevel3Shift = 0xfe03

// canvasModifierKeyMask returns the GdkModifierType bit a modifier key toggles,
// or 0 when keyval is not a modifier key.
func canvasModifierKeyMask(keyval uint32) uint32 {
	switch keyval {
	case 0xffe1, 0xffe2: // Shift_L, Shift_R
		return canvasGdkShiftMask
	case 0xffe3, 0xffe4: // Control_L, Control_R
		return canvasGdkControlMask
	case 0xffe9, 0xffea, 0xffe7, 0xffe8: // Alt_L, Alt_R, Meta_L, Meta_R
		return canvasGdkMod1Mask
	case 0xffeb, 0xffec: // Super_L, Super_R
		return canvasGdkSuperMask
	case canvasKeyvalISOLevel3Shift: // AltGr: Mod5, which canvasMods ignores
		return canvasGdkMod5Mask
	}
	return 0
}

// canvasModifierKeyMods computes the modifier set after a modifier key event.
// GDK reports the state from before the event, so the toggled bit is
// added on press and removed on release. ok is false for other keys.
func canvasModifierKeyMods(keyval, state uint32, press bool) (m input.Modifiers, ok bool) {
	bit := canvasModifierKeyMask(keyval)
	if bit == 0 {
		return 0, false
	}
	if bit == canvasGdkSuperMask {
		state &^= canvasGdkMod4Mask
	}
	if press {
		state |= bit
	} else {
		state &^= bit
	}
	return canvasMods(state), true
}

// canvasButton maps a GDK button number (1 left, 2 middle, 3 right); other
// buttons (back / forward, 8 / 9) are not reported.
func canvasButton(n uint32) (input.Button, bool) {
	switch n {
	case 1:
		return input.ButtonLeft, true
	case 2:
		return input.ButtonMiddle, true
	case 3:
		return input.ButtonRight, true
	}
	return 0, false
}

// canvasHeldButton is the button reported with a motion event: the first held
// button from state, left when none is held (as the darwin backend).
func canvasHeldButton(state uint32) input.Button {
	switch {
	case state&canvasGdkButton1Mask != 0:
		return input.ButtonLeft
	case state&canvasGdkButton3Mask != 0:
		return input.ButtonRight
	case state&canvasGdkButton2Mask != 0:
		return input.ButtonMiddle
	}
	return input.ButtonLeft
}

// canvasCursorNames lists the names tried with gdk_cursor_new_from_name for c:
// the CSS name first, then legacy X cursor-font names for themes without
// it. An empty list means "inherit" (gdk_window_set_cursor(win, NULL)).
func canvasCursorNames(c input.Cursor) []string {
	switch c {
	case input.CursorPointer:
		return []string{"pointer", "hand2"}
	case input.CursorText:
		return []string{"text", "xterm"}
	case input.CursorNotAllowed:
		return []string{"not-allowed", "crossed_circle"}
	case input.CursorResizeEW:
		return []string{"ew-resize", "sb_h_double_arrow"}
	case input.CursorResizeNS:
		return []string{"ns-resize", "sb_v_double_arrow"}
	case input.CursorGrab:
		return []string{"grab", "openhand", "hand1"}
	case input.CursorGrabbing:
		return []string{"grabbing", "closedhand", "fleur"}
	case input.CursorMove:
		return []string{"move", "fleur"}
	case input.CursorCopy:
		return []string{"copy", "dnd-copy"}
	}
	return []string{"default"}
}

// Scroll scaling. canvasScrollLine matches the darwin backend (logical pixels per
// wheel notch). Smooth deltas from wheels are in notches; touchpads and
// trackpoints report finger travel in GDK's scroll units (on Wayland GDK
// divides wl_pointer axis pixels by 10), so they use a smaller factor.
const (
	canvasScrollLine         = 40
	canvasScrollTouchpadUnit = 10
)

// canvasScrollDelta converts a GdkEventScroll to input.Scroll deltas (positive DY
// reveals content below). GDK already reports positive deltas / "down"
// for that direction (natural scrolling is applied by the system), so no
// sign change is needed. ok is false for an unknown direction.
func canvasScrollDelta(direction uint32, dx, dy float64, fingerSource bool) (float32, float32, bool) {
	switch direction {
	case canvasGdkScrollUp:
		return 0, -canvasScrollLine, true
	case canvasGdkScrollDown:
		return 0, canvasScrollLine, true
	case canvasGdkScrollLeft:
		return -canvasScrollLine, 0, true
	case canvasGdkScrollRight:
		return canvasScrollLine, 0, true
	case canvasGdkScrollSmooth:
		f := float64(canvasScrollLine)
		if fingerSource {
			f = canvasScrollTouchpadUnit
		}
		return float32(dx*f + 0), float32(dy*f + 0), true // +0: no negative zero
	}
	return 0, 0, false
}

// canvasClickCounter counts multi-clicks for Pointer.Clicks. GDK only emits
// GDK_2BUTTON_PRESS / GDK_3BUTTON_PRESS after the plain press it belongs
// to, too late to fix that press's count, so the canvas counts presses
// itself with the same rule (gtk-double-click-time / -distance).
type canvasClickCounter struct {
	button input.Button
	time   uint32 // GDK event time, ms (wraps)
	x, y   float64
	count  int
	valid  bool
}

// press records a press and returns its click count.
func (cc *canvasClickCounter) press(b input.Button, t uint32, x, y float64, maxMS uint32, maxDist float64) int {
	dx, dy := x-cc.x, y-cc.y
	if cc.valid && b == cc.button && t-cc.time <= maxMS && dx*dx+dy*dy <= maxDist*maxDist && cc.count < 3 {
		cc.count++
	} else {
		cc.count = 1
	}
	cc.button, cc.time, cc.x, cc.y, cc.valid = b, t, x, y, true
	return cc.count
}

// release returns the click count for a release of b.
func (cc *canvasClickCounter) release(b input.Button) int {
	if cc.valid && b == cc.button {
		return cc.count
	}
	return 1
}

// canvasGdkPressCount is the click count GDK's synthetic event type stands for.
func canvasGdkPressCount(evType uint32) int {
	switch evType {
	case canvasGdk2ButtonPress:
		return 2
	case canvasGdk3ButtonPress:
		return 3
	}
	return 1
}

// canvasIdleTicks: the tick callback removes itself after this many frame
// clock ticks without a frame request, so a still window costs nothing.
const canvasIdleTicks = 4

// canvasFrameSched is the per-canvas frame state machine (main thread). A frame
// request sets wanted and makes sure the GdkFrameClock tick callback is
// installed; each tick turns a pending request into a redraw (queue_draw,
// painted in the same frame clock cycle) and idle ticks eventually remove
// the callback.
type canvasFrameSched struct {
	wanted  bool // a frame was requested since the last tick
	ticking bool // the tick callback is installed
	idle    int  // consecutive ticks without a request
}

// request records a frame request; it returns true when the caller must
// install the tick callback.
func (f *canvasFrameSched) request() (install bool) {
	f.wanted = true
	f.idle = 0
	if f.ticking {
		return false
	}
	f.ticking = true
	return true
}

// tick runs on each frame clock tick. draw: queue a redraw now; keep:
// the tick callback stays installed (false: GTK removes it).
func (f *canvasFrameSched) tick() (draw, keep bool) {
	if f.wanted {
		f.wanted = false
		f.idle = 0
		return true, true
	}
	f.idle++
	if f.idle >= canvasIdleTicks {
		f.ticking = false
		f.idle = 0
		return false, false
	}
	return false, true
}

// drew notes that a frame was painted for another reason (expose,
// resize): a pending request is satisfied.
func (f *canvasFrameSched) drew() { f.wanted = false }

// stopped notes the tick callback is gone (widget destroyed).
func (f *canvasFrameSched) stopped() { f.ticking, f.wanted, f.idle = false, false, 0 }

// canvasTimeoutMS is the g_timeout_add interval for a deadline: rounded up so
// the timer never fires early, 0 when already due.
func canvasTimeoutMS(now, at time.Time) uint32 {
	d := at.Sub(now)
	if d <= 0 {
		return 0
	}
	ms := (d + time.Millisecond - 1) / time.Millisecond
	if ms > 1<<31 {
		ms = 1 << 31
	}
	return uint32(ms)
}

// canvasCairoFormatARGB32 is CAIRO_FORMAT_ARGB32: 32-bit premultiplied ARGB in
// native byte order.
const canvasCairoFormatARGB32 = 0

// canvasRGBAToARGB32 converts w x h premultiplied RGBA pixels (image.RGBA order:
// bytes R, G, B, A) into cairo's ARGB32 (a native-endian uint32 0xAARRGGBB;
// bytes B, G, R, A on little-endian targets). Both are premultiplied, so
// only the channel order changes.
func canvasRGBAToARGB32(dst []byte, dstStride int, src []byte, srcStride, w, h int) {
	for y := 0; y < h; y++ {
		s := src[y*srcStride : y*srcStride+w*4]
		d := dst[y*dstStride : y*dstStride+w*4]
		for i := 0; i < len(s); i += 4 {
			v := binary.LittleEndian.Uint32(s[i:]) // A<<24 | B<<16 | G<<8 | R
			argb := v&0xff00ff00 | v&0xff<<16 | v>>16&0xff
			binary.NativeEndian.PutUint32(d[i:], argb)
		}
	}
}

// canvasClampRunes clamps an IM pre-edit cursor (characters) to text.
func canvasClampRunes(text string, pos int) int {
	return max(0, min(pos, utf8.RuneCountInString(text)))
}

// canvasCleanCommit drops control characters (Tab, Return, DEL ...) that some
// input methods commit; text input carries text only, keys arrive as
// KeyEvents.
func canvasCleanCommit(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// canvasRect mirrors GdkRectangle / cairo_rectangle_int_t.
type canvasRect struct{ X, Y, W, H int32 }

// canvasCaretRect rounds a logical caret rectangle outwards to the integer
// GdkRectangle the IM context takes (at least 1x1).
func canvasCaretRect(r geom.Rect) canvasRect {
	x0, y0 := canvasFloor(r.X), canvasFloor(r.Y)
	x1, y1 := canvasCeil(r.X+r.W), canvasCeil(r.Y+r.H)
	return canvasRect{X: x0, Y: y0, W: max(x1-x0, 1), H: max(y1-y0, 1)}
}

func canvasFloor(v float32) int32 {
	i := int32(v)
	if float32(i) > v {
		i--
	}
	return i
}

func canvasCeil(v float32) int32 {
	i := int32(v)
	if float32(i) < v {
		i++
	}
	return i
}

// canvasEditKey maps the Edit menu commands of menuspec.go (WebKit editing
// command names) to the shortcut a canvas receives instead.
func canvasEditKey(cmd string) (input.Key, input.Modifiers, bool) {
	switch cmd {
	case "Copy":
		return input.KeyC, input.ModCtrl, true
	case "Cut":
		return input.KeyX, input.ModCtrl, true
	case "Paste":
		return input.KeyV, input.ModCtrl, true
	case "SelectAll":
		return input.KeyA, input.ModCtrl, true
	case "Undo":
		return input.KeyZ, input.ModCtrl, true
	case "Redo":
		return input.KeyZ, input.ModCtrl | input.ModShift, true
	}
	return input.KeyUnknown, 0, false
}
