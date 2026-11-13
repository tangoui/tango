//go:build darwin

package uihost

// macOS: the canvas switches to a CAMetalLayer (platform.CanvasMetal) and
// the Metal presenter draws into it.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/gpu/metal"
)

var (
	platformGPUTarget    = metalTarget
	platformNewPresenter = metal.NewPresenter
	platformGPURelease   func(platform.Canvas) // the layer / GPU window stays: Present(img) uses it
)
