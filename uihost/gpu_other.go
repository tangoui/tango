//go:build !darwin && !windows

package uihost

// Other platforms have no GPU presenter yet (Linux GL is GPU milestone 4):
// every window stays on the CPU path.

import (
	"errors"
	"image"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/paint"
)

var errNoGPUPresenter = errors.New("no GPU presenter on this platform")

func platformGPUTarget(platform.Canvas) (uintptr, error) { return 0, errNoGPUPresenter }

func platformNewPresenter(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
	return nil, nil, errNoGPUPresenter
}
