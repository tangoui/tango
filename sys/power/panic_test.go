package power_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/power"
)

// Each handler is guarded on its own: a panicking OnSuspend / OnChange
// handler goes to OnPanic as "event handler" and the next one still runs.
func TestHandlerPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.NewApp(tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	suspended, changed := 0, 0
	power.OnSuspend(a, func() { panic("boom") })
	power.OnSuspend(a, func() { suspended++ })
	power.OnChange(a, func(power.Status) { panic("boom") })
	power.OnChange(a, func(power.Status) { changed++ })
	faketest.Start(t, a, fb)

	waitFor(t, "handler", func() bool { return fb.EmitPower(platform.PowerSuspending) })
	fb.SetPower(platform.PowerStatus{Source: power.Battery, HasBattery: true, Battery: 0.5})
	waitFor(t, "change", func() bool { return tango.Await(a, func() bool { return changed == 1 }) })
	n := tango.Await(a, func() int { return suspended })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"event handler", "event handler"}) || n != 1 {
		t.Fatalf("where %q, suspended %d", where, n)
	}
}
