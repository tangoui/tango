//go:build ios && !tango_dev && !tango_selftest

package ios

// testHooks: see testhooks_on.go. Release builds leave the hooks out.
const testHooks = false
