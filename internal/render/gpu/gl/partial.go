//go:build linux || darwin

package gl

import (
	"image"

	"github.com/tangoui/tango/internal/render/gpu"
)

// Retained implements gpu.Retainer: the target holds the last frame
// executed at size, and nothing since recreated it, failed a frame that
// may have drawn into it, or lost or closed the device.
func (d *Device) Retained(size image.Point) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && d.lost == nil && d.hasFrame && d.target.tex != 0 && d.target.size == size
}

// ensureScratch makes d.scratch fit the sources of copies stacked
// vertically (scratchPlan); on the context's thread. Sources too large
// for one texture return an error wrapping gpu.ErrNotRetained (not a
// failure); a failed creation returns an allocError.
func (d *Device) ensureScratch(copies []gpu.CopyRegion, limit int) error {
	used, create, size, err := scratchPlan(copies, d.scratch.size, d.scratch.tex != 0, limit)
	if err != nil {
		return err
	}
	if !used {
		return nil
	}
	d.scratchIdle = 0
	if !create {
		return nil
	}
	t, err := newRenderTex(d.f, gpu.TexLayer, size)
	if err != nil {
		return err
	}
	d.scratch.release(d.f)
	d.scratch = t
	return nil
}

// scratchBytes returns the bytes of the copy scratch texture (tests).
func (d *Device) scratchBytes() int {
	if d.scratch.tex == 0 {
		return 0
	}
	return texBytes(d.scratch.size)
}

// copyRegions issues a partial frame's copies within the target. A
// glBlitFramebuffer whose source and destination overlap in one
// framebuffer is undefined, so every source is first blitted into the
// scratch texture, stacked vertically, then from there to its
// destination; GL_NEAREST between equal sizes copies the texels exactly.
// Both textures store row 0 at the top, so the rectangles are the frame
// contract's. Blits honour the scissor test, which is off for them; the
// image and layer units hold the placeholder (encode binds it before
// anything else), so neither texture is sampled while it is drawn into.
// GL runs the blits in order, before the draws issued after them.
func (d *Device) copyRegions(copies []gpu.CopyRegion) {
	f := d.f
	f.Disable(glSCISSOR_TEST)
	for pass := range 2 {
		from, to := d.fbo, d.scratch.fbo
		if pass == 1 {
			from, to = to, from
		}
		f.BindFramebuffer(glREAD_FRAMEBUFFER, from)
		f.BindFramebuffer(glDRAW_FRAMEBUFFER, to)
		y := 0
		for _, c := range copies {
			w, h := int32(c.Src.Dx()), int32(c.Src.Dy())
			sy := int32(y)
			if pass == 0 {
				f.BlitFramebuffer(int32(c.Src.Min.X), int32(c.Src.Min.Y), int32(c.Src.Max.X), int32(c.Src.Max.Y),
					0, sy, w, sy+h, glCOLOR_BUFFER_BIT, glNEAREST)
			} else {
				f.BlitFramebuffer(0, sy, w, sy+h,
					int32(c.Dst.Min.X), int32(c.Dst.Min.Y), int32(c.Dst.Max.X), int32(c.Dst.Max.Y), glCOLOR_BUFFER_BIT, glNEAREST)
			}
			y += int(h)
		}
	}
	f.Enable(glSCISSOR_TEST)
}
