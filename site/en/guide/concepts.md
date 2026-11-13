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

Package `ipc` adds three patterns on top of raw messages; the page side is `@tangoui/runtime` (injected as `window.tango` by `runtimejs.Script`, or installed from npm).

```go
r := ipc.NewRouter()
app.AddBridgeScript(runtimejs.Script)
app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })

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
- **Errors** reject with a `TangoError` carrying a `code`: `not_found`, `bad_args`, `internal`, `canceled` (plus `forbidden`, `unsupported`, `not_running` from jsapi). Return `ipc.NewError(code, msg)` to choose one.
- **Back-pressure**: a stream sender may have at most 64 unacknowledged messages; the page acknowledges every 16. A slow page slows the producer instead of piling up memory.
- **TypeScript generation**: `tan generate bindings` finds every `bind.Register` call in the main package and writes typed clients, mapping Go types the way `encoding/json` encodes them.

## Asset serving

`Options.Assets` is a plain `http.Handler` served under the backend's asset origin (`tango://app` on macOS and Linux, `https://tango.localhost` on Windows) — no local port is opened.

```go
Assets: asset.FromEnv(site, asset.Options{SPA: true, Headers: csp})
```

`asset.FS` serves an `fs.FS` (usually `embed.FS`); `asset.FromEnv` switches to a reverse proxy to the dev server when `TANGO_DEV_URL` is set, which `tan dev` does, but only in development builds (`-tags tango_dev`, added by `tan dev`); a plain `go build` / `go run .` always serves the embedded files. Non-GET/HEAD requests to the asset origin whose `Origin` is another site get 403 before they reach your handler (`tango.Options.AllowCrossOriginAssets` turns this off). Compose it with your own handlers freely; `service/fileserver` mounts a user directory safely (no `..`, opened through `os.Root`). Handlers may be called off the main thread.

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

App-level notifications arrive through `App.OnEvent`: open URL, open files, theme changed, displays changed, reopen (macOS Dock click), notification opened, and on mobile safe area, orientation, push and WebView-restored events. The deep link that launched the app is kept until a listener is registered. `sys/single` forwards a second launch's arguments to the running instance. `Options.QuitOnLastClose` decides whether closing the last window quits (default: no on macOS, yes on Windows and Linux).

## Security model

Tango assumes the page is **not fully trusted**:

1. **Message origin check** — the core accepts messages only from the asset origin and from the origin it last navigated the surface to; everything else is dropped before `OnMessage`. Only main-frame messages are delivered.
2. **jsapi allow-list** — page access to system capabilities is grouped (`Window`, `Menu`, `Info` on by default; `ClipboardRead`, `ClipboardWrite`, `Dialogs`, `Notify`, `OpenURL`, `OpenPath`, `AppControl`, `Hotkeys` off by default) and enabled in Go with `jsapi.Install(app, r, jsapi.Options{Allow: …})`. Disabled capabilities reject with `forbidden`. Window methods only ever act on the calling window.
3. **Narrowed operations** — `shell.OpenURL` accepts only `http`, `https` and `mailto` unless you change it (`file:` never); page-described menus may only use edit roles; global shortcuts belong to the page that registered them.
4. **WebView policies** — `SetNavigationPolicy`, `SetPermissionPolicy` / `SetPermissionHandler` (camera, microphone, geolocation, …) and `SetNewWindowHandler` let Go decide; by default new windows for the app's own origin open as surfaces, `http(s)`/`mailto` go to the system browser, and anything else is denied.
5. **Production by default** — every build without `-tags tango_dev` (plain `go build`, `go run .`, goreleaser, `tan build` including `-debug`) leaves out the development features (hot reload, automation channel, dev-server proxy) and ignores their environment variables, so another process of the same user cannot turn them on. Only `tan dev` adds `tango_dev`. The old `tango_release` tag no longer does anything.

Methods you expose yourself with `bind` or `Handle` are reachable from the page too: validate their arguments. Report vulnerabilities privately as described in [SECURITY.md](https://github.com/tangoui/tango/blob/main/SECURITY.md).
