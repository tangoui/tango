# 移动端（实验性）

同一个 Tango 应用不改代码就能构建成 iOS 和 Android 应用。界面仍是系统 WebView（iOS WKWebView、Android 系统 WebView），Go 代码编译成库，由 Tango 自带的原生宿主加载。

::: warning 实验性
iOS 已在模拟器（Apple Silicon，arm64）上验证，真机 SDK 能编译但**未在真机上运行验证**；Android 已在 API 36 模拟器上验证（启动、生命周期、消息往返、剪贴板、对话框、返回键），**未上真机**。接口和行为可能变化。
:::

## 运行模型

用户代码和桌面完全一样：

```go
func main() {
    app := tango.New(tango.Options{Name: "Hello", Assets: assets})
    app.OnReady(func() { app.NewSurface(tango.SurfaceOptions{}) })
    app.Run()
}
```

不同在于「谁拥有主线程」：

| | 桌面 | 移动端 |
|---|---|---|
| 进程入口 | Go 的 `main` | 原生宿主（iOS `UIApplicationMain`，Android `Activity`） |
| Go 产物 | 可执行文件 | iOS：`-buildmode=c-archive`；Android：`-buildmode=c-shared` |
| `main.main` 由谁调用 | Go 运行时 | 宿主完成启动后，在一个新 goroutine 里调用 |
| `App.Run` 所在线程 | 主线程（锁定） | 普通 goroutine，阻塞到退出 |
| 回调线程 | 主线程 | 主线程（宿主线程上的 cgo 回调） |

`main` 包零改动：宿主通过 `go:linkname` 取到 `main.main`。

- **Surface = 页面**：每个 Surface 是全屏的一页（iOS 视图控制器，Android 根布局里的一层），后建的盖在上面；关闭最上面的页面回到下一页。Android 返回键先在 WebView 历史里后退，没有历史时关闭页面。
- **生命周期**：`Started`、`Foreground`、`Background`、`Terminating` 对应系统回调；系统终止应用时投递 `Terminating` 并最多等 2 秒让 `main` 跑完。在 `Background` 里保存状态。
- **WebView 被系统回收**：内存紧张时系统可能杀掉 WebView 内容进程，后端会重新加载页面并发 `EventWebViewRestored`（`App.OnWebViewRestored`），页面状态需要自己恢复。
- **主线程不能嵌套等待（Android）**：在主线程回调里调用需要等待用户的函数会立即返回（`TryAwaitAsync` 给出 `ErrNestedWait`）。从 goroutine 调用对话框、分享、权限请求、定位等函数。

## 能力

基础能力（核心）：

| 能力 | iOS | Android |
|---|---|---|
| 剪贴板文本 | `UIPasteboard`（读取时系统可能提示） | `ClipboardManager`（Android 10+ 只有前台应用能读） |
| 消息框 | `UIAlertController` | `AlertDialog`（返回键 / 点外部 → `ErrCanceled`） |
| 打开文件 | 文档选择器，导入为沙盒副本 | 存储访问框架，复制到应用缓存目录 |
| 保存对话框 | `ErrUnsupported`（写入沙盒后用分享导出） | `ErrUnsupported` |
| 菜单 / 托盘 / 窗口几何 | 不适用 | 不适用 |

移动端专用的包在 `sys/` 下：[permission](/guide/system/permission)、[share](/guide/system/share)、[haptics](/guide/system/haptics)、[safearea](/guide/system/safearea)、[orientation](/guide/system/orientation)、[biometric](/guide/system/biometric)、[securestore](/guide/system/securestore)、[media](/guide/system/media)、[location](/guide/system/location)、[push](/guide/system/push)。深链（自定义 scheme、通用链接 / App Links）走 `EventOpenURL`，冷启动和热启动都处理。

### 安全区

页面要铺满刘海屏时，在 viewport 里加 `viewport-fit=cover`，再用 `safearea.SyncPage(app)` 把内边距写进 CSS 变量：

```css
header { padding-top: var(--tango-safe-top, env(safe-area-inset-top, 0px)); }
```

## 权限

在 `tan.json` 的 `permissions` 里列出用到的能力，`tan build` 生成 iOS Info.plist 的用途说明键和 Android 清单的 `<uses-permission>`：

```json
{
  "permissions": ["camera", "location", "notifications"],
  "ios": { "usageDescriptions": { "camera": "扫描二维码时使用相机。" } }
}
```

| 名字 | 用到它的 API | iOS | Android |
|---|---|---|---|
| `camera` | `media.TakePhoto`、`permission` | `NSCameraUsageDescription` | `CAMERA` |
| `photos` | `permission`（`media.Pick` 不需要） | `NSPhotoLibraryUsageDescription` 等 | `READ_MEDIA_IMAGES` 等 |
| `location` | `location.Current` / `Watch` | `NSLocationWhenInUseUsageDescription` | `ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION` |
| `location-always` | `permission` | 另加 `NSLocationAlwaysAndWhenInUseUsageDescription` | 另加 `ACCESS_BACKGROUND_LOCATION` |
| `microphone` | `permission` | `NSMicrophoneUsageDescription` | `RECORD_AUDIO` |
| `biometric` | `biometric.Authenticate` | `NSFaceIDUsageDescription` | `USE_BIOMETRIC` |
| `notifications` | `push.Register`、`permission` | 运行时询问（远程推送另需 `aps-environment`） | `POST_NOTIFICATIONS` |

iOS 上缺用途说明就调用相关 API 会被系统终止，Tango 在调用前检查，缺键时返回 `ErrNotConfigured`（错误信息写出缺的键名）。默认用途说明是英文模板，上架前请在 `ios.usageDescriptions` 里写具体用途。

完整对照（含隐私清单、通用链接、Android FCM 接入）见仓库的 [docs/mobile-permissions.md](https://github.com/tangoui/tango/blob/main/docs/mobile-permissions.md)。

## 构建与签名

### iOS

```bash
tan run -platform ios-simulator             # 构建、装到模拟器并启动，Go 日志接到终端
tan build -platform ios-simulator           # 模拟器 .app（ad-hoc 签名）
tan build -platform ios                     # 真机 .app + .ipa
tan build -platform ios-simulator -xcode    # 另外生成 Xcode 工程，便于调试
```

`tan build` 不需要 Xcode 工程：用 Xcode 的 clang 以 `c-archive` 编译 Go，链接 Tango 的宿主 `main.m`，按 `tan.json` 生成 Info.plist、编译图标、复制隐私清单并签名。

真机签名需要：

| tan.json | 说明 |
|---|---|
| `ios.signIdentity` | 签名身份，如 `Apple Development: Jane Doe (TEAMID)`；环境变量 `TANGO_IOS_SIGN_IDENTITY` 优先 |
| `ios.provisioningProfile` | `.mobileprovision`，tan 会检查它的 application-identifier 覆盖 `id` |
| `ios.entitlements` | 可选，默认使用描述文件授予的 entitlements |
| `ios.minVersion` / `ios.build` | 最低系统（默认 17.0）/ `CFBundleVersion` |

已知限制：只支持 arm64（真机与 Apple Silicon 模拟器）；不支持 iPad 多窗口场景；资源响应不能是 3xx 重定向（页面 URL 用 `/` 而不是 `/index.html`）。

### Android

```bash
export JAVA_HOME=...                              # JDK 17+
export ANDROID_HOME=~/Library/Android/sdk         # 含 NDK（或设置 ANDROID_NDK_HOME）
tan run -platform android                         # 设备 ABI 的 debug APK，安装、启动、显示日志
tan build -platform android -debug                # debug APK（arm64-v8a + x86_64）
tan build -platform android/arm64,android/amd64   # release：签名的 APK 与 AAB
```

每个 ABI 用 NDK clang 以 `c-shared` 编译出 `libtango.so`（16 KB 段对齐，满足 Android 15+ 设备），同步 Tango 自带的 Gradle 工程，写入应用名、图标、清单，再离线运行 Gradle。宿主只用平台 API，不依赖 androidx。

release 签名：

| tan.json / 环境变量 | 说明 |
|---|---|
| `android.keystore`、`android.keyAlias` | 成对出现 |
| `TANGO_ANDROID_KEYSTORE_PASSWORD` | keystore 密码 |
| `TANGO_ANDROID_KEY_PASSWORD` | key 密码，默认同上 |
| `android.minSdk` / `targetSdk` / `versionCode` | 默认 24 / 36 / 由版本号推出（1.2.3 → 10203） |

`tan run -platform android` 没有在线设备时会启动第一个 AVD（`-avd` 指定），日志里 Go 的输出在 `Tango` 标签下，页面 console 在 `chromium` 标签下。

## 调试与 CI

- `bash scripts/smoke.sh ios` / `android`：构建 `examples/mobile-hello`，在模拟器里运行，等页面发来消息即 PASS，见 [CI](/guide/ci)。
- 移动端的完整设计说明（宿主类、线程细节、各能力的实现方式）在仓库的 [docs/mobile.md](https://github.com/tangoui/tango/blob/main/docs/mobile.md)。
