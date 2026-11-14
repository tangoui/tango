//go:build darwin && !ios && !tango_dev

package darwin

// Production builds have no streaming probe (scheme_probe_dev.go):
// TANGO_SCHEME_PROBE is ignored and /__tango/probe/stream goes to
// Host.ServeAsset like any other path.

func schemeProbe() bool { return false }

func serveProbe(id) bool { return false }
