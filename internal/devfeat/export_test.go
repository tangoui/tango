//go:build tango_dev

package devfeat

// Internals for the external tests of this package.

// DevWin is one window's saved place in the state file.
type DevWin = devWin

// DevStateDelay points at the delay before the state file is written.
var DevStateDelay = &devStateDelay

// AutomationChannel is the internal channel of Eval results.
const AutomationChannel = automationChannel

// EvalScript builds the page script of one Eval.
var EvalScript = evalScript
