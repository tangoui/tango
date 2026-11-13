//go:build darwin && !ios

package darwin

// platform.ContrastPreference: System Settings > Accessibility > Display >
// Increase contrast, read with NSWorkspace
// accessibilityDisplayShouldIncreaseContrast (macOS 10.10+). Changes arrive
// with NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification, which
// motion.go already observes for reduce motion (watchDisplayOptions), and
// are reported as AppContrastChanged.

import (
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.ContrastPreference = (*backend)(nil)

var contrastState struct {
	on atomic.Bool
}

// readIncreaseContrast asks NSWorkspace. Main thread.
func readIncreaseContrast() bool {
	var on bool
	withPool(func() {
		ws := send(class("NSWorkspace"), "sharedWorkspace")
		if respondsTo(ws, "accessibilityDisplayShouldIncreaseContrast") {
			on = sendBool(ws, "accessibilityDisplayShouldIncreaseContrast")
		}
	})
	return on
}

func (b *backend) contrastMaybeChanged() {
	on := readIncreaseContrast()
	if contrastState.on.Swap(on) == on || b == nil || b.h == nil {
		return
	}
	b.appEvent(platform.AppEvent{Kind: platform.AppContrastChanged})
}

// HighContrast implements platform.ContrastPreference. Any goroutine: off
// the main thread it returns the value last seen there (and starts
// watching if nobody asked on the main thread yet).
func (b *backend) HighContrast() bool {
	if isMainThread() {
		b.watchDisplayOptions()
		return contrastState.on.Load()
	}
	runOnMain(b.watchDisplayOptions)
	return contrastState.on.Load()
}
