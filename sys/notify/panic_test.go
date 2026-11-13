package notify_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/notify"
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
	clicked := make(chan string, 1)
	notify.OnClick(a, func(string) { panic("boom") })
	notify.OnClick(a, func(id string) { clicked <- id })
	id, err := notify.Send(a, notify.Message{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if !fb.ClickNotification(id) {
		t.Fatal("click not delivered")
	}
	select {
	case got := <-clicked:
		if got != id {
			t.Fatal(got)
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
