package d3d11

import "unsafe"

// The binary interface of Direct3D 11 and DXGI used by this package:
// vtable layouts, structures and constants, transcribed from the Windows
// SDK headers d3d11.h, d3dcommon.h, dxgi.h, dxgi1_2.h, dxgiformat.h,
// dxgicommon.h, d3dcompiler.h and winerror.h (checked against the
// mingw-w64 copies of those headers, which list the same declarations in
// the same order).
//
// This file builds on every platform so that abi_test.go can check slot
// numbers and structure layouts on the development machine; only the
// *_windows.go files use it.

// ---- Vtables ----
//
// A COM interface pointer addresses an object whose first word points at
// its vtable, an array of function pointers in the order the interface
// (and the interfaces it derives from) declare their methods. Each vt*
// type below spells that order out as one uintptr field per method, so a
// slot is found by naming the method: slot = field offset / slotSize.
// Derived interfaces embed their base, exactly as the SDK's C vtables
// repeat the base methods first.

// slotSize is the size of one vtable entry.
const slotSize = unsafe.Sizeof(uintptr(0))

// vtIUnknown is the start of every vtable (unknwn.h).
type vtIUnknown struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

// vtID3D11DeviceChild (d3d11.h) is the base of every device object.
type vtID3D11DeviceChild struct {
	vtIUnknown
	GetDevice               uintptr
	GetPrivateData          uintptr
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
}

// vtID3D11Device (d3d11.h).
type vtID3D11Device struct {
	vtIUnknown
	CreateBuffer                         uintptr
	CreateTexture1D                      uintptr
	CreateTexture2D                      uintptr
	CreateTexture3D                      uintptr
	CreateShaderResourceView             uintptr
	CreateUnorderedAccessView            uintptr
	CreateRenderTargetView               uintptr
	CreateDepthStencilView               uintptr
	CreateInputLayout                    uintptr
	CreateVertexShader                   uintptr
	CreateGeometryShader                 uintptr
	CreateGeometryShaderWithStreamOutput uintptr
	CreatePixelShader                    uintptr
	CreateHullShader                     uintptr
	CreateDomainShader                   uintptr
	CreateComputeShader                  uintptr
	CreateClassLinkage                   uintptr
	CreateBlendState                     uintptr
	CreateDepthStencilState              uintptr
	CreateRasterizerState                uintptr
	CreateSamplerState                   uintptr
	CreateQuery                          uintptr
	CreatePredicate                      uintptr
	CreateCounter                        uintptr
	CreateDeferredContext                uintptr
	OpenSharedResource                   uintptr
	CheckFormatSupport                   uintptr
	CheckMultisampleQualityLevels        uintptr
	CheckCounterInfo                     uintptr
	CheckCounter                         uintptr
	CheckFeatureSupport                  uintptr
	GetPrivateData                       uintptr
	SetPrivateData                       uintptr
	SetPrivateDataInterface              uintptr
	GetFeatureLevel                      uintptr
	GetCreationFlags                     uintptr
	GetDeviceRemovedReason               uintptr
	GetImmediateContext                  uintptr
	SetExceptionMode                     uintptr
	GetExceptionMode                     uintptr
}

// vtID3D11DeviceContext (d3d11.h).
type vtID3D11DeviceContext struct {
	vtID3D11DeviceChild
	VSSetConstantBuffers                      uintptr
	PSSetShaderResources                      uintptr
	PSSetShader                               uintptr
	PSSetSamplers                             uintptr
	VSSetShader                               uintptr
	DrawIndexed                               uintptr
	Draw                                      uintptr
	Map                                       uintptr
	Unmap                                     uintptr
	PSSetConstantBuffers                      uintptr
	IASetInputLayout                          uintptr
	IASetVertexBuffers                        uintptr
	IASetIndexBuffer                          uintptr
	DrawIndexedInstanced                      uintptr
	DrawInstanced                             uintptr
	GSSetConstantBuffers                      uintptr
	GSSetShader                               uintptr
	IASetPrimitiveTopology                    uintptr
	VSSetShaderResources                      uintptr
	VSSetSamplers                             uintptr
	Begin                                     uintptr
	End                                       uintptr
	GetData                                   uintptr
	SetPredication                            uintptr
	GSSetShaderResources                      uintptr
	GSSetSamplers                             uintptr
	OMSetRenderTargets                        uintptr
	OMSetRenderTargetsAndUnorderedAccessViews uintptr
	OMSetBlendState                           uintptr
	OMSetDepthStencilState                    uintptr
	SOSetTargets                              uintptr
	DrawAuto                                  uintptr
	DrawIndexedInstancedIndirect              uintptr
	DrawInstancedIndirect                     uintptr
	Dispatch                                  uintptr
	DispatchIndirect                          uintptr
	RSSetState                                uintptr
	RSSetViewports                            uintptr
	RSSetScissorRects                         uintptr
	CopySubresourceRegion                     uintptr
	CopyResource                              uintptr
	UpdateSubresource                         uintptr
	CopyStructureCount                        uintptr
	ClearRenderTargetView                     uintptr
	ClearUnorderedAccessViewUint              uintptr
	ClearUnorderedAccessViewFloat             uintptr
	ClearDepthStencilView                     uintptr
	GenerateMips                              uintptr
	SetResourceMinLOD                         uintptr
	GetResourceMinLOD                         uintptr
	ResolveSubresource                        uintptr
	ExecuteCommandList                        uintptr
	HSSetShaderResources                      uintptr
	HSSetShader                               uintptr
	HSSetSamplers                             uintptr
	HSSetConstantBuffers                      uintptr
	DSSetShaderResources                      uintptr
	DSSetShader                               uintptr
	DSSetSamplers                             uintptr
	DSSetConstantBuffers                      uintptr
	CSSetShaderResources                      uintptr
	CSSetUnorderedAccessViews                 uintptr
	CSSetShader                               uintptr
	CSSetSamplers                             uintptr
	CSSetConstantBuffers                      uintptr
	VSGetConstantBuffers                      uintptr
	PSGetShaderResources                      uintptr
	PSGetShader                               uintptr
	PSGetSamplers                             uintptr
	VSGetShader                               uintptr
	PSGetConstantBuffers                      uintptr
	IAGetInputLayout                          uintptr
	IAGetVertexBuffers                        uintptr
	IAGetIndexBuffer                          uintptr
	GSGetConstantBuffers                      uintptr
	GSGetShader                               uintptr
	IAGetPrimitiveTopology                    uintptr
	VSGetShaderResources                      uintptr
	VSGetSamplers                             uintptr
	GetPredication                            uintptr
	GSGetShaderResources                      uintptr
	GSGetSamplers                             uintptr
	OMGetRenderTargets                        uintptr
	OMGetRenderTargetsAndUnorderedAccessViews uintptr
	OMGetBlendState                           uintptr
	OMGetDepthStencilState                    uintptr
	SOGetTargets                              uintptr
	RSGetState                                uintptr
	RSGetViewports                            uintptr
	RSGetScissorRects                         uintptr
	HSGetShaderResources                      uintptr
	HSGetShader                               uintptr
	HSGetSamplers                             uintptr
	HSGetConstantBuffers                      uintptr
	DSGetShaderResources                      uintptr
	DSGetShader                               uintptr
	DSGetSamplers                             uintptr
	DSGetConstantBuffers                      uintptr
	CSGetShaderResources                      uintptr
	CSGetUnorderedAccessViews                 uintptr
	CSGetShader                               uintptr
	CSGetSamplers                             uintptr
	CSGetConstantBuffers                      uintptr
	ClearState                                uintptr
	Flush                                     uintptr
	GetType                                   uintptr
	GetContextFlags                           uintptr
	FinishCommandList                         uintptr
}

// vtID3D11Resource (d3d11.h) is the base of buffers and textures.
type vtID3D11Resource struct {
	vtID3D11DeviceChild
	GetType             uintptr
	SetEvictionPriority uintptr
	GetEvictionPriority uintptr
}

// vtID3D11Texture2D (d3d11.h); ID3D11Buffer has the same shape.
type vtID3D11Texture2D struct {
	vtID3D11Resource
	GetDesc uintptr
}

// vtID3D11View (d3d11.h) is the base of shader-resource and render-target
// views.
type vtID3D11View struct {
	vtID3D11DeviceChild
	GetResource uintptr
}

// vtIDXGIObject (dxgi.h) is the base of every DXGI interface.
type vtIDXGIObject struct {
	vtIUnknown
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr
}

// vtIDXGIDevice (dxgi.h), obtained from the D3D11 device with
// QueryInterface.
type vtIDXGIDevice struct {
	vtIDXGIObject
	GetAdapter             uintptr
	CreateSurface          uintptr
	QueryResourceResidency uintptr
	SetGPUThreadPriority   uintptr
	GetGPUThreadPriority   uintptr
}

// vtIDXGIDevice1 (dxgi.h): IDXGIDevice plus the frame latency control.
type vtIDXGIDevice1 struct {
	vtIDXGIDevice
	SetMaximumFrameLatency uintptr
	GetMaximumFrameLatency uintptr
}

// vtIDXGIAdapter (dxgi.h).
type vtIDXGIAdapter struct {
	vtIDXGIObject
	EnumOutputs           uintptr
	GetDesc               uintptr
	CheckInterfaceSupport uintptr
}

// vtIDXGIFactory (dxgi.h).
type vtIDXGIFactory struct {
	vtIDXGIObject
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr
}

// vtIDXGIFactory1 (dxgi.h).
type vtIDXGIFactory1 struct {
	vtIDXGIFactory
	EnumAdapters1 uintptr
	IsCurrent     uintptr
}

// vtIDXGIFactory2 (dxgi1_2.h).
type vtIDXGIFactory2 struct {
	vtIDXGIFactory1
	IsWindowedStereoEnabled       uintptr
	CreateSwapChainForHwnd        uintptr
	CreateSwapChainForCoreWindow  uintptr
	GetSharedResourceAdapterLuid  uintptr
	RegisterStereoStatusWindow    uintptr
	RegisterStereoStatusEvent     uintptr
	UnregisterStereoStatus        uintptr
	RegisterOcclusionStatusWindow uintptr
	RegisterOcclusionStatusEvent  uintptr
	UnregisterOcclusionStatus     uintptr
	CreateSwapChainForComposition uintptr
}

// vtIDXGIDeviceSubObject (dxgi.h).
type vtIDXGIDeviceSubObject struct {
	vtIDXGIObject
	GetDevice uintptr
}

// vtIDXGISwapChain (dxgi.h).
type vtIDXGISwapChain struct {
	vtIDXGIDeviceSubObject
	Present             uintptr
	GetBuffer           uintptr
	SetFullscreenState  uintptr
	GetFullscreenState  uintptr
	GetDesc             uintptr
	ResizeBuffers       uintptr
	ResizeTarget        uintptr
	GetContainingOutput uintptr
	GetFrameStatistics  uintptr
	GetLastPresentCount uintptr
}

// vtIDXGISwapChain1 (dxgi1_2.h).
type vtIDXGISwapChain1 struct {
	vtIDXGISwapChain
	GetDesc1                 uintptr
	GetFullscreenDesc        uintptr
	GetHwnd                  uintptr
	GetCoreWindow            uintptr
	Present1                 uintptr
	IsTemporaryMonoSupported uintptr
	GetRestrictToOutput      uintptr
	SetBackgroundColor       uintptr
	GetBackgroundColor       uintptr
	SetRotation              uintptr
	GetRotation              uintptr
}

// vtID3DBlob is ID3D10Blob (d3dcommon.h), the buffer D3DCompile returns
// bytecode and messages in.
type vtID3DBlob struct {
	vtIUnknown
	GetBufferPointer uintptr
	GetBufferSize    uintptr
}

// Vtable slots used by the device and presenter, named <interface><Method>.
const (
	unkQueryInterface = int(unsafe.Offsetof(vtIUnknown{}.QueryInterface) / slotSize)
	unkAddRef         = int(unsafe.Offsetof(vtIUnknown{}.AddRef) / slotSize)
	unkRelease        = int(unsafe.Offsetof(vtIUnknown{}.Release) / slotSize)

	devCreateBuffer             = int(unsafe.Offsetof(vtID3D11Device{}.CreateBuffer) / slotSize)
	devCreateTexture2D          = int(unsafe.Offsetof(vtID3D11Device{}.CreateTexture2D) / slotSize)
	devCreateShaderResourceView = int(unsafe.Offsetof(vtID3D11Device{}.CreateShaderResourceView) / slotSize)
	devCreateRenderTargetView   = int(unsafe.Offsetof(vtID3D11Device{}.CreateRenderTargetView) / slotSize)
	devCreateInputLayout        = int(unsafe.Offsetof(vtID3D11Device{}.CreateInputLayout) / slotSize)
	devCreateVertexShader       = int(unsafe.Offsetof(vtID3D11Device{}.CreateVertexShader) / slotSize)
	devCreatePixelShader        = int(unsafe.Offsetof(vtID3D11Device{}.CreatePixelShader) / slotSize)
	devCreateBlendState         = int(unsafe.Offsetof(vtID3D11Device{}.CreateBlendState) / slotSize)
	devCreateRasterizerState    = int(unsafe.Offsetof(vtID3D11Device{}.CreateRasterizerState) / slotSize)
	devCreateSamplerState       = int(unsafe.Offsetof(vtID3D11Device{}.CreateSamplerState) / slotSize)
	devGetFeatureLevel          = int(unsafe.Offsetof(vtID3D11Device{}.GetFeatureLevel) / slotSize)
	devGetDeviceRemovedReason   = int(unsafe.Offsetof(vtID3D11Device{}.GetDeviceRemovedReason) / slotSize)
	devGetImmediateContext      = int(unsafe.Offsetof(vtID3D11Device{}.GetImmediateContext) / slotSize)

	ctxVSSetConstantBuffers   = int(unsafe.Offsetof(vtID3D11DeviceContext{}.VSSetConstantBuffers) / slotSize)
	ctxPSSetShaderResources   = int(unsafe.Offsetof(vtID3D11DeviceContext{}.PSSetShaderResources) / slotSize)
	ctxPSSetShader            = int(unsafe.Offsetof(vtID3D11DeviceContext{}.PSSetShader) / slotSize)
	ctxPSSetSamplers          = int(unsafe.Offsetof(vtID3D11DeviceContext{}.PSSetSamplers) / slotSize)
	ctxVSSetShader            = int(unsafe.Offsetof(vtID3D11DeviceContext{}.VSSetShader) / slotSize)
	ctxDraw                   = int(unsafe.Offsetof(vtID3D11DeviceContext{}.Draw) / slotSize)
	ctxMap                    = int(unsafe.Offsetof(vtID3D11DeviceContext{}.Map) / slotSize)
	ctxUnmap                  = int(unsafe.Offsetof(vtID3D11DeviceContext{}.Unmap) / slotSize)
	ctxPSSetConstantBuffers   = int(unsafe.Offsetof(vtID3D11DeviceContext{}.PSSetConstantBuffers) / slotSize)
	ctxIASetInputLayout       = int(unsafe.Offsetof(vtID3D11DeviceContext{}.IASetInputLayout) / slotSize)
	ctxIASetVertexBuffers     = int(unsafe.Offsetof(vtID3D11DeviceContext{}.IASetVertexBuffers) / slotSize)
	ctxDrawInstanced          = int(unsafe.Offsetof(vtID3D11DeviceContext{}.DrawInstanced) / slotSize)
	ctxIASetPrimitiveTopology = int(unsafe.Offsetof(vtID3D11DeviceContext{}.IASetPrimitiveTopology) / slotSize)
	ctxOMSetRenderTargets     = int(unsafe.Offsetof(vtID3D11DeviceContext{}.OMSetRenderTargets) / slotSize)
	ctxOMSetBlendState        = int(unsafe.Offsetof(vtID3D11DeviceContext{}.OMSetBlendState) / slotSize)
	ctxRSSetState             = int(unsafe.Offsetof(vtID3D11DeviceContext{}.RSSetState) / slotSize)
	ctxRSSetViewports         = int(unsafe.Offsetof(vtID3D11DeviceContext{}.RSSetViewports) / slotSize)
	ctxRSSetScissorRects      = int(unsafe.Offsetof(vtID3D11DeviceContext{}.RSSetScissorRects) / slotSize)
	ctxCopySubresourceRegion  = int(unsafe.Offsetof(vtID3D11DeviceContext{}.CopySubresourceRegion) / slotSize)
	ctxCopyResource           = int(unsafe.Offsetof(vtID3D11DeviceContext{}.CopyResource) / slotSize)
	ctxUpdateSubresource      = int(unsafe.Offsetof(vtID3D11DeviceContext{}.UpdateSubresource) / slotSize)
	ctxClearRenderTargetView  = int(unsafe.Offsetof(vtID3D11DeviceContext{}.ClearRenderTargetView) / slotSize)
	ctxClearState             = int(unsafe.Offsetof(vtID3D11DeviceContext{}.ClearState) / slotSize)
	ctxFlush                  = int(unsafe.Offsetof(vtID3D11DeviceContext{}.Flush) / slotSize)

	tex2DGetDesc    = int(unsafe.Offsetof(vtID3D11Texture2D{}.GetDesc) / slotSize)
	viewGetResource = int(unsafe.Offsetof(vtID3D11View{}.GetResource) / slotSize)

	dxgiObjGetParent            = int(unsafe.Offsetof(vtIDXGIObject{}.GetParent) / slotSize)
	dxgiDevGetAdapter           = int(unsafe.Offsetof(vtIDXGIDevice{}.GetAdapter) / slotSize)
	dxgiDev1SetMaxFrameLatency  = int(unsafe.Offsetof(vtIDXGIDevice1{}.SetMaximumFrameLatency) / slotSize)
	dxgiAdapterGetDesc          = int(unsafe.Offsetof(vtIDXGIAdapter{}.GetDesc) / slotSize)
	factMakeWindowAssoc         = int(unsafe.Offsetof(vtIDXGIFactory2{}.MakeWindowAssociation) / slotSize)
	fact2CreateSwapChainForHwnd = int(unsafe.Offsetof(vtIDXGIFactory2{}.CreateSwapChainForHwnd) / slotSize)

	swapPresent             = int(unsafe.Offsetof(vtIDXGISwapChain1{}.Present) / slotSize)
	swapGetBuffer           = int(unsafe.Offsetof(vtIDXGISwapChain1{}.GetBuffer) / slotSize)
	swapResizeBuffers       = int(unsafe.Offsetof(vtIDXGISwapChain1{}.ResizeBuffers) / slotSize)
	swap1Present1           = int(unsafe.Offsetof(vtIDXGISwapChain1{}.Present1) / slotSize)
	swap1GetDesc1           = int(unsafe.Offsetof(vtIDXGISwapChain1{}.GetDesc1) / slotSize)
	swap1SetBackgroundColor = int(unsafe.Offsetof(vtIDXGISwapChain1{}.SetBackgroundColor) / slotSize)

	blobGetBufferPointer = int(unsafe.Offsetof(vtID3DBlob{}.GetBufferPointer) / slotSize)
	blobGetBufferSize    = int(unsafe.Offsetof(vtID3DBlob{}.GetBufferSize) / slotSize)
)

// ---- Interface identifiers ----

// guid is a COM GUID (guiddef.h).
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidID3D11Device        = guid{0xdb6f6ddb, 0xac77, 0x4e88, [8]byte{0x82, 0x53, 0x81, 0x9d, 0xf9, 0xbb, 0xf1, 0x40}}
	iidID3D11DeviceContext = guid{0xc0bfa96c, 0xe089, 0x44fb, [8]byte{0x8e, 0xaf, 0x26, 0xf8, 0x79, 0x61, 0x90, 0xda}}
	iidID3D11Resource      = guid{0xdc8e63f3, 0xd12b, 0x4952, [8]byte{0xb4, 0x7b, 0x5e, 0x45, 0x02, 0x6a, 0x86, 0x2d}}
	iidID3D11Texture2D     = guid{0x6f15aaf2, 0xd208, 0x4e89, [8]byte{0x9a, 0xb4, 0x48, 0x95, 0x35, 0xd3, 0x4f, 0x9c}}
	iidIDXGIDevice         = guid{0x54ec77fa, 0x1377, 0x44e6, [8]byte{0x8c, 0x32, 0x88, 0xfd, 0x5f, 0x44, 0xc8, 0x4c}}
	iidIDXGIDevice1        = guid{0x77db970f, 0x6276, 0x48ba, [8]byte{0xba, 0x28, 0x07, 0x01, 0x43, 0xb4, 0x39, 0x2c}}
	iidIDXGIAdapter        = guid{0x2411e7e1, 0x12ac, 0x4ccf, [8]byte{0xbd, 0x14, 0x97, 0x98, 0xe8, 0x53, 0x4d, 0xc0}}
	iidIDXGIFactory2       = guid{0x50c83a1c, 0xe072, 0x4c48, [8]byte{0x87, 0xb0, 0x36, 0x30, 0xfa, 0x36, 0xa6, 0xd0}}
	iidIDXGISwapChain1     = guid{0x790a45f7, 0x0d42, 0x4876, [8]byte{0x98, 0x3a, 0x0a, 0x55, 0xcf, 0xe6, 0xf4, 0xaa}}
)

// ---- Structures ----
//
// Field for field the SDK structures, with the SDK field names. Every SDK
// enum, UINT, INT, BOOL (WINBOOL), LONG and FLOAT is 4 bytes; pointers and
// SIZE_T are 8 bytes on both supported architectures. Pointers to memory
// the caller provides are unsafe.Pointer (or a typed pointer) so the Go
// garbage collector sees them; pointers the driver returns are uintptr.

// sampleDesc is DXGI_SAMPLE_DESC (dxgicommon.h).
type sampleDesc struct {
	Count   uint32
	Quality uint32
}

// inputElementDesc is D3D11_INPUT_ELEMENT_DESC.
type inputElementDesc struct {
	SemanticName         *byte // NUL-terminated ASCII
	SemanticIndex        uint32
	Format               uint32 // DXGI_FORMAT
	InputSlot            uint32
	AlignedByteOffset    uint32
	InputSlotClass       uint32 // D3D11_INPUT_CLASSIFICATION
	InstanceDataStepRate uint32
}

// texture2DDesc is D3D11_TEXTURE2D_DESC.
type texture2DDesc struct {
	Width          uint32
	Height         uint32
	MipLevels      uint32
	ArraySize      uint32
	Format         uint32 // DXGI_FORMAT
	SampleDesc     sampleDesc
	Usage          uint32 // D3D11_USAGE
	BindFlags      uint32
	CPUAccessFlags uint32
	MiscFlags      uint32
}

// bufferDesc is D3D11_BUFFER_DESC.
type bufferDesc struct {
	ByteWidth           uint32
	Usage               uint32 // D3D11_USAGE
	BindFlags           uint32
	CPUAccessFlags      uint32
	MiscFlags           uint32
	StructureByteStride uint32
}

// subresourceData is D3D11_SUBRESOURCE_DATA.
type subresourceData struct {
	SysMem           unsafe.Pointer
	SysMemPitch      uint32
	SysMemSlicePitch uint32
}

// mappedSubresource is D3D11_MAPPED_SUBRESOURCE; Data points at driver
// memory.
type mappedSubresource struct {
	Data       uintptr
	RowPitch   uint32
	DepthPitch uint32
}

// renderTargetBlendDesc is D3D11_RENDER_TARGET_BLEND_DESC.
type renderTargetBlendDesc struct {
	BlendEnable           int32  // BOOL
	SrcBlend              uint32 // D3D11_BLEND
	DestBlend             uint32
	BlendOp               uint32 // D3D11_BLEND_OP
	SrcBlendAlpha         uint32
	DestBlendAlpha        uint32
	BlendOpAlpha          uint32
	RenderTargetWriteMask uint8
}

// blendDesc is D3D11_BLEND_DESC.
type blendDesc struct {
	AlphaToCoverageEnable  int32 // BOOL
	IndependentBlendEnable int32 // BOOL
	RenderTarget           [8]renderTargetBlendDesc
}

// samplerDesc is D3D11_SAMPLER_DESC.
type samplerDesc struct {
	Filter         uint32 // D3D11_FILTER
	AddressU       uint32 // D3D11_TEXTURE_ADDRESS_MODE
	AddressV       uint32
	AddressW       uint32
	MipLODBias     float32
	MaxAnisotropy  uint32
	ComparisonFunc uint32 // D3D11_COMPARISON_FUNC
	BorderColor    [4]float32
	MinLOD         float32
	MaxLOD         float32
}

// rasterizerDesc is D3D11_RASTERIZER_DESC.
type rasterizerDesc struct {
	FillMode              uint32 // D3D11_FILL_MODE
	CullMode              uint32 // D3D11_CULL_MODE
	FrontCounterClockwise int32  // BOOL
	DepthBias             int32
	DepthBiasClamp        float32
	SlopeScaledDepthBias  float32
	DepthClipEnable       int32 // BOOL
	ScissorEnable         int32 // BOOL
	MultisampleEnable     int32 // BOOL
	AntialiasedLineEnable int32 // BOOL
}

// viewport is D3D11_VIEWPORT.
type viewport struct {
	TopLeftX float32
	TopLeftY float32
	Width    float32
	Height   float32
	MinDepth float32
	MaxDepth float32
}

// rect is RECT (windef.h), which D3D11_RECT is a typedef of and
// DXGI_PRESENT_PARAMETERS points at.
type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// box is D3D11_BOX.
type box struct {
	Left   uint32
	Top    uint32
	Front  uint32
	Right  uint32
	Bottom uint32
	Back   uint32
}

// swapChainDesc1 is DXGI_SWAP_CHAIN_DESC1 (dxgi1_2.h).
type swapChainDesc1 struct {
	Width       uint32
	Height      uint32
	Format      uint32 // DXGI_FORMAT
	Stereo      int32  // BOOL
	SampleDesc  sampleDesc
	BufferUsage uint32 // DXGI_USAGE
	BufferCount uint32
	Scaling     uint32 // DXGI_SCALING
	SwapEffect  uint32 // DXGI_SWAP_EFFECT
	AlphaMode   uint32 // DXGI_ALPHA_MODE
	Flags       uint32
}

// dxgiRGBA is DXGI_RGBA (dxgitype.h), D3DCOLORVALUE: four floats.
type dxgiRGBA struct {
	R, G, B, A float32
}

// point is POINT (windef.h).
type point struct {
	X int32
	Y int32
}

// presentParameters is DXGI_PRESENT_PARAMETERS (dxgi1_2.h).
type presentParameters struct {
	DirtyRectsCount uint32
	DirtyRects      *rect
	ScrollRect      *rect
	ScrollOffset    *point
}

// adapterDesc is DXGI_ADAPTER_DESC (dxgi.h).
type adapterDesc struct {
	Description           [128]uint16 // WCHAR, NUL-terminated
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr // SIZE_T
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           struct {
		LowPart  uint32
		HighPart int32
	}
}

// ---- Constants ----

// D3D_DRIVER_TYPE (d3dcommon.h).
const (
	driverTypeHardware = 1
	driverTypeWARP     = 5
)

// D3D_FEATURE_LEVEL (d3dcommon.h).
const (
	featureLevel10_0 = 0xa000
	featureLevel10_1 = 0xa100
	featureLevel11_0 = 0xb000
	featureLevel11_1 = 0xb100
)

// D3D11CreateDevice (d3d11.h).
const (
	sdkVersion                = 7          // D3D11_SDK_VERSION
	createDeviceDebug         = 0x2        // D3D11_CREATE_DEVICE_DEBUG
	createDeviceBGRASupport   = 0x20       // D3D11_CREATE_DEVICE_BGRA_SUPPORT
	maxTexture2DDimension     = 16384      // D3D11_REQ_TEXTURE2D_U_OR_V_DIMENSION
	appendAlignedElement      = 0xffffffff // D3D11_APPEND_ALIGNED_ELEMENT
	simultaneousRenderTargets = 8          // D3D11_SIMULTANEOUS_RENDER_TARGET_COUNT
)

// DXGI_FORMAT (dxgiformat.h).
const (
	formatUnknown           = 0
	formatR32G32B32A32Float = 2
	formatR32G32B32A32Uint  = 3
	formatR32G32B32A32Sint  = 4
	formatR32G32B32Float    = 6
	formatR32G32B32Uint     = 7
	formatR32G32B32Sint     = 8
	formatR32G32Float       = 16
	formatR32G32Uint        = 17
	formatR32G32Sint        = 18
	formatR32Float          = 41
	formatR32Uint           = 42
	formatR32Sint           = 43
	formatR16G16B16A16Unorm = 11
	formatR8G8B8A8Unorm     = 28
	formatR8Unorm           = 61
	formatB8G8R8A8Unorm     = 87
)

// D3D11_USAGE, D3D11_BIND_FLAG, D3D11_CPU_ACCESS_FLAG and D3D11_MAP.
const (
	usageDefault   = 0
	usageImmutable = 1
	usageDynamic   = 2
	usageStaging   = 3

	bindVertexBuffer   = 0x1
	bindConstantBuffer = 0x4
	bindShaderResource = 0x8
	bindRenderTarget   = 0x20

	cpuAccessWrite = 0x10000
	cpuAccessRead  = 0x20000

	mapRead             = 1
	mapWrite            = 2
	mapWriteDiscard     = 4
	mapWriteNoOverwrite = 5
)

// Input assembly: D3D11_INPUT_CLASSIFICATION and D3D_PRIMITIVE_TOPOLOGY.
const (
	inputPerVertexData   = 0
	inputPerInstanceData = 1

	primitiveTopologyTriangleList  = 4
	primitiveTopologyTriangleStrip = 5
)

// Output merger: D3D11_BLEND, D3D11_BLEND_OP, D3D11_COLOR_WRITE_ENABLE.
const (
	blendZero        = 1
	blendOne         = 2
	blendInvSrcAlpha = 6
	blendOpAdd       = 1
	colorWriteAll    = 0xf
)

// Samplers and rasterizer: D3D11_FILTER, D3D11_TEXTURE_ADDRESS_MODE,
// D3D11_COMPARISON_FUNC, D3D11_FILL_MODE, D3D11_CULL_MODE.
const (
	filterMinMagMipPoint  = 0x0
	filterMinMagMipLinear = 0x15
	textureAddressClamp   = 3
	comparisonNever       = 1
	fillSolid             = 3
	cullNone              = 1
	float32Max            = 3.402823466e+38 // D3D11_FLOAT32_MAX (the sampler's MaxLOD: no mip clamp)
)

// Swap chains: DXGI_USAGE, DXGI_SCALING, DXGI_SWAP_EFFECT, DXGI_ALPHA_MODE
// and IDXGIFactory::MakeWindowAssociation flags.
const (
	usageShaderInput        = 0x10
	usageRenderTargetOutput = 0x20

	scalingStretch = 0
	scalingNone    = 1

	swapEffectFlipSequential = 3
	swapEffectFlipDiscard    = 4

	alphaModeIgnore = 3

	mwaNoAltEnter = 0x2
)

// D3DCompile flags (d3dcompiler.h).
const (
	compileOptimizationLevel3 = 0x8000 // D3DCOMPILE_OPTIMIZATION_LEVEL3
)

// HRESULT values (winerror.h).
const (
	sOK                          = 0x00000000
	dxgiStatusOccluded           = 0x087A0001
	eFail                        = 0x80004005
	eInvalidArg                  = 0x80070057
	eOutOfMemory                 = 0x8007000E
	eNoInterface                 = 0x80004002
	dxgiErrorInvalidCall         = 0x887A0001
	dxgiErrorUnsupported         = 0x887A0004
	dxgiErrorDeviceRemoved       = 0x887A0005
	dxgiErrorDeviceHung          = 0x887A0006
	dxgiErrorDeviceReset         = 0x887A0007
	dxgiErrorWasStillDrawing     = 0x887A000A
	dxgiErrorDriverInternalError = 0x887A0020
	dxgiErrorSDKComponentMissing = 0x887A002D
)

// hrFailed reports whether an HRESULT is an error (its sign bit is set).
// Only the low 32 bits of a syscall result are the HRESULT.
func hrFailed(hr uintptr) bool { return uint32(hr)&0x80000000 != 0 }

// hrDeviceLost reports whether hr says the device is gone for good: it
// was removed (adapter unplugged, driver updated), reset, hung (a TDR) or
// its driver failed internally. The application must create a new device.
func hrDeviceLost(hr uint32) bool {
	switch hr {
	case dxgiErrorDeviceRemoved, dxgiErrorDeviceReset, dxgiErrorDeviceHung, dxgiErrorDriverInternalError:
		return true
	}
	return false
}

// hrName is the SDK name of a well-known HRESULT, or "".
func hrName(hr uint32) string {
	switch hr {
	case sOK:
		return "S_OK"
	case dxgiStatusOccluded:
		return "DXGI_STATUS_OCCLUDED"
	case eFail:
		return "E_FAIL"
	case eInvalidArg:
		return "E_INVALIDARG"
	case eOutOfMemory:
		return "E_OUTOFMEMORY"
	case eNoInterface:
		return "E_NOINTERFACE"
	case dxgiErrorInvalidCall:
		return "DXGI_ERROR_INVALID_CALL"
	case dxgiErrorUnsupported:
		return "DXGI_ERROR_UNSUPPORTED"
	case dxgiErrorDeviceRemoved:
		return "DXGI_ERROR_DEVICE_REMOVED"
	case dxgiErrorDeviceHung:
		return "DXGI_ERROR_DEVICE_HUNG"
	case dxgiErrorDeviceReset:
		return "DXGI_ERROR_DEVICE_RESET"
	case dxgiErrorWasStillDrawing:
		return "DXGI_ERROR_WAS_STILL_DRAWING"
	case dxgiErrorDriverInternalError:
		return "DXGI_ERROR_DRIVER_INTERNAL_ERROR"
	case dxgiErrorSDKComponentMissing:
		return "DXGI_ERROR_SDK_COMPONENT_MISSING"
	}
	return ""
}
