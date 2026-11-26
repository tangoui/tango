# tan.json

`tan.json` 是项目配置文件，放在项目根目录。文件路径通常相对于项目目录。`frontend.dist` 相对于 `frontend.dir`。`tan init` 会生成一份；允许添加 `cloud` 等应用自定义的顶层字段，`tan` 会忽略它们，由应用自行读取。内置字段仍按原规则校验，`frontend`、`windows` 等内置对象内部仍拒绝未知字段。不要在配置中保存密码。

`tan init` 生成的文件第一行是 `"$schema": "https://tangoui.github.io/tango/schema/tan.schema.json"`，VS Code 等编辑器据此补全字段、显示说明并标出内置字段的类型错误和内置对象中的未知字段。已有项目手动加上这一行即可；tan 会忽略它。较旧的 `tan` 可能仍拒绝自定义顶层字段。

## 示例

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
    "usageDescriptions": { "camera": "扫描二维码时使用相机。" }
  },
  "android": { "minSdk": 24, "keystore": "mobile/release.jks", "keyAlias": "upload" }
}
```

## 通用字段

| 字段 | 说明 |
|---|---|
| `$schema` | 可选的 JSON Schema URL，供编辑器使用 |
| `name` | 必填。应用显示名和包名。生成可执行文件名时只保留 ASCII 字母、数字、连字符和下划线，至少需要一个 ASCII 字母或数字 |
| `id` | 必填。反向域名 ID（Bundle ID、applicationId、MSIX Identity），如 `com.example.notes` |
| `version` | 必填。1 至 3 段数字，可带 `-` 或 `+` 后缀。iOS 的 `CFBundleShortVersionString` 去掉后缀 |
| `icon` | 主图标：正方形、铺满画布的 PNG，最好 1024×1024；桌面的圆角形状由 tan 派生 |
| `icons` | 各平台图标覆盖，见下 |
| `copyright` | 写进 Info.plist、安装器等 |
| `localizations` | Apple 应用包的 `CFBundleLocalizations`，默认 `en`、`zh-Hans`、`zh-Hant`、`ja` |
| `urlSchemes` | 深链 scheme（`notes` 对应 `notes://...`）；应用代码里的 `tango.Options.URLSchemes` 也要声明 |
| `appLinks` | 通用链接 / Android App Links 的域名，如 `example.com` 或 `*.example.com`。用 `tan applinks` 生成网站端验证文件 |
| `appLinkPaths` | 限定链接的路径前缀，如 `/open/`。默认允许所有路径 |
| `fileAssociations` | 打开的文档类型：`ext`（不带点）、`name`、`role`（`editor` 默认 / `viewer`）、`mime`（默认 `application/x-<exe>-<ext>`） |
| `permissions` | 移动端用到的能力，见 [移动端](/guide/mobile#权限) |

## 桌面构建

| 字段 | 说明 |
|---|---|
| `features` | `tan build`、`tan package` 的桌面后端配置。省略时编译全部能力。可填 `["webview"]`、`["canvas"]` 或 `["webview", "canvas"]`，目前仅 macOS 支持裁剪。`-features` 可覆盖，`-features all` 恢复全部能力 |
| `desktopBuildTags` | 给 `tan build`、`tan package` 的桌面应用构建追加 Go 标签，和 `GOFLAGS` 中的标签合并。每项须为单个合法标签。保留名 `tango_dev`、`tango_release` 及 `tango_backend_` 前缀不可用 |
| `cgo` | 桌面应用及桌面开发构建默认设置 `CGO_ENABLED=0`。需要链接 C 库时设为 `true`，交叉编译时还需在 `CC` 中指定目标工具链。移动端使用自己的 cgo 工具链 |
| `desktopHelpers` | 随桌面应用分发的辅助程序。每项填 `name`（无扩展名，字母或数字开头，可含 `-`、`_`，忽略大小写时须唯一）和 `build`（从项目根目录运行的 shell 命令） |

每个辅助程序的命令可读 `TAN_TARGET_OS` 和 `TAN_TARGET_ARCH`（通用 macOS 构建为 `universal`），须在 `TAN_HELPER_OUTPUT` 写出非空普通文件。构建时 macOS 放入 `.app/Contents/Helpers/`，Windows 和 Linux 放入构建目录的 `cli/`，Windows 文件名自动加 `.exe`。安装包内的位置随格式变化。 [桌面辅助程序](/guide/packaging#桌面辅助程序)

## frontend

| 字段 | 说明 |
|---|---|
| `dir` | 前端根目录，命令在这里执行 |
| `dist` | 构建输出，相对 `dir`；源码直接作为静态文件时为 `.` |
| `install` | `dir` 有 `package.json` 但没有 `node_modules` 时执行，默认 `npm install` |
| `dev` | 开发服务器命令，`tan dev` 读取它打印的第一个 http 地址 |
| `devURL` | 地址读不出来时直接指定 |
| `parallelDevBuild` | 默认 `false`。设为 `true` 时，桌面 `tan dev` 自行运行 `frontend.dev` 并等待其打印 URL，同时执行首次 Go 构建 |
| `build` | 生产构建命令 |

依赖安装成功后才开始并行准备。应用会等到前端打印 URL 且 Go 构建成功后启动。启用前，请确认 `frontend.dev` 不会生成或更新首次 Go 构建依赖的文件，例如 `go:embed` 匹配的文件。指定 `-url` 或 `frontend.devURL`、直接提供静态文件、使用移动端开发模式或执行发布构建时，该字段无效。

纯自绘 UI 的应用省略整个 `frontend`。

## macos / windows

| 字段 | 说明 |
|---|---|
| `macos.minVersion` | `LSMinimumSystemVersion`，默认 11.0 |
| `macos.entitlements` | `tan sign` 使用的 entitlements |
| `windows.certificate` | Authenticode 证书（`.pfx` / `.p12`）；密码只来自 `TANGO_WINDOWS_CERT_PASSWORD` 或 `-password` |
| `windows.timestampURL` | RFC 3161 时间戳服务器，默认 `http://timestamp.digicert.com` |
| `windows.publisher` | 证书主题，`CN=` 开头；打 MSIX 时必填，必须与证书一致 |

## ios / android

| 字段 | 说明 |
|---|---|
| `ios.minVersion` | 最低系统与编译目标，默认 17.0 |
| `ios.build` | `CFBundleVersion`，默认等于去掉后缀的版本号 |
| `ios.signIdentity` | 真机签名身份（`TANGO_IOS_SIGN_IDENTITY` 优先） |
| `ios.provisioningProfile` | 真机必需的描述文件 |
| `ios.entitlements` | 可选，默认用描述文件授予的 |
| `ios.usageDescriptions` | 按权限名覆盖系统弹窗里的用途说明 |
| `android.minSdk` / `targetSdk` | 默认 24 / 36。`minSdk` 至少为 24，`targetSdk` 至少为 `minSdk` |
| `android.versionCode` | 默认由版本号推出：`major*10000 + minor*100 + patch`（`minor`、`patch` 各封顶 99，结果至少为 1） |
| `android.keystore` / `keyAlias` | release 签名，成对出现；密码来自 `TANGO_ANDROID_KEYSTORE_PASSWORD` 和 `TANGO_ANDROID_KEY_PASSWORD`，后者默认沿用前者 |

## linux

| 字段 | 说明 |
|---|---|
| `linux.package` | deb / rpm 的包名，默认由应用名转为小写并用 `-` 连接单词 |
| `linux.maintainer` | Debian 的 `Maintainer` 和 RPM 的 `Packager`，格式为 `Name <email>` |
| `linux.description` | 首行为摘要，后面为详细说明。默认用应用名 |
| `linux.homepage` | 软件包主页 URL |
| `linux.license` | RPM 许可证，默认 `Proprietary` |
| `linux.depends` / `linux.rpmRequires` | 分别追加 deb / rpm 依赖。每项写一条依赖 |
| `linux.flatpak.runtimeVersion` | `org.gnome.Platform` 分支，默认 `50` |
| `linux.flatpak.finishArgs` | 追加 Flatpak 沙盒权限，如 `--socket=pulseaudio` 或 `--filesystem=xdg-documents` |

## installer

| 字段 | 说明 |
|---|---|
| `installer.windows.scope` | NSIS 安装范围。`user`（默认）用 `%LOCALAPPDATA%\\Programs` 和 HKCU。`machine` 用 Program Files 和 HKLM，需要管理员权限 |
| `installer.windows.license` | 许可页的纯文本文件（支持 `.txt`、`.md` 或无后缀）或 `.rtf` |
| `installer.windows.welcome` | 显示欢迎页，默认 `true` |
| `installer.windows.sidebarImage` / `headerImage` | 可选的 164×314 侧栏图及 150×57 页眉图，支持 BMP 或 PNG |
| `installer.windows.runAfterInstall` | 完成页提供运行选项。默认按用户安装时开启、全机安装时关闭 |
| `installer.windows.desktopShortcut` | `ask`（默认）、`always` 或 `never` |
| `installer.windows.languageDialog` | 启动时询问安装语言。默认跟随 Windows 显示语言 |
| `installer.windows.include` | NSIS 脚本扩展，可定义 `customInit`、`customInstall` 和 `customUnInstall` 宏 |
| `installer.windows.template` | 用 Go `text/template` 文件替换生成的 NSIS 脚本 |
| `installer.macos.dmg.background` | Finder 窗口背景，支持 PNG、JPEG、TIFF。同目录的 `@2x` PNG 可作 Retina 背景 |
| `installer.macos.dmg.windowSize` / `iconSize` | 窗口宽高和图标大小，单位为点，默认 `[540, 380]` / `128` |
| `installer.macos.dmg.appPosition` / `applicationsPosition` | 应用和「应用程序」链接图标的中心坐标，相对于窗口左上角，单位为点，默认 `[140, 190]` / `[400, 190]` |
| `installer.macos.dmg.title` | 卷名，默认应用名 |

## icons

每个字段都可选，不设的从 `icon` 派生：

```json
"icons": {
  "macos":   "icons/macos.png",
  "windows": "icons/app.ico",
  "linux":   { "app": "icons/linux.png", "symbolic": "icons/symbolic.svg" },
  "ios":     { "dark": "icons/ios-dark.png", "tinted": "icons/ios-tinted.png" },
  "android": { "foreground": "icons/fg.png", "background": "#1677FF", "monochrome": "icons/mono.png" },
  "notification": "icons/notify.png"
}
```

| 字段 | 说明 |
|---|---|
| `macos` | 已是 macOS 形状的 1024×1024 PNG，原样使用 |
| `windows` | 带形状、透明背景的 PNG（256 px 以上）或直接用的 `.ico` |
| `linux.app` / `linux.symbolic` | 替代派生形状的 PNG / 面板用的单色 SVG |
| `ios.dark` / `ios.tinted` | iOS 18 深色、着色外观的变体 |
| `android.foreground` / `background` / `monochrome` | 自适应图标的前景、背景（PNG 或 `#RRGGBB`）、Android 13 主题图标 |
| `notification` | Android 状态栏通知图标（白色形状、透明背景） |

`tan doctor` 会检查图标是否符合各平台要求。
