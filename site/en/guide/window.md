# Windows & appearance

On desktop, each Surface is a native window. Call its methods from any goroutine. The corresponding page APIs live in the `Window` module of `@tangoui/runtime`. Desktop backends support different options and ignore options they cannot provide. [Platform support](/en/guide/platforms)

## Create a window

```go
s, err := app.NewSurface(tango.SurfaceOptions{
    Title: "Editor", Width: 1100, Height: 720,
    MinWidth: 640, MinHeight: 400,
    Resizable: true, Hidden: true, Centered: true,
    State: tango.StateNormal, Kind: tango.KindWindow,
    Appearance: tango.Appearance{TitleBar: tango.TitleBarHidden},
    AcceptFiles: true, URL: "/", Debug: true,
})
```

Dimensions describe the content area in logical pixels. A zero width or height uses the backend default. Zero maximum dimensions impose no limit. Use `Show()` to reveal a hidden window. `X` and `Y` position the outer frame. Window states include normal, maximized, fullscreen and minimized. `KindPanel` is a floating tool window. `KindModal` and `KindSheet` require a parent. A macOS sheet attaches to its parent and other desktop backends treat it as a modal window.

## Geometry and state

Use `Size` / `SetSize`, `SetSizeLimits`, `Bounds` / `SetBounds` and `Center` for geometry. `State`, `Maximize`, `Minimize`, `Fullscreen` and `Restore` manage window state. Listen with `OnResized`, `OnMoved`, `OnStateChanged`, `OnFocused` and `OnBlurred`.

`Rect.X` and `Rect.Y` locate the top left of the outer frame, including decoration, in logical pixels from the top left of the primary display. Y increases downward. Width and height describe the content area. Display and tray bounds use the same coordinates.

```go
s, _ := app.NewSurface(tango.SurfaceOptions{Hidden: true, Resizable: true})
windowstate.Track(s, "main", nil) // <UserConfigDir>/<App.ID>/windows.json
s.Show()
```

Track the window before showing it. Windows saved off all current displays are recentered. [windowstate](/guide/system/windowstate)

## Appearance

Use `SetAppearance(tango.Appearance{...})` to replace the window's appearance and `Appearance()` to read it. The zero value describes a normal window.

- `TitleBarHidden` extends content under the title bar while keeping native controls. `TitleBarNone` removes the border.
- `Transparent` makes the window and WebView backgrounds transparent. `Background` is a `0xRRGGBBAA` color for an opaque window.
- `MaterialBlur` uses native blur or Acrylic where available. `MaterialMica` is for Windows 11.
- `AlwaysOnTop`, `Opacity`, `NoMinimize`, `NoClose`, `NoMaximize` and `NoShadow` control individual properties. `AllSpaces` is supported on macOS and Linux.

On windows with a hidden or absent title bar, mark draggable page regions with `--tango-drag: drag` or `app-region: drag`. Use `--tango-drag: no-drag` on controls inside such a region. Native edge resizing depends on the backend.

## File drag and drop

Set `AcceptFiles` and use `s.OnFilesDropped(func(paths []string, x, y int))` or `Window.onFilesDropped`. The callback gets absolute paths. The page can use native `dragenter`, `dragover` and `dragleave` for highlighting and pass callback coordinates to `document.elementFromPoint`. On macOS, the page receives `dragleave` on release, so use the native callback to process the drop.

To drag files out, call `s.DragFiles(paths, iconPNG, done)` while the pointer is held down. Forward the page's `mousedown` or `dragstart` to Go and prevent the page's default drag operation.

## WebView controls

| Methods | Purpose |
|---|---|
| `Navigate(url)` / `Reload()` | Navigate relative to the asset origin or reload |
| `Eval(js)` / `Send(msg)` | Evaluate a script or send a raw message |
| `SetZoom(f)` / `Zoom()` | Page zoom |
| `Print()` / `PrintToPDF(opts)` | Print or export PDF |
| `Screenshot()` | PNG screenshot |
| `Find(text, opts)` / `FindNext()` / `FindPrevious()` / `StopFind()` | Find in page |
| `OpenDevTools()` | Open the inspector when `Debug` is enabled |
| `SetUserAgent` / `SetExtraHeaders` | Set the user agent and extra headers |
| `Cookies` / `SetCookie` / `DeleteCookie` / `ClearBrowsingData` | Cookies and website data |
| `OnLoaded` / `OnNavigating` / `OnLoadFailed` | Load events |

Go handlers control downloads, HTTP authentication, new windows, permissions and navigation policy. [Security model](/en/guide/concepts#security-model)

## Close a window

```go
s.OnClose(func() bool { return !dirty }) // any false return vetoes a user close
s.OnClosed(func() { r.Drop(s) })         // once, after native destruction
s.Close()                              // programmatic close skips OnClose
```

## Control a window from the page

```js
import { Window } from "@tangoui/runtime";
await Window.setTitle("Untitled");
await Window.setSize(900, 600);
const off = Window.onResize(({ width, height }) => layout(width, height));
```

The default `Window` capability only affects the calling window. Find, PDF, screenshots and cookies use the separately gated `Web` module. [JS runtime API](/en/reference/js-api)
