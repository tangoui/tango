//go:build windows

package windows

// doubleArgs: Go's ARM64 syscall stub does not load the floating-point
// registers, so methods taking a double (put_Expires, put_PageWidth) are
// not called: cookies with an expiry go through the DevTools protocol and
// the PDF page size keeps the default.
const doubleArgs = false
