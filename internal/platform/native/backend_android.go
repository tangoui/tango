//go:build android

package native

import "github.com/tangoui/tango/internal/platform/android"

func init() {
	newBackend = android.New
	mobileMainGoroutine = android.MainGoroutine
}
