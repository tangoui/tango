//go:build tango_guiprobe

// Package guiprobe is a passive, main-thread-only input observer for the
// explicitly tagged GUI acceptance fixture. Default builds inline a no-op.
package guiprobe

import "github.com/tangoui/tango/ui/input"

const Enabled = true

var observer func(input.Event)

// SetObserver installs a callback on the UI thread. It cannot alter delivery.
func SetObserver(fn func(input.Event)) { observer = fn }
func Observe(ev input.Event) {
	if observer != nil {
		observer(ev)
	}
}
