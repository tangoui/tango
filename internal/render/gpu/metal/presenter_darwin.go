//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui/paint"
)

// Presentation to a CAMetalLayer.
//
// The Renderer keeps drawing into the Device's offscreen RGBA8Unorm
// target, exactly as without a layer (so ReadPixels and the parity tests
// see what is shown). present then copies the target into the layer's
// next BGRA8Unorm drawable with one full-screen triangle whose fragment
// shader reads the target texel at the same pixel (texture.read, no
// sampler, no blending): writing the read float4 to a BGRA attachment does
// the channel swizzle, and x/255 is re-quantized to exactly x, so the
// drawable holds the target's bytes. A blit cannot do it: blits need
// matching pixel formats and a framebufferOnly drawable (which lets Core
// Animation pick the fastest memory layout) cannot be a blit destination.
// The pass uses loadAction dontCare, so the copy is the only GPU work
// touching the drawable. Pixels of a drawable larger than the target
// (the target is ceil(Size*Scale), the drawable round(Size*Scale), and
// both can lag a resize) repeat the target's last row/column instead of
// showing garbage; the excess of a smaller drawable is cropped.
//
// The copy is encoded in its own command buffer on the Device's queue
// after the frame's, so Metal orders it after the frame's draws; present
// does not wait for the GPU. Its command buffer is waited for, like the
// frame's, by the next Execute, so its errors are reported the same way
// (see Device). nextDrawable is the back-pressure: it blocks while all the
// layer's drawables (3 by default) are in flight.
//
// The layer's colorspace is sRGB and the target holds sRGB-encoded bytes
// (blending happens on those bytes, as in paint.CPU), so the drawable is
// BGRA8Unorm, not BGRA8Unorm_sRGB: no conversion on store, the same
// pixels the CPU path shows.
//
// presentsWithTransaction stays NO (the default): drawables are shown at
// the next display refresh independently of Core Animation transactions.
// During a live resize this may show a frame whose size lags the window
// by a frame (the platform keeps the drawable top-left aligned); turning
// it on would sync them but make present wait for the GPU (scheduled) on
// every frame. M1 keeps the default.

// MTLPixelFormatBGRA8Unorm and the present pass's enums.
const (
	pixelFormatBGRA8Unorm = 80
	loadActionDontCare    = 0
	primitiveTriangle     = 3
)

// presentMSL is the copy pass. It is Metal-specific (not generated from
// internal/render/gpu/shaders): a full-screen triangle and a texel read.
const presentMSL = `#include <metal_stdlib>
using namespace metal;

struct PresentOut { float4 pos [[position]]; };

vertex PresentOut present_vs(uint vid [[vertex_id]]) {
	float2 p = float2(float((vid << 1) & 2), float(vid & 2)); // (0,0) (2,0) (0,2)
	PresentOut o;
	o.pos = float4(p * 2.0 - 1.0, 0.0, 1.0);
	return o;
}

fragment float4 present_fs(PresentOut in [[stage_in]], texture2d<float, access::read> src [[texture(0)]]) {
	uint2 last = uint2(src.get_width(), src.get_height()) - 1;
	return src.read(min(uint2(in.pos.xy), last));
}
`

// cgSize is CGSize (an HFA of two doubles: returned and passed in float
// registers on both architectures, so plain objc_msgSend).
type cgSize struct{ W, H float64 }

var (
	presentLoadOnce sync.Once
	presentLoadErr  error

	cgColorSpaceCreateWithName func(name uintptr) uintptr
	cgColorSpaceRelease        func(cs uintptr)
	kCGColorSpaceSRGB          uintptr // CFStringRef

	msgBool    func(o id, s objc.SEL) bool
	msgBoolID  func(o id, s objc.SEL, a id) bool
	msgSize    func(o id, s objc.SEL) cgSize
	msgSetSize func(o id, s objc.SEL, v cgSize)
	msgSetPtr  func(o id, s objc.SEL, v uintptr)
	msgNSError func(c id, s objc.SEL, domain id, code uint64, info id) id
)

// loadPresenter loads Metal (load) plus QuartzCore (CAMetalLayer) and
// CoreGraphics (the sRGB color space).
func loadPresenter() error {
	if err := load(); err != nil {
		return err
	}
	presentLoadOnce.Do(func() {
		if _, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
			presentLoadErr = fmt.Errorf("%w: load QuartzCore.framework: %v", ErrNoDevice, err)
			return
		}
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			presentLoadErr = fmt.Errorf("%w: load CoreGraphics.framework: %v", ErrNoDevice, err)
			return
		}
		purego.RegisterLibFunc(&cgColorSpaceCreateWithName, cg, "CGColorSpaceCreateWithName")
		purego.RegisterLibFunc(&cgColorSpaceRelease, cg, "CGColorSpaceRelease")
		if sym, err := purego.Dlsym(cg, "kCGColorSpaceSRGB"); err == nil {
			kCGColorSpaceSRGB = **(**uintptr)(unsafe.Pointer(&sym)) // the variable's value, not its address
		}
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			presentLoadErr = fmt.Errorf("%w: load libobjc: %v", ErrNoDevice, err)
			return
		}
		send, err := purego.Dlsym(libobjc, "objc_msgSend")
		if err != nil {
			presentLoadErr = fmt.Errorf("%w: objc_msgSend: %v", ErrNoDevice, err)
			return
		}
		for _, fn := range []any{&msgBool, &msgBoolID, &msgSize, &msgSetSize, &msgSetPtr, &msgNSError} {
			purego.RegisterFunc(fn, send)
		}
	})
	return presentLoadErr
}

// NewPresenter attaches a GPU renderer to a CAMetalLayer obtained from
// platform.CanvasMetal.UseMetalLayer. Main thread only. On non-darwin it
// returns an error wrapping ErrUnsupported.
//
// It creates a Device (failing with ErrNoDevice without a Metal GPU),
// sets the layer's device, pixelFormat (BGRA8Unorm), framebufferOnly
// (YES) and colorspace (sRGB), and leaves contentsScale, frame, opacity
// and actions to the platform. r is a gpu.Renderer over the Device; it
// also implements interface{ Close() error }, which releases the Device
// and the layer reference (main thread; r and present fail with ErrClosed
// afterwards).
//
// present(size, scale) shows the frame r rendered last: size is the
// drawable size in device pixels, round(Size*Scale) (layer.drawableSize is
// set when it differs; scale is informational). It returns nil without
// presenting when size is empty, no frame was rendered, or the layer has
// no drawable (not visible; nextDrawable timed out). It returns the error
// of a failed earlier GPU frame (which r.Render also reports once), and an
// error wrapping ErrDeviceLost once the device is unusable — after which
// r.Render fails with ErrDeviceLost too, and the caller should go back to
// the CPU renderer.
func NewPresenter(layer uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	p, err := newPresenter(layer)
	if err != nil {
		return nil, nil, err
	}
	return p.r, p.present, nil
}

// presenter is NewPresenter's state.
type presenter struct {
	layer id // retained until Close
	dev   *Device
	r     paint.Renderer

	presented    int // drawables presented (tests)
	drawableSets int // drawableSize changes (tests)
}

// presenterRenderer is the Renderer NewPresenter returns: a gpu.Renderer
// that can be closed.
type presenterRenderer struct {
	*gpu.Renderer
	p *presenter
}

// Close releases the Device and the presenter's reference to the layer.
func (r *presenterRenderer) Close() error {
	r.p.Close()
	return nil
}

func newPresenter(layer uintptr) (*presenter, error) {
	if layer == 0 {
		return nil, errors.New("metal: NewPresenter: nil layer")
	}
	if err := loadPresenter(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()

	l := id(layer)
	if cls := class("CAMetalLayer"); cls == 0 || !msgBoolID(l, sel("isKindOfClass:"), cls) {
		return nil, errors.New("metal: NewPresenter: not a CAMetalLayer")
	}
	dev, err := NewDevice()
	if err != nil {
		return nil, err
	}
	pipe, err := dev.newPresentPipeline()
	if err != nil {
		dev.Close()
		return nil, err
	}
	dev.presentPipeline = pipe

	msgSetID(l, sel("setDevice:"), dev.dev)
	msgSetU64(l, sel("setPixelFormat:"), pixelFormatBGRA8Unorm)
	msgSetBool(l, sel("setFramebufferOnly:"), true)
	if kCGColorSpaceSRGB != 0 {
		if cs := cgColorSpaceCreateWithName(kCGColorSpaceSRGB); cs != 0 {
			msgSetPtr(l, sel("setColorspace:"), cs) // the layer retains it
			cgColorSpaceRelease(cs)
		}
	}
	p := &presenter{layer: retain(l), dev: dev}
	p.r = &presenterRenderer{Renderer: gpu.NewRenderer(dev), p: p}
	return p, nil
}

// newPresentPipeline compiles presentMSL for a BGRA8Unorm attachment.
func (d *Device) newPresentPipeline() (id, error) {
	var e id
	lib := msgWithOptErr(d.dev, sel("newLibraryWithSource:options:error:"), nsString(presentMSL), 0, &e)
	if lib == 0 {
		return 0, nsError("metal: compile present shader", e)
	}
	defer release(lib)
	vs := lib.Send(sel("newFunctionWithName:"), nsString("present_vs"))
	fs := lib.Send(sel("newFunctionWithName:"), nsString("present_fs"))
	defer release(vs)
	defer release(fs)
	if vs == 0 || fs == 0 {
		return 0, errors.New("metal: present shader lacks present_vs/present_fs")
	}
	desc := newObject("MTLRenderPipelineDescriptor")
	defer release(desc)
	msgSetID(desc, sel("setVertexFunction:"), vs)
	msgSetID(desc, sel("setFragmentFunction:"), fs)
	ca := msgIdx(desc.Send(sel("colorAttachments")), sel("objectAtIndexedSubscript:"), 0)
	msgSetU64(ca, sel("setPixelFormat:"), pixelFormatBGRA8Unorm)
	e = 0
	p := msgWithErr(d.dev, sel("newRenderPipelineStateWithDescriptor:error:"), desc, &e)
	if p == 0 {
		return 0, nsError("metal: create present pipeline", e)
	}
	return p, nil
}

// encodeCopy encodes the present pass: the target copied into dst (a
// BGRA8Unorm render target) pixel for pixel.
func (d *Device) encodeCopy(cb, dst id) {
	rp := class("MTLRenderPassDescriptor").Send(sel("renderPassDescriptor"))
	ca := msgIdx(rp.Send(sel("colorAttachments")), sel("objectAtIndexedSubscript:"), 0)
	msgSetID(ca, sel("setTexture:"), dst)
	msgSetU64(ca, sel("setLoadAction:"), loadActionDontCare) // every pixel is written
	msgSetU64(ca, sel("setStoreAction:"), storeActionStore)
	enc := cb.Send(sel("renderCommandEncoderWithDescriptor:"), rp)
	msgSetID(enc, sel("setRenderPipelineState:"), d.presentPipeline)
	msgSetIDIdx(enc, sel("setFragmentTexture:atIndex:"), d.target, 0)
	msgDraw(enc, sel("drawPrimitives:vertexStart:vertexCount:instanceCount:"), primitiveTriangle, 0, 3, 1)
	enc.Send(sel("endEncoding"))
}

// maxDrawable is the largest drawable side: Metal's 2D texture limit
// (maxTextureSize). CAMetalLayer ignores a larger drawableSize.
const maxDrawable = maxTextureSize

// maxInflight bounds the command buffers present lets pile up when it is
// called without Execute in between (Execute waits for all of them).
const maxInflight = 4

func (p *presenter) present(size image.Point, scale float32) error {
	d := p.dev
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if d.lost != nil {
		return d.lost
	}
	if size.X <= 0 || size.Y <= 0 {
		return d.pendingErr // minimized or empty: nothing to show
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()

	if len(d.inflight) >= maxInflight {
		d.wait()
	}
	if d.lost != nil {
		return d.lost
	}
	if d.pendingErr != nil {
		return d.pendingErr // Execute reports it too and resets the Renderer
	}
	size = image.Pt(min(size.X, maxDrawable), min(size.Y, maxDrawable))
	if cur := msgSize(p.layer, sel("drawableSize")); cur.W != float64(size.X) || cur.H != float64(size.Y) {
		msgSetSize(p.layer, sel("setDrawableSize:"), cgSize{float64(size.X), float64(size.Y)})
		p.drawableSets++
	}
	if !d.hasFrame {
		return nil
	}
	drawable := p.layer.Send(sel("nextDrawable"))
	if drawable == 0 {
		return nil // not visible, or every drawable busy for a second
	}
	cb := d.queue.Send(sel("commandBuffer"))
	if cb == 0 {
		return d.syncFailed(errors.New("metal: commandBuffer failed"))
	}
	d.encodeCopy(cb, drawable.Send(sel("texture")))
	msgSetID(cb, sel("presentDrawable:"), drawable)
	d.commit(cb)
	d.commits++
	p.presented++
	return nil
}

// Close closes the Device and releases the layer. Safe to call more than
// once.
func (p *presenter) Close() {
	p.dev.Close()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p.dev.mu.Lock()
	defer p.dev.mu.Unlock()
	release(p.layer)
	p.layer = 0
}
