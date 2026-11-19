# System capabilities

Most system capabilities live in `github.com/tangoui/tango/sys/<package>`. APIs generally take a `*tango.App` or operate on a `*tango.Surface`. Call them from any goroutine. Native callbacks run on the main thread. Unsupported operations return `ErrUnsupported`, and operations that need a running app can return `tango.ErrNotRunning` after it exits. On Android, use a separate goroutine for operations that wait for the user, such as dialogs, sharing or permission requests. [Threading](/en/guide/concepts#threading)

Desktop backends are verified on macOS hardware and covered by CI smoke tests on Windows and Linux. Mobile capabilities are experimental. iOS has run in simulators and on one iPhone 12 Pro, and Android has run in emulators. [Platform support](/en/guide/platforms)

## Desktop

| Package | Purpose | JS module |
|---|---|---|
| [clipboard](/guide/system/clipboard) | Text, PNG images and file lists | `Clipboard` |
| [dialog](/guide/system/dialog) | Message, open and save dialogs | `Dialog` |
| [notify](/guide/system/notify) | System notifications and click handling | `Notify` |
| [hotkey](/guide/system/hotkey) | Global and in-page shortcuts | `Hotkey` |
| [screen](/guide/system/screen) | Displays, primary display and display at a point | `Screens` |
| [theme](/guide/system/theme) | Light/dark mode, accent color and page synchronization | `Theme` |
| [shell](/guide/system/shell) | Open URLs and files or reveal them in the file manager | `Shell` |
| [appctl](/guide/system/appctl) | Visibility, badge, progress, attention and Dock icon | `App` |
| [autostart](/guide/system/autostart) | Start at login | |
| [env](/guide/system/env) | OS, architecture, build and WebView version | `App.info` |
| [recent](/guide/system/recent) | Recently opened documents | |
| [single](/guide/system/single) | Single instance and argument forwarding | `App.onSecondInstance` |
| [windowstate](/guide/system/windowstate) | Restore window geometry | |
| [power](/guide/system/power) | Power status and idle-sleep prevention, with sleep/wake callbacks on desktop | |
| [menu](/guide/system/menu) | App, window and context menus | `Menu` |
| [tray](/guide/system/tray) | Tray icons | |

[JS runtime capabilities and permissions](/en/reference/js-api#system-capability-modules)

## Mobile

| Package | Purpose | `tan.json` permission |
|---|---|---|
| [permission](/guide/system/permission) | Check and request runtime permissions | As requested |
| [share](/guide/system/share) | Native share sheet, also available on macOS | |
| [haptics](/guide/system/haptics) | Haptic feedback | |
| [safearea](/guide/system/safearea) | Safe-area insets and CSS variables | |
| [orientation](/guide/system/orientation) | Read and lock the screen orientation | |
| [biometric](/guide/system/biometric) | Face ID, Touch ID or BiometricPrompt. Also supports macOS | `biometric` |
| [securestore](/guide/system/securestore) | Small secrets in Keychain or Keystore. Also supports macOS | |
| [media](/guide/system/media) | Pick photos or videos, or take a photo | `camera` for capture |
| [location](/guide/system/location) | One-shot and continuous location | `location` |
| [push](/guide/system/push) | APNs or FCM push notifications | `notifications` |

[Permission declarations](/en/guide/mobile#permissions)

Other capabilities include mobile background tasks (`sys/background`), device settings (`sys/device`), widgets (`sys/widget`) and in-app purchases (`sys/purchase`), as well as network status (`sys/network`), tray flyouts (`sys/flyout`), launch shortcuts (`sys/jumplist`), printing (`sys/print`) and pseudo-terminals (`sys/pty`). [Go API reference](/en/reference/api)

## Services

The `service` package binds Go values to application startup and shutdown, with optional IPC bindings. `service/paths` finds standard app directories, `service/kv` provides a concurrent key-value store, `service/log` provides rotating logs, and `service/fileserver` exposes files under a resource prefix. `service/updater` implements signed desktop updates. [Packaging, signing & updates](/en/guide/packaging#automatic-updates)
