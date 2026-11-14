# 组件规范逐项对齐（Ant Design 6.7 / Pro v6 / Element Plus 2.14.8 / shadcn v4）

> 状态：已批准（2026-11-14；审阅意见已并入 §7.1）
> 基线规则见 [docs/design-baseline.md](../design-baseline.md)

## 1. 为什么做

用户要求 Tango 自绘 UI 严格按最新规范来做，并且让以后所有 AI 都按同一套规范开发：

- Ant Design 6.7
- Ant Design Pro v6
- Element Plus 2.14.8
- shadcn/ui v4

现有的 [ui-coverage.md](ui-coverage.md) 不能当作对齐依据，原因有三：

- Pro 部分是按 procomponents v2 的印象整理的。
- 组件细节没有逐页对照官方文档。
- 判断“完整”的标准是有同名入口加上主要 API，不看默认值、令牌和交互细节。

抽查的结果：

- Element Segmented 缺“自定义每项内容”。
- Pro Components v3 导出的 9 个组件 Tango 完全没有。
- shadcn 主题还是 Tailwind v3 的灰阶 hex 值，默认圆角 8px，组件清单也是旧的。

成功的标准：每个组件都有一份可以复查的对照记录，写明对照了哪个页面或源码文件、在哪一天。每个缺口都有结论，要么已补齐并有测试，要么写明是 Go 风格下的等价写法，要么写明因为平台或渲染层的原因暂时做不到。

## 2. 目标、非目标、反目标

**目标**

- 四套基线逐个组件核对：功能、默认值、受控和非受控、交互和键盘、禁用和加载状态、尺寸和令牌、明暗主题。核对完把缺口补到代码里，同时补示例（gallery 页）和回归测试。
- Pro v6 / Pro Components v3 新增的组件全部补上（§4.4）。
- shadcn 主题改成 v4：oklch 变量值、默认圆角 0.625rem、v4 的主题色列表，并补齐 `new-york-v4` 里新增的组件。
- 用新的覆盖率记录替换 `ui-coverage.md` 里过时的结论。

**非目标**

- 不照搬 Vue / React 特有的写法。插槽、render prop、`ref`、`useXxx`，以及 `props` 字段映射这种只因 JS 弱类型才需要的配置，都改成 Go 的等价能力，或者注明不需要。
- 不做官方没有的扩展。
- 不改基础层的架构（Runner、布局求解、渲染器）。缺基础能力时单独提出来，比如 FooterToolbar 毛玻璃需要背景模糊原语。

**反目标**（技术上做到了也算失败）

- 只加同名入口、不对齐行为就标“完整”。
- 为了对齐 Element 或 shadcn，改坏了 Ant 主题的默认外观。Ant 是默认主题，也是参考实现。
- 破坏稳定层 API（`api/stable` 白名单和稳定包）。新符号默认是实验性的。
- 凭印象写结论，不注明来源。

## 3. 约束

- 主工作区 `/Users/admin/min/tango` 同时有好几个会话在写，比如编辑器、终端、更新窗口。本工作在独立 worktree `/Users/admin/min/tango-align`（分支 `spec-align`，从 origin/main 开）里做，不碰别人还没提交的文件。
- 提交作者固定为 `tango <903300326@qq.com>`，不加 Co-Authored-By。可以在 `spec-align` 分支上按批次本地提交；合并到 main 或推送要用户点头。
- 官方源码统一下载到 `/tmp/tango-ref`，不进仓库：
  - `element-plus@2.14.8`
  - `@ant-design/pro-components@3.1.14-2`（Ant Design Pro v6.0.3 依赖的就是 `^3.1.14-2`）
  - `antd@6.7.0`
  - shadcn-ui/ui 仓库的 `apps/v4/registry`，锁定到 2026-11-14 当天的 commit
- 精确数值，比如令牌、尺寸和 oklch，都从这些源码里读，不估值。
- 只有 macOS 能实机看；Windows、Linux 只做编译和单测。

## 4. 范围和清单

### 4.1 Element Plus 2.14.8

范围是官网组件页的全部组件（约 79 个），按官网的分组走。每页都对照文档里的属性、事件、插槽、方法、示例，以及组件级 CSS 变量；需要确认默认值时读包里的 `es/components/<名>` 源码和 `theme-chalk`。

### 4.2 Ant Design 6.7

范围是官网组件总览的 72 个组件（不含 Util）。先对照组件的 API 表和组件令牌（`antd@6.7.0` 的 `es/<组件>/style/token`），再看 v6 的变化：语义化 `classNames`/`styles` 在 Tango 里对应什么、新增组件、List 已弃用。

### 4.3 shadcn/ui v4

令牌部分：

- 按 `themes.ts` 的 oklch 值重写 `ui/theme/shadcn.go` 的明暗变量，包括 sidebar、chart、暗色下的半透明边框和输入框。
- 默认 `Radius` 改成 10px（0.625rem）。
- 主题色按 v4 的列表来：neutral、stone、zinc、mauve、olive、mist、taupe，以及 amber 到 yellow 的彩色主题。
- 需要实现 oklch 转 sRGB，放在 `ui/theme/color.go`，并加测试。

组件部分：逐个对照 `new-york-v4/ui` 的 61 个组件，看尺寸、间距、状态和动效。v4 新增、而 Tango 没有对应物的，按需要补上：

- Spinner、Field、InputGroup、NativeSelect、Sonner、Empty、Combobox
- AI 对话类的 Attachment、Bubble、Message、MessageScroller、Marker（这一类先和 comp-chat 分支核对，避免重复，见 §7）

### 4.4 Ant Design Pro v6 / Pro Components v3

范围是 v3 导出的全部组件（card、descriptions、field、form、layout、list、provider、skeleton、table 九个模块），并对照 Ant Design Pro v6 模板里的页面模式：登录页、列表页、表单页、详情页、结果页和设置抽屉。

v3 有、Tango 目前完全没有的组件：

- 表格：`CellEditorTable`、`RowEditorTable`、`TableDropdown`、`ListToolBar`
- 布局：`ProBreadcrumb`、`PageLoading`、`TopNavHeader`
- 表单：`ProFormCaptcha`、`ProFormSegmented`

`ui-coverage.md` 里标“完整”的 28 项 Pro 组件，一律按 v3 重新核对。

## 5. 方案

考虑过三种方案：

1. **先全量核对，再集中补代码**：全局缺口最清楚，但第一批成果出来得最晚，而且核对表很快会和代码脱节。
2. **按组件组逐批闭环（用户选了这个）**：每组先核对官方页面和源码、写记录，再实现、测试、更新示例，然后进入下一组。发现跨组件的基础能力缺失时，集中处理一次。
3. **按设计体系分开做**（先 Element 全部，再 Ant、shadcn）：同一个组件要被改三次，冲突最多。

选了方案 2，因为每批都能独立验收，而且同一个组件的四套规范在同一批里一起对，改一次就够了。

## 6. 批次

| 批 | 内容 |
|---|---|
| 0 | 准备：下载并锁定参考源码；写对照记录模板；**shadcn v4 令牌和 oklch**。放第一批是因为它会改所有 shadcn 截图，后面各批的视觉对照要建立在新令牌上 |
| 1 | 基础和表单输入：Button、Link、Icon、Typography、Divider、Space、Layout/Container、Grid、Input 系列、InputNumber、InputOTP、InputTag、Select/SelectV2、AutoComplete、Mentions、Cascader、TreeSelect、Checkbox、Radio、Switch、Slider、Rate、ColorPicker、DatePicker、TimePicker、TimeSelect、Upload、Transfer、Form、Segmented，以及 shadcn 的 Field、InputGroup、NativeSelect、Combobox、Toggle |
| 2 | 数据展示和表格：Table/TableV2、Tree/TreeV2、Descriptions、List/Listy、Card、Collapse、Tabs、Tag、Badge、Avatar、Image、Carousel、Calendar、Statistic、Timeline、Empty、Skeleton、Progress、QRCode、Watermark、Masonry、Tooltip/Popover |
| 3 | 导航和反馈：Menu、Menubar、Dropdown、Breadcrumb、Pagination、Steps、Anchor、Affix、Backtop、PageHeader、Tour、Alert、Message/Sonner、Notification、Modal/Dialog/MessageBox、Drawer/Sheet、Popconfirm、Result、Spin/Loading/Spinner、Splitter/Resizable、Command、ContextMenu、HoverCard、NavigationMenu、Sidebar |
| 4 | Pro：§4.4 全部，以及 Ant Design Pro v6 的页面模式在 gallery 里各有一个完整示例页 |
| 5 | 收尾：重写覆盖率文档，三套主题加明暗全页重新截图并检查，CHANGELOG |

**每一批的步骤：**

1. 逐个组件写对照记录（`docs/specs/align/<批>-<组>.md`）。每个组件一张表，列是规范项、官方定义（来源和日期）、Tango 现状（文件和符号）、结论（已对齐 / 补齐 / 等价写法 / 暂不做加原因）。
2. 实现缺口。新组件和新方法按 [ui/AUTHORING.md](../../ui/AUTHORING.md) 的写法，接上 `ConfigProvider.Defaults`、语义树（a11y）和三套主题。
3. 测试：每个补上的缺口至少一条行为测试；令牌数值和官方源码比对的测试放在 `ui/theme`。
4. gallery：新能力有演示，三套主题加明暗都能渲染。
5. 验证：`go vet ./ui/... ./uihost/...`，`go test ./ui/... ./uihost/... ./examples/ui-gallery/...`（gallery 的局部重绘校验测试很慢，单独跑，放宽超时），apitxt 和 apidiff 不报稳定层破坏，gallery `-audit` 没有新问题。
6. 在 `spec-align` 上提交一次，向用户汇报这批补了什么、还剩什么、哪些没验证。

**并行**：每批内部按组件拆给 2 到 4 个子 agent，每个 agent 独占一组文件；共享文件（theme 令牌、locale、Defaults 表、gallery 注册表）由主线程统一合并。

## 7. 已定的取舍（我的决定，用户可以改）

- shadcn 旧的 `slate`、`gray` 主题色 v4 已经没有了，但继续接受这两个名字，按 v3 的值保留，标“旧名”。这样不破坏现有调用。
- 默认圆角 8→10、颜色数值改变，属于视觉变化，不算 API 破坏。写进 CHANGELOG 的“变更”一节。
- `ui-coverage.md` 不再在原文件上修补，最后一批重写成新格式，指向 `docs/specs/align/` 下的记录。
- AI 对话类组件（Bubble、Message 等）和 `comp-chat` 分支高度重叠：comp-chat 合并前先只核对、不实现，合并后再按 shadcn v4 补外观差异。
- Element 的 `props` 字段映射、`validate-event`、`name`/`id` 这类只服务 HTML 表单的属性，判为“不需要”，不实现。

## 7.1 审阅后补充的规则

- **四套规范冲突时**：一个组件只有一套 API，取各家功能的并集。功能只有一家有时（比如 Element Segmented 的自定义项内容），照样加进这个统一 API，所有主题都能用。默认值不一样的地方，按主题分别给，通过主题令牌或 `ConfigProvider.Defaults` 实现，不在组件里按 `th.Flavor` 写分支。行为上以 antd 6.7 为准；Element 和 shadcn 只有在官方行为明显不同、而且属于这套设计语言自己的特征时，才用令牌区分（我的决定）。
- **去重**：遵守 [ui/AUTHORING.md](../../ui/AUTHORING.md) 第 14 节，每个组件只有一个公开入口。shadcn 的 Sonner = Message/Notification、Spinner = Spin、Field = Form.Item、Empty = Empty、Sheet = Drawer，这些都只补外观和缺的能力，**不新增同义入口**。§4.3 列出的“v4 新增组件”先在总表里找对应物，真没有对应的（比如 InputGroup、NativeSelect、Combobox 若和 Select/AutoComplete 不同）才新建，文件名用 `shad_` 前缀。
- **默认值或行为要对齐规范时**（用户定）：已有组件的默认值或交互行为不符合规范的，直接改成规范的样子，不加保留旧行为的开关，在 CHANGELOG 的“变更”一节逐条写明。API 签名照样不能破坏稳定层。
- **批次节奏**（用户定）：每批做完在 `spec-align` 上提交并汇报，不等用户验收，接着做下一批；用户随时可以叫停。
- **缺基础能力时**：标为“暂不做，缺 X”，记进本批汇报里的“基础能力缺口”清单，然后继续往下做，不阻塞本批。基础能力由用户决定谁做、什么时候做。
- **分支同步**：每批提交前 rebase 到最新的 origin/main，并跑一遍本批的验证命令；共享文件冲突由主线程处理。comp-* 分支合进 main 之后，下一批开始前同步（我的决定）。

## 8. 留给实施者决定

- 对照记录表格的具体列宽和措辞。
- 每批内部怎么拆给子 agent。
- 测试放在已有的测试文件里，还是新建文件。
