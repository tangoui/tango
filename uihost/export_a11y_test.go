package uihost

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

// A11yTap subscribes to r's semantics tree (a11yBridge.tap). Main thread.
func A11yTap(r *ui.Runner, fn func(*a11y.Tree, []a11y.NodeID)) (cancel func()) {
	return a11yFor(r).tap(fn)
}

// A11yForce keeps r's tree built (a11yBridge.force). Main thread.
func A11yForce(r *ui.Runner) (release func()) { return a11yFor(r).force() }

// HasA11y reports whether r has a bridge.
func HasA11y(r *ui.Runner) bool { return a11yFor(r) != nil }
