//go:build ios && (tango_dev || tango_selftest)

package ios

// Compiles bridge.m's streaming probe in (see testhooks_on.go). A #cgo
// directive applies to every C and Objective-C file of the package.

// #cgo CFLAGS: -DTANGO_TEST_HOOKS=1
import "C"
