package gpu

import "errors"

// Device errors shared by every backend (internal/render/gpu/metal, internal/render/gpu/d3d11, …),
// so a caller choosing between the GPU and the CPU renderer tests one
// value whichever device it opened. Backends wrap them with details and
// may re-export them under their own names (metal.ErrDeviceLost is
// gpu.ErrDeviceLost).
var (
	// ErrDeviceLost is wrapped by the errors a Device's Execute (so the
	// Renderer's Render) and its presenter return once the GPU is
	// unusable: the device was removed or reset, the app's GPU access was
	// revoked, or several frames in a row failed. It is permanent for that
	// Device; switch to the CPU renderer.
	ErrDeviceLost = errors.New("gpu: device lost")
	// ErrTargetTooLarge is wrapped by the error a Device's Execute (so the
	// Renderer's Render) returns for a frame whose render target, or a
	// texture it samples, exceeds the device's 2D texture limit (16384 on
	// every Mac GPU family and on D3D11 feature level 11): for example a
	// window wider than 16384 device pixels. Nothing is allocated or
	// submitted and the frame is not a GPU failure (it never leads to
	// ErrDeviceLost); the device keeps working for frames of a valid size.
	// Draw such a frame with the CPU renderer.
	ErrTargetTooLarge = errors.New("gpu: frame larger than the device texture limit")
)
