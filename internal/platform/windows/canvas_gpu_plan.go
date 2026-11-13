package windows

// Platform-independent part of GPU presentation on a canvas
// (platform.CanvasHWND, canvas_gpu_windows.go): what WM_PAINT does and
// what Present does to the CPU-fallback sibling window. Kept free of
// build tags so it is tested on every OS.

// canvasGPU is the GPU state of a canvas. The zero value is the CPU path.
type canvasGPU struct {
	on       bool // UseGPUWindow succeeded: the HWND belongs to a swap chain
	cpuShown bool // the sibling window is visible with a CPU frame
}

// paint is what WM_PAINT does: frame (call CanvasFrame) and blit (draw the
// last CPU frame on the canvas HWND with GDI). ready: sink callbacks are
// allowed; changed: the geometry changed; wanted: a frame was requested;
// haveFrame: a CPU frame exists. On the GPU path WM_PAINT never draws on
// the HWND (DXGI flip-model swap chains do not mix with GDI) and only
// runs a frame that is due: a plain expose is recomposed by DWM.
func (g canvasGPU) paint(ready, changed, wanted, haveFrame bool) (frame, blit bool) {
	if g.on {
		return ready && (changed || wanted), false
	}
	return ready && (changed || wanted || !haveFrame), true
}

// canvasSiblingOp is what Present does with the sibling window.
type canvasSiblingOp uint8

const (
	siblingNone   canvasSiblingOp = iota // nothing (CPU path, or already hidden)
	siblingShow                          // show it above the HWND with the new frame
	siblingRedraw                        // already shown: repaint it with the new frame
	siblingHide                          // the GPU presented: hide it
)

// present updates g for Present(img) (cpu true) or Present(nil) (cpu
// false) and returns what to do with the sibling. On the CPU path it is
// always siblingNone (the canvas blits CPU frames itself).
func (g *canvasGPU) present(cpu bool) canvasSiblingOp {
	switch {
	case !g.on:
		return siblingNone
	case cpu && g.cpuShown:
		return siblingRedraw
	case cpu:
		g.cpuShown = true
		return siblingShow
	case g.cpuShown:
		g.cpuShown = false
		return siblingHide
	}
	return siblingNone
}

// canvasGPUSupport reports whether a canvas can switch to GPU
// presentation: not attached to a WebView surface (attached canvases may
// be layered or pass-through) and not see-through (an opaque swap chain
// cannot show the window background).
func canvasGPUSupport(attached, transparent bool) bool { return !attached && !transparent }
