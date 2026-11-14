// Package location reads the device's location once or continuously
// (Core Location on iOS, LocationManager on Android — no Google Play
// services needed).
//
//	loc, err := location.Current(a, location.Options{Accuracy: location.High})
//	stop, err := location.Watch(a, location.Options{}, func(l location.Location, err error) { ... })
//	defer stop()
//
// Both ask for when-in-use permission first if the user was never asked;
// ErrPermissionDenied if refused. iOS needs
// NSLocationWhenInUseUsageDescription (permission "location"; returns
// ErrNotConfigured without it); Android needs ACCESS_FINE_LOCATION and/or
// ACCESS_COARSE_LOCATION. ErrNotAvailable means location services are off.
//
// Functions return ErrUnsupported where the backend has no location and
// tango.ErrNotRunning after the app quit. On Android call Current from a
// goroutine, not from a main-thread callback.
package location

import (
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported      = platform.ErrUnsupported
	ErrNotAvailable     = platform.ErrNotAvailable
	ErrPermissionDenied = platform.ErrPermissionDenied
	ErrNotConfigured    = platform.ErrNotConfigured
)

// Location is one fix.
type Location = platform.Location

// Accuracy trades precision for battery.
type Accuracy = platform.LocationAccuracy

const (
	Balanced = platform.AccuracyBalanced // ~100 m (default)
	High     = platform.AccuracyHigh     // best available, GPS
	Low      = platform.AccuracyLow      // ~kilometres
)

// Options configures a request.
type Options struct {
	_ [0]func()

	Accuracy Accuracy
	// DistanceFilter (metres): Watch only reports moves at least this far;
	// 0 reports every update.
	DistanceFilter float64
}

func (o Options) platform() platform.LocationOptions {
	return platform.LocationOptions{Accuracy: o.Accuracy, DistanceFilter: o.DistanceFilter}
}

// Current returns one fix.
func Current(a *tango.App, o Options) (Location, error) {
	l, err := capcall.Backend[platform.Locator](a)
	if err != nil {
		return Location{}, err
	}
	opts := o.platform()
	return capcall.Async(a, func(done func(Location, error)) { l.CurrentLocation(opts, done) })
}

// watchers multiplexes the backend's single update session.
type watchers struct {
	a    *tango.App
	mu   sync.Mutex
	next int
	m    map[int]watcher
}

type watcher struct {
	opts Options
	fn   func(Location, error)
}

var states sync.Map // *tango.App -> *watchers

func state(a *tango.App) *watchers {
	v, _ := states.LoadOrStore(a, &watchers{a: a, m: map[int]watcher{}})
	return v.(*watchers)
}

func rank(acc Accuracy) int {
	switch acc {
	case High:
		return 3
	case Low:
		return 1
	}
	return 2
}

// merged is the session all watchers need: the best accuracy and the
// smallest distance filter. ok is false without watchers.
func (w *watchers) merged() (o Options, ok bool) {
	first := true
	for _, x := range w.m {
		if first {
			o, first = x.opts, false
			continue
		}
		if rank(x.opts.Accuracy) > rank(o.Accuracy) {
			o.Accuracy = x.opts.Accuracy
		}
		o.DistanceFilter = min(o.DistanceFilter, x.opts.DistanceFilter)
	}
	return o, !first
}

// apply (re)starts or stops the backend session. Main thread.
func (w *watchers) apply(l platform.Locator) {
	w.mu.Lock()
	o, ok := w.merged()
	w.mu.Unlock()
	if !ok {
		l.StopLocationUpdates()
		return
	}
	l.StartLocationUpdates(o.platform(), func(loc Location, err error) {
		w.mu.Lock()
		fns := make([]func(Location, error), 0, len(w.m))
		for _, x := range w.m {
			fns = append(fns, x.fn)
		}
		if err != nil {
			clear(w.m) // the session ended
		}
		w.mu.Unlock()
		// The backend calls this directly: guard each watcher on its own
		// so a panic in one does not skip the rest.
		for _, fn := range fns {
			corehook.Guard(w.a, "event handler", func() { fn(loc, err) })
		}
	})
}

// Watch calls fn on the main thread with every new fix until stop is
// called. An error passed to fn (permission revoked, services turned off)
// ends the watch. Several watches share one session with the best
// accuracy any of them asked for.
func Watch(a *tango.App, o Options, fn func(Location, error)) (stop func(), err error) {
	l, err := capcall.Backend[platform.Locator](a)
	if err != nil {
		return func() {}, err
	}
	w := state(a)
	w.mu.Lock()
	w.next++
	id := w.next
	w.m[id] = watcher{o, fn}
	w.mu.Unlock()
	if _, err := capcall.Sync(a, func() (struct{}, error) { w.apply(l); return struct{}{}, nil }); err != nil {
		w.mu.Lock()
		delete(w.m, id)
		w.mu.Unlock()
		return func() {}, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			w.mu.Lock()
			_, live := w.m[id]
			delete(w.m, id)
			w.mu.Unlock()
			if live {
				a.Post(func() { w.apply(l) })
			}
		})
	}, nil
}
