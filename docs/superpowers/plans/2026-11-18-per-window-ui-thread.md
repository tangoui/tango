# 每个窗口独立 UI 线程（共享层 + macOS）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** macOS 上每个 `uihost.Open` 窗口的页面函数、布局、绘制、渲染、上屏都在该窗口自己的 UI 线程上运行，主线程被 AppKit 占住时窗口照常出帧。

**Architecture:** 新增 `internal/uithread`（锁定 OS 线程的任务循环）。`ui.Runner` 增加线程归属与 `Post`，开发构建检查调用线程。uihost 的 `host` 把平台回调转成任务投递到 UI 线程；平台契约新增可选能力 `platform.CanvasThreaded`，darwin 实现它：显示链接回调、`Present`、Metal/Core Animation 提交都在 UI 线程上执行。其他平台和 `Attach`、`tan dev` 宿主保持"UI 线程 = 主线程"。

**Tech Stack:** Go 1.27.3、purego/objc、AppKit、Core Animation、Metal。

**Spec:** `docs/superpowers/specs/2026-11-18-per-window-ui-thread-design.md`

## Global Constraints

- 工具链：`GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0`；go 命令统一加 `nice -n 19 -p 1`。
- 提交作者：`git -c user.name=tango -c user.email=903300326@qq.com commit`，不加任何 trailer。
- 只有 macOS 的 `uihost.Open` 窗口绑定独立 UI 线程；`uihost.Attach`、`tan dev` 宿主（`remote_dev.go`）、其他平台、无头 `ui.Driver` 保持"UI 线程 = 主线程"。
- 主线程从不无限期等待 UI 线程：改尺寸最多等 1 个刷新周期（16.7 ms），应用退出最多等 1 秒；关闭窗口不等待。UI 线程正在等主线程时，改尺寸跳过等待。
- 开发构建检查只对绑定了独立 UI 线程的 Runner 生效，只在 `tango_dev` 构建中编译；违规时 panic，消息包含 `"ui: called off the window's UI thread; use Runner.Post"`。
- 持平判定（§7）：P99 差距 ≤ 1 ms、超过 20 ms 的帧比例差距 ≤ 0.3 个百分点、内存和 CPU 差距 ≤ 5% 视为持平；超出就停下报告用户，不得放宽。
- GUI 测量（前台窗口、计时）须先向 min-78 约时段；监控脚本不得使用 `vmmap`（会挂起被测进程）；全部样本保留，不剔除。
- 不改变公共稳定 API 的签名；新增符号均为实验性（不加入 `api/stable/*.txt`），并运行 `go run ./internal/cmd/apitxt -check`。

## Review Focus

1. **窗口在 UI 线程等主线程（对话框开着）时用户拖动改大小或关闭窗口**：不能死锁；改大小立即返回、关闭在对话框结束后完成。归 Task 6 测试 `TestResizeWhileUIThreadAwaitsMain`、`TestCloseWhileUIThreadAwaitsMain`。
2. **`Runner.Post` 在窗口关闭后、或在 UI 线程上调用**：关闭后丢弃不 panic；UI 线程上调用排在当前帧之后、不重入。归 Task 3 测试 `TestPostAfterCloseDropped`、`TestPostFromUIThreadRunsAfterFrame`。
3. **多个窗口同时出帧**：共享的字体、字形、图片缓存和 GPU 设备在并发下无数据竞争。归 Task 9 测试 `TestTwoWindowsRenderConcurrently`（`-race`）。
4. **第一帧之前就到达的输入、尺寸变化、主题通知**：入队后按顺序在第一帧处理，不丢失。归 Task 4 测试 `TestEventsBeforeFirstFrameAreKept`。
5. **GPU 失败降级到 CPU、native 因按键升级到 Metal 发生在 UI 线程上**：需要主线程的切换步骤同步完成，切换后的第一帧正常上屏。归 Task 7 测试 `TestPromoteToMetalFromUIThread`、`TestFallbackToCPUFromUIThread`。

---

### Task 1：验证 native/CPU 图层从后台线程按时上屏（闸门）

本任务不改产品代码，产出一份结论，决定 Task 7 走 native 还是让数据窗口改走 Metal（设计 §6.3）。

**Files:**
- Create: `internal/platform/darwin/offmain_probe_darwin_test.go`（`//go:build darwin && !ios && tango_offmainprobe`，默认不编译）
- Create: `docs/specs/2026-11-18-offmain-present-probe.md`（结论记录）

**Interfaces:**
- Produces: 结论"native 后台上屏可靠 / 不可靠"，写进结论文档第一行，Task 7 读取。

- [ ] **Step 1：写探针测试 `TestOffMainLayerPresentProbe`**

测试创建一个真实 Tango canvas 窗口（800×600），把它的根 `CALayer` 交给一条锁定 OS 线程的 goroutine。该 goroutine 以 60 Hz 节拍：在显式 `CATransaction` 里设置 contents 为两张交替的 `CGImage` 之一，再在一个同层级的 16×16 `CAMetalLayer` 上 `presentDrawable`，在同一事务里提交后 `CATransaction.flush()`。用该 Metal 层 drawable 的 `addPresentedHandler` 记录上屏时间（作为同一事务内 contents 上屏的代理）。主线程上挂 4 ms 探针计时器。先安静 15 秒，再每 2 秒切换一次前台（Finder ↔ 该窗口）20 秒。另跑一组对照：同样的提交改在主线程的显示链接回调里执行。

断言只做记录，不做阈值：测试把两组的上屏间隔 P99、超过 20 ms 比例、主线程阻塞次数写到 `t.Logf` 和 `$TANGO_OFFMAIN_OUT`（JSON）。

- [ ] **Step 2：约时段后运行**

先向 min-78 约约 3 分钟前台时段。

Run: `TANGO_OFFMAIN_OUT=/Users/admin/min/outputs/offmain-probe-$(date +%Y%m%d)/result.json nice -n 19 go test -tags tango_offmainprobe -run TestOffMainLayerPresentProbe -v -count=1 ./internal/platform/darwin`
Expected: 两组都有数据；主线程组切换阶段出现主线程阻塞（复现问题，证明探针有效）。

- [ ] **Step 3：写结论文档**

判定：后台组切换阶段的上屏 P99 与安静阶段持平（Global Constraints 的阈值），且主线程组复现了阻塞 → 第一行写 `结论：native 后台上屏可靠`；否则写 `结论：native 后台上屏不可靠`，并附数据。若不可靠，停下请用户确认设计 §6.3 的回退（数据窗口改走 Metal，约多 70 MB）后再继续 Task 7。

- [ ] **Step 4：提交**

```bash
git add internal/platform/darwin/offmain_probe_darwin_test.go docs/specs/2026-11-18-offmain-present-probe.md
git -c user.name=tango -c user.email=903300326@qq.com commit -m "test(darwin): probe native layer presentation off the main thread"
```

---

### Task 2：`internal/uithread`：锁定 OS 线程的任务循环

**Files:**
- Create: `internal/uithread/uithread.go`
- Create: `internal/uithread/uithread_test.go`

**Interfaces:**
- Produces:
  - `type Thread struct` — 一条锁定 OS 线程的 goroutine 和它的任务队列。
  - `func Start(name string) *Thread` — 启动线程，返回前该线程已开始运行并记录了 goroutine id。
  - `func (t *Thread) Post(fn func()) bool` — 任何 goroutine 可调用；按调用顺序在线程上执行；线程已停止返回 false 并丢弃。
  - `func (t *Thread) Do(fn func()) bool` — 在线程上同步执行并等待；在线程自身上调用时直接执行；线程已停止返回 false。
  - `func (t *Thread) On() bool` — 调用者是否就是这条线程。
  - `func (t *Thread) Stop()` — 让线程处理完已入队的任务后退出；不等待；重复调用无效果。
  - `func (t *Thread) Done() <-chan struct{}` — 线程退出后关闭。
  - `func (t *Thread) SetAwaitingMain(on bool)`、`func (t *Thread) AwaitingMain() bool` — 设计 §5.3 的"正在等主线程"标记（原子变量）。
  - `func GoroutineID() uint64` — 从 `tango` 根包 `loop.go` 的 `goroutineID` 移来并导出，根包改为调用它。

- [ ] **Step 1：写失败的测试**

```go
func TestPostRunsInOrderOnOneLockedThread(t *testing.T) // 100 个 Post 按顺序执行；全部在同一 OS 线程（通过 syscall 取线程 id 比较）
func TestPostFromOwnThreadDoesNotReenter(t *testing.T)   // 线程上 Post 的任务在当前任务返回后才执行
func TestDoWaitsAndRunsInlineOnOwnThread(t *testing.T)   // 其他 goroutine Do 会等结果；线程自身 Do 直接执行
func TestStopDrainsThenDropsLaterPosts(t *testing.T)     // Stop 前已入队的执行完；Stop 后 Post 返回 false；Done 关闭
func TestOnReportsOwnThread(t *testing.T)
func TestAwaitingMainFlag(t *testing.T)
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 ./internal/uithread`
Expected: FAIL（包不存在）

- [ ] **Step 3：实现**

队列用带锁切片 + 唤醒通道（`chan struct{}`，容量 1）；线程函数开头 `runtime.LockOSThread()`，不解锁；任务 panic 时恢复并交给 `Start` 时传入的处理器（`func StartWith(name string, onPanic func(any)) *Thread`，`Start` 使用默认的重新 panic）。

- [ ] **Step 4：运行测试（含 race）**

Run: `nice -n 19 go test -p 1 -race ./internal/uithread && nice -n 19 go test -p 1 . -run Await`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add internal/uithread loop.go
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uithread): locked OS thread task loop for window UI threads"
```

---

### Task 3：`ui.Runner` 的线程归属、`Post` 与开发构建检查

**Files:**
- Modify: `ui/runner.go`（字段、`Post`、`Invalidate` 线程安全）
- Create: `ui/runner_thread.go`（`BindThread`、检查入口）
- Create: `ui/threadcheck_dev.go`（`//go:build tango_dev`）、`ui/threadcheck_release.go`（`//go:build !tango_dev`）
- Modify: `ui/context.go`（`State`、`Invalidate`、`WakeAt`、`SetFocus` 调用检查）；其他公开的 Runner 方法见 Step 3 列表
- Test: `ui/runner_thread_test.go`、`ui/runner_thread_dev_test.go`（`//go:build tango_dev`）

**Interfaces:**
- Consumes: Task 2 的 `uithread.Thread`。
- Produces:
  - `func (r *Runner) BindThread(t *uithread.Thread)` — 未导出到稳定 API（实验性，uihost 使用）；`Attach` 之前调用；绑定后不可更改。
  - `func (r *Runner) Post(fn func())` — 绑定线程时投递到该线程并请求一帧；未绑定时直接在调用 goroutine 上执行 `fn` 后 `Invalidate`（保持"UI 线程 = 主线程"语义，调用者须已在 UI 线程）。关闭后丢弃。
  - `func (r *Runner) checkThread()` — 开发构建：绑定了线程且 `!t.On()` 时 panic，消息含 `"ui: called off the window's UI thread; use Runner.Post"`；正式构建为空函数。

- [ ] **Step 1：写失败的测试**

```go
func TestPostAfterCloseDropped(t *testing.T)            // Close 后 Post 不执行、不 panic
func TestPostFromUIThreadRunsAfterFrame(t *testing.T)   // 在 UI 线程上的页面函数里 Post，任务在本帧 Frame 返回后执行
func TestPostUnboundRunsInline(t *testing.T)            // 未 BindThread：Post 立即执行并 Invalidate
func TestInvalidateSafeFromAnyGoroutine(t *testing.T)   // -race 下多个 goroutine 同时 Invalidate
// dev 标签：
func TestStateOffThreadPanics(t *testing.T)             // 绑定线程后在其他 goroutine 调 ui.State → panic 且消息含指定文本
func TestStateOnThreadOK(t *testing.T)
func TestUnboundRunnerNeverChecks(t *testing.T)         // 未绑定的 Runner 在任意 goroutine 调用都不 panic
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'Post|ThreadPanics|OnThread|NeverChecks|InvalidateSafe' ./ui && nice -n 19 go test -p 1 -tags tango_dev -run 'ThreadPanics|OnThread|NeverChecks' ./ui`
Expected: FAIL（未定义）

- [ ] **Step 3：实现**

检查入口（开发构建调用 `checkThread`）：`Runner` 的 `SetReduceMotion`、`EnableAccessibility`、`SetTheme`、`HandleEvent`、`Frame`、`PerformAction`、`SetKeyboardInset`、`ClearKeyboardInset`、`SetSafeArea`、`ClearSafeArea`、`FileDragOver`、`FileDragLeave`、`DropFiles`、`TrimCaches`、`SetTouchPointerEmulation`；`ui.State`、`(*Context).Invalidate`、`WakeAt`、`WakeAtDamaged`、`SetFocus`、`InvalidateImage`。`Post`、`Invalidate`、`InvalidateAt`、`Close`、只读的 `Animating`、`NextWake`、`NativeRegions`、`AccessibilityTree`、`PointerPos`、`FileDrag` 不检查。`Context` 目前没有指向 Runner 的字段：新增 `runner *Runner`，在 `NewRunner` 里设置；无头 `Driver` 创建的 Context 同样设置（其 Runner 未绑定线程，检查直接返回）。

- [ ] **Step 4：运行测试（含 race、全包）**

Run: `nice -n 19 go test -p 1 -race ./ui && nice -n 19 go test -p 1 -tags tango_dev ./ui`
Expected: PASS（现有测试不受影响，因为它们的 Runner 未绑定线程）

- [ ] **Step 5：提交**

```bash
git add ui
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(ui): Runner.Post and UI-thread ownership checks in development builds"
```

---

### Task 4：平台契约 `CanvasThreaded` 与 uihost 的任务转发

**Files:**
- Modify: `internal/platform/canvas.go`（新增接口）
- Create: `uihost/threaded.go`（`host` 的线程化分支）
- Modify: `uihost/uihost.go`（`Open` 按能力选择线程模型；`CanvasInput/Resized/Focus/Frame` 转发）
- Test: `uihost/threaded_test.go`（假 canvas）

**Interfaces:**
- Consumes: Task 2、Task 3。
- Produces:
  ```go
  // CanvasThreaded is implemented by a Canvas whose frames may be drawn and
  // presented on a thread other than the main thread.
  type CanvasThreaded interface {
      // StartFrameClock starts per-refresh callbacks on the calling thread
      // (the window's UI thread); request asks for a callback at the next
      // refresh (frameAtOnce semantics apply). tick runs on that thread.
      StartFrameClock(tick func()) (request func(), stop func(), err error)
      // MainGeometry returns the latest size and scale as set on the main
      // thread; safe from any thread.
      MainGeometry() (w, h, scale float32)
  }
  ```
  - `uihost` 内部：`type threadedHost struct`，字段 `th *uithread.Thread`、`request func()`、`stopClock func()`。

- [ ] **Step 1：写失败的测试**

```go
func TestThreadedHostForwardsInputInOrder(t *testing.T)    // 主线程连续送 Pointer、Key、Scroll，UI 线程上 Runner 收到同样顺序
func TestEventsBeforeFirstFrameAreKept(t *testing.T)       // 第一帧前的输入、尺寸、主题通知都在第一帧处理
func TestNonThreadedCanvasKeepsMainThreadModel(t *testing.T) // 不实现 CanvasThreaded 的 canvas：Runner 未绑定线程
func TestAttachNeverBindsThread(t *testing.T)               // uihost.Attach 的 Runner 未绑定线程
func TestFrameRunsOnUIThread(t *testing.T)                  // 页面函数里 uithread.GoroutineID() 等于 UI 线程 id
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'Threaded|BeforeFirstFrame|NonThreaded|AttachNever|FrameRunsOnUI' ./uihost`
Expected: FAIL

- [ ] **Step 3：实现**

`Open` 中：若 `cv` 实现 `CanvasThreaded` 且平台允许（darwin），启动 `uithread.Thread`，在其上用 `Do` 创建 Runner、`BindThread`、`setupGPU`、`StartFrameClock`；`CanvasInput/Resized/Focus` 在主线程上 `th.Post` 后返回；时钟回调在 UI 线程上执行原 `CanvasFrame` 的逻辑；`h.cv.Invalidate()` 改为调用 `request()`。

- [ ] **Step 4：运行测试**

Run: `nice -n 19 go test -p 1 -race ./uihost ./internal/platform/...`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add internal/platform/canvas.go uihost
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uihost): run frames on a window UI thread for threaded canvases"
```

---

### Task 5：`ui.Window` 回调与全局通知跨线程

**Files:**
- Modify: `uihost/uihost.go`（`window` 的 `SetCursor`、`SetTextInput`、`ReadClipboard`、`WriteClipboard`）
- Modify: `uihost/frame.go`（`SetWindowState`、`CloseWindow`、`StartWindowMove`）
- Modify: `uihost/follow.go`、`uihost/memory.go`、`uihost/contrast.go`、`uihost/motion.go`（主线程收到通知后 `Runner.Post`）
- Modify: `uihost/a11y.go`（`AccessibilityAction` 入队）、`uihost/textstate.go`
- Test: `uihost/threaded_window_test.go`

**Interfaces:**
- Consumes: Task 4 的 `threadedHost`。
- Produces: 无新符号。规则：最终状态类（光标、输入法位置、窗口状态、开始拖动、关闭）用 `app.Post`，不等待；`ReadClipboard` 用 `tango.Await`，等待前 `th.SetAwaitingMain(true)`、返回后 `false`。

- [ ] **Step 1：写失败的测试**

```go
func TestCursorAndTextInputPostedToMain(t *testing.T)      // UI 线程调用后，假 canvas 的 SetCursor/SetTextInput 在主线程上被调用
func TestReadClipboardAwaitsMainAndSetsFlag(t *testing.T)  // 等待期间 AwaitingMain()==true，返回后 false
func TestThemeChangePostedToUIThread(t *testing.T)         // 系统主题通知在主线程到达，SetTheme 在 UI 线程执行
func TestAccessibilityActionRunsOnUIThread(t *testing.T)
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'PostedToMain|ReadClipboardAwaits|ThemeChangePosted|AccessibilityActionRuns' ./uihost`
Expected: FAIL

- [ ] **Step 3：实现**（按 Interfaces 的规则逐个方法改）

- [ ] **Step 4：运行测试**

Run: `nice -n 19 go test -p 1 -race ./uihost && nice -n 19 go test -p 1 -tags tango_dev ./uihost`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add uihost
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uihost): hand window callbacks and system notifications across threads"
```

---

### Task 6：防死锁规则、改尺寸同步、关闭、退出、对话框

**Files:**
- Modify: `uihost/threaded.go`（改尺寸等待、关闭、退出）
- Modify: `sys/dialog/dialog.go`（UI 线程上调用时 `Await` 主线程并设置等待标记）
- Modify: `tango.go` 或 `loop.go`（应用退出时通知并等待 UI 线程，上限 1 秒）
- Test: `uihost/threaded_lifecycle_test.go`、`sys/dialog/dialog_thread_test.go`

**Interfaces:**
- Consumes: Task 2 的 `SetAwaitingMain`/`AwaitingMain`、`Stop`/`Done`。
- Produces:
  - `const resizeWait = 16700 * time.Microsecond`、`const quitWait = time.Second`（`uihost/threaded.go`）。
  - `func (h *threadedHost) resizedOnMain(w, hgt, scale float32)`：投递尺寸任务；若 `!th.AwaitingMain()`，等待"新尺寸帧已上屏"信号或 `resizeWait` 超时。
  - 注册到 `tango` 退出流程的钩子：`internal/corehook` 目前没有退出钩子，新增 `func OnQuit(app any, fn func())`，由 `tango` 根包在 `App.Run` 退出前、`OnLifecycle(Terminating)` 之后依次调用（每个已注册的 fn 都要在 `quitWait` 总时限内）。

- [ ] **Step 1：写失败的测试**

```go
func TestResizeWaitsForNewSizeFrame(t *testing.T)          // 改尺寸后返回时，UI 线程已出过新尺寸帧
func TestResizeGivesUpAfterOneRefresh(t *testing.T)        // UI 线程阻塞 50 ms，改尺寸在 ~16.7 ms 后返回
func TestResizeWhileUIThreadAwaitsMain(t *testing.T)       // UI 线程在 Await 主线程期间改尺寸：立即返回，不死锁（测试超时 2 s）
func TestCloseDoesNotWait(t *testing.T)                    // 关闭立即返回；UI 线程随后释放渲染器并退出（Done 关闭）
func TestCloseWhileUIThreadAwaitsMain(t *testing.T)        // 对话框（Await）期间关闭：对话框结束后 UI 线程退出，不死锁
func TestQuitWaitsAtMostOneSecond(t *testing.T)            // 一个 UI 线程卡 5 s，退出在 ~1 s 后继续
func TestDialogFromUIThreadAwaitsMain(t *testing.T)        // sys/dialog：在 UI 线程上调用时请求在主线程执行，期间 AwaitingMain()==true
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'Resize|Close|Quit' ./uihost && nice -n 19 go test -p 1 -run DialogFromUIThread ./sys/dialog`
Expected: FAIL

- [ ] **Step 3：实现**（按设计 §5.3 的规则；`dialog` 判断"当前在某个 UI 线程上"用 `uithread` 包提供的 `func Current() *Thread`，在 Task 2 的文件里补一个按 goroutine id 查表的函数）

- [ ] **Step 4：运行测试（race，重复 20 次查不稳定）**

Run: `nice -n 19 go test -p 1 -race -count=20 -run 'Resize|Close|Quit' ./uihost && nice -n 19 go test -p 1 -race ./sys/dialog`
Expected: PASS，20 次全部通过

- [ ] **Step 5：提交**

```bash
git add uihost sys/dialog tango.go loop.go internal/corehook internal/uithread
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uihost): deadlock-free resize, close, quit and dialogs with window UI threads"
```

---

### Task 7：darwin 实现 `CanvasThreaded`（显示链接、Metal、native、CPU）

**Files:**
- Create: `internal/platform/darwin/canvas_threaded.go`（`StartFrameClock`、`MainGeometry`、UI 线程 run loop）
- Modify: `internal/platform/darwin/canvas.go`（主线程上的 `checkGeometry` 只更新几何快照并通知 sink；`setNeedsDisplay` 在线程化模式下改为调用 `request`）
- Modify: `internal/platform/darwin/canvas_metal.go`、`canvas_damage.go`（`Present`/`PresentDamage` 在 UI 线程上设置图层内容并显式提交事务 + `flush`）
- Modify: `internal/platform/darwin/canvas_view.go`（线程化模式下 CPU 路径不再依赖 `updateLayer`：视图改为 layer-hosting——与 `UseMetalLayer` 同样的方式设置自有 `CALayer`——使 AppKit 不再调用 `updateLayer`，CPU 帧由 UI 线程上的帧时钟驱动，`Present` 直接设置该层 contents）
- Modify: `internal/render/gpu/metal/presenter_darwin.go`（"只能在主线程"改为"在创建它的线程上使用"，`drawableSize` 由 UI 线程设置）
- Modify: `internal/render/quartz/renderer_darwin_arm64.go`（事务在 UI 线程上提交并 `flush`）
- Modify: `uihost/gpu.go`、`uihost/native.go`（`UseMetalLayer`/`UseCoreGraphicsLayer` 在 UI 线程上经 `tango.Await` 请主线程执行，等待时设 `AwaitingMain`）
- Test: `internal/platform/darwin/canvas_threaded_darwin_test.go`、`uihost/threaded_gpu_test.go`

**Interfaces:**
- Consumes: Task 1 结论（若"不可靠"且用户确认回退：`setupGPU` 在线程化模式下对 `ui.ContentData` 也选 Metal，不选 native）；Task 4 的 `CanvasThreaded`。
- Produces: darwin `*canvas` 实现 `platform.CanvasThreaded`。

- [ ] **Step 1：验证显示链接能挂到 UI 线程的 run loop（设计 §6.2）**

写测试 `TestDisplayLinkOnUIThreadRunLoop`：在锁定线程上创建 `CFRunLoop`，把视图的 `displayLink` 加到这个 run loop，1 秒内回调次数在 55–65 之间，且回调线程等于该线程。若不成立，改用 `CVDisplayLink` 回调投递到 UI 线程，测试改为断言投递后的回调线程等于 UI 线程、次数在 55–65 之间。把选定方案写在 `canvas_threaded.go` 的包注释里。

- [ ] **Step 2：写失败的测试**

```go
func TestThreadedMetalPresentsFromUIThread(t *testing.T)    // 假页面画 3 帧，Metal 呈现器的 present 在 UI 线程上执行，ReadPixels 结果正确
func TestThreadedCPUPresentsFromUIThread(t *testing.T)      // TANGO_RENDERER=cpu：Present 在 UI 线程上设置 contents；截图像素正确
func TestThreadedNativePresentsFromUIThread(t *testing.T)   // arm64：native 图层树在 UI 线程上提交；renderInContext 截图正确
func TestPromoteToMetalFromUIThread(t *testing.T)           // native 窗口收到按键后在 UI 线程上升级到 Metal，切换后第一帧上屏
func TestFallbackToCPUFromUIThread(t *testing.T)            // 注入 GPU 失败：降级到 CPU 后下一帧上屏
func TestFrameAtOnceSemanticsOnUIThread(t *testing.T)       // b25960c 的停顿后立即出帧在 UI 线程模式下保持
```

- [ ] **Step 3：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'Threaded|Promote|Fallback|FrameAtOnce|DisplayLinkOnUI' ./internal/platform/darwin ./uihost`
Expected: FAIL

- [ ] **Step 4：实现**（按设计 §6.1–6.4；主线程上保留：窗口创建、视图层级、跟踪区域、光标、第一响应者、输入法客户端、窗口状态、拖动、关闭）

- [ ] **Step 5：运行测试（含现有 darwin、metal、quartz 全包）**

Run: `nice -n 19 go test -p 1 -race ./internal/platform/darwin ./internal/render/gpu/metal ./internal/render/quartz ./uihost`
Expected: PASS

- [ ] **Step 6：提交**

```bash
git add internal/platform/darwin internal/render/gpu/metal internal/render/quartz uihost
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(darwin): draw and present uihost windows on their UI threads"
```

---

### Task 8：主线程快照判断（快捷键、拖动区域）与嵌入 WebView

**Files:**
- Modify: `uihost/shortcuts.go`（主线程按快照匹配）
- Create: `uihost/snapshot.go`（`type uiSnapshot struct`，UI 线程每帧发布、主线程原子读取）
- Modify: `ui/runner.go`（每帧结束导出快照所需字段的只读访问器）
- Modify: `uihost/webview.go`（每帧生成摆放列表投递主线程；新增包级开关 `var threadedNativeViews = true`，为 false 时 `Open` 对使用了 `ui.RunnerOptions` 中声明含 NativeView 的窗口不绑定线程——由于页面是否含 NativeView 要到第一帧才知道，开关为 false 时改为：首帧出现 NativeView 就在 UI 线程上把窗口切回主线程模型，只允许发生一次）
- Test: `uihost/snapshot_test.go`、`uihost/webview_threaded_test.go`

**Interfaces:**
- Consumes: Task 4、5。
- Produces:
  ```go
  type uiSnapshot struct {
      editableFocus bool          // 焦点在可编辑文本上
      composing     bool          // 输入法组字中
      capturing     bool          // 正在录制快捷键
      dragRegions   []geom.Rect   // 自绘标题栏的拖动区域（逻辑像素）
  }
  ```
  `func (h *threadedHost) snapshot() *uiSnapshot`（`atomic.Pointer`）。

- [ ] **Step 1：写失败的测试**

```go
func TestShortcutMatchUsesSnapshot(t *testing.T)         // 文本框有焦点时快照 editableFocus=true，Cmd+字母快捷键不在主线程消费（与现有规则一致）
func TestShortcutConsumedKeyNeverReachesUI(t *testing.T) // 命中的按键不入队
func TestDragRegionStartsMoveOnMain(t *testing.T)        // 鼠标按下落在快照的拖动区域内，主线程直接开始拖动，UI 线程不收到该事件
func TestWebViewPlacementPostedEachFrame(t *testing.T)   // 每帧一次摆放任务在主线程执行，位置等于本帧 NativeRegions
func TestNativeViewWindowCanOptOut(t *testing.T)         // 设计 §5.6 的回退点：uihost 内部开关 threadedNativeViews=false 时，页面里出现 NativeView 的窗口在 Open 时不绑定线程（开关默认 true，Task 11 若 WebView 错位超标再改为 false 并报告用户）
```

- [ ] **Step 2：运行确认失败**

Run: `nice -n 19 go test -p 1 -run 'Snapshot|ShortcutConsumed|DragRegion|WebViewPlacement' ./uihost`
Expected: FAIL

- [ ] **Step 3：实现**

- [ ] **Step 4：运行测试**

Run: `nice -n 19 go test -p 1 -race ./uihost ./ui`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add uihost ui
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uihost): main-thread decisions from per-frame UI snapshots; post WebView placement"
```

---

### Task 9：共享缓存并发安全、自动化宿主迁移

**Files:**
- Modify: 按 Step 1 的 race 结果修改 `ui/text`、`ui/paint`、`internal/render/gpu`、`internal/nativeglyph` 中的全局缓存（加锁或每窗口一份）
- Modify: `uihost/automation_dev.go`（请求经 `Runner.Post` 在 UI 线程执行，通道等待结果）
- Test: `uihost/concurrent_windows_test.go`、`uihost/automation_threaded_dev_test.go`（`//go:build tango_dev`）

**Interfaces:**
- Consumes: Task 4–8。
- Produces: 无新符号。

- [ ] **Step 1：写失败的测试**

```go
func TestTwoWindowsRenderConcurrently(t *testing.T)   // 两个线程化窗口各画 200 帧（文字、图片、阴影），-race 无报告
func TestAutomationFindAndPerformOnUIThread(t *testing.T) // dev：Find/Perform/Snapshot 在 UI 线程执行，结果正确
```

- [ ] **Step 2：运行确认失败（race 报告即失败）**

Run: `nice -n 19 go test -p 1 -race -run TwoWindowsRenderConcurrently ./uihost && nice -n 19 go test -p 1 -tags tango_dev -run AutomationFindAndPerform ./uihost`
Expected: FAIL（race 报告或未实现）

- [ ] **Step 3：逐个修复 race 报告指向的缓存；迁移自动化宿主**

- [ ] **Step 4：运行全量测试**

Run: `nice -n 19 go test -p 1 -race ./ui/... ./uihost/... ./internal/render/... ./internal/platform/darwin/... && (cd cmd/tan && nice -n 19 go test -p 1 ./...)`
Expected: PASS

- [ ] **Step 5：提交**

```bash
git add -A ui uihost internal
git -c user.name=tango -c user.email=903300326@qq.com commit -m "fix: make shared render caches safe across window UI threads; automation on UI threads"
```

---

### Task 10：示例、文档、迁移说明

**Files:**
- Modify: `examples/ledger/main.go`、`examples/editor-demo/main.go`、`examples/editor-demo/document.go`，以及 `grep -rn "app.Post(" examples` 列出的、在投递函数里修改页面状态或调用 Runner 的其他示例（改为 `runner.Post`）
- Modify: `uihost/uihost.go` 包注释、`docs/specs/canvas-surface.md`（"UI 线程 = 主线程"改为按平台说明）、`ui/runner.go` 的 `Runner` 注释
- Modify: `CHANGELOG.md`（【未发布】"不兼容变更"）、`docs/release.md`（迁移说明：`app.Post` 改 `Runner.Post` 的示例、`go test -race` 建议）
- Test: `examples/...` 构建；开发构建下各示例启动烟测（不开窗口的部分用 `-tags tango_dev go vet`；开窗口的放到 Task 11 一起约时段）

**Interfaces:**
- Consumes: Task 3 的 `Runner.Post`。

- [ ] **Step 1：改示例和文档**

- [ ] **Step 2：验证**

Run: `nice -n 19 go build -p 1 ./examples/... && nice -n 19 go vet -p 1 -tags tango_dev ./examples/... && nice -n 19 go run ./internal/cmd/apitxt -check`
Expected: 构建通过；`apitxt` 报告 up to date（新增符号为实验性）

- [ ] **Step 3：提交**

```bash
git add examples uihost/uihost.go ui/runner.go docs CHANGELOG.md
git -c user.name=tango -c user.email=903300326@qq.com commit -m "docs: window UI threads on macOS; migrate examples to Runner.Post"
```

---

### Task 11：真实窗口验收（设计 §7）

**Files:**
- Create: `docs/specs/2026-11-18-per-window-ui-thread-report.md`

- [ ] **Step 1：约时段**（向 min-78 约约 40 分钟前台；确认无 UU 远程、无人操作屏幕、负载 < 7）

- [ ] **Step 2：切换前台测量**

复用 `outputs/tango-link-fallback-20261118/switch_ab.sh` 的流程：改前（`origin/main`）与改后，各 3 次 AB/BA 交替，每次 20 秒安静 + 20 秒每 2 秒切换前台，加 mygo 1 次。
Expected: 改后切换阶段的 P99 和超过 20 ms 比例与安静阶段持平（阈值见 Global Constraints），且不比 mygo 差。

- [ ] **Step 3：安静场景门槛**

复用 gate13 的 `run.sh`（grid、list，5 轮 AB/BA + 65 秒长跑，监控无 vmmap）。
Expected: 内存、CPU、P99 与改前持平。

- [ ] **Step 4：编辑器打字延迟**

请 min-78 用其 GUI 矩阵（Tango 编辑器 typing、三光标、滚动）跑改前改后。
Expected: 中位数与 P95 持平。

- [ ] **Step 5：正确性真机项**

拖动改大小（逐帧截屏无拉伸、无错位）、VoiceOver 读取与动作、中文输入法组字、`examples/webview-embed` 滚动错位 ≤ 1 帧、各示例开发构建下启动操作不触发线程检查。

- [ ] **Step 6：写报告并提交**

任何一项超出阈值：停下，把数据报告给用户，不合并。

```bash
git add docs/specs/2026-11-18-per-window-ui-thread-report.md
git -c user.name=tango -c user.email=903300326@qq.com commit -m "docs: acceptance report for per-window UI threads on macOS"
```
