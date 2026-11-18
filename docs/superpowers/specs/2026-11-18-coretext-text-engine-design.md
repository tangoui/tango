# macOS 文字引擎改用 CoreText（不链接 go-text）设计

日期：2026-11-18 · 状态：待用户审阅

## 背景与目的

Tango 自绘应用的 macOS 二进制约 10.5 MB，Viva 只有 3.8 MB。按包统计代码量（`/tmp/pkgsize`，从 pclntab 汇总），Tango counter 的 4.25 MB 代码里，go-text 约占 780 KB：`font/opentype/tables` 340 KB、`harfbuzz` 242 KB，另有 `font`、`cff`、`bidi`、`segmenter`、`language` 等包。pclntab 等元数据随代码量同比例增长，所以 go-text 实际约占二进制的 15–18%。Viva 在 macOS 上直接用 CoreText 排版，排版相关代码只有约 22 KB。

用户决定：**macOS 构建默认用 CoreText 完成全部文字排版，不再链接 go-text**；其他平台不变。用户已接受 macOS 的断行、字距、字体回退会和其他平台有细微差别。

## 目标

1. `GOOS=darwin`（非 iOS）的默认构建里没有任何 `github.com/go-text/typesetting/...` 包。用 `go list -deps` 检查最小自绘应用（`examples` 里的 counter 类样本）来确认。
2. `ui/text` 的公开 API（`api/go/ui_text.txt`）不变，签名和语义都保持兼容，调用方（`ui`、`ui/editor`、终端、组件）不用改代码。
3. 现有功能在 CoreText 下全部可用：
   - 自动换行、`MaxWidth`、`MaxLines`、`Ellipsis`、`NoWrap`、`TrimSpaceRight`。
   - `ShapeRuns` 多样式混排，`Weight`、`Italic`（含合成斜体 `ItalicSkew`），`Mono`。
   - 双向文字：`Direction`、`Line.RTL`、`Caret`、`IndexAt`、`SelectionRects`、`LineOf`。
   - `Layout` 字段的含义不变：`Glyphs`、`GlyphCluster`、`GlyphRun`、`Runes`、`Lines`、`Ascent`、`LineHeight`。
   - `LineCache`（编辑器、终端），包括 `TabSize`、`Ligatures`、`Lang`、`Direction`，以及等宽快速路径。
   - `RegisterFont`（TTF/OTF/TTC，可变字体的 wght 轴实例化到 300–700）、`Families`、`UseSystemFonts`。
   - `SetDefaultLang` / `DefaultLang`：中日字形差异要按语言生效。
   - `Face.Metrics`、`HasRune`、`GlyphPath`、`RasterGlyph`、`SystemRasterized`。
   - `TrimCaches`、`TrimFontCaches`，以及现有的字体句柄回收（`ownership_darwin.go`）。
4. `-tags tango_gotext` 时，macOS 恢复到今天的行为（go-text 整形加 CoreText 光栅化），和改动前逐字节一致的实现路径。
5. 实测并在文档里写明体积收益和性能对比（见“验收”）。

## 非目标

- iOS、Windows、Linux、Android、js/wasm 继续用 go-text，代码路径不变。
- 不追求和 go-text 的输出逐像素一致。
- 不处理 `ui` 包经由 Upload 组件链接 `net/http`（连带 TLS、加密）的问题，那是另一项体积优化，单独立项。
- 不改升级到 Metal 的阈值和渲染器选择。

## 反目标（技术上能跑但算失败）

- 只用到 CoreText、CoreGraphics、CoreFoundation 的公开 API。不用 `libicucore` 这类没公开头文件的库（Mac App Store 审核风险）。
- 不引入 cgo，继续 `CGO_ENABLED=0` + purego。
- 编辑器输入、滚动的帧耗时和终端吞吐不能明显变差（数值见“验收”）。
- 不照搬其他框架（Viva、mygo、Wails、NSGo）的代码。只参考思路，以 Apple 官方文档为准。
- 不能因为 go-text 没了，就让 macOS 上的确定性测试（golden 图）静默失效。

## 约束

- Go 1.27，模块 `github.com/tangoui/tango`，purego 调 CoreText（已有 `engine_darwin.go`、`coretext_shape_darwin.go`、`nativeglyph_darwin.go`、`ownership_darwin.go`、`table_resource_darwin.go` 可复用）。
- `ui/text` 是唯一直接导入 go-text 的包，改动限制在这个包内（加上测试和 CI）。
- 构建标签：
  - CoreText 引擎文件：`//go:build darwin && !ios && !tango_gotext`
  - go-text 引擎文件：`//go:build !darwin || ios || tango_gotext`
  - 引擎无关的公共文件：不加标签
- 提交作者固定为 `tango <903300326@qq.com>`，不加 Claude 署名。

## 方案对比（用户已选 A）

| 方案 | 做法 | 取舍 |
|---|---|---|
| **A（已选）** | `ui/text` 拆成引擎无关的公共层，加两套引擎实现，用构建标签切换。macOS 的整形、断行、双向、字体全部用 CoreText；保留 `tango_gotext` 应急开关 | 唯一能彻底去掉 go-text 的方案，预计 macOS 二进制小约 1.5 MB。要新写一整套 CoreText 排版，并逐项对齐现有测试 |
| B | 保留 go-text 的断行和双向算法，只把 HarfBuzz 整形换成 CoreText | 改动小、风险低，但 go-text 的字体包会把 opentype 字体表解析器一起带进来，体积大多省不掉，估计只省一半 |
| C | 断行和双向算法由 Tango 自己用 Go 实现，CoreText 只负责整形 | 断行规则能和其他平台一致，但要自己维护 UAX #14 和 UAX #9 及其 Unicode 数据表，体积会回来一部分，也偏离“macOS 用系统排版”的方向 |

用户选 A 的理由：只有 A 达到“不链接 go-text”的目的，而且 macOS 已有约一半的 CoreText 代码（光栅化、字体句柄、短文本整形）可以复用。

## 已定的设计

### 1. 分层

- **公共层（不加标签）**：`Style`、`Options`、`Line`、`Layout` 及其方法（`Caret`、`IndexAt`、`LineOf`、`SelectionRects`、`Size`）、`Run`、`LineOptions`、`Direction`、`Weight`、语言标签、所有缓存（`layout_cache.go`、`prepared.go` 的缓存部分、`runs.go` 的缓存键）、`Face` 的公开方法外壳。
- **引擎接口（内部，不导出）**：公共层通过一个包内接口拿到“一段文字在给定宽度下排好的行”。每行包含：可见顺序的 run、每个 run 的方向和字形（id、x/y 位置、advance、对应的 rune 下标）、所用 `*Face`、行的 rune 范围、宽度和度量。具体签名由实现者决定，要求是两套引擎都能自然填充，公共层不出现任何 go-text 类型。
- **go-text 引擎**：现有的 `layout.go`、`linecache.go`、`compact.go`、`bidi.go`（算法部分）、`fontsxml.go`、`hvar_specialize.go`、`variation_prune.go`、`table_resource.go`、`simple_numeric.go`，以及 `engine_*.go` 里和 go-text 绑定的部分，归到 go-text 标签下。行为不变。
- **CoreText 引擎**：新文件，带 CoreText 标签。

### 2. CoreText 排版流程（每个段落，即按 `\n` 分开的块）

1. 按 `Run` 和样式构造 `CFAttributedString`。每段样式设 `kCTFontAttributeName`（由字体链得到的主字体）和 `kCTLanguageAttribute`（`resolveLang` 的结果，让中日字形按语言区分）。`Ligatures=false` 时关闭连字（`kCTLigatureAttributeName`=0）。基础方向用 `kCTParagraphStyleSpecifierBaseWritingDirection` 设置：`DirAuto` 对应 natural，`DirLTR`、`DirRTL` 显式设置。
2. **字体回退**：主字体的 `kCTFontCascadeListAttribute` 设成 Tango 自己的回退链（先放 `RegisterFont` 注册的字体，再放系统链，等宽文字用等宽链），保持现在“用户字体优先”的顺序。链里都找不到的字符交给 CoreText 系统回退。`UseSystemFonts(false)` 时，回退链只放注册字体。
3. **断行**：用 `CTTypesetterCreateWithAttributedString` 加 `CTTypesetterSuggestLineBreak`（宽度为 `MaxWidth`），逐行生成 `CTLine`。`NoWrap` 时整段一行。`MaxLines` 达到上限后截断。`Ellipsis` 时最后一行用 `CTLineCreateTruncatedLine`（尾部省略号），并设置 `Truncated`。行尾空白和 `TrimSpaceRight` 的行为要和现有测试一致。
4. **取结果**：遍历 `CTLineGetGlyphRuns`。CoreText 给出的就是可见顺序。每个 `CTRun` 读取字形、位置、advance、string indices（UTF-16 下标，要换算成 rune 下标）、`kCTRunStatusRightToLeft`，以及 run 属性里的 `kCTFontAttributeName`（实际使用的字体，可能是回退字体）。
5. **CTFont 映射到 `*Face`**：用 PostScript 名加 traits 做键，同一个 CTFont 必须得到同一个 `*Face`。原因是 `*Face` 是 `paint.GlyphSource`，GPU atlas 用它当键。新遇到的回退字体懒建 `*Face`，光栅化走现有的 `ctRaster`。
6. **填充 `Layout`**：`Glyphs`、`GlyphCluster`、`GlyphRun`、`Lines`（`Start`、`End`、`Top`、`Baseline`、`Width`、`RTL`），行高和 `Ascent` 沿用现有的 `Style.norm()` 规则（多 run 时取最大 `LineHeight`），和 go-text 引擎一致。
7. **双向文字的光标、命中测试和选区**：公共层的 `bidiLine` 现在依赖 go-text 给出的 embedding level。CoreText 引擎改为从 run 的可见顺序和方向生成同等信息（每个 rune 的视觉位置和 caret 边），喂给公共层。行为以现有的 bidi 测试为准。

### 3. 字体

- `Face` 的内部字段按引擎区分：CoreText 引擎保存 CTFont 句柄（复用 `ctFontOwner` 的所有权和回收），没有 go-text 的 `*font.Face`。
- `Metrics`：`CTFontGetAscent` / `CTFontGetDescent`，按字号缩放，与 `upem` 一致。
- `HasRune`：`CTFontGetGlyphsForCharacters`（处理 UTF-16 代理对）。
- `GlyphPath`：`CTFontCreatePathForGlyph`，用 `CGPathApply` 转成 `paint.Path`，注意 y 轴方向和单位要与现有约定一致。
- `RegisterFont`：`CTFontManagerCreateFontDescriptorsFromData` 从内存创建描述符，不注册到进程级字体管理器。TTC 取全部面。可变字体按 wght 300–700 实例化（`kCTFontVariationAttribute`）。weight 和 italic 从 traits 读取（复用 `cssWeightFromCT`）。
- 系统字体链沿用 `engine_darwin.go` 现有的族名列表和 `famSpec`。
- `Families`：顺序语义不变。

### 4. `LineCache`

- 缓存键、LRU、统计这些引擎无关的部分放到公共层。
- 一般路径：调用 CoreText 段落流程（单行、`NoWrap`）。`TabSize` 在构造属性串前展开，或用 `kCTTabStopsAttribute` 实现，由实现者选，但列位置要和现有测试一致。
- 等宽快速路径：全部 run 都是 `Mono`、连字关闭、文字是“简单文本”（复用 `simpleText`、`maybeRTL` 的判断）时，用 `CTFontGetGlyphsForCharacters` 加 `CTFontGetAdvancesForGlyphs` 按字符直接摆放，不建 `CTLine`。条件不满足时回退到一般路径。

### 5. 运行时开关在 CoreText 构建下的语义

- `Engine()` 返回 `"coretext"`。
- `UseSystemEngine(false)`：CoreText 构建里没有纯 Go 路径，所以调用不改变状态，返回之前的值（始终是 true），并在文档注释里写明。`TANGO_TEXT_ENGINE=go` 同样无效。需要纯 Go 确定性渲染的场合，用 `-tags tango_gotext` 构建。
- 现有的 `TANGO_TEXT_ENGINE=coretext` 短文本路径并入新引擎，在 CoreText 构建下不再有单独含义。

### 6. 测试与 CI

- 依赖纯 Go 路径的测试（目前 8 个文件调用 `UseSystemEngine(false)` 或设 `TANGO_TEXT_ENGINE=go`，3 个文件调用 `UseSystemFonts(false)`，18 个文件涉及 golden）：按测试内容分别处理。
  - 测排版语义的（断行位置、cluster、光标、选区）：改成两套引擎都能通过的断言，比如按位置关系、按单调性断言，不逐像素比对。
  - 必须逐像素比对 golden 的：加 `//go:build !darwin || ios || tango_gotext`，只在 go-text 引擎下跑。
- 新增只在 CoreText 构建下跑的测试，覆盖“目标 3”的每一项，至少包括：中英混排断行与禁则（“，。！？）”不在行首）、阿拉伯语和希伯来语混排的光标与选区、`MaxLines` 加省略号、`RegisterFont` 内存字体与 TTC、等宽快速路径与一般路径结果一致、回退字体映射到稳定的 `*Face`。
- CI 的 macOS 任务同时跑默认构建和 `-tags tango_gotext` 两套。
- 新增一个检查：默认 darwin 构建的依赖里不能出现 go-text（可以放进现有的 API 或 CI 检查脚本）。

### 7. 验收

1. `GOOS=darwin GOARCH=arm64 go list -deps` 检查最小自绘样本，没有 go-text；`-tags tango_gotext` 时有。
2. 体积：用 `scripts/size-report.sh` 口径（`-trimpath -ldflags='-s -w'`），给出最小自绘样本和默认自绘模板改前改后的字节数，写进 `docs/package-size-optimization.md`。不预设数字，以实测为准。
3. 性能：按现有六方测量口径（`outputs/tango-6way-full-20261117/bench`），同一批交错测 counter、quotes、list、grid，改前改后各 5 轮。要求：
   - 内存不比改前高。
   - 进程 CPU 增幅不超过 2 个百分点。
   - P99 不变差。
   - 首帧中位数不慢 20 ms 以上。
   - 编辑器的 `LineCache` 基准和 `ui/editor` 的性能测试，耗时增幅不超过 20%。
   - 超出时先报告用户，由用户决定是否接受。
4. 实机：macOS 上用 `automation`（`TANGO_LAUNCH_BACKGROUND=1`）打开 gallery 和编辑器示例，截图核对中文、英文、阿拉伯文、emoji 和混排，人工查看。
5. 全量测试：默认构建和 `tango_gotext` 构建都通过；Windows、Linux 交叉构建通过。

## 审查后补充的决定

下面几项是设计审查时提出的问题。标“答复”的是项目里已有答案的，标“我的决定”的由设计者拍板，用户审阅时可以推翻。

1. **引擎接口分两步，整形结果缓存（我的决定）**。为了保留现在“段落整形一次，换宽度只重新断行”的特性（flex 布局多次测量、拖动窗口时都依赖它），引擎接口分成两步：
   - prepare：段落 → 整形结果。CoreText 下是 `CTTypesetter`，和宽度无关。
   - break：给定宽度 → 行。
   
   prepare 的结果放进现有的 `prepared` 缓存，沿用 `preparedBudget` 预算，由 `clearPrepared`、`TrimCaches` 释放。缓存里的 CoreText 对象要有明确的所有者，释放时 `CFRelease`。
   
   `Measure`、`Width` 和 `Shape` 走同一条路径，宽度必须一致。数字快速路径（`simple_numeric.go`、`ShapeInto`）在 CoreText 下保留：用主字体的字形和 advance 直接摆放，但只在字体对这组字符没有 kerning 时启用，否则走一般路径。非等宽 ASCII 一律走一般路径，不逐字符摆放，保证 kerning 和宽度一致。

2. **最低系统版本（答复）**。计划用到的 API 最低要求为：

   | API | 最低版本 |
   |---|---|
   | `kCTLanguageAttributeName` | macOS 10.15 |
   | `CTFontManagerCreateFontDescriptorsFromData` | macOS 10.13 |
   | `CTTypesetterSuggestLineBreak`、`CTRunGetStringIndices`、`kCTFontCascadeListAttribute`、`kCTFontVariationAttribute`、`CTFontCreatePathForGlyph` | macOS 10.5 |

   以上版本均以 SDK 头文件的 `CT_AVAILABLE` 为准。`tan` 默认的最低系统版本是 11.0，全部满足，不需要降级方案。

3. **`UseSystemFonts(false)` 的语义（我的决定）**。CoreText 的系统级联关不干净。这个模式下，Tango 自己按回退链为每段文字挑字体（用 `HasRune` 判断），只把注册字体放进 cascade list。少数仍被 CoreText 系统回退接管的字符，按“允许有系统回退”处理，不保证显示成方框。依赖精确缺字结果（方框或 `.notdef`）的测试，归入“仅 go-text”。

4. **注册字体也走 CoreText 光栅化（我的决定）**。这和“macOS 用系统文字栈”的方向一致：
   - `RegisterFont` 注册的字体在 CoreText 构建下 `SystemRasterized()` 为 true，带系统的字形渲染和 gamma。
   - CoreText 不能解析的字体数据，`RegisterFont` 直接返回错误，不回退到 go-text。
   - 可变字体只实例化 wght 轴（300–700），其他轴取字体默认值。不再沿用 go-text 路径里 `opsz` 固定为 16 的做法，改由 CoreText 按字号自动选。

5. **双向文字的对齐范围（我的决定）**。只保证常见混排正确：阿拉伯文、希伯来文与拉丁文、数字、中文、标点混排时，光标、命中测试和选区与 CoreText 给出的可见顺序一致。显式方向控制符（LRE/RLE/LRI/RLI/FSI/PDF/PDI）的效果和嵌套层级，交给 CoreText 自己的实现，不承诺和 go-text 的层级逐项相同。依赖精确 embedding level 的测试（如 `TestBidiExactLevels`）归入“仅 go-text”。CoreText 构建另写一套行为测试，覆盖光标是否单调、选区是否连续、行首行尾的命中。

6. **合成斜体（我的决定）**。判断某个字体有没有真斜体，看实际使用字体（含回退字体）的 `CTFontGetSymbolicTraits` 里的 italic 位，结果记在对应 `*Face` 的 `Italic` 字段里。请求斜体时用描述符匹配真斜体，不让 CoreText 自己合成倾斜。只有字体没有真斜体时，才由 Tango 按 `ItalicSkew` 倾斜，避免倾斜两次。

7. **样式相同的相邻 run 一起整形（我的决定）**。属性串只在真正影响整形的样式边界（字体、字号、字重、斜体、语言、连字）处切分属性，所以跨 token 的连字和 kerning 都保留。`GlyphRun`（字形属于哪个 `Run`）用字形的 string index 换算出 rune 下标，再查它所在的 `Run` 得到，不加自定义属性，以免把整形切断。

8. **等宽快速路径的前提（我的决定）**。只在同时满足以下条件时启用：
   - 全部 run 都是 `Mono`，连字关闭。
   - 文字通过 `simpleText` 检查，不含 RTL。
   - 每个字符都能在主等宽字体里取到字形（`CTFontGetGlyphsForCharacters`，不缺字）。
   - 主等宽字体没有 kerning：没有 `kern`/`kerx` 表，或者 GPOS 里没有 kern 特性。这个结果每个字体查一次后缓存。

   只要有一个字符需要回退字体，或含 CJK、emoji，整行走一般路径。验收标准是“快速路径和一般路径的结果一致”测试。

## 留给实现者决定

- 引擎接口的具体签名和文件拆分方式。
- UTF-16 到 rune 下标的换算实现。
- 截断和省略号的细节，只要满足现有测试。
- Tab 展开的方式（属性还是预处理）。
- `*Face` 映射缓存的键和回收策略，但必须和 `TrimFontCaches` 及现有所有权模型协同。
- 测试迁移时，每个测试归入“语义断言”还是“仅 go-text”的具体判断。

## 风险

- CoreText 的断行规则和 go-text（UAX #14）不同：中文禁则、URL、数字串的断点会有差异。这是用户已接受的平台差异，但现有测试要改成表达意图的断言。
- 双向文字的光标语义：CoreText 的 run 粒度和 go-text 的 level 粒度不同，需要专门测试。
- 编辑器性能：`CTLine` 的创建成本高于 go-text 的复用整形器，等宽快速路径必须覆盖绝大多数代码行。
- 回退字体的 `*Face` 稳定性：如果映射不稳定，GPU atlas 会重复上传，内存上涨。
