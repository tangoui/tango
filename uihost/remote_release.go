//go:build !tango_dev

package uihost

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
)

// Release builds have no dev host: Open always opens local windows.
func devOpen(*tango.App, Options, func(*ui.Context), ui.RunnerOptions) (*ui.Runner, *tango.Surface, bool, error) {
	return nil, nil, false, nil
}

// Persist keeps a value across hot reloads under tan dev; in production
// builds (without -tags tango_dev) it does nothing.
func Persist(ptr any) {}
