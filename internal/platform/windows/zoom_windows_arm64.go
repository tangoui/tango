//go:build windows

package windows

// putZoom zooms with CSS on ARM64: Go's Windows ARM64 syscall stub does not
// load the floating-point registers, where put_ZoomFactor reads its double
// (D0). The zoom is reapplied after each navigation. (Same approach as
// mygo; MIT, see doc.go.)
func putZoom(s *surface, f float64) { s.cssZoom(f) }

const zoomByScript = true
