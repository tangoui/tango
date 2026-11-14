# tan.json

`tan.json` 是项目配置文件，放在项目根目录。路径字段都相对于项目目录。`tan init` 会生成一份；未知字段会报错（防止拼错，也防止把密码之类写进来）。

`tan init` 生成的文件第一行是 `"$schema": "https://tangoui.github.io/tango/schema/tan.schema.json"`，VS Code 等编辑器据此补全字段、显示说明并标出拼错的字段。已有项目手动加上这一行即可；tan 会忽略它。未知字段报错也意味着按新版 tan 写的 `tan.json` 需要同样新的 tan。

## 完整示例

```json
{
  "name": "Notes",
  "id": "com.example.notes",
  "version": "1.2.0",
  "icon": "icon.png",
  "copyright": "© 2026 Example Ltd",
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
| `name` | 显示名，也决定可执行文件名 |
| `id` | 反向域名 ID（Bundle ID、applicationId、MSIX Identity），如 `com.example.notes` |
| `version` | `x.y.z`（可带预发布后缀；iOS 的 `CFBundleShortVersionString` 会去掉后缀） |
| `icon` | 主图标：正方形、铺满画布的 PNG，最好 1024×1024；桌面的圆角形状由 tan 派生 |
| `icons` | 各平台图标覆盖，见下 |
| `copyright` | 写进 Info.plist、安装器等 |
| `urlSchemes` | 深链 scheme（`notes` 对应 `notes://...`）；应用代码里的 `tango.Options.URLSchemes` 也要声明 |
| `fileAssociations` | 打开的文档类型：`ext`（不带点）、`name`、`role`（`editor` 默认 / `viewer`）、`mime`（默认 `application/x-<exe>-<ext>`） |
| `permissions` | 移动端用到的能力，见 [移动端](/guide/mobile#权限) |

## frontend

| 字段 | 说明 |
|---|---|
| `dir` | 前端根目录，命令在这里执行 |
| `dist` | 构建输出，相对 `dir`；源码直接作为静态文件时为 `.` |
| `install` | `dir` 有 `package.json` 但没有 `node_modules` 时执行，默认 `npm install` |
| `dev` | 开发服务器命令，`tan dev` 读取它打印的第一个 http 地址 |
| `devURL` | 地址读不出来时直接指定 |
| `build` | 生产构建命令 |

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
| `ios.build` | `CFBundleVersion`，默认等于版本号 |
| `ios.signIdentity` | 真机签名身份（`TANGO_IOS_SIGN_IDENTITY` 优先） |
| `ios.provisioningProfile` | 真机必需的描述文件 |
| `ios.entitlements` | 可选，默认用描述文件授予的 |
| `ios.usageDescriptions` | 按权限名覆盖系统弹窗里的用途说明 |
| `android.minSdk` / `targetSdk` | 默认 24 / 36，minSdk 不能低于 24 |
| `android.versionCode` | 默认由版本号推出：`major*10000 + minor*100 + patch` |
| `android.keystore` / `keyAlias` | release 签名，成对出现；密码见 `TANGO_ANDROID_KEYSTORE_PASSWORD` |

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
