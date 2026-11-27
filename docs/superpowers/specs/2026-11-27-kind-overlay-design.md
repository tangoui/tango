# KindOverlay：不取焦点的浮层窗口（第一期 macOS）

日期：2026-11-27 · 分支 `feat/kind-overlay`（基于 main 235dd9b9，已含 sys/ime）

## 目的

用户要用 Tango 做一个对标搜狗、百度、豆包的输入法。输入法的候选窗需要浮在任意应用的光标旁边，用户的键盘焦点必须始终留在原来的输入框里。Tango 现在的窗口一显示就会抢走焦点，所以要先给框架补一种"永远不取焦点"的窗口。这个能力是通用的，悬浮工具条、字幕、截图标注提示也都能用。

## 目标

- 新增窗口类型 `tango.KindOverlay`（`platform.KindOverlay`），WebView 窗口和自绘画布窗口（`NewSurface`、`uihost.Open`）都可以用。
- 在 macOS 上，浮层窗口具备以下行为：
  1. 永远不成为 key window，也不成为 main window。显示、点击、拖动都不会把键盘焦点从别的应用（或本应用的其他窗口）抢走。
  2. 显示时不激活本应用；即使本应用是后台应用（`LSBackgroundOnly` / Prohibited，例如输入法进程），也能盖在当前前台应用的窗口上面。
  3. 层级高于普通窗口、浮动窗口和全屏应用（输入法候选窗的常见层级），不随应用失活而隐藏。
  4. 出现在所有桌面上，也能出现在其他应用的全屏空间里。
  5. 鼠标可用：第一次点击直接生效（不需要先点一下"激活"），应用不在前台时也能收到悬停、移入移出事件，可以用来高亮候选词。
  6. 默认无边框、没有标题栏、不出现在窗口菜单和 Mission Control 中，也不能被拖成全屏。
- 在 Windows、Linux（以及移动端）上，创建 `KindOverlay` 会返回 `ErrUnsupported`。不会悄悄退回成会抢焦点的普通窗口，那样会让候选窗把用户的输入打断。
- 更新示例 `examples/inputmethod`：用一个自绘浮层显示候选词，并通过 `Session.Caret()` 定位到光标下方，以此证明这个能力能撑起候选窗。

## 非目标

- Windows 的 `WS_EX_NOACTIVATE`、Linux 的 X11 override-redirect / Wayland 实现留到下一期，本期只保证接口能套得上。
- 不提供中途切换：已经创建的普通窗口不能改成浮层，浮层也不能改回普通窗口。
- 候选窗的视觉设计、分页、竖排横排切换不在本期。示例只做到能显示、能点击选词。
- 不做"浮层上的输入框接收键盘"。浮层永远不接收键盘输入。

## 反目标

- 不能改变现有任何 Kind（Window、Panel、Modal、Sheet）的行为。
- 浮层出现、点击或隐藏时，前台应用不能失去焦点，也不能出现闪烁、标题栏变灰之类的焦点切换痕迹。
- 不能因为显示浮层，让输入法进程出现在 Dock 或 Cmd+Tab 里。

## 约束

- 只用 purego，不加 .m 文件。
- 接口按跨平台设计：浮层的行为约定写在 `KindOverlay` 的文档注释里，各平台照着实现。
- `SurfaceOptions` 里与浮层冲突的选项按以下规则处理，写进文档：
  - `TitleBar` 一律当作 `TitleBarNone`；
  - `Resizable` 被忽略；
  - `AlwaysOnTop` 被忽略，浮层本来就在更高层级；
  - `Parent` 不需要，设置了也被忽略；
  - 对浮层调用 `Focus()` 不做任何事，`IsFocused()` 永远返回 false；
  - `Show()` 不激活应用。

## 思路比较

1. **新增 `KindOverlay`（选用）**：不取焦点、不激活、高层级、所有桌面、全屏辅助、鼠标可用，这几项只有一起才有意义，打包成一种窗口类型最不容易用错。它和 `KindPanel`、`KindModal` 并列，以后接其他平台也不用改接口。
2. 在 `SurfaceOptions` 加 `NoFocus` 开关：可以自由组合，但配套设置得由应用自己记得一起打开，漏一项就会出问题，比如全屏应用里候选窗消失。
3. 在 `sys/ime` 里提供专用候选窗组件：对输入法开发者最省事，但别的场景用不上，而且把窗口能力藏进了输入法包，不符合 Tango 的分层。

用户选了 1。

## macOS 实现要点（已经确定的部分）

- 窗口类：新增一个 NSPanel 子类，比如 `TangoOverlay`，覆盖 `canBecomeKeyWindow` 和 `canBecomeMainWindow`，都返回 NO。样式为 `NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel`。
- `Show()`：浮层改走 `orderFrontRegardless`，不调用 `makeKeyAndOrderFront:`，也不受 `launchBackground` 分支影响。
- 层级：使用菜单之上、光标之下的层级，按 Squirrel 等输入法的做法取 `CGWindowLevelForKey(kCGCursorWindowLevelKey) - 1` 或 `NSPopUpMenuWindowLevel`，由实现者实测在全屏应用和 Spotlight 下的表现后选定，并在代码注释中写明理由。
- collectionBehavior：`CanJoinAllSpaces | FullScreenAuxiliary | Stationary | IgnoresCycle | Transient`，并且不加 `FullScreenPrimary`。
- `setHidesOnDeactivate:NO`，`setFloatingPanel:YES`。
- 自绘画布：浮层窗口的 NSTrackingArea 用 `NSTrackingActiveAlways`（0x80）替代 `ActiveInActiveApp`（0x40），这样后台应用也能收到悬停事件。`acceptsFirstMouse:` 已经返回 YES，保持不变。WebView 浮层也需要第一次点击就生效，由实现者核对 WKWebView 的行为。
- 核心层（`surface_ext.go`）：`KindOverlay` 交给 backend 前，先检查 backend 是否声明支持（例如实现 `platform.OverlaySupport` 标记接口）；不支持时 `NewSurface` 和 `uihost.Open` 返回包装了 `ErrUnsupported` 的错误。
- 窗口事件：浮层不会产生 focus 和 blur 事件。

## 验收

- **单元测试**：创建浮层后，`canBecomeKeyWindow` 为 NO、层级符合预期、collectionBehavior 包含上述各项；Show 之后 `isKeyWindow` 为 NO，`NSApp.isActive` 不变；对浮层调用 Focus 不改变 key window；不支持的后端返回 ErrUnsupported。
- **真实场景（需要用户同意短暂占用前台）**：
  - 文本编辑在前台打字时，输入法进程显示候选浮层，文本编辑保持 key 状态（光标仍在闪、继续打字照常进入文本编辑）；
  - 在浮层上点击候选词，能选中并上屏，文本编辑不失焦；
  - 全屏应用里浮层可见；
  - 输入法进程不出现在 Dock 和 Cmd+Tab 中。

## 留给实现者

- 层级取值、WKWebView 第一次点击的处理细节。
- 浮层在多显示器之间的定位沿用现有的 `SetBounds` 坐标系（逻辑像素，原点在主屏左上角），不新增 API。
- 示例里候选窗的具体画法用 ui 包现有组件，按 antd 6.7 基线取令牌。

## 实现者审查后补充的决定（均为作者的决定，用户可推翻）

1. **WebView 浮层的悬停**：只保证能创建、不抢焦点、第一次点击就生效。应用在后台时的悬停、`:hover`、选区颜色尽力而为，限制写进 `KindOverlay` 文档。完整的后台悬停只承诺给自绘画布浮层（tracking area 改为 ActiveAlways）。理由：候选窗用自绘画布实现；WKWebView 内部的跟踪区域改不了，自己合成事件代价大。
2. **`tan dev` 热重载**：创建浮层返回 `ErrUnsupported`，开发代理 backend 不声明支持。理由：输入法本来就跑不了 dev 模式；通过 devhost 协议转发 Kind 是额外的一整块工作，等有需要再做。
3. **应用启动时的激活**：本期只保证"浮层窗口本身显示、点击时不激活应用"。一个普通的、未打包的 Regular 应用（比如 `go run` 示例）启动时仍会照旧激活一次，这一点写进文档。输入法打包后是 `LSBackgroundOnly`，不受影响。理由：改启动激活会碰到应用级的生命周期，影响面大，留到真正有"只有浮层的普通应用"需求时再做。
4. **运行中修改外观和状态**：对浮层调用 `SetAppearance` 时套用和创建时相同的规则。`Transparent`、`Background`、`Material`、`NoShadow`、`Opacity` 生效；`TitleBar`、`AlwaysOnTop`、系统按钮相关字段被忽略。`applyAppearance` 必须为浮层保留 Borderless、NonactivatingPanel 样式、浮层层级和 collectionBehavior，不能把它们重置掉。`SetState` 只接受 Normal（最小化、最大化、全屏返回 `ErrUnsupported`）。`SetResizable` 被忽略。测试要覆盖"运行中 SetAppearance 之后，浮层特性仍然保留"。
