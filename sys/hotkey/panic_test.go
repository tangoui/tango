package hotkey

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// A panicking global hotkey callback goes to OnPanic as "hotkey"; other
// hotkeys on the same accelerator still fire.
func TestRegisterPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	second := 0
	if _, err := Register(a, "CmdOrCtrl+Shift+P", func() { panic("boom") }); err != nil {
		t.Fatal(err)
	}
	if _, err := Register(a, "CmdOrCtrl+Shift+P", func() { second++ }); err != nil {
		t.Fatal(err)
	}
	if n := fb.PressHotkey("CmdOrCtrl+Shift+P"); n != 2 {
		t.Fatalf("fired %d", n)
	}
	n := tango.Await(a, func() int { return second })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"hotkey"}) || n != 1 {
		t.Fatalf("where %q, second %d", where, n)
	}
}
