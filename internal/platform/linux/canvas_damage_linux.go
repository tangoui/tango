//go:build linux && !android

package linux

// Partial frames (platform.CanvasDamagePresenter). The cairo image surface
// always holds the last frame; PresentDamage converts only the dirty
// rectangles of the new frame into it and queues a redraw of those areas
// only (gtk_widget_queue_draw_area, in logical widget coordinates: GTK 3
// scales them by the integer scale factor), so the "draw" handler's
// cairo_paint is clipped to them. Frames normally run from the frame clock
// tick (onTick), before GTK paints, which is what makes this work: a frame
// rendered inside the draw handler (a size change, the first frame) has
// the whole widget invalid already and presents whole.
//
// The whole frame is presented instead (Present) when there is no surface
// of the frame's size yet and when the damage does not pay off
// (platform.DamageRects).

import (
	"image"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasDamagePresenter = (*canvas)(nil)

// canvasDamageMaxRects is the longest dirty list presented partially.
const canvasDamageMaxRects = 32

// PresentDamage implements platform.CanvasDamagePresenter. Main thread.
func (c *canvas) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	if c.closed {
		return canvasErrClosed
	}
	if img == nil || c.inDraw {
		return c.Present(img)
	}
	b := img.Bounds()
	if c.surf == 0 || c.surfW != b.Dx() || c.surfH != b.Dy() {
		return c.Present(img)
	}
	rects, full := platform.DamageRects(dirty, b, canvasDamageMaxRects)
	if full {
		return c.Present(img)
	}
	f := &canvasFFI
	stride := int(f.imageSurfaceGetStride(c.surf))
	data := f.imageSurfaceGetData(c.surf)
	if data == 0 || stride < c.surfW*4 {
		return c.Present(img)
	}
	f.surfaceFlush(c.surf) // finish pending cairo access before writing directly
	dst := unsafe.Slice((*byte)(toPtr(data)), stride*c.surfH)
	for _, r := range rects {
		d := r.Sub(b.Min) // surface coordinates
		so := img.PixOffset(r.Min.X, r.Min.Y)
		canvasRGBAToARGB32(dst[d.Min.Y*stride+d.Min.X*4:], stride, img.Pix[so:], img.Stride, d.Dx(), d.Dy())
	}
	f.surfaceMarkDirty(c.surf)
	for _, r := range rects {
		l := platform.DamageToLogical(r.Sub(b.Min), c.scale)
		f.widgetQueueDrawArea(c.widget, int32(l.Min.X), int32(l.Min.Y), int32(l.Dx()), int32(l.Dy()))
	}
	return nil
}
