// Package input defines the events a platform host feeds into the Tango
// self-drawn UI (pointer, multi-touch, wheel, keyboard, committed text
// and IME composition), the cursor shapes the UI asks the host to show, and the
// focus/Tab-order bookkeeping shared by focusable widgets.
//
// All coordinates are logical pixels relative to the window's content area.
//
// API tiers: only the symbols listed in api/stable/ui_input.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package input

import (
	"runtime"

	"github.com/tangoui/tango/ui/geom"
)

// Event is any input event.
type Event interface{ isEvent() }

// PointerKind is the pointer action.
//
// Append-only: never reorder or insert; the zero value stays Move.
type PointerKind uint8

const (
	Move PointerKind = iota
	Press
	Release
	Leave  // pointer left the window
	Cancel // gesture cancelled by the system
)

// Button is a mouse button.
//
// Append-only: never reorder or insert; the zero value stays ButtonLeft.
type Button uint8

const (
	ButtonLeft Button = iota
	ButtonRight
	ButtonMiddle
)

// Modifiers is a set of held modifier keys.
//
// Append-only: a new flag takes the next free bit; existing bits never
// change.
type Modifiers uint8

const (
	ModShift Modifiers = 1 << iota
	ModCtrl
	ModAlt
	ModMeta // Command on macOS, Windows key elsewhere
)

// Has reports whether all of m2 are held.
func (m Modifiers) Has(m2 Modifiers) bool { return m&m2 == m2 }

// Shortcut reports whether the platform shortcut modifier (Cmd on Apple
// platforms, Ctrl elsewhere) is held.
func (m Modifiers) Shortcut() bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return m&ModMeta != 0
	}
	return m&ModCtrl != 0
}

// Pointer is a mouse/touch/pen event.
type Pointer struct {
	Kind   PointerKind
	Pos    geom.Point
	Button Button
	Mods   Modifiers
	// Clicks is the click count for Press (1 single, 2 double...). Hosts
	// that do not know may leave it 0; the UI then counts itself.
	Clicks int
	// Source is what produced the event: a mouse or trackpad (the zero
	// value) or a finger (pointer events the UI synthesizes from the
	// primary touch, or hosts that report touches as pointers).
	Source PointerSource
}

// PointerSource tells mouse-made pointer events from touch-made ones.
//
// Append-only: never reorder or insert; the zero value stays SourceMouse.
type PointerSource uint8

const (
	SourceMouse PointerSource = iota // mouse, trackpad, pen hovering
	SourceTouch                      // a finger
)

// Scroll is a wheel / trackpad scroll. Positive DY scrolls content up
// (reveals what is below), in logical pixels.
type Scroll struct {
	Pos    geom.Point
	DX, DY float32
	Mods   Modifiers
}

// Key identifies a physical/logical key.
//
// Append-only: never reorder or insert; the zero value stays KeyUnknown.
type Key uint16

const (
	KeyUnknown Key = iota
	KeyEnter
	KeyTab
	KeyEscape
	KeyBackspace
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeySpace
	KeyA
	KeyC
	KeyV
	KeyX
	KeyZ
	KeyY
	// Remaining letters and digits (appended so earlier values stay stable).
	KeyB
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyW
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9

	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	// Modifier keys themselves (pressed / released on their own). Mods on
	// the event already includes (on press) or excludes (on release) the
	// modifier.
	KeyShiftLeft
	KeyShiftRight
	KeyCtrlLeft
	KeyCtrlRight
	KeyAltLeft  // Option on macOS
	KeyAltRight // Option on macOS
	KeyMetaLeft // Command on macOS, Windows key elsewhere
	KeyMetaRight

	// Punctuation, named after the US layout key.
	KeyMinus        // -
	KeyEqual        // =
	KeyBracketLeft  // [
	KeyBracketRight // ]
	KeyBackslash    // \
	KeySemicolon    // ;
	KeyQuote        // '
	KeyComma        // ,
	KeyPeriod       // .
	KeySlash        // /
	KeyBackquote    // `

	// Numeric keypad.
	KeyNumpad0
	KeyNumpad1
	KeyNumpad2
	KeyNumpad3
	KeyNumpad4
	KeyNumpad5
	KeyNumpad6
	KeyNumpad7
	KeyNumpad8
	KeyNumpad9
	KeyNumpadAdd
	KeyNumpadSubtract
	KeyNumpadMultiply
	KeyNumpadDivide
	KeyNumpadDecimal
	KeyNumpadEnter

	KeyInsert
	KeyCapsLock
	KeyContextMenu

	// KeyBack is never a Key of its own: it is the Code of the Escape event
	// that stands for Android's system Back (see KeyEvent).
	KeyBack

	keyCount // sentinel, keep last
)

var letterKeys = [26]Key{KeyA, KeyB, KeyC, KeyD, KeyE, KeyF, KeyG, KeyH, KeyI, KeyJ, KeyK, KeyL, KeyM,
	KeyN, KeyO, KeyP, KeyQ, KeyR, KeyS, KeyT, KeyU, KeyV, KeyW, KeyX, KeyY, KeyZ}

var digitKeys = [10]Key{Key0, Key1, Key2, Key3, Key4, Key5, Key6, Key7, Key8, Key9}

// KeyForRune maps an ASCII letter (either case) or digit to its Key, or
// KeyUnknown. Hosts use it for layout-aware shortcut keys.
func KeyForRune(r rune) Key {
	switch {
	case r >= 'a' && r <= 'z':
		return letterKeys[r-'a']
	case r >= 'A' && r <= 'Z':
		return letterKeys[r-'A']
	case r >= '0' && r <= '9':
		return digitKeys[r-'0']
	}
	return KeyUnknown
}

// Rune returns the character a letter, digit, punctuation, space or
// keypad key produces on the US layout without Shift (letters lower-case),
// or 0 for other keys.
func (k Key) Rune() rune {
	for i, l := range letterKeys {
		if l == k {
			return 'a' + rune(i)
		}
	}
	switch {
	case k >= Key0 && k <= Key9:
		return '0' + rune(k-Key0)
	case k >= KeyNumpad0 && k <= KeyNumpad9:
		return '0' + rune(k-KeyNumpad0)
	}
	switch k {
	case KeySpace:
		return ' '
	case KeyMinus, KeyNumpadSubtract:
		return '-'
	case KeyEqual:
		return '='
	case KeyBracketLeft:
		return '['
	case KeyBracketRight:
		return ']'
	case KeyBackslash:
		return '\\'
	case KeySemicolon:
		return ';'
	case KeyQuote:
		return '\''
	case KeyComma:
		return ','
	case KeyPeriod, KeyNumpadDecimal:
		return '.'
	case KeySlash, KeyNumpadDivide:
		return '/'
	case KeyBackquote:
		return '`'
	case KeyNumpadAdd:
		return '+'
	case KeyNumpadMultiply:
		return '*'
	}
	return 0
}

// Modifier returns the modifier bit a modifier key controls (0 for other
// keys). Hosts that only learn the new modifier state (macOS
// flagsChanged:) use it to decide whether the key went down or up.
func (k Key) Modifier() Modifiers {
	switch k {
	case KeyShiftLeft, KeyShiftRight:
		return ModShift
	case KeyCtrlLeft, KeyCtrlRight:
		return ModCtrl
	case KeyAltLeft, KeyAltRight:
		return ModAlt
	case KeyMetaLeft, KeyMetaRight:
		return ModMeta
	}
	return 0
}

// Base folds keypad keys that duplicate a main key onto it (NumpadEnter →
// Enter, Numpad0..9 → 0..9) and returns other keys unchanged, so
// shortcuts can match either.
func (k Key) Base() Key {
	switch {
	case k == KeyNumpadEnter:
		return KeyEnter
	case k >= KeyNumpad0 && k <= KeyNumpad9:
		return Key0 + (k - KeyNumpad0)
	case k == KeyNumpadSubtract:
		return KeyMinus
	case k == KeyNumpadDecimal:
		return KeyPeriod
	case k == KeyNumpadDivide:
		return KeySlash
	}
	return k
}

var keyNames = [...]string{
	KeyUnknown:        "Unknown",
	KeyEnter:          "Enter",
	KeyTab:            "Tab",
	KeyEscape:         "Escape",
	KeyBackspace:      "Backspace",
	KeyDelete:         "Delete",
	KeyLeft:           "Left",
	KeyRight:          "Right",
	KeyUp:             "Up",
	KeyDown:           "Down",
	KeyHome:           "Home",
	KeyEnd:            "End",
	KeyPageUp:         "PageUp",
	KeyPageDown:       "PageDown",
	KeySpace:          "Space",
	KeyA:              "A",
	KeyB:              "B",
	KeyC:              "C",
	KeyD:              "D",
	KeyE:              "E",
	KeyF:              "F",
	KeyG:              "G",
	KeyH:              "H",
	KeyI:              "I",
	KeyJ:              "J",
	KeyK:              "K",
	KeyL:              "L",
	KeyM:              "M",
	KeyN:              "N",
	KeyO:              "O",
	KeyP:              "P",
	KeyQ:              "Q",
	KeyR:              "R",
	KeyS:              "S",
	KeyT:              "T",
	KeyU:              "U",
	KeyV:              "V",
	KeyW:              "W",
	KeyX:              "X",
	KeyY:              "Y",
	KeyZ:              "Z",
	Key0:              "0",
	Key1:              "1",
	Key2:              "2",
	Key3:              "3",
	Key4:              "4",
	Key5:              "5",
	Key6:              "6",
	Key7:              "7",
	Key8:              "8",
	Key9:              "9",
	KeyF1:             "F1",
	KeyF2:             "F2",
	KeyF3:             "F3",
	KeyF4:             "F4",
	KeyF5:             "F5",
	KeyF6:             "F6",
	KeyF7:             "F7",
	KeyF8:             "F8",
	KeyF9:             "F9",
	KeyF10:            "F10",
	KeyF11:            "F11",
	KeyF12:            "F12",
	KeyShiftLeft:      "ShiftLeft",
	KeyShiftRight:     "ShiftRight",
	KeyCtrlLeft:       "CtrlLeft",
	KeyCtrlRight:      "CtrlRight",
	KeyAltLeft:        "AltLeft",
	KeyAltRight:       "AltRight",
	KeyMetaLeft:       "MetaLeft",
	KeyMetaRight:      "MetaRight",
	KeyMinus:          "Minus",
	KeyEqual:          "Equal",
	KeyBracketLeft:    "BracketLeft",
	KeyBracketRight:   "BracketRight",
	KeyBackslash:      "Backslash",
	KeySemicolon:      "Semicolon",
	KeyQuote:          "Quote",
	KeyComma:          "Comma",
	KeyPeriod:         "Period",
	KeySlash:          "Slash",
	KeyBackquote:      "Backquote",
	KeyNumpad0:        "Numpad0",
	KeyNumpad1:        "Numpad1",
	KeyNumpad2:        "Numpad2",
	KeyNumpad3:        "Numpad3",
	KeyNumpad4:        "Numpad4",
	KeyNumpad5:        "Numpad5",
	KeyNumpad6:        "Numpad6",
	KeyNumpad7:        "Numpad7",
	KeyNumpad8:        "Numpad8",
	KeyNumpad9:        "Numpad9",
	KeyNumpadAdd:      "NumpadAdd",
	KeyNumpadSubtract: "NumpadSubtract",
	KeyNumpadMultiply: "NumpadMultiply",
	KeyNumpadDivide:   "NumpadDivide",
	KeyNumpadDecimal:  "NumpadDecimal",
	KeyNumpadEnter:    "NumpadEnter",
	KeyInsert:         "Insert",
	KeyCapsLock:       "CapsLock",
	KeyContextMenu:    "ContextMenu",
	KeyBack:           "Back",
}

// String returns the key name ("A", "7", "F5", "ShiftLeft", "Minus",
// "NumpadEnter"...). Unknown values print as "Key(n)".
func (k Key) String() string {
	if int(k) < len(keyNames) && keyNames[k] != "" {
		return keyNames[k]
	}
	return "Key(" + itoa(int(k)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// KeyNamed is the inverse of String; it reports false for unknown names.
func KeyNamed(name string) (Key, bool) {
	for k, n := range keyNames {
		if n == name && n != "" {
			return Key(k), true
		}
	}
	return KeyUnknown, false
}

// KeyEvent is a key press or release.
// KeyEvent is a key press or release.
//
// Platform hosts report Key as follows: letters and digits by the character
// they produce in the current keyboard layout (KeyForRune); every other key
// (punctuation, F-keys, keypad, left/right modifiers) by its physical
// position on a US layout. Keypad keys follow NumLock: with NumLock on they
// are reported as Numpad* keys (Code 0); with NumLock off as the navigation
// key they stand for (Up, Down, Left, Right, Home, End, PageUp, PageDown,
// Insert, Delete) with the physical Numpad* key in Code, and keypad 5 as
// KeyUnknown with Code KeyNumpad5. macOS has no NumLock and always reports
// Numpad* keys. A key's release reports the same Key/Code as its press even
// if NumLock changed in between. NumpadEnter is distinct from Enter.
//
// Before components see an event, the runner sets Code to the reported key
// and folds Key with Key.Base(), so NumpadEnter arrives as KeyEnter and
// Numpad digits as digits; use Code to tell them apart.
//
// Android's system Back (button or gesture), while the UI takes it
// (ui.BackWindow), is reported as Key KeyEscape with Code KeyBack: it
// closes overlays like Escape, and widgets that claim Escape for
// themselves (ui.Context.ClaimEscape) can tell it from a keyboard Esc.
//
// For a modifier key's own event, Mods includes that modifier while it is
// down and excludes it on release, unless the same modifier on the other
// side is still held.
type KeyEvent struct {
	Key    Key
	Code   Key // physical key as reported by the platform; 0 means same as Key
	Down   bool
	Repeat bool
	Mods   Modifiers
}

// TextInput carries committed text (typing, paste from IME, dictation).
type TextInput struct{ Text string }

// Composition reports the IME pre-edit (composition) string. An empty
// Text ends the composition without committing; the commit itself arrives
// as TextInput. Cursor is the caret position in runes within Text.
type Composition struct {
	Text   string
	Cursor int
}

// WindowFocus reports the window gaining or losing key focus.
type WindowFocus struct{ Focused bool }

func (Pointer) isEvent()     {}
func (Scroll) isEvent()      {}
func (KeyEvent) isEvent()    {}
func (TextInput) isEvent()   {}
func (Composition) isEvent() {}
func (WindowFocus) isEvent() {}

// Cursor is a pointer shape requested by the UI.
//
// Append-only: never reorder or insert; the zero value stays CursorDefault.
type Cursor uint8

const (
	CursorDefault Cursor = iota
	CursorPointer
	CursorText
	CursorNotAllowed
	CursorResizeEW
	CursorResizeNS
	CursorGrab
	CursorGrabbing
	CursorMove
	// CursorCopy is the arrow with a plus badge shown while a drag will
	// copy rather than move (macOS dragCopyCursor, X11/Wayland "copy").
	// Platforms without such a shape show the default arrow.
	CursorCopy
)

// FocusOrder records focusable IDs in paint order for one frame and answers
// Tab / Shift+Tab navigation.
type FocusOrder[ID comparable] struct {
	ids   []ID
	noTab []bool // focusable by pointer only; Tab skips it
}

// Reset clears the order for a new frame.
func (f *FocusOrder[ID]) Reset() { f.ids, f.noTab = f.ids[:0], f.noTab[:0] }

// Add appends a focusable ID.
func (f *FocusOrder[ID]) Add(id ID) { f.ids = append(f.ids, id); f.noTab = append(f.noTab, false) }

// AddNoTab appends an ID that can hold focus (after a click) but that Tab
// navigation skips, such as selectable text.
func (f *FocusOrder[ID]) AddNoTab(id ID) { f.ids = append(f.ids, id); f.noTab = append(f.noTab, true) }

// Len returns the number of registered IDs.
func (f *FocusOrder[ID]) Len() int { return len(f.ids) }

// Contains reports whether id was registered.
func (f *FocusOrder[ID]) Contains(id ID) bool {
	for _, x := range f.ids {
		if x == id {
			return true
		}
	}
	return false
}

// Next returns the ID after cur (before it when back), wrapping around. If
// cur is not registered the first (or last) ID is returned.
func (f *FocusOrder[ID]) Next(cur ID, back bool) (ID, bool) {
	var zero ID
	n := len(f.ids)
	tab := func(i int) bool { return i >= len(f.noTab) || !f.noTab[i] }
	start := -1
	for i, x := range f.ids {
		if x == cur {
			start = i
			break
		}
	}
	step := 1
	if back {
		step = -1
	}
	if start < 0 {
		if back {
			start = 0
		} else {
			start = n - 1
		}
	}
	for k := 1; k <= n; k++ {
		i := ((start+step*k)%n + n) % n
		if tab(i) {
			return f.ids[i], true
		}
	}
	return zero, false
}
