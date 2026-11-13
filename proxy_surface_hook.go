//go:build tango_dev

package tango

// Proxy surfaces for tan dev's hot-reload workers: see
// corehook.NewProxySurface. Only built with -tags tango_dev. Unlike the
// other development features (internal/devfeat) it stays in this package:
// it builds a Surface from core internals (addSurface, the surface state,
// host event routing) that no hook exposes.

import (
	"errors"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

var (
	errProxyNotMain = errors.New("tango: proxy surfaces must be created on the main thread")
	errProxyClosed  = errors.New("tango: proxy surface closed while being created")
)

func init() {
	corehook.NewProxySurface = func(app any, opts any, create func(platform.SurfaceID, corehook.ProxyEvents) (platform.Surface, error)) (any, error) {
		a := app.(*App)
		if a.b == nil {
			return nil, ErrNoBackend
		}
		if !a.isMain() {
			return nil, errProxyNotMain
		}
		o := opts.(SurfaceOptions)
		s, err := a.addSurface(o, "", true, func(s *Surface) (platform.Surface, error) {
			s.ext.appearance = o.Appearance
			s.ext.resizable = o.Resizable
			return create(s.id, proxyEvents{a, s.id})
		})
		if err != nil {
			return nil, err
		}
		if s.closed.Load() {
			return nil, errProxyClosed
		}
		return s, nil
	}
}

// proxyEvents routes a proxy surface's events like the backend's would.
type proxyEvents struct {
	a  *App
	id platform.SurfaceID
}

func (p proxyEvents) Event(ev platform.SurfaceEvent) { host{p.a}.OnSurfaceEvent(p.id, ev) }
func (p proxyEvents) ShouldClose() bool              { return host{p.a}.ShouldClose(p.id) }
