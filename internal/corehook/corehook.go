// Package corehook lets Tango's own feature packages (menu, tray, ...) reach
// core internals without the root package exporting them.
//
// It is a leaf package (it imports only internal/platform). The root tango
// package and the menu package fill in the variables from their init
// functions; they are nil until those packages are linked in.
package corehook

import "github.com/tangoui/tango/internal/platform"

// InternalPrefix starts every internal page→Go message:
//
//	"\x00tango-int:" + channel + ":" + payload
//
// The core consumes such messages before App.OnMessage handlers (and so
// before ipc) see them. A channel name never contains ':'.
const InternalPrefix = "\x00tango-int:"

// HandleInternal registers fn for internal messages on channel of app (a
// *tango.App). fn runs on the main thread with the sending *tango.Surface
// (as any) and the payload. Registering a channel again replaces fn. Set by
// package tango.
var HandleInternal func(app any, channel string, fn func(surface any, payload string))

// OnSurfaceCreated registers fn to run on the main thread each time app (a
// *tango.App) has created a surface (WebView or canvas); fn receives the
// *tango.Surface as any. It is App.OnSurfaceCreated for code that only
// holds the app as any (tan dev's host, which wires windowstate into apps
// it hot-reloads). Set by package tango.
var OnSurfaceCreated func(app any, fn func(surface any))

// AttachMenu shows the *menu.Menu m (as any) at a persistent place
// identified by key: apply receives the converted items and the click
// router and is called on the main thread now and whenever the menu is
// updated. A nil m detaches key and calls apply(nil, nil) once. Set by
// package menu.
var AttachMenu func(app any, key any, m any, apply func(items []platform.MenuItem, onClick func(id uint32))) error

// DetachMenu forgets key without calling its apply function. Set by
// package menu.
var DetachMenu func(key any)
