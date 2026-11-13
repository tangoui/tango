//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/gen"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/shader/sl"
)

// Device executes gpu.Frames with Direct3D 11 into a persistent offscreen
// R8G8B8A8_UNORM render target (premultiplied alpha), recreated when
// Frame.Size changes. It follows ui/gpu/metal's Device (the reference for
// every behaviour below); the differences come from Direct3D 11 itself.
//
// Synchronization. Everything goes through the device's immediate
// context, whose commands the GPU executes in the order they are issued;
// Execute issues a frame and flushes it to the GPU without waiting (the
// CPU work of the next frame overlaps the GPU work of this one, as with
// Metal's commit), and ReadPixels waits by mapping a staging copy of the
// target. The runtime tracks resource hazards on the immediate context:
// a resource written by the CPU (UpdateSubresource, a discarding Map)
// while earlier commands still read it is renamed or waited for by the
// driver, and a released resource lives on until the commands using it
// ran.
//
// Uploads. Each upload is an UpdateSubresource into its texture, issued in
// command order: Frame.Uploads, then for every batch its Uploads followed
// by its draw. UpdateSubresource copies the pixels before returning and
// the immediate context orders the copy after the draws issued before it,
// so a later batch's upload never changes what an earlier batch sampled
// (Metal gets this with blits encoded between render passes). A
// (re)created texture's contents are undefined until uploaded; the
// Renderer re-uploads everything it samples when
// MaskSize/ColorSize/RampSize change.
//
// Instances. All of a frame's instances are written with one Map into a
// dynamic vertex buffer used as a ring (instRing): after the previous
// frame's with D3D11_MAP_WRITE_NO_OVERWRITE while they fit, otherwise
// from the start with D3D11_MAP_WRITE_DISCARD. Each batch sets the vertex
// buffer at its first instance and draws a 4-vertex triangle strip per
// instance with DrawInstanced(4, n, 0, 0), scissored to its Scissor
// (empty: the whole target), with the viewport its target's size.
//
// Images. Each gpu.ImageUpload creates an immutable R8G8B8A8_UNORM
// texture from its pixels (replacing one of the same ID), kept in
// d.images until Frame.ImageFrees names it (freed first). Every batch
// binds its Batch.Image texture to slot TexImage, or a 1×1 placeholder.
//
// Layers. Each gpu.Frame.Layers entry is drawn into a render-target
// texture from a pool (layerPool, layers.go: Metal's assignLayers with
// the same reuse rule, 128 MiB budget and 120-frame idle release). A
// layer's first bind clears it to transparent; switching targets first
// unbinds the image and layer slots, so a pooled texture is never bound
// as a shader resource and a render target at once. Batches drawing into
// a layer run with ViewSize its texture's size and Origin its
// Bounds.Min; composites sampling a larger pooled texture get their UVs
// rescaled (rescaleComposites).
//
// Partial frames (gpu.Frame.Dirty). The target persists between frames,
// so the device is a gpu.Retainer. A partial frame is refused with
// gpu.ErrNotRetained unless the target holds the last frame (not
// recreated, the device not lost). Its Copies go first: every source is
// copied into a scratch texture with CopySubresourceRegion, then from
// there to its destination (overlapping regions of one resource are
// undefined). The target is not cleared: each dirty rectangle gets one
// quad drawn without blending, scissored to it, in Frame.Clear; every
// batch drawing into the target is then drawn once per dirty rectangle
// with its scissor intersected with it. Layers are cleared and drawn as
// always.
//
// Failures. Errors are known synchronously: an HRESULT from creating a
// texture or buffer or from Map, or a removed reason the device reports
// after the frame is flushed. Every step that can fail runs before the
// first command of the frame is issued. Each failed frame is counted
// (health); maxFailures in a row with no successful frame in between, a
// call returning DXGI_ERROR_DEVICE_REMOVED, _RESET, _HUNG or
// _DRIVER_INTERNAL_ERROR, or any removed reason (also asked for after
// any other failed call) mark the device lost: from then on every call
// returns an error wrapping gpu.ErrDeviceLost, and the caller should
// switch to the CPU renderer. A frame whose textures exceed the device's
// size limit (16384 from feature level 11_0 on, 8192 below), or whose
// instances exceed the instance buffer, is rejected up front with
// gpu.ErrTargetTooLarge, as an invalid frame is with a plain error:
// neither is a failure, counted, or able to lose the device.
//
// Methods are serialized by a mutex (the immediate context is not
// thread-safe); a Device serves one Renderer.
type Device struct {
	mu sync.Mutex

	dev, ctx     uintptr // ID3D11Device, its immediate ID3D11DeviceContext
	featureLevel uint32
	maxTex       int // largest texture side

	pipe        *pipeline
	inst        instBuffer
	target      texture    // the frame's render target (RTV)
	tex         [3]texture // gpu.TexMask, gpu.TexColor, gpu.TexRamp (SRV)
	placeholder texture    // 1×1 transparent texture bound to gpu.TexImage and gpu.TexLayer when unused
	readback    texture    // staging copy of the target for ReadPixels
	images      map[int]texture
	layers      *layerPool[texture]
	scratch     texture // gpu.Frame.Copies go through it
	scratchIdle int     // frames since scratch was last used

	hasFrame bool // the target holds the last frame executed (ReadPixels may read it)
	closed   bool
	health   health
	commits  int // frames flushed to the GPU (tests)

	// The last successful frame, for the presenter's dirty-rectangle hint.
	frameSeq  int               // successful frames executed
	lastDirty []image.Rectangle // pixels it changed when partial (Dirty and the Copies' Dst)
	lastFull  bool              // it was a full frame
	lastClear color.NRGBA       // its Frame.Clear (the presenter's background color)

	// Per-frame scratch.
	gather  instGather // the frame's instances, in instance buffer order
	started []bool     // targets bound (cleared) already
}

// injectHR, set by tests, replaces the HRESULT of the named call (such as
// "Map", "CreateTexture2D", "CreateBuffer" or "GetDeviceRemovedReason")
// with the value it returns when that is not 0.
var injectHR func(call string) uintptr

// checkHR returns hr, or the HRESULT injectHR substitutes for call.
func checkHR(call string, hr uintptr) uintptr {
	if injectHR != nil {
		if r := injectHR(call); r != 0 {
			return r
		}
	}
	return hr
}

// NewDevice creates a Direct3D 11 device (a hardware adapter, else WARP)
// and compiles the quad shader. It fails with an error wrapping
// ErrNoDevice when there is no device or no d3dcompiler_47.dll; the
// caller then uses the CPU renderer (it should for any error). When the
// pipeline cannot be built on a hardware adapter of feature level 10_x
// (shader model 4), the device is created on WARP instead.
//
// With the environment variable TANGO_D3D_DEBUG=1 the device is created
// with the Direct3D debug layer when it is installed (Windows' optional
// "Graphics Tools" feature; without it the device is created normally)
// and shader compiler warnings are logged.
func NewDevice() (*Device, error) {
	dev, ctx, fl, err := createDevice(false)
	if err != nil {
		return nil, err
	}
	warp := isWARP(dev)
	d, err := newDeviceOn(dev, ctx, fl)
	if err == nil || warp || fl >= featureLevel11_0 || errors.Is(err, ErrNoDevice) {
		return d, err
	}
	dev, ctx, fl, werr := createDevice(true)
	if werr != nil {
		return nil, fmt.Errorf("%w (WARP fallback: %v)", err, werr)
	}
	d, werr = newDeviceOn(dev, ctx, fl)
	if werr != nil {
		return nil, fmt.Errorf("%w (WARP fallback: %v)", err, werr)
	}
	return d, nil
}

// newDeviceOn builds the device on a created D3D11 device and context,
// which it owns from then on (released on error).
func newDeviceOn(dev, ctx uintptr, fl uint32) (*Device, error) {
	d := &Device{dev: dev, ctx: ctx, featureLevel: fl, maxTex: maxTextureSizeFor(fl)}
	d.layers = d.newLayerPool()
	var err error
	if d.pipe, err = newPipeline(dev, fl); err != nil {
		d.release()
		return nil, err
	}
	// Transparent black: what a batch binding no image or layer gets
	// (nothing samples it).
	if d.placeholder, err = newTexture(dev, image.Pt(1, 1), colorSpec, make([]byte, 4), 4); err != nil {
		d.release()
		return nil, err
	}
	return d, nil
}

// Execute implements gpu.Device. A frame without pixels is skipped without
// issuing anything.
func (d *Device) Execute(f *gpu.Frame) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.health.lost != nil {
		return d.health.lost
	}
	if f.Size.X <= 0 || f.Size.Y <= 0 {
		return nil
	}
	// Invalid or oversized frames: rejected before anything is allocated
	// or issued, and not failures.
	if err := checkTextureSizes(f, d.maxTex); err != nil {
		return err
	}
	nInst, err := checkFrame(f)
	if err != nil {
		return err
	}
	d.scratchIdle++
	partial := len(f.Dirty) > 0
	if partial {
		if !d.hasFrame || d.target.tex == 0 || d.target.size != f.Size {
			return fmt.Errorf("d3d11: partial frame: %w", gpu.ErrNotRetained)
		}
		if err := checkPartial(f); err != nil {
			return err
		}
		if err := d.ensureScratch(f.Copies); err != nil {
			if isAllocError(err) {
				return d.failed(err)
			}
			return err // too large to stage (ErrNotRetained): drawn in full; not a failure
		}
	}
	if d.scratch.tex != 0 && d.scratchIdle > scratchIdleFrames {
		d.scratch.release() // no copies for a while: do not keep a window-sized texture
	}
	// As in Metal, the textures are (re)created before the uploads are
	// validated: a frame that recreates the target and then turns out
	// invalid leaves hasFrame false.
	if err := d.ensureTextures(f); err != nil {
		return d.failed(err)
	}
	if err := d.checkImages(f); err != nil {
		return err
	}
	if err := d.checkUploads(f); err != nil {
		return err
	}
	if err := d.applyImages(f); err != nil {
		return d.failed(err)
	}
	if err := d.layers.assign(f); err != nil {
		if isAllocError(err) {
			return d.failed(err)
		}
		return err
	}

	// Instances last: the final step that can fail, before anything
	// touches a target.
	instOff := 0
	if nInst > 0 {
		ins := d.gather.gather(f, d.layers.Size)
		off, err := d.inst.write(d.dev, d.ctx, ins, nInst)
		d.gather.reset() // keep no reference to the Frame
		if err != nil {
			return d.failed(err)
		}
		instOff = off
	}
	d.encode(f, instOff, nInst)
	comCall(d.ctx, ctxFlush)
	d.commits++
	if err := d.removed(); err != nil {
		d.hasFrame = false
		return d.health.lose(err)
	}
	d.hasFrame = true
	d.health.succeeded()
	d.frameSeq++
	d.lastFull = len(f.Dirty) == 0
	d.lastClear = f.Clear
	d.lastDirty = append(d.lastDirty[:0], f.Dirty...)
	for _, c := range f.Copies {
		d.lastDirty = append(d.lastDirty, c.Dst)
	}
	return nil
}

// encode issues the frame's commands (see Device): copies, the target's
// clear (or its dirty rectangles' clearing quads), the uploads and the
// batches, each into its target. Nothing here can fail.
func (d *Device) encode(f *gpu.Frame, instOff, nInst int) {
	ctx := d.ctx
	partial := len(f.Dirty) > 0
	d.pipe.bind(ctx)
	srvs := [5]uintptr{
		gpu.TexMask:  d.tex[gpu.TexMask].srv,
		gpu.TexColor: d.tex[gpu.TexColor].srv,
		gpu.TexRamp:  d.tex[gpu.TexRamp].srv,
		gpu.TexImage: d.placeholder.srv,
		gpu.TexLayer: d.placeholder.srv,
	}
	comCall(ctx, ctxPSSetShaderResources, 0, uintptr(len(srvs)), uintptr(unsafe.Pointer(&srvs[0])))
	if partial {
		d.copyRegions(f.Copies) // before anything is drawn
	}
	clear(d.started)
	d.started = append(d.started[:0], make([]bool, len(f.Layers)+1)...)

	stride := uint32(gen.QuadInstanceStride)
	buf := d.inst.buf
	setVB := func(off int) {
		o := uint32(off)
		// IASetVertexBuffers(StartSlot, NumBuffers, ppVertexBuffers,
		// pStrides, pOffsets)
		comCall(ctx, ctxIASetVertexBuffers, 0, 1, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&stride)), uintptr(unsafe.Pointer(&o)))
	}
	scissor := func(r rect) {
		comCall(ctx, ctxRSSetScissorRects, 1, uintptr(unsafe.Pointer(&r)))
	}
	draw := func(n int) {
		// DrawInstanced(VertexCountPerInstance, InstanceCount,
		// StartVertexLocation, StartInstanceLocation)
		comCall(ctx, ctxDrawInstanced, 4, uintptr(n), 0, 0)
	}

	// The frame's target first: cleared, or in a partial frame its dirty
	// rectangles cleared by quads drawn without blending.
	d.bindTarget(f, 0)
	d.started[0] = true
	if !partial {
		cc := clearColor(f.Clear)
		// ClearRenderTargetView(pRenderTargetView, ColorRGBA[4]): the
		// whole view, regardless of the scissor.
		comCall(ctx, ctxClearRenderTargetView, d.target.rtv, uintptr(unsafe.Pointer(&cc)))
	} else {
		clearOff := instOff + (nInst-len(f.Dirty))*gen.QuadInstanceStride
		d.pipe.setBlend(ctx, false)
		for k, r := range f.Dirty {
			scissor(rect{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)})
			setVB(clearOff + k*gen.QuadInstanceStride)
			draw(1)
		}
		d.pipe.setBlend(ctx, true)
	}
	cur := 0

	d.upload(f.Uploads)
	base := instOff
	for i := range f.Batches {
		b := &f.Batches[i]
		d.upload(b.Uploads) // after every earlier batch's draws
		n := len(b.Instances)
		off := base
		base += n * gen.QuadInstanceStride
		if b.Target != cur {
			d.bindTarget(f, b.Target)
			cur = b.Target
			if !d.started[cur] {
				// A layer is transparent before the first batch drawing
				// into it.
				var transparent [4]float32
				comCall(ctx, ctxClearRenderTargetView, d.layers.Tex[cur].rtv, uintptr(unsafe.Pointer(&transparent)))
				d.started[cur] = true
			}
		}
		full, origin := image.Rectangle{Max: f.Size}, image.Point{}
		if b.Target != 0 {
			ld := f.Layers[b.Target-1]
			full, origin = image.Rectangle{Max: ld.Size}, ld.Bounds.Min
		}
		sc, ok := scissorFor(b.Scissor, origin, full)
		if n == 0 || !ok {
			continue
		}
		views := [2]uintptr{d.placeholder.srv, d.placeholder.srv}
		if b.Image != 0 {
			views[0] = d.images[b.Image].srv // held: checked by checkImages
		}
		if b.Layer != 0 {
			views[1] = d.layers.Tex[b.Layer].srv // drawn by an earlier batch: checked by assign
		}
		comCall(ctx, ctxPSSetShaderResources, gpu.TexImage, 2, uintptr(unsafe.Pointer(&views[0])))
		setVB(off)
		if partial && b.Target == 0 {
			// Drawn once per dirty rectangle (disjoint: no pixel twice).
			full := image.Rect(int(sc.Left), int(sc.Top), int(sc.Right), int(sc.Bottom))
			for _, r := range f.Dirty {
				s := full.Intersect(r)
				if s.Empty() {
					continue
				}
				scissor(rect{Left: int32(s.Min.X), Top: int32(s.Min.Y), Right: int32(s.Max.X), Bottom: int32(s.Max.Y)})
				draw(n)
			}
			continue
		}
		scissor(sc)
		draw(n)
	}
}

// bindTarget makes target (0: the frame's target, n: layer n) the render
// target, with the viewport and FrameU for it. The image and layer slots
// are reset to the placeholder first, so no texture is bound as a shader
// resource while it becomes the render target (a pooled layer texture
// may have been sampled by an earlier batch under another layer number).
func (d *Device) bindTarget(f *gpu.Frame, target int) {
	ctx := d.ctx
	none := [2]uintptr{d.placeholder.srv, d.placeholder.srv}
	comCall(ctx, ctxPSSetShaderResources, gpu.TexImage, 2, uintptr(unsafe.Pointer(&none[0])))
	rtv := d.target.rtv
	size := f.Size
	fu := shaders.Frame{ViewSize: sl.V2(float32(size.X), float32(size.Y))}
	if target != 0 {
		ld := f.Layers[target-1]
		rtv = d.layers.Tex[target].rtv
		size = d.layers.Size[target] // ≥ ld.Size: the layer is its top-left part
		fu = shaders.Frame{ViewSize: sl.V2(float32(size.X), float32(size.Y)), Origin: sl.V2(float32(ld.Bounds.Min.X), float32(ld.Bounds.Min.Y))}
	}
	// OMSetRenderTargets(NumViews, ppRenderTargetViews, pDepthStencilView)
	comCall(ctx, ctxOMSetRenderTargets, 1, uintptr(unsafe.Pointer(&rtv)), 0)
	vp := viewport{Width: float32(size.X), Height: float32(size.Y), MinDepth: 0, MaxDepth: 1}
	comCall(ctx, ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))
	d.pipe.setFrameU(ctx, fu)
}

// failed counts a failed frame (see health) and returns the error to
// report. A device-lost HRESULT loses the device; after any other failed
// call the device is asked for a removed reason, and one loses it too
// (a failed creation is often the first sign of a removed device).
func (d *Device) failed(err error) error {
	if isDeviceLost(err) {
		d.hasFrame = false
		return d.health.fail(err)
	}
	var he *hresultError
	if errors.As(err, &he) {
		if rerr := d.removed(); rerr != nil {
			d.hasFrame = false
			return d.health.lose(fmt.Errorf("%w (after %w)", rerr, err))
		}
	}
	return d.health.fail(err)
}

// removed returns an error when the device reports a removed reason (it
// was removed, reset or hung, or a call was invalid enough to remove it).
func (d *Device) removed() error {
	hr := checkHR("GetDeviceRemovedReason", comCall(d.dev, devGetDeviceRemovedReason))
	return hrError("device removed: GetDeviceRemovedReason", hr)
}

// ReadPixels waits for the last frame and returns a copy of the render
// target: RGBA8, premultiplied alpha, Frame.Size of that frame. It fails
// before the first frame with pixels and after a failed frame that
// recreated the target.
func (d *Device) ReadPixels() (*image.RGBA, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if d.health.lost != nil {
		return nil, d.health.lost
	}
	if !d.hasFrame || d.target.tex == 0 {
		return nil, errors.New("d3d11: ReadPixels before any frame")
	}
	size := d.target.size
	if d.readback.tex == 0 || d.readback.size != size {
		d.readback.release()
		t, err := newTexture(d.dev, size, readbackSpec, nil, 0)
		if err != nil {
			return nil, d.readFailed(err)
		}
		d.readback = t
	}
	// CopyResource(pDstResource, pSrcResource): same size and format.
	comCall(d.ctx, ctxCopyResource, d.readback.tex, d.target.tex)
	var m mappedSubresource
	// Map with D3D11_MAP_READ waits for the copy (and so the frame).
	hr := checkHR("Map", comCall(d.ctx, ctxMap, d.readback.tex, 0, mapRead, 0, uintptr(unsafe.Pointer(&m))))
	if err := hrError("Map(readback)", hr); err != nil {
		if m.Data != 0 {
			comCall(d.ctx, ctxUnmap, d.readback.tex, 0) // mapped for real; the failure was injected
		}
		return nil, d.readFailed(err)
	}
	w, h := size.X, size.Y
	row := w * 4
	if m.Data == 0 || int(m.RowPitch) < row {
		comCall(d.ctx, ctxUnmap, d.readback.tex, 0)
		return nil, fmt.Errorf("d3d11: Map(readback) returned row pitch %d for %d-pixel rows", m.RowPitch, w)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		src := unsafe.Slice((*byte)(foreign(m.Data+uintptr(y)*uintptr(m.RowPitch))), row)
		copy(img.Pix[y*img.Stride:][:row], src)
	}
	comCall(d.ctx, ctxUnmap, d.readback.tex, 0)
	return img, nil
}

// readFailed returns the error of a failed read-back; one saying the
// device is gone loses it (the frame itself is not counted again).
func (d *Device) readFailed(err error) error {
	if isDeviceLost(err) {
		d.hasFrame = false
		return d.health.lose(err)
	}
	return err
}

// Close releases every Direct3D object. It is safe to call more than once.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	d.release()
}

// release unbinds everything from the context and drops every reference
// the device holds.
func (d *Device) release() {
	if d.ctx != 0 {
		// Unbind the views and buffers first so releasing them frees them.
		comCall(d.ctx, ctxClearState)
		comCall(d.ctx, ctxFlush)
	}
	d.readback.release()
	d.scratch.release()
	for id, t := range d.images {
		t.release()
		delete(d.images, id)
	}
	if d.layers != nil {
		d.layers.ReleaseAll()
	}
	d.target.release()
	for i := range d.tex {
		d.tex[i].release()
	}
	d.placeholder.release()
	d.inst.release()
	d.pipe.release()
	d.pipe = nil
	release(d.ctx)
	release(d.dev)
	d.ctx, d.dev = 0, 0
	d.hasFrame = false
}

var (
	_ gpu.Device      = (*Device)(nil)
	_ gpu.PixelReader = (*Device)(nil)
	_ gpu.Retainer    = (*Device)(nil)
)
