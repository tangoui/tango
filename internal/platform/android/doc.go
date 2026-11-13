// Package android is Tango's Android backend: one Activity (the Java host in
// mobile/android) whose content is a stack of full-screen pages, WebViews or
// self-drawn canvases, driven over JNI. Go is built as a c-shared library
// (libtango.so) that the host loads with System.loadLibrary.
//
// Files without a build tag hold pure logic (the page script and its
// injection into HTML, net/http adaptation of intercepted requests,
// response streaming, lifecycle replay) and are unit tested on any OS.
// Files ending in _android.go use cgo and JNI.
//
// # Running user code
//
// In c-shared mode the Go runtime initializes packages but never calls
// main.main. The host's Activity.onCreate calls Tango.nativeInit, which
// starts main.main (reached with go:linkname, so user code is the same
// `func main() { app := tango.New(...); app.Run() }` as on desktop) on a
// new goroutine, once per process. App.Run therefore runs off the Android
// main thread: Backend.Run only blocks until the app terminates. When
// main.main returns the host finishes the Activity and exits the process.
//
// # Threads
//
// Backend and Surface methods run on the Android main thread (the core
// reaches it through Wake: Java posts a Runnable to the main Looper that
// calls back into Host.Drain). Host calls are made from Java callbacks that
// already run there, except ServeAsset, which runs on the WebView's
// request thread as the contract (round 2) allows. Step cannot pump the
// Looper from inside a callback; see Backend.Step.
//
// # Pages
//
// Each Surface is a WebView added to the Activity's root FrameLayout; the
// first fills the screen and later ones are stacked over it (no new
// Activities). Canvas surfaces (uihost.Open, canvas_android.go) are pages
// of the same stack whose full-page SurfaceView Go draws into. The system back gesture goes back in the top page's history
// and, when there is none, asks Host.ShouldClose for that page; closing the
// last page that way finishes the Activity, which ends the app.
//
// # Assets and the bridge
//
// Pages load from https://tango.localhost, intercepted in
// WebViewClient.shouldInterceptRequest. Android never hands request bodies
// to that callback; the page script carries them to Go separately
// (body.go). Main-frame HTML responses get the bridge script (shim +
// BridgeScript) inserted at the start of <head>, so it runs before any page
// script. A document of the other trusted origin (below) that Go did not
// serve, such as a page on a dev server the app navigated to, gets it with
// evaluateJavascript when the page starts loading, which can be after the
// page's own early scripts. Documents of any other origin get no script.
//
// Only the app's own documents reach Go. The trusted origins are the
// core's (Surface.trusts): the asset origin and the origin of the URL the
// core last navigated the surface to. The script is per document: it holds
// a random token in its closure, a new one for every main-frame document,
// and Go accepts messages and request bodies only with the current
// document's token; the previous one stops working when the next document
// starts (token.go). The origin reported to the core is that of the
// document the token was issued to, fixed when the message arrives.
//
// The channel object (window.__tangoAndroid, which the shim takes into its
// closure and deletes) is a WebMessageListener (androidx.webkit) when the
// WebView supports it: the WebView puts it only in frames of the trusted
// origins and reports each message's origin and frame; the host passes on
// main-frame messages only, and Go also requires the origin to be the
// token's. Older WebViews get a JavascriptInterface instead, which every
// frame of every document sees and which does not say who called: there
// the token alone, which only the main frame's script holds, tells the
// app's document apart.
//
// Android pitfalls were learned from Wails v3's Android port
// (github.com/wailsapp/wails, MIT): main.main not running in c-shared
// mode, shouldInterceptRequest lacking request bodies. The design and code
// here are Tango's own.
package android
