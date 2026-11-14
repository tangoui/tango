// Package devkit holds the backend-neutral parts of the GPU devices
// (internal/render/gpu/d3d11, internal/render/gpu/gl): the layer texture pool, the partial-frame
// checks, the copy scratch sizing and the clearing quads, written over an
// abstract texture handle so they are tested on any machine. They follow
// internal/render/gpu/metal's assignLayers, takeLayer, releaseLayers, trimLayers,
// putLayer, rescaleComposites, checkPartial and ensureScratch line for
// line.
//
// Errors carry no backend prefix; a device wraps the invalid-frame errors
// with its own ("d3d11: %w"). An AllocError, wrapping the error of the
// device's own NewTex, is returned as is.
package devkit

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"slices"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

// LayerIdleFrames is how many frames a pooled layer texture may go unused
// before it is released.
const LayerIdleFrames = 120

// LayerPoolBytes bounds the bytes of the layer textures the pool keeps
// (beyond what one frame holds at once).
const LayerPoolBytes = 128 << 20

// TexBytes returns the bytes of an RGBA8 texture of size.
func TexBytes(size image.Point) int { return size.X * size.Y * 4 }

// AllocError marks an error creating a GPU resource (a failure the device
// counts), as opposed to an invalid frame.
type AllocError struct{ Err error }

func (e *AllocError) Error() string { return e.Err.Error() }
func (e *AllocError) Unwrap() error { return e.Err }

// IsAllocError reports whether err is (or wraps) an AllocError.
func IsAllocError(err error) bool {
	var ae *AllocError
	return errors.As(err, &ae)
}

// ClearColor is the float color a target is cleared to for c: paint.CPU
// fills the target with the 8-bit premultiplied color, and c/255 converts
// back to exactly those bytes in an RGBA8 UNORM target.
func ClearColor(c color.NRGBA) [4]float32 {
	p := color.RGBAModel.Convert(c).(color.RGBA)
	return [4]float32{float32(p.R) / 255, float32(p.G) / 255, float32(p.B) / 255, float32(p.A) / 255}
}

// PooledLayer is a texture of the layer pool.
type PooledLayer[T comparable] struct {
	Tex  T
	Size image.Point
	Busy bool // assigned to a layer whose last use is still to come
	Used int  // frames of the last frame that used it
}

// LayerPool gives each layer of a frame a render-target texture (see
// Assign). T is the device's texture handle; NewTex creates one of a size
// and Free releases one.
type LayerPool[T comparable] struct {
	texs   []PooledLayer[T]
	frames int // frames assigned (the pool's clock)
	NewTex func(size image.Point) (T, error)
	Free   func(T)

	// Per frame, indexed by layer number (1-based; entry 0 unused):
	Tex  []T           // the layer's texture
	Size []image.Point // the size of Tex[n] (≥ the layer's Size)
	Has  []bool        // Tex[n] is assigned
}

// Count returns the number of textures the pool holds.
func (p *LayerPool[T]) Count() int { return len(p.texs) }

// Bytes returns the bytes of the textures the pool holds.
func (p *LayerPool[T]) Bytes() int {
	n := 0
	for _, l := range p.texs {
		n += TexBytes(l.Size)
	}
	return n
}

// Assign checks the frame's use of layers (every batch's Target and Layer
// name one of f.Layers, a batch samples only a layer an earlier batch
// drew into, never its own target) and gives each layer drawn into a
// texture: at the first batch drawing into it the smallest free texture
// at least its Size and at most twice its Size on each axis, creating one
// of exactly its Size when none is free, and frees the texture after the
// layer's last use so a later layer of the frame reuses it. Creating a
// texture first releases the free textures no earlier batch of this frame
// used that it covers on both axes and, oldest first, free textures while
// the pool would exceed LayerPoolBytes. Afterwards textures unused for
// LayerIdleFrames frames are released and the pool is trimmed again.
// A texture that cannot be created returns an AllocError.
func (p *LayerPool[T]) Assign(f *gpu.Frame) error {
	p.frames++
	p.Trim(0) // last frame's textures may go now
	n := len(f.Layers) + 1
	var zero T
	p.Tex = append(p.Tex[:0], make([]T, n)...)
	p.Size = append(p.Size[:0], make([]image.Point, n)...)
	p.Has = append(p.Has[:0], make([]bool, n)...)
	for i := range p.Tex {
		p.Tex[i] = zero
	}
	if len(f.Layers) > 0 {
		last := make([]int, n)
		for i := range f.Batches {
			b := &f.Batches[i]
			for _, l := range [2]int{b.Target, b.Layer} {
				if l < 0 || l > len(f.Layers) {
					return fmt.Errorf("batch %d names layer %d of %d", i, l, len(f.Layers))
				}
				last[l] = i
			}
			if b.Target != 0 && b.Target == b.Layer {
				return fmt.Errorf("batch %d samples the layer it draws into", i)
			}
		}
		for i := range f.Batches {
			b := &f.Batches[i]
			if l := b.Target; l != 0 && !p.Has[l] {
				ld := f.Layers[l-1]
				if ld.Size.X <= 0 || ld.Size.Y <= 0 || ld.Bounds.Dx() > ld.Size.X || ld.Bounds.Dy() > ld.Size.Y {
					p.FreeAll()
					return fmt.Errorf("layer %d of size %v for bounds %v", l, ld.Size, ld.Bounds)
				}
				t, size, err := p.Take(ld.Size)
				if err != nil {
					p.FreeAll()
					return &AllocError{err}
				}
				p.Tex[l], p.Size[l], p.Has[l] = t, size, true
			}
			if l := b.Layer; l != 0 && !p.Has[l] {
				p.FreeAll()
				return fmt.Errorf("batch %d samples layer %d before any batch drew into it", i, l)
			}
			for _, l := range [2]int{b.Target, b.Layer} {
				if l != 0 && last[l] == i {
					p.Put(p.Tex[l])
				}
			}
		}
	}
	p.Release(func(l *PooledLayer[T]) bool { return p.frames-l.Used > LayerIdleFrames })
	p.Trim(0)
	return nil
}

// Take returns the smallest free texture of at least size and at most
// twice size on each axis, and its size, creating one of size when there
// is none, and marks it busy.
func (p *LayerPool[T]) Take(size image.Point) (T, image.Point, error) {
	best := -1
	for i := range p.texs {
		l := &p.texs[i]
		if l.Busy || l.Size.X < size.X || l.Size.Y < size.Y || l.Size.X > 2*size.X || l.Size.Y > 2*size.Y {
			continue
		}
		if best < 0 || TexBytes(l.Size) < TexBytes(p.texs[best].Size) {
			best = i
		}
	}
	if best >= 0 {
		l := &p.texs[best]
		l.Busy, l.Used = true, p.frames
		return l.Tex, l.Size, nil
	}
	p.Release(func(l *PooledLayer[T]) bool {
		return l.Used < p.frames && l.Size.X <= size.X && l.Size.Y <= size.Y
	})
	p.Trim(TexBytes(size))
	t, err := p.NewTex(size)
	if err != nil {
		var zero T
		return zero, image.Point{}, err
	}
	p.texs = append(p.texs, PooledLayer[T]{Tex: t, Size: size, Busy: true, Used: p.frames})
	return t, size, nil
}

// Release releases the free textures drop selects.
func (p *LayerPool[T]) Release(drop func(l *PooledLayer[T]) bool) {
	keep := p.texs[:0]
	for i := range p.texs {
		if l := p.texs[i]; !l.Busy && drop(&l) {
			p.Free(l.Tex)
			continue
		}
		keep = append(keep, p.texs[i])
	}
	clear(p.texs[len(keep):])
	p.texs = keep
}

// Trim releases free textures, least recently used first, while the
// pool's bytes plus extra exceed LayerPoolBytes. Textures this frame uses
// are kept.
func (p *LayerPool[T]) Trim(extra int) {
	total := p.Bytes() + extra
	for total > LayerPoolBytes {
		old := -1
		for i := range p.texs {
			if l := &p.texs[i]; !l.Busy && l.Used < p.frames && (old < 0 || l.Used < p.texs[old].Used) {
				old = i
			}
		}
		if old < 0 {
			return
		}
		total -= TexBytes(p.texs[old].Size)
		p.Free(p.texs[old].Tex)
		p.texs = slices.Delete(p.texs, old, old+1)
	}
}

// Put marks texture t free.
func (p *LayerPool[T]) Put(t T) {
	for i := range p.texs {
		if p.texs[i].Tex == t {
			p.texs[i].Busy = false
			return
		}
	}
}

// FreeAll marks every texture free (a frame that is not drawn after all).
func (p *LayerPool[T]) FreeAll() {
	for i := range p.texs {
		p.texs[i].Busy = false
	}
}

// ReleaseAll releases every texture (Close).
func (p *LayerPool[T]) ReleaseAll() {
	for _, l := range p.texs {
		p.Free(l.Tex)
	}
	p.texs = nil
	var zero T
	for i := range p.Tex {
		p.Tex[i] = zero
	}
}

// RescaleComposites converts the layer composites among ins, whose UVs
// and half-texel are normalized by the layer's Size, to the layer's
// texture of size tex (larger when pooled for a larger layer).
func RescaleComposites(ins []shaders.Instance, size, tex image.Point) {
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

// CheckPartial validates a partial frame's Dirty and Copies against its
// target: dirty rectangles non-empty, inside and disjoint; copies of
// equal non-empty sizes inside the target.
func CheckPartial(f *gpu.Frame) error {
	t := image.Rectangle{Max: f.Size}
	for i, r := range f.Dirty {
		if r.Empty() || !r.In(t) {
			return fmt.Errorf("dirty rectangle %v outside the target %v", r, f.Size)
		}
		for _, o := range f.Dirty[:i] {
			if o.Overlaps(r) {
				return fmt.Errorf("dirty rectangles %v and %v overlap", o, r)
			}
		}
	}
	for _, c := range f.Copies {
		if c.Src.Size() != c.Dst.Size() || c.Src.Empty() || !c.Src.In(t) || !c.Dst.In(t) {
			return fmt.Errorf("copy %v → %v in a %v target", c.Src, c.Dst, f.Size)
		}
	}
	return nil
}

// ScratchPlan decides the copy scratch texture for copies, whose sources
// are stacked vertically in it, given the current scratch (have; exists
// false when there is none) and the texture limit. used is false when
// there are no copies (nothing changes). Otherwise create says whether to
// replace the scratch with one of size: when it does not fit, or is more
// than four times the area needed (the window shrank); when growing, the
// larger side of the old one is kept too, so alternating shapes do not
// recreate it every frame. Sources too tall to stack in one texture
// return an error wrapping gpu.ErrNotRetained, so the Renderer draws the
// frame in full (it is not a device failure).
func ScratchPlan(copies []gpu.CopyRegion, have image.Point, exists bool, limit int) (used, create bool, size image.Point, err error) {
	var need image.Point
	for _, c := range copies {
		need.X = max(need.X, c.Src.Dx())
		need.Y += c.Src.Dy()
	}
	if need.X == 0 {
		return false, false, image.Point{}, nil
	}
	fits := exists && need.X <= have.X && need.Y <= have.Y
	if fits && TexBytes(have) <= 4*TexBytes(need) {
		return true, false, have, nil
	}
	if need.X > limit || need.Y > limit {
		return true, false, image.Point{}, fmt.Errorf("copy sources %d×%d exceed one scratch texture (max %d): %w", need.X, need.Y, limit, gpu.ErrNotRetained)
	}
	size = need
	if !fits && exists && TexBytes(have) <= 4*TexBytes(need) {
		size = image.Pt(max(need.X, have.X), max(need.Y, have.Y))
	}
	return true, true, size, nil
}

// ClearInstances are the quads that clear a partial frame's dirty
// rectangles to c, one per rectangle and slightly larger than it (each is
// drawn scissored to its rectangle, without blending). paint.CPU fills
// the target with the 8-bit premultiplied color; c/255 gives exactly
// those bytes (the box coverage of a whole-pixel rectangle is exactly 1
// inside it).
func ClearInstances(dst []shaders.Instance, dirty []image.Rectangle, c color.NRGBA) []shaders.Instance {
	cc := ClearColor(c)
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
