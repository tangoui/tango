# 第 2 批 · 组 E1 表格外观与令牌 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、@rc-component/table（`rc-table/package`）。下文源码路径省略这些前缀，`antd` 指 `antd/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`。

本组只管 Table 的外观与令牌（`ui/table_header.go`、`ui/table_body.go`、`ui/table_icons.go`、`ui/theme/comp_table.go`）；行为与 API 见组 E2 的记录。本组测试：`ui/align_b2e1_test.go`（绘制 / 行为）、`ui/theme/align_b2e1_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b2e1.go`（`TableLookAlign`，Parent `Table`）。

## Table 令牌（antd prepareComponentToken）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| headerBg / headerColor | antd colorFillAlter 叠 colorBgContainer = #fafafa（暗 #1d1d1d）、colorTextHeading（antd/table/style/index.js:198-199，2026-11-15） | comp_table.go `HeaderBg`、`HeaderColor` | 已对齐（TestAlignB2E1AntTableTokens、TestAlignB2E1AntDarkTableTokens） |
| headerSortActiveBg / headerSortHoverBg | antd 均为 colorFillSecondary（= colorFillContent）叠 colorBgContainer = #f0f0f0（index.js:200-201；sorter.js 悬停用 hover、已排序用 active） | 只有 `HeaderSortActiveBg`（FillTertiary 叠在表头 #fafafa 上 = #f0f0f0），排序与悬停共用 | 已补齐（TestAlignB2E1SortHeaderBg）：新令牌 `HeaderSortHoverBg`；合成结果与源码一致（亮 #f0f0f0，暗 #2f2f2f vs #303030） |
| 排序/悬停时隐藏分隔线 | antd sorter.js `&::before { backgroundColor: transparent }` | 分隔线照画 | 已补齐（演示页）；**变更** |
| bodySortBg | antd colorFillAlter 叠底 = #fafafa（index.js:202） | `BodySortBg` = FillQuaternary 叠行底，等价 | 已对齐；Element 改为令牌零值（去掉 `!f.el` 分支） |
| rowHoverBg | antd colorFillAlterSolid：亮 #fafafa、暗 #1d1d1d（index.js:203） | 亮 #fafafa；暗 #262626（theme.go:471） | 已对齐（TestAlignB2E1AntDarkTableTokens）：主线程按共享请求 1 改为 #1d1d1d；**变更** |
| rowSelectedBg / HoverBg | antd controlItemBgActive / controlItemBgActiveHover = colorPrimaryBg / colorPrimaryBgHover（index.js:204-205，alias.js:81-82） | `RowSelectedBg`、`RowSelectedHoverBg` | 已对齐（TestAlignB2E1AntTableTokens） |
| rowExpandedBg | antd colorFillAlter（index.js:206；expand.js:99-104） | 面板里 `if !f.el` 写死 FillAlter | 已补齐（TestAlignB2E1ExpandedPanelBg）：令牌 `RowExpandedBg`（Element 零值 = fill-color-blank） |
| cellPadding* | antd padding 16/16、MD paddingSM 12 / paddingXS 8、SM 8/8（index.js:207-212） | `CellPadding*` | 已对齐 |
| cellFontSize / MD / SM | antd 均为 fontSize 14（index.js:217-219；size.js） | 无 MD | 已补齐：`CellFontSizeMD`（metrics 在 E2 的 table_show.go，见共享请求 3） |
| borderColor / headerSplitColor | antd colorBorderSecondary（index.js:213,220） | `BorderColor`、`HeaderSplitColor` | 已对齐 |
| headerBorderRadius | antd borderRadiusLG 8，只圆上角（index.js:214；radius.js） | `HeaderBorderRadius` 8；带边框时四角都圆 | 已补齐：新令牌 `BottomRadius`（Ant/Element 0，shadcn rounded-md）；表体已改读令牌；外框在 table_show.go（共享请求 2）；**变更** |
| footerBg / footerColor | antd colorFillAlterSolid / colorTextHeading，用于 `.ant-table-footer`（index.js:156-161）；summary 底色是 tableBg = colorBgContainer（summary.js） | `FooterBg` 实为汇总行底色（BgContainer，正确） | 已补齐：新令牌 `FooterAreaBg` / `FooterAreaColor` 给 E2 的 `Footer()` 区（共享请求 4）；`FooterBg` 仍是汇总行 |
| fixedHeaderSortActiveBg | antd colorFillSecondarySolid（index.js:221） | 固定列表头同样叠 `HeaderSortActiveBg` | 已对齐（合成值相同） |
| headerFilterHoverBg | antd colorFillContent（index.js:222；filter.js `&:hover`） | 写死 FillSecondary | 已补齐（TestAlignB2E1FilterTriggerHover）：`FilterHoverBg` |
| filterDropdownBg / MenuBg | antd colorBgContainer（index.js:223-224） | BgElevated（暗色 #1f1f1f） | 已补齐（TestAlignB2E1AntDarkTableTokens）：`FilterDropdownBg`；**变更**：Ant 暗色筛选下拉底 #1f1f1f → #141414 |
| 筛选下拉圆角 | antd `.ant-table-filter-dropdown` borderRadius 6（filter.js） | RadiusLG 8 | 已补齐：`FilterDropdownRadius`；**变更**：8 → 6 |
| expandIconBg / size / scale / marginTop | antd colorBgContainer；size = (16/2 − 1)×2 + 3 = 17，scale 16/17，border tableBorder，borderRadius 6，hover border currentcolor、色 colorLinkHover（index.js:225-234；expand.js:44-98） | 17px 盒不缩放、RadiusSM 4、悬停 Primary.Hover | 已补齐（TestAlignB2E1ExpandIconScaled）：`ExpandIconSize/Scale/Radius/Bg/Border/Color/HoverColor`；**变更**：Ant 展开图标绘制为 16px、圆角 5.6、悬停色 colorLinkHover；布局盒仍 17（transform 不影响布局）。marginTop 由行内垂直居中代替（等价写法） |
| selectionColumnWidth | antd controlHeight 32（带下拉 +12+4，bordered +16）（index.js:226；selection.js:20-31） | table_show.go 写死 max(48, 16+2ph) | 已补齐令牌 `SelectionColumnWidth`（Ant 32、Element 48）；列宽计算在 E2 文件（共享请求 3） |
| 选择格内边距 | antd selection.js:34-41 选择列 paddingInline paddingXS 8、复选框居中；EP `.el-table-column--selection .cell` 左右 14px（table-column.scss:6-12） | 选择格沿用单元格内边距 16，列宽 32 时表头全选塌成 0 宽 | 已补齐（TestAlignB2E1SelectAllCentred，三套 × 带边框/不带边框）：令牌 `SelectionPaddingInline`（Ant/shadcn 8、Element 14），实际取 min(令牌, (w−16)/2)；**变更** |
| tableExpandColumnWidth | antd controlInteractiveSize 16 + padding×2 = 48（index.js:306） | 48 | 已对齐：令牌 `ExpandColumnWidth` 48 |
| headerIconColor / HoverColor | antd colorIcon × opacityLoading = rgba(0,0,0,.29)；悬停 colorIconHover × .65 = .57（index.js:230-231；sorter.js `-column-sorters:hover`） | 亮 .29；暗用 TextQuaternary .25；悬停不变色 | 已补齐：`SorterHoverColor`（**变更**：悬停表头时未激活箭头变深）；暗色 idle .25 → .29 已由主线程按共享请求 1 合并（TestAlignB2E1AntDarkTableTokens） |
| stickyScrollBarBg / Radius / 尺寸 | antd 有 scroll.y 的表体用 `scrollbarColor: tableScrollThumbBg`（= stickyScrollBarBg colorTextPlaceholder .25，index.js:55,227,315），粗 tableScrollThumbSize 8；hover colorTextHeading 与 ×.65 透明度只用于 `sticky` 滚动条（sticky.js） | 滚动滑块 TextTertiary × .5 / .75，宽 6 | 已补齐：`StickyScrollBarBg` = colorTextPlaceholder、`Size` 8；原生滚动条无悬停色，按住/悬停用 ui/scroll.go 的 TextTertiary（`StickyScrollBarHoverBg`）；**变更** |

## Table 外观（表头、排序、筛选、展开、边框、尺寸）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 表头分隔线 | antd `th:not(:last-child):not(.selection):not(.expand):not([colspan])::before`，高 1.6em（index.js:95-106） | 非分组：除最后一列外都画；分组：所有格画整高竖线 | 已补齐（TestAlignB2E1HeaderSplitLeafOnly）：只在非 colspan、非本行最后一格画；令牌零值不画（Element / shadcn），去掉 Flavor 分支；**变更**：Ant 无边框分组表头不再画整高竖线。高度 22 ≈ 1.6em（22.4），保留 |
| 分组表头下边线 | antd `tr:not(:last-child) > th[colspan] { borderBottom: 0 }`（index.js:108-110）；EP 只有 `th.is-leaf` 有下边线（table.scss:225-228）；bordered 时都有（bordered.js:56-58） | 全部画 | 已补齐（TestAlignB2E1HeaderSplitLeafOnly、TestAlignB2E1BorderedGroupBottom）；**变更** |
| 排序箭头 | antd caret 12px 图标、上下叠 −0.3em（sorter.js:69-85），约 8×4.9、间距 3.4；EP sort-caret border 5px、ascending top −5、descending bottom −3（table.scss:528-544）→ 10×5、间距 2 | 写死 Ant 8×4.5 间距 3，Element 10×5 间距 3 | 已补齐：令牌 `SorterArrowWidth/Height/Gap`；Ant 保持（差 <0.5px）；**变更**：Element 间距 3 → 2 |
| 筛选按钮 | antd 图标 fontSizeSM 12、idle headerIconColor、hover colorIcon + headerFilterHoverBg、borderRadius 6、active colorPrimary（filter.js:38-62） | 圆角 RadiusSM 4 | 已补齐（TestAlignB2E1FilterTriggerHover）：`FilterIconSize/IdleColor/HoverColor/HoverBg/Radius/FilterArrow`；**变更**：Ant 悬停底圆角 4 → 6 |
| 筛选下拉 | antd minWidth 120、boxShadowSecondary、菜单项 5/12 高 32、悬停 controlItemBgHover（filter.js、dropdown） | 同 | 已对齐（底色/圆角见令牌表） |
| 展开面板 | antd 底 rowExpandedBg、padding 同单元格（expand.js:99-119） | 同；Element 用 Flavor 分支 | 已补齐：`RowExpandedBg`、`ExpandedPaddingBlock/Inline`（Element 20/50） |
| 带边框 | antd 容器 borderInlineStart + borderTop，单元格 borderInlineEnd/Bottom，只有上角圆（bordered.js、radius.js） | 四角圆 | 已补齐（表体部分）：`BottomRadius`；外框在 table_show.go（共享请求 2） |
| size small / middle / large | antd size.js：padding 与 font 取 MD / SM 令牌 | metrics 在 table_show.go | 已对齐（令牌已全） |
| 空态 | antd 占位行 colorTextDisabled、悬停不变色（empty.js），内容为 Empty simple | `Empty(c).Simple()` | 已对齐 |
| 汇总行 | antd summary 底 tableBg、每格下边线（summary.js） | `FooterBg` = BgContainer | 已对齐；字重令牌 `FooterFontWeight`（Ant 400） |
| 排序提示（E2 请求） | antd `showSorterTooltip`，文字按下一次点击方向（locale triggerAsc/triggerDesc/cancelSort） | 表头按固定循环写死 | 已补齐（TestAlignB2E1SorterTooltip、TestTableSorterTooltipToken）：改调 E2 的 `f.sorterTip(col)`；`ShowSorterTooltip(false)` 生效；主题默认读令牌 `SorterTooltip`（Element false） |
| 筛选重置（E2 请求） | antd `filterResetToDefaultFilteredValue` | 重置一律清空 | 已补齐（TestAlignB2E1FilterResetToDefault）：改调 `f.filterResetValues(col)` |
| indentSize 令牌（E2 请求） | antd `expandable.indentSize` 15；EP `indent` 16（ep/es/components/table/src/table/defaults.mjs:147-150） | table.go 写死 15 | 已补齐令牌 `IndentSize`（Ant/shadcn 15、Element 16）；默认值接入在 table.go（共享请求 6） |
| 行右键 / 单元格点击（E2 请求，可选） | EP row-contextmenu、cell-click | — | 暂不做（需要 table.go 新查询方法，API 形状由 E2 定；本组只做外观） |
| sortIcon / filterIcon / filterMode tree / 展开列 fixed / expand-column-key（E2 暂不做行） | antd / EP | — | 暂不做（都需 table.go 新字段或方法；已在 2-table-api.md 对应行注明 E1 复核结论） |
| 明暗主题 | 令牌 | 新绘制只读令牌，无 th.Dark 判断 | 已对齐（三套 × 明暗出图 /tmp/spec-align-b2/e1） |

## Element Plus el-table / el-table-v2 外观

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 表头文字 / 字重 | `--el-table-header-text-color` text-color-secondary #909399；th font-weight 600（var.scss:1006；table.scss:142-147） | `HeaderColor` TextTertiary、600 | 已对齐（TestAlignB2E1ElementTableTokens） |
| 单元格文字 / 行悬停 / 边框 / 当前行 | text-color-regular、fill-color-light #f5f7fa、border-color-lighter #ebeef5、color-primary-light-9（var.scss:1003-1009） | 同 | 已对齐 |
| 斑马纹 | `--striped` 行 fill-color-lighter：亮 #fafafa、暗 #1d1d1d（table.scss:560-565；dark/css-vars） | FillAlter（暗 #191919 = extra-light） | 已补齐（TestAlignB2E1ElementTableTokens）：`RowStripedBg` 按源码；**变更**：Element 暗色斑马纹 #191919 → #1d1d1d |
| 分组表头下边线 | 只有 `th.is-leaf` 有 border-bottom，带边框时所有 th 都有（table.scss:225-228,313-315） | 全部画 | 已补齐（TestAlignB2E1HeaderSplitLeafOnly 同一规则）；**变更**：Element 无边框分组表头的分组格不画下边线 |
| 分组表头底色 | `thead.is-group th` fill-color-light（table.scss:149-152） | `f.el` 分支读 RowHoverBg | 已补齐：令牌 `HeaderGroupBg`，去掉分支 |
| 尺寸 | $table-padding 12/8/4、$table-cell-padding 16/12/8、font small extra-small 12；.cell line-height 23（var.scss:1022-1049；table.scss:194） | 同；行高写死在 table_show.go | 已补齐：`CellLineHeight` 23（共享请求 3） |
| 排序 caret | 见上表 | | 已补齐 |
| 筛选触发 | `__column-filter-trigger i` color-info、14px，无悬停变色（table.scss:620-630） | 箭头 12px、idle placeholder 色、悬停主色 | 已补齐：Element 令牌 14 / #909399 / 不变色；**变更** |
| 筛选面板 | `.el-table-filter` border-color-lighter、radius 2px（table-column.scss:15-20） | Radius 4 | 已补齐：`FilterDropdownRadius` 2、`FilterDropdownBorder`；**变更** |
| 展开图标 | `__expand-icon` text-color-regular 12px，无悬停色，展开旋转 90°（table.scss:82-101） | 悬停变主色 | 已补齐：`ExpandIconArrow`、`ExpandIconColor/HoverColor`；**变更**：悬停不再变主色；复审后展开列的图标盒也读 `ExpandIconSize`（与树形开关一致），Element 17 → 16 |
| 展开单元格 | `__expanded-cell` 底 fill-color-blank、padding 20px 50px（table.scss:104-114） | 同（Flavor 分支） | 已补齐：令牌 |
| 滚动条 | $scrollbar text-color-secondary opacity .3 / hover .5，6px（var.scss:1375-1378；scrollbar.scss） | TextTertiary × .5 / .75 | 已补齐：`StickyScrollBar*`；**变更** |
| el-table-v2 高度 | row-height 50、header-height 50（es/components/table-v2 默认） | — | 已补齐令牌 `VirtualRowHeight/VirtualHeaderHeight`（Element 50，其余 0）；接到 `Virtual()` 在 E2 文件（共享请求 3） |
| highlight-current-row / stripe / border | EP | `HighlightCurrentRow`、`Striped`、`Bordered` | 已对齐 |

## shadcn table.tsx 外观

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| TableHead | `h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground`（shadcn/table.tsx:67-77） | 表头 muted-foreground（v3 写法） | 已补齐（TestAlignB2E1ShadcnTableTokens）：`HeaderColor` = foreground；**变更**（shadcn） |
| TableCell | `p-2 align-middle`；text-sm（table.tsx:80-90,14） | 8/8、行高 20 | 已对齐：`CellLineHeight` 20 |
| TableRow | `border-b hover:bg-muted/50 data-[state=selected]:bg-muted`（table.tsx:54-64） | 选中 Primary.Bg（= accent） | 已补齐：`RowSelectedBg` 直接取 `--muted`（悬停不变） |
| has-aria-expanded:bg-muted/50 | 展开中的行着 muted/50；table.tsx 没有展开面板 | 面板底 FillAlter（= muted/50） | 已对齐：`RowExpandedBg` 保持 FillAlter（外观不变）；展开中的父行着色暂不做（要改 rowBg 的状态来源，收益小） |
| TableFooter | `border-t bg-muted/50 font-medium`（table.tsx:41-51） | BgContainer、常规字重 | 已补齐：`FooterBg` = muted/50、`FooterFontWeight` 500；**变更**（shadcn） |
| TableCaption | `mt-4 text-sm text-muted-foreground`（table.tsx:92-102） | — | 已补齐令牌 `FooterAreaColor` = muted-foreground；绘制在 E2 的 Footer/Title（共享请求 4） |
| 外框 | data-table 示例 `overflow-hidden rounded-md border`（examples/data-table-demo.tsx:254） | RadiusLG（--radius） | 已补齐：`HeaderBorderRadius`/`BottomRadius` = rounded-md（radius − 2）；**变更** |
| 筛选按钮 / 下拉圆角 | data-table 表头控件是 `Button variant="ghost"`（button.tsx rounded-md）；下拉为 popover `rounded-md`（popover.tsx） | 悬停底 RadiusSM（6）；下拉 RadiusLG（10） | 已补齐：`FilterRadius`、`FilterDropdownRadius` = rounded-md（8）；**变更** |
| 分组表头竖线 | table.tsx 只有行 `border-b`，无竖向分隔 | 无边框分组表头画整高竖线（沿用 Ant 旧写法） | 已补齐：不画（令牌 `HeaderSplitColor` 零值、非 bordered 无竖线）；**变更** |
| 排序按钮 | data-table 示例 `Button variant="ghost"` + ArrowUpDown（data-table-demo.tsx:136） | shad2b_table.go 幽灵按钮 | 已对齐 |
| 滚动滑块 | scroll-area `w-2.5 border-l p-px` + thumb `rounded-full bg-border`（scroll-area.tsx:39-52）→ 7px | TextTertiary | 已补齐：`StickyScrollBar*` = border、7px |
| 展开图标 | 无（shadcn 无展开表格组件） | Ant 方框，悬停 Primary.Hover | 不需要（保留 17px 不缩放、RadiusSM、悬停 Primary.Hover，外观不变） |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 无（本组外观项都能用现有 paint 原语完成） | — | — | — |

仍留在本组文件里的 Flavor 分支（结构差异，不是数值，暂不做令牌化）：`table_header.go` 的筛选按钮位置（Element 紧跟标题）、排序器占位宽度、Element 单选筛选列表与下拉 placement；`table_body.go` 的 Element 纯文字空态。`f.el` / `f.shad` 本身在 E2 的 `table_show.go` 里设置。

## CHANGELOG 草稿

### 新增

- Table 外观令牌（`theme.TableToken`）：`HeaderSortHoverBg`、`HeaderGroupBg`、`RowExpandedBg`、`BottomRadius`、`FooterFontWeight`、`FooterAreaBg`、`FooterAreaColor`、`SorterHoverColor`、`SorterArrowWidth/Height`、`SorterGap`、`FilterIconSize`、`FilterIdleColor`、`FilterHoverColor`、`FilterHoverBg`、`FilterRadius`、`FilterArrow`、`FilterDropdownBg/Radius/Border`、`ExpandIconSize/Scale/Radius/Bg/Border/Color/HoverColor`、`ExpandIconArrow`、`ExpandedPaddingBlock/Inline`、`SelectionColumnWidth`、`SelectionBorderedExtra`、`SorterTooltip`、`ExpandColumnWidth`、`StickyScrollBarBg/HoverBg/Size`、`CellFontSizeMD`、`CellLineHeight`、`IndentSize`、`SelectionPaddingInline`、`VirtualRowHeight/VirtualHeaderHeight`（数值取自 antd 6.7 / Element Plus 2.14.8 / shadcn v4 源码）。

### 变更

- Table（Ant）：无边框分组表头不再画整高竖线，只在非合并、非本行最后一格画短分隔线；分组单元格（非最后一行）不画下边线（antd table/style/index.js:95-110）。
- Table（Ant）：排序列/悬停表头着色时隐藏该格分隔线；悬停可排序表头时未激活的排序箭头 rgba(0,0,0,.29) → .57（sorter.js，headerIconHoverColor）。
- Table（Ant）：筛选按钮悬停底圆角 4 → 6；筛选下拉圆角 8 → 6、暗色底 #1f1f1f → #141414（filter.js，filterDropdownBg）。
- Table（Ant）：展开 / 树形图标绘制尺寸 17 → 16（17 × 16/17）、圆角 4 → 5.6、悬停色 colorPrimaryHover → colorLinkHover（expand.js）。
- Table（Ant）：带边框表格表体底角不再圆角（antd radius.js 只圆上角；外框见主线程合并）。
- Table（Ant）：表格滚动滑块 6px → 8px，静止色 colorTextPlaceholder（.25，index.js:55 scrollbarColor），悬停/拖动 TextTertiary（同 ui/scroll.go）。
- Table：选择列复选框居中，选择格左右内边距 Ant/shadcn 8（antd selection.js paddingXS）、Element 14（EP el-table-column--selection）。
- Table（Ant 暗色）：行悬停底 #262626 → #1d1d1d、排序/筛选图标 rgba(255,255,255,.25) → .29（rowHoverBg、headerIconColor；主线程合并 theme.go）。
- Table（Element）：排序 caret 间距 3 → 2；筛选箭头 12px → 14px、颜色 color-info、悬停不变色；筛选面板圆角 4 → 2；展开箭头悬停不再变主色；暗色斑马纹 #191919 → #1d1d1d；滚动条 text-color-secondary .3 / .5、6px；无边框分组表头的分组格不画下边线（EP 只给 th.is-leaf 下边线）。
- Table（shadcn）：表头文字 muted-foreground → foreground；汇总行 bg-muted/50 + font-medium；选中行取 --muted；外框圆角 --radius → rounded-md；筛选按钮悬停圆角 6 → 8、筛选下拉圆角 10 → 8（ghost Button / popover rounded-md）；无边框分组表头不再画整高竖线（table.tsx 只有行 border-b）；滚动滑块 bg-border 7px。

## 共享文件改动请求

1. **（已合并）`ui/theme/theme.go`（Ant 暗色）**：`TableRowHoverBg` #262626 → #1d1d1d（antd rowHoverBg = colorFillAlter 叠 colorBgContainer，table/style/index.js:203）；`TableSorterIdle` 暗色 `t.TextQuaternary` → `RGBA(255,255,255,.29)`（headerIconColor = colorIcon .45 × opacityLoading .65，index.js:230）。合并后可在 `ui/theme/align_b2e1_test.go` 的 `TestAlignB2E1AntDarkTableTokens` 加 `tk.RowHoverBg == Hex("#1d1d1d")`、`tk.SorterIdleColor == RGBA(255,255,255,.29)`。CHANGELOG 变更加一条「Ant 暗色表格行悬停 #262626 → #1d1d1d、排序图标 .25 → .29」。
2. **`ui/table_show.go`（E2）外框圆角**：`frame.Radius(rad)`（带边框时四角）改为 `frame.Radii(geom.Radii{TL: rad, TR: rad, BL: tk.BottomRadius, BR: tk.BottomRadius})`，底部圆角条 `if t.bordered && rad > 0` 改用 `tk.BottomRadius`，`l.Stroke(r, geom.Rad(rad), …)` 改用同一 Radii；loading 遮罩同。否则 Ant 带边框表格外框仍是四角圆，而表体（本组）已改为方角。
3. **`ui/table_show.go`（E2）metrics / columns**：行高 `f.lh` 改读 `tk.CellLineHeight`（Ant 22 / Element 23 / shadcn 20，可删 shad2b_table.go 里的 `f.lh = 20`）；Middle 尺寸字号读 `tk.CellFontSizeMD`；选择列宽 `max(48, 16+2*f.ph)` / Element 48 改读 `tk.SelectionColumnWidth`（antd 32，带下拉 +16、bordered +16；`SelectionColumnWidth(px)` 覆盖优先）；展开列宽 48 读 `tk.ExpandColumnWidth`；`Virtual()` 时 `tk.VirtualRowHeight/VirtualHeaderHeight` > 0 则作为默认行高 / 表头高（EP el-table-v2 50/50，`RowHeight()`/`HeaderHeight()` 优先）。注意 Ant 选择列 48 → 32 是 Ant 外观变更，需写 CHANGELOG。
4. **E2 的 `Title()` / `Footer()` 区**：底色 `tk.FooterAreaBg`（Ant #fafafa，Element / shadcn 无）、文字 `tk.FooterAreaColor`（Ant colorTextHeading、Element text-color-regular、shadcn caption muted-foreground），padding 同单元格；Ant 带边框时 footer 无上边框（三条边），底圆角不论是否带边框都是 `HeaderBorderRadius`（bordered.js、radius.js `&-footer`）。复审后已按此实现（TestTableTitleFooterBorders、TestTableFooterRadius）。
5. **（已合并）`ui/g6_tokens_test.go`（主线程）**：`TestG6TableTokenDefaults` 断言 `k.RowStripedBg != th.FillAlter` 对 Element 暗色不再成立（EP 斑马纹是 fill-color-lighter #1d1d1d，Tango FillAlter 暗色是 extra-light #191919）。改成 `(!el && k.RowStripedBg != th.FillAlter)`，Element 由 `TestAlignB2E1ElementTableTokens` 覆盖。**合并前该用例失败。**
6. **`ui/table.go`（E2）indentSize 默认**：`propFloat(c, "Table", "indentSize", 15)` 的默认值改为 `c.ThemeFor(theme.CompTable).Components.Table.IndentSize`（Element 16，EP el-table `indent` 默认 16）；构造处 `indent: 15` 同。Element 树形表格缩进 15 → 16 写 CHANGELOG 变更。
