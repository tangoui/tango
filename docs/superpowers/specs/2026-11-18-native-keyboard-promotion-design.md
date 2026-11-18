# 大窗口默认 native，收到键盘输入后升级 Metal（简要设计）

日期：2026-11-18 · 状态：待用户审阅

## 目的

`3acc4cd` 把 macOS 大窗口的默认渲染器改成 native（Core Animation 图层），省下 63–71% 的内存。但 min-78 实测，native 下编辑器的“输入→画面”延迟比 Metal 慢约 2 帧（打字 78.8 对 47.2 ms，三光标 87.6 对 47.2 ms，滚动约 80 对 46 ms），所以已在 `a8f5cd4` 回退。

用户决定：大窗口仍默认用 native，**窗口一收到键盘输入就自动切到 Metal**。滚动不算触发条件。这样，有键盘输入的窗口（编辑器、终端、表单）恢复 Metal 的输入延迟，只看不打字的窗口（数据表格、行情、列表、仪表盘）保住 native 的内存优势。

## 行为

- `TANGO_RENDERER` 未设置时，macOS Apple Silicon 上的普通自绘窗口**不论大小**都先用 native，即恢复 `3acc4cd` 的选择逻辑。
- 窗口收到以下任一输入时，立即标记这个窗口需要升级，下一帧切到 Metal：
  - 按键按下的 `input.KeyEvent`；只按修饰键（Shift、Control、Option、Command、Caps Lock、Fn）不算；
  - `input.TextInput`；
  - `input.Composition`（输入法组字）。
- 被窗口快捷键吃掉的按键同样算，因为按快捷键之后通常紧接着就要打字。
- 升级是单向的，和现有的 `promoteNative` 共用同一条路径：之后一直留在 Metal；升级失败时呈现 CPU 图像，并且不再重试。
- 不触发升级的输入：滚轮、触控板滚动、指针、触摸、窗口焦点变化。
- 现有的“连续 6 帧绘制超过 8 ms 就升级”规则保留。
- `TANGO_RENDERER=native` 强制 native，任何输入都不升级。`gpu`、`cpu`、`auto` 三种取值的行为都不变。
- 只影响收到键盘输入的那一个窗口，其他窗口不受影响。

## 已知代价

- 第一次按键的那一帧要建立 Metal（设备、管线、atlas），会多一次性开销。用户选择自动判断时已接受这一点。
- 窗口只要打过一次字，就永久失去 native 的内存优势。
- 纯滚动浏览的窗口仍比 Metal 慢约 2 帧，这是用户的取舍。

## 方案对比

| 方案 | 取舍 |
|---|---|
| **自动：收到键盘输入就升级（用户已选）** | 对应用透明；首次按键有一次切换开销 |
| 由应用声明窗口类型 | 没有切换开销，但应用开发者必须知道要设置 |
| 键盘和滚动都触发 | 滚动延迟也能恢复，但几乎所有窗口迟早都会滚动，内存优势基本没了，用户已否决 |

## 实现要点（交给实现者）

- 在 `uihost/uihost.go` 的 `(*host).CanvasInput` 里判断事件类型。如果 `h.gpu` 处于默认 native 模式（`h.gpu.native != nil`），就设置 `h.gpu.native.ready = true`（或者一个意义相同的新字段）。下一次 `Render` 调用现有的 `promoteNative()`。
- 修饰键的判断用 `ui/input` 现有的键码定义。
- 恢复 `3acc4cd` 对 `uihost/gpu.go`、`uihost/native_test.go` 的改动，这部分可以 revert 那次回退，也可以手写。文档按新行为更新，不要原样恢复旧文案。

## 测试

在 `uihost` 包里写单元测试，用现有的 `nativeTestHooks` 等测试替身：
- 默认 native 下，非修饰键的 `KeyEvent` 送达后，下一次 `Render` 完成升级（`nativeMode` 变为 false，`gpu` 非空）。
- `TextInput`、`Composition` 同样触发升级。
- 只按修饰键、`Scroll`、`Pointer`、`WindowFocus` 都不触发升级。
- `TANGO_RENDERER=native` 时，`KeyEvent` 不触发升级。
- 两个窗口里只有收到按键的那个升级。
- 大窗口默认选 native（恢复 `TestNativeDefaultCoversLargeWindows`）。

## 验收

1. `go test ./uihost/` 默认构建通过，`go test -tags tango_dev ./uihost/` 也通过。
2. 请 min-78 用同样的方法（真实窗口、CGEvent 输入、ScreenCaptureKit 采样、AB/BA/AB 配对）复测编辑器的打字和三光标场景，比较默认设置和 `TANGO_RENDERER=gpu`：两者的中位数差距在 1 帧（16.7 ms）以内。第一次按键单独记录，不计入中位数。
3. quotes、list 这类不打字的窗口仍是 native：日志里没有升级，内存和 11-17 的实测相当。
4. 更新 `docs/dev-builds.md` 和 `CHANGELOG.md`，写明新规则、内存数据和延迟数据。

## 2026-11-18 修订：与 `ui.RunnerOptions.Content` 合并

`36b6d3b`（origin/main）新增了实验性的 `ui.RunnerOptions.Content`：`ui.ContentData` 标明表格、网格、行情、列表、仪表盘这类数据窗口，让它们不限大小走 native；默认的 `ui.ContentGeneral` 让大窗口留在 Metal。用户决定把它和本设计合并，最终行为如下：

1. `TANGO_RENDERER` 未设置时（支持的 macOS），所有窗口不论大小、不论 `Content` 都先用 native。
2. `ContentGeneral` 窗口按上面“行为”一节的规则，收到键盘输入后在下一帧升级 Metal。
3. `ContentData` 窗口收到键盘输入不升级，只有“连续 6 帧超过 8 ms”这条规则会让它升级。这样用方向键、翻页键浏览的数据表格能一直保住 native 的内存优势，代价是它的输入延迟按 native 计。
4. `TANGO_RENDERER=native` 强制 native，永不升级；`gpu`、`cpu`、`auto` 以及 `RunnerOptions.Renderer` 的优先级都不变。

实现方式：`startNative` 把 `ropts.Content` 记进 `nativeRendererState.content`，`keyboardInput` 遇到 `ContentData` 时什么也不做。测试见 `TestContentDataIgnoresKeyboard`、`TestContentGeneralPromotesOnKeyboard`。
