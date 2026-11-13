# 自绘 UI 上屏（Canvas Surface）

状态：设计稿，2026-11-12。实现分 A–D 四步，A 先做（macOS 能实机验证）。

## 目标

`ui/` 现在只有无头 `Driver`（渲染出 PNG）。本设计让自绘 UI 显示在真实窗口里，并为"WebView 和自绘同窗"留好接口：

```go
app := tango.New(tango.Options{Name: "Demo"})
app.OnReady(func() {
    r, s, err := uihost.Open(app, uihost.Options{
        Surface: tango.SurfaceOptions{Title: "Gallery", Width: 1200, Height: 800},
    }, gallery.Page, ui.RunnerOptions{})
    _ = r; _ = s; _ = err
})
app.Run()
```

`uihost.Open` 返回的 `*tango.Surface` 是普通 Surface：标题、大小、状态、外观、关闭事件等现有 API 全部可用，只是内容由 `ui.Runner` 绘制而不是 WebView。

## 分层

```
ui（纯 Go，不依赖平台）   Runner / Window / Host 契约，组件
uihost（新包）             把 tango.App 适配成 ui.Host；调度 Frame / 事件
internal/platform          新文件 canvas.go：Canvas 契约（可选接口）
  darwin / windows / linux / ios / android   各自实现 CanvasCreator
根包                        新文件 canvas_hook.go：通过 corehook 暴露创建 canvas Surface
```

- `ui` 包保持零平台依赖；根包不导入 `ui`。
- 不修改现有契约文件（backend.go / types.go / caps.go / ext.go），只新增 `internal/platform/canvas.go`。

## 平台契约（internal/platform/canvas.go）

```go
// CanvasSink 由核心层（uihost）实现，后端只在主线程调用。
type CanvasSink interface {
    CanvasFrame()                    // 该画了：Invalidate 之后、尺寸/缩放变化、首次显示
    CanvasInput(ev input.Event)      // 逻辑坐标，相对内容区左上角
    CanvasResized(w, h, scale float32)
    CanvasFocus(focused bool)
}

// Canvas 是后端提供给核心的绘制目标。
type Canvas interface {
    Size() (w, h float32)            // 逻辑像素
    Scale() float32                  // 设备像素比
    Invalidate()                     // 任意 goroutine；只安排一帧，可合并
    InvalidateAt(t time.Time)        // 任意 goroutine；更早的时间覆盖更晚的
    SetCursor(c input.Cursor)
    SetTextInput(enabled bool, caret geom.Rect)
    Present(img *image.RGBA) error   // 主线程；img 为预乘 RGBA，返回前须复制或上传
}

// CanvasCreator 可选：后端支持"内容是 canvas 的 Surface"。
type CanvasCreator interface {
    NewCanvasSurface(id SurfaceID, opts SurfaceOptions, ex SurfaceExtras, sink CanvasSink) (Surface, Canvas, error)
}

// CanvasAttacher 可选（D 步）：把 canvas 叠在 WebView Surface 上。
type CanvasAttacher interface {
    AttachCanvas(id SurfaceID, layer CanvasLayer, sink CanvasSink) (Canvas, error)
}
type CanvasLayer struct {
    Bounds Rect          // 相对内容区，逻辑像素；零值 = 铺满
    Above  bool          // true 叠在 WebView 之上；false 在其下（WebView 需透明）
    PassThrough bool     // 透明像素的鼠标事件交给下层
    HitAlpha uint8       // 命中阈值，0 视为 1
    Anchor CanvasAnchor  // 非零 Bounds 缩放时保持到哪个角/中心的距离
}
```

返回的 `Surface` 对 WebView 专用方法（Navigate / Eval / OpenDevTools）做空操作；`Native()` 返回窗口句柄。剪贴板复用已有的 `platform.Clipboard` 能力，不放进 Canvas。

## 根包钩子（canvas_hook.go）

`corehook.NewCanvasSurface func(app any, opts any, sink platform.CanvasSink) (surface any, canvas platform.Canvas, err error)`，由根包在 init 里赋值：分配 SurfaceID、走与 `NewSurface` 相同的 `extras`/`afterCreate` 流程、登记到 surface 表，使关闭、事件、外观等逻辑全部复用。后端不支持时返回 `ErrUnsupported`。

## uihost 包

- `uihost.Open(app, Options, page, ui.RunnerOptions) (*ui.Runner, *tango.Surface, error)`，必须在主线程（如 OnReady 里）或通过 `app.Post` 调用。
- 内部的 `window` 实现 `ui.Window`：Size/Scale/Invalidate/InvalidateAt/SetCursor/SetTextInput/Present 转给 `platform.Canvas`；ReadClipboard/WriteClipboard 走 `clipboard` 能力。
- sink：`CanvasFrame` → `Runner.Frame()`，帧后若 `Runner.Animating()` 则再 Invalidate，若 `NextWake()` 非零则 InvalidateAt；`CanvasInput` → `Runner.HandleEvent` 后 Invalidate；`CanvasFocus` → `input.WindowFocus`；Surface 关闭 → `Runner.Close()`。
- UI 线程 = 主线程。

## A 步：macOS 实现（先做）

`internal/platform/darwin/canvas*.go`，用 objc 运行时（purego）注册 `TangoCanvasView : NSView <NSTextInputClient>`：

- 绘制：layer-backed（`wantsLayer`、`wantsUpdateLayer` = YES），`isFlipped` = YES。`Present` 用 `CGDataProviderCreateWithData`（复制像素）+ `CGImageCreate`（`kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big`）设到 `layer.contents`，`contentsScale` = `backingScaleFactor`，关闭隐式动画（`CATransaction setDisableActions:`）。
- 帧调度：`Invalidate` → 合并标志 + 主线程 `setNeedsDisplay:YES`；`updateLayer` 里调 `sink.CanvasFrame()`。`InvalidateAt` 用单个可重设的 dispatch 定时器。动画期间用 `CVDisplayLink` 或 `NSView.displayLink`（macOS 14+，可选）对齐 vsync；没有也可退回 16ms 定时器。
- 尺寸：`setFrameSize:` / `viewDidChangeBackingProperties` → `CanvasResized` + 重绘。
- 鼠标：`mouseDown/Up/Dragged/Moved`、`right*`、`other*`、`NSTrackingArea`（`mouseEntered/Exited`），`clickCount` → `Pointer.Clicks`；`acceptsFirstMouse:` = YES。
- 滚轮：`scrollWheel:`，`hasPreciseScrollingDeltas` 时用像素增量，否则乘行高；方向按 `ui/window.go` 约定（正 DY 显示下方内容）。
- 键盘：`acceptsFirstResponder` = YES；`keyDown:` 在文本输入开启时交给 `interpretKeyEvents:`，同时把功能键映射成 `input.KeyEvent`；`keyUp:`、`flagsChanged:`；`performKeyEquivalent:` 不吞菜单快捷键。
- 输入法：完整实现 `NSTextInputClient`（`insertText:replacementRange:` → `TextInput`，`setMarkedText:selectedRange:replacementRange:` → `Composition`，`unmarkText`，`hasMarkedText`，`markedRange`，`selectedRange`，`firstRectForCharacterRange:actualRange:` 用 `SetTextInput` 传来的 caret 换算到屏幕坐标，`validAttributesForMarkedText`，`attributedSubstringForProposedRange:actualRange:` 返回 nil，`characterIndexForPoint:` 返回 NSNotFound，`doCommandBySelector:` 映射为功能键）。
- 光标：`input.Cursor` → `NSCursor`（arrow / pointingHand / IBeam / operationNotAllowed / resizeLeftRight / resizeUpDown / openHand / closedHand / 移动用 openHand），在 `mouseMoved` 和 `cursorUpdate:` 里 `set`。
- 窗口：复用现有 NSWindow 创建与 `SurfaceExtras` 处理，只把 contentView 换成 canvas view。

验收（macOS 实机）：

1. `examples/ui-gallery` 增加窗口模式（`-window` 参数或默认），能打开窗口并显示画廊，Retina 下清晰。
2. 鼠标悬停/点击/拖动、滚轮、Tab 焦点、文本框输入英文、拼音输入法上屏、复制粘贴、光标形状随控件变化。
3. 改变窗口大小重排，无闪烁；拖到外接非 Retina 屏能切换缩放。
4. 动画（按钮过渡、加载动画）流畅，静止时 CPU 接近 0。
5. `go test ./...`、`GOOS=windows/linux CGO_ENABLED=0 go build ./... && go vet` 通过；`uihost` 用 fake 后端写单元测试（fake 实现 CanvasCreator）。

## B 步：Windows / Linux

- Windows：子窗口或客户区，`WM_PAINT` + `StretchDIBits`（或 `UpdateLayeredWindow` 支持透明）；`WM_MOUSE*`、`WM_MOUSEWHEEL/HWHEEL`、`WM_KEY*`、`WM_CHAR`；输入法用 IMM32（`WM_IME_COMPOSITION`、`ImmSetCompositionWindow`）；`SetCursor` + `WM_SETCURSOR`；DPI 用 `WM_DPICHANGED`。
- Linux：GTK3 `GtkDrawingArea`，`draw` 信号里 cairo 绘制 `cairo_image_surface_create_for_data`（注意 cairo 是预乘 ARGB32，需要通道换序）；事件用 `button-press/release`、`motion-notify`、`scroll`（含 smooth）、`key-press/release`；输入法 `GtkIMContext`；光标 `gdk_cursor_new_from_name`。
- 两端只能交叉编译 + vet，实机验证待补。

## C 步：移动端

iOS `UIView` + `CALayer.contents`，触摸映射为 Pointer（单指）；Android `SurfaceView`/`TextureView` + `ANativeWindow_lock` 或 Java 侧 Bitmap。输入法与安全区在移动端扩展里一起做。

现状：按 [mobile-canvas.md](mobile-canvas.md) 实现了 M1–M4，比本节原计划多：多点触控（`input.TouchEvent`）、iOS Metal GPU 路径、安全区与键盘 inset、两端输入法、两端无障碍。M5（Android GPU 与移动端 `CanvasAttacher` 叠放）未做。Android 自绘页有模拟器实测记录（[../mobile.md](../mobile.md)），两端都还没在真机上跑过。

## D 步：WebView 与自绘同窗

实现 `CanvasAttacher`：macOS 把 canvas view 作为 WKWebView 的兄弟视图加在上方或下方；Windows 用子 HWND（WebView2 的 controller 父窗口同级）；Linux 用 `GtkOverlay`。`uihost.Attach(surface, layer, page, ropts)` 是对应的公开 API。PassThrough 由 `hitTest:` / `WM_NCHITTEST` 返回透明 / `gtk_widget_input_shape_combine_region` 实现。

### D 步实现状态（2026-11-12）

- 公开 API：`uihost.Attach(surface, uihost.Layer{Bounds, Above, PassThrough}, page, ropts) (*ui.Runner, *uihost.Attached, error)`；`Attached.SetLayer / Detach / Layer / Size / Detached / Surface`。Surface 关闭或 Detach 时 Runner 关闭。`tan dev` 热加载 worker 中返回 `ErrUnsupported`（附着的 canvas 不转发给 dev host）。`tan dev` 在首次构建后检查二进制是否链接了 `uihost.AttachWith`（Attach 经由它），是则不进热加载、改为整进程重启模式并打日志（cmd/tan/dev_hot.go `hotCapable`）。
- 契约追加（不改已有签名）：`platform.AttachedCanvas{Canvas; SetLayer(CanvasLayer) error; Detach()}`，AttachCanvas 返回的 Canvas 可选实现；钩子 `corehook.AttachCanvas`（根包 `canvas_attach_hook.go`）。
- macOS（`darwin/canvas_attach.go`）：`TangoCanvasOverlay`（`TangoCanvasView` 子类）加进 WKWebView 所在容器，上/下由 `addSubview:positioned:relativeTo:` 决定；零 Bounds 自动随容器缩放，非零 Bounds 钉在左上角。PassThrough 用上一帧 alpha 做 `hitTest:`（alpha 0 返回 nil）；鼠标移动另由本地事件监视器分流：浮层不透明处只给浮层（网页看不到 hover、WebKit 光标不覆盖浮层光标），透明处浮层不设光标并报 Leave。下层 canvas 存在时关闭 WebView 的 `drawsBackground`、under-page 颜色设为透明，页面 CSS 也须透明；下层 canvas 在 WebView 覆盖处收不到指针输入。
- 验收：`internal/platform/darwin/internal/smokeattach -smoke`；示例 `examples/webview-overlay`。
- Windows（`windows/canvas_attach_windows.go`，2026-11-12，仅交叉编译 + 纯 Go 单测，未实机）：canvas 是 surface 顶层窗口（也是 WebView2 controller 的父窗口）下的 `TangoCanvas` 子窗口，复用 canvas_windows.go 的帧调度/DIB/输入/IME。上/下用 `SetWindowPos(HWND_TOP/HWND_BOTTOM)`；WebView2 子窗口异步创建且落在最上，所以 controller 就绪（`setUp`）和每次 WM_SIZE 都重新排序。位置：零 Bounds 铺满客户区，非零 Bounds 按 `AnchoredBounds`（相对设定时的内容区尺寸）换算成设备像素，父窗口 WM_SIZE 时跟随；DPI 变化经 WM_SIZE + `WM_DPICHANGED_AFTERPARENT` 生效。上层 canvas 是分层子窗口（`WS_EX_LAYERED` + `UpdateLayeredWindow`，预乘 BGRA、`AC_SRC_ALPHA`），分层窗口不走 WM_PAINT，`paintNow` 改为直接出帧并推送；系统拒绝分层（缺 Win8 manifest 兼容项）或 ULW 失败时永久退回不透明 BitBlt 并打日志。下层 canvas 是普通子窗口、去掉 `WS_CLIPSIBLINGS`，存在时 `put_DefaultBackgroundColor` 设 alpha 0，最后一个下层 canvas 移走后恢复外观颜色。PassThrough：Present 记录 `alpha >= HitThreshold()` 的掩码；`WM_NCHITTEST` 低于阈值返回 `HTTRANSPARENT`（只对同线程窗口有效），另外按掩码生成窗口区域（`ExtCreateRegion` + `SetWindowRgn`，按行程合并成矩形带，变化时最多 30Hz，经投递消息在绘制之外设置），区域外系统按命中测试把输入交给下面的窗口，不管它属于哪个进程；首帧前区域为空（整块穿透）。区域同时裁剪绘制，HitAlpha>1 时低于阈值的半透明像素不显示。焦点：点击 canvas 时 SetFocus（已有）；点网页时 WebView2 拿走焦点，canvas 从 WM_KILLFOCUS 得知；surface 记住最后持焦点的是 canvas 还是网页，窗口重新激活（WM_SETFOCUS → `focusContent`）时还原；Detach 一个持焦点的 canvas 后 `MoveFocus` 回 WebView。验收：`internal/platform/windows/internal/smokeattach`（`.\smokeattach.exe -smoke`，会移动真实鼠标并读屏，运行时窗口须在前台、不要动鼠标）。
- Linux（`linux/canvas_attach_linux.go`，2026-11-12，仅交叉编译 + 纯 Go 单测，未实机）：WebView surface 的内容改为 box → `GtkOverlay` → web view，web view 是 overlay 的第一个 *overlay* 子控件而不是主子控件（overlay 没有主子控件）。原因：主子控件总画在所有 overlay 之下，若 web view 做主子控件，下层 canvas 就得在附着或 SetLayer 翻转时重新挂 web view（WebKit 控件 unrealize/realize）；全部做 overlay 子控件后，上下只是列表顺序，用 `gtk_overlay_reorder_overlay`（GTK 3.18+）调整，它会重排各 overlay 子窗口的 GdkWindow，绘制与事件分发都按这个顺序（另按顺序 `gdk_window_raise` 兜底）。位置由 overlay 的 `get-child-position` 信号给出（零 Bounds 铺满，非零按 Anchor）；新分配经 size-allocate 报 CanvasResized 并出帧。canvas 设 app-paintable、用 OVER 绘制，与 web view 的混合发生在 GDK 客户端窗口树内，因此不另设 RGBA visual（那只用于透出桌面，即 `Appearance.Transparent`，且子窗口 visual 与顶层不一致会被迫变成原生窗口反而无法混合）。下层 canvas 存在时 `webkit_web_view_set_background_color` 设 alpha 0，移走后按当前外观恢复。PassThrough：overlay 子控件设 `gtk_overlay_set_overlay_pass_through`，canvas 控件的输入形状（`gtk_widget_input_shape_combine_region`）按掩码生成（设备像素矩形向外取整到逻辑像素，`cairo_region_create_rectangles`，空闲时设置、最多 30Hz）；形状外的点在 canvas 这一支里没有命中，GDK 交给下一个窗口即 web view；首帧前形状为空。附着不抢焦点；点 canvas 抢焦点（已有），点网页 WebKit 抢回，窗口重新激活由 GtkWindow 还原；Detach 持焦点的 canvas 后 grab 回 web view。验收：`internal/platform/linux/internal/smokeattach`（X11：`xvfb-run -a -s '-screen 0 1280x800x24' ./smokeattach -smoke`；Wayland：headless weston 下 `GDK_BACKEND=wayland` 运行，合成点击/移动/读屏不可用，相应检查记 SKIP）。
- 两端共同的已知限制：附着下层 canvas 后若再调 `SetAppearance` 改背景色，外观代码会把 WebView 背景设回不透明（未接入外观路径），应先改外观再附着下层 canvas（或重新附着）。

### 同窗与 canvas 遗留问题修复（2026-11-12）

- **契约追加**（`internal/platform/canvas.go`，只追加）：`CanvasLayer.HitAlpha uint8`（PassThrough 命中阈值，0 视为 1，即 alpha>0 才命中；32 可让阴影/抗锯齿边缘穿透）；`CanvasLayer.Anchor CanvasAnchor`（`AnchorTopLeft` 零值 / `TopRight` / `BottomLeft` / `BottomRight` / `Center`，只对非零 Bounds 生效：Bounds 仍按当前内容区从左上角给出，之后窗口缩放时保持到该角（或中心）的距离）；辅助方法 `HitThreshold()`、`AnchoredBounds(oldW, oldH, newW, newH)` 供各端实现共用。可选接口 `CanvasDropTarget{CanvasDragOver(x, y float32); CanvasDragLeave()}`：sink 实现时，文件拖过 canvas 会收到悬停坐标（canvas 局部逻辑像素）与离开，用于高亮；根包 `canvasSink` 包装会透传。`uihost.Layer` 同步加 `HitAlpha`、`Anchor`（`uihost.AnchorTopLeft…AnchorCenter`）并透传。uihost 的 sink 目前未实现 `CanvasDropTarget`（ui 包没有对应事件），需要高亮时再在 ui/uihost 加。
- **canvas 窗口拖入文件**：`SurfaceExtras.AcceptFiles` 时 canvas 窗口（及 AcceptFiles 的 WebView 窗口上附着的 canvas）接受文件，向 Host 报 `SurfaceFilesDropped{Paths, X, Y}`，X/Y 为内容区逻辑坐标，与 WebView 窗口一致；不含文件的拖动拒绝。顺序：`CanvasDragOver`…→`CanvasDragLeave`→`SurfaceFilesDropped`。macOS：`TangoCanvasView` `registerForDraggedTypes:`（public.file-url）+ NSDraggingDestination（`darwin/canvas_drop.go`）。Windows：OLE `IDropTarget` + `RegisterDragDrop`（复用 sysx COM 对象工具，能给悬停回调；只回 copy/link，不回 move）。Linux：`gtk_drag_dest_set` + `drag-motion/leave/data-received`（text/uri-list）。
- **macOS 网页 hover 清除**：鼠标从网页移到不透明浮层时（本地事件监视器开始吞 mouseMoved），给 WebKit 的鼠标跟踪对象（WKWebView 跟踪区 owner，即私有 `WKMouseTrackingObserver`；WKWebView 自己的 `mouseMoved:` 不处理）发一个合成 `mouseExited:`，页面的 `:hover`、`mouseleave/mouseout` 随之清除；回到透明区或页面时 WebKit 的正常 mouseMoved 恢复 hover。注意：直接给 WKWebView 发 `mouseMoved:` 无效，发到视图外的 mouseMoved 也会被 WebKit 丢弃（第一响应者 + 不在 visibleRect）。
- **macOS HitAlpha / Anchor**：`hitTest:` 与鼠标移动分流按 `alpha >= HitThreshold()` 判断。四角锚点用 autoresizingMask；`AnchorCenter` 的 autoresizing 会按边距比例分配，故在 `windowDidResize:` 里按 `AnchoredBounds` 从设定时的内容区尺寸精确重算。
- **macOS 编辑菜单状态**：`TangoCanvasView` 实现 `validateMenuItem:` / `validateUserInterfaceItem:`：文本输入关闭时 Copy/Cut/Paste/Select All/Undo/Redo 禁用，开启时启用（Paste 另看剪贴板是否有文本），其他动作保持启用。
- **Windows canvas 透明**：没有用 `UpdateLayeredWindow`——子窗口分层只能与父窗口混合、透不到桌面且需 Win8 + 清单；顶层窗口分层会替换标题栏/边框，并与 `Opacity`（`SetLayeredWindowAttributes`）冲突。改为沿用透明 WebView 窗口已有的"DWM 边框扩到整个客户区"：canvas 子窗口画在父窗口的重定向表面上，BitBlt 32 位预乘 DIB 时保留 alpha；透明时帧未覆盖处填透明黑。限制：透明像素仍接收点击（与透明 WebView 窗口一致）。若实机透明处显示为黑，需改 DirectComposition。
- **AltGr**：Windows 识别 AltGr 先发的伪 LCtrl（非扩展 `VK_CONTROL`，下一条是同方向、同时间戳、带扩展位的 `VK_MENU`；`PeekMessage(PM_NOREMOVE)` + `GetMessageTime`）并丢弃（按下与抬起都配对），AltGr 报 `KeyAltRight`，按住期间 Mods 去掉 Ctrl（右 Ctrl 真按着时保留）。Linux：`ISO_Level3_Shift`（0xfe03）报 `KeyAltRight`，Mod5 不映射到任何修饰键，Mods 不含 Ctrl。判断逻辑在 `canvas_keys.go` 纯 Go 单测覆盖。
- **tan dev 窗口位置不再闪**：根包新增内部钩子 `App.onPrepareSurface`（`surface_prepare.go`），WebView 与 canvas Surface 在创建前调用（`createSurface`、`createCanvasSurface` 各一行）；`devstate.go` 用它把记录的 X/Y/Width/Height/State 写进 SurfaceOptions，窗口直接在原位置创建，不再创建后 SetBounds。记录位置恰为 (0,0) 时（SurfaceOptions 里表示居中）仍走创建后 SetBounds。
- 验收（macOS）：smokecanvas 新增 `drop-files`、`drop-feedback`、`drop-refused`、`edit-menu`；smokeattach 新增 `hit-alpha`、`web-hover`、`drop-files`、`edit-menu`、`anchor`。拖放用伪造的 NSDraggingInfo 对象合成。Windows smokecanvas 新增 `drop-cancel`、`file-drop`（待实机）；Linux 拖放与两端 AltGr 只有单测。

## GPU 呈现（2026-11-13）

GPU 渲染器（[gpu-renderer.md](gpu-renderer.md) 6.5）通过 canvas 的**可选接口**拿到原生呈现目标。核心用类型断言发现它；没实现或返回错误时，窗口留在 CPU 路径，CPU 路径（`Present(img)`）不变。末尾"不做"一节的第一条（GPU 渲染）已由 GPU 里程碑 1–3 取代，原文保留。

### macOS：`CanvasMetal`（已实现，里程碑 1）

```go
type CanvasMetal interface {
    UseMetalLayer() (layer uintptr, err error) // CAMetalLayer*
}
```

语义（摘自 `internal/platform/canvas.go` 的文档注释）：

- 把 view 的 backing layer 换成 `CAMetalLayer`，只换一次，并返回该 layer。layer 归 view 所有，在 surface 关闭前有效，需要更久时自己 retain。再次调用返回同一个 layer。只能在主线程调用；它本身不调用 sink，但会安排一次 `CanvasFrame`，所以切换后不需要 Invalidate，第一帧 GPU 画面也会到来。
- 帧调度不变：`CanvasFrame` 仍然只在 Invalidate/InvalidateAt、尺寸或缩放变化、或这次切换之后到来，每个显示刷新周期最多一次，静止的窗口不出帧。输入、输入法、光标、焦点的行为都与原来一样。
- 平台负责：layer 的 frame 跟随 view 的 bounds；`contentsScale` 跟随 backing scale（在报告 `CanvasResized` 之前更新）；opaque；关闭隐式动画。其余都归调用方（GPU 呈现器）：device、pixelFormat、framebufferOnly、colorspace，以及 drawableSize。drawableSize 必须在 `CanvasResized` 时或每次 `CanvasFrame` 开始时设为 `Size()*Scale()` 设备像素。
- 调用成功后，`Present(nil)` 表示"GPU 已经呈现"，返回 nil。`Present(img)` 仍然可以用作 CPU 回退（例如 GPU 呈现器起不来）：图像显示在 drawable 之上的 sublayer 里，不经过 Metal 拷贝；下一次 `Present(nil)` 会再把它隐藏。
- 附着的 canvas（`CanvasAttacher`）、透明 canvas（`Appearance.Transparent` 或 Material）以及 PassThrough 层都返回 `ErrUnsupported`。在非主线程调用，或 surface 关闭后调用，返回错误。

uihost 胶水（`uihost/gpu.go`，按 GOOS 拆分，见下文 Windows 一节）：`setupGPU` 在 macOS 上断言 `CanvasMetal` 后调用 `metal.NewPresenter(layer)`，得到 `paint.Renderer` 和 `present(size, scale)`。runner 用包装后的 `gpuRenderer` 渲染，然后调用 `Window.Present(nil)`，由 `presentFrame` 依次调用 `present` 和 `cv.Present(nil)`。错误处理如下：
- `ErrTargetTooLarge`：这一帧临时在 CPU 上画，之后每帧仍先试 GPU。日志行带上错误文本（纹理超限与实例数超限都包装这个错误）。
- `ErrDeviceLost`：这个窗口从此永久改用 `paint.CPU`，并 Invalidate 一次。
- 呈现器创建失败：窗口留在 CPU 模式，canvas 保留 Metal layer，用 `Present(img)` 显示 CPU 帧。
- `TANGO_RENDERER=cpu`：跳过 GPU。

tan dev worker 和附着层（Attach）一直走 CPU。

### Windows：`CanvasHWND`（已实现，里程碑 3；仅交叉编译，实机清单待跑）

```go
type CanvasHWND interface {
    UseGPUWindow() (hwnd uintptr, err error) // canvas 自己的子 HWND
}
```

GPU 侧：`d3d11.NewPresenter(hwnd uintptr) (paint.Renderer, present func(size image.Point, scale float32) error, error)`（`ui/gpu/d3d11/presenter_windows.go`），形状与 `metal.NewPresenter` 相同，在该 HWND 上建 DXGI flip-model 交换链（`FLIP_DISCARD`，2 个缓冲，B8G8R8A8，`SCALING_NONE`，`DXGI_MWA_NO_ALT_ENTER`）。平台实现在 `internal/platform/windows/canvas_gpu_windows.go`，纯逻辑（WM_PAINT 做什么、兄弟窗口显示/隐藏）在无构建标签的 `canvas_gpu_plan.go`，各 OS 上都有单测。

语义（摘自 `internal/platform/canvas.go` 的文档注释，即原草案与"提议"各条，均已采纳）：

- **返回的 HWND**：就是 canvas 自己的子 HWND（类 `TangoCanvas`）。它照常接收输入、IME、光标、焦点和 UI Automation 查询，所以这些行为与 CPU 路径完全相同。HWND 归 surface 所有，surface 关闭前有效；调用方须在 surface 关闭前或关闭时释放交换链。
- **可以重复调用**：只切换一次，再次调用返回同一个 HWND。只能在主线程调用（按 `GetWindowThreadProcessId` 与当前线程比对）；非主线程、surface 关闭后调用返回错误。它本身不调用 sink，但会经消息循环安排一次 `CanvasFrame`，所以第一帧 GPU 画面不需要 Invalidate。
- **帧调度不变**：每个刷新周期最多一帧，静止窗口不出帧，InvalidateAt 与节流计时器照旧。WM_SIZE / WM_DPICHANGED_AFTERPARENT 照旧报告 `CanvasResized` 并同步出一帧；`ResizeBuffers` 由呈现器在 `present` 开头、发现尺寸变化时调用（主线程）。DPI 行为不变。
- **交换链 HWND 上永远不画 GDI**：切换后 WM_PAINT 只 `BeginPaint`/`EndPaint` 使区域有效，需要时（尺寸变化或有帧请求）调用 `CanvasFrame`，不再 BitBlt DIB；`WM_ERASEBKGND` 照旧吞掉。`Present(nil)` 表示"GPU 已经呈现"，返回 nil。
- **CPU 回退用兄弟子窗口**：切换时在 surface 的客户区里创建一个隐藏的兄弟子窗口（类 `TangoCanvasCPU`，`WS_CHILD|WS_CLIPSIBLINGS`），位置与大小同 canvas，Z 序在 canvas 之上。`Present(img)` 照旧把图像拷进 canvas 的 DIB section，然后显示（第一次，`SWP_SHOWWINDOW` + `HWND_TOP`）或重画（已显示时）兄弟窗口，都用 `RDW_UPDATENOW` 同步画，所以 live resize 不出旧帧；下一次 `Present(nil)` 把它隐藏（`SWP_HIDEWINDOW`）。兄弟窗口的 WM_PAINT 用 canvas 的 DIB 与 CPU 路径相同的 blit。它是惰性的：`WM_NCHITTEST` 返回 `HTTRANSPARENT`（同一线程的 canvas 接收指针输入、WM_SETCURSOR 和捕获），`WM_MOUSEACTIVATE` 返回 `MA_NOACTIVATE`，收到焦点就交给 canvas。兄弟窗口建不出来时 `UseGPUWindow` 返回错误、canvas 不切换（否则没有 CPU 回退）。随 surface 一起销毁。
- **切换后的帧不经 WM_PAINT 调度**（实现时补充的决定）：CPU 路径的 `paintNow` 用 `RedrawWindow(RDW_INVALIDATE|RDW_UPDATENOW)` 同步触发 WM_PAINT。切换后兄弟窗口可能盖住 canvas 的整个可见区域（canvas 有 `WS_CLIPSIBLINGS`），更新区域为空时系统不发 WM_PAINT，帧就会停下，GPU 也就回不来。所以切换后 `paintNow` 直接检查几何变化并调用 `CanvasFrame`（与同步 WM_PAINT 等价）；在 sink 回调或 paint 里则改为投递一次帧请求（不重入）。WM_PAINT 仍会运行到期的帧。窗口不可见或最小化时与 CPU 路径一样不出帧（CPU 路径此时收不到 WM_PAINT）：请求保留，恢复时由父窗口的 WM_SIZE（`fitParent`）或 WM_PAINT 补上。
- **不支持的 canvas 一律走 CPU**：附着的 canvas（`CanvasAttacher`，含分层与 PassThrough 层）和透明 canvas（`Appearance.Transparent` 或 Material，按调用时的外观判断）返回 `ErrUnsupported`。切换之后再把外观改成透明不会切回 GDI：GPU 帧保持不透明（与 macOS 一样）。
- **`NewPresenter` 失败 → CPU，经兄弟子窗口显示**：包括缺少 `d3dcompiler_47.dll`（错误包装 `d3d11.ErrNoDevice`）。canvas 保持切换状态，`gpuRenderer` 在 CPU 模式下每帧 `Present(img)`，兄弟窗口一直显示。
- **关闭顺序**：顶层窗口的 WM_DESTROY 先到，`surface.destroyed` 报告 `SurfaceClosed`，uihost 在同一调用里关闭 `gpuRenderer`（释放交换链），之后系统才销毁子窗口（canvas 与兄弟窗口），即"先释放交换链，再 `DestroyWindow`"。

uihost 胶水的文件组织：共享逻辑（`gpuRenderer`、`setupGPU`、错误处理、`metalTarget`/`hwndTarget`）在无构建标签的 `uihost/gpu.go`；`gpu_darwin.go` 选 `CanvasMetal` + `metal.NewPresenter`（行为与里程碑 1 相同），`gpu_windows.go` 选 `CanvasHWND` + `d3d11.NewPresenter`，`gpu_other.go` 一律 CPU（Linux GL 留到里程碑 4）。错误判断改用 `gpu.ErrTargetTooLarge` / `gpu.ErrDeviceLost`（与 metal、d3d11 再导出的值相同）。测试用 `SetGPUHooksHWND` 在任何 OS 上走 HWND 路径（fake 后端的 `EnableCanvasHWND`）。

实机验收（[gpu-renderer-m3-report.md](gpu-renderer-m3-report.md) 第 5 节的 12 项）尚未进行，平台侧额外需要看：兄弟窗口能否盖住 flip-model 交换链（DWM 对子窗口交换链与 GDI 兄弟窗口的 Z 序）；兄弟窗口显示时指针、光标、滚轮、IME 候选窗是否仍正常；UIA 树里是否多出一个空的 pane（兄弟窗口显示时）；遮挡/最小化/恢复后画面是否正确。

#### 其他注意（GPU 侧既有行为）
- 呈现器设置 `SetMaximumFrameLatency(1)`，并以 vsync 呈现（`Present1(1, 0, …)`），`present` 在已有一帧排队时阻塞。平台自己的刷新节流（每个刷新周期最多一帧）照常有效。
- 窗口被遮挡时，`Present1` 返回 `DXGI_STATUS_OCCLUDED`，按成功处理。呈现器不用 `DXGI_PRESENT_TEST` 轮询遮挡状态；被遮挡期间的 CPU 占用靠平台的每刷新周期最多一帧来限制。
- 连续失败：`present` 有自己的连续失败计数（成功的 `Render` 不会清零它），连续 3 次失败（例如 `ResizeBuffers` 一直返回 `DXGI_ERROR_INVALID_CALL`）返回 `gpu.ErrDeviceLost`，胶水据此永久回退 CPU，不会一直空转。
- 实例数超过设备实例缓冲上限（约 93 万个实例）时，`Execute` 返回包装 `gpu.ErrTargetTooLarge` 的错误，与纹理超限的处理相同（这一帧临时在 CPU 上画），但错误文本是 "instance count exceeds the device buffer limit"。`uihost/gpu.go` 的日志行已改为 "exceeds a GPU limit, drawing on the CPU until it fits: <err>"，可以区分两种原因。
- 关闭顺序：先关闭渲染器（释放交换链），再 `DestroyWindow`。平台侧按此顺序（见上文"关闭顺序"）；反过来不应崩溃，但未实机验证。

Linux（GL）的对应接口留到 GPU 里程碑 4。

## 脏矩形上屏（DamagePresenter，已实现 2026-11-13）

`ui` 包新增可选接口（`ui/window.go`，不改 `Window`）：

```go
type DamagePresenter interface {
    PresentDamage(img *image.RGBA, dirty []image.Rectangle) error
}
```

- Runner 在 CPU 渲染器下、且本帧带有重绘提示（组件声明了 `Context.Damage` / `ScrollShift`）时调用它代替 `Present`。`img` 仍是完整的整帧（CPU 仍整帧光栅化，局部光栅化不在本期），`dirty` 是相对上一次上屏有变化的设备像素矩形（已裁到 `img.Bounds()`，向外取整），宿主若自己保留了上一帧的副本，可以只拷贝这些区域（`CALayer` 局部 `setNeedsDisplayInRect`、`StretchDIBits`/`BitBlt` 子矩形、`cairo` 局部 `queue_draw_area`）。
- 首帧、尺寸/scale/主题/ReduceMotion 变化、组件什么都没声明的帧仍调用 `Present`；GPU 渲染器路径不调用（提示随 `paint.List` 的 `Damage`/`ScrollRegion` op 交给渲染器）。
- `img` 的所有权与 `Present` 相同（返回前复制或上传）。实现该接口的宿主必须保证上一帧内容仍在（交换链/后备缓冲被丢弃时应退回整帧拷贝）。
- 平台侧（2026-11-13 实现）：`internal/platform/canvas.go` 新增可选接口 `CanvasDamagePresenter`（签名同上，脏区为设备像素；不能复用上一帧时由 canvas 自己整帧上屏；返回错误时调用方再整帧 `Present`），以及辅助函数 `DamageRects`（裁剪、去空、矩形过多或面积超过一半时要求整帧）和 `DamageToLogical`（设备像素矩形向外取整到逻辑像素）。
- uihost：`window.PresentDamage` 在 canvas 支持时转发；首帧、`CanvasResized` 之后的第一帧、图像尺寸变化、上一帧上屏失败后一律整帧；`PresentDamage` 出错时本帧改为整帧 `Present`，连续 3 次出错后该窗口不再尝试局部上屏。GPU 路径不变。
- macOS：整帧仍是视图 layer 的 contents；局部帧把帧切成 256 设备像素的瓦片，只给被脏区碰到的瓦片建 CGImage、作为子 layer 盖在上面（同一 CATransaction），下一次整帧把瓦片全部隐藏。Metal 模式、瓦片含非不透明像素、瓦片面积超过一半时整帧。
- Windows（GDI）：DIB 只转换脏区；在 WM_PAINT 内（paintNow 同步触发）只 BitBlt 脏区和 paintNow 之前系统已挂起的更新矩形（`GetUpdateRect`），WM_PAINT 外则只 `InvalidateRect` 脏区。UseGPUWindow 之后、DIB 尺寸不符、本次绘制中几何变化时整帧。
- Linux（GTK 3）：cairo surface 只转换脏区，`gtk_widget_queue_draw_area`（逻辑坐标）只重画脏区。为此普通帧改在 frame clock 的 tick（update 阶段）里渲染，尺寸变化和首帧仍在 draw 里渲染（整帧）。
- 附着层（Attach）三个平台都屏蔽局部路径，一律整帧（命中遮罩/分层位图只按整帧维护）。
- Windows / Linux 只交叉编译通过，局部上屏需实机验证（尤其 Linux Wayland/X11 下 GTK 保留无效区以外内容、Windows DWM 下的局部 BitBlt）。

## 不做

- GPU 渲染（Metal / D3D11 / OpenGL）：`RunnerOptions.Renderer` 已预留，等 CPU 路径稳定后再做。
- 多 canvas 共享同一 Runner。
