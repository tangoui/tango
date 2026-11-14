# Core concepts

This page covers the ideas every Tango app uses: App and Surface, the threading model, IPC, asset serving, lifecycle and app events, and the security model. The Chinese documentation splits them into [separate pages](/guide/concepts/app-surface) with more detail.

## App and Surface

The root package `github.com/tangoui/tango` has two core types. An **App** is the application: it owns the native event loop and a queue of work for the main thread. A **Surface** is one window on desktop, or one page on mobile, hosting a system WebView (WKWebView on macOS and iOS, WebView2 on Windows, WebKitGTK 4.1 on Linux, the system WebView on Android).

```go
app := tango.New(tango.Options{
    Name:       "Notes",
    ID:         "com.example.notes",
    Assets:     asset.FS(site, asset.Options{}),
    URLSchemes: []string{"notes"},
})
app.OnReady(func() {
    s, _ := app.NewSurface(tango.SurfaceOptions{Title: "Notes", Width: 960, Height: 640, Resizable: true})
    s.OnClose(func() bool { return !dirty }) // the user closes the window; false keeps it open
})
if err := app.Run(); err != nil { // blocks until the app quits
    log.Fatal(err)
}
```

At the bottom, pages and Go exchange **raw string messages**: `window.__tango.post(str)` / `onMessage(fn)` in the page, `App.OnMessage` / `Surface.Send` in Go. Everything else builds on that.

System capabilities outside the window live in one package each under `sys/` (`sys/clipboard`, `sys/dialog`, `sys/tray`, …). Their first parameter is the `*tango.App`; they return the package's `ErrUnsupported` where the platform cannot help, and `tango.ErrNotRunning` after the app quit.

Go-drawn UI (package `ui`) opens in its own window with `uihost.Open`, or on top of / below a WebView in the same window with `uihost.Attach` (macOS only so far).

## Threading

1. Call `App.Run` from the `main` goroutine. On desktop the root package locks it to the process' main thread in `init`.
2. Callbacks (`OnReady`, `OnMessage`, `Surface.OnClose`, menu and tray clicks, …) run **on the main thread**. Keep them short.
3. Everything else is safe from any goroutine; the core hops to the main thread when it needs to.

```go
app.Post(func() { s.SetTitle("done") })                       // queue, return at once
n := tango.Await(app, func() int { return len(app.Surfaces()) }) // run there, wait for the result
path := tango.AwaitAsync(app, func(done func(string)) {          // start async work there, wait for done
    pick(func(p string) { done(p) })
})
```

Called on the main thread itself, `Await` runs directly and `AwaitAsync` keeps pumping native events until `done` is called, so a dialog opened from a menu callback does not deadlock. After `Run` returns, `Post` drops work and `Await`/`AwaitAsync` return zero values.

On mobile the native host owns the main thread and Go's `main` runs on an ordinary goroutine; the rules for your code stay the same. Android cannot nest its main loop, so call functions that wait for the user (dialogs, share, permission requests) from a goroutine there.

## IPC: calls, events, streams

Package `ipc` adds three patterns on top of raw messages: calls, events, and full-duplex streams that carry JSON values and bytes. The page side is `@tangoui/runtime` (injected as `window.tango` by `runtimejs.Script`, or installed from npm).

```go
r := ipc.NewRouter()
app.AddBridgeScript(runtimejs.Script)
app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) }) // fast lanes, see below

bind.Register(r, "Notes", &Notes{})           // exported methods become Notes.Get, Notes.Search, …
r.On("draft-changed", func(c ipc.Conn, data json.RawMessage) { /* page → Go event */ })
r.Emit(s, "sync-progress", map[string]any{"pct": 42}) // Go → page event
r.HandleStream("Logs.Tail", func(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
    for line := range follow(ctx) {
        if err := w.Write(line); err != nil {
            return err // the page cancelled or went away
        }
    }
    return nil
})
```

```js
const note = await tango.call("Notes.Get", 42);
tango.emit("draft-changed", { id: 1 });
tango.on("sync-progress", ({ pct }) => (bar.value = pct));
for await (const line of tango.stream("Logs.Tail")) { if (enough) break; } // break cancels in Go
```

- **Binding**: methods may take a leading `context.Context`, then JSON-decodable parameters, and return nothing, `T`, `error` or `(T, error)`.
- **Errors** reject with a `TangoError` carrying a `code`: `not_found`, `bad_args`, `internal`, `canceled` and `transport` (frames lost on a broken fast lane, see below), plus `forbidden`, `unsupported` and `not_running` from jsapi. Return `ipc.NewError(code, msg)` to choose one.
- **Back-pressure** is counted in bytes: each direction of a stream has a credit window (1 MiB by default, `ipc.DefaultWindowBytes`; Go's side is set with `Router.SetStreamWindow`), and each frame costs its payload size plus 64 bytes. Credit returns only as the reader consumes frames, so a slow page slows the producer instead of piling up memory. A single frame carries at most 16 MiB (`ipc.MaxFrameBytes`); a larger write fails with `bad_args` and leaves the stream usable.
- **Bidirectional streams**: `HandleStream` handlers only send. `r.HandleDuplex(name, func(s *ipc.Stream, args json.RawMessage) error)` gets the stream itself: read with `s.Receive` / `ReceiveBytes` / `ReceiveJSON`, write JSON with `s.Write` and bytes with `s.WriteBytes`, concurrently if needed. `s.CloseSend()` half-closes (end), `s.Close()` closes gracefully (this side stops receiving too; the peer's writes then fail with "stream closed by peer"), `s.CloseWithError(err)` fails it (the peer gets the frames sent before it, then `err`; this side's own `Receive*`/`Write*` then return canceled "stream closed"); returning nil from the handler closes, returning an error fails. `s.ReadWriter()` views the byte frames as an `io.ReadWriteCloser` for `io.Copy`. On the page, `tango.stream(name)` returns a `TangoStream` with `send`, `sendBytes`, `end`, `accepted`, `bufferedAmount`, and Web Streams views `readable` / `writable` (`file.stream().pipeTo(s.writable)` uploads a file at Go's pace); `tango.socket(name)` wraps it in a WebSocket-like object whose `close()` sends `close` after the queued sends and fires the `close` event (1000) without waiting for Go. Streams Go opens with `r.OpenStream` are bidirectional too, and must be closed by Go. `examples/duplex` shows an echo, an upload and a small terminal.
- **In process**: Go code in the same process (a Go-drawn UI) uses the same services with `r.Call(ctx, method, args...)` and `r.Open(ctx, method, args...)`, with identical semantics but no JSON messages or base64; `ipc.ConnFrom` reports `ok=false` there. Close a stream from `Open` (or cancel its `ctx`) when done: it belongs to no page, so `Drop` does not end it. These calls block; never make them on the UI thread (see [Background data and threads](/en/guide/ui#background-data-and-threads)).
- **Fast lanes**: binary side channels next to the message channel that carry stream **byte frames** without JSON or base64 (JSON frames, control messages and the handshake stay on the message channel). Each direction is negotiated separately and frames of one stream may be split between a lane and the message channel; the receiver reorders them, so semantics do not change. Lane names: `down:scheme` / `up:post` (a streaming response and POSTs under `/__tango/lane/` on the asset origin; `down:scheme` takes frames of 4 KiB and more, `up:post` 16 KiB up to about 1 MiB) and the platform lanes `down:android` / `up:android`, `down:shm` / `up:shm`. A page gets lanes at its handshake only when **all four** hold:
  1. the app registered at least one `app.OnBinary` handler (the line above; it runs on the goroutine that received the packet, not the main thread, and the packet is only valid during the call);
  2. the page comes from the asset origin (`tango://app`, or `https://tango.localhost` on Windows and Android). A page loaded straight from a dev server (`SurfaceOptions.URL` set to `http://localhost:5173` and the like) never gets lanes; `tan dev` proxies the dev server through `asset.FromEnv`, so its pages are still on the asset origin;
  3. the platform backend reports lanes;
  4. they are not turned off with `tango.Options{DisableFastLanes: true}` or `TANGO_IPC_LANES=0` (any build; for troubleshooting).

  **Only macOS turns a lane on by default** (`up:post`, upstream byte frames); iOS and Android can opt in and are off by default; Windows and Linux still use the message channel until each platform's backend passes its benchmark. Wire `OnBinary` now and nothing changes later.

  **macOS and iOS** offer `up:post` only; downstream byte frames stay on the message channel, because WKWebView holds back part of a large burst on a streamed `tango://` response until more data arrives on it. **On macOS `up:post` is on by default** (64 MiB uploads run about 2.73x faster than on the message channel, see `docs/ipc-bench.md`); **on iOS it is off by default** until it is measured on a device. Both read `TANGO_IPC_LANES_DARWIN` once when a window or page is created (trimmed, any case): `0`, `false` or `off` turns lanes off on both, in any build; unset or empty means the platform default (macOS `up:post`, iOS off); `all` offers `down:scheme` as well, **for experiments only** (held-back frames can fail a stream with `transport`), on macOS only in development builds (`-tags tango_dev`; release builds treat it like the default) and on iOS in any build; any other value (normally `1`) offers `up:post`. `TANGO_IPC_LANES=0` or `DisableFastLanes` always wins. An app on a real iOS device cannot get an environment variable from outside: to opt in, call `os.Setenv("TANGO_IPC_LANES_DARWIN", "1")` in Go before the first window or page is created. **CSP**: `up:post` starts by POSTing an empty packet and is ready only on its 204, so a page whose Content-Security-Policy does not allow `connect-src` to the asset origin (for example `connect-src https://api.example.com`, or a `default-src` without `'self'`) simply keeps its byte frames on the message channel, with no error; add `'self'` to `connect-src` to get the lane. Known limitations: iframes cannot obtain the lane token (the scheme handler cannot tell which frame asked, so the token is only handed to the main frame's bridge; measured on macOS, not separately on iOS: lane requests from opaque-origin frames such as sandboxed `srcdoc` or `data:` and from a same-origin frame were all refused; a same-origin frame can of course reach `parent.tango`, which is app code); WebKit does not deliver `Blob` request bodies, so `up:post` sends an `ArrayBuffer` (iOS rewrites an app's own Blob bodies; on macOS, Blob bodies were not separately measured, so send an `ArrayBuffer` or a string); WebKit sends no `Origin` on `tango://` requests, so Tango substitutes the main document's origin, and a cross-origin iframe inside an app page passes the asset CSRF check (pre-existing; lanes are still protected by their per-generation token), so do not embed untrusted frames in asset pages; iOS was tested on the simulator only, not on devices, and the lane-breaking scenario is not automated on iOS (the fallback path is platform-independent and verified on macOS).

  **Android** offers `down:android` and `up:android`: byte frames in both directions travel as `ArrayBuffer`s over the page's existing `WebMessageListener` channel, with no minimum frame size. They need WebView 116 or later and are **off by default** (tested on the emulator only): `TANGO_IPC_LANES_ANDROID` is read once when a page is created (trimmed, any case); unset, empty, `0`, `false` or `off` is off, any other value (normally `1`) is on, and `TANGO_IPC_LANES=0` or `DisableFastLanes` always wins. An Android app cannot get an environment variable from outside: call `os.Setenv("TANGO_IPC_LANES_ANDROID", "1")` in Go before the first page is created. Every packet carries the page token and the lane generation, so packets of an old page or generation never reach a new one in either direction; a packet Go refuses makes the page drop `up:android` at once and later frames take the message channel. Messages from subframes are dropped (measured on the emulator: a same-origin iframe forging packets through its own `__tangoAndroid` got none to Go, and an opaque-origin iframe has no such object); a same-origin frame can reach `parent.__tangoNative`, which is app code. Each packet is copied once across processes and once through JNI. See `docs/mobile.md`.

  `tango.lanes()` on the page returns the lanes in use, e.g. `{down: "fallback", up: "up:post"}` on macOS by default, or `"fallback"` for a direction without one. A lane that breaks is dropped at once and later frames take the message channel; a frame lost on it is not resent: when a stream misses a frame for 5 s (`ipc.GapTimeout`, `GAP_TIMEOUT_MS` in JS) once a reliable lane sequence boundary, later frame, or its end exposes the missing frame (including an open duplex lost tail), the stream fails on both sides with code `transport` (`ipc.CodeTransport`); treat it like a dropped connection and reopen the stream. In development builds (`-tags tango_dev`) and self-test builds (`-tags tango_selftest`, which `tan build` keeps; the iOS simulator self-test uses it) `TANGO_IPC_LANE_FAULT=<lane>:<N>` (e.g. `down:scheme:3`) makes that lane fail after N data packets sent (downstream) or received (upstream) per page generation (empty packets, such as `up:post`'s start probe, are not counted), to test a lane breaking midway; release builds ignore it.

  Fast-lane delivery is bound to the document session (lane generation): the lane token is per generation and used by every lane request in it, and a reload or leaving the asset origin retires it. Hand `OnBinary`'s `gen` to `r.HandleBinarySession(s, gen, p)` so packets of a previous document never reach the new one; `r.HandleBinary` is only for simple connections that never host a second document, and drops every packet for a `*tango.Surface` (logging one warning). Feed a surface to one `Router` only: each handshake starts a new lane generation, so two Routers on one surface would invalidate each other. Reliable stream `boundary` messages (sent on the message channel once a lane frame is on its way) expose even a lost final frame of an open duplex; only streams with unresolved receive sequences fail after 5 seconds. For a stalled page (`alert()`, a long task) the guarantee is: the runtime sends a lane frame's boundary only once the frame actually went out (`up:post`: when its batch is handed to `fetch`), and every later message-channel post of that stream (data, end, close) waits for it too, so Go never starts timing a frame still queued in the page; the page's gap timer and the `up:post` request timer, when they fire more than 1 s late, wait one more full timeout, and a second expiry fails as usual. Abort paths (writable abort, a refused chunk, `cancel()`, a local error) still post their close/error at once, so under a stall longer than 5 s Go may report `transport` instead of the abort reason. Each `up:post` request aborts after 4 seconds (below the 5 s gap timeout, so a hung POST fails the lane and later frames fall back before streams fail); retained encoded POST bytes, in flight included, are capped at 8 MiB, and excess frames take the message channel without changing sequence or credit. Lane requests resolve against the document's actual location, independent of HTML `<base>`, and refuse redirects.
- **Wire protocol**: version 2. The runtime and Go shake hands on load and Go rejects any other version ("runtime-js protocol mismatch"), so use the `@tangoui/runtime` release that matches the Go module. The message format is documented in the `ipc` package comment.
- **TypeScript generation**: `tan generate bindings` finds every `bind.Register` call in the main package and writes typed clients, mapping Go types the way `encoding/json` encodes them.

## Asset serving

`Options.Assets` is a plain `http.Handler` served under the backend's asset origin (`tango://app` on macOS, iOS and Linux, `https://tango.localhost` on Windows and Android) — no local port is opened.

```go
Assets: asset.FromEnv(site, asset.Options{SPA: true, Headers: csp})
```

`asset.FS` serves an `fs.FS` (usually `embed.FS`); `asset.FromEnv` switches to a reverse proxy to the dev server when `TANGO_DEV_URL` is set, which `tan dev` does, but only in development builds (`-tags tango_dev`, added by `tan dev`); a plain `go build` / `go run .` always serves the embedded files. Non-GET/HEAD requests to the asset origin whose `Origin` is another site get 403 before they reach your handler (`tango.Options.AllowCrossOriginAssets` turns this off). Compose it with your own handlers freely; `service/fileserver` mounts a user directory safely (no `..`, opened through `os.Root`). Handlers may be called off the main thread. On macOS and iOS the `tango://` scheme handler is asynchronous: each request runs your handler on its own goroutine, never on the main thread (so requests can be served concurrently). Large or long-lived responses (big files, server-sent events) therefore do not block the UI, and when WebKit cancels a request, `r.Context()` is cancelled. A handler that touches main-thread-only state should hand the work to the main thread with `app.Post` or use a lock; `tango.Await` works too, but blocks the response until the main thread has run it.

## Lifecycle and app events

```go
app.OnLifecycle(func(ev tango.Lifecycle) {
    switch ev {
    case tango.Started, tango.Foreground, tango.Background, tango.Terminating:
    }
})
app.OnQuitRequest(func() bool { return !unsaved }) // only user quits (Cmd+Q, a Quit item)
app.OnOpenURL(func(u string) { route(u) })         // deep links
app.OnOpenFiles(func(paths []string) { open(paths) })
```

App-level notifications arrive through `App.OnEvent`: open URL, open files, theme changed, displays changed, reopen (macOS Dock click), notification opened, and on mobile safe area, orientation, push and WebView-restored events. The deep link that launched the app is kept until a listener is registered. `sys/single` forwards a second launch's arguments to the running instance. `Options.QuitOnLastClose` decides whether closing the last window quits (default: no on macOS, yes on Windows and Linux); windows created with `SurfaceOptions{Auxiliary: true}` (an update window, a tool palette) do not count.

## Security model

Tango assumes the page is **not fully trusted**:

1. **Message origin check** — the core accepts messages only from the asset origin and from the origin it last navigated the surface to; everything else is dropped before `OnMessage`. Only main-frame messages are delivered.
2. **jsapi allow-list** — page access to system capabilities is grouped (`Window`, `Menu`, `Info` on by default; `ClipboardRead`, `ClipboardWrite`, `Dialogs`, `Notify`, `OpenURL`, `OpenPath`, `AppControl`, `Hotkeys` off by default) and enabled in Go with `jsapi.Install(app, r, jsapi.Options{Allow: …})`. Disabled capabilities reject with `forbidden`. Window methods only ever act on the calling window.
3. **Narrowed operations** — `shell.OpenURL` accepts only `http`, `https` and `mailto` unless you change it (`file:` never); page-described menus may only use edit roles; global shortcuts belong to the page that registered them.
4. **WebView policies** — `SetNavigationPolicy`, `SetPermissionPolicy` / `SetPermissionHandler` (camera, microphone, geolocation, …) and `SetNewWindowHandler` let Go decide; by default new windows for the app's own origin open as surfaces, `http(s)`/`mailto` go to the system browser, and anything else is denied.
5. **Production by default** — every build without `-tags tango_dev` (plain `go build`, `go run .`, goreleaser, `tan build` including `-debug`) leaves out the development features (hot reload, automation channel, dev-server proxy) and ignores their environment variables, so another process of the same user cannot turn them on. Only `tan dev` adds `tango_dev`. The old `tango_release` tag no longer does anything.

Methods you expose yourself with `bind` or `Handle` are reachable from the page too: validate their arguments. Report vulnerabilities privately as described in [SECURITY.md](https://github.com/tangoui/tango/blob/main/SECURITY.md).
