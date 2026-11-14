# 第 2 批 · 组 E2 表格行为与 API（含 EP TableV2）：对照记录

核对日期：2026-11-15。参考源码：`/tmp/tango-ref`（antd@6.7.0、element-plus@2.14.8、`@rc-component/table@1.12.0` 解包在 `/tmp/tango-ref/rc-table/package`）。下文路径缩写：`antd/` = `antd/package/es/table/`，`ep/` = `ep/package/es/components/table/src/`，`epv2/` = `ep/package/es/components/table-v2/src/`，`rc/` = `rc-table/package/lib/`。

分工：表格拆成 E1（外观与令牌：`table_header.go`、`table_body.go`、`table_icons.go`、`ui/theme/comp_table.go`，记录 `2-table-look.md`）和 E2（本文件：行为与 API，`table.go`、`table_model.go`、`table_show.go`、`table_keys.go`、`table_edit.go`、`table_transpose.go`）。需要改 E1 文件的地方写在末尾「共享文件改动请求」。

去重：EP TableV2 = `Table` 的虚拟滚动（`Virtual`/`ScrollY`）；EP `el-table` 与 antd `Table` 是同一个 `TableW`。行为测试在 `ui/align_b2e2_test.go`，演示页 `examples/ui-gallery/page_b2e2.go`（`TableApiAlign`，Parent `Table`）。

### Table · 列（columns）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| key / title / dataIndex / render | antd/interface.d.ts `ColumnType`（2026-11-15） | table.go:`TableColumn.Key/Title/Text/Render` | 等价写法（`dataIndex` 由 `Text(row)` 取值） |
| align | rc `align: left\|center\|right` | `Align`（逻辑 start/end，RTL 镜像；`AlignPhysical*` 不翻转） | 已对齐 |
| ellipsis / ellipsis.showTitle；EP 列 show-overflow-tooltip | antd `ellipsis`；ep/table-column `showOverflowTooltip` | `Ellipsis`（截断时悬停提示） | 已对齐 |
| EP 表级 show-overflow-tooltip | ep/table/defaults.mjs:`showOverflowTooltip` | 新 `ShowOverflowTooltip()`、Defaults `showOverflowTooltip`（复制列，不改应用的切片） | 已补齐（TestTableHeaderOptions） |
| fixed: left/right/true | rc `fixed` | `Fixed`（`FixedLeft/FixedRight`，逻辑侧）、`ColumnState.Fixed` | 已对齐 |
| width / minWidth | rc `width`、`minWidth` | `Width`、`MinWidth`（默认 80）、`Grow` | 已对齐 |
| hidden | antd 5.13+ `hidden` | `Hidden`、`ColumnState.Hidden` | 已对齐 |
| children（分组表头） | antd `ColumnGroupType.children` | `Children` | 已对齐 |
| onCell / onHeaderCell（colSpan、rowSpan、事件） | rc `onCell`、`onHeaderCell` | 合并用 `Span`；单元格样式用 `Render` 或 `RowClass` | 等价写法；单元格级事件（EP cell-click/cell-dblclick、header-click）暂不做（需 table_body/header 点击钩子，见请求 4） |
| responsive（按断点隐藏列） | antd `responsive: Breakpoint[]` | `Hidden` + 应用读 `c.Breakpoint` | 等价写法 |
| shouldCellUpdate、className、rowScope | rc | — | 不需要（即时模式每帧重建；样式钩子） |

### Table · 排序（sorter）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| sorter: boolean / compare / {compare, multiple} | antd `ColumnSorter`；antd/hooks/useSorter.js | `Sortable`（服务端排序）、`Compare`（本地）、`SortMultiple` | 已对齐 |
| sortOrder 受控 / defaultSortOrder | antd `sortOrder`、`defaultSortOrder` | `Sort(&SortState)`、`MultiSort(&[]SortState)`、`DefaultSort` | 已对齐 |
| sortDirections（列与表）、EP sort-orders | antd interface.d.ts `sortDirections?: SortOrder[]`（列、表两级）；useSorter.js `nextSortDirection`：未排序 → dirs[0]，最后一个 → 取消，不在列表里 → dirs[0]；EP table-column `sortOrders` 默认 `['ascending','descending',null]` | 旧：固定 升→降→取消 → 新 `TableColumn.SortDirections`、`(*TableW).SortDirections(...)`（列优先）；EP 不取消的循环写 `{SortDesc, SortAsc, SortDesc}` | 已补齐（TestTblNextOrderIn、TestTableSortDirections） |
| showSorterTooltip（表、列） | antd `showSorterTooltip` 默认 true；locale `triggerAsc/triggerDesc/cancelSort` 按下一次点击的方向 | 旧：表头里按固定循环写死提示文字，Element 不显示 → 新 `ShowSorterTooltip(on)`、Defaults `showSorterTooltip`；提示文字由 `(*tblFrame).sorterTip` 按 SortDirections 计算 | 已补齐（TestTableSortDirections）；表头调用点 E1 已改（TestAlignB2E1SorterTooltip）；列级开关暂不做（少用） |
| sortIcon | antd `sortIcon` | — | 暂不做（E1 复核：需要 table.go 的 TableColumn 新字段承载图标函数，绘制点在 table_header.go；排序箭头尺寸已令牌化 `SorterArrow*`） |
| aria-sort | rc 表头 `aria-sort` | 表头 `a11y.Sort` | 已对齐（TestTableA11ySelection） |
| EP sort/clearSort 方法、default-sort、sort-change | ep/table `sort(prop, order)`、`clearSort()`；`defaultSort` | 应用改 `SortState`；`DefaultSort`；`SortChanged()` | 等价写法 |

### Table · 筛选（filter）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| filters / onFilter / filterMultiple | antd interface.d.ts；useFilter | `Filters`、`OnFilter`、`FilterSingle`（= filterMultiple false） | 已对齐 |
| filteredValue 受控 / filtered | antd `filteredValue`、`filtered` | `Filters(&map)` | 已对齐 |
| defaultFilteredValue | antd `defaultFilteredValue` | 旧：无 → 新 `TableColumn.DefaultFilter`：首帧写入表自己的状态或绑定 map 中没有该列的项 | 已补齐（TestTableDefaultFilter） |
| filterResetToDefaultFilteredValue | antd `filterResetToDefaultFilteredValue` | 新 `TableColumn.FilterResetToDefault` + `(*tblFrame).filterResetValues` | 已补齐（TestTableDefaultFilter）；重置按钮调用点在 table_header.go，要 E1 改（请求 2） |
| filterSearch（布尔） | antd `filterSearch` | `FilterSearch` | 已对齐 |
| filterSearch（函数） | antd `(input, record) => boolean` | 按标签不分大小写包含 | 暂不做（少用；需要时加 `FilterSearchFunc`） |
| filterMode: tree | antd `filterMode: 'menu'\|'tree'`、`ColumnFilterItem.children` | — | 暂不做（E1 复核：需 `ColumnFilterItem.children` 等 table.go 新字段与树形面板，工作量大；可用 `FilterPanel` + `Tree` 自绘） |
| filterOnClose | antd 默认 true（点外面关闭时应用草稿）；EP 关闭不应用 | Ant 主题点外面应用，Element 不应用（table_header.go `closeFilter(col, !f.el)`） | 已对齐（默认）；关掉的开关暂不做 |
| filterDropdown / filterDropdownProps / filterIcon | antd | `FilterPanel(*TableFilterPanel)`（Confirm/Reset/Close） | 等价写法；filterIcon 暂不做（E1 复核：需 table.go 新字段；图标尺寸/颜色已令牌化 `Filter*`，Element 箭头由 `FilterArrow`） |
| EP clearFilter / filter-change / filter-method / filter-placement | ep/table | 应用改 map；`FilterChanged()`；`OnFilter` | 等价写法；filter-placement 暂不做 |

### Table · 行选择（rowSelection / EP selection）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| type: checkbox / radio | antd `RowSelectionType` | `Selection(&[]int)` / `SelectedKeys(&[]string)`、`SingleSelect()` | 已对齐 |
| selectedRowKeys / defaultSelectedRowKeys / onChange | antd | 绑定切片即受控；初值由应用给；`SelectionChanged()` | 等价写法 |
| getCheckboxProps（disabled）；EP selectable | antd；ep/table-column `selectable` | `SelectDisabled(func)` | 已对齐 |
| hideSelectAll | antd | `HideSelectAll()` | 已对齐 |
| selections | antd `selections`；`SELECTION_ALL/INVERT/NONE` | `Selections(...)`、`SelectionAll/Page/Invert/None` | 已对齐 |
| checkStrictly 默认 | antd/hooks/useSelection.js:77 `checkStrictly = true`；EP `tree-props.checkStrictly` 默认 false | 旧：树形多选默认父子联动 → 新：默认不联动（antd），`CheckLinked()` 打开联动，Defaults `checkStrictly`（Element 风格应用设 false）；`CheckStrictly()` 保留 | 已补齐（TestTableCheckStrictlyDefault；TestTableTreeCheckLinked 改为显式 `CheckLinked`）；**默认值变更，请用户确认** |
| preserveSelectedRowKeys；EP reserve-selection | antd useSelection.js:190-216 `setSelectedKeys`：不保留时只留 dataSource 里有的 key；EP `reserveSelection` 默认 false | 旧：key 永远保留（跨服务端分页） → 新：按 antd，选择变化时才丢掉数据里没有的 key/下标（不变化时不动，越界值不 panic）；`PreserveSelectedRowKeys()`、Defaults `preserveSelectedRowKeys`。**Element 不同**：EP 在数据变化时就清理/清空选择（ep/table/src/store/index.mjs:35-37），Tango 统一按 antd | 已补齐（TestTablePreserveSelectedRowKeys）；ProTable 同样丢弃其它页的 key，已核对与 pro-components 一致（见请求 3） |
| Shift 多选（onSelectMultiple） | useSelection.js:465-476 + _util/hooks/useMultipleSelect.js：checkStrictly 且已有选中时，Shift 点击把上次勾选行到当前行整段选中；整段都已选中则整段取消 | 旧：无 → 新：指针 Shift+点击勾选框按段选/取消（只在本页、跳过禁用行、键盘 Space 不触发） | 已补齐（TestTableShiftRangeSelect） |
| columnWidth | antd `columnWidth` 默认 token `selectionColumnWidth`（controlHeight 32，InternalTable.js:157-162） | 旧：Ant `max(48, 16+2×ph)`、Element 48、带下拉 +18 → 新：令牌 `TableToken.SelectionColumnWidth`（E1：Ant 32、Element 48），带下拉 +16（fontSizeIcon + padding/4），Ant bordered +16（2×paddingXS，selection.js:22-32；EP 无此规则，Element 保持 48）；`SelectionColumnWidth(px)` 优先；展开列宽读 `TableToken.ExpandColumnWidth` | 已补齐（TestTableSelectionColumnOptions、TestTableThemeMetrics）；单元格内边距 paddingXS 8 在 E1 文件（请求 10） |
| fixed | antd `rowSelection.fixed`；未设时跟随相邻列（useSelection.js:541-552） | 旧：首列固定时跟随 → 新 `SelectionFixed()` 强制固定 | 已补齐（TestTableSelectionColumnOptions） |
| 选择列与展开列顺序 | useSelection.js:525-533「Always after expand icon」 | 旧：拖拽、选择、展开 → 新：拖拽、展开、选择 | 已补齐（TestTableExpandRowByClick） |
| columnTitle / renderCell / align / onCell / getTitleCheckboxProps | antd | — | 暂不做（单元格构建在 table_body/header；renderCell 可用 `Render` 列 + 自己的 Checkbox） |
| a11y 名称 | antd locale `selectRow/selectedRow/selectionMenu`、全选 `selectAll` | `Table.SelectRowFmt`（带行号）、`SelectAll`、`Table.Selections` | 已对齐（TestTableA11ySelection） |
| EP toggleRowSelection / clearSelection / toggleAllSelection / getSelectionRows；select / select-all 事件 | ep/table 方法 | 应用改绑定的切片；`SelectionChanged()` | 等价写法 |
| EP select-on-indeterminate 默认 true | ep/table/defaults.mjs | 半选时点表头 = 全选 | 已对齐 |

### Table · 展开与树形（expandable / EP tree-props）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| expandedRowRender / rowExpandable / expandedRowKeys / onExpand | rc `ExpandableConfig` | `Expandable`、`RowExpandable`、`ExpandedKeys`、`ExpandChanged()` | 已对齐 |
| expandRowByClick | rc `expandRowByClick` | 旧：无 → 新 `ExpandRowByClick()`、Defaults `expandRowByClick`；行内的勾选框/展开图标点击不触发 | 已补齐（TestTableExpandRowByClick） |
| showExpandColumn | rc `showExpandColumn` 默认 true | 新 `HideExpandColumn()`、Defaults `showExpandColumn` | 已补齐（TestTableExpandRowByClick） |
| defaultExpandAllRows / defaultExpandedRowKeys；EP default-expand-all | rc `defaultExpandAllRows`；ep `defaultExpandAll` | 新 `DefaultExpandAllRows()`、Defaults `defaultExpandAllRows`/`defaultExpandAll`：只作用于表自己持有的展开状态（明细与树），未加载的懒加载节点不展开 | 已补齐（TestTableDefaultExpandAllRows）；defaultExpandedRowKeys 等价写法（绑定切片初值） |
| indentSize 15；EP indent 16 | rc `indentSize` 默认 15；ep defaults.mjs `indent` 16 | `Indent(px)`；新 Defaults `indentSize`，默认取令牌 `TableToken.IndentSize`（Ant/shadcn 15，Element 16） | 已补齐（TestTableThemeMetrics） |
| childrenColumnName / EP tree-props children/hasChildren | rc | `Tree(roots, children)`、`TreeLazy(has, load)` | 不需要（Go 用函数给子行） |
| EP lazy / load | ep `lazy`、`load` | `TreeLazy` | 已对齐 |
| expandIcon / expandIconColumnIndex / columnTitle / columnWidth / fixed（展开列） | rc | 展开列跟随首列固定 | 暂不做（E1 复核：展开列宽已有令牌 `ExpandColumnWidth`，接入在 table_show.go 列计算（E1 共享请求 3）；图标/位置需 table.go 新 API） |
| expandedRowClassName / expandedRowOffset | rc | — | 不需要 / 暂不做 |
| EP toggleRowExpansion、expand-change、expand-row-keys | ep | 改绑定切片；`ExpandChanged()`；`ExpandedKeys` | 等价写法 |

### Table · 表格级属性、分页、事件与方法

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| size: large / middle / small；EP size | antd `size`；ep `size` | `Size`、Defaults `size` | 已对齐 |
| bordered；EP border、stripe | antd/ep | `Bordered`、`Striped`、Defaults `bordered`/`striped`/`stripe` | 已对齐 |
| showHeader；EP show-header | rc `showHeader` 默认 true；ep 默认 true | 旧：无 → 新 `HideHeader()`、Defaults `showHeader` | 已补齐（TestTableHeaderOptions） |
| title / footer | rc `title`、`footer` | 旧：无 → 新 `Title(fn)`、`Footer(fn)`：单元格内边距；页脚底色 `TableToken.FooterAreaBg`，文字色 `FooterAreaColor`（作用域主题的 Text）；bordered 时标题无下边框、页脚无上边框、页脚带底圆角（bordered.js、radius.js `&-title`/`&-footer`），有标题/页脚时表格对应侧不再圆角 | 已补齐（TestTableHeaderOptions） |
| sticky {offsetHeader, offsetSummary, getContainer} | antd `sticky` | `Sticky`、`StickySummary`、`StickyContainer` | 已对齐 |
| scroll.x / scroll.y | rc `scroll` | `ScrollX`、`ScrollY` | 已对齐 |
| scroll.scrollToFirstRowOnChange | antd InternalTable.js:229-233，默认 true | 旧：排序/筛选/翻页后滚动位置不变 → 新：回到首行；`ScrollToFirstRowOnChange(on)`、Defaults | 已补齐（TestTableScrollToFirstRowOnChange） |
| virtual | antd `virtual` | `Virtual`（ScrollY 时总是虚拟） | 已对齐 |
| loading | antd `loading: boolean\|SpinProps` | `Loading(bool)` | 已对齐；SpinProps 暂不做 |
| locale.emptyText；EP empty-text | antd `locale.emptyText`；ep `emptyText` | `EmptyText` | 已对齐（TestTableEmptyAndUnknownKeys） |
| pagination: false | antd | 不调 `Pagination` | 已对齐 |
| pagination.placement（v6，`position` 弃用） | antd interface.d.ts `TablePaginationPlacement`；InternalTable.js:385-412 | 旧：只在底部右侧 → 新 `PaginationPlacement(...)`、常量 `PageTopStart … PageBottomEnd`、`PageNone` | 已补齐（TestTablePaginationDefaults） |
| pagination 其余配置（showTotal、showQuickJumper、simple…） | antd `TablePaginationConfig extends PaginationProps` | 旧：总是显示总数 → 新 `PaginationConfig(func(*PaginationW))`；默认不显示总数 | 已补齐（TestTablePaginationDefaults） |
| 分页尺寸 | antd util.js:39-47 `getPaginationSize`：small/medium 表格用 small | 旧：总是默认尺寸 → 新：Small、Middle 表格用小号 | 已补齐（gallery TableApiAlign） |
| 无数据不显示分页 | InternalTable.js:385 `mergedPagination?.total` | 旧：总数 0 也显示 → 新：不显示 | 已补齐（TestTablePaginationDefaults） |
| summary；EP show-summary / sum-text / summary-method | antd `summary`；ep defaults.mjs `showSummary`、`sumText`（locale `el.table.sumText`）、`summaryMethod`；ep 默认合计：首列 sumText，数字列求和、精度取最大小数位，非数字跳过 | `Summary(fn)`、`SummaryFixed`；新 `ShowSummary()`、Defaults `showSummary`、Locale `Table.SumText`；与 `Summary` 同用时只出自定义行（EP table-footer/index.mjs:49-53 二选一）；数字按 EP `Number(toFixed)` 去尾零（1.50 + 2 → 3.5）。**差异**：EP 把 sumText 放在第 0 列（有选择列时就在选择列），Tango 放在第一个数据列（特殊列不画汇总文字） | 已补齐（TestTableShowSummary） |
| rowKey | antd `rowKey` | `RowKey(func)` | 已对齐 |
| onRow（onClick/onDoubleClick/onContextMenu…）；EP row-click/row-dblclick/row-contextmenu | rc `onRow` | `RowClicked()`、`RowDoubleClicked()` | 等价写法；右键行暂不做（需 table_body 行节点上读 `ContextMenuFor`，请求 4） |
| rowClassName；EP row-class-name/row-style | rc | `RowClass(func) RowStyle` | 等价写法 |
| rowHoverable | antd 5.16+ | `NoHover()` | 已对齐 |
| EP highlight-current-row / current-row-key / setCurrentRow / current-change | ep defaults.mjs `highlightCurrentRow`、`currentRowKey` | `HighlightCurrentRow()`；新 `CurrentRowKey(&key)`（受控，"" 清除）、`CurrentChanged()`（只要当前行因点击/Enter 改变就为真，不论是否绑定 key）、Defaults `highlightCurrentRow` | 已补齐（TestTableCurrentRowKey、TestTableCurrentChangedUnbound） |
| onChange(pagination, filters, sorter, {action}) | antd | `PageChanged`/`FilterChanged`/`SortChanged` | 等价写法 |
| EP span-method | ep `spanMethod` | `Span` | 已对齐 |
| EP fit / table-layout / flexible / scrollbar-always-on / tooltip-effect / tooltip-options / doLayout / scrollTo / setScrollTop | ep | 列宽算法固定（等同 fixed 布局）；滚动由容器管理 | 不需要 / 暂不做（scrollTo 需公开滚动 API） |
| EP header-dragend；antd 无 | ep | `Resizable()`、`Resized()` | 已对齐 |
| getPopupContainer / dropdownPrefixCls / classNames / styles / components | antd | — | 不需要（JS 渲染与样式钩子） |

### Table · 键盘、无障碍、禁用、越界受控值

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 角色 grid / row / columnheader / cell，aria-sort，选中/多选状态 | rc + antd（AUTHORING §17） | `RoleGrid`、`Multiselectable`、`Busy`、表头 `Sort`、行列号 | 已对齐（TestTableA11ySelection） |
| 选择框名称来自 Locale | antd locale `selectRow` 等 | `Table.SelectRowFmt`、`SelectAll` | 已对齐（TestTableA11ySelection） |
| 键盘 | antd 无网格键盘（依赖原生 Tab） | 方向键/Home/End/PageUp/PageDown 移动活动单元格，Space 选择，Enter 编辑或展开，Ctrl+C 复制，Alt+↑↓ 拖拽排序 | 已对齐（超出 antd） |
| 禁用（Disabled / ConfigProvider.ComponentDisabled） | antd 表格无 disabled，勾选框读 DisabledContext | 整个子树禁用：排序、勾选、按行点击展开都不响应，查询方法为 false | 已对齐（TestTableDisabledInteractions） |
| 空数据、nil 绑定、未知 key | Review Focus 2 | rows 0 / 列为 nil / 选择、展开、树、当前行的 key 不存在时不 panic，显示 Empty，越界页码夹回 1 | 已对齐（TestTableEmptyAndUnknownKeys） |

### EP TableV2（= `Table.Virtual`）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 虚拟滚动、fixed（列固定） | epv2/table.mjs `fixed: Boolean`、columns `fixed` | `Virtual` + `ScrollY`；列虚拟；`TableColumn.Fixed` | 已对齐 |
| row-height 50 / header-height 50 | epv2/table.mjs:28-31、header.mjs:16-24 | `RowHeight(px)`；新 `HeaderHeight(px)`；`Virtual()` 时默认读令牌 `TableToken.VirtualRowHeight/VirtualHeaderHeight`（E1：Element 50/50，Ant/shadcn 0 = 按尺寸），`RowHeight`/`HeaderHeight` 优先 | 已补齐（TestTableHeaderOptions、TestTableThemeMetrics） |
| estimated-row-height（动态行高） | epv2 `estimatedRowHeight` | `Wrap` 列测量 + 估算缓存 | 等价写法 |
| sort-by / sort-state / column-sort | epv2 `sortBy` | `Sort`/`MultiSort` | 已对齐 |
| expand-column-key / expanded-row-keys / default-expanded-row-keys | epv2 `expandColumnKey` | 树形展开图标在首个数据列；`TreeOpenKeys`、`DefaultExpandAllRows` | expand-column-key 暂不做（E1 复核：需 table.go 新方法指定列；table_body 的 treePrefix 目前固定在首个数据列） |
| cell-props / header-props / row-props / row-event-handlers | epv2 | `RowClass`、`RowClicked` | 等价写法 |
| cache / h-scrollbar-size / v-scrollbar-size / scrollbar-always-on / footer-height / data-getter | epv2 | 列虚拟化自带缓冲 | 不需要 |

## 基础能力缺口

本组没有新的基础层缺口；暂不做的项都卡在 E1 的文件（表头/表体绘制与点击钩子）或属于少用功能。

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| — | — | — | — |

## CHANGELOG 草稿

### 新增

- Table（行为与 API，对照 antd 6.7 / Element Plus 2.14.8）：`TableColumn.SortDirections`、`DefaultFilter`、`FilterResetToDefault`；`(*TableW).SortDirections`、`ShowSorterTooltip`、`PreserveSelectedRowKeys`、`CheckLinked`、`SelectionColumnWidth`、`SelectionFixed`、`ExpandRowByClick`、`HideExpandColumn`、`DefaultExpandAllRows`、`HideHeader`、`HeaderHeight`（EP TableV2 header-height）、`Title`、`Footer`、`PaginationPlacement`（常量 `PageTopStart`…`PageBottomEnd`、`PageNone`）、`PaginationConfig`、`CurrentRowKey`/`CurrentChanged`（EP current-row-key）、`ShowSummary`（EP show-summary，Locale `Table.SumText`）、`ShowOverflowTooltip`（EP 表级）、`ScrollToFirstRowOnChange`；Shift+点击勾选框按段选择。
- Defaults `Table`：`checkStrictly`、`preserveSelectedRowKeys`、`expandRowByClick`、`defaultExpandAllRows`（兼收 EP `defaultExpandAll`）、`showExpandColumn`、`showHeader`、`showSorterTooltip`、`indentSize`、`scrollToFirstRowOnChange`、`highlightCurrentRow`、`showOverflowTooltip`、`showSummary`。

### 变更

- Table：树形数据多选默认不再父子联动（旧：联动），依据 antd useSelection.js:77 `checkStrictly = true`；要联动用 `CheckLinked()` 或 Defaults `checkStrictly: false`（Element 风格）。
- Table：选择变化时丢掉数据里已不存在的 key / 下标（旧：一直保留，跨服务端分页），依据 antd useSelection `setSelectedKeys` 与 EP `reserve-selection` 默认 false；保留用 `PreserveSelectedRowKeys()`。
- Table：同时有展开列和选择列时，展开列在前（旧：选择列在前），依据 useSelection.js:525「Always after expand icon」。
- Table 分页：默认不显示总数（旧：总显示「共 N 条」），Small/Middle 表格用小号分页（旧：默认尺寸），没有数据时不显示分页（旧：显示），依据 antd InternalTable.js:385-393、util.js `getPaginationSize`；总数用 `PaginationConfig(func(p){p.ShowTotal()})`。
- Table：排序、筛选、翻页后有界表体滚回首行（旧：保持滚动位置），依据 antd `scroll.scrollToFirstRowOnChange` 默认 true。
- ProTable / EditableProTable：随 Table 变化——服务端分页时勾选变化会丢掉其它页的 key（pro-components 不设 preserveSelectedRowKeys，已核对 /tmp/tango-ref/pro/package/es/table），Middle/Small 密度用小号分页，翻页/排序/筛选后表体滚回首行；总数仍显示（两者已传 `PaginationConfig(ShowTotal)`，pro-components mergePagination 默认 showTotal）。
- Table（Ant 主题）：选择列宽 48 → 32（带选择下拉 +16，bordered +16），依据 antd `selectionColumnWidth` = controlHeight 与 selection.js；展开列宽、行高、Middle 字号改读表格令牌（E1 定值）。
- Table（Ant 主题，bordered）：外框底角跟随 `TableToken.BottomRadius`（与表体一致），有 Title/Footer 时表格对应侧不再圆角，依据 antd radius.js。
- Table（Element 主题）：树形缩进 15 → 16，依据 el-table `indent` 默认 16；`Virtual()` 默认行高与表头高 50，依据 el-table-v2 `row-height`/`header-height`。
- Table：`ShowSummary` 合计数字去掉尾零（EP `Number(toFixed)`）；同时设置 `Summary` 时只显示自定义汇总行。

## 共享文件改动请求

1. **E1 · `ui/table_header.go`（排序提示）**：约 103-112 行 `if ia.Hovered && !f.el { … tip := loc.SortAsc … }` 改为 `if ia.Hovered { if tip, ok := f.sorterTip(col); ok { f.tip(n.id, tip, false) } }`。`sorterTip`（table_show.go）按 `SortDirections` 算下一次点击的动作，并读 `ShowSorterTooltip`/Defaults `showSorterTooltip`；它暂时仍保留 Element 不显示的分支（`f.el`），若 E1 给 `TableToken` 加布尔令牌 `SorterTooltip`（Ant/shadcn true、Element false），我把 `f.el` 换成令牌。
2. **E1 · `ui/table_header.go`（筛选重置）**：`filterDropdown` 末尾 `case reset:` 里 `f.applyFilter(col.Key, nil); st.fDraft = nil` 改为 `v := f.filterResetValues(col); f.applyFilter(col.Key, v); st.fDraft = v`（FilterResetToDefault）。重置按钮的禁用条件 `len(st.fDraft) == 0` 在 FilterResetToDefault 时可放开（可选）。
3. **已由主线程完成**：`ui/table_pro.go`、`ui/p2e_editable.go` 传 `PaginationConfig(ShowTotal)`（pro-components mergePagination 默认 showTotal）。跨页选择：pro-components 不设 preserveSelectedRowKeys（主线程已核对 /tmp/tango-ref/pro/package/es/table），**已核对，与 Pro 一致**，ProTable 不加 `PreserveSelectedRowKeys`。
4. **E1 · `ui/table_body.go`（可选，下一批）**：行节点上读 `c.ContextMenuFor(n.id)` 记录右键行（EP row-contextmenu）、单元格点击（cell-click）需要的钩子；本组只在记录里列为暂不做。
5. **已完成**：E1 加了 `TableToken.IndentSize` 等令牌；本组按 E1 请求 2、3、4、6 改了 table_show.go / table.go（外框圆角、行高/字号/列宽/虚拟行高、Title/Footer 区、缩进默认）。`ui/shad2b_table.go` 的 `f.lh = 20` 不归本组，未删（令牌 shadcn 也是 20，结果相同），由主线程决定是否删。
6. **Locale（已改，小块 Edit 插入）**：`ui/locale.go` `TableLocale` 末尾新增 `SumText`；`ui/locale_packs.go` 四个包各加 `SumText`（合计 / Sum / 合計 / 合計，依据 EP locale `el.table.sumText`）。
7. **api/**：新增方法与字段见「新增」；`TableW`、`TableColumn` 在稳定层，只有新增；新 Defaults 键见上。`CheckStrictly()` 签名不变，语义变为显式指定默认值（文档已改）。
8. **AUTHORING §14** Table 行：「树形勾选联动/`CheckStrictly`」改为「树形勾选（默认独立，`CheckLinked` 联动）」。
10. **E1 · `ui/table_header.go` / `table_body.go`（选择列内边距，阻塞 TestTableSelectionPageAndHalf）**：选择列改成 32 宽后，表头全选框完全点不中——调试显示该点没有任何勾选框命中区（a11y 边框在 x 16..32，但命中区为空）：表头选择单元格仍用 `Padding(0, f.ph)`（16），内容宽 0，包住 Checkbox 的 `Row` 宽 0，命中区被裁掉；行内勾选框是单元格的直接子节点所以还能点。antd selection.js:34-41 选择列单元格 `paddingInline: paddingXS`（8）、居中。请 E1 给 `tkSelect`（及展开列）单元格用 8 的横向内边距（表头和表体），表头那层 `Row` 不要被压成 0 宽。这不是点击坐标问题（改成 x+16 同样失败，用 a11y Press 则成功），所以测试坐标保持 x+24（当前勾选框在列内 16..32，x+24 正是其中心）；E1 修好前该用例失败。
9. 已改本组 gallery：`page_table_treecheck.go` 第一个示例加 `CheckLinked()` 并改标题；新页 `page_b2e2.go`（TableApiAlign）。现有表格 gallery 页（TableBasic 等）的分页不再显示总数，属上面的「变更」。
