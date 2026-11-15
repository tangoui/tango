# 编辑器里程碑 2 验收报告

日期：2026-11-15（验收欠项计划 Task 10 重写；历史性能为 2026-11-14 22:16 – 11-15 01:30，F/P 候选离屏复测为 2026-11-15 18:52 – 18:55）。范围：`docs/specs/code-editor.md` §4 的 **1、2、11、12、13** 及 §6.6；依据里程碑 2 Tasks 1–15、验收欠项计划 Tasks 1–9 和 F/P 渲染候选的独立审查记录。

**结论：自动化验收仍未通过。** 第 4.2 节保留 Task 10 的历史结论，第 4.11 节保留 A–E 后续修复；本轮第 4.12 节新增的是 F-only/F+P 的锁屏离屏 paired 和绝对基准，不能替代真实窗口、present/vsync、IME、Windows 实机或已被审计为未证的 Monaco E2E 对照。锁屏离屏数据证明候选路径在同一 Wait 口径下没有引入长行回归，并改善了局部 copy 的离屏 GPU wait，但不把这些结果写成完整 M2 通过。

当前仍未通过或未判定的项目：

1. **真实窗口/屏幕呈现验收：未完成。** 测量时 `CGSSessionScreenIsLocked=Yes`，因此本轮只能记录离屏诊断；真实 present/vsync 和 Monaco 窗口 trace 仍待解锁后执行。
2. **滚动对 Monaco 的端到端门槛：未判定/暂不改判。** 历史 Monaco headless 数字只计主线程结束，不是 GPU-complete；本轮没有伪造新的 Monaco E2E 证据。Tango 自身绝对门槛的锁屏离屏结果另列第 4.12 节。
3. **相对 M1 与显示能力 ≤10% 的正式统计结论：仍未完成。** 本轮是 F-only 对 F+P 的单变量验证，不是 M1 对最终候选的完整 paired；历史 CI 结论保留。

锁屏状态下的绝对离屏数字（包括百万行输入、滚动、打开、TS20k 和 HeapInuse）只作为诊断/候选验证，不覆盖上述正式验收结论。全量编辑器回归、F-only/F+P 定向测试和 `go build ./...` 均通过；实窗、IME、Windows 实机仍待人工验收。

## 1. 演示与复现

在仓库根目录执行（Go 1.27）：

```sh
go run ./examples/editor-demo -split -wrap -gen-lines 10000
go run ./examples/editor-demo -file ui/editor/view.go -theme hc-black
go run ./examples/editor-demo -gen-lines 1000000 -renderer gpu -auto -quit-after 30s
go run ./examples/editor-demo -theme hc-light -language css
```

- `-split`：左右两份独立 `State`，共享同一个 `Model`，不是复制两份文本。光标、滚动、折叠各自独立；编辑、撤销历史、语言共同维护。
- `-wrap`：两侧均设置 `Options.WordWrap=WrapOn`。**大文件有效选项仍为 WrapOff**，不能用百万行命令宣称已经演示换行；正常规模示例用于组合验收。
- 顶栏可搜索语言选择器修改 `Model.SetLanguage`；两侧同时更新。支持四种基础主题启动参数：`vs`、`vs-dark`、`hc-black`、`hc-light`。
- 正文右键使用编辑器内置菜单；宿主注册 `editor.action.quickCommand` 后右键出现 Command Palette，顶栏按钮也可打开。面板按 `Commands()` 完整列出 Label 与 ID，可搜索并执行；目标是最后聚焦的分屏，执行前恢复该视图焦点。不绕过只读或命令前置条件。
- 这是简单演示面板，不是 Monaco QuickInput 的完整交互复刻：命令不附参数，依赖参数的命令可能拒绝/无动作；没有参数编辑器，也未额外绑定 F1/Cmd+Shift+P。
- 保留已有 `-auto` 的 `c.Now()` 每帧去重：Runner 多次构建不能重复记时或投递事件。该日志是显示节拍的**帧间隔**，不是独立测得的硬件按键到屏幕光子延迟。

## 2. 能力 → 命令/选项 → 自动化测试 → 已知差异

测试名未带包名前缀的位于 `ui/editor`；本表陈述已覆盖的行为，不把“有字段/有接口”当作功能已完成。

### 清单 1：基础编辑

| 能力 | 命令 / 选项 / API | 测试 | 已知差异或边界 |
|---|---|---|---|
| 多光标输入、重叠合并 | `SetSelections`、`type` | `TestMultiCursorTyping`、`TestSelectionsMergeOverlap`、`TestManyCursorsTyping` | 重叠删除取并集，Monaco 会丢弃冲突的后序光标操作 |
| 同词追加、全选同词 | `editor.action.addSelectionToNextFindMatch`、`editor.action.selectHighlights` | `TestAddNextOccurrence`、`TestSelectHighlights`、`find_fold_test.go` | Cmd+D 无动作仍可能返回 executed=true；超大/非法 UTF-8 选区采用逐字大小写折叠，不编译巨型正则 |
| 列选择、鼠标多光标 | `cursorColumnSelect*`、Alt+点击/拖选 | `TestColumnSelectMixedWidths`、`TestColumnSelectMouseFromCaret`、`TestAltClickAddsCursor` | 部分列取整、跨行起止细节见 Task4；换行行尾 Alt+点击 affinity 不完整 |
| 复制/移动/删除/合并行 | `editor.action.copyLinesDownAction`、`moveLinesUpAction`、`deleteLines`、`joinLines` | `TestCopyLines`、`TestMoveLines`、`TestDeleteLines`、`TestJoinLines`、`TestLineOpsMultiCursorOneUndo` | 同行光标去重、相邻块合并与 Monaco 冲突丢弃策略不同 |
| 插行、复制选区、转置、排序、清尾空白 | `editor.action.insertLineBefore/After`、`duplicateSelection`、`transpose`、`sortLinesAscending/Descending`、`trimTrailingWhitespace` | `TestInsertLine`、`TestDuplicateSelection`、`TestTranspose`、`TestSortLines`、`TestTrimTrailingWhitespace` | 排序不是 Intl.Collator；trim 的文本扫描仍为 O(n)，语法分类现有命令级预算；超预算保留未知空白并报告 incomplete，不能外推为任意大文件低延迟 |
| 单词与驼峰移动/删除 | `cursorWord*`、`cursorWordPart*`、`deleteWord*` | `TestMoveByGraphemeAndWord`、`TestWordPartMovement` | 嵌入语言辅助使用文档语言；不是完整语言服务词法语义 |
| 行/块注释 | `editor.action.commentLine`、`editor.action.blockComment`、`Options.Comments` | `comment_test.go`、`TestLineOpsReadOnly` | 共享行只操作一次；多选区冲突策略不同 |
| 缩进、Tab、重缩进、移动行缩进 | `Options.AutoIndent`、共享的 `Model.SetTabSize/SetInsertSpaces`、仅本帧的 `ViewW.TabSize`、`editor.action.reindentlines` | `TestTabAndOutdent`、`TestReindentLines`、`TestAutoIndentCommands`、`TestAutoIndentShiftJSDoc`、`TestIndentationOptionsShared`、`TestModifiersArePerFrame` | Model缩进设置由分屏共享，ViewW.TabSize只覆盖本帧绘制/导航，不改Model。Enter会将保留的既有缩进规范化为Model风格；有了缩进检测（下一行）后典型 Tab 文件不再被改成空格，混合缩进文件仍按检测结果规范化（与 Monaco 一致）。辅助仍有预算/文档语言边界 |
| **缩进检测（验收欠项 Task 6 新增）** | `ModelOptions.DetectIndentation`（`DefaultModelOptions` 开启，`NewModel` 默认检测）、`GuessIndentation`、`Model.DetectIndentation()`、命令 `editor.action.detectIndentation`、`ModelPool.Options`；demo 状态栏 `Spaces: N`/`Tab Size: N` | `TestGuessIndentationMonacoParity`（36 样本×4 组默认值与 Monaco 0.56 `indentationGuesser` 导出结果一致）、`TestGuessIndentationRandom`、`TestDetectIndentationNotReapplied`、`TestDetectIndentationCommand`、`TestGuessIndentationLongLinesBoundedAlloc`、demo `TestIndentStatus` | 只在创建与命令时检测，`SetTabSize`/`SetInsertSpaces` 后不再覆盖；命令的回退默认值是创建时选项（Monaco 每次读当前配置）；无单独 `indentSize`（同 Monaco `'tabSize'`）；`ModelPool.Options` 零值 = 开启检测（文档写明） |
| 自动闭合、包裹、跳过闭合、配对删除 | `AutoClosingBrackets/Quotes/Delete/Overtype`、`AutoSurround` | `TestAutoCloseRules`、`TestAutoClosedTracking`、`TestTypingAssistUndoExact` | IME 提交不执行自动闭合；空 AutoCloseBefore 作为未设置；部分反斜线/列边界保留差异 |
| 剪贴板与多光标粘贴 | `editor.action.clipboardCopyAction/CutAction/PasteAction`、`EmptySelectionClipboard`、`MultiCursorPaste` | `TestCopyCutEmptySelectionLine`、`TestMultiCursorPasteSpread`、`TestCopyPasteMultiCursor` | Windows 不强制 CRLF，使用文档 EOL；spread 后主光标选择策略不同；粘贴不跑输入辅助 |
| 文本拖放、复制拖放 | `Options.DragAndDrop`、内部选区拖放 | `TestDragMoveText`、`TestDragCopyWithAlt`、`TestDragReleasePos`、`TestDragCaptureEndsWithoutRelease` | 以主选区开始拖放；不支持外部应用文件/文本拖入。Task7 的 ReleasePos 框架缺口已由后续任务接入，不列为现存缺陷 |
| 撤销/重做、光标撤销 | `undo`、`redo`、`cursorUndo/Redo` | `TestInsertDeleteUndo`、`TestCursorUndo`、`TestTypingAssistUndoExact` | Enter 单独撤销步；换行边界 affinity 不随 undo 精确恢复 |
| 版本号 | `Snapshot.Version()`、`Buffer.AltVersion()` | `buffer.TestAltVersion`、`TestUndoRedoHighlightIncremental` | Go 名称不同；UTF-8 内部偏移，外部有 UTF-16 换算，不把两者混作列号 |
| IME 组字 | View 输入与 composition 事件 | `TestIMECompositionLifecycle`、`TestIMEComposingSkipsKeymap` | 组字预览只画主光标，提交才编辑所有光标；真实中文输入法 + 多光标本轮待实测；双击修饰键在 IME 后仍有误触发边界 |
| 只读 | `Options.ReadOnly`、`ViewW.ReadOnly`、`Command.Mutates` | `TestReadOnlyRejectsAllMutations`、`TestReadOnlyRejectsEdits` | 只读 Cut 只复制；允许导航/复制，不代表禁用整个控件 |

### 清单 2：显示

| 能力 | 命令 / 选项 / API | 测试 | 已知差异或边界 |
|---|---|---|---|
| 行号（绝对、相对、间隔） | `Options.LineNumbers` | `TestRelativeLineNumbers`、`TestViewGolden` | 字体默认统一 14，Monaco macOS 默认 12；非完全相同的字体度量 |
| 缩进/marker/provider 折叠 | `Folding`、`FoldingStrategy`、`ViewW.FoldingProvider`、`editor.fold*`/`editor.unfold*` | `TestIndentFoldingRanges`、`TestMarkerFolding`、`TestFoldingProvider`、`TestFoldSurvivesRemoteEdit` | 大文件关闭；>20k 行编辑后控件可能有约 200ms 防抖；隐藏选区边界导航有细节差异 |
| 自动换行与折叠组合 | `WordWrap`、`WordWrapColumn`、`WrappingIndent`、`editor.action.toggleWordWrap` | `TestWrapMixedScripts`、`TestWrapMultiCursorAffinity`、`TestWrapViewState`、`TestStickyWrappedFoldedHeaderClips` | 惰性视觉行数是估计；续行 Tab 原点、字体宽度与 Monaco 有差别；undo affinity 尚未恢复。**长单行增量换行（Task 5/5b）**：行内编辑只从受影响的断点续扫并在收敛后复用旧断点（`TestWrapIncrementalMatchesFull` 8 组配置×500 次随机编辑与整行重算逐项相等、`TestWrapIncrementalBounded`、`TestWrapIncrementalEditor`、`TestLongLineKeystrokeWork`）；括号、样式区间、长行判定、整行复制等每键整行开销也已按窗口/检查点处理。仍为 O(行长) 的情形：无断点长串（压缩 JS、data URI）中编辑、键入不配对括号（其后栈确实改变）、不换行的长行括号着色与 `longLineOf` 块表、括号对参考线在长行上 |
| Sticky scroll | `Options.StickyScroll` | `TestStickyScrollHeaders`、`TestStickyMaxLines`、`TestStickyFractionalPushReallyPartial` | 来自 folding ranges，不是完整语义节点 provider；大文件关闭 |
| 小地图 | `Options.Minimap` | `TestMinimapColors`、`TestMinimapIncremental`、`TestMinimapScrolledSampleClick`、`TestMinimapDragScroll`、**`TestMinimapCharacters`（与 Monaco `renderChar` 逐像素 ±1）、`TestMinimapCharactersIncremental`、`TestMinimapCharactersFillShortDoc`、`TestMinimapBlocksWhenOff`、`TestMinimapRowsCoverEveryViewRow`、`TestViewDamageVerified/minimapChars`** | **字符形态已实现（Task 8）**：Monaco `minimapPreBaked` 1x/2x 字形，16 行条带图片增量合成。附带修复 M2 既有缺陷：Fit/Fill 下 float32 采样行号跳行（某些行从不绘制、点击落到下一行）。差异：大文件关闭（比 Monaco 多一项降级）；按字素/显示列排单元（Monaco 按 UTF-16 码元）；采样模式（行高 < 1px）滚动时条带每帧重合成，`force1pxHeight` 处理不同；非整数缩放下条带略软；色块模式在高行 Fill 下不居中（Monaco 居中），为保持 `RenderCharacters=false` 与改动前逐像素一致而保留；非大文件上小地图使滚动平均帧时间约 +15%（第 4.5 节） |
| 空白与控制字符 | `RenderWhitespace`、`RenderControlCharacters` | `TestRenderWhitespaceModes`、`TestControlCharacters` | C0/DEL 可见；双向格式字符未做 Monaco 的 `[U+XXXX]` 宽占位 |
| 括号配对着色与匹配 | `BracketPairColorization`、`MatchBrackets`、括号导航命令 | `TestBracketPairColors`、`TestMatchBrackets`、`TestMatchManyCursorsConverge`、`TestBracketGuidesMergedEdits`、`TestBracketGuidesRandomEdits`、`TestBracketLongIndexMatchesFull`、`TestLongLineBracketColorsDrawn` | 非着色未闭合括号层级有差异；嵌入区用文档配置；只扫描 final 行；大文档旧式匹配预算/防抖与 Monaco 不同。**修复 M2 既有缺陷（Task 3 发现）**：括号树在多光标/撤销等合并编辑区域时行数少算，含换行的插入后 `start/minD` 整体错位直到下次整树重建（randomized 3000 步测试覆盖）。长行（> 20000 字节）用检查点索引，换行长行只给可见行与 sticky 窗口着色 |
| 缩进参考线与激活线 | `Guides.Indentation`、`HighlightActiveIndentation` | `TestIndentGuidesActive`、`TestIndentGuidesWrapped`、`TestActiveGuideFirstCallFast` | 列网格近似；与括号对参考线同列时让位（Monaco `getGuidesByLine`） |
| **括号对参考线（Task 3 完成）** | `Guides.BracketPairs`（`BracketGuideMode` `"false"`/`"true"`/`"active"`，默认关，同 Monaco）、`BracketPairsHorizontal`（默认 active）、`HighlightActiveBracketPair`（默认 true） | `TestBracketGuidesBasic/Active/Horizontal/FoldWrap/Unclosed/OffByDefault/ActiveUnclosed`、`TestBracketGuidesIndexEdits/IndexDeadline/IndexTwoTabSizes/IndexLongLines`、`TestViewDamageVerified/bracketGuides`、金样图 `TestViewGolden/*/bracketguides` | 按 Monaco `getLinesBracketGuides`：竖线只画跨行对，单行对在水平线开启时画底线，活动对可为单行对；未闭合对不画；不越过折叠、只画可见行；列计算用按块的最小缩进索引并受帧预算约束（未决的对不画，不画猜测的列）。差异：层级取着色层级（Monaco 用 nestingLevel，全部对都着色时相同）；无 `independentColorPoolPerBracketType`；长行上仍整行扫描；无括号树（> 5 000 000 字节）时不画 |
| 同词高亮、选区匹配 | `OccurrencesHighlight`、`OccurrencesHighlightDelay`、`SelectionHighlight` | `TestOccurrencesDebounced`、`TestSelectionHighlight`、`TestDocumentHighlightQuota`、`TestSelectionHighlightUTF16Boundary`、**`TestDocumentMatchesResumable`、`TestDocumentMatchesEditCancels`、`TestDocumentMatchesLRU`、`TestOccurrencesFirstFrameBudget`、`TestDocumentMatchesShortLinesBudget`、`TestDocumentMatchesProvisionalQuota`、`TestViewDamageVerified/docScan`** | 默认额外延迟 0 + 内部 50ms；整文档前 999 个匹配。**稀有查询改为可续扫描（Task 7）**：每帧共享 2 ms 预算（每 64 行或 32 KiB 读一次时钟），可见行当帧给出临时结果，编辑使旧扫描作废；查询缓存为 4 条 LRU（A→B→A 续扫）。差异：超出 999 额度的可见匹配在扫描确认前会短暂出现后消失（Monaco 同步扫描无此中间态）；扫描中无变化的帧框架仍整帧重绘（后续项）；概览标尺不画出现位置标记；跨编辑器 blur 清除；没有语义 documentHighlight provider（里程碑 3） |
| Unicode 可疑字符 | `Options.UnicodeHighlight` | `TestUnicodeAmbiguousHighlight`、`TestInvisibleCharacters`、`TestDecorationLimits` | **区域表已实现（Task 8）**：`UnicodeHighlightOptions.AllowedLocales LocaleSet`（零值 = Monaco 默认 `{_os, _vscode}`）、`AllowLocales`、`LocaleSetOf`、`SetSystemLocales`，按 Monaco `AmbiguousCharacters.getInstance` 求交并并入 `_common`，数据与 Monaco 逐字节一致（`TestUnicodeLocaleTables`、`TestUnicodeSystemLocales`、`TestUnicodeLocalesConcurrent`）。差异：类型为可比较的 `LocaleSet` 而非 map；环境变量回退同时给出区域形式与语言部分（比 Monaco 宽）；运行中 `SetSystemLocales` 下一帧生效不主动唤醒；1000 上限按已布局行；没有 Monaco hover/banner；长行跳过 |
| 链接检测 | `Options.Links`、`ViewW.OnOpenLink` | `TestLinkDetect` | 需宿主实际打开 URL（demo 不调用系统浏览器）；下划线/hover 呈现非完全一致；超长行与 >50MiB 文件关闭 |
| 颜色装饰与颜色选择器 | `ColorDecorators` | `TestColorDecoratorEdit`、`TestColorSwatchPointer`、**`TestColorInjectedLayout`、`TestColorInjectedCaretStops`、`TestColorInjectedWrap`、`TestColorInjectedSelectionRect`、`TestColorInjectedSelectionToRightSide`、`TestColorInjectedRightsMoves`、`TestColorInjectedLargeFile`、`TestColorInjectedGolden`** | **真实占位间隙已实现（Task 4）**：字面量前插入 1.2em 间隙，swatch 画在间隙内；光标两侧停靠语义按 Monaco（两半都是字面量起点，只是画在间隙左/右侧，不落进间隙）；选区、装饰、命中测试、换行宽度都计入间隙；> 50 MB 关闭。差异：换行按间隙像素（约 2 列的小数）计入，Monaco simple 策略算 1 列，有颜色的行会略早断行；sticky 标题不显示间隙；有间隙的行上 IME 组字以覆盖方式绘制；上下键目标列按模型列；swatch 垂直居中（Monaco 基线对齐）；连字跨间隙边界时整体画在左段（默认关连字）；没有 `colorDecoratorsLimit`（Monaco 每文档 500）；无颜色字面量的行 `computeBreaks` 约 +2.6% 固定开销（已接受）；没有颜色 provider（里程碑 3） |
| 滚轮缩放 | `MouseWheelZoom`、`editor.action.fontZoomIn/Out/Reset` | `TestWheelZoom` | 全局缩放；统一近似非物理滚轮路径，事件无法分类物理滚轮/触控板 |
| Padding、当前行高亮 | `Padding`、`RenderLineHighlight` | `TestPadding`、`TestViewGolden`、`TestViewDamageVerified` | 默认与大文件有效选项各自生效；不能只看字段推断已画 |
| 概览标尺 | `OverviewRulerLanes/Border`、`State.SetOverviewMarks(owner, marks)` | `TestOverviewMarks`、`TestOverviewDragAndLaneMasks`、`TestOverviewOwnerOrderDamage` | 仅模型行 + lane + color；诊断/查找服务尚未接入，不宣称完整诊断体验；不画同词/选区匹配标记（Monaco 会画） |
| GPU 局部重绘 | View damage/scroll hints、production verify 默认关闭 | `TestViewDamageVerified`、`TestViewDamageVerifiedTask14`、`TestNavigationInteractionExecution` | strict 测试区分真实主帧与 full reference；minimap grab/release 两个转换帧允许 full，拖动/细微 sticky 位移须真实 partial |

### 清单 11、12、13：命令、菜单、模型

| 能力 | 命令 / 选项 / API | 测试 | 已知差异或边界 |
|---|---|---|---|
| 带标签命令注册及执行 | `RegisterCommand`、`Commands`、`LookupCommand`、`State.Execute` | `TestCommandsListed`、`TestExecuteAndContextKeys`、demo `TestDemoCommandOptions` | demo 面板不收集参数；无动作与被拒均可返回 false |
| 快捷键与组合键 | `keymap.New`、`DefaultRules`、`ViewW.Keymap` | `keymap.TestParseKeyMinCodeCompat`、`TestChordPlatformEvents`、`TestChordStateClearedOnBlur`、`TestDefaultKeymapPerView` | 每焦点作用域要独立 Keymap；未实现的命令不绑定；IME 后双修饰键误触发边界未消除 |
| 上下文键与 when | `ViewW.Context`、`when.Parse` | `when.TestWhenParseEval`、`when.TestWhenRegex`、`TestExecuteAndContextKeys` | 接受双引号扩展，512 长度限制，数值解析严格，in 使用 Go 容器；不做 Monaco 的全部表达式规范化 |
| 右键菜单与宿主贡献 | `RegisterMenuItem`、`ViewW.MenuItems`、`ContextMenu` | `TestContextMenuItemsFiltered`、`TestContextMenuOrder`、`TestContextMenuRunsCommand`、`TestContextMenuHostContext`、`TestContextMenuAccessibility` | 按 when 隐藏、precondition/只读禁用；按字节排序而非 localeCompare；无匹配快捷键时不做 Monaco fallback。已接入 `NoAutoSelect`，早期“自动选首项”不再是现存缺口 |
| 共享模型与多视图 | `NewModel`、`NewView`、`Model.Subscribe` | `TestTwoViewsShareModel`、`TestOtherViewMapsAllSelections`、`highlight.TestTwoViewsKeepTheirWindows`、demo `TestDemoSplitSharedModel` | 模型/池不是并发安全 API，UI 线程操作；重入编辑排队，失效排队编辑可能静默丢弃 |
| 模型池生命周期 | `ModelPool.Open/Get/Release`、事件订阅、`State.Close` | `TestPoolOpenShares`、`TestPoolLanguageEvents`、`TestCloseDropsHighlightWindow` | **宿主必须分别 Close 视图、Release 池引用、取消自己启动的后台高亮**；视图 Close 不自动 Release；没有自动生命周期管理 |
| 语言切换 | `Model.SetLanguage`、demo 顶栏 / `-language` | `TestPoolLanguageEvents`、`TestBracketColorsLanguageSwitch`、`TestAllLanguagesInView`、demo `TestDemoSplitSharedModel` | 同一模型所有视图一起切换；未知语言由模型回退纯文本；嵌入区编辑辅助仍按文档语言 |
| demo后台高亮生命周期 | `modelHighlight`、`Model.Subscribe(ModelChange.Language)`、`SetNotify/Close` | demo `TestDemoPlaintextSetupStartsSharedBackground`、`TestDemoHighlightPlaintextLanguageRoundtrip`、`TestDemoHighlightReadyBeforeWindowAttachment`、`TestDemoHighlightCloseCancelsAndUnsubscribes` | 初始纯文本后首次出现lexer才启动一次；split共用同一后台，语言往返不重复Start；窗口invalidator可晚附加并补发ready。退出取消context、退订并安全关种子channel；正在执行的一批可按上游高亮器取消协议结束，不宣称同步抢占任意正则 |
| 视图状态保存/恢复 | `SaveViewState`、`RestoreViewState` | `TestViewStateRoundTrip`、`TestRestoreViewStateClamps`、`TestFoldViewState`、`TestWrapViewState` | 折叠/选区/顶部行/像素偏移可恢复，但不持久保存完整 wrap 行内亲和态；恢复时清空旧光标的换行边界侧与颜色间隙侧（验收欠项 Task 10） |
| 编辑器基础无障碍语义 | `Box.Semantics`、`A11yState`，宿主可覆盖 Box 名称 | `TestViewAccessibilityCodeEditor` | 本轮补为文件 URI 命名的 **textbox + Multiline + Focusable**，空 URI 兜底 Code editor；未增加全文可访问值/选区范围等文本接口，不宣称完整读屏支持 |

## 3. 语言与主题冒烟

- `TestAllLanguagesInView` 覆盖**实际全部 90 个注册 ID**，超过 brief 的 85。每种语言用 View 打开，开启换行与可选装饰，通过真实 View TextInput 输入，执行撤销并逐字核对还原，检查无被恢复而掩盖的 tokenizer panic。
- 有独立上游样本的使用 `langs/testdata/tokenize` 中首个非空 Monaco 用例；c→cpp、verilog→systemverilog、vue→html，freemarker2 默认 ID 复用 auto/dollar 变体。**csp、ecl 的上游用例为空，ini、pgsql、redshift 无对应导出用例，五种使用明确标注的本地样本**，不声称这五种已有上游用例覆盖。
- 这是冒烟，不是逐语言功能完备性证明；全部上游词法对拍由 `monarch.TestMonacoFixtures` 单独负责。语言装饰不适用时允许没有相应图形。
- `TestAllBaseThemesInView` 覆盖 `vs/vs-dark/hc-black/hc-light`；检查基础正文、背景、行号、光标颜色存在且非透明，验证实际背景绘制。可合法透明的装饰颜色不被强行要求不透明。
- 四张 PNG 在任务证据目录 `themes/`，已逐张读取检查。截图使用 `ui.Driver` 的纯 Go 字体路径（CJK 可显示缺字框），不是 Retina 实窗/输入法验收，也未覆盖任何已有 M1/GPU 报告或 golden。

## 4. §6.6 性能测量

本节全部由验收欠项计划 Task 10 在 2026-11-14 22:16 – 11-15 01:30 重测（Task 9 的 Monaco 数字除外）。上一版报告“拿 M1 历史数字直接比较”得出的 +37%/+45% 等差异是跨会话噪声，已被本节的成对比值取代，不再引用。

### 4.1 环境与口径

- Apple M4（10 逻辑 CPU）、Go 1.27.2 darwin/arm64；1920×1080 逻辑窗口 scale2（3840×2160 Metal 离屏）；`SetVerify(false)`；无屏幕 vsync，fps 是完成帧吞吐，不是实窗显示帧率。
- **成对测量**（计划 Global Constraints 的判据）：`ui/editor/perfab/arms.sh`。每臂每轮一个新进程，臂的顺序每轮轮转（ABAB），任一样本 1 分钟负载 ≥ 6 的整轮作废重跑；各指标取各轮中位数，比值 = 中位数之比；95% CI 为按轮成对 bootstrap；每组带一个“当前默认”的 A/A 副本臂量噪声。A = M1 树（`git archive 0ef80e4`），B = 当前工作区（测量开始时一次性编译的测试二进制）。所有臂共用同一份生成文件 `$TMPDIR/tango-editor-perf/gen1000000-15d1551d.ts`。
- **机器状态如实说明**：机器与其他会话共享（另一会话的 `go test`、vitepress 构建等）。第 1 组 10 个有效轮之外作废 30 次；保留轮负载 2.72–5.93。本次的绝对数值普遍比 Task 1（2026-11-14 上午，负载 2.26–5.91）差：例如当前默认的百万行中段输入 p95 中位数 10.63 ms（Task 1 为 6.89 ms），差异主要在 GPU 等待（3.24 ms 对 2.08 ms）与 p95 的进程间双峰，CPU 部分基本不变（3.73 对 3.62 ms）；跨会话的绝对数不可比，成对比值不受影响。
- CPU 项（cpu-ms）是事件投递 + `Runner.Frame`（构建、布局、绘制、编码、提交）；gpuwait 是提交后等待 GPU 完成。
- 原始日志：`.superpowers/sdd/2026-11-14-editor-m2-acceptance/t10/`（s1 成对 M1/默认/全关，s2 中等规模逐项，s3 长行换行，s4json 括号参考线，s4b 内存与 TS20k，s5 局部重绘滚动，s6 输入 CPU 分布与 damage 测试），每组含 `meta.txt`（二进制 sha）、`load.log`、`summary*.md`；Task 9 的 Monaco 原始数据见同目录 `monaco-raw/`。

### 4.2 门槛逐项结论

| 门槛（规格 6.6 / 计划） | 本次结果 | 结论 |
|---|---|---|
| 打开 100 MB 到可编辑 ≤ 1 s（首屏高亮 ≤ 1.5 s） | open-ms 中位 36.3 ms（10 轮，含读文件，热页缓存），首屏高亮同帧 36.4 ms | 通过 |
| 百万行输入 p95 ≤ 16 ms | 中段 10.63、末行 10.91、后台高亮中 13.38 ms（10 轮中位；各轮 p95 最大 16.22 / 13.37 / 20.63，中段与后台各有 1 轮 > 16 ms） | 通过（按各轮中位；余量比 Task 1 小，个别轮超 16 ms） |
| 连续滚动 ≥ 60 fps、p99 ≤ 33 ms | 全量重绘 147.9 fps / 14.92 ms；局部重绘（ScrollRegion）230.8 fps / 12.58 ms | 通过（绝对门槛） |
| TS 2 万行后台全量高亮 ≤ 2 s，期间输入不受影响 | 纯后台 319.0 ms；边输入边高亮 429.3 ms；期间输入 p95 12.38 ms，完成后 11.32 ms（1.09 倍） | 通过 |
| 内存：Go 堆 HeapInuse 增量 ≤ 3 倍（用户 2026-11-13 选定口径） | 打开 158.2–158.4 MB、全文高亮后 164.1–164.3 MB（2 个独立进程），文件 100.5 MB → 约 1.6 倍 | 通过 |
| 相对 M1 不退化 > 10%（成对，各轮中位数之比） | 当前默认/M1：输入 p95 0.962 / 0.932 / 1.003、滚动 fps 1.012、p99 1.015、打开 1.075（第 4.3 节） | 按计划判据通过；10 轮的 95% CI 上界最高 1.175，统计上未能排除 > 10% |
| 新增显示能力在百万行上合计不退化 > 10%（默认 / 全关） | 输入 p95 1.003 / 0.993 / 1.033，滚动 fps 1.010、p99 1.051，打开 1.012；CPU-ms 0.991–1.017（第 4.4 节） | 按计划判据通过；中段 p95 的 CI 0.796–1.131，**未能把 Task 1 的 CI 上界 1.105 收紧到 ≤ 1.10**，统计上未判定 |
| 长单行（1 MB，换行）输入 p95 ≤ 16 ms（计划 Task 5/5b） | **18.97 ms**（10 轮中位，各轮 16.35–35.79，没有一轮 ≤ 16）；Task 5 前 59.56 ms（3.14 倍） | **未通过**（第 4.6 节） |
| 若 Monaco 同场景更好则以 Monaco 为门槛：滚动 | 端到端 Tango 局部重绘 230.8 fps / p99 12.58 ms 对 Monaco 默认 667.8 fps / 5.02 ms；CPU 工作 Tango 均值 2.11 / p99 3.78 ms 对 Monaco work 2.26 / 8.76 ms | **未通过**（端到端 fps 与 p99 均差于 Monaco；CPU 工作口径 Tango 更好，第 4.8 节） |
| 同上：输入、打开、TS20k | 输入端到端 p95 Tango 10.63 对 Monaco 20.02 ms；输入 CPU 工作 p95 Tango 3.78 对 Monaco work 3.86 ms（p50 3.60 对 2.62，Monaco 更好）；打开 Tango 远好；TS20k 后台 319 对 324 ms | 通过（门槛统计量 p95；CPU 工作 p95 只好 2%，在噪声内；中位数 Monaco 默认档更好） |

### 4.3 对照 M1（第 1 组，10 轮，成对）

worse > 1 表示当前比 M1 慢（fps 换算为同方向）。A/A = 当前默认对自身副本的偏差。

| 场景 | 指标 | M1 中位 | 当前默认 | worse | 95% CI | 当前全关 / M1 | A/A |
|---|---|---|---|---|---|---|---|
| 百万行中段输入 | p95-ms | 11.045 | 10.625 | 0.962 | 0.878–1.089 | 0.959 | 4.8% |
| 百万行末行输入 | p95-ms | 11.710 | 10.910 | 0.932 | 0.759–1.027 | 0.939 | 2.6% |
| 后台高亮中输入 | p95-ms | 13.340 | 13.380 | 1.003 | 0.894–1.152 | 0.971 | 0.3% |
| 滚动 2x | fps | 149.7 | 147.9 | 1.012 | 0.985–1.164 | 1.002 | 4.0% |
| 滚动 2x | p99-ms | 14.690 | 14.915 | 1.015 | 0.849–1.175 | 0.966 | 0.4% |
| 打开 100 MB | open-ms | 33.805 | 36.345 | 1.075 | 0.985–1.158 | 1.063 | 5.5% |
| 中段输入 | cpu-ms | 3.699 | 3.726 | 1.007 | 0.985–1.047 | — | 0.6% |
| 末行输入 | cpu-ms | 3.814 | 3.674 | 0.963 | 0.947–1.000 | — | 1.2% |

结论：没有任何门槛指标的比值超过 1.10；与 Task 1（20 轮，2026-11-14 上午，当时的树还没有 Task 3–8 的改动）的结论一致（当时当前/M1 0.973–1.011，CI 上界 ≤ 1.087）。但本次 10 轮在更嘈杂的机器上，CI 上界到 1.15–1.18，**不能单凭本次数据以 95% 置信度排除 > 10%**。

### 4.4 显示能力在百万行上的合计成本（默认 / 全关，第 1、5 组）

全关 = `TANGO_EDITOR_PERF_OFF=all`（14 项显示能力）。成本 > 1 表示默认更慢。

| 场景 | 指标 | 全关中位 | 默认中位 | 成本 | 95% CI |
|---|---|---|---|---|---|
| 中段输入 | p95-ms | 10.590 | 10.625 | 1.003 | 0.796–1.131 |
| 末行输入 | p95-ms | 10.990 | 10.910 | 0.993 | 0.814–1.150 |
| 后台高亮中输入 | p95-ms | 12.950 | 13.380 | 1.033 | 0.843–1.166 |
| 滚动 2x（全量） | fps | 149.4 | 147.9 | 1.010 | 0.945–1.113 |
| 滚动 2x（全量） | p99-ms | 14.185 | 14.915 | 1.051 | 0.945–1.208 |
| 打开 100 MB | open-ms | 35.925 | 36.345 | 1.012 | 0.914–1.106 |
| 中段 / 末行 / 后台输入 | cpu-ms | — | — | 1.016 / 0.991 / 1.017 | 0.955–1.069 |
| 滚动 2x（全量，第 5 组 10 轮） | fps / p99 | 151.9 / 14.815 | 149.0 / 15.100 | 1.019 / 1.019 | 0.997–1.054 / 0.991–1.053 |
| 滚动 2x（局部重绘，第 5 组 10 轮） | fps / p99 | 235.6 / 13.015 | 230.8 / 12.575 | 1.021 / 0.966 | 0.921–1.028 / 0.904–1.009 |

结论：所有点估计 ≤ 1.051，CPU 部分 ≤ 1.017；滚动在第 5 组（10 轮、负载 2.77–3.89，噪声小）的 CI 上界 ≤ 1.054。输入 p95 的 CI 仍宽（p95 在进程间双峰，随 GPU 等待一起跳），中段 p95 上界 1.131 —— Task 10 未能完成“收紧到 ≤ 1.10”的要求，原因是 10 轮的机器噪声大于 Task 1 的 20 轮。

逐项归因：百万行文件上 8 项能力被大文件降级关闭（minimap、sticky、folding、occurrences、selectionhl、links、colors、brackets），whitespace 在默认 `selection` 模式无选区时不画，所以真正起作用的只有 overview、guides、matchbrackets、unicode、linehl。Task 1 的逐项成对（12 轮）没有任何一项 > 1.10（最大 linehl 中段 p95 1.059，在空操作对照的噪声范围内）。本计划新增/改动能力在百万行上的成对结果：括号对参考线（默认关；开启时 1M 无括号树）输入 p95 1.036、滚动 fps 1.058 更快（Task 3，A/A 8–16%）；颜色间隙 1.036 / 1.055 / 1.051 / 0.990（Task 4，A/A 3–5%）；缩进检测打开 0.985（Task 6）；小地图字符 1.009 / 0.994 / 1.013（Task 8）。都在 1.10 内，但都是 5 轮、各自当时的噪声。

### 4.5 非大文件上的逐项成本（第 2 组，仅报告，不是规格门槛）

2 万行 TypeScript（`BenchmarkTypeTS20k`/`BenchmarkScrollTS20k`，Task 10 新增，全量重绘）与 2 万行带颜色字面量的 CSS（`BenchmarkTypeCSS20k/colors`、`BenchmarkScrollCSS20k/colors`），文件低于所有大文件阈值，各能力实际生效。11 臂、5 轮、负载 2.28–4.42。成本 = 默认 / 关闭该项（fps 为关/开），> 1 表示该能力让默认变慢。

**噪声如实说明**：本组只有 5 轮，A/A 副本臂对参考臂的偏差在 CPU-ms 上达 14–19%、CSS 滚动 fps 达 14%，单项 CI 普遍跨 0.7–1.4；**除“全关”与“小地图”外，单项比值都在噪声内，不能据此判断单项成本**。按控制者指示不再追加轮数。selectionhl 与 whitespace 只在有选区时工作，这些基准没有选区，对它们而言是空操作对照。

| 关闭项 | TS 输入 p95 | TS 滚动 fps | CSS 输入 p95 | CSS 滚动 fps |
|---|---|---|---|---|
| A/A（副本） | 1.036 | 0.999 | 0.976 | 1.141 |
| **全部 14 项（all）** | **1.391**（1.11–1.67） | **1.360**（0.85–1.56） | **1.615**（0.90–2.09） | 1.018 |
| **minimap** | 0.966 | **1.190** | **1.250**（1.02–1.42） | **1.128**（1.01–1.31） |
| sticky | 0.952 | 0.889 | 1.003 | 0.954 |
| folding | 0.915 | 0.873 | 1.093 | 0.914 |
| occurrences | 0.989 | 0.856 | 1.071 | 0.958 |
| selectionhl（空操作） | 0.922 | 0.916 | 1.080 | 0.933 |
| brackets | 0.999 | 0.894 | 1.042 | 0.947 |
| links | 1.033 | 0.847 | 1.044 | 0.941 |
| colors | 1.026 | 0.920 | 1.025 | 0.930 |

如实结论：
- 非大文件上，**全部显示能力合计让输入 p95 慢约 39%（TS）到 61%（CSS），TS 滚动吞吐低约 36%**；CPU-ms 默认 2.70 对全关 1.02（TS 输入）。这不在规格门槛内（门槛只定义在百万行文件上，那里大部分能力被降级关闭），但说明“每项 ≤ 10%”在非大文件上不成立于合计层面。
- **小地图**是唯一单独看得出的项：CSS 滚动 fps 约 +13%、TS 滚动 +19%、CSS 输入 p95 +25%（与 Task 8 的“小地图在 2 万行文件上滚动平均帧时间约 +15%，字符与色块模式都如此”一致）。这是 M2 引入的小地图本身的成本，不是本计划的字符形态带来的（Task 8：字符模式 CPU 比色块更省）。
- 其余单项在本组的噪声内无法分辨。合计成本大于各单项之和看起来的量级，说明成本分散在多项中（或单项比值被噪声掩盖）。

### 4.6 长单行换行输入（第 3 组）

`BenchmarkTypeLongLineWrap`：1 MB 单行（含 Tab 与中文注释），换行开启，行中连续输入 300 键。3 臂（当前、Task 5 前快照 3312ad6、当前 A/A）、10 轮、负载 3.13–5.69。

| 臂 | p50 | p95（10 轮中位） | 各轮 p95 | max | cpu-ms | gpuwait-ms |
|---|---|---|---|---|---|---|
| Task 5 前（3312ad6） | 52.39 | 59.56 | — | 71.38 | 46.65 | 6.69 |
| 当前 | 11.11 | **18.97** | 16.35 35.79 20.57 22.07 19.27 17.73 17.53 17.48 19.00 18.93 | 32.57 | 6.44 | 5.94 |
| 当前 A/A | 11.40 | 18.83 | — | 29.29 | 6.36 | 6.07 |

- 相对 Task 5 前：p95 快 3.14 倍（CI 2.90–3.43），CPU 少 7.2 倍。每键工作不再随行长增长（`TestLongLineKeystrokeWork` 等计数测试）。
- **门槛 p95 ≤ 16 ms 本次未通过**：10 轮没有一轮 ≤ 16 ms。Task 5b 在另一时段（负载 2.3–5.6）四次测得 12.97 / 13.02 / 16.06 / 12.85 ms；本次 CPU 部分（6.44 ms）反而比 Task 5b（7.2–7.4 ms）少，差在 GPU 等待（5.94 对约 4.6 ms）与帧调度，与第 4.1 节所述本时段整体 GPU 等待偏高一致。但门槛按实测判，**不能把它写成通过**：p95 对帧调度与 GPU 等待敏感，余量不足。

### 4.7 其他测量

- **TS 2 万行**（第 4b 组，负载 3.6–3.8）：纯后台全量高亮 319.0 ms（10 次均值）；边输入边高亮 429.3 ms（15 次），期间 392 键 p50 6.93 / p95 12.38 / max 17.43 ms，完成后 1500 键 p95 11.32 ms。
- **百万行全文后台高亮**（参考，无门槛）：16.20 / 16.18 s（随内存基准的两个进程）。
- **括号对参考线 JSON**（`BenchmarkTypeJSONBracketGuides`，4.4 MB、12.5 万行 JSON，第 4 组 5 轮，负载 3.71–4.18）：开 / 关 p50 1.011（0.996–1.031）、p95 1.013（0.976–1.052）、cpu-ms 1.047（1.011–1.389），A/A p95 0.937。Task 3 修正轮 2 之后无法复测的一项至此复测完成，≤ 1.10。
- **颜色装饰（2 万行 CSS）**：见第 4.5 节 colors 行（输入 p95 1.025、滚动 fps 0.930，均在噪声内）；Task 4 的 5 轮结果（输入 +1%，滚动 fps −3% 到 −5%）仍为最近的低噪声数据。

### 4.8 Monaco 0.56 同场景基线（Task 9）与可比性

Monaco 条件：Edge 157 无头、每轮全新进程、`--disable-frame-rate-limit --disable-gpu-vsync`、1920×1080@2x、同一生成文件，各项 5 轮、负载 2.87–5.72；“默认档” = Monaco 默认 `largeFileOptimizations: true`（100 MB 上语法高亮整体关闭），“nolfo 档” = 关闭大文件优化（高亮开启，工作量与 Tango 相当）。Monaco 帧只计到主线程渲染结束，**光栅化与 GPU 合成不计入**；Tango 端到端包含等待 GPU 完成。规格 6.6 写明对比时 Monaco 用默认配置（会降级），本节以默认档为门槛对照，nolfo 档同列参考。Tango 数字为本次 Task 10 测量（与 Monaco 不是同时段交替运行，跨引擎无法同进程交替）。

| 场景 | Tango（本次） | Monaco 默认档 | Monaco nolfo 档 | 可比口径与判定 |
|---|---|---|---|---|
| 打开 100 MB → 可编辑 | 36.3 ms（含读文件） | 262.8 ms（不含取文件 85 ms） | 360.6 ms | Tango 更好 |
| 首屏 token | 36.4 ms | 无（不分词） | 390.4 ms | Tango 更好 |
| 百万行中段输入 端到端 p95 | 10.63 ms | 20.02 ms（受 60 Hz 帧时钟主导） | 19.01 ms | Tango 更好 |
| 百万行中段输入 CPU 工作 p50 / p95 | 3.60 / 3.78 ms（`BenchmarkTypeAt1MLinesCPU`，5 进程中位，负载 3.7–4.9） | work 2.62 / 3.86 ms | 3.86 / 5.02 ms | p95 Tango 略好（2%，噪声内）；p50 Monaco 默认档更好（它不分词）；对同工作量的 nolfo 档 Tango 两项都更好 |
| 滚动 端到端 fps / p99 | 全量 147.9 / 14.92 ms；**局部重绘 230.8 / 12.58 ms** | **667.8 / 5.02 ms** | 490.0 / 4.73 ms | **Monaco 更好 → 未通过** |
| 滚动 CPU 工作 均值 / p99 | 局部重绘 2.11 / 3.78 ms（cpu-fps 约 474）；全量 2.41 ms 均值 | work 2.26 / 8.76 ms | 3.45 / 8.73 ms | Tango 更好 |
| TS20k 后台全量 | 319.0 ms（边输入 429.3） | 323.7 ms；同步 force 129.6 ms（阻塞主线程，非同一场景） | — | 相当（Tango 略好） |
| TS20k 高亮期间输入 p95 | 12.38 ms | 延迟 20.12 / work 10.49 ms | — | 端到端 Tango 更好；work 口径 Monaco 略好（10.49 对 Tango CPU 均值 1.88 ms 不同统计量，未判定） |
| 内存（打开 / 全文高亮后增量） | HeapInuse 158.3 / 164.2 MB；footprint 387 / 356 MB；RSS 232 / 289–321 MB | JS 116.1 MB / 不适用；footprint 184.5 MB | JS 141.9 / 290.1 MB；footprint 228.6 / 601–604 MB | 不设 Monaco 门槛（用户口径为 Go 堆 ≤ 3 倍）；高亮开启时 Tango 更省 |
| 百万行全文后台高亮（无门槛） | 16.2 s | 不适用 | **7.20 s** | Monaco 快约 2.2 倍（后续项） |

**滚动可比性（按 Task 9 的裁定执行，不把门槛解释掉）**：Task 10 新增 `BenchmarkScrollPartial`（`ui/editor/perf_task10_test.go`），走与应用相同的局部重绘路径：设备为 Retainer，View 的 ScrollRegion 提示变成保留目标上的像素平移（实测 99.9% 的帧为局部帧且带平移），只重画新露出的行。这对应 Monaco “合成器平移已光栅化图块、只画新露出两行”的工作方式。结果：
- **CPU 工作口径**（Tango 事件 + `Runner.Frame`，不含 GPU 等待；Monaco 主线程 work，不含光栅/合成）：Tango 均值 2.11 ms、p99 3.78 ms，Monaco 默认档 2.26 / 8.76 ms、nolfo 3.45 / 8.73 ms —— **Tango 更好**。（注：Tango 每帧 CPU 分布双峰，中位 3.35 ms、均值 2.11 ms。）
- **端到端口径**（Tango 含等待 GPU 完成，Monaco 为 rAF 间隔、不含光栅/合成）：Tango 230.8 fps / p99 12.58 ms，Monaco 默认档 667.8 fps / 5.02 ms —— **Monaco 更好**，Tango 的 fps 只有其 35%，p99 是其 2.5 倍。两边端到端不是同一工作量（Tango 的帧同步等待 GPU、不做流水线；Monaco 的光栅与合成在其他线程/进程且不计入），但按规格 6.6 字面与 Task 9 裁定，**该项以 Monaco 为门槛判为未通过**，差距主要在 GPU 等待（Tango 局部重绘帧 gpuwait 2.21 ms，且 p99 尾部 12.6 ms 远高于 CPU p99 3.8 ms）。
- 规格的绝对门槛（≥ 60 fps、p99 ≤ 33 ms）Tango 三种口径都满足。

### 4.9 内存：口径已定（2026-11-13 用户选定 Go 堆增量）

更正上一版“尚未选定唯一指标、本报告不代用户选择”的表述：**2026-11-13 用户已选定“文件相关内存 = Go 堆 `HeapInuse` 增量，≤ 文件大小 3 倍”**（`docs/specs/code-editor-m1-report.md` 第 35 行）；进程 footprint 与 GPU 内存归规格 6.8 的窗口级预算，不计入本条门槛。

本次 `BenchmarkMemory100MB` 两个独立进程（负载 3.2–3.6；各在 GC + `debug.FreeOSMemory` 后采样，相对打开前基线的增量，MB = 10^6 字节，文件 100.5 MB，3 倍约 301 MB）：

| 指标 | 进程 1：打开 / 全文高亮后 | 进程 2：打开 / 全文高亮后 | 判定 |
|---|---|---|---|
| **HeapInuse（门槛口径）** | **158.4 / 164.3 MB**（基线 1.94） | **158.2 / 164.1 MB**（基线 1.84） | **约 1.6 倍，通过** |
| RSS | 231.8 / 289.3 MB（基线 26.3） | 231.5 / 320.9 MB（基线 26.3） | 窗口级预算项（6.8），不计入门槛 |
| phys_footprint | 388.0 / 358.6 MB（基线 11.5） | 386.9 / 355.5 MB（基线 11.5） | 约 3.5–3.9 倍；含 Retina 渲染目标与字形图集约 127 MB（M1 报告剖析），归窗口级预算（6.8），不计入门槛 |
| 峰值 RSS（绝对） | 376.0 MB | 376.1 MB | 含读文件临时副本 |

RSS 与 footprint 作为规格 6.8 的窗口级预算项继续跟踪；若将来改用进程 footprint 为门槛，现状不达标。

### 4.10 未测或待定

1. 真实屏幕与 Windows 实机：本轮无实窗（锁屏），Windows 只有交叉编译；不构成实窗/Windows 性能结果。
2. 统计置信度：相对 M1 与显示能力合计成本的输入 p95 CI 未收紧到 ≤ 1.10（第 4.3、4.4 节）；需要在空闲机器上 ≥ 20 轮重测才能给出 95% 置信的结论。
3. 最坏情况：海量光标、病态正则、无断点长串里的长行编辑、键入不配对括号的长行仍可能超过 16 ms/键（第 5 节）。

### 4.11 2026-11-15 后续修复与复测（A–E，独立于 Task 10 历史数据）

来源：本地工作副本 `.work/task-ab-report.md`、`.work/task-c-report.md`、`.work/task-e-report.md`。本节汇总已有证据，Task D 不新增测量或测试；第 4.2–4.10 节仍为 Task 10 当时的结果，不能视为本轮重测。

**A：长单行输入。** `BenchmarkTypeLongLineWrap` 在负载 **4.43–5.76** 下做 **12 轮交替**（diffview-final 基线 / 改后 / 同二进制 A/A）：输入 p95 逐轮中位数 **17.17 → 13.98 ms**，A/A 改后臂 **12.91 ms**；改后/基线比 0.814（95% CI 0.710–0.941）。修复包括小地图缓存依赖避免每键重扫所有预览行、截断点之后的长行编辑复用有效 token、减少无必要的整行复制及长行换行 ASCII 快路径。**按逐轮 p95 中位数判据，≤16 ms 门槛已满足；不等于稳定地每轮达标。** 主改后臂有 **4/12** 轮超过16 ms（16.02、16.33、16.27、16.29），A/A臂另有16.63 ms，合计 **5/24**。A+B源报告正文的“4/24”为算术错误，此处按其逐轮表更正。改后 max 中位33.83 ms（基线28.87），级联换行会更新整个小地图，尾部与稳定性仍须跟踪；本节不因此把所有性能门槛改判通过。

**B：小地图剩余工作。** 共享高亮截止时间后仍有未处理行时，视图现在请求下一帧；持续零进展有界，完成后停止请求。严格 damage 夹具移除 Task 10 的无条件沉淀帧，改成实际执行视图请求的帧（最多50帧、计数且严格验证），blink 只损坏光标的原断言保留。A+B报告记录压力下12/12通过；不能把“等待到像素碰巧相同”当作产品完成信号。

**C：滚动。** 14:52–15:22按负载<6进行base/before/after交替（每臂1000帧），仅取得 **3个有效完整轮**，负载 **4.17–5.55**；另两轮因结束负载≥6作废，达到30分钟墙钟上限后停止，**≥5轮复测仍PENDING**。结果为各轮中位数，仅作暂存，不宣称fps提升：

| 指标 | diffview-final基线 | before C（含A/B） | after C |
|---|---:|---:|---:|
| fps | 256.2 | 256.1 | 214.9 |
| p99 ms | 8.473 | 9.255 | 12.430 |
| CPU均值 ms | 2.241 | 2.294 | 2.616 |
| CPU p99 ms | 4.928 | 5.015 | 6.647 |
| wait均值 ms | 1.726 | 1.594 | 2.038 |

其中after首轮max149 ms、p99 36.49 ms，前后1分钟load仍<6，依既定规则保留；不能删掉该轮来宣称改善。确定的编辑器侧收益只有样式边界scratch复用：单次多token行样式转换 **7→2 allocations**，返回runs/paints不引用scratch。新增计数/严格像素测试约束scale1/2稳态两行滚动damage≤两行+2 logical px；计数测试×3通过，damage/navigation/goldens通过，三平台vet通过。详细阶段计时仍待低负载补测。

渲染路径已确认：ScrollRegion/ScrollShift生效，稳态 **1 copy、1 dirty rect，重画目标约3.86%**，不是全屏重画gutter/小地图；Metal把约95.4%目标区域经scratch做两次blit，partial pass是load+局部clear。GPU项目需求（本轮未改internal/render）：提供独立encode/commit、前帧等待、queue latency与GPUStart/EndTime计时及fence接口，避免基准靠再提交1px帧等待；量化并降低双blit流量、retained load/store及单缓冲同步固定成本，给出copy-only/strip-only/合成帧和实际present的低负载证据。初版合成帧测量曾因atlas尺寸切换引入纹理重建，已修正测量器并作废其耗时，不能拿它证明GPU瓶颈。

**Monaco判定不变。** Tango上述端到端含GPU完成，Monaco默认headless **667.8 fps / p99 5.02 ms**仅计主线程、不含raster/compositor（主线程work均值2.26 / p99 8.76 ms；nolfo保留高亮490 fps / p99 4.73 ms）。CPU工作与主线程work应单独比较，不与Tango含GPU总时长混用；本轮3轮after CPU均值2.616 / p99 6.647 ms也不足以替代正式复测。按控制者对§6.6的裁定，不把口径差异解释为豁免：**滚动对Monaco仍NOT PASSED**；长行中位数达标、分配减少或绝对60fps达标均不替代该门槛，整体自动化验收仍未通过。

**E：CI测试修正（据Task E报告，最终集成验证交控制者）。** Bounded/ActiveGuide墙钟门槛统一走gateLimit，Converge使用产品侧maxMatchFrames且保留进展/完成断言；Decorations对具体操作逐帧严格核对，只有小数设备像素边界导致无法安全拷贝时允许横滚整帧；长行括号颜色、短行复制计数按有界token/括号工作完成后断言；reindent仅在SyntaxCommandIncomplete且文本/选区未改变时有界重试。A新增长行截断测试先建立初始final前提，再于每次编辑后立即断言，不能靠编辑后settle掩盖弄脏token。

Task E报告记录：12×yes争用且倍率3时8项连续5次通过；同争用无倍率的6项语义测试连续5次通过；24×yes下长行/短行/reindent连续5次通过；三平台交叉vet通过。这些不是性能达标证据，也不是Windows/Linux实机运行。Windows小地图golden失败为autocrlf转换CRLF，整合时须带入上游0d3b59a的`.gitattributes`递归LF规则，本任务未改golden或越界修改根配置。E原副本最终复核中，无倍率的纯墙钟门槛仍曾因抢占失败；另发现新增highlight测试把已接受的降级结果当成完整token比较。该测试现已改为在完整比较前有界获取非降级结果，且保留重试前“不额外分词”的断言；注入降级用例与12×yes下连续30次通过。最终在最新origin/main集成工作区运行 `go test ./ui/editor/... ./examples/editor-demo -count=1` 全部通过，随后修改的highlight包再次单独全量通过；darwin/windows/linux交叉vet、apitxt -check、demo构建、diff-check均通过，递归LF规则已包含在基线。以上不代表Windows/Linux实机测试或推送后CI已通过。真实窗口/IME/平台验收，以及≤10%门槛进一步统计收紧仍待后续。

### 4.12 2026-11-15 F/P 候选：锁屏离屏 paired 复测

本节是 F-only 与 F+P 的单变量候选验证，不覆盖 M1→最终候选的完整正式 paired，也不覆盖真实窗口呈现。测量时 `CGSSessionScreenIsLocked=Yes`；所有结果均标为**锁屏离屏诊断**。F 引入真实 GPU Wait 并统一了两臂的等待口径，P 引入 Metal retained copy 的自适应 ping-pong。两臂均基于 `9382b0b`，使用同一生成文件、同一 scale2（3840×2160 设备像素）、同一测试宿主和独立进程；负载阈值为 6，保留轮负载 1.84–3.02。

补丁校验：F `c64d5b6f9cdd218312ab43440fc40c006d4e1bc22a7990087d91efcd5b49ba63`；P `5a65351c9c1fce8092f13acf0463834ea9960684f785514017aee705f3797744`。F-only、F+P 两臂均通过 Metal 包测试、编辑器全量回归和 `go build ./...`；原始日志在 `.superpowers/sdd/2026-11-15-editor-m2-perf-acceptance/paired/`。

#### 4.12.1 长单行换行：10 轮

`BenchmarkTypeLongLineWrap`，1 MB 单行、换行开启、每臂 300 次输入，F+P 对 F-only：

| 指标 | F-only 中位 | F+P 中位 | F+P / F-only | 95% CI |
|---|---:|---:|---:|---:|
| p50-ms | 3.875 | 3.875 | 1.000 | 0.997–1.003 |
| p95-ms | 4.928 | 4.889 | 0.992 | 0.986–1.014 |
| max-ms | 16.090 | 15.980 | 0.993 | 0.933–1.053 |
| cpu-ms | 1.474 | 1.454 | 0.986 | 0.971–1.018 |
| gpuwait-ms | 2.635 | 2.637 | 1.001 | 0.998–1.007 |

结果说明：F+P 没有引入长行 p95 回归；但这是锁屏离屏、统一 Wait 后的候选对候选数据，不能替代原 Task 10 的 M1/当前正式口径，也不能据此把历史长行门槛改写成实窗稳定通过。

#### 4.12.2 局部滚动：20 轮

`BenchmarkScrollPartial`，1M 行、scale2、Retainer/ScrollRegion，每轮 300 帧；A/A 控制臂噪声 p99 约 0.6%。F+P 对 F-only：

| 指标 | F-only 中位 | F+P 中位 | 结果 | 95% CI |
|---|---:|---:|---:|---:|
| fps | 208.8 | 209.0 | +0.1% | 0.996–1.000（worse 口径） |
| p99-ms | 6.264 | 6.514 | +4.0% | 0.986–1.098 |
| mean-ms | 4.790 | 4.663 | -2.6% | 0.971–0.975 |
| cpu-ms | 3.357 | 3.359 | +0.1% | 0.998–1.002 |
| gpuwait-ms | 1.434 | 1.306 | -8.9% | 0.905–0.914 |
| partial | 0.997 | 0.997 | 不变 | 1.000–1.000 |

20 轮所有轮次负载低于 6，无效轮次为 0。F+P 降低了离屏 GPU wait，CPU 工作量和 partial 比例不变；p99 点估计上升 4.0%，CI 上界 9.8% 未超过 10%，但这仍是锁屏诊断，不能替代真实 present/vsync。

#### 4.12.3 F+P 锁屏离屏绝对基准

单独运行 F+P 二进制得到以下数字；输入/滚动/打开/TS20k/内存均不是实窗验收结果：

| 场景 | 结果 |
|---|---:|
| 百万行中段输入 p95 | 6.371 ms（cpu 3.692 / gpuwait 1.975 ms） |
| 百万行末行输入 p95 | 5.729 ms（cpu 3.613 / gpuwait 1.996 ms） |
| 百万行后台高亮中输入 p95 | 8.651 ms（cpu 3.657 / gpuwait 3.334 ms） |
| 全量滚动 scale2 | 179.8 fps / p99 5.855 ms |
| 局部滚动 20 轮中位 | 214.4 fps / p99 6.514 ms |
| 打开 100 MB | 30.17 ms mean / 27.53 ms p95 / 30.19 ms max |
| TS20k 输入 | p95 6.681 ms（cpu 2.452 / gpuwait 2.406 ms） |
| TS20k 滚动 | 291.1 fps / p99 6.561 ms |
| TS20k 后台高亮 | 本次单独绝对 benchmark 未覆盖完整后台时长；既有 Task 10 数字仍为 319 ms |

`BenchmarkMemory100MB` 本次：文件约 100.5 MB；HeapInuse 增量 open 158.1 MB、全文高亮 164.0 MB，按锁屏离屏诊断口径低于 Go HeapInuse 的 3 倍数值参考线，不能单独计入正式验收通过。该进程的 footprint 增量 open 353.0 MB、全文高亮 352.3 MB，属于窗口级预算，不是本条 HeapInuse 门槛。

P 的原生纹理成本必须单独计入：启用过大型 retained-copy 路径且尚未 idle 回收时，每个 Device 最多额外持有一张 Retina spare target，约 33,177,600 B（33.18 MB / 31.64 MiB）；原 target + spare 合计约 66.36 MB，不含 atlas/images/layers/buffers。fallback/small-copy 期间 scratch 可能暂时共存，spare 在 resize、Close 或 120 个未使用帧后回收。Go HeapInuse 数字不包含这项 Metal 原生纹理成本；多窗口时按 Device 线性增加，窗口级预算仍待真实窗口测量。

#### 4.12.4 本节结论

- F-only 与 F+P 在统一真实 Wait 口径下通过了本轮锁屏离屏正确性和性能回归；P 的局部 copy 路径降低 GPU wait，未改变 CPU 工作或 partial 覆盖率。
- 长行、绝对输入、绝对滚动、打开和 HeapInuse 的锁屏离屏数字满足各自绝对数值，但不替代真实屏幕解锁后的验收。
- 真实窗口 present/vsync、Monaco GPU-complete trace、M1→最终候选的正式 paired、IME 和 Windows 实机仍未验收；整体自动化验收保持未通过。
- `go test -race ./ui/editor/... -count=1` 本轮全量在 10 分钟墙钟限制内超时于 `TestViewDamageVerifiedTask14` 与 `diffview.TestNavigationKeys`；两项分别单独运行均通过，未出现数据竞争报告，按已知 race 并行减速限制记录。

## 5. 降级、已知欠项与 M3 接口

列出历史台账中的接受/延期只是在记录事实，**不代表本轮获得放宽验收或将缺失能力自动移交M3的授权**。完整规格仍按未满足处理；是否带这些欠项结项由维护者决定。

### 产品降级（创建 Model 时判定，不随随后跨阈值自动反转）

- `>20*1024*1024` UTF-8字节或 `>300000` 行：折叠、同词/选区匹配、自动换行、sticky scroll、小地图关闭。小地图比 Monaco 额外降级，不能声称完全一致。
- `>5000000` UTF-8字节：不构建括号着色树；`MatchBrackets` 走有预算的旧路径。
- `>50*1024*1024` UTF-8字节：链接与颜色装饰关闭。Unicode按视口处理；**语法高亮保持开启**。
- **阈值按UTF-8字节，Monaco相应判断按UTF-16字符长度**；中文/emoji文件可能提前降级，此差异未消除。

### 不因任务1–14“复审完成”而消失的欠项

- **验收欠项计划已补齐（不再是欠项）**：括号对参考线（Task 3）、颜色占位真实间隙（Task 4）、长单行增量换行与每键整行开销（Task 5/5b）、缩进检测 detectIndentation（Task 6）、稀有查询可续扫描与 4 条 LRU 查询缓存（Task 7）、Unicode 区域表与小地图字符形态（Task 8）、Monaco 同场景基线（Task 9，`tools/monaco-bench`）。另修复两个 M2 既有缺陷：括号树合并编辑区域行数少算（Task 3）、小地图 Fit/Fill 采样跳行（Task 8）。
- **仍缺失（属里程碑 3）**：颜色 provider（`ColorProvider` 注册接口）与语义 documentHighlight provider。
- **长行/预算**：长单行增量换行后仍为 O(行长) 的情形——无断点长串中编辑、键入不配对括号、不换行长行的括号着色与块表、长行上的括号对参考线；Enter改变行数仍需O(n)行树重建；trim仍需O(n)文本扫描，但不再强制无限制全文分词；trim/reindent在语法预算不足时安全跳过/整次取消并报告 incomplete。稀有查询扫描期间无变化的帧框架整帧重绘（后续 A：后台 goroutine 扫描；后续 B：框架让“已声明但 damage 为空”的帧跳过渲染，需改 `ui/` 与 `internal/render`）。
- **显示差异（验收欠项计划各任务记录）**：颜色间隙换行按像素小数列（Monaco 1 列）、sticky 标题不显示间隙、有间隙行上 IME 组字覆盖绘制、swatch 垂直居中、无 `colorDecoratorsLimit` 500；小地图色块在高行 Fill 下不居中、采样模式条带每帧重合成；概览标尺不画同词/选区匹配标记；百万行全文后台高亮 Monaco 快约 2.2 倍（7.2 s 对 16.2 s，第 4.8 节）。
- **视图语义**：undo后affinity、Alt+点击换行末端affinity、保存状态的完整视觉行亲和态；折叠隐藏选区方向边界；组字只在主光标预览；Enter按共享Model风格规范化已有缩进（缩进检测后与 Monaco 一致）。模型池和后台 goroutine一般仍需宿主显式管理；本轮demo已由每Model生命周期管理器负责首次有lexer时启动、语言往返复用、退出取消/退订/关闭种子channel，不把该管理责任推给State.Close。
- **高亮/括号**：嵌入编辑辅助使用文档语言；final token之外的括号树要等高亮推进，未开启后台高亮时复杂多光标匹配可能跨许多帧收敛（台账记录过约370帧）；小范围活跃视口的顺滑不代表全文件立即正确收敛。
- **分词器暂停修复必须保留**：独立复验已通过，不在Task15回退。一次灾难回溯仍需完整100ms尝试×2，约200ms加检查粒度确认；两个长暂停仍可能误降级，生产重试耗尽可能接受截断结果为final。不得宣称完美抗负载或可1ms抢占任意正则。
- **命令/菜单细节**：同行冲突去重、重叠删除并集、Intl.Collator差异、IME后双击修饰键抑制、静默丢弃不可应用的排队编辑、无匹配菜单快捷键fallback等仍见任务报告。
- **基础层保留问题**：Task1台账中的复杂damage矩形合并成本、不可见图像触及/保留策略等不在Task15修复范围。本轮不修改框架/GPU/平台实现，也不以编辑器通过掩盖它们。

### 台账中仍需维护者逐项处理的边角记录

下列来自各任务的 deferred 行；本轮未逐一重新定性，**没有把它们默认为已修复**：

- Task2：不可应用的重入排队编辑现可通过 `Model.DroppedEdits()` 累计计数观察；排队调用仍立即返回 nil，不新增 ErrQueued 协议。
- Task3：swallow 标志到下一次 keydown 才清除；IME 后双击修饰键缺少专门抑制。
- Task4：Cmd+D no-op 返回值、空 Sels 补齐时 lastAdded、不重叠情形以外的列取整/上下移动起止语义。
- Task5：Intl.Collator 不同；`_deltaColumn` 仅作用于head。trim/reindent 的无限制同步语法分类与长行截断误判已在 final fix 修复，代价是预算不足时安全保留/取消，详见 §8。
- Task6：IME提交不自动闭合、Enter独立undo步、列1反斜线overtype；processBrackets对ColorizedBracketPairs、显式空AutoCloseBefore的处理差异。
- Task8：ContextMenu=false时gutter右键聚焦/移动边界、无匹配when时菜单快捷键fallback；早期首项自动选择缺口则已由NoAutoSelect消除。
- Task10：内部ModelToView/ViewToModel调用整合的技术债、控制/全宽字符列宽、视图状态的wrap行内信息、tab修改到paint间的短暂状态，以及已在正文列出的affinity/长行问题。
- Task11：>20k行折叠控件防抖闪现、16px控件命中区、控件隐藏时ranges重算时机，以及隐藏选区Shift/方向键边界。
- Task12：现有收敛测试没有独立隔离“View侧结果累积”与cache作用；不据缓存测试全绿就宣称所有多光标远距离场景即时完成。
- Task13：编辑器间blur、链接hover/下划线（locale 表与单查询缓存 A→B→A 重扫已由验收欠项 Task 8、Task 7 解决）。Task14真正partial执行路径的验证缺口已修复，不能与这些遗留混淆。

### 留给 M3 的真实接口与尚缺接口

| 接入点 | 当前事实 | M3还需要 |
|---|---|---|
| `FoldingRangeProvider.FoldingRanges(context.Context, buffer.Snapshot)`、`ViewW.FoldingProvider` | 已有同步、快速返回/缓存结果协议，错误或nil回退缩进策略 | 宿主语言选择器注册、异步版本淘汰与服务结果缓存；不可在UI线程阻塞LSP请求 |
| 颜色 | 当前内置CSS字面量扫描与picker | **尚无公开ColorProvider注册接口**；要新增颜色信息/展示形式协议、选择器与版本取消；ui/text内联占位应由归属会话实现 |
| documentHighlight | 当前纯文本同词/选区匹配 | **尚无语义documentHighlight provider**；接入按语言/版本返回的读/写/文本范围，不能把文本匹配宣称语义高亮 |
| `State.SetOverviewMarks(owner, []OverviewMark)` | 已有owner替换/删除与模型行/lane/color输入，视图本地 | 桥接诊断/查找结果、按编辑映射标记、过期结果丢弃；当前不是通用稳定范围装饰 |
| `Model.Subscribe`、Snapshot、UTF-16转换 | 共享模型与快照可供provider读取 | 服务异步取消/回调必须回UI线程、版本校验、池销毁取消 |

## 6. 实窗验收清单（待人工验收，不伪造截图）

`desktop-probe.log` 记录 `CGSSessionScreenIsLocked=Yes`。因此不向锁屏会话投递输入、不把离屏PNG冒充实窗截图，也不启动遗留app进程。M1/GPU M2已有同事实窗补测保留原样，但不能作为新split/palette的本轮通过记录。

| 实机步骤 | 本轮状态 | 自动化替代与差距 |
|---|---|---|
| `-split` 两侧编辑、撤销、不同位置滚动/折叠、切换语言 | 待解锁实测 | demo共享模型/焦点测试、`TestTwoViewsShareModel`，不是实窗 |
| 右键菜单→Command Palette→搜索→执行，焦点回原分屏 | 待解锁实测 | 菜单Driver测试与demo目标焦点测试 |
| 中文输入法组字中Cmd+D、多个光标提交、撤销 | 待用户真实输入法实测 | 合成composition事件通过不等于拼音输入法验收 |
| 选区拖动、Option复制、同帧松开/移动、撤销 | 待解锁实测 | `dnd_test.go`真实事件路径，仅Driver |
| `-wrap` + 折叠 + sticky + resize，CJK/tab/emoji跨行点击与导航 | 待解锁实测 | mixed script、fold、strict damage组合测试 |
| 百万行GPU `-auto`，滚动/输入显示节拍，关闭后无残留进程 | 待解锁实测 | 本轮只测离屏，历史M1实窗60Hz结果不挪作本轮结果 |
| 四主题、系统字体、读屏名称和多行角色 | 待实窗/辅助技术复测 | 四张纯Go截图、语义树单测；不等于完整VoiceOver文本编辑支持 |
| 括号对参考线：`Guides.BracketPairs` 设为 `true` 与 `active`，在嵌套 `{}`/`()` 中移动光标，看活动对变色、水平线到括号；折叠一段、开换行后参考线不越过折叠、续行连续；输入未闭合 `{` 时不画 | 待解锁实测 | `TestBracketGuides*`、`TestViewDamageVerified/bracketGuides`、金样图 |
| 颜色占位：CSS 文件中 `#ff0000`、`rgb(...)` 前出现间隙与色块；点击间隙左/右半、左右键/上下键/Home 经过间隙，光标不进间隙；点色块打开选择器、改色、撤销；换行窄窗口下有颜色的行断行正确；在该行用中文输入法组字 | 待解锁实测（IME 部分待用户输入法） | `TestColorInjected*`、`TestColorSwatchPointer`、金样图 `view_vs*_colors.png` |
| 缩进检测：分别打开 Tab 缩进、2 空格、4 空格、混合缩进文件，看状态栏 `Spaces: N`/`Tab Size: N`、Tab 键与 Enter 的缩进；手动改 tab 大小后再编辑不被检测覆盖；命令面板执行 “Detect Indentation from Content” | 待解锁实测 | `TestGuessIndentationMonacoParity`、`TestDetectIndentation*`、demo `TestIndentStatus` |
| 小地图字符：2x 与 1x 屏、深浅主题下小地图显示字符形态而非色块，滚动/输入时增量更新无闪烁；`RenderCharacters=false` 回到色块；Fit/Fill 下点击各行跳转到对应行 | 待解锁实测 | `TestMinimapCharacters*`、`TestMinimapRowsCoverEveryViewRow`、`TestViewDamageVerified/minimapChars` |
| 长单行换行：`-wrap` 打开含 1 MB 单行（含 CJK、Tab）的文件，在行中连续输入、粘贴 64 KB、删除整段，观察每键无可感卡顿 | 待解锁实测 | `TestWrapIncremental*`、`BenchmarkTypeLongLineWrap`（第 4.6 节） |
| 稀有查询：20 MB 以下文件，选中只在文末出现一次的词，首帧不卡、文末匹配随后出现；扫描期间输入 | 待解锁实测 | `TestDocumentMatches*`、`TestOccurrencesFirstFrameBudget` |
| Unicode 区域表：系统语言为俄文/中文等时，西里尔/全角可疑字符的高亮与 Monaco 一致 | 待实机多语言环境 | `TestUnicodeLocaleTables`、`TestUnicodeSystemLocales` |
| Windows 实机：上述新增能力与性能 | 待 Windows 实机（阻塞里程碑 5） | 仅 `GOOS=windows go vet` |

## 7. 最终工程验证与证据

原始日志/备份/截图：`.superpowers/sdd/2026-11-13-editor-core-m2/task-15-evidence/`；执行记录：同级 `task-15-report.md`。本轮不执行任何git写操作，不新增依赖，不读取mygo，不覆盖既有M1或GPU验收记录。

- `go build ./...`、`go vet ./...`：darwin通过。
- `go build -tags tango_dev ./...`、`go vet -tags tango_dev ./ui/editor/... ./examples/editor-demo`：darwin通过。
- `GOOS=windows/linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`、`go vet ./ui/...`，以及两平台 `-tags tango_dev` 全量构建：均通过。交叉编译不是实机运行。
- demo单二进制使用 `go build -o <证据目录>/editor-demo`，`-help` 列出新增参数并正常退出，没有把可执行文件落到仓库根目录。
- 首次规定全量测试（包间并行）中，既有 `TestActiveGuideFirstCallFast` 测得25.253625ms，超过原有5ms上限，失败。其余包包括GPU/UI/demo通过；没有修改该测试或产品guide实现来变绿，初次失败完整保留在 `suite.log`。
- 该测试原门限下隔离10/10通过，1.851–3.210ms；仅说明隔离时未复现，不宣称已修复或已证明唯一原因。
- `go test -p 1 ./ui/editor/... ./ui/gpu/... ./ui/ ./examples/editor-demo -count=1` 串行完整复验全部通过（editor52.781s、GPU102.252s），退出0。
- 初轮（Fix round1生命周期修复前）`go test -p 1 ./... -count=1` **全仓通过**（editor53.299s、GPU102.365s），退出0，`repository-suite.log`保留完整结果。该记录不冒充Fix round1后的全仓重跑。
- 最终新增冒烟与demo测试再次通过，90个语言ID、四主题、a11y、目标分屏焦点和auto guard均通过；最新四张PNG已刷新并读取。
- 最终strict damage/导航专项再次全绿：`TestViewDamageVerified`26.93s、`TestViewDamageVerifiedTask14`8.56s、交互执行/真实partial/clip/scroll后点击组合通过，未放松任何断言。证据 `final-strict-navigation.log`。
- 格式与限定增量 `git diff --check`通过；无自建demo进程残留。本执行会话仅做作者自审，协调者复审意见及修复见下；用户禁止子代理，未派发新的独立复审，不冒称修复已独立复审通过。

### Fix round1：后台生命周期与缩进说明补正

复审指出的初始纯文本后切换语言缺少后台启动、退出未取消的问题已在demo本地修复：一个Model一个生命周期，订阅Language变化，首次出现lexer只启动一次，同一个内部Highlighter跨纯文本/Go/TS往返复用。ready不在后台读取Model/State；窗口晚附加invalidator时补发pending ready。退出先取消/退订/安全关闭种子channel，再关闭两侧State；已排队UI回调会检查生命周期是否关闭。

- 功能红测复现初始纯文本切换后5s仍无后台final/ready、未启动生命周期Close不取消；修复后测试直接观察后台ready和Done，不调用View/帧内分词来替代。
- 最新demo **10项测试通过**（0.912s）；包含真实共享split初始化、语言往返、先完成后附窗口、取消退订与关闭后不再处理reset、通知/关闭竞争及原面板/auto测试。
- `go test -race ./examples/editor-demo -count=5` **通过**（19.379s）；editor共享模型/每帧修饰/共享缩进/Enter辅助及语言主题a11y定向通过（editor1.930s）。
- darwin/arm64、windows/amd64、linux/amd64：demo+editor vet、demo production/dev `go build -o /dev/null`、gofmt/diff-check均通过，`fix1-final.log`保存退出码。
- 按复审范围没有重复性能套件；§4数据仍是初轮实测，性能夹具和编辑器实现本轮未改。缩进矩阵已补共享Model setters vs每帧ViewW.TabSize、detectIndentation缺失及Enter规范化既有tab的默认行为。

本报告中的最终通过声明以最终日志退出码为准；不存在用基准 PASS 自动代替功能/实窗验收的结论。

### 验收欠项计划 Task 10：最终检查（2026-11-15）

- `go test ./ui/editor/... -count=1`：全部通过（editor 107.1 s，负载约 5；Task 10 全部改动之后的复跑）。
- `go test -race ./ui/editor/... -count=1`：首次运行（17 分 18 秒）除 editor 包外全部通过，editor 包两项失败、无数据竞争报告：`TestLongLineBracketColorsDrawn`（单独 `-race` 也必然失败）与 `TestBracketGuidesIndexTwoTabSizes`（只在并行全量中失败）。原因见下，修正测试夹具后复跑 `go test -race ./ui/editor -count=1`（17 分 34 秒）**仍失败两项、无数据竞争报告**：`TestLongLineBracketColorsDrawn`（这次在被测视图一侧，step 0）与 `TestMinimapBlocksWhenOff`（录制的色块绘制命令在第 377 条起不同）。三项单独 `-race -count=3` 全部通过。归类：依赖墙钟帧预算（括号 2 ms 级预算、分词/小地图 3 ms 截止时间）的测试在 `-race` 减速加包内并行测试时不稳定，不是数据竞争；本轮未再修改（需要给这些测试注入时钟或在 `-race` 下放宽预算，超出“小修正”范围）。**`-race` 全量因此记为未全绿。**
- `TestViewDamageVerified` 单独 ×3：Task 10 修正前 14 次单独运行中 3 次失败（负载 3.3–5.9），修正后单独 ×3 通过（负载 3.1–4.3），12 个 `yes` 压满 CPU（负载约 40）下 ×3 通过。
- darwin、windows、linux 的 `go vet ./ui/editor/...`，darwin `go vet ./examples/editor-demo`，`gofmt -l ui/editor examples/editor-demo`：无输出；`go build -o /dev/null ./examples/editor-demo`：通过。
- `go run ./internal/cmd/apitxt -check`：`api/go/ui_editor.txt: +7 -0`（Task 8 新增的 `LocaleSet`、`AllowLocales`、`LocaleSetOf`、`SetSystemLocales`、`LocaleSet.IsDefault/Names`、`UnicodeHighlightOptions.AllowedLocales`），退出 1；按约定在提交时运行 `-w`，本轮未运行。

**`TestViewDamageVerified` blink 帧间歇失败的原因（Tasks 3/4/5/7 在改动前的树上都复现过）**：失败时 blink 帧除光标外多出的 damage 全部落在小地图矩形内（480×360 窗口里小地图为 x 348–468，例如 `{348 192 73 64}`；早先记录的 `{468 0 12 360}` 是紧邻的概览标尺列），各 scale 都可能出现，频率随 CPU 争用上升。机制：`prepareMinimap` 与帧的分词共享 3 ms 截止时间（`paint.go` 的 `hlDeadline`），超时后未缓存的小地图行留到下一帧，而且按设计不请求新帧（minimap.go 注释“A preview must not create a new frame loop”）；测试里输入/回车之后的下一帧正是第一个 blink 帧，于是该帧画出这些行。验证：同一脚本 15 次，压满 CPU 时开小地图 7/15 出现、关小地图 0/15；在 blink 之前加入不计数的“沉淀帧”后压满 CPU 也 0/15。damage 声明本身正确（严格校验每帧都通过），是测试对“blink 帧只有光标”的假设过严，不是编辑器缺陷；产品上的影响是 CPU 紧张时小地图最多滞后到下一个帧（通常是 500 ms 的光标闪烁帧）。

**`-race` 下两项失败的原因**：都是帧预算（墙钟）与 `-race` 的 10–20 倍减速相互作用。`TestLongLineBracketColorsDrawn` 的参照视图关闭长行索引、对 5.7 万字节的行整行扫描括号，`-race` 下超出帧括号预算，颜色晚一帧到，与被测视图（索引，一帧完成）像素不同；让参照视图多帧沉淀后 `-race` 通过（诊断副本 48 帧全部相同）。`TestBracketGuidesIndexTwoTabSizes` 的 5 帧预热在并行 `-race` 全量里不够画出参考线。两者都只改测试夹具，被测路径与断言不变。

**Task 10 改动的代码文件**（其余为测量与报告）：
- `ui/editor/perf_task10_test.go`（新，darwin，仅测量）：`BenchmarkScrollPartial`、`BenchmarkTypeAt1MLinesCPU`、`BenchmarkTypeTS20k`、`BenchmarkScrollTS20k`。
- `ui/editor/damage_test.go`：blink 断言前的沉淀帧循环（见上）。
- `ui/editor/longline_work_test.go`、`ui/editor/bracketguides_test.go`：上述 `-race` 夹具修正；`bracketguides_test.go` 另随计数器类型改一处读取。
- `ui/editor/bracketguides.go`：台账遗留 minor，测试计数器 `indentLinesRead` 改为 `atomic.Int64`（原为生产路径上无同步的包级 int 写入）。
- `ui/editor/viewstate.go`、`ui/editor/injected_test.go`：台账遗留 minor，`RestoreViewState` 清空 `ups`/`rights`，新增 `TestColorInjectedRightsMoves/restore`（修前失败、修后通过）。

## 8. Whole-plan final fix（2026-11-14）

本轮仅修复整体复审六项集成缺陷，不将 M2 改判为完整验收通过。新增回归集中于 `ui/editor/final_fix_test.go`；逐项修改、执行命令与结果见 `.superpowers/sdd/2026-11-13-editor-core-m2/final-fix-report.md`。

- **队列坐标**：排队操作和其输出选区共同重基，插入/替换文本内保留相对偏移；反向选区、多光标及 redo 精确恢复。`Model.DroppedEdits()` 可观察最终被拒绝的排队编辑。
- **通知事务**：源命令的 undo 分组、自动闭合范围、picker 字面量元数据完成后才执行观察者排队的变更；一个源命令后续多次编辑也进入映射日志。模型订阅者与 pool 观察者共用一个有序通知事务；`ModelChange.OldLanguage/NewLanguage` 与回调中的当前语言一致。
- **Picker 只读**：浮层仍在构建期创建，文档写入延迟至 finalized-frame `finish`，并在实际写入点再次检查 `ReadOnly`、版本、Model 与 ColorDecorators。只读帧丢弃未授权颜色变化，不创建 undo 记录。
- **语法刷新**：`Highlighter.GrammarGeneration()` 在每次 Reset 时递增（互斥锁保护），括号树/着色/匹配缓存随之失效，包括已缓存的“无匹配”。注册表覆盖语法后可执行 `model.SetLanguage(model.Language())`；lexer 未改变时仍无事件，改变时保留同一个 Highlighter、重置语法并发送同语言 Language 通知。也可直接 Reset(newLexer)。demo 的后台 once-start 生命周期保持不变。
- **语法命令预算**：trim/reindent 共享最多 **128 行、32 KiB UTF-8、4ms 启动预算**；每次原子 lexer 调用前检查。保留 tokenizer 的单次完整100ms正则尝试及确认重试（最坏约200ms+时钟粒度），一次 lexer 调用仍可能超出4ms，但之后不再逐行续领预算。降级 token 从不授权破坏性修改；达到 `monarch.MaxLineLength` 的行保守跳过（UTF-8 字节判断比其 UTF-16 门限更严格）。trim 没有待清理空白时只做 O(n) 文本扫描，不强制词法工作；分类不足时仅应用已证明安全的前缀，未知空白保留。reindent 任一所需行无法分类则整次无修改。`State.SyntaxCommandIncomplete()` 报告该情况。
- **明确代价**：当前使用预算内新分词而非缓存猜测 token；大语法文档可能只清理前缀或完全无法 reindent，且没有异步重试完成机制。该安全回退不是“任意规模命令完成”保证，也没有证明 ≤16ms UI 延迟。

原有逐显示能力 **≤10% 性能退化门槛**、100MB 内存 **≤3倍（待核口径/已有超标）**、缺失 Monaco 同场景基线、真实窗口/IME/平台验收仍未通过。单行 wrap 约49ms、稀有查询同步扫描、`Guides.BracketPairs` no-op、inline swatch 覆盖、detectIndentation/locales 缺失继续保留。（**2026-11-15 注**：本段是整体终审修复当时的状态；内存口径、Monaco 基线、逐能力成对测量、长行换行、稀有查询、括号对参考线、颜色间隙、缩进检测、区域表已由验收欠项计划 Tasks 1–10 处理，现状以第 2、4、5 节为准。）minimap token 色块、grab/release 两个 full 过渡帧、大文件 wrap-off 及显式测试 override、UTF-8/UTF-16 阈值差异均未被改动或豁免。

最终工程验证：`go test ./ui/editor/... -count=1` 全部通过（editor53.274s），demo测试0.863s通过；`go test -race -p 1 ./ui/editor/... ./examples/editor-demo -count=1` 全部通过（editor553.213s、demo6.270s）。strict damage/navigation专项、darwin全仓vet/build、windows/linux的UI+demo vet及全仓交叉构建、三平台prod/dev demo `-o /dev/null`、gofmt/diff-check均通过。此前出现过既有 `TestBracketCommandsTokenize` 和 `TestActiveGuideFirstCallFast` 的时限失败，原门限隔离及最终复验通过；完整经过和日志保留在 final-fix-report，未放宽门限，也不把其起因强行归为负载。


