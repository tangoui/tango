# 自绘布局里嵌入原生 WebView（WebView Embed）

状态：实验性，2026-11-14。macOS 已实现并实机冒烟；Windows、Linux 已实现，只有交叉编译与纯 Go 单测（Linux 另在容器 Xvfb 里跑过窗口测试），未在真机验证。

## 目标

`uihost.Attach` 只能把整层 canvas 叠在 WebView 上下（[canvas-surface.md](canvas-surface.md) 的 D 步）。本能力反过来：在 `uihost.Open` 的自绘窗口里，WebView 是布局中的一个组件，占据一块布局矩形，宿主每帧让原生 WebView 子视图跟随它。

```go
uihost.Open(app, uihost.Options{...}, func(c *ui.Context) {
    ui.Row(c, func() {
        ui.Scroll(c, func() { /* 自绘列表 */ }, layout.W(320))
        w := uihost.WebView(c, "docs", uihost.WebViewOptions{
            URL:     "/index.html",                       // 资源路径或绝对 URL
            Created: func(s *tango.Surface) { /* OnLoaded、ipc、Bind… */ },
        }, layout.Grow(1))
        if s := w.Surface(); s != nil { _ = s } // Navigate / Eval / Send / Find / Screenshot …
    }, layout.Fill())
}, ui.RunnerOptions{})
```

## 分层

```
ui           NativeView(c, key, opts...)：预留一块盒子，参与 Tab 焦点；Runner.NativeRegions() 帧后报告
             每个原生区域的 Rect、祖先裁剪后的 Visible、是否被浮层覆盖 Covered、是否持有 UI 焦点
uihost       WebView(c, key, WebViewOptions, opts...)：基于 NativeView；每帧 Frame 之后 sync：
             创建/放置/冻结/销毁 WebView，焦点交接
根包         webembed.go：corehook.NewEmbeddedWebView 创建"嵌入 Surface"（普通 *tango.Surface，
             消息、资源、WebView 扩展都按自己的 SurfaceID 走）；corehook.EmbeddedParent
internal/platform/webembed.go   契约：WebViewEmbedder、EmbeddedWebView、WebPlacement、RectF、EmbedGeometry
darwin / windows / linux / fake 各自实现
```

嵌入 WebView 是一个独立的 `*tango.Surface`（独立 SurfaceID），所以 `Navigate`、`Eval`、`Send`、`App.OnMessage`、ipc、自定义 scheme 资源、`Find`/`Cookies`/`Screenshot`/下载/导航策略等全部复用。窗口类方法（`SetTitle`、`SetSize`、`Bounds`…）对它无效或返回 `ErrUnsupported`。它会触发 `OnSurfaceCreated`；`uihost.EmbeddedIn(s)` 区分嵌入 Surface 与窗口（开发功能 windowstate 据此跳过）。

## 平台契约

`WebViewEmbedder.NewEmbeddedWebView(id, parent, opts, web)`：在父 Surface 内容区里建 WebView，初始隐藏，叠在 canvas 之上，后建的在上；父窗口关闭时先关闭全部嵌入视图（各报 `SurfaceClosed`）。`EmbeddedWebView`：`SetPlacement(WebPlacement{Frame, Clip})`（内容区左上角为原点的逻辑像素；Clip 为空即隐藏；不变时廉价，可每帧调用）、`FocusWeb`、`BlurWeb`；网页获得/失去键盘焦点时报 `SurfaceFocused/SurfaceBlurred`。`WebPlacement.Geometry(scale, hidden)` 把放置换算成"裁剪容器"几何：容器 = Clip（按设备像素取整），WebView 在容器内偏移 Frame-Clip.Min，三个平台共用。

| 平台 | 结构 | 状态 |
|---|---|---|
| macOS | 内容容器上加一个翻转的 `TangoEmbedHost`（铺满、`hitTest:` 对自身返回 nil，空白处点击落到 canvas），每个视图一个翻转且裁剪子视图的 `TangoEmbedClip`，里面是 `TangoWebView`。WebView 自己有一份 surface 记录（不进 `backend.surfaces`），委托、scheme、webview_ext 全部复用。焦点由 `TangoWebView` 的 `becomeFirstResponder/resignFirstResponder` 上报（显式以 TangoWebView 为 super 类调用：WebKit 给 web view 加 KVO，`SendSuper` 会按动态子类找父类而递归） | 实机冒烟通过 |
| Windows | 顶层窗口下的子窗口 `TangoWebEmbed`（`WS_CLIPSIBLINGS\|WS_CLIPCHILDREN`）作裁剪容器、WebView2 controller 的父窗口；`SetWindowPos` 到 Clip，`put_Bounds` 到 Frame（可为负），`put_IsVisible`；`GotFocus/LostFocus` 报焦点；`MoveFocus` / `SetFocus(canvas)` | 交叉编译 + 纯 Go 单测，未实机 |
| Linux | canvas 已有的 `GtkOverlay` 里每个视图一个 `GtkLayout`（有自己的 GdkWindow，天然裁剪），`get-child-position` 定位到 Clip，`gtk_layout_move` 把 web view 放到 Frame 偏移；`focus-in/out-event` 报焦点 | 交叉编译 + 单测 + 容器 Xvfb 窗口测试，未在真机桌面验证 |

## 每帧流程

1. Runner 构建、布局、绘制、Present。
2. `host.CanvasFrame` 在 `Frame` 返回后调用 `embeds.sync`：读 `Runner.NativeRegions()`，对每个本帧构建过的 WebView：首次出现时创建；计算是否需要冻结；`SetPlacement(Rect, Visible 或空)`；处理焦点进入。没再构建的 WebView：隐藏，`KeepAlive` 到期（默认 0，即立刻）后关闭。
3. 原生视图的位置与本帧画面在同一个主线程回合里提交（macOS 同一个 CATransaction），滚动、动画、窗口缩放、DPI 变化（canvas 报 Resized → 出帧）都跟随。

## 浮层冲突：快照冻结（默认 CoverSnapshot）

原生视图永远在 canvas 之上，自绘的下拉、Tooltip、Popover、对话框与它重叠时会被盖住。方案：

- `NativeRegions` 用本帧浮层根节点的屏幕矩形（外扩 16px 容纳阴影）判断与 WebView 可见部分是否相交（`Covered`）。`WebViewW.Freeze(true)` 可手动标记（主树里画在它上面的东西 ui 看不出）。
- 被覆盖且还没有快照：异步 `Surface.Screenshot()`（复用 WebSnapshot），原生视图此时仍在最上（几毫秒到几十毫秒）。
- 快照到达 → 下一帧在盒子里画快照（在浮层之下）→ 本帧 sync 确认快照已画，才把原生视图隐藏。因为"画快照"依据上一帧的覆盖状态、"隐藏原生视图"依据本帧的精确几何，两者不会出现空洞或闪烁。
- 不再覆盖 → 立即显示原生视图并丢弃快照。冻结期间网页继续运行，但更新不可见。
- 平台拿不到快照时退化为 `CoverHide`（临时隐藏，显示盒子背景）；`CoverIgnore` 保持原生视图在最上。

备选方案（未采用）：按浮层形状给原生视图设裁剪区域（macOS layer mask / Windows SetWindowRgn / GTK input+shape）——浮层的圆角、阴影、半透明遮罩都画不出来，三端实现差异大；临时缩小可见区只适合与边缘相交的浮层。

## 焦点

- `NativeView` 盒子是 Tab 停靠点（`ownKeys`，Enter/Space 不当作点击）。UI 焦点移到盒子上（Tab/程序/点击冻结的快照）→ sync 调 `FocusWeb`；若是键盘或程序移入，再 `Eval("__tangoEmbedFocus(last)")` 聚焦页面第一个（Shift+Tab 时最后一个）可聚焦元素。
- 用户点网页 → 平台报 `SurfaceFocused` → 组件把 UI 焦点设到盒子上（不回推 FocusWeb）。canvas 此时失去第一响应者（`WindowFocus{false}`），按键只到网页。
- 离开网页：注入页面的脚本在最后一个可聚焦元素上按 Tab（第一个上按 Shift+Tab）或任意位置按 F6 / Shift+F6 时发内部消息 `webembed:next|prev` → `BlurWeb`（第一响应者回到 canvas）并给 Runner 注入一次 Tab/Shift+Tab，焦点移到盒子之后/之前的控件。点击自绘区域也会把焦点交回 canvas。
- 内部消息走桥接，只对可信来源（资源 origin 或应用导航到的 origin）有效；被导航到外站的页面里 Tab/F6 交还不生效，只能点击自绘区域。

## 已知限制

- 只支持 `uihost.Open` 的窗口；`Attach` 的 canvas 与 `tan dev` 热加载宿主里 `Err()` 返回 `ErrNoEmbedding`。
- 盒子上的变换（`Box.Transform/Scale`）只按包围盒放置，不旋转/缩放网页；圆角裁剪不作用于原生视图。
- 主树里画在 WebView 盒子之上的自绘内容（兄弟节点 `Abs` 覆盖、滚动条与 WebView 重叠处）会被原生视图挡住；需要时用 `Freeze(true)`。
- 嵌入网页的无障碍树不并入 canvas 的语义树（盒子是 `RoleGroup`，网页自身对读屏可见）。
- 冻结期间的快照是静态的；快照到达前浮层有一两帧被盖住。
- macOS：canvas 的鼠标跟踪区域在 WebView 下面仍收到移动事件（ui 里落在盒子上，不影响其它控件）。
