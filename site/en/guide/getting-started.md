# Getting started

This page takes you from nothing to a running Tango app: install the `tan` command, create a project from a template, run it in development mode, and make a release build — once for desktop and once for mobile.

::: warning Current status
Tango is an early 0.x release. The desktop backends are verified on **macOS** hardware; the Windows and Linux backends are implemented and pass cross-compilation and CI smoke tests, but have not been verified systematically on real machines. Mobile (iOS, Android) is **experimental**: verified on simulators/emulators only, not on devices.
:::

## Requirements

| | Needed |
|---|---|
| All platforms | Go 1.27 or newer |
| macOS | Xcode command line tools (signing, `lipo`, `hdiutil`); WKWebView ships with the OS |
| Windows | WebView2 runtime (usually present on Windows 10 1803+ / 11); NSIS (`makensis`) for installers |
| Linux | GTK 3, WebKitGTK 4.1, libayatana-appindicator3 (tray); `appimagetool` for AppImages |
| Front-end templates (optional) | Node.js and npm, only for the Vite-based templates; the default `vanilla` and `ui` templates need neither |
| iOS (optional) | macOS with Xcode (and its simulators) |
| Android (optional) | JDK 17, Android SDK and NDK |

Desktop builds need **no cgo**: the backends call the system libraries through purego / syscall, so one machine can cross-compile all three desktop platforms. Mobile builds use cgo (`c-archive` / `c-shared`), which `tan build` sets up.

## Install tan

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan version
tan doctor        # checks the toolchain and platform dependencies
```

Prebuilt `tan` binaries are attached to each [release](https://github.com/tangoui/tango/releases).

::: details Before the first release: install from source
```bash
git clone https://github.com/tangoui/tango
cd tango/cmd/tan && go install .
```
Create projects against your checkout with `-local`: `tan init -local ~/src/tango hello`.
:::

## Create a project

```bash
tan init hello        # create with defaults
tan init              # in a terminal: answer a few questions
tan init -yes         # no questions, all defaults (directory tango-app)
cd hello
```

| Flag | Default | Meaning |
|---|---|---|
| `-template` | `vanilla` | project template (below) |
| `-name` | directory name | display name |
| `-id` | `com.example.<name>` | reverse-DNS app id (bundle id / applicationId) |
| `-module` | directory name | Go module path |
| `-mobile` | `true` | write the `ios` and `android` sections of tan.json |
| `-local` | | use a local Tango checkout (replace directive) |

| Template | Contents |
|---|---|
| `vanilla` | plain HTML/CSS/JS, no npm |
| `vanilla-ts` / `react-ts` / `vue-ts` | TypeScript, React or Vue with Vite |
| `shadcn` | React + Vite with Tailwind CSS and shadcn/ui |
| `ui` | self-drawn Go UI (package `ui`), no WebView, no npm (experimental) |
| `hybrid` | a WebView page with a self-drawn tool panel on top (experimental; macOS only for now) |

The heart of the generated `main.go`:

```go
app := tango.New(tango.Options{
    Name:   "hello",
    ID:     "com.example.hello",
    Assets: asset.FromEnv(files, asset.Options{SPA: true}), // proxies to the dev server under tan dev
})

r := ipc.NewRouter()
bind.Register(r, "Greeter", &Greeter{})        // page: tango.call("Greeter.Hello", "Tom")
app.AddBridgeScript(runtimejs.Script)           // injects window.tango
app.OnMessage(func(s *tango.Surface, msg string) { r.HandleMessage(s, msg) })

app.OnReady(func() {
    app.NewSurface(tango.SurfaceOptions{Title: "hello", Width: 960, Height: 640, Resizable: true})
})
app.Run()
```

## Development: tan dev

```bash
tan dev
```

`tan dev` starts the front-end dev server when `frontend.dev` is set (Vite templates: `npm run dev`) and hands its URL to the app through `TANGO_DEV_URL` (read only by development builds: `tan dev` builds with `-tags tango_dev`); builds and runs the Go program; rebuilds and restarts it when Go sources change, restoring window positions; and hot-reloads self-drawn (`uihost`) windows without closing them. `-url` uses a dev server that is already running; `-C <dir>` picks the project; arguments after `--` go to the app.

## Release build: tan build

```bash
tan build                                        # this machine
tan build -platform darwin/universal             # universal macOS binary (needs lipo)
tan build -platform windows/amd64,linux/arm64    # cross-compile
```

`tan build` runs `frontend.build`, then compiles a production build (without `tango_dev`, so no hot-reload host, automation channel or dev proxy; a plain `go build` / `go run .` is a production build too) into `build/<os>-<arch>/`: a signed-ad-hoc `.app` on macOS, an `.exe` with icon, manifest and version info on Windows, an executable on Linux. Installers come from `tan package`, signing from `tan sign`.

## Mobile (experimental)

The same project builds for mobile without code changes. `name`, `id`, `version` and `icon` from tan.json are shared; declare camera, location and similar capabilities in `permissions`.

```bash
tan run -platform ios-simulator                  # build, install, launch; Go logs in the terminal
tan build -platform ios                          # device .app + .ipa (needs signing identity and profile)

tan run -platform android                        # debug APK for the device's ABI, install, launch, logcat
tan build -platform android/arm64,android/amd64  # release: signed APK and AAB
```

## Next

- How `App`, `Surface`, the main thread and IPC fit together: [Core concepts](/en/guide/concepts)
- The command reference: [tan CLI](/en/reference/cli)
- A complete demo: `examples/showcase` in the repository (`go run ./examples/showcase`)
