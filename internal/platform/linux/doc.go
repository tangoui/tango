// Package linux is Tango's Linux desktop backend: GTK 3 windows hosting
// WebKitGTK 4.1 web views, loaded at run time with purego (no cgo).
//
// Files without a build tag hold pure logic (accelerator and label
// conversion, menu roles, HTTP adaptation, the callback dispatch table,
// lifecycle aggregation) and are unit tested on any OS. Files ending in
// _linux.go are tagged `linux && !android` and talk to the native
// libraries.
//
// Why GTK 3 rather than GTK 4: GtkMenuBar, window placement and broad
// distribution coverage (WebKitGTK 4.1 targets GTK 3); see MyGo's
// docs/architecture.md ("Why not GTK 4"), whose Linux backend this one
// borrows from (MIT, Copyright (c) 2026 MyGo contributors).
//
// Callbacks: purego callbacks are limited in number and never freed, so the
// package creates a fixed handful of trampolines once (callbacks_linux.go)
// and routes every signal, idle source and scheme request through a
// dispatch table keyed by the id passed as C user_data (dispatch.go).
// Entries are removed by the GDestroyNotify / GClosureNotify that GLib calls
// when the connection or source goes away.
package linux
