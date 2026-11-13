//go:build linux && !android

package uihost

// Linux: the canvas hands over a GtkGLArea above its drawing area
// (platform.CanvasGL) and the OpenGL presenter blits each frame into it
// from the area's render signal. CPU frames (oversized frames, device
// loss) are shown by the canvas with cairo, the GLArea hidden. When the
// presenter cannot start, or runs on a software rasterizer (gpu.go), the
// canvas gives the GLArea back (glAreaRelease) and the window stays on
// the plain cairo path.

import "github.com/tangoui/tango/ui/gpu/gl"

var (
	platformGPUTarget    = glAreaTarget
	platformNewPresenter = gl.NewPresenter
	platformGPURelease   = glAreaRelease
)
