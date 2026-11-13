package linux

// Platform-independent part of GPU presentation on a canvas
// (platform.CanvasGL, canvas_gl_linux.go): where frames run, what the
// drawing area paints and what Present does to the GtkGLArea. Kept free of
// build tags so it is tested on every OS.

// canvasGL is the GL state of a canvas. The zero value is the cairo path.
type canvasGL struct {
	on       bool // UseGLArea succeeded: the GtkGLArea sits above the drawing area
	cpuShown bool // the GLArea is hidden and the drawing area shows a CPU frame
	released bool // ReleaseGLArea returned the canvas to cairo: UseGLArea is refused
}

// release returns g to the cairo path for good (ReleaseGLArea) and
// reports whether it was on the GL path (there is a GLArea to destroy).
func (g *canvasGL) release() bool {
	was := g.on
	if was {
		*g = canvasGL{released: true}
	}
	return was
}

// tick is what a frame clock tick with a pending request does: draw
// (queue a redraw of the drawing area, whose draw handler renders the
// frame) or frame (call CanvasFrame now, in the update phase). changed:
// the geometry changed; wanted: the next draw must render (drawWanted);
// ready: start ran.
//
// On the cairo path a size change and the first frame render inside the
// draw handler, so the frame matches the allocation it paints. On the GL
// path frames never run from the drawing area's draw handler: the
// presenter queues a render of the GLArea, which belongs before GTK's
// paint phase, not inside it (canvas_gl_linux.go).
// CanvasFrame runs at once (frame() remembers a frame before start).
func (g canvasGL) tick(changed, wanted, ready bool) (draw, frame bool) {
	if g.on {
		return false, true
	}
	if changed || wanted || !ready {
		return true, false
	}
	return false, true
}

// resizeFrame reports whether a size or scale change runs CanvasFrame
// synchronously from "size-allocate" / "notify::scale-factor" (GL path)
// instead of from the draw handler it queues (cairo path).
func (g canvasGL) resizeFrame() bool { return g.on }

// drawFrames reports whether the drawing area's draw handler renders a
// wanted frame (cairo path only, see tick).
func (g canvasGL) drawFrames() bool { return !g.on }

// paintSurface reports whether the drawing area's draw handler paints the
// cairo surface: always on the cairo path, on the GL path only while a CPU
// frame is shown (otherwise the opaque GLArea covers it and the surface
// may be stale).
func (g canvasGL) paintSurface() bool { return !g.on || g.cpuShown }

// partial reports whether PresentDamage may present only the dirty
// rectangles. On the GL path every frame is whole: the GLArea redraws its
// whole buffer, and a CPU frame that hides it must fill the drawing area.
func (g canvasGL) partial() bool { return !g.on }

// canvasGLAreaOp is what Present does with the GLArea.
type canvasGLAreaOp uint8

const (
	glAreaNone canvasGLAreaOp = iota // nothing (cairo path, or no change)
	glAreaHide                       // a CPU frame came: hide the GLArea, the drawing area shows it
	glAreaShow                       // the GPU presented again: show the GLArea
)

// present updates g for Present(img) (cpu true) or Present(nil) (cpu
// false) and returns what to do with the GLArea. On the cairo path it is
// always glAreaNone.
func (g *canvasGL) present(cpu bool) canvasGLAreaOp {
	switch {
	case !g.on:
		return glAreaNone
	case cpu && !g.cpuShown:
		g.cpuShown = true
		return glAreaHide
	case !cpu && g.cpuShown:
		g.cpuShown = false
		return glAreaShow
	}
	return glAreaNone
}

// canvasGLSupport reports whether a canvas can switch to GL presentation:
// not attached to a WebView surface (attached canvases are layered, may be
// pass-through and need per-pixel alpha) and not transparent (the GLArea
// has no alpha channel and cannot show the desktop behind the window).
func canvasGLSupport(attached, transparent bool) bool { return !attached && !transparent }
