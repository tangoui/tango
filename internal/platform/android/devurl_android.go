//go:build android

package android

import (
	"os"

	"github.com/tangoui/tango/asset"
)

// applyDevURL copies the front-end dev server URL that tan dev -platform
// android passes with the launch Intent (am start -e TANGO_DEV_URL <url>)
// into the environment, where asset.FromEnv looks for it as on desktop. An
// Android process cannot be given environment variables, hence the extra.
//
// The Java side (Mobile.devURL) answers only in debuggable APKs, so release
// builds ignore the extra. A variable already set wins. Main thread, from
// nativeInit, before main.main runs.
func applyDevURL() {
	if os.Getenv(asset.DevURLEnv) != "" {
		return
	}
	u, err := mobileCall(0, "devURL", nil)
	if err != nil || u == "" {
		return
	}
	os.Setenv(asset.DevURLEnv, u)
	logf(logInfo, "tan dev: front end at %s", u)
}
