package d3d11

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Method declaration order of each interface, counted from IUnknown, as
// the Windows SDK headers declare it (the C vtable structs <Interface>Vtbl
// in d3d11.h, dxgi.h, dxgi1_2.h and d3dcommon.h; cross-checked against
// the mingw-w64 copies at
// https://github.com/mingw-w64/mingw-w64/tree/master/mingw-w64-headers/include
// and the interface pages under
// https://learn.microsoft.com/windows/win32/api/d3d11/ and
// https://learn.microsoft.com/windows/win32/api/dxgi/).
//
// The lists are written out in full, independently of the vt* structs in
// abi.go, so a method dropped or swapped in either place fails the test.
var (
	iunknownMethods    = "QueryInterface AddRef Release"
	deviceChildMethods = iunknownMethods + " GetDevice GetPrivateData SetPrivateData SetPrivateDataInterface"
	dxgiObjectMethods  = iunknownMethods + " SetPrivateData SetPrivateDataInterface GetPrivateData GetParent"
	dxgiFactoryMethods = dxgiObjectMethods + " EnumAdapters MakeWindowAssociation GetWindowAssociation CreateSwapChain CreateSoftwareAdapter"
	dxgiSwapMethods    = dxgiObjectMethods + " GetDevice Present GetBuffer SetFullscreenState GetFullscreenState GetDesc ResizeBuffers ResizeTarget GetContainingOutput GetFrameStatistics GetLastPresentCount"
)

var expectedVtables = []struct {
	name    string
	vt      any
	methods string
}{
	{"ID3D11Device", vtID3D11Device{}, iunknownMethods + ` CreateBuffer CreateTexture1D CreateTexture2D
		CreateTexture3D CreateShaderResourceView CreateUnorderedAccessView CreateRenderTargetView
		CreateDepthStencilView CreateInputLayout CreateVertexShader CreateGeometryShader
		CreateGeometryShaderWithStreamOutput CreatePixelShader CreateHullShader CreateDomainShader
		CreateComputeShader CreateClassLinkage CreateBlendState CreateDepthStencilState
		CreateRasterizerState CreateSamplerState CreateQuery CreatePredicate CreateCounter
		CreateDeferredContext OpenSharedResource CheckFormatSupport CheckMultisampleQualityLevels
		CheckCounterInfo CheckCounter CheckFeatureSupport GetPrivateData SetPrivateData
		SetPrivateDataInterface GetFeatureLevel GetCreationFlags GetDeviceRemovedReason
		GetImmediateContext SetExceptionMode GetExceptionMode`},
	{"ID3D11DeviceContext", vtID3D11DeviceContext{}, deviceChildMethods + ` VSSetConstantBuffers
		PSSetShaderResources PSSetShader PSSetSamplers VSSetShader DrawIndexed Draw Map Unmap
		PSSetConstantBuffers IASetInputLayout IASetVertexBuffers IASetIndexBuffer
		DrawIndexedInstanced DrawInstanced GSSetConstantBuffers GSSetShader IASetPrimitiveTopology
		VSSetShaderResources VSSetSamplers Begin End GetData SetPredication GSSetShaderResources
		GSSetSamplers OMSetRenderTargets OMSetRenderTargetsAndUnorderedAccessViews OMSetBlendState
		OMSetDepthStencilState SOSetTargets DrawAuto DrawIndexedInstancedIndirect
		DrawInstancedIndirect Dispatch DispatchIndirect RSSetState RSSetViewports RSSetScissorRects
		CopySubresourceRegion CopyResource UpdateSubresource CopyStructureCount
		ClearRenderTargetView ClearUnorderedAccessViewUint ClearUnorderedAccessViewFloat
		ClearDepthStencilView GenerateMips SetResourceMinLOD GetResourceMinLOD ResolveSubresource
		ExecuteCommandList HSSetShaderResources HSSetShader HSSetSamplers HSSetConstantBuffers
		DSSetShaderResources DSSetShader DSSetSamplers DSSetConstantBuffers CSSetShaderResources
		CSSetUnorderedAccessViews CSSetShader CSSetSamplers CSSetConstantBuffers
		VSGetConstantBuffers PSGetShaderResources PSGetShader PSGetSamplers VSGetShader
		PSGetConstantBuffers IAGetInputLayout IAGetVertexBuffers IAGetIndexBuffer
		GSGetConstantBuffers GSGetShader IAGetPrimitiveTopology VSGetShaderResources VSGetSamplers
		GetPredication GSGetShaderResources GSGetSamplers OMGetRenderTargets
		OMGetRenderTargetsAndUnorderedAccessViews OMGetBlendState OMGetDepthStencilState
		SOGetTargets RSGetState RSGetViewports RSGetScissorRects HSGetShaderResources HSGetShader
		HSGetSamplers HSGetConstantBuffers DSGetShaderResources DSGetShader DSGetSamplers
		DSGetConstantBuffers CSGetShaderResources CSGetUnorderedAccessViews CSGetShader
		CSGetSamplers CSGetConstantBuffers ClearState Flush GetType GetContextFlags
		FinishCommandList`},
	{"ID3D11Texture2D", vtID3D11Texture2D{}, deviceChildMethods + " GetType SetEvictionPriority GetEvictionPriority GetDesc"},
	{"ID3D11View", vtID3D11View{}, deviceChildMethods + " GetResource"},
	{"IDXGIDevice", vtIDXGIDevice{}, dxgiObjectMethods + " GetAdapter CreateSurface QueryResourceResidency SetGPUThreadPriority GetGPUThreadPriority"},
	{"IDXGIDevice1", vtIDXGIDevice1{}, dxgiObjectMethods + " GetAdapter CreateSurface QueryResourceResidency SetGPUThreadPriority GetGPUThreadPriority SetMaximumFrameLatency GetMaximumFrameLatency"},
	{"IDXGIAdapter", vtIDXGIAdapter{}, dxgiObjectMethods + " EnumOutputs GetDesc CheckInterfaceSupport"},
	{"IDXGIFactory2", vtIDXGIFactory2{}, dxgiFactoryMethods + ` EnumAdapters1 IsCurrent
		IsWindowedStereoEnabled CreateSwapChainForHwnd CreateSwapChainForCoreWindow
		GetSharedResourceAdapterLuid RegisterStereoStatusWindow RegisterStereoStatusEvent
		UnregisterStereoStatus RegisterOcclusionStatusWindow RegisterOcclusionStatusEvent
		UnregisterOcclusionStatus CreateSwapChainForComposition`},
	{"IDXGISwapChain1", vtIDXGISwapChain1{}, dxgiSwapMethods + ` GetDesc1 GetFullscreenDesc GetHwnd
		GetCoreWindow Present1 IsTemporaryMonoSupported GetRestrictToOutput SetBackgroundColor
		GetBackgroundColor SetRotation GetRotation`},
	{"ID3D10Blob", vtID3DBlob{}, iunknownMethods + " GetBufferPointer GetBufferSize"},
}

// vtableMethods flattens a vt* struct into its method names in slot
// order, descending into embedded base interfaces.
func vtableMethods(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			names = append(names, vtableMethods(f.Type)...)
			continue
		}
		if f.Type.Kind() != reflect.Uintptr {
			panic("vtable field " + f.Name + " is not a uintptr")
		}
		names = append(names, f.Name)
	}
	return names
}

func TestABISlots(t *testing.T) {
	for _, tc := range expectedVtables {
		typ := reflect.TypeOf(tc.vt)
		got := vtableMethods(typ)
		want := strings.Fields(tc.methods)
		if len(got) != len(want) {
			t.Errorf("%s: %d methods, want %d", tc.name, len(got), len(want))
		}
		for i := 0; i < min(len(got), len(want)); i++ {
			if got[i] != want[i] {
				t.Errorf("%s: slot %d is %s, want %s", tc.name, i, got[i], want[i])
				break
			}
		}
		if size := typ.Size(); size != uintptr(len(want))*slotSize {
			t.Errorf("%s: vtable size %d, want %d slots", tc.name, size, len(want))
		}
	}

	// The slot constants the device uses, against their position in the
	// lists above (e.g. ID3D11DeviceContext::Flush is the well-known 111).
	slots := []struct {
		name string
		got  int
		want int
	}{
		{"IUnknown::QueryInterface", unkQueryInterface, 0},
		{"IUnknown::AddRef", unkAddRef, 1},
		{"IUnknown::Release", unkRelease, 2},
		{"ID3D11Device::CreateBuffer", devCreateBuffer, 3},
		{"ID3D11Device::CreateTexture2D", devCreateTexture2D, 5},
		{"ID3D11Device::CreateShaderResourceView", devCreateShaderResourceView, 7},
		{"ID3D11Device::CreateRenderTargetView", devCreateRenderTargetView, 9},
		{"ID3D11Device::CreateInputLayout", devCreateInputLayout, 11},
		{"ID3D11Device::CreateVertexShader", devCreateVertexShader, 12},
		{"ID3D11Device::CreatePixelShader", devCreatePixelShader, 15},
		{"ID3D11Device::CreateBlendState", devCreateBlendState, 20},
		{"ID3D11Device::CreateRasterizerState", devCreateRasterizerState, 22},
		{"ID3D11Device::CreateSamplerState", devCreateSamplerState, 23},
		{"ID3D11Device::GetFeatureLevel", devGetFeatureLevel, 37},
		{"ID3D11Device::GetDeviceRemovedReason", devGetDeviceRemovedReason, 39},
		{"ID3D11Device::GetImmediateContext", devGetImmediateContext, 40},
		{"ID3D11DeviceContext::VSSetConstantBuffers", ctxVSSetConstantBuffers, 7},
		{"ID3D11DeviceContext::PSSetShaderResources", ctxPSSetShaderResources, 8},
		{"ID3D11DeviceContext::PSSetShader", ctxPSSetShader, 9},
		{"ID3D11DeviceContext::PSSetSamplers", ctxPSSetSamplers, 10},
		{"ID3D11DeviceContext::VSSetShader", ctxVSSetShader, 11},
		{"ID3D11DeviceContext::Draw", ctxDraw, 13},
		{"ID3D11DeviceContext::Map", ctxMap, 14},
		{"ID3D11DeviceContext::Unmap", ctxUnmap, 15},
		{"ID3D11DeviceContext::PSSetConstantBuffers", ctxPSSetConstantBuffers, 16},
		{"ID3D11DeviceContext::IASetInputLayout", ctxIASetInputLayout, 17},
		{"ID3D11DeviceContext::IASetVertexBuffers", ctxIASetVertexBuffers, 18},
		{"ID3D11DeviceContext::DrawInstanced", ctxDrawInstanced, 21},
		{"ID3D11DeviceContext::IASetPrimitiveTopology", ctxIASetPrimitiveTopology, 24},
		{"ID3D11DeviceContext::OMSetRenderTargets", ctxOMSetRenderTargets, 33},
		{"ID3D11DeviceContext::OMSetBlendState", ctxOMSetBlendState, 35},
		{"ID3D11DeviceContext::RSSetState", ctxRSSetState, 43},
		{"ID3D11DeviceContext::RSSetViewports", ctxRSSetViewports, 44},
		{"ID3D11DeviceContext::RSSetScissorRects", ctxRSSetScissorRects, 45},
		{"ID3D11DeviceContext::CopySubresourceRegion", ctxCopySubresourceRegion, 46},
		{"ID3D11DeviceContext::CopyResource", ctxCopyResource, 47},
		{"ID3D11DeviceContext::UpdateSubresource", ctxUpdateSubresource, 48},
		{"ID3D11DeviceContext::ClearRenderTargetView", ctxClearRenderTargetView, 50},
		{"ID3D11DeviceContext::ClearState", ctxClearState, 110},
		{"ID3D11DeviceContext::Flush", ctxFlush, 111},
		{"ID3D11Texture2D::GetDesc", tex2DGetDesc, 10},
		{"ID3D11View::GetResource", viewGetResource, 7},
		{"IDXGIObject::GetParent", dxgiObjGetParent, 6},
		{"IDXGIDevice::GetAdapter", dxgiDevGetAdapter, 7},
		{"IDXGIDevice1::SetMaximumFrameLatency", dxgiDev1SetMaxFrameLatency, 12},
		{"IDXGIAdapter::GetDesc", dxgiAdapterGetDesc, 8},
		{"IDXGIFactory::MakeWindowAssociation", factMakeWindowAssoc, 8},
		{"IDXGIFactory2::CreateSwapChainForHwnd", fact2CreateSwapChainForHwnd, 15},
		{"IDXGISwapChain::Present", swapPresent, 8},
		{"IDXGISwapChain::GetBuffer", swapGetBuffer, 9},
		{"IDXGISwapChain::ResizeBuffers", swapResizeBuffers, 13},
		{"IDXGISwapChain1::Present1", swap1Present1, 22},
		{"IDXGISwapChain1::GetDesc1", swap1GetDesc1, 18},
		{"IDXGISwapChain1::SetBackgroundColor", swap1SetBackgroundColor, 25},
		{"ID3D10Blob::GetBufferPointer", blobGetBufferPointer, 3},
		{"ID3D10Blob::GetBufferSize", blobGetBufferSize, 4},
	}
	for _, s := range slots {
		if s.got != s.want {
			t.Errorf("%s: slot %d, want %d", s.name, s.got, s.want)
		}
	}
}

// layout is the expected size of a structure and the offsets of selected
// fields.
type layout struct {
	size    uintptr
	offsets map[string]uintptr
}

// structLayouts64 are the SDK structure layouts on the two supported
// 64-bit targets, Windows x64 and Windows ARM64, worked out from the
// declarations in d3d11.h, dxgi.h, dxgi1_2.h and dxgicommon.h (field
// order as at https://learn.microsoft.com/windows/win32/api/d3d11/ns-d3d11-d3d11_texture2d_desc
// and the sibling ns-* pages) with the MSVC rules both ABIs share: 4-byte
// UINT/INT/BOOL/FLOAT/enums, 8-byte pointers and SIZE_T, natural
// alignment, the struct padded to its largest member.
var structLayouts64 = map[string]layout{
	"D3D11_INPUT_ELEMENT_DESC": {32, map[string]uintptr{
		"SemanticName": 0, "SemanticIndex": 8, "Format": 12, "InputSlot": 16,
		"AlignedByteOffset": 20, "InputSlotClass": 24, "InstanceDataStepRate": 28}},
	"D3D11_TEXTURE2D_DESC": {44, map[string]uintptr{
		"Width": 0, "Height": 4, "MipLevels": 8, "ArraySize": 12, "Format": 16,
		"SampleDesc": 20, "Usage": 28, "BindFlags": 32, "CPUAccessFlags": 36, "MiscFlags": 40}},
	"D3D11_BUFFER_DESC": {24, map[string]uintptr{
		"ByteWidth": 0, "Usage": 4, "BindFlags": 8, "CPUAccessFlags": 12, "MiscFlags": 16,
		"StructureByteStride": 20}},
	"D3D11_SUBRESOURCE_DATA":   {16, map[string]uintptr{"SysMem": 0, "SysMemPitch": 8, "SysMemSlicePitch": 12}},
	"D3D11_MAPPED_SUBRESOURCE": {16, map[string]uintptr{"Data": 0, "RowPitch": 8, "DepthPitch": 12}},
	"D3D11_RENDER_TARGET_BLEND_DESC": {32, map[string]uintptr{
		"BlendEnable": 0, "SrcBlend": 4, "DestBlend": 8, "BlendOp": 12, "SrcBlendAlpha": 16,
		"DestBlendAlpha": 20, "BlendOpAlpha": 24, "RenderTargetWriteMask": 28}},
	"D3D11_BLEND_DESC": {264, map[string]uintptr{
		"AlphaToCoverageEnable": 0, "IndependentBlendEnable": 4, "RenderTarget": 8}},
	"D3D11_SAMPLER_DESC": {52, map[string]uintptr{
		"Filter": 0, "AddressU": 4, "AddressV": 8, "AddressW": 12, "MipLODBias": 16,
		"MaxAnisotropy": 20, "ComparisonFunc": 24, "BorderColor": 28, "MinLOD": 44, "MaxLOD": 48}},
	"D3D11_RASTERIZER_DESC": {40, map[string]uintptr{
		"FillMode": 0, "CullMode": 4, "FrontCounterClockwise": 8, "DepthBias": 12,
		"DepthBiasClamp": 16, "SlopeScaledDepthBias": 20, "DepthClipEnable": 24,
		"ScissorEnable": 28, "MultisampleEnable": 32, "AntialiasedLineEnable": 36}},
	"D3D11_VIEWPORT": {24, map[string]uintptr{
		"TopLeftX": 0, "TopLeftY": 4, "Width": 8, "Height": 12, "MinDepth": 16, "MaxDepth": 20}},
	"D3D11_RECT": {16, map[string]uintptr{"Left": 0, "Top": 4, "Right": 8, "Bottom": 12}},
	"D3D11_BOX": {24, map[string]uintptr{
		"Left": 0, "Top": 4, "Front": 8, "Right": 12, "Bottom": 16, "Back": 20}},
	"DXGI_SAMPLE_DESC": {8, map[string]uintptr{"Count": 0, "Quality": 4}},
	"DXGI_SWAP_CHAIN_DESC1": {48, map[string]uintptr{
		"Width": 0, "Height": 4, "Format": 8, "Stereo": 12, "SampleDesc": 16,
		"BufferUsage": 24, "BufferCount": 28, "Scaling": 32, "SwapEffect": 36,
		"AlphaMode": 40, "Flags": 44}},
	"DXGI_PRESENT_PARAMETERS": {32, map[string]uintptr{
		"DirtyRectsCount": 0, "DirtyRects": 8, "ScrollRect": 16, "ScrollOffset": 24}},
	"DXGI_ADAPTER_DESC": {304, map[string]uintptr{
		"Description": 0, "VendorID": 256, "DeviceID": 260, "SubSysID": 264, "Revision": 268,
		"DedicatedVideoMemory": 272, "DedicatedSystemMemory": 280, "SharedSystemMemory": 288,
		"AdapterLuid": 296}},
	"GUID": {16, map[string]uintptr{"Data1": 0, "Data2": 4, "Data3": 6, "Data4": 8}},
}

// expectedLayouts holds the expected layouts per GOARCH. Both supported
// targets are 64-bit with the same rules, so the numbers coincide; they
// are listed per architecture so a difference would have a place to go.
var expectedLayouts = map[string]map[string]layout{
	"amd64": structLayouts64,
	"arm64": structLayouts64,
}

func TestABIStructSizes(t *testing.T) {
	want, ok := expectedLayouts[runtime.GOARCH]
	if !ok {
		t.Skipf("no Direct3D 11 backend on GOARCH=%s", runtime.GOARCH)
	}
	types := map[string]reflect.Type{
		"D3D11_INPUT_ELEMENT_DESC":       reflect.TypeOf(inputElementDesc{}),
		"D3D11_TEXTURE2D_DESC":           reflect.TypeOf(texture2DDesc{}),
		"D3D11_BUFFER_DESC":              reflect.TypeOf(bufferDesc{}),
		"D3D11_SUBRESOURCE_DATA":         reflect.TypeOf(subresourceData{}),
		"D3D11_MAPPED_SUBRESOURCE":       reflect.TypeOf(mappedSubresource{}),
		"D3D11_RENDER_TARGET_BLEND_DESC": reflect.TypeOf(renderTargetBlendDesc{}),
		"D3D11_BLEND_DESC":               reflect.TypeOf(blendDesc{}),
		"D3D11_SAMPLER_DESC":             reflect.TypeOf(samplerDesc{}),
		"D3D11_RASTERIZER_DESC":          reflect.TypeOf(rasterizerDesc{}),
		"D3D11_VIEWPORT":                 reflect.TypeOf(viewport{}),
		"D3D11_RECT":                     reflect.TypeOf(rect{}),
		"D3D11_BOX":                      reflect.TypeOf(box{}),
		"DXGI_SAMPLE_DESC":               reflect.TypeOf(sampleDesc{}),
		"DXGI_SWAP_CHAIN_DESC1":          reflect.TypeOf(swapChainDesc1{}),
		"DXGI_PRESENT_PARAMETERS":        reflect.TypeOf(presentParameters{}),
		"DXGI_ADAPTER_DESC":              reflect.TypeOf(adapterDesc{}),
		"GUID":                           reflect.TypeOf(guid{}),
	}
	if len(types) != len(want) {
		t.Fatalf("%d structures checked, %d expected", len(types), len(want))
	}
	for name, typ := range types {
		l, ok := want[name]
		if !ok {
			t.Errorf("%s: no expected layout", name)
			continue
		}
		if typ.Size() != l.size {
			t.Errorf("%s (%s): size %d, want %d", name, typ, typ.Size(), l.size)
		}
		if typ.NumField() != len(l.offsets) {
			t.Errorf("%s: %d fields, want %d", name, typ.NumField(), len(l.offsets))
		}
		for field, off := range l.offsets {
			f, ok := typ.FieldByName(field)
			if !ok {
				t.Errorf("%s: no field %s", name, field)
				continue
			}
			if f.Offset != off {
				t.Errorf("%s.%s: offset %d, want %d", name, field, f.Offset, off)
			}
		}
	}

	// Fixed-size members whose length is part of the ABI.
	if n := len(blendDesc{}.RenderTarget); n != simultaneousRenderTargets {
		t.Errorf("D3D11_BLEND_DESC.RenderTarget has %d entries, want %d", n, simultaneousRenderTargets)
	}
	if n := unsafe.Sizeof(adapterDesc{}.Description); n != 256 {
		t.Errorf("DXGI_ADAPTER_DESC.Description is %d bytes, want 256 (WCHAR[128])", n)
	}
}

// TestHRESULTs pins the HRESULT values (winerror.h) and the device-lost
// classification the device relies on.
func TestHRESULTs(t *testing.T) {
	for hr, want := range map[uint32]bool{
		0x887A0005: true,  // DXGI_ERROR_DEVICE_REMOVED
		0x887A0006: true,  // DXGI_ERROR_DEVICE_HUNG
		0x887A0007: true,  // DXGI_ERROR_DEVICE_RESET
		0x887A0020: true,  // DXGI_ERROR_DRIVER_INTERNAL_ERROR
		0x80070057: false, // E_INVALIDARG
		0x8007000E: false, // E_OUTOFMEMORY
		0x087A0001: false, // DXGI_STATUS_OCCLUDED (success)
		0:          false,
	} {
		if got := hrDeviceLost(hr); got != want {
			t.Errorf("hrDeviceLost(%#08x) = %v, want %v", hr, got, want)
		}
	}
	if hrFailed(dxgiStatusOccluded) || hrFailed(sOK) || !hrFailed(dxgiErrorDeviceRemoved) {
		t.Error("hrFailed misclassifies S_OK, DXGI_STATUS_OCCLUDED or DXGI_ERROR_DEVICE_REMOVED")
	}
	// Garbage in the upper half of a 64-bit result register is ignored.
	var dirty uint64 = 0xFFFFFFFF_00000000
	if hrFailed(uintptr(dirty)) {
		t.Error("hrFailed looks past the low 32 bits")
	}
	if hrName(dxgiErrorDeviceRemoved) != "DXGI_ERROR_DEVICE_REMOVED" {
		t.Error("hrName(DXGI_ERROR_DEVICE_REMOVED)")
	}
}
