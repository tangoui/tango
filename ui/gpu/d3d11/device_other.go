//go:build !windows || !(amd64 || arm64)

package d3d11

import (
	"fmt"
	"image"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/paint"
)

// Device is the Direct3D 11 device; on this platform it cannot be created.
type Device struct{}

// NewDevice always fails with ErrUnsupported on this platform.
func NewDevice() (*Device, error) { return nil, ErrUnsupported }

// Execute implements gpu.Device.
func (d *Device) Execute(f *gpu.Frame) error { return ErrUnsupported }

// ReadPixels reads back the render target.
func (d *Device) ReadPixels() (*image.RGBA, error) { return nil, ErrUnsupported }

// Close releases the device.
func (d *Device) Close() {}

var _ gpu.Device = (*Device)(nil)

// NewPresenter attaches a GPU renderer to the window hwnd through a DXGI
// swap chain (on Windows r also implements Close() error). On this
// platform it returns an error wrapping ErrUnsupported.
func NewPresenter(hwnd uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	return nil, nil, fmt.Errorf("d3d11: NewPresenter: %w", ErrUnsupported)
}
