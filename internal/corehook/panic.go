package corehook

// Guard runs fn, recovering a panic in it and handing it to app's panic
// handling (a *tango.App: Options.OnPanic or the default, see
// docs/panics.md) as a tango.Panic with the given Where. It reports
// whether fn returned normally. With a nil app fn runs unguarded (a
// panic propagates to the caller). Packages that call application code from
// callbacks the core does not route (uihost frames, ...) wrap them with it.
// Set by package tango.
var Guard func(app any, where string, fn func()) bool

// ReportPanic reports a panic another package already recovered (ipc
// bindings, sys/background tasks) to the app owning conn, when conn is a
// *tango.Surface, or to conn itself when it is a *tango.App. It only
// reports: a fatal decision (keepRunning false) does not end the process.
// It returns false when conn belongs to no app. Set by package tango.
var ReportPanic func(conn any, where string, value any, stack []byte) bool
