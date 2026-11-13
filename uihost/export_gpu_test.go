package uihost

import (
	"image"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/paint"
)

// SetGPUHooks replaces the presenter constructor (metal.NewPresenter) and
// the GPU log function, selects the macOS target (platform.CanvasMetal)
// whatever the OS, and resets the once-per-process "GPU unavailable" log.
// It returns a restore function.
func SetGPUHooks(np func(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error), logf func(string, ...any)) func() {
	return setGPUHooks(metalTarget, np, logf)
}

// SetGPUHooksHWND is SetGPUHooks with the Windows target
// (platform.CanvasHWND, d3d11.NewPresenter's shape) on every OS.
func SetGPUHooksHWND(np func(hwnd uintptr) (paint.Renderer, func(image.Point, float32) error, error), logf func(string, ...any)) func() {
	return setGPUHooks(hwndTarget, np, logf)
}

func setGPUHooks(target func(platform.Canvas) (uintptr, error), np presenterFunc, logf func(string, ...any)) func() {
	oldTarget, oldNP, oldLog := gpuTarget, newPresenter, gpuLogf
	gpuTarget = target
	if np != nil {
		newPresenter = np
	}
	if logf != nil {
		gpuLogf = logf
	}
	gpuLogged.mu.Lock()
	gpuLogged.done = false
	gpuLogged.mu.Unlock()
	return func() { gpuTarget, newPresenter, gpuLogf = oldTarget, oldNP, oldLog }
}
