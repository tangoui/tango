package corehook

// StartWindowMove hands the primary-button press in progress on surface (a
// *tango.Surface) to the window manager as a window move, where the
// platform's surface supports it (platform.Dragger). Set by package tango;
// package uihost calls it for the drag areas of a self-drawn title bar.
var StartWindowMove func(surface any)

// WatchAppearance registers fn for SetAppearance on surface (a
// *tango.Surface). Registration and notifications run on the main thread.
var WatchAppearance func(surface any, fn func())

// IsOverlaySurface reports whether surface (a *tango.Surface) is an overlay.
var IsOverlaySurface func(surface any) bool

// FollowUILocale installs next if the current default is last or next.
// Both arguments are *ui.Locale. A nil last permits the first update.
// Set by package ui.
var FollowUILocale func(last, next any) bool
