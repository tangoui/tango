# 打包、签名与自动更新

本页讲从 `tan build` 的产物到可以分发给用户的安装包：安装包格式、macOS 签名与公证、Windows Authenticode 签名、自动更新。移动端的签名见 [移动端](/guide/mobile#构建与签名)。

::: warning 验证状态
macOS 的打包、签名与公证流程已在本机使用；Windows（NSIS、MSIX、Authenticode）和 Linux（tar.gz、AppImage）的流程由单元测试覆盖（用假的外部工具），**尚未在真机上验证安装运行**。CI 里的发布流程目前也不签名。
:::

## 安装包：tan package

```bash
tan package                                   # 本机平台
tan package -platform darwin/universal
tan package -platform windows/amd64,linux/amd64
tan package -platform windows/amd64 -format msix
```

`tan package` 先执行 `tan build`，再按平台生成：

| 平台 | 默认产物 | 需要的工具 |
|---|---|---|
| macOS | `.dmg`（含应用和指向「应用程序」的链接） | `hdiutil`（只能在 macOS 上做） |
| Windows | NSIS 安装器：按用户安装到 `%LOCALAPPDATA%\Programs`，开始菜单快捷方式，卸载项 | `makensis` |
| Windows（`-format msix`） | MSIX 包，需要 `windows.publisher` | `makeappx` 或等价工具 |
| Linux | `.tar.gz`（可执行文件、图标、`.desktop`、按用户安装的脚本）与 AppImage | `appimagetool`（不在 PATH 时只生成 `.AppDir` 并提示） |

文件名形如 `<Exe>-<version>-<os>-<arch>.<ext>`。`tan.json` 的 `urlSchemes`、`fileAssociations`、`icon`、`copyright` 会写进各平台的元数据（Info.plist、安装器注册表项、`.desktop` / MIME 包、AppxManifest）。

## macOS：签名与公证

`tan build` 给 `.app` 做 ad-hoc 签名，本机能运行；分发给别人需要 Developer ID 签名和公证：

```bash
export TANGO_SIGN_IDENTITY="Developer ID Application: Example Ltd (TEAMID)"
export TANGO_NOTARY_PROFILE=notary          # xcrun notarytool store-credentials 存的 profile
tan sign -notarize build/darwin-arm64/Hello.app
tan sign -notarize build/Hello-1.0.0-darwin-arm64.dmg
```

| 参数 / 环境变量 | 说明 |
|---|---|
| `-identity` / `TANGO_SIGN_IDENTITY` | codesign 身份 |
| `-entitlements` / `macos.entitlements` | entitlements plist |
| `-notarize` | 提交公证并装订票据 |
| `-profile` / `TANGO_NOTARY_PROFILE` | notarytool 钥匙串 profile；或者用 `TANGO_APPLE_ID`、`TANGO_APPLE_TEAM_ID`、`TANGO_APPLE_PASSWORD`（App 专用密码） |

通知（`sys/notify`）在 macOS 上要求应用从带 Bundle ID 的 `.app` 运行，裸二进制会得到明确的错误。

## Windows：Authenticode

```bash
export TANGO_WINDOWS_CERT_PASSWORD=...      # 密码只来自环境变量或参数，不写进 tan.json
tan sign build/windows-amd64/Hello.exe
tan sign build/Hello-1.0.0-windows-amd64-setup.exe
```

```json
{
  "windows": {
    "certificate": "windows/codesign.pfx",
    "timestampURL": "http://timestamp.digicert.com",
    "publisher": "CN=Example Ltd, O=Example Ltd, C=US"
  }
}
```

在 Windows 上用 `signtool`（Windows SDK），其他系统上用 `osslsigncode`；证书是 PKCS#12（`.pfx`）。配置了证书时 `tan package` 会先签可执行文件再打包。细节见仓库的 [docs/windows.md](https://github.com/tangoui/tango/blob/main/docs/windows.md)。

## 自动更新

`service/updater` 给桌面应用做签名的自动更新：取签名的更新清单 → 拒绝比当前（或曾见过的）更旧的版本 → 下载二进制补丁或完整包并校验 → 应用退出后安装 → 新版本没有确认启动成功就回滚。

```go
u := &updater.Updater{
    ManifestURL: "https://example.com/notes/latest.json",
    PublicKey:   notesPubKey,          // tan keygen 生成的 .pub
    Version:     version,              // 当前版本（SemVer）
    AppID:       "com.example.notes",  // 清单里的 app 必须一致
    OnEvent:     func(e updater.Event) { showProgress(e) },
}
u.Confirm()                            // 每次启动尽早调用：确认新版本启动成功
rel, err := u.Check(ctx)               // nil, nil：已是最新
if rel != nil {
    p, err := u.Download(ctx, rel, "") // 先试补丁，失败回落到完整包
    if err == nil {
        u.Apply(p)                      // 然后退出，新版本启动
    }
}
```

安装方式按平台不同：macOS 替换 `.app` 包；Windows 替换可执行文件或运行安装器；Linux 替换 AppImage 或安装目录。

### 签名密钥与更新清单

```bash
tan keygen -o update           # 生成 update.key（私钥，保密）和 update.pub（公钥，编进应用）
tan package                    # 先做出本次发布的安装包 / 应用
tan release-manifest -key update.key -prev ../release-1.3.0 -url https://example.com/notes/
                               # 在 release/ 下生成更新文件、相对上一版的二进制补丁和签名的 latest.json
tan sign-update -key update.key latest.json   # 只重新签名一个手改过的清单
```

把 `release/` 的内容上传到 `Updater.ManifestURL` 所在位置即可。`-windows exe|nsis` 选择 Windows 更新方式（替换可执行文件或运行安装器），`-linux appimage|dir` 选择 Linux 的方式，`-notes` 写更新说明。私钥也可以通过 `TANGO_UPDATE_KEY_FILE` 或 `TANGO_UPDATE_KEY`（base64）提供，适合 CI。`Updater.PublicKeys` 可以同时信任多个公钥，用来轮换密钥：先发布带新公钥的版本，再用新私钥签名。

::: info
清单格式（平台条目、补丁、签名信封）见 [service/updater 的 API 参考](/reference/api/service-updater)；完整参数见 [命令行 tan](/reference/cli#完整参数)。自动更新属于实验性功能，Windows、Linux 的安装流程尚未实机验证。
:::

## 发布 Tango 自己

Tango 框架本身的版本号规则和发布流程见仓库的 [docs/release.md](https://github.com/tangoui/tango/blob/main/docs/release.md)。
