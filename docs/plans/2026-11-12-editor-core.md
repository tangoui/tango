# 代码编辑器 文本核心（buffer / regex / monarch / langs / theme）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 做出编辑器与渲染无关的底层：rope 文本模型（快照、撤销、锚点）、JS 兼容的混合正则、Monarch 解释器、Monaco 内置语言与基础主题数据，全部用 Monaco 原版测试数据验证。

**Architecture:** `ui/editor/buffer`（持久化 rope + 快照 + 历史）独立；`ui/editor/regex` 把 JS 正则翻译为 Go `regexp`，翻译不了的交给 regexp2；`ui/editor/monarch` 用 regex 编译并执行 Monarch 语法，逐行分词、状态驻留；`ui/editor/langs` 嵌入由 Node 脚本从 `monaco-editor@0.56.0` 导出的语法、语言配置与 Monaco 官方分词测试用例；`ui/editor/theme` 解析 Monaco 主题并把 token 解析为样式。这五个包都不依赖 `ui`。

**Tech Stack:** Go 1.27、`github.com/dlclark/regexp2`（新依赖，MIT）、Node（仅生成脚本，开发期）、esbuild（脚本里转译 TS 测试文件）。

**Spec:** `docs/specs/code-editor.md`（6.2、6.3、6.4、第 7 节；里程碑 1 中与渲染无关的部分）

## Global Constraints

- 模块 `github.com/tangoui/tango`，Go 1.27；新依赖仅 `github.com/dlclark/regexp2`（本计划不引入 JSON Schema 库）。
- `buffer`、`regex`、`monarch`、`langs`、`theme` 不得导入 `ui` 及其子包（`ui/editor/...` 下的这几个包除外互相依赖）。
- 不照搬 mygo `ui/editor`（不得打开阅读）与 monaco-vim；Monaco 的语法/主题/测试**数据**（MIT）可以转换后内置，`ui/editor/langs/NOTICE` 写明来源与许可证。
- 生成脚本固定 `monaco-editor@0.56.0` 与 Monaco 源码仓库标签 `v0.56.0`；运行时不依赖 JS。
- 偏移：内部 UTF-8 字节；对外另提供 UTF-16 列换算（LSP 与 Monaco 语义）。
- **不提交 git**（用户未授权）。每个任务结束跑 `go build ./... && go vet ./...`。
- 其他会话可能同时修改本仓库：本计划只新建 `ui/editor/` 下的文件，唯一共享修改是 `go.mod`/`go.sum`（Task 3 加 regexp2），修改前先读最新内容。

## Review Focus

1. 含 CRLF 与混合行尾的文件：加载后行数、行内容、`LineCount` 正确，按原 EOL 写回不变 —— Task 1 `TestEOLRoundTrip`。
2. 非 ASCII 文本（中文、emoji、代理对）：UTF-16 列与字节列互换正确，编辑切不坏多字节字符 —— Task 1 `TestUTF16Columns`、`TestEditRejectsSplitRune`。
3. 回溯爆炸的插件正则（如 `(a+)+$` 加前瞻）：单次匹配超时后视为不匹配，不卡住分词 —— Task 3 `TestBacktrackTimeout`。
4. 很长的单行（1 MB 的压缩 JS）：分词不崩溃、按 Monaco 的 `maxTokenizationLineLength` 语义（默认 20000 字符后的部分按纯文本）—— Task 5 `TestLongLine`。
5. 嵌入语言（HTML 里的 `<script>`、Markdown 代码块）跨行进入与退出：状态正确恢复 —— Task 5 由 Monaco 官方 html/markdown 测试覆盖，另加 `TestEmbeddedAcrossLines`。

---

### Task 1: rope 文本模型与快照

**Files:**
- Create: `ui/editor/buffer/rope.go`（持久化平衡树，叶子为文本块）、`ui/editor/buffer/snapshot.go`、`ui/editor/buffer/pos.go`
- Test: `ui/editor/buffer/snapshot_test.go`、`ui/editor/buffer/bench_test.go`

**Interfaces:**
- Produces:
  - `type Snapshot struct{...}`（值类型，不可变，可跨 goroutine 读）
  - `func NewSnapshot(text string) Snapshot`（检测 EOL：CRLF 多数则记为 `"\r\n"`，否则 `"\n"`；内部统一为 `\n`）
  - 方法：`Len() int`、`LineCount() int`、`Line(i int) string`（不含换行，i 从 0 起）、`LineStart(i int) int`、`LineOf(off int) int`、`Slice(start, end int) string`、`String() string`（`\n`）、`TextWithEOL() string`、`EOL() string`、`Version() uint64`
  - `type Pos struct{ Line, Col int }`（0 起，Col 为字节）；`PosOf(off int) Pos`、`OffsetOf(p Pos) int`
  - `UTF16Col(line, byteCol int) int`、`ByteCol(line, utf16Col int) int`
  - `type Edit struct{ Start, End int; Text string }`；`func (s Snapshot) Apply(edits []Edit) (Snapshot, error)`：edits 按 Start 升序、互不重叠、偏移基于 s；任何边界落在多字节字符中间返回错误；新快照 `Version()` 递增。
  - 叶子块目标大小 1 KiB（最大 2 KiB），内部节点缓存（字节数、换行数），`LineStart/LineOf` O(log n)。
- [ ] **Step 1: 写失败测试** `TestApplyBasic`、`TestApplyMulti`（三处编辑一次应用）、`TestLines`（空文本 `LineCount()==1`；`"a\nb\n"` 为 3 行，最后一行为空）、`TestEOLRoundTrip`（`"a\r\nb\r\n"` → `EOL()=="\r\n"`，`TextWithEOL` 与原文相同；混合 `"a\r\nb\nc\r\n"` → CRLF）、`TestUTF16Columns`（行 `"a中😀b"`：字节列 8 ↔ UTF-16 列 4）、`TestEditRejectsSplitRune`、`TestRandomEditsMatchString`（1 万次随机编辑，与普通字符串模型逐步比对）。
- [ ] **Step 2: 写基准** `BenchmarkInsertIn1MLines`（100 万行、约 100 MB 文本中部插入单字符）与 `BenchmarkNewSnapshot100MB`。
- [ ] **Step 3: 运行** `go test ./ui/editor/buffer/` —— FAIL。
- [ ] **Step 4: 实现**。
- [ ] **Step 5: 运行** —— PASS；`go test -bench . ./ui/editor/buffer/`：单次插入 < 50 µs，`NewSnapshot` 100 MB < 500 ms（规格 6.6"≤1 秒到首屏可编辑"的预算），数字写进任务报告。

### Task 2: 撤销历史、版本语义与锚点

**Files:**
- Create: `ui/editor/buffer/buffer.go`、`ui/editor/buffer/anchor.go`
- Test: `ui/editor/buffer/buffer_test.go`

**Interfaces:**
- Consumes: Task 1 `Snapshot`、`Edit`。
- Produces:
  - `type Buffer struct{...}`（可变句柄，非并发安全，主线程用；快照可交给其他 goroutine）
  - `func New(text string) *Buffer`；`Snapshot() Snapshot`；`Edit(edits []Edit, cursorsBefore, cursorsAfter []Selection) (Snapshot, error)`；`PushUndoStop()`；`Undo() (Snapshot, []Selection, bool)`、`Redo()` 同签名；`AltVersion() uint64`（Monaco `alternativeVersionId` 语义：撤销回到某状态时等于该状态当时的值）
  - `type Selection struct{ Anchor, Head int }`
  - 编辑分组：两次 `PushUndoStop` 之间的编辑合为一步；未调用时，连续输入在 300 ms 内且光标连续的编辑合并（时间来源可注入：`Buffer.Now func() time.Time`）。
  - `type Bias int`（`Left`、`Right`）；`type Anchor struct{...}`；`func (b *Buffer) Anchor(off int, bias Bias) Anchor`；`func (b *Buffer) Resolve(a Anchor) int`（按编辑日志把锚点变换到当前版本；落在被删除区间里的锚点移到删除位置）。Monaco 四种 `TrackedRangeStickiness` 由区间两端各自的 Bias 组合表达。
- [ ] **Step 1: 写失败测试** `TestUndoRedo`、`TestUndoRestoresSelections`、`TestAltVersion`（编辑→撤销后 `AltVersion` 回到编辑前的值，而 `Version` 继续递增）、`TestTypingMerges`（注入时钟：100 ms 间隔连打 5 个字一次撤销全撤；间隔 400 ms 分两步）、`TestAnchorBias`（在锚点处插入：Left 不动、Right 后移；删除包含锚点的区间：锚点到删除起点）。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 3: 混合正则

**Files:**
- Create: `ui/editor/regex/regex.go`、`ui/editor/regex/translate.go`
- Modify: `go.mod`、`go.sum`（`go get github.com/dlclark/regexp2@latest`，记录版本）
- Test: `ui/editor/regex/regex_test.go`、`ui/editor/regex/translate_test.go`

**Interfaces:**
- Produces:
  - `type Engine int`（`Std`、`Backtrack`）
  - `type Regexp interface{ MatchStart(s string) []int; Engine() Engine; Source() string; NumGroups() int }` —— `MatchStart` 只接受从 `s` 开头开始的匹配（Monaco Monarch 对"行的剩余部分"做 `^(?:…)` 匹配，语义一致），返回字节偏移对（长度 `2*(1+NumGroups())`，未参与的组为 -1），不匹配返回 nil。
  - `func Compile(source, flags string) (Regexp, error)`；flags 支持 JS 的 `i`、`s`、`u`、`m`（其他报错）。
  - `func Translate(source, flags string) (goSyntax string, ok bool)` —— JS → RE2 翻译；`ok=false` 表示语义无法等价（前瞻、后顾、反向引用等），由 `Compile` 改用 regexp2（`regexp2.ECMAScript`，`MatchTimeout = 100 * time.Millisecond`（终审修订：原 20 ms；另有每行 50 ms 预算与 LineInfo.Degraded 降级信号），超时视为不匹配并计数）。
  - `func Stats() (stdCount, backtrackCount, timeouts int64)`（全局计数，供基准与诊断）。
  - 翻译规则（以测试为准）：`\s`/`\S` → JS 空白集合（`[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]` 及其补集，包括字符类内部的展开）；`[^]` → `[\s\S]`（JS 空白集合版）；`\uXXXX` 与 `u` 标志下 `\u{X…}` → `\x{…}`；`(?<name>` 保留（Go 1.22+ 支持）；`\/` 等标点转义保留；`\cX`、八进制转义、`[\b]` 按 JS 语义转换；`.` 在无 `s` 标志时按 JS 排除 `\n\r  `。
- [ ] **Step 1: 写失败测试**
  - `TestTranslate`（表驱动，每行 JS 源、flags、期望 Go 源或 `ok=false`）：至少覆盖上列每条规则，以及 `a(?=b)`、`(?<!x)y`、`(a)\1` → `ok=false`。
  - `TestMatchStart`：`Compile("[a-z]+", "")` 对 `"abc1"` 返回 `[0,3]`；对 `"1abc"` 返回 nil（必须从开头匹配）；`\bfoo` 对 `"foo"` 匹配。
  - `TestEngines`：`"\\d+"` → `Std`；`"-(?!>)"` → `Backtrack` 且对 `"->"` 不匹配、对 `"-a"` 返回 `[0,1]`。
  - `TestUnicodeSpace`：`\s` 匹配 `"　"`（两种引擎一致）。
  - `TestBacktrackTimeout`：`Compile("(a+)+(?=b)", "")` 对 50 个 `a` 加 `c` 在 100 ms 内返回 nil，`Stats` 的 timeouts 增加。
- [ ] **Step 2: 运行** `go test ./ui/editor/regex/` —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** —— PASS。

### Task 4: Monaco 数据导出（语法、语言配置、基础主题、官方测试用例）

**Files:**
- Create: `ui/editor/langs/gen/package.json`（固定 `monaco-editor@0.56.0`、`esbuild`）、`ui/editor/langs/gen/export.mjs`、`ui/editor/langs/gen/README.md`（如何重新生成）
- Create（生成物）：`ui/editor/langs/data/<id>.json`（每种语言一个）、`ui/editor/langs/data/themes.json`、`ui/editor/langs/testdata/tokenize/<id>.json`
- Create: `ui/editor/langs/langs.go`、`ui/editor/langs/NOTICE`
- Test: `ui/editor/langs/langs_test.go`

**Interfaces:**
- 导出格式（脚本产出，Go 侧读取）：
  - `data/<id>.json`：`{"registration": {"id","extensions","filenames","filenamePatterns","aliases","mimetypes","firstLine"}, "language": <Monarch 定义>, "conf": <语言配置>}`；所有 `RegExp` 值写成 `{"$regex": source, "flags": flags}`；遇到函数值时脚本报错并列出位置（不得静默丢弃）。来源：`esm/vs/languages/definitions/<id>/<id>.js` 的 `language`、`conf` 导出与 `register.js` 的注册信息。`.vue` 额外生成一条注册（复用 html 语法，与 MinCode `monaco-runtime.js` 现有做法一致）。
  - `data/themes.json`：`vs`、`vs-dark`、`hc-black`、`hc-light` 四个基础主题的 `{base, inherit, rules, colors}`，来源 `esm/vs/editor/standalone/common/themes.js`。
  - `testdata/tokenize/<id>.json`：`[[{"line": "...", "tokens": [{"startIndex": 0, "type": "comment.go"}]}]]`，来源：Monaco 仓库标签 `v0.56.0` 的 `src/languages/definitions/<id>/<id>.test.ts`（脚本下载到临时目录，用 esbuild 转译并把 `testTokenization` 替换为收集函数）。
- Produces（Go）：
  - `type Language struct{ ID string; Extensions, Filenames, FilenamePatterns, Aliases, MimeTypes []string; FirstLine string; Grammar json.RawMessage; Conf Conf }`
  - `type Conf struct{ LineComment string; BlockComment [2]string; Brackets [][2]string; AutoClosingPairs []Pair; SurroundingPairs []Pair; WordPattern *RegexSpec; Indentation struct{ Increase, Decrease, IndentNextLine, Unindented *RegexSpec }; Folding struct{ OffSide bool; Start, End *RegexSpec } }`；`type Pair struct{ Open, Close string; NotIn []string }`；`type RegexSpec struct{ Source, Flags string }`（JSON 中 `{"$regex","flags"}` 的解码类型，也供 monarch 使用）。
  - `func Builtin() []*Language`（按 ID 排序，惰性解析一次）；`func ByID(id string) *Language`；`func ForFilename(name string) *Language`（文件名精确匹配 > 模式 > 扩展名，大小写不敏感，与 Monaco 一致）。
  - `func BaseThemes() map[string]json.RawMessage`。
- [ ] **Step 1: 编写并运行导出脚本** `cd ui/editor/langs/gen && npm install && node export.mjs` —— 生成 84 个语言定义（含 `.vue` 共 85 条注册）、四个基础主题、每个有测试文件的语言一份 tokenize 用例；脚本最后打印计数，与这些数字不符时以退出码 1 结束。
- [ ] **Step 2: 写失败测试** `TestBuiltinCount`（`len(Builtin()) == 85`）、`TestForFilename`（`main.go`→go、`Dockerfile`→dockerfile、`x.vue`→vue、`a.TS`→typescript）、`TestConfDecodes`（每种语言的 `Conf` 都能解码，`go` 的 `LineComment=="//"`）、`TestFixturesPresent`（go、typescript、yaml、html、markdown 的 tokenize 用例存在且非空）。
- [ ] **Step 3: 运行** `go test ./ui/editor/langs/` —— FAIL。
- [ ] **Step 4: 实现** `langs.go`（`//go:embed data/*.json`）与 `NOTICE`。
- [ ] **Step 5: 运行** —— PASS。

### Task 5: Monarch 解释器

**Files:**
- Create: `ui/editor/monarch/compile.go`（语法 → 规则表）、`ui/editor/monarch/lexer.go`（逐行分词）、`ui/editor/monarch/state.go`（状态栈驻留）、`ui/editor/monarch/registry.go`
- Test: `ui/editor/monarch/monarch_test.go`、`ui/editor/monarch/bench_test.go`

**Interfaces:**
- Consumes: Task 3 `regex.Compile`；Task 4 `langs.ByID`、`langs.RegexSpec`、`testdata/tokenize/*.json`。
- Produces:
  - `type Registry struct{...}`；`func NewRegistry() *Registry`（默认装入全部内置语言，惰性编译）；`func (r *Registry) Add(id string, grammar json.RawMessage) error`（插件贡献语言；同 ID 覆盖）；`func (r *Registry) Lexer(id string) (*Lexer, error)`
  - `type State uint32`（驻留后的状态 ID；0 保留为"无"）
  - `type Token struct{ Start int; Type string }`（与 Monaco `startIndex`/`type` 对应，Start 为**UTF-16 列**以便直接与 Monaco 用例比对；另提供 `ByteStart int` 字段供渲染使用）
  - `func (l *Lexer) Initial() State`；`func (l *Lexer) TokenizeLine(line string, st State, dst []Token) ([]Token, State)`
  - `const MaxLineLength = 20000`（超出部分作为一个 `""` 类型 token，状态不变，与 Monaco 默认 `maxTokenizationLineLength` 一致）
  - 必须支持的 Monarch 特性（以官方用例为准）：`tokenizer` 状态与 `include`、`@attr` 在正则与 cases 中的展开、`ignoreCase`、`unicode`、`tokenPostfix`、`defaultToken`、`brackets`（只需产出 token 类型）、动作的 `token`/数组形式（按捕获组分配）/`cases`（`@name`、`$n`、`$Sn`、`@default`、`@eos`、`~regex`、`!=`/`==` 比较）、`next`（`@push`、`@pop`、`@popall`、具名状态、`$S` 引用）、`switchTo`、`goBack`、`nextEmbedded`（进入其他语言直到 `@pop`）、`log`（忽略）。
  - 状态驻留：（状态栈, 嵌入语言, 嵌入语言状态）元组 → `State`；同一元组得到同一 ID。
- [ ] **Step 1: 写失败测试**
  - `TestMonacoFixtures`：遍历 `langs/testdata/tokenize/*.json`，对每个用例逐行分词（行间传递状态），与期望 `startIndex`/`type` 完全相等；失败信息给出语言、用例序号、行、期望与实际。本任务要求 **go、typescript、yaml 三种全部通过**（里程碑 1 选定的语言）；其余语言的通过率写进任务报告，未通过的列为 Task 6 的输入。
  - `TestStateInterning`：同一 Lexer 对相同行与相同输入状态两次分词，输出 State 相等；不同栈得到不同 State。
  - `TestLongLine`：1 MB 单行 JS 不 panic，`MaxLineLength` 之后为一个 `""` token。
  - `TestEmbeddedAcrossLines`：HTML 三行 `<script>`、`let x = 1`、`</script>`，第二行 token 类型带 `.js` 后缀（或 Monaco 用例中的同等表现），第三行回到 HTML。
- [ ] **Step 2: 写基准** `BenchmarkTokenizeTS20k`：2 万行 TypeScript（由 Monaco 用例里的 TS 片段重复拼成）全量分词。
- [ ] **Step 3: 运行** `go test ./ui/editor/monarch/` —— FAIL。
- [ ] **Step 4: 实现**。
- [ ] **Step 5: 运行** —— PASS；基准 ≤ 2 s（规格 6.6"2 万行 TypeScript 全量高亮 ≤ 2 秒"），并报告 `regex.Stats()` 中两种引擎的比例与超时次数。

### Task 6: 全部内置语言对齐 Monaco 用例

**Files:**
- Modify: `ui/editor/monarch/*.go`、`ui/editor/regex/translate.go`（按失败用例修正）
- Test: 同 Task 5 `TestMonacoFixtures`

**Interfaces:**
- Consumes/Produces: 不新增接口。
- [ ] **Step 1: 运行** `go test ./ui/editor/monarch/ -run TestMonacoFixtures -v` 并汇总每种语言的失败用例。
- [ ] **Step 2: 逐个修正**：失败原因只能是解释器或正则翻译的问题，不得修改生成的数据与用例。
- [ ] **Step 3: 运行** —— 所有带用例的语言 100% 通过；`TestMonacoFixtures` 改为对全部语言强制断言。

### Task 7: Monaco 主题解析与 token → 样式

**Files:**
- Create: `ui/editor/theme/theme.go`
- Test: `ui/editor/theme/theme_test.go`

**Interfaces:**
- Consumes: Task 4 `langs.BaseThemes()`。
- Produces:
  - `type Style struct{ Foreground, Background color.NRGBA; Bold, Italic, Underline, Strikethrough bool }`
  - `type Theme struct{...}`；`func Parse(data []byte) (*Theme, error)`（Monaco `IStandaloneThemeData`：`base` 为四个基础主题之一，`inherit` 为 true 时先应用基础主题规则；`rules[].token/foreground/background/fontStyle`；颜色接受 `RGB`、`RRGGBB`、`RRGGBBAA`，可带 `#`）；`func Base(name string) *Theme`
  - `func (t *Theme) Style(tokenType string) Style`：按 Monaco 规则匹配——把 token 按 `.` 分段，取能匹配的最长规则前缀（`keyword.go` 匹配规则 `keyword`；规则 `keyword.go` 优先于 `keyword`）；未匹配取默认前景/背景（`colors["editor.foreground"]` / `editor.background`）；结果按 tokenType 缓存（并发安全）。
  - `func (t *Theme) Color(key string) (color.NRGBA, bool)`（`colors` 表，如 `editor.lineHighlightBackground`）。
- [ ] **Step 1: 写失败测试** `TestBaseVSDark`（`Base("vs-dark").Style("comment.go").Foreground` 等于 Monaco vs-dark 中 `comment` 规则的颜色，从 `themes.json` 读出比对，不手写色值）、`TestInheritOverride`（自定义主题 inherit vs，把 `keyword` 改色，`keyword.go` 取新色、`string.go` 仍取 vs 的）、`TestLongestPrefix`、`TestColorFormats`。
- [ ] **Step 2: 运行** —— FAIL。
- [ ] **Step 3: 实现**。
- [ ] **Step 4: 运行** `go test ./ui/editor/...` —— 全部 PASS；`go vet ./ui/editor/...` 干净；`go list -deps ./ui/editor/buffer ./ui/editor/regex ./ui/editor/monarch ./ui/editor/langs ./ui/editor/theme | grep 'tango/ui$\|tango/ui/[a-df-z]'` 无输出（未依赖 ui 其他包）。
