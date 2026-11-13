# 移动端（iOS / Android）

本文件是 iOS 与 Android 后端共用的说明：先讲两端一致的运行模型和根包适配，再分平台写构建与行为差异。契约见 `internal/platform/{backend,types,caps}.go`，总体架构见 `architecture.md`。

## 运行模型（两端共用）

用户代码在所有平台一致，不需要任何移动端专用改动：

```go
func main() {
    app := tango.New(tango.Options{Name: "Hello", Assets: assets})
    app.OnReady(func() { app.NewSurface(tango.SurfaceOptions{}) })
    app.Run()
}
```

不同在于"谁拥有主线程"：

| | 桌面 | 移动端 |
|---|---|---|
| 进程入口 | Go 的 `main` | 原生宿主（iOS `UIApplicationMain` / Android `Activity`） |
| Go 产物 | 可执行文件 | iOS：`-buildmode=c-archive`；Android：`-buildmode=c-shared` |
| `main.main` 由谁调用 | Go 运行时 | 后端导出的启动函数（iOS `TangoStart`），在**新 goroutine** 里调用 |
| `App.Run` 所在线程 | 主线程（根包 `init` 里 `LockOSThread`） | 普通 goroutine，不锁线程 |
| `Backend.Init` / `Run` | 主线程，`Run` 跑原生事件循环 | Run 的 goroutine；`Run` 只阻塞到退出 |
| `Wake` | 唤醒原生循环后在主线程 `Host.Drain` | 同左（iOS：CFRunLoopSource） |
| `Step(block)` | 主线程跑一次原生事件 | 主线程跑一次 run loop（iOS：`CFRunLoopRunInMode`） |
| `Host.*` 回调 | 主线程 | 主线程（宿主线程上的 cgo 回调） |

要点：

1. **启动**：宿主在 UIKit/Android 完成启动时调用后端导出的启动函数。它安装原生侧（唤醒源、生命周期监听、窗口），然后 `go main.main()`。c-archive / c-shared 里 Go 运行时只做包初始化、不会自己调用 `main.main`，所以后端用 `//go:linkname userMain main.main` 取到它——用户的 main 包零改动，也不需要 build overlay。
2. **Run**：`App.Run` 在 main 的 goroutine 上调用 `Backend.Init`、`Backend.Run`。这两个调用不在主线程（与契约"Init/Run 在主线程"的字面说法不同，见文末"契约建议"）；后端在 Init 里只记录 Host，然后 `Wake`。第一次 Drain（主线程）时投递 `LifecycleStarted`，应用已处于前台则紧接着投递 `Foreground`。
3. **退出**：`App.Quit` → 后端在主线程投递 `Terminating`，`Run` 返回 → `main` 返回 → 后端结束进程（与桌面"main 返回即退出"一致）。系统终止应用（iOS `willTerminate`）时同样投递 `Terminating`、让 `Run` 返回，并最多等 2 秒让 `main` 跑完 defer。
4. **主线程判定**：核心不能把 Run 的 goroutine 当主线程。桌面上 `Run` 在整个运行期间记录自己的 goroutine 为主线程；移动端不记录，只依赖每个 `Host` 回调进入时的记录（`enterMain`）。因此在 Host 回调里（OnReady、OnMessage……）`Await` 直接执行、`AwaitAsync` 用 `Step` 泵事件；在其他 goroutine（包括 main 自己）里则排队等主线程。
5. **Run 之前在 main 里等主线程**：桌面上会 panic（防死锁），移动端同样：后端提供 main 所在 goroutine 的 id（`mobileMainGoroutine`），核心据此判断。

### 根包适配（由 iOS 子项目完成，Android 复用）

| 文件 | 内容 |
|---|---|
| `lock_desktop.go`（`!ios && !android`，已有） | 桌面在 `init` 里 `LockOSThread`；新增 `runOwnsMainThread = true`、`isMainGoroutine()` |
| `thread_mobile.go`（`ios \|\| android`，新增） | `runOwnsMainThread = false`；`mobileMainGoroutine` 钩子与移动端 `isMainGoroutine()` |
| `tango.go`（改一处） | `Run` 只在 `runOwnsMainThread` 时 `enterMain` |
| `loop.go`（改一处） | `mainBeforeRun` 改用 `isMainGoroutine()` |
| `backend_ios.go`（`ios`，新增） | `defaultBackend = ios.New`；`mobileMainGoroutine = ios.MainGoroutine` |
| `backend_android.go`（`android`） | Android 子项目添加，同样设置这两个变量 |

桌面行为不变（`CGO_ENABLED=1 go test -race ./ ./clipboard ./dialog ./menu` 通过）。

## iOS

### 构建

```bash
tan build -platform ios-simulator          # 模拟器 .app（ad-hoc 签名）
tan build -platform ios                    # 真机 .app + .ipa（需签名，见下）
tan build -platform ios-simulator -xcode   # 另外生成 Xcode 工程 build/xcode/<Exe>.xcodeproj
tan run -platform ios-simulator            # 构建、装到模拟器、启动，日志接到终端
```

`mobile/ios/build.sh <app-dir> [simulator|device]` 保留为 CI 入口，内部调用 `tan build`。

`tan build` 做的事（无需 Xcode 工程）：

1. `GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC=$(xcrun --sdk <sdk> --find clang)`，`CGO_CFLAGS/LDFLAGS=-isysroot <sdk> -target arm64-apple-ios<minVersion>[-simulator]`，`go build -buildmode=c-archive -tags tango_release`（`tango_release` 已无作用、仅为兼容保留；release 再加 `-trimpath -ldflags "-s -w"`，`-debug` 不加）。`tan build`（含 `-debug`）都是生产构建；只有 `tan dev -platform …` 加 `-tags tango_dev`，见 [开发功能与生产构建](dev-builds.md)。
2. 用同一 clang 把 Tango 模块里的 `mobile/ios/main.m` 与静态库链接：`-Wl,-ObjC -lresolv -framework Foundation UIKit WebKit UniformTypeIdentifiers CoreFoundation Security`，再加上各 Go 包 `#cgo LDFLAGS` 里的框架（c-archive 不携带链接参数，tan 用 `go list -deps` 收集）。`-ObjC` 保证只在 Info.plist 里按名字引用的 `TangoSceneDelegate` 不被裁掉。
3. 由 tan.json 生成 Info.plist（名称、ID、版本、最低版本、场景清单、`UILaunchScreen`、URL scheme、权限用途说明），`icon` 经 `actool` 编进 `Assets.car`（AppIcon，单张 1024×1024，透明处铺白）。
4. `mobile/ios/PrivacyInfo.xcprivacy`（隐私清单）复制进 .app 根目录。模拟器：把 entitlements（`application-identifier`、`keychain-access-groups`，值为 `id`，`ios.signIdentity` 带团队 ID 时加前缀）用 `-Wl,-sectcreate,__TEXT,__entitlements` 链接进可执行文件（Xcode 对模拟器的做法；否则 Keychain/`securestore` 报 `errSecMissingEntitlement`），再 `codesign -s -`。真机：放入 `embedded.mobileprovision`，用描述文件授予的 entitlements（或 `ios.entitlements`）以 `ios.signIdentity` 签名，再打包 `Payload/<Name>.app` 为 `<Exe>-<version>.ipa`。缺签名配置时在编译前报错。

产物：`build/ios-simulator/<Name>.app`、`build/ios/<Name>.app` 与 `.ipa`。`tan run -platform ios-simulator` 选已启动的模拟器（否则最新运行时的 iPhone，`-device` 指定名称或 UDID），`simctl install`，然后 `simctl launch --console-pty` 把 Go 的 log 输出接到终端；Ctrl-C 结束应用。

### 接入 Xcode 工程

`tan build -platform ios[-simulator] -xcode` 生成 `build/xcode/<Exe>.xcodeproj`（`Host/` 下是 main.m、Info.plist、图标）：第一个构建阶段按 Xcode 当前的 `$PLATFORM_NAME`、`$SDKROOT`、`$IPHONEOS_DEPLOYMENT_TARGET` 运行 `go build -buildmode=c-archive -o $DERIVED_FILE_DIR/libapp.a`，`OTHER_LDFLAGS` 链接它；自动签名（`DEVELOPMENT_TEAM` 取自 `ios.signIdentity` 括号里的团队 ID）。再次运行会覆盖这些文件。

手工接入已有工程时：

1. 加一个 "Run Script" 构建阶段（放在 Compile Sources 之前）做上面第 1 步，`ENABLE_USER_SCRIPT_SANDBOXING = NO`（脚本要读 Go 模块缓存）。
2. Build Settings：`OTHER_LDFLAGS` 加 `"$DERIVED_FILE_DIR/libapp.a" -ObjC -lresolv`；Link Binary With Libraries 加 WebKit、UniformTypeIdentifiers。
3. AppDelegate 的 `application:didFinishLaunchingWithOptions:` 里调用 `TangoStart()`（Swift：在桥接头里声明 `void TangoStart(void);`）。
4. Info.plist 的 Scene Manifest 把 `UISceneDelegateClassName` 设为 `TangoSceneDelegate`（`mobile/ios/Info.plist` 是参考模板）；必须有 `UILaunchScreen`，否则应用以兼容模式（黑边）运行。

### 原生宿主

`mobile/ios/main.m` 只有一个 `AppDelegate`：`didFinishLaunching` 里调 `TangoStart()`，`main()` 里关闭 stdio 缓冲后进入 `UIApplicationMain`。其余原生部分（场景代理、页面、scheme handler、对话框）都在 Go 静态库里（`internal/platform/ios/bridge.m`）。窗口由 `TangoSceneDelegate` 在场景连接时创建；Info.plist 没有场景清单时（老式窗口生命周期）由 `TangoStart` 直接创建。

Go 运行时在 `TangoStart` 之前就已由 c-archive 的加载初始化启动，但用户的 `main` 只在 UIKit 完成启动之后才开始跑，不与启动握手竞争。

### Surface 映射为页面

- 每个 Surface 是一个 `UIViewController`，视图是铺满的 `WKWebView`。
- 窗口的 `rootViewController` 是一个常驻容器控制器；**第一个显示的 Surface** 作为子控制器嵌入其中（根页面），之后显示的以**全屏模态** present 在当前最上层页面之上。不替换 `rootViewController`，避免首个页面紧跟窗口出现时 UIKit 的外观转场不配对。显示中的页面构成一个栈，`bridge.m` 的 `syncStack` 逐次调和 UIKit 的呈现链与该栈（等上一个转场动画结束再做下一个，有对话框在上面时等对话框结束）。
- `Hide`：页面出栈（模态 dismiss；根页面出栈时下一个页面成为根页面），可再 `Show`。`Close`：出栈、释放 WebView，投递 `SurfaceClosed`。没有显示中的页面时窗口只显示空白容器，应用不退出。
- iOS 没有关闭页面的用户手势，`ShouldClose` 不会被询问。
- `SetTitle` 只设 `UIViewController.title`（不显示）；`SetSize` 忽略；`Size` 返回页面当前尺寸（点）；尺寸变化（旋转、分屏）投递 `SurfaceResized`；`Focus`、`OpenDevTools` 为空操作；`Native` 返回 `UIViewController*`。
- **安全区**：WebView 铺满整个页面（含状态栏、Home 指示条区域）。页面不做处理时，WKWebView 自动按安全区内缩内容；页面若声明 `<meta name="viewport" content="viewport-fit=cover">`，则内容铺满，用 CSS `env(safe-area-inset-*)` 自行留边。
- `Debug` → `WKWebView.inspectable`，在 Mac 的 Safari "开发" 菜单里调试。
- JS 的 `alert()` / `confirm()` 用 `UIAlertController` 显示（WKWebView 默认什么都不显示）。

### JS 桥与资源

- 文档开始时注入 `window.__tangoNative={post:s=>webkit.messageHandlers.tango.postMessage(String(s))}`，随后注入核心的 BridgeScript。只接受主框架的消息，origin 取自 `WKSecurityOrigin`。
- 资源 origin 为 `tango://app`，`WKURLSchemeHandler` 在主线程同步调用 `Host.ServeAsset`；响应体每 64 KiB 或 `Flush` 时交给 WebKit，handler 返回即结束。WebKit 取消任务时请求的 `Context` 被取消。
- `didFinishNavigation` → `SurfaceLoaded`。

### 生命周期

| UIKit | Tango |
|---|---|
| `didFinishLaunching`（宿主调用 `TangoStart`）且 `App.Run` 已 Init | `Started`（在第一次 Drain 时，主线程） |
| `UIApplicationDidBecomeActiveNotification` | `Foreground` |
| `WillResignActive` / `DidEnterBackground` | `Background`（两者连续到达时只投递一次） |
| `WillTerminate` | `Terminating`，随后 `Run` 返回 |

使用通知中心而不是 AppDelegate 方法，宿主的 AppDelegate 因此不需要转发任何东西。

### 能力

| 能力 | iOS 实现 |
|---|---|
| Clipboard | `UIPasteboard.generalPasteboard`（读取时系统可能弹出粘贴授权提示） |
| MessageDialog | `UIAlertController`；`Kind` 无视觉对应；无按钮时显示 "OK"；只有应用退出时返回 `ErrCanceled` |
| OpenFileDialog | `UIDocumentPickerViewController`，文件以副本导入沙盒 `tmp/`，返回副本路径；`PickFolders` 返回文件夹真实路径（沙盒外需安全作用域访问）；`Title` 不显示 |
| SaveFileDialog | 返回 `ErrUnsupported`（iOS 没有"选择保存位置"面板；请写入应用沙盒，再用分享等方式导出） |
| AppMenu | 不实现（`menu` 包返回 `ErrUnsupported`） |

### 线程细节

- `Wake` 用加到主 run loop **common modes** 的 version-0 `CFRunLoopSource`（`CFRunLoopSourceSignal` + `CFRunLoopWakeUp`，任意线程安全），而不是 `dispatch_async_f` 到主队列：主队列是串行的，当一个主队列 block（或 Drain 本身）里嵌套跑 run loop（`AwaitAsync` 在主线程上 `Step`）时，后续主队列 block 不会执行，会死锁；run loop source 在嵌套循环和滚动跟踪模式里都会触发。
- `Step(block)` = `CFRunLoopRunInMode(kCFRunLoopDefaultMode, block ? ∞ : 0, true)`，处理一个源后返回；Wake 会触发 Drain 源从而结束阻塞的 Step。

### 自绘页（uihost，docs/specs/mobile-canvas.md M1、M3、M4）

- **上屏**（`ios/canvas.go`、`canvas.m`）：自绘页是页面栈里的 `TGCanvasPage`，内容由 `TGCanvasView` 画。实现 `platform.CanvasMetal`：GPU 路径在视图里加铺满的 `CAMetalLayer` 子层，复用 `ui/gpu/metal`；CPU 只是回退，整帧上屏（不实现 `CanvasDamagePresenter`）。
- **帧调度**：与 Android 共用 `mobilecanvas.Sched`。`Invalidate` 可在任意 goroutine 调用，只在空闲变为待处理时触发主 run loop common modes 里的 `CFRunLoopSource`（滚动跟踪和嵌套 run loop 中也会触发），由它恢复视图的 `CADisplayLink`；空闲几个 tick 后暂停。`layoutSubviews` 报尺寸变化后同步出一帧。后台时不出帧（iOS 会终止在后台提交 GPU 命令的应用）。
- **输入法**（M3，`canvas_text.{go,m}`、`canvas_textdoc.go`）：视图实现 `UITextInput`，UIKit 的高频查询由镜像文档回答，只有编辑才调 Go。M3a 只有组合串；控件发来 `input.TextInputState` 时（M3b）镜像是控件的文本，UIKit 对已提交文本的修改变成 `input.TextEdit` / `input.SelectionChange`。模拟器上 simctl 不能点按和打字，用 `SIMCTL_CHILD_TANGO_CANVAS_IME_SELFTEST` 运行自检（`canvas_selftest.go`）。
- **无障碍**（M4，`canvas_a11y.{go,m}`、`a11ybridge/ios.go`）：实现 `platform.CanvasAccessible`，把新增或变化的节点推给视图，UIKit 的查询不调 Go。VoiceOver 或切换控制运行时，或视图的 `accessibilityElements` 第一次被查询时（语音控制、XCUITest、辅助功能检查器）启用；两者都不在且 30 秒没有查询后关闭。
- **叠放层**（M5，`canvas_attach.{go,m}`）：`uihost.Attach` 把自绘层（`TGCanvasOverlay`，也是 `TGCanvasView`）插在页面的 `WKWebView` 上方或下方，随页面显示、隐藏、旋转和关闭；按 alpha 决定是否穿透触摸。叠放层只走 CPU。
- **iPad 指针与触控板**（M5，`canvas_pointer.{go,m}`）：悬停、触控板与滚轮滚动、指针形状（`UIPointerInteraction`）；点击仍按触摸处理。
- **内存警告**（M5）：系统内存警告变成 `tango.EventMemoryWarning`，uihost 裁剪 GPU 图集、图片纹理与排版缓存（`uihost/memory.go`）。
- M5 部分只有单测，本节没有模拟器实测记录。

### 已知限制

- 未在真机上验证（模拟器已验证，真机 SDK 能编译）。
- 资源响应若是 3xx 重定向，WKURLSchemeHandler 不会跟随，页面空白。典型坑：`http.FileServer` 会把 `/index.html` 重定向到 `./`，所以 URL 用 `/` 而不是 `/index.html`（macOS 后端同样如此）。
- 冷启动：iPhone 18 Pro 模拟器上从进程创建到首页 `didFinishNavigation` 约 1.2 s（8 次中位数；优化前 2.9 s）。两处优化：① `main` 之前的框架加载是主要瓶颈（PhotosUI 约 +2.5 s、StoreKit 约 +2 s、Photos/AVFoundation 各 +0.3–0.4 s），这些框架不再链接，`mobile.m`/`mobile2.m` 首次使用时 `dlopen`（StoreKit 在后台线程加载）；② `TangoStart` 时预热首个页面的 `WKWebView`（加载空文档，提前拉起 WebContent/Networking 进程），`tg_surface_new` 直接接管——页面级的 `WKWebViewConfiguration` 设置因此都要写在 `bridge.m` 的 `pageWebView()` 里。`TANGO_TRACE_STARTUP=1` 输出各阶段耗时（`TangoStart`、首次 Drain、WebView 创建、首个 `tango://` 请求、`didFinish`），`TANGO_NO_PREWARM=1` 关闭预热以便对比。
- `tango://` 是自定义 scheme，WebKit 不为它保存 cookie，`Set-Cookie` 不生效、请求不带 `Cookie`；`tg_task_respond` 的响应头同名只保留最后一个。需要会话时用 JS 桥或 `Authorization` 头。
- fetch/XHR 的 `Blob`、`FormData` 请求体经自定义 scheme 到不了 handler：`bodyshim.go` 注入的脚本先把它们转成 `ArrayBuffer`（文本、JSON、`ArrayBuffer`、`URLSearchParams` 原本就正常）。
- 多场景（iPad 多窗口）不支持：只用第一个场景的窗口。
- 只支持 arm64（真机与 Apple Silicon 模拟器）；x86_64 模拟器未适配。
- `Quit` 会结束进程，不符合 Apple 的人机界面指南，一般不要在 iOS 上调用。

## tan.json 的移动端字段

```json
{
  "name": "Hello", "id": "com.example.hello", "version": "1.2.0", "icon": "icon.png",
  "urlSchemes": ["hello"],
  "permissions": ["camera", "photos", "location"],
  "ios": {
    "minVersion": "17.0",
    "build": "42",
    "signIdentity": "Apple Development: Jane Doe (ABCDE12345)",
    "provisioningProfile": "mobile/hello.mobileprovision",
    "entitlements": "mobile/hello.entitlements",
    "usageDescriptions": {"camera": "Hello 用相机扫描二维码。"}
  },
  "android": {
    "minSdk": 24, "targetSdk": 36, "versionCode": 12,
    "keystore": "mobile/release.jks", "keyAlias": "upload"
  }
}
```

| 字段 | 说明 |
|---|---|
| `name` / `id` / `version` / `icon` / `urlSchemes` | 共用：显示名、Bundle ID 与 applicationId、版本（iOS 的 `CFBundleShortVersionString` 去掉 `-beta` 之类后缀）、图标、深链 scheme（iOS `CFBundleURLTypes`，Android `VIEW`+`BROWSABLE` intent-filter） |
| `permissions` | 用到的移动端能力，见下表；未知名字报错 |
| `appLinks` / `appLinkPaths` | 共用：iOS Universal Links 与 Android App Links 的域名和可选路径前缀，见下文"通用链接" |
| `ios.minVersion` | `MinimumOSVersion` 与编译目标，默认 17.0 |
| `ios.build` | `CFBundleVersion`，默认等于版本号 |
| `ios.signIdentity` | 真机签名身份（环境变量 `TANGO_IOS_SIGN_IDENTITY` 优先） |
| `ios.provisioningProfile` | 真机必需的 `.mobileprovision`；tan 检查它的 application-identifier 覆盖 `id` |
| `ios.entitlements` | 可选；默认用描述文件授予的 entitlements |
| `ios.usageDescriptions` | 按权限名覆盖系统弹窗里的用途说明（默认是 "<name> uses the camera." 这类英文；上架前应写具体用途） |
| `android.minSdk` / `targetSdk` | 默认 24 / 36；minSdk 不能低于 24（宿主所需），也决定 NDK clang 的 API 级别 |
| `android.versionCode` | 默认由版本号推出：`major*10000 + minor*100 + patch` |
| `android.keystore` / `keyAlias` | release 签名（成对出现）；密码见上文环境变量 |

路径都相对项目目录。

`tan init` 生成的 tan.json 只写安全默认值：`"permissions": []`、`"ios": {"minVersion": "17.0"}`、`"android": {"minSdk": 24}`，签名相关字段（`signIdentity`、`provisioningProfile`、`keystore`、`keyAlias`）一律不写，需要时再补。`tan init -mobile=false`（或交互向导里"Add iOS and Android support"答 n）不写 `ios`、`android` 两段。

### 权限

名字与 `sys/permission` 的 `Kind` 一致，详细说明见 `docs/mobile-permissions.md`：

| 名字 | iOS Info.plist | Android |
|---|---|---|
| `camera` | `NSCameraUsageDescription` | `CAMERA`；`uses-feature android.hardware.camera` required=false |
| `photos` | `NSPhotoLibraryUsageDescription`、`NSPhotoLibraryAddUsageDescription` | `READ_MEDIA_IMAGES`、`READ_MEDIA_VIDEO`、`READ_MEDIA_VISUAL_USER_SELECTED`、`READ_EXTERNAL_STORAGE`（maxSdk 32） |
| `location` | `NSLocationWhenInUseUsageDescription` | `ACCESS_COARSE_LOCATION`、`ACCESS_FINE_LOCATION`；`uses-feature android.hardware.location` required=false |
| `location-always` | 上一行 + `NSLocationAlwaysAndWhenInUseUsageDescription`（`UIBackgroundModes` 不自动加） | 上一行 + `ACCESS_BACKGROUND_LOCATION` |
| `microphone` | `NSMicrophoneUsageDescription` | `RECORD_AUDIO` |
| `biometric` | `NSFaceIDUsageDescription` | `USE_BIOMETRIC`、`USE_FINGERPRINT`（maxSdk 27） |
| `notifications` | 无（运行时请求；远程推送还需描述文件里的 `aps-environment`） | `POST_NOTIFICATIONS` |
| `background` | `UIBackgroundModes` `fetch`、`processing`；`BGTaskSchedulerPermittedIdentifiers` `<id>.tango.refresh`、`<id>.tango.processing` | `RECEIVE_BOOT_COMPLETED`（`TangoJobService` 总是声明） |
| `widget` | 无（iOS 小组件未实现） | 不是权限：声明 `org.tango.host.TangoWidget` 接收器 |

`INTERNET` 总是声明。表在 `cmd/tan/permissions.go`。

### 通用链接（iOS Universal Links / Android App Links）

让 `https://example.com/...` 的链接直接打开应用（不弹浏览器）。运行时两端都已接好：iOS 的 `NSUserActivity`（`continueUserActivity`）与 Android 的 `ACTION_VIEW` 都以 `AppEvent` 的 `OpenURL` 交给应用，`URL` 是完整的 https 链接。构建侧由 tan.json 两个字段驱动：

```json
{
  "appLinks": ["example.com", "*.example.com"],
  "appLinkPaths": ["/open/", "/share/"]
}
```

| 字段 | 说明 |
|---|---|
| `appLinks` | 域名列表，iOS 与 Android 共用。只写小写域名，不带 scheme、路径、端口（`https://example.com`、`example.com/app`、`example.com:8443` 都报错）；`*.example.com` 匹配所有子域名（不含 example.com 本身，需要的话两个都写） |
| `appLinkPaths` | 可选的路径前缀，两端共用；不写 = 该域名下所有路径。只允许字母、数字和 `- . _ ~ % /`（不允许 `*`、`?`） |

`tan build` 据此生成：

- **iOS 真机**：签名用的 entitlements 里加 `com.apple.developer.associated-domains` = `applinks:<域名>` 数组。基础 entitlements 照旧（`ios.entitlements`，否则取描述文件授予的），只合并这一个键：已有的其他条目（如 `webcredentials:`）保留，描述文件里的占位值 `*` 被替换。描述文件不含 Associated Domains 能力时构建会给出警告——这种包 iOS 拒绝安装，需要在 developer.apple.com 的 Identifiers 里给该 App ID 勾选 Associated Domains，再重新下载描述文件。`tan doctor` 在项目目录里也会检查这一点（"App Links" 一行）。
- **iOS 模拟器**：同样写进链接到可执行文件里的模拟器 entitlements。
- **`-xcode`**：生成 `Host/<Exe>.entitlements`（在 `ios.entitlements` 基础上合并）并设置 `CODE_SIGN_ENTITLEMENTS`；Xcode 自动签名会顺带给 App ID 打开该能力。
- **Android**：在宿主 Activity 里、深链 intent-filter 之后，每个域名一个 `<intent-filter android:autoVerify="true">`：`VIEW` + `DEFAULT` + `BROWSABLE`，scheme `https` 与 `http`，`android:host`，以及每个 `appLinkPaths` 一条 `android:pathPrefix`。Activity 已声明同一 host 时跳过，重复处理不会重复插入。

服务器那边要证明域名属于这个应用，用 `tan applinks` 生成两个文件：

```sh
tan applinks                                  # 两个都生成，写到 build/applinks/.well-known/
tan applinks -team ABCDE12345                 # 显式给 Apple Team ID
tan applinks -platform android -debug         # 只生成 assetlinks.json，用 debug keystore 的指纹
tan applinks -platform android -fingerprint AB:CD:...:EF -fingerprint 12:34:...:56
```

- `apple-app-site-association`（JSON，无扩展名）：`applinks.details[0].appIDs` 是 `<TeamID>.<id>`，`components` 由 `appLinkPaths` 生成（`{"/": "/open/*"}`；没写路径时是 `{"/": "*"}`）。Team ID 依次取 `-team`、`ios.provisioningProfile` 的 `TeamIdentifier`、`ios.signIdentity` 证书的 OU（读钥匙串，仅 macOS）；都读不到就报错要求 `-team`（在 developer.apple.com 的 Membership details 里能查到）。注意签名身份括号里的 ID 不一定是 Team ID，所以不从名字里猜。
- `assetlinks.json`：`relation` 为 `delegate_permission/common.handle_all_urls`，`target` 是 `android_app` + `package_name`（即 `id`）+ `sha256_cert_fingerprints`。指纹依次取 `-fingerprint`（可重复，带不带冒号都行）、`android.keystore`/`keyAlias` 经 `keytool -list -v`（需要 `$TANGO_ANDROID_KEYSTORE_PASSWORD`）；`-debug` 用 `~/.android/debug.keystore`（`androiddebugkey`/`android`），并提示它只对 debug 包有效。**用了 Play App Signing 的话，商店分发的包是 Google 的应用签名密钥签的**，要把 Play Console（测试与发布 → 应用完整性）里的 SHA-256 用 `-fingerprint` 加进去，上传密钥的指纹只对自己签的包有效。

把它们放到每个域名的 `https://<域名>/.well-known/` 下（`tan applinks` 会列出完整 URL）：必须 https、证书有效、不能重定向，`Content-Type: application/json`。通配符域名 `*.example.com` 的文件放在 `example.com`。两个文件可以放在同一个目录同时服务两端。

验证：

- **iOS（需要真机）**：Apple 通过自己的 CDN 抓 `apple-app-site-association`，安装或更新应用时下载，服务器改了文件后可能要等一段时间。安装后在"备忘录"或"信息"里点 `https://example.com/open/...`，应直接打开应用；Safari 地址栏里直接输入不会触发（这是系统行为）。也可以在设备的 设置 → 开发者 → Associated Domains Development 里诊断。服务器侧可用 `curl -sI https://example.com/.well-known/apple-app-site-association` 确认 200、无重定向、类型为 JSON。
- **Android**：装好后等系统验证（Android 12+ 可手动触发）：

  ```sh
  adb shell pm set-app-links --package <id> 0 all     # 重置
  adb shell pm verify-app-links --re-verify <id>      # 重新验证
  adb shell pm get-app-links <id>                     # 每个域名应为 verified
  adb shell am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d "https://example.com/open/1"
  ```

  `none` 表示还没验证完，稍后再查；其他状态（`legacy_failure`、≥1024 的错误码）多半是文件不可达、包名或指纹不对。可用 `aapt2 dump xmltree --file AndroidManifest.xml <apk>` 查看生成的 intent-filter。

## 契约建议（已在契约第三轮落地）

1. ~~`Backend.Init` / `Backend.Run` 在移动端的线程~~：`backend.go` 已写明移动端 Init/Run 在调用 `App.Run` 的 goroutine 上，`Run` 只阻塞到退出。
2. ~~`Terminating` 的线程~~：已注明 Started/Terminating 都在主线程投递，`Run` 在 Terminating 之后才返回。

## Android

### 构建

```bash
export JAVA_HOME=...      # JDK 17+
export ANDROID_HOME=~/Library/Android/sdk
tan build -platform android -debug                  # debug APK，arm64-v8a + x86_64
tan build -platform android/arm64,android/amd64     # release：签名的 APK 和 AAB
tan run -platform android                           # 构建设备 ABI 的 debug APK、安装、启动、显示日志
```

`mobile/android/build.sh [APP_DIR]` 保留为 CI 入口（`TANGO_ABIS`、`TANGO_BUILD=debug|release`、`TANGO_GRADLE_ARGS`），内部调用 `tan build`。

`tan build` 的步骤：

1. 对每个 ABI（`android/<arch>`：`arm64`、`amd64`、`arm`、`386`，也接受 `arm64-v8a` 这类 ABI 名；只写 `android` 时为 arm64 + amd64）用 NDK 的 clang（`<ndk>/toolchains/llvm/prebuilt/<host>/bin/<triple><minSdk>-clang`）执行 `GOOS=android GOARCH=… CGO_ENABLED=1 go build -buildmode=c-shared`，输出到 Gradle 工程的 `jniLibs/<abi>/libtango.so`。NDK 取 `ANDROID_NDK_HOME`，否则 `$ANDROID_HOME/ndk/` 下最新版本。Go 链接出的段按 16 KB 对齐，满足 Android 15+ 16 KB 页设备的要求。
2. 把 Tango 模块里的 `mobile/android` 同步到 `build/android/project`（保留 `.gradle`、`app/build` 以便增量构建），然后按 tan.json 写入：`res/values/tango_app.xml`（应用名）、`mipmap-*/ic_launcher.png`（由 `icon` 缩放）、AndroidManifest.xml（在模板上插入权限、`uses-feature required=false`、`TangoActivity` 上的深链 `intent-filter`、图标；模板里其余声明原样保留）。
3. `sh gradlew --offline -PtangoAppId=… -PtangoVersionName=… -PtangoVersionCode=… -PtangoMinSdk=… -PtangoTargetSdk=…`：`-debug` 时 `assembleDebug`；否则 `assembleRelease bundleRelease`，用 `android.keystore`/`android.keyAlias` 签名，密码从环境变量 `TANGO_ANDROID_KEYSTORE_PASSWORD`、`TANGO_ANDROID_KEY_PASSWORD`（默认同前者）读，缺配置时在编译前报错。release 加 `-tags tango_release` 与 `-s -w`。宿主依赖 Android Gradle 插件与 `androidx.webkit`（WebView 桥的 WebMessageListener）；未设 `TANGO_GRADLE_ARGS` 且 `--offline` 构建失败时（例如 Gradle 缓存里还没有这些依赖），tan 自动去掉 `--offline` 联网重试一次，所以首次构建需要网络，之后可离线。

产物：`build/android/<Exe>-<version>-debug.apk`，或 `<Exe>-<version>.apk` 与 `.aab`。

`tan run -platform android`：用 `adb devices` 里唯一在线的设备（`-device` 指定序列号）；没有设备时启动第一个 AVD（`-avd` 指定）并等开机完成；按 `ro.product.cpu.abilist` 只编该设备的 ABI；`adb install -r`（签名不符时先卸载）、`am start`，然后 `adb logcat --pid=<应用进程>` 显示本应用日志（Go 的 stdout/stderr 与 log 包在 `Tango` 标签下，页面 console 在 `chromium` 标签下），应用退出或 Ctrl-C 时结束。

`mobile/android` 是普通 Gradle 工程（Groovy DSL、Gradle wrapper 9.3.1、AGP 9.1.0，默认 minSdk 24、compileSdk/targetSdk 36），宿主只用平台 API，不依赖 androidx，有 Gradle 缓存即可离线构建。AGP 9 即便是纯 Java 工程也要求 Kotlin Gradle 插件在 classpath 上，根 `build.gradle` 固定为常见已缓存的 2.4.0；`android.builtInKotlin=false` 防止 kotlin-stdlib 被打进 APK。

### 原生宿主

`mobile/android/app/src/main/java/org/tango/host/`，Java 包名固定为 `org.tango.host`（Go 侧的 JNI 符号 `Java_org_tango_host_Tango_*` 依赖它），与 applicationId 无关：

| 类 | 作用 |
|---|---|
| `Tango` | JNI 边界：`System.loadLibrary("tango")`，`native` 方法由 Go 实现；静态方法供 Go 调用（`wake`、`newPage`、剪贴板、对话框、`quit`/`exit`） |
| `TangoActivity` | 唯一的 Activity：根 `FrameLayout` 承载页面栈（`PageBase`）、生命周期、返回键、按页面分发 inset、遮挡判断 |
| `PageBase` | 页面栈条目的公共部分：id、标题、显示/隐藏/聚焦、`view()`、`applyInsets`、返回 |
| `Page` | WebView 页（`PageBase` 子类）：WebView、`WebViewClient`（资源拦截、注入、加载事件）、`JavascriptInterface` |
| `CanvasPage`、`CanvasView` | 自绘页（`uihost.Open`）：整页 `SurfaceView`，`Choreographer` 帧回调，触摸/鼠标/按键转发 |
| `TangoCanvas` | 自绘页的 JNI 边界（`Java_org_tango_host_TangoCanvas_*`，Go 侧 `canvas_android.go`） |
| `Dialogs` | `AlertDialog` 与文档选择器 |

Go 侧调用 Java 时，方法 ID 在 `nativeInit`（主线程）里一次查好：此后 Go 自己创建的线程挂到 JVM 上只能看到系统类加载器，`FindClass` 找不到应用类。字符串按 UTF-16 传递（JNI 的 "modified UTF-8" 会弄坏 BMP 以外的字符，如 emoji）。JNI 对象引用在 Go 里是 `uintptr`（cgo 对 `jobject` 系列的约定）。

### 启动、线程与退出

| Android | Tango |
|---|---|
| `onCreate`：`Tango.nativeInit()`（每进程一次：查类和方法、把 fd 1/2 接到 logcat、`go main.main()`），然后报告 Started | `Started`（`App.Run` 开始后第一次 Drain 时投递） |
| `onResume` | `Foreground`（Run 前发生的会在 Started 之后补投） |
| `onPause` | `Background` |
| `onDestroy` 且 `isFinishing()`（返回键关掉最后一页、`App.Quit`、`finish()`） | `Terminating`，随后 `Backend.Run` 返回 |
| `onDestroy` 但不是结束（系统回收、“不保留活动”、未就地处理的配置变化） | 不报告：页面栈移交给下一个 Activity 实例（见下） |

- `Wake`（任意 goroutine）：Go 端用原子标志合并，再经 JNI 调 `Tango.wake()` → `Handler(Looper.getMainLooper()).post(…)` → 主线程 `nativeDrain` → `Host.Drain`。非 Java 线程调用时临时 `AttachCurrentThread`，调用完分离。
- `Quit`：`finishAndRemoveTask()`；`onDestroy` 投递 `Terminating`、`Run` 返回；`main.main` 返回后宿主结束 Activity 并 `System.exit(0)`，与桌面“main 返回即退出”一致。
- 配置变化（旋转、深色模式、字体缩放……）在清单的 `android:configChanges` 里声明为就地处理，Activity 不重建；`launchMode="singleTask"`。
- **Activity 重建**：页面栈属于进程而不是某个 Activity 实例（`TangoActivity` 的静态栈）。系统销毁 Activity 但应用没有结束（`isFinishing()` 为假：开发者选项“不保留活动”、后台回收已停止的 Activity、宿主子类去掉了 `configChanges` 后的旋转/深色模式切换）时，WebView 从旧根布局摘下、继续存活（JS 状态、表单、滚动位置都在），其 `MutableContextWrapper` 改指向 Application；新实例 `onCreate` 时把它们挂进新的根布局并改指向新实例，再重放状态栏/常亮/亮度设置。Go 只看到 `Background` → `Foreground`，`Started` 不会重复；有保存状态的 `onCreate` 不重复投递启动 Intent 里的深链。没有 Activity 时 Go 创建的页面（后台任务启动的进程）先建在 Application 上下文里，等 Activity 出现再显示。只有真正结束时才发 `Terminating`。进程在后台被直接杀掉时仍不会收到 `Terminating`，需要保存的数据应在 `Background` 时保存；进程被杀后系统用保存状态重建 Activity 时，Go 从头启动（页面重新加载）。
- **`Step` 与 `AwaitAsync`**：Android 主线程不能嵌套泵消息（嵌套 `Looper.loop()` 无法在不退出主 Looper 的情况下返回；`MessageQueue.next` 是被禁用的隐藏 API）。所以 `Step` 只执行 Wake 请求的 Drain（核心队列），`block` 时最多等 10 ms 或下一个 Wake，始终返回 true（终止后 false）。因此后端实现 `platform.NestedLoop` 并报告不支持（契约第三轮）：在主线程回调（`OnReady`、`OnMessage`、`OnLifecycle`、`Surface.OnClose` 等）里调用 `AwaitAsync` 会立即返回零值，`TryAwaitAsync` 返回 `tango.ErrNestedWait`；对话框、通知权限、`sys/` 下的异步能力和 `menu.Popup` 都用后者，所以直接返回这个错误，而不是卡死到 ANR。在 Android 上从 goroutine 调用它们：`go func() { n, err := dialog.Message(app, …) }()`。

### Surface 映射为页面

- 每个 Surface 是一个铺满的 `WebView`，加进 Activity 根 `FrameLayout`：**第一个**即根页面，**之后的叠加在上面**（同一个 Activity 里的视图栈，不开新 Activity，免去 Activity 启动的异步与生命周期交错）。最上面一个可见页面是“当前页”，它变化时投递 `SurfaceFocused` / `SurfaceBlurred`，并把它的标题设为 Activity 标题（最近任务里可见）。
- `Hide`/`Show`：`View.GONE`/`VISIBLE`；`Focus`：移到栈顶；`Close`：移除并销毁 WebView，投递 `SurfaceClosed`。代码关闭最后一个页面不会退出应用（留下空白 Activity）。
- **返回键**（API 33+ 用 `OnBackInvokedCallback`——targetSdk 36 起 `onBackPressed` 不再被调用；更低版本用 `onBackPressed`）：当前页能后退（`canGoBack`）就后退；否则询问 `Host.ShouldClose`，返回 true 则关闭该页；关闭的是最后一个页面时 `finish()`，应用随之终止。
- `SetSize` 忽略；`Size` 返回最近一次布局的 dp 尺寸（首次布局前为 0, 0），布局变化投递 `SurfaceResized`。`OpenDevTools` 为空操作。`Native` 返回 `android.webkit.WebView` 的 JNI 全局引用，Surface 关闭前有效。
- `Debug` → `WebView.setWebContentsDebuggingEnabled(true)`（全进程），在电脑 Chrome 的 `chrome://inspect` 调试。
- `onPageStarted` → `SurfaceNavigating`，`onPageFinished` → `SurfaceLoaded`，主框架 `onReceivedError` → `SurfaceLoadFailed`；渲染进程崩溃或被系统回收（`onRenderProcessGone`）时重建该页的 WebView 并重新加载（见下文「系统能力」），10 秒内连续崩溃 3 次以上才关闭该页。
- 系统栏：Android 15+ 强制 edge-to-edge，`adjustResize` 不再随键盘缩小窗口。根布局不再加内边距，inset 按页面分发（API 30+，`PageBase.applyInsets`）：WebView 页给自己的 WebView 设与原内边距相同的 margin（系统栏、刘海、输入法），页面不会被遮挡，也不需要处理 `env(safe-area-inset-*)`，外观与以前一致；自绘页保持铺满，把系统栏 + 刘海作为安全区、输入法作为键盘 inset 报给 Go（见下节）。`sys/device` 的状态栏颜色仍画在 WebView 页上方空出的条带里。API 24–29 仍由根布局 `fitsSystemWindows` 处理。
- 自绘页（`uihost.Open`）与 WebView 页同在一个栈里，规则相同（先显示的为根，后来的叠在上面，返回键关闭）。被上面可见页面完全遮住的自绘页设为 `INVISIBLE`（`SurfaceView` 随之释放 Surface，不出帧）；同一窗口里多个 SurfaceView 的相对 z 序没有保证，这样也就不会出现下层盖住上层。

### 自绘页（uihost，docs/specs/mobile-canvas.md M2–M4）

- **上屏**：`CanvasView` 是整页 `SurfaceView`（`RGBX_8888`，不透明）。`surfaceChanged` 时 Go 用 `ANativeWindow_fromSurface` 取窗口并 `setBuffersGeometry`，报 `CanvasResized` 后**同步**出一帧（旋转时不出现拉伸的旧帧）；`surfaceDestroyed` 时同步 `ANativeWindow_release`。`Present` 逐行拷贝（按 `stride`；图像比缓冲小 1 px 时重复最后一行/列；缓冲是 `RGB_565` 时转换并打一次日志）。实现了 `platform.CanvasDamagePresenter`：脏矩形取外接矩形交给 `ANativeWindow_lock` 的 `inOutDirtyBounds`，按系统返回（可能扩大）的区域拷贝。整页默认走 GPU（M5）：OpenGL ES 3.0 + EGL 呈现器（`ui/gpu/gl/presenter_android.go`，经 `platform.CanvasNativeWindow` 接入，见 `uihost/gpu_android.go`），Surface 回来时重建窗口表面，上下文丢失后重建；呈现器起不来或在软件光栅化上运行时回退到上面的 CPU 路径（带局部上屏）。`adb shell setprop debug.tango.renderer cpu` 强制走 CPU，设为 `gpu` 则连软件光栅化也用（应用启动时读取，对应桌面的 `TANGO_RENDERER`）。
- **帧调度**：`mobilecanvas.Sched`。`Invalidate` 可在任意 goroutine 调用，只在空闲变为待处理时经 JNI 调 `TangoCanvas.requestFrame`（主线程上直接调；其他 goroutine 交给一个只 attach 一次的常驻线程）→ `Handler.post` → `nativeWake`；`Choreographer` 帧回调每个 vsync 调 `nativeFrame`，有请求才出帧，空闲 4 个 tick 后停；`InvalidateAt` 用 `Handler.postAtTime`。调度器的结论（帧回调开/停、定时器）作为各 native 调用的 long 返回值交给 Java 执行，Go 不在这些回调里反调 Java。Surface 不存在时（后台、被遮住、隐藏）调度器处于后台状态，不出帧，请求保留到 Surface 回来。
- **尺寸**：`Scale()` = `DisplayMetrics.density`（模拟器 2.625），`Size()` = Surface 像素 ÷ density。`Surface.Size` 返回 dp 并投递 `SurfaceResized`；`Native` 返回 `CanvasView` 的 JNI 全局引用。
- **输入**：触摸屏事件整包（所有手指、历史点）打成一个 `float[]` 过 JNI，经 `mobilecanvas.Touches`/`Coalesce` 生成 `input.TouchEvent`（ID 递增，历史点最多补 4 个，`ACTION_CANCEL` 与窗口失焦取消全部手指）；第一根手指由 Runner 合成指针事件。鼠标（`TOOL_TYPE_MOUSE`）、悬停（`ACTION_HOVER_*`）和滚轮（`ACTION_SCROLL`，系统滚动系数）产生 `input.Pointer` / `input.Scroll`。硬件键盘：键码与 meta 状态按 `canvas_keys.go` 映射（字母、数字按当前布局产生的字符，其余按位置；小键盘按 NumLock），文本输入开启时可打印字符另发 `TextInput`。返回键不交给自绘页，按页面栈规则处理（`ShouldClose` 后关闭）。`SetCursor` → `PointerIcon`。
- **安全区与键盘**：`platform.CanvasInsets`，值来自上面的按页面分发（`systemBars | displayCutout`、`ime`），变化时 Invalidate。
- **系统设置**：后端实现 `SystemTheme`（夜间模式 + Android 12+ 系统强调色 `system_accent1_500`，`onConfigurationChanged` 时发 `AppThemeChanged`）、`MotionPreference`（`ANIMATOR_DURATION_SCALE == 0`，`ContentObserver` → `AppReduceMotionChanged`）、`Locales` + `LocaleWatcher`（`LocaleList`，`onConfigurationChanged`）、`ContrastPreference`（Android 14+ `UiModeManager.getContrast() >= 0.5`，即“中/高对比度”，`addContrastChangeListener` → `AppContrastChanged`；更早的版本没有公开 API，报 false）。uihost 的主题、语言、减少动态效果与对比度跟随直接生效。
- **文字**：纯 Go 光栅化，字体由 `ui/text/sys_android.go` 解析 `/system/etc/font_fallback.xml`（Android 15+；API 36 模拟器上应用读不到，SELinux）或 `/system/etc/fonts.xml`，加 `/product/etc/fonts_customization.xml`：`sans-serif` 为默认家族，无名家族按文档顺序组成回退链，CJK 按 `text.DefaultLang()` 选 `zh-Hans`/`zh-Hant`/`ja`/`ko` 家族（TTC 子字体序号、`wght` 轴），每个家族只实例化 300–700 字重。彩色 emoji（CBDT）没有轮廓，画成空白（符号与 emoji 家族优先匹配 emoji 区段，避免遍历加载全部回退字体）。
- **调试**：`adb shell setprop debug.tango.canvas 1`（应用启动时读取）记录输入事件和帧耗时摘要，`2` 再记录每帧（含 `ANativeWindow_lock` 等待、拷贝、提交各自耗时）和调度动作。
- **软键盘与输入法**（M3，`canvas_ime.go`、`canvas_ime_android.go`、`CanvasInput.java`）：文本输入开启时 `CanvasView` 作为文本编辑器，`onCreateInputConnection` 交给输入法一个 `CanvasInput`，每次编辑同步调一次 `TangoCanvas.nativeIME`，Go 把它转成输入事件并更新 Java 侧的镜像文档（`SpannableStringBuilder`），输入法的高频查询不过 JNI。控件不描述文本时（M3a）镜像只含组合串；控件发来 `input.TextInputState` 时（M3b）镜像是选区附近的文本窗口，对已提交文本的修改变成 `input.TextEdit`。
- **无障碍**（M4，`canvas_a11y.go`、`canvas_a11y_android.go`、`CanvasA11y.java`、`a11ybridge/android.go`）：`CanvasView` 的 `AccessibilityNodeProvider` 同步向 Go 取节点（视图本身是根，其余是虚拟视图），动作经 `nativeA11yAction` 回到 Go，内容变化、焦点与播报经 `CanvasPage` 排队发出。
- **叠放层**（M5，`canvas_attach_android.go`、`CanvasOverlay.java`）：`uihost.Attach` 用半透明 `TextureView` 加在页面 frame 里 WebView 的上方或下方，帧、输入、输入法、无障碍与整页共用代码；缓冲是预乘 `RGBA_8888`，只走 CPU。
- **返回键**（M5，`ui/back.go`、`uihost/back.go`）：自绘界面有可关闭的东西（Modal、Drawer、弹出菜单、移动端 Popup 等）时，返回键作为 Escape 交给 Go，逐层关闭；没有时按页面栈规则处理。
- **内存**（M5）：`onTrimMemory` 变成 `tango.EventMemoryWarning`，uihost 裁剪 GPU 图集、图片纹理与排版缓存。
- M3、M4、M5 本节没有模拟器实测记录（上面的模拟器结果是 M2 的）。

模拟器结果（AVD API 36，1080×2400，density 2.625，`examples/mobile-ui`）：拉丁、中文、希腊、阿拉伯文正常显示，安全区上 52 / 下 48 dp；旋转后 914×411，安全区变为左 52（刘海）/ 右 48（导航栏）/ 上 28；`input tap/swipe` 驱动点按与滚动，`input keyevent` 与 `input mouse` 产生按键、指针与滚轮事件；深色模式与“移除动画”实时跟随；WebView 页叠在自绘页上时自绘页 Surface 释放，返回后重建并同步出帧；Home 后不再出帧；根页按返回键关闭后应用退出。帧耗时：滚动帧约 10–12 ms，其中 `ANativeWindow_lock` 等待空闲缓冲约 8 ms（vsync 背压，在主线程上）、拷贝 0.3–0.9 ms，光栅化约 2–3 ms；整页重画（切换深色）约 25 ms（光栅化约 7 ms，其余为 lock 与提交）；冷启动首帧约 0.3–0.45 s（字体解析）。

### 资源

- origin `https://tango.localhost`，在 `shouldInterceptRequest` 里拦截（只拦截该 origin，其余请求照常走网络）→ JNI `nativeServeAsset` → `Host.ServeAsset`。
- **线程**：`shouldInterceptRequest` 在 WebView 的请求线程上调用。契约第二轮已允许 `ServeAsset` 在任意线程调用，所以直接在该线程调用，不切到主线程（切过去同步等待会让慢请求卡住界面，且主线程在 Step 里等待时有死锁风险）。handler 在独立 goroutine 里运行。
- **流式响应**：请求线程等到响应提交（`WriteHeader`、第一次 `Write` 或 `Flush`）或 handler 返回。已返回的整体交给 WebView；还在写的经 `InputStream`（`Page.GoStream` → `nativeStreamRead`）边写边读，所以 SSE 之类长响应可用。写在前面最多缓冲 256 KiB，之后 `Write` 等读取。WebView 关闭流时请求的 `Context` 被取消，`Write` 返回错误。
- **请求体**：Android 的 `WebResourceRequest` 不提供请求体，所以注入的引导脚本（`body.go` 的 `bodyScript`，在 shim 的闭包里）包装了 `fetch` 与 `XMLHttpRequest`：目标是资源 origin（`https://tango.localhost`）、方法不是 GET/HEAD 且有 body 的请求，先把 body 序列化（`fetch` 用 `new Request(...).clone().arrayBuffer()`，所以文本、JSON、`URLSearchParams`、`FormData`（含文件，multipart 边界与 `Content-Type` 一致）、`Blob`、`ArrayBuffer`/TypedArray 都按浏览器规则编码；XHR 的字符串/`URLSearchParams`/`ArrayBuffer` 同步编码，`Blob`/`FormData` 经 `new Response(body)` 异步读出后再 `send`），base64 后经 `__tangoAndroid.body(token, id, contentType, data)` **同步**交给 Go 暂存（`nativeStashBody`，校验页面 token），然后带 `X-Tango-Body: <随机 id>` 头、空 body 发出请求；`shouldInterceptRequest` 时 Go 按 id 取回 body（`takeBody`），去掉该头，用暂存的 `Content-Type` 覆盖 WebView 为空 body 生成的那个。限制：单个 body 64 MiB、待取合计 128 MiB、1 分钟未取即丢弃，超限时退回原请求（body 为空）；同步 XHR 的 `Blob`/`FormData`、`navigator.sendBeacon`、HTML `<form method=post>` 导航、ReadableStream body 不覆盖（仍为空）。307/308 同源重定向会带着 body 重发。
- **状态码**：`WebResourceResponse` 拒绝 3xx。同源重定向在 Go 侧内部跟随（最多 5 次，307/308 保留方法），所以 `http.FileServer` 把 `/index.html` 重定向到 `./` 也能正常显示；跨源重定向返回 502。条件请求头（`If-None-Match`、`If-Modified-Since`）不转发，避免 304。
- 响应头：`WebResourceResponse` 的头映射每个名字只有一个值，同名多值按 RFC 9110 §5.3 用 `, ` 合并。`Set-Cookie` 例外（值里可以有逗号）：Go 用换行连接各值，`Page.java` 在把响应交给 WebView 之前逐条 `CookieManager.setCookie(url, …)` 并移除该头（WebView 本来就忽略拦截响应里的 Set-Cookie）；`HttpOnly` 有效（`document.cookie` 看不到，请求会带）。反方向，拦截到的请求本身不带 Cookie，宿主从 `CookieManager.getCookie(url)` 补上 `Cookie` 头，所以基于 cookie 的会话在 Go handler 里可用。

### JS 桥

- `addJavascriptInterface` 暴露 `window.__tangoAndroid.post(token, msg)`（在 JavaBridge 线程上调用，宿主再 post 到主线程交给 Go）。shim：
  `window.__tangoNative = { post: s => __tangoAndroid.post(<token>, String(s)) }`，不可改写。
- **为什么带 token**：JavascriptInterface 对所有 frame 可见，而且不告诉调用者是哪个 frame。每个页面有随机 token，只写在注入主框架的脚本里，shim 在子框架（`window !== window.top`）中不定义；Go 丢弃 token 不对的消息。跨源 iframe 读不到父页面的脚本，也就伪造不了消息。报告给核心的 origin 是主框架当前 URL 的 origin（`WebView.getUrl()`，在主线程读）。注入的 `<script>` 执行后把自己从 DOM 移除。
- **注入时机（取舍）**：androidx.webkit 的 `addDocumentStartJavaScript` 能在文档开始时注入，但要引入 androidx 依赖，且依赖 WebView 版本支持。这里不引依赖：
  1. `tango.localhost` 的主框架 HTML 响应：Go 把 shim + BridgeScript 作为内联 `<script>` 插到 `<head>` 开头（没有 `<head>` 时插在 `<html>`/doctype 之后），先于页面任何脚本执行。脚本先转成纯 ASCII（`\uXXXX`），并转义 `</script` 与 `<!--`，任何字符集的页面都安全。插入后去掉 `Content-Length`。
  2. 其他 origin（开发服务器等）或无法改写的响应（`Content-Encoding` 压缩过的 HTML）：`onPageStarted` 时 `evaluateJavascript` 注入（先检查 `window.__tangoNative` 避免重复），**可能晚于页面自己的早期脚本**。开发时可以在 `Assets` 里用 `httputil.ReverseProxy` 代理开发服务器，让页面走 `tango.localhost` 以获得第 1 种注入。
  3. 页面 CSP 禁止内联脚本时第 1 种会被拦截：给 CSP 加 `'unsafe-inline'` 或相应 hash/nonce。

### 能力

| 能力 | Android 实现 |
|---|---|
| Clipboard | `ClipboardManager`（Android 10+ 只有前台获得焦点的应用能读取） |
| MessageDialog | `AlertDialog`：≤3 个按钮依次放在 positive / negative / neutral 位置，更多按钮显示为列表；返回键或点外部关闭 → `ErrCanceled`；`Info`/`Warning`/`Error` 显示系统图标 |
| OpenFileDialog | `ACTION_OPEN_DOCUMENT`（存储访问框架），按扩展名过滤（`MimeTypeMap`，有无法识别的扩展名则不过滤）；选中的文档在后台线程**复制到应用缓存目录** `cache/tango-picked/…`，返回副本路径，`os.Open` 可直接用。`PickFolders` → `ErrUnsupported`（SAF 给的是树 URI 不是路径） |
| SaveFileDialog | `ErrUnsupported`（`ACTION_CREATE_DOCUMENT` 只给 content URI；写到应用目录再分享） |
| AppMenu | 不实现 |

### 模拟器验证

Android 36（`system-images;android-36;default;arm64-v8a`，emulator 37.2.13，Apple silicon）上用 `tan run -platform android` 验证过：mobile-hello 启动后 Go 日志依次有 `Started`、`Foreground`，页面加载并完成消息往返（启动时的 hello 与点按钮）；Home 键 → `Background`，再回到应用 → `Foreground`；剪贴板在主线程回调和 goroutine 里读写（含 emoji）正确；`dialog.Message`（goroutine 中调用）显示 3 个按钮，点击返回对应下标，返回键取消返回 `ErrCanceled`；返回键先在 WebView 历史里后退，没有历史时关闭页面，随后 `Background`、`Terminating`、`main` 返回、进程退出。

第四轮修补的验证（同一 AVD，`examples/mobile-hello` 的 caps.html；通过 `adb forward tcp:9222 localabstract:webview_devtools_remote_<pid>` 用 DevTools 协议执行页面脚本，截图在 `build/verify/android-*.png`）：

- **POST 请求体**：`window.__tangoRunHTTP()` 的 10 项全部 PASS——fetch 文本、JSON、FormData（字段 + 70 000 字节文件，Go 端 SHA-256 一致）、Blob（类型保留）、PUT ArrayBuffer、XHR 文本、XHR FormData、URLSearchParams；Go 日志可见对应 method/Content-Type/大小。单测：`body_test.go`（暂存、过期、307 带 body、Set-Cookie 扁平化，以及用 Node 实跑 `bodyScript` 检查 multipart 边界与空 body）。
- **多个 Set-Cookie**：`/api/cookies` 发两个 `Set-Cookie`（一个 HttpOnly）和两个 `X-Tango-Multi`：页面看到 `X-Tango-Multi: one, two`、`document.cookie` 只有 `tango_a`；随后 `/api/whoami` 的 Go handler 收到 `tango_a` 与 `tango_b`。
- **Activity 重建**：页面计数器点到 5 → 开发者选项打开“不保留活动”（`settings put global always_finish_activities 1` 不会通知 ActivityManager，需在设置界面切换一次）→ Home（事件日志 `wm_on_destroy_called`）→ 回到应用（`wm_on_create_called`）：进程号不变、计数器仍为 5、页面加载时间不变，Go 只收到 Background/Foreground，没有 Terminating。旋转（`user_rotation 1`）与深色模式（`cmd uimode night yes`）按 `configChanges` 就地处理，无重建，状态不变。测试后关闭“不保留活动”。
- **后台任务**：`Schedule` 后立即运行一次；`Earliest` 1 小时的请求在应用进程被 `am kill` 后用 `cmd jobscheduler run -f dev.tango.mobilehello 1` 触发：系统为 `TangoJobService` 启动进程，Go 启动、任务执行 2 秒后 `jobFinished`；随后打开应用，后台期间建好的页面挂进 Activity 并显示任务事件。
- 设备控制：常亮、亮度（0.40 → 0.20 → 交还系统）、红色状态栏 + 浅色图标、键盘弹出事件 343.2 dp、`HideKeyboard` 后事件 0；小组件 `Update` 成功、`dumpsys appwidget` 列出 `TangoWidget` 提供者（未在桌面上添加实例截图）。

### 已知限制

- 未在真机上运行过（模拟器结果见 Android 节末尾的"模拟器验证"）。
- 单 Activity、单任务；多窗口模式下仍是一个页面栈。
- `log` 包与 `fmt.Print*` 的输出经管道转到 logcat（`Tango` 标签），按行。

### 契约建议（Android）

1. `ServeAsset` 已改为可在任意线程调用（第二轮），Android 依赖这一点；建议在注释里写明“handler 可能并发执行，后端可在 ServeAsset 返回前流式交付已写入的数据”。
2. ~~`Backend.Step` 无法嵌套泵事件~~：已加可选接口 `platform.NestedLoop`，见上文（契约第三轮）。
3. `Host.OnMessage` 的 origin：Android 无法得知发消息的 frame，目前靠 token 保证只有主框架能发；建议契约注明“后端须保证消息来自主框架”，不限定实现手段。


## 系统能力（sys/...）

契约在 `internal/platform/mobile_ext.go`（全部是可选接口），公开包在 `sys/` 下：`share`、`haptics`、`safearea`、`orientation`、`biometric`、`securestore`、`media`（相册选择 + 拍照）、`location`、`push`、`permission`。用法与其他能力包一致：第一个参数是 `*tango.App`，不支持返回 `ErrUnsupported`，应用退出后返回 `tango.ErrNotRunning`；需要等用户操作的调用会阻塞（主线程上调用时 iOS 会泵事件；**Android 上请在 goroutine 里调用**，主线程上会直接返回 `tango.ErrNestedWait`，原因见上文 Step）。需要的 Info.plist 键与清单权限见 `mobile-permissions.md`；`examples/mobile-hello` 的 caps.html 是演示页。

新增应用事件（`AppEventKind` ≥ 100，根包常量见 `mobile_events.go`），数据放在 `AppEvent` 的专用字段（契约第三轮，不再借用 URL/Paths）：`EventSafeAreaChanged`（`Insets`）、`EventOrientationChanged`（`Orientation`）、`EventPushToken`（`Push.Token`）、`EventPushReceived` / `EventPushOpened`（`Push.Payload`，即 `platform.PushPayload`，含通知 `ID`）、`EventWebViewRestored`（`Surface` + `URL`；用 `App.OnWebViewRestored`）。原生宿主仍把推送内容以 JSON 字符串交给后端，后端用 `platform.ParsePushPayload` 解析。只关心一种事件时用 `App.OnEventKind`，它不会吞掉其它种类的待投递事件。冷启动时（Started 之前）到达的链接与推送由后端排队，在 Started 之后投递。

| 能力 | iOS | Android | macOS |
|---|---|---|---|
| share | `UIActivityViewController`（iPad 用 popover）；关掉面板 → `ErrCanceled` | `ACTION_SEND(_MULTIPLE)` 选择器，文件经宿主 `TangoFileProvider` 给 content URI；弹出即返回 | `NSSharingServicePicker` |
| haptics | `UIImpact/Selection/NotificationFeedbackGenerator`（17.5+ 绑定窗口） | `View.performHapticFeedback`，按 API 等级映射 | 不支持 |
| safearea | 窗口 `safeAreaInsets`；窗口里垫一个透明跟踪视图，旋转/状态栏变化时发事件 | WebView 被系统栏/刘海/输入法遮住的部分（宿主已留边，通常为 0） | 不支持 |
| orientation | `effectiveGeometry.interfaceOrientation`；锁定 = 根控制器与页面的 `supportedInterfaceOrientations` + `requestGeometryUpdate` | `Display` 旋转；`setRequestedOrientation` | 不支持 |
| biometric | `LAContext`（缺 `NSFaceIDUsageDescription` → `ErrNotConfigured`） | `BiometricPrompt`（28+） | `LAContext` |
| securestore | Keychain 通用密码（本机、首次解锁后可用） | Keystore AES-GCM + 私有 SharedPreferences | Keychain |
| media | `PHPickerViewController`（不需相册权限）；`UIImagePickerController` 拍照；结果复制到 `Library/Caches/tango-media` | Photo Picker（33+）/ `GET_CONTENT`；`ACTION_IMAGE_CAPTURE`；`cache/tango-media` | 不支持（用 `dialog`） |
| location | `CLLocationManager`（`requestLocation` / `startUpdatingLocation`），未询问时先请求 when-in-use | `LocationManager`，不依赖 Play 服务 | 不支持 |
| push | APNs：请求通知授权后 `registerForRemoteNotifications`；`UNUserNotificationCenter` 代理（宿主未设置时）报告前台收到与点击 | 不含 Firebase：`TangoPush` 静态方法供应用自己的 FCM 服务调用，未接入返回 `ErrUnsupported` | 不支持 |
| permission | AVCaptureDevice / PHPhotoLibrary / CLLocationManager / UNUserNotificationCenter | `checkSelfPermission` / `requestPermissions` + “问过”标记 | 不支持 |
| 链接 | 场景代理 `openURLContexts` / `continueUserActivity`（含冷启动 connectionOptions）；无场景时运行时给 AppDelegate 补方法 | `onCreate` / `onNewIntent` 的 `ACTION_VIEW` | 已有 |
| WebView 恢复 | `webViewWebContentProcessDidTerminate` → 先报 Navigating，再重新加载并发 `EventWebViewRestored` | `onRenderProcessGone` → 重建 WebView | — |

iOS 实现在 `internal/platform/ios/mobile.{go,m,h}`（`bridge_private.h` 是 bridge.m 与 mobile.m 共用的 ObjC 声明）。宿主的 AppDelegate 已自己实现推送/URL 回调时 Tango 不覆盖，需要在其中调用 `TangoDidRegisterForRemoteNotifications(token, error)`。iOS 桥接修正：页面消息按长度传递，含 NUL 的核心内部消息（`"\x00tango-int:"`）此前在 iOS 上被截断成空串。

## 系统能力（第四轮）：后台任务、应用内购买、设备控制、小组件

契约在 `internal/platform/mobile_ext.go` 末尾（可选接口），公开包：`sys/background`、`sys/purchase`、`sys/device`、`sys/widget`；fake 后端在 `internal/platform/fake/mobile2.go`。新增应用事件（接续 100–105）：`AppKeyboardChanged`（106，`Insets.Bottom` = 键盘覆盖高度，CSS 像素，0 = 收起）、`AppBackgroundTask`（107）、`AppPurchaseUpdated`（108）。后两个是无负载的信号：`AppEvent` 没有对应字段，监听者在主线程上用 `TakeBackgroundTasks` / `TakePurchaseUpdates` 取走后端排队的更新（更新留在后端队列里，直到有人取：`OnTask` / `OnUpdate` 注册时会先取一次，所以后台启动、冷启动时、监听注册前到达的任务和交易不会丢）。

| 能力 | iOS | Android |
|---|---|---|
| background | `BGTaskScheduler`：`<id>.tango.refresh`（`BGAppRefreshTask`）、`<id>.tango.processing`（`BGProcessingTask`） | `JobScheduler`（不用 WorkManager，无 androidx）：job id 1 = refresh、2 = processing，`TangoJobService`；有 `RECEIVE_BOOT_COMPLETED` 时 `setPersisted` |
| purchase | StoreKit（ObjC 可用的 `SKProductsRequest` / `SKPaymentQueue`） | 不支持：`ErrUnsupported`（见下） |
| device.RequestReview | `SKStoreReviewController`（是否弹出由系统决定） | 打开商店页：`market://details?id=…`，无商店时 `https://play.google.com/store/apps/details?id=…` |
| device.OpenSettings | `UIApplicationOpenSettingsURLString` | `ACTION_APPLICATION_DETAILS_SETTINGS` |
| device.KeepScreenOn | `idleTimerDisabled` | 窗口 `FLAG_KEEP_SCREEN_ON` |
| device.Brightness | 屏幕 `brightness` | 读：窗口覆盖值，未设置时系统设置 `SCREEN_BRIGHTNESS/255`；写：窗口 `screenBrightness`（只在应用在前台时生效，`ResetBrightness` 交还系统） |
| device.SetStatusBar | 页面控制器的 `preferredStatusBarStyle` / `prefersStatusBarHidden`；`Color` → `ErrUnsupported`（页面自己画在状态栏下面） | `WindowInsetsController` 外观与隐藏（滑动临时显示）；`Color` 画在根布局为状态栏留出的顶部条上（Android 15 起 `setStatusBarColor` 无效） |
| device 键盘 | `UIKeyboardWillChangeFrame` / `WillHide` 通知；`HideKeyboard` = `endEditing:` | 根布局的 IME insets（API 30+，更低版本报 0）；`HideKeyboard` = `WindowInsetsController.hide(ime())` + `InputMethodManager` |
| widget | 不支持（`ErrUnsupported`，见 `docs/specs/mobile-extensions.md`） | `TangoWidget`（AppWidget）：标题 + 文本，存在 SharedPreferences 里，应用没运行也显示，点击打开应用 |

窗口状态（常亮、亮度、状态栏）由宿主保存，Activity 重建后重新应用。

iOS 补充：StoreKit 首次使用时才加载（见冷启动），交易观察者随之安装；`purchase.OnUpdate` 注册后会在应用启动时调用一次后端，使上次运行遗留的交易能送达。模拟器上 `BGTaskScheduler` 不可用（`Schedule` 返回 `ErrNotAvailable`）；商品必须在 App Store Connect 或 Xcode 的 StoreKit 配置里存在，否则为 invalid；模拟器亮度固定。

### 后台任务

```go
background.OnTask(app, func(ctx context.Context, t background.Task) error { return refresh(ctx) }) // main 里、Run 之前
background.Schedule(app, background.Request{Kind: background.Refresh, Earliest: 15 * time.Minute})
```

- tan.json `permissions` 加 `"background"`：iOS 写入 `UIBackgroundModes`（`fetch`、`processing`）与 `BGTaskSchedulerPermittedIdentifiers`（`<id>.tango.refresh`、`<id>.tango.processing`）；Android 加 `RECEIVE_BOOT_COMPLETED`（`TangoJobService` 总在模板清单里声明，`BIND_JOB_SERVICE` 保护）。
- 系统可能只为跑任务而启动进程：iOS 在后台启动应用（没有场景），Android 由 `TangoJobService.onStartJob` 调 `Tango.start` 启动 Go。此时 `main`、`App.Run`、`OnReady` 照常执行，`NewSurface` 创建的页面不显示（Android 建在 Application 上下文里，Activity 出现后挂上）。
- 任务函数在独立 goroutine 上执行，ctx 在系统要求停止（iOS `expirationHandler`、Android `onStopJob`）或截止时间（iOS 约 30 秒 / 数分钟，Android 报告 9 分钟）时取消；返回后 `FinishBackgroundTask`（iOS `setTaskCompletedWithSuccess:`，Android `jobFinished(params, false)`）。请求是一次性的，下一次在任务里重新 `Schedule`。
- 调试：Android `adb shell cmd jobscheduler run -f <applicationId> 1`；iOS 模拟器见 `sys/background` 包注释（lldb `_simulateLaunchForTaskWithIdentifier:`）。

### 应用内购买（Android 接入 Google Play Billing）

Google Play Billing 只能通过 Play Billing Library（`com.android.billingclient:billing`，依赖 Google Play 服务）使用，没有平台 API；宿主坚持零第三方依赖、离线可构建，所以 Android 后端不实现 `platform.Purchases`，`sys/purchase` 全部返回 `ErrUnsupported`（`CanMakePayments` 为 false）。需要时的接入方式：

1. 在 `mobile/android/app/build.gradle` 加 `implementation "com.android.billingclient:billing:<版本>"`（需要联网解析一次依赖），清单里由库自动合并 `com.android.vending.BILLING` 权限。
2. 在宿主里新增一个类（例如 `TangoBilling`）：`BillingClient.newBuilder(ctx).setListener(purchasesUpdatedListener).enablePendingPurchases(...)`，连接后实现 `queryProductDetailsAsync`（→ `Product`）、`launchBillingFlow`（需要 Activity，→ `Purchase` 的 done）、`queryPurchasesAsync`（→ 恢复）、`acknowledgePurchase` / `consumeAsync`（→ `FinishTransaction`）。
3. 在 `Mobile.call` / `Device.call` 里加对应 op，用 `Mobile.event(108, …)` 发 `AppPurchaseUpdated`；Go 侧在 `internal/platform/android` 里按 `mobile2_android.go` 的方式实现 `platform.Purchases`（`TakePurchaseUpdates` 取 Java 侧队列）。`Transaction.ID` 用 `orderId`，`purchaseToken` 交给服务器校验（可放进 `PurchaseReceipt` 或扩展字段）。

### 推送（Android）

现状不变：`push.Register` 在 Android 上返回 `ErrUnsupported`，除非应用自己接入 Firebase（见 `mobile-permissions.md` 与 `TangoPush`）。评估过不依赖 Firebase 的 [UnifiedPush](https://unifiedpush.org)：它的分发器（ntfy、NextPush 等）是用户另装的应用，应用通过广播（`org.unifiedpush.android.connector.*` 的 `REGISTER` / `NEW_ENDPOINT` / `MESSAGE`）与分发器交换消息，协议本身不需要库，可以用平台 API 写一个 `BroadcastReceiver` 实现；但它要求用户装分发器、服务器按 WebPush/HTTP 往 endpoint 推，在国内与 Google Play 场景下覆盖率都很低，与 `push.Register` "返回令牌交给服务器" 的语义也不同（返回的是 endpoint URL）。因此暂不内置；需要时建议作为单独的 `TangoUnifiedPush` 接收器加到宿主，令牌位置放 endpoint，`AppPushReceived` 复用现有事件。

### 小组件与扩展

Android：tan.json `permissions` 加 `"widget"`，tan 在清单里声明 `org.tango.host.TangoWidget`（`APPWIDGET_UPDATE` + `@xml/tango_widget_info`；布局 `res/layout/tango_widget.xml` 在宿主模板里），`widget.Update(app, widget.Content{Title, Text})` 写入并刷新所有实例；没声明时返回 `ErrNotConfigured`。iOS 小组件（WidgetKit/SwiftUI）与分享扩展的成本和设计见 `docs/specs/mobile-extensions.md`。
