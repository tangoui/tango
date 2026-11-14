//go:build !ios || tango_dev || tango_selftest

package ios

// testHooks says whether the simulator self-test hooks are compiled in: the
// lane counters and streaming probe (TANGO_SCHEME_PROBE, lanes.go and
// bridge.m), TANGO_IPC_LANES_DARWIN=all, and the canvas self-tests
// (TANGO_CANVAS_IME_SELFTEST, TANGO_CANVAS_A11Y_SELFTEST). An iOS app has
// them only when built with -tags tango_dev (tan dev) or -tags
// tango_selftest. tan build keeps every GOFLAGS tag except tango_dev, so
// GOFLAGS=-tags=tango_selftest bash mobile/ios/build.sh ... builds a
// simulator app with the hooks (scripts/smoke.sh ios-probe, ios-selftest).
// A plain release build reads none of these variables.
//
// Off iOS (go test of the pure-Go files on the host) the hooks are on, so
// their tests run.
const testHooks = true
