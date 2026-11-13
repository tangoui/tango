//go:build !tango_dev

package uihost

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
)

// automationWatch does nothing in release builds: there is no automation
// control channel (automation_dev.go, docs/automation.md).
func automationWatch(*tango.App, *tango.Surface, *ui.Runner, *host) {}
