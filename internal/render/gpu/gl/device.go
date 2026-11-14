//go:build linux || darwin

package gl

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/gen"
	"github.com/tangoui/tango/internal/render/gpu/internal/devkit"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

// Device executes gpu.Frames with OpenGL 3.3 core into a persistent
// offscreen GL_RGBA8 texture attached to a framebuffer object
// (premultiplied alpha), recreated when Frame.Size changes. It follows
// internal/render/gpu/metal's Device (the reference for every behaviour below); the
// differences come from OpenGL itself. The target stores row 0 at the top
// (the quad vertex shader flips y), so glViewport, glScissor and
// glReadPixels take the frame contract's top-left coordinates unchanged.
//
// Threads. GL calls run only on the thread the device's context is
// current on: every method hands its GL work to the run function the
// device was built with — for NewDevice the goroutine, locked to its OS
// thread, of the device's own headless context; for a presenter its
// caller's thread with the context made current. Every GL object belongs
// to the device (nothing GL lives in package variables), so a lost
// context is replaced by a new context and a new device.
//
// Synchronization. GL executes a context's commands in the order they are
// issued. Execute issues a frame and calls glFlush without waiting (the
// CPU work of the next frame overlaps the GPU work of this one, as with
// Metal's commit); ReadPixels waits through glReadPixels. A buffer or
// texture the CPU writes while earlier commands still read it is renamed
// or waited for by the driver, and a deleted object lives on until the
// commands using it ran.
//
// Uploads. Each upload is a glTexSubImage2D into its texture, issued in
// command order: Frame.Uploads, then for every batch its Uploads followed
// by its draw, so a later batch's upload never changes what an earlier
// batch sampled (Metal gets this with blits between render passes). Rows
// are GL_UNPACK_ROW_LENGTH = Stride/bpp texels apart (GL_UNPACK_ALIGNMENT
// 1). A (re)created texture's contents are undefined until uploaded; the
// Renderer re-uploads everything it samples when
// MaskSize/ColorSize/RampSize change.
//
// Instances. All of a frame's instances are written into one array
// buffer, orphaned first with glBufferData(nil) (fresh storage while the
// previous frame's draws may still read the old) and filled with
// glBufferSubData, one call per batch. Each batch points the vertex
// attributes (divisor 1) at its first instance and draws a 4-vertex
// triangle strip per instance with glDrawArraysInstanced, scissored to
// its Scissor (empty: the whole target), with the viewport its target's
// size, blending with glBlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA). A
// partial frame's clearing quads follow the batches' instances.
//
// Images. Each gpu.ImageUpload creates a GL_RGBA8 texture of its own from
// its pixels (replacing one of the same ID), kept in d.images until
// Frame.ImageFrees names it (deleted first; GL keeps a deleted texture
// until the draws already issued that sample it have run). Every batch
// binds its Batch.Image texture to unit TexImage, or a 1×1 transparent
// placeholder.
//
// Layers. Each gpu.Frame.Layers entry is drawn into a GL_RGBA8 texture
// with a framebuffer of its own, from a pool (layerPool, layers.go:
// Metal's assignLayers with the same reuse rule, 128 MiB budget and
// 120-frame idle release). A layer's first bind clears it to
// transparent. Switching framebuffers first binds the placeholder to the
// image and layer units, so a pooled texture is never bound for sampling
// while it is the render target (a feedback loop, undefined in GL; a
// pooled texture may have been sampled by an earlier batch under another
// layer number), and GL orders the new draws after the ones that sampled
// it. Batches drawing into a layer run with ViewSize its texture's size
// and Origin its Bounds.Min; composites sampling a larger pooled texture
// get their UVs rescaled (devkit.RescaleComposites). The frame's target
// is never bound for sampling.
//
// Backdrop grabs (gpu.Batch.Grab) are a glBlitFramebuffer (GL_NEAREST,
// equal sizes) from the source target's framebuffer into the grab layer's,
// with the image and layer units on the placeholder (grab, device.go).
//
// Partial frames (gpu.Frame.Dirty). The target persists between frames,
// so the device is a gpu.Retainer. A partial frame is refused with
// gpu.ErrNotRetained unless the target holds the last frame (not
// recreated, no failure since that may have drawn into it, the device
// not lost). Its Copies go first: every source is blitted
// (glBlitFramebuffer, GL_NEAREST) into a scratch texture, then from
// there to its destination (overlapping blits within one framebuffer are
// undefined). The target is not cleared: each dirty rectangle gets one
// quad drawn without blending, scissored to it, in Frame.Clear; every
// batch drawing into the target is then drawn once per dirty rectangle
// with its scissor intersected with it. Layers are cleared and drawn as
// always. A full frame's target is cleared to Frame.Clear with glClear.
//
// Failures. GL reports errors through glGetError, read (until it returns
// GL_NO_ERROR) at the end of every frame, and through
// glGetGraphicsResetStatus where the context offers it (OpenGL 4.5,
// GL_KHR_robustness or GL_ARB_robustness). A frame ending with a GL
// error, or whose new render target makes an incomplete framebuffer, is a
// failure: it may have drawn part of the frame, so the target holds no
// frame (ReadPixels fails, Retained is false) until the next successful
// one; so does a failure creating a layer or scratch framebuffer, found
// before anything is drawn, as Metal's failures before encoding.
// maxFailures
// failed frames in a row with no successful one in between, a
// GL_OUT_OF_MEMORY, or a context reset mark the device lost: from then on
// every call returns an error wrapping gpu.ErrDeviceLost, and the caller
// should switch to the CPU renderer (with a new device on a new context
// GL may work again). A frame whose textures exceed the limit
// (min(GL_MAX_TEXTURE_SIZE, 16384)) is rejected up front with
// gpu.ErrTargetTooLarge, as an invalid frame (an unknown or out-of-range
// upload) is with a plain error: nothing is allocated or issued, and
// neither is a failure, counted, or able to lose the device.
//
// Methods are serialized by a mutex; a Device serves one Renderer.
type Device struct {
	mu  sync.Mutex
	run func(func()) // runs a function on the context's thread
	f   *glFuncs     // the context's function table (only on run)
	ctx *headless    // the headless context NewDevice created; nil for newDevice
	// resetStatus is glGetGraphicsResetStatus when the context offers it.
	resetStatus func() uint32

	maxTex      int          // min(GL_MAX_TEXTURE_SIZE, 16384)
	es          bool         // an OpenGL ES 3.0 context (gles.go)
	formats     [3]texFormat // sampledFormats, but the ramp's on ES without norm16
	halfBuf     []byte       // ramp uploads converted to half floats (formats[gpu.TexRamp].half)
	pipe        *pipeline
	fbo         uint32     // framebuffer with target as its color attachment
	target      texture    // the frame's render target
	tex         [3]texture // gpu.TexMask, gpu.TexColor, gpu.TexRamp
	placeholder texture    // 1×1 transparent texture bound to gpu.TexImage and gpu.TexLayer when unused
	images      map[int]texture
	layers      *layerPool
	scratch     renderTex // gpu.Frame.Copies go through it
	scratchIdle int       // frames since scratch was last used

	hasFrame bool        // the target holds the last frame executed (ReadPixels may read it)
	clear    color.NRGBA // Frame.Clear of the last successful frame; opaque black before (the presenter's background)
	closed   bool        // Close was called
	failures int         // consecutive failed frames
	lost     error       // non-nil (wrapping gpu.ErrDeviceLost) once the device is lost
	commits  int         // frames issued and flushed (tests)

	// Per-frame scratch.
	gather  instGather // the frame's instances, in instance buffer order
	started []bool     // targets bound (cleared) already
}

// maxFailures consecutive failed frames mark the device lost (as
// internal/render/gpu/metal's maxFailures).
const maxFailures = 3

// maxTextureSize caps the texture limit at Metal's and Direct3D 11's.
const maxTextureSize = 16384

// maxTextureOverride, when > 0 (tests), replaces the device's texture
// size limit.
var maxTextureOverride int

// injectGLError, when set (tests), is asked at every frame's error check;
// a non-zero result is taken as the error glGetError returned.
var injectGLError func() uint32

// NewDevice creates a headless device: an OpenGL 3.3 core context of its
// own (EGL surfaceless on Linux, CGL on darwin, where it serves tests
// only) on a dedicated OS thread, and the quad shader compiled for it. It
// fails with an error wrapping ErrNoDevice when no such context can be
// created, and with the driver's info log when the shader does not
// compile or link; the caller then uses the CPU renderer.
func NewDevice() (*Device, error) {
	h, err := newHeadlessContext()
	if err != nil {
		return nil, err
	}
	d, err := newDevice(h.thread.do, h.funcs)
	if err != nil {
		h.close()
		return nil, err
	}
	d.ctx = h
	return d, nil
}

// newDevice builds a device on a context whose function table is f, run
// executing functions on the thread it is current on. The context stays
// the caller's (Close deletes the device's objects, not the context).
func newDevice(run func(func()), f *glFuncs) (d *Device, err error) {
	d = &Device{run: run, f: f, clear: color.NRGBA{A: 255}, formats: sampledFormats}
	d.layers = d.newLayerPool()
	run(func() {
		drainErrors(f) // errors left on a shared context are not the device's
		var maxTex int32
		f.GetIntegerv(glMAX_TEXTURE_SIZE, &maxTex)
		d.maxTex = min(int(maxTex), maxTextureSize)
		if isES(f.GetString(glVERSION)) {
			d.es = true
			if !hasGLExt(f, "GL_EXT_texture_norm16") {
				d.formats[gpu.TexRamp] = rgba16fFormat
			}
		}
		d.resetStatus = loadResetStatus(f, d.es)
		if d.pipe, err = newPipeline(f, d.es); err != nil {
			return
		}
		// Transparent black: what a batch binding no image or layer gets
		// (nothing samples it).
		d.placeholder = newTexture(f, gpu.TexImage, image.Pt(1, 1), rgba8Format, make([]byte, 4))
		if e := drainErrors(f); e != glNO_ERROR {
			err = fmt.Errorf("gl: creating the device: GL error %#x", e)
		}
	})
	if err != nil {
		run(d.release)
		return nil, err
	}
	return d, nil
}

// loadResetStatus returns glGetGraphicsResetStatus (or its ARB/KHR form;
// on ES, es, the KHR or EXT form) when f's context offers it, else nil.
// The extension is checked first: libepoxy aborts the process when a
// function without a provider is called.
func loadResetStatus(f *glFuncs, es bool) func() uint32 {
	major, minor := f.version()
	names := []string{}
	if !es && (major > 4 || major == 4 && minor >= 5) {
		names = append(names, "glGetGraphicsResetStatus")
	}
	var n int32
	f.GetIntegerv(glNUM_EXTENSIONS, &n)
	for i := range uint32(max(n, 0)) {
		switch ext := f.GetStringi(glEXTENSIONS, i); {
		case es && ext == "GL_KHR_robustness":
			names = append(names, "glGetGraphicsResetStatusKHR")
		case es && ext == "GL_EXT_robustness":
			names = append(names, "glGetGraphicsResetStatusEXT")
		case es:
		case ext == "GL_KHR_robustness":
			names = append(names, "glGetGraphicsResetStatus")
		case ext == "GL_ARB_robustness":
			names = append(names, "glGetGraphicsResetStatusARB")
		}
	}
	if len(names) == 0 {
		return nil
	}
	proc, err := glProc()
	if err != nil {
		return nil
	}
	for _, name := range names {
		if a := proc(name); a != 0 {
			var fn func() uint32
			purego.RegisterFunc(&fn, a)
			return fn
		}
	}
	return nil
}

// hasGLExt reports whether f's context lists ext in GL_EXTENSIONS.
func hasGLExt(f *glFuncs, ext string) bool {
	var n int32
	f.GetIntegerv(glNUM_EXTENSIONS, &n)
	for i := range uint32(max(n, 0)) {
		if f.GetStringi(glEXTENSIONS, i) == ext {
			return true
		}
	}
	return false
}

// drainErrors reads glGetError until it returns GL_NO_ERROR (GL keeps one
// flag per kind of error) and returns GL_OUT_OF_MEMORY if it was among
// them, else the first error, else GL_NO_ERROR.
func drainErrors(f *glFuncs) uint32 {
	first := uint32(glNO_ERROR)
	for range 16 { // a broken context may never report GL_NO_ERROR
		e := f.GetError()
		if e == glNO_ERROR {
			break
		}
		if first == glNO_ERROR || e == glOUT_OF_MEMORY {
			first = e
		}
	}
	return first
}

// Execute implements gpu.Device. A frame without pixels is skipped without
// issuing anything.
func (d *Device) Execute(fr *gpu.Frame) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.lost != nil {
		return d.lost
	}
	if fr.Size.X <= 0 || fr.Size.Y <= 0 {
		return nil
	}
	// Invalid or oversized frames, and partial frames on a target that
	// does not hold the last frame: rejected before anything is allocated
	// or issued, and not failures.
	limit := d.maxTex
	if maxTextureOverride > 0 {
		limit = maxTextureOverride
	}
	if err := checkTextureSizes(fr, limit); err != nil {
		return err
	}
	if len(fr.Dirty) == 0 && len(fr.Copies) > 0 {
		return errors.New("gl: frame with Copies but no Dirty")
	}
	if len(fr.Dirty) > 0 {
		if !d.hasFrame || d.target.tex == 0 || d.target.size != fr.Size {
			return fmt.Errorf("gl: partial frame: %w", gpu.ErrNotRetained)
		}
		if err := checkPartial(fr); err != nil {
			return err
		}
	}
	var err error
	d.run(func() { err = d.execute(fr, limit) })
	d.gather.reset() // keep no reference to the Frame
	return err
}

// execute runs a frame on the context's thread: the steps that can find
// the frame invalid or fail before anything is drawn (the copy scratch
// texture, the textures, the image and layer checks, the image textures,
// the layer textures), then the frame's commands. limit is the texture
// size limit.
func (d *Device) execute(fr *gpu.Frame, limit int) error {
	f := d.f
	drainErrors(f) // errors left by others sharing the context are not this frame's
	d.scratchIdle++
	if len(fr.Dirty) > 0 {
		if err := d.ensureScratch(fr.Copies, limit); err != nil {
			if isAllocError(err) {
				return d.allocFailed(err)
			}
			return err // too large to stage (ErrNotRetained): drawn in full; not a failure
		}
	}
	if d.scratch.tex != 0 && d.scratchIdle > scratchIdleFrames {
		d.scratch.release(f) // no copies for a while: do not keep a window-sized texture
	}
	// As in Metal, the textures are (re)created before the uploads are
	// validated: a frame that recreates the target and then turns out
	// invalid leaves hasFrame false.
	if err := d.ensureTextures(fr); err != nil {
		return d.failed(err)
	}
	if err := d.checkImages(fr); err != nil {
		return err
	}
	if err := d.checkUploads(fr); err != nil {
		return err
	}
	d.applyImages(fr)
	if err := d.layers.assign(fr); err != nil {
		if isAllocError(err) {
			return d.allocFailed(err)
		}
		return err
	}
	ins := d.gather.gather(fr, d.layers.Size)
	n := 0
	for _, b := range ins {
		n += len(b)
	}
	d.encode(fr, ins, n)
	f.Flush()
	d.commits++
	if err := d.frameError(); err != nil {
		return err
	}
	d.hasFrame = true
	d.clear = fr.Clear
	d.failures = 0
	return nil
}

// encode issues the frame's commands (see Device): the copies, the
// target's clear (or its dirty rectangles' clearing quads), the uploads
// and the batches, each into its target. ins are the frame's instances
// in buffer order (instGather: the batches', then the clearing quads),
// n their count.
func (d *Device) encode(fr *gpu.Frame, ins [][]shaders.Instance, n int) {
	f := d.f
	partial := len(fr.Dirty) > 0
	d.pipe.bind(f)
	units := [5]uint32{
		gpu.TexMask:  d.tex[gpu.TexMask].tex,
		gpu.TexColor: d.tex[gpu.TexColor].tex,
		gpu.TexRamp:  d.tex[gpu.TexRamp].tex,
		gpu.TexImage: d.placeholder.tex,
		gpu.TexLayer: d.placeholder.tex,
	}
	for i, t := range units {
		f.ActiveTexture(glTEXTURE0 + uint32(i))
		f.BindTexture(glTEXTURE_2D, t)
	}
	if n > 0 {
		d.pipe.writeInstances(f, ins, n)
	}
	if partial {
		d.copyRegions(fr.Copies) // before anything is drawn
	}
	clear(d.started)
	d.started = append(d.started[:0], make([]bool, len(fr.Layers)+1)...)
	scissor := func(r image.Rectangle) {
		f.Scissor(int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}

	// The frame's target first: cleared, or in a partial frame its dirty
	// rectangles cleared by quads drawn without blending.
	d.bindTarget(fr, 0)
	d.started[0] = true
	off := 0
	for _, b := range ins[:len(fr.Batches)] {
		off += len(b) * gen.QuadInstanceStride
	}
	if !partial {
		clearBound(f, devkit.ClearColor(fr.Clear))
	} else {
		f.Disable(glBLEND)
		for k, r := range fr.Dirty {
			scissor(r)
			d.pipe.bindAt(f, off+k*gen.QuadInstanceStride)
			f.DrawArraysInstanced(glTRIANGLE_STRIP, 0, 4, 1)
		}
		f.Enable(glBLEND)
	}
	cur := 0

	d.upload(fr.Uploads)
	off = 0
	for i := range fr.Batches {
		b := &fr.Batches[i]
		d.upload(b.Uploads) // after every earlier batch's draws
		k := len(ins[i])
		at := off
		off += k * gen.QuadInstanceStride
		if b.Grab != 0 {
			d.grab(fr, b)
			cur = -1 // the next batch binds its target again
			continue
		}
		if b.Target != cur {
			d.bindTarget(fr, b.Target)
			cur = b.Target
			if !d.started[cur] {
				// A layer is transparent before the first batch drawing
				// into it.
				clearBound(f, transparent)
				d.started[cur] = true
			}
		}
		full, origin := image.Rectangle{Max: fr.Size}, image.Point{}
		if b.Target != 0 {
			ld := fr.Layers[b.Target-1]
			full, origin = image.Rectangle{Max: ld.Size}, ld.Bounds.Min
		}
		sc, ok := scissorFor(b.Scissor, origin, full)
		if k == 0 || !ok {
			continue
		}
		img, layer := d.placeholder.tex, d.placeholder.tex
		if b.Image != 0 {
			img = d.images[b.Image].tex // held: checked by checkImages
		}
		if b.Layer != 0 {
			layer = d.layers.Tex[b.Layer].tex // drawn by an earlier batch: checked by assign
		}
		d.bindSampled(img, layer)
		d.pipe.bindAt(f, at)
		if partial && b.Target == 0 {
			// Drawn once per dirty rectangle (disjoint: no pixel twice).
			for _, r := range fr.Dirty {
				s := sc.Intersect(r)
				if s.Empty() {
					continue
				}
				scissor(s)
				f.DrawArraysInstanced(glTRIANGLE_STRIP, 0, 4, int32(k))
			}
			continue
		}
		scissor(sc)
		f.DrawArraysInstanced(glTRIANGLE_STRIP, 0, 4, int32(k))
	}
}

// grab issues the grab batch b (gpu.Batch.Grab): its source target is
// cleared first if nothing drew into it yet (a layer starts transparent),
// the grab layer is cleared to transparent, then its Bounds are blitted (GL_NEAREST, equal sizes: an
// exact copy) from the source's framebuffer to texel (0, 0) of the
// layer's. The image and layer units hold the placeholder meanwhile, so
// the layer is not bound for sampling while it is drawn into; GL orders
// the blit after the draws issued before it and before the ones after.
func (d *Device) grab(fr *gpu.Frame, b *gpu.Batch) {
	f := d.f
	if !d.started[b.Target] {
		d.bindTarget(fr, b.Target)
		clearBound(f, transparent) // the frame's target is always started
		d.started[b.Target] = true
	}
	d.bindSampled(d.placeholder.tex, d.placeholder.tex)
	src := d.fbo
	if b.Target != 0 {
		src = d.layers.Tex[b.Target].fbo
	}
	r := devkit.GrabRect(fr, b)
	// The layer starts transparent (texels the copy does not reach
	// included: a pooled texture holds an earlier layer's pixels).
	d.bindTarget(fr, b.Grab)
	clearBound(f, transparent)
	f.Disable(glSCISSOR_TEST)
	f.BindFramebuffer(glREAD_FRAMEBUFFER, src)
	f.BindFramebuffer(glDRAW_FRAMEBUFFER, d.layers.Tex[b.Grab].fbo)
	f.BlitFramebuffer(int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y),
		0, 0, int32(r.Dx()), int32(r.Dy()), glCOLOR_BUFFER_BIT, glNEAREST)
	f.Enable(glSCISSOR_TEST)
}

// bindSampled binds textures img and layer to the image and layer units.
func (d *Device) bindSampled(img, layer uint32) {
	f := d.f
	f.ActiveTexture(glTEXTURE0 + gpu.TexImage)
	f.BindTexture(glTEXTURE_2D, img)
	f.ActiveTexture(glTEXTURE0 + gpu.TexLayer)
	f.BindTexture(glTEXTURE_2D, layer)
}

// bindTarget makes target (0: the frame's target, n: layer n) the draw
// framebuffer, with the viewport and FrameU for it. The image and layer
// units get the placeholder first, so no texture is bound for sampling
// while it becomes the render target (a pooled layer texture may have
// been sampled by an earlier batch under another layer number).
func (d *Device) bindTarget(fr *gpu.Frame, target int) {
	f := d.f
	d.bindSampled(d.placeholder.tex, d.placeholder.tex)
	fbo := d.fbo
	size := fr.Size
	fu := shaders.Frame{ViewSize: sl.V2(float32(size.X), float32(size.Y))}
	if target != 0 {
		ld := fr.Layers[target-1]
		fbo = d.layers.Tex[target].fbo
		size = d.layers.Size[target] // ≥ ld.Size: the layer is its top-left part
		fu = shaders.Frame{ViewSize: sl.V2(float32(size.X), float32(size.Y)), Origin: sl.V2(float32(ld.Bounds.Min.X), float32(ld.Bounds.Min.Y))}
	}
	f.BindFramebuffer(glFRAMEBUFFER, fbo)
	f.Viewport(0, 0, int32(size.X), int32(size.Y))
	d.pipe.setFrameU(f, fu)
}

// scissorFor returns the scissor rectangle, in the target's texels, of a
// batch whose Scissor is s (main-target coordinates; empty means the whole
// target) drawing into a target whose texel (0, 0) is at origin and whose
// drawable texels are full. ok is false when nothing would be drawn.
func scissorFor(s image.Rectangle, origin image.Point, full image.Rectangle) (image.Rectangle, bool) {
	sc := full
	if !s.Empty() {
		sc = s.Sub(origin).Intersect(full)
	}
	return sc, !sc.Empty()
}

// frameError checks the frame just issued: GL errors (or the injected
// one) and a context reset. It records the outcome and returns the error
// to report, nil when the frame succeeded.
func (d *Device) frameError() error {
	e := drainErrors(d.f)
	if injectGLError != nil {
		if ie := injectGLError(); ie != glNO_ERROR {
			e = ie
		}
	}
	if d.resetStatus != nil {
		if rs := d.resetStatus(); rs != glNO_ERROR {
			return d.lose(fmt.Errorf("gl: the context was reset (glGetGraphicsResetStatus %#x)", rs))
		}
	}
	switch e {
	case glNO_ERROR:
		return nil
	case glOUT_OF_MEMORY:
		return d.lose(fmt.Errorf("gl: frame failed: GL_OUT_OF_MEMORY (%#x)", e))
	}
	return d.failed(fmt.Errorf("gl: frame failed: GL error %#x", e))
}

// failed records a failed frame and returns the error to report: err, or
// the loss error once maxFailures frames in a row failed. The frame may
// have touched the target: it holds no frame any more.
func (d *Device) failed(err error) error {
	d.hasFrame = false
	d.failures++
	if d.lost == nil && d.failures >= maxFailures {
		d.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	if d.lost != nil {
		return d.lost
	}
	return err
}

// allocFailed records a failure creating a layer or scratch render
// texture, found before anything was drawn: a GL_OUT_OF_MEMORY behind it
// loses the device; otherwise it is counted (Metal's failures before
// encoding), and the target keeps its frame.
func (d *Device) allocFailed(err error) error {
	if drainErrors(d.f) == glOUT_OF_MEMORY {
		return d.lose(err)
	}
	d.failures++
	if d.lost == nil && d.failures >= maxFailures {
		d.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	if d.lost != nil {
		return d.lost
	}
	return err
}

// lose records a failure that loses the device whatever the count and
// returns the loss error.
func (d *Device) lose(err error) error {
	d.hasFrame = false
	d.failures++
	if d.lost == nil {
		d.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	return d.lost
}

// ReadPixels waits for the last frame and returns a copy of the render
// target: RGBA8, premultiplied alpha, Frame.Size of that frame, row 0 at
// the top. It fails before the first frame with pixels, after a failed
// frame, and with an error wrapping gpu.ErrDeviceLost once the device is
// lost.
func (d *Device) ReadPixels() (*image.RGBA, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if d.lost != nil {
		return nil, d.lost
	}
	if !d.hasFrame || d.target.tex == 0 {
		return nil, errors.New("gl: ReadPixels before any frame")
	}
	size := d.target.size
	img := image.NewRGBA(image.Rectangle{Max: size})
	var e uint32
	d.run(func() {
		f := d.f
		drainErrors(f)
		f.BindFramebuffer(glREAD_FRAMEBUFFER, d.fbo)
		f.ReadBuffer(glCOLOR_ATTACHMENT0)
		f.PixelStorei(glPACK_ALIGNMENT, 1)
		f.PixelStorei(glPACK_ROW_LENGTH, 0)
		// Tightly packed rows (img.Stride = 4·width), row 0 first: the
		// target's top row.
		f.ReadPixels(0, 0, int32(size.X), int32(size.Y), glRGBA, glUNSIGNED_BYTE, unsafe.Pointer(&img.Pix[0]))
		f.PixelStorei(glPACK_ALIGNMENT, 4)
		e = drainErrors(f)
	})
	switch e {
	case glNO_ERROR:
		return img, nil
	case glOUT_OF_MEMORY:
		return nil, d.lose(fmt.Errorf("gl: ReadPixels: GL_OUT_OF_MEMORY (%#x)", e))
	}
	return nil, fmt.Errorf("gl: ReadPixels: GL error %#x", e)
}

// Close deletes every GL object of the device and, for a device from
// NewDevice, its context and thread. It is safe to call more than once.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.closed = true
	if d.ctx == nil || d.ctx.thread.alive() {
		d.run(d.release)
	}
	if d.ctx != nil {
		d.ctx.close() // deletes whatever is left with the context
		d.ctx = nil
	}
}

// release deletes every object the device holds; on the context's thread.
func (d *Device) release() {
	f := d.f
	d.target.release(f)
	for i := range d.tex {
		d.tex[i].release(f)
	}
	d.placeholder.release(f)
	for id, t := range d.images {
		t.release(f)
		delete(d.images, id)
	}
	if d.layers != nil {
		d.layers.ReleaseAll()
	}
	d.scratch.release(f)
	if d.fbo != 0 {
		f.DeleteFramebuffers(1, &d.fbo)
		d.fbo = 0
	}
	d.pipe.release(f)
	d.pipe = nil
	d.hasFrame = false
}

var (
	_ gpu.Device      = (*Device)(nil)
	_ gpu.PixelReader = (*Device)(nil)
	_ gpu.Retainer    = (*Device)(nil)
)
