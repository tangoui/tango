//go:build windows

package windows

// doubleArgs: double arguments can be passed to COM methods as their bits
// (Go's x64 syscall stub copies the integer arguments into XMM0-XMM3, see
// zoom_windows_amd64.go). Used for cookie expiry and PDF page size.
const doubleArgs = true
