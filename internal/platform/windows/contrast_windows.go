//go:build windows && (amd64 || arm64)

package windows

// The system high-contrast preference (platform.ContrastPreference):
// Settings > Accessibility > Contrast themes, read with SPI_GETHIGHCONTRAST
// (HIGHCONTRASTW.dwFlags & HCF_HIGHCONTRASTON). Switching a contrast theme
// broadcasts WM_SETTINGCHANGE (SPI_SETHIGHCONTRAST); every
// WM_SETTINGCHANGE reaching the backend's top-level windows
// (systemMessage in integration_windows.go) reads it again and reports
// AppContrastChanged when it changed, as motion_windows.go does for
// reduce motion.

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.ContrastPreference = (*Backend)(nil)

const (
	spiGetHighContrast = 0x0042
	hcfHighContrastOn  = 0x00000001
)

// highContrastW is HIGHCONTRASTW.
type highContrastW struct {
	cbSize            uint32
	dwFlags           uint32
	lpszDefaultScheme uintptr
}

// contrastState is main-thread state.
var contrastState struct {
	last bool // high contrast as last reported
}

// HighContrast implements platform.ContrastPreference. Any goroutine.
func (b *Backend) HighContrast() bool { return readHighContrast() }

func readHighContrast() bool {
	hc := highContrastW{cbSize: uint32(unsafe.Sizeof(highContrastW{}))}
	if r, _, _ := canvasSystemParametersInfo.Call(spiGetHighContrast, uintptr(hc.cbSize), uintptr(unsafe.Pointer(&hc)), 0); r == 0 {
		return false
	}
	return hc.dwFlags&hcfHighContrastOn != 0
}

// initContrast records the starting value (initSystemState).
func initContrast() { contrastState.last = readHighContrast() }

// contrastMaybeChanged runs for every WM_SETTINGCHANGE.
func (b *Backend) contrastMaybeChanged() {
	v := readHighContrast()
	if v == contrastState.last {
		return
	}
	contrastState.last = v
	if hx, ok := b.h.(platform.HostEx); ok {
		hx.OnAppEvent(platform.AppEvent{Kind: platform.AppContrastChanged})
	}
}
