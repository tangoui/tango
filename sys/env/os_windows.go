//go:build windows

package env

import (
	"fmt"
	"syscall"
	"unsafe"
)

// RtlGetVersion reports the real version; GetVersionEx lies to programs
// without a compatibility manifest.
var procRtlGetVersion = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")

type osVersionInfo struct {
	Size, Major, Minor, Build, Platform uint32
	CSDVersion                          [128]uint16
}

func osVersion() (name, version string) {
	var vi osVersionInfo
	vi.Size = uint32(unsafe.Sizeof(vi))
	if procRtlGetVersion.Find() != nil {
		return "Windows", ""
	}
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&vi)))
	name = "Windows"
	switch {
	case vi.Major == 10 && vi.Build >= 22000:
		name = "Windows 11"
	case vi.Major == 10:
		name = "Windows 10"
	}
	return name, fmt.Sprintf("%d.%d.%d", vi.Major, vi.Minor, vi.Build)
}
