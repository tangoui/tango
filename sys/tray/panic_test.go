package tray_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/tray"
)

// A panicking click handler goes to OnPanic as "tray click" and does not
// keep the next handler from running.
func TestClickPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	tr, err := tray.New(a, tray.Options{Icon: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	second := 0
	tr.OnClick(func() { panic("boom") })
	tr.OnClick(func() { second++ })
	if !fb.Trays()[0].Click() {
		t.Fatal("click not delivered")
	}
	n := tango.Await(a, func() int { return second })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"tray click"}) || n != 1 {
		t.Fatalf("where %q, second %d", where, n)
	}
}
