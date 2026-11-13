package darwin

// Platform-independent helpers of the window capabilities (surface_ext.go),
// kept free of build tags so they are tested everywhere.

import (
	"math"

	"github.com/tangoui/tango/internal/platform"
)

// topLeft converts an AppKit frame origin (bottom-left, y up, relative to
// the primary display's bottom) to the contract's top-left coordinates.
func topLeft(originX, originY, frameH, primaryH float64) (x, y int) {
	return int(math.Round(originX)), int(math.Round(primaryH - (originY + frameH)))
}

// bottomLeftY is the AppKit frame origin y for a window whose top edge is
// at top (contract coordinates).
func bottomLeftY(top int, frameH, primaryH float64) float64 {
	return primaryH - float64(top) - frameH
}

// windowState derives the contract state; fullscreen wins over the rest.
func windowState(fullscreen, miniaturized, zoomed bool) platform.WindowState {
	switch {
	case fullscreen:
		return platform.StateFullscreen
	case miniaturized:
		return platform.StateMinimized
	case zoomed:
		return platform.StateMaximized
	}
	return platform.StateNormal
}

// rgbaParts splits 0xRRGGBBAA into components in [0, 1].
func rgbaParts(c uint32) (r, g, b, a float64) {
	return float64(c>>24&0xff) / 255, float64(c>>16&0xff) / 255, float64(c>>8&0xff) / 255, float64(c&0xff) / 255
}
