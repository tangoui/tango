# 代码编辑器 里程碑 2（编辑核心）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把里程碑 1 的单光标性能原型做成完整的编辑核心：模型池与多视图、多光标、全部基础编辑操作、命令/快捷键/when 表达式、右键菜单、显示能力（折叠、自动换行、sticky scroll、小地图、概览标尺、括号着色、缩进参考线等），并接入 GPU 局部重绘提示，保持规格 6.6 的性能门槛。

**Architecture:** `ui/editor` 拆成"模型（Model，多视图共享）+ 视图状态（State，每个编辑器一份）+ 视图行映射（折叠与换行）+ 命令层"四层；所有编辑与光标操作都是注册在命令表里的命令，按键经 `editor/keymap` 解析（Monaco/VS Code 键位文本与 `when` 表达式，兼容 MinCode 预设），`editor/when` 求值上下文键。显示能力以装饰层的形式画在已有的 View 绘制顺序里。View 根据本帧变化向 `ui.Context` 声明 `Damage` / `ScrollShift`，由 GPU 渲染器局部重绘；开发构建的校验模式守住正确性。

**Tech Stack:** Go 1.27；已完成的 `ui/editor/{buffer,regex,langs,monarch,theme,highlight}`、`ui/text`（`ShapeRuns`、`LineCache`）、`ui`（即时模式、`Context.Damage/ScrollShift/EventsFor/CaptureTab/Overlay/ContextMenuFor`）、`ui/gpu`（局部重绘与校验模式）。

**Spec:** `docs/specs/code-editor.md`（第 4 节清单 1、2、11、12、13；6.2、6.3、6.5、6.6、6.7 里程碑 2、6.8）；里程碑 1 验收记录 `docs/specs/code-editor-m1-report.md`（"瓶颈与里程碑 2 的应对"一节）。

## Global Constraints

- 模块 `github.com/tangoui/tango`，Go 1.27；不新增外部依赖。
- 对齐的是 Monaco 0.56 的**概念、数据格式与默认行为**（命令 ID 用 Monaco 原名，如 `editor.action.copyLinesDownAction`、`cursorWordLeft`；选项默认值取 Monaco 0.56 `EditorOptions` 默认值；颜色默认值取 Monaco 0.56 颜色注册表在 vs / vs-dark / hc-black / hc-light 下的值）；Go API 按 Tango 即时模式风格（`ui/AUTHORING.md`）。不照搬 Monaco / mygo / monaco-vim 代码（Monaco 源码 MIT，可读以理解语义；不打开 `~/go/pkg/mod/github.com/egoist/mygo`）。
- 文本核心契约不变：`buffer.Snapshot` 不可变；偏移内部为 UTF-8 字节，对外另提供 UTF-16 换算；`Token.Start` 是 UTF-16 列、`ByteStart` 是字节；`monarch` 的 `Degraded` 行稍后重试。
- 性能门槛（规格 6.6，参考机 Apple Silicon、1920×1080 @2x，GPU 渲染）：100 万行文件输入按键到上屏 p95 ≤ 16 ms；连续滚动平均 ≥ 60 fps、p99 ≤ 33 ms；打开 100 MB 到可编辑 ≤ 1 秒；新增的每项显示能力在 100 万行文件上不得让这三项退化超过 10%（相对 M1 报告的数字）。
- 大文件降级：按 Monaco 0.56 `largeFileOptimizations` 的阈值与降级项（实现者从 Monaco 源码核对 `TextModel` 的大文件阈值，预期 20 MB 或 30 万行），**语法高亮保持开启**。
- 只做桌面；不做触控编辑；运行形态只支持纯自绘窗口（规格 6.8）。
- **文件归属：** 本计划只改 `ui/editor/**`、`examples/editor-demo/**`、`ui/damage.go`（Task 9 创建的文件，归本项目）、`ui/gpu/images.go` 与 `ui/gpu/renderer.go`（Task 1 列出的修复）、`ui/gpu/cpuexec/**`（Task 9）。不改 `ui/runner.go`、`ui/window.go`、`ui/text/**`、`ui/paint/**`、`uihost/**`、`internal/platform/**`；需要基础层新能力时，在任务报告里写清需求，由控制者转给对应会话（`ui/text`、`ui/paint` 归 min-55）。
- **不提交 git**（提交由会话 min-a6 统一做）。每个任务结束时 `go build ./... && go vet ./...` 在 darwin 通过，`GOOS=windows`、`GOOS=linux` 的 `go build ./... && go vet ./ui/...` 通过；`go test ./ui/editor/...` 全部通过；**Task 9 之后每个任务都必须保持 `TestViewDamageVerified` 通过**（新增的显示能力要么正确声明变化区域，要么在变化时不发提示）。
- 所有新选项放进 `editor.Options`（Task 2 定义，后续任务各自加字段），默认值等于 Monaco 0.56 默认值并在字段注释里写出 Monaco 的选项名。枚举型选项用带类型的字符串常量，取值就是 Monaco 的字符串（例：`type LineNumbersMode string`，`LineNumbersOn = "on"`、`LineNumbersRelative = "relative"`；`type WrapMode string`，`WrapOff = "off"`、`WrapOn = "on"`、`WrapColumn = "wordWrapColumn"`、`WrapBounded = "bounded"`），下文写作字符串的选项都按此规则定义类型。

## Review Focus

1. 全选同词后有上万个光标（`Cmd+Shift+L` 选中 1 万处）再输入：不卡死，单次输入 ≤ 50 ms，撤销一步全部恢复 —— Task 4 `TestManyCursorsTyping`、`BenchmarkTypeWith10kCursors`。
2. 同一文件开两个编辑器，A 在 B 的光标、选区与折叠所在处编辑或删除整段：B 的光标与折叠随之移动或合并，不 panic、不越界 —— Task 2 `TestTwoViewsShareModel`、Task 11 `TestFoldSurvivesRemoteEdit`。
3. 自动换行打开时，含 CJK、Tab、emoji、组合字符的超长行：上下键按视觉行移动、点击定位、选区矩形都正确 —— Task 10 `TestWrapMixedScripts`。
4. 输入法组字过程中按下与快捷键冲突的组合（如组字时按 `Cmd+D`）、两段式快捷键的第一段之后焦点移走：组字不被打断、不误触发命令，两段式状态被清除 —— Task 3 `TestIMEComposingSkipsKeymap`、`TestChordStateClearedOnBlur`。
5. 多光标 + 自动闭合 + 注释切换混合操作后撤销/重做：文本与全部光标精确恢复 —— Task 6 `TestTypingAssistUndoExact`。

---

### Task 1: 局部重绘的两项前置修复

**Files:**
- Modify: `ui/damage.go`（动画自动脏区：动画期间比较全部节点上一帧与本帧的包围盒，位移的节点新旧包围盒都并入）、`ui/gpu/images.go`、`ui/gpu/renderer.go`（局部帧里在屏但不在脏区、因此被裁掉的图片仍记为本帧使用，不被过期预算淘汰）
- Test: `ui/runner_damage_test.go`（新增用例）、`ui/gpu/images_test.go`、`ui/gpu/damage_test.go`

**Interfaces:**
- 不新增公开接口。`damage.go` 的比较只在本帧有动画推进（现有 `animating` 判定）时进行，复杂度 O(节点数)；矩形相等（逐字段）视为未动。
- [ ] **Step 1: 写失败测试** `TestRunnerAutoDamageSiblingShift`（一个节点高度由 `c.Anim` 从 20 变到 60，下方兄弟节点随之下移：本帧 Damage 覆盖兄弟节点的旧位置与新位置）、`TestRunnerAutoDamageSiblingShiftReduceMotion`（ReduceMotion 下动画直接跳到终点的那一帧同样覆盖）；`TestPartialFrameKeepsOnScreenImages`（整帧画 3 张图，接着局部帧只碰到第一张：图片缓存不释放另外两张，下一整帧无重新上传）。
- [ ] **Step 2: 运行** `go test ./ui/ -run 'Damage' ./ui/gpu/ -run 'Image|Partial'` —— FAIL。
- [ ] **Step 3: 实现**；改动前通知控制者（控制者会告知 min-55：`ui/damage.go` 的动画并入逻辑变化）。
- [ ] **Step 4: 运行** `go test ./ui/... ./ui/gpu/...`、`MTL_DEBUG_LAYER=1 go test -count=1 ./ui/gpu/metal/` —— PASS。

### Task 2: 模型、模型池、多视图与视图状态

**Files:**
- Create: `ui/editor/model.go`（重写：`Model`）、`ui/editor/pool.go`、`ui/editor/state.go`（视图状态，从现 `model.go` 迁出）、`ui/editor/options.go`、`ui/editor/viewstate.go`
- Modify: `ui/editor/view.go`、`ui/editor/input.go`、`ui/editor/paint.go`、`ui/editor/selection.go`、`ui/editor/viewport.go`、`examples/editor-demo/main.go`
- Test: `ui/editor/model_test.go`、`ui/editor/pool_test.go`

**Interfaces:**
- Produces:
  - `type Model struct{...}`：`func NewModel(uri, text string, reg *monarch.Registry) *Model`（按 URI 的文件名选语言）；方法 `URI() string`、`Buffer() *buffer.Buffer`、`Snapshot() buffer.Snapshot`、`Language() string`、`SetLanguage(id string)`、`Conf() *langs.Conf`、`Highlighter() *highlight.Highlighter`、`EOL() string`、`SetEOL(eol string)`（`"\n"` 或 `"\r\n"`，记一个撤销步）、`Subscribe(fn func(ModelChange)) (cancel func())`。
  - `type ModelChange struct{ Old, New buffer.Snapshot; Edits []buffer.Edit; Source *State /* 发起编辑的视图，撤销/重做时为执行它的视图 */; Undo, Redo, Language, EOL bool }`。
  - `type ModelPool struct{...}`：`func NewModelPool(reg *monarch.Registry) *ModelPool`；`Open(uri, text string) *Model`（已存在则引用计数 +1 并返回同一个，忽略 text）；`Get(uri string) *Model`；`Release(m *Model)`（计数归零时销毁）；`OnCreate(fn func(*Model)) (cancel func())`、`OnWillDispose(fn func(*Model)) (cancel func())`、`OnLanguageChange(fn func(m *Model, old string)) (cancel func())`（对应 Monaco `onDidCreateModel` / `onWillDisposeModel` / `onDidChangeModelLanguage`）。
  - `State`（每个编辑器一份）：`func NewView(m *Model, th *theme.Theme) *State`；保留 `NewState(text, filename string, th *theme.Theme) *State`（内部建私有 Model）；字段 `Model *Model`、`Theme *theme.Theme`、`Options Options`、`Viewport Viewport`；删除旧字段 `Buffer`、`Language`、`Conf`、`TabSize`、`InsertSpaces`（改为 `Options.TabSize`、`Options.InsertSpaces`，调用处一并改）。别的视图编辑同一模型时，本视图的选区按 `ModelChange.Edits` 映射（Monaco 语义：插入点处的光标随插入后移，被删除区间内的光标移到删除起点）。
  - `type Options struct{ FontSize float32; TabSize int; InsertSpaces bool; ReadOnly bool; LineNumbers LineNumbersMode }`（`LineNumbers` 本任务只支持 `"on"`/`"off"`，Task 12 补其余取值；里程碑 1 的 `ViewW.LineNumbers(on bool)` 等修饰改为写这些字段）；`func DefaultOptions() Options`。
  - `type ViewState struct{ TopLine int; OffsetY, ScrollX float32; Selections []Selection; Folded []int }`；`(*State).SaveViewState() ViewState`、`(*State).RestoreViewState(v ViewState)`（越界值夹在文档范围内）。
  - 撤销历史属于 Model（多视图共享）；撤销时光标恢复到执行撤销的视图（Monaco 语义）。
- [ ] **Step 1: 写失败测试** `TestPoolOpenShares`（同 URI 两次 Open 得同一 Model，Release 两次后 OnWillDispose 触发一次）、`TestPoolLanguageEvents`、`TestTwoViewsShareModel`（视图 A 在视图 B 的光标前插入一行：B 的光标行号 +1；A 删除包含 B 选区的整段：B 的选区折叠到删除起点；B 撤销：文本恢复、B 的光标恢复到 B 撤销前记录的选区）、`TestViewStateRoundTrip`、`TestRestoreViewStateClamps`、`TestSetEOLUndo`；里程碑 1 的全部测试改用新字段后仍通过。
- [ ] **Step 2: 运行** `go test ./ui/editor/...` —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `go test ./ui/editor/...`、`go build ./examples/editor-demo/` —— PASS。

### Task 3: when 表达式、键位解析与命令表

**Files:**
- Create: `ui/editor/when/when.go`、`ui/editor/keymap/keymap.go`、`ui/editor/keymap/defaults.go`、`ui/editor/command.go`、`ui/editor/contextkeys.go`
- Modify: `ui/editor/input.go`（固定键表改为经 keymap 分派；里程碑 1 的每个键位改成注册的命令）
- Test: `ui/editor/when/when_test.go`、`ui/editor/keymap/keymap_test.go`、`ui/editor/command_test.go`

**Interfaces:**
- Produces（`when`）：`func Parse(expr string) (Expr, error)`；`type Expr interface{ Eval(ctx Context) bool; String() string }`；`type Context interface{ Value(key string) (any, bool) }`；`type MapContext map[string]any`。语法与 VS Code 一致：`!`、`&&`、`||`、`==`、`!=`、`===`、`!==`、`<`、`<=`、`>`、`>=`、`=~ /re/flags`（经 `editor/regex` 编译）、`in`、`not in`、括号、`true`/`false`、单引号字符串、裸词（未定义键为假）。兼容性以 MinCode `vue-app/src/domain/commands/when.ts` 及其测试为准：把其测试用例（表达式、上下文、期望值）转写为 Go 表驱动测试。
- Produces（`keymap`）：`type Platform int`（`Mac`、`Windows`、`Linux`）；`func ParseKey(text string, p Platform) (Sequence, error)`——接受 MinCode / VS Code 键位文本：`mod`（mac 为 ⌘、其余为 Ctrl）、`cmd`/`meta`/`win`、`ctrl`、`shift`、`alt`/`option`，按键名为 VS Code 名（`up`、`pagedown`、`/`、`[`、`f12`、`numpad0`…），以空格分隔的两段式（最多 2 段），以及双击修饰键（`shift shift`）；按物理键匹配（`input.KeyEvent.Code`），规则同 MinCode `domain/keybindings/keys.ts`（把其测试用例转写为 Go 测试）。`type Rule struct{ Key, Command, When string; Args any }`（`Command` 以 `-` 开头表示移除同键同命令的已有规则，VS Code 语义）；`type Keymap struct{...}`；`func New(p Platform, rules ...[]Rule) *Keymap`（后面的规则集优先）；`(*Keymap).Resolve(ev input.KeyEvent, ctx when.Context) (cmd string, args any, pending bool)`（两段式第一段返回 `pending=true`；`ClearPending()`）；`func DefaultRules(p Platform) []Rule`——Monaco 0.56 对本计划已实现命令的默认键位（以 MinCode `domain/keybindings/monaco-defaults.ts` 为对照表，未实现的命令不列入）。
- Produces（命令）：`type Command struct{ ID, Label string; Precondition string /* when */; Mutates bool /* 修改文本，只读时拒绝 */; Run func(st *State, args any) bool }`；`func RegisterCommand(cmd Command)`（全局默认表）；`func Commands() []Command`（按 ID 排序，供命令面板）；`(*State).Execute(id string, args any) bool`；`(*ViewW).Keymap(km *keymap.Keymap)`、`(*ViewW).Context(fn func(key string) (any, bool))`（宿主追加上下文键）。内置上下文键：`editorFocus`、`editorTextFocus`、`textInputFocus`、`editorReadonly`、`editorHasSelection`、`editorHasMultipleSelections`、`editorLangId`、`editorTabMovesFocus`、`isMac`/`isWindows`/`isLinux`。
- IME 组字期间不解析快捷键（按键交给输入法）；失焦或窗口失活时清除两段式状态。
- [ ] **Step 1: 写失败测试** `TestWhenParseEval`（转写的 MinCode 用例 + VS Code 优先级：`!a && b || c` 等）、`TestWhenRegex`、`TestParseKeyMinCodeCompat`（转写的 MinCode 用例，含 `mod+k mod+c`、`shift shift`、mac 下 ⌥ 字母按物理键）、`TestKeymapRemovalRule`、`TestKeymapWhenPriority`（同键两条规则按 when 选择，后加的优先）、`TestChordAndTimeoutFree`（两段式：第一段 pending，第二段命中；中间按其他键清除）、`TestChordStateClearedOnBlur`、`TestIMEComposingSkipsKeymap`、`TestM1KeysStillWork`（里程碑 1 `TestKeyboardNavigation` 全部表项经新系统仍通过）、`TestCommandsListed`（每个注册命令有 Label，ID 唯一）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `go test ./ui/editor/...` —— PASS。

### Task 4: 多光标与列选择

**Files:**
- Create: `ui/editor/cursors.go`
- Modify: `ui/editor/state.go`、`ui/editor/selection.go`、`ui/editor/input.go`、`ui/editor/paint.go`、`ui/editor/ime.go`
- Test: `ui/editor/cursors_test.go`、`ui/editor/bench_cursors_test.go`

**Interfaces:**
- Produces: `State.Sels []Selection`（`Sels[0]` 为主光标，与 Monaco `getSelections()[0]` 一致；删除 `State.Sel`，调用处改为 `Primary()`）；`(*State).Primary() Selection`、`SetSelections(sels []Selection)`（排序、合并重叠/相接的选区，Monaco 规则）。命令（Monaco ID）：`editor.action.insertCursorAbove`/`Below`、`editor.action.addSelectionToNextFindMatch`（Cmd+D）、`editor.action.selectHighlights`（Cmd+Shift+L）、`editor.action.insertCursorAtEndOfEachLineSelected`、`removeSecondaryCursors`（Esc）、`cursorUndo`/`cursorRedo`、`cursorColumnSelectUp/Down/Left/Right`（Shift+Alt+方向，列选择）；鼠标：Alt+单击加光标、Alt+Shift+拖动列选择（Monaco 默认 `multiCursorModifier: "alt"`）。所有编辑与移动命令对每个光标生效；编辑一次应用为一个 `buffer.Edit` 集合（一个撤销步，光标全部记入历史）；IME 提交文本插入到每个光标；绘制每个光标与选区。
- [ ] **Step 1: 写失败测试** `TestMultiCursorTyping`、`TestSelectionsMergeOverlap`、`TestAddNextOccurrence`（含大小写、全词规则同 Monaco：无选区时先选词）、`TestSelectHighlights`、`TestColumnSelectMixedWidths`（Tab 与 CJK 行上的列选择按视觉列对齐）、`TestCursorUndo`、`TestManyCursorsTyping`（1 万个光标插入一个字符 ≤ 50 ms，撤销一步恢复）、`BenchmarkTypeWith10kCursors`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；记录基准。

### Task 5: 行操作、缩进与注释

**Files:**
- Create: `ui/editor/lineops.go`、`ui/editor/indent.go`、`ui/editor/comment.go`
- Test: `ui/editor/lineops_test.go`、`ui/editor/indent_test.go`、`ui/editor/comment_test.go`

**Interfaces:**
- Produces 命令（Monaco ID，多光标下每个光标所在行去重后生效，一个撤销步）：`editor.action.copyLinesUpAction`/`DownAction`、`editor.action.moveLinesUpAction`/`DownAction`、`editor.action.deleteLines`、`editor.action.joinLines`、`editor.action.insertLineBefore`/`After`、`editor.action.duplicateSelection`、`editor.action.transpose`、`editor.action.sortLinesAscending`/`Descending`、`editor.action.trimTrailingWhitespace`、`editor.action.indentLines`/`outdentLines`、`tab`/`outdent`（有选区时整行缩进，无选区时插入/删除到下一个制表位；Shift+Tab 反缩进，补上里程碑 1 缺的行为）、`editor.action.reindentlines`（按 `Conf.Indentation` 规则）、`editor.action.indentationToSpaces`/`ToTabs`、`editor.action.commentLine`（切换行注释）、`editor.action.addCommentLine`/`removeCommentLine`、`editor.action.blockComment`（`Conf.LineComment` 为空时 `commentLine` 退化为块注释，Monaco 规则）。
- [ ] **Step 1: 写失败测试** 每个命令的表驱动用例（输入文本 + 光标 → 期望文本 + 光标；用例取 Monaco 对应单元测试的语义，至少包括：移动行到文件首尾的边界、空行与只有空白的行的注释切换、混合缩进的反缩进、CRLF 文件的复制行）、`TestCommentToggleLanguages`（go、python、html 三种注释风格）、`TestLineOpsMultiCursorOneUndo`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 6: 输入辅助（自动闭合、包裹、Enter 规则、驼峰分词移动）

**Files:**
- Create: `ui/editor/autoclose.go`、`ui/editor/onenter.go`、`ui/editor/wordpart.go`
- Modify: `ui/editor/langs/langs.go`（`Conf` 新增 `OnEnterRules []OnEnterRule`、`AutoCloseBefore string`、`ColorizedBracketPairs [][2]string`，从 `ConfRaw` 解码；不改生成的数据）
- Test: `ui/editor/autoclose_test.go`、`ui/editor/onenter_test.go`、`ui/editor/wordpart_test.go`、`ui/editor/langs/langs_test.go`

**Interfaces:**
- Produces（langs）：`type OnEnterRule struct{ BeforeText, AfterText, PreviousLineText *RegexSpec; Action EnterAction }`、`type EnterAction struct{ Indent string /* "none"|"indent"|"indentOutdent"|"outdent" */; AppendText string; RemoveText int }`。
- Produces（editor）：选项 `Options.AutoClosingBrackets`、`AutoClosingQuotes`（`"always"|"languageDefined"|"beforeWhitespace"|"never"`，默认 `languageDefined`）、`AutoClosingOvertype`（默认 `"auto"`）、`AutoClosingDelete`（默认 `"auto"`）、`AutoSurround`（默认 `"languageDefined"`）、`AutoIndent`（默认 `"full"`）；行为按 Monaco 0.56：在 `AutoCloseBefore` 字符前才自动闭合、字符串/注释 token 内按 `Pair.NotIn` 不闭合、输入闭合符时越过自动插入的闭合符、退格成对删除、有选区时输入开符号包裹选区；Enter 依次应用 `OnEnterRules` → 缩进规则 → 保持上一行缩进（`AutoIndent:"full"`）。命令 `cursorWordPartLeft`/`Right`（及 Select 变体，驼峰与下划线分词）、`deleteWordPartLeft`/`Right`、`cursorWordStartLeft`/`EndRight` 等 Monaco 词移动族。
- [ ] **Step 1: 写失败测试** `TestDecodeOnEnterRules`（typescript 的 JSDoc 规则解码正确；全部 85 种语言 Conf 仍能解码）、`TestAutoCloseRules`（表驱动：`(` 在单词前不闭合、字符串内 `'` 不闭合、overtype、成对删除、包裹）、`TestOnEnterJSDoc`（`/**` 后回车生成 ` * `）、`TestOnEnterIndentRules`（python `:` 结尾缩进、go `}` 反缩进）、`TestWordPartMovement`、`TestTypingAssistUndoExact`（多光标下自动闭合 + 注释切换 + Enter 后逐步撤销与重做，文本与全部光标精确恢复）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 7: 剪贴板、拖放与只读

**Files:**
- Create: `ui/editor/clipboard.go`、`ui/editor/dnd.go`
- Modify: `ui/editor/input.go`
- Test: `ui/editor/clipboard_test.go`、`ui/editor/dnd_test.go`

**Interfaces:**
- Produces 命令：`editor.action.clipboardCopyAction`/`CutAction`/`PasteAction`——无选区时复制/剪切整行（`emptySelectionClipboard: true`），粘贴整行内容插入到行首；多光标复制按光标顺序以换行连接，粘贴时若剪贴板行数等于光标数则逐个分发（Monaco `multiCursorPaste: "spread"`）；记住"整行复制"元数据只在本进程内有效（同一剪贴板文本）。拖放：在选区内按下并拖动移动文本（Alt/Option 按住为复制，Monaco `dragAndDrop: true`），拖动中显示插入点；从外部拖入文件用 `ui.DropZone` 等价能力时只插入路径文本——不做（Monaco 默认不插入文件内容），在报告里写明。只读模式：所有修改类命令（命令元数据 `Mutates bool`）被拒绝，`editorReadonly` 为真，粘贴/剪切/拖放移动无效但复制与导航可用。
- [ ] **Step 1: 写失败测试** `TestCopyCutEmptySelectionLine`、`TestMultiCursorPasteSpread`、`TestDragMoveText`、`TestDragCopyWithAlt`、`TestReadOnlyRejectsAllMutations`（遍历 `Commands()` 中 `Mutates` 为真的命令，文本不变）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**（核对 Task 3–6 注册的命令的 `Mutates` 标注是否正确）。
- [ ] **Step 4: 运行** —— PASS。

### Task 8: 右键菜单

**Files:**
- Create: `ui/editor/menu.go`
- Modify: `ui/editor/view.go`
- Test: `ui/editor/menu_test.go`

**Interfaces:**
- Produces: `type MenuItem struct{ Command, Label string; Group string; Order float64; When string; Args any; Submenu []MenuItem }`；`func RegisterMenuItem(menu string, item MenuItem)`（全局，`menu` 为 `"EditorContext"`，对应 Monaco `MenuId.EditorContext`）；`(*ViewW).MenuItems(items ...MenuItem)`（宿主/插件贡献项，按视图追加）。内置项（Monaco 0.56 编辑器右键菜单中本计划已实现的命令）：剪切、复制、粘贴（组 `9_cutcopypaste`）、全选、切换行注释（组 `1_modification`）、命令面板项只在宿主注册了 `editor.action.quickCommand` 时显示。分组按名字排序、组间分隔线、组内按 `Order`；`When` 用 Task 3 的上下文求值；标签旁显示当前键位（来自视图的 Keymap）。在 `ui.ContextMenuFor(id)` 位置用 `ui` 现有的菜单/下拉组件绘制（不新写浮层机制）；Esc 或点外部关闭；键盘可导航（沿用组件能力）。
- [ ] **Step 1: 写失败测试** `TestContextMenuItemsFiltered`（when 为假的项不出现、空组不出分隔线）、`TestContextMenuOrder`、`TestContextMenuRunsCommand`（无头 Driver 右键 → 点"复制" → 剪贴板内容正确）、`TestContextMenuShowsKeybinding`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 9: 局部重绘提示接入与校验测试

**Files:**
- Create: `ui/editor/damage.go`、`ui/gpu/cpuexec/device.go`（把 `ui/gpu` 测试里的 `cpuDevice` 提升为导出的 `cpuexec.Device`：实现 `gpu.Device`、`gpu.Retainer`、`gpu.PixelReader`，支持 `Dirty`/`Copies`；`ui/gpu` 的测试改用它）
- Modify: `ui/editor/view.go`、`ui/editor/paint.go`、`ui/editor/perf_test.go`（更新过时注释，加局部重绘场景）
- Test: `ui/editor/damage_test.go`、`ui/gpu/cpuexec/device_test.go`

**Interfaces:**
- Produces（cpuexec）：`func NewDevice() *Device`；`(*Device).Execute(f *gpu.Frame) error`、`Retained(size image.Point) bool`、`ReadPixels() (*image.RGBA, error)`。
- View 的声明规则（规格 6.2，M1 报告"瓶颈"第 2 条）：只有光标闪烁 → `c.Damage(光标矩形 ∪ 上一帧光标矩形)`；编辑 → 变化行（按 `ModelChange` 的行范围，换行数变化时到可见区底部）的整行矩形 + 对应行号区；纵向滚动 → 在文本区与行号区内容节点的 Body 内 `c.ScrollShift(区域, 位移)`，新露出的行由渲染器补画；选区或多光标变化 → 新旧选区所在行；任何无法确定的变化（主题、字号、选项改变、尺寸变化、折叠/换行映射改变、IME 候选区变化）→ 本帧不发提示（整帧）。后续任务新增的显示元素必须加入这些规则或明确"变化时不发提示"。
- [ ] **Step 1: 写失败测试** `TestDeviceMatchesGPUTests`（cpuexec.Device 跑 `ui/gpu` 现有局部帧测试）；`TestViewDamageVerified`：`gpu.NewRenderer(cpuexec.NewDevice())`、校验模式严格（`SetVerify(true)`、`SetVerifyStrict(true)`），用无头 Driver 跑脚本——输入 20 个字符、退格、回车、光标闪烁 3 次、滚轮滚动 10 次（含 1.25 倍与 1.5 倍缩放、小数偏移）、Shift 选择、多光标输入、IME 组字与提交——每帧 `Render` 无错误，且至少 70% 的帧是局部帧（Frame.Dirty 非空）；`TestViewNoHintOnThemeChange`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；`perf_test.go` 加 `BenchmarkTypeAt1MLinesPartial`（Metal，校验关闭），报告按键到帧完成的 p50/p95 与 M1 对比。

### Task 10: 视图行映射与自动换行

**Files:**
- Create: `ui/editor/viewlines.go`（模型行 ↔ 视图行映射：折叠隐藏 + 换行分段，Task 11 接入折叠）、`ui/editor/wrap.go`
- Modify: `ui/editor/selection.go`（上下移动按视图行，保持目标视觉列）、`ui/editor/viewport.go`（`TopLine` 改为视图行）、`ui/editor/paint.go`、`ui/editor/input.go`（命中测试）、`ui/editor/damage.go`
- Test: `ui/editor/wrap_test.go`、`ui/editor/viewlines_test.go`

**Interfaces:**
- Produces: 选项 `Options.WordWrap`（`WrapOff`|`WrapOn`|`WrapColumn`|`WrapBounded`，默认 `WrapOff`）、`WordWrapColumn int`（默认 80）、`WrappingIndent`（`"none"|"same"|"indent"|"deepIndent"`，默认 `"same"`）；换行断点按 Monaco 规则（空白后、CJK 字符之间、`wordWrapBreakBeforeCharacters`/`AfterCharacters` 的 Monaco 默认集合）；`type viewLines` 提供 `ModelToView(line, col int) (vline int, x float32)`、`ViewToModel(vline int, x float32) (line, col int)`、`Count() int`；视图宽度变化只重算可见区附近，远处按需惰性计算（100 万行文件开启换行后打开 ≤ 1 秒）；`editor.action.toggleWordWrap`（Alt+Z）。
- [ ] **Step 1: 写失败测试** `TestWrapBreakRules`（英文按空白、CJK 逐字、URL 中的 `/` 后可断）、`TestWrapMixedScripts`（CJK/Tab/emoji/组合字符超长行：上下键按视觉行、`Home`/`End` 先到视觉行首尾再到逻辑行首尾（Monaco 行为）、点击定位与光标矩形一致）、`TestWrapIndentModes`、`TestWrapResizeLazy`（100 万行 + 换行：首帧只计算可见区附近）、`TestViewDamageVerified` 加开启换行的脚本。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；`BenchmarkScroll` 开启换行时与 M1 对比。

### Task 11: 折叠

**Files:**
- Create: `ui/editor/folding.go`、`ui/editor/folding_provider.go`
- Modify: `ui/editor/viewlines.go`、`ui/editor/gutter.go`（折叠控件）、`ui/editor/paint.go`（折叠行末尾的 `…` 占位）、`ui/editor/viewstate.go`
- Test: `ui/editor/folding_test.go`

**Interfaces:**
- Produces: `type FoldingRange struct{ Start, End int /* 模型行，0 起，含 */; Kind string /* "comment"|"imports"|"region"|"" */ }`；`type FoldingRangeProvider interface{ FoldingRanges(ctx context.Context, snap buffer.Snapshot) ([]FoldingRange, error) }`（里程碑 3 接 LSP；本任务提供接口与注册 `(*ViewW).FoldingProvider(p)`）；内置策略：按缩进（`Conf.Folding.OffSide` 语义同 Monaco）+ `Conf.Folding.Start/End` 标记区域；选项 `Options.Folding`（默认 true）、`FoldingStrategy`（`"auto"|"indentation"`）、`ShowFoldingControls`（`"mouseover"|"always"|"never"`，默认 `"mouseover"`）。命令：`editor.fold`、`editor.unfold`、`editor.toggleFold`、`editor.foldAll`、`editor.unfoldAll`、`editor.foldLevel1`…`7`、`editor.foldRecursively`、`editor.foldAllBlockComments`、`editor.foldAllMarkerRegions`。折叠状态以锚点保存，编辑后随之移动；光标落进折叠区（键盘或编辑）时自动展开该区（Monaco 行为）；删除折叠起始行时该折叠消失；大文件按 Monaco 降级规则关闭折叠。
- [ ] **Step 1: 写失败测试** `TestIndentFoldingRanges`（python/yaml off-side，go 大括号）、`TestMarkerFolding`（`// #region`）、`TestFoldCommands`、`TestCaretEntersFoldUnfolds`、`TestFoldSurvivesRemoteEdit`（另一视图在折叠区上方插入 10 行、在折叠区内删除、删除折叠起始行）、`TestFoldViewState`、`TestFoldingDisabledForLargeFile`、`TestViewDamageVerified` 加折叠/展开（预期整帧）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 12: 显示装饰（一）——行号、空白、缩进参考线、括号与默认颜色

**Files:**
- Create: `ui/editor/theme/colors_gen.go`（生成物）、`ui/editor/langs/gen/export_colors.mjs`（生成脚本，加进 `langs/gen/README.md`）、`ui/editor/brackets.go`、`ui/editor/guides.go`、`ui/editor/whitespace.go`
- Modify: `ui/editor/theme/theme.go`（`Color` 未定义的键回退到所在基础主题的默认颜色表）、`ui/editor/paint.go`（删除手写的 `monacoDefaults`，改用主题回退）、`ui/editor/gutter.go`、`ui/editor/damage.go`
- Test: `ui/editor/theme/colors_test.go`、`ui/editor/brackets_test.go`、`ui/editor/display_test.go`

**Interfaces:**
- 默认颜色：脚本用 Node 加载 `monaco-editor@0.56.0` 的颜色注册表（必要时用最小 DOM 垫片），对 vs / vs-dark / hc-black / hc-light 逐键解析出最终颜色（含 `transparent()`、`lighten()` 等派生与引用），写成 `map[string][4]color.NRGBA`；无法解析的键让脚本以退出码 1 结束并列出键名。`theme.Color(key)` 先查主题 `colors`，再查默认表。
- Produces 选项（Monaco 名与默认值）：`LineNumbers`（`"on"|"off"|"relative"|"interval"`，默认 `"on"`）、`RenderWhitespace`（`"none"|"boundary"|"selection"|"trailing"|"all"`，默认 `"selection"`）、`RenderControlCharacters`（默认 true）、`Guides.Indentation`（默认 true）、`Guides.HighlightActiveIndentation`（默认 true）、`Guides.BracketPairs`（默认 false）、`BracketPairColorization`（默认 true）、`MatchBrackets`（`"always"|"near"|"never"`，默认 `"always"`）、`RenderLineHighlight`（`"none"|"gutter"|"line"|"all"`，默认 `"line"`）、`Padding{Top, Bottom float32}`（默认 0）。括号配对着色：括号对取 `Conf.ColorizedBracketPairs`，为空时取 `Conf.Brackets`；字符串与注释 token 内的括号不计；嵌套层按 `editorBracketHighlight.foreground1…6` 循环；不匹配的用 `editorBracketHighlight.unexpectedBracket.foreground`；每行行首的括号栈深度增量计算（编辑只重算到深度稳定为止，同高亮管线的提前停止思路）。命令 `editor.action.jumpToBracket`、`editor.action.selectToBracket`。
- [ ] **Step 1: 写失败测试** `TestDefaultColorsResolved`（vs-dark 的 `editorIndentGuide.background1`、`editorBracketHighlight.foreground1`、`editorWhitespace.foreground` 等与 Monaco 一致——期望值从生成表读出并抽查 5 个与脚本日志一致）、`TestBracketPairColors`（嵌套 7 层循环取色；字符串内括号不着色；不匹配括号）、`TestBracketDepthIncremental`（100 万行在第 10 行插入 `{`：重算行数受限且结果等于全量计算）、`TestMatchBrackets`、`TestRelativeLineNumbers`、`TestRenderWhitespaceModes`、`TestIndentGuidesActive`、`TestViewGolden` 新增两张黄金图（vs、vs-dark，开启全部装饰；重录前 Read 新旧图并描述差异）、`TestViewDamageVerified` 加括号匹配高亮变化。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**；运行生成脚本。
- [ ] **Step 4: 运行** —— PASS；100 万行性能复测（Global Constraints 的 10% 门槛）。

### Task 13: 显示装饰（二）——同词高亮、可疑字符、链接、颜色、滚轮缩放、大文件降级

**Files:**
- Create: `ui/editor/occurrences.go`、`ui/editor/unicode.go`、`ui/editor/links.go`、`ui/editor/colors.go`、`ui/editor/largefile.go`
- Modify: `ui/editor/view.go`、`ui/editor/paint.go`、`ui/editor/input.go`、`ui/editor/damage.go`
- Test: `ui/editor/decorations_test.go`

**Interfaces:**
- Produces 选项（Monaco 名与默认值）：`OccurrencesHighlight`（默认 `"singleFile"`，按光标所在词的全词匹配，防抖 250 ms，Monaco `wordHighlighter` 行为；本里程碑只做文本匹配，LSP 的 documentHighlight 在里程碑 3 接）、`SelectionHighlight`（默认 true）、`UnicodeHighlight.AmbiguousCharacters`（默认 true）、`InvisibleCharacters`（默认 true）、`NonBasicASCII`（默认 `"inUntrustedWorkspace"`，本项目视为 false）——可疑字符表从 Monaco 0.56 源码导出（`ambiguousCharacters` 数据，加入 Task 12 的脚本，生成 `ui/editor/unicode_gen.go`）；`Links`（默认 true，Monaco `LinkComputer` 规则的 URL 识别，Cmd/Ctrl+单击经 `(*ViewW).OnOpenLink(fn func(url string))` 交给宿主）；`ColorDecorators`（默认 true，识别 CSS 风格颜色字面量 `#rgb`/`#rrggbb`/`#rrggbbaa`/`rgb()`/`rgba()`/`hsl()`/`hsla()`，行内画色块；点击色块用 `ui.ColorPickerPanel` 浮层修改并写回文本，一个撤销步；颜色 provider 接口留给里程碑 3）；`MouseWheelZoom`（默认 false；开启后 Cmd/Ctrl+滚轮调整 `FontSize`，命令 `editor.action.fontZoomIn/Out/Reset`）；大文件降级：超过 Global Constraints 所述的大文件阈值时关闭折叠、小地图、括号配对着色、同词高亮、链接检测、颜色装饰、可疑字符高亮（以 Monaco 0.56 实际降级项为准，实现者核对源码并在报告列出），高亮保持开启；`(*State).LargeFile() bool`。
- [ ] **Step 1: 写失败测试** `TestOccurrencesDebounced`、`TestUnicodeAmbiguousHighlight`（西里尔 `а` 在英文行中被标出）、`TestInvisibleCharacters`、`TestLinkDetect`（URL 末尾标点规则同 Monaco）、`TestColorDecoratorEdit`（选择新颜色写回文本，撤销一步恢复）、`TestWheelZoom`、`TestLargeFileDegrades`（列出的每项被关闭、高亮仍在）、`TestViewDamageVerified` 加同词高亮出现与消失。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；100 万行性能复测。

### Task 14: sticky scroll、小地图与概览标尺

**Files:**
- Create: `ui/editor/sticky.go`、`ui/editor/minimap.go`、`ui/editor/overview.go`
- Modify: `ui/editor/view.go`、`ui/editor/paint.go`、`ui/editor/input.go`、`ui/editor/damage.go`
- Test: `ui/editor/sticky_test.go`、`ui/editor/minimap_test.go`、`ui/editor/overview_test.go`

**Interfaces:**
- Produces 选项（Monaco 名与默认值）：`StickyScroll.Enabled`（默认 true）、`StickyScroll.MaxLineCount`（默认 5），作用域来自折叠范围（Task 11 的内置策略 + provider），点击 sticky 行跳到该行；`Minimap.Enabled`（默认 true）、`Minimap.Side`（默认 `"right"`）、`Minimap.Size`（默认 `"proportional"`）、`Minimap.MaxColumn`（默认 120）、`Minimap.RenderCharacters`（默认 true；**本里程碑按色块绘制**——每个 token 一段该 token 前景色的矩形，`RenderCharacters` 为 true 时同样按色块画并在报告记为与 Monaco 的已知差异）、`Minimap.ShowSlider`（默认 `"mouseover"`），点击与拖动滑块滚动；小地图按行缓存（只重画变化行）；`OverviewRuler`（`OverviewRulerLanes` 默认 3，`OverviewRulerBorder` 默认 true）：显示光标位置、选区与 `(*State).SetOverviewMarks(owner string, marks []OverviewMark)` 设置的标记（`type OverviewMark struct{ Line int; Color color.NRGBA; Lane int /* 1 左 2 中 4 右 */ }`，按 owner 覆盖，供查找结果与诊断在后续里程碑使用），点击跳转。大文件降级时关闭小地图。
- [ ] **Step 1: 写失败测试** `TestStickyScrollHeaders`（嵌套函数中滚动：显示外层到内层的作用域起始行，最多 5 行；点击跳转）、`TestMinimapColors`（色块颜色等于 token 前景色）、`TestMinimapDragScroll`、`TestMinimapIncremental`（编辑一行只重画该行的小地图缓存）、`TestOverviewMarks`（按 owner 覆盖、点击跳转）、`TestViewGolden` 更新、`TestViewDamageVerified` 加小地图与 sticky 区域的变化。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS；100 万行性能复测。

### Task 15: 演示程序更新与里程碑 2 验收

**Files:**
- Modify: `examples/editor-demo/main.go`（两个编辑器分屏共享模型的 `-split` 开关、`-wrap`、语言切换菜单、右键菜单与命令面板（列出 `Commands()`））
- Create: `docs/specs/code-editor-m2-report.md`
- Test: `ui/editor/langs_smoke_test.go`、`ui/editor/perf_test.go`

- [ ] **Step 1: 写测试** `TestAllLanguagesInView`（85 种语言注册各取一段 Monaco 用例文本，在 View 中打开、输入、撤销、开启全部装饰，不 panic、无降级之外的错误）、`TestAllBaseThemesInView`（4 个基础主题各一张截图，不出现未定义颜色导致的透明/零值）。
- [ ] **Step 2: 运行基准**（负载 `uptime` < 6；否则记录负载；校验关闭）：规格 6.6 全部场景（打开 100 MB、100 万行输入 p50/p95、滚动 fps/p99、TS 2 万行后台高亮、内存），开启全部默认显示能力，与 M1 报告对照；局部重绘单行编辑帧时间。
- [ ] **Step 3: 写报告**：清单 1、2、11、12、13 逐项对照表（能力 → 命令/选项 → 测试名 → 与 Monaco 的已知差异）、性能对照、降级项、留给里程碑 3 的接口（FoldingRangeProvider、颜色 provider、documentHighlight、OverviewMark）、实机检查清单（分屏同步、右键菜单、中文输入法 + 多光标、拖放、换行 + 折叠组合）。
- [ ] **Step 4: 运行** `go build ./... && go vet ./...`、windows/linux 交叉编译、`go test ./ui/editor/... ./ui/gpu/... ./ui/` —— 通过。
