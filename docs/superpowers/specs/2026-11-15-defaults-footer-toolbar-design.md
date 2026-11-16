# ConfigProvider Defaults 与 FooterToolbar 验收设计

状态：已批准（2026-11-15，用户确认方案 3 与全部待决项）
日期：2026-11-15

归档复核：2026-11-16，源码基线 `116b9e63`。本设计的实现与主要回归代码已由 `9877d9a` 集成到 main；原批准记录保留。落地提交和未重验的历史验收项目见 [历史实施计划](../plans/2026-11-15-defaults-footer-toolbar-plan.md)。本次归档没有重新运行测试、截图或跨平台构建，不据此新增验收通过结论。

## 1. 意图

继续推进 Tango 的组件完善专项，先把两个容易形成长期质量问题的部分做扎实：

1. 审计并补齐 `ConfigProvider.Defaults` 的真实覆盖范围，避免组件文档声称支持 Defaults、但构造器没有读取，或显式链式属性不能覆盖默认值。
2. 对已经实现的 `FooterToolbar` 毛玻璃能力做完整验收，确保 sticky / flow、高对比度、无 blur renderer、三套主题、RTL、窗口缩放和局部重绘都符合契约。

当前判断：FooterToolbar 的核心实现已存在（`ui/pro2_page.go`、`ui/theme/comp_footertoolbar.go`、`paint.BackdropBlur`），重点是边界验收和发现回归；真正的代码缺口更可能在 Defaults 覆盖审计。

## 2. 目标

### Defaults

- 建立组件矩阵：公开构造器、Defaults 组件名、支持的 key、读取位置、显式修饰器、测试状态。
- 找出三类问题：
  - 文档 / 测试声称支持，但构造器未读取；
  - 默认被读取，但显式 modifier 不能覆盖；
  - 组件嵌套时默认错误泄漏到内部面板，或应继承时没有继承。
- 以 `docs/specs/ui-coverage.md` §6.1 为 Defaults 审计权威基线：本轮分母是根 `ui` 包的 177 个公开入口，沿用其中的“已接线 / 不适用 / 未声明”口径。
- 只为**已有 Defaults 契约**的根 `ui` 组件补回归测试和真实漏接；CodeBlock、JSONView、LogView、Dock、StatusBar、TitleBar、NotificationCenter、HotkeyInput、Chat、Graph、DiffView 如果没有现有 Defaults 契约，本轮记录为“未声明”，不新增公开 Defaults key。
- Chat 不建立统一 `Defaults("Chat", ...)` 命名空间；若未来纳入，按 `Bubble`、`Sender` 等独立构造器命名并另立设计。
- `ui/graph` 和 `ui/editor/diffview` 不纳入本轮根 `ui` Defaults 审计；它们不是通过根 `ui.Context` 构造的 Defaults 组件。
- 保持显式链式 API 优先于 ConfigProvider 默认；类型错误和未知 key 使用现有的忽略行为，不引入运行时 panic。
- 不将所有组件强行改造成一个反射型 Defaults 框架；优先沿用 Tango 当前的显式 `Prop` / `ConfigProp` 读取方式。

### FooterToolbar

- sticky 时：支持 backdrop blur 的 renderer 发出正确的 `BackdropBlur`，半透明 token 背景覆盖正确区域；不支持 blur 的 renderer 和高对比度主题使用不透明背景。
- flow 时：不发出 blur，不误伤普通内容绘制。
- Ant、Element、shadcn 的明暗主题与 RTL 均通过布局、颜色和对比度检查。
- FooterToolbar 的阴影、顶部边线、主体按钮、额外内容、滚动覆盖关系和语义角色保持正确。
- blur 相关变化进入已有 damage 机制，不引入整帧重绘回归。

## 3. 非目标

- 不重写 ConfigProvider 的整体架构。
- 不新增新的设计系统或改变现有三套主题的默认视觉语言。
- 不实现 FooterToolbar 的新交互能力（拖动、折叠、响应式按钮菜单等）。
- 不把所有未覆盖 Defaults 的历史组件一次性扩展；本轮只处理 `ui-coverage.md §6.1` 已声明的 Defaults 契约和其中确认的漏接。
- 不新增 Chat、Dock、Graph、Viewer、Shell、DiffView 的 Defaults 公开契约；这些组件是否支持 Defaults 留在后续独立设计。
- 不改变实验性 / 稳定 API 分层，除非审计发现已有稳定 API 的实现错误。

## 4. 约束

- 仓库：`github.com/tangoui/tango`，Go 1.27。
- 所有数据库规则不适用；本任务只改 Go UI、主题、测试和文档。
- 默认值入口继续使用 `ConfigProvider(c).Defaults(component, Props)`。
- 现有组件使用 `Prop`、`ConfigProp`、组件专用 defaults helper；审计必须记录组件名和 key 的精确拼写。
- FooterToolbar 当前使用 `Theme.Components.FooterToolbar`（`Bg`、`FrostedBg`、`BlurRadius`）和 Runner 的 `noBlur` 检测，不应绕过这些 token。
- 不使用全局 `git add -A`；提交只包含任务相关文件。

## 5. 方案对比与已选方案

### 方案 A：逐个补明显漏项

扫描发现一个漏项就直接补一个 constructor 和测试。

- 优点：改动小，容易回滚。
- 缺点：没有覆盖矩阵，无法知道哪些组件已检查；新增组件仍可能漏接。
- 不选：当前组件数量已经较多，局部修补不能证明 Defaults 完整性。

### 方案 B：反射式统一 Defaults

给所有组件定义通用字段映射和反射应用层，构造器不再各自读取。

- 优点：理论上可以统一读取和类型转换。
- 缺点：会削弱 Go 类型安全，破坏组件显式优先级和内部 panel 边界，改动大且难调试。
- 不选：与现有 Tango 组件实现风格不符。

### 方案 C：覆盖矩阵审计 + 显式接线 + FooterToolbar 专项验收（已选）

先生成只读审计报告和矩阵；对真实缺口用现有 defaults helper 或小型 helper 显式接线；为 FooterToolbar 添加跨主题、renderer、RTL、flow/sticky、damage 的严格测试，并增加 gallery 验收页或扩展现有页。

- 优点：保留现有 API 风格，风险可控，结果可审查；能证明“已覆盖”和“未覆盖”的边界。
- 缺点：需要维护矩阵和若干测试，组件全部统一 Defaults 仍需未来逐批扩展。
- 选用原因：既解决当前缺口，又不会为了 Defaults 牺牲 Tango 的类型安全和渲染性能。

## 6. 实施设计

### 6.1 审计矩阵

不新建第二份权威 Defaults 总表；直接更新 `docs/specs/ui-coverage.md` §6.1，并在必要时补充审计备注。矩阵至少包含：

| 包 / 入口 | Defaults 组件名 | key | 是否已声明 | 读取位置 | 默认值 / 类型 | 显式覆盖方法 | 内部组件继承边界 | 测试 | 结论 |
|---|---|---|---|---|---|---|---|---|---|

本轮只审计根 `ui` 包的 177 个公开入口。`ui/mobile`、`ui/chart`、`ui/editor`、`ui/editor/diffview`、`ui/graph` 不纳入本轮分母；它们若未来需要 Defaults，要另立子包 API 设计。

审计来源按优先级排列：

1. `docs/specs/ui-coverage.md` §6.1 的既有清单；
2. `ConfigProvider.Defaults` 和 `ConfigProp` 的实现；
3. 组件注释中的 Defaults 文档；
4. 现有 defaults helper、`defaults_more_test.go` 和组件构造器 / 链式 modifier。

矩阵结论必须区分：

- `已声明且已接线`
- `已声明但未接线`
- `未声明，不是 bug`
- `复用其他组件 key`
- `内部 panel 明确不继承`
- `没有可逆的显式 modifier，因此显式覆盖不适用`

“未声明”不自动视为缺陷，本轮不为未声明组件新增公开 key。

### 6.2 Defaults 接线

对每个确认缺口：

- constructor 创建对象后立即调用对应 defaults helper；
- defaults 只写入尚未被显式设置的字段，或按当前实现的 `sizeSet` / `formatSet` / `phSet` 等标志处理；
- 显式 modifier 在 defaults 之后执行并覆盖默认值；
- 不把外层 Defaults 自动传给明确标注“不继承”的内部 panel；
- 新增组件的 defaults key 只选择稳定、可序列化、无需回调的值；回调和复杂对象不通过 Props 注入。

测试采用表驱动方式，分别验证：默认生效、显式覆盖、嵌套 provider 内层覆盖外层、错误类型不 panic、内部 panel 的继承边界。

### 6.3 FooterToolbar 验收

扩展现有 `ui/blur_test.go`，实现与断言不重写 FooterToolbar：

- 主题矩阵：Ant / Element / shadcn × light / dark / HighContrast light / HighContrast dark；
- 状态矩阵：flow、sticky 初始帧、滚动到 affixed、继续滚动、滚回脱离 affixed、窗口 resize 前后；默认 viewport 和 `Target(scrollBox.ID())` 的滚动容器都要覆盖；
- renderer 矩阵：CPU / 支持 blur 的 GPU 测试替身 / `noBlurRenderer`；
- sticky 只有在 `Affix.Affixed()` 后才允许发出 `BackdropBlur`，测试必须按帧序列验证，不能仅在首次构建后断言；
- 支持 blur 的 sticky：`BackdropBlur.Radius == FooterToolbarToken.BlurRadius`，矩形等于 FooterToolbar 的 box rect，随后使用 `FrostedBg`；
- flow：不发出 blur，使用 `Bg`；
- no-blur renderer 与 HighContrast：不发出 blur，最终 Fill 使用 `theme.BgContainer` 的 alpha 255 不透明背景；
- 断言阴影 / 顶部边线按三套主题现有约定工作，语义角色为 Toolbar、默认文案存在、按钮可键盘到达；
- RTL 采用逻辑布局：Extra 位于 inline-start，actions 位于 inline-end，RTL 时物理左右交换；surface 和 blur 矩形仍覆盖整个 bar；专门断言 Extra/actions、bar rect 和 blur rect，不把全局 MirrorCheck 当作唯一依据；
- 严格重绘序列：初次绘制可 full-frame；blur reach 内滚动必须包含 bar rect；reach 外变化不应额外扩大到整个 viewport；blur 状态切换要覆盖旧 / 新 bar 区域；无事件时不持续产生 full-frame damage；GPU strict verify 必须通过；
- 对比度：用现有 contrast audit 方法检查三套主题的背景、正文和按钮。

扩展现有 `examples/ui-gallery/page_pro2_page.go` 的 FooterToolbar 展示，不新建重复组件页；修改 `examples/ui-gallery/highcontrast.go` 的页面白名单，使 FooterToolbar 能参与 HighContrast gallery。截图只作为视觉核对，不替代 display-list、像素、语义和 damage 测试。

### 6.4 文档

- `docs/specs/ui-coverage.md` §6.1：更新 Defaults 审计矩阵，不另建第二份权威清单。
- `ui/AUTHORING.md`：新增组件时必须在构造器接 Defaults（只有明确声明支持时）、显式 modifier 覆盖，并加入矩阵 / 测试的规则。
- `docs/specs/ui-coverage.md` 中关于 shadcn FooterToolbar“没有毛玻璃”的旧记录要与当前实现统一：FooterToolbar 是 Tango 对 Pro 的适配，Element / shadcn 的 blur 是 Tango 主题适配，不宣称来自官方组件默认值。
- `CHANGELOG.md` 只记录实际代码修复，不把纯审计误写成新增功能。

## 7. 验证

- `gofmt -l` 对新增 / 修改 Go 文件为空。
- `go vet ./ui/...` 与 `go test -count=1 ./ui/...`。
- `go test -count=1 ./examples/ui-gallery/...`。
- `GOOS=windows go build ./ui/...`、`GOOS=linux go build ./ui/...`、`GOOS=js GOARCH=wasm go build ./ui/...`。
- API：先检查是否有新增 Defaults key；若有，运行 `go run ./internal/cmd/apitxt -w`，检查生成 diff 后再 `go run ./internal/cmd/apitxt -check`；只有审计文档变化时不无条件改 API golden。
- gallery：扩展后的 FooterToolbar 页面运行 `-theme all`、`-dir both`、`-theme hc` 和 `-mirror-check` / `-audit`；HighContrast 页面白名单已包含 FooterToolbar。
- damage：使用 `runner_damage_test.go` 的 damage presenter 和 gallery GPU strict verify；以 dirty rect 并集是否覆盖 viewport 定义 full-frame。
- 不宣称整个 M2 验收通过；若性能计时受机器负载影响，报告单测结果与独立复测结果。

## 8. 决定

- Defaults 审计权威范围：`docs/specs/ui-coverage.md` §6.1 的根 `ui` 177 个公开入口；本轮只审计已声明契约，不为未声明组件新增 Defaults key。
- Chat 不纳入本轮 Defaults；未来若纳入，按每个子组件命名，不建立统一 `Chat` 命名空间。
- `ui/graph` 与 `ui/editor/diffview` 不纳入本轮 Defaults 审计。
- no-blur renderer 与 HighContrast 的 FooterToolbar 回退使用 `theme.BgContainer` 的 alpha 255 不透明背景。
- FooterToolbar 验收覆盖 Ant / Element / shadcn 的 light、dark、HighContrast light、HighContrast dark；sticky Target、状态转换、窗口 resize、damage 和 GPU strict verify 都纳入。
- RTL 使用逻辑布局：Extra 在 inline-start，actions 在 inline-end，RTL 时物理左右交换。
- FooterToolbar 加入 HighContrast gallery 页面白名单。
- 新增 Defaults key 若未来出现，必须先明确公开契约并更新 API golden；本轮不新增。

## 9. 风险与默认决策

- 组件数量大、默认文档历史不完全一致：矩阵将“未声明”和“已声明但漏接”分开，避免过度扩 API。
- FooterToolbar GPU 真实后端跨平台差异：先用现有 CPU / no-blur / GPU 测试替身验证契约，再保留 macOS 实机截图验证。
- 已确认 no-blur fallback 不透明，颜色固定来自 `theme.BgContainer`，不由实现者自行猜测。
- 新增 defaults 可能改变已有页面：每个确认缺口必须有前后行为测试；默认值只按组件文档或设计基线确定。
- 默认不为 FooterToolbar 新增 API；默认不把未声明的历史组件升级为支持 Defaults。

## 10. 完成标准

- 审计矩阵覆盖所有当前公开组件构造器和已有 Defaults 文档。
- 所有“已声明支持”组件都有至少一个默认生效 + 显式覆盖测试，或明确记录为什么不适用。
- FooterToolbar 通过完整状态矩阵与 damage / contrast 检查。
- 代码、文档、API 检查和相关测试通过；结果中明确列出仍未声明 Defaults 的组件，不把“未声明”误报成缺陷。
