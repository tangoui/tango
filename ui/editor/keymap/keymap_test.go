package keymap

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/when"
	"github.com/tangoui/tango/ui/input"
)

// TestParseKeyMinCodeCompat transcribes MinCode's
// vue-app/scripts/keybinding-keys.test.mjs (the terminal find key test is
// MinCode-specific and omitted).
func TestParseKeyMinCodeCompat(t *testing.T) {
	t.Run("platform detection", func(t *testing.T) {
		for _, c := range []struct {
			name string
			want Platform
		}{
			{"MacIntel", Mac}, {"macOS", Mac}, {"Win32", Windows}, {"Windows", Windows},
			{"Linux x86_64", Linux}, {"", Linux},
			{"darwin", Mac}, {"windows", Windows}, {"linux", Linux}, {"freebsd", Linux},
		} {
			if got := PlatformFrom(c.name); got != c.want {
				t.Errorf("PlatformFrom(%q) = %v, want %v", c.name, got, c.want)
			}
		}
	})

	id := func(text string, p Platform) string {
		t.Helper()
		s, err := ParseKey(text, p)
		if err != nil {
			t.Fatalf("ParseKey(%q, %v): %v", text, p, err)
		}
		return s.String()
	}

	t.Run("mod expands to meta on macOS and ctrl elsewhere", func(t *testing.T) {
		for _, c := range []struct {
			text string
			p    Platform
			want string
		}{
			// sequenceId orders modifiers ctrl, shift, alt, meta.
			{"mod+shift+p", Mac, "shift+meta+p"},
			{"mod+shift+p", Windows, "ctrl+shift+p"},
			{"Mod+Shift+P", Linux, "ctrl+shift+p"},
			{"cmd+alt+up", Windows, "alt+meta+up"},
			{"option+f12", Mac, "alt+f12"},
		} {
			if got := id(c.text, c.p); got != c.want {
				t.Errorf("%q on %v = %q, want %q", c.text, c.p, got, c.want)
			}
		}
	})

	t.Run("chords, double modifiers and named keys", func(t *testing.T) {
		s, err := ParseKey("cmd+k cmd+s", Mac)
		if err != nil {
			t.Fatal(err)
		}
		want := Sequence{Chords: []Chord{{input.ModMeta, input.KeyK}, {input.ModMeta, input.KeyS}}}
		if !s.Equal(want) {
			t.Errorf("cmd+k cmd+s = %+v", s)
		}
		if s, _ := ParseKey("shift shift", Mac); !s.Equal(Sequence{Double: input.ModShift}) {
			t.Errorf("shift shift = %+v", s)
		}
		if s, _ := ParseKey("mod mod", Windows); !s.Equal(Sequence{Double: input.ModCtrl}) {
			t.Errorf("mod mod on windows = %+v", s)
		}
		for _, c := range []struct {
			text string
			p    Platform
			want string
		}{
			{"ctrl+`", Windows, "ctrl+`"},
			{"ctrl+shift+-", Mac, "ctrl+shift+-"},
			{"ctrl+pagedown", Windows, "ctrl+pagedown"},
			{"escape", Windows, "escape"},
			{"esc", Windows, "escape"},
			// Tango additions: VS Code names MinCode lacks.
			{"numpad7", Linux, "numpad7"},
			{"ctrl+numpad_add", Linux, "ctrl+numpad_add"},
			{"mod+k mod+c", Mac, "meta+k meta+c"},
			{"mod+k mod+c", Linux, "ctrl+k ctrl+c"},
			{"return", Linux, "enter"},
			{"del", Linux, "delete"},
			{"space", Linux, "space"},
			{"shift+[", Linux, "shift+["},
			{"\\", Linux, "\\"},
		} {
			if got := id(c.text, c.p); got != c.want {
				t.Errorf("%q on %v = %q, want %q", c.text, c.p, got, c.want)
			}
		}
	})

	t.Run("invalid key text is rejected", func(t *testing.T) {
		for _, text := range []string{
			"", "   ", "mod+", "+k", "ctrl+ctrl+k", "hyper+k", "ctrl+shift", "a b c",
			"shift alt", "ctrl+f25", strings.Repeat("x", 80), "ctrl+k shift",
		} {
			if _, err := ParseKey(text, Mac); err == nil {
				t.Errorf("ParseKey(%q) accepted", text)
			}
		}
	})

	t.Run("events map through the physical code, not the produced character", func(t *testing.T) {
		chord := func(ev input.KeyEvent) string {
			c, ok := ChordOf(ev)
			if !ok {
				return "<none>"
			}
			return c.String()
		}
		// MinCode's case: an event whose Code is the physical key (a
		// browser's KeyboardEvent.code; in Tango a platform that fills
		// Code, such as Windows) maps by Code even when Key is not the
		// letter (⌥L typing "¬"). Tango's macOS host does not deliver
		// this today: it sends no KeyEvent for text-producing keys and
		// never fills Code (see the package doc).
		if got := chord(input.KeyEvent{Key: input.KeyUnknown, Code: input.KeyL, Mods: input.ModAlt, Down: true}); got != "alt+l" {
			t.Errorf("⌥L = %q", got)
		}
		if got := chord(input.KeyEvent{Key: input.KeyShiftLeft, Code: input.KeyShiftLeft, Mods: input.ModShift, Down: true}); got != "<none>" {
			t.Errorf("ShiftLeft = %q", got)
		}
		if got := chord(input.KeyEvent{Key: input.KeyMetaRight, Code: input.KeyMetaRight, Mods: input.ModMeta, Down: true}); got != "<none>" {
			t.Errorf("MetaRight = %q", got)
		}
		if c, _ := ChordOf(input.KeyEvent{Key: input.KeyEnter, Code: input.KeyNumpadEnter, Down: true}); c.Key != input.KeyEnter {
			t.Errorf("NumpadEnter → %v", c.Key)
		}
		if c, _ := ChordOf(input.KeyEvent{Key: input.Key7, Code: input.KeyNumpad7, Down: true}); c.Key != input.Key7 {
			t.Errorf("Numpad7 → %v", c.Key)
		}
		// Events without a code fall back to the key.
		if c, _ := ChordOf(input.KeyEvent{Key: input.KeyK, Mods: input.ModMeta, Down: true}); c.Key != input.KeyK {
			t.Errorf("no code, key K → %v", c.Key)
		}
		if c, _ := ChordOf(input.KeyEvent{Key: input.KeyUp, Down: true}); c.Key != input.KeyUp {
			t.Errorf("no code, key Up → %v", c.Key)
		}
		// MinCode's "Process" (IME) key: an unknown key has no chord.
		if got := chord(input.KeyEvent{Key: input.KeyUnknown, Down: true}); got != "<none>" {
			t.Errorf("unknown key = %q", got)
		}
		// Tango: a layout letter differing from the physical key (AZERTY A
		// on the Q position) matches by position.
		if got := chord(input.KeyEvent{Key: input.KeyA, Code: input.KeyQ, Mods: input.ModCtrl, Down: true}); got != "ctrl+q" {
			t.Errorf("AZERTY A = %q", got)
		}
		// NumLock off: keypad 8 reported as Up keeps Up.
		if got := chord(input.KeyEvent{Key: input.KeyUp, Code: input.KeyNumpad8, Down: true}); got != "up" {
			t.Errorf("keypad Up = %q", got)
		}
	})

	t.Run("labels use mac glyphs and Windows/Linux text", func(t *testing.T) {
		for _, c := range []struct {
			text string
			p    Platform
			want string
		}{
			{"mod+shift+p", Mac, "⇧⌘P"},
			{"ctrl+alt+shift+cmd+k", Mac, "⌃⌥⇧⌘K"},
			{"mod+shift+p", Windows, "Ctrl+Shift+P"},
			{"mod+k mod+s", Mac, "⌘K ⌘S"},
			{"mod+k mod+s", Linux, "Ctrl+K Ctrl+S"},
			{"shift shift", Mac, "⇧⇧"},
			{"shift shift", Windows, "Shift Shift"},
			{"alt+up", Mac, "⌥↑"},
			{"alt+up", Windows, "Alt+Up"},
			{"mod+enter", Mac, "⌘↩"},
			{"mod+backspace", Mac, "⌘⌫"},
			{"escape", Mac, "⎋"},
			{"escape", Windows, "Esc"},
			{"ctrl+tab", Mac, "⌃⇥"},
			{"mod+/", Windows, "Ctrl+/"},
			{"meta+k", Windows, "Win+K"},
			{"meta+k", Linux, "Super+K"},
			{"f12", Mac, "F12"},
			{"not a key!", Mac, "not a key!"},
			{"mod+space", Mac, "⌘␣"},
		} {
			if got := Label(c.text, c.p); got != c.want {
				t.Errorf("Label(%q, %v) = %q, want %q", c.text, c.p, got, c.want)
			}
		}
	})
}

func press(k input.Key, mods input.Modifiers) input.KeyEvent {
	return input.KeyEvent{Key: k, Code: k, Down: true, Mods: mods}
}

func release(k input.Key, mods input.Modifiers) input.KeyEvent {
	return input.KeyEvent{Key: k, Code: k, Mods: mods}
}

func TestKeymapRemovalRule(t *testing.T) {
	defaults := []Rule{
		{Key: "ctrl+a", Command: "selectAll"},
		{Key: "ctrl+b", Command: "selectAll"},
		{Key: "ctrl+c", Command: "copy", When: "a && b"},
		{Key: "ctrl+c", Command: "copy", When: "c"},
		{Key: "ctrl+d", Command: "dup"},
		{Key: "ctrl+e", Command: "dup"},
	}
	user := []Rule{
		{Key: "ctrl+a", Command: "-selectAll"},          // that key only
		{Key: "ctrl+c", Command: "-copy", When: "a&&b"}, // that when only (same expression)
		{Command: "-dup"},                    // every binding
		{Key: "ctrl+x", Command: "-nothing"}, // no match: harmless
	}
	k := New(Linux, defaults, user)
	if errs := k.Errors(); len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	ctx := when.MapContext{"a": true, "b": true, "c": true}
	resolve := func(key input.Key) string {
		cmd, _, _ := k.Resolve(press(key, input.ModCtrl), ctx)
		return cmd
	}
	if got := resolve(input.KeyA); got != "" {
		t.Errorf("ctrl+a = %q, want removed", got)
	}
	if got := resolve(input.KeyB); got != "selectAll" {
		t.Errorf("ctrl+b = %q, want kept", got)
	}
	if got := resolve(input.KeyC); got != "copy" {
		t.Errorf("ctrl+c = %q, want the c rule", got)
	}
	ctx["c"] = false
	if got := resolve(input.KeyC); got != "" {
		t.Errorf("ctrl+c without c = %q: the a && b rule was not removed", got)
	}
	if resolve(input.KeyD) != "" || resolve(input.KeyE) != "" {
		t.Error("-dup without key did not remove every dup binding")
	}
	// A removal only targets default rules: a user rule stays, wherever
	// the removal is.
	k = New(Linux, []Rule{{Key: "ctrl+a", Command: "x"}}, []Rule{{Key: "ctrl+a", Command: "-x"}, {Key: "ctrl+a", Command: "x"}})
	if cmd, _, _ := k.Resolve(press(input.KeyA, input.ModCtrl), nil); cmd != "x" {
		t.Errorf("rule after its removal: %q", cmd)
	}
	// Bad rules are reported, not fatal.
	k = New(Linux, []Rule{{Key: "hyper+a", Command: "x"}, {Key: "ctrl+a", Command: "y", When: "a &&"}, {Key: "ctrl+b", Command: "z"}})
	if len(k.Errors()) != 2 {
		t.Errorf("errors = %v, want 2", k.Errors())
	}
	if cmd, _, _ := k.Resolve(press(input.KeyB, input.ModCtrl), nil); cmd != "z" {
		t.Errorf("valid rule after bad ones: %q", cmd)
	}
}

func TestKeymapWhenPriority(t *testing.T) {
	k := New(Mac, []Rule{
		{Key: "cmd+d", Command: "first", When: "editorTextFocus"},
		{Key: "cmd+d", Command: "second", When: "editorTextFocus && editorHasSelection", Args: 2},
	}, []Rule{{Key: "cmd+e", Command: "e1"}, {Key: "cmd+e", Command: "e2", When: "never"}})
	ctx := when.MapContext{"editorTextFocus": true}
	if cmd, _, _ := k.Resolve(press(input.KeyD, input.ModMeta), ctx); cmd != "first" {
		t.Errorf("without selection: %q", cmd)
	}
	ctx["editorHasSelection"] = true
	cmd, args, _ := k.Resolve(press(input.KeyD, input.ModMeta), ctx)
	if cmd != "second" || args != 2 {
		t.Errorf("with selection: %q %v (the later rule wins)", cmd, args)
	}
	ctx["editorTextFocus"] = false
	if cmd, _, _ := k.Resolve(press(input.KeyD, input.ModMeta), ctx); cmd != "" {
		t.Errorf("no rule's when holds: %q", cmd)
	}
	// A later rule whose when fails falls back to the earlier one.
	if cmd, _, _ := k.Resolve(press(input.KeyE, input.ModMeta), ctx); cmd != "e1" {
		t.Errorf("cmd+e: %q", cmd)
	}
	// A later rule set overrides the same key.
	k = New(Mac, []Rule{{Key: "cmd+d", Command: "default"}}, []Rule{{Key: "cmd+d", Command: "user"}})
	if cmd, _, _ := k.Resolve(press(input.KeyD, input.ModMeta), nil); cmd != "user" {
		t.Errorf("user over default: %q", cmd)
	}
	// Modifiers must match exactly.
	if cmd, _, _ := k.Resolve(press(input.KeyD, input.ModMeta|input.ModShift), nil); cmd != "" {
		t.Errorf("shift+cmd+d matched cmd+d: %q", cmd)
	}
}

func TestChordAndTimeoutFree(t *testing.T) {
	k := New(Mac, []Rule{
		{Key: "mod+k mod+c", Command: "addComment", When: "editorTextFocus"},
		{Key: "mod+k mod+u", Command: "removeComment"},
		{Key: "mod+k", Command: "single"},
		{Key: "mod+c", Command: "copy"},
	})
	ctx := when.MapContext{"editorTextFocus": true}
	// mod+k alone is bound later than the chords, so it wins as a single
	// key (VS Code: the highest-priority matching rule decides).
	if cmd, _, pending := k.Resolve(press(input.KeyK, input.ModMeta), ctx); cmd != "single" || pending {
		t.Fatalf("mod+k with a later single binding: %q pending %v", cmd, pending)
	}
	k = New(Mac, []Rule{
		{Key: "mod+k", Command: "single"},
		{Key: "mod+k mod+c", Command: "addComment", When: "editorTextFocus"},
		{Key: "mod+k mod+u", Command: "removeComment"},
		{Key: "mod+c", Command: "copy"},
	})
	step := func(ev input.KeyEvent, wantCmd string, wantPending bool) {
		t.Helper()
		cmd, _, pending := k.Resolve(ev, ctx)
		if cmd != wantCmd || pending != wantPending || k.Pending() != wantPending {
			t.Fatalf("%v: %q pending %v (Pending %v), want %q %v", ev, cmd, pending, k.Pending(), wantCmd, wantPending)
		}
	}
	step(press(input.KeyK, input.ModMeta), "", true)
	step(release(input.KeyK, input.ModMeta), "", true)
	step(press(input.KeyMetaLeft, input.ModMeta), "", true) // modifiers keep the chord
	step(press(input.KeyC, input.ModMeta), "addComment", false)
	// No timeout: the second chord may come any time later.
	step(press(input.KeyK, input.ModMeta), "", true)
	step(press(input.KeyU, input.ModMeta), "removeComment", false)
	// Another key in between cancels the chord and is swallowed.
	step(press(input.KeyK, input.ModMeta), "", true)
	step(press(input.KeyX, 0), "", false)
	step(press(input.KeyC, input.ModMeta), "copy", false)
	// Escape cancels too.
	step(press(input.KeyK, input.ModMeta), "", true)
	step(press(input.KeyEscape, 0), "", false)
	// The when is checked: without editorTextFocus mod+k mod+c does not
	// apply (mod+k is still a prefix, of mod+k mod+u).
	ctx["editorTextFocus"] = false
	step(press(input.KeyK, input.ModMeta), "", true)
	step(press(input.KeyC, input.ModMeta), "", false)
	// ClearPending.
	step(press(input.KeyK, input.ModMeta), "", true)
	k.ClearPending()
	if k.Pending() {
		t.Fatal("ClearPending left the chord")
	}
	step(press(input.KeyC, input.ModMeta), "copy", false)
	// A Clone has its own state.
	step(press(input.KeyK, input.ModMeta), "", true)
	c := k.Clone()
	if c.Pending() {
		t.Error("Clone copied the chord state")
	}
	if cmd, _, _ := c.Resolve(press(input.KeyC, input.ModMeta), ctx); cmd != "copy" {
		t.Errorf("clone: %q", cmd)
	}
}

func TestKeymapNumpadAndDoubleTap(t *testing.T) {
	k := New(Linux, []Rule{
		{Key: "7", Command: "seven"},
		{Key: "numpad8", Command: "pad8"},
		{Key: "8", Command: "eight"},
		{Key: "enter", Command: "enter"},
		{Key: "shift shift", Command: "searchEverywhere", Args: "x"},
	})
	res := func(ev input.KeyEvent) string {
		cmd, _, _ := k.Resolve(ev, nil)
		return cmd
	}
	if got := res(input.KeyEvent{Key: input.Key7, Code: input.KeyNumpad7, Down: true}); got != "seven" {
		t.Errorf("Numpad7: %q", got)
	}
	if got := res(input.KeyEvent{Key: input.Key8, Code: input.KeyNumpad8, Down: true}); got != "eight" {
		t.Errorf("Numpad8: %q (the later rule \"8\" wins)", got)
	}
	if got := res(input.KeyEvent{Key: input.Key8, Code: input.Key8, Down: true}); got != "eight" {
		t.Errorf("8: %q", got)
	}
	if got := res(input.KeyEvent{Key: input.KeyEnter, Code: input.KeyNumpadEnter, Down: true}); got != "enter" {
		t.Errorf("NumpadEnter: %q", got)
	}

	tap := func() (string, any) {
		k.Resolve(input.KeyEvent{Key: input.KeyShiftLeft, Code: input.KeyShiftLeft, Down: true, Mods: input.ModShift}, nil)
		cmd, args, _ := k.Resolve(input.KeyEvent{Key: input.KeyShiftLeft, Code: input.KeyShiftLeft}, nil)
		return cmd, args
	}
	if cmd, _ := tap(); cmd != "" {
		t.Fatalf("first tap fired %q", cmd)
	}
	if cmd, args := tap(); cmd != "searchEverywhere" || args != "x" {
		t.Fatalf("second tap: %q %v", cmd, args)
	}
	// Shift+A between the taps is not a double tap.
	tap()
	res(press(input.KeyA, input.ModShift))
	if cmd, _ := tap(); cmd != "" {
		t.Errorf("tap after shift+a fired %q", cmd)
	}
	// Timing with a clock.
	now := time.Unix(0, 0)
	k.SetClock(func() time.Time { return now })
	k.ClearPending()
	tap()
	now = now.Add(time.Second)
	if cmd, _ := tap(); cmd != "" {
		t.Errorf("taps 1s apart fired %q", cmd)
	}
	now = now.Add(100 * time.Millisecond)
	if cmd, _ := tap(); cmd != "searchEverywhere" {
		t.Errorf("taps 100ms apart: %q", cmd)
	}
}

func TestDefaultRulesParse(t *testing.T) {
	for _, p := range []Platform{Mac, Windows, Linux} {
		k := New(p, DefaultRules(p))
		if errs := k.Errors(); len(errs) != 0 {
			t.Errorf("%v: %v", p, errs)
		}
	}
	// Spot checks of Monaco's platform split.
	ctx := when.MapContext{"textInputFocus": true, "editorTextFocus": true}
	for _, c := range []struct {
		p    Platform
		ev   input.KeyEvent
		want string
	}{
		{Mac, press(input.KeyLeft, input.ModAlt), "cursorWordLeft"},
		{Windows, press(input.KeyLeft, input.ModCtrl), "cursorWordLeft"},
		{Linux, press(input.KeyLeft, input.ModAlt), ""},
		{Mac, press(input.KeyLeft, input.ModMeta), "cursorHome"},
		{Mac, press(input.KeyUp, input.ModMeta), "cursorTop"},
		{Windows, press(input.KeyHome, input.ModCtrl), "cursorTop"},
		{Mac, press(input.KeyZ, input.ModMeta|input.ModShift), "redo"},
		{Linux, press(input.KeyY, input.ModCtrl), "redo"},
		{Mac, press(input.KeyY, input.ModMeta), ""},
		{Windows, press(input.KeyInsert, input.ModShift), "editor.action.clipboardPasteAction"},
		{Mac, press(input.KeyM, input.ModCtrl|input.ModShift), "editor.action.toggleTabFocusMode"},
		{Linux, press(input.KeyM, input.ModCtrl), "editor.action.toggleTabFocusMode"},
		{Mac, press(input.KeyBackspace, input.ModMeta), "deleteAllLeft"},
		{Linux, press(input.KeyBackspace, input.ModCtrl), "deleteWordLeft"},
	} {
		k := New(c.p, DefaultRules(c.p))
		if cmd, _, _ := k.Resolve(c.ev, ctx); cmd != c.want {
			t.Errorf("%v %v: %q, want %q", c.p, c.ev, cmd, c.want)
		}
	}
	// Keys lists a command's bindings, highest priority first: the
	// primary key (Monaco's kbOpts.primary) before the secondary ones.
	k := New(Mac, DefaultRules(Mac))
	keys := k.Keys("cursorHome", nil)
	if len(keys) != 2 || keys[0].Label(Mac) != "↖" || keys[1].Label(Mac) != "⌘←" {
		t.Errorf("Keys(cursorHome) = %v", keys)
	}
	// Monaco's ⌃⇧M is the primary key of toggleTabFocusMode on macOS,
	// before the editor's Ctrl+M extension.
	if keys := k.Keys("editor.action.toggleTabFocusMode", nil); len(keys) != 2 || keys[0].Label(Mac) != "⌃⇧M" {
		t.Errorf("Keys(toggleTabFocusMode) = %v", keys)
	}
}

func TestKeymapRemovalMonaco(t *testing.T) {
	res := func(k *Keymap, ctx when.Context, evs ...input.KeyEvent) string {
		t.Helper()
		k.ClearPending()
		var cmd string
		for _, ev := range evs {
			cmd, _, _ = k.Resolve(ev, ctx)
		}
		return cmd
	}
	kc := press(input.KeyK, input.ModCtrl)
	cc := press(input.KeyC, input.ModCtrl)
	all := when.MapContext{"a": true, "b": true, "c": true}

	// 1. The removal's key is a prefix of the default's sequence.
	defaults := []Rule{
		{Key: "ctrl+k ctrl+c", Command: "x"},
		{Key: "ctrl+k ctrl+d", Command: "y"},
		{Key: "ctrl+c", Command: "x"},
	}
	k := New(Linux, defaults, []Rule{{Key: "ctrl+k", Command: "-x"}})
	if got := res(k, nil, kc, cc); got != "" {
		t.Errorf("ctrl+k ctrl+c → %q, want removed by -x on ctrl+k", got)
	}
	if got := res(k, nil, kc, press(input.KeyD, input.ModCtrl)); got != "y" {
		t.Errorf("other command under the prefix: %q", got)
	}
	if got := res(k, nil, cc); got != "x" {
		t.Errorf("ctrl+c → %q: not under the prefix, must stay", got)
	}
	k = New(Linux, defaults, []Rule{{Key: "ctrl+k ctrl+c ", Command: "-x"}, {Key: "ctrl+c ctrl+c", Command: "-x"}})
	if res(k, nil, kc, cc) != "" || res(k, nil, cc) != "x" {
		t.Error("full-sequence removal / longer removal key")
	}

	// 2. The default's when entails the removal's.
	defaults = []Rule{
		{Key: "ctrl+a", Command: "x", When: "b && a"},
		{Key: "ctrl+b", Command: "x", When: "a || c"},
		{Key: "ctrl+c", Command: "x"},
		{Key: "ctrl+d", Command: "x", When: "(a && b) && c"},
		{Key: "ctrl+e", Command: "x", When: "c"},
	}
	k = New(Linux, defaults, []Rule{{Command: "-x", When: "a"}})
	for _, c := range []struct {
		key  input.Key
		want string
	}{
		{input.KeyA, ""},  // a && b entails a
		{input.KeyB, "x"}, // a || c does not
		{input.KeyC, "x"}, // no when: not targeted by a removal with a when
		{input.KeyD, ""},  // nesting and order ignored
		{input.KeyE, "x"},
	} {
		if got := res(k, all, press(c.key, input.ModCtrl)); got != c.want {
			t.Errorf("-x when a, ctrl+%v: %q, want %q", c.key, got, c.want)
		}
	}
	k = New(Linux, defaults, []Rule{{Command: "-x", When: "a && b && c"}})
	if res(k, all, press(input.KeyA, input.ModCtrl)) != "x" || res(k, all, press(input.KeyD, input.ModCtrl)) != "" {
		t.Error("-x when a && b && c: removes only defaults with all three terms")
	}
	k = New(Linux, defaults, []Rule{{Command: "-x", When: "c || a"}})
	if res(k, all, press(input.KeyB, input.ModCtrl)) != "" {
		t.Error("-x when c || a did not remove a || c")
	}

	// 3. Only defaults (the first rule set) are targeted, in any order.
	k = New(Linux,
		[]Rule{{Key: "ctrl+a", Command: "-x"}, {Key: "ctrl+a", Command: "x"}}, // removal before its target, both defaults
		[]Rule{{Key: "ctrl+b", Command: "x"}, {Key: "ctrl+b", Command: "-x"}}, // user rule, then removal
		[]Rule{{Key: "ctrl+c", Command: "x"}},                                 // later user set
		[]Rule{{Command: "-x"}},
	)
	if got := res(k, nil, press(input.KeyA, input.ModCtrl)); got != "" {
		t.Errorf("default after its removal: %q, want removed", got)
	}
	if res(k, nil, press(input.KeyB, input.ModCtrl)) != "x" || res(k, nil, press(input.KeyC, input.ModCtrl)) != "x" {
		t.Error("a removal removed a user rule")
	}
	// A double-tap removal needs the same modifier.
	k = New(Linux, []Rule{{Key: "shift shift", Command: "s"}, {Key: "alt alt", Command: "s"}}, []Rule{{Key: "shift shift", Command: "-s"}})
	if len(k.Keys("s", nil)) != 1 || k.Keys("s", nil)[0].Double != input.ModAlt {
		t.Errorf("double-tap removal: %v", k.Keys("s", nil))
	}
}

func TestResolveEnabled(t *testing.T) {
	k := New(Linux, []Rule{
		{Key: "ctrl+z", Command: "low"},
		{Key: "ctrl+z", Command: "undo"},
		{Key: "ctrl+k ctrl+c", Command: "comment"},
	})
	off := func(cmd string) bool { return cmd != "undo" && cmd != "comment" }
	if cmd, _, _ := k.ResolveEnabled(press(input.KeyZ, input.ModCtrl), nil, off); cmd != "low" {
		t.Errorf("disabled command shadows: %q", cmd)
	}
	if cmd, _, _ := k.ResolveEnabled(press(input.KeyZ, input.ModCtrl), nil, nil); cmd != "undo" {
		t.Errorf("enabled: %q", cmd)
	}
	if _, _, p := k.ResolveEnabled(press(input.KeyK, input.ModCtrl), nil, off); p {
		t.Error("a disabled chord's first key is consumed")
	}
}

func TestEventForText(t *testing.T) {
	for _, c := range []struct {
		text string
		key  input.Key
		mods input.Modifiers
	}{
		{"c", input.KeyC, 0}, {"C", input.KeyC, input.ModShift}, {"7", input.Key7, 0},
		{" ", input.KeySpace, 0}, {"/", input.KeySlash, 0}, {"?", input.KeySlash, input.ModShift},
		{"[", input.KeyBracketLeft, 0}, {"+", input.KeyEqual, input.ModShift}, {"`", input.KeyBackquote, 0},
	} {
		ev, ok := EventForText(c.text)
		if !ok || ev.Key != c.key || ev.Code != c.key || ev.Mods != c.mods || !ev.Down {
			t.Errorf("EventForText(%q) = %+v %v", c.text, ev, ok)
		}
	}
	for _, bad := range []string{"", "ab", "é", "你", "\n", "\t"} {
		if _, ok := EventForText(bad); ok {
			t.Errorf("EventForText(%q) ok", bad)
		}
	}
}

// TestKeymapRemovalTrueWhen: a when of "true" counts as absent on both
// sides of a removal (Monaco's whenIsEntirelyIncluded).
func TestKeymapRemovalTrueWhen(t *testing.T) {
	defaults := []Rule{
		{Key: "ctrl+a", Command: "x"},
		{Key: "ctrl+b", Command: "x", When: "a"},
		{Key: "ctrl+c", Command: "x", When: "true"},
	}
	all := when.MapContext{"a": true}
	has := func(k *Keymap, key input.Key) bool {
		k.ClearPending()
		cmd, _, _ := k.Resolve(press(key, input.ModCtrl), all)
		return cmd == "x"
	}
	k := New(Linux, defaults, []Rule{{Command: "-x", When: "true"}})
	if has(k, input.KeyA) || has(k, input.KeyB) || has(k, input.KeyC) {
		t.Error(`-x when "true" must remove every default`)
	}
	k = New(Linux, defaults, []Rule{{Key: "ctrl+a", Command: "-x", When: "true"}})
	if has(k, input.KeyA) || !has(k, input.KeyB) {
		t.Error(`-x on ctrl+a when "true": must remove the default without a when, only on ctrl+a`)
	}
	k = New(Linux, defaults, []Rule{{Command: "-x", When: "a"}})
	if !has(k, input.KeyC) || has(k, input.KeyB) || !has(k, input.KeyA) {
		t.Error(`-x when a: a default with when "true" (absent) is not targeted`)
	}
	k = New(Linux, defaults, []Rule{{Key: "ctrl+c", Command: "-x"}})
	if has(k, input.KeyC) {
		t.Error(`-x without a when must remove the default with when "true"`)
	}
}
