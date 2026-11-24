package corehook

import (
	"net/http"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// NoteSurfaceActivity records input observed by the canvas or dev host.
var NoteSurfaceActivity func(surface any, at time.Time)

// WatchSurfaceActivity sets the dev host relay on its top-level surface.
var WatchSurfaceActivity func(surface any, fn func(time.Time))

// NewEmbeddedWebView creates a WebView embedded in the content area of the
// canvas surface parent (a *tango.Surface), see
// docs/specs/webview-embed.md. opts is a tango.SurfaceOptions: URL, Debug
// and WebView apply, window fields are ignored. script is appended to the
// bridge scripts of the new page. The new *tango.Surface is an ordinary
// WebView surface for Navigate, Eval, Send, OnMessage, OnLoaded, the
// WebView extensions and OnClosed; ev places it. It is closed with its
// parent. Main thread only. It returns an error wrapping
// platform.ErrUnsupported when the backend cannot embed WebViews. Set by
// package tango.
var NewEmbeddedWebView func(parent any, canvas platform.Canvas, opts any, script string) (surface any, ev platform.EmbeddedWebView, err error)

// EmbeddedParent returns the *tango.Surface (as any) an embedded WebView
// surface lives in, or nil when surface is a window. Set by package tango.
var EmbeddedParent func(surface any) any

// ProxyWebViewEvents relays the backend callbacks of a dev host WebView.
type ProxyWebViewEvents struct {
	Event      func(platform.SurfaceEvent)
	Message    func(origin, msg string)
	Asset      func(http.ResponseWriter, *http.Request)
	Navigation func(uint64, platform.WebNavigationKind, platform.WebPageState, string)
}

// ProxyWebViewParent is the native part of a canvas surface in a dev worker.
type ProxyWebViewParent interface {
	NewProxyWebView(platform.SurfaceID, platform.SurfaceOptions, platform.WebOptions, ProxyWebViewEvents) (platform.EmbeddedWebView, error)
}

// HostProxyWebView opens the native side of a dev worker's embedded view.
// The bridge and asset callbacks belong to the worker's surface.
var HostProxyWebView func(parent any, opts platform.SurfaceOptions, web platform.WebOptions, events ProxyWebViewEvents) (platform.EmbeddedWebView, error)

// AdoptEmbeddedWebView returns an existing embedded surface's view.
// Main thread only. Set by package tango.
var AdoptEmbeddedWebView func(parent any, canvas platform.Canvas, child any) (platform.EmbeddedWebView, error)

// CommitWebPlacement applies the native placement and records its revision
// for page operations. Main thread only. Set by package tango.
var CommitWebPlacement func(surface any, ev platform.EmbeddedWebView, placement platform.WebPlacement)

// NewEmbeddedNativeView embeds the application's native view (NSView*,
// HWND, GtkWidget*) in the content area of the canvas surface parent (a
// *tango.Surface), see platform.NativeViewEmbedder. closed runs on the
// main thread if the backend removes the view because parent closed. Main
// thread only. It returns an error wrapping platform.ErrUnsupported when
// the backend or parent cannot host native views. Set by package tango.
var NewEmbeddedNativeView func(parent any, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error)
