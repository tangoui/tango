# Map familiar desktop APIs to Tango

| If you need | Use this Tango API |
|---|---|
| An application menu | `sys/menu.New()`, then `m.Submenu("File").Add("New", fn).Accelerator = "CmdOrCtrl+N"`, then `menu.SetAppMenu(app, m)` |
| A tray icon | `sys/tray.New(app, tray.Options{Icon: pngBytes})`, then `Tray.SetMenu` with a `*menu.Menu` |
| A notification | `sys/notify.RequestPermission(app)`, then `notify.Send(app, notify.Message{Title: "Done"})` |
| An app data path | `service/paths.Data(appID)` |
| A directory picker | `sys/dialog.Open(app, dialog.FileOptions{PickFolders: true})`, with `errors.Is(err, dialog.ErrCanceled)` for cancellation |
| A window's location and size | `Surface.Bounds()`, with `Surface.OnMoved` and `Surface.OnResized` for changes |
| A persisted window location | `sys/windowstate.Track(surface, "main", nil)` |
| A Go event for a page | `ipc.Router.Emit(surface, name, payload)` for each target window |
| The caller of a bound method | `ipc.ConnFrom(ctx)`, then check for a `*tango.Surface` |
| Calling Go and subscribing in the page | `call<T>("Service.Method", ...args)` and `on(name, fn)` from `@tangoui/runtime` |

Full import paths and examples are in [Capabilities](capabilities.md).
