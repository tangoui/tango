# 应用图标

一个 Tango 应用最少只需要一张图：`tan.json` 里的 `"icon"`。`tan build` / `tan package` 会从它派生出各平台需要的全部图标；某个平台派生的效果不满意时，再在 `"icons"` 段单独给图。托盘图标在运行时由代码提供，不经过 `tan.json`。

运行 `tan doctor` 会检查项目里的图标；有 error 时退出码为 1，可以放进 CI。

## 主图标 `"icon"`

- 1024×1024 PNG，**满版**：图案铺满整个画布，不留透明边，不画圆角。
- iOS 和 Android 会按系统的形状自己裁切，直接使用满版图。
- 桌面系统要求图标自带形状，tan 自动派生：1024 画布中央 824×824 的连续曲率圆角方形，四周透明，带柔和投影（macOS 图标网格）。Windows 和 Linux 也使用这个形状。
- 图案主体放在中间约 60% 的范围内，这样被裁成圆形（Android）或加上圆角后都不会被切掉。

## 各平台生成什么

| 平台 | 产物 | 来源（未配置时） |
|---|---|---|
| macOS | `.app/Contents/Resources/icon.icns`，16–1024 共 10 张 | 主图标派生的桌面形状 |
| Windows | 嵌入 exe 的图标资源（16–256 共 8 档，组 ID 1，窗口、任务栏、托盘默认图标都用它），以及 manifest 和版本信息 | 主图标派生的桌面形状 |
| Linux | `share/icons/hicolor/<n>x<n>/apps/<exe>.png`（16–512），AppDir 根目录的 `<exe>.png` 和 `.DirIcon` | 主图标派生的桌面形状 |
| iOS | asset catalog 中 1024 的 AppIcon（不允许透明，透明处铺白） | 主图标原样 |
| Android | 自适应图标（108dp 前景/背景层，可选单色层）、API 26 以下用的方形和圆形图标 | 主图标缩到可见的 72dp 区域；背景取主图标边缘的平均色 |

## 单独给图 `"icons"`

所有字段都可选，路径相对项目目录：

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

| 字段 | 要求 |
|---|---|
| `macos` | 1024 PNG，已经是 macOS 形状（四周透明），原样使用 |
| `windows` | `.ico` 原样拆用；或 256 以上的 PNG（自带形状、透明背景） |
| `linux.app` | PNG，代替派生形状 |
| `linux.symbolic` | 单色 SVG，安装为 `<exe>-symbolic.svg`，供面板使用 |
| `ios.dark` / `ios.tinted` | 1024 PNG，iOS 18 的深色、着色外观；着色版用灰度图，由系统上色 |
| `android.foreground` | 432×432 透明 PNG，图案放在中心 66/108 安全区内 |
| `android.background` | 颜色 `#RRGGBB` 或 PNG |
| `android.monochrome` | 只用透明度的单色 PNG，Android 13 主题图标使用；不配置就不生成主题图标 |
| `notification` | Android 状态栏通知小图标：透明背景上的白色图形，约 96×96。系统只取透明度，彩色或不透明的图会显示成白方块 |

## 托盘图标

托盘图标由代码在运行时提供，通常用 `go:embed` 嵌入：

```go
//go:embed icons/tray.png
var trayPNG []byte // 彩色，Windows / Linux 使用，32×32 或 64×64

//go:embed icons/tray-mac.png
var trayMacPNG []byte // macOS 菜单栏模板图：黑色图形 + 透明背景，36×36 或 44×44

t, err := tray.New(app, tray.Options{
	Icon: trayPNG, MacIcon: trayMacPNG, Template: true, Tooltip: "我的应用",
})
```

- macOS 菜单栏应使用模板图（`Template: true`），系统会按浅色或深色菜单栏自动着色；Windows 和 Linux 显示彩色图。
- 只给 `Icon` 时所有平台都用它；`MacIcon` 只在 macOS 上代替 `Icon`。
- 各平台会按屏幕 DPI 自动缩放，给一张足够大的图即可。

完整示例见 `examples/ledger`：`tan.json` + `icons/icon.png`（主图标）+ `icons/tray*.png`（托盘）。
