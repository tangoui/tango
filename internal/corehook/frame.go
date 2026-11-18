package corehook

// StartWindowMove hands the primary-button press in progress on surface (a
// *tango.Surface) to the window manager as a window move, where the
// platform's surface supports it (platform.Dragger). Set by package tango;
// package uihost calls it for the drag areas of a self-drawn title bar.
var StartWindowMove func(surface any)

// WatchAppearance registers fn for SetAppearance on surface (a
// *tango.Surface). Registration and notifications run on the main thread.
var WatchAppearance func(surface any, fn func())

// DefaultUILocale returns the current global default as a *ui.Locale,
// independently of any Context's provider stack. Set by package ui.
var DefaultUILocale func() any
