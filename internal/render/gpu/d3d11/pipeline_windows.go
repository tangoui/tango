//go:build windows && (amd64 || arm64)

package d3d11

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/tangoui/tango/internal/render/gpu/gen"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// pipeline is the fixed state the quad shader draws with: shaders, input
// layout, blend, rasterizer and sampler states, and the FrameU constant
// buffer. Every field is a reference the pipeline owns.
type pipeline struct {
	vs, ps   uintptr // ID3D11VertexShader, ID3D11PixelShader
	layout   uintptr // ID3D11InputLayout
	blend    uintptr // ID3D11BlendState: premultiplied "over"
	noBlend  uintptr // ID3D11BlendState: blending off (clears dirty rectangles)
	raster   uintptr // ID3D11RasterizerState: solid, no culling, scissor on
	samplers []uintptr
	frameU   uintptr // ID3D11Buffer: the FrameU constant buffer (b0)
}

// newPipeline compiles gen.QuadHLSL for the device's feature level and
// creates the quad pipeline's state objects. On error everything created
// so far is released.
func newPipeline(dev uintptr, featureLevel uint32) (p *pipeline, err error) {
	vsTarget, psTarget := shaderProfiles(featureLevel)
	vsCode, err := compileShader(gen.QuadHLSL, gen.QuadVSEntry, vsTarget)
	if err != nil {
		return nil, err
	}
	psCode, err := compileShader(gen.QuadHLSL, gen.QuadPSEntry, psTarget)
	if err != nil {
		return nil, err
	}
	p = &pipeline{}
	// "return nil, err" sets p to nil before deferred calls run, so release
	// the pipeline through its own variable.
	built := p
	defer func() {
		if err != nil {
			built.release()
			p = nil
		}
	}()

	// CreateVertexShader(pShaderBytecode, BytecodeLength, pClassLinkage, ppVertexShader)
	hr := comCall(dev, devCreateVertexShader, uintptr(unsafe.Pointer(&vsCode[0])), uintptr(len(vsCode)), 0, uintptr(unsafe.Pointer(&p.vs)))
	if err := hrError("CreateVertexShader", hr); err != nil {
		p.vs = 0
		return nil, err
	}
	hr = comCall(dev, devCreatePixelShader, uintptr(unsafe.Pointer(&psCode[0])), uintptr(len(psCode)), 0, uintptr(unsafe.Pointer(&p.ps)))
	if err := hrError("CreatePixelShader", hr); err != nil {
		p.ps = 0
		return nil, err
	}
	if p.layout, err = newInputLayout(dev, vsCode); err != nil {
		return nil, err
	}

	// Premultiplied alpha "over": ONE, INV_SRC_ALPHA for color and alpha.
	var bd blendDesc
	bd.RenderTarget[0] = renderTargetBlendDesc{
		BlendEnable:           1,
		SrcBlend:              blendOne,
		DestBlend:             blendInvSrcAlpha,
		BlendOp:               blendOpAdd,
		SrcBlendAlpha:         blendOne,
		DestBlendAlpha:        blendInvSrcAlpha,
		BlendOpAlpha:          blendOpAdd,
		RenderTargetWriteMask: colorWriteAll,
	}
	hr = checkHR("CreateBlendState", comCall(dev, devCreateBlendState, uintptr(unsafe.Pointer(&bd)), uintptr(unsafe.Pointer(&p.blend))))
	if err := hrError("CreateBlendState", hr); err != nil {
		release(p.blend) // non-zero only when the call succeeded and a test injected the failure
		p.blend = 0
		return nil, err
	}

	// The clear pipeline of partial frames: the fragment replaces the
	// target's pixel (Metal's clearPipeline).
	var nd blendDesc
	nd.RenderTarget[0] = renderTargetBlendDesc{
		BlendEnable:           0,
		SrcBlend:              blendOne,
		DestBlend:             blendZero,
		BlendOp:               blendOpAdd,
		SrcBlendAlpha:         blendOne,
		DestBlendAlpha:        blendZero,
		BlendOpAlpha:          blendOpAdd,
		RenderTargetWriteMask: colorWriteAll,
	}
	hr = comCall(dev, devCreateBlendState, uintptr(unsafe.Pointer(&nd)), uintptr(unsafe.Pointer(&p.noBlend)))
	if err := hrError("CreateBlendState(no blending)", hr); err != nil {
		p.noBlend = 0
		return nil, err
	}

	rd := rasterizerDesc{
		FillMode:        fillSolid,
		CullMode:        cullNone,
		DepthClipEnable: 1,
		ScissorEnable:   1,
	}
	hr = comCall(dev, devCreateRasterizerState, uintptr(unsafe.Pointer(&rd)), uintptr(unsafe.Pointer(&p.raster)))
	if err := hrError("CreateRasterizerState", hr); err != nil {
		p.raster = 0
		return nil, err
	}

	// One clamp-to-edge sampler per texture binding, with the filter the
	// shader generator chose for it (gen.QuadTextures).
	p.samplers = make([]uintptr, len(gen.QuadTextures))
	for i, td := range gen.QuadTextures {
		if td.Binding != i {
			return nil, fmt.Errorf("d3d11: quad texture %d has binding %d", i, td.Binding)
		}
		filter, err := samplerFilter(td.Filter)
		if err != nil {
			return nil, err
		}
		sd := samplerDesc{
			Filter:         filter,
			AddressU:       textureAddressClamp,
			AddressV:       textureAddressClamp,
			AddressW:       textureAddressClamp,
			MaxAnisotropy:  1,
			ComparisonFunc: comparisonNever,
			MaxLOD:         float32Max,
		}
		var s uintptr
		hr = comCall(dev, devCreateSamplerState, uintptr(unsafe.Pointer(&sd)), uintptr(unsafe.Pointer(&s)))
		if err := hrError("CreateSamplerState", hr); err != nil {
			return nil, err
		}
		p.samplers[i] = s
	}

	// FrameU: 16 bytes (ViewSize, Origin), updated with UpdateSubresource,
	// so a DEFAULT buffer.
	cd := bufferDesc{
		ByteWidth: uint32(unsafe.Sizeof(shaders.Frame{})),
		Usage:     usageDefault,
		BindFlags: bindConstantBuffer,
	}
	hr = comCall(dev, devCreateBuffer, uintptr(unsafe.Pointer(&cd)), 0, uintptr(unsafe.Pointer(&p.frameU)))
	if err := hrError("CreateBuffer(FrameU)", hr); err != nil {
		p.frameU = 0
		return nil, err
	}
	runtime.KeepAlive(vsCode)
	runtime.KeepAlive(psCode)
	return p, nil
}

// newInputLayout creates the input layout of the per-instance vertex
// buffer (slot 0, stride gen.QuadInstanceStride) from
// gen.QuadInstanceAttrs: attribute at location n is TEXCOORDn of the
// shader's Instance struct.
func newInputLayout(dev uintptr, vsCode []byte) (uintptr, error) {
	semantic := cstr("TEXCOORD")
	elems := make([]inputElementDesc, len(gen.QuadInstanceAttrs))
	for i, at := range gen.QuadInstanceAttrs {
		format, err := inputFormat(at.Kind, at.N)
		if err != nil {
			return 0, fmt.Errorf("%w (attribute %d)", err, at.Location)
		}
		if at.Offset+4*at.N > gen.QuadInstanceStride {
			return 0, fmt.Errorf("d3d11: attribute %d at offset %d overflows the %d-byte instance", at.Location, at.Offset, gen.QuadInstanceStride)
		}
		elems[i] = inputElementDesc{
			SemanticName:         &semantic[0],
			SemanticIndex:        uint32(at.Location),
			Format:               format,
			InputSlot:            0,
			AlignedByteOffset:    uint32(at.Offset),
			InputSlotClass:       inputPerInstanceData,
			InstanceDataStepRate: 1,
		}
	}
	var layout uintptr
	// CreateInputLayout(pInputElementDescs, NumElements,
	// pShaderBytecodeWithInputSignature, BytecodeLength, ppInputLayout).
	// The descriptors point at semantic (Go memory inside a structure):
	// kept alive explicitly below.
	hr := comCall(dev, devCreateInputLayout,
		uintptr(unsafe.Pointer(&elems[0])), uintptr(len(elems)),
		uintptr(unsafe.Pointer(&vsCode[0])), uintptr(len(vsCode)),
		uintptr(unsafe.Pointer(&layout)))
	runtime.KeepAlive(semantic)
	runtime.KeepAlive(elems)
	runtime.KeepAlive(vsCode)
	if err := hrError("CreateInputLayout", hr); err != nil {
		return 0, err
	}
	return layout, nil
}

// bind sets the pipeline on the immediate context ctx: input assembly
// (layout, triangle strip), shaders, FrameU at b0 for both stages, the
// samplers, blend and rasterizer state. The instance buffer, textures,
// render target, viewport and scissor are set by the caller.
func (p *pipeline) bind(ctx uintptr) {
	comCall(ctx, ctxIASetInputLayout, p.layout)
	comCall(ctx, ctxIASetPrimitiveTopology, primitiveTopologyTriangleStrip)
	comCall(ctx, ctxVSSetShader, p.vs, 0, 0)
	comCall(ctx, ctxPSSetShader, p.ps, 0, 0)
	cb := p.frameU
	comCall(ctx, ctxVSSetConstantBuffers, 0, 1, uintptr(unsafe.Pointer(&cb)))
	comCall(ctx, ctxPSSetConstantBuffers, 0, 1, uintptr(unsafe.Pointer(&cb)))
	comCall(ctx, ctxPSSetSamplers, 0, uintptr(len(p.samplers)), uintptr(unsafe.Pointer(&p.samplers[0])))
	p.setBlend(ctx, true)
	comCall(ctx, ctxRSSetState, p.raster)
}

// setBlend selects premultiplied "over" blending, or none (the clear
// pipeline of partial frames).
func (p *pipeline) setBlend(ctx uintptr, blend bool) {
	bs := p.blend
	if !blend {
		bs = p.noBlend
	}
	// OMSetBlendState(pBlendState, BlendFactor (NULL: {1, 1, 1, 1},
	// unused by these factors), SampleMask)
	comCall(ctx, ctxOMSetBlendState, bs, 0, 0xffffffff)
}

// setFrameU writes the FrameU constant buffer. UpdateSubresource is queued
// in command order: draws issued before it keep the old values.
func (p *pipeline) setFrameU(ctx uintptr, fu shaders.Frame) {
	// UpdateSubresource(pDstResource, DstSubresource, pDstBox (NULL: the
	// whole constant buffer, the only form allowed), pSrcData,
	// SrcRowPitch, SrcDepthPitch). The data is copied before it returns.
	comCall(ctx, ctxUpdateSubresource, p.frameU, 0, 0, uintptr(unsafe.Pointer(&fu)), 0, 0)
}

// release drops every reference the pipeline holds. Safe on a partially
// built pipeline.
func (p *pipeline) release() {
	if p == nil {
		return
	}
	for i, s := range p.samplers {
		release(s)
		p.samplers[i] = 0
	}
	for _, o := range []*uintptr{&p.frameU, &p.raster, &p.noBlend, &p.blend, &p.layout, &p.ps, &p.vs} {
		release(*o)
		*o = 0
	}
}

// ---- Instance buffer ----

// instBuffer is the dynamic per-instance vertex buffer, allocated as a
// ring (instRing, plan.go).
type instBuffer struct {
	buf  uintptr // ID3D11Buffer, DYNAMIC, VERTEX_BUFFER, CPU write
	ring instRing
}

// write copies the instances of every batch of batches, back to back,
// into the buffer and returns the byte offset of the first; the caller
// sets the vertex buffer at that offset plus 144 bytes per earlier
// instance. n is their total count (> 0).
func (ib *instBuffer) write(dev, ctx uintptr, batches [][]shaders.Instance, n int) (off int, err error) {
	a, err := ib.ring.alloc(n * gen.QuadInstanceStride)
	if err != nil {
		return 0, err
	}
	if a.grow > 0 {
		release(ib.buf)
		ib.buf = 0
		bd := bufferDesc{
			ByteWidth:      uint32(a.grow),
			Usage:          usageDynamic,
			BindFlags:      bindVertexBuffer,
			CPUAccessFlags: cpuAccessWrite,
		}
		var buf uintptr
		hr := checkHR("CreateBuffer", comCall(dev, devCreateBuffer, uintptr(unsafe.Pointer(&bd)), 0, uintptr(unsafe.Pointer(&buf))))
		if err := hrError(fmt.Sprintf("CreateBuffer(instances, %d bytes)", a.grow), hr); err != nil {
			// A failed CreateBuffer leaves buf 0; it is non-zero only when
			// the call succeeded and a test injected the failure.
			release(buf)
			ib.ring.reset()
			return 0, err
		}
		ib.buf = buf
	}
	mapType := uintptr(mapWriteNoOverwrite)
	if a.discard {
		mapType = mapWriteDiscard
	}
	var m mappedSubresource
	// Map(pResource, Subresource, MapType, MapFlags, pMappedResource)
	hr := checkHR("Map", comCall(ctx, ctxMap, ib.buf, 0, mapType, 0, uintptr(unsafe.Pointer(&m))))
	if err := hrError("Map(instances)", hr); err != nil {
		if m.Data != 0 {
			comCall(ctx, ctxUnmap, ib.buf, 0) // mapped for real; the failure was injected
		}
		ib.ring.reset()
		release(ib.buf)
		ib.buf = 0
		return 0, err
	}
	if m.Data == 0 {
		comCall(ctx, ctxUnmap, ib.buf, 0)
		ib.ring.reset()
		release(ib.buf)
		ib.buf = 0
		return 0, fmt.Errorf("d3d11: Map(instances) returned no memory")
	}
	dst := unsafe.Slice((*shaders.Instance)(foreign(m.Data+uintptr(a.off))), n)
	k := 0
	for _, ins := range batches {
		k += copy(dst[k:], ins)
	}
	comCall(ctx, ctxUnmap, ib.buf, 0)
	return a.off, nil
}

func (ib *instBuffer) release() {
	release(ib.buf)
	ib.buf = 0
	ib.ring.reset()
}
