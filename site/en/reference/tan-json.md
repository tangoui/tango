# tan.json

`tan.json` lives at the project root. File paths usually resolve from that directory. `frontend.dist` resolves from `frontend.dir`. `tan init` adds the optional `$schema` field for editor completion and validation. Tan rejects unknown fields, so a configuration that uses newly added fields needs a tan version that knows them.

[JSON Schema](https://tangoui.github.io/tango/schema/tan.schema.json)

## Example

```json
{
  "name": "Notes",
  "id": "com.example.notes",
  "version": "1.2.0",
  "icon": "icon.png",
  "copyright": "© 2026 Example Ltd",
  "desktopBuildTags": ["production"],
  "frontend": {
    "dir": "frontend",
    "dist": "dist",
    "dev": "npm run dev",
    "build": "npm run build"
  },
  "urlSchemes": ["notes"],
  "fileAssociations": [{ "ext": "note", "name": "Notes document" }],
  "macos": { "minVersion": "11.0", "entitlements": "macos/notes.entitlements" },
  "windows": {
    "certificate": "windows/codesign.pfx",
    "publisher": "CN=Example Ltd, O=Example Ltd, C=US"
  },
  "permissions": ["camera", "location"],
  "ios": {
    "minVersion": "17.0",
    "signIdentity": "Apple Development: Jane Doe (TEAMID)",
    "provisioningProfile": "mobile/notes.mobileprovision",
    "usageDescriptions": { "camera": "Scan QR codes with the camera." }
  },
  "android": { "minSdk": 24, "keystore": "mobile/release.jks", "keyAlias": "upload" }
}
```

## General fields

| Field | Meaning |
|---|---|
| `$schema` | Optional schema URL for editors |
| `name` | Required display name. Also determines the executable name |
| `id` | Required reverse-DNS identifier for the bundle ID, Android application ID and MSIX identity |
| `version` | Required. One to three dot-separated numbers with an optional `-` or `+` suffix. iOS strips the suffix from `CFBundleShortVersionString` |
| `icon` | Square, full-bleed master PNG, ideally 1024×1024. Tan derives the desktop shapes |
| `icons` | Per-platform icon overrides |
| `copyright` | Copyright metadata for bundles and installers |
| `localizations` | Apple bundle `CFBundleLocalizations`. Defaults to `en`, `zh-Hans`, `zh-Hant` and `ja` |
| `urlSchemes` | Lowercase custom schemes, such as `notes` for `notes://…`. Declare the same schemes in `tango.Options.URLSchemes` |
| `appLinks` | Universal Link and Android App Link domains, such as `example.com` or `*.example.com`. Use `tan applinks` to generate website verification files |
| `appLinkPaths` | Allowed path prefixes, such as `/open/`. Defaults to every path |
| `fileAssociations` | Document types with `ext` (without a dot), `name`, `role` (`editor` by default or `viewer`) and `mime` (defaults to `application/x-<exe>-<ext>`) |
| `permissions` | Mobile capabilities. See [Mobile](/en/guide/mobile#permissions) |

## Desktop builds

| Field | Meaning |
|---|---|
| `features` | Desktop backend profile for `tan build` and `tan package`. Omit to build all capabilities. macOS supports `["webview"]`, `["canvas"]` and `["webview", "canvas"]`. The `-features` flag overrides it and `-features all` selects all capabilities |
| `desktopBuildTags` | Extra Go tags for desktop builds from `tan build` and `tan package`, merged with tags from `GOFLAGS`. Each entry is a single Go tag. The names `tango_dev`, `tango_release` and tags prefixed with `tango_backend_` are reserved |
| `cgo` | Desktop builds and desktop development builds default to `CGO_ENABLED=0`. Set to `true` to link C libraries. For cross-compilation, set `CC` to the target toolchain. Mobile has its own cgo toolchain |
| `desktopHelpers` | Executables shipped with the desktop app. Each entry has a `name` without an extension and a shell `build` command run from the project root. Names start with a letter or digit, may also contain hyphens or underscores, and are unique without regard to case |

The helper command receives `TAN_TARGET_OS` and `TAN_TARGET_ARCH` (`universal` for a universal macOS build). It must write a nonempty regular file to `TAN_HELPER_OUTPUT`. The build puts helpers in `.app/Contents/Helpers/` on macOS and `cli/` on Windows and Linux, adding `.exe` on Windows. Installer paths depend on the format. [Desktop helpers](/en/guide/packaging#desktop-helpers)

## frontend

| Field | Meaning |
|---|---|
| `dir` | Front-end root and working directory for front-end commands |
| `dist` | Build output relative to `dir`. Use `.` to serve sources directly |
| `install` | Runs if `dir` contains a `package.json` without `node_modules`. Defaults to `npm install` |
| `dev` | Dev-server command. `tan dev` reads its first printed HTTP URL |
| `devURL` | Explicit dev-server URL when it cannot be read from the output |
| `build` | Production build command |

Omit `frontend` for a fully Go-drawn app.

## macos / windows

| Field | Meaning |
|---|---|
| `macos.minVersion` | `LSMinimumSystemVersion`, default 11.0 |
| `macos.entitlements` | Entitlements plist for `tan sign` |
| `windows.certificate` | Authenticode certificate (`.pfx` or `.p12`). Set the password with `TANGO_WINDOWS_CERT_PASSWORD` or `-password` |
| `windows.timestampURL` | RFC 3161 timestamp server, default `http://timestamp.digicert.com` |
| `windows.publisher` | Certificate subject beginning with `CN=`. Required for MSIX and must match the certificate |

## ios / android

| Field | Meaning |
|---|---|
| `ios.minVersion` | Minimum iOS version and deployment target, default 17.0 |
| `ios.build` | `CFBundleVersion`, defaults to `version` without its suffix |
| `ios.signIdentity` | Device signing identity. `TANGO_IOS_SIGN_IDENTITY` takes precedence |
| `ios.provisioningProfile` | Provisioning profile required for device builds |
| `ios.entitlements` | Optional. Defaults to the entitlements granted by the profile |
| `ios.usageDescriptions` | Custom iOS prompt text by declared permission name |
| `android.minSdk` / `targetSdk` | Defaults to 24 / 36. The minimum cannot be below 24, and the target must be at least the minimum |
| `android.versionCode` | Defaults to `major*10000 + minor*100 + patch`, with minor and patch capped at 99 and a minimum result of 1 |
| `android.keystore` / `keyAlias` | Required together for release signing. Passwords come from `TANGO_ANDROID_KEYSTORE_PASSWORD` and `TANGO_ANDROID_KEY_PASSWORD`. The key password defaults to the keystore password |

## linux

| Field | Meaning |
|---|---|
| `linux.package` | deb and rpm package name. Defaults to the lowercase app name with words joined by hyphens |
| `linux.maintainer` | Debian Maintainer and RPM Packager, in `Name <email>` form |
| `linux.description` | Summary on the first line, with an optional long description below. Defaults to the app name |
| `linux.homepage` | Package homepage URL |
| `linux.license` | RPM License, default `Proprietary` |
| `linux.depends` / `linux.rpmRequires` | Additional deb and rpm dependencies, one dependency per entry |
| `linux.flatpak.runtimeVersion` | `org.gnome.Platform` branch, default `50` |
| `linux.flatpak.finishArgs` | Additional sandbox permissions, such as `--socket=pulseaudio` or `--filesystem=xdg-documents` |

## installer

| Field | Meaning |
|---|---|
| `installer.windows.scope` | NSIS installation scope. `user` (default) uses `%LOCALAPPDATA%\Programs` and HKCU. `machine` uses Program Files and HKLM and requires administrator privileges |
| `installer.windows.license` | License page text file (`.txt`, `.md` or no extension) or `.rtf` |
| `installer.windows.welcome` | Show the welcome page, default `true` |
| `installer.windows.sidebarImage` / `headerImage` | Optional 164×314 sidebar and 150×57 header images, in BMP or PNG |
| `installer.windows.runAfterInstall` | Offer to run the app on the finish page. On by default for per-user installs, off for machine-wide installs |
| `installer.windows.desktopShortcut` | `ask` (default), `always` or `never` |
| `installer.windows.languageDialog` | Ask for the installer language at startup. By default the installer follows the Windows display language |
| `installer.windows.include` | NSIS script extension for the `customInit`, `customInstall` and `customUnInstall` macros |
| `installer.windows.template` | Go `text/template` file that replaces the generated NSIS script |
| `installer.macos.dmg.background` | Finder window background in PNG, JPEG or TIFF. A sibling `@2x` PNG can supply a Retina background |
| `installer.macos.dmg.windowSize` / `iconSize` | Window width and height and icon size in points, default `[540, 380]` / `128` |
| `installer.macos.dmg.appPosition` / `applicationsPosition` | Icon centers in points from the window's top left, default `[140, 190]` / `[400, 190]` |
| `installer.macos.dmg.title` | Volume name, defaults to the app name |

## icons

All fields are optional. Tan derives missing variants from `icon`.

| Field | Meaning |
|---|---|
| `macos` | 1024×1024 PNG with the macOS icon shape, used as supplied |
| `windows` | Shaped PNG with a transparent background (at least 256 px) or an `.ico` |
| `linux.app` / `linux.symbolic` | Replacement app PNG and monochrome panel SVG |
| `ios.dark` / `ios.tinted` | Dark and tinted variants for iOS 18 and later |
| `android.foreground` / `background` / `monochrome` | Adaptive icon layers. The background is a PNG or `#RRGGBB`. The monochrome layer supports Android 13 themed icons |
| `notification` | Android notification status-bar icon, with a white shape on a transparent background |

Run `tan doctor` to check icon assets.
