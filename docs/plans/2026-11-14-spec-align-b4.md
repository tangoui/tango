# 规范对齐第 4 批：Pro 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal：** 按 Ant Design Pro v6 / `@ant-design/pro-components@3.1.14-2` 逐项复核 card、descriptions、field、form、layout、list、provider、skeleton、table 九个模块的 28 个已覆盖组件，补齐 9 个 v3 组件，并在 gallery 提供六种 Pro 页面模式的可运行示例。

**Architecture：** 沿用现有 `ui/pro*.go`、`ui/form_pro*.go`、`ui/table_pro.go` 和 gallery 注册方式；按文件边界拆成 4 组，组内先记录官方 API/令牌/默认值，再以现有 Tango 组件能力实现和测试。theme、locale、ConfigProvider.Defaults、API goldens、gallery 共享注册及 CHANGELOG 只由主线程合并；基础层没有能力时记录“暂不做，缺基础能力”，不绕过渲染或布局层。

**Tech Stack：** Go；Tango 自绘 UI；参考源码 `/tmp/tango-ref/antd@6.7.0`、`/tmp/tango-ref/pro@3.1.14-2`、`/tmp/tango-ref/element-plus@2.14.8`、shadcn v4；gallery 三套主题和明暗变体。

**Spec：** `docs/specs/2026-11-14-spec-alignment-design.md` §4.4、§6、§7.1；API 稳定规则见 `docs/release.md`；作者约定见 `ui/AUTHORING.md`。

## Global Constraints

- 只在 `/Users/admin/min/tango-align` 的 `spec-align` 分支修改；不碰主工作区和其他 worktree；禁止裸 `git stash`。
- 精确 API、默认值、令牌和行为从上述锁定版本源码读取，不按记忆补写；每个组件记录来源文件和核对日期。
- 四套规范取功能并集为一套 Go API；行为以 antd 6.7 为准，Element/shadcn 的差异只通过主题令牌或 Defaults 表达，不在组件中按 `th.Flavor` 分支。
- 每个组件只能有一个公开入口；现有组件的同义 shadcn 名称只补外观或缺失能力，不新增重复入口。
- 稳定 API 只能新增，不能给既有函数加参数、改变枚举值或破坏零值；新稳定类型的方法会进入稳定承诺，API 记录必须同步更新。
- 新组件接入 `ConfigProvider.Defaults`、语义树/a11y、三套主题和明暗渲染；行为测试覆盖键盘、禁用、加载、受控/非受控等适用状态。
- 默认值或交互按规范改变时直接改，并只在 CHANGELOG 的 `## [未发布]` 下记录；不修改历史发布条目。
- 实施 agent 不提交；最终主线程 rebase 最新 `origin/main` 后由主线程以 `tango <903300326@qq.com>` 提交，不添加 Claude 署名，不推送、不合并。
- 每个里程碑把带时间戳的结果写入 `.superpowers/sdd/2026-11-14-spec-align-b4/progress.md`。

## Review Focus

- **Pro v3 新组件的真实 API 与返回类型：** 不把 React render-prop 或 ref 机械搬到 Go；每个新增入口必须有对应官方源码记录和行为测试。由各组新增组件测试覆盖。
- **编辑表格的提交/取消/校验边界：** 单元格编辑和整行编辑必须区分，取消不得泄漏暂存值，校验错误不得提交。由 table 组行为测试覆盖。
- **Pro 表单异步与倒计时状态：** Captcha 的发送、禁用、倒计时和失败恢复不能依赖睡眠竞态；由 form 组状态测试覆盖。
- **ProLayout 页面模式的导航/a11y：** breadcrumb、顶栏、加载页在键盘、折叠和窄宽度下仍有名称、角色和可达焦点。由 layout/gallery 组测试和 audit 覆盖。
- **主题与 Defaults 隔离：** Ant、Element、shadcn 的令牌不能互相污染，明暗主题均须有对比度；由 provider/主题测试及 gallery `-audit`、`TestGalleryTextContrast` 覆盖。

## 文件边界与接口约定

- **组 A（card/descriptions/field/skeleton）：** 主要修改 `ui/pro2_card.go`、`ui/pro3_descriptions.go`、`ui/pro3_fieldset.go`、`ui/pro2_skeleton.go` 及对应测试/记录；不得直接改共享 Defaults/theme。
- **组 B（form）：** 主要修改 `ui/form_pro*.go`、`ui/pro3_login.go` 及对应测试/记录；新增 `ProFormCaptcha`、`ProFormSegmented` 时沿用现有 `FormW`/`Field` 链式约定，不改共享注册。
- **组 C（layout/list/provider）：** 主要修改 `ui/prolayout.go`、`ui/pro3_list.go`、`ui/pro4_*.go` 及对应测试/记录；新增 `ProBreadcrumb`、`PageLoading`、`TopNavHeader`，并核对 provider/config/settings 既有 API。
- **组 D（table）：** 主要修改 `ui/table_pro.go`、`ui/table_edit.go`、`ui/table_*.go` 及对应测试/记录；新增 `CellEditorTable`、`RowEditorTable`、`TableDropdown`、`ListToolBar`，不得改动组外组件实现。
- **共享主线程文件：** `ui/theme/*`、locale 文件、Defaults 注册、gallery 注册/页面公共 helper、`api/go/*`、`api/stable/*`、`CHANGELOG.md`。组 agent 只在记录中提交改动请求。

---

### Task 0：准备、基线和官方源码核对

**Files：**
- Create: `docs/plans/2026-11-14-spec-align-b4.md`
- Create: `docs/specs/align/4-*.md`（由各组按模板创建）
- Modify: `.superpowers/sdd/2026-11-14-spec-align-b4/progress.md`

- [ ] 检查 `origin/main` 是否已有 comp-* 对应实现；若已有，记录符号和行为，禁止回退。
- [ ] 在 `/tmp/tango-ref` 中逐模块读取 Pro v3 导出列表、组件源码、默认 props/令牌和 Pro v6 页面模板；将每项结论写入 4 组记录，注明官方路径和 `2026-11-15` 核对日期。
- [ ] 从 `docs/specs/ui-coverage.md` 找出标记完整的 28 项，逐项映射到现有 Tango 符号，明确“完整”仍成立或需要补齐。
- [ ] 建立进度文件并记录准备完成时间；不得把参考源码复制进仓库。

### Task 1：组 A——Card、Descriptions、Field、Skeleton

**Files：**
- Modify: `ui/pro2_card.go`, `ui/pro3_descriptions.go`, `ui/pro3_fieldset.go`, `ui/pro2_skeleton.go`
- Test: `ui/pro3_test.go` 或对应现有测试文件
- Create: `docs/specs/align/4-card-descriptions-field-skeleton.md`

- [ ] 先写覆盖表：每个组件记录 API、默认值、响应式/加载/空状态、令牌、a11y、三套主题结论；把缺口和共享文件请求列出。
- [ ] 为每个缺口先写行为测试；至少覆盖 Card actions/tabs/loading/split、Descriptions 响应式和 request/inline edit、Field 模式/枚举/只读、Skeleton active/paragraph/title/size。
- [ ] 补实现并复核公开构造函数返回专属类型、链式方法兼容性、Defaults 读取点；若缺渲染层能力，记录暂不做而不是伪造。
- [ ] 运行组内 `go test ./ui -run 'Pro(Card|Descriptions|Field|Skeleton)'` 与 `go vet ./ui/... ./uihost/...` 的局部检查，修复失败。
- [ ] 独立审阅本组：CONFIRMED 问题修复后更新记录、测试结果和共享请求，并写进度。

### Task 2：组 B——Form 与 ProForm 新增组件

**Files：**
- Modify: `ui/form_pro.go`, `ui/form_pro_field.go`, `ui/form_pro_types.go`, `ui/form_pro_test.go`, `ui/pro3_login.go`
- Test: `ui/form_pro_test.go`, `ui/pro3_test.go`
- Create: `docs/specs/align/4-form.md`

- [ ] 先对照 Pro v3 form 导出和 Pro v6 登录/表单模板，写出 `ProFormCaptcha`、`ProFormSegmented` 的 Go 等价 API；不引入 React props/ref/render-prop。
- [ ] 写失败测试：Captcha 发送成功/失败、倒计时期间禁用与完成恢复；Segmented 受控值、选项禁用、自定义项内容和键盘切换。
- [ ] 实现新增组件并复核既有 ProForm、QueryFilter、StepsForm、ModalForm、DrawerForm、LoginForm 等 28 项覆盖结论；API 只能新增，稳定类型新方法进入 API 承诺。
- [ ] 接入统一 locale/Defaults 的改动整理为共享请求，不直接覆盖共享文件；三主题令牌差异写入记录。
- [ ] 运行 `go test ./ui -run 'ProForm|Captcha|Segmented|LoginForm'`；独立审阅并修复 CONFIRMED 问题，写进度。

### Task 3：组 C——Layout、List、Provider 新增组件

**Files：**
- Modify: `ui/prolayout.go`, `ui/pro3_list.go`, `ui/pro4_route.go`, `ui/pro4_app.go`, `ui/pro4_setting.go`
- Test: `ui/pro3_test.go` 及对应 layout/list 测试
- Create: `docs/specs/align/4-layout-list-provider.md`

- [ ] 对照 Pro v3 card/layout/list/provider 导出源码和 Pro v6 页面模板，逐项复核 ProLayout、PageContainer、GridContent、RouteContext、SettingDrawer、ProSettingsProvider、App、ProList 等现有 API。
- [ ] 写失败测试：`ProBreadcrumb` 的层级/当前项/a11y，`PageLoading` 的 loading 语义与取消/显示，`TopNavHeader` 的导航/折叠/用户区和窄宽度行为；同时覆盖 ProList 空/加载/分页/选择/展开/网格。
- [ ] 实现三个新增入口，遵守现有布局求解和语义树，不伪造 CSS/浏览器能力；确认专属返回类型和稳定 API 记录。
- [ ] 将设置抽屉、provider defaults、locale、gallery 注册等共享变更写请求，避免在组分支直接改共享文件。
- [ ] 运行 `go test ./ui -run 'Pro(Layout|List)|Breadcrumb|PageLoading|TopNavHeader|Setting|Route'`；独立审阅并修复 CONFIRMED 问题，写进度。

### Task 4：组 D——Table 四个新增组件

**Files：**
- Modify: `ui/table_pro.go`, `ui/table_edit.go`, `ui/table_model.go`, `ui/table_body.go`
- Test: `ui/table_test.go`, `ui/table_model_test.go` 或新增 `ui/table_pro_v3_test.go`
- Create: `docs/specs/align/4-table.md`

- [ ] 对照 Pro v3 table 导出和源码，写出 CellEditorTable、RowEditorTable、TableDropdown、ListToolBar 的 Go API、默认值、触发行为、令牌和 a11y 结论。
- [ ] 写失败测试：单元格编辑提交/取消/校验、整行编辑提交/取消/校验、Dropdown 键盘打开/选择/关闭、ListToolBar 搜索/筛选/密度/全屏等已有能力的等价组合。
- [ ] 实现新增组件，复用现有 `TableW`/编辑状态模型和 `DropdownW`/toolbar 能力；不得复制第二套表格状态机，不得改变既有 `ProTable` 签名。
- [ ] 处理默认列状态、分页、loading、空状态、批量操作和主题令牌；共享 Defaults/theme 请求单列记录。
- [ ] 运行 `go test ./ui -run 'Table|CellEditor|RowEditor|TableDropdown|ListToolBar'`；独立审阅并修复 CONFIRMED 问题，写进度。

### Task 5：主线程合并共享 API、主题、locale、gallery

**Files：**
- Modify: `ui/theme/*`, locale/Defaults 文件, `examples/ui-gallery/*`, `api/go/*`, `api/stable/*`, `CHANGELOG.md`
- Create: `examples/ui-gallery/page_pro4_patterns.go` 或按现有注册边界拆分页面文件

- [ ] 汇总四组共享请求，读取官方精确令牌后统一实现 Defaults、locale 和三套主题；不把主题判断写进组件逻辑。
- [ ] 在 gallery 添加六个完整页面模式且使用真实 API：登录页、列表页、表单页、详情页、结果页、设置抽屉页；每页注册 Ant/Element/shadcn 明暗变体，覆盖新增 Pro 组件，禁止伪造 API。
- [ ] 为新增公开符号运行 `go run ./internal/cmd/apitxt -w`，审阅稳定/实验分层；只新增 API goldens，不手工合并黄金文件。
- [ ] 在 `CHANGELOG.md` 仅 `## [未发布]` 下记录新增 Pro 组件、默认值变化和实验性 API；检查作者/提交者不写 Claude 署名。
- [ ] 运行 gallery `-audit`、`TestGalleryTextContrast`；失败按系统化调试处理并在进度中记录。

### Task 6：本批验证、复审与提交

**Files：**
- Modify: `.superpowers/sdd/2026-11-14-spec-align-b4/progress.md`, 4 组记录及必要代码

- [ ] 先获取最新 `origin/main`，确认工作树可安全 rebase；提交前 rebase，不使用裸 stash。
- [ ] 运行 `go build ./...`。
- [ ] 运行 `go vet ./ui/... ./uihost/...`。
- [ ] 运行 `go test ./ui/... ./uihost/...`。
- [ ] 运行 `go test -timeout 60m ./examples/ui-gallery/...`。
- [ ] 运行 `go run ./internal/cmd/apitxt -check` 和 `bash scripts/apidiff.sh`，稳定层无破坏。
- [ ] 运行 gallery `-audit`、`TestGalleryTextContrast`；`ui/editor` 性能失败时单独重跑确认并如实记录。
- [ ] 做一次全分支独立审阅，逐项修复 CONFIRMED 问题；同步更新“暂不做/基础能力缺口/待用户决定/未验证”。
- [ ] 写最终时间戳，确认 `git diff --check`、`git status` 和变更范围；以作者 `tango <903300326@qq.com>` 提交，不推送、不合并。

## 决定记录

- 采用按文件边界拆四组，避免四组同时修改共享 theme、locale、Defaults、gallery 注册和 API goldens；共享变更由主线程统一合并。
- 9 个 v3 缺失组件必须新增，但先检查 `origin/main` 和 comp-* 相关实现；若已有等价能力，优先扩展既有入口，不新增同义组件。
- Pro 页面模式采用当前 Tango API 的组合示例，不照搬 React 模板的 props/ref/render-prop；缺少基础布局、动画、背景模糊等能力时记录“暂不做，缺 X”。
- 稳定类型上的新增方法视为稳定承诺；新独立类型默认实验性，只有用户另行批准才加入稳定白名单。
- 现有 Pro 代码已含部分页面示例和 v2/v3 能力；第 4 批重点是逐项复核 v3 精确行为、补 9 个缺失入口和六种完整模式，不重复实现已有能力。

## 第4批旧28项复核索引（2026-11-15）

旧表的完整符号范围并非全部 v3 API；逐项默认值/行为/剩余缺口以下列记录为准。按设计第5批再整体重写 ui-coverage.md，本批不把旧✅继续当作无缺口声明。

|旧项目|本批记录|
|---|---|
|ProLayout|[对照记录](../specs/align/4-layout-list-provider.md)|
|SettingDrawer|[对照记录](../specs/align/4-layout-list-provider.md)|
|RouteContext / GridContent|[对照记录](../specs/align/4-layout-list-provider.md)|
|PageContainer|[对照记录](../specs/align/4-layout-list-provider.md)|
|FooterToolbar|[对照记录](../specs/align/4-layout-list-provider.md)|
|ProCard|[对照记录](../specs/align/4-card-descriptions-field-skeleton.md)|
|StatisticCard|[对照记录](../specs/align/4-statistic-checkcard.md)|
|CheckCard|[对照记录](../specs/align/4-statistic-checkcard.md)|
|WaterMark|不属于当前 Pro v3 layout/index.d.ts/index.d.ts 导出；沿用 antd Watermark，已在第2批 `2-media.md` 核对，不增加重复入口|
|ProTable|[对照记录](../specs/align/4-table.md)|
|EditableProTable|[对照记录](../specs/align/4-table.md)|
|DragSortTable|[对照记录](../specs/align/4-table.md)|
|ProList|[对照记录](../specs/align/4-layout-list-provider.md)|
|ProDescriptions|[对照记录](../specs/align/4-card-descriptions-field-skeleton.md)|
|ProForm|[对照记录](../specs/align/4-form.md)|
|ModalForm / DrawerForm|[对照记录](../specs/align/4-form.md)|
|QueryFilter|[对照记录](../specs/align/4-form.md)|
|LightFilter|[对照记录](../specs/align/4-form.md)|
|StepsForm|[对照记录](../specs/align/4-form.md)|
|LoginForm / LoginFormPage|[对照记录](../specs/align/4-form.md)|
|ProFormList|[对照记录](../specs/align/4-form.md)|
|ProFormFieldSet|[对照记录](../specs/align/4-card-descriptions-field-skeleton.md)|
|ProFormDependency|[对照记录](../specs/align/4-form.md)|
|ProFormGroup|[对照记录](../specs/align/4-form.md)|
|SchemaForm（BetaSchemaForm）|[对照记录](../specs/align/4-form.md)|
|ProForm 字段组件（ProFormText/Select…）|[对照记录](../specs/align/4-form.md)|
|ProField|[对照记录](../specs/align/4-card-descriptions-field-skeleton.md)|
|ProSkeleton|[对照记录](../specs/align/4-card-descriptions-field-skeleton.md)|

### 九个缺失名称的 Go 对应

- CellEditorTable / RowEditorTable / TableDropdown / ListToolBar：四个独立专属类型入口，复用既有编辑与工具栏状态。
- ProBreadcrumb / PageLoading / TopNavHeader：三个专属薄入口，复用 Breadcrumb / Spin / ProLayout 顶栏。
- ProFormCaptcha / ProFormSegmented：源码是字段包装器，已有 ValueCaptcha / ValueSegmented，补 ProField.Captcha / Segmented 配置而不新增同义入口。

### 六种完整页面模式

`examples/ui-gallery/page_pro4_patterns.go` 注册 ProPatternLogin / List / Form / Detail / Result / Settings；参考 `/tmp/tango-ref/adpro/repo@42f2cb1` 的 `src/pages/user/login`、`table-list`、`form/basic-form`、`profile/basic`、`result/success` 和 `src/app.tsx`。全是本地示例数据，登录、验证码与表单不连接真实服务。三套主题×明暗均通过 gallery 通用变体生成。
