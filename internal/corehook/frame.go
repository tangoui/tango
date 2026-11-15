package corehook

// StartWindowMove hands the primary-button press in progress on surface (a
// *tango.Surface) to the window manager as a window move, where the
// platform's surface supports it (platform.Dragger). Set by package tango;
// package uihost calls it for the drag areas of a self-drawn title bar.
var StartWindowMove func(surface any)
