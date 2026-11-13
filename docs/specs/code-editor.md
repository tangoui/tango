# 代码编辑器（ui/editor）

状态：已批准（2026-11-12，用户授权由设计者自审后开工）。

## 1. 意图

MinCode 要**彻底摆脱 WebView**，工作台改用 Tango 自绘 UI。现有编辑器是 WebView 里的 Monaco（`mincode/vue-app` 中约 74 个文件、2.9 万行与之相关），是迁移中最难、也最关键的一块。本项目在 Tango 的自绘 `ui/` 里做一个代码编辑器，**功能上完整替代 MinCode 对 Monaco 的全部用法**，性能不低于 Monaco。

为什么是"摆脱 WebView"而不是"更快"：目标是让 MinCode 能跑在 Tango 支持的所有平台（含移动端、鸿蒙），不受各平台 WebView 差异约束。因此**功能完整替代优先**，性能是硬门槛但不是第一驱动力。

## 2. 目标 / 非目标 / 反目标

**目标**

- 第 4 节清单里的能力**全部做齐**之后才接入 MinCode（用户明确选择"做齐"，不分批上线）。
- 数据格式与 Monaco 兼容：Monarch 语法、Monaco 主题规则、代码片段、JSON Schema 关联、快捷键规则与 when 表达式。MinCode 插件现有贡献（`languages`、`editorThemes`/`themes`、`snippets`、`jsonValidation`、`keymaps`、`menus`、`formatters`、`linters`、`languageFeatures`、`editorFrameworks`）**原样可用**。
- 内置 Monaco 自带的约 80 种语言高亮（由 Monaco 的 Monarch 定义转换而来）。
- 大文件可用，LSP 能力完整（通过 provider 接口接入）。
- 纯 Go，桌面不引入 cgo。

**非目标**

- 编辑器内不实现 LSP 客户端、调试适配器、Git：只提供接口与视图，进程与协议由宿主（MinCode 的 Go 后端，已有 `lsp-service`、调试、Git）负责。
- 不做 MinCode 工作台本身的迁移（侧栏、终端、插件界面、笔记里的 CodeMirror），那是后续单独的项目。
- 不追求 API 与 Monaco 的 TypeScript 签名一一对应；对齐的是**概念和数据格式**，Go 侧 API 按 Tango 风格设计。
- 不做 TextMate 语法、Tree-sitter（以后可通过可替换的 Highlighter 接口加）。
- **不做移动端（用户确认）**：本项目只在桌面（macOS / Windows / Linux）验收。触控编辑（选区拖柄、长按菜单、软键盘、移动端 IME、惯性滚动）另立项目；本项目只在输入与命中测试接口上不排斥触控（例如选区、光标操作经命令而非直接绑定鼠标事件）。

**反目标（技术上能用但算失败）**

- 大文件卡顿、输入延迟明显高于 Monaco。
- 插件作者需要改写已有的 Monarch 语法或主题才能用。
- 某个语言的高亮颜色/分词与 Monaco 明显不一致（同一主题下）。
- 照搬 mygo `ui/editor` 或 monaco-vim 的代码（只借思路，见 `docs/architecture.md` 与"Duet/Tango 不照搬"原则）。

## 3. 约束

- Go 1.27，模块 `github.com/tangoui/tango`，MIT。
- 桌面无 cgo；新依赖只允许纯 Go。本设计新增外部依赖仅两个：`github.com/dlclark/regexp2`（MIT）、一个纯 Go JSON Schema 库（候选 `github.com/santhosh-tekuri/jsonschema`，Apache-2.0；最终选择留给实现者，须纯 Go、支持 draft-07 与 2020-12）。
- 自绘 `ui/` 现状：即时模式 API + 保留状态（`ui.State`），CPU 光栅化（`ui/paint`），文字排版 `ui/text`（go-text/typesetting，**一段文字只支持单一 Style**，按整串缓存），IME 经 `Window.SetTextInput`。上屏（Canvas Surface）另有设计 `docs/specs/canvas-surface.md`，编辑器依赖它才能在真实窗口里验收；在它完成前用无头 `Driver` 测试。
- `ui/AUTHORING.md` 规定组件不得绕开基础层；本项目需要扩展基础层（第 6.1 节），按规定单独列出改动。
- 不照搬 mygo / monaco-vim / Monaco 的实现代码。Monaco 的 **Monarch 语法定义数据**（MIT）作为数据转换后内置，需在 THIRD_PARTY_NOTICES 中声明。

## 4. 必须对齐的能力清单

来源：2026-11-12 对 `mincode/vue-app/src` 的清点（`monaco-runtime.js` 注册的 features、`languages.register*` 调用、编辑器实例 API、装饰类名、插件扩展点）。

| # | 分组 | 能力 |
|---|---|---|
| 1 | 基础编辑 | 多光标（含列选择）、行操作（复制/移动/删除行、合并行等）、单词与驼峰分词移动、注释切换（行/块）、自动缩进与缩进调整、括号/引号自动闭合与包裹、剪贴板、拖放与粘贴、撤销/重做（含光标撤销、版本号 `versionId`/`alternativeVersionId` 语义）、IME 组字、只读模式 |
| 2 | 显示 | 行号（含相对行号，供 Vim）、折叠（基于缩进 + provider）、sticky scroll、小地图、自动换行、显示空白字符、括号配对着色与缩进参考线、括号匹配、同词高亮、Unicode 可疑字符高亮、链接检测、颜色装饰与颜色选择器、鼠标滚轮缩放、内边距、当前行高亮、概览标尺（诊断/查找结果） |
| 3 | 高亮与主题 | Monarch 解释器 + 约 80 种内置语言 + `.vue`（复用 HTML 规则）；插件贡献语言；LSP 语义高亮（semantic tokens）叠加；Monaco 主题（`base`/`inherit`/`rules`/`colors`）与插件主题 |
| 4 | 查找与导航 | 查找/替换（正则、大小写、全词、选区内、保留大小写）、跳转到行、跳转到符号、按语法扩大/收缩选区、跳转到下一个错误 |
| 5 | 语言功能（provider） | 补全（含触发字符、snippet 插入与占位符跳转）、悬停、参数提示、定义/类型定义/实现/引用（含 peek 结果列表）、文档符号（供大纲面板）、同符号高亮、重命名（含预检）、Code Action（含灯泡与快速修复）、CodeLens、Inlay Hints、格式化（文档/选区/输入时/粘贴时）、诊断标记（`setModelMarkers` 语义）、联动编辑（linked editing）、**内联补全（AI 灰字）** |
| 6 | JSON | JSON 解析与诊断、按 schema 校验/补全/悬停（`jsonValidation` 扩展点：文件模式 → schema） |
| 7 | Diff | 左右并排与内联两种视图、行级 + 单词级差异、原文只读/新文可编辑、未改动区域折叠、差异导航、`onDidUpdateDiff` 等价事件。用于 AI 待确认修改、Git 冲突解决 |
| 8 | Vim | normal/insert/visual(行/块)/replace 模式、动作与操作符、文本对象、计数、寄存器（含系统剪贴板）、宏、标记、`.` 重复、搜索、ex 命令（含宿主自定义 ex 命令，对应现有 `defineEx`）、状态栏信息 |
| 9 | 调试 | 断点/条件断点/日志断点的槽位（glyph margin）交互、暂停行与箭头、行内变量值、调试悬停（宿主提供内容） |
| 10 | 其他装饰 | Git blame 侧栏（侧栏文字 + 行背景）、通用装饰 API（行/区间/行尾/行前内容、侧栏、概览标尺、稳定性 stickiness） |
| 11 | 快捷键与命令 | 命令注册、编辑器动作（带标签，可进命令面板）、快捷键规则（含组合键）、上下文键与 when 表达式（兼容 MinCode 现有预设 `domain/keybindings`） |
| 12 | 菜单 | 右键菜单（内置项 + 宿主/插件贡献项，按 when 过滤） |
| 13 | 模型 | 多个视图共享一个文本模型（同一文件开两个编辑器、Diff 的两侧）、模型池（按 URI 取/建/销毁）、语言切换、视图状态保存/恢复（滚动、光标、折叠） |
| 14 | 使用场景 | 项目主编辑器、Diff 编辑器、只读查看器（SFTP、数据库、插件结果页）、数据库查询编辑器、设置页里的 JSON 编辑 |

## 5. 方案选择

考虑过三种整体方案（2026-11-12 与用户讨论）：

| 方案 | 要点 | 取舍 |
|---|---|---|
| **一、Monaco 数据格式兼容，Go 原生实现**（选定） | 内部原生设计（rope、快照、增量分词）；对外概念与数据格式对齐 Monaco | 保住插件生态与 80 种语言规则，MinCode 迁移"换语言不换结构"；需要 JS 兼容正则 |
| 二、Zed/mygo 式全新设计 | API 干净，自有词法器 | 插件语法/主题需转换或重写；80 种语言要自己写 |
| 三、拼现成库（chroma 等） | 起步最快 | chroma 分类与 Monaco 主题不对应、不支持增量、插件 Monarch 不可用——破坏插件生态，排除（chroma 可作以后的后备高亮，见 6.4） |

正则引擎另做了选择。实测 Monaco 0.56 的 84 个语言定义：约 3700 个正则中 148 个（4%，涉及 54 种语言）使用前瞻 `(?=`/`(?!`，无后顾、无反向引用；其中约 110 个前瞻在末尾，约 40 个在中间。候选：A 全用 regexp2；B 全用标准库并手工改写（第三方语法无法覆盖，排除）；C 自写 JS 正则引擎（留作升级路线）；**D 混合（选定）**：编译时自动判定，标准库能接受且语义一致的走 `regexp`（线性时间），其余走 regexp2（加超时）。

## 6. 已确定的设计

### 6.1 基础层扩展（ui/text、ui/paint、Runner）

编辑器需要、但现有基础层没有的能力，作为基础层改动单独提交：

- **多样式文本段**：一行内按区间指定颜色、字重、斜体、下划线（波浪线用于诊断）、删除线、背景。
- **按行排版缓存**：以（行内容哈希、样式段、字体参数）为键缓存字形，编辑一行只重排该行。等宽字体走快速路径（固定前进宽度、跳过换行算法），但必须正确处理 CJK 宽字符、制表符、零宽字符、组合字符、emoji 与连字（`fontLigatures` 选项）。
- **局部重绘**：Runner/Renderer 支持脏矩形，只重绘变化区域（光标闪烁、单行编辑、滚动时只画新露出的行加位图平移）。
- **GPU 渲染是必需的（用户确认）**，由独立项目 `docs/specs/gpu-renderer.md` 提供（Metal / D3D11 / OpenGL，Go 语法着色器编译器）。编辑器只产出 `paint.List`（含新增的"区域平移"提示，用于滚动），不直接调用图形接口；按行字形缓存与 GPU 字形图集协同（字形光栅化规则共用）。

### 6.2 分层

```
ui/editor            View（编辑器组件）、DiffView、Vim、命令/快捷键/菜单、装饰、provider 接口
  ├─ editor/buffer   rope 文本 + 不可变快照 + 撤销历史 + 版本号 + 锚点（装饰随编辑移动）
  ├─ editor/monarch  Monarch 解释器（状态栈、@attr 展开、cases、$S/$#、next/@push/@pop、nextEmbedded）
  ├─ editor/regex    混合正则（regexp ⇄ regexp2 自动选择、JS→Go 语法预处理、超时与统计）
  ├─ editor/langs    内置语言：转换后的 Monarch JSON（go:embed）+ 语言配置（括号、注释、自动闭合、缩进规则、wordPattern）
  ├─ editor/theme    Monaco 主题规则解析与 token→样式解析（带缓存）
  ├─ editor/json     JSON 解析/诊断 + JSON Schema 校验/补全/悬停（实现成内置 provider）
  ├─ editor/diff     行级与单词级 diff 算法
  └─ editor/when     when 表达式解析与求值（上下文键）
```

包名与拆分粒度为建议，实现者可调整，但 `buffer`、`monarch`、`regex` 必须不依赖 `ui`，可独立测试与基准测试。

### 6.3 文本模型

- rope（分块平衡树），编辑复制 O(log n) 路径，快照不可变、跨 goroutine 无锁读取（分词、diff、provider 在后台读快照）。
- 偏移与位置：内部以 UTF-8 字节偏移；对外同时提供 Monaco 语义的（行号从 1、列号按 UTF-16 码元）换算，供 LSP 与 MinCode 迁移使用。
- 撤销历史按编辑组（`pushUndoStop` 语义）组织，光标状态随撤销恢复；多个视图共享同一模型与历史。
- 行尾：保留原文件 EOL（LF/CRLF），支持切换。
- 模型池按 URI 管理，提供创建、销毁、语言变更事件（对应 `onDidCreateModel`/`onWillDisposeModel`/`onDidChangeModelLanguage`）。

### 6.4 高亮

- Monarch 以行为单位分词，每行结束保存解释器状态；编辑后从改动行重新分词，直到某行结束状态与旧值相同为止。
- 状态栈驻留（interning）：每行只存一个状态 ID，判等是整数比较，节省内存并让增量重分词尽早停止。
- 分词按时间预算推进：每帧在绘制前对可见行之前的部分做有限时间的分词，未完成的行先显示猜测结果（上一版 token 或纯文本），后续帧继续；其余部分由后台 goroutine 读快照完成。（思路参考 mygo 的 `Highlighter` 时间预算设计，不复用其代码。）
- 内置语言来源：构建期一次性脚本（Node）读取 `monaco-editor` 的 `esm/vs/languages/definitions/*`，导出为 JSON（正则字面量转为 `{source, flags}`），提交到仓库；运行时不依赖 JS。脚本与 monaco-editor 版本记录在 `editor/langs/README`。
- 语义高亮（semantic tokens）作为叠加层，按 provider 的 legend 映射到主题。
- 高亮接口可替换（`Highlighter`），以后可接 Tree-sitter 等。
- 以后可选：Tree-sitter（Zed 的做法：语法 + `highlights.scm` 等查询，扩展语法编译为 WASM 由 wasmtime 运行）。精度最高且同时提供折叠/缩进/语法选区/大纲，但 Go 侧需 cgo 或 WASM 运行时（如 wazero），且与 Monarch 插件不兼容；作为 `Highlighter` 的升级路线，不进第一版。
- 以后可选（不在本项目范围）：用 chroma（311 种语言，Pygments 体系）给 Monaco 不支持的语言做后备高亮，经一张 Pygments token → Monarch token 的通用映射表接到主题。chroma 不能做主方案：token 分类与主题体系不同、不支持带状态栈的逐行增量、无法运行插件的 Monarch 语法。

### 6.5 provider 与宿主集成

- 每种语言功能定义一个 Go 接口（参数含快照、位置、取消用 `context.Context`），可按语言选择器注册多个，结果按 Monaco 的合并规则处理。编辑器负责触发时机、防抖、过期结果丢弃（按版本号）与展示。
- 位置与区间类型同时提供 LSP 友好的换算，MinCode 宿主可把 `lsp-service` 的结果几乎直接转进来。
- 诊断使用"按来源（owner）设置标记"的语义，对应 `setModelMarkers`。

### 6.6 性能目标（验收门槛）

参考机器：Apple Silicon Mac（M 系列）与一台中端 Windows x64 笔记本，窗口 1920×1080（Retina 下实际 2 倍像素）。macOS 可在开发机实测；Windows 实机是否可用尚未确认——没有实机前，Windows 指标标记为"待实测"，不阻塞里程碑 1 结项，但阻塞里程碑 5 验收。

| 场景 | 目标 |
|---|---|
| 打开 100 MB / 100 万行文本，到首屏可编辑 | ≤ 1 秒（首屏高亮 ≤ 1.5 秒） |
| 在 100 万行文件中输入，按键到上屏 | p95 ≤ 16 ms |
| 连续滚动（触控板/滚轮） | 平均 ≥ 60 fps，p99 帧时间 ≤ 33 ms |
| 2 万行 TypeScript 全量高亮（后台） | ≤ 2 秒，期间输入不受影响 |
| 内存：100 MB 文件 | ≤ 文件大小的 3 倍 |

与 Monaco 的同场景对比数据在里程碑 1 测出并记录（MinCode 已有 `e2e-large-text`、`e2e-perf` 场景可用）；若 Monaco 更好，以 Monaco 为门槛。

**大文件降级（用户确认）**：允许像 Monaco 一样按阈值降级，阈值与降级项参照 Monaco 的 `largeFileOptimizations` 行为（大小/行数阈值下关闭折叠、小地图、括号配对着色、同词高亮、链接检测等）；**但语法高亮在大文件下保持开启**（Monaco 会关闭）。与 Monaco 对比时，Monaco 使用默认配置（即会降级），我们的数字须同时满足上表与"高亮仍开启"。

### 6.7 里程碑

所有里程碑完成前**不接入 MinCode**。

1. **性能原型**：基础层扩展（6.1）+ buffer + 混合正则 + Monarch 解释器 + 3 种语言（Go、TypeScript、YAML）+ 最小 View（显示、光标、输入、滚动）。在 GPU 渲染项目里程碑 1（macOS Metal）之上测第 6.6 节指标。buffer / regex / monarch / langs 不依赖渲染，可在 GPU 项目完成前开工。
2. **编辑核心**：清单 1、2、11、12、13 + 全部内置语言与主题。
3. **语言功能**：清单 4、5、6（provider 接口 + 展示组件：补全列表、悬停、参数提示、peek、灯泡、CodeLens、Inlay Hints、内联补全）。
4. **Diff、Vim、调试与其他装饰**：清单 7、8、9、10。
5. **对齐验收**：逐项对照第 4 节清单，并用 gallery 示例 + 无头 Driver 的回归测试覆盖；输出 MinCode 迁移对照表（Monaco API → Tango API）。

### 6.8 边界决定（审阅后补充，设计者的决定）

- **插件贡献点的消费分工**：Tango 编辑器只负责认识 Monaco 格式的数据——Monarch 语法、语言配置、主题规则、代码片段、JSON Schema 关联、快捷键规则与 when 表达式、菜单项数据。MinCode 自有概念与策略——`editorFrameworks`（项目框架检测与补全注入）、格式化器冲突选择、`languageFeatures` 结果的二次校验、插件语法的正则回溯探测——留在 MinCode 的 Go 宿主，随工作台迁移项目实现。里程碑 5 的"原样可用"验收：取 MinCode 官方插件包中贡献了 languages / editorThemes / snippets / jsonValidation / keymaps 的真实包作为测试夹具，加载后与 Monaco 下的分词与配色结果比对。
- **运行形态**：只支持纯自绘窗口（`uihost` 打开的 Surface）。不支持"自绘 canvas 叠在 WebView 上"的混合形态；浮层（补全、悬停、peek、菜单）在自绘窗口内自行绘制。
- **上屏与 GPU**：上屏与局部重绘由 GPU 渲染项目负责（`docs/specs/gpu-renderer.md` 6.2、6.5）；编辑器需要的渲染能力（区域平移提示、脏区域）作为对该项目的需求提出，不在编辑器里自行实现。
- **Vim 范围**：功能集以 MinCode 现用的 monaco-vim（CodeMirror vim）为准，外加宿主自定义 ex 命令（对应 `defineEx`）；行为细节有歧义时以真实 Vim 为准。`/` 搜索与 `:s` 使用 JS 正则语义（与现状一致，复用 `editor/regex`）。

## 7. 留给实现者

- 各子包的具体 API 形状（遵循 `ui/AUTHORING.md` 的即时模式风格：`editor.View(c, st)` 一类入口，状态由应用持有）。
- rope 的分块大小、树的平衡策略、撤销分组的时间阈值。
- JS 正则 → Go 语法预处理的具体规则（`\s`、`\d`、`\w`、`\b` 的 Unicode 差异、`u`/`i` 标志、`$` 语义等），以对照测试为准：用 Monaco 原版分词结果为黄金数据，逐语言比对 token 边界与类型。
- diff 算法选择（Myers / histogram / patience）与大文件降级策略。
- JSON Schema 库的最终选择（满足第 3 节条件）。
- 小地图、sticky scroll 等的视觉细节，以 Monaco 默认外观为参照、Tango 主题令牌为准。
