//go:build !tango_dev

package gpu

import "testing"

// verifyDefault is the default of verify mode (Renderer.SetVerify) in
// builds without the tango_dev tag: on inside test binaries (so go test
// keeps checking partial redraws against full frames, spec 6.8), off in
// every program binary. It reads no environment variable.
var verifyDefault = testing.Testing()
