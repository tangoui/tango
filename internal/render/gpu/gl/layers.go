//go:build linux || darwin

package gl

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"unsafe"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/devkit"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// The layer texture pool, the partial-frame checks, the copy scratch
// sizing and the clearing quads live in internal/render/gpu/internal/devkit (shared
// with internal/render/gpu/d3d11), written over an abstract texture handle. The
// wrappers here give their invalid-frame errors the "gl: " prefix, as
// internal/render/gpu/d3d11/layers.go does with "d3d11: ".

// scratchIdleFrames is how many frames without Copies the copy scratch
// texture is kept for (as internal/render/gpu/metal's).
const scratchIdleFrames = 120

func texBytes(size image.Point) int { return devkit.TexBytes(size) }

// allocError marks an error creating a GPU resource (a failure the device
// counts), as opposed to an invalid frame.
type allocError = devkit.AllocError

// isAllocError reports whether err is (or wraps) an allocError.
func isAllocError(err error) bool { return devkit.IsAllocError(err) }

// renderTex is an RGBA8 texture with a framebuffer object of its own that
// has it as its color attachment: a layer or the copy scratch texture.
// It is comparable (the layer pool finds its textures with ==).
type renderTex struct {
	texture
	fbo uint32
}

// newRenderTex creates a render texture of size, bound to texture unit
// unit and with its framebuffer bound to GL_FRAMEBUFFER. An incomplete
// framebuffer (the allocation failed, say) deletes both and returns an
// allocError.
func newRenderTex(f *glFuncs, unit uint32, size image.Point) (renderTex, error) {
	rt := renderTex{texture: newTexture(f, unit, size, rgba8Format, nil)}
	f.GenFramebuffers(1, &rt.fbo)
	f.BindFramebuffer(glFRAMEBUFFER, rt.fbo)
	f.FramebufferTexture2D(glFRAMEBUFFER, glCOLOR_ATTACHMENT0, glTEXTURE_2D, rt.tex, 0)
	if st := f.CheckFramebufferStatus(glFRAMEBUFFER); st != glFRAMEBUFFER_COMPLETE {
		rt.release(f)
		return renderTex{}, &allocError{Err: fmt.Errorf("gl: render texture %d×%d: framebuffer status %#x", size.X, size.Y, st)}
	}
	return rt, nil
}

// release deletes the framebuffer and the texture.
func (rt *renderTex) release(f *glFuncs) {
	if rt.fbo != 0 {
		f.DeleteFramebuffers(1, &rt.fbo)
	}
	rt.texture.release(f)
	*rt = renderTex{}
}

// layerPool gives each layer of a frame a render texture
// (devkit.LayerPool; assign prefixes its invalid-frame errors).
type layerPool struct {
	devkit.LayerPool[renderTex]
}

// newLayerPool returns the pool of layer render textures of the device;
// its textures are created and freed on the context's thread (Assign
// runs there).
func (d *Device) newLayerPool() *layerPool {
	return &layerPool{devkit.LayerPool[renderTex]{
		NewTex: func(size image.Point) (renderTex, error) {
			return newRenderTex(d.f, gpu.TexLayer, size)
		},
		Free: func(t renderTex) { t.release(d.f) },
	}}
}

// assign is devkit.LayerPool.Assign (backdrop grabs included) with the invalid-frame errors
// prefixed "gl: "; an allocError is returned as is.
func (p *layerPool) assign(f *gpu.Frame) error {
	err := p.Assign(f)
	if err != nil && !isAllocError(err) {
		return fmt.Errorf("gl: %w", err)
	}
	return err
}

// layerTextures returns the number of layer textures the device holds
// (tests).
func (d *Device) layerTextures() int { return d.layers.Count() }

// checkPartial is devkit.CheckPartial.
func checkPartial(f *gpu.Frame) error {
	if err := devkit.CheckPartial(f); err != nil {
		return fmt.Errorf("gl: %w", err)
	}
	return nil
}

// scratchPlan is devkit.ScratchPlan.
func scratchPlan(copies []gpu.CopyRegion, have image.Point, exists bool, limit int) (used, create bool, size image.Point, err error) {
	used, create, size, err = devkit.ScratchPlan(copies, have, exists, limit)
	if err != nil {
		err = fmt.Errorf("gl: %w", err)
	}
	return used, create, size, err
}

// checkImageUpload validates an image upload: a positive ID and size and
// enough pixels for its rows.
func checkImageUpload(u gpu.ImageUpload) error {
	w, h := u.Size.X, u.Size.Y
	if u.ID <= 0 || w <= 0 || h <= 0 {
		return fmt.Errorf("gl: image upload %d of size %v", u.ID, u.Size)
	}
	if u.Stride < 0 || u.Stride > 1<<30 || len(u.Pix) < (h-1)*u.Stride+4*w {
		return fmt.Errorf("gl: image upload %d: %d bytes with stride %d for %v", u.ID, len(u.Pix), u.Stride, u.Size)
	}
	return nil
}

// checkImages validates f's image uploads and that every batch binding an
// image binds one the device holds after the frame's frees and uploads.
func (d *Device) checkImages(f *gpu.Frame) error {
	for _, u := range f.ImageUploads {
		if err := checkImageUpload(u); err != nil {
			return err
		}
	}
	for i := range f.Batches {
		id := f.Batches[i].Image
		if id == 0 {
			continue
		}
		held := d.images[id].tex != 0 && !slices.Contains(f.ImageFrees, id)
		uploaded := slices.ContainsFunc(f.ImageUploads, func(u gpu.ImageUpload) bool { return u.ID == id })
		if !held && !uploaded {
			return fmt.Errorf("gl: batch %d binds image %d, which the device does not hold", i, id)
		}
	}
	return nil
}

// applyImages deletes the textures of f.ImageFrees, then creates an RGBA8
// texture from each of f.ImageUploads (validated by checkImages),
// replacing one of the same ID. GL keeps a deleted texture alive until
// the draws already issued that sample it have run. Allocation failures
// surface as GL_OUT_OF_MEMORY at the frame's error check.
func (d *Device) applyImages(f *gpu.Frame) {
	if d.images == nil {
		d.images = map[int]texture{}
	}
	for _, id := range f.ImageFrees {
		t := d.images[id]
		t.release(d.f)
		delete(d.images, id)
	}
	for _, u := range f.ImageUploads {
		t := newImageTexture(d.f, u)
		old := d.images[u.ID]
		old.release(d.f)
		d.images[u.ID] = t
	}
}

// newImageTexture creates the texture of a validated image upload, bound
// to texture unit gpu.TexImage. Rows u.Stride bytes apart are read with
// GL_UNPACK_ROW_LENGTH = Stride/4 (reading (h-1)·Stride + 4·w bytes,
// inside u.Pix); a stride that is no multiple of 4, or shorter than a
// row, is packed into a tight copy first.
func newImageTexture(f *glFuncs, u gpu.ImageUpload) texture {
	w, h := u.Size.X, u.Size.Y
	row := 4 * w
	pix, rowLen := u.Pix, int32(u.Stride/4)
	if h == 1 {
		rowLen = 0
	} else if u.Stride%4 != 0 || u.Stride < row {
		pix, rowLen = make([]byte, row*h), 0
		for y := range h {
			copy(pix[y*row:][:row], u.Pix[y*u.Stride:][:row])
		}
	}
	t := texture{size: u.Size}
	f.GenTextures(1, &t.tex)
	f.ActiveTexture(glTEXTURE0 + gpu.TexImage)
	f.BindTexture(glTEXTURE_2D, t.tex)
	f.TexParameteri(glTEXTURE_2D, glTEXTURE_BASE_LEVEL, 0)
	f.TexParameteri(glTEXTURE_2D, glTEXTURE_MAX_LEVEL, 0)
	f.PixelStorei(glUNPACK_ALIGNMENT, 1)
	f.PixelStorei(glUNPACK_ROW_LENGTH, rowLen)
	f.TexImage2D(glTEXTURE_2D, 0, glRGBA8, int32(w), int32(h), 0, glRGBA, glUNSIGNED_BYTE, unsafe.Pointer(&pix[0]))
	restoreUnpack(f)
	return t
}

// instGather collects a frame's instances in instance buffer order (per
// batch, then the clearing quads of a partial frame), reusing its
// buffers from frame to frame (as internal/render/gpu/d3d11's).
type instGather struct {
	batches  [][]shaders.Instance
	rescaled []shaders.Instance // composites rescaled to a larger pooled layer texture
	clears   []shaders.Instance // the clearing quads of a partial frame
}

// gather returns each batch's instances, in batch order, and then the
// clearing quads of f.Dirty (in f.Clear) as the last entry (empty for a
// full frame). A batch compositing layer n held in a texture larger than
// the layer (layerSize[n], from the layer pool) gets a rescaled copy of
// its instances (devkit.RescaleComposites); every other batch's slice is
// the frame's own. The result is valid until the next gather or reset.
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
			devkit.RescaleComposites(ins, f.Layers[b.Layer-1].Size, layerSize[b.Layer])
		}
		g.batches = append(g.batches, ins)
	}
	g.clears = devkit.ClearInstances(g.clears[:0], f.Dirty, f.Clear)
	g.batches = append(g.batches, g.clears)
	return g.batches
}

// reset drops the references gather keeps to the last frame's slices.
func (g *instGather) reset() {
	clear(g.batches)
	g.batches = g.batches[:0]
}

// clearBound clears all of the bound framebuffer to c (premultiplied);
// glClear honours the scissor test, so it is off for it.
func clearBound(f *glFuncs, c [4]float32) {
	f.Disable(glSCISSOR_TEST)
	f.ClearColor(c[0], c[1], c[2], c[3])
	f.Clear(glCOLOR_BUFFER_BIT)
	f.Enable(glSCISSOR_TEST)
}

// transparent is the color a layer starts as.
var transparent = devkit.ClearColor(color.NRGBA{})
