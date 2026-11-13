package d3d11

import (
	"errors"
	"fmt"
)

// hresultError is a failed COM or Direct3D call. It builds on every
// platform so the device's failure accounting (state.go) is tested on the
// development machine.
type hresultError struct {
	Op string // the call, e.g. "CreateTexture2D"
	HR uint32
}

func (e *hresultError) Error() string {
	if name := hrName(e.HR); name != "" {
		return fmt.Sprintf("d3d11: %s failed: %s (HRESULT %#08x)", e.Op, name, e.HR)
	}
	return fmt.Sprintf("d3d11: %s failed: HRESULT %#08x", e.Op, e.HR)
}

// DeviceLost reports whether the HRESULT means the device was removed,
// reset or hung (DXGI_ERROR_DEVICE_REMOVED, _RESET, _HUNG, or
// DXGI_ERROR_DRIVER_INTERNAL_ERROR).
func (e *hresultError) DeviceLost() bool { return hrDeviceLost(e.HR) }

// hrError returns a *hresultError for a failed call, or nil if hr is a
// success code.
func hrError(op string, hr uintptr) error {
	if !hrFailed(hr) {
		return nil
	}
	return &hresultError{Op: op, HR: uint32(hr)}
}

// isDeviceLost reports whether err is or wraps a device-lost HRESULT.
func isDeviceLost(err error) bool {
	var he *hresultError
	return errors.As(err, &he) && he.DeviceLost()
}
