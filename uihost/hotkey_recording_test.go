package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

// The host must give even InTextInput bindings and function keys to a
// recording HotkeyInput, without removing/re-registering the bindings.
func TestHotkeyRecordingPreemptsWindowShortcut(t *testing.T) {
	for _, tc := range []struct {
		name, accel string
		key         input.Key
		mods        input.Modifiers
	}{
		{"primary", "CmdOrCtrl+K", input.KeyK, cmd()},
		{"function", "F5", input.KeyF5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, fb := faketest.Run(t, tango.Options{})
			value, focus := "", false
			var hk *ui.HotkeyInputW
			_, s, cv := open(t, a, fb, 400, 160, func(c *ui.Context) {
				hk = ui.HotkeyInput(c, &value)
				if !focus {
					c.SetFocus(hk.ID())
					focus = true
				}
			})
			cv.FlushFrame()
			if !cv.ShortcutCapture() {
				t.Fatal("native capture not enabled")
			}
			if on, _ := cv.TextInput(); on {
				t.Fatal("recorder enabled IME")
			}
			n := 0
			shortcut(t, s, tc.accel, uihost.ShortcutOptions{InTextInput: true}, func() { n++ })
			cv.Input(input.KeyEvent{Key: tc.key, Down: true, Mods: tc.mods})
			cv.FlushFrame()
			if cv.ShortcutCapture() {
				t.Fatal("native capture not released")
			}
			// Recording is now over, but holding the same key must not fire it.
			cv.Input(input.KeyEvent{Key: tc.key, Down: true, Repeat: true, Mods: tc.mods})
			cv.FlushFrame()
			cv.Input(input.KeyEvent{Key: tc.key, Down: false, Mods: tc.mods})
			fb.Do(func() {
				if n != 0 {
					t.Errorf("shortcut fired %d times during recording", n)
				}
				if value != tc.accel || hk.Recording() {
					t.Errorf("value=%q recording=%v, want %q done", value, hk.Recording(), tc.accel)
				}
			})
			press(cv, tc.key, tc.mods)
			fb.Do(func() {
				if n != 1 {
					t.Errorf("shortcut not restored: %d calls", n)
				}
			})
		})
	}
}

func TestHotkeyRecordingCancelRestoresShortcut(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	value, focus := "CmdOrCtrl+S", false
	_, s, cv := open(t, a, fb, 400, 160, func(c *ui.Context) {
		hk := ui.HotkeyInput(c, &value)
		if !focus {
			c.SetFocus(hk.ID())
			focus = true
		}
	})
	cv.FlushFrame()
	hits := 0
	shortcut(t, s, "Escape", uihost.ShortcutOptions{InTextInput: true}, func() { hits++ })
	press(cv, input.KeyEscape, 0)
	cv.FlushFrame()
	if cv.ShortcutCapture() {
		t.Fatal("cancel did not release native capture")
	}
	fb.Do(func() {
		if hits != 0 || value != "CmdOrCtrl+S" {
			t.Fatalf("cancel: calls=%d value=%q", hits, value)
		}
	})
	press(cv, input.KeyEscape, 0)
	fb.Do(func() {
		if hits != 1 {
			t.Fatalf("shortcut after cancel: %d calls", hits)
		}
	})
}
