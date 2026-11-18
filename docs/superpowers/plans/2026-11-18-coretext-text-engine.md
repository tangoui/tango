# macOS CoreText 文字引擎实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** macOS（非 iOS）默认构建的 `ui/text` 全部用 CoreText 实现，不再链接 `github.com/go-text/typesetting`；加 `-tags tango_gotext` 时恢复今天的行为。

**Architecture:** 现有排版流程保持不变：先整形，得到每个 rune 的 advance、字形、bidi level 和段落；再用 `breakAt` 按宽度断行；最后放置字形、生成光标、处理省略号。这条流程本身和引擎无关，真正依赖 go-text 的只有五个“原语”：字体 `Face`、逐 rune 选字体、整形 `shapeItems`、断行机会 `lineEnds`、段落 bidi level `paraLevels`。本计划把这五个原语抽到按构建标签区分的文件里，再写 CoreText 版本。缓存、断行、放置、光标、选区、省略号、`LineCache` 的一般路径，两套引擎共用同一份代码。

**Tech Stack:** Go 1.27.3，purego（`CGO_ENABLED=0`）调用 CoreText、CoreFoundation、CoreGraphics；断行机会用 CoreFoundation 的 `CFStringTokenizer`（`kCFStringTokenizerUnitLineBreak`）。

**Spec:** `docs/superpowers/specs/2026-11-18-coretext-text-engine-design.md`

## 与设计的一处偏差（需用户在审阅计划时确认）

设计 §2 第 3、4、7 步写的是“用 `CTTypesetterSuggestLineBreak` 断行、`CTLineCreateTruncatedLine` 截断、从 CTLine run 推光标”。计划改为保留 Tango 共用的断行、省略号和光标代码，CoreText 只提供以下原语：

- 断行机会：`CFStringTokenizer`。这是 CoreFoundation 的公开 API，macOS 10.5 起可用，按 Unicode 断行规则给出候选断点。
- bidi level：由 CTLine 的 run 方向推导。
- 整形：按“单一方向、单一字体”的片段建 CTLine。

理由有四条：
1. 设计里“整形一次、按宽度断行”的缓存要求，用共用代码最容易满足。
2. 现有大量关于截断、省略号、光标、选区的测试可以原样用在两套引擎上。
3. 逐 rune 选字体沿用 Tango 自己的回退链，CoreText 只整形本来就有字形的字符，所以设计决定 3（`UseSystemFonts(false)` 不被系统回退接管）自然成立。
4. 设计里“macOS 用系统断行规则”这一用户诉求仍然满足，因为候选断点来自系统。

设计文档已同步改写这一节。

## Global Constraints

- 工具链：`GOTOOLCHAIN=go1.27.3 GOWORK=off CGO_ENABLED=0`。所有 go 命令都带这三个变量；本机 `GOPROXY=off`，只用本地模块缓存。
- 只用 Apple 公开 API。禁止 `libicucore` 和任何私有符号。用到的 API 最高要求 macOS 10.15（`kCTLanguageAttributeName`），低于 `tan` 默认最低版本 11.0。
- 构建标签（最终状态）：
  - CoreText 文件：`//go:build darwin && !ios && !tango_gotext`
  - go-text 文件：`//go:build !darwin || ios || tango_gotext`
  - 共用文件：不加标签
  - Task 2–5 期间，CoreText 文件用临时标签 `//go:build darwin && !ios && tango_coretext`，go-text 文件不加标签；Task 6 切换为最终状态。
- `ui/text` 的公开 API 不变：`go run ./internal/cmd/apitxt -check` 通过，`api/go/ui_text.txt` 不改。
- 共用文件不得导入 `github.com/go-text/...`。
- 不照搬 Viva、mygo、Wails、NSGo 的代码；CoreText 用法以 Apple 文档为准。
- 提交作者：`git -c user.name=tango -c user.email=903300326@qq.com commit`，不加 Co-Authored-By。
- GUI 规则：功能验证用 `automation.Launch`（默认带 `TANGO_LAUNCH_BACKGROUND=1`）或 `cua-driver`，不抢前台。性能测量要先向协调会话 min-78 申请前台时段，测完通知它。
- 性能门槛（来自设计“验收”）：
  - 内存不比改前高。
  - 进程 CPU 增幅不超过 2 个百分点。
  - P99 不变差。
  - 首帧中位数不慢 20 ms 以上。
  - `LineCache` 基准和 `ui/editor` 性能测试的耗时增幅不超过 20%。
  - 超出时停下报告用户，不自行放宽。

## Review Focus

1. **emoji、ZWJ 序列、国旗（RI 对）、组合附加符号**：一个字形对应多个 rune、多个字形对应一个 rune 时，`GlyphCluster` 必须指向簇的第一个 rune，光标不能落在簇中间。归 Task 4 测试 `TestCTShapeClusters`。
2. **UTF-16 代理对**：CoreText 的 string index 是 UTF-16 下标，BMP 以外的字符（emoji、𠀀）换算成 rune 下标时不能错位。归 Task 4 测试 `TestCTShapeAstral`。
3. **空串、只有 `\n` 的文本、只有空格或 tab 的行**：不崩溃，行数和 `LineHeight` 与 go-text 引擎相同。归 Task 4 测试 `TestCTShapeEmptyParagraphs`。
4. **注册字体缺字**：用 `RegisterFont` 注册的字体没有某个字符，同时 `UseSystemFonts(false)` 时，该字符不能变成系统字体字形，也不能导致 panic。归 Task 2 测试 `TestCTRegisteredFontMissingRune`。
5. **同一个 CTFont 反复出现**：多次 `Shape` 中同一个回退字体，必须得到同一个 `*Face`（pointer 相等），否则 GPU atlas 会重复上传。归 Task 4 测试 `TestCTFallbackFaceStable`。

---

## 文件结构（最终状态）

| 文件 | 职责 | 标签 |
|---|---|---|
| `ui/text/level.go`（新） | `type level uint8`，替代共用代码里的 `bidi.Level` | 无 |
| `ui/text/bidi.go`（改） | 只保留与引擎无关的部分：`itemize`、`lineLevels`、`visualOrder`、`bidiLine`、选区、语言标签 | 无 |
| `ui/text/bidi_gotext.go`（新） | `paraLevels`（go-text bidi） | go-text |
| `ui/text/bidi_coretext_darwin.go`（新） | `paraLevels`（CTLine run 方向） | CoreText |
| `ui/text/layout.go`（改） | 删除 `shapeItems`、`lineEnds`、`fontmap` 里的 go-text 部分；保留 `layoutRuns`、`breakAt`、放置 | 无 |
| `ui/text/shape_gotext.go`（新） | `shapeItems`、`shapeRange` 的 go-text 版，含 `compactShaper`、`segm` | go-text |
| `ui/text/shape_coretext_darwin.go`（新） | `shapeItems` 的 CoreText 版 | CoreText |
| `ui/text/breaks_gotext.go`（新） | `lineEnds`（go-text segmenter） | go-text |
| `ui/text/breaks_coretext_darwin.go`（新） | `lineEnds`（`CFStringTokenizer`） | CoreText |
| `ui/text/font.go`（改） | `Face` 公开方法外壳、`family`、`registry`；`Face` 的引擎字段改成嵌入 `faceImpl` | 无 |
| `ui/text/face_gotext.go`（新） | `faceImpl{face *font.Face, ...}`，以及 `Metrics`、`HasRune`、`outline`、`parseFaces` 的 go-text 实现 | go-text |
| `ui/text/face_coretext_darwin.go`（新） | `faceImpl{ct *ctFontOwner, ...}`，以及 CoreText 版的同名函数和 `RegisterFont` 的数据解析 | CoreText |
| `ui/text/ctbind_darwin.go`（新） | 从 `engine_darwin.go` 移出的 purego 绑定和 CF 工具函数（`cfStr`、`goStr`、`cfDict` 等），再加新增绑定 | `darwin`（两套引擎都用） |
| `ui/text/engine_darwin.go`（改） | 系统字体链；go-text 专属部分（`goTextFace`、`shapingTables`）移到 `engine_gotext_darwin.go` | `darwin` |
| `ui/text/engine_gotext_darwin.go`（新） | 用 CT 字体表构造 go-text Face 的部分 | `darwin && (ios \|\| tango_gotext)`，Task 6 定稿 |
| `compact.go`、`hvar_specialize.go`、`variation_prune.go`、`table_resource.go`、`table_resource_darwin.go`、`fontsxml.go`、`simple_numeric.go`、`coretext_shape_darwin.go` | 原样保留，只加 go-text 标签 | go-text |
| `ui/text/numeric_coretext_darwin.go`（新） | 数字快速路径的 CoreText 版 | CoreText |
| `ui/text/linecache.go`（改） | 缓存和一般路径共用；快速路径改成调用 `fastShape` 原语 | 无 |
| `ui/text/linefast_gotext.go`、`ui/text/linefast_coretext_darwin.go`（新） | 等宽快速路径的两版实现 | 对应标签 |
| `ui/text/engine_mode_gotext.go`、`ui/text/engine_mode_coretext_darwin.go`（新） | `UseSystemEngine`、`Engine` 的两种语义 | 对应标签 |
| `scripts/check-gotext-free.sh`（新） | 检查默认 darwin 构建的依赖里没有 go-text | — |
| `.github/workflows/ci.yml`（改） | macOS 加一个 `-tags tango_gotext` 测试步骤，加依赖检查 | — |

---

### Task 1：把 go-text 隔离到 `*_gotext.go`（行为不变）

**Files:**
- Create: `ui/text/level.go`、`bidi_gotext.go`、`shape_gotext.go`、`breaks_gotext.go`、`face_gotext.go`、`ctbind_darwin.go`、`engine_gotext_darwin.go`
- Modify: `ui/text/bidi.go`、`layout.go`、`font.go`、`linecache.go`、`prepared.go`、`runs.go`、`engine.go`、`engine_darwin.go`
- Test: `ui/text/gotext_isolation_test.go`（新）

**Interfaces:**
- Produces（共用代码调用、每个引擎都要实现的原语，签名固定）：
  - `type level uint8`，加上 `func (l level) rtl() bool`
  - `func paraLevels(runes []rune, a, b int, dir Direction, lv []level) (base level, mixed bool)`
  - `func lineEnds(runes []rune, a, b int) []int`：返回 `runes[a:b]` 里每个允许断行的位置（rune 下标，表示“在该下标之前可以断行”），升序，末尾含 `b`
  - `func shapeItems(runes []rune, items []item, fms []*fontmap, adv []float32) []sg`：语义与现有实现相同
  - `item.lang` 改为 `string`（BCP 47 标签）
  - `fontmap` 去掉 `feats []shaping.FontFeature`，改为 `noLiga bool`；`byFF`、`inlineFont`、`remember`、`fromFont`、`ResolveFace` 移入 `shape_gotext.go`，作为 go-text 专用的辅助类型 `gtFaceMap` 嵌入
  - `type faceImpl struct{...}` 嵌入 `Face`。每个引擎提供 `func (f *Face) metricsUnits() (upem, asc, desc float32)`、`func (f *Face) hasRune(r rune) bool`、`func (f *Face) unitOutline(id uint32) *paint.Path`；公开方法 `Metrics`、`HasRune`、`GlyphPath` 留在 `font.go`，调用这三个函数
  - `func parseFaces(name string, data []byte) ([]*Face, error)`：`RegisterFont` 调用；参数从 `font.Resource` 改为 `[]byte`

- [ ] **Step 1：写失败测试 `TestSharedFilesImportNoGoText`**

在 `gotext_isolation_test.go` 里用 `go/parser` 解析 `ui/text` 下所有不以 `_gotext.go` 或 `_test.go` 结尾的 `.go` 文件，排除已经带 go-text 标签的旧文件（`compact.go`、`hvar_specialize.go`、`variation_prune.go`、`table_resource.go`、`table_resource_darwin.go`、`fontsxml.go`、`simple_numeric.go`、`coretext_shape_darwin.go`、`engine_linux.go`、`engine_windows.go`、`engine_gotext_darwin.go`）。断言这些文件的 import 里没有以 `github.com/go-text/` 开头的路径，违反时报出文件名。

- [ ] **Step 2：运行，确认失败**

运行：`go test ./ui/text/ -run TestSharedFilesImportNoGoText -count=1`
预期：FAIL，列出 `bidi.go`、`layout.go`、`font.go`、`engine.go`、`linecache.go`、`prepared.go`、`runs.go`、`engine_darwin.go`

- [ ] **Step 3：按上面的 Interfaces 搬移代码**

规则：只搬移、改类型，不改算法。`lineLevels`、`visualOrder`、`bidiLine` 改用 `level`。`prepareText` 里的 `language.NewLanguage` 改为直接传字符串。`linecache.go` 快速路径里用到 go-text 的部分（`fastPath` 的 `split`、`unit`、`shapeUnit`、`enforced`、`fastScript`）整体移到 `linefast_gotext.go`，共用文件只留一个调用 `c.fastShape(line, rs, sopt)`，其签名不变，函数体在 `linefast_gotext.go`。purego 绑定和 CF 工具函数移到 `ctbind_darwin.go`（标签 `darwin`）。`engine_darwin.go` 里构造 go-text Face 的部分（`goTextFace` 调用点、`shapingTables`、`ctRaster.tables`）移到 `engine_gotext_darwin.go`，暂不加标签。

- [ ] **Step 4：运行测试，确认通过且行为不变**

运行：`go test ./ui/text/ ./ui/ ./ui/editor/... -count=1 && go vet ./ui/text/ && go run ./internal/cmd/apitxt -check`
预期：全部 PASS；apitxt 无差异

- [ ] **Step 5：交叉构建**

运行：`for t in linux/amd64 windows/amd64 darwin/amd64 android/arm64; do GOOS=${t%/*} GOARCH=${t#*/} go build ./ui/... || exit 1; done`
预期：全部成功

- [ ] **Step 6：提交**

```bash
git add ui/text
git commit -m "refactor(text): isolate go-text behind engine primitives"
```

---

### Task 2：CoreText 字体（`faceImpl`、注册、系统链）

**Files:**
- Create: `ui/text/face_coretext_darwin.go`、`ui/text/face_coretext_darwin_test.go`
- Modify: `ui/text/ctbind_darwin.go`（新增绑定：`CTFontManagerCreateFontDescriptorsFromData`、`CTFontCopyPostScriptName`、`CTFontGetGlyphsForCharacters`、`CFDataCreate`）、`ui/text/engine_darwin.go`（`load` 在 CoreText 构建下构造 CT-only Face）
- 本 Task 的 CoreText 文件都用临时标签 `darwin && !ios && tango_coretext`

**Interfaces:**
- Consumes: Task 1 的 `faceImpl` 三个函数签名、`parseFaces(name string, data []byte) ([]*Face, error)`
- Produces:
  - `faceImpl{own *ctFontOwner; upem, asc, desc float32; italic bool}`
  - `func faceForCTFont(f uintptr) *Face`：按 PostScript 名 + 字重 + 斜体 + 变体坐标去重，返回同一个 `*Face`；Task 4 用它把 run 字体映射到 Face
  - `func (f *Face) ctFont(size float32) uintptr`：按字号取 CTFont（复用 `ctRaster.font`），Task 4 用它整形

- [ ] **Step 1：写失败测试**（`go test -tags tango_coretext`）

```go
func TestCTRegisterFontFromData(t *testing.T)        // 用 testdata 里的 TTF（没有就从 /System/Library/Fonts 读一个 .ttf 字节）注册为 "T"；Families() 第一项是 "T"；Face.HasRune('A') == true；Metrics(16) 的 ascent > 0 且 descent > 0
func TestCTRegisterFontRejectsGarbage(t *testing.T)  // RegisterFont("bad", []byte("not a font")) 返回非 nil error
func TestCTRegisterTTCAllFaces(t *testing.T)         // /System/Library/Fonts/PingFang.ttc 字节注册后，同一族下有多个字重（len(faces) >= 2）
func TestCTGlyphPathMatchesAdvanceBox(t *testing.T)  // 'H' 的 GlyphPath(…,16) 非空，包围盒高度在 (0, 16) 之间，y 轴向下（与 go-text 版约定一致：顶部 y 为负）
func TestCTRegisteredFontMissingRune(t *testing.T)   // Review Focus 4：UseSystemFonts(false)，只注册拉丁字体；Shape("A中", …) 不 panic；"中" 对应字形的 Src 不是任何系统族的 Face
func TestCTFaceDedup(t *testing.T)                   // 同一 CTFont 两次调用 faceForCTFont 返回同一指针
func TestCTTrimFontCachesKeepsLiveFaces(t *testing.T) // 先 Shape 出含回退字体的 Layout，再调 TrimFontCaches()：faceForCTFont 的去重表清空（之后再取到的 Face 是新指针），但旧 Layout 里的 Face 调 RasterGlyph 仍返回 ok == true
```

- [ ] **Step 2：运行，确认失败**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT(Register|GlyphPath|RegisteredFont|FaceDedup)' -count=1`
预期：编译失败，或因 `faceImpl` 未实现而 FAIL

- [ ] **Step 3：实现**

- 注册：`CFDataCreate` 加 `CTFontManagerCreateFontDescriptorsFromData`，不调用 `CTFontManagerRegister*`。
- 可变字体：wght 取 300–700 内字体支持的实例（复用 `ctVariation`、`withVariation`），其他轴保持默认。
- 字重、斜体：字重用 `ctWeight`；斜体看 `ctFontGetSymbolicTraits & kCTFontItalicTrait(1)`。
- `hasRune`：用 `CTFontGetGlyphsForCharacters` 处理 UTF-16 代理对，字形 id 为 0 视为缺字。
- 轮廓：`unitOutline` 复用 `ctRaster.unitPath`。
- 光栅化：继续走 `ctRaster`，`SystemRasterized()` 返回 true。

- [ ] **Step 4：运行，确认通过**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT' -count=1 && go test ./ui/text/ -count=1`
预期：两次都 PASS（默认构建仍是 go-text）

- [ ] **Step 5：提交**

```bash
git add ui/text
git commit -m "feat(text): CoreText faces and in-memory font registration (tango_coretext)"
```

---

### Task 3：CoreText 断行机会与 bidi level

**Files:**
- Create: `ui/text/breaks_coretext_darwin.go`、`ui/text/bidi_coretext_darwin.go`、`ui/text/breaks_bidi_coretext_darwin_test.go`
- Modify: `ui/text/ctbind_darwin.go`（新增：`CFStringTokenizerCreate`、`CFStringTokenizerAdvanceToNextToken`、`CFStringTokenizerGetCurrentTokenRange`、`CTTypesetterCreateWithAttributedString`、`CTTypesetterCreateLine`、`CTRunGetStatus`、`CTRunGetStringRange`）

**Interfaces:**
- Consumes: Task 1 的 `lineEnds`、`paraLevels` 签名
- Produces: 两个原语的 CoreText 实现，以及辅助函数 `func utf16Index(runes []rune, a, b int) (toRune []int32)`（UTF-16 下标到 rune 下标的换算表）；Task 4 复用这个辅助函数

- [ ] **Step 1：写失败测试**

```go
func TestCTLineEndsChinesePunct(t *testing.T) // "你好，世界。再见" 的 ends 里不含 "，" 和 "。" 的下标（它们不能出现在行首），含末尾 len
func TestCTLineEndsLatinWords(t *testing.T)   // "hello world foo" 的 ends == []int{6, 12, 15}
func TestCTLineEndsAstral(t *testing.T)       // "a😀b" 的下标按 rune 计，最大值 == 3
func TestCTParaLevelsPureLTR(t *testing.T)    // "abc" -> base 0，mixed == false，lv 全为 0
func TestCTParaLevelsHebrewInLatin(t *testing.T) // "abc שלום def"：base 0，mixed == true，希伯来字母的 level 为奇数，拉丁字母为偶数
func TestCTParaLevelsRTLBase(t *testing.T)    // DirRTL 下 "abc" -> base 为奇数
func TestCTParaLevelsDigitsInRTL(t *testing.T) // "שלום 123"：数字的 level 为偶数且大于 0，visualOrder 后数字仍按 1,2,3 从左到右
```

- [ ] **Step 2：运行，确认失败**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT(LineEnds|ParaLevels)' -count=1`
预期：FAIL，原语未定义

- [ ] **Step 3：实现**

- `lineEnds`：对 `runes[a:b]` 建 CFString，用 locale 为 nil 的 `kCFStringTokenizerUnitLineBreak(3)` tokenizer 遍历，每个 token 的结束位置换算成 rune 下标，即为一个断点。
- `paraLevels`：对段落建带基础方向段落样式的 attributed string，用 `CTTypesetterCreateLine` 建整段一行，读每个 run 的 `CTRunGetStatus & kCTRunStatusRightToLeft(1)` 和 string range。level 规则：
  - RTL run 取“大于 base 的最小奇数”。
  - LTR run：base 为奇数时取 base+1；base 为偶数时，被 RTL run 夹在逻辑顺序中间的取 base+2，否则取 base。
- `mixed` 的判定：存在非 base 的 level。
- `DirAuto` 的 base：由 CoreText 判出的第一个强方向字符决定，用 `kCTWritingDirectionNatural` 后看第一个 run 的方向。

- [ ] **Step 4：运行，确认通过**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT' -count=1`
预期：PASS

- [ ] **Step 5：提交**

```bash
git add ui/text
git commit -m "feat(text): CoreText line-break opportunities and bidi levels (tango_coretext)"
```

---

### Task 4：CoreText 整形 `shapeItems`

**Files:**
- Create: `ui/text/shape_coretext_darwin.go`、`ui/text/shape_coretext_darwin_test.go`

**Interfaces:**
- Consumes: Task 1 的 `shapeItems`、`item`、`fontmap.resolve`；Task 2 的 `faceForCTFont`、`(*Face).ctFont`；Task 3 的 `utf16Index`
- Produces: `shapeItems` 的 CoreText 实现。完成本 Task 后，`-tags tango_coretext` 下 `Shape`、`ShapeRuns`、`LineCache`（一般路径）、省略号全部可用

- [ ] **Step 1：写失败测试**

```go
func TestCTShapeBasicLatin(t *testing.T)      // Shape("Hello", Style{Size:16}, Options{})：len(Glyphs)==5，GlyphCluster == [0 1 2 3 4]，Width > 0 且等于各 rune advance 之和
func TestCTShapeKerningAffectsWidth(t *testing.T) // Width("AV") < Width("A") + Width("V")（系统字体有字距调整）
func TestCTShapeNoLigatures(t *testing.T)     // LineCache Ligatures:false 时 "ffi" 的字形数 == 3；Ligatures:true 时 <= 3
func TestCTShapeClusters(t *testing.T)        // Review Focus 1："é"、"👨‍👩‍👧"、"🇨🇳" 每个字形的 GlyphCluster 都是簇首 rune；Caret(1) 对 ZWJ 序列返回的 x 与 Caret(0) 或簇尾一致，不落在簇中间
func TestCTShapeAstral(t *testing.T)          // Review Focus 2："a😀b"：len(Runes)==3，'b' 的字形 GlyphCluster == 2
func TestCTShapeEmptyParagraphs(t *testing.T) // Review Focus 3：""、"\n"、"\n\n"、"   "、"\t" 不 panic；"\n\n" 有 3 行；LineHeight 与 Style.norm().LineHeight 相等
func TestCTShapeMixedCJK(t *testing.T)        // "Tango 中文"：中文字形的 Src 是中文字体族的 Face，拉丁字母不是
func TestCTFallbackFaceStable(t *testing.T)   // Review Focus 5：两次 Shape("中", …) 的 Glyphs[0].Src 指针相同
func TestCTWrapAndEllipsis(t *testing.T)      // MaxWidth 窄于文字宽度时 len(Lines) > 1；MaxLines:1, Ellipsis:true 时 Truncated == true，最后一个字形是 '…'
func TestCTItalicNoDoubleSkew(t *testing.T)   // 系统字体 Italic:true：有真斜体的 Face 的 Glyph.Skew == 0；用一个只有正体的注册字体时 Skew == ItalicSkew
func TestCTLangAttributeCJK(t *testing.T)     // "直" 在 Lang:"ja" 与 Lang:"zh-Hans" 下的字形 ID 不同（PingFang 与 Hiragino 的字形差异，或同一字体的 locl），任一满足即可
func TestCTBidiCaretAndSelection(t *testing.T) // "abc שלום def" 宽度足够不换行：希伯来字母内部 Caret(i) 的 x 随 i 增大而递减；SelectionRects(4, 8) 恰好返回 1 个矩形，宽度等于这 4 个希伯来字母的 advance 之和；IndexAt 点在行首左侧返回 0，点在行尾右侧返回 len(Runes)
func TestCTTabsExpand(t *testing.T)           // LineCache TabSize:4，Mono："a\tb" 中 'b' 的 X 等于 4 个空格宽
```

- [ ] **Step 2：运行，确认失败**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT(Shape|Fallback|Wrap|Italic|Lang|Tabs)' -count=1`
预期：FAIL

- [ ] **Step 3：实现 `shapeItems`**

对每个 item（单一方向、单一样式），用 `fms[it.run].resolve` 逐 rune 选字体，把相邻同 Face 的 rune 合成一段。每段处理如下：

1. **建 attributed string**，属性有三项：
   - `kCTFontAttributeName` = `face.ctFont(it.st.Size)`
   - `kCTLanguageAttributeName` = `it.lang`
   - `noLiga` 为真时，`kCTLigatureAttributeName` = 0
2. **方向**：用段落样式强制单一方向，等于 item 的方向。
3. **建行取 run**：`CTLineCreateWithAttributedString`，然后遍历 run。
4. **逐个 run 填字形**：从 run 属性取实际字体 → `faceForCTFont`，得到 `sg.face`。CoreText 偶尔会自己换字体，所以必须按实际字体映射。再读字形、位置、advance、string indices：
   - indices 经 `utf16Index` 换成 rune 下标，即 `sg.cluster`。
   - `xoff` = 位置减去笔位累计值，`yoff` = -位置 y。
   - advance 累加到 `adv[cluster]`。
5. **RTL 段**：输出的 run 和字形已经是可见顺序，与 go-text 版 `shapeItems` 的约定一致（RTL item 的字形按可见顺序给出）。
6. **没有任何字体时**：`fms[0].primary()==nil`，沿用共用的 `approxAdvance` 分支。

- [ ] **Step 4：运行全部 `ui/text` 测试（CoreText 构建）**

运行：`go test -tags tango_coretext ./ui/text/ -count=1`
预期：`TestCT*` 全部通过。其余失败的测试逐个分成两类：
- 测排版语义的：修改断言，让两套引擎都通过。
- 依赖 go-text 精确输出的：记下名字，留给 Task 6 加标签。

在 `ui/text/ENGINE_TESTS.md`（临时文件，Task 6 删除）里记录清单。

- [ ] **Step 5：运行 `ui`、`ui/editor`（CoreText 构建）**

运行：`go test -tags tango_coretext ./ui/ ./ui/editor/... -count=1`
预期：PASS；失败项按 Step 4 的规则处理并记录

- [ ] **Step 6：提交**

```bash
git add ui
git commit -m "feat(text): CoreText shaping for Shape, ShapeRuns and LineCache (tango_coretext)"
```

---

### Task 5：CoreText 快速路径（数字标签、等宽行）

**Files:**
- Create: `ui/text/numeric_coretext_darwin.go`、`ui/text/linefast_coretext_darwin.go`、`ui/text/fastpath_coretext_darwin_test.go`
- Modify: `ui/text/ctbind_darwin.go`（`CTFontCopyAvailableTables` 已有）

**Interfaces:**
- Consumes: Task 1 的 `simpleNumericLayout(s string, st Style, opt Options, version int) *Layout`、`simpleNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout`、`(*LineCache).fastShape(line string, runs []Run, opt Options) *Layout`；Task 2 的 `(*Face).ctFont`
- Produces: 三个函数的 CoreText 实现。条件不满足时返回 nil，由调用方走一般路径

- [ ] **Step 1：写失败测试**

```go
func TestCTNumericFastMatchesGeneral(t *testing.T) // "12345.67"、"-0.5"、"100%" 下，simpleNumericLayout 与 shapeRunsUncached 的 Width、Glyph ID、X 相同（X 误差 1e-3 以内）
func TestCTNumericFastSkipsKerningFont(t *testing.T) // 对有 kern/kerx 表或 GPOS kern 特性的字体，simpleNumericLayout 返回 nil
func TestCTMonoFastMatchesGeneral(t *testing.T)    // Mono、Ligatures:false，ASCII 代码行 "func main() { return 42 }"：fastShape 与 shapeRunsUncached 的 Glyphs 和 X 相同，且 c.stats.fast 增加
func TestCTMonoFastFallsBackOnCJK(t *testing.T)    // "x := \"中文\"" 时 fastShape 返回 nil，c.stats.slow 增加
func TestCTMonoFastFallsBackOnRTL(t *testing.T)    // 含希伯来字母时 fastShape 返回 nil
```

- [ ] **Step 2：运行，确认失败**

运行：`go test -tags tango_coretext ./ui/text/ -run 'TestCT(Numeric|Mono)' -count=1`
预期：FAIL

- [ ] **Step 3：实现**

- **无字距判定**：每个 `*Face` 判定一次，结果缓存在 `faceImpl.noKern`。`CTFontCopyAvailableTables` 里没有 `kern`、`kerx`，并且没有 `GPOS`（有 GPOS 时保守判为有字距），才算无字距。
- **前提条件**：
  - 数字路径：字符集为 `simpleNumericText` 的范围。
  - 等宽路径：全部 run 是 `Mono`，`noLiga` 为真，`simpleText` 成立，`!maybeRTL`。
  - 每个字符在主字体里取到的字形 id 都不为 0。
- **摆放**：满足条件时，用 `CTFontGetGlyphsForCharacters` 加 `CTFontGetAdvancesForGlyphs` 逐字符摆放，tab 交给共用的 `expandTabs`。

- [ ] **Step 4：运行，确认通过**

运行：`go test -tags tango_coretext ./ui/text/ -count=1`
预期：PASS

- [ ] **Step 5：提交**

```bash
git add ui/text
git commit -m "feat(text): CoreText numeric and monospace fast paths (tango_coretext)"
```

---

### Task 6：切换默认、引擎开关语义、测试归类、CI 与依赖检查

**Files:**
- Modify: 所有 `*_coretext_darwin.go` 的标签改为 `darwin && !ios && !tango_gotext`；所有 `*_gotext.go` 以及 Task 1 列出的旧 go-text 文件改为 `!darwin || ios || tango_gotext`；`engine_gotext_darwin.go` 改为 `darwin && (ios || tango_gotext)`
- Create: `ui/text/engine_mode_gotext.go`、`ui/text/engine_mode_coretext_darwin.go`、`scripts/check-gotext-free.sh`
- Modify: `ui/text/engine.go`（`UseSystemEngine`、`Engine` 的函数体移入两个 mode 文件，doc 注释留在共用文件里并写明 macOS 语义）、`ui/text/font.go`（包注释写明 macOS 默认用 CoreText）、`.github/workflows/ci.yml`、`ENGINE_TESTS.md` 清单中的测试文件（加标签），删除 `ENGINE_TESTS.md`

**Interfaces:**
- Consumes: Task 1–5 的全部原语
- Produces: 最终构建标签；`Engine()` 在 macOS 默认构建下返回 `"coretext"`；`UseSystemEngine(false)` 在该构建下不改状态，返回 `true`

- [ ] **Step 1：写失败测试**（默认标签，在 macOS 上运行）

```go
// ui/text/engine_mode_coretext_darwin_test.go
func TestEngineIsCoreText(t *testing.T)          // Engine() == "coretext"
func TestUseSystemEngineFalseIsNoop(t *testing.T) // UseSystemEngine(false) 返回 true；之后 Engine() 仍为 "coretext"
// ui/text/gotext_isolation_test.go 追加
func TestDefaultDarwinBuildHasNoGoText(t *testing.T) // 只在 darwin && !ios 下运行：exec `go list -deps -f '{{.ImportPath}}' .`（cwd 为 ui/text，带 GOFLAGS 继承），输出里没有 "github.com/go-text/"；在 -tags tango_gotext 下跳过
```

- [ ] **Step 2：运行，确认失败**

运行：`go test ./ui/text/ -run 'TestEngineIsCoreText|TestUseSystemEngineFalseIsNoop|TestDefaultDarwinBuildHasNoGoText' -count=1`
预期：FAIL（默认仍为 go-text）

- [ ] **Step 3：改标签、实现开关语义、给 `ENGINE_TESTS.md` 里的测试加 `//go:build !darwin || ios || tango_gotext`**

`TANGO_TEXT_ENGINE` 在 CoreText 构建下忽略。`scripts/check-gotext-free.sh` 的内容：

```bash
#!/usr/bin/env bash
# Fails if the default macOS build of the self-drawn UI links go-text.
set -euo pipefail
deps=$(GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go list -deps ./ui/... ./uihost/...)
if grep -q '^github.com/go-text/' <<<"$deps"; then
  echo "go-text is linked into the default darwin build:" >&2
  grep '^github.com/go-text/' <<<"$deps" >&2
  exit 1
fi
```

`ci.yml`：在 macOS 的 test 矩阵后面加一步 `bash ../../scripts/ci-go-test.sh -tags tango_gotext ./ui/...`（条件 `runner.os == 'macOS'`）；在 api 或 lint 任务里加一步 `bash scripts/check-gotext-free.sh`。

- [ ] **Step 4：全量验证**

运行：

```bash
go test ./... -count=1 -p 1 -timeout 40m
go test -tags tango_gotext ./ui/... -count=1
bash scripts/check-gotext-free.sh
go run ./internal/cmd/apitxt -check
for t in linux/amd64 windows/amd64 darwin/amd64 android/arm64 ios/arm64; do GOOS=${t%/*} GOARCH=${t#*/} go build ./ui/... || exit 1; done
```

预期：全部成功。iOS 交叉构建如果因为 cgo 需要特殊环境而失败，改用 `scripts/smoke.sh ios` 的构建步骤验证，并记录结果。

- [ ] **Step 5：提交**

```bash
git add ui/text scripts/check-gotext-free.sh .github/workflows/ci.yml
git commit -m "feat(text): CoreText is the default macOS text engine; tango_gotext restores go-text"
```

---

### Task 7：体积、性能、实机验收与文档

**Files:**
- Modify: `docs/package-size-optimization.md`（新增一节，写改前改后的字节数）、`docs/dev-builds.md`（`TANGO_TEXT_ENGINE`、`UseSystemEngine` 在 macOS 上的新语义，`tango_gotext` 标签）、`CHANGELOG.md`（未发布 → 变更）
- 无新增代码；如果实测超出门槛，停在 Step 3 报告用户

- [ ] **Step 1：体积**

运行：用 `scripts/size-report.sh` 的口径（`-trimpath -ldflags='-s -w'`），在 origin/main 和本分支各构建最小自绘样本与 `tan init -template ui` 样本（darwin/arm64），记录字节数，并用 `/tmp/pkgsize`（pclntab 按包汇总）确认没有 go-text 的包。
预期：本分支没有 go-text 的包；把字节数写进 `docs/package-size-optimization.md`，不预设数值。

- [ ] **Step 2：基准**

运行：`go test ./ui/text/ -run '^$' -bench 'LineCache|Layout' -count=5`，在 origin/main 和本分支各跑一次，用 `benchstat` 对比；`go test ./ui/editor/ -run 'Perf' -count=1`，同样两边各跑一次。
预期：耗时增幅不超过 20%。超出时停下报告用户。

- [ ] **Step 3：同批性能测量（要先向 min-78 申请前台时段）**

运行：`outputs/tango-6way-full-20261117/bench` 的 runner，构建两个 Tango 变体（改前、改后），counter、quotes、list、grid 各 5 轮交错。
预期：内存不升，进程 CPU 增幅不超过 2 个百分点，P99 不变差，首帧中位数慢不超过 20 ms。超出时停下报告用户。

- [ ] **Step 4：实机截图核对**

运行：`TANGO_E2E=1` 的 automation 测试，打开 `examples/ui-gallery` 和编辑器示例，截图包含中文、英文、阿拉伯文、emoji、混排、选区的页面。
预期：截图人工查看无乱码、无重叠、光标位置正确；截图路径写进提交说明。

- [ ] **Step 5：文档并提交**

```bash
git add docs/package-size-optimization.md docs/dev-builds.md CHANGELOG.md
git commit -m "docs(text): CoreText engine on macOS, size and performance results"
```
