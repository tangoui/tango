package flyout

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/tray"
)

// A panicking OnOpen handler goes to OnPanic as "event handler"; the next
// handler still runs and the panel is open.
func TestOpenPanic(t *testing.T) {
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
	f, err := New(a, tr, Options{Width: 300, Height: 200, URL: "/panel.html"})
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	f.OnOpen(func() { panic("boom") })
	f.OnOpen(func() { opened++ })
	if !fb.Trays()[0].Click() {
		t.Fatal("click not delivered")
	}
	waitFor(t, "open", f.IsOpen)
	n := tango.Await(a, func() int { return opened })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"event handler"}) || n != 1 {
		t.Fatalf("where %q, opened %d", where, n)
	}
}
