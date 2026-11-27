# KindOverlay（不取焦点浮层，macOS）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 Tango 新增窗口类型 `KindOverlay`：永远不取焦点、显示时不激活应用、层级高于全屏应用、出现在所有桌面、鼠标可用。第一期只做 macOS，并用它给示例输入法加上候选窗。

**Architecture:** 核心层新增 `platform.KindOverlay` 和标记接口 `platform.OverlaySupport`；backend 不支持时，核心层返回 ErrUnsupported。darwin 用 NSPanel 子类 `TangoOverlay`（不能成为 key/main），显示时走 orderFrontRegardless，使用专用的层级和 collectionBehavior；`applyAppearance`、`SetState`、`Focus` 对浮层套用固定规则。自绘画布的浮层使用 ActiveAlways 跟踪区域。`tan dev` 下复用现有协议的 `WindowOpts.Kind`。

**Tech Stack:** Go、purego/objc、AppKit。

**Spec:** `docs/superpowers/specs/2026-11-27-kind-overlay-design.md`（务必通读，尤其是"实现者审查后补充的决定"和"自审补充"两节）

## Global Constraints

- 只用 purego，不加 .m 文件，不开 cgo。
- 现有各 Kind（Window/Panel/Modal/Sheet）的行为一字不变。
- 非 darwin 后端创建 `KindOverlay` 返回包装了 `ErrUnsupported` 的错误，绝不退回成普通窗口。
- 浮层永远不成为 key/main window，`Focus()` 不做任何事，`IsFocused()` 恒为 false，`Show()` 不激活应用。
- 浮层规则：`TitleBar` 一律当 None；`Resizable`、`AlwaysOnTop`、`Parent` 被忽略；`SetAppearance` 中 Transparent、Background、Material、NoShadow、Opacity 生效；`SetState` 只接受 Normal，其余返回 ErrUnsupported。
- 提交作者固定 `tango <903300326@qq.com>`，不加 Co-Authored-By。每次提交前：`gofmt -l`（含带构建标签的文件）无输出；`GOOS=windows`、`GOOS=linux` 交叉 vet 改动的包；`go run ./internal/cmd/apitxt -check` 通过；改了 cmd/tan 就用 GOWORK=off 单独编译和测试。
- 只在 worktree `/Users/admin/min/.tango-overlay` 工作，不推送、不合并。

## Review Focus

1. 运行中对浮层调用 `SetAppearance`（比如切换透明或阴影）之后，浮层仍然不能成为 key、层级和 collectionBehavior 不变。→ Task 2 的 smokeext 检查。
2. 通过 `uihost.Open` 创建自绘浮层时，打开流程里的 Centered、初始 State、afterShow 都不会让它变成 key 或激活应用。→ Task 3 的 smokeext 检查。
3. 应用不在前台时，鼠标在自绘浮层上移动能收到 hover 和 enter 事件。→ Task 3 的跟踪区域选项测试，加上 Task 6 的实测。
4. 在 Windows/Linux backend 或不支持的 dev 宿主上创建浮层：返回 ErrUnsupported，不能 panic，也不能建出普通窗口。→ Task 1、Task 4。
5. 候选窗显示时，前台文本编辑始终保持 key window，继续打字照常进入文本编辑。→ Task 6。

---

### Task 1: 核心层 KindOverlay 与支持检查

**Files:** Modify `internal/platform/ext.go`（在 `KindSheet` 之后加 `KindOverlay`，并新增 `type OverlaySupport interface{ SupportsOverlay() bool }`）、`surface_ext.go`（导出 `KindOverlay = platform.KindOverlay`，写完整的文档注释，内容来自设计文档的"目标"和"约束"，包括 WebView 悬停尽力而为、未打包应用启动时会激活一次这两条限制）、`surface_ext.go` 的 `extras`/`prepareNative` 路径（`KindOverlay` 时检查 `a.b.(platform.OverlaySupport)`，不支持就返回 `fmt.Errorf("tango: KindOverlay: %w", ErrUnsupported)`）；更新 `api/go` 下根包的 golden 文件；在 site/guide 中列举 Kind 的页面补一行。
**Test:** 根包新建 `overlay_test.go`。

**Interfaces:**
- Produces: `platform.KindOverlay`、`tango.KindOverlay`、`platform.OverlaySupport`。

- [ ] **Step 1:** 用根包已有的假 backend 测试设施（找 surface_ext 相关测试里的 fake backend）写测试。`TestOverlayUnsupported`：假 backend 没实现 OverlaySupport 时，`NewSurface(SurfaceOptions{Kind: KindOverlay})` 返回 `errors.Is(err, ErrUnsupported)`，而且假 backend 没收到创建调用。`TestOverlaySupported`：实现了并返回 true 时，backend 收到的 `SurfaceExtras.Kind == platform.KindOverlay`。`TestOverlayIgnoresParent`：设置了 Parent 也不报 errNeedParent。
- [ ] **Step 2:** 运行 `go test -run Overlay .`，应为 FAIL。
- [ ] **Step 3:** 实现。canvas surface 的创建路径（`corehook.NewCanvasSurface` 的实现）也要走同一个检查。
- [ ] **Step 4:** 测试通过，`apitxt -check` 通过，交叉 vet 通过。
- [ ] **Step 5:** 提交 `tango: KindOverlay surface kind (backend opt-in)`。

### Task 2: darwin 浮层窗口

**Files:** Modify `internal/platform/darwin/surface_ext.go`（`windowClass`、`kindStyle`、`initExt`、`applyAppearance`、collectionBehavior 的计算、`SetState`、`SetResizable`）、`internal/platform/darwin/surface.go`（`Show`、`Focus`、`IsFocused`）、窗口类注册处（新增类 `TangoOverlay`，父类 NSPanel，`canBecomeKeyWindow` 和 `canBecomeMainWindow` 都返回 NO），backend 实现 `SupportsOverlay() bool { return true }`。
**Test:** `internal/platform/darwin/overlay_test.go`（纯函数），以及在 `internal/platform/darwin/internal/smokeext/main.go` 里加真实窗口检查（照现有 panel 检查的写法）。

**Interfaces:**
- Produces（darwin 包内）：`overlayStyle() uint`（Borderless 0 | NonactivatingPanel 1<<7）、`overlayCollectionBehavior() uint`（CanJoinAllSpaces 1 | Stationary 1<<4 | IgnoresCycle 1<<6 | FullScreenAuxiliary 1<<8 = 337；Managed、Transient、Stationary 三者互斥，只用 Stationary）、`overlayLevel() int`（按设计文档的做法实测选定，在注释里写明理由）。

- [ ] **Step 1:** 写纯函数测试。`TestOverlayStyle`：等于 128，并且不含 Titled(1)、Resizable(8)。`TestOverlayCollectionBehavior`：等于 337，不含 FullScreenPrimary(128)、Transient(8)。`TestOverlayLevel`：大于 `nsFloatingWindowLevel`，并且大于等于 101（NSPopUpMenuWindowLevel）。在 smokeext 里加 `overlay` 检查：建一个 KindOverlay surface 并 Show，断言 `canBecomeKeyWindow` 为 NO、`isKeyWindow` 为 NO、`[NSApp isActive]` 和 Show 之前相同、`level == overlayLevel()`、`collectionBehavior == 337`、`isVisible` 为 YES；调用 `Focus()` 后 key window 不变；`SetAppearance(Transparent: true, NoShadow: true)` 之后再断言一遍，并检查 styleMask 仍不含 Titled；`SetState(Maximized)` 返回 ErrUnsupported。
- [ ] **Step 2:** 运行 `go test ./internal/platform/darwin -run Overlay`，应为 FAIL；smokeext 的 overlay 检查也应失败。
- [ ] **Step 3:** 实现。Show 中浮层分支只调用 `orderFrontRegardless`，放在 `launchBackground` 判断之前。`applyAppearance` 在浮层上跳过标题栏、样式重置和层级重置，collectionBehavior 固定为 337。`setHidesOnDeactivate:NO`，`setFloatingPanel:YES`。
- [ ] **Step 4:** 纯函数测试和 smokeext 都通过；现有的 `TestWindowCollectionBehavior` 等测试不变且通过。
- [ ] **Step 5:** 提交 `darwin: KindOverlay windows never take focus`。

### Task 3: 自绘画布浮层与 WebView 浮层

**Files:** Modify `internal/platform/darwin/canvas.go`（`installScheduling` 中，浮层用 trackActiveAlways 0x80 替代 0x40）；如有必要，修改 WebView 浮层的第一次点击处理；在 smokeext 中补检查；在 `KindOverlay` 文档注释里写明 WebView 的限制。
**Test:** `internal/platform/darwin/overlay_test.go` 追加测试。

**Interfaces:** Produces `canvasTrackingOptions(overlay bool) uint`。

- [ ] **Step 1:** `TestCanvasTrackingOptions`：overlay=false 时等于现有值 0x247（Entered|Moved|Cursor|ActiveInApp|InVisibleRect）；overlay=true 时等于 0x287（把 0x40 换成 0x80）。smokeext 中用 `NewCanvasSurface`（以及 uihost.Open，如果 smokeext 方便使用）建一个 KindOverlay 并 Show，断言不是 key、应用未被激活、canvas view 的 `acceptsFirstMouse:` 为 YES；再建一个 WebView KindOverlay，断言不是 key、应用未被激活。
- [ ] **Step 2:** 运行测试，应为 FAIL。
- [ ] **Step 3:** 实现。WKWebView 在浮层里的第一次点击：实测一下；如果需要先点一次才响应，就给浮层 WebView 的容器 view 处理 acceptsFirstMouse。实在做不到，把限制写进文档。
- [ ] **Step 4:** 测试通过。
- [ ] **Step 5:** 提交 `darwin: overlay canvases track the mouse while the app is inactive`。

### Task 4: tan dev 下的浮层

**Files:** 涉及 `uihost/remote_dev.go`（worker 侧 Open）、`internal/devhost/host/host.go`（宿主按 `WindowOpts.Kind` 建窗口），以及 WebView 代理路径 `proxy_surface_hook.go`（核对 Kind 是否传递、开发代理是否需要声明 OverlaySupport）。测试放在 `internal/devhost/host/host_test.go` 和/或 `uihost` 中带 tango_dev 标签的测试里，照现有的 dev 测试写法。

- [ ] **Step 1:** 写测试。`TestDevOverlayKind`：worker 发出 `WindowOpts{Kind: int(tango.KindOverlay)}`，宿主（使用支持浮层的假 backend 或真实 darwin backend）建出的 surface 的 Kind 是 KindOverlay。`TestDevOverlayUnsupported`：宿主 backend 不支持时，worker 的 Open 收到包装了 ErrUnsupported 的错误，不会卡住。
- [ ] **Step 2:** 运行 `go test -tags tango_dev ./internal/devhost/... ./uihost/...`，应为 FAIL（如果已经能通过，就记录"现有协议已经支持"，只保留测试）。
- [ ] **Step 3:** 补上缺失的部分：错误回传给 worker；WebView 代理声明支持或透传 Kind。
- [ ] **Step 4:** 测试通过；`go test ./uihost/...`（不带标签）也通过。
- [ ] **Step 5:** 提交 `uihost: KindOverlay under tan dev`。

### Task 5: 示例输入法的候选窗

**开始前：** `git fetch` 或直接 `git rebase main`，拿到 Codex 在 main 上对 `examples/inputmethod`、`sys/ime` 的最新改动。有冲突时以 main 为准，并在汇报里说明。

**Files:** Modify `examples/inputmethod/main.go`；新建 `examples/inputmethod/candidates.go`（候选窗）和 `examples/inputmethod/candidates_test.go`。

**Interfaces:** Consumes `tango.KindOverlay`、`uihost.Open`、`ime.Session.Caret()`、`engine.Result.Candidates`。Produces：`candidateBounds(caret tango.Rect, w, h int, work tango.Rect) tango.Rect`，把候选窗放在光标下方 4px；超出屏幕底部时放到光标上方；水平方向夹在工作区内。另有 `pick(c *engine.Composer, index int) string`：选第 index 个候选（从 0 开始），返回要上屏的文字并清空组字。

- [ ] **Step 1:** 写测试。`TestCandidateBounds`：caret {100,200,2,18}、尺寸 300x40、工作区 {0,0,1440,900} 时得到 {100,222,300,40}；caret Y=880 时窗口放到光标上方 {100,836,300,40}；caret X=1300 时 X 被夹到 1140。`TestPick`：组字 "ni" 时 pick(1) 返回 "泥"，之后 Composing() 为 false；index 越界时返回 ""，并保持组字不变。数字键 1–9 选词：`TestHandleDigitPicks`，组字 "nihao" 时按 Digit2 上屏 "拟好"。
- [ ] **Step 2:** 运行 `go test ./examples/inputmethod/...`，应为 FAIL。
- [ ] **Step 3:** 实现候选窗：一个 KindOverlay 的 uihost 窗口（Transparent，圆角卡片，候选词横向排列，每个前面带序号），只在有候选时显示，组字结束（上屏、Esc、Deactivate）时 Hide；位置用 `candidateBounds(s.Caret(), …)`，工作区取光标所在显示器（用 sys/screen）。点击候选词等同于 pick 后 `s.Commit`；悬停高亮。引擎中 pick 的逻辑放进 engine 包（`(*Composer).Pick(i int) (string, bool)`），engine 的已有测试保持通过。
- [ ] **Step 4:** 测试通过；`tan build` 成功。
- [ ] **Step 5:** 提交 `examples: candidate window on a KindOverlay surface`。

### Task 6: 真机验收（必须先问用户）

**本任务会占用用户的前台和键盘，并切换系统输入源，切换期间用户在任何应用里的真实键入都会被测试输入法截走。开始前必须停下来，明确征得用户同意，并请用户在测试期间不要打字。** 用户没同意就跳过本任务，只汇报"待验收"。

- [ ] **Step 1:** 用户同意后：记录当前输入源 → 安装测试输入法 → register、enable → select（失败返回 -50 时每秒重试，最多 10 次）。让文本编辑新文档到前台，用真实按键输入：`ni` 时检查候选窗出现在光标附近，文本编辑仍是 key window；按 `2` 上屏 "泥"；输入 `nihao` 后用鼠标点击第一个候选，上屏 "你好" 且文本编辑仍是 key；输入 `wo` 后按 Esc，候选窗消失。同时确认输入法进程不出现在 Dock 和 Cmd+Tab 中。再在一个全屏应用里输入一次，确认浮层可见。
- [ ] **Step 2:** 截图和读回的文本存到 `/Users/admin/min/outputs/tango-overlay/`。
- [ ] **Step 3:** 收尾：切回原输入源，disable 并删除测试输入法，关闭测试文档（不保存），退出全屏。最后核对原输入源已恢复。
- [ ] **Step 4:** 如果有脚本改动，提交 `examples: overlay candidate window acceptance`。
