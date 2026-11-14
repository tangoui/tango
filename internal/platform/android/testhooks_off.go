//go:build android && !tango_dev && !tango_selftest

package android

// testHooks: see testhooks_on.go. Release builds leave the hooks out.
const testHooks = false
