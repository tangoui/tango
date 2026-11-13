//go:build ios

package native

import "github.com/tangoui/tango/internal/platform/ios"

func init() {
	newBackend = ios.New
	mobileMainGoroutine = ios.MainGoroutine
}
