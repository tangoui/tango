//go:build windows && (amd64 || arm64)

package d3d11

import (
	"image"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu"
)

// Retained implements gpu.Retainer: the target holds the last frame
// executed at size, and nothing since recreated it or lost the device.
func (d *Device) Retained(size image.Point) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && d.health.lost == nil && d.hasFrame && d.target.tex != 0 && d.target.size == size
}

// ensureScratch makes d.scratch fit the sources of copies stacked
// vertically (scratchPlan). Sources too large for one texture return an
// error wrapping gpu.ErrNotRetained (not a failure); a failed creation
// returns an allocError.
func (d *Device) ensureScratch(copies []gpu.CopyRegion) error {
	used, create, size, err := scratchPlan(copies, d.scratch.size, d.scratch.tex != 0, d.maxTex)
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
	t, err := newTexture(d.dev, size, scratchSpec, nil, 0)
	if err != nil {
		return &allocError{err}
	}
	d.scratch.release()
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
// CopySubresourceRegion between overlapping regions of one resource is
// undefined in Direct3D 11 (and the same resource as source and
// destination is only allowed for disjoint regions), so every source is
// first copied into the scratch texture, stacked vertically, then from
// there to its destination. The immediate context runs the copies in
// order, before the draws issued after them.
func (d *Device) copyRegions(copies []gpu.CopyRegion) {
	for pass := range 2 {
		y := 0
		for _, c := range copies {
			w, h := c.Src.Dx(), c.Src.Dy()
			if pass == 0 {
				src := box{Left: uint32(c.Src.Min.X), Top: uint32(c.Src.Min.Y), Front: 0, Right: uint32(c.Src.Max.X), Bottom: uint32(c.Src.Max.Y), Back: 1}
				d.copyRegion(d.scratch.tex, 0, y, d.target.tex, &src)
			} else {
				src := box{Left: 0, Top: uint32(y), Front: 0, Right: uint32(w), Bottom: uint32(y + h), Back: 1}
				d.copyRegion(d.target.tex, c.Dst.Min.X, c.Dst.Min.Y, d.scratch.tex, &src)
			}
			y += h
		}
	}
}

// copyRegion copies the src box of texture from to (x, y) of texture to.
func (d *Device) copyRegion(to uintptr, x, y int, from uintptr, src *box) {
	// CopySubresourceRegion(pDstResource, DstSubresource, DstX, DstY,
	// DstZ, pSrcResource, SrcSubresource, pSrcBox)
	comCall(d.ctx, ctxCopySubresourceRegion, to, 0, uintptr(x), uintptr(y), 0, from, 0, uintptr(unsafe.Pointer(src)))
}
