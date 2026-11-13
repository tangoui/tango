# 触摸输入与安全区（宿主对接约定）

状态：ui 层已实现（`ui/input/touch.go`、`ui/touch.go`、`ui/pointer_capture.go`、`ui/safearea.go`）；iOS / Android 上自绘界面上屏、把平台触摸事件送进 Runner 是后续工作，本文是 ui 层与平台层之间的约定。组件作者的用法见 `ui/AUTHORING.md` 第 7 节。

## 1. 接口

```go
// ui/input：一根手指的一次变化。一帧里可以有多个（每根变化的手指一个）。
type TouchEvent struct {
	ID       int64      // 手指标识：按下到抬起/取消期间不变，期间不得复用
	Phase    TouchPhase // TouchBegan / TouchMoved / TouchEnded / TouchCancelled
	Pos      geom.Point // 逻辑像素，相对内容区左上角（与 Pointer.Pos 相同）
	Pressure float32    // 归一化 0..1（1 ≈ 正常按压）；设备不报告时填 0
	Time     time.Time  // 平台时间戳；零值表示未知，Runner 用帧时间
}

// ui：宿主侧
r.HandleEvent(input.TouchEvent{...})     // 与其他事件一样入队，UI 线程调用
r.SetTouchPointerEmulation(false)        // 宿主自己模拟鼠标时关闭 ui 的指针合成
type SafeAreaWindow interface {          // Window 的可选接口，每帧像 Size 一样读取
	SafeAreaInsets() geom.Insets
}
r.SetSafeArea(in) / r.ClearSafeArea()   // 或者由宿主在变化时推送（优先于 SafeAreaWindow）
```

`ui.Window`、`ui.Host` 签名不变；不送触摸事件、不实现 `SafeAreaWindow` 的桌面宿主行为不变。

## 2. 平台触摸事件 → TouchEvent

通用规则：

- **一根手指一个 ID**。同一帧里多根手指变化时，按平台给出的顺序依次 `HandleEvent`；Runner 在下一帧开始前一次性处理队列，所以一帧可以有多条。
- **坐标**：设备像素 ÷ `Window.Scale()`，相对 ui 内容区（不含原生标题栏、状态栏覆盖层由安全区表达，不要再减掉）。
- **不要同时送模拟鼠标事件**。Runner 会把一次手势的第一根手指合成为 `Pointer`（按下：Move+Press；移动：Move；抬起：Move+Release+Leave；取消：Cancel+Leave），没有触摸处理的组件因此照常工作。若平台（或 WebView）已经替你生成了鼠标事件，二选一：要么只送 `TouchEvent`，要么调用 `SetTouchPointerEmulation(false)` 后两者都送。
- **取消**：系统打断（来电、系统手势、视图被移走）时对每根仍按着的手指送 `TouchCancelled`；窗口失去焦点时额外送 `input.WindowFocus{Focused:false}`。
- 压力：不支持压力的设备填 0，不要填 1。

### 2.1 iOS（UIView / UIKit）

| UIKit | TouchEvent |
|---|---|
| `touchesBegan:withEvent:` | 每个 `UITouch` 一条 `TouchBegan` |
| `touchesMoved:withEvent:` | `TouchMoved`（可用 `event.coalescedTouchesForTouch:` 补齐中间点，逐条送） |
| `touchesEnded:withEvent:` | `TouchEnded` |
| `touchesCancelled:withEvent:` | `TouchCancelled` |

- `ID`：`(int64)(uintptr_t)(__bridge void *)touch`（同一根手指 UITouch 对象不变）；或在 Began 时分配递增序号、存在字典里，Ended/Cancelled 时删除。
- `Pos`：`[touch locationInView:view]`，已是点（逻辑像素）。
- `Pressure`：`touch.force / touch.maximumPossibleForce`（`maximumPossibleForce == 0` 时填 0）。
- `Time`：`touch.timestamp` 是开机以来的秒数，宿主记录一次 `timestamp` 与 `time.Now()` 的对应关系后换算。
- 视图要设 `multipleTouchEnabled = YES`，否则只收到一根手指。

### 2.2 Android（View.onTouchEvent / MotionEvent）

| MotionEvent 动作 | TouchEvent |
|---|---|
| `ACTION_DOWN`、`ACTION_POINTER_DOWN` | `getActionIndex()` 那根手指 `TouchBegan` |
| `ACTION_MOVE` | **每一根**手指 `TouchMoved`（`getPointerCount()` 遍历；历史点 `getHistoricalX/Y` 可逐条补送） |
| `ACTION_UP`、`ACTION_POINTER_UP` | `getActionIndex()` 那根手指 `TouchEnded` |
| `ACTION_CANCEL` | 所有仍按着的手指 `TouchCancelled` |

- `ID`：`getPointerId(index)`（Android 在手指抬起后才会复用该值，符合约定）。
- `Pos`：`getX(index) / density`、`getY(index) / density`（`density` 与 `Window.Scale()` 一致）。
- `Pressure`：`getPressure(index)`，按设备范围截到 0..1。
- `Time`：`getEventTime()`（`SystemClock.uptimeMillis` 基准）换算到墙钟。
- 鼠标、触控笔悬停（`getToolType == TOOL_TYPE_MOUSE`、`ACTION_HOVER_MOVE`）仍送 `input.Pointer`，不送 TouchEvent。

### 2.3 Web（wasm 宿主）

用 Pointer Events：`pointerType == "touch"` 的 `pointerdown/move/up/cancel` → TouchEvent（`ID = pointerId`，`Pressure = pressure`），并对画布 `preventDefault()` / `touch-action: none` 阻止浏览器自己的滚动和模拟鼠标事件；`pointerType` 为 mouse/pen 的仍送 `input.Pointer`。

### 2.4 桌面（Windows / Linux canvas，2026-11-13 实现，待实机验证）

- Windows：canvas 窗口处理 `WM_POINTERDOWN/UPDATE/UP/CAPTURECHANGED` 中 `GetPointerType == PT_TOUCH` 的指针（不调用 `EnableMouseInPointer`，鼠标仍走 `WM_MOUSE*`；笔仍交给 DefWindowProc 变成鼠标消息）。触摸消息返回 0、不交给 DefWindowProc，系统因此不再为该触点合成鼠标消息；仍带触摸签名（`GetMessageExtraInfo` 为 `0xFF5157xx` 且含 0x80 位）的鼠标消息被丢弃。`ID` = 指针 id；坐标为屏幕物理像素 → `ScreenToClient` → ÷ scale；`Pressure` = `POINTER_TOUCH_INFO.pressure/512`（截到 1，未报告为 0）；`Time` 由 `GetMessageTime` 换算。抬起带 `POINTER_MESSAGE_FLAG_CANCELED`、`WM_POINTERCAPTURECHANGED`、`WM_CANCELMODE` 送 `TouchCancelled`。
- Linux（GTK 3）：canvas 选 `GDK_TOUCH_MASK`，`touch-event` 的 `GDK_TOUCH_BEGIN/UPDATE/END/CANCEL` → TouchEvent；`ID` = `GdkEventSequence` 指针；坐标已是逻辑像素（GDK 已按整数缩放换算）；`Pressure` 取 `GDK_AXIS_PRESSURE`（没有为 0）；`Time` 为零（Runner 用帧时间）。被 GDK 标为模拟（`gdk_event_get_pointer_emulated`）或来源设备为触摸屏的按键/移动/进出事件被丢弃。
- 两个平台都只送 TouchEvent，由 Runner 合成指针（不调用 `SetTouchPointerEmulation(false)`）。

## 3. Runner 怎样分发

- 手指按下时，用**上一帧**的几何把它路由给所有在它下面、声明了 `Box.Touches()` 且没有被后绘制控件盖住的盒子（祖先和后代都收到，即冒泡但不可阻止）；之后这根手指一直绑定在这些盒子上，直到抬起，即使移出盒子。
- 第一根手指（按下时屏幕上没有别的手指）合成为指针事件；第二根及以后只作为触摸送达。按下点落在 `Box.RawTouch()` 的盒子里时不合成（画板、地图自己处理所有手指）。
- 合成的指针参与「指针接管」：拖动起点在输入框、按钮上，移动超过 `ui.DragSlop`（8px）且方向符合外层容器 `Box.TakePointer(axes)` 声明的轴时，外层容器接管指针，内层控件 `c.PointerCancelled(id)` 为真且抬起时不产生点击（相当于浏览器的 `touch-action` + 指针捕获）。ui/mobile 的 ScrollView、SwipeCell、Tabs、Picker 等手势区都声明了自己的轴。

## 4. 安全区

安全区是内容区里被系统界面遮挡的边距（状态栏、刘海/挖孔、圆角、Home 指示条、导航按钮），逻辑像素。

- 二选一：
  - 窗口实现 `SafeAreaInsets() geom.Insets`，Runner 每帧读取（必须廉价，返回缓存值）；
  - 或宿主在系统通知变化时调用 `Runner.SetSafeArea(in)`（UI 线程），它优先于 `SafeAreaWindow`；`ClearSafeArea()` 撤销。
- 变化时 Runner 整帧重画（局部重绘提示失效一帧），组件用 `c.SafeArea()` 读取；ui/mobile 的 `SafeArea(c, edge)`、NavBar/TabBar、底部弹层使用它（`mobile.Options.SafeArea` 仍可覆盖）。
- iOS：`view.safeAreaInsets`（点），在 `safeAreaInsetsDidChange`、`viewSafeAreaInsetsDidChange` 时更新；横竖屏旋转后左右边距会变化。
- Android：`WindowInsets` 的 `getInsets(Type.systemBars() | Type.displayCutout())`（API 30+；更早用 `getSystemWindowInset*` 与 `getDisplayCutout()`），像素 ÷ `density`，在 `setOnApplyWindowInsetsListener` 回调里更新；应用需绘制到系统栏下（`setDecorFitsSystemWindows(false)`）才需要安全区，否则填 0。软键盘（IME inset）不算安全区。
- 桌面宿主不实现即为 0。

## 5. 测试

`ui.Driver`：`d.Touch(id, phase, p)`、`d.Touches(evs...)`（同一帧多条）、`d.Tap(p)`、`d.TouchDrag(from, to)`、`d.Pinch(center, fromDist, toDist)`、`d.TwoFingerPan(from, to, spread)`、`d.SetSafeArea(in)`。手势辅助方法每步推进 16ms 模拟时间，识别器能算出速度。用例见 `ui/touch_test.go`、`ui/mobile/takeover_test.go`。

## 6. 已知差距

- `ui.Scroll`（桌面滚动容器）没有声明 `TakePointer`，触屏上只能用滚动条或滚轮滚动；手指拖动内容滚动需要 ui/mobile 的 ScrollView。
- 双指捏合期间第一根手指仍合成为指针，外层 ScrollView 会跟着平移；需要纯捏合的组件用 `RawTouch`。
- 合成的指针事件不区分来源（`input.Pointer` 没有 Source 字段），组件无法据此关闭悬停效果；手指抬起后的 `Leave` 让悬停状态消失。
