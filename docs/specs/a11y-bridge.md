# 自绘 UI 无障碍的平台桥接（a11y bridge）

状态：接口已确认（min-a6，2026-11-13），实现中。

`ui` 层每帧生成平台中立的语义树（`ui/a11y`，约定见 `docs/accessibility.md`），本设计把它接到桌面平台的 canvas（`internal/platform/canvas.go`）上，由各后端映射到 NSAccessibility、UI Automation、AT-SPI2。iOS/Android 的自绘页后来也接入了同一接口（`CanvasAccessible`），见下文实现状态的"移动端"一项与 [mobile-canvas.md](mobile-canvas.md) §5。

## 分层

```
ui                      Runner 生成 a11y.Tree；ui.AccessibilityHost（可选，Window 实现）
uihost/a11y.go          window 实现 AccessibilityHost，转给 CanvasAccessible；
                        host 实现 CanvasAccessibleSink，把动作交给 Runner.PerformAction
internal/platform       canvas.go 末尾新增两个可选接口（下文 ①）
internal/platform/a11ybridge   新包，纯 Go：角色/状态映射、焦点与 live 计算、命中测试、坐标换算（可单测，各平台共用）
darwin/windows/linux    canvas_a11y*.go：原生对象树；已有 canvas 文件只加转发
fake                    canvas_a11y.go：记录调用、模拟辅助技术接入与动作
```

不改 `ui/`、根包、`cmd/`、`go.mod`；已有接口签名一律不变。

## ① 平台契约（追加在 internal/platform/canvas.go 末尾）

```go
// 由 Canvas 实现（CanvasCreator 和 CanvasAttacher 返回的都可以），核心用类型断言发现。
type CanvasAccessible interface {
	AccessibilityActive() bool                                       // 任意 goroutine，廉价（原子读）
	SetAccessibilityTree(tree *a11y.Tree)                            // 主线程；整棵替换，nil = 丢弃
	UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID) // 主线程；增量
	AccessibilityFocusChanged(id a11y.NodeID)                        // 主线程；0 = 焦点离开自绘内容
	AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) // 主线程；source 0 = Context.Announce
}

// 由 CanvasSink 可选实现（uihost 的 host 实现），后端只在主线程调用。
type CanvasAccessibleSink interface {
	AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error
}
```

`CanvasAccessible` 不内嵌 `Canvas`（同 `CanvasMetal`，已确认）。

约定（Go 注释里写全）：

- **激活**：后端在检测到辅助技术后把 `AccessibilityActive` 置 true 并调用自身的 `Invalidate`。下一帧核心调 `SetAccessibilityTree(全树)`，之后每个有变化的帧调 `UpdateAccessibilityNodes`。后端判定辅助技术离开时置 false；核心在下一帧调 `SetAccessibilityTree(nil)`，Runner 也停止建树。关窗/Detach 后核心不再调用；后端在 destroy 里自行释放原生对象。
- **快照**：`tree` 不可变、调用返回后仍有效，后端直接保存为数据源；`changed` 的语义与 `ui.AccessibilityHost` 相同（先增改、后删除，`tree.Node(id)==nil` 即删除）。旧快照由后端自己保留，用来判断哪些字段变了（值、名称、状态）以决定发哪种原生通知。
- **一帧内的顺序**：Set/Update → FocusChanged（仅在有效焦点变化时）→ 零到多次 Announce。全部在 `CanvasFrame` 内、`Present` 之前调用。后端在这些方法里不得回调 sink；需要发的原生通知可以立即发，也可以排到帧结束后发。
- **有效焦点**：核心传入的 id 已解析 `ActiveDescendant`（组合框、树、表格的当前项），后端不用再算。
- **播报**：核心把 `tree.Announcements`（source 0）和 live 区域的内容变化（source 为 live 根节点）都转成 `AccessibilityAnnounce`。激活后的第一棵树不播 live 区域。原生支持 live 区域的平台（UIA）用 source 发原生事件，否则朗读 text。
- **动作**：后端收到辅助技术请求后在主线程调用 `AccessibilityAction`；它校验并排队（`Runner.PerformAction`），返回 nil 不代表已经执行，组件在随后的一帧处理。`a11y.Focus` 时后端先让 canvas 取得键盘焦点（makeFirstResponder / SetFocus / gtk_widget_grab_focus）再调用。请求来自其他线程时后端先回主线程（macOS `runOnMain`，Windows 投递窗口消息，Linux `g_idle_add`）。不在 `CanvasFrame` 内调用。

### 减少动态效果（与无障碍接口分开，追加在 internal/platform/ext.go 末尾）

```go
type MotionPreference interface { ReduceMotion() bool }      // Backend 可选能力，任意 goroutine
const AppReduceMotionChanged AppEventKind = 200               // 信号，无负载；避开桌面 1… 与移动端 100… 段
```

后端在系统设置变化时经 `HostEx.OnAppEvent(AppEvent{Kind: AppReduceMotionChanged})` 上报，与主题变化同一路径。读取：macOS `NSWorkspace.accessibilityDisplayShouldReduceMotion`，监听 `NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification`（回调一律经 `runOnMain`）；Windows 在 `integration_windows.go` 的 `systemMessage`（顶层 `WM_SETTINGCHANGE` 统一处理点）每次重读 `SPI_GETCLIENTAREAANIMATION`，值变了才发；Linux 读 `GtkSettings` 的 `gtk-enable-animations`，监听 `notify::gtk-enable-animations`。uihost 侧在 `uihost/motion.go`（follow.go 旁）：窗口打开时 `r.SetReduceMotion(ropts.ReduceMotion || mp.ReduceMotion())`，`app.OnEventKind(AppReduceMotionChanged)` 时对该 app 的所有窗口重算；应用显式开启的 `RunnerOptions.ReduceMotion` 优先。

## ② uihost/a11y.go

### 接线

- `window`（`uihost.go` 的 ui.Window 实现）在 a11y.go 里新增方法 `AccessibilityEnabled()` 与 `AccessibilityUpdate(tree, changed)`，于是对 Runner 而言它就是 `ui.AccessibilityHost`。canvas 不支持 `CanvasAccessible` 时 `AccessibilityEnabled` 恒 false，零开销。
- `host` 在 a11y.go 里实现 `CanvasAccessibleSink`。
- `uihost.go` 只加两处：`window` 结构加字段 `ax *a11yBridge`；`Open` 里 `r.Attach(&window{…, ax: newA11y(cv, r, ropts)})`。`attach.go` 的 `AttachWith` 同样一行。`close()` 里一行 `h.ax.close()` 由 a11y.go 提供（也可以挂在 `s.OnClosed`，免改 close）。

### 语义树的消费者：订阅点（扇出）

uihost 里语义树有两个消费者：平台桥接（本文）和整机自动化测试接口（min-43，`uihost/automation*.go`）。两者都从 `a11yBridge` 的同一个订阅点取树，谁也不覆盖谁；平台桥接本身也只是其中一个订阅者。a11y.go 提供（包内，主线程）：

```go
// a11yTreeFunc 收到每次更新：tree 为最近一帧的快照（不可变），changed 同 ui.AccessibilityHost。
// 语义树被丢弃时（不再有人需要）收到 (nil, nil)。
type a11yTreeFunc func(tree *a11y.Tree, changed []a11y.NodeID)

// a11yFor 返回 Open / AttachWith 创建的 runner 对应的桥（没有则 nil）。
func a11yFor(r *ui.Runner) *a11yBridge

// tap 订阅更新，返回取消函数（幂等）。已有树时先同步回调一次 fn(当前树, 全部节点 ID)，
// 晚到的订阅者也拿到完整基线；之后每个有变化的帧回调一次。订阅本身不开启建树。
func (b *a11yBridge) tap(fn a11yTreeFunc) (cancel func())

// force 强制建树（引用计数）：自动化运行时没有真正的辅助技术，平台不会请求开启。
// 调用即 Invalidate，下一帧开始出树；release 后计数归零且平台未激活时停止建树。
func (b *a11yBridge) force() (release func())
```

- 扇出顺序：先平台桥接，再按订阅顺序调其他订阅者；某个订阅者 panic 不影响其他（`corehook.Guard`）。订阅者在回调里不得修改树；取消订阅可在回调中进行。
- 不复用 `Runner.EnableAccessibility`：那是单个布尔量，应用或测试也可能调用，两个消费者共用会互相覆盖。uihost 自己计数，结果反映在 `AccessibilityEnabled()` 上。
- 平台桥接只在平台真的激活时收到树：仅因 `force` 建树时不调 `CanvasAccessible` 的任何方法（不在没有辅助技术时创建原生对象、发通知）。平台随后激活时，桥接首先收到 `SetAccessibilityTree(当前整树)`。
- 强制开启不改 ui 层：uihost 的 `window` 在 `forced > 0` 时直接对 Runner 报告 `AccessibilityEnabled() = true`。
- tan dev（热加载）：worker 侧 `remoteWin` 实现 AccessibilityHost（`uihost/remote_a11y_dev.go`），宿主窗口的 canvas 用同一个 `a11yBridge`（无 Runner，只走平台一侧）。协议见 `internal/devhost/proto_a11y.go`（`ProtoVersion` 升为 2）：宿主在 CanvasFrame 开头检测到平台激活后发 `A11yState{On, Epoch}`（新 worker 打开/接管窗口时也发），worker 先发整树、之后发增量 `A11yTree`（只在 Runner 报告变化时发，带 Epoch，宿主丢弃旧 Epoch 的）；辅助技术动作经宿主校验后以 `A11yAction` 回传，worker 调 `Runner.PerformAction`。宿主为窗口的每个 worker 各存一棵树，只把 owner 的交给平台；重载 commit 后先 `SetAccessibilityTree(nil)` 再整树设新树（焦点随之清零再按新树报告），新 worker 尚无树或 owner 断开时只清空。worker 侧的 `a11yFor` 仍返回 nil（自动化 tap/force 不支持热加载窗口）。

### 开关时机

```
AccessibilityEnabled()  = b.forced > 0 || (ca != nil && ca.AccessibilityActive())   // 每帧开头，廉价
AccessibilityUpdate(tree, changed):
    if 平台激活:
        if !b.sent { ca.SetAccessibilityTree(tree); b.sent = true } else { ca.UpdateAccessibilityNodes(tree, changed) }
        f := a11ybridge.EffectiveFocus(tree); if f != b.focus { ca.AccessibilityFocusChanged(f) }
        for a in tree.Announcements { ca.AccessibilityAnnounce(a.Text, a.Live, 0) }
        for 每个 live 根 in a11ybridge.LiveChanges(prev, tree, changed) { ca.AccessibilityAnnounce(text, live, root) }
    for 每个 tap { fn(tree, changed) }
帧开头发现平台由激活变为未激活：ca.SetAccessibilityTree(nil)，b.sent=false（force 仍在时树继续给订阅者）
帧开头发现 AccessibilityEnabled 整体由 true 变 false：Runner 丢树，各订阅者收到 (nil, nil)
```

- Runner 只在树有变化时调 `AccessibilityUpdate`，所以静止界面零调用；订阅者需要“当前树”时读 `Runner.AccessibilityTree()` 或依赖 `tap` 的首次同步回调。
- `LiveChanges`：变化（新增或修改）的节点自身或祖先 `Live != LiveOff` 时，取最近的 live 祖先为根，同一帧同一根只报一次；文字为 `tree.Text(root)`，与上一帧该根的文字相同则不报。激活后首帧不报。

### 线程模型

| 步骤 | 线程 |
|---|---|
| 建树、Diff、`AccessibilityUpdate` → `CanvasAccessible` 各方法 | UI 线程 = 应用主线程（`CanvasFrame` 内） |
| 原生查询（AX 属性、UIA provider 方法、AT-SPI D-Bus 方法） | 主线程：macOS AppKit 本就在主线程；Windows provider 返回 `ProviderOptions_UseComThreading`，UIA 把调用编组进主线程 STA；Linux 在主线程 GMainContext 上注册 D-Bus 对象 |
| `AccessibilityAction` → `Runner.PerformAction` | 主线程（= UI 线程，直接调用）；动作在下一帧开头执行 |

后端只读已保存的快照，不在查询里回调 Runner。即使三平台查询都落在主线程，快照指针仍用 `atomic.Pointer` 保存，防止以后的离线程查询读到撕裂状态。

### 坐标换算

`Node.Bounds` 是 canvas 内容区左上角起的逻辑像素。换算在**后端、查询发生时**做，窗口移动、跨屏、缩放变化都不需要重发树：

| 平台 | 节点 → 屏幕 | 屏幕点 → 节点（命中测试） |
|---|---|---|
| macOS | 视图是 flipped，逻辑像素 = 点：`[view convertRect:r toView:nil]` → `[window convertRectToScreen:]`（Cocoa 屏幕坐标，左下原点；与 `caretScreenRect` 同法）。Retina 不乘 scale | `convertPointFromScreen:` → `convertPoint:fromView:nil` |
| Windows | `x = round(X·scale) + ClientToScreen(canvas HWND)`，scale = DPI/96（per-monitor），得物理屏幕像素 | `ScreenToClient` 后除以 scale |
| Linux | `GetExtents(coordType)`：WINDOW = `gtk_widget_translate_coordinates(widget, toplevel)` + X；SCREEN 再加 `gdk_window_get_origin`（Wayland 下取不到全局原点，返回窗口坐标，与 GTK 自身一致）；GDK 逻辑像素，不乘 scale | 反向同上 |

叠加层（Attach）的 canvas 自有原点，上面的换算从 canvas 自身的视图/HWND/widget 出发，自动包含 Layer 偏移与 Anchor 变化。

`a11ybridge` 提供纯函数：`ToScreen(r, originX, originY, scale)`、`FromScreen`、`HitTest(tree, x, y)`（逆序遍历子节点使上层浮层优先，跳过 `Offscreen`，返回最深命中），供三个平台复用并单测。

## ③ 各平台实现要点与最小接入点

### 共用：internal/platform/a11ybridge（新包，纯 Go）

- 角色：解析 `a11y.Mappings`（"AXGroup/AXApplicationDialog" 拆成 role+subrole），再按 `docs/accessibility.md` 第 3 节的补充说明做覆盖（切换按钮、多行、密码框等）；UIA ControlType 名称 → `UIA_<X>ControlTypeId` 数值表；AT-SPI 角色名 → `ATSPI_ROLE_<X>` 数值表。测试保证 Mappings 里每个名字都能查到数值。
- 状态：`a11y.State` → AT-SPI 状态位集合（两个 uint32）、UIA 布尔属性、AX 属性。
- 焦点、live 计算、命中测试、坐标换算（见上）。

### macOS：darwin/canvas_a11y.go

- 节点对象：运行时注册 `TangoA11yElement : NSAccessibilityElement`，按节点缓存（`map[NodeID]id`，保留引用）；实现 `accessibilityRole/Subrole/RoleDescription`、`Label`（Name）、`Help`（Description）、`Value`（文本、勾选 0/1/2、范围值）、`MinValue/MaxValue`、`PlaceholderValue`、`isAccessibilityEnabled/Focused/Selected/Expanded/Required`、`accessibilityParent/Children`（按快照的 ID 现查现建）、`accessibilityFrame`（查询时按上表换算）、`accessibilityPerformPress/Increment/Decrement/Cancel/ShowMenu`、`setAccessibilityValue:`、`setAccessibilityFocused:`、`accessibilityPerformScrollToVisible`（方法名以 NSAccessibility 协议为准）、`isAccessibilitySelectorAllowed:`（按 `Node.Actions` 过滤）。
- 通知：焦点 `NSAccessibilityFocusedUIElementChangedNotification`（只在 canvas 是第一响应者时发）；值/名称/选中变化发 `ValueChanged`/`TitleChanged`/`SelectedChildrenChanged`；结构变化给父元素发 `LayoutChanged`；删除发 `UIElementDestroyed` 并释放；播报 `NSAccessibilityPostNotificationWithUserInfo(view, AnnouncementRequested, {Announcement: text, Priority: High(assertive)/Medium(polite)})`。
- 激活：视图第一次被问 `accessibilityChildren`/`accessibilityHitTest:`/`accessibilityFocusedUIElement` 时置 active 并 Invalidate，此时先回空数组；首棵树到达后对视图发 `LayoutChanged`，VoiceOver 会重新读取。第一期不做自动停用（AppKit 没有可靠的“断开”信号）。
- **对已有文件的最小接入**：
  - `canvas_view.go`：`methods = append(methods, canvasA11yMethods()...) // canvas_a11y.go` 一行，与 `canvasDropMethods` 同法。转发的方法：`isAccessibilityElement`（YES）、`accessibilityRole`（AXGroup，代表 tree.Root）、`accessibilityLabel`、`accessibilityChildren`、`accessibilityHitTest:`、`accessibilityFocusedUIElement`。
  - `canvas.go`：`canvas` 结构加一个字段 `ax *canvasA11y`；`destroy()` 里一行 `c.a11yDestroy()`。`CanvasAccessible` 的方法写在 canvas_a11y.go 里挂到 `*canvas` 上。

### Windows：windows/canvas_a11y_windows.go

- COM 对象（Go 写 vtable，`syscall.NewCallback`，沿用 `com_windows.go` 的引用计数思路）：canvas 根实现 `IRawElementProviderSimple` + `IRawElementProviderFragment` + `IRawElementProviderFragmentRoot`；每个节点一个 Fragment 对象（Simple + Fragment），按需实现模式：`IInvokeProvider`、`IToggleProvider`、`IValueProvider`、`IRangeValueProvider`、`IExpandCollapseProvider`、`ISelectionItemProvider`、`IScrollItemProvider`（第一期必做 Invoke/Toggle/Value/RangeValue/ExpandCollapse，其余按时间）。
- 属性：`GetPropertyValue` 返回 ControlType、Name、HelpText、IsEnabled、HasKeyboardFocus、IsKeyboardFocusable、IsOffscreen、IsPassword、LocalizedControlType、AutomationId（节点 ID）、LiveSetting、PositionInSet/SizeOfSet、Level 等；`RuntimeId = [UiaAppendRuntimeId, lo32(id), hi32(id)]`；`BoundingRectangle` 查询时换算；根的 `HostRawElementProvider` 用 `UiaHostProviderFromHwnd(canvasHWND)`，节点返回 nil；`ProviderOptions = ServerSideProvider | UseComThreading`。
- 导航：`Navigate(Parent/FirstChild/LastChild/Next/PreviousSibling)` 读快照；`ElementProviderFromPoint` 用 `a11ybridge.HitTest`；`GetFocus` 返回有效焦点。
- 事件（先 `UiaClientsAreListening`）：焦点 `UiaRaiseAutomationEvent(UIA_AutomationFocusChangedEventId)`；属性变化 `UiaRaiseAutomationPropertyChangedEvent`；结构 `UiaRaiseStructureChangedEvent`；播报 `UiaRaiseNotificationEvent`（source 非 0 且节点有 LiveSetting 时改发 `UIA_LiveRegionChangedEventId`）。删除的节点标记失效，之后的方法返回 `UIA_E_ELEMENTNOTAVAILABLE`，并 `UiaDisconnectProvider`。
- 激活：收到 `WM_GETOBJECT` 且 `lParam == UiaRootObjectId` 时置 active；第一期不自动停用。
- **对已有文件的最小接入**：
  - `canvas_windows.go` 的 `message()`：在 `if c.closed` 之前加 `case wmGetObject: if r, ok := c.a11yGetObject(wp, lp); ok { return r, true }`（返回 `UiaReturnRawElementProvider(hwnd, wp, lp, root)`）。
  - `destroy()`：一行 `c.a11yDestroy()`（`UiaReturnRawElementProvider(hwnd, 0, 0, 0)` + `UiaDisconnectAllProviders` 本 canvas 的对象）。`canvas` 结构加字段 `ax *canvasA11y`。

### Linux：linux/canvas_a11y_linux.go（AT-SPI2 over D-Bus）

- 总线：会话总线 `org.a11y.Bus.GetAddress` → `g_dbus_connection_new_for_address_sync` 连接无障碍总线；`org.a11y.Status` 的 `IsEnabled`/`ScreenReaderEnabled` 决定 active，订阅 `PropertiesChanged` 实现真正的启用/停用。
- 对象：所有对象在一棵 `g_dbus_connection_register_subtree` 子树里（一次注册、不按节点注册，在主线程的 GMainContext 上分发）。GDBus 子树只分发一层子节点，所以路径是扁平的：`/org/a11y/atspi/accessible/root`（应用根，仅备用方案用）、`…/c<canvas>`（canvas 根）、`…/c<canvas>_<nodeID>`（节点）。实现接口的必要子集：`org.a11y.atspi.Accessible`（Name、Description、Parent、ChildCount、GetChildAtIndex、GetChildren、GetIndexInParent、GetRole、GetRoleName、GetLocalizedRoleName、GetState、GetAttributes、GetRelationSet、GetInterfaces、GetApplication）、`Component`（GetExtents、Contains、GetAccessibleAtPoint、GrabFocus、ScrollTo）、`Action`（NActions、GetName、GetDescription、DoAction：click/expand/collapse/dismiss）、`Value`（MinimumValue、MaximumValue、MinimumIncrement、CurrentValue 可写）、`Text`（文本框与静态文本：CharacterCount、GetText、CaretOffset 只读子集）、`EditableText.SetTextContents`。
- 事件：`org.a11y.atspi.Event.Object` 的 `StateChanged`（focused、checked、expanded…）、`ChildrenChanged`、`PropertyChange`（accessible-name/description/value）、`Announcement`（播报；旧版 Orca 不支持时退回对 live 节点发 `TextChanged`）。
- 挂接到 GTK 的树：GTK3 进程里 atk-bridge 已把应用根注册到 `org.a11y.atspi.Registry`。canvas 用 `AtkSocket` 嵌入我们的子树：注册 `GtkDrawingArea` 子类 `TangoCanvasArea`，覆盖 `GtkWidgetClass.get_accessible` 返回一个 `AtkSocket`，再 `atk_socket_embed(socket, "<我们的唯一总线名>:<根路径>")`；atk-bridge 引用该 socket 时会指向我们的根，我们的根 `Parent` 指回 socket。进程未加载 atk-bridge（`NO_AT_BRIDGE=1` 等）时退回直接 `org.a11y.atspi.Socket.Embed` 到 Registry 根。**此处 AtkSocket 的转发行为需在 GNOME 实机确认**；`NO_AT_BRIDGE`、`GTK_A11Y=none` 或 `TANGO_ATSPI_EMBED=registry` 时走备用方案。GType 注册用 purego：`class_init` 回调里写 `GtkWidgetClass.get_accessible` 槽位，偏移按 canvas 事件结构体的做法写 Go 镜像结构体并用 `unsafe.Offsetof` 单测校验。

  **备用方案（AtkSocket 不成立或进程未加载 atk-bridge 时）**：不经 GTK，直接对 `org.a11y.atspi.Registry` 的 `/org/a11y/atspi/accessible/root` 调 `org.a11y.atspi.Socket.Embed`，把我们自己的应用根（`/org/a11y/atspi/accessible/root`，Role APPLICATION，子节点为各 canvas 根）注册成一个独立应用。缺点：GTK 自己的部分（窗口、WebKitGTK 网页）与 canvas 在屏幕阅读器里显示为两个应用；所以只作退回。
- **对已有文件的最小接入**：
  - `canvas_linux.go`：`newCanvasWidget` 只改创建控件那一行为 `g_object_new(tangoCanvasAreaType())`（类型注册在 canvas_a11y_linux.go，行为与 GtkDrawingArea 相同；事件、信号连接一律不动，拖放、AltGr、canvas_attach_linux.go 的叠放都依赖它们）；`canvas` 结构加字段 `ax *canvasA11y`；`"destroy"` 处理里一行 `c.a11yDestroy()`。

### fake：fake/canvas_a11y.go

`fake.Canvas` 实现 `CanvasAccessible`；测试辅助：`SetA11yActive(bool)`（置标志并 Invalidate）、`A11yTree()`、`A11yChanged()`、`A11yFocus()`、`A11yAnnouncements()`、`A11yAction(id, act, arg) error`（像后端一样在“主线程”调 sink 的 `AccessibilityAction`）。`fake.Backend` 实现 `MotionPreference`，`SetReduceMotion(bool)` 改值并发 `AppReduceMotionChanged`。uihost 的测试全部基于它。

## ④ 叠加层与 Metal 模式

- **Metal 模式**（`UseMetalLayer`）只换视图的 backing layer，NSView 本身、它的 AX 方法与坐标都不变；无障碍完全不受影响。GPU 帧仍然先走 `Runner.Frame` → `AccessibilityUpdate`，再 `Present(nil)`。
- **macOS 叠加层**：`TangoCanvasOverlay` 是 `TangoCanvasView` 的子类，继承全部 AX 方法；它与 `WKWebView` 同为容器视图的子视图，AppKit 的容器 `accessibilityChildren` 会同时列出两者，顺序按子视图叠放。命中测试：AppKit 默认沿 `hitTest:` 下行，而叠加层的 `hitTest:` 在透明像素返回 nil，所以穿透区域的 AX 命中自然落到网页；命中到叠加层后再由 `accessibilityHitTest:` 在节点里细分。焦点通知只在叠加层是第一响应者时发，网页有焦点时 `accessibilityFocusedUIElement` 交给 WebKit。
- **Windows 叠加层**：附加的 canvas 是子 HWND（上方时为分层窗口）；UIA 的 HWND 代理会把它当作顶层窗口的子元素，`WM_GETOBJECT` 同样送达。穿透用的 `SetWindowRgn` 也作用于 UIA 的点命中（WindowFromPoint 语义）。
- **Linux 叠加层**：canvas 是 `GtkOverlay` 的子控件，GTK 的容器 accessible 会列出它的 socket，与 WebKitGTK 的网页 accessible 并列。
- 多个 canvas（多个 Attach 或一窗多层）各自是一棵独立子树，节点 ID 只在本 canvas 内唯一；原生对象一律以（canvas, NodeID）为键。

## 验证计划（第二步）

- 单测：`a11ybridge`（映射全覆盖、状态转换、命中测试、坐标换算、live/焦点计算）；uihost + fake（激活后首帧整树、增量更新、焦点与播报顺序、动作回传到组件、停用丢树、ReduceMotion 初值与变化、两个订阅者互不覆盖、晚到订阅者拿到整树基线、仅 force 时平台不收树）；Windows/Linux 纯逻辑单测 + `GOOS=windows/linux` 交叉编译。
- macOS 实机：`examples/a11y-demo -smoke`：后台线程用 purego 调 `AXUIElementCreateApplication(getpid())` + `AXUIElementCopyAttributeValue` 读自己的树，确认按钮角色与名称、`AXUIElementPerformAction(AXPress)` 触发点击、焦点通知（`AXObserver`）。AX 客户端调用必须在非主线程（主线程要处理请求，否则自锁超时）；进程未获“辅助功能”授权（`AXIsProcessTrusted` false）时退回进程内直接调用 NSAccessibility 方法验证同一组断言，并打印 SKIP 说明。加入 `scripts/smoke.sh`；回归 smokecanvas、smokeattach、smokemetal、hello。

## 已确认的决定（2026-11-13）

1. `CanvasAccessible` 不内嵌 `Canvas`。
2. 减少动态效果移出无障碍接口，改为 `MotionPreference` + `AppReduceMotionChanged`（见 ①）。
3. Linux 用 `TangoCanvasArea` 子类型；备用方案见 ③。
4. 订阅点 `tap` 与强制开关 `force` 供平台桥接与自动化（min-43）共用；强制开启由 uihost 的 window 报告 `AccessibilityEnabled() = true`，不改 ui 层。
5. 热加载模式由 devhost 协议转发语义树（见 ②，已实现）。

## 实现状态（2026-11-13）

- 接口：`internal/platform/canvas.go` 末尾 `CanvasAccessible` / `CanvasAccessibleSink`；`ext.go` 末尾 `MotionPreference` + `AppReduceMotionChanged = 200`。根包 `canvas_hook.go` 的 `canvasSink` 转发 `AccessibilityAction`（与 `CanvasDropTarget` 同法，经 `enterMain` + `guard`）。
- 共用：`internal/platform/a11ybridge`（映射、状态、动作、焦点、live、命中测试、坐标；含 `atspi_names.go`）。
- uihost：`a11y.go`（扇出、`tap`、`force`、`a11yFor`、动作）、`motion.go`；`uihost.go`/`attach.go` 各加挂钩。host 已关闭（关窗、Detach）时动作直接返回错误；应用自己调用 `Runner.Close` 而窗口仍在的情况 ui 层没有查询接口，暂不能识别（动作会排队但不再执行）。
- macOS：`darwin/canvas_a11y.go`（`TangoA11yElement : NSAccessibilityElement`）、`darwin/motion.go`；`canvas_view.go` 一行、`canvas.go` 一个字段 + `destroy` 一行。
- Windows：`windows/canvas_a11y_windows.go`、`uia_plan.go`（纯逻辑）、`motion_windows.go`。`ElementProviderFromPoint` 与 `RangeValue.SetValue` 的 double 参数 `syscall.NewCallback` 收不到，用一小段可执行内存里的转接指令把浮点寄存器挪到整数寄存器（amd64/arm64），未在实机执行过。
- Linux：`linux/canvas_a11y_linux.go`、`atspi_plan.go`、`motion_linux.go`；`GtkWidgetClass.get_accessible` 偏移 688（GTK 3.24 gtkwidget.h）由镜像结构体单测校验。
- 移动端（mobile-canvas M4）：iOS `ios/canvas_a11y.{go,m}`（UIAccessibility 元素容器），映射在 `a11ybridge/ios.go`；Android `android/canvas_a11y.go`、`canvas_a11y_android.go` + `CanvasA11y.java`（`AccessibilityNodeProvider`），映射在 `a11ybridge/android.go`。只有单测，未记录 VoiceOver / TalkBack 实测。
- 验证：`examples/a11y-demo -smoke`（加入 `scripts/smoke.sh`）。辅助功能授权时走 AX API（后台线程），否则进程内调用 NSAccessibility；`A11Y_SMOKE_INPROCESS=1` 强制后者。
