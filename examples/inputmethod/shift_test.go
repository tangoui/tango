package main

import (
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/examples/inputmethod/engine"
	"github.com/tangoui/tango/sys/ime"
)

func TestShiftTapSimple(t *testing.T) {
	var tap shiftTap
	now := time.Now()
	tap.Down("ShiftLeft", now)
	if !tap.Up("ShiftLeft", now.Add(100*time.Millisecond)) {
		t.Fatal("a short Shift press should toggle")
	}
}

func TestShiftTapWithKey(t *testing.T) {
	var tap shiftTap
	now := time.Now()
	tap.Down("ShiftLeft", now)
	tap.Key()
	if tap.Up("ShiftLeft", now.Add(100*time.Millisecond)) {
		t.Fatal("Shift plus another key should pass through")
	}
}

func TestShiftTapSlow(t *testing.T) {
	var tap shiftTap
	now := time.Now()
	tap.Down("ShiftLeft", now)
	if tap.Up("ShiftLeft", now.Add(600*time.Millisecond)) {
		t.Fatal("a long Shift press should pass through")
	}
}

func TestShiftTapOtherModifier(t *testing.T) {
	now := time.Now()
	for _, codes := range [][2]string{
		{"ShiftLeft", "ControlLeft"},
		{"MetaLeft", "ShiftLeft"},
		{"ShiftRight", "ShiftLeft"},
	} {
		var tap shiftTap
		tap.Down(codes[0], now)
		tap.Down(codes[1], now.Add(10*time.Millisecond))
		for _, code := range codes {
			if tap.Up(code, now.Add(100*time.Millisecond)) {
				t.Errorf("%v: releasing %s should pass through", codes, code)
			}
		}
	}
}

func TestEnglishModePassesLetters(t *testing.T) {
	c := engine.New()
	if _, _, consumed := handle(c, ime.Key{Code: "KeyA", Text: "a"}, "en"); consumed || c.Composing() {
		t.Fatal("English mode should pass letters through")
	}
}

func TestCapsLockPassesLetters(t *testing.T) {
	c := engine.New()
	for _, key := range []ime.Key{
		{Code: "KeyA", Text: "a", CapsLock: true, Shift: true},
		{Code: "KeyA", Text: "A", CapsLock: true},
		{Code: "KeyA", Text: "A", Shift: true},
	} {
		if _, _, consumed := handle(c, key, "zh"); consumed || c.Composing() {
			t.Errorf("key %+v should pass through", key)
		}
	}
}

func TestToggleFlushes(t *testing.T) {
	c := engine.New()
	for _, r := range "nihao" {
		c.Feed(engine.Key{Rune: r})
	}
	var calls []string
	mode := pinyinMode{current: "zh", set: func(id string) error {
		calls = append(calls, id)
		return nil
	}, flush: func() { calls = append(calls, c.Flush()) }}
	var tap shiftTap
	now := time.Now()
	tap.Down("ShiftLeft", now)
	if tap.Up("ShiftLeft", now.Add(100*time.Millisecond)) {
		mode.toggle()
	}
	if mode.current != "en" || !slices.Equal(calls, []string{"nihao", "en"}) || c.Composing() {
		t.Fatalf("mode = %q, calls = %v, composing = %t", mode.current, calls, c.Composing())
	}
}

func TestModeChangedSync(t *testing.T) {
	flushed := 0
	mode := pinyinMode{current: "zh", flush: func() { flushed++ }}
	mode.changed("en")
	mode.changed("en")
	if mode.current != "en" || flushed != 1 {
		t.Fatalf("mode = %q, flush calls = %d", mode.current, flushed)
	}
}

func TestModeMenu(t *testing.T) {
	opened := false
	mode := pinyinMode{current: "zh", about: func() { opened = true }}
	got := mode.menu()
	if len(got) != 3 || got[0].Title != "切换中/英" || !got[0].Checked || !got[1].Separator || got[2].Title != "关于" {
		t.Fatalf("menu = %+v", got)
	}
	mode.click(got[2].ID)
	if !opened {
		t.Fatal("About callback did not run")
	}
}
