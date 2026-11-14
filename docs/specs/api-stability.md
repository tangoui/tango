# Tango 公开 API 稳定化方案

> 状态：已批准（2026-11-14，§7 全部按建议）；Phase 0、Phase 1 已完成
> 范围：Go 模块 `github.com/tangoui/tango`、CLI 模块 `cmd/tan`（`tan` 命令行参数与 `tan.json`）、npm 包 `@tangoui/runtime`（`runtime-js`）
> 目标（用户原话）："API 必须稳定；内部怎么大改都不应影响 API。"

---

## 0. 结论摘要

1. **现状**：分层机制已有（`docs/release.md` 的「API 稳定性」、`scripts/apidiff.sh`、`.github/workflows/api.yml`），但有三个缺口。
   - **实验层太大**。`ui` 一个包就导出约 2500 个标识符（556 个顶层声明加 1960 个方法），整层只报警告。模板 `cmd/tan/templates` 和 `uihost` 用的正是这一层。
   - **内部细节直接出现在公开 API 里**。
     - 渲染器与着色器编译器共 17 个包是公开路径：`ui/gpu/...` 11 个，`ui/shader/...` 6 个加 1 个命令。
     - `ui/paint` 混有约 24 个光栅化辅助函数。
     - 根包有 25 个类型别名、约 60 个常量别名直接指向 `internal/platform`，`sys/*` 下还有约 30 个类型别名。所以 internal 一改，公开 API 就跟着变，PR 里却看不出来。
   - **检查只看"不兼容"**。apidiff 不报告新增，上次对比新增了 1366 个导出标识符，没有人审过。字符串键的 API 它也看不到（`ui.Prop`/`Defaults` 有 155 处）。
2. **路线**：
   - 渲染器与着色器整体移到 `internal/render/...`（phase 1）。
   - `ui` 不拆包，用"稳定符号白名单"定义稳定核心，约 50 个类型、1000 个左右标识符（phase 2）。
   - 冻结前修掉已经观察到的几类破坏隐患（phase 3）。
   - 引入按包的 `api/go/*.txt` 黄金文件，让每次 API 变化都出现在 diff 里。
   - 以第一个标签作为 apidiff 基线（phase 4）。
3. **时机**：`CHANGELOG.md` 里 0.1.0 仍是「未发布」，仓库也没有任何 git tag。现在破坏兼容的成本最低。冻结之前把所有该做的不兼容调整一次做完。

---

## 1. 包清单与分层建议

### 1.1 计数口径

- **T** = `go doc -short <pkg>` 的行数：顶层导出声明，一个 const/var 组按一行算。
- **M** = `go doc -all <pkg>` 里 `func (…)` 的方法数。
- 结构体字段与组内常量不计。实际标识符数比 T+M 大，`ui` 的 T+M = 2516，与"约 2500"吻合。
- **导入方**：用 `go list -f '{{.Imports}} {{.TestImports}} {{.XTestImports}}' ./... ./cmd/tan/...` 统计，排除包自身子树。

### 1.2 非 internal 包清单

| 包 | T / M | 包外导入方 | 建议 | 理由 |
|---|---|---|---|---|
| `tango`（根） | 60 / 98 | 18 个 example、cmd/tan 模板、多数 sys 包 | **稳定** | 核心入口。注意 25 个 `type X = platform.X` 和约 60 个常量别名（见 §3 R8） |
| `asset` | 7 / 0 | cmd/tan（`dev.go`）、5 个 example | 稳定 | `DevProxy` 刚加了可变参数，是上次唯一的稳定层破坏 |
| `automation` | 14 / 72 | 2 个 example | 稳定 | **跨层泄漏**：`Locator.All() []a11y.Node` 暴露了 `ui/a11y`，所以 `ui/a11y` 必须升为稳定 |
| `bind` | 1 / 0 | cmd/tan bindgen（`BindPath`、生成代码调用 `bind.Register`）、2 个 example | 稳定 | 生成代码依赖它 |
| `i18n` | 19 / 0 | 3 个 example、internal | 稳定 | |
| `ipc` | 13 / 15 | 3 个 example、jsapi 等 | 稳定 | `.claude/worktrees/ipc-duplex`（locked）正在改，apidiff 会拦 |
| `jsapi` | 13 / 5 | 2 个 example | 稳定 | `Cap` 的位值也是 API（JS 侧文档列出） |
| `runtime-js/runtimejs` | 1 / 0 | example、模板 | 稳定 | `Script` |
| `service`、`service/{fileserver,kv,log,paths}` | 7 / 0 等 | example | 稳定 | |
| `service/updater` | 38 / 7 | **cmd/tan**（`sign.go`、`release.go`） | 实验性（维持） | 清单格式与签名格式是 CLI 与运行时之间的契约，见 §6 |
| `sys/*`（40 个包） | 各 2–14 | example、jsapi | 稳定 | 普遍有到 `internal/platform` 的别名（biometric、orientation、permission、purchase 各 12 行） |
| `sys/theme/uivars` | 5 / 0 | — | 稳定 | **跨层泄漏**：`Default`、`Vars`、`Gen` 暴露 `*ui/theme.Theme`，所以 theme 核心必须稳定 |
| `site/tools/apigen`、`site/tools/playground` | 0 / 0 | — | 排除（main 包） | 加入 tiers 的排除列表 |
| `examples/...` | — | site/tools 导入 `examples/ui-gallery/gallery` | 排除 | 不是 API；`gallery` 只给站点用 |
| **`ui`** | 556 / 1960 | 9 个 example、模板、site playground、uihost | **拆两层：白名单稳定核心 + 其余实验性**（§2） | |
| `ui/a11y` | 21 / 23 | 根包与 automation（稳定层）、10 个 internal、4 个 example | **升为稳定** | 稳定包已经依赖它 |
| `ui/geom` | 20 / 29 | 14 个 internal、7 个 example、site | **稳定** | 几何基础类型，平台层也用 |
| `ui/input` | 36 / 18 | 根包、14 个 internal、4 个 example、site | **稳定**（`FocusOrder[ID]` 等辅助标为实验性） | `ui` 的 API 里引用 38 处 |
| `ui/layout` | 42 / 1 | 8 个 example、模板（`layout.Gap`）、site | **稳定（Option 构造函数）**；`Node/Solve/SolveAt/Pos` 改为内部 | 引擎类型只有 `ui` 用，`layout.Style` 只有 `ui/mobile` 用（§5 phase 3） |
| `ui/icon` | 9 / 3（85 个图标变量） | 3 个 example | **稳定** | 图标变量只增不删 |
| `ui/text` | 28 / 13 | 3 个 example、site、uihost | **稳定子集**：`Style/Weight/Direction/Options/Measure/Width/RegisterFont/Families/UseSystemFonts/DefaultLang/SetDefaultLang`；其余（`Shape/Layout/Line/LineCache/Run/Engine/UseSystemEngine/TrimCaches`）实验性 | `ui` 的 API 引用 `text.Options/Weight/Direction` |
| `ui/theme` | 122 / 18 | 6 个 example、模板（`theme.Shadcn/Dark/AntDesign`）、uivars | **稳定核心**：`Theme`、构造与选项（`AntDesign/Shadcn/Dark/WithPrimary/ShadcnOptions`…）、`Color` 与颜色函数、`Alias`；**实验性**：各 `XxxToken`、`Components`、`Comp` | 主题令牌会话正在改；`NumComps` 是隐患（§5 phase 3） |
| `ui/paint` | 69 / 45 | `ui.Custom`、`Box.Paint(func(*paint.List, geom.Rect))`，11 个 example 文件 | **拆分**：显示列表（List、Path、各 Op、StrokeStyle、Cap、Stop、Gradient、Shadow…）**稳定**；光栅化辅助移到 `internal/render/raster` | 用户自绘必须用它（§1.3） |
| `ui/chart` | 79 / 251 | 1 个 example | 实验性 | |
| `ui/mobile` | 116 / 195 | 2 个 example | 实验性 | 上次删掉了 `SetSafeArea/SetTouches` |
| `ui/editor` + 8 个子包 | 86 / 69 等 | 2 个 example | 实验性（整棵树） | 编辑器会话活跃（工作区有 64 个未提交文件，绝大多数在 `ui/editor`）。`regex`、`when` 本可内部化，但 `monarch.MatchTimeout` 引用 `regex`、`keymap` 引用 `when.Context/Implies`，暂不动 |
| `ui/gpu` 及其 `atlas/cpuexec/d3d11/gen/gl/gl/nativewindow/metal/shaders` | 共约 78 / 43 | 见 §1.3 | **移到 `internal/render/gpu/...`** | |
| `ui/shader/{front,glsl,hlsl,ir,msl,sl}`、`ui/shader/cmd/tanshader` | 共约 75 / 39 | 只有 `ui/gpu` 树 | **移到 `internal/render/shader/...`** | |
| `uihost` | 27 / 8 | 7 个 example、模板（`uihost.Open/Attach/Layer/Persist/Options`）、**cmd/tan**（按符号名找 `uihost.AttachWith`） | **稳定核心**：`Open/Attach/AttachWith/Options/Layer/Persist/FilePicker/AcceptDrops/FollowSystemTheme`；其余实验性 | 自绘应用的入口，与 `ui` 稳定核心同进退 |

### 1.3 渲染器与着色器包为什么能内部化（逐个核对导入方）

**非测试代码**里，`ui/gpu`、`ui/shader` 树之外的导入方只有：

- `uihost/gpu*.go`：7 个文件，导入 `ui/gpu`、`ui/gpu/{metal,gl,d3d11}`。
- `internal/platform/android/canvas_gl_android.go`：导入 `ui/gpu/gl/nativewindow`。

**测试代码**里的导入方：

- `ui/damage_fp_test.go`
- `ui/editor/{damage_test,navigation_fix2_test,perf_test}.go`
- `examples/ui-gallery/damage_verify_test.go`（`gpu.NewRenderer(cpuexec.NewDevice())`）

**cmd/tan、模板、site 都不导入这些包**：

- cmd/tan 只导入 `asset`、`service/updater`、`internal/devhost`、`bind`、`ipc`。
- 模板与 site 也都没有引用。

**公开 API 不引用 gpu 类型**：对 `ui`、`uihost`、`ui/editor`、`ui/mobile`、`ui/chart`、`ui/theme`、`ui/text`、`ui/icon` 的 `go doc -all` 搜 `gpu.|cpuexec.|metal.|gl.|d3d11.|atlas.|shaders.|sl.|ir.`，结果为 0。

**规格本身已表态**：`docs/specs/gpu-renderer.md` 第 26 行写明"着色器编译器先作为 Tango 内部工具；是否对外开放以后再定"。

**目标路径必须是模块根下的 `internal/`**，不能是 `ui/internal/`。因为 `uihost` 和 `internal/platform/android` 不在 `ui/` 之下，`ui/internal/...` 对它们不可见。

**`ui/paint` 不能整体内部化，但要拆**。下面这些导出函数实际只被渲染器用（grep `paint.<Fn>` 的结果）：

- `CatmullRomWeights`、`RRectCoverage`、`ShadowCoverage`、`StrokeCoverage` → 只有 `ui/gpu/shaders`
- `CheckerColors`、`GradientDrawable`、`ImageDrawable`、`ImageReduction`、`PathDeviceBounds`、`PrepareImage`、`ShadowPlacement` → 只有 `ui/gpu`
- `Dither4x4`、`DitherPremul`、`GradientRamp`、`GradientT`、`RampPremul`、`RampSample`、`CheckerSelect` → `ui/gpu` 与 `ui/gpu/shaders`
- `GlyphPlacement` → `ui/gpu`、`ui/paint`
- `Orient`、`Resample`、`ToRGBA` → 只有 `ui`
- `ResizeBilinear`、`SampleCatmullRom`、`DecodeImage` → 仓库内无调用方
- `NewCPU` → `ui`、`ui/icon`、`ui/text`、`uihost` 与渲染器（都在模块内）

这些函数连同 `CPU`、`Renderer`、`GlyphSource`、`GlyphRasterizer`、`Damage`、`ScrollRegion` 一起移到 `internal/render/raster`（依赖方向：raster → paint，paint 不反向依赖）。用户需要离屏渲染时，通过 `ui.Driver` 的截图能力提供，不暴露 CPU 光栅器。

### 1.4 不在 Go API 里、但同样是公开契约的东西

| 契约 | 位置 | 现状 |
|---|---|---|
| `cmd/tan` 依赖 `internal/devhost`（跨模块使用 internal：路径前缀相同所以合法） | `cmd/tan/dev_hot.go` | 有 `devhost.ProtoVersion = 2` 和 `Marker`，没有跨版本测试 |
| `cmd/tan` 按符号名 `github.com/tangoui/tango/uihost.AttachWith` 识别自绘应用 | `cmd/tan/dev_hot.go:44` | 重命名 `uihost.AttachWith` 会悄悄让热加载失效 |
| `cmd/tan build` 要求模块里有 `mobile/` 目录 | `cmd/tan/build_mobile.go:83` | 目录布局契约 |
| `ui.Prop(c, "Card", "bordered", …)`/`ConfigProvider.Defaults` 的字符串键 | 155 处 | apidiff 看不到 |
| IPC 线上协议 | `docs/architecture.md`「IPC 线上协议」 | 没有版本字段 |
| CSS 变量名（`theme.CSSPrefix = "--tango-"`、`Theme.CSSVars`） | `sys/theme/uivars` | 网页依赖 |

---

## 2. `ui` 包：稳定核心与实验性部分

### 2.1 三个方案对比

| | (a) 单包 + 稳定符号白名单 | (b) 实验性组件拆到 `ui/x/<group>` | (c) 收起误导出的辅助符号 |
|---|---|---|---|
| 代码改动 | 几乎为零（新增 `api/stable/ui.txt`，改脚本） | 很大：Pro 系列文件约 1 万行，用到 27 个 `Context` 未导出成员（`c.box/c.interact/c.add/c.controlHeight`…）和大量包级私有函数，拆包要么导出这些钩子（API 反而更大），要么建 `internal/uikit` 再把组件全挪过去 | 中等，逐个改名 |
| 对活跃会话的冲击 | 无 | 大：`ui/` 近期 349 次文件变更，是改动最密集的目录 | 小到中 |
| 用户可见性 | 弱：要靠文档徽标和包文档说明 | 强：import 路径本身就说明是实验性 | — |
| 升为稳定 | 白名单加一行 | 要搬回 `ui`，对用户是又一次破坏 | — |
| 能防"新增即承诺"吗 | 能：默认实验性，进白名单才是承诺 | 能 | 不能 |

### 2.2 推荐：(a) 为主，(c) 作为 phase 3 的局部清理，(b) 推迟到 v0.3 之后再评估

推荐 (a) 的理由：

- 白名单是"默认不承诺"（fail-safe）：新写的组件自动是实验性，升为稳定是一次能被审查的显式 diff。
- 不改 `ui/*.go` 一行，和 GPU、编辑器、主题三个活跃会话零冲突。
- 将来某组（比如 `Pro*`）成熟、内部钩子也理清后，仍可以整组迁到 `ui/pro`，届时再决定。

(c) 的范围：开发工具类导出（`AuditTheme/AuditIssue/AuditReport/MirrorReport/MirrorViolation/IconViolation`），只有 `examples/ui-gallery` 用。另有宿主管道接口（`DamagePresenter/AccessibilityHost/Host`），只由 `uihost`、`internal/platform` 实现，可以收进 `internal/uiport`。这两类先列为实验性，phase 3 视情况内部化。

### 2.3 稳定核心草案（来源：`ui/AUTHORING.md` §14、`docs/specs/ui-coverage.md`、模板、example 实际用量）

先看实际用量：

- 非 gallery 的 example 加模板，一共只用了 51 个 `ui.X`：`Alert AlignRight Button Card Check Checkbox Column Context Custom DatePicker Div Driver DriverOptions Email Empty Form FormW ID Input InputNumber Invalidate Key Message Modal NewDriver Notification Option Opts Paragraph Popconfirm Progress Rate Required Row Runner RunnerOptions Scroll Search Segmented Select Slider Spacer Statistic Switch TabItem Table TableColumn Tabs Tag Text TextArea Title`。
- 算上 docs/site 的 Markdown 是 133 个。
- `ui-gallery` 用了 349 个。

稳定核心分三组：

- **框架**：`Context`（70 个方法，逐个审，预计保留 45 个左右）、`ID`、`Runner`/`RunnerOptions`、`Window`（见 §3 R5）、`Box` 与纯容器（`Row/Column/Div/StackBox/Custom/PaintFunc/Spacer/Gap/Key/Disabled/State/Scroll/ScrollH/ScrollBox`）、`Direction`、`Placement`、`ControlSize`、`TextAlign`、`Locale` 与 `EnUS/ZhCN/ZhTW/Ja`、`Driver/NewDriver/DriverOptions`（用户写测试要用）。
- **基础组件（约 45 个）**：`Button/IconButton/Icon`、`Text/Title/Paragraph/Link`（`Label`）、`Divider`、`Tag`、`Input/TextArea/Search/InputNumber`、`Select`、`Checkbox(+Group)/Radio(+Group)/Switch/Slider/Rate/Segmented`、`DatePicker/TimePicker`、`Form/FormItem` 与规则（`Required/Email/MinLen/MaxLen/Pattern/Match/Check`）、`Table/TableColumn`、`Tabs/TabItem`、`Menu/MenuItem`、`Dropdown`、`Pagination`、`Breadcrumb`、`Steps`、`Modal/Drawer/Alert/Confirm/Popconfirm/Message/Notification`、`Tooltip/Popover`、`Spin/Progress/Empty/Result/Statistic`、`Card/Badge/Avatar/Collapse/Tree/List`、`Space/Flex/GridRow/Col`、`Layout/Header/Sider/Content/Footer`、`ConfigProvider`。
- **实验性（其余）**：
  - 全部 `Pro*`/`*Form`（ProTable、ProForm、ProLayout、ProCard、StepsForm、ModalForm、QueryFilter、LightFilter、LoginForm…，约 28 个）
  - shadcn 专属（`Command/Kbd/Toggle*`）
  - 虚拟化（`VirtualList/VirtualSelect/VirtualTree/Listy/InfiniteScroll`）
  - `Markdown/MarkdownEditor/RichText/Tour/Watermark/QRCode/BorderBeam/Masonry/Carousel/Calendar/Transfer/Cascader*/TreeSelect/Mentions/Upload/DropZone/ColorPicker*/Image/Countdown/Timeline/Anchor/Affix/Splitter/FloatButton*/App/UseApp`
  - 开发工具类导出
  - 可变全局变量（`MessageDuration/NotifyStackThreshold/ContextMenuSource`）

**规模估计**：对上面约 50 个核心类型逐个数 `go doc ./ui <T>` 的 func 行数：

- `Context` 70、`Table` 68、`Driver` 53、`Box` 47、`Label` 30、`Input` 28、`Runner` 26、`Form` 25、`Button`/`Select`/`Tree` 各 24，合计 **921 个函数/方法**。
- 加上常量与导出字段，约 **1000–1100 个标识符，占 `ui` 的 40% 左右**。
- 想再收窄，可以把 `Table` 的高级方法和 `Driver` 的半数方法留在实验性，降到 800 个左右。

---

## 3. API 设计规则（针对已观察到的破坏类型）

下面每条都对应 apidiff 实际报出的问题。这些规则写进 `docs/release.md`，替换现有的"用可变参数追加选项"一句。那句和同一文档第 46 行自相矛盾，`asset.DevProxy` 那次破坏正是照着它做的。

**R1 枚举只追加，值永不改变，零值语义在创建时就定死。**

上次的破坏：`DrawerRight` 原来是 0（`git show 9047122:ui/feedback.go:817`），现在 `DrawerEnd = 0`、`DrawerRight = 1`。`Notify*`、`Placement` 同理。

```go
// DrawerPlacement selects the edge a Drawer slides from.
// Append-only: never reorder or insert; the zero value stays DrawerRight.
type DrawerPlacement uint8

const (
	DrawerRight DrawerPlacement = iota // 0: the zero value, forever
	DrawerLeft
	DrawerTop
	DrawerBottom
	// added in v0.3: logical edges
	DrawerEnd
	DrawerStart
)
```

想让"逻辑方向"成为默认：不改零值，改由主题或 `ConfigProvider` 的默认值解析，或者新增 `DrawerAuto`，在 `Placement()` 未调用时生效。

- **不导出计数常量**：`theme.NumComps = int(numComps)` 每加一个组件，值就变一次，apidiff 判为不兼容。改成未导出常量或函数 `func NumComps() int`。
- **枚举值不和结构体字段顺序绑定**：`theme.Comp` 的注释写着 "Comp(i) is the i-th field of Components"。这样一来，在中间插入字段就会同时破坏两处。规定新组件只能追加到末尾。
- 开放集合（设计系统名称、形状、颜色名）用带类型的字符串常量：`type TagEffect string; const TagEffectDark TagEffect = "dark"`。新增值不会移动已有值。现有的裸 `string` 参数（`Tag.Effect(e string)`、`SkeletonButton(…, shape string, …)`）保持兼容，不强改。

**R2 构造函数从第一天起返回专属类型。**

上次的破坏：`Tooltip`、`Divider` 从返回 `*Box` 改为返回专属类型；`Label.Color/Type/Secondary` 改了返回类型。

- 仍返回 `*Box` 的非容器函数：`Kbd`、`BadgeRibbon`、`ListItemMeta`、`ListActions`、`AnchorSection`、`Sider`（已有 `CollapsibleSider`）。
- 返回 `bool`/`int`/无返回值的组件：`CheckboxGroup`、`CheckAll`（bool）、`Breadcrumb`（int）、`CardAction`（bool）、`CardMeta`、`Skeleton*`，共 32 个。

```go
// TooltipW is the builder Tooltip returns. It is a distinct type even while
// it adds nothing to Box, so options can be added without changing Tooltip.
type TooltipW struct {
	_ [0]func() // not comparable; see R4
	box *Box
}

func Tooltip(c *Context, tip string, fn func()) *TooltipW
func (t *TooltipW) Placement(p Placement) *TooltipW
```

只有纯布局容器（`Row/Column/Div/StackBox/Layout/Header/Content/Footer`）可以返回 `*Box`。

**R3 不给已有函数加参数，可变参数也不行。**

- 组件用链式方法加选项（现有风格）。
- 非组件函数从第一天就接收 `XxxOptions` 结构体。
- 漏掉了就新增 `XxxWith`：仓库里已有 `ContextMenuAt`/`ContextMenuAtWith`、`uihost.Attach`/`AttachWith` 两例。

```go
// v0.1
func DevProxy(target string) http.Handler
// v0.2: do NOT change DevProxy; add:
func DevProxyWith(target string, opts DevProxyOptions) http.Handler
```

`asset.DevProxy` 那次已经发生，作为 phase 3 的一项单独确认（见 §5）。

**R4 可比较性在创建时决定。**

所有带未导出字段的导出结构体，以及可能增长的 Options/Locale 结构体，**第一个字段**写 `_ [0]func()`。原因有三：

- 一开始就不可比较，以后内部加 slice、map、func 字段就不算破坏。
- `_` 是未导出字段，包外不能写无键字面量，所以以后加字段也安全。
- 零长度字段放在第一位，不会引入尾部填充。（Go 规定：结构体最后一个字段的大小为 0 时会补齐填充，防止取址越界。）

```go
type TagW struct {
	_ [0]func()
	c     *Context
	label string
	// later: icons []*icon.Icon — no longer an API change
}

type SurfaceOptions struct {
	_ [0]func()
	Title string
	// ...
}
```

影响面：

- `ui` 有 129 个 `*W` 类型，另有 `Box/Context/Runner/Driver`。
- 稳定层里目前仍可比较的 Options 结构体：`tango.SurfaceOptions`、`tango.WebViewOptions`、`asset.Options`、`asset.DevProxyOptions`、`jsapi.Options`、`service/fileserver.Options`、`service/log.Options`、`sys/biometric.Options`、`sys/flyout.Options`、`sys/location.Options`、`sys/media.PickOptions`、`sys/print.Options`。
- `ui` 里字段全导出的结构体共 77 个，其中 44 个仍可比较。

**R5 接口。**

- 不导出给用户实现的接口，除非专门这样设计过。
- 需要封闭就加未导出方法：`input.Event`、`paint.Op` 已经这样做了。
- 扩展能力用可选接口，不往已有接口里加方法。仓库已有好例子：`BackWindow/SafeAreaWindow/KeyboardInsetWindow/TextInputStateWindow` 都是对 `ui.Window` 的扩展。
- `ui.Window`（9 个方法）如果允许用户实现（测试替身），就提供可嵌入的基类：

```go
type Window interface {
	Size() geom.Size
	// ...
	window() // sealed: implementations must embed WindowBase
}

// WindowBase gives default implementations; embedding it keeps user
// windows compiling when Window gains methods.
type WindowBase struct{}

func (WindowBase) window() {}
```

**R6 先弃用再删除。** 写 `// Deprecated: use X. Removed in v0.(N+1).`，至少保留一个 MINOR 版本。CI 用 staticcheck（SA1019）拦住仓库内部对已弃用符号的新调用。

**R7 零值有意义且永不改变。** 零值 Options 等于默认行为；新字段的零值必须等于"旧行为"。

**R8 公开 API 不新增指向 internal 类型的别名。**

- 新类型在公开包里定义，internal 负责转换。
- 存量别名（根包 25 个类型别名、约 60 个常量别名；`sys/*` 约 30 个类型别名；`automation` 5 个）视为"钉住的类型"。
  - `api.txt` 生成器会**展开别名目标的字段与方法**（§4.1），所以改 `internal/platform.AppEvent` 的字段会出现在 `api/go/tango.txt` 的 diff 里，用户的目标"内部大改不影响 API"因此可以被检查。
  - 顺带一个问题：`go doc . AppEvent` 目前看不到任何字段，文档质量差。de-alias 列为开放问题（§7）。

**R9 不导出可变的包级配置变量。** 涉及 `ui.MessageDuration`、`ui.NotifyStackThreshold`、`ui.ContextMenuSource`、`sys/windowstate.SaveDelay`、`a11y.Mappings`。改用函数或 Options。存量的标为实验性（`ui`）或冻结类型（`sys`）。

**R10 泛型。** 已有类型（`ProTableW[T]`、`FormListW[T]`…）不新增类型参数，也不收紧约束。

**R11 字符串键 API 也是 API。** `Prop(c, comp, key, def)` 的组件名、键名、值类型进入黄金文件（§4.1），改名按 R6 处理。

**R12 跨 IPC 的结构体，其 JSON 标签是线上 API。** 涉及 `jsapi` 参数与返回结构、`devhost` 消息、`updater` 清单。改字段名等于破坏。

---

## 4. 工具

### 4.1 黄金文件 `api/go/<pkg>.txt`：建议采用

**为什么要**：

- apidiff 只报不兼容变化，上次新增的 1366 个标识符无人审查，而每个新增都是承诺。
- 黄金文件让新增、钉住类型的漂移、枚举值、可比较性、字符串键全部出现在 PR diff 里，配合 CODEOWNERS 强制审查。

**生成器**：

- 新命令 `internal/cmd/apitxt`：放在 internal 下，不成为 API；`go run ./internal/cmd/apitxt` 在模块内可用。
- **只用标准库**：`go list -export -json` 拿编译导出数据，加 `go/importer.ForCompiler(fset, "gc", lookup)`、`go/types`。这与 `site/tools/apigen` 只用标准库的做法一致，不往 `go.mod` 引入 `x/tools`。
- 不用 `apidiff -w`：它写的是二进制导出数据，不能拿来 diff 审查。
- 按 `linux darwin windows` 各导出一次再合并；平台独有的行加 `[darwin]` 后缀。可选加上 `android ios`：现有 apidiff 完全没覆盖这两个平台，不过目前 sys 下只有 2 个移动端专属文件。

**格式**（参考 Go 的 `api/go1.N.txt`，按行排序，一行一个事实）：

```text
pkg github.com/tangoui/tango, func New(Options) *App
pkg github.com/tangoui/tango, type AppEvent = internal/platform.AppEvent
pkg github.com/tangoui/tango, type AppEvent struct, Kind AppEventKind
pkg github.com/tangoui/tango, type AppEvent struct, URL string
pkg github.com/tangoui/tango, type SurfaceOptions struct, noncomparable
pkg github.com/tangoui/tango/ui, const DrawerRight DrawerPlacement = 0
pkg github.com/tangoui/tango/ui, func Tooltip(*Context, string, func()) *TooltipW
pkg github.com/tangoui/tango/ui, method (*TooltipW) Placement(Placement) *TooltipW
pkg github.com/tangoui/tango/ui, type TagW struct, noncomparable
pkg github.com/tangoui/tango/ui, prop "Card" "bordered" bool
pkg github.com/tangoui/tango/ui/theme, const NumComps untyped int = 49
```

- 常量带值，所以枚举移位会直接改动行。
- `noncomparable` 行让 R4 可见。
- `prop` 行由 go/ast 扫描 `Prop(c, "<lit>", "<lit>", def)` 得到。

**命令**：

- `go run ./internal/cmd/apitxt -w`：重写。
- `-check`：工作区与生成结果不一致时退出 1，并提示运行 `-w`。
- 沿用仓库里 `-update` 风格的黄金测试习惯（`ui/golden_test.go`）。

**冲突**：按包分文件、按行排序，并行会话只会在相邻行冲突，解决办法统一是重新生成。`api/go/ui.txt` 约 3000 行，可以接受。

### 4.2 分层配置取代 `EXPERIMENTAL_RE`

- `api/tiers.txt`：包路径 → `stable | experimental | exclude`，支持 `/...`。
- **未分类的新包直接让 CI 失败**，逼作者做决定。现有正则里的 `mobile` 已经过时：仓库根 `mobile/` 下只有 android/ios，没有 Go 包。
- `api/stable/<pkg>.txt`：用于 `ui`、`ui/theme`、`ui/text`、`ui/input`、`uihost`、`ui/paint` 这类"包内分层"的包，内容是稳定符号白名单。
  - 每行一个 `Func`、`Type`（覆盖它的全部字段和方法）或 `Type.Member`。
  - 不支持"稳定类型上的实验性方法"，以免混淆。
- `scripts/apidiff.sh` 的改动：
  1. 解析 apidiff 输出行 `- ./ui.Tooltip: …`，得到（包, 顶层符号, 成员）。
  2. 查 tiers；包内分层的包再查白名单，命中就算稳定层破坏。
  3. 稳定包、白名单里的符号报不兼容就失败；其余照旧警告。`ALLOW_API_BREAK` 和 `api-break` 流程不变。

### 4.3 静态检查（并入 `apitxt -check`）

1. **跨层泄漏**：稳定符号的签名、字段、方法只能引用标准库、稳定包或白名单符号。目前会报出：`automation` → `ui/a11y`，`sys/theme/uivars` → `ui/theme`，`uihost` → `ui`，`ui` 稳定核心 → `layout/input/text/paint/a11y/theme/icon/geom`。phase 2 用"升级被引用的符号"清零。
2. **实验性必须有标注**：实验性包的包文档必须包含 "Experimental" 或 "实验性"；白名单包的包文档必须说明分层规则并链接参考站。参考站由 `site/tools/apigen` 读白名单，给符号加「实验性」徽标。不要求在 1500 个符号的注释里逐个打标签，避免和活跃会话冲突。
3. **白名单不能有过期条目**：条目必须对应真实存在的符号。
4. **退役路径守卫**（`internal/apitest/retired_test.go`）：维护一个列表 `ui/gpu`、`ui/gpu/...`、`ui/shader/...`、`ui/paint` 中已移走的符号。旧路径下出现任何 `.go` 文件就失败。

   这是为 phase 1 准备的。其他会话 rebase 时，git 能识别"被修改的文件"已重命名，但**新建在旧路径的文件**会原样落回去，悄悄恢复一个公开包。

5. **CLI 与运行时的符号契约**：uihost 加一个测试，断言 `runtime.FuncForPC(reflect.ValueOf(AttachWith).Pointer()).Name()` 等于 `devhost.AttachSymbol`。这个常量从 `cmd/tan/dev_hot.go:44` 上移到 `internal/devhost`，两边共用。

### 4.4 CI 接线

- `api.yml` 的步骤依次是：
  1. `go run ./internal/cmd/apitxt -check`（黄金文件最新、无泄漏、分层完整、无退役路径）
  2. `bash scripts/apidiff.sh "$BASE"`
  3. 若 `api/go/**` 有变化而 `CHANGELOG.md` 没变化就失败，可用标签 `no-changelog` 豁免。
- `.github/CODEOWNERS`：`/api/`、`/docs/release.md`、`scripts/apidiff.sh`、`internal/cmd/apitxt/`、`internal/apitest/` 指给 API 审查人 `tangoui`（已决定）。
- `release.yml`：打标签前加一个 job，运行 `bash scripts/apidiff.sh <上一个标签>`，生成兼容性报告附到草稿 Release。
- `ci.yml` 的 lint 作业加 staticcheck SA1019（R6）。

---

## 5. 迁移计划

### Phase 0：冻结规则与工具（1–1.5 天，不改业务代码，可与所有会话并行）

- `docs/release.md`：写入 §3 规则，删掉"用可变参数追加选项"。
- `internal/cmd/apitxt`、`api/tiers.txt`、首批 `api/go/*.txt`、退役路径守卫（列表先留空）、apidiff.sh 按 tiers 工作。
- CI 第一周只警告，之后转为失败。

### Phase 1：渲染器与着色器内部化（实施 1 天，加协调）

路径映射：

| 旧路径 | 新路径 |
|---|---|
| `ui/gpu` | `internal/render/gpu` |
| `ui/gpu/atlas` | `internal/render/gpu/atlas` |
| `ui/gpu/cpuexec` | `internal/render/gpu/cpuexec` |
| `ui/gpu/d3d11` | `internal/render/gpu/d3d11` |
| `ui/gpu/gen` | `internal/render/gpu/gen` |
| `ui/gpu/gl`、`ui/gpu/gl/nativewindow` | `internal/render/gpu/gl`、`.../gl/nativewindow` |
| `ui/gpu/metal` | `internal/render/gpu/metal` |
| `ui/gpu/shaders` | `internal/render/gpu/shaders` |
| `ui/gpu/internal/{devkit,gpuconf,gputest,racetest}` | `internal/render/gpu/internal/...` |
| `ui/shader/{front,glsl,hlsl,ir,msl,sl}` | `internal/render/shader/...` |
| `ui/shader/cmd/tanshader` | `internal/render/shader/cmd/tanshader` |

影响面：

- **Go 文件**：共 126 个导入了 `ui/gpu` 或 `ui/shader`，绝大多数在这两棵树内部。树外的有：
  - `uihost` 7 个
  - `internal/platform/android` 2 个
  - `ui/damage_fp_test.go`
  - `ui/editor` 3 个测试
  - `examples/ui-gallery/damage_verify_test.go`
- **非 Go 文件**：
  - `ui/gpu/gen/gen.go` 的 `//go:generate go run ../../shader/cmd/tanshader`：相对路径层级会变。
  - `ui/gpu/shaders/quad_test.go:126` 的 `exec "go run ../../shader/cmd/tanshader"`。
  - `.github/workflows/ci.yml:136,144` 的 `slow='/ui$|/ui/gpu|...'`。
  - `docs/dev-builds.md`（引用 `ui/gpu/verify_release.go`）、`docs/mobile.md` 两处。
  - `docs/plans/*`、`docs/specs/gpu-*` 是历史记录，不改。
- **cmd/tan、模板、site、文档站代码样例：0 处。**

协调：

- **编辑器会话**：`ui/editor/damage_test.go`、`perf_test.go` 当前有未提交修改，`navigation_fix2_test.go` 是未跟踪的新文件。Phase 1 必须等编辑器会话提交，或者事先约定由它改这 3 个文件里的 import。
- **GPU 会话**：选在两个里程碑之间，用一次纯机械提交完成（`git mv` 加 import 路径替换加 `gofmt`，不夹带任何逻辑改动）。提交后通知各会话 rebase。同时把退役路径写进守卫列表，旧路径下的新文件会立即被 CI 拦下。
- `ui/paint` 拆分（光栅化辅助移到 `internal/render/raster`）放在同一阶段的第二个提交里，约 0.5 天。调用方都在渲染器内和 `ui`、`ui/icon`、`ui/text`、`uihost` 中。

### Phase 2：定义稳定核心（2–3 天，不改 `ui/*.go`）

- 生成并人工审定 `api/stable/ui.txt`（约 50 个类型，§2.3）。
- 同时审定 `ui/theme`、`ui/text`、`ui/input`、`ui/layout`、`uihost`、`ui/paint` 的白名单。
- `ui/a11y`、`ui/geom`、`ui/icon` 整包设为稳定。
- 把跨层泄漏清零：稳定符号引用到的符号一并升级。
- `apigen` 加徽标；包文档写明分层规则。

### Phase 3：冻结前修复隐患（3–5 天，带 `api-break` 标签，按包分批）

1. **R4 不可比较守卫**：129 个 `*W` 加 `Box/Context/Runner/Driver`，以及 §3 R4 列出的 12 个稳定层 Options 结构体和 `ui` 里 44 个可比较的导出结构体（Locale 等）。纯机械，按文件分批，避开 `ui/editor` 和 `ui/theme` 的令牌文件。
2. **R2 专属返回类型**，只处理稳定核心里的：
   - `Sider`：4 处 `ui` 内调用、4 处 example。
   - `CheckboxGroup`：4 / 2；`CheckAll`：5 / 2（若纳入核心）。
   - `Breadcrumb`：8 / 1。
   - `CardAction`：1 / 5；`CardMeta`。
   - `Kbd`、`BadgeRibbon`、`ListItemMeta` 等属实验性，可以顺手改，也可以留着。
3. **枚举审查**：
   - 每个组写明零值与"只追加"注释，枚举值进入黄金文件。
   - 逐组过：`Placement`（31 个值，逻辑和物理混排）、`DrawerPlacement`、`NotifyPlacement`、`theme.Comp`、`input.Cursor`、`input.Key`（已有"appended so earlier values stay stable"注释，好例子）、`ButtonVariant`、`ControlSize`（零值 `Middle`）、`Trigger`、`MenuMode`、`TextAlign`、`Direction`、`a11y.Role`。
   - **现在**就把 `DrawerPlacement` 等的零值最终定下来：逻辑方向已经是 0，保持即可，冻结后不能再改。
4. `theme.NumComps` 改成未导出常量或函数；去掉 `Comp` 与字段顺序的绑定说明，或改为"只追加"。需要和主题令牌会话协调。
5. `layout.Option func(*Style)` 改为不透明类型，`Node/Solve/SolveAt/Pos/Style` 内部化（唯一的外部用户是 `ui/mobile` 的 1 处 `layout.Style`）。
6. `ui` 里可变的全局变量（R9）改为 `ConfigProvider` 或函数，或者留在实验性。
7. `asset.DevProxy`：要么接受现状（CHANGELOG 已写），要么按 R3 恢复成单参数并新增 `DevProxyWith`。建议恢复，因为 0.1.0 尚未发布，这是最后一次改回来的机会。
8. CLI 契约：`devhost.AttachSymbol` 常量与测试（§4.3 第 5 条）。

### Phase 4：打标签作为基线（0.5–1 天）

- 打 `v0.2.0`（或 v0.1.0，见 §7）。`cmd/tan/go.mod` 的 `require github.com/tangoui/tango` 从伪版本改为该标签；npm 包同一版本号（CHANGELOG 开头已约定三者同号）。
- apidiff 的两个基准：PR 对比目标分支（现状），发布前对比上一个标签。
- **CHANGELOG 策略**：
  - `api/go/**` 有变化就必须写 CHANGELOG（CI 强制）。
  - 稳定层破坏只能进下一个 `0.MINOR.0`，并在「变更/移除」里写迁移方法。
  - 实验层破坏写一句即可。
- **实验性升为稳定的标准**：连续一个 MINOR 没有不兼容变化、有文档、有黄金或单元测试、通过 §3 规则审查，然后白名单加一行。

| Phase | 工作量 | 和活跃会话的冲突 |
|---|---|---|
| 0 | 1–1.5 天 | 无 |
| 1 | 1–1.5 天 | GPU（高，需要选时间点）、编辑器（3 个测试文件） |
| 2 | 2–3 天 | 无 |
| 3 | 3–5 天 | 主题（`NumComps`/`Comp`）、`ui` 主线（机械修改） |
| 4 | 0.5–1 天 | 无 |
| JS/CLI 契约（§6） | 1.5–2 天 | ipc-duplex（线上协议版本字段） |

---

## 6. JS 运行时与 CLI 契约

### 6.1 `@tangoui/runtime`

- **API 报告**：引入 `@microsoft/api-extractor`（只是 devDependency），从 `dist/index.d.ts` 生成 `runtime-js/etc/runtime.api.md` 并提交。
  - CI 运行 `api-extractor run`（非 `--local`）：报告有变化就失败，强制本地更新并在审查时看到 diff。
  - `index.d.ts` 用 `export * as Window from …` 重导出 12 个命名空间，api-extractor 能正确展开。
  - 不想加依赖的替代方案：把 `tsc --declaration` 的产物合并成 `runtime-js/api/runtime.d.ts.txt` 作为黄金文件（注意 `dist/` 已被 gitignore）。
- **线上方法名与错误码**：
  - Go 测试枚举 `jsapi` 注册的全部 `tango:<模块>.<函数>` 和事件名，写入 `api/jsapi-methods.txt`。
  - JS 测试（`runtime-js/test/api.test.mjs`）断言 `src/api/*.ts` 调用的名字都在其中。
  - `ErrorCode` 联合类型和 `docs/runtime-api.md` 的表格保持一致。
  - `jsapi.Cap` 的位值已经由 Go 黄金文件覆盖（常量带值）。
- **协议版本**：IPC 消息（`docs/architecture.md`「IPC 线上协议」）没有版本号。
  - Go 内嵌的 IIFE（`runtimejs.Script`）与 Go 同步，但用户自己打包的 npm 版本可能和 Go 模块版本不一致。
  - 建议在 hello 里带 `proto` 字段：Go 不认识就在开发构建里告警。
  - 和 ipc-duplex 会话一起定。
  - 线上消息的 JSON 夹具放在 `runtime-js/runtimejs/testdata`，Go 和 JS 两边共用同一份黄金。

### 6.2 `tan` CLI

- **参数黄金**：`site/tools/clidoc.mjs` 已经抓取 `tan help` 和每个子命令的 `-h`（13 个子命令，约 54 个 flag 定义）。改成 Go 测试 `cmd/tan/cli_golden_test.go`，输出写到 `cmd/tan/testdata/cli.golden`，用 `-update` 重写；clidoc 改为读这份黄金。
  - 规则：flag 和子命令不删除、不改名、不改语义；默认值的改变写进 CHANGELOG；弃用的 flag 保留并打印警告，至少一个 MINOR。
- **`tan.json`**：
  - 从 `Config` 用反射加注释生成 JSON Schema `cmd/tan/tan.schema.json` 并提交，配黄金测试，同时发布到文档站，供编辑器补全。
  - 注意 `ParseConfig` 用了 `DisallowUnknownFields`（`config.go:227`）：
    - 用户写 `"$schema"` 会被拒，需要放行这个键。
    - 新版 tan 加的字段会让旧版 tan 直接报错。这本身是合理的，但要写明"按 tan vX 写的 tan.json 需要 tan ≥ vX"，或者新增可选字段 `"tan": ">=0.3"`，给出可读的错误。
  - 字段只增不删；改名时旧名继续解析并告警。
- **其他契约**：
  - `devhost.ProtoVersion` 加跨版本测试：当前 tan 对上一个标签的 devhost 握手。
  - `service/updater` 清单格式（`tan release-manifest`/`sign-update` 生成）加黄金夹具，验签旧清单的测试必须一直通过。
  - 模板 `cmd/tan/templates/*` 只能使用稳定符号：用 `apitxt` 检查模板里的 `ui.`/`uihost.`/`theme.`/`layout.` 引用（模板用了约 25 个 `ui.X`）。

---

## 7. 开放问题（附建议）

1. **稳定核心的大小**：约 50 个类型、1000 个标识符，还是收窄到 30 个类型、600 个左右？**建议 50**，覆盖模板和 example 的实际用量（51 个 `ui.X`）以及 Ant/Element 基础组件。`Table`、`Driver` 的高级方法可以先放实验性。
2. **白名单（默认实验性）还是文档标签（默认稳定）？** **建议白名单**：新增不会自动成为承诺，也不用改 1500 处注释。
3. **`Pro*` 系列是否最终迁到 `ui/pro`？** **建议 v0.3 之后再定**，前提是先把它们依赖的 27 个 `Context` 私有成员整理成 `internal/uikit`。
4. **现在就把所有 `*W` 和 Options 设为不可比较？** **建议是**：一次 `api-break`，以后内部加字段不再算破坏。代价是用户不能写 `opts == tango.SurfaceOptions{}`，这种写法罕见。
5. **基线版本号**：0.1.0 从未打标签，CHANGELOG 里也是「未发布」。**建议直接把 phase 3 之后的状态发布为 v0.1.0**，作为第一个基线，省掉一次"0.1 到 0.2 的破坏"。如果 0.1.0 已经对外宣传，就用 v0.2.0。
6. **根包和 `sys/*` 的别名是否 de-alias**（在公开包里定义类型，internal 负责转换）？**建议暂时不做**，先靠 `api.txt` 展开别名来守住漂移；文档可读性的问题在 v0.3 评估，工作量约 2–3 天，因为 `sys` 有 40 个包。
7. **`ui/paint` 拆多深？** **建议**稳定层只留显示列表的记录 API，`CPU/Renderer/GlyphSource/GlyphRasterizer` 一并内部化；需要离屏渲染的用户走 `ui.Driver`。
8. **`layout.Option` 改为不透明类型**：**建议做**。现在 `func(*Style)` 把 `Style` 的全部字段都变成了 API。
9. **JS 侧是否接受 api-extractor 依赖？** **建议接受**（只是 devDependency）；否则用 d.ts 黄金文件代替。
10. **IPC hello 加协议版本**：**建议加**，由 ipc-duplex 会话一并实现。
11. **`asset.DevProxy` 恢复单参数并新增 `DevProxyWith`？** **建议恢复**，趁还没发布。
12. **CODEOWNERS 的 API 审查人**：已定为 `tangoui`。

---

## 实施关键文件

- `/Users/admin/min/tango/scripts/apidiff.sh`
- `/Users/admin/min/tango/docs/release.md`
- `/Users/admin/min/tango/.github/workflows/api.yml`
- `/Users/admin/min/tango/ui/feedback.go`（`DrawerPlacement` 等枚举）、`/Users/admin/min/tango/ui/overlay.go`（`Placement`）、`/Users/admin/min/tango/ui/theme/components.go`（`Comp`/`NumComps`）
- `/Users/admin/min/tango/uihost/gpu.go` 与 `/Users/admin/min/tango/cmd/tan/dev_hot.go`（渲染器导入方；`uihost.AttachWith` 符号契约）
- `/Users/admin/min/tango/site/tools/apigen/main.go`（参考站徽标；`apitxt` 可沿用它只用标准库的写法）
- `/Users/admin/min/tango/cmd/tan/config.go`（`tan.json` schema 与 `DisallowUnknownFields`）
- `/Users/admin/min/tango/runtime-js/package.json`、`/Users/admin/min/tango/runtime-js/src/index.ts`
