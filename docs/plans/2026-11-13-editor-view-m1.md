# 代码编辑器 视图里程碑 1（性能原型）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 GPU 渲染之上做出最小可用的代码编辑器视图（显示、语法高亮、行号、光标、选区、输入、IME、滚动），并按规格 6.6 实测大文件性能，决定后续编辑核心（里程碑 2）的实现方向。

**Architecture:** `ui/editor/highlight` 把 `monarch` 接成按时间预算推进、后台补齐、按编辑增量失效的高亮管线；`ui/text` 新增一行多样式整形 `ShapeRuns` 与按行缓存 `LineCache`（等宽快速路径）；`ui/editor` 的 `View` 组件只排版与绘制可见行，读快照、按主题把 token 映射成样式，用 `paint.Damage` / `paint.ScrollRegion` 提示局部重绘（GPU 里程碑 2 提供）。

**Tech Stack:** Go 1.27；已完成的 `ui/editor/{buffer,regex,langs,monarch,theme}`；`ui/text`、`ui/paint`、`ui`（即时模式组件）；GPU 渲染（`ui/gpu`、`uihost`）。

**Spec:** `docs/specs/code-editor.md`（6.1 基础层扩展、6.3、6.4、6.6、6.7 里程碑 1；第 4 节清单的显示/编辑子集）。

## Global Constraints

- 模块 `github.com/tangoui/tango`，Go 1.27；不新增外部依赖。
- 文本核心的契约（见各包文档）：`buffer.Snapshot` 不可变、`Version` 只在同一 Buffer 内单调；锚点每次 `Resolve` 后重新锚定；`monarch.Lexer.TokenizeLineInfo` 的 `Degraded` 行不可缓存为"状态未变"、需稍后重试；`Registry.Add` 之后该语言已有 State 全部作废；`Token.Start` 是 UTF-16 列、`ByteStart` 是字节。
- 组件 API 遵循 `ui/AUTHORING.md` 的即时模式风格：`editor.View(c *ui.Context, st *editor.State) *ViewW`，状态由应用持有。
- **文件归属：** `ui/text` 由会话 min-43 先完成双向文字（bidi）与 `Lang`，**Task 3、Task 4 必须等控制者确认 min-43 完成后才开始**，并在其接口之上扩展（保持 `Layout` 现有语义与 min-a6 的 RichText/Markdown 兼容——只新增 API）。`ui/paint`、`ui/runner.go`、`ui/window.go` 本计划不改（Damage/ScrollRegion 由 GPU 里程碑 2 提供；若届时尚未提供，View 不发提示，功能不受影响）。`uihost/**`、`internal/platform/**` 不改。
- 本里程碑只做桌面；不做移动端触控。
- **不提交 git**（提交由会话 min-a6 统一做）。每个任务结束时 `ui/editor/**` 与 `ui/text/**` 必须可编译，`go vet` 在 darwin/linux/windows 干净。
- 不打开 `~/go/pkg/mod/github.com/egoist/mygo`；不照搬 monaco-vim。Monaco 源码（MIT）可读以理解语义。

## Review Focus

1. 在 100 万行文件末尾输入后立即滚到开头：高亮不阻塞输入，开头的行显示已有 token，不出现整屏闪烁 —— Task 1 `TestHighlightEditFarAway`。
2. 含 CJK、Tab、emoji、组合字符的行：光标位置、选区矩形与字形对齐，点击定位到正确的 UTF-8 偏移 —— Task 6 `TestCaretAndHitTestMixedScripts`。
3. 输入法组字中途滚动或失焦：组字文本不进入撤销历史、不丢失，候选窗位置跟随光标 —— Task 6 `TestIMECompositionLifecycle`。
4. 一行 2 万字符的压缩 JS：横向滚动与光标移动不卡顿，降级行稍后重试 —— Task 5 `TestVeryLongLineView`。
5. 文件为空 / 只有一个换行 / 无语言：视图正常显示、可输入 —— Task 5 `TestEmptyAndPlainDocuments`。

---

### Task 1: 高亮管线

**Files:**
- Create: `ui/editor/highlight/highlight.go`、`ui/editor/highlight/store.go`
- Test: `ui/editor/highlight/highlight_test.go`、`ui/editor/highlight/bench_test.go`

**Interfaces:**
- Consumes: `buffer.Snapshot`、`monarch.Lexer`（`Initial`、`TokenizeLineInfo`）、`monarch.Token`。
- Produces:
  - `type Highlighter struct{...}`；`func New(lx *monarch.Lexer) *Highlighter`
  - `func (h *Highlighter) Edit(old, new buffer.Snapshot, edits []buffer.Edit)`：按编辑把受影响行起的状态标记为脏（行号偏移随插入/删除行数平移）。
  - `func (h *Highlighter) Highlight(snap buffer.Snapshot, upToLine int, budget time.Duration) bool`：前台在预算内把 `upToLine` 之前的脏行分词，返回是否全部就绪。
  - `func (h *Highlighter) Tokens(snap buffer.Snapshot, line int, dst []monarch.Token) ([]monarch.Token, bool)`：返回该行 token 与"是否最终"（未就绪时返回上一版 token 或空）。
  - `func (h *Highlighter) Start(ctx context.Context, snapshots <-chan buffer.Snapshot, ready func())`：后台 goroutine 读最新快照补齐剩余行，完成一批后调用 `ready`（View 用它 Invalidate）。
  - 行状态存储：每行一个 `monarch.State`（uint32）+ token 切片（按需，超出可见区附近 ±2000 行的 token 可丢弃、只留状态）；增量停止条件：某行结束状态与旧值相同且该行不是 Degraded。
- [ ] **Step 1: 写失败测试** `TestHighlightMatchesFullTokenize`（随机编辑 200 次后，所有行 token 等于从头全量分词）、`TestHighlightStopsEarly`（改一行内的字符只重新分词该行）、`TestHighlightBudget`（预算 2 ms 内返回且返回值正确报告未就绪）、`TestHighlightEditFarAway`（100 万行末尾编辑后前 100 行 `Tokens` 仍最终、未被作废）、`TestDegradedLineRetried`（病态语法行 Degraded 不被当作状态未变，后续 `Highlight` 会重试）、`TestBackgroundCatchUp`（后台在快照更新后补齐，`ready` 被调用）。
- [ ] **Step 2: 写基准** `BenchmarkHighlightTS20k`（2 万行 TS 全量 ≤ 2 s，规格 6.6）、`BenchmarkEditInvalidate1M`。
- [ ] **Step 3: 运行** `go test -race ./ui/editor/highlight/` —— FAIL。
- [ ] **Step 4: 实现**。
- [ ] **Step 5: 运行** —— PASS；记录基准。

### Task 2: 编辑器状态与视口模型（不依赖 ui）

**Files:**
- Create: `ui/editor/model.go`（`State`）、`ui/editor/viewport.go`、`ui/editor/selection.go`
- Test: `ui/editor/model_test.go`

**Interfaces:**
- Consumes: `buffer.Buffer`、`langs.ForFilename`、`monarch.Registry`、`theme.Theme`、Task 1 `highlight.Highlighter`。
- Produces:
  - `type State struct{ Buffer *buffer.Buffer; Language string; Theme *theme.Theme; ... }`；`func NewState(text, filename string, th *theme.Theme) *State`（按文件名选语言，未知语言为纯文本、无高亮器）
  - 视口：`type Viewport struct{ TopLine int; ScrollX float32; Height, Width float32; LineHeight float32 }`；`func (s *State) VisibleLines() (first, last int)`；`func (s *State) ScrollBy(dy, dx float32)`（像素，按行高换算，夹在文档范围内）；`func (s *State) RevealCaret()`。
  - 选区（本里程碑单光标单选区）：`type Selection = buffer.Selection`；`func (s *State) MoveCaret(to int, extend bool)`；按字（grapheme）/按词/按行移动的辅助：`func (s *State) Move(dir Direction, unit Unit, extend bool)`，`Unit` = `Char | Word | Line | LineEdge | Page | Doc`，词边界用 `langs.Conf.WordPattern`（无则按 Unicode 字母数字）。
  - 编辑：`func (s *State) Insert(text string)`、`func (s *State) Delete(dir Direction, unit Unit)`、`func (s *State) Undo() / Redo()`；每次编辑同步通知 Highlighter.Edit。
- [ ] **Step 1: 写失败测试** `TestNewStatePicksLanguage`、`TestMoveByGraphemeAndWord`（含 emoji 族、组合字符、CJK）、`TestInsertDeleteUndo`、`TestScrollClamp`、`TestRevealCaret`、`TestEmptyDocumentState`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**（grapheme 边界：标准库无，自行实现扩展字素簇的常用子集：组合标记、ZWJ 序列、区域指示符对、变体选择符；文档写明子集范围）。
- [ ] **Step 4: 运行** —— PASS。

### Task 3: ui/text 一行多样式整形（等待 min-43 完成）

> **前置：** 控制者确认会话 min-43 已完成 `ui/text` 的 bidi 与 `Lang`，并提供了"字形 → 源 rune 索引"映射与 `SelectionRects`；在其接口之上扩展，只新增 API。

**Files:**
- Create: `ui/text/runs.go`
- Test: `ui/text/runs_test.go`

**Interfaces:**
- Produces: `type Run struct{ Start, End int /* rune 区间 */; Style Style }`；`func ShapeRuns(s string, runs []Run, opt Options) *Layout`——按 bidi run 与样式 run 的交集分段整形；`Layout` 新增 `GlyphRun []int32`（每个字形所属的 run 下标，与 `Glyphs` 等长；`Shape` 产生的 Layout 中全部为 0）。行高取各 run 的最大值，基线对齐。
- [ ] **Step 1: 写失败测试** `TestShapeRunsMatchesShape`（单 run 与 `Shape` 结果完全一致）、`TestShapeRunsMixedWeights`（粗体与常规混排，前进宽度等于分别 `Width` 之和）、`TestShapeRunsGlyphRun`、`TestShapeRunsBidi`（阿拉伯文与英文混排的 run 映射正确）、`TestRichTextUnchanged`（`go test ./ui/ -run 'RichText|Markdown'` 仍通过）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `go test ./ui/text/ ./ui/` —— PASS。

### Task 4: ui/text 按行缓存与等宽快速路径（等待 min-43 完成）

**Files:**
- Create: `ui/text/linecache.go`
- Test: `ui/text/linecache_test.go`、`ui/text/linecache_bench_test.go`

**Interfaces:**
- Consumes: Task 3 `ShapeRuns`、`Run`。
- Produces: `type LineCache struct{...}`；`func NewLineCache(capacity int) *LineCache`；`func (c *LineCache) Shape(line string, runs []Run, opt LineOptions) *Layout`（键 = 行内容哈希 + runs + 样式 + Lang + Direction + TabSize + Ligatures；LRU）；`type LineOptions struct{ TabSize int; Ligatures bool; Lang <与 min-43 的 Style.Lang 同类型>; Direction <min-43 定义的方向类型> }`（类型以 min-43 完成后的 ui/text 为准，不另定义）。等宽快速路径：当 runs 全为 `Mono` 且不开连字时，按字体的等宽前进宽度直接排布（Tab 展开到 TabSize 列；CJK 等全角字符占 2 列宽度的实际字形宽；零宽字符 0；组合字符叠在前一字符上；emoji 按字形实际宽度），不走通用换行算法；结果必须与 `ShapeRuns` 的光标位置一致。
- [ ] **Step 1: 写失败测试** `TestLineCacheHit`（同键第二次不重新整形）、`TestMonoFastPathMatchesShapeRuns`（ASCII、Tab、CJK、零宽、组合、emoji 各一行：Caret 位置与 `ShapeRuns` 相同）、`TestLigaturesDisableFastPath`、`TestCacheKeyIncludesLangDirection`。
- [ ] **Step 2: 写基准** `BenchmarkLineCacheMono200`（200 列等宽行，命中与未命中）。
- [ ] **Step 3: 运行** —— FAIL。
- [ ] **Step 4: 实现**。
- [ ] **Step 5: 运行** —— PASS；记录基准。

### Task 5: View 组件——显示与滚动

**Files:**
- Create: `ui/editor/view.go`、`ui/editor/paint.go`、`ui/editor/gutter.go`
- Test: `ui/editor/view_test.go`（无头 `ui.Driver` 出 PNG + 断言）

**Interfaces:**
- Consumes: Task 1、2、3、4；`theme.Theme.Style(tokenType)`、`theme.Theme.Color(key)`（缺省颜色由本任务补齐：`editor.background`、`editor.foreground`、`editorLineNumber.foreground`、`editor.lineHighlightBackground`、`editor.selectionBackground`、`editorCursor.foreground`，取 Monaco 0.56 vs/vs-dark 的默认值，写成常量表并注明来源）。
- Produces: `func View(c *ui.Context, st *State) *ViewW`；修饰：`LineNumbers(on bool)`、`FontSize(px float32)`、`TabSize(n int)`、`ReadOnly(on bool)`；查询：`Changed() bool`、`Focused() bool`。只排版与绘制可见行（加上下各 2 行缓冲）；每行用 `LineCache.Shape` + token→样式生成 runs；绘制顺序：背景、当前行高亮、选区矩形、文字（按 run 着色）、光标、行号栏。滚轮与触控板滚动更新 `Viewport`；局部重绘提示：若控制者在派发时说明 GPU 里程碑 2 的 `paint.List.Damage` / `paint.List.Scroll` 已可用，则滚动时发 `Scroll`、编辑时对变化行发 `Damage`；否则不发提示（功能不受影响），并在报告里注明待接入。
- [ ] **Step 1: 写失败测试** `TestViewRendersVisibleOnly`（10 万行文档只整形可见行：计数的 LineCache 调用 ≤ 可见行 + 4）、`TestViewSyntaxColors`（Go 源码关键字像素颜色等于主题 `keyword` 前景色）、`TestViewScrollWheel`、`TestEmptyAndPlainDocuments`、`TestVeryLongLineView`（2 万字符压缩 JS：横向滚动、可见区外字形不整形）、黄金图 `TestViewGolden`（Go 引擎、vs 与 vs-dark 两张）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `go test ./ui/editor/...` —— PASS。

### Task 6: View 组件——光标、选区、输入与 IME

**Files:**
- Create: `ui/editor/input.go`、`ui/editor/ime.go`
- Test: `ui/editor/input_test.go`

**Interfaces:**
- Consumes: Task 2 编辑与移动 API、Task 5 View；`ui.Context` 的键盘/指针事件与 `Window.SetTextInput(enabled, caret)`（读 `ui/input`、`ui/window.go` 了解事件形状，不修改它们）。
- Produces: 键位（本里程碑固定，里程碑 2 换成快捷键体系）：方向键、Shift+方向、Alt/Option+左右按词、Cmd/Ctrl+左右到行首尾、Home/End、PageUp/Down、Cmd/Ctrl+A、Backspace/Delete（含按词）、Enter（保持上一行缩进）、Tab（插入 TabSize 空格或制表符按 `insertSpaces`）、Cmd/Ctrl+Z/Shift+Z、Cmd/Ctrl+C/X/V（经 `ui` 的剪贴板接口）；鼠标单击定位、拖动选择、双击选词、三击选行；IME：组字期间显示带下划线的组字文本、不写入 Buffer、提交时一次 `Insert`（一个撤销步），候选窗位置为光标矩形；失焦取消组字。
- [ ] **Step 1: 写失败测试** `TestTypingAndUndo`、`TestKeyboardNavigation`（表驱动）、`TestMouseSelection`（单击/拖动/双击/三击）、`TestCaretAndHitTestMixedScripts`（CJK、Tab、emoji、组合字符：`IndexAt` 与光标矩形一致）、`TestIMECompositionLifecycle`（组字→滚动→提交；组字→失焦；撤销只撤整段提交）、`TestReadOnlyRejectsEdits`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 7: 演示程序与规格 6.6 性能实测

**Files:**
- Create: `examples/editor-demo/main.go`（`uihost.Open` 打开一个窗口，`-file path`、`-renderer cpu|gpu`、`-theme vs|vs-dark`；无文件时生成指定行数的示例 TS 文件 `-gen-lines N`）
- Create: `ui/editor/perf_test.go`（无头 + Metal 离屏，darwin）、`docs/specs/code-editor-m1-report.md`

- [ ] **Step 1: 写基准**（与规格 6.6 一一对应）：`BenchmarkOpen100MB`（加载到首帧可编辑；首屏高亮就绪时间单独计）、`BenchmarkTypeAt1MLines`（按键→帧完成，报告 p50/p95）、`BenchmarkScroll`（连续滚动 200 帧，报告平均 fps 与 p99 帧时间，GPU 渲染）、`BenchmarkHighlightTS20kBackground`（期间输入延迟不受影响）、内存：100 MB 文件的 `runtime.MemStats.HeapInuse` ≤ 300 MB。
- [ ] **Step 2: 运行**（`uptime` 负载 < 6 时；否则记录负载）并写报告：每项对照目标与 Monaco（MinCode `vue-app` 的 `e2e-large-text`、`e2e-perf` 场景已有的 Monaco 数据若可得则引用，否则写明待补）；未达标项写清瓶颈与里程碑 2 的应对。
- [ ] **Step 3: 实机** `go run ./examples/editor-demo -gen-lines 1000000 -renderer gpu`：截图、输入、滚动、IME（中文输入法）手动检查清单写进报告（能自动化的自动化，其余列为待用户确认）。
- [ ] **Step 4: 运行** `go build ./... && go vet ./...` 与 windows/linux 交叉编译 —— 通过。
