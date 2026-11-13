//go:build tango_dev

package uihost

// devActive reports whether the app runs as a tan dev worker connected to
// the hot reload host, where Attach is not supported.
func devActive() bool { return devClient() != nil }
