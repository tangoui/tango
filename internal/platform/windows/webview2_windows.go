//go:build windows && (amd64 || arm64)

package windows

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// Creating the WebView2 environment. The documented way is
// CreateCoreWebView2EnvironmentWithOptions from WebView2Loader.dll; the
// loader itself only finds the installed runtime and forwards to an export
// of the runtime's client DLL. An official loader shipped next to the
// executable is used when present. Otherwise the backend does the loader's
// work: findWebView2 (webview2find.go) locates the runtime and the client
// export is called directly, with the arguments the loader passes.

// boolArg converts a Go bool to a Win32 BOOL argument.
func boolArg(b bool) uintptr {
	var v uintptr
	if b {
		v = 1
	}
	return v
}

// envStarter starts environment creation for the user data folder
// dataDir, reporting to the completion handler h, and returns the
// HRESULT of the start call.
type envStarter func(dataDir *uint16, h uintptr) uintptr

// bundledLoader returns the loader's creation function when an official
// WebView2Loader.dll sits beside the executable, else nil.
func bundledLoader() *syscall.Proc {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dll := filepath.Join(filepath.Dir(exe), "WebView2Loader.dll")
	if _, err := os.Stat(dll); err != nil {
		return nil
	}
	lib, err := syscall.LoadDLL(dll)
	if err != nil {
		return nil
	}
	create, err := lib.FindProc("CreateCoreWebView2EnvironmentWithOptions")
	if err != nil {
		return nil
	}
	return create
}

// pickEnvStarter chooses how to create the environment and names the call
// for error messages.
func pickEnvStarter() (envStarter, string, error) {
	if create := bundledLoader(); create != nil {
		// (browserExecutableFolder, userDataFolder, environmentOptions,
		// environmentCreatedHandler); a nil folder means "the installed
		// runtime".
		return func(dataDir *uint16, h uintptr) uintptr {
			hr, _, _ := create.Call(0, uintptr(unsafe.Pointer(dataDir)), 0, h)
			return hr
		}, "CreateCoreWebView2EnvironmentWithOptions", nil
	}

	dir, fixedVersion, err := findWebView2(os.Getenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER"), runtime.GOARCH, regString)
	if err != nil {
		return nil, "", err
	}
	client, err := syscall.LoadDLL(webview2Client(dir, runtime.GOARCH))
	if err != nil {
		return nil, "", err
	}
	create, err := client.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
	if err != nil {
		return nil, "", err
	}
	// The internal export takes a leading flag the loader always sets, the
	// kind of runtime (0 installed, 1 fixed version), then the folder,
	// options and handler. This export is undocumented; its argument list
	// is adapted from github.com/egoist/mygo (MIT, see doc.go).
	var kind uintptr
	if fixedVersion {
		kind = 1
	}
	return func(dataDir *uint16, h uintptr) uintptr {
		hr, _, _ := create.Call(1, kind, uintptr(unsafe.Pointer(dataDir)), 0, h)
		return hr
	}, "creating the WebView2 environment", nil
}

// createEnvironment starts creating the WebView2 environment that keeps
// its profile in userData. done later runs on the UI thread, from the
// message loop, with a referenced environment or the error.
func createEnvironment(userData string, done func(env uintptr, err error)) error {
	start, what, err := pickEnvStarter()
	if err != nil {
		return err
	}
	dataDir := u16(userData)
	onCreated := func(hr, env uintptr) {
		if env == 0 || failed(hr) {
			done(0, hresultError("creating the WebView2 environment", hr))
			return
		}
		addRef(env) // the handler's argument is only borrowed
		done(env, nil)
	}
	hr := withHandler(onCreated, func(h uintptr) uintptr { return start(dataDir, h) })
	if failed(hr) {
		return hresultError(what, hr)
	}
	return nil
}
