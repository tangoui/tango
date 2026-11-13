// Package capcall holds the plumbing every sys/... capability package
// shares: finding the backend interface and running calls on the main
// thread with tango.ErrNotRunning after the app quit.
package capcall

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// Backend returns a's backend as I, or platform.ErrUnsupported.
func Backend[I any](a *tango.App) (I, error) {
	i, ok := a.Backend().(I)
	if !ok {
		var zero I
		return zero, platform.ErrUnsupported
	}
	return i, nil
}

type result[T any] struct {
	v   T
	err error
	ran bool
}

// Sync runs fn on the main thread and returns its result.
func Sync[T any](a *tango.App, fn func() (T, error)) (T, error) {
	r := tango.Await(a, func() result[T] {
		v, err := fn()
		return result[T]{v, err, true}
	})
	if !r.ran {
		var zero T
		return zero, tango.ErrNotRunning
	}
	return r.v, r.err
}

// Async starts asynchronous native work on the main thread and blocks
// until it calls done. On the main thread it keeps the event loop running
// meanwhile — except on Android, where the main thread cannot pump native
// events: there it fails at once with tango.ErrNestedWait (call from a
// goroutine instead).
func Async[T any](a *tango.App, start func(done func(T, error))) (T, error) {
	r, err := tango.TryAwaitAsync(a, func(done func(result[T])) {
		start(func(v T, err error) { done(result[T]{v: v, err: err}) })
	})
	if err != nil {
		return r.v, err
	}
	return r.v, r.err
}
