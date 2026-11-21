package uihost

import (
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
)

func bindTestSequence(t *testing.T, s *shortcuts, sequence string, fn func(ShortcutContext) bool) {
	t.Helper()
	b, err := parseShortcutSequence(sequence)
	if err != nil {
		t.Fatal(err)
	}
	b.fn = fn
	s.contextual = append(s.contextual, b)
}

func TestContextChordTimeoutMismatchAndFallback(t *testing.T) {
	s := newShortcuts(nil, nil)
	var got []string
	bindTestSequence(t, s, "Ctrl+K Ctrl+S", func(c ShortcutContext) bool {
		if c.Event.Down && !c.Event.Repeat {
			if c.Prefix {
				got = append(got, "prefix")
			} else {
				got = append(got, "save")
			}
		}
		return true
	})
	bindTestSequence(t, s, "Ctrl+K Ctrl+X", func(c ShortcutContext) bool {
		if !c.Prefix && c.Event.Down {
			got = append(got, "close")
		}
		return true
	})
	bindTestSequence(t, s, "Ctrl+S", func(ShortcutContext) bool {
		got = append(got, "single")
		return true
	})
	at := time.Now()
	press := func(k input.Key, delay time.Duration) bool {
		return s.contextKey(input.KeyEvent{Key: k, Down: true, Mods: input.ModCtrl}, false, false, at.Add(delay))
	}
	press(input.KeyK, 0)
	press(input.KeyX, 50*time.Millisecond)
	press(input.KeyK, 100*time.Millisecond)
	if press(input.KeyJ, 110*time.Millisecond) {
		t.Error("mismatched stroke was consumed")
	}
	press(input.KeyS, 120*time.Millisecond)
	press(input.KeyK, 130*time.Millisecond)
	press(input.KeyS, 1700*time.Millisecond)
	press(input.KeyK, 1800*time.Millisecond)
	press(input.KeyS, 1900*time.Millisecond)
	want := []string{"prefix", "close", "prefix", "single", "prefix", "single", "prefix", "save"}
	if !slices.Equal(got, want) {
		t.Fatalf("callbacks %v, want %v", got, want)
	}
}

func TestContextDoubleModifierRequiresTwoPhysicalTaps(t *testing.T) {
	s := newShortcuts(nil, nil)
	bindTestSequence(t, s, "Shift Shift", func(c ShortcutContext) bool {
		return !c.Event.Down
	})
	at := time.Now()
	send := func(key input.Key, down bool, ms int) bool {
		var mods input.Modifiers
		if down {
			mods = input.ModShift
		}
		return s.contextKey(input.KeyEvent{Key: key, Down: down, Mods: mods}, false, false, at.Add(time.Duration(ms)*time.Millisecond))
	}
	send(input.KeyShiftLeft, true, 0)
	if send(input.KeyShiftLeft, false, 400) {
		t.Fatal("a long hold counted as a tap")
	}
	send(input.KeyShiftLeft, true, 450)
	send(input.KeyShiftLeft, false, 460)
	send(input.KeyJ, true, 470)
	send(input.KeyShiftRight, true, 480)
	if send(input.KeyShiftRight, false, 490) {
		t.Fatal("an intervening key failed to cancel the first tap")
	}
	send(input.KeyShiftRight, true, 500)
	if !send(input.KeyShiftRight, false, 510) {
		t.Fatal("the second physical release was not consumed")
	}
	send(input.KeyShiftRight, true, 520)
	send(input.KeyShiftRight, false, 530)
	s.handle(input.WindowFocus{Focused: false}, false)
	send(input.KeyShiftRight, true, 540)
	if send(input.KeyShiftRight, false, 550) {
		t.Fatal("sequence carried over after losing focus")
	}
}

func TestContextChordDeclinedFinalStrokeDoesNotRestartItself(t *testing.T) {
	s := newShortcuts(nil, nil)
	bindTestSequence(t, s, "Ctrl+K Ctrl+K", func(c ShortcutContext) bool { return c.Prefix })
	press := input.KeyEvent{Key: input.KeyK, Down: true, Mods: input.ModCtrl}
	release := press
	release.Down = false
	now := time.Now()
	if !s.contextKey(press, false, false, now) {
		t.Fatal("prefix did not arm")
	}
	s.contextKey(release, false, false, now)
	if s.contextKey(press, false, false, now) {
		t.Fatal("declined final stroke was consumed as a new prefix")
	}
}
