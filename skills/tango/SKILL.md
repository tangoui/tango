---
name: tango
description: Use when creating, changing, debugging, verifying, or packaging a Tango desktop or mobile application using github.com/tangoui/tango, the tan CLI, or tan.json.
metadata:
  tango-version: dev
---

# Develop Tango applications

Before writing Go, look up system capabilities in `sys/*` and `service/*`. Use package functions such as `menu.SetAppMenu(app, m)`; storage paths take the application ID. Run `go doc github.com/tangoui/tango/<package> <Symbol>` from the app directory to check the installed version's signatures. Look up the root package directly for `App` and `Surface`.

| Need | Package | Start here |
|---|---|---|
| Windows, visibility, geometry | `tango` | `App.NewSurface`, `SurfaceOptions`, `Surface.IsVisible`, `Surface.Bounds`, `Surface.OnMoved`, `Surface.OnResized` |
| Remember placement | `sys/windowstate` | `Track` |
| Menus and shortcuts | `sys/menu` | `New`, `Menu.Submenu`, `Menu.Add`, `Item.Accelerator`, `SetAppMenu`, `SetSurfaceMenu`, `EditMenu`, `Menu.Update` |
| Tray icon and menu | `sys/tray` | `New` with PNG bytes in `Options.Icon`, `Tray.SetMenu`, `Tray.SetTitle` (macOS) |
| System notifications | `sys/notify` | `RequestPermission`, `Send` (run a macOS `.app`) |
| Pick a file or folder | `sys/dialog` | `Open`, `FileOptions.PickFolders`, `ErrCanceled` |
| App data directory | `service/paths` | `Data(appID)` |
| App visibility and Dock | `sys/appctl` | `Hide`, `Show`, `SetDockVisible` |
| Startup and reopen | `tango` | `App.OnReady`, `App.OnEventKind`, `EventReopen`, `Options.QuitOnLastClose` |
| Go-to-page events | `ipc` | `Router.Emit(surface, name, data)` per page. Use `App.Surfaces` to find open windows. |
| Calling window in a binding | `ipc` | `ConnFrom(ctx)` and a checked `*tango.Surface` assertion |
| Clipboard | `sys/clipboard` | `ReadText`, `WriteText` |
| Single instance | `sys/single` | `Claim`, `OnSecondInstance` |
| Watch files | `github.com/fsnotify/fsnotify` | Add a file-watching dependency. |

Identify the project route from `tan.json`, the Go entry point, and the presence of a frontend directory. For `ui` and other Go-drawn views, read [Native UI](references/ui-native.md). For Vue, React, shadcn, vanilla or hybrid WebView content, read [WebView apps](references/webview-app.md).

Run the app and exercise the changed behavior before reporting completion. Use the development build for automation and a packaged `.app` for macOS notifications. [Verification](references/verify.md)

## References

- Look up the imports, call order, and minimal Go examples for a capability. [Capabilities](references/capabilities.md)
- Translate API assumptions from Wails or Electron before writing code. [API mappings](references/coming-from-wails-electron.md)
- Write a Go-drawn window with stable list keys, UI-thread updates, and timed redraws. [Native UI](references/ui-native.md)
- Set up page assets, bindings, the runtime, and events. [WebView apps](references/webview-app.md)
- Diagnose code that compiles but behaves incorrectly. [Pitfalls](references/pitfalls.md)
- Start an app in the background, drive it, and capture a screenshot. [Verification](references/verify.md)
- Build desktop artifacts and find the settings for packaging or signing. [Packaging](references/packaging.md)
- Find the mobile setup and target-specific requirements. [Mobile guide](https://tangoui.github.io/tango/guide/mobile)

## Runnable examples

- Try a native checklist, focus timer, tray, and menus. [Native tray timer](examples/native-tray-timer/main.go)
- Try an embedded Markdown editor with Go bindings, menu events, a save dialog, and window state. [WebView notes](examples/webview-notes/main.go)
