// Package windows is Tango's Windows backend: Win32 windows hosting
// Microsoft Edge WebView2, called through package syscall and COM vtables
// (no cgo).
//
// Files without a platform suffix hold pure logic (DPI conversion,
// accelerator parsing, menu planning, HTTP adaptation, UTF-16 helpers,
// runtime discovery) and build and test on every OS. Files ending in
// _windows*.go hold the native code.
//
// WebView2Loader.dll is not required: like mygo, the backend locates the
// installed (Evergreen) or fixed-version WebView2 Runtime itself (registry
// EdgeUpdate\ClientState\<channel>\EBWebView, or the
// WEBVIEW2_BROWSER_EXECUTABLE_FOLDER variable) and calls
// CreateWebViewEnvironmentWithOptionsInternal exported by the runtime's
// EmbeddedBrowserWebView.dll, which is what the loader does. An official
// WebView2Loader.dll placed next to the executable takes precedence.
//
// Parts of this package are adapted from MIT-licensed code:
//
//   - mygo (https://github.com/egoist/mygo), Copyright (c) 2026 MyGo
//     contributors: the ARM64/x64 RECT passing (abi_*.go, zoom_*.go) and
//     the argument order of the undocumented
//     CreateWebViewEnvironmentWithOptionsInternal export. The COM helpers,
//     runtime discovery, dialogs and activation context were rewritten
//     from Microsoft's documentation and no longer derive from it.
//   - Wails v3 (https://github.com/wailsapp/wails), Copyright (c)
//     2018-Present Lea Anthony: WebView2 interface vtable layouts.
//
// Both are distributed under the MIT License:
//
//	Permission is hereby granted, free of charge, to any person obtaining a
//	copy of this software and associated documentation files (the
//	"Software"), to deal in the Software without restriction, including
//	without limitation the rights to use, copy, modify, merge, publish,
//	distribute, sublicense, and/or sell copies of the Software, and to
//	permit persons to whom the Software is furnished to do so, subject to
//	the following conditions:
//
//	The above copyright notice and this permission notice shall be included
//	in all copies or substantial portions of the Software.
//
//	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
//	OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
//	MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
//	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
//	CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
//	TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
//	SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
package windows

// assetOrigin is where Host.ServeAsset is mounted. WebView2 treats
// *.localhost as a secure context; requests to it are intercepted with
// WebResourceRequested and never reach the network.
const assetOrigin = "https://tango.localhost"

// nativeShim defines window.__tangoNative before any page script runs.
const nativeShim = `window.__tangoNative={post:s=>chrome.webview.postMessage(String(s))};`

// topFrameOnly wraps a document-created script so it runs in the main frame
// only: WebView2 injects AddScriptToExecuteOnDocumentCreated scripts into
// every frame, and iframes get no WebMessageReceived delivery anyway.
func topFrameOnly(js string) string {
	return "if (window === window.top) {\n" + js + "\n}"
}
