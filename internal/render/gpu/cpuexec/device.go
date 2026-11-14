package cpuexec

import (
	"fmt"
	"image"
	"image/draw"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/racetest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// maxImageSide is the largest image texture side a device accepts. It
// must equal package gpu's maxImageSide (internal/render/gpu/images.go), which is
// unexported: keep the two in step by hand when either changes.
const maxImageSide = 16384

// Device is a gpu.Device that executes frames on the CPU with Draw,
// keeping its target and textures across frames like a GPU device would.
// It implements gpu.Retainer and gpu.PixelReader, so a gpu.Renderer sends
// it partial frames (Frame.Dirty, Frame.Copies) and can run in verify mode
// (Renderer.SetVerify) on it: tests of anything that emits redraw hints
// can check them without a GPU. It also checks the frame contract
// (gpu.Frame) and returns an error for a frame that breaks it.
//
// A Device is not safe for concurrent use, and Execute must not run
// concurrently with any other Draw or Run (see Run).
type Device struct {
	// NoRetain makes Retained report false, so a Renderer draws every
	// frame in full.
	NoRetain bool
	// SampledOnly binds to each batch only the textures its instances
	// sample (see sampledTextures), which leaves every pixel the same and
	// saves converting the whole glyph atlas and patch page for every
	// batch; a GPU binds them all. It is always on under the race
	// detector.
	SampledOnly bool

	calls, executed, partial int
	img                      *image.RGBA
	mask                     *image.Alpha
	color                    *image.RGBA
	ramp                     *image.RGBA64
	// images are the image textures by ID (ImageUploads, ImageFrees).
	images map[int]*image.RGBA
	last   *gpu.Frame // shallow copy of the last executed frame
	// batchUploads counts uploads applied between batches (patch page
	// flushes) over all frames.
	batchUploads int
	// liveLayers is the number of layer images still held after the last
	// Execute (each is dropped after its last use: always 0).
	liveLayers int
}

// NewDevice returns a Device holding no frame.
func NewDevice() *Device { return &Device{} }

// Stats are a Device's counters.
type Stats struct {
	Calls    int // Execute calls
	Executed int // frames executed successfully
	Partial  int // partial frames (Frame.Dirty) among them
	// BatchUploads counts the uploads applied between batches (patch page
	// flushes), over all frames.
	BatchUploads int
	// LiveLayers is the number of layer images still held after the last
	// Execute (each is dropped after its last use: always 0).
	LiveLayers int
}

// Stats returns the device's counters.
func (d *Device) Stats() Stats {
	return Stats{Calls: d.calls, Executed: d.executed, Partial: d.partial, BatchUploads: d.batchUploads, LiveLayers: d.liveLayers}
}

// Target returns the frame target itself (not a copy; nil before the
// first frame with pixels). It is valid until the next Execute.
func (d *Device) Target() *image.RGBA { return d.img }

// Textures returns the mask, patch page (color) and ramp textures
// themselves (nil before the first frame).
func (d *Device) Textures() (mask *image.Alpha, color *image.RGBA, ramp *image.RGBA64) {
	return d.mask, d.color, d.ramp
}

// Images returns the image textures the device holds, by ID (the map
// itself).
func (d *Device) Images() map[int]*image.RGBA { return d.images }

// Last returns a shallow copy of the last frame executed successfully
// (its slices of Dirty, Copies, Batches, ImageUploads and ImageFrees
// copied), nil before the first.
func (d *Device) Last() *gpu.Frame { return d.last }

// Execute implements gpu.Device.
func (d *Device) Execute(f *gpu.Frame) error {
	d.calls++
	if d.mask == nil || d.mask.Rect.Size() != f.MaskSize {
		d.mask = image.NewAlpha(image.Rectangle{Max: f.MaskSize})
	}
	if d.color == nil || d.color.Rect.Size() != f.ColorSize {
		d.color = image.NewRGBA(image.Rectangle{Max: f.ColorSize})
	}
	if d.ramp == nil || d.ramp.Rect.Size() != f.RampSize {
		d.ramp = image.NewRGBA64(image.Rectangle{Max: f.RampSize})
	}
	for _, id := range f.ImageFrees {
		delete(d.images, id)
	}
	for _, u := range f.ImageUploads {
		if u.ID <= 0 || u.Size.X <= 0 || u.Size.Y <= 0 || u.Size.X > maxImageSide || u.Size.Y > maxImageSide {
			return fmt.Errorf("bad image upload %d %v", u.ID, u.Size)
		}
		if len(u.Pix) < (u.Size.Y-1)*u.Stride+4*u.Size.X {
			return fmt.Errorf("image upload %d: %d bytes", u.ID, len(u.Pix))
		}
		m := image.NewRGBA(image.Rectangle{Max: u.Size})
		for y := range u.Size.Y {
			copy(m.Pix[y*m.Stride:][:4*u.Size.X], u.Pix[y*u.Stride:])
		}
		if d.images == nil {
			d.images = map[int]*image.RGBA{}
		}
		d.images[u.ID] = m
	}
	if err := d.upload(f, f.Uploads); err != nil {
		return err
	}
	if len(f.Dirty) > 0 {
		// Partial frame: keep the target, apply the copies, clear the
		// dirty rectangles; batches into the target draw only inside them.
		if d.img == nil || d.img.Rect.Size() != f.Size {
			return fmt.Errorf("partial frame: %w", gpu.ErrNotRetained)
		}
		for _, c := range f.Copies {
			if c.Src.Size() != c.Dst.Size() || !c.Src.In(d.img.Rect) || !c.Dst.In(d.img.Rect) {
				return fmt.Errorf("bad copy %v", c)
			}
			CopyRect(d.img, c.Src, c.Dst.Min)
		}
		for i, r := range f.Dirty {
			if !r.In(d.img.Rect) {
				return fmt.Errorf("dirty %v outside the target", r)
			}
			for _, o := range f.Dirty[:i] {
				if o.Overlaps(r) {
					return fmt.Errorf("dirty rectangles %v and %v overlap", o, r)
				}
			}
			Fill(d.img, r, f.Clear)
		}
	} else {
		if len(f.Copies) > 0 {
			return fmt.Errorf("copies in a full frame")
		}
		d.img = image.NewRGBA(image.Rectangle{Max: f.Size})
		draw.Draw(d.img, d.img.Rect, image.NewUniform(f.Clear), image.Point{}, draw.Src)
	}
	// Layers: created transparent at the first batch drawing into them,
	// dropped after their last use (drawn into or sampled).
	last := make([]int, len(f.Layers)+1)
	for i, b := range f.Batches {
		for _, n := range []int{b.Target, b.Layer} {
			if n < 0 || n > len(f.Layers) {
				return fmt.Errorf("batch %d: layer %d of %d", i, n, len(f.Layers))
			}
			last[n] = i
		}
		if b.Target != 0 && b.Target == b.Layer {
			return fmt.Errorf("batch %d samples its own target", i)
		}
	}
	layers := make([]*image.RGBA, len(f.Layers)+1)
	d.liveLayers = 0
	for i, b := range f.Batches {
		if err := d.upload(f, b.Uploads); err != nil {
			return err
		}
		d.batchUploads += len(b.Uploads)
		target, origin := d.img, image.Point{}
		if b.Target != 0 {
			ld := f.Layers[b.Target-1]
			if !ld.Bounds.Size().In(image.Rectangle{Max: ld.Size.Add(image.Pt(1, 1))}) {
				return fmt.Errorf("layer %d: bounds %v larger than size %v", b.Target, ld.Bounds, ld.Size)
			}
			if layers[b.Target] == nil {
				layers[b.Target] = image.NewRGBA(image.Rectangle{Max: ld.Size})
				d.liveLayers++
			}
			target, origin = layers[b.Target], ld.Bounds.Min
		}
		var layer image.Image
		if b.Layer != 0 {
			if layers[b.Layer] == nil {
				return fmt.Errorf("batch %d samples layer %d before any batch drew into it", i, b.Layer)
			}
			layer = layers[b.Layer]
		}
		sc := target.Rect
		if !b.Scissor.Empty() {
			sc = b.Scissor.Sub(origin).Intersect(target.Rect)
		}
		release := func() {
			for _, n := range []int{b.Target, b.Layer} {
				if n != 0 && last[n] == i && layers[n] != nil {
					layers[n] = nil
					d.liveLayers--
				}
			}
		}
		if sc.Empty() {
			release()
			continue
		}
		var img image.Image
		if b.Image != 0 {
			m, ok := d.images[b.Image]
			if !ok {
				return fmt.Errorf("batch binds image %d the device does not hold", b.Image)
			}
			img = m
		}
		for _, in := range b.Instances {
			if in.Kind.X == shaders.KindImage && b.Image == 0 {
				return fmt.Errorf("KindImage instance in a batch without an image")
			}
			if in.Kind.X == shaders.KindPatch && in.Kind.W > 0.5 && b.Layer == 0 {
				return fmt.Errorf("layer composite in a batch without a layer")
			}
		}
		scs := []image.Rectangle{sc}
		if b.Target == 0 && len(f.Dirty) > 0 {
			scs = scs[:0]
			for _, r := range f.Dirty {
				scs = append(scs, sc.Intersect(r))
			}
		}
		tex := Textures{Mask: d.mask, Color: d.color, Ramp: d.ramp, Img: img, Layer: layer}
		if d.SampledOnly || racetest.Enabled {
			tex = sampledTextures(b.Instances, tex) // see sampledTextures
		}
		for _, sc := range scs {
			if sc.Empty() {
				continue
			}
			dst := target.SubImage(sc).(*image.RGBA)
			Draw(b.Instances, Target{Size: target.Rect.Size(), Origin: origin}, tex, dst)
		}
		release()
	}
	d.executed++
	if len(f.Dirty) > 0 {
		d.partial++
	}
	cp := *f
	cp.Dirty = append([]image.Rectangle(nil), f.Dirty...)
	cp.Copies = append([]gpu.CopyRegion(nil), f.Copies...)
	cp.Batches = append([]gpu.Batch(nil), f.Batches...)
	cp.ImageUploads = append([]gpu.ImageUpload(nil), f.ImageUploads...)
	cp.ImageFrees = append([]int(nil), f.ImageFrees...)
	d.last = &cp
	return nil
}

// Retained implements gpu.Retainer: the target persists between frames
// (unless NoRetain is set).
func (d *Device) Retained(size image.Point) bool {
	return !d.NoRetain && d.img != nil && d.img.Rect.Size() == size
}

// ReadPixels implements gpu.PixelReader: a copy of the target.
func (d *Device) ReadPixels() (*image.RGBA, error) {
	if d.img == nil {
		return nil, fmt.Errorf("no frame")
	}
	m := image.NewRGBA(d.img.Rect)
	copy(m.Pix, d.img.Pix)
	return m, nil
}

func (d *Device) upload(f *gpu.Frame, ups []gpu.Upload) error {
	for _, u := range ups {
		bpp, pix, stride := 1, d.mask.Pix, d.mask.Stride
		tex := image.Rectangle{Max: f.MaskSize}
		switch u.Tex {
		case gpu.TexMask:
		case gpu.TexColor:
			bpp, pix, stride = 4, d.color.Pix, d.color.Stride
			tex.Max = f.ColorSize
		case gpu.TexRamp:
			// Little-endian uint16 channels into big-endian RGBA64.
			if !u.Rect.In(image.Rectangle{Max: f.RampSize}) {
				return fmt.Errorf("upload %v outside texture %v", u.Rect, f.RampSize)
			}
			for y := 0; y < u.Rect.Dy(); y++ {
				src := u.Pix[y*u.Stride:][:u.Rect.Dx()*8]
				dst := d.ramp.Pix[d.ramp.PixOffset(u.Rect.Min.X, u.Rect.Min.Y+y):]
				for k := 0; k < len(src); k += 2 {
					dst[k], dst[k+1] = src[k+1], src[k]
				}
			}
			continue
		default:
			return fmt.Errorf("bad texture %d", u.Tex)
		}
		if !u.Rect.In(tex) {
			return fmt.Errorf("upload %v outside texture %v", u.Rect, tex)
		}
		n := u.Rect.Dx() * bpp
		for y := 0; y < u.Rect.Dy(); y++ {
			copy(pix[(u.Rect.Min.Y+y)*stride+u.Rect.Min.X*bpp:][:n], u.Pix[y*u.Stride:][:n])
		}
	}
	return nil
}

// sampledTextures returns tex with only the textures some instance of
// insts samples (shaders.FS: Mask for KindMask and a path-shaped
// KindGradient, Ramp for KindGradient, Color for KindPatch, Layer for a
// layer composite, Img for KindImage); the others are nil.
//
// Device uses it with SampledOnly and always under -race
// (racetest.Enabled): Draw converts every bound texture to floats on each
// call, which for the 2048² mask atlas and patch page dominates the
// tests' time, the more so under the race detector. An unsampled texture
// does not change a pixel, so the output is the same; otherwise every
// texture stays bound, as on a GPU. Keep the
// conditions in step with shaders.FS (a texture FS samples for another
// kind would read as transparent here under -race only).
func sampledTextures(insts []shaders.Instance, tex Textures) Textures {
	var mask, color, ramp, img, layer bool
	for _, in := range insts {
		k, w := in.Kind.X, in.Kind.W
		switch {
		case k > shaders.KindChecker+0.5:
			img = true
		case k > shaders.KindGradient+0.5: // KindChecker
		case k > shaders.KindShadow+0.5: // KindGradient
			ramp = true
			mask = mask || w > 0.5
		case k > shaders.KindPatch+0.5, k < shaders.KindSolid+0.5: // analytic, KindSolid
		case k < shaders.KindMask+0.5:
			mask = true
		case w > 0.5: // layer composite
			layer = true
		default: // KindPatch
			color = true
		}
	}
	if !mask {
		tex.Mask = nil
	}
	if !color {
		tex.Color = nil
	}
	if !ramp {
		tex.Ramp = nil
	}
	if !img {
		tex.Img = nil
	}
	if !layer {
		tex.Layer = nil
	}
	return tex
}
