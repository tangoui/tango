package theme

import (
	"fmt"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// Appearance is the look SetAppearance forces on the app.
type Appearance string

const (
	AppearanceSystem Appearance = ""      // follow the system (the default)
	AppearanceLight  Appearance = "light" // always light
	AppearanceDark   Appearance = "dark"  // always dark
)

// SetAppearance forces the whole app light or dark — window title bars and
// traffic lights, menus, system controls and the pages'
// prefers-color-scheme — or, with AppearanceSystem, lets it follow the
// system again. While forced, System and OnChange (and so SyncPage and
// uivars) report the forced look, as if the system had switched.
//
// Supported on macOS (NSApp.appearance); elsewhere it returns
// ErrUnsupported and the window keeps the system look. Safe from any
// goroutine; returns tango.ErrNotRunning after the app quit.
func SetAppearance(a *tango.App, ap Appearance) error {
	switch ap {
	case AppearanceSystem, AppearanceLight, AppearanceDark:
	default:
		return fmt.Errorf("theme: unknown appearance %q", string(ap))
	}
	aa, ok := a.Backend().(platform.AppAppearance)
	if !ok {
		return ErrUnsupported
	}
	if !tango.Await(a, func() bool { aa.SetAppAppearance(string(ap)); return true }) {
		return tango.ErrNotRunning
	}
	return nil
}
