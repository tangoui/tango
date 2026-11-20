# Mobile (experimental)

On iOS and Android, open a system WebView page or a Go-drawn page with `uihost.Open`. TanGo compiles the Go app as a library and loads it through the native host. iOS has run in Apple Silicon simulators and on one iPhone 12 Pro. Android has been tested in an API 36 emulator. [Platform support](/en/guide/platforms)

## Runtime model

The entry point on iOS is `UIApplicationMain`; on Android it is the host Activity. The host calls Go's `main` in a goroutine after startup. `App.Run` blocks there until exit. UI callbacks run on the native main thread.

A Surface fills a page in the host's stack. Closing the top Surface reveals the previous one. On Android, Back first navigates WebView history when available. A Go-drawn page can consume Back to dismiss a popup or handle its own navigation. Otherwise the host handles Back. Save state on `Background`. When the system terminates the app, the host posts `Terminating` and waits up to two seconds for `main` to finish. Handle `App.OnWebViewRestored` to restore page state after the system reclaims the WebView process.

Android cannot nest a user wait on the main thread. Call dialogs, share sheets, permission requests and location operations from a goroutine. [Threading](/en/guide/concepts#threading)

## Capabilities

| Capability | iOS | Android |
|---|---|---|
| Text clipboard | UIPasteboard, system may prompt | ClipboardManager, foreground reads on Android 10+ |
| Message dialog | UIAlertController | AlertDialog, Back or outside click cancels |
| Open file | Document picker, imports a sandbox copy | Storage Access Framework, copies to app cache |
| Save dialog | `ErrUnsupported`, use the sandbox and share | `ErrUnsupported` |
| Menus, tray, window geometry | Unavailable | Unavailable |

Mobile packages cover permissions, sharing, haptics, safe areas, orientation, biometrics, secure storage, media, location and push notifications. Custom schemes, Universal Links and App Links arrive through `EventOpenURL` at launch or while the app is running. [System capabilities](/en/guide/system)

Use `viewport-fit=cover` and `safearea.SyncPage(app)` to expose safe-area insets as CSS variables. For example, `padding-top: var(--tango-safe-top, env(safe-area-inset-top, 0px))`.

## Permissions

List capabilities in `tan.json`. The build generates iOS usage-description keys and Android manifest permissions where applicable. For distribution, supply a specific explanation in `ios.usageDescriptions`.

```json
{
  "permissions": ["camera", "location", "notifications"],
  "ios": { "usageDescriptions": { "camera": "Scan QR codes with the camera." } }
}
```

| Name | Used for |
|---|---|
| `camera` | `media.TakePhoto` and camera permission |
| `photos` | Photo-library permission. The system picker used by `media.Pick` can work without it |
| `location` | Foreground location |
| `location-always` | Background location |
| `microphone` | Microphone access |
| `biometric` | Biometric authentication |
| `notifications` | Notifications and remote push |
| `background` | Background tasks |
| `widget` | Home-screen widgets |

Supply required signing and privacy files for iOS distribution. For Universal Links, App Links, or integrating FCM, see the permission mapping and host setup. [Mobile permissions](https://github.com/tangoui/tango/blob/main/docs/mobile-permissions.md)

## Builds and signing

### iOS

```bash
tan run -platform ios-simulator             # install and run, with logs
tan build -platform ios-simulator           # ad-hoc signed simulator .app
tan build -platform ios                     # signed device .app and .ipa
tan build -platform ios-simulator -xcode    # also generate an Xcode project
```

Tan uses Xcode's clang to link the Go `c-archive` and the native host, writes Info.plist, and prepares icons, the privacy manifest and signing. Device builds require a signing identity (`TANGO_IOS_SIGN_IDENTITY` overrides `ios.signIdentity`) and a provisioning profile covering the app ID. `ios.entitlements` defaults to the profile's grants. `ios.minVersion` defaults to 17.0, and `ios.build` sets `CFBundleVersion`. iOS builds target arm64. iPad multi-window scenes and 3xx asset responses are unsupported.

### Android

Set `JAVA_HOME` to a JDK 17 or later and `ANDROID_HOME` to an SDK with an NDK, or set `ANDROID_NDK_HOME` separately.

```bash
tan run -platform android
tan build -platform android -debug
tan build -platform android/arm64,android/amd64
```

The host links a `c-shared` Go library per ABI with 16 KB segment alignment. Debug builds produce APKs. Release builds produce signed APKs and AABs. Set both `android.keystore` and `android.keyAlias` for release signing. Set `TANGO_ANDROID_KEYSTORE_PASSWORD` and optionally `TANGO_ANDROID_KEY_PASSWORD`, which defaults to the store password. The default SDK levels and version-code rules are in [tan.json](/en/reference/tan-json#ios-android).

If no device is online, `tan run -platform android` starts the first AVD, or the one selected with `-avd`. Logcat shows Go output under `Tango` and page console output under `chromium`.

## Development and CI

`tan dev -platform ios-simulator` and `tan dev -platform android` run a development build on a running simulator, emulator or connected Android device. Use `-device` to select it. The `ios` and `android` commands in `scripts/smoke.sh` build `examples/mobile-hello` and wait for the page to message Go. [CI & smoke tests](/en/guide/ci)

[Mobile host implementation](https://github.com/tangoui/tango/blob/main/docs/mobile.md)
