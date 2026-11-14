package android

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestCanvasKey(t *testing.T) {
	for _, tc := range []struct {
		name             string
		code, base, meta int
		key, phys        input.Key
	}{
		{"letter by its character", akA, 'a', 0, input.KeyA, 0},
		{"AZERTY: the A key types q", akA, 'q', 0, input.KeyQ, 0},
		{"letter without a character", akA + 25, 0, 0, input.KeyZ, 0},
		{"digit", akNum0 + 7, '7', 0, input.Key7, 0},
		{"AZERTY digit row types &", akNum0 + 1, '&', 0, input.Key1, 0},
		{"enter", akEnter, '\n', 0, input.KeyEnter, 0},
		{"backspace", akDel, 0, 0, input.KeyBackspace, 0},
		{"forward delete", akForwardDel, 0, 0, input.KeyDelete, 0},
		{"escape", akEscape, 0, 0, input.KeyEscape, 0},
		{"arrows", akDpadLeft, 0, 0, input.KeyLeft, 0},
		{"home", akMoveHome, 0, 0, input.KeyHome, 0},
		{"F12", akF1 + 11, 0, 0, input.KeyF12, 0},
		{"punctuation", akSemicolon, ';', 0, input.KeySemicolon, 0},
		{"grave", akGrave, '`', 0, input.KeyBackquote, 0},
		{"left shift", akShiftLeft, 0, 0, input.KeyShiftLeft, 0},
		{"right meta", akMetaRight, 0, 0, input.KeyMetaRight, 0},
		{"numpad digit with NumLock", akNumpad0 + 3, '3', akMetaNumLockOn, input.KeyNumpad3, 0},
		{"numpad 8 without NumLock is Up", akNumpad0 + 8, 0, 0, input.KeyUp, input.KeyNumpad8},
		{"numpad 5 without NumLock", akNumpad0 + 5, 0, 0, input.KeyUnknown, input.KeyNumpad5},
		{"numpad dot without NumLock is Delete", akNumpadDot, 0, 0, input.KeyDelete, input.KeyNumpadDecimal},
		{"numpad enter", akNumpadEnter, '\n', 0, input.KeyNumpadEnter, 0},
		{"unknown code", 999, 0, 0, input.KeyUnknown, 0},
	} {
		k, p := canvasKey(tc.code, tc.base, tc.meta)
		if k != tc.key || p != tc.phys {
			t.Errorf("%s: canvasKey(%d, %q, %#x) = %v/%v, want %v/%v", tc.name, tc.code, rune(tc.base), tc.meta, k, p, tc.key, tc.phys)
		}
	}
}

func TestCanvasMods(t *testing.T) {
	if m := canvasMods(akMetaShiftOn | akMetaShiftLeftOn | akMetaCtrlOn | akMetaCtrlLeftOn); m != input.ModShift|input.ModCtrl {
		t.Errorf("shift+ctrl = %v", m)
	}
	if m := canvasMods(akMetaAltOn | akMetaMetaOn); m != input.ModAlt|input.ModMeta {
		t.Errorf("alt+meta = %v", m)
	}
	if m := canvasMods(akMetaNumLockOn); m != 0 {
		t.Errorf("NumLock alone = %v, want none", m)
	}
}

func TestCanvasKeysEvents(t *testing.T) {
	var s canvasKeys
	// Back is the system's while back handling is off.
	if _, ok := s.event(true, akBack, 0, 0, 0); ok {
		t.Error("Back delivered to the canvas")
	}
	if _, ok := s.event(false, akBack, 0, 0, 0); ok {
		t.Error("Back release delivered to the canvas")
	}
	// While on (SetBackHandling), Back is Escape marked with Code KeyBack;
	// its release follows the press even if handling went off in between.
	s.back = true
	if ev, ok := s.event(true, akBack, 0, 0, 0); !ok || ev.Key != input.KeyEscape || ev.Code != input.KeyBack || !ev.Down {
		t.Errorf("Back with handling on = %+v %v", ev, ok)
	}
	s.back = false
	if ev, ok := s.event(false, akBack, 0, 0, 0); !ok || ev.Key != input.KeyEscape || ev.Code != input.KeyBack || ev.Down {
		t.Errorf("Back release after handling went off = %+v %v", ev, ok)
	}
	if _, ok := s.event(true, akBack, 0, 0, 0); ok {
		t.Error("Back delivered after handling went off")
	}
	// A keyboard Escape stays unmarked.
	s.back = true
	if ev, _ := s.event(true, akEscape, 0, 0, 0); ev.Key != input.KeyEscape || ev.Code != 0 {
		t.Errorf("Escape = %+v", ev)
	}
	s.event(false, akEscape, 0, 0, 0)
	s.back = false
	// A key press repeats; the release reports the Key of the press even
	// if NumLock changed in between.
	if ev, ok := s.event(true, akNumpad0+8, '8', akMetaNumLockOn, 0); !ok || ev.Key != input.KeyNumpad8 || !ev.Down || ev.Repeat {
		t.Errorf("press = %+v %v", ev, ok)
	}
	if ev, _ := s.event(true, akNumpad0+8, '8', akMetaNumLockOn, 3); !ev.Repeat {
		t.Errorf("repeat = %+v", ev)
	}
	if ev, _ := s.event(false, akNumpad0+8, 0, 0, 0); ev.Key != input.KeyNumpad8 || ev.Code != 0 || ev.Down {
		t.Errorf("release after NumLock off = %+v", ev)
	}
	// A modifier's own event: included on press, excluded on release unless
	// the other side is still held.
	if ev, _ := s.event(true, akShiftLeft, 0, 0, 0); ev.Mods != input.ModShift {
		t.Errorf("shift press mods = %v", ev.Mods)
	}
	if ev, _ := s.event(false, akShiftLeft, 0, akMetaShiftOn|akMetaShiftLeftOn, 0); ev.Mods != 0 {
		t.Errorf("shift release mods = %v", ev.Mods)
	}
	if ev, _ := s.event(false, akShiftLeft, 0, akMetaShiftOn|akMetaShiftRightOn, 0); ev.Mods != input.ModShift {
		t.Errorf("left shift release with right shift held: mods = %v", ev.Mods)
	}
	// Ctrl+C: the letter with Ctrl.
	if ev, _ := s.event(true, akA+2, 'c', akMetaCtrlOn|akMetaCtrlLeftOn, 0); ev.Key != input.KeyC || ev.Mods != input.ModCtrl {
		t.Errorf("ctrl+c = %+v", ev)
	}
	s.reset()
	if len(s.down) != 0 {
		t.Errorf("reset kept %d keys", len(s.down))
	}
}

func TestCanvasText(t *testing.T) {
	for _, tc := range []struct {
		unicode, meta int
		want          string
		ok            bool
	}{
		{'a', 0, "a", true},
		{'A', akMetaShiftOn, "A", true},
		{'é', 0, "é", true},
		{'c', akMetaCtrlOn, "", false},
		{'v', akMetaMetaOn, "", false},
		{'x', akMetaAltOn, "", false},
		{'\n', 0, "", false},
		{0x7f, 0, "", false},
		{0, 0, "", false},
		{0x80000000 | '`', 0, "", false}, // a dead key (COMBINING_ACCENT)
	} {
		got, ok := canvasText(tc.unicode, tc.meta)
		if got != tc.want || ok != tc.ok {
			t.Errorf("canvasText(%#x, %#x) = %q %v, want %q %v", tc.unicode, tc.meta, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCanvasPointerIcon(t *testing.T) {
	for c, want := range map[input.Cursor]int{
		input.CursorDefault: apiArrow, input.CursorPointer: apiHand, input.CursorText: apiText,
		input.CursorNotAllowed: apiNoDrop, input.CursorResizeEW: apiHorizontal, input.CursorResizeNS: apiVertical,
		input.CursorGrab: apiGrab, input.CursorGrabbing: apiGrabbing, input.CursorMove: apiAllScroll,
		input.CursorCopy: apiCopy,
	} {
		if got := canvasPointerIcon(c); got != want {
			t.Errorf("canvasPointerIcon(%v) = %d, want %d", c, got, want)
		}
	}
}
