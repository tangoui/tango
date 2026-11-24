# 应用原生视图嵌入（uihost.NativeHost）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让应用把自己创建的原生视图（macOS `NSView*`、Windows 子 `HWND`、Linux `GtkWidget*`）放进 Tango 自绘窗口的布局里，每帧随盒子移动、缩放、裁剪，可选让鼠标键盘穿透到画布；用于视频播放、远程桌面画面等必须由系统直接合成的内容。

**Architecture:** 平台层新增可选接口 `platform.NativeViewEmbedder`，三个桌面后端复用现有 WebView 内嵌的宿主视图/容器窗口和 `WebPlacement.Geometry` 摆放逻辑，只把内层子视图换成应用的视图。核心包通过 `corehook` 暴露创建入口。uihost 新增 `NativeHost` 构建器：UI 线程每帧根据 `NativeRegions` 决定摆放，交给主线程执行（非线程化窗口直接执行，线程化窗口合并后 `Post` 到主线程），共用一套代码。

**Tech Stack:** Go；Tango `internal/platform/{darwin,windows,linux,fake}`、`internal/corehook`、根包、`uihost`。

**Spec:** `/Users/admin/min/mmdesk/docs/superpowers/specs/2026-11-24-mmdesk-spike-design.md` 第 6.2 节（mmdesk 第一步验证原型对 Tango 的需求）。

## Global Constraints

- 公开 API 只新增：`uihost.NativeHostOptions`、`uihost.NativeHost`、`uihost.NativeHostW`（及其 `Err()`）、`uihost.ErrNoNativeHost`。标注 Experimental（同 WebView）。`api/go/uihost.txt` 用 `go run ./internal/cmd/apitxt -w` 更新。
- 所有权：应用创建视图（`Create`，主线程），Tango 只负责挂到窗口、摆放、移除；移除后在主线程调用 `Release(view)`，由应用销毁。Tango 不 retain/destroy 应用视图之外的东西（macOS 挂入时父视图会 retain，移除时释放；Windows 移除时 `SetParent` 回 NULL 并隐藏；Linux 应用须自己持有引用）。
- 摆放坐标、可见裁剪、DPI 换算与 WebView 内嵌完全一致（`platform.WebPlacement`、`Geometry`）。与 WebView 共用同一个宿主/容器层，按创建顺序叠放。
- `PassInput: true` 时：macOS 裁剪视图 `hitTest:` 返回 nil；Windows 容器窗口和应用顶层子窗口都应答 `WM_NCHITTEST` 为 `HTTRANSPARENT`（子类化）；Linux 设置 overlay pass-through 并对子 GdkWindow 设 pass-through。键盘不进入视图（不调用 FocusWeb，盒子默认不是 Tab 停靠点）。
- `Covered`：`CoverHide`/`CoverIgnore` 同 WebView；`CoverSnapshot`（零值）对原生视图没有快照，按 `CoverHide` 处理并在文档写明。
- 不支持的情况（headless、Attach 的 overlay 画布、后端未实现）：盒子画背景，`Err()` 返回 `ErrNoNativeHost`（包装 `tango.ErrUnsupported`），不调用 `Create`。
- 推送前检查同上次：`gofmt -l`、6 目标交叉 `go vet`（含 `-tags tango_dev`、`cmd/tan`）、`apitxt -check`、受影响包测试（含 `-race` 平台包）。编译测试 `nice -n 19`。
- 提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By；不推送、不合并，除非用户要求。

## Review Focus

1. 父窗口关闭时，每个原生视图都被移除且 `Release` 恰好调用一次（不重复、不遗漏），即使 `KeepAlive` 期间或线程化窗口里有未应用的帧。
2. 同一 key 在一帧消失、下一帧以新盒子出现时，旧视图释放、新视图创建；`Create` 返回错误时不重试刷屏，`Err()` 报告。
3. 线程化窗口：多帧合并时只应用最新摆放；`Create`/`Release` 只在主线程调用。
4. 缩放比变化（窗口拖到另一块屏）和被滚动裁掉时，视图隐藏/重新摆放正确。
5. `PassInput` 关闭时应用视图能正常接收点击（不影响默认行为）。

---

### Task 1: 平台接口、fake 后端、核心钩子与 uihost.NativeHost

**Files:**
- Create: `internal/platform/nativeembed.go`
- Create: `internal/platform/fake/nativeembed.go`
- Modify: `internal/corehook/webembed.go`（新增钩子变量）
- Modify: 根包 `webembed.go`（设置钩子：找到父 surface 的 backend，断言 `platform.NativeViewEmbedder`）
- Create: `uihost/nativehost.go`
- Modify: `uihost/webview.go` / `uihost/uihost.go`（`sync` 后调用 native 同步；窗口关闭时释放）
- Test: `uihost/nativehost_test.go`

**Interfaces:**
- Produces（平台层）:
  ```go
  type NativeViewEmbedder interface {
      // NewEmbeddedNativeView puts the app's view (NSView*, HWND, GtkWidget*) into
      // parent's content area above the canvas, hidden until a SetPlacement with a
      // non-empty Clip. closed runs (main thread) when the backend removes the view
      // itself because parent closed. Main thread.
      NewEmbeddedNativeView(parent SurfaceID, view uintptr, opts NativeViewOptions, closed func()) (EmbeddedNativeView, error)
  }
  type NativeViewOptions struct{ PassInput bool }
  type EmbeddedNativeView interface {
      SetPlacement(p WebPlacement) // main thread; cheap when unchanged
      Close()                      // removes the view from parent; does not run closed
  }
  ```
- Produces（核心钩子）: `corehook.NewEmbeddedNativeView func(parent *tango.Surface /*any*/, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error)`，`nil` 后端或不支持时返回包装 `tango.ErrUnsupported` 的错误。
- Produces（公开）:
  ```go
  type NativeHostOptions struct {
      Create    func() (view uintptr, err error) // main thread, once per box lifetime
      Release   func(view uintptr)               // main thread, after removal
      PassInput bool
      KeepAlive time.Duration
      Covered   CoverMode
  }
  type NativeHostW struct{ *ui.NativeViewW /* + unexported */ }
  func NativeHost(c *ui.Context, key string, opts NativeHostOptions, lopts ...layout.Option) *NativeHostW
  func (w *NativeHostW) Err() error
  var ErrNoNativeHost error
  ```

- [ ] **Step 1: fake 后端** —— `fake.Backend` 实现 `NativeViewEmbedder`：记录每个视图的 `view` 句柄、`opts`、最近一次 placement、是否已移除；父 surface 关闭时移除并调用 `closed`；提供测试读取函数（照 `fake/webembed.go` 的 `Placement`/`Embedded` 写法，例如 `NativeViews(parent) []FakeNativeView`）和 `SetNativeEmbedErr(error)`。

- [ ] **Step 2: 写失败测试** `uihost/nativehost_test.go`（照 `uihost/webview_test.go` 的 `open`/`settle` 辅助）：
  - `TestNativeHostFollowsBox`：页面放一个 200×100 的 NativeHost（`Create` 返回 0x1234 并计数），跑两帧；断言 `Create` 调用 1 次、fake 记录 view==0x1234、placement.Frame 等于盒子矩形、Clip 等于可见部分；改变盒子位置后 placement 跟随。
  - `TestNativeHostClippedAndRemoved`：盒子滚出可见区 → Clip 为空；不再构建 → 视图移除、`Release(0x1234)` 调用 1 次。
  - `TestNativeHostKeepAlive`：`KeepAlive: time.Hour` 时不再构建只隐藏不释放；再次构建复用同一视图（`Create` 仍为 1 次）。
  - `TestNativeHostCoverHide`：打开覆盖盒子的 overlay（照 `TestWebViewCoverHideAndIgnore`）→ Clip 为空；`CoverIgnore` 时保持可见；零值 `CoverSnapshot` 表现同 `CoverHide`。
  - `TestNativeHostWindowCloseReleasesOnce`：关闭窗口 → `Release` 恰好 1 次。
  - `TestNativeHostUnsupportedAndCreateError`：`SetNativeEmbedErr` 时 `Err()` 为 `ErrNoNativeHost` 且不调用 `Create`；`Create` 返回错误时 `Err()` 报告该错误，之后的帧不再调用 `Create`。
  - `TestNativeHostPassInputOption`：fake 记录的 opts.PassInput 与选项一致。
  - `TestNativeHostThreaded`（照 `webview_threaded_test.go` 的 `openThreaded`）：多帧后 placement 为最新一帧；`Create`/`Release` 在主线程（用 fake 的主线程断言或记录 goroutine 标记，照现有线程化测试的做法）。

- [ ] **Step 3: 运行确认失败**：`nice -n 19 go test ./uihost -run NativeHost -count=1` → 未定义。

- [ ] **Step 4: 实现**
  - `internal/platform/nativeembed.go`：上面的接口与文档注释。
  - 钩子：根包按 `Surface` 找到 backend，断言接口后调用；父 surface 不是画布（WebView 窗口）或已关闭时返回错误。
  - `uihost/nativehost.go`：
    - UI 侧 `nativeHost{id, opts, built frame, lastSeen, err, failed bool, live bool}`，存于窗口 `embeds` 的新 map（与 WebView 的 `views` 分开）。
    - 每帧 `syncNative()`：对 `NativeRegions()` 中本帧构建的 host 用与 `embedView.decide` 相同的规则算 `platform.WebPlacement`（`CoverSnapshot` 视为 `CoverHide`）；处理 KeepAlive；生成 `[]nativePlace{id, h, opts, placement}`。
    - 主线程侧 `mainNatives{views map[ui.ID]*mainNative}`：`apply(list)` 创建缺失的（调 `Create` → `corehook.NewEmbeddedNativeView`；失败时把错误送回 UI 侧并标记不再重试）、`SetPlacement`、移除不在列表中的（`Close` 后 `Release`）。backend 主动关闭（父窗口关）时的 `closed` 回调里调用 `Release` 并从 map 删除，保证只调用一次。
    - 非线程化窗口直接调用 `apply`；线程化窗口复用 `mainEmbeds.post` 的合并方式（`pending` + 单个 `app.Post` 任务），或为 native 单独一份同样的合并逻辑，二选一以代码最少为准。
    - `NativeHost` 构建器：`ui.NativeView` + `Bg(c.Theme.BgContainer)` + `Focusable(false)`（PassInput 时）；注册到窗口；`Err()` 返回 UI 侧错误。
  - 窗口关闭路径：在现有 embeds 关闭处（WebView 关闭附近）释放所有 native 视图（未被 backend 回调释放的）。

- [ ] **Step 5: 运行确认通过**：`nice -n 19 go test ./uihost ./internal/platform/... -count=1`。
- [ ] **Step 6: API 文件**：`go run ./internal/cmd/apitxt -w`，`-check` 通过。
- [ ] **Step 7: 提交** `feat(uihost): NativeHost embeds an app's native view in the layout`

### Task 2: macOS 后端

**Files:**
- Modify: `internal/platform/darwin/webembed.go`（新增 `embeddedNative` 类型、`TangoEmbedClipPass` 类、`NewEmbeddedNativeView`、`destroyEmbeds` 中关闭原生视图）
- Test: `internal/platform/darwin/nativeembed_test.go`（需真实窗口的测试按现有约定用环境变量门控）

- [ ] **Step 1: 写测试**：门控测试（`TANGO_TEST_NATIVE_EMBED=1`）创建隐藏画布窗口，用 `NSView` 作为应用视图，`SetPlacement(Frame{10,20,200,100}, Clip 同)` 后读回裁剪视图与子视图 frame（scale 换算）、`isHidden` 为 false；Clip 为空时 hidden；`PassInput` 时对裁剪视图中心点 `hitTest:` 结果不是应用视图；`Close` 后应用视图 `superview` 为 nil；父窗口关闭时 `closed` 回调被调用。另加无窗口的单元测试覆盖纯逻辑（若有抽出的函数）。
- [ ] **Step 2: 实现**：复用 `embedHost`、`apply` 的写法（抽出共用的 `placeClip(clip, child, place, hidden, scale)` 或直接复制十几行，以清晰为准）；`TangoEmbedClipPass` 的 `hitTest:` 恒返回 nil。
- [ ] **Step 3: 运行**：`nice -n 19 go test ./internal/platform/darwin -count=1`；`TANGO_TEST_NATIVE_EMBED=1` 的门控测试本机跑一次（窗口隐藏，不抢前台）。
- [ ] **Step 4: 实机核对**：临时程序用 `NativeHost` 嵌入一个带背景色图层的 NSView，PassInput 开；后台截图确认位置正确、点击落到画布（页面收到 Press）。
- [ ] **Step 5: 提交** `feat(darwin): embed app native views`

### Task 3: Windows 后端

**Files:**
- Modify: `internal/platform/windows/webembed_windows.go`（新增 `embeddedNative`：容器窗口同 `TangoWebEmbed` 类，`SetParent(view, container)`，`embedPlan` 摆放，`restackEmbedded` 纳入，父关闭时关闭并回调）
- 可能新增：`internal/platform/windows/nativeembed_windows.go`（子类化与 `HTTRANSPARENT`）

- [ ] **Step 1: 测试**：纯逻辑部分（若新增 plan 函数）放无构建标签文件并测试；其余靠 `GOOS=windows go vet` 与 `go test -c` 编译。
- [ ] **Step 2: 实现**：容器创建与摆放照 `newEmbedded…`/`apply`；应用视图作为容器子窗口，摆放用 `SetWindowPos`（Frame 相对容器）；`PassInput` 时对容器和应用视图 `SetWindowSubclass`，`WM_NCHITTEST` 返回 `HTTRANSPARENT`；`Close`：移除子类、`ShowWindow(SW_HIDE)`、`SetParent(view, 0)`、销毁容器。
- [ ] **Step 3: 运行**：`GOOS=windows nice -n 19 go vet ./internal/platform/windows/ ./uihost/`；`GOOS=windows nice -n 19 go test -c -o /dev/null ./internal/platform/windows ./uihost`；本机能跑的 windows 包测试。
- [ ] **Step 4: 提交** `feat(windows): embed app native views`

### Task 4: Linux 后端

**Files:**
- Modify: `internal/platform/linux/webembed_linux.go`（新增 `embeddedNative`：同样的 GtkLayout overlay 子项，`gtk_layout_put(view)`，`embedTracker` 摆放，PassInput 用 overlay pass-through + realize 时 `gdk_window_set_pass_through`）

- [ ] **Step 1–3**：实现；`GOOS=linux nice -n 19 go vet ./internal/platform/linux/ ./uihost/`；`GOOS=linux go test -c`；纯逻辑测试本机跑。
- [ ] **Step 4: 提交** `feat(linux): embed app native views`

### Task 5: 文档与收尾

- [ ] `docs/specs/webview-embed.md` 增加「应用原生视图」一节（所有权、PassInput、Covered、平台差异、不支持的情况）；`CHANGELOG.md` 未发布「新增」一条。
- [ ] 全量检查（同 Global Constraints），全部通过后提交 `docs: NativeHost`。
- [ ] 独立审查一次整分支，修复后重跑检查；汇报并询问是否合并推送。
