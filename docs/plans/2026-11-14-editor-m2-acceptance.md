# 代码编辑器 里程碑 2 验收欠项 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 补齐 `docs/specs/code-editor-m2-report.md` 列出的里程碑 2 验收欠项：定位并修复相对 M1 超过 10% 的性能退化，补上缺失的显示能力（括号对参考线、颜色占位真实间隙、缩进检测、按区域的可疑字符表、小地图字符形态），消除长单行换行与稀有查询的同步全量工作，测出 Monaco 同场景基线，最后按真实数据重写验收报告。

**Architecture:** 先建立“同机、同负载、交替运行”的成对测量工具（M1 源码树导出到临时目录做 A，当前树做 B；当前树内再按显示能力逐项开关做 A/B），以数据决定修什么；能力补齐各自独立成任务，每个默认开启的能力都用同一工具证明自己不超过 10%；Monaco 基线用本机 Edge 无头模式 + Monaco 0.56 发行包 + Node 自带的 WebSocket 驱动 CDP，不新增依赖。

**Tech Stack:** Go 1.27；`ui/editor` 现有包；Node v26（内置 `WebSocket`、`http`）；本机 `/Applications/Microsoft Edge.app`；Monaco 0.56（`/Users/admin/min/mincode/vue-app/node_modules/monaco-editor`，只读）。

**Spec:** `docs/specs/code-editor.md`（第 4 节清单 2；6.6 性能目标；6.7 里程碑 2；7 节小地图等视觉以 Monaco 默认外观为参照）；里程碑 2 计划 `docs/plans/2026-11-13-editor-core-m2.md` 的 Global Constraints；现状 `docs/specs/code-editor-m2-report.md`；内存口径决定见 `docs/specs/code-editor-m1-report.md` 第 35 行。

## Global Constraints

- 模块 `github.com/tangoui/tango`，Go 1.27；**不新增 Go 依赖，不新增 npm 依赖**，不下载浏览器。
- 对齐对象是 Monaco 0.56：选项名、默认值、颜色键、算法以 `/Users/admin/min/mincode/vue-app/node_modules/monaco-editor/esm/vs/**` 为准（只读，可导出数据）。不读 `~/go/pkg/mod/github.com/egoist/mygo`。
- 性能门槛（规格 6.6，Apple Silicon、1920×1080 @2x，GPU 渲染）：100 万行输入 p95 ≤ 16 ms；连续滚动平均 ≥ 60 fps、p99 ≤ 33 ms；打开 100 MB 到可编辑 ≤ 1 s（首屏高亮 ≤ 1.5 s）；TS 2 万行后台全量高亮 ≤ 2 s 且期间输入不受影响；**新增的每项显示能力在 100 万行文件上不得让这三项（输入 p95、滚动 fps 与 p99、打开）退化超过 10%**。
- “退化”的判定一律用本计划 Task 1 的成对测量（同进程序列、ABAB 交替、≥ 5 轮、取各轮中位数之比），不再拿历史报告里的数字直接比。若 Monaco 同场景更好，以 Monaco 为门槛（规格 6.6）。
- **内存口径已由用户在 2026-11-13 决定：文件相关内存 = Go 堆 `HeapInuse` 增量，≤ 文件大小 3 倍；进程 footprint 与 GPU 内存归规格 6.8 的窗口级预算。** M2 报告中“尚未选定口径”的表述是错误的，Task 9 更正。三种口径仍全部报告。
- 大文件降级保持现状（Monaco `largeFileOptimizations` 阈值，语法高亮保持开启）。
- **文件归属**：只改 `ui/editor/**`、`examples/editor-demo/**`、新建 `tools/monaco-bench/**`、`docs/specs/code-editor-m2-report.md`。不改 `ui/` 框架、`internal/render/**`（原 ui/gpu、ui/shader，2026-11-14 已挪动；守卫 internal/apitest TestRetiredPaths）、`ui/text/**`、`ui/paint/**`；确需框架改动时停下报告控制者。
- **不执行任何 git 写操作**（含 `git add`、`commit`、`stash`、`reset`、`checkout`、`clean`、`worktree add`）。读取历史树只用 `git archive <rev> | tar -x -C <临时目录>` 或 `git show`。主工作区有其他会话的未提交改动，不要回退或格式化别人的文件。
- 每个任务结束：darwin `go build ./... && go vet ./ui/editor/... ./examples/editor-demo`；`GOOS=windows`、`GOOS=linux` 的 `go vet ./ui/editor/...`；`go test ./ui/editor/... -count=1` 全过；改动的 Go 文件 `gofmt -l` 无输出；Task 2 起凡改动渲染路径的任务另跑 `TestViewDamageVerified` 与严格导航测试。
- 已接受且不得改动的行为：分词器暂停保护（单次正则尝试 100 ms × 2 确认、行耗尽锁存）；minimap grab/release 两个整帧过渡；大文件关闭换行及测试专用 override；UTF-8/UTF-16 阈值差异已在报告注明。
- 报告必须诚实：不能把“测试通过”写成“验收通过”；做不到的项（锁屏下的实窗、IME、Windows 实机）保留为待人工验收。

## Review Focus

1. 超长单行（1 MB，含 CJK 与 Tab）开启换行后在中间连续输入、粘贴 64 KB、删除整段：换行结果必须与整行重算完全相同，且每次按键不随行长线性变慢 —— Task 5 `TestWrapIncrementalMatchesFull`、`BenchmarkTypeLongLineWrap`。
2. 颜色字面量所在行的点击、上下键、选区、换行与查找高亮：加入占位间隙后，光标不能落进占位、点击占位右半部分落在字面量起点、换行宽度把占位算进去 —— Task 4 `TestColorInjectedCaretStops`、`TestColorInjectedWrap`。
3. 打开混合缩进文件（一半 Tab 一半 2 空格，含大量空行与注释）：检测结果与 Monaco `guessIndentation` 一致，且用户手动 `SetTabSize` 之后不被再次检测覆盖 —— Task 6 `TestGuessIndentationMonacoParity`、`TestDetectIndentationNotReapplied`。
4. 稀有查询（20 MB 以下、只出现一次的词）在文档末尾：首帧不卡（≤ 16 ms 内返回可见区结果），后续帧继续扫描直到完整，期间编辑使旧扫描作废 —— Task 7 `TestDocumentMatchesResumable`、`TestDocumentMatchesEditCancels`。
5. 括号对参考线在折叠、换行、跨数千行的大括号对与未闭合括号下：参考线不越过折叠区、不画到不可见行，未闭合括号不画线 —— Task 3 `TestBracketGuidesFoldWrap`、`TestBracketGuidesUnclosed`。

---

### Task 1: 成对性能测量工具与退化定位

**Files:**
- Create: `ui/editor/perf_ab_test.go`、`ui/editor/perfab/run.sh`（驱动脚本）、`.superpowers/sdd/2026-11-14-editor-m2-acceptance/perf-ab.md`（结果，不入库）
- Modify: `ui/editor/perf_test.go`（只加特性开关读取，不改既有基准语义）

**Interfaces:**
- Produces:
  - 环境变量 `TANGO_EDITOR_PERF_OFF`：逗号分隔的特性名，基准建立 `State` 后据此关闭对应选项。特性名固定为：`minimap`、`sticky`、`overview`、`folding`、`brackets`（`BracketPairColorization`）、`guides`（`Guides.Indentation` 与 `HighlightActiveIndentation`）、`matchbrackets`、`occurrences`、`selectionhl`、`unicode`、`links`、`colors`、`whitespace`、`linehl`、`all`（以上全部）。未知名字让基准 `b.Fatalf`。
  - `func perfApplyOff(o *Options, spec string) error`（`perf_ab_test.go`），供各基准调用。
  - `run.sh A_DIR B_DIR BENCH_REGEX ROUNDS`：在两棵树里分别 `go test -c` 出测试二进制，按 A B A B… 交替运行 ROUNDS 轮，每轮记录 `uptime` 负载，输出每轮 `p50/p95/p99/fps/open-ms` 指标与 B/A 中位数比。负载 ≥ 6 时该轮作废重跑。
- Consumes: 现有基准 `BenchmarkTypeAt1MLines`、`BenchmarkTypeAt1MLinesPartial`、`BenchmarkScroll`、`BenchmarkOpen100MB` 的指标输出格式。

- [ ] **Step 1: 写失败测试** `TestPerfApplyOff`：对 `DefaultOptions()` 依次应用每个特性名，断言对应字段变成关闭值，其余字段不变；`"all"` 关闭全部；`"nope"` 返回错误。
- [ ] **Step 2: 运行** `go test ./ui/editor -run TestPerfApplyOff` —— FAIL（未定义）。
- [ ] **Step 3: 实现** `perfApplyOff` 与各基准对它的调用；写 `run.sh`。M1 树用 `git archive 0ef80e4 | tar -x -C /tmp/tango-m1` 导出（`0ef80e4` 是 M1 报告定稿提交），M1 树里没有该环境变量，只做“M1 对 当前全开”与“M1 对 当前 all 关闭”两组比较。
- [ ] **Step 4: 运行** `TestPerfApplyOff` —— PASS。
- [ ] **Step 5: 测量**（每组 ≥ 5 轮，负载 < 6）：
  1. M1 对 当前默认：输入 p95（中段、末行、后台高亮中）、滚动 2x fps 与 p99、打开 100 MB 均值；
  2. M1 对 当前 `all`：区分“显示能力的成本”与“基础层改动的成本”；
  3. 当前默认 对 当前 `OFF=<每个特性>`：逐项归因，每项给出三个指标的比值。
  结果写入 `perf-ab.md`：表格列出每个超过 1.10 的比值、其所属特性或“基础层”，并对前三名各附一次 `-cpuprofile` 的前 15 个热点函数。
- [ ] **Step 6: 交付** 报告退化清单（按超出幅度排序），作为 Task 2 的输入。本任务不改产品代码。

### Task 2: 修复超过 10% 的退化

**Files:**
- Modify: 由 Task 1 的 `perf-ab.md` 决定，限 `ui/editor/**`
- Test: 相关包的既有测试 + 每处修复一个回归基准或计数测试

**Interfaces:**
- Consumes: Task 1 的 `TANGO_EDITOR_PERF_OFF`、`run.sh`、`perf-ab.md` 的退化清单。
- Produces: 不新增公开 API；行为不变。

- [ ] **Step 1: 对清单里每一项先写能稳定复现的计数型测试**（例如“每帧调用 X 次数”“每次按键分配字节数”`testing.AllocsPerRun`），不能只依赖墙钟；无法计数的项用 `run.sh` 成对比值作为证据，并在报告里写明。
- [ ] **Step 2: 运行** —— 计数测试按当前代码 FAIL。
- [ ] **Step 3: 实现修复**。优先顺序：帧内重复工作（同一帧重复取行文本、重复分词、重复计算装饰）→ 每帧分配 → 不必要的整帧重绘。不得通过关闭默认特性、缩小视口、跳过校验来达标。
- [ ] **Step 4: 重跑 Task 1 Step 5 的第 1、3 组** —— 每个默认开启的特性比值 ≤ 1.10；M1 对 当前默认 的三项比值 ≤ 1.10。若某项在穷尽修复后仍 > 1.10，写明剖析证据、剩余热点与原因，标为未通过，不得放宽门槛。
- [ ] **Step 5: 全量检查**（Global Constraints 的命令）与 `TestViewDamageVerified` —— PASS。

### Task 3: 括号对参考线

**Files:**
- Create: `ui/editor/bracketguides.go`
- Modify: `ui/editor/options.go`（`GuidesOptions`）、`ui/editor/guides.go`、`ui/editor/paint.go`、`ui/editor/damage.go`
- Test: `ui/editor/bracketguides_test.go`、`ui/editor/view_test.go`（金样图）

**Interfaces:**
- Produces（Monaco 0.56 `GuideOptions`）：
  - `type BracketGuideMode string`，常量 `BracketGuidesOff = "false"`、`BracketGuidesOn = "true"`、`BracketGuidesActive = "active"`。
  - `GuidesOptions` 中 `BracketPairs bool` 改为 `BracketPairs BracketGuideMode`（默认 `BracketGuidesOff`），新增 `BracketPairsHorizontal BracketGuideMode`（默认 `BracketGuidesActive`）、`HighlightActiveBracketPair bool`（默认 true）。空字符串视为 `BracketGuidesOff`。所有调用处随之修改。
  - 颜色：`editorBracketPairGuide.background1`…`6` 与 `editorBracketPairGuide.activeBackground1`…`6`（主题生成表中已有），按嵌套层级循环取色，层级与 `BracketPairColorization` 的层级一致。
  - 算法参照 Monaco `guidesTextModelPart.getLinesBracketGuides`：只画跨行括号对；竖线在 `min(开括号所在列, 对内最小缩进)` 的可视列；`BracketGuidesActive` 只画包含主光标的那一对；水平线按 `BracketPairsHorizontal` 在开、闭括号行画到括号列。
- Consumes: Model 的括号树（`(*State).bracketTreeFor`、`enclosingBrackets`），其截止时间与降级规则不变；大文件下括号着色关闭时参考线同样不画。

- [ ] **Step 1: 写失败测试** `TestBracketGuidesBasic`（三层嵌套函数：竖线条数与列、颜色层级正确）、`TestBracketGuidesActive`（光标在内层时只画内层一对，且用 activeBackground）、`TestBracketGuidesHorizontal`（`"active"`/`"true"`/`"false"` 三种水平线）、`TestBracketGuidesFoldWrap`（折叠区内不画、换行行的续行竖线连续）、`TestBracketGuidesUnclosed`（未闭合括号不画）、`TestBracketGuidesOffByDefault`（默认不画、与改动前的绘制命令一致）；`TestViewDamageVerified` 加“光标移入另一对括号”的用例；金样图加一张开启参考线的。
- [ ] **Step 2: 运行** `go test ./ui/editor -run 'BracketGuides|ViewDamageVerified'` —— FAIL。
- [ ] **Step 3: 实现**。参考线计算只针对可见行，复用本帧的括号树结果；光标变化导致的活动对变化必须进入 damage 指纹。
- [ ] **Step 4: 运行** —— PASS；开启 `BracketPairs=true` 的 100 万行输入与滚动成对测量（对 关闭），比值记录在报告（非默认能力，不设 1.10 门槛，但超过时写明）。

### Task 4: 颜色装饰的真实占位间隙

**Files:**
- Create: `ui/editor/injected.go`
- Modify: `ui/editor/colors.go`、`ui/editor/paint.go`、`ui/editor/viewlines.go`、`ui/editor/wrap.go`、`ui/editor/navigation_coords.go`、`ui/editor/input.go`
- Test: `ui/editor/injected_test.go`、`ui/editor/colors_test.go`

**Interfaces:**
- Produces:
  - 行内注入：`type injection struct{ col int /* 字节列 */; w float32 /* 逻辑像素 */ }`；`func (w *ViewW) lineInjections(line int, text string, dst []injection) []injection` 汇总本行注入（本任务只有颜色占位一种来源）。
  - 颜色占位宽度：`colorSwatchW(fontSize) = round(1.2 × fontSize)`（Monaco CSS：0.8em 宽 + 左右各 0.2em 外边距）；方块 0.8em × 0.8em，0.1em 边框，暗色主题边框 `#eee`、亮色 `#000`；位于颜色字面量之前（Monaco `before` 注入）。
  - 光标停靠：光标不能停在占位内部；点击占位左半落在字面量前一列，右半落在字面量起点；左右键越过占位不额外占一步。
  - 换行：`computeBreaks` 把注入宽度计入行宽。
  - 删除原“叠画在前列”的实现。
- Consumes: 现有 `computeColors`、`swatchPress`、颜色选择器（只读帧保护不变）。

- [ ] **Step 1: 写失败测试** `TestColorInjectedLayout`（`a #ff0000 b`：`#` 的 x 比无颜色时右移 `colorSwatchW`，其后字符同步右移）、`TestColorInjectedCaretStops`（Home/End/左右键序列与点击占位左右半的落点）、`TestColorInjectedWrap`（窄视口下换行位置把占位宽度计入，与 `computeBreaks` 整行重算一致）、`TestColorInjectedSelectionRect`（选区矩形覆盖字面量但不覆盖占位）、`TestColorInjectedLargeFile`（大文件下不注入、布局与无颜色时相同）；更新 `TestColorSwatchPointer` 与金样图。
- [ ] **Step 2: 运行** `go test ./ui/editor -run 'ColorInjected|ColorSwatch'` —— FAIL。
- [ ] **Step 3: 实现**。注入只影响含颜色字面量的行，其他行布局路径不得多做工作（以 Task 1 的 `colors` 开关成对测量证明）。
- [ ] **Step 4: 运行** —— PASS；`ColorDecorators` 默认开启，`run.sh` 中 `OFF=colors` 对 默认 的三项比值 ≤ 1.10。

### Task 5: 长单行换行增量计算

**Files:**
- Modify: `ui/editor/wrap.go`、`ui/editor/viewlines.go`
- Test: `ui/editor/wrap_incremental_test.go`、`ui/editor/perf_test.go`（新增基准）

**Interfaces:**
- Produces:
  - `func rewrapEdit(old lineBreaks, oldText, newText string, editAt, oldLen, newLen int, cfg wrapConfig) (lb lineBreaks, scanned int)`：保留编辑点之前、离编辑点最近的已确定断点之前的断点；从那里重新扫描；当某个新断点与旧断点（按 `newLen-oldLen` 平移后）重合且其后字符的换行类状态相同即停止，余下断点平移复用。`scanned` 为本次扫描的 rune 数（供测试）。
  - `viewLines.syncEdits` 对单行内编辑调用 `rewrapEdit`，跨行编辑仍整行重算受影响的行。
- Consumes: `computeBreaks`（作为正确性参照，不改语义）。

- [ ] **Step 1: 写失败测试** `TestWrapIncrementalMatchesFull`（固定种子随机生成 1 MB 行，含 CJK、Tab、emoji、组合字符、长无空格段；随机 500 次插入/删除/替换，每次断言 `rewrapEdit` 结果与 `computeBreaks(newText)` 完全相同）、`TestWrapIncrementalBounded`（1 MB 行中间插入 1 个字符：`scanned ≤ 4096`；插入 64 KB：`scanned ≤ 64*1024 + 4096`）；`BenchmarkTypeLongLineWrap`（1 MB 单行、换行开启、中段输入 300 次，输出 p50/p95）。
- [ ] **Step 2: 运行** `go test ./ui/editor -run WrapIncremental` —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；`BenchmarkTypeLongLineWrap` p95 ≤ 16 ms（M2 报告记录的约 49 ms/键作对照，写入报告）。

### Task 6: 缩进检测（detectIndentation）

**Files:**
- Create: `ui/editor/indentguess.go`
- Modify: `ui/editor/model.go`、`ui/editor/pool.go`、`ui/editor/corecommands.go`
- Test: `ui/editor/indentguess_test.go`、`ui/editor/testdata/indentguess/*.json`（由 Monaco 导出的黄金数据）

**Interfaces:**
- Produces:
  - `func GuessIndentation(snap buffer.Snapshot, defaultTabSize int, defaultInsertSpaces bool) (tabSize int, insertSpaces bool)`：逐行移植 Monaco 0.56 `indentationGuesser.js` 的 `guessIndentation`（扫描前 10000 行，同样的空格差值统计与候选 `[2,4,6,8,3,5,7]`）。
  - `type ModelOptions struct{ TabSize int; InsertSpaces bool; DetectIndentation bool }`；`func DefaultModelOptions() ModelOptions` 返回 `{4, true, true}`（Monaco `tabSize: 4`、`insertSpaces: true`、`detectIndentation: true`）。
  - `func NewModelWithOptions(uri, text string, reg *monarch.Registry, o ModelOptions) *Model`；`NewModel` 等价于用 `DefaultModelOptions()`；`ModelPool` 新增字段 `Options ModelOptions`（零值时用默认），`Open` 新建模型时使用。
  - `func (m *Model) DetectIndentation()`：立即按当前文本重新检测并设置；命令 `editor.action.detectIndentation` 调用它。
  - 只在创建模型与执行该命令时检测；之后 `SetTabSize`/`SetInsertSpaces` 的手动设置不被覆盖。
- Consumes: 现有 `Model.SetTabSize`、`SetInsertSpaces`。

- [ ] **Step 1: 导出黄金数据**：在 `ui/editor/langs/gen` 已有的 Node 环境里写一次性脚本（不入库也可，但黄金 JSON 入 `testdata`），用 Monaco 0.56 的 `guessIndentation` 对 30 个样本（Tab、2/4/8 空格、混合、仅注释、空文件、超过 10000 行、CRLF）求结果。
- [ ] **Step 2: 写失败测试** `TestGuessIndentationMonacoParity`（逐样本与黄金一致）、`TestNewModelDetectsIndentation`、`TestDetectIndentationOptOut`（`DetectIndentation=false` 时用给定默认值）、`TestDetectIndentationNotReapplied`（手动设置后编辑整份文本不触发重检测）、`TestDetectIndentationCommand`。
- [ ] **Step 3: 运行** `go test ./ui/editor -run 'GuessIndentation|DetectIndentation'` —— FAIL。
- [ ] **Step 4: 实现**；更新 demo 显示检测结果（状态栏文字即可）。
- [ ] **Step 5: 运行** —— PASS；打开 100 MB 的成对测量比值 ≤ 1.10（检测只扫前 10000 行）。

### Task 7: 稀有查询的可续扫描与查询缓存

**Files:**
- Modify: `ui/editor/occurrences.go`
- Test: `ui/editor/occurrences_test.go`

**Interfaces:**
- Produces:
  - `documentMatches` 改为可续扫描：保存 `next int`（下一扫描行）；`func (c *documentMatches) step(buf *buffer.Buffer, snap buffer.Snapshot, q *selQuery, deadline time.Time) (done bool)`；每帧预算 `docScanBudget = 2 * time.Millisecond`，未完成时请求下一帧（沿用现有 wake 机制），并保证可见行的匹配在本帧就有（可见行单独匹配，与整文档结果合并去重）。
  - 999 个上限（`maxFindMatches`）与“全文档第一个 999 个”的语义不变：扫描未完成时概览标尺只显示已扫描部分。
  - 查询缓存改为最多 4 条的 LRU，键为 `(buf, version, text, whole, fold)`，解决 A→B→A 重扫。
  - 版本变化使未完成的扫描作废并从头开始。
- Consumes: 现有 `selQuery.matchesLimit`、帧唤醒接口。

- [ ] **Step 1: 写失败测试** `TestDocumentMatchesResumable`（20 MB 以下、只在最后一行出现的词：首帧 `step` 在预算内返回且可见行结果正确；循环至 done 后结果与一次性扫描相同）、`TestDocumentMatchesEditCancels`（扫描进行中编辑：旧结果不出现在新版本）、`TestDocumentMatchesLRU`（A→B→A 第三次 `scans` 不增加）、`TestOccurrencesFirstFrameBudget`（模拟时钟：单帧扫描工作不超过预算对应的行数）。
- [ ] **Step 2: 运行** `go test ./ui/editor -run 'DocumentMatches|OccurrencesFirstFrame'` —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；`TestOccurrencesDebounced`、`TestSelectionHighlight`、`TestDocumentHighlightQuota`、`TestViewDamageVerified` 仍 PASS。

### Task 8: 可疑字符的区域表与小地图字符形态

**Files:**
- Modify: `ui/editor/unicode.go`、`ui/editor/unicode_gen.go`（生成）、生成脚本（`ui/editor/langs/gen` 下现有导出 `ambiguousCharacters` 的脚本）、`ui/editor/options.go`、`ui/editor/minimap.go`
- Create: `ui/editor/minimap_chars.go`、`ui/editor/minimap_chars_gen.go`（生成）
- Test: `ui/editor/unicode_test.go`、`ui/editor/minimap_test.go`

**Interfaces:**
- Produces（可疑字符，Monaco 0.56 `unicodeHighlight.allowedLocales` 默认 `{_os: true, _vscode: true}`）：
  - 生成表包含 Monaco `ambiguousCharacterData` 的全部区域键（`_common`、`_default` 及各区域），不再只合并默认区域。
  - `UnicodeHighlightOptions` 新增 `AllowedLocales map[string]bool`；nil 等于默认 `{"_os": true, "_vscode": true}`。
  - `func SetSystemLocales(os, ui []string)`（包级，宿主在启动时调用）；未调用时 `_os` 取环境变量 `LC_ALL`、`LC_MESSAGES`、`LANG` 的语言部分，`_vscode` 取 `"en"`。区域选择规则与 Monaco `AmbiguousCharacters.getInstance(locales)` 一致：`_common` 加上各所选区域的交集规则照 Monaco 源码。
- Produces（小地图，Monaco 0.56 `minimap.renderCharacters: true`）：
  - 从 Monaco `minimapPreBaked.js` 导出 1x 与 2x 预烘焙字形 alpha 数据到 `minimap_chars_gen.go`（MIT 许可，文件头保留来源与许可说明）。
  - `RenderCharacters=true` 时每行渲染为 alpha 位图并按 token 前景色着色，按行缓存（只重画变化行，沿用 `TestMinimapIncremental` 的计数语义）；整张小地图以图片绘制。`RenderCharacters=false` 保持现有色块。
- Consumes: 现有 `minimap.go` 行缓存与 damage 指纹。

- [ ] **Step 1: 写失败测试** `TestUnicodeLocaleTables`（`AllowedLocales={"ru":true}` 时西里尔 `а` 不标记；默认 `_os="en"` 时标记）、`TestUnicodeSystemLocales`（`SetSystemLocales` 生效；环境变量回退）、`TestMinimapCharacters`（`RenderCharacters=true`：行位图非全满块，字符 `a` 与 `.` 的 alpha 覆盖率不同且与预烘焙数据一致；颜色等于 token 前景色）、`TestMinimapCharactersIncremental`（编辑一行只重画一行位图）、`TestMinimapBlocksWhenOff`（`false` 与现状绘制命令一致）。
- [ ] **Step 2: 运行** `go test ./ui/editor -run 'UnicodeLocale|UnicodeSystem|MinimapChar|MinimapBlocks'` —— FAIL。
- [ ] **Step 3: 生成数据并实现**。
- [ ] **Step 4: 运行** —— PASS；`minimap` 默认开启：`OFF=minimap` 对 默认 的三项比值 ≤ 1.10；`TestViewDamageVerified` PASS。

### Task 9: Monaco 同场景基线

**Files:**
- Create: `tools/monaco-bench/README.md`、`tools/monaco-bench/server.mjs`、`tools/monaco-bench/page.html`、`tools/monaco-bench/bench.mjs`
- Output: `.superpowers/sdd/2026-11-14-editor-m2-acceptance/monaco-baseline.md`

**Interfaces:**
- Produces:
  - `node tools/monaco-bench/bench.mjs --monaco <monaco-editor 目录> --edge "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge" --scenario <open|type|scroll|ts20k|memory> --runs N`：Node 内置 `http` 起本地静态服务（只监听 127.0.0.1，提供 Monaco `min/` 与 `page.html`），以 `--headless=new --remote-debugging-port=0 --user-data-dir=<临时目录> --window-size=1920,1080 --force-device-scale-factor=2` 启动 Edge，用 Node 内置 `WebSocket` 走 CDP。不安装任何包，不访问网络。
  - 场景与 Tango 基准一一对应、使用同一份生成文件（`perf_test.go` 的生成器输出路径）：打开 100 MB/100 万行到首屏可编辑与首屏 token；第 50 万行输入 300 次（CDP `Input.dispatchKeyEvent` → 下一次 `requestAnimationFrame` 后记时）p50/p95；每帧滚 2 行 300 帧的 fps 与 p99；TS 2 万行全量 tokenization 时长；`Performance.getMetrics` 的 JSHeapUsedSize 增量。
  - Monaco 使用默认配置（大文件会降级、语法高亮关闭），结果表单独标注“Monaco 降级状态”。
- Consumes: Tango 的生成文件与场景定义（`perf_test.go`）。

- [ ] **Step 1: 写 `bench.mjs --self-test`**：启动服务与 Edge、打开空 Monaco 页面、读取 `monaco.version` 等于 `0.56.0` 后退出，退出码 0。
- [ ] **Step 2: 运行** `node tools/monaco-bench/bench.mjs --self-test` —— 先 FAIL（文件不存在），实现后 PASS。
- [ ] **Step 3: 实现全部场景**，每场景 5 轮，记录负载。
- [ ] **Step 4: 运行并写 `monaco-baseline.md`**：Monaco 与 Tango 同表；注明无头模式不受 vsync 限制、Monaco 大文件降级与 Tango“高亮仍开启”的差别。若 Monaco 在某项更好，按规格 6.6 把该项门槛改为 Monaco 的数字，并在 Task 10 判定。
- [ ] **Step 5: 清理** Edge 进程与临时目录；确认无残留监听端口。

### Task 10: 最终测量与验收报告

**Files:**
- Modify: `docs/specs/code-editor-m2-report.md`

**Interfaces:**
- Consumes: Task 1–9 的结果文件与测试。

- [ ] **Step 1: 最终成对测量**：`run.sh` 的三组比较全部重跑（≥ 5 轮，负载 < 6），加 `BenchmarkMemory100MB` 两个独立进程、`BenchmarkHighlightTS20kBackground`、`BenchmarkTypeLongLineWrap`。
- [ ] **Step 2: 重写报告**：
  - 第 4 节性能：用成对比值替代“历史数字直接比较”，逐能力列出比值与 ≤ 1.10 结论；Monaco 基线同表。
  - 内存：更正为“2026-11-13 用户已选定 Go 堆增量口径”，给出结论，另两种口径照旧列出并说明归规格 6.8。
  - 第 5 节欠项：删去本计划已完成的项（括号对参考线、颜色占位、detectIndentation、长行换行、稀有查询、locale 表、小地图字符），保留仍未做的（颜色 provider 与语义 documentHighlight 属里程碑 3）。
  - 第 6 节实窗清单：保留为待人工验收，列出新增能力的手工检查步骤（括号参考线、颜色占位点击、缩进检测、小地图字符）。
  - 结论段：只有在全部自动化门槛通过时才写“自动化验收通过，实窗/IME/Windows 待人工”；任何一项未过就写未通过及原因。
- [ ] **Step 3: 全量检查**：Global Constraints 的全部命令；`go test -race ./ui/editor/... -count=1` 一次；`go build -o /dev/null ./examples/editor-demo`。
