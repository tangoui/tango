# Desktop capabilities

Go imports use the prefix `github.com/tangoui/tango`. Run `go doc` on the listed types and functions from the application directory. The snippets below are function bodies for an existing app and window unless a full function is shown. Handle errors from native operations, including when the current platform lacks a capability.

## Windows and lifecycle

Create a window with `app.NewSurface(tango.SurfaceOptions{Title: "Notes", Width: 800, Height: 600, Resizable: true})` in `app.OnReady`. For a native UI window, use `uihost.Open` on that same thread. Use `s.IsVisible()`, `s.Show()`, and `s.Hide()` for visibility. Read `s.Bounds()` for geometry, and register `s.OnMoved(func(x, y int) {})` or `s.OnResized(func(width, height int) {})` to observe changes.

```go
func rememberWindow(s *tango.Surface) error {
	return windowstate.Track(s, "main", nil)
}
```

Import `sys/windowstate`. Use `Hidden: true` in `SurfaceOptions`, attach the tracker, then show the window. A nil store uses the app's default configuration path. Pass `windowstate.NewFileStore(path)` to choose another location.

Use `app.OnEventKind(tango.EventReopen, func(tango.AppEvent) { s.Show(); s.Focus() })` for macOS Dock reopen. To keep a tray app running when every window closes, pass a pointer to `false` in `tango.Options.QuitOnLastClose`. Give users an explicit Quit action wired to `app.Quit()`.

## Menus and shortcuts

Import `sys/menu`. On macOS the first submenu occupies the application-menu slot. Include an Edit menu for standard WebView editing shortcuts.

```go
func installMenu(app *tango.App, onNew func()) error {
	m := menu.New()
	m.Submenu("Notes").Role(menu.Quit)
	m.Submenu("File").Add("New Note", onNew).Accelerator = "CmdOrCtrl+N"
	m.Submenu("Edit").Role(menu.EditMenu)
	return menu.SetAppMenu(app, m)
}
```

Use `menu.SetSurfaceMenu(s, m)` for window-specific menus where supported. After an installed item's `SetLabel` or `SetChecked`, call `Update()` on the installed root menu and handle the error.

## Tray

Import `sys/tray` and `sys/menu`. Embed or load PNG bytes before creating the tray. On macOS a monochrome icon with transparency and `Template: true` adapts to the menu bar appearance.

```go
func makeTray(app *tango.App, s *tango.Surface, pngData []byte) error {
	t, err := tray.New(app, tray.Options{Icon: pngData, Tooltip: "Notes", Template: true})
	if err != nil {
		return err
	}
	m := menu.New()
	m.Add("Show Notes", func() { s.Show(); s.Focus() })
	m.Add("Quit", app.Quit)
	return t.SetMenu(m)
}
```

## Notifications

Import `sys/notify`. Start the permission request from a goroutine after startup. The request may wait for the user's response.

```go
func notifyFinished(app *tango.App) (bool, error) {
	ok, err := notify.RequestPermission(app)
	if err != nil || !ok {
		return false, err
	}
	_, err = notify.Send(app, notify.Message{Title: "Export finished", Body: "Your file is ready."})
	return err == nil, err
}
```

Test macOS notification delivery from an installed `.app`. [Troubleshooting](pitfalls.md)

## File and folder dialogs

Import `sys/dialog` and the standard `errors` package. Pass the caller's window as the parent for a window-scoped dialog.

```go
func chooseFolder(app *tango.App, parent *tango.Surface) ([]string, error) {
	selected, err := dialog.Open(app, dialog.FileOptions{Parent: parent, PickFolders: true})
	if errors.Is(err, dialog.ErrCanceled) {
		return nil, nil
	}
	return selected, err
}
```

Use `dialog.Save` with `dialog.FileOptions` for a save dialog.

## Storage and file watching

Import `service/paths`. Resolve the app's private data directory with `paths.Data(appID)` and create it before writing. Use `paths.Config(appID)` for settings. Tango has no file-watch API. A file-watching dependency such as `github.com/fsnotify/fsnotify` can watch the directories the user selects.

```go
func prepareDataDir(appID string) (string, error) {
	dir, err := paths.Data(appID)
	if err != nil {
		return "", err
	}
	return dir, os.MkdirAll(dir, 0700)
}
```

## App visibility, clipboard, and single instance

- Import `sys/appctl`. In a running app use `appctl.Hide(app)`, `appctl.Show(app)`, or `appctl.SetDockVisible(app, false)` where supported. To hide one window, use `s.Hide()`.
- Import `sys/clipboard`. Read with `text, err := clipboard.ReadText(app)` and write with `err := clipboard.WriteText(app, text)`.
- Import `sys/single`. Call `primary, err := single.Claim(app, single.Options{})` before `app.Run()`. Exit if `primary` is false. Check and report `err`, including when the app may continue as primary without a lock. Register `single.OnSecondInstance(app, func(args []string, cwd string) { /* deliver to the UI */ })` to handle arguments forwarded by later launches.

## Events and caller windows

Import `ipc`. Send a Go event with `r.Emit(surface, "notes:changed", payload)`. For several WebView windows, loop over `app.Surfaces()` and emit to each intended recipient. Drop each closed window with `r.Drop(surface)`.

```go
func callerSurface(ctx context.Context) *tango.Surface {
	conn, ok := ipc.ConnFrom(ctx)
	if !ok {
		return nil
	}
	s, _ := conn.(*tango.Surface)
	return s
}
```

Declare `context.Context` as the first parameter of the bound Go method and check the returned surface before using it. [WebView wiring](webview-app.md)

See complete startup and callback wiring in the [native tray timer](../examples/native-tray-timer/main.go) and [WebView notes](../examples/webview-notes/main.go) examples.
