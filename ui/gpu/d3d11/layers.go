package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/shader/sl"
)

// The layer texture pool, the partial-frame checks and the copy scratch
// sizing, written over an abstract texture handle so plan_test.go and
// layers_test.go run them on the development machine. They follow
// ui/gpu/metal's assignLayers, takeLayer, releaseLayers, trimLayers,
// putLayer, rescaleComposites, checkPartial and ensureScratch line for
// line.

// layerIdleFrames is how many frames a pooled layer texture may go unused
// before it is released.
const layerIdleFrames = 120

// layerPoolBytes bounds the bytes of the layer textures the pool keeps
// (beyond what one frame holds at once).
const layerPoolBytes = 128 << 20

// scratchIdleFrames is how many frames without Copies the copy scratch
// texture is kept for.
const scratchIdleFrames = 120

func texBytes(size image.Point) int { return size.X * size.Y * 4 }

// allocError marks an error creating a GPU resource (a failure the device
// counts), as opposed to an invalid frame.
type allocError struct{ err error }

func (e *allocError) Error() string { return e.err.Error() }
func (e *allocError) Unwrap() error { return e.err }

// isAllocError reports whether err is (or wraps) an allocError.
func isAllocError(err error) bool {
	var ae *allocError
	return errors.As(err, &ae)
}

// pooledLayer is a texture of the layer pool.
type pooledLayer[T comparable] struct {
	tex  T
	size image.Point
	busy bool // assigned to a layer whose last use is still to come
	used int  // frames of the last frame that used it
}

// layerPool gives each layer of a frame a render-target texture (see
// assign). T is the device's texture handle; newTex creates one of a size
// and free releases one.
type layerPool[T comparable] struct {
	texs   []pooledLayer[T]
	frames int // frames assigned (the pool's clock)
	newTex func(size image.Point) (T, error)
	free   func(T)

	// Per frame, indexed by layer number (1-based; entry 0 unused):
	tex  []T           // the layer's texture
	size []image.Point // the size of tex[n] (≥ the layer's Size)
	has  []bool        // tex[n] is assigned
}

// count returns the number of textures the pool holds.
func (p *layerPool[T]) count() int { return len(p.texs) }

// bytes returns the bytes of the textures the pool holds.
func (p *layerPool[T]) bytes() int {
	n := 0
	for _, l := range p.texs {
		n += texBytes(l.size)
	}
	return n
}

// assign checks the frame's use of layers (every batch's Target and Layer
// name one of f.Layers, a batch samples only a layer an earlier batch
// drew into, never its own target) and gives each layer drawn into a
// texture: at the first batch drawing into it the smallest free texture
// at least its Size and at most twice its Size on each axis, creating one
// of exactly its Size when none is free, and frees the texture after the
// layer's last use so a later layer of the frame reuses it. Creating a
// texture first releases the free textures no earlier batch of this frame
// used that it covers on both axes and, oldest first, free textures while
// the pool would exceed layerPoolBytes. Afterwards textures unused for
// layerIdleFrames frames are released and the pool is trimmed again.
// A texture that cannot be created returns an allocError.
func (p *layerPool[T]) assign(f *gpu.Frame) error {
	p.frames++
	p.trim(0) // last frame's textures may go now
	n := len(f.Layers) + 1
	var zero T
	p.tex = append(p.tex[:0], make([]T, n)...)
	p.size = append(p.size[:0], make([]image.Point, n)...)
	p.has = append(p.has[:0], make([]bool, n)...)
	for i := range p.tex {
		p.tex[i] = zero
	}
	if len(f.Layers) > 0 {
		last := make([]int, n)
		for i := range f.Batches {
			b := &f.Batches[i]
			for _, l := range [2]int{b.Target, b.Layer} {
				if l < 0 || l > len(f.Layers) {
					return fmt.Errorf("d3d11: batch %d names layer %d of %d", i, l, len(f.Layers))
				}
				last[l] = i
			}
			if b.Target != 0 && b.Target == b.Layer {
				return fmt.Errorf("d3d11: batch %d samples the layer it draws into", i)
			}
		}
		for i := range f.Batches {
			b := &f.Batches[i]
			if l := b.Target; l != 0 && !p.has[l] {
				ld := f.Layers[l-1]
				if ld.Size.X <= 0 || ld.Size.Y <= 0 || ld.Bounds.Dx() > ld.Size.X || ld.Bounds.Dy() > ld.Size.Y {
					p.freeAll()
					return fmt.Errorf("d3d11: layer %d of size %v for bounds %v", l, ld.Size, ld.Bounds)
				}
				t, size, err := p.take(ld.Size)
				if err != nil {
					p.freeAll()
					return &allocError{err}
				}
				p.tex[l], p.size[l], p.has[l] = t, size, true
			}
			if l := b.Layer; l != 0 && !p.has[l] {
				p.freeAll()
				return fmt.Errorf("d3d11: batch %d samples layer %d before any batch drew into it", i, l)
			}
			for _, l := range [2]int{b.Target, b.Layer} {
				if l != 0 && last[l] == i {
					p.put(p.tex[l])
				}
			}
		}
	}
	p.release(func(l *pooledLayer[T]) bool { return p.frames-l.used > layerIdleFrames })
	p.trim(0)
	return nil
}

// take returns the smallest free texture of at least size and at most
// twice size on each axis, and its size, creating one of size when there
// is none, and marks it busy.
func (p *layerPool[T]) take(size image.Point) (T, image.Point, error) {
	best := -1
	for i := range p.texs {
		l := &p.texs[i]
		if l.busy || l.size.X < size.X || l.size.Y < size.Y || l.size.X > 2*size.X || l.size.Y > 2*size.Y {
			continue
		}
		if best < 0 || texBytes(l.size) < texBytes(p.texs[best].size) {
			best = i
		}
	}
	if best >= 0 {
		l := &p.texs[best]
		l.busy, l.used = true, p.frames
		return l.tex, l.size, nil
	}
	p.release(func(l *pooledLayer[T]) bool {
		return l.used < p.frames && l.size.X <= size.X && l.size.Y <= size.Y
	})
	p.trim(texBytes(size))
	t, err := p.newTex(size)
	if err != nil {
		var zero T
		return zero, image.Point{}, err
	}
	p.texs = append(p.texs, pooledLayer[T]{tex: t, size: size, busy: true, used: p.frames})
	return t, size, nil
}

// release releases the free textures drop selects.
func (p *layerPool[T]) release(drop func(l *pooledLayer[T]) bool) {
	keep := p.texs[:0]
	for i := range p.texs {
		if l := p.texs[i]; !l.busy && drop(&l) {
			p.free(l.tex)
			continue
		}
		keep = append(keep, p.texs[i])
	}
	clear(p.texs[len(keep):])
	p.texs = keep
}

// trim releases free textures, least recently used first, while the
// pool's bytes plus extra exceed layerPoolBytes. Textures this frame uses
// are kept.
func (p *layerPool[T]) trim(extra int) {
	total := p.bytes() + extra
	for total > layerPoolBytes {
		old := -1
		for i := range p.texs {
			if l := &p.texs[i]; !l.busy && l.used < p.frames && (old < 0 || l.used < p.texs[old].used) {
				old = i
			}
		}
		if old < 0 {
			return
		}
		total -= texBytes(p.texs[old].size)
		p.free(p.texs[old].tex)
		p.texs = slices.Delete(p.texs, old, old+1)
	}
}

// put marks texture t free.
func (p *layerPool[T]) put(t T) {
	for i := range p.texs {
		if p.texs[i].tex == t {
			p.texs[i].busy = false
			return
		}
	}
}

// freeAll marks every texture free (a frame that is not drawn after all).
func (p *layerPool[T]) freeAll() {
	for i := range p.texs {
		p.texs[i].busy = false
	}
}

// releaseAll releases every texture (Close).
func (p *layerPool[T]) releaseAll() {
	for _, l := range p.texs {
		p.free(l.tex)
	}
	p.texs = nil
	var zero T
	for i := range p.tex {
		p.tex[i] = zero
	}
}

// rescaleComposites converts the layer composites among ins, whose UVs
// and half-texel are normalized by the layer's Size, to the layer's
// texture of size tex (larger when pooled for a larger layer).
func rescaleComposites(ins []shaders.Instance, size, tex image.Point) {
	if size == tex || tex.X <= 0 || tex.Y <= 0 {
		return
	}
	kx, ky := float32(size.X)/float32(tex.X), float32(size.Y)/float32(tex.Y)
	for i := range ins {
		in := &ins[i]
		if in.Kind.X != shaders.KindPatch || in.Kind.W < 0.5 {
			continue
		}
		in.UV = sl.V4(in.UV.X*kx, in.UV.Y*ky, in.UV.Z*kx, in.UV.W*ky)
		in.Kind.Y, in.Kind.Z = 0.5/float32(tex.X), 0.5/float32(tex.Y)
	}
}

// ---- Partial frames ----

// checkPartial validates a partial frame's Dirty and Copies against its
// target: dirty rectangles non-empty, inside and disjoint; copies of
// equal non-empty sizes inside the target.
func checkPartial(f *gpu.Frame) error {
	t := image.Rectangle{Max: f.Size}
	for i, r := range f.Dirty {
		if r.Empty() || !r.In(t) {
			return fmt.Errorf("d3d11: dirty rectangle %v outside the target %v", r, f.Size)
		}
		for _, o := range f.Dirty[:i] {
			if o.Overlaps(r) {
				return fmt.Errorf("d3d11: dirty rectangles %v and %v overlap", o, r)
			}
		}
	}
	for _, c := range f.Copies {
		if c.Src.Size() != c.Dst.Size() || c.Src.Empty() || !c.Src.In(t) || !c.Dst.In(t) {
			return fmt.Errorf("d3d11: copy %v → %v in a %v target", c.Src, c.Dst, f.Size)
		}
	}
	return nil
}

// scratchPlan decides the copy scratch texture for copies, whose sources
// are stacked vertically in it, given the current scratch (have; exists
// false when there is none) and the texture limit. used is false when
// there are no copies (nothing changes). Otherwise create says whether to
// replace the scratch with one of size: when it does not fit, or is more
// than four times the area needed (the window shrank); when growing, the
// larger side of the old one is kept too, so alternating shapes do not
// recreate it every frame. Sources too tall to stack in one texture
// return an error wrapping gpu.ErrNotRetained, so the Renderer draws the
// frame in full (it is not a device failure).
func scratchPlan(copies []gpu.CopyRegion, have image.Point, exists bool, limit int) (used, create bool, size image.Point, err error) {
	var need image.Point
	for _, c := range copies {
		need.X = max(need.X, c.Src.Dx())
		need.Y += c.Src.Dy()
	}
	if need.X == 0 {
		return false, false, image.Point{}, nil
	}
	fits := exists && need.X <= have.X && need.Y <= have.Y
	if fits && texBytes(have) <= 4*texBytes(need) {
		return true, false, have, nil
	}
	if need.X > limit || need.Y > limit {
		return true, false, image.Point{}, fmt.Errorf("d3d11: copy sources %d×%d exceed one scratch texture (max %d): %w", need.X, need.Y, limit, gpu.ErrNotRetained)
	}
	size = need
	if !fits && exists && texBytes(have) <= 4*texBytes(need) {
		size = image.Pt(max(need.X, have.X), max(need.Y, have.Y))
	}
	return true, true, size, nil
}

// clearInstances are the quads that clear a partial frame's dirty
// rectangles to c, one per rectangle and slightly larger than it (each is
// drawn scissored to its rectangle, without blending). paint.CPU fills
// the target with the 8-bit premultiplied color; c/255 gives exactly
// those bytes (the box coverage of a whole-pixel rectangle is exactly 1
// inside it).
func clearInstances(dst []shaders.Instance, dirty []image.Rectangle, c color.NRGBA) []shaders.Instance {
	cc := clearColor(c)
	col := sl.V4(cc[0], cc[1], cc[2], cc[3])
	for _, r := range dirty {
		dst = append(dst, shaders.Instance{
			Rect:  sl.V4(float32(r.Min.X-1), float32(r.Min.Y-1), float32(r.Dx()+2), float32(r.Dy()+2)),
			Color: col,
			Kind:  sl.V4(shaders.KindSolid, 0, 0, 0),
		})
	}
	return dst
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
