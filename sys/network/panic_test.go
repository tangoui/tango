package network_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/network"
)

// A panicking OnChange handler goes to OnPanic as "event handler" and the
// next handler still gets the status.
func TestHandlerPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.NewApp(tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	ch := make(chan network.Status, 4)
	network.OnChange(a, func(network.Status) { panic("boom") })
	network.OnChange(a, func(s network.Status) { ch <- s })
	faketest.Start(t, a, fb)

	wifi := network.Status{Online: true, Kind: network.WiFi}
	deadline := time.Now().Add(faketest.Timeout)
	for !fb.SetNetwork(wifi) {
		if time.Now().After(deadline) {
			t.Fatal("handler never installed")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case s := <-ch:
		if s != wifi {
			t.Fatal(s)
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
