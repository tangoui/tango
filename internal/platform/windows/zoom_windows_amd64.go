//go:build windows

package windows

import "math"

// putZoom calls ICoreWebView2Controller::put_ZoomFactor. On x64 a double
// argument travels in an XMM register, and Go's Windows syscall stub copies
// the first four integer arguments into XMM0-XMM3, so the bits of the
// double passed as an integer arrive where the callee reads them.
func putZoom(s *surface, f float64) {
	comCall(s.controller, ctlPutZoomFactor, uintptr(math.Float64bits(f)))
}

const zoomByScript = false
