//go:build windows

package windows

// putBounds calls ICoreWebView2Controller::put_Bounds. On ARM64, a 16-byte
// struct is passed by value in two registers. (After mygo and Wails; MIT,
// see doc.go.)
func putBounds(controller uintptr, r rect) {
	comCall(controller, ctlPutBounds,
		uintptr(uint32(r.Left))|uintptr(uint32(r.Top))<<32,
		uintptr(uint32(r.Right))|uintptr(uint32(r.Bottom))<<32)
}
