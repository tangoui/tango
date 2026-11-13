//go:build windows

package windows

import "unsafe"

// putBounds calls ICoreWebView2Controller::put_Bounds. On x64, structs
// larger than 8 bytes are passed by reference. (After mygo and Wails; MIT,
// see doc.go.)
func putBounds(controller uintptr, r rect) {
	comCall(controller, ctlPutBounds, uintptr(unsafe.Pointer(&r)))
}
