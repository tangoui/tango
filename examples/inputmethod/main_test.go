package main

import (
	"testing"

	"github.com/tangoui/tango/examples/inputmethod/engine"
	"github.com/tangoui/tango/sys/ime"
)

func TestHandleKey(t *testing.T) {
	for _, tc := range []struct {
		name, preedit string
		key           ime.Key
		flush         string
		consumed      bool
	}{
		{"shortcut commits raw text", "nihao", ime.Key{Code: "KeyA", Text: "a", Meta: true}, "nihao", false},
		{"tab while composing", "nihao", ime.Key{Code: "Tab"}, "", true},
		{"tab while idle", "", ime.Key{Code: "Tab"}, "", false},
		{"arrow while composing", "nihao", ime.Key{Code: "ArrowLeft"}, "", true},
		{"unknown while composing", "nihao", ime.Key{}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := engine.New()
			for _, r := range tc.preedit {
				c.Feed(engine.Key{Rune: r})
			}
			res, flush, consumed := handle(c, tc.key, "zh")
			if flush != tc.flush || consumed != tc.consumed || res.Handled {
				t.Fatalf("handle = %+v, %q, %t", res, flush, consumed)
			}
			if want := tc.preedit != "" && tc.flush == ""; c.Composing() != want {
				t.Errorf("Composing = %t, want %t", c.Composing(), want)
			}
		})
	}
}
