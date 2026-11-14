package d3d11

import (
	"fmt"
	"image"
	"image/color"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/devkit"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// The layer texture pool, the partial-frame checks, the copy scratch
// sizing and the clearing quads live in internal/render/gpu/internal/devkit (shared
// with internal/render/gpu/gl), written over an abstract texture handle so they are
// tested on any machine. The wrappers here give their invalid-frame
// errors the "d3d11: " prefix.

// scratchIdleFrames is how many frames without Copies the copy scratch
// texture is kept for.
const scratchIdleFrames = 120

func texBytes(size image.Point) int { return devkit.TexBytes(size) }

// allocError marks an error creating a GPU resource (a failure the device
// counts), as opposed to an invalid frame.
type allocError = devkit.AllocError

// isAllocError reports whether err is (or wraps) an allocError.
func isAllocError(err error) bool { return devkit.IsAllocError(err) }

// layerPool gives each layer of a frame a render-target texture
// (devkit.LayerPool; assign prefixes its invalid-frame errors).
type layerPool[T comparable] struct {
	devkit.LayerPool[T]
}

// assign is devkit.LayerPool.Assign with the invalid-frame errors
// prefixed "d3d11: "; an allocError is returned as is.
func (p *layerPool[T]) assign(f *gpu.Frame) error {
	err := p.Assign(f)
	if err != nil && !isAllocError(err) {
		return fmt.Errorf("d3d11: %w", err)
	}
	return err
}

// rescaleComposites is devkit.RescaleComposites.
func rescaleComposites(ins []shaders.Instance, size, tex image.Point) {
	devkit.RescaleComposites(ins, size, tex)
}

// checkPartial is devkit.CheckPartial.
func checkPartial(f *gpu.Frame) error {
	if err := devkit.CheckPartial(f); err != nil {
		return fmt.Errorf("d3d11: %w", err)
	}
	return nil
}

// scratchPlan is devkit.ScratchPlan.
func scratchPlan(copies []gpu.CopyRegion, have image.Point, exists bool, limit int) (used, create bool, size image.Point, err error) {
	used, create, size, err = devkit.ScratchPlan(copies, have, exists, limit)
	if err != nil {
		err = fmt.Errorf("d3d11: %w", err)
	}
	return used, create, size, err
}

// clearInstances is devkit.ClearInstances.
func clearInstances(dst []shaders.Instance, dirty []image.Rectangle, c color.NRGBA) []shaders.Instance {
	return devkit.ClearInstances(dst, dirty, c)
}

// imageUploadPix returns the pixels of an image upload as rows of pitch
// bytes for a texture's initial data, and the pitch: u.Pix itself when
// it holds every row at u.Stride (so the runtime may read Size.Y whole
// rows), otherwise a tightly packed copy. u must be valid
// (checkImageUpload).
func imageUploadPix(u gpu.ImageUpload) (pix []byte, pitch int) {
	row := 4 * u.Size.X
	if u.Stride >= row && len(u.Pix) >= u.Size.Y*u.Stride {
		return u.Pix, u.Stride
	}
	pix = make([]byte, row*u.Size.Y)
	for y := range u.Size.Y {
		copy(pix[y*row:][:row], u.Pix[y*u.Stride:][:row])
	}
	return pix, row
}

// checkImageUpload validates an image upload: a positive ID and size and
// enough pixels for its rows.
func checkImageUpload(u gpu.ImageUpload) error {
	w, h := u.Size.X, u.Size.Y
	if u.ID <= 0 || w <= 0 || h <= 0 {
		return fmt.Errorf("d3d11: image upload %d of size %v", u.ID, u.Size)
	}
	if u.Stride < 0 || len(u.Pix) < (h-1)*u.Stride+4*w {
		return fmt.Errorf("d3d11: image upload %d: %d bytes with stride %d for %v", u.ID, len(u.Pix), u.Stride, u.Size)
	}
	return nil
}

// instGather collects a frame's instances in instance buffer order (per
// batch, then the clearing quads of a partial frame), reusing its
// buffers from frame to frame.
type instGather struct {
	batches  [][]shaders.Instance
	rescaled []shaders.Instance // composites rescaled to a larger pooled layer texture
	clears   []shaders.Instance // the clearing quads of a partial frame
}

// gather returns each batch's instances, in batch order, and then the
// clearing quads of f.Dirty (in f.Clear) as the last entry (empty for a
// full frame). A batch compositing layer n held in a texture larger than
// the layer (layerSize[n], from the layer pool) gets a rescaled copy of
// its instances (rescaleComposites); every other batch's slice is the
// frame's own. The result is valid until the next gather or reset.
func (g *instGather) gather(f *gpu.Frame, layerSize []image.Point) [][]shaders.Instance {
	rescale := func(b *gpu.Batch) bool {
		return b.Layer > 0 && b.Layer < len(layerSize) && len(b.Instances) > 0 && f.Layers[b.Layer-1].Size != layerSize[b.Layer]
	}
	// Copy first, slice after: appending may move g.rescaled.
	g.rescaled = g.rescaled[:0]
	for i := range f.Batches {
		if b := &f.Batches[i]; rescale(b) {
			g.rescaled = append(g.rescaled, b.Instances...)
		}
	}
	g.batches = g.batches[:0]
	k := 0
	for i := range f.Batches {
		b := &f.Batches[i]
		ins := b.Instances
		if rescale(b) {
			ins = g.rescaled[k : k+len(ins) : k+len(ins)]
			k += len(ins)
			rescaleComposites(ins, f.Layers[b.Layer-1].Size, layerSize[b.Layer])
		}
		g.batches = append(g.batches, ins)
	}
	g.clears = clearInstances(g.clears[:0], f.Dirty, f.Clear)
	g.batches = append(g.batches, g.clears)
	return g.batches
}

// reset drops the references gather keeps to the last frame's slices.
func (g *instGather) reset() {
	clear(g.batches)
	g.batches = g.batches[:0]
}
