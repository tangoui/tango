# Packaging, signing & updates

macOS packaging, signing and notarization have been used on hardware. Windows installation remains unverified on hardware. Linux deb and AppImage have run on Debian 12 arm64 under Xvfb. For iOS and Android signing, see [Mobile](/en/guide/mobile#builds-and-signing).

## Installers with tan package

```bash
tan package
tan package -platform darwin/universal
tan package -platform windows/amd64,linux/amd64
tan package -platform windows/amd64 -format msix
```

| Platform | Output | Tools |
|---|---|---|
| macOS | DMG containing the app and an Applications link | `hdiutil` on macOS |
| Windows | NSIS installer, per-user by default | `makensis` |
| Windows with `-format msix` | MSIX package, requires `windows.publisher` | `makeappx`. If unavailable, tan writes the layout only |
| Linux | tar.gz and AppImage by default | `appimagetool`. If unavailable, tan writes the AppDir only |

Linux also accepts a comma-separated subset of `tar,appimage,deb,rpm,flatpak`. Tan writes deb and rpm itself. A Flatpak bundle requires `flatpak-builder`. Desktop artifact names usually start with `<Exe>-<version>-<os>-<arch>`. A restricted backend profile adds its name. The packaging step includes supported URL schemes, file associations, icons and copyright metadata from `tan.json`. [Linux packaging](https://github.com/tangoui/tango/blob/main/docs/linux-packaging.md)

### Desktop helpers

`desktopHelpers` runs the configured build commands for each desktop target. macOS puts the helpers in `.app/Contents/Helpers/`. Windows NSIS and MSIX put them in `cli/`. The Linux tarball puts them in `cli/`, and its install script copies them to `$PREFIX/lib/<Exe>/`. AppImage uses `usr/bin/cli/`, deb and rpm use `/usr/lib/<Exe>/`, and Flatpak uses `/app/lib/<Exe>/`. [Configuration fields](/en/reference/tan-json#desktop-builds)

## macOS signing and notarization

`tan build` signs the app ad hoc. For distribution, use a Developer ID signature and notarize it.

```bash
export TANGO_SIGN_IDENTITY="Developer ID Application: Example Ltd (TEAMID)"
export TANGO_NOTARY_PROFILE=notary
tan sign -notarize build/darwin-arm64/Hello.app
tan sign -notarize build/Hello-1.0.0-darwin-arm64.dmg
```

| Option | Purpose |
|---|---|
| `-identity` / `TANGO_SIGN_IDENTITY` | codesign identity |
| `-entitlements` / `macos.entitlements` | Entitlements plist |
| `-notarize` | Submit for notarization and staple the ticket |
| `-profile` / `TANGO_NOTARY_PROFILE` | notarytool keychain profile. Alternatively set `TANGO_APPLE_ID`, `TANGO_APPLE_TEAM_ID` and `TANGO_APPLE_PASSWORD` |

System notifications need a running `.app` with a bundle ID on macOS.

## Windows Authenticode

```bash
export TANGO_WINDOWS_CERT_PASSWORD=...
tan sign build/windows-amd64/Hello.exe
tan sign build/Hello-1.0.0-windows-amd64-setup.exe
```

Use a PKCS#12 certificate. Set `windows.certificate` and, if needed, `windows.timestampURL` and `windows.publisher` in `tan.json`. Supply the password by environment or flag. Tan uses `signtool` on Windows and `osslsigncode` on other hosts. When a certificate is configured, `tan package` signs the executable before making the package. [Windows signing and packaging](https://github.com/tangoui/tango/blob/main/docs/windows.md)

## Automatic updates

`service/updater` verifies signed manifests, rejects older versions, downloads and verifies a binary patch or full package, and installs after exit. An update that has not been confirmed can roll back. Call `Confirm()` on startup, then use `Check(ctx)`, `Download(ctx, rel, "")` and `Apply(p)`, handling their returned errors. Quit the app promptly after `Apply` succeeds so the helper can install the update. Set the updater's `ManifestURL`, `Version`, `AppID` and trusted `PublicKey`. On macOS it replaces the app bundle. Windows can replace the executable or use NSIS. Linux can replace an AppImage or an installation directory.

```bash
tan keygen -o update
tan package
tan release-manifest -key update.key -prev ../release-1.3.0 -url https://example.com/notes/
tan sign-update -key update.key latest.json
```

Publish the contents of `release/` at the updater's manifest URL. Use `-windows exe|nsis` and `-linux appimage|dir` to choose update mechanisms and `-notes` to add release notes. The private key can come from `TANGO_UPDATE_KEY_FILE` or base64 `TANGO_UPDATE_KEY`. To rotate keys, ship a client that also trusts the new public key through `Updater.PublicKeys` before signing releases with the new private key. [Updater API](/reference/api/service-updater) · [CLI flags](/en/reference/cli#all-flags)

## Releasing TanGo

[Project versioning and release workflow](https://github.com/tangoui/tango/blob/main/docs/release.md)
