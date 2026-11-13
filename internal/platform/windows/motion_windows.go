//go:build windows && (amd64 || arm64)

package windows

// The system "reduce motion" preference (platform.MotionPreference):
// Settings > Accessibility > Visual effects > Animation effects, read with
// SPI_GETCLIENTAREAANIMATION. Every WM_SETTINGCHANGE reaching the
// backend's top-level windows (systemMessage in integration_windows.go)
// reads it again and reports AppReduceMotionChanged when it changed.

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.MotionPreference = (*Backend)(nil)

const spiGetClientAreaAnimation = 0x1042

// motionState is main-thread state.
var motionState struct {
	last bool // reduce motion as last reported
}

// ReduceMotion implements platform.MotionPreference. Any goroutine.
func (b *Backend) ReduceMotion() bool { return readReduceMotion() }

func readReduceMotion() bool {
	var on int32 = 1 // BOOL: client-area animations enabled
	if r, _, _ := canvasSystemParametersInfo.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&on)), 0); r == 0 {
		return false
	}
	return on == 0
}

// initMotion records the starting value (initSystemState).
func initMotion() { motionState.last = readReduceMotion() }

// motionMaybeChanged runs for every WM_SETTINGCHANGE.
func (b *Backend) motionMaybeChanged() {
	v := readReduceMotion()
	if v == motionState.last {
		return
	}
	motionState.last = v
	if hx, ok := b.h.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppReduceMotionChanged})
	}
}
