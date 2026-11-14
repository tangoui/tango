//go:build !android || tango_dev || tango_selftest

package android

// testHooks says whether the emulator self-test hooks are compiled in: the
// lane counters (LaneStatsSnapshot, lanes.go), TANGO_ANDROID_NO_ARRAYBUFFER
// (lanes_android.go) and the TANGO_TEST_ENV launch extra
// (testenv_android.go). An Android app has them only when built with
// -tags tango_dev (tan dev) or -tags tango_selftest, as on iOS
// (internal/platform/ios/testhooks_on.go). A plain release build reads
// none of these inputs.
//
// Off Android (go test of the pure-Go files on the host) the hooks are on,
// so their tests run.
const testHooks = true
