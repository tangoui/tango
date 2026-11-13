//go:build windows && (amd64 || arm64)

package d3d11

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// The system libraries are loaded by absolute path from the system
// directory (never the application or current directory, which would
// allow DLL preloading), lazily: nothing is loaded until a device is
// created or a shader compiled.

// systemDirectory is System32, as GetSystemDirectoryW reports it.
// kernel32.dll is a KnownDLL, so loading it by name is safe.
var systemDirectory = sync.OnceValue(func() string {
	const maxPath = 260
	var buf [maxPath]uint16
	n, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&buf[0])), maxPath)
	if n == 0 || n > maxPath {
		return `C:\Windows\System32`
	}
	return string(utf16.Decode(buf[:n]))
})

// sysDLL is a library of the system directory, opened on first use.
type sysDLL struct {
	name string
	dll  func() *syscall.LazyDLL
}

func newSysDLL(name string) *sysDLL {
	d := &sysDLL{name: name}
	d.dll = sync.OnceValue(func() *syscall.LazyDLL {
		return syscall.NewLazyDLL(systemDirectory() + `\` + name)
	})
	return d
}

// sysProc is an exported function of a sysDLL.
type sysProc struct {
	dll  *sysDLL
	name string
	proc func() *syscall.LazyProc
}

func (d *sysDLL) proc(name string) *sysProc {
	return &sysProc{dll: d, name: name, proc: sync.OnceValue(func() *syscall.LazyProc {
		return d.dll().NewProc(name)
	})}
}

// find loads the library and resolves the function, reporting why it is
// unavailable.
func (p *sysProc) find() error {
	if err := p.proc().Find(); err != nil {
		return fmt.Errorf("d3d11: %s!%s: %w", p.dll.name, p.name, err)
	}
	return nil
}

// call invokes the function, which must have been found, and returns its
// result register. Like comCall it is go:uintptrescapes.
//
//go:uintptrescapes
func (p *sysProc) call(args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(p.proc().Addr(), args...)
	return r
}

var (
	d3d11DLL       = newSysDLL("d3d11.dll")
	dxgiDLL        = newSysDLL("dxgi.dll")
	d3dcompilerDLL = newSysDLL("d3dcompiler_47.dll")

	procD3D11CreateDevice  = d3d11DLL.proc("D3D11CreateDevice")
	procCreateDXGIFactory1 = dxgiDLL.proc("CreateDXGIFactory1")
	procD3DCompile         = d3dcompilerDLL.proc("D3DCompile")
)

// forceHardwareFail makes createDevice treat the hardware adapter as
// unavailable, so tests exercise the WARP fallback on any machine.
var forceHardwareFail bool

// warpEnv is the environment variable that restricts createDevice to
// WARP when set to "1".
const warpEnv = "TANGO_D3D_WARP"

// createDevice creates a Direct3D 11 device and its immediate context
// with BGRA support (needed for DXGI swap chains in B8G8R8A8) at feature
// level 10_0 or above. It tries a hardware adapter first and falls back
// to WARP, Windows' software rasterizer; warpOnly, or TANGO_D3D_WARP=1,
// skips the hardware attempt. Failures wrap ErrNoDevice.
func createDevice(warpOnly bool) (dev, ctx uintptr, featureLevel uint32, err error) {
	if err := procD3D11CreateDevice.find(); err != nil {
		return 0, 0, 0, fmt.Errorf("%w: %w", ErrNoDevice, err)
	}
	if os.Getenv(warpEnv) == "1" {
		warpOnly = true
	}
	var hwErr error
	if !warpOnly {
		if forceHardwareFail {
			hwErr = errors.New("hardware device disabled by test")
		} else if dev, ctx, featureLevel, hwErr = createDeviceOfType(driverTypeHardware); hwErr == nil {
			return dev, ctx, featureLevel, nil
		}
	}
	dev, ctx, featureLevel, err = createDeviceOfType(driverTypeWARP)
	if err != nil {
		if hwErr != nil {
			return 0, 0, 0, fmt.Errorf("%w: hardware: %v; WARP: %v", ErrNoDevice, hwErr, err)
		}
		return 0, 0, 0, fmt.Errorf("%w: WARP: %v", ErrNoDevice, err)
	}
	return dev, ctx, featureLevel, nil
}

// createDeviceOfType calls D3D11CreateDevice for one driver type. With
// TANGO_D3D_DEBUG=1 it asks for the debug layer first and, when that is
// not installed (D3D11CreateDevice then fails, typically with
// DXGI_ERROR_SDK_COMPONENT_MISSING), creates the device without it.
func createDeviceOfType(driverType uintptr) (dev, ctx uintptr, featureLevel uint32, err error) {
	levels := []uint32{featureLevel11_1, featureLevel11_0, featureLevel10_1, featureLevel10_0}
	create := func(flags uintptr) uintptr {
		call := func(levels []uint32) uintptr {
			return procD3D11CreateDevice.call(
				0,          // default adapter
				driverType, // D3D_DRIVER_TYPE
				0,          // no software rasterizer module
				flags,      // D3D11_CREATE_DEVICE_FLAG
				uintptr(unsafe.Pointer(&levels[0])), uintptr(len(levels)),
				sdkVersion,
				uintptr(unsafe.Pointer(&dev)),
				uintptr(unsafe.Pointer(&featureLevel)),
				uintptr(unsafe.Pointer(&ctx)),
			)
		}
		hr := call(levels)
		if uint32(hr) == eInvalidArg {
			// Windows 7 without the platform update does not know 11_1 and
			// rejects the whole list.
			hr = call(levels[1:])
		}
		return hr
	}
	flags := uintptr(createDeviceBGRASupport)
	if debugEnabled() {
		hr := create(flags | createDeviceDebug)
		if !hrFailed(hr) {
			runtime.KeepAlive(levels)
			return checkFeatureLevel(dev, ctx, featureLevel)
		}
		logOnce("debug-layer", "d3d11: %s=1 but the Direct3D debug layer is unavailable (%v); install Windows' Graphics Tools feature", debugEnv, hrError("D3D11CreateDevice(debug)", hr))
		dev, ctx, featureLevel = 0, 0, 0
	}
	hr := create(flags)
	runtime.KeepAlive(levels)
	if err := hrError(fmt.Sprintf("D3D11CreateDevice(driver type %d)", driverType), hr); err != nil {
		return 0, 0, 0, err
	}
	return checkFeatureLevel(dev, ctx, featureLevel)
}

// checkFeatureLevel returns a created device, or releases it and fails
// when its feature level is below 10_0.
func checkFeatureLevel(dev, ctx uintptr, featureLevel uint32) (uintptr, uintptr, uint32, error) {
	if featureLevel < featureLevel10_0 {
		release(ctx)
		release(dev)
		return 0, 0, 0, fmt.Errorf("d3d11: feature level %#x below 10_0", featureLevel)
	}
	return dev, ctx, featureLevel, nil
}

// debugEnv is the environment variable that, set to "1", creates the
// device with the Direct3D debug layer (when installed) and logs shader
// compiler warnings.
const debugEnv = "TANGO_D3D_DEBUG"

func debugEnabled() bool { return os.Getenv(debugEnv) == "1" }

// logged holds the keys logOnce has logged.
var logged sync.Map

// logOnce logs a message with log.Printf the first time it is called with
// key.
func logOnce(key, format string, args ...any) {
	if _, dup := logged.LoadOrStore(key, true); !dup {
		log.Printf(format, args...)
	}
}

// adapterOf returns the description of the adapter dev runs on.
func adapterOf(dev uintptr) (adapterDesc, error) {
	var desc adapterDesc
	dxgiDev, err := queryInterface(dev, &iidIDXGIDevice, "IDXGIDevice")
	if err != nil {
		return desc, err
	}
	defer release(dxgiDev)
	var adapter uintptr
	if err := hrError("IDXGIDevice.GetAdapter", comCall(dxgiDev, dxgiDevGetAdapter, uintptr(unsafe.Pointer(&adapter)))); err != nil {
		return desc, err
	}
	defer release(adapter)
	err = hrError("IDXGIAdapter.GetDesc", comCall(adapter, dxgiAdapterGetDesc, uintptr(unsafe.Pointer(&desc))))
	return desc, err
}

// WARP ("Microsoft Basic Render Driver") reports this PCI vendor and
// device.
const (
	warpVendorID = 0x1414
	warpDeviceID = 0x8c
)

// isWARP reports whether dev runs on the WARP software rasterizer.
func isWARP(dev uintptr) bool {
	d, err := adapterOf(dev)
	return err == nil && d.VendorID == warpVendorID && d.DeviceID == warpDeviceID
}

// adapterName is the adapter description as a Go string.
func (d *adapterDesc) adapterName() string {
	n := 0
	for n < len(d.Description) && d.Description[n] != 0 {
		n++
	}
	return string(utf16.Decode(d.Description[:n]))
}
