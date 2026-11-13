//go:build windows

package uihost

// Windows: the canvas hands over its child HWND (platform.CanvasHWND) and
// the Direct3D 11 presenter puts a DXGI flip-model swap chain on it. CPU
// frames (presenter failure, oversized frames, device loss) are shown by
// the canvas in a sibling window above the swap chain.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/gpu/d3d11"
)

var (
	platformGPUTarget    = hwndTarget
	platformNewPresenter = d3d11.NewPresenter
	platformGPURelease   func(platform.Canvas) // the layer / GPU window stays: Present(img) uses it
)
