//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/gen"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

// Device executes gpu.Frames with Metal into a persistent offscreen
// MTLPixelFormatRGBA8Unorm texture (premultiplied alpha), recreated when
// Frame.Size changes.
//
// Synchronization. Execute encodes one command buffer per frame and
// commits it without waiting; the next Execute (and ReadPixels, Close)
// first waits for it to complete. So every CPU-side write a frame makes —
// the staging buffer holding its uploads, the instance buffer — happens
// while no command buffer is in flight, and the atlas/patch/ramp textures the
// previous frame sampled are only overwritten after it finished (M1:
// single-buffered; the CPU work of building the next Frame still overlaps
// the GPU work of the last one).
//
// Uploads. All of a frame's upload pixels are copied into one shared
// staging MTLBuffer, then copied into the private textures by blit
// encoders inside the frame's command buffer: Frame.Uploads and the first
// batch's Uploads before the first render pass; a later batch with
// Uploads ends the render pass, blits, and starts a new pass with
// loadAction=load. Metal's resource hazard tracking orders each blit after
// the draws encoded before it, so uploads and draws keep command order.
// A (re)created texture's contents are undefined until uploaded; the
// Renderer re-uploads everything it samples when MaskSize/ColorSize/RampSize
// change.
//
// Images. Each gpu.ImageUpload creates a private RGBA8Unorm texture of its
// own (replacing one of the same ID), filled by the first blit like the
// other uploads, and kept in d.images until Frame.ImageFrees names it
// (freed before the frame's uploads; the previous frame has completed by
// then). Every batch binds its Batch.Image texture to slot TexImage, or a
// 1×1 placeholder when it has none.
//
// Layers. Each gpu.Frame.Layers entry is drawn into a private RGBA8Unorm
// render-target texture taken from d.layers, a pool: before encoding,
// assignLayers walks the batches and gives each layer, at the first batch
// drawing into it, the smallest free texture at least its Size and at
// most twice its Size on each axis (creating one of exactly its Size only
// when none is free), and frees the texture again after the layer's last
// use (the last batch drawing into or sampling it), so a later layer in
// the same frame reuses it. A layer in a larger texture uses its top-left
// Size texels: its passes get the texture's size as ViewSize and its
// composites' UVs are rescaled to the texture (see gpu.Frame.Layers).
// Every texture is free again when the frame is encoded. The pool is
// bounded: creating a texture first releases the free textures no
// earlier pass of this frame used that it covers on both axes (a group
// that grows leaves no trail of smaller textures) and, oldest first, free
// textures while the pool would exceed layerPoolBytes; after each frame
// textures unused for layerIdleFrames frames are released and the pool
// is trimmed to layerPoolBytes again (a single frame may need more: its
// textures are all busy at once). Metal's hazard tracking orders a
// reused texture's new render pass after the passes that sampled it.
// Execute starts a render pass wherever the target changes (or a batch has
// Uploads): a layer's first pass clears it to transparent, the frame
// target's first pass clears it to Frame.Clear, later passes load.
//
// Partial frames (gpu.Frame.Dirty). The target persists between frames,
// so the device is a gpu.Retainer. A partial frame is refused with
// gpu.ErrNotRetained unless the target holds the last frame (not
// recreated, no failure since). Its Copies are encoded first: every source
// is blitted into a scratch texture, then from there to its destination
// (a blit between overlapping regions of one texture is undefined). The
// target's first pass loads instead of clearing and draws one quad per
// dirty rectangle with a pipeline without blending (clearPipeline),
// scissored to the rectangle, to clear it to Frame.Clear; every batch
// drawing into the target is then drawn once per dirty rectangle with its
// scissor intersected with it. Layers are cleared and drawn as always;
// present still copies the whole target.
//
// Failures. An error of a committed command buffer is only known when it
// is waited for (by the next Execute, by ReadPixels or Close). Whoever
// waits records it with gpuFailed: the next Execute returns it (so the
// Renderer re-uploads every texture it samples), and present (Task 9)
// returns it too until then. maxFailures consecutive failed frames with no
// successful one in between, or one error saying the GPU is gone, mark the
// device lost: from then on Execute and present return an error wrapping
// ErrDeviceLost, and the caller should switch to the CPU renderer. A
// frame whose textures exceed Metal's size limit (maxTextureSize) is
// rejected up front with ErrTargetTooLarge and is not a failure: it is
// neither counted nor able to lose the device.
//
// Methods are serialized by a mutex; a Device serves one Renderer.
type Device struct {
	mu sync.Mutex

	dev, queue, pipeline id
	presentPipeline      id // RGBA target → BGRA drawable copy (presenter_darwin.go)

	target, mask, color, ramp                 id
	targetSize, maskSize, colorSize, rampSize image.Point
	images                                    map[int]id // image textures by gpu.ImageUpload.ID
	noImage                                   id         // 1×1 texture bound to TexImage for batches without an image
	instBuf, staging, readBuf                 id
	instCap, stagingCap, readCap              int
	inflight                                  []id // committed, not yet waited command buffers, in commit order
	hasFrame, closed                          bool
	commits                                   int      // frames committed by Execute and present (tests)
	failures                                  int      // consecutive failed frames
	pendingErr                                error    // waited-for GPU error not yet returned by Execute
	lost                                      error    // non-nil (wrapping ErrDeviceLost) once the device is lost
	staged                                    []staged // per-frame scratch

	layers      []layerTex    // layer texture pool (see Layers above)
	layerTex    []id          // per frame: layerTex[n] is layer n's texture (0: none)
	layerSize   []image.Point // per frame: layerSize[n] is the size of layerTex[n]
	layerFrames int           // frames encoded (the pool's clock)

	// Partial frames (gpu.Frame.Dirty; see Device).
	clearPipeline id          // the quad pipeline without blending: clears dirty rectangles
	scratch       id          // gpu.Frame.Copies go through it
	scratchSize   image.Point // size of scratch
	scratchIdle   int         // frames since scratch was last used
}

// scratchIdleFrames is how many frames without Copies the copy scratch
// texture is kept for.
const scratchIdleFrames = 120

// layerTex is a pooled layer texture.
type layerTex struct {
	tex  id
	size image.Point
	busy bool // assigned to a layer whose last use is still to come
	used int  // layerFrames of the last frame that used it
}

// layerIdleFrames is how many frames a pooled layer texture may go unused
// before it is released.
const layerIdleFrames = 120

// layerPoolBytes bounds the bytes of the layer textures the pool keeps
// (beyond what one frame holds at once).
const layerPoolBytes = 128 << 20

// layerTextures returns the number of layer textures the device holds
// (tests).
func (d *Device) layerTextures() int { return len(d.layers) }

// layerBytes returns the bytes of the layer textures the device holds.
func (d *Device) layerBytes() int {
	n := 0
	for _, l := range d.layers {
		n += texBytes(l.size)
	}
	return n
}

func texBytes(size image.Point) int { return size.X * size.Y * 4 }

// staged is an upload copied into the staging buffer.
type staged struct {
	tex      id
	off, bpr uint64
	rect     image.Rectangle
}

// instanceSize is the per-instance vertex buffer stride: the size of
// shaders.Instance, which internal/render/gpu/gen asserts at compile time.
const instanceSize = gen.QuadInstanceStride

// NewDevice creates a device on the system default Metal GPU and compiles
// the quad shader. It fails with ErrNoDevice when there is no Metal device.
func NewDevice() (*Device, error) {
	if err := load(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()

	d := &Device{dev: mtlCreateSystemDefaultDevice()}
	if d.dev == 0 {
		return nil, ErrNoDevice
	}
	d.queue = d.dev.Send(sel("newCommandQueue"))
	if d.queue == 0 {
		d.release()
		return nil, fmt.Errorf("%w: newCommandQueue failed", ErrNoDevice)
	}
	p, err := d.newPipeline(pixelFormatRGBA8Unorm, true)
	if err != nil {
		d.release()
		return nil, err
	}
	d.pipeline = p
	return d, nil
}

// newPipeline compiles gen.QuadMSL and builds the quad pipeline for a
// color attachment of the given pixel format (Task 9 adds BGRA8 for the
// layer): the per-instance attributes of shaders.Instance from buffer 0,
// laid out as gen.QuadInstanceAttrs says (generated with the MSL and
// checked against the Go struct at compile time), premultiplied "over"
// blending (one, oneMinusSourceAlpha), or with blend unset none (the
// fragment replaces the target's pixel).
func (d *Device) newPipeline(format uint64, blend bool) (id, error) {
	opts := newObject("MTLCompileOptions")
	defer release(opts)
	// Fast math (Metal's default) lets the compiler reassociate and fuse
	// the coverage arithmetic; the CPU reference is IEEE float32, so ask
	// for safe math to keep parity at the coverage edges.
	if msgBool1(opts, sel("respondsToSelector:"), sel("setMathMode:")) {
		msgSetU64(opts, sel("setMathMode:"), mathModeSafe)
	} else {
		msgSetBool(opts, sel("setFastMathEnabled:"), false)
	}
	var e id
	lib := msgWithOptErr(d.dev, sel("newLibraryWithSource:options:error:"), nsString(gen.QuadMSL), opts, &e)
	if lib == 0 {
		return 0, nsError("metal: compile quad shader", e)
	}
	defer release(lib)
	vs := lib.Send(sel("newFunctionWithName:"), nsString("vs_main"))
	fs := lib.Send(sel("newFunctionWithName:"), nsString("fs_main"))
	defer release(vs)
	defer release(fs)
	if vs == 0 || fs == 0 {
		return 0, errors.New("metal: quad shader lacks vs_main/fs_main")
	}

	desc := newObject("MTLRenderPipelineDescriptor")
	defer release(desc)
	msgSetID(desc, sel("setVertexFunction:"), vs)
	msgSetID(desc, sel("setFragmentFunction:"), fs)

	vd := class("MTLVertexDescriptor").Send(sel("vertexDescriptor"))
	attrs := vd.Send(sel("attributes"))
	for _, at := range gen.QuadInstanceAttrs {
		format, err := vertexFormat(at.Kind, at.N)
		if err != nil {
			return 0, fmt.Errorf("metal: attribute %d: %w", at.Location, err)
		}
		a := msgIdx(attrs, sel("objectAtIndexedSubscript:"), uint64(at.Location))
		msgSetU64(a, sel("setFormat:"), format)
		msgSetU64(a, sel("setOffset:"), uint64(at.Offset))
		msgSetU64(a, sel("setBufferIndex:"), 0)
	}
	lay := msgIdx(vd.Send(sel("layouts")), sel("objectAtIndexedSubscript:"), 0)
	msgSetU64(lay, sel("setStride:"), instanceSize)
	msgSetU64(lay, sel("setStepFunction:"), stepPerInstance)
	msgSetU64(lay, sel("setStepRate:"), 1)
	msgSetID(desc, sel("setVertexDescriptor:"), vd)

	ca := msgIdx(desc.Send(sel("colorAttachments")), sel("objectAtIndexedSubscript:"), 0)
	msgSetU64(ca, sel("setPixelFormat:"), format)
	msgSetBool(ca, sel("setBlendingEnabled:"), blend)
	msgSetU64(ca, sel("setSourceRGBBlendFactor:"), blendOne)
	msgSetU64(ca, sel("setSourceAlphaBlendFactor:"), blendOne)
	msgSetU64(ca, sel("setDestinationRGBBlendFactor:"), blendOneMinusSourceAlpha)
	msgSetU64(ca, sel("setDestinationAlphaBlendFactor:"), blendOneMinusSourceAlpha)

	e = 0
	p := msgWithErr(d.dev, sel("newRenderPipelineStateWithDescriptor:error:"), desc, &e)
	if p == 0 {
		return 0, nsError("metal: create quad pipeline", e)
	}
	return p, nil
}

// Execute implements gpu.Device. A frame without pixels is skipped without
// submitting anything. An error of the previous frame's command buffer is
// reported by the next call (it is only known once that frame completes).
func (d *Device) Execute(f *gpu.Frame) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.lost != nil {
		return d.lost
	}
	if f.Size.X <= 0 || f.Size.Y <= 0 {
		return nil
	}
	if err := checkTextureSizes(f); err != nil {
		// Not a GPU failure: no failure count, no pendingErr consumed.
		// (Creating such a texture is a Metal validation failure that
		// aborts the process.)
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()

	d.wait()
	if err := d.pendingErr; err != nil {
		d.pendingErr = nil
		if d.lost != nil {
			return d.lost
		}
		return err
	}
	partial := len(f.Dirty) > 0
	if !partial && len(f.Copies) > 0 {
		// Copies belong to partial frames only; executing this as a full
		// frame would clear the target and drop them.
		return fmt.Errorf("metal: frame with Copies but no Dirty")
	}
	d.scratchIdle++
	if partial {
		// The target must still hold the previous frame: not recreated,
		// and the last frame did not fail (gpuFailed clears hasFrame).
		if !d.hasFrame || d.target == 0 || d.targetSize != f.Size {
			return fmt.Errorf("metal: partial frame: %w", gpu.ErrNotRetained)
		}
		if err := checkPartial(f); err != nil {
			return err
		}
		if d.clearPipeline == 0 {
			p, err := d.newPipeline(pixelFormatRGBA8Unorm, false)
			if err != nil {
				return d.syncFailed(err)
			}
			d.clearPipeline = p
		}
		if err := d.ensureScratch(f.Copies); err != nil {
			if errors.Is(err, gpu.ErrNotRetained) {
				return err // too large to stage: the Renderer draws the frame in full; not a failure
			}
			return d.syncFailed(err)
		}
	}
	if d.scratch != 0 && d.scratchIdle > scratchIdleFrames {
		release(d.scratch) // no copies for a while: do not keep a window-sized texture
		d.scratch, d.scratchSize = 0, image.Point{}
	}
	if err := d.ensureTextures(f); err != nil {
		return d.syncFailed(err)
	}
	if err := d.applyImages(f); err != nil {
		return d.syncFailed(err)
	}
	if err := d.stage(f); err != nil {
		return err
	}
	if err := d.assignLayers(f); err != nil {
		return err
	}
	nInst := 0
	for i := range f.Batches {
		nInst += len(f.Batches[i].Instances)
	}
	nClear := 0
	if partial {
		nClear = len(f.Dirty) // one clearing quad per dirty rectangle, after the batches'
	}
	if err := ensureBuffer(d.dev, &d.instBuf, &d.instCap, (nInst+nClear)*instanceSize, storageModeShared|cpuCacheModeWC); err != nil {
		return d.syncFailed(err)
	}
	if nClear > 0 {
		dst := unsafe.Slice((*shaders.Instance)(msgPtr(d.instBuf, sel("contents"))), nInst+nClear)[nInst:]
		// paint.CPU fills the target with the 8-bit premultiplied clear
		// color; c/255 converts back to exactly those bytes (the box
		// coverage of a whole-pixel rectangle is exactly 1 inside it).
		c := color.RGBAModel.Convert(f.Clear).(color.RGBA)
		col := sl.V4(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
		for k, r := range f.Dirty {
			dst[k] = shaders.Instance{
				Rect:  sl.V4(float32(r.Min.X-1), float32(r.Min.Y-1), float32(r.Dx()+2), float32(r.Dy()+2)), // scissored to r
				Color: col,
				Kind:  sl.V4(shaders.KindSolid, 0, 0, 0),
			}
		}
	}
	if nInst > 0 {
		dst := unsafe.Slice((*shaders.Instance)(msgPtr(d.instBuf, sel("contents"))), nInst)
		k := 0
		for i := range f.Batches {
			b := &f.Batches[i]
			n := copy(dst[k:], b.Instances)
			if b.Layer != 0 {
				d.rescaleComposites(dst[k:k+n], f.Layers[b.Layer-1].Size, d.layerSize[b.Layer])
			}
			k += n
		}
	}

	cb := d.queue.Send(sel("commandBuffer"))
	if cb == 0 {
		return d.syncFailed(errors.New("metal: commandBuffer failed"))
	}
	if partial {
		d.copyRegions(cb, f.Copies) // before anything is drawn
	}
	ups := d.staged
	first := len(f.ImageUploads) + len(f.Uploads)
	if len(f.Batches) > 0 {
		first += len(f.Batches[0].Uploads)
	}
	d.blit(cb, ups[:first])
	ups = ups[first:]
	var enc id
	cur := -1                                // the open pass's target
	started := make([]bool, len(f.Layers)+1) // targets cleared already
	base := uint64(0)
	for i := range f.Batches {
		b := &f.Batches[i]
		if i > 0 && len(b.Uploads) > 0 {
			if enc != 0 {
				enc.Send(sel("endEncoding"))
				enc = 0
			}
			d.blit(cb, ups[:len(b.Uploads)])
			ups = ups[len(b.Uploads):]
		}
		if enc == 0 || b.Target != cur {
			if enc != 0 {
				enc.Send(sel("endEncoding"))
			}
			first := !started[b.Target]
			enc, cur = d.beginPass(cb, f, b.Target, first && !(partial && b.Target == 0)), b.Target
			if first && partial && b.Target == 0 {
				d.clearDirty(enc, f, uint64(nInst)*instanceSize)
			}
			started[b.Target] = true
		}
		n := uint64(len(b.Instances))
		off := base
		base += n * instanceSize
		// The scissor in the target's texels.
		full, origin := image.Rectangle{Max: f.Size}, image.Point{}
		if b.Target != 0 {
			ld := f.Layers[b.Target-1]
			full, origin = image.Rectangle{Max: ld.Size}, ld.Bounds.Min
		}
		sc := full
		if !b.Scissor.Empty() {
			sc = b.Scissor.Sub(origin).Intersect(full)
		}
		if n == 0 || sc.Empty() {
			continue
		}
		setScissorRect(enc, mtlScissorRect{uint64(sc.Min.X), uint64(sc.Min.Y), uint64(sc.Dx()), uint64(sc.Dy())})
		img := d.noImage
		if b.Image != 0 {
			img = d.images[b.Image] // present: checked by applyImages
		}
		msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), img, gpu.TexImage)
		layer := d.noImage
		if b.Layer != 0 {
			layer = d.layerTex[b.Layer] // drawn by an earlier batch: checked by assignLayers
		}
		msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), layer, gpu.TexLayer)
		msgBufOffIdx(enc, sel("setVertexBuffer:offset:atIndex:"), d.instBuf, off, 0)
		if partial && b.Target == 0 {
			// Drawn once per dirty rectangle (disjoint: no pixel twice).
			for _, r := range f.Dirty {
				s := sc.Intersect(r)
				if s.Empty() {
					continue
				}
				setScissorRect(enc, mtlScissorRect{uint64(s.Min.X), uint64(s.Min.Y), uint64(s.Dx()), uint64(s.Dy())})
				msgDraw(enc, sel("drawPrimitives:vertexStart:vertexCount:instanceCount:"), primitiveTriangleStrip, 0, 4, n)
			}
			continue
		}
		msgDraw(enc, sel("drawPrimitives:vertexStart:vertexCount:instanceCount:"), primitiveTriangleStrip, 0, 4, n)
	}
	if enc != 0 {
		enc.Send(sel("endEncoding"))
	}
	if !started[0] {
		// No batch drew into the target: it is still cleared (in a
		// partial frame, its dirty rectangles).
		enc := d.beginPass(cb, f, 0, !partial)
		if partial {
			d.clearDirty(enc, f, uint64(nInst)*instanceSize)
		}
		enc.Send(sel("endEncoding"))
	}
	d.commit(cb)
	d.commits++
	d.hasFrame = true
	clear(d.staged)
	return nil
}

// beginPass starts a render pass on target (0: the frame's target, n:
// layer n), clearing it (to f.Clear, or transparent for a layer) or
// keeping its contents, with the pipeline, the FrameU uniform for that
// target and the textures bound.
func (d *Device) beginPass(cb id, f *gpu.Frame, target int, clearTarget bool) id {
	tex := d.target
	frame := shaders.Frame{ViewSize: sl.V2(float32(f.Size.X), float32(f.Size.Y))} // uniform FrameU
	if target != 0 {
		ld := f.Layers[target-1]
		tex = d.layerTex[target]
		ts := d.layerSize[target] // ≥ ld.Size: the layer is its top-left part
		frame = shaders.Frame{ViewSize: sl.V2(float32(ts.X), float32(ts.Y)), Origin: sl.V2(float32(ld.Bounds.Min.X), float32(ld.Bounds.Min.Y))}
	}
	rp := class("MTLRenderPassDescriptor").Send(sel("renderPassDescriptor"))
	ca := msgIdx(rp.Send(sel("colorAttachments")), sel("objectAtIndexedSubscript:"), 0)
	msgSetID(ca, sel("setTexture:"), tex)
	msgSetU64(ca, sel("setStoreAction:"), storeActionStore)
	if clearTarget && target != 0 {
		msgSetU64(ca, sel("setLoadAction:"), loadActionClear)
		msgClearColor(ca, sel("setClearColor:"), mtlClearColor{})
	} else if clearTarget {
		// paint.CPU fills the target with the 8-bit premultiplied clear
		// color; c/255 converts back to exactly those bytes.
		c := color.RGBAModel.Convert(f.Clear).(color.RGBA)
		msgSetU64(ca, sel("setLoadAction:"), loadActionClear)
		msgClearColor(ca, sel("setClearColor:"), mtlClearColor{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255})
	} else {
		msgSetU64(ca, sel("setLoadAction:"), loadActionLoad)
	}
	enc := cb.Send(sel("renderCommandEncoderWithDescriptor:"), rp)
	msgSetID(enc, sel("setRenderPipelineState:"), d.pipeline)
	msgBytesLenIdx(enc, sel("setVertexBytes:length:atIndex:"), unsafe.Pointer(&frame), uint64(unsafe.Sizeof(frame)), frameBuffer)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.mask, gpu.TexMask)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.color, gpu.TexColor)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.ramp, gpu.TexRamp)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.noImage, gpu.TexImage)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.noImage, gpu.TexLayer)
	return enc
}

// clearDirty clears the dirty rectangles of a partial frame in the target
// pass enc: the clearing quads at instance buffer offset off (one per
// rectangle, slightly larger than it) drawn with the non-blending pipeline,
// each scissored to its rectangle. It leaves the quad pipeline bound.
func (d *Device) clearDirty(enc id, f *gpu.Frame, off uint64) {
	msgSetID(enc, sel("setRenderPipelineState:"), d.clearPipeline)
	for k, r := range f.Dirty {
		setScissorRect(enc, mtlScissorRect{uint64(r.Min.X), uint64(r.Min.Y), uint64(r.Dx()), uint64(r.Dy())})
		msgBufOffIdx(enc, sel("setVertexBuffer:offset:atIndex:"), d.instBuf, off+uint64(k)*instanceSize, 0)
		msgDraw(enc, sel("drawPrimitives:vertexStart:vertexCount:instanceCount:"), primitiveTriangleStrip, 0, 4, 1)
	}
	msgSetID(enc, sel("setRenderPipelineState:"), d.pipeline)
}

// checkPartial validates a partial frame's Dirty and Copies against its
// target (a Metal validation failure would abort the process).
func checkPartial(f *gpu.Frame) error {
	t := image.Rectangle{Max: f.Size}
	for i, r := range f.Dirty {
		if r.Empty() || !r.In(t) {
			return fmt.Errorf("metal: dirty rectangle %v outside the target %v", r, f.Size)
		}
		for _, o := range f.Dirty[:i] {
			if o.Overlaps(r) {
				return fmt.Errorf("metal: dirty rectangles %v and %v overlap", o, r)
			}
		}
	}
	for _, c := range f.Copies {
		if c.Src.Size() != c.Dst.Size() || c.Src.Empty() || !c.Src.In(t) || !c.Dst.In(t) {
			return fmt.Errorf("metal: copy %v → %v in a %v target", c.Src, c.Dst, f.Size)
		}
	}
	return nil
}

// ensureScratch makes d.scratch large enough to hold the sources of
// copies stacked vertically. A scratch more than four times the area
// needed is replaced by one of the needed size (the window shrank), and
// one unused for scratchIdleFrames frames is released by Execute. Sources
// too tall to stack in one texture return an error wrapping
// gpu.ErrNotRetained, so the Renderer draws the frame in full instead (it
// is not a device failure).
func (d *Device) ensureScratch(copies []gpu.CopyRegion) error {
	var need image.Point
	for _, c := range copies {
		need.X = max(need.X, c.Src.Dx())
		need.Y += c.Src.Dy()
	}
	if need.X == 0 {
		return nil
	}
	d.scratchIdle = 0
	fits := d.scratch != 0 && need.X <= d.scratchSize.X && need.Y <= d.scratchSize.Y
	if fits && texBytes(d.scratchSize) <= 4*texBytes(need) {
		return nil
	}
	if need.X > maxTextureSize || need.Y > maxTextureSize {
		return fmt.Errorf("metal: copy sources %d×%d exceed one scratch texture (max %d): %w", need.X, need.Y, maxTextureSize, gpu.ErrNotRetained)
	}
	size := need
	if !fits && d.scratch != 0 && texBytes(d.scratchSize) <= 4*texBytes(need) {
		// Growing: keep the larger side too, so alternating shapes do not
		// recreate it every frame.
		size = image.Pt(max(need.X, d.scratchSize.X), max(need.Y, d.scratchSize.Y))
	}
	t, err := d.newTexture(size.X, size.Y, pixelFormatRGBA8Unorm, usageShaderRead)
	if err != nil {
		return err
	}
	release(d.scratch)
	d.scratch, d.scratchSize = t, size
	return nil
}

// scratchBytes returns the bytes of the copy scratch texture (tests).
func (d *Device) scratchBytes() int {
	if d.scratch == 0 {
		return 0
	}
	return texBytes(d.scratchSize)
}

// copyRegions encodes a partial frame's copies within the target. A blit
// between overlapping regions of one texture is undefined in Metal, so
// every source is first copied into the scratch texture (stacked
// vertically) by one blit encoder, then from there to its destination by a
// second; Metal's hazard tracking orders the second after the first, and
// both before the render passes that follow.
func (d *Device) copyRegions(cb id, copies []gpu.CopyRegion) {
	if len(copies) == 0 {
		return
	}
	for pass := range 2 {
		enc := cb.Send(sel("blitCommandEncoder"))
		y := 0
		for _, c := range copies {
			size := mtlSize{uint64(c.Src.Dx()), uint64(c.Src.Dy()), 1}
			at := mtlOrigin{0, uint64(y), 0}
			if pass == 0 {
				copyTextureToTexture(enc, d.target, mtlOrigin{uint64(c.Src.Min.X), uint64(c.Src.Min.Y), 0}, size, d.scratch, at)
			} else {
				copyTextureToTexture(enc, d.scratch, at, size, d.target, mtlOrigin{uint64(c.Dst.Min.X), uint64(c.Dst.Min.Y), 0})
			}
			y += c.Src.Dy()
		}
		enc.Send(sel("endEncoding"))
	}
}

// Retained implements gpu.Retainer: the target holds the last frame
// encoded at size, and no frame failed since (a failure the device only
// learns of when it waits makes the next Execute return it, which makes
// the Renderer draw the frame after that in full).
func (d *Device) Retained(size image.Point) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && d.lost == nil && d.hasFrame && d.target != 0 && d.targetSize == size
}

// assignLayers checks the frame's use of layers (every batch's Target and
// Layer name one of f.Layers, a batch samples only a layer an earlier
// batch drew into, never its own target) and gives each layer drawn into a
// texture from the pool (see Device), filling d.layerTex. It runs before
// anything is encoded, so a texture that cannot be created fails the frame
// cleanly. Unused pooled textures are released after layerIdleFrames.
func (d *Device) assignLayers(f *gpu.Frame) error {
	d.layerFrames++
	d.trimLayers(0) // last frame's textures may go now (its command buffer holds them)
	clear(d.layerTex)
	d.layerTex = append(d.layerTex[:0], make([]id, len(f.Layers)+1)...)
	d.layerSize = append(d.layerSize[:0], make([]image.Point, len(f.Layers)+1)...)
	if len(f.Layers) > 0 {
		last := make([]int, len(f.Layers)+1)
		for i := range f.Batches {
			b := &f.Batches[i]
			for _, n := range [2]int{b.Target, b.Layer} {
				if n < 0 || n > len(f.Layers) {
					return fmt.Errorf("metal: batch %d names layer %d of %d", i, n, len(f.Layers))
				}
				last[n] = i
			}
			if b.Target != 0 && b.Target == b.Layer {
				return fmt.Errorf("metal: batch %d samples the layer it draws into", i)
			}
		}
		for i := range f.Batches {
			b := &f.Batches[i]
			if n := b.Target; n != 0 && d.layerTex[n] == 0 {
				ld := f.Layers[n-1]
				if ld.Size.X <= 0 || ld.Size.Y <= 0 || ld.Bounds.Dx() > ld.Size.X || ld.Bounds.Dy() > ld.Size.Y {
					d.freeLayers()
					return fmt.Errorf("metal: layer %d of size %v for bounds %v", n, ld.Size, ld.Bounds)
				}
				t, size, err := d.takeLayer(ld.Size)
				if err != nil {
					d.freeLayers()
					return d.syncFailed(err)
				}
				d.layerTex[n], d.layerSize[n] = t, size
			}
			if n := b.Layer; n != 0 && d.layerTex[n] == 0 {
				d.freeLayers()
				return fmt.Errorf("metal: batch %d samples layer %d before any batch drew into it", i, n)
			}
			for _, n := range [2]int{b.Target, b.Layer} {
				if n != 0 && last[n] == i {
					d.putLayer(d.layerTex[n])
				}
			}
		}
	}
	// Release textures idle for too long (all are free now), then trim
	// the pool to its byte budget.
	d.releaseLayers(func(l *layerTex) bool { return d.layerFrames-l.used > layerIdleFrames })
	d.trimLayers(0)
	return nil
}

// takeLayer returns the smallest free pooled layer texture of at least
// size and at most twice size on each axis, and its size, creating one of
// size when there is none (see Device: releasing superseded textures and
// keeping the pool's bytes bounded first), and marks it busy.
func (d *Device) takeLayer(size image.Point) (id, image.Point, error) {
	best := -1
	for i := range d.layers {
		l := &d.layers[i]
		if l.busy || l.size.X < size.X || l.size.Y < size.Y || l.size.X > 2*size.X || l.size.Y > 2*size.Y {
			continue
		}
		if best < 0 || texBytes(l.size) < texBytes(d.layers[best].size) {
			best = i
		}
	}
	if best >= 0 {
		l := &d.layers[best]
		l.busy, l.used = true, d.layerFrames
		return l.tex, l.size, nil
	}
	d.releaseLayers(func(l *layerTex) bool {
		return l.used < d.layerFrames && l.size.X <= size.X && l.size.Y <= size.Y
	})
	d.trimLayers(texBytes(size))
	t, err := d.newTexture(size.X, size.Y, pixelFormatRGBA8Unorm, usageRenderTarget|usageShaderRead)
	if err != nil {
		return 0, image.Point{}, err
	}
	d.layers = append(d.layers, layerTex{tex: t, size: size, busy: true, used: d.layerFrames})
	return t, size, nil
}

// releaseLayers releases the free pooled textures drop selects.
func (d *Device) releaseLayers(drop func(l *layerTex) bool) {
	keep := d.layers[:0]
	for i := range d.layers {
		if l := d.layers[i]; !l.busy && drop(&l) {
			release(l.tex)
			continue
		}
		keep = append(keep, d.layers[i])
	}
	clear(d.layers[len(keep):])
	d.layers = keep
}

// trimLayers releases free pooled textures, least recently used first,
// while the pool's bytes plus extra exceed layerPoolBytes. Textures this
// frame uses are kept (encoding still binds them), so after a frame that
// needs more than layerPoolBytes at once the pool is trimmed when the next
// frame starts.
func (d *Device) trimLayers(extra int) {
	total := d.layerBytes() + extra
	for total > layerPoolBytes {
		old := -1
		for i := range d.layers {
			if l := &d.layers[i]; !l.busy && l.used < d.layerFrames && (old < 0 || l.used < d.layers[old].used) {
				old = i
			}
		}
		if old < 0 {
			return // all used by this frame
		}
		total -= texBytes(d.layers[old].size)
		release(d.layers[old].tex)
		d.layers = slices.Delete(d.layers, old, old+1)
	}
}

// putLayer marks the pooled texture t free.
func (d *Device) putLayer(t id) {
	for i := range d.layers {
		if d.layers[i].tex == t {
			d.layers[i].busy = false
			return
		}
	}
}

// freeLayers marks every pooled texture free (a frame that is not
// encoded after all).
func (d *Device) freeLayers() {
	for i := range d.layers {
		d.layers[i].busy = false
	}
}

// rescaleComposites converts the layer composites among ins, whose UVs
// and half-texel are normalized by the layer's Size, to the layer's
// texture of size tex (larger when pooled for a larger layer).
func (d *Device) rescaleComposites(ins []shaders.Instance, size, tex image.Point) {
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

// blit copies staged uploads into their textures, in order.
func (d *Device) blit(cb id, ups []staged) {
	if len(ups) == 0 {
		return
	}
	enc := cb.Send(sel("blitCommandEncoder"))
	for _, u := range ups {
		w, h := uint64(u.rect.Dx()), uint64(u.rect.Dy())
		if w == 0 || h == 0 {
			continue
		}
		copyBufferToTexture(enc, d.staging, u.off, u.bpr, u.bpr*h, mtlSize{w, h, 1}, u.tex, mtlOrigin{uint64(u.rect.Min.X), uint64(u.rect.Min.Y), 0})
	}
	enc.Send(sel("endEncoding"))
}

// applyImages releases the textures of f.ImageFrees, creates one for each
// of f.ImageUploads (validated; the pixels are staged by stage) and checks
// that every batch's Image is held.
func (d *Device) applyImages(f *gpu.Frame) error {
	if d.images == nil {
		d.images = map[int]id{}
	}
	for _, i := range f.ImageFrees {
		release(d.images[i])
		delete(d.images, i)
	}
	if d.noImage == 0 {
		t, err := d.newTexture(1, 1, pixelFormatRGBA8Unorm, usageShaderRead)
		if err != nil {
			return err
		}
		d.noImage = t
	}
	for _, u := range f.ImageUploads {
		w, h := u.Size.X, u.Size.Y
		if u.ID <= 0 || w <= 0 || h <= 0 {
			return fmt.Errorf("metal: image upload %d of size %v", u.ID, u.Size)
		}
		if len(u.Pix) < (h-1)*u.Stride+4*w {
			return fmt.Errorf("metal: image upload %d: %d bytes with stride %d for %v", u.ID, len(u.Pix), u.Stride, u.Size)
		}
		t, err := d.newTexture(w, h, pixelFormatRGBA8Unorm, usageShaderRead)
		if err != nil {
			return err
		}
		release(d.images[u.ID])
		d.images[u.ID] = t
	}
	for i := range f.Batches {
		if b := f.Batches[i].Image; b != 0 && d.images[b] == 0 {
			return fmt.Errorf("metal: batch %d binds image %d, which the device does not hold", i, b)
		}
	}
	return nil
}

// newTexture creates a private w×h texture.
func (d *Device) newTexture(w, h int, format, usage uint64) (id, error) {
	desc := msgTexDesc(class("MTLTextureDescriptor"), sel("texture2DDescriptorWithPixelFormat:width:height:mipmapped:"), format, uint64(w), uint64(h), false)
	msgSetU64(desc, sel("setUsage:"), usage)
	msgSetU64(desc, sel("setStorageMode:"), textureStoragePrivate)
	t := d.dev.Send(sel("newTextureWithDescriptor:"), desc)
	if t == 0 {
		return 0, fmt.Errorf("metal: cannot create %d×%d texture", w, h)
	}
	return t, nil
}

// stage validates every upload of f and copies its pixels into the staging
// buffer, filling d.staged in command order (Frame.ImageUploads, whose
// textures applyImages created, then Frame.Uploads, then each batch's),
// one entry per upload so the per-batch counts line up; empty uploads get
// an entry that blit skips. Rows are packed tightly, each upload starting
// 16-byte aligned.
func (d *Device) stage(f *gpu.Frame) error {
	d.staged = d.staged[:0]
	total := 0
	for _, u := range f.ImageUploads {
		total = align16(total)
		d.staged = append(d.staged, staged{tex: d.images[u.ID], off: uint64(total), bpr: uint64(4 * u.Size.X), rect: image.Rectangle{Max: u.Size}})
		total += 4 * u.Size.X * u.Size.Y
	}
	lists := 1 + len(f.Batches)
	list := func(i int) []gpu.Upload {
		if i == 0 {
			return f.Uploads
		}
		return f.Batches[i-1].Uploads
	}
	for li := range lists {
		for _, u := range list(li) {
			bpp, tex, size, err := d.uploadTarget(u)
			if err != nil {
				return err
			}
			w, h := u.Rect.Dx(), u.Rect.Dy()
			if w > 0 && h > 0 && len(u.Pix) < (h-1)*u.Stride+w*bpp {
				return fmt.Errorf("metal: upload %v: %d bytes with stride %d", u.Rect, len(u.Pix), u.Stride)
			}
			if !u.Rect.In(image.Rectangle{Max: size}) {
				return fmt.Errorf("metal: upload %v outside texture %v", u.Rect, size)
			}
			total = align16(total)
			d.staged = append(d.staged, staged{tex: tex, off: uint64(total), bpr: uint64(w * bpp), rect: u.Rect})
			total += w * h * bpp
		}
	}
	if total == 0 {
		return nil
	}
	if err := ensureBuffer(d.dev, &d.staging, &d.stagingCap, total, storageModeShared|cpuCacheModeWC); err != nil {
		return err
	}
	buf := unsafe.Slice((*byte)(msgPtr(d.staging, sel("contents"))), total)
	k := 0
	for _, u := range f.ImageUploads {
		s := d.staged[k]
		k++
		row := int(s.bpr)
		for y := range u.Size.Y {
			copy(buf[int(s.off)+y*row:][:row], u.Pix[y*u.Stride:][:row])
		}
	}
	for li := range lists {
		for _, u := range list(li) {
			s := d.staged[k]
			k++
			row := int(s.bpr)
			for y := range u.Rect.Dy() {
				copy(buf[int(s.off)+y*row:][:row], u.Pix[y*u.Stride:][:row])
			}
		}
	}
	return nil
}

func (d *Device) uploadTarget(u gpu.Upload) (bpp int, tex id, size image.Point, err error) {
	switch u.Tex {
	case gpu.TexMask:
		return 1, d.mask, d.maskSize, nil
	case gpu.TexColor:
		return 4, d.color, d.colorSize, nil
	case gpu.TexRamp:
		return 8, d.ramp, d.rampSize, nil
	}
	return 0, 0, image.Point{}, fmt.Errorf("metal: upload to unknown texture %d", u.Tex)
}

func align16(n int) int { return (n + 15) &^ 15 }

// maxTextureSize is Metal's largest 2D texture side on every Mac GPU
// family (Apple2+ and Mac2).
const maxTextureSize = 16384

// checkTextureSizes returns an error wrapping ErrTargetTooLarge when a
// texture of f (the render target, the mask atlas, the patch page, the
// ramp texture) is
// larger than maxTextureSize on a side.
func checkTextureSizes(f *gpu.Frame) error {
	for _, t := range []struct {
		what string
		size image.Point
	}{{"render target", f.Size}, {"mask texture", f.MaskSize}, {"color texture", f.ColorSize}, {"ramp texture", f.RampSize}} {
		if t.size.X > maxTextureSize || t.size.Y > maxTextureSize {
			return fmt.Errorf("%w: %s %d×%d (max %d)", ErrTargetTooLarge, t.what, t.size.X, t.size.Y, maxTextureSize)
		}
	}
	for _, u := range f.ImageUploads {
		if u.Size.X > maxTextureSize || u.Size.Y > maxTextureSize {
			return fmt.Errorf("%w: image texture %d×%d (max %d)", ErrTargetTooLarge, u.Size.X, u.Size.Y, maxTextureSize)
		}
	}
	for _, l := range f.Layers {
		if l.Size.X > maxTextureSize || l.Size.Y > maxTextureSize {
			return fmt.Errorf("%w: layer texture %d×%d (max %d)", ErrTargetTooLarge, l.Size.X, l.Size.Y, maxTextureSize)
		}
	}
	return nil
}

// ensureTextures (re)creates the render target and the three sampled
// textures (mask atlas, patch page, gradient ramps) at the frame's sizes. A zero-sized texture is created as 1×1
// so something is always bound.
func (d *Device) ensureTextures(f *gpu.Frame) error {
	type texSpec struct {
		tex    *id
		have   *image.Point
		want   image.Point
		format uint64
		usage  uint64
	}
	for _, s := range []texSpec{
		{&d.target, &d.targetSize, f.Size, pixelFormatRGBA8Unorm, usageRenderTarget | usageShaderRead}, // read by the present pass
		{&d.mask, &d.maskSize, f.MaskSize, pixelFormatR8Unorm, usageShaderRead},
		{&d.color, &d.colorSize, f.ColorSize, pixelFormatRGBA8Unorm, usageShaderRead},
		{&d.ramp, &d.rampSize, f.RampSize, pixelFormatRGBA16Unorm, usageShaderRead},
	} {
		if *s.tex != 0 && *s.have == s.want {
			continue
		}
		release(*s.tex)
		*s.tex, *s.have = 0, image.Point{}
		if s.tex == &d.target {
			d.hasFrame = false // the new target's contents are undefined until a frame completes
		}
		t, err := d.newTexture(max(1, s.want.X), max(1, s.want.Y), s.format, s.usage)
		if err != nil {
			return err
		}
		*s.tex, *s.have = t, s.want
	}
	return nil
}

// frameBuffer is the vertex buffer index of the FrameU uniform: MSL
// buffer(N) of //sl:uniform N (0, the instance buffer, comes first).
const frameBuffer = 1

// vertexFormat returns the MTLVertexFormat of an attribute of n
// components of kind ("f32", "i32", "u32").
func vertexFormat(kind string, n int) (uint64, error) {
	base := map[string]uint64{"f32": vertexFormatFloat, "i32": vertexFormatInt, "u32": vertexFormatUInt}
	b, ok := base[kind]
	if !ok || n < 1 || n > 4 {
		return 0, fmt.Errorf("no vertex format for %d × %s", n, kind)
	}
	return b + uint64(n-1), nil // Float, Float2, Float3, Float4 are consecutive (Int…, UInt… too)
}

// ensureBuffer grows *buf to at least n bytes (n > 0).
func ensureBuffer(dev id, buf *id, capacity *int, n int, opts uint64) error {
	if n <= *capacity && *buf != 0 {
		return nil
	}
	if n == 0 {
		return nil
	}
	c := max(n, *capacity*3/2, 4096)
	release(*buf)
	*buf, *capacity = 0, 0
	b := msgNewBuffer(dev, sel("newBufferWithLength:options:"), uint64(c), opts)
	if b == 0 {
		return fmt.Errorf("metal: cannot allocate a %d-byte buffer", c)
	}
	*buf, *capacity = b, c
	return nil
}

// commit commits cb and keeps it (retained) until wait.
func (d *Device) commit(cb id) {
	cb.Send(sel("commit"))
	d.inflight = append(d.inflight, retain(cb))
}

// wait blocks until every committed command buffer completes and returns
// the GPU time of the last one. A failed buffer is recorded (gpuFailed)
// and its error returned; when all succeeded the failure count resets.
func (d *Device) wait() (time.Duration, error) {
	if len(d.inflight) == 0 {
		return 0, nil
	}
	var g time.Duration
	var first error
	for i, cb := range d.inflight {
		cb.Send(sel("waitUntilCompleted"))
		if st := msgU64(cb, sel("status")); st != cbStatusCompleted {
			e := cb.Send(sel("error"))
			err := nsError(fmt.Sprintf("metal: command buffer status %d", st), e)
			if first == nil {
				first = err
				d.gpuFailed(err, cbErrorLost(e))
			}
		} else {
			g = time.Duration((msgF64(cb, sel("GPUEndTime")) - msgF64(cb, sel("GPUStartTime"))) * float64(time.Second))
		}
		release(cb)
		d.inflight[i] = 0
	}
	d.inflight = d.inflight[:0]
	if first != nil {
		return 0, first
	}
	d.failures = 0
	return g, nil
}

// maxFailures consecutive failed frames mark the device lost.
const maxFailures = 3

// gpuFailed records a failed command buffer: the target holds no frame,
// the next Execute (and present until then) returns err, and the device
// is lost after maxFailures failures in a row or when removed is set.
func (d *Device) gpuFailed(err error, removed bool) {
	d.hasFrame = false
	d.pendingErr = err
	d.failures++
	if d.lost == nil && (removed || d.failures >= maxFailures) {
		d.lost = fmt.Errorf("%w: %w", ErrDeviceLost, err)
	}
}

// syncFailed counts a failure to encode a frame (no command buffer or
// texture/buffer allocation failed) and returns err, or the loss error
// once the device counts as lost.
func (d *Device) syncFailed(err error) error {
	d.failures++
	if d.lost == nil && d.failures >= maxFailures {
		d.lost = fmt.Errorf("%w: %w", ErrDeviceLost, err)
	}
	if d.lost != nil {
		return d.lost
	}
	return err
}

// MTLCommandBufferError codes that mean the GPU will not come back.
const (
	cbErrorAccessRevoked = 4  // MTLCommandBufferErrorAccessRevoked (the app lost GPU access)
	cbErrorDeviceRemoved = 11 // MTLCommandBufferErrorDeviceRemoved (eGPU unplugged)
)

// cbErrorLost reports whether a command buffer's NSError means the device
// is gone.
func cbErrorLost(e id) bool {
	if e == 0 {
		return false
	}
	switch msgU64(e, sel("code")) {
	case cbErrorAccessRevoked, cbErrorDeviceRemoved:
		return true
	}
	return false
}

// ReadPixels waits for the last frame and returns a copy of the render
// target: RGBA8, premultiplied alpha, Frame.Size of that frame.
func (d *Device) ReadPixels() (*image.RGBA, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if !d.hasFrame {
		return nil, errors.New("metal: ReadPixels before any frame")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	if _, err := d.wait(); err != nil {
		return nil, err // kept for the next Execute by gpuFailed
	}
	w, h := d.targetSize.X, d.targetSize.Y
	n := w * h * 4
	if err := ensureBuffer(d.dev, &d.readBuf, &d.readCap, n, storageModeShared); err != nil {
		return nil, err
	}
	cb := d.queue.Send(sel("commandBuffer"))
	if cb == 0 {
		return nil, errors.New("metal: commandBuffer failed")
	}
	enc := cb.Send(sel("blitCommandEncoder"))
	copyTextureToBuffer(enc, d.target, mtlOrigin{}, mtlSize{uint64(w), uint64(h), 1}, d.readBuf, 0, uint64(w*4), uint64(n))
	enc.Send(sel("endEncoding"))
	d.commit(cb)
	if _, err := d.wait(); err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, unsafe.Slice((*byte)(msgPtr(d.readBuf, sel("contents"))), n))
	return img, nil
}

// Close waits for outstanding work and releases every Metal object. It is
// safe to call more than once.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	d.wait()
	d.release()
}

func (d *Device) release() {
	for k, t := range d.images {
		release(t)
		delete(d.images, k)
	}
	for _, l := range d.layers {
		release(l.tex)
	}
	d.layers = nil
	clear(d.layerTex)
	for _, o := range []*id{&d.target, &d.mask, &d.color, &d.ramp, &d.noImage, &d.scratch, &d.instBuf, &d.staging, &d.readBuf, &d.pipeline, &d.clearPipeline, &d.presentPipeline, &d.queue, &d.dev} {
		release(*o)
		*o = 0
	}
}

var _ gpu.Device = (*Device)(nil)
