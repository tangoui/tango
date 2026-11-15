# Diff 视图与编辑器根包新接口 设计稿

- 起草：min-3c，2026-11-14
- 状态：**已批准（2026-11-15，用户：待决问题全部按默认）**
- 依据：`docs/specs/code-editor.md`（第 4 节清单第 7 项 Diff、第 10 项装饰、第 13 项模型；6.2 分层；6.3 偏移；6.5 provider；6.6 性能门槛）、`ui/AUTHORING.md`、Monaco 0.56 源码（`ui/editor/langs/gen/node_modules/monaco-editor/esm/vs/editor/`，只借设计）、算法层 `ui/editor/diff`（origin/main 165f8b7）、min-3d 的提案草稿 `.superpowers/sdd/2026-11-14-editor-m2-acceptance/diff-view-api-proposal.md`，以及编辑器会话（min-0d）的评审意见。
- **基线说明**：文中的文件:行号都以**主工作区当前未提交的 M2 代码**为准（`overview.go`、`minimap.go`、`navigation_coords.go`、`injected.go`、`longwork.go` 等只在主工作区存在，origin/main 上没有）。实现必须建立在 M2 改动落地（提交）之后的代码上，见第 6 节风险 R1。

---

## 0. 结论先行

1. **装饰挂在 Model 上**（多个视图共享，只做一次平移）。对齐 Monaco `IModelDecorationOptions` 的一个子集，加上 `TrackedRangeStickiness` 的四种稳定性。区间用 UTF-8 字节偏移，LSP 换算复用 `buffer.Snapshot.UTF16Col/ByteCol`。每个装饰集合可以带一个"只在某个 State 显示"的过滤，对应 Monaco 的 ownerId：Diff 的修改侧 Model 常常同时开在主编辑器里，diff 装饰不能漏到主编辑器上。
2. **View zones 和按 owner 的隐藏区域挂在 State 上**。view zone 的高度**按整行计**，直接并进 `viewLines` 现有的 Fenwick 前缀和（与折叠、换行用同一套结构）。这样 `Viewport.TopLine`/`OffsetY`、`lineY`、滚动条、概览尺、小地图、粘性滚动、`ModelToView/ViewToModel` 都不需要另起一套像素映射。
3. **局部重绘**：装饰进入每行指纹（`damage.go` 的 `rowHashes`）；zone 的每一行有自己的指纹（owner、ID、`Version`、行内序号）。`Version` 为 0 时由编辑器对 zone 回调画出的绘制命令做指纹兜底。zone 或隐藏区的布局变化（增删、改高度、改隐藏）和折叠一样走 `damageGlobal` 整视图重画。`TestViewDamageVerified` 必须继续通过，另外新增三条同类严格校验测试。
4. **DiffView 建议放在 `ui/editor/diffview` 子包**，只用根包的公开 API（同时证明这些 API 够用）。代价是规格 6.2 里写的是"DiffView 在 ui/editor"，已按决定 Q1 修改规格 6.2。
5. **实施顺序**：先在基于 origin/main 的独立 worktree 里只新增文件（diffview 的纯逻辑；根包里的装饰存储、zone 集合、隐藏区集合三个纯数据结构，不接线）。M2 提交以后再改根包现有文件接线。`options.go`、`minimap.go` 要等编辑器会话的 Task 8 结束才动。

---

## 1. 现状：编辑器现在怎么组织

### 1.1 Model / State / ViewW

| 类型 | 位置 | 要点 |
|---|---|---|
| `Model` | `model.go:101` | 一份文档：`buffer.Buffer`、语言、`Highlighter`、括号树、缩进索引、长行缓存 `lineMemo`。多个 State 共享。`Subscribe(fn func(ModelChange))`（`model.go:411`）同步通知；`ModelChange`（`model.go:58`）带 `Old/New` 快照和 `Edits`（撤销、重做同样带 edits），以及 `Source *State`。 |
| `State` | `state.go:18` | 一个编辑器对 Model 的视图：`Sels`、`Viewport`、`Options`、`vl viewLines`、`fold foldState`、`overview map[string][]OverviewMark`。`NewView(m, th)`（`state.go:107`）订阅 Model；`modelChanged`（`state.go:140`）随别的视图的编辑平移选区、`syncEdits` 换行与折叠，并用锚点保持顶部行（`anchorTop`）。 |
| `ViewW` | `view.go:53` | 每帧的构建器：`View(c, st)`（`view.go:184`）在 build 阶段处理滚轮（`st.ScrollBy`），用 `ui.Custom(measure, paint)` 自绘；`viewState`（`view.go:68`）保存跨帧状态（布局缓存、`dmg` 上一帧绘制记录等）。修饰器 `ReadOnly/LineNumbers/FontSize/TabSize`（`view.go:322-354`）只覆盖当帧。 |
| `Viewport` | `viewport.go:21` | **滚动位置 = 视图行 `TopLine` + 像素 `OffsetY`，所有行等高 `LineHeight`**。`scrollY = TopLine*lh + OffsetY`（`viewport.go:44`），顶部内边距时 `OffsetY` 可以到 `-PadTop`。`ScrollX` 是横向。 |

### 1.2 视图行映射（viewLines，`viewlines.go`）

- `viewLines`（`viewlines.go:46`）对 Model 行建一棵 Fenwick 树，存每行的"额外行数"：换行多出来的行为正数，被隐藏的行记 -1（`extra`，`viewlines.go:170`）。`Count = n + total`；`viewOf(l) = l + prefix(l)`（`:273`）；`find(v)`（`:283`）返回视图行 v 落在哪个 Model 行的第几行，复杂度 O(log n)。
- 不换行也不隐藏时，映射是恒等的，不分配树（`mapped()`，`:95`）。**这是 100 万行性能门槛的关键快路径，新能力在不用时不能破坏它。**
- 换行是惰性的：只量视口附近的行，没量过的行按 1 行算。行数变化时，`State` 用锚点（顶部的 Model 行加行内序号）保持画面不动（`anchorTop`，`:721`）。
- 隐藏行：`setHidden([]lineSpan)`（`:241`）**只接受一组区间**，由 `State.syncFolds`（`folding.go:199`）、`applyFolds`（`folding.go:341`）从 `fold.hidden(n)`（`folding.go:263`）传入；文本变化时 `vl.sync/syncEdits` 先 `dropHidden` 再由 State 重新隐藏。
- 长行与增量换行：`syncEdits` → `rewrapped`（`:422`）→ `wrap.go` 的 `rewrapEdit` 只重扫改动附近；`memo` 指向 Model 的 `lineMemo`（`longwork.go`）。

### 1.3 绘制（`paint.go:104` `ViewW.paint`）

顺序：设置 Viewport 尺寸 → `setWrap` → `foldPrepare` → `measureViewport` → 高亮预算 → 小地图预取 → 括号（`bracketSpans`，`bracketpairs.go:841`，多窗口）→ `layoutRows(lo..hi)`（`paint.go:651`，**按视图行连续产出 `[]lineView`，后续大量代码用 `lines[r-lo]` 索引**）→ 背景 → 当前行高亮 → 折叠背景 → 选区 → 缩进与括号参考线 → **显示装饰** `decorations` + `paintDecoBoxes`（`paint.go:1172`、`:1335`）→ 括号匹配 → 文字与空白 → `paintDecoOver` → IME 与光标 → gutter（`gutter.go:153`）→ 折叠控件 → 粘性滚动、小地图、概览尺 → `declareDamage`。

- 行纵坐标：`lineY(r) = ty + (r-TopLine)*lh`（`paint.go:241`），**等高行假设**贯穿 paint、input（`pointRow`，`input.go:717`）、概览尺（`overview.go:49`）、`navigation_coords.go`、小地图。
- 现有"显示装饰"是内部的 `deco`/`decoKind`（`paint.go:1139-1155`：同词、选区匹配、可疑字符、链接、颜色块），每帧按布局到的行计算，不随编辑平移，也不对外开放。
- 区间 → x 的换算：`decoRects`（`paint.go:1296`），已处理 `injected.go` 的颜色间隙（`lineView.inj`、`withGaps`，`injected.go:214`）。长行只有已塑形的块才有精确 x。

### 1.4 局部重绘（`damage.go`）

- 每个画出的视图行有一个指纹（`rowHashes`，`damage.go:136`），覆盖行号、文字、颜色、选区、当前行、参考线、括号匹配、装饰签名（`pf.decos`）、折叠码、IME 等。行指纹变化 → 整行（gutter + 文本）标脏。
- 光标和拖放插入条按矩形比较（marks）；纵向滚动是 `Box.ScrollShift`（`damage.go:347`）；横向滚动只平移文本区。
- `damageGlobal`（`damage.go:77`）里任何字段变化都整视图重画，其中 `foldGen` 是"折叠变化 → 映射整体变化"的先例。
- 固定区域（概览尺、小地图）的绘制记录为 `fixedPaint`，按屏幕空间集合比较（`declareDamage` 的 `oldFixed/newFixed`）。
- 约定（`damage.go:73-75`）：**新增的显示元素必须被行指纹、marks 或 damageGlobal 覆盖之一**。
- 验证：`TestViewDamageVerified`（`damage_test.go:149`），三个缩放比例，严格校验模式下每个局部帧必须等于整帧重画。注意：scale1 和 scale1.25 有已知的间歇性闪烁帧问题（progress.md 记录为 M2 之前就存在），我们的测试要区分。

### 1.5 gutter / 概览尺 / 滚动 / 其他

- gutter：行号区 + **装饰边栏 `lineDecorationsW = 18`**（`gutter.go:19`，Monaco 的 linesDecorations 区，正好放 diff 的 +/− 标记）+ 折叠控件 16px。行号文字由 `Options.lineNumber`（`gutter.go:125`）决定，宽度由 `numbersWidth`（`gutter.go:116`）决定。
- 概览尺：`State.SetOverviewMarks(owner, []OverviewMark)`（`overview.go:24`）已经是"按 owner 整体替换、挂在 State 上"的样板。y 坐标 = `navigationRow * LineHeight`，经 `navigationHeight` 归一化（`navigation_coords.go`）。
- 滚动：只有 `ScrollBy`、`RevealCaret`、`RevealX`、内部的 `revealLine/revealRow/jumpLine`；**没有对外的滚动回调**；`SaveViewState` 记录的是 Model 行加 `OffsetY`（`viewstate.go:28`）。
- 只读：`Options.ReadOnly` / `ViewW.ReadOnly`（`options.go:146`、`view.go:354`），会拦截所有标了 Mutates 的命令和 IME。
- 锚点：`buffer.Anchor` + `Bias`（`buffer/anchor.go:47`），注释里已经写明用两端的 Bias 表达 Monaco 四种 stickiness（`anchor.go:37-43`，`TestStickiness` 覆盖）。`buffer.Tracker`（`buffer/tracker.go:15`）不依赖 Buffer，用同样的规则批量映射偏移，O((点数 + edits)·log edits)。
- 主题：编辑器用 VS Code/Monaco 主题（`ui/editor/theme`）；`diffEditor.*`、`diffEditorGutter.*`、`diffEditorOverview.*` 的 Monaco 默认值**已经生成**在 `theme/colors_gen.go:55-72`；`Theme.OwnColor` 可以区分主题自己写的颜色和注册表默认值。
- 框架：`c.Announce(text, live)`（`ui/a11y_tree.go:94`）可以宣告读屏信息；`ui.Splitter`（`ui/nav_splitter.go:128`）可用于并排模式。

### 1.6 可以直接复用的能力

| 需求 | 复用 |
|---|---|
| 装饰随编辑平移 | `buffer.Tracker` 的批量映射规则与 `Anchor.Resolve` 相同，四种 stickiness 即两端 Bias 的组合（已有测试） |
| 按 owner 替换 | `SetOverviewMarks` 的形态 |
| 隐藏行 | `viewLines.setHidden` + 锚点跟随编辑（folding 的 `fold.s0/s1/e` 做法） |
| zone 占行 | `viewLines` 的 Fenwick 树（再加一项"zone 行数"） |
| 区间背景 x | `decoRects`（含颜色间隙） |
| 局部重绘 | 行指纹 / `damageGlobal`（照搬 `foldGen` 先例）/ `fixedPaint` |
| 原侧只读 | `Options.ReadOnly` |
| 后台 diff | `buffer.Snapshot` 不可变、可以跨 goroutine 读；`diff.Snapshots(ctx, a, b, opts)` 支持取消 |
| 光标跳过隐藏区 | `State.overFolds`（`selection.go:154`）基于 `vl.hidden`，对按 owner 的隐藏区同样生效 |
| 命令与快捷键 | `RegisterCommand`（`command.go:48`）+ keymap / when |

---

## 2. 根包新接口（精确到 Go 签名）

> 命名已在主工作区 grep 过（`Decoration`、`ViewZone`、`Stickiness`、`HiddenArea`、`ScrollEvent`、`LineTop`、`RevealLine` 都没有冲突）。内部标识统一用 `decoration*`/`zone*`/`hidArea*` 前缀，避免和 M2 已有的 `deco`/`decoKind` 撞名。

### 2.1 装饰（挂 Model）

```go
// decorations.go（新文件）

// Stickiness 是装饰两端在边缘输入时的行为（Monaco TrackedRangeStickiness）。
type Stickiness uint8

const (
	AlwaysGrowsWhenTypingAtEdges Stickiness = iota // start Left,  end Right（Monaco 默认值 0）
	NeverGrowsWhenTypingAtEdges                    // start Right, end Left
	GrowsOnlyWhenTypingBefore                      // start Left,  end Left
	GrowsOnlyWhenTypingAfter                       // start Right, end Right
)

// DecorationOptions 是 Monaco IModelDecorationOptions 的子集。
// 同一个 *DecorationOptions 可以被很多装饰共享（对应 ModelDecorationOptions.register），
// 设置以后不要原地修改（编辑器在 Set 时缓存它的指纹）。
type DecorationOptions struct {
	Description string     // 调试与读屏用（Monaco description）
	Stickiness  Stickiness
	ZIndex      int        // 越大越后画（Monaco zIndex）
	WholeLine   bool       // isWholeLine：铺满所覆盖行的文本区
	ShowIfCollapsed bool   // 空区间也画（Monaco showIfCollapsed）

	Background  color.NRGBA // className 的背景（WholeLine 时铺整行）
	Border      color.NRGBA // 可选描边（diffEditor.insertedTextBorder 等高对比主题用）
	Inline      *InlineStyle // inlineClassName：绘制时覆盖文字样式，不改变排版

	Gutter          *GutterMark // linesDecorationsClassName：装饰边栏（18px）
	MarginBackground color.NRGBA // marginClassName：整个 gutter 的行背景（diffEditorGutter.*）
	LineNumberColor  color.NRGBA // lineNumberClassName 的颜色部分

	Overview *OverviewStyle // overviewRuler：{color, position}
	// Minimap 留到 Task 8 结束后再加（minimap.go 暂时不动）。
}

// InlineStyle 只含不改变字形排版的属性（第一版）。粗体、斜体会改变塑形，见决定 Q5。
type InlineStyle struct {
	Color         color.NRGBA // 覆盖 token 前景色（A==0 表示不覆盖）
	Underline     bool
	Strikethrough bool
	Opacity       float32 // 0 表示不变
}

type GutterMark struct {
	Background color.NRGBA // 整个装饰边栏格的背景
	Bar        color.NRGBA // 左缘 3px 色条（git 改动条样式）
	Sign       string      // 1～2 个字符，等宽字体居中（diff 的 "+" "−"）
	SignColor  color.NRGBA
}

type OverviewStyle struct {
	Color color.NRGBA
	Lane  int // 与 OverviewMark.Lane 相同：1 左、2 中、4 右
}

// Decoration 是一个区间装饰。Start/End 是设置时那一份快照里的 UTF-8 字节偏移，半开区间。
type Decoration struct {
	Start, End int
	Options    *DecorationOptions
}

// Model 级：所有视图都显示。owner 整体替换；ds 为空表示删除这个 owner。
// 输入会被复制，区间会被裁到文档内、对齐到字符边界。O(D log D)。
func (m *Model) SetDecorations(owner string, ds []Decoration)

// Decorations 返回 owner 当前（已随编辑平移的）装饰，顺序与设置时一致，
// 下标可以当作 owner 内的稳定 ID（替代 Monaco 返回的字符串 ID）。
func (m *Model) Decorations(owner string) []Decoration

// DecorationsIn 返回与 [start, end) 相交的装饰（所有 owner，不含 State 级的），按 Start 排序。
func (m *Model) DecorationsIn(start, end int, dst []Decoration) []Decoration

// State 级：仍然存放并平移在 Model 上，但只在 s 上显示（Monaco 的 ownerId 过滤）；
// s.Close 时自动删除。DiffView 用这一层。
func (s *State) SetDecorations(owner string, ds []Decoration)
func (s *State) Decorations(owner string) []Decoration

// 便捷构造与 LSP 换算（复用 buffer.Snapshot.ByteCol / UTF16Col）。
func LineDecoration(snap buffer.Snapshot, first, last int, o *DecorationOptions) Decoration // [行首(first), 行尾(last))
func DecorationFromLSP(snap buffer.Snapshot, startLine, startUTF16, endLine, endUTF16 int, o *DecorationOptions) Decoration
```

**语义**

- `WholeLine`：覆盖 `LineOf(Start)` 到 `LineOf(End)` 的每一行（与 Monaco 相同：End 落在下一行第 0 列时也包含那一行。所以 `LineDecoration` 把 End 设为最后一行的行尾，而不是下一行的行首）。
- 层次：整行背景画在当前行高亮之后、选区之前；区间背景画在选区之后、现有 `paintDecoBoxes` 之前（Monaco 的 decorations overlay 层）；`Inline` 在 `drawSeg` 时覆盖 `runPaint`（前景色、下划线、删除线、透明度），**不进 `styleRuns`、不改塑形，因此不影响 `LineCache`、换行和长行块表**。
- 同一像素上有多个装饰时，按 (ZIndex, owner 名, 下标) 稳定排序后依次画。

**随编辑平移（实现）**

- 存储：每个 Model 一个 `decorationStore`，所有 owner 的条目放进**按 Start 排序的单一切片**，条目为 `{start, end int32/int; opts *DecorationOptions; owner uint16; view *State; idx int32}`。另外维护 `maxSpan`：不超过 64 KiB 的条目最大跨度。跨度更大的条目放一个单独的小切片线性扫描，这样一条覆盖整个文档的装饰不会拖慢查询。
- 在 `Model.notify`（`model.go:413`）里、**订阅者之前**，对每个带 `Edits` 的 `ModelChange` 跑一遍 `buffer.NewTracker(ch.Edits)`：
  - 第一个可能受影响的下标用二分查找（`Start >= 第一处编辑起点 - maxSpan`）；之前的条目不动；
  - 与编辑相交的条目用 `Tracker.Map(off, bias)` 逐个映射（bias 由 stickiness 决定两端），然后按 `Resolve` 的规则对齐到字符边界（Tracker 本身不对齐）；
  - 完全在所有编辑之后的尾部条目统一加上 `Tracker.pre[len]` 的位移（一次线性加法循环）；
  - 因为两端 bias 可能不同，并列的 Start 可能换序：只对受影响的窗口做一次插入排序。
- 撤销和重做与 Monaco 一致：**装饰不进撤销历史**，撤销只是又一组 edits，装饰照样平移（Monaco 撤销后也不会恢复被删掉的装饰区间）。整文替换（重新加载、格式化）沿用 `Anchor` 的规则，区间保持偏移、裁到新长度。
- 多光标：一次多光标编辑在 `ModelChange` 里是一组有序的 `Edits`，一次 Tracker 批量处理即可。装饰与选区互不影响（不像 Monaco 那样对光标用 forceMoveMarkers）。
- 编辑器内部"订阅者里再编辑"的排队机制（`Model.enqueue/drain`）不受影响：每次 notify 都先平移装饰。

**查询与绘制**

- 每帧对布局到的 Model 行范围 `[LineStart(mlo), lineEnd(mhi)]` 二分查找，复杂度 O(log D + k)，**不扫描行文本**。这条对长行路径（`brackets_long.go`、`longwork.go`）很重要：装饰的开销与行长无关。
- 长行（`isLong`）上的区间背景只画与已塑形块相交的部分。块外的部分按列网格换算 x（与 `contentX` 的兜底一致）；带颜色间隙的行一律经 `decoRects`（`injected.go` 的列 → x）。
- 概览尺：按 (store 版本, 快照版本, 视图过滤) 缓存"行 → 颜色"列表，只在变化时重算（避免每帧对 1 万个装饰做 1 万次 `LineOf`）。绘制前按像素 y 和颜色去重，绘制量不超过概览尺高度 × 车道数。仍然走 `navigationFill`，所以 `fixedPaint` 比较自动覆盖。

**局部重绘**

- 每行的装饰签名并进 `rowHashes`（扩展现有的 `pf.decos`，或者新增 `pf.udecos []uint64`）。签名 = 相交装饰的 (opts 指纹, 行内字节区间)。opts 指纹在 Set 时算一次并缓存。
- gutter 标记、整个 gutter 的背景、行号颜色同样进行指纹（gutter 和文本同属一行，整行一起标脏，与现有规则一致）。
- 结果：`SetDecorations` 只让真正变化的行和概览尺上变化的格子变脏。

**性能（10 万行、1 万个装饰）预算与测法**

| 操作 | 预算 | 测法 |
|---|---|---|
| `SetDecorations`（1 万条） | ≤ 3 ms | `BenchmarkSetDecorations10k` |
| 单字符输入时的平移 | ≤ 50 µs（尾部只做加法） | `BenchmarkDecorationsTypeShift`，`AllocsPerRun == 0` |
| 每帧查询 + 绘制（视口约 60 行） | ≤ 100 µs | `BenchmarkDecorationsFrame` |
| 没有装饰时 | 0 开销：100 万行输入 p95、滚动的 ABAB 成对比值 ≤ 1.10 | M2 Task 1 的 `perfab/run.sh` |
| 1 万个装饰 + 100 万行 | 输入 p95 ≤ 16 ms | 新增 `BenchmarkTypeAt1MLinesDeco10k` |

如果平移实测超标，备选方案是 Monaco 那种带延迟增量的区间树。第一版不做，等基准说明需要再做。

### 2.2 View zones（挂 State）

```go
// zones.go（新文件）

// ViewZone 是插在 Model 行之后、占整数行高度的一块区域（Monaco IViewZone，见决定 Q2）。
type ViewZone struct {
	ID        uint64 // owner 内唯一；指纹和指针事件用它
	AfterLine int    // 在这一 Model 行（0 起）之后；-1 表示在第一行之前
	Rows      int    // 高度，单位为行（>= 1）
	MinWidth  float32 // 内容最小宽度（参与 ContentWidth，横向滚动用；Monaco minWidthInPx）
	ShowInHidden bool // AfterLine 被隐藏（折叠或隐藏区）时是否仍显示（Monaco showInHiddenAreas）

	// Version 每次 Paint 的结果会变时由调用者递增；0 表示"让编辑器给我画出的命令做指纹"。
	Version uint64
	// Fixed 为 true 时文本区部分不随横向滚动（如居中的"显示隐藏的 N 行"），
	// 指纹带上 ScrollX，横向滚动时重画。
	Fixed bool

	Paint  func(l *paint.List, z ZonePaint) // 文本区，nil 表示空白（对齐垫高）
	PaintGutter func(l *paint.List, z ZonePaint) // gutter 部分，nil 表示只画 gutter 背景

	// SuppressMouseDown 为 true 时按下不移动光标（Monaco 默认 false，diff 的垫高区设为 true）。
	SuppressMouseDown bool
	OnPointer func(e ZonePointer) // 可选：点击等（"显示隐藏的 N 行"、还原箭头）
	Cursor    input.Cursor
	Label     string // 读屏名称
}

type ZonePaint struct {
	Rect    geom.Rect // 这一块（文本区或 gutter）的窗口坐标矩形，已裁剪
	OriginX float32   // 内容 x=0 的窗口 x（文本区 = text.X - ScrollX；Fixed 时为 text.X）
	RowH    float32   // 行高
	FirstRow int      // 这一次画到的第一行（zone 只有部分可见时）
	Theme   *theme.Theme
}

type ZoneArea uint8 // ZoneText / ZoneGutter
type ZonePointer struct {
	Kind  input.PointerKind // Press / Release / Move
	Pos   geom.Point        // 相对于 zone 左上角
	Area  ZoneArea
	Clicks int
}

func (s *State) SetViewZones(owner string, zs []ViewZone) // 整体替换；nil 表示删除
func (s *State) ViewZones(owner string) []ViewZone       // AfterLine 是随编辑平移后的当前值
```

**接入映射（不另起一套）**

- 对每个 zone 记一个锚点：`AfterLine` 行行尾的 `buffer.Anchor`，bias 为 Right（与 Monaco 用 afterLineNumber/afterColumn 跟踪的效果一致：在该行末尾输入换行时，zone 跟着上一行）；`AfterLine == -1` 不需要锚点。和折叠一样，在 `vl.sync/syncEdits` 之后由 State 重新解析，得到"有效行"：**AfterLine 被隐藏时挂到它之前最近的可见行**；`ShowInHidden == false` 的直接不计。
- `viewLines` 增加 `zr` 和 `lead`：`zr` 是有效行 → zone 行数的稀疏表（按行排序的切片，条目数等于 zone 数），`lead` 是第一行之前的 zone 行数。树里的额外行数改为：
  `extra(l) = hidden(l) ? -1 : wrapExtra(l) + zr(l)`，`Count = n + total + lead`，`viewOf(l) = lead + l + prefix(l)`。
  `mapped()` 加上 `|| len(zr) > 0 || lead > 0`。**没有 zone 时代码路径与现在完全相同**（恒等快路径不变）。
- 行块：一个 Model 行的块 = 文本行（`rowsOf`）+ zone 行。`find(v)` 仍然返回 `(line, row)`，但 `row` 可能 ≥ `rowsOf(line)`（表示落在 zone 行上）；`lead` 行返回 `(0, row<0)`。新增 `vl.zoneAt(line, row) (*zoneRow, k int, ok bool)`。
- `Viewport.TopLine` 仍是视图行，所以顶部可以停在 zone 中间；`anchorTop`/`topAnchor`（`viewlines.go:721/740`）的行号裁剪要改成"文本行 + zone 行"（现在用 `lb.rows()-1` 裁剪，会把顶部的 zone 行吃掉，画面会跳）。

**各子系统的影响与改动点（实现时逐个审查）**

| 子系统 | 影响 | 改动 |
|---|---|---|
| 滚动高度、滚动条 | `Count` 已包含 zone，`setScrollY` 的上限自动正确 | 无 |
| `layoutRows`（`paint.go:651`） | 必须保持"每个视图行一个 `lineView`" | zone 行产出 `lineView{i: 有效行, row: textRows+k, zone: &zr}`，`segs` 为空，`text` 为空串 |
| 选区、当前行、参考线、括号匹配、空白符、折叠码、装饰 | 这些循环会遍历 `lines` | 一律跳过 `lv.zone != nil` 的行（当前行高亮按 `rowsOf` 只覆盖文本行，本来就不会碰到） |
| 光标上下移动 `stepRows`（`viewlines.go:939`） | 用的是 `breaks(l).rows()`，不含 zone | 天然跳过 zone；PageUp/Down 用 `pageLines` 换算文本行，要核对它和视图行的差别 |
| 命中测试 `hitSide/visualAt/placeholderAt/foldControlPress`（`input.go:650-860`）、`dnd.go:91` | `find` 可能落在 zone 行 | zone 行先交给 zone 的 `OnPointer`；`SuppressMouseDown == false` 时按 Monaco 把光标放到 zone 上方行的行尾或下方行的行首（按落点在上半还是下半） |
| 行号 | `lv.row != 0` 的行不画行号 | zone 行自然没有行号；zone 自己的 `PaintGutter` 可以画（内联删除行用原侧行号，见 2.5） |
| 括号着色 `bracketSpans`、括号对参考线 `bracketguides.go:173`、`guidesFor` | 按视图行取 `find` | 跳过 zone 行；参考线在 zone 行处断开（Monaco 也断开） |
| 粘性滚动 `sticky.go:58` | `viewOf*lh` 已经包含 zone | 无；粘性行之下的 zone 照常画 |
| 小地图 `minimap.go:231` | `find(vr)` 的 `part` 可能 ≥ 文本行数 | zone 行画成空白（**Task 8 结束后再改 minimap.go**；在那之前，DiffView 按 Monaco 的做法关掉小地图，普通编辑器在出现第一个 zone 前不会走到这条路径） |
| 概览尺、`navigation_coords.go` | 按视图行 × 行高 | `navigationRow` 用 `viewOf`，已经包含 zone，无需改 |
| 换行 | zone 排在一行的全部换行行之后 | 无；换行行数从未知变为已知时，`anchorTop` 照常保持画面 |
| 横向内容宽 | `ContentWidth = maxW + colW` | 并入可见 zone 的 `MinWidth` |
| `SaveViewState` | 记录 Model 行 | 顶部在 zone 中时记为该行的块内偏移（`OffsetY` 可以超过一行，恢复时裁剪） |

**绘制与局部重绘契约**

- 绘制：`paint` 在"4. 文字"之后对可见的 zone 行按 zone 分组：`l.Clip(zone 矩形与文本区的交集)` → `Paint` → `Pop`；gutter 部分在"6. Gutter"阶段同样处理。zone 背景默认用 `editor.background`（垫高区由 DiffView 的 `Paint` 画 `diffEditor.diagonalFill` 斜纹）。
- 指纹：zone 的每一行在 `rowHashes` 里写入 `(标记 12, owner, ID, Version, k, Fixed ? ScrollX : 0)`；gutter 部分并进同一行的指纹（一行 = gutter + 文本）。
- **`Version != 0`：编辑器信任调用者**，Paint 的输出只能取决于 (Version, ZonePaint 的矩形尺寸与 OriginX, Theme)。违反契约会留下旧像素，严格校验测试能发现。
- **`Version == 0`：编辑器记下回调前后 `l.Ops` 的区间，按相对 zone 原点的坐标给这些 op 做指纹**（只支持 Fill/Stroke/Glyphs/Path/Line/Clip/Pop，遇到其他 op 时这一帧整视图重画），保证不会出错，代价是每帧一次哈希。在 `editor` 包内写一个精简版的 op 指纹函数（`ui/damage_fp.go` 没有导出，不改框架）。
- zone 的增删、`Rows`/`AfterLine` 变化会改变映射，计入 `damageGlobal.zoneGen`，整视图重画（与 `foldGen` 相同的取舍，见决定 Q14）。

### 2.3 按 owner 的隐藏区域（挂 State）

```go
// LineRange 是半开的 Model 行区间 [Start, End)，与 diff.LineRange 同义（根包自定义一份，避免根包反向依赖 diff）。
type LineRange struct{ Start, End int }

func (s *State) SetHiddenAreas(owner string, rs []LineRange) // 整体替换；nil 表示删除
func (s *State) HiddenAreas(owner string) []LineRange       // 随编辑平移后的当前值
func (s *State) HiddenAreaAt(line int) (owner string, ok bool)
```

- 存储：每个区间存两个锚点（起始行行首 Left、结束行行尾 Right），在 `vl.sync/syncEdits` 后与折叠一起重新解析，做法同 `fold.s0/e`。
- **合并规则**：`State.syncHidden()` 取代 `syncFolds/applyFolds` 里直接调用 `vl.setHidden(fold.hidden(n))` 的两处（`folding.go:201`、`:344`），改为 `vl.setHidden(union(fold.hidden(n), 所有 owner 的区间))`。`setHidden` 本身不变，仍然只接受一组排好序、互不相交的区间。
- **用户折叠命令不碰 owner 隐藏区**：展开全部、按级别展开、`toggleFoldAt` 都只改 `s.fold`。`revealFolds`（`folding.go:488`）现在只看 `vl.hidden(l)` 判断选区是否进了隐藏行，要改成只看折叠隐藏的行，否则选区落进 diff 隐藏区时会白跑一遍展开逻辑。
- **选区进入隐藏区**：根包不会自动展开。绘制时沿用现有映射（隐藏行上的位置画在前一个可见行的行尾，`locate`，`viewlines.go:312`）；左右键沿用 `overFolds` 跳过。是否展开由 owner 决定：DiffView 每帧检查主选区的行，若在自己的隐藏区内就展开那一段（Monaco `ensureModifiedLineIsVisible`，`hideUnchangedRegionsFeature.js:72-85`）。
- **"显示隐藏的 N 行"占位**：owner 隐藏 `[a, b)` 的同时放一个 `ViewZone{AfterLine: a-1, Rows: 1, Fixed: true, ShowInHidden: true, OnPointer: …}`。zone 的 `Paint` 画分隔线、"⋯ 已隐藏 N 行"，以及"展开上方 20 行 / 全部 / 下方 20 行"按钮（Monaco 的 `CollapsedCodeOverlayWidget`，`revealLineCount` 默认 20）。`a == 0` 时 `AfterLine = -1`（lead）。`Version` = hash(N, 悬停按钮, 语言)。Monaco 的占位高 24px，我们是 1 行（偏差，决定 Q16）。
- 局部重绘：隐藏区的变化计入 `damageGlobal.hidGen`，整视图重画（同 `foldGen`）。

### 2.4 滚动

```go
// scroll.go（新文件）。像素均为逻辑像素；ScrollTop 0 = 文档最顶（含 PadTop），对齐 Monaco getScrollTop。

func (s *State) ScrollTop() float64          // = Viewport.scrollY() + PadTop
func (s *State) SetScrollTop(px float64)
func (s *State) ScrollLeft() float32
func (s *State) SetScrollLeft(px float32)
func (s *State) ScrollHeight() float64       // (Count-1)*lh + PadTop + Height（与 navigationHeight 同口径）

// 模型行与像素
func (s *State) LineTop(line int) float64             // 该行第一个文本行的顶部（getTopForLineNumber）；隐藏行返回其前一可见行块的底部
func (s *State) LineBlock(line int) (top float64, textRows, zoneRows int) // 行块 = 文本行 + 之后的 zone
func (s *State) LineAtTop() (line int, offset float32) // 顶部所在的 Model 行，以及距该行块顶部的像素
func (s *State) SetLineAtTop(line int, offset float32)
func (s *State) RowsOf(line int) int                   // 该行的文本行数（会测量这一行，换行时用）

type RevealWhere uint8
const (
	RevealNearest  RevealWhere = iota // 最少滚动（现有 revealRow）
	RevealCenter
	RevealCenterIfOutside
	RevealTop
)
func (s *State) RevealLine(line int, where RevealWhere)
func (s *State) RevealRange(start, end int, where RevealWhere) // 字节偏移，纵向

type ScrollEvent struct {
	ScrollTop, ScrollLeft       float64
	TopChanged, LeftChanged     bool
	Line   int     // LineAtTop
	Offset float32
}
// OnDidScroll 在滚动位置（按"Model 行 + 块内像素"比较，量出换行行数导致的 TopLine 变化不算）
// 变化后同步回调。检查点：View() 构建末尾（滚轮已处理）、finish()（输入、reveal）、paint 末尾。
// 回调里可以设置别的 State 的滚动位置，但不要设置本 State。
func (s *State) OnDidScroll(fn func(ScrollEvent)) (cancel func())
```

- 为什么在检查点比较、而不是在每个写入点触发：现有代码有十几处直接改 `Viewport.TopLine/OffsetY`，应用也能直接写导出的 `Viewport`。按锚点比较可以一网打尽，而且换行量出新行数时 `anchorTop` 引起的 `TopLine` 变化不会被误报成滚动。
- 帧内顺序：DiffView 在 build 阶段先对两侧都调用 `ViewW.Changed()`（触发 `finish`），让键盘 reveal 发生在 build 里，而不是拖到 paint（否则跟随侧会晚一帧）。

### 2.5 自定义行号

```go
// LineNumberFunc 返回 Model 行 line 的行号标签；ok=false 表示不显示。
type LineNumberFunc func(line int) (label string, ok bool)

// LineNumbersFunc 是当帧修饰器（与 LineNumbers/FontSize 同类）：maxChars 决定行号栏宽度
// （取 max(maxChars, lineNumbersMinChars)），gen 由调用者在映射变化时递增（进 damageGlobal）。
func (w *ViewW) LineNumbersFunc(fn LineNumberFunc, maxChars int, gen uint64) *ViewW
```

- `paintGutter`（`gutter.go:182`）在有 fn 时用标签替代 `opts.lineNumber`；`rowHashes` 对标签字符串做哈希（`damage.go:157` 那段），`numbersWidth` 用 `maxChars`。
- 内联模式下"删除行用原侧行号"：删除行是 zone，由 zone 的 `PaintGutter` 在行号栏位置画原侧行号（原侧行号随 zone 的 Version 一起变）。修改侧的普通行仍显示修改侧行号。是否像 GitHub 那样显示两列行号，见决定 Q11。
- **改动集中在 `gutter.go`，不碰 `options.go`**（不新增 `LineNumbersMode` 值，避免和 Task 8 冲突）。

### 2.6 内联删除行的绘制助手（根包）

DiffView 放在子包时，无法访问 `layoutLine/shapeSeg/TokensFor`，所以根包导出一个只读的绘制助手：

```go
// ModelLines 返回一个 zone 绘制回调：用 src 的语法高亮和当前主题画 src 的 [from, to) 行，
// 外加 decos（只用 Background/Inline），不换行；Version 由 (src 快照版本, 高亮代数, decos 指纹) 算出。
func ModelLines(src *Model, from, to int, decos []Decoration) (paint func(*paint.List, ZonePaint), version uint64)
```

内部复用 `w.vs.lines`（同一个 LineCache）和 `Highlighter.TokensFor`（为 src 单独分配一个 `ViewKey`）。

---

## 3. DiffView 组件

### 3.1 放在哪里

**建议 `ui/editor/diffview`**（决定 Q1；规格 6.2 的分层图已同步修改）。

理由：
1. Monaco 的 diff editor 也只依赖编辑器的公开 API（view zones、decorations、hidden areas、scroll）。放子包等于强制检验第 2 节的 API 是否够用，以后 MinCode 的"AI 待确认修改"等宿主也能用同一套 API 自己拼装。
2. 根包已经约 4.2 万行、正处在 M2 收尾期。DiffView 的大部分逻辑（对齐、导航、未改动区状态机、后台调度、还原）是纯逻辑，放子包可以先行开发、单独测试，`apitxt` 也有单独的 golden（`api/go/ui_editor_diffview.txt`，包注释必须带 Experimental 声明，否则 `apitxt -check` 失败）。
3. 代价：需要根包导出 `ModelLines`（2.6）；`diff.LineRange` 和 `editor.LineRange` 两个同义类型并存（子包里一行就能换算）。

### 3.2 公开 API

```go
package diffview // Experimental

type Mode uint8
const (SideBySide Mode = iota; Inline)

type Options struct { // 默认值对齐 Monaco diffEditor.js
	Mode                         Mode          // SideBySide（renderSideBySide: true）
	InlineBreakpoint             float32       // 900（renderSideBySideInlineBreakpoint）
	UseInlineWhenSpaceIsLimited  bool          // true
	IgnoreTrimWhitespace         bool          // true
	ComputeMoves                 bool          // false（experimental.showMoves）
	MaxComputationTime           time.Duration // 5s（diff.DefaultTimeout）
	Debounce                     time.Duration // 200ms（DiffEditorViewModel 的 RunOnceScheduler）
	HideUnchangedRegions struct {
		Enabled                 bool // false（Monaco 默认关；AI 待确认场景由宿主打开）
		ContextLineCount, MinimumLineCount, RevealLineCount int // 3, 3, 20
	}
	OriginalEditable       bool // false：原侧 Options.ReadOnly = true
	RenderIndicators       bool // true：+/− 标记
	RenderMarginRevertIcon bool // true
	RenderOverviewRuler    bool // true：两车道合并概览尺（Monaco 30px）
	DiffWordWrap           editor.WrapMode // "inherit"
}

type State struct {
	Original, Modified *editor.State // 两侧视图；内联模式下 Original 不显示，但保留（滚动、选区可以切回并排）
	Options Options
	// 未导出：最新结果、代数、未改动区展开状态、同步滚动的状态等
}

// New 用两个 Model 建立 diff。两个 Model 各自独立；修改侧 Model 可以同时开在别的编辑器里
// （diff 的装饰、zone、隐藏区都是 State 级的，不会漏过去）。
func New(original, modified *editor.Model, th *theme.Theme) *State
func (d *State) Close() // 取消后台计算、关闭两个 State

func (d *State) Result() (r diff.Result, upToDate bool)
func (d *State) OnDidUpdateDiff(fn func(diff.Result)) (cancel func()) // 规格清单 7 的 onDidUpdateDiff
func (d *State) Next() bool; func (d *State) Prev() bool; func (d *State) GoTo(i int)
func (d *State) Revert(i int) error                     // 用原文替换修改侧第 i 块，一个撤销步
func (d *State) ShowAllUnchanged(); func (d *State) CollapseAllUnchanged()

func View(c *ui.Context, d *State) *ViewW
func (w *ViewW) Mode(m Mode) *ViewW                     // 当帧覆盖
func (w *ViewW) Layout(opts ...layout.Option) *ViewW
func (w *ViewW) HunkActions(fn func(i int, ch diff.Change) []HunkAction) *ViewW // 扩展点
func (w *ViewW) Labels(original, modified string) *ViewW // 读屏名称与可选标题
func (w *ViewW) Changed() bool                          // 任一侧文本改了

type HunkAction struct {
	Label string; Icon *icon.Icon; Tooltip string
	Run   func()
}
```

### 3.3 两侧 Model 与只读

- 两侧总是两个 Model（规格清单 13："Diff 的两侧"）。原侧通常是私有 Model，或者模型池里的 `git:` URI；修改侧可以和主编辑器共享同一个 Model，此时编辑会实时同步，撤销历史也共享。
- 原侧默认 `ReadOnly`（Monaco `originalEditable: false`）；内联模式下不显示原侧。
- 强制两侧一致的选项：字号、`Padding`、`LineHeight`（缩放是全局的，`ZoomLevel` 本来就一致）。行高一致是"zone 按整行计"能精确对齐的前提。
- 照搬 Monaco 子编辑器的选项覆盖（`diffEditorEditors.js:105-170`）：`Folding=false`、小地图关闭、开启 HideUnchangedRegions 时关闭粘性滚动、内联模式下原侧不换行。DiffView 只写两个 State 的 `Options` 字段，不改 `options.go`。

### 3.4 后台计算（ctx 取消 + 防抖）

- 订阅两个 Model；任一侧有 `ModelChange` → 标记 `upToDate=false`，取消正在跑的计算（`cancel()`），重设 200 ms 防抖。
- 防抖到点（用 `c.WakeAt`，或者 `time.AfterFunc` 加 `ui.Invalidate`）→ 取两侧不可变快照，启动 goroutine 跑 `diff.Snapshots(ctx, o, m, opts)`，超时 `MaxComputationTime`，结果带上两侧快照版本和代数。
- 结果回到 UI 线程（`Invalidator`）：代数过期就丢弃；如果在计算期间又有编辑，就用 `applyEdits` 把结果映射到当前文本再用（Monaco `applyModifiedEdits/applyOriginalEdits`，`diffEditorViewModel.js:486-578`）。
- **防抖窗口内**同样用 `applyEdits` 把上一次结果映射到当前文本：编辑处以下的块整体平移，被编辑触及的块扩大并标为改动。这样装饰和对齐不会错位到别的行，等新结果到了再精确替换。
- `applyEdits` 是 `diffview` 内的纯函数 `applyEdits(r diff.Result, side diff.Side, ch editor.ModelChange) diff.Result`，可以单独测试。
- 大文件：`Model.LargeFile()` 时仍然计算（Monaco 也算），但 `ComputeMoves` 强制关闭；超时时 `Result.TimedOut` 在工具条上提示。

### 3.5 装饰映射（每次结果更新时）

| 来源 | 原侧（State 级，owner "diff"） | 修改侧 |
|---|---|---|
| 块的行 | `WholeLine` + `diffEditor.removedLineBackground`；gutter `MarginBackground` = `diffEditorGutter.removedLineBackground`；`Gutter.Sign` "−" | `insertedLineBackground`、"+" |
| `Inner` 字符区间 | `removedTextBackground`（+ 高对比时的 `removedTextBorder`），`NeverGrowsWhenTypingAtEdges` | `insertedTextBackground` |
| 概览尺 | 合并概览尺的左车道 `diffEditorOverview.removedForeground` | 右车道 `insertedForeground` |

整块删除或整块插入时，对面那侧没有行，靠对齐 zone 垫高并画斜纹（`diffEditor.diagonalFill`）。

### 3.6 同步滚动与对齐

**对齐块**：由 `Result.Changes` 得到交替出现的"未改动段"（原侧 `[o, o+n)` ↔ 修改侧 `[m, m+n)`，逐行一一对应）和"改动块"（`Change.Original` ↔ `Change.Modified`）。

**垫高（并排模式）**
- 改动块：`hO = Σ RowsOf(原侧块内行)`，`hM` 同理。较短的一侧在块末尾放 `Rows = |hO-hM|` 的空白 zone（`AfterLine = range.End-1`；空区间时是 `range.Start-1`，在开头时为 -1），`Paint` 画斜纹，`SuppressMouseDown`，`ShowInHidden`。与 Monaco 的 `diffEditorViewZones.js` 第 200-290 行一致。
- 未改动段：不换行时两侧行数相同，不需要 zone。开启换行时逐行比较两侧的 `RowsOf`，差值放在较短那行之后（Monaco 的 `wrappingZoneHeights`）。
- 换行是惰性的：只对**视口上下各一屏以内**的块用实测行数计算 zone（DiffView 每帧调用 `RowsOf` 测量这些行），更远的块按估计值算。测量结果变化时只替换受影响的 zone（每个 zone 的 ID = 块序号 × 2 + 侧，Version 不变，只有 Rows 变）。

**同步算法（以对齐块为锚，而不是像素直接相等）**

Monaco 直接让两侧的 scrollTop 相等（`diffEditorViewZones.js:333-350`），前提是两侧总高度处处一致。我们的换行是惰性的，远处行数是估计值，两侧上方的累计高度可能不相等，所以改为以对齐块为锚：

1. 选主导侧 L：本帧 `OnDidScroll` 报告变化的一侧；两侧都变时取有焦点的一侧，再不行取修改侧。用 `syncing` 标志防止回调互相触发。
2. `(lineL, offL) = L.LineAtTop()`，找到 lineL 所在的对齐块 B（在 `Changes` 上二分查找）。
3. 块内像素 `y = L.LineTop(lineL) - blockTop_L(B) + offL`。
4. 跟随侧 F：`F.SetScrollTop(blockTop_F(B) + min(y, blockHeight_F(B)))`。未改动段内逐行一一对应，等价于 `lineF = lineL - mL + mF` 加上相同的 offset。
5. 横向：`F.SetScrollLeft(L.ScrollLeft())`（Monaco 同样同步 scrollLeft）。
6. `blockTop`：非空区间取 `LineTop(Start)`；空区间（对面是插入）取 `LineTop(Start-1)` 加上 `Start-1` 的文本行高，即垫高 zone 的顶部。

当两侧估计值一致时，这个结果和 Monaco 完全相同；不一致时仍能保证视口内对齐，误差只在远处，滚到那里时会重新测量。

**内联模式**：只显示修改侧一个 View。删除行是 zone（`AfterLine = Modified.Start-1`，`Paint/PaintGutter` = `ModelLines(原侧 Model, Original.Start, Original.End, 删除字符装饰)` 加上原侧行号和 "−"），`Rows = Original.Len()`（第一版不换行，决定 Q9）；插入行是修改侧装饰。没有同步滚动。

### 3.7 未改动区域折叠与展开

- 由 `Result.UnchangedRegions(context=3, minHidden=3)` 得到区域；每个区域状态为 `{revealTop, revealBottom int; all bool}`，按区域首行锚点跟随编辑。
- 两侧对每个区域隐藏 `[Start+revealTop, End-revealBottom)`（两侧长度相同，对齐自动保持），并各放一个占位 zone（2.3）。点击"上方 / 下方 20 行"调整 reveal 计数，"全部"展开整段。
- 选区进入隐藏区时，展开该区域（Monaco `ensure*LineIsVisible`），查找跳转、转到行号同样适用。
- 新的 diff 结果到来时，按区域锚点把旧的展开状态迁移到新区域（Monaco 也是这么做，以免每次输入后重新折回去）。

### 3.8 差异导航

- `Next/Prev` 用 `Result.Next/Prev(side, line)`（`regions.go:64/85`，到头回绕）：以有焦点那侧的光标行为准，移动光标到块首，`RevealLine(…, RevealCenterIfOutside)`，展开相关隐藏区，并宣告（3.11）。
- 命令（`RegisterCommand`，when = `isInDiffEditor`）：`editor.action.diffReview.next/prev`，默认键 **Alt+F5 / Shift+Alt+F5**（VS Code 的 compareEditor.nextChange/previousChange）；`F7 / Shift+F7` 留给无障碍差异查看器（决定 Q13）。另有 `diffEditor.revert`、`diffEditor.switchSide`、`diffEditor.toggleCollapseUnchangedRegions`、`diffEditor.showAllUnchangedRegions`、`diffEditor.collapseAllUnchangedRegions`（id 与 Monaco 相同）。

### 3.9 概览尺

- 默认照搬 Monaco：并排模式的修改侧右边是一个 30px 宽的两车道合并概览尺（左原侧、右修改侧，`overviewRulerFeature.js:42`），用 `ui.Custom` 自绘，y 按修改侧的 `ScrollHeight` 和 `LineTop` 换算，原侧按对齐块映射到修改侧的 y。上面画视口滑块，点击和拖动会滚动修改侧（再同步到原侧）。两侧编辑器自己的概览尺车道数设为 0。
- 绘制记录做矩形集合比较（与 `fixedPaint` 同理），只在 diff 结果或滚动变化时重画。
- 内联模式：装饰的 `Overview` 进修改侧编辑器自己的概览尺。

### 3.10 每块"还原"与扩展点

- 默认动作：修改侧可编辑且 `RenderMarginRevertIcon` 时，在修改侧块首行的 gutter 装饰栏（或者删除块的垫高 zone 的 `PaintGutter`）画 "→" 箭头，点击执行 `Revert(i)`。`Revert(i)` = `Modified.Edit([]buffer.Edit{{Start: 块首, End: 块尾, Text: 原文}}, …)`，作为一个撤销步，然后立即重算。
- 扩展点 `HunkActions`：宿主提供每块的动作（AI 待确认修改的"接受 / 拒绝"、Git 冲突的"采用当前 / 采用传入"），画在块首行上方的一个 1 行 zone 里（类似 CodeLens 的按钮条）。提供 `HunkActions` 后不再画默认的还原箭头。`Run` 在 UI 线程同步执行，执行后 DiffView 立即重算。

### 3.11 主题 token

- **编辑器区域**：用编辑器主题的 Monaco 键 `diffEditor.insertedLineBackground / insertedTextBackground / removedLineBackground / removedTextBackground / *TextBorder / diagonalFill / unchangedRegionBackground / unchangedRegionForeground / unchangedRegionShadow / move.border`、`diffEditorGutter.*`、`diffEditorOverview.*`（默认值已生成）。
- **三套设计主题（antd / element / shadcn，明暗）**：新增组件令牌 `theme.CompDiffView`（`ui/theme/comp_diffview.go`，`deriveDiffView` 从全局令牌派生）：
  - `InsertedLineBg = Success.Bg`，`InsertedTextBg = Success.BgHover` 叠加约 40% 透明度，`RemovedLineBg = Error.Bg`，`RemovedTextBg = Error.BgHover`，概览尺 = `Success.Base / Error.Base`；
  - 占位条用 `FillTertiary`、`TextSecondary`；按钮用 `Primary`；垫高斜纹用 `Split`；
  - 暗色由令牌自动处理，不判断 `th.Dark`（`ui/AUTHORING.md` 第 5 节）。
- **优先级**：`View.Colors(...)` 显式覆盖 > 编辑器主题自己写的颜色（`Theme.OwnColor`，用户装的 VS Code 主题优先）> `CompDiffView` 令牌 > Monaco 默认值。
- 改 `ui/theme` 属于框架改动，需要和 min-39 或框架负责人协调（决定 Q12）；第一版可以先只用编辑器主题键。

### 3.12 无障碍

- 两侧 View 的读屏名称："原文（只读）— 文件名" / "修改后 — 文件名"（可用 `Labels` 覆盖），文字走 Locale。
- 导航和光标进入改动块时调用 `c.Announce("第 3 处差异，共 12 处：删除第 40–41 行，新增第 40–44 行", a11y.Polite)`，宣告词条加进 `Locale().A11y`。
- 占位 zone 和按钮条有 `Label`；它们的按钮是否作为 a11y 树节点暴露，取决于 zone 能否生成 a11y 子节点。第一版用宣告加命令代替（点击能做的事都有命令和快捷键）。
- 第二期：Monaco 的无障碍差异查看器（F7 打开，逐行列出 +/− 的只读列表）。

### 3.13 RTL

- 代码文本保持现有规则：基准方向为 LTR，行内做 bidi 重排（`lineOptions`，`paint.go:806`）。
- `c.IsRTL()` 时，**两侧窗格不镜像**（原文仍然在左，与 VS Code、GitHub 一致）；工具条、按钮条、占位条的按钮顺序和对齐跟随 RTL；合并概览尺仍在修改侧的外侧（决定 Q15）。

---

## 4. 分步实现计划

前提：
- 编辑器会话没有提交权限，根包要等 Task 8 评审后才冻结。**M2 改动只在主工作区、未提交**，所以：
  - **阶段 A（只新增文件）**：在基于 origin/main 的独立 worktree 里做（origin/main 有 `ui/editor/diff`，主工作区没有），文件全是新建的，M2 落地后可以无冲突合入。
  - **阶段 B、C（改根包现有文件）**：必须等 M2 提交（包括 `apitxt -w` 更新 golden）以后，基于新的 main 开分支。`options.go`、`minimap.go` 要等 Task 8 结束。
- 每一步结束的检查同 M2 的 Global Constraints：`go build ./... && go vet ./ui/editor/...`，windows/linux 的 vet，`go test ./ui/editor/... -count=1`，`gofmt -l`，改了渲染路径就跑 `TestViewDamageVerified` 和严格导航测试，`go run ./internal/cmd/apitxt -check`。

### 阶段 A：只新增文件（可以和编辑器会话并行）

| 步骤 | 文件 | 内容 | 测试 |
|---|---|---|---|
| A1 | `ui/editor/diffview/align.go` | 对齐块、垫高计算（输入 `diff.Result` 和 `rows func(side, line) int`）、块定位、同步滚动的锚点换算 | 表驱动加随机：不换行时垫高后两侧总行数相等；对每个视图行做"往返映射"的性质测试；空区间、开头、结尾 |
| A2 | `diffview/apply.go` | `applyEdits`（防抖窗口内映射旧结果） | 随机编辑后与重新完整计算比较：每块的行区间只能偏大、不能错位；Monaco 用例对照 |
| A3 | `diffview/regions.go` | 未改动区状态机（reveal 计数、迁移、按选区展开） | 状态迁移表、迁移规则 |
| A4 | `diffview/runner.go` | 后台调度（ctx 取消、防抖、代数、过期丢弃），时钟可注入 | 假时钟：连续输入只算一次；取消可见（`-race`）；过期结果被丢弃 |
| A5 | `diffview/doc.go`、`announce.go`、`revert.go`（计算 edits） | 包注释带 Experimental 声明；宣告文案；还原用的 edits | 单元测试 |
| A6 | `ui/editor/decorations_store.go` | 根包里的纯数据结构 `decorationStore`（排序、Tracker 平移、查询），**不接线** | 随机编辑 3000 步 × 四种 stickiness，与逐个 `Anchor.Resolve` 的参考实现逐条比较；1 万条装饰的基准 |
| A7 | `ui/editor/zones_store.go`、`hidareas_store.go` | zone 集合与隐藏区集合（锚点、有效行解析、并集），**不接线** | 与暴力实现比较 |

A6、A7 虽然在根包里，但只是新文件、不被引用。合入前要在主工作区 grep 一遍，确认没有新的重名。

### 阶段 B：根包接线（M2 提交后）

| 步骤 | 改动文件 | 内容 | 测试与门槛 |
|---|---|---|---|
| B1 装饰 | `model.go`（notify 里先平移）、`state.go`（Close 时清理）、新增 `decorations.go`（公开 API）、`paint.go`（三层绘制、Inline 覆盖 `runPaint`）、`gutter.go`（标记、gutter 背景、行号颜色）、`overview.go`（缓存与去重）、`damage.go`（行签名） | 2.1 | `TestDecorationsPaint*`（像素或绘制命令断言，含颜色间隙行、长行、换行行）；**新增 `TestDecorationsDamageVerified`**（严格校验：设置、平移、换 owner、只改一行的装饰时只脏一行）；`TestViewDamageVerified` 原样通过；性能：2.1 节的表，以及没有装饰时的 ABAB 比值 ≤ 1.10 |
| B2 隐藏区 | `folding.go`（`syncHidden` 取代两处 setHidden；`revealFolds` 只看折叠）、`state.go`（modelChanged 路径）、`damage.go`（`hidGen`）、新增 `hidareas.go` | 2.3 | 与折叠交叉的性质测试（展开全部不影响 owner 隐藏区；两者重叠、相邻、编辑跨边界）；`TestHiddenAreasDamageVerified`；光标左右键跳过 |
| B3 View zones | `viewlines.go`（zr、lead、extra、find、anchorTop 裁剪）、`paint.go`（layoutRows 产出 zone 行、zone 绘制、跳过 zone 行）、`input.go`/`dnd.go`（命中与 OnPointer）、`bracketpairs.go`、`bracketguides.go`、`guides.go`（跳过）、`damage.go`（zone 行指纹、`zoneGen`、Version==0 时的 op 指纹）、`viewstate.go`、新增 `zones.go`、`zone_fp.go` | 2.2 | **映射性质测试**：随机 zone + 折叠 + 换行 + 编辑，`find/viewOf/Count` 与暴力实现一致；导航（上下键、PageUp/Down、点击 zone 上下半）；**新增 `TestViewZonesDamageVerified`**（含 Version 递增、Version==0 兜底、Fixed zone 的横向滚动、顶部停在 zone 中间的滚动）；没有 zone 时 ABAB 比值 ≤ 1.10；1000 个 zone 时滚动 p99 ≤ 33 ms |
| B3' 小地图 | `minimap.go`（**Task 8 结束后**） | zone 行画成空白 | `minimap_test` 加 zone 用例 |
| B4 滚动 | 新增 `scroll.go`；`view.go`、`input.go`（finish）、`paint.go` 末尾的三个检查点 | 2.4 | OnDidScroll：滚轮、键盘 reveal、概览尺拖动、量出换行行数（**不应触发**）；LineTop 与 zone/隐藏区/换行的一致性；RevealLine 四种方式 |
| B5 行号 | `gutter.go`、`damage.go`、`view.go`（修饰器） | 2.5 | 渲染加指纹测试；行号栏宽度 |
| B6 绘制助手 | 新增 `modellines.go` | 2.6 | 与普通视图同一行的 glyph 序列一致；Version 随高亮完成而变 |

### 阶段 C：DiffView 视图层（B1–B6 之后，只在 diffview 子包里）

| 步骤 | 内容 | 测试与门槛 |
|---|---|---|
| C1 | 并排：两个 `editor.View` + `ui.Splitter`、选项覆盖、装饰、对齐 zone、同步滚动 | 无头 Driver：滚动一侧，另一侧对齐块顶部相同（像素误差 0）；换行开启时的视口内对齐；**新增 `TestDiffViewDamageVerified`**（两个编辑器同时严格校验：输入、防抖后结果到达、同步滚动、导航） |
| C2 | 未改动区折叠、占位 zone、展开按钮、选区展开 | 点击按钮后的隐藏区；查找跳转进隐藏区 |
| C3 | 导航命令、宣告、合并概览尺 | 键盘；宣告文案；概览尺点击与拖动 |
| C4 | 内联模式（删除 zone 用 `ModelLines`、原侧行号）与断点自动切换 | 渲染断言；宽度 < 900 时自动切换 |
| C5 | 还原与 `HunkActions` | 撤销一步回到原状；只读时不可还原 |
| C6 | `CompDiffView` 令牌（协调后）与三主题明暗截图 | `components_test`、对比度测试 |
| C7 | `examples/editor-demo` 增加 `-diff a b` 模式；必要时加 gallery 页 | 冒烟；性能：两侧各 10 万行、1000 块，滚动 p95 ≤ 16 ms；后台计算期间输入 p95 不变（与不开 diff 的 ABAB 比值 ≤ 1.10） |

---

## 5. 与提案草稿的差异

| 提案（min-3d） | 本稿 | 原因 |
|---|---|---|
| `SetDecorations` 挂哪里没写 | Model 存储与平移 + State 级过滤 | 编辑器会话的评审意见；修改侧共享 Model 时防止装饰外漏 |
| `ViewZone{AfterLine, Height, Paint}` | `Rows`（整行）+ `Version` / op 指纹 + `Fixed` + gutter 部分 + 指针事件 | 并进 Fenwick 树、局部重绘契约、"显示 N 行"占位要能点击 |
| `TopLine/SetTopLine(line, offsetPx)` | `ScrollTop/LineTop/LineAtTop/SetLineAtTop/OnDidScroll/RevealLine` | 同步算法需要块级像素；回调按锚点比较 |
| 隐藏区与折叠取并集 | 同上，并明确用户的展开命令不碰 owner 隐藏区、选区进入时由 owner 决定 | 编辑器会话意见；Monaco 行为 |

---

## 6. 风险与决定

### 风险

- **R1 基线**：M2 的大量改动只在主工作区、未提交，origin/main 上没有 `overview.go`、`minimap.go`、`injected.go`、`navigation_coords.go` 等文件。本稿的行号和接入点都以 M2 代码为准，**阶段 B、C 必须基于 M2 落地后的代码**；如果 M2 在落地前又有改动（Task 7 修复轮、Task 8 至 10），开工前要重新核对第 1 节和 B3 的审查清单。
- **R2 `find` 的语义扩展**（row 可能落在 zone 行上，lead 为负）会影响 24 处调用点（`bracketpairs.go:860`、`bracketguides.go:173`、`folding.go:343`、`dnd.go:91`、`minimap.go:231`、`input.go:658/676/701/850`、`navigation_coords.go:42`、`state.go:165`、`paint.go:166/167/448/657/665`、`viewlines.go` 中 9 处）。漏掉一处就可能出现越界或光标进入 zone，靠映射性质测试加严格导航测试兜底。
- **R3 局部重绘**：`TestViewDamageVerified` 在 scale1 和 1.25 上有已知的间歇性闪烁帧失败（M2 之前就存在）。我们的新测试要能区分"老问题"和"新回归"：失败时单独重跑，并比较是否为闪烁帧。
- **R4 惰性换行加对齐**：远处行数是估计值，垫高 zone 可能频繁变化，导致整视图重画。缓解：只在视口附近按实测值重算，其余用估计值且不更新。
- **R5 性能门槛**：新能力在不使用时必须走原来的快路径；用 M2 Task 1 的成对测量工具（`perfab/run.sh`）验证，不拿历史数字比较。
- **R6 并行开发冲突**：阶段 A 在根包里新增的文件可能和 M2 后续新增的标识符重名，合入前要 grep 检查。

### 决定（原待决问题，用户 2026-11-15 批准全部按默认建议）

| # | 问题 | 决定（默认建议） |
|---|---|---|
| Q1 | DiffView 放 `ui/editor/diffview` 还是根包（规格 6.2 写的是根包） | **子包**，同时修改规格 6.2 的分层图 |
| Q2 | view zone 高度按整行还是按像素 | **整行**（直接并进 Fenwick 树；DiffView 强制两侧行高一致，因此对齐是精确的）。像素高度以后有需要再加，届时要第二棵前缀和 |
| Q3 | zone 局部重绘契约 | **`Version != 0` 时信任调用者；`Version == 0` 时编辑器给 op 做指纹兜底** |
| Q4 | 装饰作用域 | **Model 存储 + 可选的 State 过滤**（`Model.SetDecorations` / `State.SetDecorations` 两个入口） |
| Q5 | `InlineStyle` 第一版是否支持粗体、斜体（会改变塑形、换行、长行块表） | **不支持**：第一版只有前景色、下划线、删除线、透明度 |
| Q6 | 规格清单 10 的"行前 / 行尾内容"（注入文字）是否放进这一期 | **不放**：留给 blame 内联、幽灵文字那一期，届时复用 `injected.go` 的间隙机制 |
| Q7 | 选区进入 owner 隐藏区时 | **根包不展开**；DiffView 自己展开（Monaco 行为） |
| Q8 | DiffView 里是否允许用户折叠、小地图、粘性滚动 | **照搬 Monaco**：折叠关闭、小地图关闭、开启隐藏未改动区时粘性滚动关闭 |
| Q9 | 内联模式的删除行是否换行 | **第一版不换行**（随文本横向滚动）；Monaco 会换行，列为第二期 |
| Q10 | 概览尺 | **Monaco 式 30px 合并概览尺**，两侧编辑器自己的概览尺车道设为 0 |
| Q11 | 内联模式的行号 | **单列**（修改侧行号）+ 删除 zone 里显示原侧行号；GitHub 式两列行号以后再做 |
| Q12 | 三主题令牌 `CompDiffView` 需要改 `ui/theme`（框架） | **加**，但需要框架负责人同意；同意之前先只用编辑器主题键 |
| Q13 | 快捷键 | **Alt+F5 / Shift+Alt+F5 上一处 / 下一处**；F7 / Shift+F7 留给第二期的无障碍差异查看器 |
| Q14 | zone 或隐藏区布局变化时的重绘粒度 | **整视图**（同 `foldGen`）；实测太频繁再细化 |
| Q15 | RTL 下两侧窗格是否镜像 | **不镜像**，只有工具条和按钮跟随 RTL |
| Q16 | "显示隐藏的 N 行"占位高度 | **1 行**（Monaco 是 24px） |
| Q17 | 移动块（ComputeMoves）的渲染 | **第一版不做**（Monaco 默认也关闭）；算法层已支持，留到以后 |
| Q18 | 默认是否显示还原箭头 | **显示**（修改侧可编辑时）；提供 `HunkActions` 时由宿主接管 |
| Q19 | 阶段 A 能否先在基于 origin/main 的 worktree 里开始 | **能**，只新增文件，M2 落地后再合入 |
