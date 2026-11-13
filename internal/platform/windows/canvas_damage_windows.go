//go:build windows && (amd64 || arm64)

package windows

// Partial frames (platform.CanvasDamagePresenter) on the GDI path. The DIB
// section always holds the last frame; PresentDamage converts only the
// dirty rectangles of the new frame into it and gets only those onto the
// screen:
//
//   - Inside WM_PAINT (the usual case: paintNow runs the frame through a
//     synchronous WM_PAINT) the paint DC's clip region is the whole
//     client area, because paintNow invalidates everything before it
//     knows what the frame will change. paintNow therefore records the
//     update rectangle that was pending before (exposed parts the system
//     wants repainted), and the paint blits that rectangle and the dirty
//     rectangles only (blitDamage), instead of the whole DIB.
//   - Outside WM_PAINT it invalidates the dirty rectangles only, so the
//     next WM_PAINT's clip region limits the blit.
//
// The whole frame is presented instead (Present) when there is no DIB of
// the frame's size yet, after UseGPUWindow (the sibling window shows CPU
// frames), when the client area changed in the same paint, and when the
// damage does not pay off (platform.DamageRects).

import (
	"image"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasDamagePresenter = (*canvas)(nil)

// canvasDamageMaxRects is the longest dirty list presented partially.
const canvasDamageMaxRects = 32

var canvasGetUpdateRect = user32.NewProc("GetUpdateRect")

// canvasDamage is the partial-frame state of a canvas. UI thread.
type canvasDamage struct {
	own     bool // the current WM_PAINT comes from paintNow
	sys     rect // update rectangle pending before paintNow (empty: none)
	partial bool // the frame of this paint was presented partially
	rects   []image.Rectangle
}

// notePendingUpdate records the update rectangle the system has pending
// before paintNow invalidates the whole window.
func (c *canvas) notePendingUpdate() {
	var r rect
	if ok, _, _ := canvasGetUpdateRect.Call(c.hwnd, uintptr(unsafe.Pointer(&r)), 0); ok == 0 {
		r = rect{}
	}
	c.dmg.own, c.dmg.sys = true, r
}

// PresentDamage implements platform.CanvasDamagePresenter.
func (c *canvas) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	if c.closed {
		return errCanvasClosed
	}
	if img == nil || c.gpu.on {
		return c.Present(img)
	}
	b := img.Bounds()
	if c.dib == 0 || c.dibW != b.Dx() || c.dibH != b.Dy() {
		return c.Present(img)
	}
	rects, full := platform.DamageRects(dirty, b, canvasDamageMaxRects)
	if full {
		return c.Present(img)
	}
	canvasGdiFlush.Call() // GDI may still read the bits from the last BitBlt
	w := c.dibW
	dst := unsafe.Slice((*byte)(native(c.dibBits)), w*c.dibH*4)
	for i, r := range rects {
		d := r.Sub(b.Min) // DIB coordinates
		rects[i] = d
		so := img.PixOffset(r.Min.X, r.Min.Y)
		do := (d.Min.Y*w + d.Min.X) * 4
		canvasRGBAToBGRA(dst[do:], w*4, img.Pix[so:], img.Stride, d.Dx(), d.Dy())
	}
	p := img.Pix[img.PixOffset(b.Min.X, b.Min.Y):]
	c.bg, c.hasBG = canvasColorRef(p[0], p[1], p[2]), true
	if c.inPaint {
		c.dmg.partial = true
		c.dmg.rects = append(c.dmg.rects, rects...)
		return nil
	}
	for _, r := range rects {
		rc := rect{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)}
		canvasInvalidateRect.Call(c.hwnd, uintptr(unsafe.Pointer(&rc)), 0)
	}
	return nil
}

// blitDamage is blit for a partially presented frame: only the update
// rectangle the system asked for (sys) and the dirty rectangles. When the
// DIB does not cover the client area it falls back to blit, which also
// fills the uncovered part.
func (c *canvas) blitDamage(hdc uintptr, sys rect) {
	if int32(c.dibW) < c.pxW || int32(c.dibH) < c.pxH {
		c.blit(hdc)
		return
	}
	pending := image.Rect(int(sys.Left), int(sys.Top), int(sys.Right), int(sys.Bottom))
	for _, r := range canvasDamageBlits(c.dmg.rects, pending, c.dibW, c.dibH) {
		canvasBitBlt.Call(hdc, uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()),
			c.memDC, uintptr(r.Min.X), uintptr(r.Min.Y), canvasSrcCopy)
	}
}
