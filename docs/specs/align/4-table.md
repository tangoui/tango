# 第 4 批 · Table（组 D）

核对日期：2026-11-15。基线：`@ant-design/pro-components@3.1.14-2`（Pro v6 配套），下文 `pro/` 为 `/tmp/tango-ref/pro/package/es/`。官方源码只读，不复制实现。检查当前 `origin/main` 与工作树：现有入口在 `ui/table_pro.go`、`ui/p2e_editable.go`，四个新增入口此前不存在。

## 重要判定

- **v3 源码并没有 `editable.type = 'cell'`**：`CellEditorTable.js` 是维护 activeColumnId、双击设 editableKeys 并按列开放 editable 的薄包装；`RowEditorTable.js` 是双击设 editableKeys 的薄包装。`utils/useEditableArray/index.js` 的 editable.type 仍是 `single`（默认）/`multiple`。Go 不新增假的 `cell` 枚举。
- 两个新入口使用专属 `*CellEditorTableW[T]` / `*RowEditorTableW[T]`，内部都复用 **同一个 EditableProTableW** 的 draft、校验、async generation、OnSave、write/cancel、TableW。新增状态仅模式、活动列与焦点，不复制表格编辑状态机。
- 源码包装的 blur 延迟 150ms 用来容纳浏览器 DOM 的焦点跳转；Tango 每帧统一构建焦点，使用编辑单元格/整行的焦点范围，不引入浏览器定时器。显式提交/取消/校验按本批验收要求：文本/数字 Enter 或离开范围提交、Escape 取消、校验失败保留草稿；选择器的 Enter 归选择器，最终 blur/Save 提交，不能一打开下拉就提交。
- 三行旧“完整”不能理解为逐 prop 完整。以下分别列出已对齐、Go 等价和未实现项，建议主线程据此更正 `ui-coverage.md`，不要继续无条件标“完整”。

## ProTable

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| request / params / pagination / sort / filter | `pro/table/typing.d.ts` request、params；`useFetchData.js`；`utils/index.js` mergePagination | `ui/table_pro.go:ProRequest/ProResult/Request/Params`；参数变化回第一页、异步结果序号防陈旧、分页 total | 已对齐（已有 Table/ProTable 回归） |
| 列 API / valueType / valueEnum / 搜索隐藏 / 表格隐藏 / 编辑字段 | `typing.d.ts:ProColumnType` 与 provider value types | `ProColumn`、`tableColumn`、`queryFilter`、`proFieldFromColumn` | 等价写法（类型化回调代替 dataIndex/ReactNode；列字段与 ProField 共用） |
| 工具栏 | `components/ToolBar/index.js` 将 title/actions/settings/toolbar 传给 ListToolBar | `ui/table_pro.go:toolbarRow` 现在调用公开 `ListToolBar`；`Toolbar(func(*ListToolBarW))` 配置，原 `ToolBar(func())` 不变 | 已补齐（TestListToolBarProTableSettings） |
| 全屏默认 | `components/ToolBar/index.js` options 合并时 `fullScreen:false`；reload/density/setting 开启 | `ProTableW.FullScreen(on)`；默认从显示改为隐藏，显式开启仍使用原 ScreenFill overlay | 已补齐（TestProTableFullscreenV3Default、TestListToolBarProTableSettings） |
| 列状态 / persistence | `typing.d.ts:ColumnStateType` value/defaultValue/onChange/persistenceKey/Type | `ColumnsState/OnColumnsStateChange/PersistColumnsState`；应用自供 ColumnsStateStore | 等价写法（不在组件私存 localStorage/sessionStorage） |
| 选择 / alert / polling / loading / empty / request error | `components/Alert/index.js`、`Table.js`、`useFetchData.js` | `Selection/TableAlertRender/TableAlertOptionRender/NoTableAlert/Polling/PausePolling/ResumePolling`；Table loading 与 Empty；错误 Alert/Retry | 已对齐（现有 Table、shad2d ProTable 回归） |
| 受控 / 非受控 | columnsState、pagination、params 与 callbacks | 指针绑定列状态、选择；内部 proState 管页码、排序、过滤，应用 Request 驱动 | 等价写法 |
| 尺寸 / 明暗主题 | antd `es/table/style/index.js` padding normal 16/16、middle 12/8、small 8/8；Pro toolbar 旧容器高64；ListToolBar padding 16/8、search 宽200 | 复用 TableW 主题与原 ProTable settings，ListToolBar 标题用 FontSizeLG、Semibold、TextHeading；三套明暗均出图 | 已对齐（继承基础 Table 的已对齐令牌；不另加 Flavor/Dark 分支） |
| a11y / 键盘 | 自绘等价 menu/grid/toolbar；官方 DOM 菜单键盘交互 | 新增 toolbar 角色；原 density/menu/column settings 键盘继续复用；修复鼠标打开密度菜单后初始焦点因 tooltip 撤销失效 | 已补齐（TestListToolBarProTableSettings） |
| DOM props / ref / render props | `typing.d.ts` actionRef/formRef/tableRender/tableViewRender/cardProps 等 | `Table()`/`Rows()`/`Reload()` 和组合回调 | 等价写法（不移植 CSS/React ref） |
| 未覆盖的 request 扩展 | `typing.d.ts` manualRequest、debounceTime、revalidateOnFocus、postData、beforeSearchSubmit、dateFormatter | Request 回调可自行格式化/转换/延迟请求；没有所有专属开关 | 等价写法（应用回调完成请求层策略；不宣称专属 API 完整） |

## EditableProTable

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| value/defaultValue/onChange / editableKeys | `components/EditableTable/index.d.ts`；`utils/useEditableArray/index.js` | `ui/p2e_editable.go:EditableProTable(c,cols,*rows)/EditableKeys` | 等价写法（应用持有 rows 指针，Saved/Changed 查询变更） |
| single / multiple / edit/save/cancel | `useEditableArray/index.js` 默认 single，校验后 save、cancel 清编辑值 | `Multiple/Edit/Save/Cancel/startEdit/stopEdit/save/write` | 已对齐（TestEditableProTableSingleRowAndCrossFieldValidate 与本批编辑回归） |
| Required / cross-field / async validation / OnSave | `EditableTable/index.js` 复用 ProForm；useEditableArray save validateFields | `ProColumn.Editor` 对应 ProField；同步、异步校验及 OnSave 错误均不写回；gen 防止取消后晚到结果 | 已对齐（TestCellEditorTablePendingEscape、TestCellEditorTableAsyncCancelAndProgrammaticRestrictions） |
| new record / maxLength / creator position | `EditableTable/index.d.ts:recordCreatorProps` cache/dataSource、top/bottom、parentKey | `NewRow/AddAtTop/MaxLength/AddRow` 新行以草稿保存，取消删除草稿 | 等价写法（cache 分支）；层级 parentKey 与直接 dataSource 创建见暂不做 |
| row actions / disabled / loading | 编辑 actionRender；单行时其他行编辑禁用；saving 按钮禁用 | `actionColumn/RowEditable/NoDelete/OnDelete`；等待禁用 editor | 已对齐（已有 EditableProTable 回归） |
| 尺寸 / 分页 / 明暗 / a11y | ProTable/Table inherited props | `Size/Pagination/ScrollX/Bordered/TableW`；editor 单元格由 p2eCellField 命名、Required/Invalid | 已对齐（既有 TestEditableProTableA11y；本批借用同一模型） |
| React Form 联动与受控重置 | `controlled`、name、editableFormRef、RecordCreator | 原生 Go rows 指针与方法 | 等价写法（不用 React Form/ref）；RecordCreator parentKey 暂不做 |

## DragSortTable

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| 导出 / 拖拽结束 / 新顺序 | `components/DragSortTable/index.d.ts` dragSortKey、onDragSortEnd(before,after,newDataSource) | `ProTable.DragSort/MovePageRow`；`EditableProTable.DragSort` 自动重排并回调 | 等价写法（保留既有方法入口，不新增同义表格；应用拿 Rows 或 *rows） |
| 把手 / 禁用 / 键盘 | `DragSortTable/index.js` HolderOutlined；style icon 宽/最小高 controlHeightSM、marginSM、grab | 复用 `TableW.DragSort` 默认把手和已有拖拽键盘/自动滚动能力 | 已对齐（TestEditableProTableDragSort 等已有 Table 回归） |
| 自定义把手所在列 / 内容 | `dragSortKey/dragSortHandlerRender` | 现有 TableW 专用把手列，没有等价自定义渲染 API | 暂不做（需基础 TableW 扩展；本组禁止改 table.go/table_show.go） |
| 数据/主题/默认 | 其他 ProTableProps 透传 | 同一 ProTable / EditableProTable | 等价写法（不维护第二份数据和主题） |

## CellEditorTable

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| 独立入口、默认边框与分页 | `components/EditableTable/CellEditorTable.d.ts/js` 返回 EditableProTable，bordered:true、pagination:false | `ui/table_pro_editor.go:CellEditorTable/CellEditorTableW`；同名 Defaults bordered/size/pagination | 已补齐（TestCellEditorTableCommitCancelValidate） |
| 单元格触发与定位 | 源码 activeColumnId（index+dataIndex/key），双击按行键和列定位 | 复用 TableW 的双击行、键盘活动列与现有 EditableProTable draft；双击或 Enter 开放一个列，非编辑列无 Editor | 已补齐（TestCellEditorTableKeyboardAndBlurValidation） |
| 提交/取消/校验 | 源码包装 blur 清 editableKeys；实际 save/validation 来自 EditableProTable | Enter/离开范围/Save 校验该列；Escape/Cancel 丢弃草稿；晚到校验/保存结果不写回 | 已补齐（TestCellEditorTableCommitCancelValidate、TestCellEditorTablePendingEscape） |
| 选择器 Enter | 包装不接管子控件按键；Select Enter 打开/选择 | 选择/日期/多行控件不被全局 Enter 提交；Save 或 blur 提交 | 已补齐（TestCellEditorTableSelectEnterDoesNotSubmit） |
| 应用状态 / OnSave / 行限制 | EditableProTableProps；rowKey、editable config | rows 指针、RowKey/RowEditable/OnSave/Edit(key,col)/EditingCell/Errors/Validating | 已补齐（TestCellEditorTableAsyncCancelAndProgrammaticRestrictions） |
| 禁用 / 只读 / 三主题 / a11y | 继承 Table/ProField | 禁用子树不进入编辑；隐藏/无 Editor 列不编辑；复用字段语义与令牌 | 已补齐（组内 keyboard/disabled/validation；6主题截图） |
| 任意重复/无 column key | 源码用 index 防标识冲突 | Go ProColumn.Key 本来就是字段身份，要求唯一非空 | 等价写法（不支持同一字段键重复定义成两个独立 editor） |

## RowEditorTable

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口 / 默认 | `components/EditableTable/RowEditorTable.d.ts/js` bordered:true、pagination:false | `ui/table_pro_editor.go:RowEditorTable/RowEditorTableW`；同名 Defaults | 已补齐（TestRowEditorTableCommitCancelValidate） |
| 双击 / 整行 / Tab | 所有列 onDoubleClick 开放该行；blur 延迟150ms不误关同一行内Tab | 双击任何单元格或 Enter 开放整行；同一行内编辑器 focus 不触发 blur save | 已补齐（TestRowEditorTableCommitCancelValidate） |
| 保存 / 取消 / 新行 | EditableProTable 校验与 recordCreatorProps | 同一个 e.save/e.stopEdit；Enter/blur 保存整行，校验失败不写，取消不泄漏；NewRow/AddRow/MaxLength | 已补齐（TestRowEditorTableCommitCancelValidate、TestRowEditorTableProgrammaticRestrictions） |
| 多行编辑 / 受控 editableKeys | EditableProTableProps editable.type single/multiple | 本入口是单行预设；需要同时编辑多行使用现有 EditableProTable.Multiple().EditableKeys(...) | 等价写法（不引入第二套 multiple 状态；预设本身未新增 Multiple，待主线程确认） |
| 禁用 / async / a11y / 明暗 | 同源 EditableProTable | 同一模型，RowEditable 与禁用树生效、同一字段校验语义；6主题出图 | 已补齐（TestRowEditorTableDisabled、程序化限制测试） |

## TableDropdown

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| menus / name / key / disabled / onSelect | `components/Dropdown/index.d.ts/js` 将 menus 转 antd Menu items | `ui/table_pro_dropdown.go:TableDropdown(c,[]MenuItem)` + OnSelect/Clicked | 已补齐（TestTableDropdownKeyboardDisabled） |
| ellipsis / children / Button | 默认 span ellipsis；Button 是普通 Button+DownOutlined，**不是 split button** | Show 默认本地化名的 More IconButton；Label 自定义文字；Button(label) 普通按钮+down | 已补齐（同测试及 gallery 两触发器） |
| 触发 / 键盘 / 禁用 / 关闭 | 继承 antd Dropdown，未覆盖 trigger（默认 hover） | 复用 DropdownW；默认 TriggerHover；TriggerClick 可选；Enter/Down 打开、上下选择跳过 disabled、Escape 关闭回焦点 | 已补齐（TestTableDropdownKeyboardDisabled） |
| Defaults / 状态 / 主题 / a11y | antd Dropdown config 与组件 menu | 同名 Defaults disabled/trigger/placement；Open(*bool)；复用 Dropdown tokens、popup语义/焦点 | 已补齐（同测试；6主题截图） |
| React children/className/style | ReactNode 与 CSS | Label/Button/Go 内容组合与统一主题 | 等价写法（不新增 CSS 字段） |

## ListToolBar

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状 | 结论 |
|---|---|---|---|
| title/subTitle/tooltip/search/onSearch/actions/settings/filter | `components/ListToolBar/index.d.ts/js` | `ui/table_pro_toolbar.go:ListToolBar` 各同义链式方法；Actions/Settings/Filter 是构建回调 | 已补齐（TestListToolBarComposition） |
| 搜索位置 / 默认 | search:false、multipleLine:false；有title搜索在右、无title在左；输入占位符 intl；search宽200 | Defaults title/subTitle/tooltip/search/multipleLine；Search 指针或 Defaults 内部值；Locale.Placeholder/Search；OnSearch 提交 | 已补齐（TestListToolBarComposition） |
| tabs/filter 多行 | tabs 仅 multipleLine=true 才显示；filter 放第二行或 tabs.extra | Tabs + MultipleLine(true)；TabsW.Extra(filter) | 已补齐（TestListToolBarComposition） |
| menu | `HeaderMenu.js` inline（默认）/tab/dropdown、active/defaultActive、disabled | Menu(kind,*active,[]TabItem)；inline/tab 用同一可键盘 TabsW，dropdown 用 TableDropdown；nil key 为内部状态 | 等价写法（inline 外观采用原生 TabsW，不照搬无键盘的 div） |
| settings 与 ProTable | ToolBar/index.js 将刷新/密度/列设置/全屏构建成 settings 传入 ListToolBar | 公开 Settings 回调；ProTable toolbarRow 调同一 ListToolBar，旧 toolbarSettings/densityMenu/columnSettings 的状态原封复用 | 已补齐（TestListToolBarProTableSettings） |
| 令牌 / 明暗 / 禁用 / a11y | style.js 标题 colorTextHeading/fontWeightStrong/fontSizeLG，gap marginXS=8；工具栏 padding=16/8，search200 | 复用 Theme 字号/颜色、布局常数来自源码；RoleToolbar；交互由 Search/Tabs/Dropdown/IconButton 提供键盘、名称、禁用 | 已补齐（组内组合测试；6主题截图） |
| 搜索 async 返回 false、resizeObserver375窄屏布局 | ListToolBar/index.js onSearch 等待 Promise，false 拦截后续；宽<375纵向 | 应用在 OnSearch 回调自行启动异步，未新增 Promise协议；组件未按实时容器宽度切布局 | 等价写法（异步回调）；窄屏实时容器测量见暂不做 |

## 暂不做与基础能力缺口

| 缺口 | 影响组件 | 需要能力 | 现状 |
|---|---|---|---|
| 自定义拖拽把手及所在列 | DragSortTable | TableW 新把手渲染/列身份接口，涉及禁止修改的 table.go/table_show.go | 暂不做，主线程决定基础Table API |
| RecordCreator.parentKey / newRecordType=dataSource | EditableProTable | 编辑树行身份和嵌套数据写回协议；当前编辑表格是平面 *[]T | 暂不做，不伪造嵌套数据支持 |
| ListToolBar 实时容器<375纵向响应式 | ListToolBar | 容器尺寸驱动布局的统一能力（不能用窗口宽度冒充容器宽度） | 暂不做；宽屏截图已验证，窄屏未验证 |

共3项暂不做；另 RowEditorTable 多行编辑使用既有 EditableProTable.Multiple，是否要求薄预设本身也暴露 Multiple，请主线程确认。Pro v6 页面模板不在本地 npm 包内，本次未核对模板，不凭知识补结论。

## CHANGELOG 草稿

### 新增

- 实验性入口：CellEditorTable、RowEditorTable、TableDropdown、ListToolBar，均返回专属 W 类型；单元格/整行编辑复用 EditableProTable 的 draft/校验/异步保存，不改变既有 ProTable/EditableProTable 函数签名。
- 实验性 ProTableW 新方法 Toolbar(func(*ListToolBarW))、FullScreen(bool)；原 ToolBar(func()) 保留。
- 本组没有在稳定类型上新增方法（TableW 稳定白名单未变）；ProEnum.Disabled 由主线程为组B添加，本组保留。

### 变更

- ProTable 全屏选项：默认显示 → 默认隐藏（pro/table/components/ToolBar/index.js 合并 options 时 fullScreen:false）；需要显示时调用 FullScreen(true)。主线程已为 a11y/i18n 原全屏用例显式开启保留测试覆盖。
- ProTable toolbar：内部单独绘制 → 复用公开 ListToolBar；标题 Ant 原 Medium500 → Semibold600（ListToolBar style fontWeightStrong），组合增加8px统一间距；基础 Table 的外观/行为未改。
- 密度菜单：鼠标打开时 hover tooltip 撤销可能导致首次键盘焦点丢失 → 初始焦点请求保持到菜单首次稳定绘制，且先回焦点再处理漫游键盘事件。
- 已有测试期望：`ui/table_test.go:TestTableFilterDropdown` 的 Reset 立即清筛选 → 仅清草稿、保持外部值/行数/面板，点击确认后清筛选（依据 antd `es/table/hooks/useFilter/FilterDropdown.js:onReset` 默认 confirm:false/closeDropdown:false）。`ui/p2e_test.go` 未改；跨组 a11y_table/i18n_wiring 和旧 B2 Reset 期望由主线程完成。
- 基础 Table 内置筛选 Reset 修正：只改草稿并清搜索、面板不关闭，不立即 applyFilter；FilterResetToDefault 时，空草稿也允许恢复默认，草稿等于重置目标时 Reset 禁用。新增回归 `TestTableFilterResetOnlyDraftV3`（恢复默认和清空两分支）、`TestTableFilterResetRestoresDefaultFromEmptyDraft`。默认值不变，其余 Table 外观和行为不变。

## 共享文件改动请求

1. `ui/AUTHORING.md` §14：新增四入口及所在 table_pro_editor/dropdown/toolbar.go；DragSortTable 继续列为既有方法等价，不加重复入口。
2. `api/go/ui.txt` 等：主线程重新生成；新独立类型/方法保持实验层；原 ProTable/EditableProTable 签名未变。Defaults 新键由生成器采集。
3. `docs/specs/ui-coverage.md` §4：三行按上述复核更正，不把 dragSortHandlerRender、编辑树记录、全部 request扩展含糊计入“完整”；新增四行并保留3项暂不做。
4. `CHANGELOG.md`：合并上节新增与变更。
5. `ui/a11y_table_test.go`、`ui/i18n_wiring_test.go`：原 fullscreen覆盖调用补 FullScreen(true)，主线程已处理。本组未触碰这些文件。
6. Locale/theme/defaults 共享文件：本组未修改，不需要新语言字段；用户提供的标题/提示不是内置文案，内置 MoreActions/Search/Placeholder/Refresh 等均取 Locale。

## 验证记录

- `go build ./ui/...`、`go vet ./ui/... ./uihost/...` 通过（最终重跑结果见组D报告）。
- `go test ./ui -run 'Table|CellEditor|RowEditor|TableDropdown|ListToolBar'` 通过；包括既有 Table/ProTable/EditableProTable 回归。
- 独立审阅两项已 RED→GREEN：pending 异步验证/保存时 Escape 应取消；Select Enter 打开/选择不能被表格提前提交。测试名 `TestCellEditorTablePendingEscape`、`TestCellEditorTableSelectEnterDoesNotSubmit`。
- gallery 每个新入口分别 `go run ./examples/ui-gallery -only '<入口>' -theme all -out /tmp/spec-align-b4/D`，24张图片出图；用 Read 检查 ListToolBar Ant 亮色与 CellEditorTable shadcn 暗色，无明显错位/低对比。
- 没跑全 ui 测试（遵守并行通用说明）；API goldens、全 gallery audit/对比度由主线程统一验证。
