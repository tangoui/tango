package menu

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// appMenuOf returns where SetAppMenu / SetSurfaceMenu send the menu: the
// backend, unless corehook.AppMenuProxy takes it (a tan dev hot-reload
// worker shows its menus in the dev host).
func appMenuOf(a *tango.App, parent platform.SurfaceID) (platform.AppMenu, bool) {
	if p := corehook.AppMenuProxy; p != nil {
		if am, ok := p(a, parent); ok {
			return am, true
		}
	}
	am, ok := a.Backend().(platform.AppMenu)
	return am, ok
}
