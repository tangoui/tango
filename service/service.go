// Package service ties long-lived Go values ("services") to the lifecycle of
// a tango.App: they start in registration order once the app is ready and
// shut down in reverse order when it terminates.
//
// A service is any value. If it implements Starter, Startup runs on the
// main thread when the app is ready (or soon after Register, if it already
// is); if it implements Stopper, Shutdown runs on the main thread when the
// app terminates. Both block the main thread, so start long work on a
// goroutine and stop it with the context Startup receives, which is
// canceled before any Shutdown runs.
//
// If a Startup fails, the services already started are shut down, the
// remaining ones are not started, and the app quits; Err reports why.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
)

// Service is any value managed by this package.
type Service interface{}

// Starter is implemented by services that need to start with the app.
type Starter interface {
	Startup(ctx context.Context) error
}

// Stopper is implemented by services that need to release resources when
// the app terminates.
type Stopper interface {
	Shutdown() error
}

// group holds the services registered on one app.
type group struct {
	app    *tango.App
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	started  []Service // in start order
	stopped  bool
	startErr error
	stopErr  error
}

var groups sync.Map // *tango.App → *group

func groupFor(app *tango.App) *group {
	if g, ok := groups.Load(app); ok {
		return g.(*group)
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &group{app: app, ctx: ctx, cancel: cancel}
	if v, loaded := groups.LoadOrStore(app, g); loaded {
		cancel()
		return v.(*group)
	}
	app.OnLifecycle(func(ev tango.Lifecycle) {
		if ev == tango.Terminating {
			g.stop()
		}
	})
	return g
}

// Register adds svcs to app. They start, in order, once the app is ready;
// services from earlier Register calls start first. Nil entries are
// ignored.
func Register(app *tango.App, svcs ...Service) {
	if app == nil {
		panic("service: Register: nil app")
	}
	list := make([]Service, 0, len(svcs))
	for _, s := range svcs {
		if s != nil {
			list = append(list, s)
		}
	}
	if len(list) == 0 {
		return
	}
	g := groupFor(app)
	app.OnReady(func() { g.start(list) })
}

// Bind registers svc's exported methods on router as "name.Method" (see
// package bind) and adds svc to app as Register does. Startup and Shutdown
// are lifecycle hooks, not API: calls to them from the page answer
// not_found.
func Bind(app *tango.App, router *ipc.Router, name string, svc Service) error {
	if app == nil {
		return errors.New("service: Bind: nil app")
	}
	if err := bind.Register(router, name, svc); err != nil {
		return err
	}
	t := reflect.TypeOf(svc)
	for _, m := range []string{"Startup", "Shutdown"} {
		if _, ok := t.MethodByName(m); ok {
			method := name + "." + m
			router.Handle(method, func(context.Context, json.RawMessage) (any, error) {
				return nil, ipc.NewError(ipc.CodeNotFound, "no method "+method)
			})
		}
	}
	Register(app, svc)
	return nil
}

// Shutdown shuts down app's services now if the app has not already
// done so, and returns Err. Use it when Run can return without the app
// emitting Terminating, e.g. defer service.Shutdown(app) after Run.
func Shutdown(app *tango.App) error {
	v, ok := groups.Load(app)
	if !ok {
		return nil
	}
	g := v.(*group)
	g.stop()
	return g.err()
}

// Err reports the Startup error that made app quit, if any, joined with
// the errors Shutdown methods returned.
func Err(app *tango.App) error {
	v, ok := groups.Load(app)
	if !ok {
		return nil
	}
	return v.(*group).err()
}

func (g *group) err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return errors.Join(g.startErr, g.stopErr)
}

func (g *group) start(list []Service) {
	for _, s := range list {
		g.mu.Lock()
		halt := g.stopped || g.startErr != nil
		g.mu.Unlock()
		if halt {
			return
		}
		if st, ok := s.(Starter); ok {
			if err := st.Startup(g.ctx); err != nil {
				g.mu.Lock()
				g.startErr = fmt.Errorf("service: starting %T: %w", s, err)
				g.mu.Unlock()
				g.stop()
				g.app.Quit()
				return
			}
		}
		g.mu.Lock()
		g.started = append(g.started, s)
		g.mu.Unlock()
	}
}

func (g *group) stop() {
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	g.stopped = true
	started := g.started
	g.started = nil
	g.mu.Unlock()

	g.cancel()
	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		if st, ok := started[i].(Stopper); ok {
			if err := st.Shutdown(); err != nil {
				errs = append(errs, fmt.Errorf("service: stopping %T: %w", started[i], err))
			}
		}
	}
	g.mu.Lock()
	g.stopErr = errors.Join(errs...)
	g.mu.Unlock()
}
