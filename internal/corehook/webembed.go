package corehook

import "github.com/tangoui/tango/internal/platform"

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
var NewEmbeddedWebView func(parent any, opts any, script string) (surface any, ev platform.EmbeddedWebView, err error)

// EmbeddedParent returns the *tango.Surface (as any) an embedded WebView
// surface lives in, or nil when surface is a window. Set by package tango.
var EmbeddedParent func(surface any) any

// AdoptEmbeddedWebView returns an existing embedded surface's view.
// Main thread only. Set by package tango.
var AdoptEmbeddedWebView func(parent, child any) (platform.EmbeddedWebView, error)

// CommitWebPlacement applies the native placement and records its revision
// for page operations. Main thread only. Set by package tango.
var CommitWebPlacement func(surface any, ev platform.EmbeddedWebView, placement platform.WebPlacement)
