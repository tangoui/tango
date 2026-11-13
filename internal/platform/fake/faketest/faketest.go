// Package faketest runs a tango.App on the fake backend for tests of
// packages built on tango (capability packages, ipc, bind, ...).
package faketest

import (
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/apphook"
)

// Timeout bounds every wait in this package.
const Timeout = 5 * time.Second

// NewApp returns an App on a fresh fake backend, not running yet.
func NewApp(opts tango.Options) (*tango.App, *fake.Backend) {
	fb := fake.New()
	return newApp(opts, fb), fb
}

// NewMinimalApp is NewApp on a backend without optional capabilities.
func NewMinimalApp(opts tango.Options) (*tango.App, *fake.Backend) {
	fb := fake.New()
	return newApp(opts, fb.Minimal()), fb
}

func newApp(opts tango.Options, b platform.Backend) *tango.App {
	if apphook.NewApp == nil {
		panic("faketest: package tango did not register apphook.NewApp")
	}
	return apphook.NewApp(b, opts).(*tango.App)
}

// Start runs a on a new goroutine, which acts as the main thread, and waits
// until it is ready. Cleanup quits it and waits for Run to return.
func Start(t testing.TB, a *tango.App, fb *fake.Backend) {
	t.Helper()
	ready := make(chan struct{})
	a.OnReady(func() { close(ready) })
	errc := make(chan error, 1)
	go func() { errc <- a.Run() }()
	select {
	case <-ready:
	case err := <-errc:
		t.Fatalf("faketest: Run returned early: %v", err)
	case <-time.After(Timeout):
		t.Fatal("faketest: app did not start")
	}
	t.Cleanup(func() {
		a.Quit()
		select {
		case <-fb.Done():
		case <-time.After(Timeout):
			t.Error("faketest: app did not quit")
		}
	})
}

// Run is NewApp followed by Start.
func Run(t testing.TB, opts tango.Options) (*tango.App, *fake.Backend) {
	t.Helper()
	a, fb := NewApp(opts)
	Start(t, a, fb)
	return a, fb
}

// RunMinimal is NewMinimalApp followed by Start.
func RunMinimal(t testing.TB, opts tango.Options) (*tango.App, *fake.Backend) {
	t.Helper()
	a, fb := NewMinimalApp(opts)
	Start(t, a, fb)
	return a, fb
}
