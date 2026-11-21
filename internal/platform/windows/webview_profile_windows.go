//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var iidWebViewEnvironment10 = guid("ee0eb9df-6f12-46ce-b53f-3f47b9c928e0")

const (
	// Base Environment has 8 slots, Environment2–9 add 12.
	env10CreateControllerOptions     = 20
	env10CreateControllerWithOptions = 21
	controllerOptionsPutProfileName  = 4
)

func (b *Backend) SupportsWebProfiles() bool {
	if b.env == 0 {
		return true // checked again when the asynchronous environment is ready
	}
	env10 := queryInterface(b.env, &iidWebViewEnvironment10)
	defer release(env10)
	return env10 != 0
}

func (s *surface) createProfileController() {
	env10 := queryInterface(s.b.env, &iidWebViewEnvironment10)
	if env10 == 0 {
		s.fail(platform.ErrUnsupported)
		return
	}
	defer release(env10)
	options := comObject(env10, env10CreateControllerOptions)
	if options == 0 {
		s.fail(platform.ErrUnsupported)
		return
	}
	defer release(options)
	name := "Tango-" + platform.WebProfileKey(s.webProfile)
	if hr := comCall(options, controllerOptionsPutProfileName, uintptr(unsafe.Pointer(u16(name)))); failed(hr) {
		s.fail(hresultError("put_ProfileName", hr))
		return
	}
	if hr := withHandler(s.controllerCreated, func(h uintptr) uintptr {
		return comCall(env10, env10CreateControllerWithOptions, s.hwnd, options, h)
	}); failed(hr) {
		s.fail(hresultError("CreateCoreWebView2ControllerWithOptions", hr))
	}
}
