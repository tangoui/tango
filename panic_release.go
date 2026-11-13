//go:build !tango_dev

package tango

// In production builds (without -tags tango_dev, the default) an unhandled
// panic is fatal by default (panic.go).
func init() { releaseBuild = true }
