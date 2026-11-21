package uihost_test

import (
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

func TestShortcutWithContextPassesDeclinedKeysToFocusedInput(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	p := keyPage{focus: true}
	_, s, cv := open(t, a, fb, 200, 100, p.page)
	cv.FlushFrame()
	var got []uihost.ShortcutContext
	_, err := uihost.ShortcutWithContext(s, "F5", uihost.ShortcutContextOptions{}, func(c uihost.ShortcutContext) bool {
		got = append(got, c)
		return false
	})
	if err != nil {
		t.Fatal(err)
	}
	cv.Input(input.Composition{Text: "ni"})
	cv.Input(input.KeyEvent{Key: input.KeyF5, Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyF5, Down: true, Repeat: true})
	cv.Input(input.TextInput{Text: "你"})
	cv.Input(input.KeyEvent{Key: input.KeyF5})
	cv.FlushFrame()
	fb.Do(func() {
		want := []uihost.ShortcutContext{
			{Event: input.KeyEvent{Key: input.KeyF5, Down: true}, TextInput: true, Composing: true},
			{Event: input.KeyEvent{Key: input.KeyF5, Down: true, Repeat: true}, TextInput: true, Composing: true},
			{Event: input.KeyEvent{Key: input.KeyF5}, TextInput: true},
		}
		if !slices.Equal(got, want) || p.text != "你" {
			t.Errorf("contexts = %+v, text = %q", got, p.text)
		}
	})
}

func TestShortcutWithContextConsumesOnlyTheEventsItClaims(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var seen []input.KeyEvent
	_, s, cv := open(t, a, fb, 200, 100, func(c *ui.Context) {
		seen = append(seen, c.KeysFor(0)...)
	})
	var got []input.KeyEvent
	_, err := uihost.ShortcutWithContext(s, "CmdOrCtrl+K", uihost.ShortcutContextOptions{}, func(c uihost.ShortcutContext) bool {
		got = append(got, c.Event)
		return c.Event.Down
	})
	if err != nil {
		t.Fatal(err)
	}
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: cmd(), Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyK, Down: true, Repeat: true})
	cv.Input(input.KeyEvent{Key: input.KeyK}) // modifier released first
	cv.FlushFrame()
	fb.Do(func() {
		if len(got) != 3 || !got[1].Repeat || got[2].Down {
			t.Errorf("callback events = %+v", got)
		}
		if len(seen) != 1 || seen[0].Down {
			t.Errorf("runner events = %+v, want just release", seen)
		}
	})
}

func TestContextualPressRetainsReleaseAcrossDeclinedRepeat(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var repeated bool
	_, s, cv := open(t, a, fb, 200, 100, func(c *ui.Context) {
		for _, event := range c.KeysFor(0) {
			repeated = repeated || event.Repeat
		}
	})
	var released bool
	_, err := uihost.ShortcutWithContext(s, "Ctrl+K", uihost.ShortcutContextOptions{}, func(c uihost.ShortcutContext) bool {
		released = !c.Event.Down
		return !c.Event.Repeat
	})
	if err != nil {
		t.Fatal(err)
	}
	shortcut(t, s, "Ctrl+K", uihost.ShortcutOptions{}, func() { t.Error("legacy shortcut fired") })
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: input.ModCtrl, Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: input.ModCtrl, Down: true, Repeat: true})
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: input.ModCtrl})
	cv.FlushFrame()
	fb.Do(func() {
		if !released || !repeated {
			t.Fatalf("context received release = %v, runner received repeat = %v", released, repeated)
		}
	})
}
