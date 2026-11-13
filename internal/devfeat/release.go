//go:build !tango_dev

package devfeat

// AttachWindowState does nothing in release builds: restoring window
// placement across restarts is a `tan dev` feature (windowstate.go).
func AttachWindowState(App) {}

// AttachAutomation does nothing in release builds: the automation control
// channel (automation.go, docs/automation.md) is a development feature and
// TANGO_AUTOMATION is ignored.
func AttachAutomation(App) {}
