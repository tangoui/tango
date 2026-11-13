package location_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/location"
)

// An update reaches every watcher even when one panics; the panic goes to
// OnPanic as "event handler".
func TestWatchPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	got := make(chan location.Location, 4)
	stop1, err := location.Watch(a, location.Options{}, func(location.Location, error) { panic("boom") })
	if err != nil {
		t.Fatal(err)
	}
	defer stop1()
	stop2, _ := location.Watch(a, location.Options{}, func(l location.Location, _ error) { got <- l })
	defer stop2()
	if !fb.MoveTo(location.Location{Latitude: 1}, nil) {
		t.Fatal("no session")
	}
	select {
	case l := <-got:
		if l.Latitude != 1 {
			t.Fatal(l)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("second watcher not called")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"event handler"}) {
		t.Fatalf("where %q", where)
	}
}
