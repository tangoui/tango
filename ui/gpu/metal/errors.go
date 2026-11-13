// Package metal is the Metal Device of the GPU renderer (ui/gpu): it
// executes gpu.Frames into a persistent offscreen RGBA8 render target that
// ReadPixels reads back, and NewPresenter shows it in a CAMetalLayer. It
// needs no cgo (purego). On other platforms NewDevice and NewPresenter
// fail with ErrUnsupported.
package metal

import (
	"errors"

	"github.com/tangoui/tango/ui/gpu"
)

var (
	// ErrUnsupported is returned by NewDevice on platforms without Metal.
	ErrUnsupported = errors.New("metal: not supported on this platform")
	// ErrNoDevice is returned by NewDevice when the system has no Metal
	// device (or Metal.framework cannot be loaded).
	ErrNoDevice = errors.New("metal: no Metal device")
	// ErrClosed is returned by a Device's methods after Close.
	ErrClosed = errors.New("metal: device closed")
	// ErrDeviceLost is gpu.ErrDeviceLost (the same value): it is wrapped
	// by the errors Device.Execute (so the Renderer's Render) and
	// NewPresenter's present return once the GPU is unusable: a command
	// buffer reported the device removed or the app's GPU access revoked,
	// or several frames in a row failed. It is permanent for that Device;
	// switch to the CPU renderer.
	ErrDeviceLost = gpu.ErrDeviceLost
	// ErrTargetTooLarge is gpu.ErrTargetTooLarge (the same value): it is
	// wrapped by the error Device.Execute (so the Renderer's Render)
	// returns for a frame whose render target, or a texture it samples,
	// exceeds Metal's 2D texture limit (16384 on every Mac GPU family): for
	// example a window wider than 16384 device pixels. Nothing is allocated
	// or submitted and the frame is not a GPU failure (it never leads to
	// ErrDeviceLost); the device keeps working for frames of a valid size.
	// Draw such a frame with the CPU renderer.
	ErrTargetTooLarge = gpu.ErrTargetTooLarge
)
