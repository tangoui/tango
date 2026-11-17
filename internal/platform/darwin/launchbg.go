//go:build darwin && !ios

package darwin

import "os"

// launchBackground is TANGO_LAUNCH_BACKGROUND=1: the app starts without
// taking activation from the frontmost app, as `open -g` launches a
// bundle. A bare executable (go run, a test binary) is otherwise
// activated when it finishes launching. automation.Launch sets it so
// tests and agents driving windows in the background do not steal focus.
// It only skips an activation, so every build reads it.
var launchBackground = os.Getenv("TANGO_LAUNCH_BACKGROUND") == "1"
