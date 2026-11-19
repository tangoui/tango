# Menus & tray

Use `sys/menu` for native menus and `sys/tray` for tray icons. A single `*menu.Menu` can appear in the app menu bar, a window, a context menu or a tray icon. Click callbacks run on the main thread. Desktop implementations are covered on macOS hardware and by Windows and Linux CI. Mobile returns `ErrUnsupported`.

## App menu

```go
m := menu.New()
m.Role(menu.AppMenu)
file := m.Submenu("File")
file.Add("New", newDoc).Accelerator = "CmdOrCtrl+N"
file.Add("Open…", openDoc).Accelerator = "CmdOrCtrl+O"
file.Separator()

var wrap *menu.Item
wrap = file.Check("Word wrap", true, func() { setWrap(wrap.IsChecked()) })

view := m.Submenu("View")
view.Radio("size", "Small", false, func() { setSize(12) })
view.Radio("size", "Large", true, func() { setSize(16) })
m.Role(menu.EditMenu)
m.Role(menu.WindowMenu)
err := menu.SetAppMenu(app, m)
```

Use standard roles such as `Quit`, `About`, `Copy`, `Paste`, `Cut`, `SelectAll`, `Undo`, `Redo`, `Minimize` and `Close` for localized labels and native actions. `CmdOrCtrl` resolves to Command on macOS and Ctrl elsewhere. `menu.SetSurfaceMenu(app, s, m)` sets a window menu on Windows and Linux. macOS normally uses the shared menu bar. [Internationalization](/en/guide/i18n)

After installing a menu, use setters such as `SetLabel` and `SetDisabled` from any goroutine, then call `Update()` on the menu or a submenu to publish changes. Toggling a check or radio item refreshes it before calling its callback.

## Context menus

Register named menus with `menu.RegisterContext(app, "row", rowMenu)` before creating windows, or use `RegisterContextFunc` for a dynamic menu. Mark the element with `data-tango-menu="row"` and optionally pass `data-tango-menu-data` as `menu.Context.Data`. Go can also call `menu.Popup(s, x, y, m)` directly. For a page-defined native menu, the default JS `Menu` capability allows editing roles.

```js
import { Menu, Window } from "@tangoui/runtime";
Menu.bindContextMenu(document.body, () => [
  { label: "Copy", role: "copy" },
  { type: "separator" },
  { label: "Always on top", type: "checkbox", onClick: (on) => Window.setAlwaysOnTop(on) },
]);
```

## Tray icons

```go
t, err := tray.New(app, tray.Options{
    Icon: iconPNG, Template: true, Tooltip: "Notes",
    MenuOn: tray.MenuOnSecondary,
})
m := menu.New()
m.Add("Show window", showMain)
m.Separator()
m.Role(menu.Quit)
t.SetMenu(m)
t.OnClick(showMain)
```

Use a 32×32 or 44×44 PNG at 2×. On macOS, `Template` adapts a monochrome icon to the menu bar. The default menu trigger is a primary click on macOS and a secondary click on Windows. `MenuOnPrimary` and `MenuOnSecondary` can set the trigger. Without a menu, macOS and Windows dispatch click callbacks. Linux AppIndicator opens the menu on click and does not distinguish buttons or supply icon bounds. Update or remove the icon with `SetIcon`, `SetTooltip`, `SetTitle` (macOS), `Bounds` and `Remove`.

Set `QuitOnLastClose` to a pointer to `false` for a tray app that stays running after the last window closes. On macOS, `appctl.SetDockVisible(app, false)` can hide its Dock icon. See `examples/showcase` for an app with a working tray menu.

[Menu API](/guide/system/menu) · [Tray API](/guide/system/tray) · [Shortcuts](/guide/system/hotkey)
