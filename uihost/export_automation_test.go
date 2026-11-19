//go:build tango_dev

package uihost

import (
	"time"

	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/ui"
)

var ReadTokenFileForTest = automation.ReadTokenFile

// AutoCanvasForTest returns an automation canvas for the threaded window
// of r that waits at most wait for its UI thread (not registered with the
// control channel).
func AutoCanvasForTest(r *ui.Runner, wait time.Duration) automation.Canvas {
	c := newAutoCanvas(r, threadedOf(r).host)
	c.wait = wait
	return c
}
