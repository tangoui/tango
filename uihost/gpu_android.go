//go:build android

package uihost

// Android: a canvas page hands over its native window holder
// (platform.CanvasNativeWindow) and the OpenGL ES presenter draws into
// the page's SurfaceView with EGL, recreating its window surface whenever
// the Surface comes back and its context after a loss
// (gl.NewPresenterAndroid). When the presenter cannot start (or runs on a
// software rasterizer, gpu.go) the canvas gives the holder back
// (nativeWindowRelease) and the window stays on the plain CPU path, with
// partial presents. Attached layers (TextureView overlays) stay on the
// CPU, as everywhere.

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/render/gpu/gl"
)

var (
	platformGPUTarget    = nativeWindowTarget
	platformNewPresenter = gl.NewPresenterAndroid
	platformGPURelease   = nativeWindowRelease
)

// nativeWindowReleaser is implemented by the Android canvas: it undoes
// UseNativeWindow (the holder goes; nothing was bound to the window yet).
type nativeWindowReleaser interface{ ReleaseNativeWindow() }

func nativeWindowRelease(cv platform.Canvas) {
	if r, ok := cv.(nativeWindowReleaser); ok {
		r.ReleaseNativeWindow()
	}
}

// nativeWindowTarget switches cv to GPU presentation on its native window
// (Android).
func nativeWindowTarget(cv platform.Canvas) (uintptr, error) {
	cn, ok := cv.(platform.CanvasNativeWindow)
	if !ok {
		return 0, errors.New("canvas has no native window")
	}
	return cn.UseNativeWindow()
}
