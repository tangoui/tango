package jumplist

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// A click reaches every OnClick handler even when one panics; the panic
// goes to OnPanic as "event handler".
func TestClickPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	got := make(chan string, 1)
	OnClick(a, func(string) { panic("boom") })
	OnClick(a, func(id string) { got <- id })
	if err := Set(a, []Item{{ID: "new", Label: "New"}}); err != nil {
		t.Fatal(err)
	}
	if !fb.ClickShortcut("new") {
		t.Fatal("click")
	}
	select {
	case id := <-got:
		if id != "new" {
			t.Fatal(id)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("second handler not called")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"event handler"}) {
		t.Fatalf("where %q", where)
	}
}
