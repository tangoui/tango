//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/paint"
)

// Presentation to a window through a DXGI flip-model swap chain.
//
// The Renderer keeps drawing into the Device's offscreen R8G8B8A8_UNORM
// target, exactly as without a window (so ReadPixels and the parity tests
// see what is shown). present then copies the target into the swap
// chain's B8G8R8A8_UNORM back buffer with one full-screen triangle whose
// pixel shader loads the target texel at the same pixel (Texture2D.Load,
// no sampler, no blending): writing the loaded float4 to a BGRA render
// target does the channel swizzle, and x/255 is re-quantized to exactly
// x, so the back buffer holds the target's bytes (ui/gpu/metal's
// presenter does the same with a CAMetalLayer drawable). Pixels of a back
// buffer larger than the target repeat the target's last row and column;
// the excess of a smaller target is cropped. Every present redraws the
// whole back buffer.
//
// The swap chain: CreateSwapChainForHwnd with DXGI_SWAP_EFFECT_FLIP_DISCARD,
// 2 buffers, B8G8R8A8_UNORM, DXGI_ALPHA_MODE_IGNORE (opaque canvases
// only: layered windows draw on the CPU) and DXGI_SCALING_NONE, so during
// a live resize a buffer of the old size stays top-left aligned instead
// of being stretched; the uncovered rest of the window shows the swap
// chain's background color, set to the last frame's clear color (black
// before the first frame). createSwapChain falls back to FLIP_SEQUENTIAL
// and then SCALING_STRETCH, the Windows 8.x combinations; Go 1.27 needs
// Windows 10 or later, where FLIP_DISCARD with SCALING_NONE always
// exists, so the fallbacks are effectively unreachable and kept only for
// odd drivers and the tests (swapChainModes). Alt+Enter
// fullscreen is disabled (MakeWindowAssociation with
// DXGI_MWA_NO_ALT_ENTER). The presenter never draws GDI on the window;
// the CPU fallback uses a sibling window.
//
// Resizing. When present's size differs from the back buffer's, it
// releases its back-buffer view, unbinds it, flushes and calls
// ResizeBuffers first (DXGI requires every reference to the buffers to be
// gone). The platform calls present on the main thread, and after
// WM_SIZE renders a frame synchronously.
//
// Frame latency. The device's maximum frame latency is set to 1
// (IDXGIDevice1::SetMaximumFrameLatency): Present blocks while a frame is
// still queued, which bounds the work queued ahead of the display to one
// frame (input-to-photon latency stays low, memory bounded) without the
// bookkeeping of a waitable swap chain. Present1 uses sync interval 1
// (vsync).
//
// Status. DXGI_STATUS_OCCLUDED (window minimized or covered) is a success
// code: the frame is simply not shown. A device removed, reset or hung
// loses the device (gpu.ErrDeviceLost) for present and Execute alike;
// other failures are counted on a counter of presents (health), which
// only a successful present resets: maxFailures failed presents in a row
// lose the device even when every frame in between executes fine.

// presentHLSL is the copy pass. It is Direct3D-specific (not generated
// from ui/gpu/shaders): a full-screen triangle and a texel load, valid for
// shader model 4.0 and 5.0.
const presentHLSL = `Texture2D<float4> src : register(t0);

float4 present_vs(uint vid : SV_VertexID) : SV_Position {
	float2 p = float2(float((vid << 1) & 2), float(vid & 2)); // (0,0) (2,0) (0,2)
	return float4(p * 2.0 - 1.0, 0.0, 1.0);
}

float4 present_ps(float4 pos : SV_Position) : SV_Target {
	uint w, h;
	src.GetDimensions(w, h);
	uint2 q = min(uint2(pos.xy), uint2(w - 1, h - 1));
	return src.Load(int3(q, 0));
}
`

// NewPresenter attaches a GPU renderer to the window hwnd: a Device (a
// hardware adapter, else WARP) and a flip-model swap chain for the
// window. Main thread only. It fails with an error wrapping ErrNoDevice
// when no device can be created; any error means: use the CPU renderer.
//
// r is a gpu.Renderer over the Device; it also implements
// interface{ Close() error }, which releases the swap chain and the
// Device (idempotent; r and present fail with ErrClosed afterwards).
//
// present(size, scale) shows the frame r rendered last: size is the back
// buffer size in device pixels (the window's client size; clamped to the
// texture limit), scale is informational. It returns nil without
// presenting when size is empty or no frame was rendered yet (it still
// resizes the buffers to size). It returns an error wrapping
// ErrDeviceLost once the device is unusable — after which r.Render fails
// with ErrDeviceLost too, and the caller should go back to the CPU
// renderer.
func NewPresenter(hwnd uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	p, err := newPresenter(hwnd)
	if err != nil {
		return nil, nil, err
	}
	return p.r, p.present, nil
}

// presenter is NewPresenter's state. Its COM fields are references it
// owns, released by Close; they are used under the Device's mutex.
type presenter struct {
	d *Device
	r paint.Renderer

	swap    uintptr     // IDXGISwapChain1
	backTex uintptr     // ID3D11Texture2D: back buffer 0 (in Direct3D 11 always the current one)
	backRTV uintptr     // its render-target view
	bufSize image.Point // the back buffers' size
	vs, ps  uintptr     // the copy pass shaders

	effect       uint32      // the swap chain's DXGI_SWAP_EFFECT
	presentedSeq int         // Device.frameSeq of the frame shown last; 0: none (or occluded)
	background   color.NRGBA // the swap chain's background color
	released     bool        // the presenter's objects are released

	presented, resizes int // successful presents and ResizeBuffers calls (tests)

	// beforePresent, set by tests, runs after the copy pass, before
	// Present1, with the back buffer texture (under the Device's mutex).
	beforePresent func(backTex uintptr)
}

// presenterRenderer is the Renderer NewPresenter returns: a gpu.Renderer
// that can be closed.
type presenterRenderer struct {
	*gpu.Renderer
	p *presenter
}

// Close releases the swap chain and the Device.
func (r *presenterRenderer) Close() error {
	r.p.Close()
	return nil
}

func newPresenter(hwnd uintptr) (p *presenter, err error) {
	if hwnd == 0 {
		return nil, errors.New("d3d11: NewPresenter: no window (hwnd 0)")
	}
	d, err := NewDevice()
	if err != nil {
		return nil, err
	}
	p = &presenter{d: d}
	// "return nil, err" sets p to nil before deferred calls run, so close
	// the presenter through its own variable.
	built := p
	defer func() {
		if err != nil {
			built.Close()
			p = nil
		}
	}()
	setFrameLatency(d.dev)
	factory, err := factoryOf(d.dev)
	if err != nil {
		return nil, err
	}
	defer release(factory)
	if p.swap, p.effect, err = createSwapChain(factory, d.dev, hwnd); err != nil {
		return nil, err
	}
	p.setBackground(color.NRGBA{A: 255})
	// MakeWindowAssociation(WindowHandle, Flags)
	if err := hrError("MakeWindowAssociation", comCall(factory, factMakeWindowAssoc, hwnd, mwaNoAltEnter)); err != nil {
		return nil, err
	}
	var desc swapChainDesc1
	if err := hrError("IDXGISwapChain1.GetDesc1", comCall(p.swap, swap1GetDesc1, uintptr(unsafe.Pointer(&desc)))); err != nil {
		return nil, err
	}
	p.bufSize = image.Pt(int(desc.Width), int(desc.Height))

	vsTarget, psTarget := shaderProfiles(d.featureLevel)
	vsCode, err := compileShader(presentHLSL, "present_vs", vsTarget)
	if err != nil {
		return nil, err
	}
	psCode, err := compileShader(presentHLSL, "present_ps", psTarget)
	if err != nil {
		return nil, err
	}
	hr := comCall(d.dev, devCreateVertexShader, uintptr(unsafe.Pointer(&vsCode[0])), uintptr(len(vsCode)), 0, uintptr(unsafe.Pointer(&p.vs)))
	if err := hrError("CreateVertexShader(present)", hr); err != nil {
		p.vs = 0
		return nil, err
	}
	hr = comCall(d.dev, devCreatePixelShader, uintptr(unsafe.Pointer(&psCode[0])), uintptr(len(psCode)), 0, uintptr(unsafe.Pointer(&p.ps)))
	if err := hrError("CreatePixelShader(present)", hr); err != nil {
		p.ps = 0
		return nil, err
	}
	runtime.KeepAlive(vsCode)
	runtime.KeepAlive(psCode)
	p.r = &presenterRenderer{Renderer: gpu.NewRenderer(d), p: p}
	return p, nil
}

// setFrameLatency limits the frames queued ahead of the display to one
// (see the Frame latency note above). A failure only loses the bound.
func setFrameLatency(dev uintptr) {
	dxgiDev1, err := queryInterface(dev, &iidIDXGIDevice1, "IDXGIDevice1")
	if err != nil {
		return
	}
	comCall(dxgiDev1, dxgiDev1SetMaxFrameLatency, 1)
	release(dxgiDev1)
}

// factoryOf returns the IDXGIFactory2 that created dev's adapter, the
// factory a swap chain for dev must come from.
func factoryOf(dev uintptr) (uintptr, error) {
	dxgiDev, err := queryInterface(dev, &iidIDXGIDevice, "IDXGIDevice")
	if err != nil {
		return 0, err
	}
	defer release(dxgiDev)
	var adapter uintptr
	if err := hrError("IDXGIDevice.GetAdapter", comCall(dxgiDev, dxgiDevGetAdapter, uintptr(unsafe.Pointer(&adapter)))); err != nil {
		return 0, err
	}
	defer release(adapter)
	var factory uintptr
	// GetParent(riid, ppParent). IDXGIFactory2 needs Windows 8 or the
	// Windows 7 platform update.
	if err := hrError("IDXGIAdapter.GetParent(IDXGIFactory2)", comCall(adapter, dxgiObjGetParent, uintptr(unsafe.Pointer(&iidIDXGIFactory2)), uintptr(unsafe.Pointer(&factory)))); err != nil {
		return 0, err
	}
	return factory, nil
}

// swapChainModes are the swap effect and scaling createSwapChain tries,
// in order (a variable so tests can force FLIP_SEQUENTIAL). On Windows 10
// and later, the only systems Go 1.27 supports, the first always works.
var swapChainModes = []struct{ effect, scaling uint32 }{
	{swapEffectFlipDiscard, scalingNone},
	{swapEffectFlipSequential, scalingNone},    // Windows 8.x
	{swapEffectFlipSequential, scalingStretch}, // Windows 8.x without SCALING_NONE
}

// createSwapChain creates the window's swap chain (see above), sized to
// the window's client area, with the first of swapChainModes that works,
// and returns the swap effect it has.
func createSwapChain(factory, dev, hwnd uintptr) (uintptr, uint32, error) {
	var errs []error
	for _, mode := range swapChainModes {
		desc := swapChainDesc1{
			Width:       0, // the window's client size
			Height:      0,
			Format:      formatB8G8R8A8Unorm,
			SampleDesc:  sampleDesc{Count: 1},
			BufferUsage: usageRenderTargetOutput,
			BufferCount: 2,
			Scaling:     mode.scaling,
			SwapEffect:  mode.effect,
			AlphaMode:   alphaModeIgnore,
		}
		var swap uintptr
		// CreateSwapChainForHwnd(pDevice, hWnd, pDesc, pFullscreenDesc
		// (NULL: windowed), pRestrictToOutput, ppSwapChain)
		hr := checkHR("CreateSwapChainForHwnd", comCall(factory, fact2CreateSwapChainForHwnd, dev, hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0, uintptr(unsafe.Pointer(&swap))))
		err := hrError(fmt.Sprintf("CreateSwapChainForHwnd(effect %d, scaling %d)", mode.effect, mode.scaling), hr)
		if err == nil {
			return swap, mode.effect, nil
		}
		release(swap) // non-zero only when a test injected the failure
		if isDeviceLost(err) {
			return 0, 0, err
		}
		errs = append(errs, err)
	}
	return 0, 0, errors.Join(errs...)
}

// setBackground sets the swap chain's background color, shown where the
// window is larger than the back buffer (SCALING_NONE during a resize),
// to c as the window shows it (backgroundRGBA: the premultiplied color
// over black, converted to the linear values DXGI expects). Failing only
// leaves the previous color.
func (p *presenter) setBackground(c color.NRGBA) {
	bg := backgroundRGBA(c)
	// IDXGISwapChain1.SetBackgroundColor(pColor)
	if !hrFailed(comCall(p.swap, swap1SetBackgroundColor, uintptr(unsafe.Pointer(&bg)))) {
		p.background = c
	}
}

// releaseBackBuffer drops the presenter's references to the back buffer
// and unbinds it from the context, as ResizeBuffers requires.
func (p *presenter) releaseBackBuffer() {
	if p.backRTV != 0 || p.backTex != 0 {
		comCall(p.d.ctx, ctxOMSetRenderTargets, 0, 0, 0)
	}
	release(p.backRTV)
	release(p.backTex)
	p.backRTV, p.backTex = 0, 0
}

func (p *presenter) present(size image.Point, scale float32) error {
	d := p.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || p.released {
		return ErrClosed
	}
	if d.health.lost != nil {
		return d.health.lost
	}
	if size.X <= 0 || size.Y <= 0 {
		return nil // minimized or empty: nothing to show
	}
	size = presentSize(size, d.maxTex)
	resized := false
	if size != p.bufSize {
		p.releaseBackBuffer()
		comCall(d.ctx, ctxFlush) // let the runtime drop deferred references to the buffers
		// ResizeBuffers(BufferCount (0: keep), Width, Height, NewFormat
		// (UNKNOWN: keep), SwapChainFlags)
		hr := checkHR("ResizeBuffers", comCall(p.swap, swapResizeBuffers, 0, uintptr(size.X), uintptr(size.Y), formatUnknown, 0))
		p.resizes++
		if err := hrError("ResizeBuffers", hr); err != nil {
			return p.failed(err)
		}
		p.bufSize, resized = size, true
	}
	if !d.hasFrame || d.target.srv == 0 {
		return nil
	}
	if p.backRTV == 0 {
		var tex uintptr
		// GetBuffer(Buffer, riid, ppSurface)
		if err := hrError("IDXGISwapChain.GetBuffer", comCall(p.swap, swapGetBuffer, 0, uintptr(unsafe.Pointer(&iidID3D11Texture2D)), uintptr(unsafe.Pointer(&tex)))); err != nil {
			return p.failed(err)
		}
		var rtv uintptr
		if err := hrError("CreateRenderTargetView(back buffer)", comCall(d.dev, devCreateRenderTargetView, tex, 0, uintptr(unsafe.Pointer(&rtv)))); err != nil {
			release(tex)
			return p.failed(err)
		}
		p.backTex, p.backRTV = tex, rtv
	}
	if d.lastClear != p.background {
		p.setBackground(d.lastClear)
	}
	p.encodeCopy()
	if p.beforePresent != nil {
		p.beforePresent(p.backTex)
	}

	// The dirty-rectangle hint (presentDirtyRects): only on a
	// FLIP_SEQUENTIAL swap chain (never on FLIP_DISCARD, which rejects
	// partial presentation), for a partial frame right after the shown
	// one; the whole buffer was redrawn either way.
	var params presentParameters
	dirty := presentDirtyRects(p.effect, d.frameSeq, p.presentedSeq, d.lastFull, resized, d.lastDirty, d.target.size, p.bufSize)
	if len(dirty) > 0 {
		params.DirtyRectsCount = uint32(len(dirty))
		params.DirtyRects = &dirty[0]
	}
	// Present1(SyncInterval, PresentFlags, pPresentParameters). params
	// points at dirty (Go memory inside a structure): kept alive below.
	hr := checkHR("Present1", comCall(p.swap, swap1Present1, 1, 0, uintptr(unsafe.Pointer(&params))))
	runtime.KeepAlive(dirty)
	runtime.KeepAlive(&params)
	if err := hrError("Present1", hr); err != nil {
		return p.failed(err)
	}
	// S_OK, or DXGI_STATUS_OCCLUDED: a success code, but the frame was
	// not shown, so the next present must not hint relative to it.
	p.presentedSeq = d.frameSeq
	if uint32(hr) == dxgiStatusOccluded {
		p.presentedSeq = 0
	}
	p.presented++
	d.health.presentSucceeded()
	return nil
}

// failed counts a failed present (health.presentFailed: a counter of its
// own, which successful frames do not reset) and returns the error to
// report. As for frames, a device-lost HRESULT, or a removed reason the
// device reports after any other failed call, loses the device at once.
func (p *presenter) failed(err error) error {
	d := p.d
	if !isDeviceLost(err) {
		var he *hresultError
		if errors.As(err, &he) {
			if rerr := d.removed(); rerr != nil {
				d.hasFrame = false
				return d.health.lose(fmt.Errorf("%w (after %w)", rerr, err))
			}
		}
	} else {
		d.hasFrame = false
	}
	return d.health.presentFailed(err)
}

// encodeCopy draws the target into the back buffer (see above). It sets
// all the state it uses; Execute sets its own again.
func (p *presenter) encodeCopy() {
	d, ctx := p.d, p.d.ctx
	rtv := p.backRTV
	comCall(ctx, ctxOMSetRenderTargets, 1, uintptr(unsafe.Pointer(&rtv)), 0)
	vp := viewport{Width: float32(p.bufSize.X), Height: float32(p.bufSize.Y), MinDepth: 0, MaxDepth: 1}
	comCall(ctx, ctxRSSetViewports, 1, uintptr(unsafe.Pointer(&vp)))
	sc := rect{Right: int32(p.bufSize.X), Bottom: int32(p.bufSize.Y)}
	comCall(ctx, ctxRSSetScissorRects, 1, uintptr(unsafe.Pointer(&sc)))
	comCall(ctx, ctxRSSetState, d.pipe.raster) // no culling, scissor on (whole buffer)
	d.pipe.setBlend(ctx, false)
	comCall(ctx, ctxIASetInputLayout, 0) // no vertex input: SV_VertexID only
	comCall(ctx, ctxIASetPrimitiveTopology, primitiveTopologyTriangleList)
	comCall(ctx, ctxVSSetShader, p.vs, 0, 0)
	comCall(ctx, ctxPSSetShader, p.ps, 0, 0)
	srv := d.target.srv
	comCall(ctx, ctxPSSetShaderResources, 0, 1, uintptr(unsafe.Pointer(&srv)))
	// Draw(VertexCount, StartVertexLocation)
	comCall(ctx, ctxDraw, 3, 0)
	// Unbind the target, which the next frame renders into.
	var none uintptr
	comCall(ctx, ctxPSSetShaderResources, 0, 1, uintptr(unsafe.Pointer(&none)))
}

// Close releases the swap chain and the presenter's objects, then closes
// the Device. Safe to call more than once.
func (p *presenter) Close() {
	d := p.d
	d.mu.Lock()
	if !p.released {
		p.released = true
		if d.ctx != 0 {
			comCall(d.ctx, ctxClearState)
			comCall(d.ctx, ctxFlush)
		}
		release(p.backRTV)
		release(p.backTex)
		release(p.vs)
		release(p.ps)
		release(p.swap)
		p.backRTV, p.backTex, p.vs, p.ps, p.swap = 0, 0, 0, 0, 0
	}
	d.mu.Unlock()
	d.Close()
}
