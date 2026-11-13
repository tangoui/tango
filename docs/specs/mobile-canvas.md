# 移动端自绘 UI（iOS / Android Canvas）

状态：设计稿写于 2026-11-13；**M1–M5 已实现**（进度见下表），两端都还没上真机。下文第 1–7 节保留设计时的调研与方案，其中"现状"一类描述指设计当时。

**实施进度**（2026-11-13 按代码核对；M5 一行按提交 `83f84b2` 更新）

| 里程碑 | 状态 | 代码证据 | 验证 |
|---|---|---|---|
| M1 iOS 上屏、触摸、安全区 | ✅ 已实现 | `internal/platform/ios/canvas.go`（`platform.Canvas`、`CanvasMetal`、`CanvasInsets`）、`canvas.m`（`TGCanvasPage/View`、CADisplayLink） | 本文未记录模拟器结果（Android 的见右侧） |
| M2 Android CPU 上屏、触摸、insets | ✅ 已实现 | `internal/platform/android/canvas_android.{go,c}`、`mobile/android/.../CanvasPage.java`、`CanvasView.java` | AVD API 36 模拟器，结果见 [../mobile.md](../mobile.md)"自绘页"一节 |
| M3 两端输入法与键盘 | ✅ 已实现（M3a 与 M3b） | iOS：`ios/canvas_text.{go,m}`（UITextInput）、`canvas_textdoc.go`；Android：`android/canvas_ime.go`、`canvas_ime_android.go`、`CanvasInput.java`；键盘 inset 两端都报给 `uihost` | iOS 有模拟器自检 `TANGO_CANVAS_IME_SELFTEST`（`ios/canvas_selftest.go`）；本文未记录人工输入法验收结果 |
| M4 两端无障碍 | ✅ 已实现 | iOS：`ios/canvas_a11y.{go,m}`、`a11ybridge/ios.go`；Android：`android/canvas_a11y.go`、`canvas_a11y_android.go`、`CanvasA11y.java`、`a11ybridge/android.go` | 单测；本文未记录 VoiceOver / TalkBack 实测结果 |
| M5 Android GPU、WebView 叠放、收尾 | ✅ 已实现（大部分） | Android GPU：`ui/gpu/gl/presenter_android.go`、`egl_android.go`（OpenGL ES 3.0 + EGL，与 Linux GL 共用设备），`uihost/gpu_android.go` 经 `platform.CanvasNativeWindow` 接入，上下文丢失后重建，失败或软件光栅化时回退 CPU，`debug.tango.renderer` 属性切换；叠放（`CanvasAttacher`）：iOS `ios/canvas_attach.{go,m}`（子视图），Android `android/canvas_attach_android.go` + `CanvasOverlay.java`（TextureView），两端叠放层只走 CPU；返回键 `ui/back.go`（`WantBack`/`ClaimBack`）与 `uihost/back.go`；内存警告 `uihost/memory.go`（`AppMemoryWarning` 后裁剪图集、纹理和排版缓存）；iPad 指针与触控板 `ios/canvas_pointer.{go,m}`。未做：黄金图比对进 CI；独立的 `ResetDevice` 接口（Android 呈现器自己重建上下文） | 单测（`presenter_android_test.go`、`canvas_attach_plan_test.go` 等）；本文未记录模拟器或真机结果 |

真机：两端都还没有在真机上跑过自绘页（[../mobile.md](../mobile.md) 两端"已知限制"）。

相关文档：[canvas-surface.md](canvas-surface.md)（C 步即本文）、[touch-input.md](touch-input.md)、[gpu-renderer.md](gpu-renderer.md)、[a11y-bridge.md](a11y-bridge.md)、[../accessibility.md](../accessibility.md)、[../mobile.md](../mobile.md)。

## 0. 摘要

- **目标**：让 `ui` / `ui/mobile` 的自绘界面经 `uihost.Open` 在 iOS 与 Android 上全屏显示。两种混排都要支持：一是自绘页和 WebView 页同在一个页面栈里，二是自绘层叠放在 WebView 页里面。此外要有多点触控、软键盘与输入法、安全区、系统设置跟随和无障碍。
- **契约尽量不改**：后端照常实现已有的 `platform.CanvasCreator` / `CanvasAttacher` / `CanvasAccessible` / `CanvasMetal`（`internal/platform/canvas.go:18-305`）。新增内容只有可选接口，放在新文件 `internal/platform/canvas_mobile.go`。
- **iOS**：Objective-C 写的 `TGCanvasView`，每帧由 `CADisplayLink` 驱动。GPU 路径直接复用 `ui/gpu/metal`：它的构建标签是 `darwin`，GOOS=ios 也满足。CPU 回退用 `CALayer.contents`。文字复用 CoreText 引擎，需要去掉 `!ios` 标签。
- **Android**：Java 宿主新增 `CanvasPage`，整页用 SurfaceView，叠放层用 TextureView，两者都经 JNI 取得 `ANativeWindow`。第一阶段用 CPU 光栅化，再 `ANativeWindow_lock/unlockAndPost` 上屏。第二阶段推荐 **OpenGL ES 3.0 + EGL**，不选 Vulkan，理由见 1.3 节。文字走纯 Go 路径，字体由解析 `fonts.xml` 发现，不打包 FreeType。
- **线程**：所有 canvas 回调都在平台主线程（UI 线程）上执行，与桌面一致，`uihost.go:6` 写明"UI 线程 = 主线程"。`Invalidate` 可以从任意 goroutine 调用，只置原子标志并唤醒主线程。
- **里程碑**：M1 iOS 上屏 + 触摸 + 安全区；M2 Android 同样内容；M3 两端输入法与键盘避让；M4 两端无障碍；M5 Android GPU、WebView 叠放、收尾。总工作量约 19–24 人周（见第 8 节）。

---

## 1. 上屏与呈现

### 1.1 现状（调研结论）

| 事实 | 出处 |
|---|---|
| `uihost.Open` 经 `corehook.NewCanvasSurface` 进根包。根包断言 `a.b.(platform.CanvasCreator)`，不支持时返回 `ErrUnsupported` | `uihost/uihost.go:65-104`、`canvas_hook.go:40` |
| （设计时）iOS、Android 后端都还没实现 `CanvasCreator`，所以移动端 `uihost.Open` 返回 `ErrUnsupported`。**现已实现**：`ios/canvas.go`、`android/canvas_android.go` | 设计时：`internal/platform/ios/*`、`internal/platform/android/*`（无 canvas 文件） |
| `uihost/gpu_darwin.go` 的标签是 `darwin`，GOOS=ios 也会编译它，它断言 `CanvasMetal` 后调 `metal.NewPresenter` | `uihost/gpu_darwin.go:1-12`、`uihost/gpu.go:50-57` |
| `ui/gpu/metal` 的标签是 `darwin`，用 purego dlopen 加载 Metal / QuartzCore / CoreGraphics，不依赖 AppKit | `ui/gpu/metal/objc_darwin.go:96-115`、`presenter_darwin.go:111-121` |
| Metal 纹理全部是 Private，缓冲区是 Shared，没有用 macOS 才有的 Managed，所以在 iOS 与 Apple Silicon 模拟器上可用 | `ui/gpu/metal/metal_darwin.go:366,882,932,1175` |
| purego 在 GOOS=ios 下**要求 cgo**（`CGO_ENABLED=0` 时故意编译失败）。iOS 产物本来就是 cgo 的 c-archive，所以满足条件 | `~/go/pkg/mod/github.com/ebitengine/purego@v0.11.1/is_ios.go`、`internal/platform/ios/backend.go:16-22` |
| CPU 光栅化的目标尺寸是 `ceil(Size*Scale)`，Metal drawable 的尺寸是 `round(Size*Scale)`，两者最多差 1 px | `ui/paint/cpu.go:90-91`、`presenter_darwin.go:33-36` |
| Runner 在 UI 线程上依次做 build、layout、paint、Render、Present，CPU 路径带脏矩形时调 `DamagePresenter` | `ui/runner.go:145-195`、`ui/window.go:46-59` |
| iOS 每个页面是一个 `TGPage`（UIViewController），`syncStack` 只认 `TGPage` 类 | `internal/platform/ios/bridge.m:129-171`、`bridge_private.h:9-13` |
| Android 页面栈是 `ArrayList<Page>`，`Page.web` 直接加进根 FrameLayout。根布局按 systemBars、cutout、ime 三类 inset **整体加内边距** | `TangoActivity.java:37,169-176,225-238` |

### 1.2 iOS

#### 1.2.1 视图结构

```
TGCanvasPage : TGPage            (新增，web == nil；进入 bridge.m 现有页面栈与 syncStack)
 └─ view = TGCanvasView : UIView <UITextInput>
      layer (CALayer)            CPU 路径：contents = CGImage
      ├─ metalLayer (CAMetalLayer, 子层, frame = bounds)   UseMetalLayer 之后
      └─ cpuLayer  (CALayer, 子层, 在 metalLayer 之上)     GPU 模式下的 CPU 回退帧，Present(nil) 时隐藏
```

- **为什么用子层**：macOS 实现会替换 view 的 backing layer（`canvas.go:148-180` 的语义）。UIView 的 `+layerClass` 只能在创建时决定，之后换不了。所以 iOS 版用子层实现同样的语义：layer 归 view 所有，`contentsScale` 由平台维护，`Present(nil)` 表示 GPU 已呈现，`Present(img)` 显示在 drawable 之上。契约文字不需要改。
- **复用页面栈**：`TGCanvasPage` 继承 `TGPage`，于是 `syncStack` 的 `isKindOfClass:TGPage.class` 判断、模态呈现、Hide/Close 全部可以直接用。`TGPage` 里给 `self.web` 发的消息在 web 为 nil 时是空操作。只需要在 `bridge_private.h` 与 `bridge.m` 里加一个函数 `void tg_page_register(TGPage *p)`，用来把页面放进 static 的 `pages` 字典（`bridge.m:74`）。这是唯一要动的现有 iOS 文件，改动是纯追加。
- **Scale**：`Scale()` 取 `traitCollection.displayScale`（2 或 3），尺寸是 view.bounds，单位为点。以后可以考虑 `nativeScale`，本期不做。

#### 1.2.2 帧调度（照搬 darwin 的状态机）

macOS 版的规则写在 `internal/platform/darwin/canvas.go:10-31`：

1. `Invalidate()` 可从任意 goroutine 调用：先 CAS 置 `pending`，只在从空闲变为待处理时才调 C 函数 `tg_canvas_signal()`。这个函数通知一个加在 main run loop common modes 上的 `CFRunLoopSource`，是线程安全的 CF 调用。之所以不用 `dispatch_async(main)`，是因为主队列在嵌套泵事件时会死锁（`docs/mobile.md:130`）。
2. 这个 source 的回调在主线程上恢复对应 canvas 的 `CADisplayLink`（`paused = NO`）。
3. 每个 tick 调 Go 的 `tangoIOSCanvasFrame(handle)`，依次经过 `sink.CanvasFrame()`、`Runner.Frame()`，最后 `Present`。连续 4 个 tick 没有请求时暂停 display link，静止页面不出帧。
4. `InvalidateAt(t)`：每个 canvas 一个可重设的 `CFRunLoopTimer`，到期后走第 2 步。
5. 高刷新率：通过 `preferredFrameRateRange` 请求 (80, 120, 120)，还需要在 Info.plist 加 `CADisableMinimumFrameDurationOnPhone = YES`（见第 6 节）。最低 iOS 17（`docs/mobile.md:171`），这些 API 都可以用。

这套调度逻辑写成**纯 Go 状态机** `internal/platform/mobilecanvas/sched.go`（不加构建标签）：输入是请求、tick、定时器、前后台事件，输出是"恢复或暂停 link、是否出帧、定时器重设到何时"。两端共用，在宿主机上单测。

#### 1.2.3 GPU 与 CPU 路径

- **GPU（默认）**：`uihost.setupGPU` 断言 `CanvasMetal` 并调 `UseMetalLayer()`，iOS 版创建 CAMetalLayer 子层并返回它。之后 `metal.NewPresenter(layer)` 设置 device、pixelFormat、framebufferOnly 和 colorspace（`presenter_darwin.go:206-230`）。错误处理沿用桌面规则（`canvas-surface.md:166-170`）：`ErrTargetTooLarge` 时该帧临时用 CPU，`ErrDeviceLost` 时永久改用 CPU。
  - **待确认**：`presenter_darwin.go:225` 的 `setColorspace:` 在 iOS 的 CAMetalLayer 上是否可用。如果不可用，加 `respondsToSelector:` 保护（只改一行）。
  - dlopen 路径都是绝对路径 `/System/Library/Frameworks/...`。模拟器里 dyld 会映射到 runtime root，推断可用，要在 M1 第 0 步实测。
- **CPU 回退**：先用 `CGDataProviderCreateWithData` 复制像素，再 `CGImageCreate`（premultipliedLast + 32Big），设给 `layer.contents`，期间关闭隐式动画，做法与 macOS A 步（`canvas-surface.md:92`）相同。以 iPhone 15 Pro 为例，1179×2556×4 ≈ 12 MB/帧，可用但费电，所以只作回退。
- **脏矩形**：GPU 路径的离屏目标本来就保留上一帧，不需要额外处理。CPU 路径如果要实现 `PresentDamage`，需要宿主自己保留一份上一帧 RGBA 拷贝。平台侧的 `PresentDamage` 由另一会话设计（`canvas-surface.md:209-222`），iOS 只在 CPU 回退时实现，优先级低。

#### 1.2.4 尺寸、旋转、生命周期、内存

- 在 `layoutSubviews` 里检测 bounds 或 scale 变化：先更新 metalLayer 的 frame 与 `contentsScale`，再调 `sink.CanvasResized`，然后**同步**出一帧，避免旋转过程中出现拉伸的旧帧。顺序与 `canvas.go:162-167` 的约定一致。
- 旋转：`viewWillTransitionToSize:` 不需要特别处理，交给 layoutSubviews。安全区与键盘的变化另由 `safeAreaInsetsDidChange` 和键盘通知触发 Invalidate。
- **后台**：收到 `UIApplicationDidEnterBackgroundNotification`（`bridge.h:14-19` 已有对应的生命周期码）后暂停 display link，标记 `suspended`，期间**绝不调 `CanvasFrame`**。iOS 会终止在后台提交 GPU 命令的进程，所以这一点是硬性规则。回到前台时如有挂起的请求就补一帧。
- **内存警告**：`UIApplicationDidReceiveMemoryWarningNotification` 时由后端发一个新的 AppEvent `AppMemoryWarning`（见 3.4），uihost 收到后让 GPU 渲染器和文字缓存释放可重建的资源（新增的 `Trim` 能力，见 3.4）。

#### 1.2.5 输入路由

见第 2 节。触摸由 `touchesBegan/Moved/Ended/Cancelled` 交给 Go，`multipleTouchEnabled = YES`。

### 1.3 Android

#### 1.3.1 宿主结构（Java，`mobile/android/.../org/tango/host/`）

| 新增 / 改动 | 作用 |
|---|---|
| `PageBase.java`（新增抽象类） | 从 `Page` 中抽出 `id / title / visible / closed / View view()`。`TangoActivity` 的栈改成 `ArrayList<PageBase>` |
| `Page.java`（改动） | `extends PageBase`，`view()` 返回 web，行为不变 |
| `CanvasPage.java`（新增） | 一个自绘页面：持有 `CanvasView`，show/hide/focus/close 与 `Page` 一致，事件走 `Tango.nativeSurfaceEvent` |
| `CanvasView.java`（新增） | `FrameLayout` 包一层 `SurfaceView`（整页）或 `TextureView`（叠放层）。实现 `SurfaceHolder.Callback` / `TextureView.SurfaceTextureListener`，转发触摸、键盘、焦点、悬停，用 `Choreographer` 驱动帧 |
| `CanvasInput.java`（M3） | `BaseInputConnection` 子类 |
| `CanvasA11y.java`（M4） | `AccessibilityNodeProvider` |
| `TangoCanvas.java`（新增） | JNI 边界：`native` 方法由 Go 导出（`Java_org_tango_host_TangoCanvas_*`），静态方法供 Go 调用 |
| `TangoActivity.java`（改动） | ① 栈改用 PageBase；② **inset 改为按子页面处理**：根布局不再 `setPadding`（`:230`），监听器保存最近的 `WindowInsets` 并分发：`Page` 给自己的 WebView 设相同数值的 margin（WebView 页的视觉行为不变），`CanvasPage` 保持铺满，把 systemBars+cutout 作为安全区、把 ime 作为键盘 inset 报给 Go；③ 被上层不透明页面完全遮住的 `CanvasPage`，把其 SurfaceView 设为 `INVISIBLE`（见下文 z 序） |

Go 侧调用 Java 的方法 ID 仍然要在 `nativeInit` 里一次查好（`docs/mobile.md:296`）。为此在 `exports_android.go:53-62` 的三个 lookup 之后加一行 `lookupCanvas(e)`，这是唯一要改的现有 Go 文件，改动是纯追加。

#### 1.3.2 SurfaceView 还是 TextureView

| | SurfaceView | TextureView |
|---|---|---|
| 合成 | 独立 Surface，由 SurfaceFlinger 直接合成，延迟最低，省电 | 进入视图树的 GPU 合成，多一次拷贝和一帧延迟 |
| z 序 / 透明 | 默认在窗口之下"打洞"，透明叠放要用 `setZOrderMediaOverlay`；**两个同类 SurfaceView 的相对 z 序不保证** | 与普通 View 一样，支持任意 z 序和 alpha |
| ANativeWindow | `ANativeWindow_fromSurface(holder.getSurface())` | `new Surface(surfaceTexture)` 后同样调用 |

**决定**：整页（`uihost.Open`）用 SurfaceView，叠放层（`uihost.Attach`，M5）用 TextureView。两者都产出 `ANativeWindow`，Go 和 C 代码完全相同。

同类 SurfaceView 的 z 序问题，靠"只有最上面一个可见的 canvas 页持有 Surface"规避：被完全遮住的页设 INVISIBLE，Surface 随之销毁，也顺带省了内存。Android 页面栈没有转场动画（`addView` 立即生效），所以这条规则不会造成闪烁。

#### 1.3.3 第一阶段：CPU 上屏（`canvas_android.c`，`#cgo LDFLAGS: -landroid`）

- `surfaceCreated` 或 `surfaceChanged`（主线程）时：`ANativeWindow_fromSurface`，然后 `ANativeWindow_setBuffersGeometry(win, wpx, hpx, AHARDWAREBUFFER_FORMAT_R8G8B8X8_UNORM)`。整页不透明，用 RGBX 让合成器跳过混合；TextureView 叠放层用 `R8G8B8A8`（预乘）。Java 侧 `holder.setFormat(PixelFormat.RGBX_8888)`（叠放层用 `TRANSLUCENT`）。
- `Present(img)`：`ANativeWindow_lock(win, &buf, &dirty)`，然后**逐行拷贝**（`buf.stride` 以像素计，可能大于宽度），宽高都取 img 与 buffer 的较小值。img 比 buffer 小（ceil/round 差 1 px）时用最后一行或一列补齐，规则与 Metal 呈现器相同（`presenter_darwin.go:33-36`）。最后 `unlockAndPost`。lock 之后检查 `buf.format`：如果不是 RGBA 或 RGBX（某些模拟器镜像可能给 RGB_565），走转换慢路径并打一次日志。
- **脏矩形**：`ANativeWindow_lock` 的 `inOutDirtyBounds` 支持局部更新。系统会把上一帧的未脏区域拷回（copy-back），并可能**扩大**脏区，所以必须以返回的边界为准，从整帧 img 中拷贝该区域。这正好对应 `ui.DamagePresenter`（`ui/window.go:57-59`）：多个 dirty 合并成一个外接矩形。平台侧接口名以另一会话的 PresentDamage 契约为准。
- **Surface 失效**：`surfaceDestroyed` 在主线程上同步 `ANativeWindow_release` 并把 window 置空。此后 `Present` 返回 nil（丢帧），标记下一帧整帧重画。`surfaceCreated` 之后依次调 `CanvasResized`、出帧。

#### 1.3.4 帧调度

- 用 Java `Choreographer.getInstance().postFrameCallback`，它在主线程执行，回调里调 `TangoCanvas.nativeFrame(handle, frameTimeNanos)`，再到 `sink.CanvasFrame()`。
- `Invalidate()` 可从任意 goroutine 调用：CAS 置 `pending`，只在从空闲变为待处理时经 JNI 调静态 `TangoCanvas.requestFrame(handle)`（通过 `Handler(main).post`，非 Java 线程用现有的 `withEnv` 临时 attach）。主线程上调用时直接 `postFrameCallback`。
- `InvalidateAt`：Java 侧用 `Handler.postAtTime`，到期后同样 `postFrameCallback`。
- 状态机与 iOS 共用 `mobilecanvas/sched.go`，所以"静止不出帧"与"每个刷新周期最多一帧"在两端语义一致。
- **density**：`Scale()` 取 `DisplayMetrics.density`（可能是 2.625 这类非整数），`Size()` 等于 Surface 像素除以 density（float32）。

#### 1.3.5 第二阶段：GPU（M5）——推荐 OpenGL ES 3.0 + EGL

| | GLES 3.0 + EGL | Vulkan |
|---|---|---|
| 覆盖 | API 18+ 全部设备，模拟器的宿主 GPU 转译稳定 | API 24 起只是"可能有"，1.1 要到 28+，驱动质量参差，模拟器支持有限 |
| 着色器后端 | `ui/shader` 目前只有 `msl/` 和 `hlsl/`（`ls ui/shader`），GLSL 330 / ES 300 在规格里已预留为"同一后端的方言开关"（`gpu-renderer.md:17,108`）。新写一个 GLSL 后端约 1,000 行，可参照 `hlsl.go` 的 1,088 行 | 需要 SPIR-V 二进制输出与结构化控制流约束，工作量大一个量级 |
| 设备层 | 约 2,000–2,500 行，可参照 d3d11 约 3,800 行、Metal 约 2,000 行的规模。与 Linux GL 后端（GPU 里程碑 4）**共享约 90%** | 内存、同步、描述符管理估计超过 5,000 行，不能与桌面共享 |
| 前景 | Android 正把 GLES 实现迁到 ANGLE-on-Vulkan，GLES 接口长期保留 | — |

**结论**：做 `ui/gpu/gl`（包名沿用 gpu-renderer.md:61 的规划），用 purego dlopen `libEGL.so` / `libGLESv3.so`，两者是公开 NDK 库，purego 在 Android 上经 cgo dlopen 可用（`dlfcn_android.go`）。这样 Linux（GLX/EGL 经 GTK）和 Android 用同一套函数表。

GPU 架构说明：`gpu.Renderer` 把 `paint.List` 转成与设备无关的 `Frame`，后端只需实现 `gpu.Device.Execute`（`ui/gpu/frame.go:1-8,223-225`），外加可选的 `Retainer` / `PixelReader`。新后端的成本是**一个着色器方言**、**一个 Device**、**一个呈现器**三件。

- 新的可选接口 `platform.CanvasNativeWindow { UseNativeWindow() (target uintptr, err error) }`。target 是平台持有的稳定句柄（C 结构 `tg_canvas_window*`），**不是** `ANativeWindow*` 本身，因为 Surface 每次前后台切换都会重建。呈现器每次 present 读取当前的 window 指针：window 换了就重建 EGLSurface，EGLContext 保留（纹理不丢）；window 为空就跳过。这样 uihost 的 `presenterFunc(target uintptr)` 形状不用变（`uihost/gpu.go:39-41`）。
- 新增 `uihost/gpu_android.go`（`platformGPUTarget = nativeWindowTarget`、`platformNewPresenter = gl.NewPresenterAndroid`），同时把 `gpu_other.go:1` 的标签改为 `!darwin && !windows && !android`，只改一行。
- EGL 上下文绑在 Java 主线程上。所有 GL 调用都发生在主线程的 cgo 回调里，回调期间 Go 固定在这个 OS 线程上，所以不需要另开 `LockOSThread` 的渲染 goroutine。
- **上下文丢失**（`EGL_CONTEXT_LOST`，部分驱动在后台会发生）：现在的胶水遇到 `ErrDeviceLost` 会永久退回 CPU（`canvas-surface.md:168`），在 Android 上过于保守。建议给 `gpu.Renderer` 追加 `ResetDevice()`（忘掉图集与纹理状态、下一帧整帧重传），由呈现器内部重建上下文后调用。这是 ui/gpu 的追加改动，M5 实现。

### 1.4 自绘与 WebView 混排

**第一层：页面级混排（M1/M2 自带）。** `uihost.Open` 产生的自绘页和 `app.NewSurface` 的 WebView 页同在一个页面栈里，先显示的作根，之后的叠在上面（`docs/mobile.md:93,316`）。iOS 靠 `TGCanvasPage : TGPage` 接入 syncStack，Android 靠 `PageBase`。

**第二层：同页叠放（M5，`CanvasAttacher` → `uihost.Attach`）。**

| | iOS | Android |
|---|---|---|
| 放置 | `TGCanvasOverlay`（`TGCanvasView` 子类）用 `insertSubview:aboveSubview:/belowSubview:` 放在 `TGPage.view` 里 web 的上方或下方；Bounds 与 Anchor 用 `CanvasLayer.AnchoredBounds`（`canvas.go:103-122`）和 autoresizingMask | `Page` 增加一个容器 FrameLayout（容纳 web 与叠放层；`Page.recover` 用 parent 和 index 替换 web，换成容器后照样成立）。叠放层是 `CanvasView(TextureView)`，在容器里 `addView(v, index)` |
| 下层 canvas | `web.opaque = NO` 已经设好（`bridge.m:411`），只需把 `TGPage.view.backgroundColor`（`:182`）改为 clear，页面 CSS 也须透明 | `web.setBackgroundColor(TRANSPARENT)`，容器背景透明 |
| PassThrough | 照搬 darwin 的 `hitTest:`：上一帧 alpha < `HitThreshold()` 时返回 nil（`canvas-surface.md:128,139`） | `CanvasView.dispatchTouchEvent` 在 ACTION_DOWN 时查 alpha 掩码，低于阈值返回 false，FrameLayout 随即把事件交给下层子视图，整个手势都跟着下层 |
| GPU | 叠放层只走 CPU，与桌面规则一致（`gpu-renderer.md:146`、`canvas.go:175-178`） | 同左 |

纯逻辑部分（掩码命中、锚点重算）沿用各平台已有的 `canvas_attach_plan.go` 思路，放在不加标签的文件里单测。

### 1.5 线程模型总览

```
              任意 goroutine                      主线程（UIKit main / Android Looper）
 Invalidate ──► CAS pending ──► 唤醒 ──────────►  调度器（mobilecanvas/sched）
                (CFRunLoopSource / Handler.post)        │ vsync（CADisplayLink / Choreographer）
                                                        ▼
                                                  sink.CanvasFrame ─► Runner.Frame
                                                        │  build → layout → paint → Render
                                                        ▼
                                                  Present：Metal present / ANativeWindow 拷贝 / GLES swap
 触摸、按键、IME、无障碍查询（系统主线程回调）──►  sink.CanvasInput / AccessibilityAction
```

- 所有 sink 调用都在主线程，满足 `canvas.go:16-18`。根包的 `canvasSink` 会在每次回调里 `enterMain`（`canvas_hook.go:77-95`），所以回调中的 `Await` 等主线程快速路径都可用。
- Android 主线程不能嵌套泵消息（`docs/mobile.md:312`），canvas 回调里禁止 `AwaitAsync`。这与 WebView 回调的现有规则一致，写进 ui/mobile 的作者文档。
- **风险**：CPU 路径整帧光栅化在 1080×2400 上可能超过 8 ms，会挤占主线程，导致触摸延迟。缓解顺序：iOS 默认用 GPU；Android M2 先实测，超标再把 `paint.CPU` 的光栅化分条并行（现在完全没有 goroutine 并行，`grep go func ui/paint` 无结果）；最后的选项（不在本期）是把 Render 与 Present 移到渲染 goroutine，SurfaceView 允许在非主线程绘制，但需要拆开 `Runner.Frame` 的同步语义。

---

## 2. 输入

### 2.1 多点触控 → `input.TouchEvent`

约定已经写在 `touch-input.md:28-66`，本节只补实现细节。纯逻辑放在 `mobilecanvas/touch.go`。

| 项 | iOS | Android |
|---|---|---|
| ID | Began 时分配递增 int64，`NSMapTable<UITouch*, NSNumber*>`（弱键）保存，Ended/Cancelled 时删除 | `getPointerId(i)`，直接作为 ID |
| 坐标 | `locationInView:`，单位为点，就是逻辑像素 | `getX(i)/density` |
| 合并 | Moved 时取 `coalescedTouchesForTouch:`，最多补 4 个中间点再逐条送，避免事件过多 | `getHistoricalX/Y`，同样最多 4 个 |
| 压力 | `force / maximumPossibleForce`，后者为 0 时填 0 | `getPressure(i)` 截到 0..1 |
| 时间 | `touch.timestamp`（systemUptime 基准）。第一次事件时记录与 `time.Now()` 的偏移。隐私清单已声明 SystemBootTime 35F9.1（`mobile/ios/PrivacyInfo.xcprivacy:44-47`） | `getEventTime()`（uptimeMillis）换算 |
| 取消 | `touchesCancelled`；失去 active 时对仍按着的手指送 Cancelled，再送 `WindowFocus{false}` | `ACTION_CANCEL`；`onWindowFocusChanged(false)` 同样处理 |
| 鼠标、触控板 | iPad：`UIHoverGestureRecognizer` 产生 `Pointer` Move；触控板滚动用 `UIPanGestureRecognizer`（`allowedScrollTypesMask`）产生 `input.Scroll`（M5） | `TOOL_TYPE_MOUSE` 与 `ACTION_HOVER_*` 产生 `Pointer`，`ACTION_SCROLL` 产生 `Scroll`（`AXIS_VSCROLL` × 行高） |

- 平台**不送**模拟鼠标事件：第一根手指由 Runner 自己合成指针事件（`ui/touch.go:10-23`），`SetTouchPointerEmulation` 保持默认开启（`ui/touch.go:51`）。
- 每条事件调一次 `sink.CanvasInput(ev)`，uihost 会 `HandleEvent` 再 Invalidate（`uihost.go:150-158`）。同一帧的多条事件在下一帧开始时一起处理。
- 长按菜单、选区手柄、放大镜：放到 M5 之后。实现方向是 ui 层组件自己画（与桌面一致），平台只提供 `UIEditMenuInteraction` / `ActionMode` 的系统菜单桥接（可选）。

### 2.2 硬件键盘

- **iOS**：用 `pressesBegan/Ended`，`UIPress.key` 给出 `keyCode`（UIKeyboardHIDUsage）、`modifierFlags` 和 `charactersIgnoringModifiers`。映射表放在 `internal/platform/ios/canvas_keys.go`（不加标签，可在任意 OS 单测），沿用 `darwin/canvas_keys.go:1-5` 的做法。
  - **防止重复投递**：文本输入开启时，第一响应者遵循 UIKeyInput，UIKit 会对 Backspace 和 Return 再调一次 `deleteBackward` / `insertText:@"\n"`。规则是：功能键（方向、Tab、Esc、Home/End、PgUp/Dn、Backspace、Return、Delete）以及带 Cmd/Ctrl 的组合键都在 `pressesBegan` 里自己发 `KeyEvent`，并且**不调 super**；能产生字符的键调 super，交给 UITextInput 走 `insertText`。这张决策表写成纯函数并单测。
- **Android**：`onKeyDown/onKeyUp/onKeyMultiple` 把 `KeyEvent.getKeyCode()` 映射为 `input.Key`，表在 `internal/platform/android/canvas_keys.go`（不加标签）；`getMetaState()` 映射为 Mods。文本输入开启时，能产生字符的键交给 InputConnection（IME 的硬件键盘模式）。
- **返回键（Android）**：M2 自绘页按现有规则处理，`canGoBack` 为假时询问 `ShouldClose` 并关闭页面（`docs/mobile.md:318`）。M5 提议：ui 增加 `Context.WantBack()`（每帧汇总），uihost 窗口加可选接口 `SetBackHandling(on)`，平台据此开关 `OnBackInvokedCallback` 的拦截；拦截时投递 `KeyEscape`，与桌面"Esc 逐层关闭浮层"的语义一致（`docs/accessibility.md` 第 6 节）。

### 2.3 输入法与软键盘

**现状**：ui 只有 `SetTextInput(enabled, caret)` 加 `TextInput`/`Composition` 两个事件（`ui/window.go:35-39`、`ui/input/input.go:480-490`、`ui/context.go:457-461`），**没有文档上下文**（光标前后的文字、选区）。UITextInput 和 InputConnection 都依赖上下文：自动纠错、联想、`deleteSurroundingText`、`setComposingRegion` 都要读它。因此分两步做。

**M3a：无上下文模式（先保证能用）。** 原生侧维护一个**影子文档**，只包含当前的标记（组合）文本，光标在末尾。

| 原生调用 | 生成的事件 |
|---|---|
| iOS `setMarkedText:selectedRange:` / Android `setComposingText` | `Composition{Text, Cursor}` |
| iOS `insertText:` / `unmarkText`；Android `commitText` / `finishComposingText` | `TextInput{Text}` + `Composition{""}` |
| iOS `deleteBackward`（无标记文本时）/ Android `deleteSurroundingText(n,0)` / `sendKeyEvent(KEYCODE_DEL)` | n 次 Backspace 的 `KeyEvent` 按下与抬起 |
| `insertText:@"\n"` / `performEditorAction` | `KeyEnter` 按下与抬起 |

这套转换写成纯 Go 的 `mobilecanvas/ime.go`（`IMEDoc`）：原生侧每次操作经 cgo 或 JNI 同步调一次 Go，Go 更新影子文档、返回事件和新文档快照，原生侧据此更新自己的镜像（`NSString` / `SpannableStringBuilder`）。UITextInput 和 InputConnection 的频繁查询（`textInRange:`、`getTextBeforeCursor`）都由镜像在本地回答，不过 cgo。UTF-16 与 rune 偏移的换算放在 `mobilecanvas/utf16.go`，参照 darwin 的 UTF-16 辅助函数。

**M3b：带上下文（ui 追加 API，旧组件不受影响）。**

```go
// ui/input/textstate.go（新文件）
type TextKind uint8   // Text, Number, Decimal, Phone, Email, URL, Password, Search
type EnterKey uint8   // Default, Done, Go, Next, Search, Send, Newline
type TextHints struct {
    Kind        TextKind
    Enter       EnterKey
    Multiline   bool
    Autocorrect bool  // 默认开；Password 一律关
    Capitalize  uint8 // None, Words, Sentences
}
// TextInputState 描述焦点输入框光标附近的状态（rune 偏移，相对 Text 开头）。
type TextInputState struct {
    Hints              TextHints
    Text               string // 光标前后各最多 256 rune
    Offset             int    // Text[0] 在整个字段里的 rune 位置
    SelStart, SelEnd   int
    CompStart, CompEnd int    // -1 表示没有组合区
}
// TextEdit：IME 替换字段 [Start,End)（字段内绝对 rune 位置）为 Text（自动纠错、deleteSurroundingText、setComposingRegion 之后的提交）。
type TextEdit struct{ Start, End int; Text string }
// SelectionChange：IME 移动光标或选区。
type SelectionChange struct{ Start, End int }
```

- ui：`Context.WantTextInputState(caret, st)`（`WantTextInput` 的扩展，在新文件 `ui/textstate.go`），以及 Window 可选接口 `TextInputStateWindow{ SetTextInputState(input.TextInputState) }`。Runner 在 `afterFrame` 里与 `SetTextInput` 一起比较、按需下发（参照 `ui/runner.go:591-594`）。
- 平台：`platform.CanvasTextInputState{ SetTextInputState(input.TextInputState) }`。uihost 在新文件里给 `*window` 加 `SetTextInputState` 方法并转发（Go 允许在同包其他文件给类型加方法，`uihost.go` 不用改）。
- **降级**：焦点组件没有提供状态时，平台继续按 M3a 处理。提供了状态但组件不认识 `TextEdit` 时，Runner 把它转换成 Backspace 或 Delete 键加 `TextInput`，Runner 侧单测覆盖。先由 `ui` 的 Input、Textarea 与 `ui/mobile` 表单组件接入。

**平台细节**

| | iOS（`canvas_text.m`，`TGCanvasView <UITextInput>`） | Android（`CanvasInput.java`） |
|---|---|---|
| 开启 | `SetTextInput(true)` 后 `becomeFirstResponder`，`canBecomeFirstResponder` 返回 textOn；关闭时 `resignFirstResponder` | `onCheckIsTextEditor` 返回 textOn，`onCreateInputConnection` 返回 `CanvasInput(fullEditor=true)`；显示用 API 30+ 的 `WindowInsetsController.show(ime())`，否则 `InputMethodManager.showSoftInput`；隐藏对应 `hideSoftInputFromWindow` |
| 位置与范围 | 自定义 `TGTextPosition`（NSInteger UTF-16 偏移）、`TGTextRange`；`tokenizer` 用 `UITextInputStringTokenizer` | `getTextBeforeCursor/AfterCursor/getSelectedText` 用镜像回答；`getExtractedText` 返回 null |
| 候选窗与光标框 | `firstRectForRange:`、`caretRectForPosition:` 用 `SetTextInput` 的 caret（view 坐标，单位为点） | `requestCursorUpdates` 之后 `imm.updateCursorAnchorInfo`（插入点框换算成屏幕像素） |
| 键盘类型与回车键 | `keyboardType`、`returnKeyType`、`secureTextEntry`、`autocorrectionType`、`autocapitalizationType` 从 Hints 映射 | `EditorInfo.inputType`（`TYPE_CLASS_*`、`VARIATION_PASSWORD` 等）、`imeOptions`（`IME_ACTION_*`、`IME_FLAG_NO_EXTRACT_UI`）；`performEditorAction` 产生 `KeyEnter` |
| 状态同步 | Go 下发状态后调 `[inputDelegate textWillChange:]/textDidChange:` 与 `selectionWillChange/DidChange` | **必须**调 `imm.updateSelection(view, sel, comp)`；切换字段时 `imm.restartInput(view)` |

映射表（Hints 到 UIKeyboardType / inputType）放在各平台不加标签的 `canvas_plan.go` 里单测。

### 2.4 键盘避让

现在 ui 没有键盘 inset 的概念。规格明确说键盘**不算**安全区（`touch-input.md:87`），所以需要单独的通道。

```go
// ui/keyboard.go（新文件）
type KeyboardInsetWindow interface { KeyboardInset() geom.Insets } // 每帧轮询，与 SafeAreaWindow 相同
func (r *Runner) SetKeyboardInset(in geom.Insets)                  // 宿主推送，优先于轮询
func (c *Context) KeyboardInset() geom.Insets
```

- **自动避让**：键盘 inset 变大或焦点切换时，如果焦点输入框的 caret 落在 `Size.H - KeyboardInset.Bottom - 8` 以下，Runner 对它最近的滚动祖先调用新增的 `c.ScrollIntoViewInset(scrollID, id, inset)`（在 `ui/scroll.go:381` 的 `ScrollIntoView` 基础上把视口底部减去 inset）。`ui/mobile` 的 ScrollView 同样接入。底部弹层（`ui/mobile/overlay.go:415,464`）的底边用 `max(SafeArea.Bottom, KeyboardInset.Bottom)`。
- **平台来源**：
  - iOS：`UIKeyboardWillChangeFrameNotification` 的结束帧换算到 view 坐标，`inset = max(0, bounds.maxY - kb.minY)`。现有的全局通知已经在 `mobile2.m:535` 注册过，canvas 自己按 view 再算一遍，因为模态呈现或分屏时 view 不等于 window。
  - Android：API 30+ 取 `WindowInsets.Type.ime()` 的 bottom 除以 density，经 1.3.1 节的按子页面分发拿到。API 24–29 时 `adjustResize`（`AndroidManifest.xml:24`）会让窗口变小，canvas 尺寸随之缩小，键盘 inset 报 0，作为可接受的降级。
- `platform.CanvasInsets` 一个接口同时提供安全区和键盘 inset（见 3.1）。现有的 `AppKeyboardChanged` 应用事件（`mobile_ext.go:357`）保持不变，供应用自用。

---

## 3. 安全区、系统栏与系统设置

### 3.1 新可选接口（`internal/platform/canvas_mobile.go`）

```go
// CanvasInsets 由移动端 Canvas 实现。uihost 每帧轮询（返回缓存值，必须廉价）。
// 变化时平台调用 Invalidate（或 CanvasResized，如果尺寸也变了）。逻辑像素，主线程。
type CanvasInsets interface {
    SafeAreaInsets() geom.Insets // 状态栏、刘海或挖孔、圆角、Home 指示条、导航栏
    KeyboardInset() geom.Insets  // 软键盘覆盖的部分，通常只有 Bottom；收起时为 0
}
type CanvasTextInputState interface { SetTextInputState(st input.TextInputState) } // M3b
type CanvasNativeWindow  interface { UseNativeWindow() (target uintptr, err error) } // Android GPU，M5
```

uihost 新文件 `uihost/insets.go` 给 `*window` 加 `SafeAreaInsets()` 和 `KeyboardInset()`：canvas 实现了 `CanvasInsets` 就转发，否则返回零值。桌面上返回零值，行为不变（Runner 只在值变化时整帧重画，`ui/safearea.go:47-60`）。

选择轮询而不是由 sink 推送，是因为推送需要在根包 `canvasSink`（`canvas_hook.go:77-131`）里加转发方法，而根包正在被另一会话重组。

### 3.2 来源

- **iOS**：`view.safeAreaInsets`，在 `safeAreaInsetsDidChange` 时更新缓存并 Invalidate。旋转后左右边距会变（`touch-input.md:86`）。自绘页铺满整个视图，包括状态栏区域。状态栏样式沿用 `device.SetStatusBar`（`preferredStatusBarStyle`），`TGCanvasPage` 继承 `TGPage` 后自动生效。
- **Android**：`WindowInsets.getInsets(systemBars() | displayCutout())`（API 30+），像素除以 density。API 24–29 用 `getSystemWindowInset*` 加 `getDisplayCutout()`（28+）。手势导航的 `systemGestures` inset **不**计入安全区，只在 ui/mobile 的边缘滑动组件里有参考价值，留作以后通过 `CanvasInsets` 的扩展方法提供。

### 3.3 深色模式、动态字体、减少动态效果、语言

现有的跟随逻辑在 uihost（`follow.go:189-260`、`motion.go`），它依赖后端能力：`platform.SystemTheme`（`ext.go:228`）、`MotionPreference`（`ext.go:411`）、`Locales` / `LocaleWatcher`（`locale.go:12,94`）。**两个移动后端目前都没有实现**（grep 无结果）。新增：

| 能力 | iOS（`system.go/.m`） | Android（`system_android.go` + `Mobile.java` 的 op） |
|---|---|---|
| SystemTheme + `AppThemeChanged` | `traitCollection.userInterfaceStyle`，在 host view 的 `traitCollectionDidChange:` 时发事件 | `uiMode & UI_MODE_NIGHT_MASK`，在 `onConfigurationChanged` 时发事件（`configChanges` 已包含 uiMode，`AndroidManifest.xml:23`） |
| MotionPreference + `AppReduceMotionChanged` | `UIAccessibilityIsReduceMotionEnabled` 与对应通知 | `Settings.Global.ANIMATOR_DURATION_SCALE == 0` 加 ContentObserver |
| Locales + LocaleWatcher | `NSLocale.preferredLanguages` 与 `NSCurrentLocaleDidChangeNotification` | `LocaleList.getDefault()`，在 `onConfigurationChanged`（locale）时触发 |
| 动态字体（提议，M5） | `UIContentSizeCategory` | `Configuration.fontScale` |

实现之后 uihost 的主题、语言和减少动态效果跟随不需要改任何代码。动态字体需要新的契约 `platform.TextScale{ TextScale() float32 }` 和 `AppTextScaleChanged = 202`（避开 1… 与 100… 两段；200 是 AppReduceMotionChanged，201 已被 AppContrastChanged 占用），另外 ui/theme 要提供字号整体缩放，这部分单独立项。

### 3.4 方向与内存

- 方向：canvas 只关心尺寸变化（CanvasResized）。现有的 `AppOrientationChanged` 和方向锁定（`sys/orientation`）照常可用。
- 内存（提议）：新增 `platform.AppMemoryWarning = 203`（无负载信号），根包 `mobile_events.go` 加一个导出常量（一行追加）。iOS 在 `didReceiveMemoryWarning` 时发出，Android 在 `onTrimMemory >= TRIM_MEMORY_RUNNING_LOW` 时发出。uihost 用 `App.OnEventKind` 订阅，收到后调 `gpuRenderer.Trim()`（新增：清空图集 LRU 与图片纹理，下一帧按需重建）和新增的 `text.TrimCaches()`（导出现有的 `clearCache`）。

---

## 4. 移动端文字

### 4.1 iOS：复用 CoreText 引擎（M1 必做）

- **问题**：`ui/text/engine_darwin.go:1` 的标签是 `darwin && !ios`，所以 iOS 上 `openEngine` 是 nil（`engine.go:131`）。系统字体于是退到 `sys_darwin.go` 的 macOS 路径（如 `/System/Library/Fonts/SFNS.ttf`），这些在 iOS 上不存在，结果是**文字不显示**（`sys_other.go` 的注释也说明了没有字体时的表现）。
- **方案**：
  - 把 `engine_darwin.go` 和 `engine_darwin_test.go` 的标签改为 `darwin`。它用 purego 加载 CoreFoundation、CoreText、CoreGraphics 的绝对路径（`engine_darwin.go:170-178`），用到的 API 是 `CTFontCreateUIFontForLanguage`、`CTFontDescriptor` 匹配、`CTFontCopyTable`、`CGBitmapContext`，iOS 都有。
  - `sys_darwin.go` 加标签 `!ios`，另建 `sys_ios.go`：CoreText 加载失败时遍历 `/System/Library/Fonts/{Core,CoreUI,CoreAddition}` 作为最后的兜底（只读，推断沙盒允许读取，需实测）。
- **验证**：在 M1 示例启动时打日志 `text.Engine()`，期望输出 `"coretext"`。iOS 上的中文（PingFang SC）和 emoji 都走 CoreText，emoji 目前按亮度画成单色（`engine_darwin.go:47-49`），与 macOS 相同。

### 4.2 Android：系统字体发现 + 纯 Go 光栅化（推荐）

- **现状**：`engine_linux.go:1` 是 `linux && !android`，排除原因是 Android 没有 fontconfig。`sys_linux.go` 没有标签，Android 也会编译它：它遍历 `/system/fonts`（`sys_linux.go:12`），按文件名匹配 Noto Sans、Roboto、`NotoSansCJK-Regular.ttc`（`:62`）。所以 Android 现在已经能显示拉丁文和中文，但字体顺序、各文字的回退、字重都不完整。
- **方案**：
  - `sys_linux.go` 加标签 `linux && !android`。
  - 新增 `sys_android.go`，按以下顺序解析字体配置：`/system/etc/font_fallback.xml`（Android 15 起）、`/system/etc/fonts.xml`，再加上 `/product/etc/fonts_customization.xml`。
    - `<family name="sans-serif">` 作为默认无衬线字体，`monospace` 作为等宽字体。
    - 没有 name 的 `<family lang="...">` 按出现顺序组成**回退链**。CJK 按 ui 当前语言（`ui/text/bidi.go:56` 的 `defaultLangTag`）选 `lang="zh-Hans"/"ja"/"ko"` 对应的家族，用 `index=` 选 TTC 里的子字体。
    - `<axis tag="wght" stylevalue=...>` 交给 go-text 的可变字体实例（已有 `weight_variation_test.go`）。
  - 解析器写成不加标签的 `fontsxml.go`。测试用的 `testdata/fonts.xml` **自己编写**，不复制 AOSP 文件，在宿主机上单测。
  - 可选增强：API 29+ 时用 purego dlsym `libandroid.so` 里的 `AFontMatcher_match` 做逐字回退，覆盖 fonts.xml 漏掉的文字，minSdk 24 的设备没有这个函数也不影响。
- **光栅化**：走纯 Go 路径，即 `GlyphPath` 加 `paint` 光栅化加 `TextGamma`，与 Linux 无引擎时的路径和黄金图基准一致。**不用 FreeType**，原因有三：
  1. `libft2.so` 不是公开 NDK 库，Android 7 起应用无法 dlopen。
  2. 打包 FreeType 源码每个 ABI 增加约 0.7 MB，还要处理 FTL/GPL 许可声明和 cgo 编译。
  3. 手机屏幕 2.6–3.5x 密度下，不做 hinting 的抗锯齿就是 Android 自己的做法，画质差异很小。
- **已知限制**：Noto Color Emoji 是 CBDT 位图字体，纯 Go 路径没有轮廓可画，会留白，Linux 引擎也是这样（`engine_linux.go:32`）。后续可以在 Go 里解码 CBDT 的 PNG，再接入 GPU 已有的 RGBA 图集页（`gpu-renderer.md:80`），各平台都能受益。部分 Android 版本另带 COLRv1 版本的 emoji 字体，需要上设备确认。

---

## 5. 无障碍

后端照常实现 `platform.CanvasAccessible`，uihost 侧不变（`uihost/a11y.go`）。纯逻辑部分追加到 `internal/platform/a11ybridge/mobile.go`（不加标签）：

- `IOSTraits(n) IOSTraitSet`：Tango 自定义的位集合（button、header、link、selected、notEnabled、adjustable、updatesFrequently、staticText、image、searchField、tabBar、toggleButton 等），按 `a11y.Mappings` 的 IOS 列（`ui/a11y/mapping.go:8-11`）和第 4 节状态表（`docs/accessibility.md:150-170`）推导。ObjC 侧再把位映射到 `UIAccessibilityTrait*` 的 extern 常量，常量值不在 Go 里写死。
- `IOSActions(n)`、`AndroidClass(n)`（取 Mappings 的 Android 列）、`AndroidActions(n)`、`AndroidStates(n)`。
- `VirtualIDs`：`a11y.NodeID`（uint64，`ui/a11y/a11y.go:26`）与 Android 虚拟视图 id（int32）之间的稳定双向映射。节点存在期间 id 不变，删除后延迟回收。

### 5.1 iOS（`canvas_a11y.go/.m`）

- `TGCanvasView.isAccessibilityElement = NO`，`accessibilityElements` 返回按树的**构建顺序**（DFS，即阅读顺序）排列的 `TGAXElement : UIAccessibilityElement`。RTL 时**不反转**（`docs/accessibility.md:76`）。列表、表格、地标、对话框节点做成嵌套容器，各自实现 `accessibilityElements` 和 `accessibilityContainerType`（List、DataTable、SemanticGroup）。对话框设 `accessibilityViewIsModal`。
- **坐标**：`accessibilityFrameInContainerSpace` 直接用 `Node.Bounds`（逻辑像素就是点，以 canvas view 为容器），坐标换算交给 UIKit，所以旋转和滚动都不需要重发树。Bounds 已经是镜像后的屏幕矩形，**不要再次翻转**（`ui/a11y/a11y.go:334-336`、`docs/accessibility.md:75`）。
- 属性：Name 对应 `accessibilityLabel`，Value 或状态文字对应 `accessibilityValue`，Description 对应 `accessibilityHint`。
- 动作：`accessibilityActivate` 对应 Press；`accessibilityIncrement/Decrement`；`accessibilityPerformEscape` 对应 Dismiss；`accessibilityScroll:`；展开、收起、设焦点用 `UIAccessibilityCustomAction`。全部经 `CanvasAccessibleSink.AccessibilityAction`，在主线程调用、不在帧内（`canvas.go:312-321`）。
- **激活**：满足任一条件即激活：`UIAccessibilityIsVoiceOverRunning()`、`IsSwitchControlRunning()`、第一次收到 `accessibilityElements` 查询（Voice Control、XCUITest 与 Accessibility Inspector 都会查询）。激活后 `AccessibilityActive` 返回 true 并 Invalidate。VoiceOver 关闭且 30 秒内没有查询时退出激活。
- 通知：结构变化发 `UIAccessibilityLayoutChangedNotification`（参数为焦点元素）；模态打开发 `ScreenChangedNotification`；`AccessibilityAnnounce` 发 `UIAccessibilityAnnouncementNotification`，Assertive 时先 `[UIAccessibility…]` 打断。
- 文本框的 VoiceOver 编辑（读出光标、逐字移动）需要让元素代理 UITextInput，放到 M4 之后。

### 5.2 Android（`canvas_a11y_android.go` + `CanvasA11y.java`）

- `CanvasView.getAccessibilityNodeProvider()` 返回 `CanvasA11y`。`createAccessibilityNodeInfo(vid)` 经 JNI 同步调 Go：`nativeA11yNode(handle, vid)` 返回一个打包结构（class、text、contentDescription、状态位、动作位、bounds、parent、children），Java 用它填 `AccessibilityNodeInfo`。数据来自 Go 保存的不可变快照，符合"只从快照回答"的规则（`canvas.go:269-275`）。查询都在主线程。
- `setBoundsInScreen` = `getLocationOnScreen()` + Bounds × density；同时设 `setVisibleToUser`（Offscreen 状态取反）。
- `performAction(vid, action, args)`：ACTION_CLICK、ACCESSIBILITY_FOCUS、SCROLL_FORWARD/BACKWARD、EXPAND/COLLAPSE、SET_TEXT、SHOW_ON_SCREEN、DISMISS 等，对应关系见 `docs/accessibility.md` 第 5 节，交给 sink。
- 悬停（触摸浏览）：在 `dispatchHoverEvent` 里用 `a11ybridge.HitTest`（`a11ybridge.go:206`）找到虚拟节点，发 `TYPE_VIEW_HOVER_ENTER/EXIT`。
- 事件：变化节点发 `TYPE_WINDOW_CONTENT_CHANGED`（SUBTREE / TEXT），焦点变化发 `TYPE_VIEW_FOCUSED`。播报优先用节点的 `setLiveRegion`；`announceForAccessibility` 从 API 33 起已弃用，只作回退。
- 激活：`AccessibilityManager.isEnabled()` 加 `AccessibilityStateChangeListener`。另外 `getAccessibilityNodeProvider` 被调用也视为激活，这样 `uiautomator dump` 也会触发，自动化测试可以利用这一点（见第 7 节）。

---

## 6. 面向应用的 API 与构建

### 6.1 应用写法

```go
func main() {
    app := tango.New(tango.Options{Name: "Mobile UI"})
    app.OnReady(func() {
        // 移动端：一个铺满全屏的自绘页，进入页面栈（第一个显示的即根页面）
        r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{Title: "首页"}}, page, ui.RunnerOptions{})
        _ = r; _ = s; _ = err
    })
    app.Run()
}
```

- 移动端会忽略 `SurfaceOptions` 的 Width、Height、Min*、Resizable，与 WebView 页一致（`docs/mobile.md:96,319`）。`s.Close()` 让页面出栈；`s.Hide/Show` 与 WebView 页相同。
- 页面级混排：`app.NewSurface(tango.SurfaceOptions{URL: "/"})` 叠在自绘页上面，关闭后回到自绘页。
- 同页叠放（M5）：`uihost.Attach(webSurface, uihost.Layer{Above: true, PassThrough: true, Bounds: ...}, page, ropts)`，API 不变（`canvas-surface.md:126`）。
- ui/mobile 组件读 `c.SafeArea()` 和新增的 `c.KeyboardInset()`。

### 6.2 tan build 与清单

- **iOS**：
  - canvas 的 `.m` 用到 CAMetalLayer 和 CADisplayLink，在 `canvas.go` 的 cgo 头里加 `-framework QuartzCore`。tan 会用 `go list -deps` 收集各包的 `#cgo LDFLAGS`（`cmd/tan/build_ios_app.go:268-280`），所以 **tan 不用改**。Metal 与 CoreText 由 purego 在运行时 dlopen，不需要链接。
  - `iosInfoPlist`（`build_ios_app.go:285` 起）加 `CADisableMinimumFrameDurationOnPhone = true`，否则 ProMotion 手机最高只有 60 Hz。建议默认开启：它只是允许更高刷新率，没有副作用。
- **Android**：canvas 文件加 `#cgo LDFLAGS: -landroid`。GLES 用 purego，不需要链接。清单不变：`hardwareAccelerated` 已开启，`configChanges` 已包含 density、uiMode、locale、fontScale（`AndroidManifest.xml:23`）。
- **烟测用的开发构建**：自动化通道只在 `tango_dev` 构建里存在（`uihost/automation_dev.go:1`），而 `tan build` 会主动去掉 `GOFLAGS` 里的 `tango_dev`（`cmd/tan/build_tags.go:24-26`）。为此新增一个**隐藏参数** `tan build -dev-features`，只给 CI 和烟测用，等价于 `tan dev` 的构建标签，产物名带 `-dev` 后缀。

### 6.3 示例 `examples/mobile-ui`

- 内容：TabBar 四个标签页。
  1. 列表页：ScrollView、下拉刷新、IndexBar。
  2. 表单页：多个输入框，用来验证输入法和键盘避让；Picker、日期选择。
  3. 混排页：按钮打开一个 WebView 页（`app.NewSurface`）；M5 起演示 `uihost.Attach` 浮层。
  4. 设置页：主题、语言、减少动态效果的跟随状态，打印 `text.Engine()` 和渲染器名称。
- `tan.json`：`id: dev.tango.mobileui`，`permissions: []`。
- 启动参数（iOS 用 `SIMCTL_CHILD_*` 环境变量，Android 用 Intent extra，仅 debuggable 包读取，做法同 `devurl_android.go`）：`TANGO_MOBILEUI_GOLDEN=1` 进入黄金图模式（见 7.3）。

---

## 7. 测试策略

### 7.1 宿主机单测（纯 Go，任意 OS，CI 必跑）

| 包 / 文件 | 覆盖 |
|---|---|
| `internal/platform/mobilecanvas` | 调度状态机（静止零帧、每刷新周期最多一帧、InvalidateAt 取最早、后台不出帧、回前台补帧）；触摸映射（ID 分配与回收、时间换算、合并点上限、取消）；`IMEDoc`（标记、提交、删除、替换、选区、UTF-16 与 rune 互换，含 emoji 和组合字符）；inset 计算 |
| `internal/platform/ios/canvas_keys.go`、`android/canvas_keys.go` | 键码表、按键路由决策表、Hints 到原生枚举的映射 |
| `internal/platform/a11ybridge/mobile.go` | traits、class、动作推导；虚拟 id 分配；RTL 不翻转 |
| `ui/text/fontsxml.go` | 自己编写的 fonts.xml 样例：默认家族、回退链顺序、lang 选择、TTC 子字体序号、轴 |
| `ui`（Driver） | `d.SetKeyboardInset`、自动避让、`TextEdit` / `SelectionChange` 及降级、`TextInputState` 下发时机 |
| `uihost`（fake 后端） | fake canvas 实现 `CanvasInsets` / `CanvasTextInputState`，校验转发、轮询和重画；`gpu_android` 选择逻辑（参照现有 `EnableCanvasMetal/HWND` 的写法，`internal/platform/fake/canvas_metal.go:51-53`） |

### 7.2 交叉编译与 vet（CI，macOS runner）

```
GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC=$(xcrun --sdk iphonesimulator --find clang) \
  CGO_CFLAGS="-isysroot $(xcrun --sdk iphonesimulator --show-sdk-path) -target arm64-apple-ios17.0-simulator" \
  go vet ./internal/platform/ios/... ./uihost/... ./ui/... (再加 -tags tango_dev 跑一遍)
GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<ndk>/aarch64-linux-android24-clang go vet ./internal/platform/android/... ./uihost/... ./ui/...
```

`-tags tango_dev` 那一遍会编译 devhost（Unix socket、共享内存），需要确认它在 ios 和 android 上能编译。

### 7.3 模拟器烟测（`scripts/smoke.sh ios-ui | android-ui`，参照现有的 `run_ios`、`run_android`，`smoke.sh:222,295`）

1. 用 `tan build -dev-features` 构建 `examples/mobile-ui`，然后安装并启动：
   - iOS：`SIMCTL_CHILD_TANGO_AUTOMATION=127.0.0.1:0`，`TANGO_AUTOMATION_TOKEN_FILE` 指向 `simctl get_app_container <udid> <id> data` 下的 `tmp/`。模拟器共享宿主机网络，可以直接连接。
   - Android：Intent extra 加 `TANGO_AUTOMATION=127.0.0.1:<端口>`，`adb forward tcp:<端口> tcp:<端口>`，token 文件写在应用 cache 目录，用 `adb shell run-as <id> cat` 读取。
2. 自动化协议（`docs/automation.md:67`）：`windows` 应返回 canvas 窗口；`tree` 和 `find` 检查关键节点；`action` 点按钮，再 `wait` 状态变化；`screenshot` 取 Runner 帧。
3. **真实触摸链路**：Android 用 `adb shell input tap/swipe` 注入真实 MotionEvent，再经 automation 的 `tree` 验证。iOS 的 simctl 没有点按命令，第一阶段只做截图与日志检查，第二阶段在 `tan build -xcode` 生成的工程里加一个 XCUITest 目标，用系统 a11y 点击。
4. **无障碍**：Android `adb shell uiautomator dump` 会触发 NodeProvider，检查 XML 里的 text 和 class；iOS 用 XCUITest 的 `app.buttons["…"]` 查询，同样走 a11y 树。
5. **键盘**：Android `adb shell input text "abc"` 加 `ime set`（可选 Pinyin 输入法镜像）；iOS 模拟器 `simctl io` 发硬件键盘事件，软键盘部分人工验收。
6. 截图：`simctl io screenshot`、`adb exec-out screencap -p`，与日志一起存档（与现有 smoke 相同）。

### 7.4 与桌面画廊的像素比对

- 黄金图模式（`TANGO_MOBILEUI_GOLDEN=1`）下：
  - 用 `text.RegisterFont` 注册内嵌的 Noto Sans 和 Noto Sans SC。注册字体**总是走纯 Go 路径**（`engine.go:37`），所以设备和宿主机的文字路径相同。
  - `UseSystemEngine(false)`，固定主题，`ReduceMotion`，用 `SetSafeArea` 固定安全区。
  - 依次显示 `examples/ui-gallery/page_mobile_gallery.go` 与 `page_mobile_screens.go` 里的页面。
- 宿主机用 `ui.Driver` 按设备的逻辑尺寸和 scale 渲染同样的页面。设备截图去掉状态栏区域后逐像素比对，阈值沿用 `gpu-renderer.md:143`：几何 ≤2/通道，文字与图片缩放 ≤4/通道，差异像素 ≤0.5%。iOS GPU、iOS CPU、Android CPU 各比一次；M5 起加 Android GLES。

---

## 8. 里程碑、工作量与风险

### 8.1 里程碑

| 里程碑 | 内容 | 新文件（主要） | 改动现有文件（全部是小的追加） | 估算 |
|---|---|---|---|---|
| **M1** iOS 上屏、触摸、安全区（模拟器） | 第 0 步：交叉编译 `ui/gpu/metal`、`ui/text`、`uihost`，确认 dlopen 路径与 `setColorspace:`；`TGCanvasPage/View`、display link、CPU 与 Metal 子层、触摸、`CanvasInsets` 安全区、前后台；CoreText 去掉 `!ios`；`mobilecanvas` 调度与触摸；`examples/mobile-ui` 的列表和设置页 | `internal/platform/ios/{canvas.go,canvas.h,canvas.m,canvas_keys.go,system.go,system.m}`、`internal/platform/canvas_mobile.go`、`internal/platform/mobilecanvas/*`、`uihost/insets.go`、`ui/text/sys_ios.go` | `ios/bridge.m` 与 `bridge_private.h`（`tg_page_register`）、`ui/text/engine_darwin*.go` 与 `sys_darwin.go`（标签）、`cmd/tan/build_ios_app.go`（plist 一项） | 3 人周 |
| **M2** Android CPU 上屏、触摸、insets（模拟器） | PageBase 重构、按子页面分发 inset、`CanvasPage/View`（SurfaceView）、Choreographer、`ANativeWindow` 拷贝与 lock 脏区、触摸与鼠标、遮挡时 INVISIBLE、Activity 重建；`sys_android.go` 与 fonts.xml；SystemTheme、Motion、Locales | `internal/platform/android/{canvas_android.go,canvas_android.c,canvas_android.h,canvas_keys.go,system_android.go}`、Java 的 `PageBase/CanvasPage/CanvasView/TangoCanvas`、`ui/text/{sys_android.go,fontsxml.go}` | `exports_android.go`（一行 lookup）、`TangoActivity.java` 与 `Page.java`、`ui/text/sys_linux.go`（标签） | 3.5 人周 |
| **M3** 两端输入法与键盘 | M3a 无上下文；M3b `TextInputState`、`TextEdit`；键盘 inset 与自动避让；硬件键盘；Hints | `ui/keyboard.go`、`ui/textstate.go`、`ui/input/textstate.go`、`mobilecanvas/ime.go`、`ios/canvas_text.m`、`CanvasInput.java`、`uihost/textstate.go` | `ui/scroll.go`（`ScrollIntoViewInset`）、ui 的 Input 和 Textarea 及 ui/mobile 表单接入 | 4 人周 |
| **M4** 两端无障碍 | iOS 元素容器、Android NodeProvider、激活、通知、动作；uiautomator 与 XCUITest 烟测 | `a11ybridge/mobile.go`、`ios/canvas_a11y.{go,m}`、`android/canvas_a11y_android.go`、`CanvasA11y.java` | — | 3 人周 |
| **M5** Android GPU、混排、收尾 | `ui/shader/glsl`（ES 300 与 330）、`ui/gpu/gl`、EGL 呈现器、`CanvasNativeWindow`、`gpu_android.go`、`ResetDevice`；两端 `CanvasAttacher`（iOS 子视图，Android TextureView 加 Page 容器）；返回键 `WantBack`；内存 Trim；iPad 指针与触控板；ProMotion 验收；黄金图比对进 CI | `ui/shader/glsl/*`、`ui/gpu/gl/*`、`uihost/gpu_android.go`、`ios/canvas_attach.{go,m}`、`android/canvas_attach_android.go` | `uihost/gpu_other.go`（标签）、`ui/gpu/renderer.go`（`ResetDevice`、`Trim`）、`mobile_events.go`（一个常量）、`Page.java`（容器） | 6–8 人周 |
| 合计 | | | | **约 19.5–21.5 人周**；如果 Linux GL（GPU 里程碑 4）先完成，M5 可以减 2–3 周 |

**依赖与顺序**：M1 和 M2 可以并行（共用 `mobilecanvas` 和 `canvas_mobile.go`，先合并接口文件）。M3 依赖 M1 和 M2 的视图。M4 可以和 M3 并行。M5 的 GLES 部分与 Linux GL 共享，等哪一个先落地。

**与其他会话的协调**：
- 根包重组（平台选择迁到 `internal/platform/native`，目前还不存在）：本设计**不改**根包的 `backend_*.go`、`canvas_hook.go`，只在 `mobile_events.go` 加一个常量，放到 M5。
- 桌面 PresentDamage：Android CPU 路径以它定下的平台接口名实现，接口还没定之前先实现 `ui.DamagePresenter` 的语义，由 uihost 转发。

### 8.2 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| purego 在 iOS 上：要求 cgo；绝对路径 dlopen 在模拟器和真机上的行为；App Store 审核 | GPU 和 CoreText 不可用 | iOS 产物本来就是 cgo；M1 第 0 步实测模拟器和真机。dlopen 的都是公开系统框架、调用公开 API 和公开选择器，运行时编译的是内嵌的 MSL 源码，不下载代码。失败时 CPU 路径照常可用，文字可以退到 `sys_ios.go` |
| `setColorspace:` 等 macOS 专有属性在 iOS 的 CAMetalLayer 上不存在 | 发送未识别的选择器导致崩溃 | 加 `respondsToSelector:` 保护（只改一行） |
| Apple Silicon 模拟器上的 Metal | 个别限制 | 纹理已经全是 Private（`metal_darwin.go:882`）；失败时按 `ErrDeviceLost` 永久退回 CPU |
| 后台提交 GPU 命令被系统终止（iOS） | 崩溃 | 后台时调度器硬性禁止出帧，并写单测 |
| cgo 与 JNI 的线程问题：非 Java 线程 attach 开销；FindClass 只能在主线程调用；Surface 回调必须同步释放 | 卡顿或崩溃 | 只在从空闲变为待处理时 attach 一次；方法 ID 在 `nativeInit` 里查好；`surfaceDestroyed` 中同步 `ANativeWindow_release` |
| ANativeWindow 格式与 stride（模拟器可能给 565、stride 大于宽度、lock 扩大脏区） | 花屏 | 显式 `setBuffersGeometry` 与 `holder.setFormat`；lock 之后检查 format，必要时走慢路径；逐行拷贝；以 lock 返回的脏区为准 |
| 同类 SurfaceView 的 z 序不确定 | 下层页面盖住上层 | 被遮挡的 canvas 页设 INVISIBLE；叠放层改用 TextureView |
| 主线程 CPU 光栅化太慢（Android M2） | 触摸延迟 | 先实测；`paint.CPU` 分条并行；M5 换 GPU |
| 输入法边界情况：Gboard 与三星输入法的 `deleteSurroundingText` 和 `setComposingRegion`；iOS 自动纠错替换整词；漏调 `updateSelection` 导致不同步；emoji 与代理对偏移 | 吞字、重复字 | M3a 降级为按键；M3b 用影子文档加纯 Go `IMEDoc` 单测；所有偏移统一经 utf16 辅助函数；在多种输入法上做人工验收清单 |
| 按子页面处理 inset 改变 WebView 页的布局 | 回归 | margin 数值与原 padding 相同；现有 `smoke.sh android` 继续通过；mobile-hello 截图对比 |
| 1 px 尺寸误差（`ceil(Size*Scale)` 与 Surface 实际像素） | 边缘出现杂色 | 取较小值拷贝并补齐边缘，与 Metal 呈现器的规则一致 |
| CBDT 彩色 emoji 留白（Android） | 显示缺字 | 记为已知限制；后续实现 CBDT 解码 |
| App Store 与隐私清单 | 被拒 | 不新增需要声明理由的 API（SystemBootTime 已声明）；不 dlopen 私有框架 |

---

## 9. 不做（本期）

- 移动端自绘热重载（`docs/specs/ui-hotreload.md:74-78` 已说明原因）。
- Vulkan、Metal 之外的 iOS 后端、鸿蒙。
- 在自绘布局里嵌入原生 WebView 矩形（"WebView 作为自绘树的子控件"）。需要时用 `uihost.Attach` 的下层模式加透明页面实现。
- 多场景（iPad 多窗口）与 Android 多窗口下的多个 canvas 栈（沿用现有限制，`docs/mobile.md:140,368`）。
