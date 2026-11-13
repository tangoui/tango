//go:build !darwin && !windows && !linux

package uihost

// Other platforms have no GPU presenter: every window stays on the CPU
// path. (The linux tag covers Android too: gpu_linux.go is Linux without
// Android, gpu_android.go Android.)

import (
	"errors"
	"image"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/paint"
)

var errNoGPUPresenter = errors.New("no GPU presenter on this platform")

func platformGPUTarget(platform.Canvas) (uintptr, error) { return 0, errNoGPUPresenter }

var platformGPURelease func(platform.Canvas) // nothing to undo

func platformNewPresenter(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
	return nil, nil, errNoGPUPresenter
}
