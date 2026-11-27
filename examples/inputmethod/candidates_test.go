package main

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/examples/inputmethod/engine"
	"github.com/tangoui/tango/sys/ime"
)

func TestCandidateBounds(t *testing.T) {
	work := tango.Rect{Width: 1440, Height: 900}
	for _, tc := range []struct {
		name  string
		caret tango.Rect
		want  tango.Rect
	}{
		{"below", tango.Rect{X: 100, Y: 200, Width: 2, Height: 18}, tango.Rect{X: 100, Y: 222, Width: 300, Height: 40}},
		{"above", tango.Rect{X: 100, Y: 880, Width: 2, Height: 18}, tango.Rect{X: 100, Y: 836, Width: 300, Height: 40}},
		{"right edge", tango.Rect{X: 1300, Y: 200, Width: 2, Height: 18}, tango.Rect{X: 1140, Y: 222, Width: 300, Height: 40}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := candidateBounds(tc.caret, 300, 40, work); got != tc.want {
				t.Errorf("bounds = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPick(t *testing.T) {
	c := composerWith("ni")
	for _, index := range []int{-1, 3} {
		if got, ok := c.Pick(index); ok || got != "" || !c.Composing() {
			t.Fatalf("Pick(%d) = %q, %t, composing=%t", index, got, ok, c.Composing())
		}
	}
	if got, ok := c.Pick(1); !ok || got != "泥" || c.Composing() {
		t.Fatalf("Pick(1) = %q, %t, composing=%t", got, ok, c.Composing())
	}
}

func TestHandleDigitPicks(t *testing.T) {
	c := composerWith("nihao")
	res, flush, consumed := handle(c, ime.Key{Code: "Digit2", Text: "2"})
	if !consumed || !res.Handled || res.Commit != "拟好" || flush != "" || c.Composing() {
		t.Fatalf("Digit2 = %+v, flush=%q, consumed=%t, composing=%t", res, flush, consumed, c.Composing())
	}
}

func composerWith(preedit string) *engine.Composer {
	c := engine.New()
	for _, ch := range preedit {
		c.Feed(engine.Key{Rune: ch})
	}
	return c
}
