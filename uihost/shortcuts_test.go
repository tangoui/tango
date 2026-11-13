package uihost_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

// cmd is the CmdOrCtrl modifier on this platform.
func cmd() input.Modifiers {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return input.ModMeta
	}
	return input.ModCtrl
}

func press(cv *fake.Canvas, k input.Key, m input.Modifiers) {
	cv.Input(input.KeyEvent{Key: k, Down: true, Mods: m})
	cv.Input(input.KeyEvent{Key: k, Down: false, Mods: m})
}

// keyPage records the key presses the UI receives.
type keyPage struct {
	text  string
	focus bool // focus the text input
}

func (p *keyPage) page(c *ui.Context) {
	in := ui.Input(c, &p.text)
	switch {
	case p.focus && !in.Focused():
		in.Focus()
	case !p.focus && in.Focused():
		c.SetFocus(0)
	}
}

func shortcut(t *testing.T, s *tango.Surface, accel string, opts uihost.ShortcutOptions, fn func()) func() {
	t.Helper()
	rm, err := uihost.ShortcutWith(s, accel, opts, fn)
	if err != nil {
		t.Fatalf("ShortcutWith(%q): %v", accel, err)
	}
	return rm
}

func TestShortcutFiresAndConsumes(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var seen []input.KeyEvent
	_, s, cv := open(t, a, fb, 200, 100, func(c *ui.Context) {
		seen = append(seen, c.KeysFor(0)...)
	})
	n := 0
	rm := shortcut(t, s, "CmdOrCtrl+N", uihost.ShortcutOptions{}, func() { n++ })

	press(cv, input.KeyN, cmd())
	// Holding the key: repeats are swallowed and do not fire again.
	cv.Input(input.KeyEvent{Key: input.KeyN, Down: true, Mods: cmd()})
	cv.Input(input.KeyEvent{Key: input.KeyN, Down: true, Repeat: true, Mods: cmd()})
	cv.Input(input.KeyEvent{Key: input.KeyN, Down: false, Mods: cmd()})
	press(cv, input.KeyN, cmd()|input.ModShift) // other modifiers: no match
	press(cv, input.KeyM, cmd())                // other key: no match
	cv.FlushFrame()
	fb.Do(func() {
		if n != 2 {
			t.Errorf("fired %d times, want 2", n)
		}
		for _, k := range seen {
			if k.Key == input.KeyN && k.Mods == cmd() {
				t.Errorf("UI saw the shortcut key %+v", k)
			}
		}
		if len(seen) != 4 { // Shift+Cmd+N and Cmd+M, down and up
			t.Errorf("UI saw %d key events, want 4: %+v", len(seen), seen)
		}
	})

	rm()
	rm() // twice is fine
	press(cv, input.KeyN, cmd())
	fb.Do(func() {
		if n != 2 {
			t.Errorf("fired after remove")
		}
	})
}

func TestShortcutKeys(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, s, cv := open(t, a, fb, 200, 100, func(c *ui.Context) {})
	var got []string
	for _, acc := range []string{"Shift+CmdOrCtrl+Z", "Alt+F4", "F5", "Escape", "CmdOrCtrl+1", "CmdOrCtrl+Plus"} {
		shortcut(t, s, acc, uihost.ShortcutOptions{}, func() { got = append(got, acc) })
	}
	press(cv, input.KeyZ, cmd()|input.ModShift)
	press(cv, input.KeyF4, input.ModAlt)
	press(cv, input.KeyF5, 0)
	press(cv, input.KeyEscape, 0)
	press(cv, input.KeyNumpad1, cmd()) // keypad digit matches the digit
	press(cv, input.KeyNumpadAdd, cmd())
	press(cv, input.KeyEqual, cmd()) // the key that has + on it
	fb.Do(func() {
		want := []string{"Shift+CmdOrCtrl+Z", "Alt+F4", "F5", "Escape", "CmdOrCtrl+1", "CmdOrCtrl+Plus", "CmdOrCtrl+Plus"}
		if len(got) != len(want) {
			t.Fatalf("fired %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("fired %v, want %v", got, want)
			}
		}
	})
}

func TestShortcutErrors(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, s, _ := open(t, a, fb, 100, 100, func(c *ui.Context) {})
	for _, bad := range []string{"", "Hyper+K", "Ctrl+", "Ctrl+F13"} {
		if _, err := uihost.Shortcut(s, bad, func() {}); !errors.Is(err, hotkey.ErrBadAccelerator) {
			t.Errorf("Shortcut(%q) err %v", bad, err)
		}
	}
	if _, err := uihost.Shortcut(nil, "F5", func() {}); !errors.Is(err, uihost.ErrNotUI) {
		t.Errorf("nil surface: %v", err)
	}
	ws, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uihost.Shortcut(ws, "F5", func() {}); !errors.Is(err, uihost.ErrNotUI) {
		t.Errorf("WebView surface: %v", err)
	}
	if !fb.RequestClose(s.ID()) {
		t.Fatal("not closed")
	}
	if _, err := uihost.Shortcut(s, "F5", func() {}); !errors.Is(err, uihost.ErrNotUI) {
		t.Errorf("closed surface: %v", err)
	}
}

func TestShortcutNotDuringComposition(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	p := &keyPage{focus: true}
	_, s, cv := open(t, a, fb, 300, 100, p.page)
	for cv.FlushFrame() {
	}
	n := 0
	shortcut(t, s, "CmdOrCtrl+K", uihost.ShortcutOptions{InTextInput: true}, func() { n++ })
	cv.Input(input.Composition{Text: "ni", Cursor: 2})
	press(cv, input.KeyK, cmd())
	cv.Input(input.TextInput{Text: "你"}) // commit ends the composition
	press(cv, input.KeyK, cmd())
	cv.Input(input.Composition{Text: "a"})
	cv.Input(input.Composition{}) // cancelled
	press(cv, input.KeyK, cmd())
	fb.Do(func() {
		if n != 2 {
			t.Errorf("fired %d times, want 2 (not while composing)", n)
		}
	})
}

func TestShortcutInTextInput(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	p := &keyPage{focus: true}
	_, s, cv := open(t, a, fb, 300, 100, p.page)
	for cv.FlushFrame() {
	}
	if on, _ := cv.TextInput(); !on {
		t.Fatal("text input not on")
	}
	fired := map[string]int{}
	for _, acc := range []string{"CmdOrCtrl+N", "CmdOrCtrl+C", "CmdOrCtrl+Left", "Escape", "Shift+A", "F5", "Alt+X"} {
		shortcut(t, s, acc, uihost.ShortcutOptions{}, func() { fired[acc]++ })
	}
	press(cv, input.KeyN, cmd())    // fires: CmdOrCtrl + a non-editing key
	press(cv, input.KeyC, cmd())    // copy stays with the field
	press(cv, input.KeyLeft, cmd()) // caret movement stays with the field
	press(cv, input.KeyEscape, 0)   // plain key stays with the field
	press(cv, input.KeyA, input.ModShift)
	press(cv, input.KeyF5, 0) // function keys fire
	press(cv, input.KeyX, input.ModAlt)
	fb.Do(func() {
		want := map[string]int{"CmdOrCtrl+N": 1, "F5": 1}
		if len(fired) != len(want) || fired["CmdOrCtrl+N"] != 1 || fired["F5"] != 1 {
			t.Errorf("fired %v, want %v", fired, want)
		}
	})

	// InTextInput takes the key even from a focused field.
	all := 0
	rm := shortcut(t, s, "Escape", uihost.ShortcutOptions{InTextInput: true}, func() { all++ })
	press(cv, input.KeyEscape, 0)
	rm()

	// Focus leaves the field: every shortcut applies again.
	fb.Do(func() { p.focus = false })
	cv.Focus(true) // any input asks for a frame
	for cv.FlushFrame() {
	}
	if on, _ := cv.TextInput(); on {
		t.Fatal("text input still on")
	}
	press(cv, input.KeyC, cmd())
	fb.Do(func() {
		if all != 1 {
			t.Errorf("InTextInput shortcut fired %d times", all)
		}
		if fired["CmdOrCtrl+C"] != 1 {
			t.Errorf("CmdOrCtrl+C without a focused field fired %d times", fired["CmdOrCtrl+C"])
		}
	})
}

func TestShortcutPanicGuarded(t *testing.T) {
	panics := make(chan tango.Panic, 1)
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool { panics <- p; return true }})
	_, s, cv := open(t, a, fb, 100, 100, func(c *ui.Context) {})
	shortcut(t, s, "F5", uihost.ShortcutOptions{}, func() { panic("boom") })
	press(cv, input.KeyF5, 0)
	select {
	case p := <-panics:
		if p.Value != "boom" || p.Where != "event handler" {
			t.Fatalf("panic %+v", p)
		}
	default:
		t.Fatal("panic not reported")
	}
	// The window keeps working after the panic.
	press(cv, input.KeyF5, 0)
	select {
	case <-panics:
	default:
		t.Fatal("second press did not fire")
	}
}
