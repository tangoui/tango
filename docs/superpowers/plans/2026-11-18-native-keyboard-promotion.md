# 大窗口默认 native、键盘输入升级 Metal 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** macOS 自绘窗口不论大小默认都用 native 图层，收到键盘输入的窗口在下一帧单向升级到 Metal。

**Architecture:** 先恢复 `3acc4cd` 的渲染器选择：`setupGPU` 的默认分支不再判断窗口面积。再在 `(*host).CanvasInput` 里识别键盘类事件，把该窗口的 `nativeRendererState` 标为待升级；下一次 `Render` 走现有的 `promoteNative()`。

**Tech Stack:** Go 1.27.3，`uihost`、`ui/input`。

**Spec:** `docs/superpowers/specs/2026-11-18-native-keyboard-promotion-design.md`

## Global Constraints

- 工具链：`GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0`；go 命令统一加 `nice -n 19` 和 `-p 1`。
- 触发升级的输入：
  - `input.KeyEvent`，同时满足 `Down == true`、`Key != 0`、`Key.Modifier() == 0`、`Key != input.KeyCapsLock`；
  - `Text` 非空的 `input.TextInput`；
  - `Text` 非空的 `input.Composition`。
  
  被窗口快捷键（`h.sc.handle`）吃掉的按键同样触发。
- 不触发升级的输入：`input.Scroll`、`input.Pointer`、`input.TouchEvent`、`input.WindowFocus`、只按修饰键或 Caps Lock、按键抬起。
- `TANGO_RENDERER=native`（强制模式，`g.native == nil`）时任何输入都不升级；`gpu`、`cpu`、`auto` 的行为不变。
- 升级只作用于收到输入的那个窗口，并且只尝试一次。
- 提交作者：`git -c user.name=tango -c user.email=903300326@qq.com commit`，不加任何 trailer。
- 不打开 GUI 窗口。

## Review Focus

1. **按键在第一帧之前到达**：窗口刚建好、还没渲染过就收到按键（`h.gpu` 还没创建，或 `g.native` 还没设置），这时不能 panic；第一次 `Render` 时要直接升级。归 Task 1 测试 `TestKeyBeforeFirstFrameStillPromotes`。
2. **升级失败后又收到按键**：`promoteNative` 失败退回 CPU 后，`g.native` 已经是 nil，后续按键不能再次触发升级尝试。归 Task 1 测试 `TestKeyAfterFailedPromotionDoesNotRetry`。
3. **自动重复的按键**：长按产生的 `Repeat == true` 按下事件触发升级没有问题，但只能升级一次。由 Task 1 测试 `TestKeyEventPromotesOnNextFrame` 覆盖：连续送 3 个按下事件，断言只升级一次。

---

### Task 1：恢复大窗口默认 native，加键盘升级，更新文档

**Files:**
- Modify: `uihost/gpu.go`（`setupGPU` 默认分支）
- Modify: `uihost/native.go`（新增 `func (g *gpuRenderer) keyboardInput()`）
- Modify: `uihost/uihost.go`（`(*host).CanvasInput`）
- Modify: `uihost/native_test.go`（恢复 `TestNativeDefaultCoversLargeWindows`，新增下面的测试）
- Modify: `docs/dev-builds.md`（“自绘窗口的渲染分级”一节）、`CHANGELOG.md`（未发布 → 变更）

**Interfaces:**
- Produces:
  - `func isKeyboardInput(ev input.Event) bool`（放在 `uihost/native.go`），按 Global Constraints 的规则判断
  - `func (g *gpuRenderer) keyboardInput()`：当 `g != nil && g.native != nil` 时设 `g.native.ready = true`，否则什么也不做
  - `CanvasInput` 在调用 `h.sc.handle` 之前，先执行 `if isKeyboardInput(ev) { h.gpu.keyboardInput() }`

- [ ] **Step 1：写失败测试**（`uihost/native_test.go`，复用 `nativeTestHooks`、`nativeCanvasTest`、`largeNativeCanvasTest`）

```go
func TestNativeDefaultCoversLargeWindows(t *testing.T)   // 恢复 3acc4cd 的版本：1000×800 窗口在默认设置下 nativeMode 为真、g.native != nil、不碰 Metal
func TestKeyEventPromotesOnNextFrame(t *testing.T)       // 默认 native：连续 3 次 g.keyboardInput()（模拟 KeyEvent{Key: input.KeyA, Down: true}，第 2、3 次 Repeat=true）后调用一次 Render，nativeMode 变为假且 g.gpu != nil；再次 Render 时 gpuTarget 不再被调用（只升级一次）
func TestIsKeyboardInput(t *testing.T)                   // 表驱动：KeyA 按下 → true；KeyA 抬起 → false；KeyShiftLeft、KeyMetaLeft、KeyCapsLock 按下 → false；Key 0 → false；TextInput{"a"} → true；TextInput{""} → false；Composition{Text:"ni"} → true；Composition{} → false；Scroll{}、Pointer{}、WindowFocus{} → false
func TestForcedNativeIgnoresKeyboard(t *testing.T)       // TANGO_RENDERER=native：keyboardInput 后 Render，nativeMode 仍为真
func TestKeyboardPromotesOnlyThatWindow(t *testing.T)    // 两个 setupGPU 结果，只对第一个调用 keyboardInput；各 Render 一次后，第一个已升级，第二个仍是 native
func TestKeyBeforeFirstFrameStillPromotes(t *testing.T)  // Review Focus 1：setupGPU 之后、第一次 Render 之前调用 keyboardInput，第一次 Render 即完成升级；对 nil *gpuRenderer 调用 keyboardInput 不 panic
func TestKeyAfterFailedPromotionDoesNotRetry(t *testing.T) // Review Focus 2：让 gpuTarget 返回错误；第一次升级失败后 g.cpu != nil；再调用 keyboardInput 并 Render，gpuTarget 的调用次数不增加
func TestCanvasInputRoutesKeyboard(t *testing.T)         // host 层：构造带默认 native gpuRenderer 的 host，CanvasInput(KeyEvent{KeyA, Down:true}) 后 g.native.ready 为真；CanvasInput(Scroll{}) 后仍为假（如果现有测试设施能构造 host；不能的话，在报告里说明，只保留上面的 renderer 层测试）
```

- [ ] **Step 2：运行，确认失败**

运行：`nice -n 19 env GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0 go test -p 1 ./uihost/ -run 'TestNativeDefaultCoversLargeWindows|TestKey|TestIsKeyboardInput|TestForcedNativeIgnoresKeyboard|TestCanvasInputRoutesKeyboard' -count=1`
预期：编译失败（`keyboardInput`、`isKeyboardInput` 未定义），或者 `TestNativeDefaultCoversLargeWindows` 失败

- [ ] **Step 3：实现**

`setupGPU` 的默认分支改为 `if mode == "native" || mode == "" {`，注释写明：默认对所有大小的窗口都用 native；窗口收到键盘输入时（见 `keyboardInput`），或连续帧开销过高时，单向升级 Metal。然后按 Interfaces 实现 `isKeyboardInput` 和 `keyboardInput`，并接入 `CanvasInput`。

- [ ] **Step 4：运行 uihost 全部测试**

运行：`nice -n 19 env GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0 go test -p 1 ./uihost/ -count=1 && nice -n 19 env GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0 go test -p 1 -tags tango_dev ./uihost/ -count=1 && nice -n 19 env GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0 go vet ./uihost/`
预期：全部通过

- [ ] **Step 5：更新文档**

在 `docs/dev-builds.md` 里，把“未设置”那一条改写为：macOS Apple Silicon 上的普通自绘窗口不论大小都先用原生图层；窗口收到键盘输入（按下非修饰键、提交文字、输入法组字）时，下一帧单向升级 Metal；滚动和指针输入不触发升级；原有的 8 ms 规则保留。同时把撤回段落改写为现行规则的说明，保留内存数据（list 39 对 135 MB、quotes 54 对 147 MB）和延迟数据（打字 78.8 对 47.2 ms、三光标 87.6 对 47.2 ms、滚动约 80 对 46 ms），并写明纯滚动浏览的窗口仍按 native 的延迟运行。

`CHANGELOG.md`【未发布】→“变更”加一条：macOS 大型自绘窗口默认改用原生图层，收到键盘输入后升级 Metal（内存与延迟数字同上）。

- [ ] **Step 6：提交**

```bash
git add uihost/gpu.go uihost/native.go uihost/uihost.go uihost/native_test.go docs/dev-builds.md CHANGELOG.md
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(uihost): native layers by default, promote to Metal on keyboard input"
```

---

### 验收（controller 负责，不派给实现代理）

1. 请 min-78 在本分支上做配对复测：编辑器打字 100 键、三光标 30 次，比较默认设置和 `TANGO_RENDERER=gpu`，按 AB/BA/AB 交替。中位数差距不超过 16.7 ms；第一次按键单独记录。
2. 用后台 automation 打开 list 示例，不按任何键，确认 stderr 里渲染器一直是 native。
3. 通过后，把分支 rebase 到 origin/main 并推送，然后通知各会话拉取。

> 2026-11-18 修订：与 `ui.RunnerOptions.Content` 合并后的最终行为（ContentData 窗口不按键升级）见设计文档的“2026-11-18 修订”一节：`docs/superpowers/specs/2026-11-18-native-keyboard-promotion-design.md`。
