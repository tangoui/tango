// Package env describes the environment the app runs in: operating system
// and version, CPU architecture, whether this is a debug build, and the
// WebView engine.
//
// Info is safe from any goroutine. Pass the app only once it runs (as
// tango.Await, it would otherwise wait for Run, or panic on the main
// goroutine); it is needed only for the WebView fields, which stay empty
// when the backend does not report them.
package env

import (
	"runtime"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// Environment is a snapshot of the environment.
type Environment struct {
	OS        string // runtime.GOOS: "darwin", "windows", "linux", ...
	Arch      string // runtime.GOARCH
	OSName    string // "macOS", "Windows", "Ubuntu 24.04.1 LTS", ...
	OSVersion string // "15.3.1", "10.0.26100", kernel release on Linux
	// Debug is true in development builds (-tags tango_dev, as tan dev
	// builds) and false in production builds, the default.
	Debug bool
	// WebView names the page engine and its version when the backend
	// reports them: "WebKit" / "WebView2" / "WebKitGTK".
	WebView        string
	WebViewVersion string
}

// Info returns the environment. a may be nil, which leaves the WebView
// fields empty; after the app quit they are empty too.
func Info(a *tango.App) Environment {
	in := Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, Debug: debug}
	in.OSName, in.OSVersion = osVersion()
	if a == nil {
		return in
	}
	if v, ok := a.Backend().(platform.WebViewInfo); ok {
		type pair struct{ e, v string }
		p := tango.Await(a, func() pair { e, ver := v.WebViewVersion(); return pair{e, ver} })
		in.WebView, in.WebViewVersion = p.e, p.v
	}
	return in
}
