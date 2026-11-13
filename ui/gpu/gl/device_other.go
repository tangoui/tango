//go:build !linux && !darwin

package gl

import (
	"image"

	"github.com/tangoui/tango/ui/gpu"
)

// Device is the OpenGL device; on this platform it cannot be created.
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
