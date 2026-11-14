// Package d3d11 is the Direct3D 11 backend of the GPU renderer: a
// gpu.Device that executes internal/render/gpu frames on a D3D11 device and a presenter
// that shows them in a window through a DXGI flip-model swap chain.
//
// It calls Direct3D through COM without cgo: d3d11.dll, dxgi.dll and
// d3dcompiler_47.dll are loaded from the system directory on first use,
// and interface methods are called through their vtables with
// syscall.SyscallN. The quad shader (internal/render/gpu/gen.QuadHLSL) is compiled at
// run time with D3DCompile, so no shader compiler is needed at build time.
//
// The device prefers a hardware adapter and falls back to WARP, the
// software rasterizer shipped with Windows, so virtual machines, remote
// desktop sessions and CI runners get a working device too. Setting the
// environment variable TANGO_D3D_WARP=1 skips the hardware adapter, to
// reproduce the software device on a machine with a GPU; the package's
// own tests run on WARP unless TANGO_D3D_HARDWARE=1. Setting
// TANGO_D3D_DEBUG=1 creates the device with the Direct3D debug layer when
// it is installed (Windows' optional "Graphics Tools" feature; otherwise
// the device is created without it and that is logged once) and logs the
// shader compiler's warnings once per entry point.
//
// The backend exists on windows/amd64 and windows/arm64. Everywhere else
// NewDevice and NewPresenter fail with ErrUnsupported.
package d3d11

import (
	"errors"

	"github.com/tangoui/tango/internal/render/gpu"
)

var (
	// ErrUnsupported is returned on platforms without Direct3D 11
	// (anything but windows/amd64 and windows/arm64).
	ErrUnsupported = errors.New("d3d11: Direct3D 11 is not available on this platform")
	// ErrNoDevice is wrapped by the error returned when no Direct3D 11
	// device could be created: d3d11.dll is missing, or neither a hardware
	// adapter nor WARP provides feature level 10_0. Use the CPU renderer.
	ErrNoDevice = errors.New("d3d11: no Direct3D 11 device")
	// ErrClosed is returned by calls on a device or presenter after Close.
	ErrClosed = errors.New("d3d11: use after Close")
	// ErrDeviceLost is gpu.ErrDeviceLost (the same value): it is wrapped
	// by the errors Device.Execute (so the Renderer's Render) returns once
	// the GPU is unusable: the device was removed, reset or hung, or
	// several frames in a row failed. It is permanent for that Device;
	// switch to the CPU renderer.
	ErrDeviceLost = gpu.ErrDeviceLost
	// ErrTargetTooLarge is gpu.ErrTargetTooLarge (the same value): it is
	// wrapped by the error Device.Execute returns for a frame whose render
	// target, or a texture it samples, exceeds the device's 2D texture
	// limit (16384 from feature level 11_0 on, 8192 on 10_x). Nothing is
	// allocated or submitted and the frame is not a GPU failure.
	ErrTargetTooLarge = gpu.ErrTargetTooLarge
)
