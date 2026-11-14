# 规范对齐 第 2 批：数据展示和表格 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 第 2 批（设计 §6）的组件逐项对照 antd 6.7 / Element Plus 2.14.8 / shadcn v4，写对照记录，补齐缺口（含行为测试、gallery 演示、`ConfigProvider.Defaults`、a11y），默认值不符的直接改并写 CHANGELOG；开工前先做第 1 批整体复审（Task 0）。

**Architecture:** 结构与[第 1 批计划](2026-11-14-spec-align-b1.md)相同：按文件拆成 E、F、G、H 四组并行，每组独占自己的文件，组后各派一个审阅 agent；共享文件只由主线程合并。各组把要改共享文件的地方写进自己记录末尾的「共享文件改动请求」。

**Tech Stack:** Go 1.27；参考源码 `/tmp/tango-ref`：antd@6.7.0（`antd/package/es/<组件>`，令牌 `es/<组件>/style/token` 或 `style/index.js` 的 `prepareComponentToken`）、element-plus@2.14.8（`ep/package/es/components/<名>`、`ep/package/theme-chalk/src`）、shadcn-ui/ui@e62c9435fff729a175f38983dee73b2cb4f6a069（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、tailwindcss@4.3.5（`tw/package`）。antd 依赖的 `@rc-component/*` 包（table、tree、tabs、image、collapse、tooltip 等）需要时 `npm pack` 到 `/tmp/tango-ref/rc-<名>`。

**Spec:** [docs/specs/2026-11-14-spec-alignment-design.md](../specs/2026-11-14-spec-alignment-design.md)（§3–§7.1，§6 第 2 批与「每一批的步骤」）；记录模板 [docs/specs/align/README.md](../specs/align/README.md)；编写约定 [ui/AUTHORING.md](../../ui/AUTHORING.md)；API 规则 [docs/release.md](../release.md)「API 稳定性」R1–R12。

## Global Constraints

与第 1 批计划的 Global Constraints 逐条相同（worktree、不提交、作者 `tango <903300326@qq.com>` 不加 Co-Authored-By、精确值读源码、功能并集一套 API、默认值走令牌/Defaults 不按 Flavor 分支、默认值不符直接改并写 CHANGELOG 草稿、稳定层只增、去重、缺基础能力写「暂不做，缺 X」、新代码英文文档中文文案走 Locale、Ant 外观除规范修正外不变）。本批另加：

- **去重**（AUTHORING §13、§14）：shadcn Badge = `Tag`（只补外观）；shadcn Accordion/Collapsible = `Collapse`；shadcn Empty = `Empty`；shadcn HoverCard = `Popover.Trigger("hover")`（第 3 批）；EP TableV2 = `Table` 虚拟滚动（`Virtual`）；EP TreeV2 = `VirtualTree`；antd 6 的 `Listy` 对照 Tango `Listy`，`List`（antd 6 已弃用但仍导出）照常对照；Statistic.Timer = `StatisticTimer`/`Countdown`。
- **v6 改名**：Tabs `tabPosition`→`tabPlacement`、`destroyInactiveTabPane`→`destroyOnHidden`、`indicatorSize`→`indicator.size`；Card `bordered`→`variant`；Tag `bordered`→`variant: filled|solid|outlined`；Collapse `expandIconPosition`→`expandIconPlacement`；Carousel `dotPosition`→`dotPlacement`；Progress `gapPosition`→`gapPlacement`；Timeline `mode` 取 `start|alternate|end`、新增 `orientation`、`items[].title/content`。Tango 已有旧名方法的，新增新名方法，旧方法加 `// Deprecated: use X.`（不删）。
- **shadcn 暗色 destructive（用户定）**：Badge（= `Tag` 的 shadcn destructive 外观）暗色背景用 v4 的 `dark:bg-destructive/60`（badge.tsx:15），与 Button 已有做法（`ui/theme/shadcn.go` 的 `ButtonDangerBg = Over(Fade(Destructive,.6), bg)`）一致；去掉白字对比 2.89 的问题。
- **comp-\* 分支**：Dock、可拖拽 Tabs（`nav_tabs_drag.go`）、chat、viewer、shell、graph 已经或正在进 main。Tabs 的改动不能改坏 `dock.go` 和拖拽/撕出；rebase 后 origin/main 改过本组文件的，以对方为准再适配，不回退。
- Ant 主题之外的改动不得改变 Ant 主题截图（第 1 批 Review Focus 1 同样适用）。

## 已定事项（主线程临时决定，用户可推翻）

1. **Button `autoInsertSpace`**：保持布尔主题令牌 `ButtonToken.AutoInsertSpace`（Ant true，Element/shadcn false），不改成 Defaults-only。
2. **Transfer 右侧顺序**：保持 antd 基线「新移入的在前」；Element 行为用 `TargetOrder("original")`（或 `Defaults("Transfer", Props{"targetOrder": "original"})`）。
3. **Link 默认颜色**：给 `Link` 单独的颜色令牌，Element 下默认常规文字色（EP 2.14.8 `type="default"` = `--el-text-color-regular`，悬停变 primary），Anchor、Paragraph 展开链接、日期面板「今天」保持链接色。涉及的 `ui/theme/comp_typography.go`、`ui/text.go` 都不归第 2 批任何组，也不归第 3 批的组 → **由主线程在本批做**（Task 0 Step 4），算 CHANGELOG 变更。
4. **Ant Sider 收起宽度**：保持 64（ProLayout 收起宽度 64 依赖同一个 Sider），作为对 antd `collapsedWidth` 80 的偏离，写在 `ui/theme/comp_layout.go` 注释、`docs/specs/align/1-basic.md` 和 `docs/design-baseline.md`「已记录的偏离」。

## 文件归属

每个文件只有一个主人；不在表里的文件只读，要改就写「共享文件改动请求」。混合文件只改括号里写的那部分。

| 组 | 组件 | 独占的源文件 | 独占的主题文件 | 独占的 gallery 文件 | 新建 |
|---|---|---|---|---|---|
| E 表格 | Table（含 EP TableV2 = 虚拟滚动、树形、勾选、排序筛选、固定列、合并、转置、可编辑单元格、展开行、汇总） | `ui/table.go`、`table_body.go`、`table_header.go`、`table_model.go`、`table_show.go`、`table_edit.go`、`table_keys.go`、`table_transpose.go`、`table_icons.go` 及同名 `_test.go`（**不含** `table_pro.go`，第 4 批） | `comp_table.go` | `page_table_basic.go`、`page_table_edit.go`、`page_table_fixed.go`、`page_table_select.go`、`page_table_sortfilter.go`、`page_table_span.go`、`page_table_transpose.go`、`page_table_tree.go`、`page_table_treecheck.go`、`page_table_virtual.go`、`page_table_wrap.go` | `docs/specs/align/2-table.md`、`ui/align_b2e_test.go`、`ui/theme/align_b2e_test.go`、`examples/ui-gallery/page_b2e.go` |
| F 树、列表、卡片、折叠 | Tree、VirtualTree（EP TreeV2）、List/ListItemMeta/ListActions、Listy、VirtualList、Card/CardMeta/CardAction、Descriptions、Collapse（shadcn Accordion/Collapsible） | `ui/data_tree.go`、`fill_virtual_tree.go`、`data_list.go`、`fill_listy.go`、`more_virtual.go`、`data_card.go`、`data_collapse.go` 及同名 `_test.go` | `comp_tree.go`、`comp_list.go`、`comp_card.go`、`comp_descriptions.go`、`comp_collapse.go` | `page_data_tree.go`、`page_fill_virtual_tree.go`、`page_data_list.go`、`page_more_virtual.go`、`page_data_collapse.go`、`page_shad2a_tree.go` | `docs/specs/align/2-data.md`、`ui/align_b2f_test.go`、`ui/theme/align_b2f_test.go`、`examples/ui-gallery/page_b2f.go` |
| G 标签、徽标与状态 | Tag（shadcn Badge）、CheckableTag、Badge/BadgeRibbon、Avatar/AvatarGroup、Statistic/Countdown/StatisticTimer、Empty、Skeleton（含 SkeletonAvatar/Button/Input/Image）、Progress、Tooltip、Popover | `ui/display.go`（**只改** Tag、Avatar、Statistic、FormatNumber、Tooltip 及其私有辅助；Divider 不动）、`more_checktag.go`、`data_badge.go`、`data_skeleton.go`、`data_result.go`（**只改** Empty 部分；Result 是第 3 批）、`data_progress.go`、`data_popover.go`（**只改** Popover 部分；Tour 是第 3 批）及它们的 `_test.go` | `comp_tag.go`、`comp_badge.go`、`comp_avatar.go`、`comp_statistic.go`、`comp_empty.go`、`comp_skeleton.go`、`comp_progress.go`、`comp_tooltip.go`、`comp_popover.go`（只改 Popover 用的字段，Popconfirm 字段不动） | `page_more_checktag.go` | `docs/specs/align/2-display.md`、`ui/align_b2g_test.go`、`ui/theme/align_b2g_test.go`、`examples/ui-gallery/page_b2g.go` |
| H 标签页、时间线与媒体 | Tabs（含拖拽排序/撕出，不改 Dock）、Timeline、Image（预览）、Carousel、Calendar、QRCode、Watermark、Masonry | `ui/nav.go`（**只改** Tabs 及其私有辅助；Breadcrumb、Pagination 是第 3 批）、`nav_tabs_drag.go`、`data_timeline.go`、`data_image.go`、`data_carousel.go`、`data_calendar.go`、`data_qrcode.go`、`data_watermark.go`、`fill_masonry.go` 及同名 `_test.go` | `comp_tabs.go`、`comp_timeline.go`、`comp_image.go`、`comp_carousel.go`、`comp_calendar.go` | `page_shad2c_tabs.go`、`page_data_timeline.go`、`page_data_image.go`、`page_data_qrcode.go`、`page_data_watermark.go` | `docs/specs/align/2-media.md`、`ui/align_b2h_test.go`、`ui/theme/align_b2h_test.go`、`examples/ui-gallery/page_b2h.go` |

**主线程独占（共享文件）：** 第 1 批计划列出的全部共享文件，加上：`ui/data_util.go`、`ui/fill_util.go`、`ui/nav_util.go`、`ui/date_util.go`、`ui/text.go`、`ui/button.go`、`ui/dock*.go`、`ui/table_pro.go`；`ui/theme/comp_typography.go`、`comp_button.go`、`comp_layout.go`、`comp_result.go`、`comp_tour.go`；混合 gallery 页 `page_display.go`、`page_data_misc.go`、`page_data_misc2.go`、`page_p2c_display.go`、`page_fill_layouts.go`、`page_shad2b_variants.go`、`page_shadcn.go`、`page_nav.go`、`page_p2c_nav.go`、`page_dock.go`、`page_nav_steps.go`、`page_more_locale.go`、`page_date_more.go`、`page_table_pro.go`、`page_table_util.go`。

**Locale 例外**与**并行纪律**同第 1 批计划（组件自己的 `XxxLocale` 段只用 Edit 小块插入；每次保存都让 `go build ./ui/...` 能过；只跑自己的测试）。日历用到的 `date_util.go` 是共享辅助：组 H 需要改时写请求。

---

## Review Focus

1. **Ant 主题外观意外变化**：只有记录里写明的规范修正能改 Ant 外观。→ Task 5 Step 3 对比 `origin/main` 与本分支 `-theme antd` 截图，差异只出现在 CHANGELOG「变更」列出的组件上；`TestGolden` 黄金图（含 Tag、Badge、Avatar、Progress）只在 G 组记录写明的变更下重录。
2. **空数据和越界受控值**：Table/Tree/List/Descriptions/Carousel/Tabs 传入空切片、`nil`、当前项 key 不在 items 里、Carousel/Image 预览索引越界、Progress 百分比 <0 或 >100、Badge count 负数、Statistic NaN/Inf 时不 panic，表现与 antd 一致（空态显示 Empty、夹到范围、active 回落到第一项）。→ 每组为改过的受控组件加一条 `Test…Empty…` 或 `Test…OutOfRange…`。
3. **禁用**：新增的交互（Tabs 新增/关闭、Collapse 头/图标触发、Tree 节点、Tag 关闭、Image 预览按钮、Carousel 箭头）在 `Disabled(true)` 或 `ConfigProvider.ComponentDisabled` 下不响应，查询方法返回 false。→ 每组每个新交互一条 `Test…Disabled…`。
4. **键盘与无障碍**：Tabs（←/→、Home/End，`tab`/`tabpanel` 角色）、Tree（↑↓←→、`tree`/`treeitem`、expanded/checked 状态）、Collapse（Enter/Space、expanded）、Carousel（←/→）、Image 预览（Esc、←/→）、Table 行选择有 a11y 角色、名称、焦点环，名称来自 Locale。→ 每组至少一条 `Test…A11y…`，并跑 gallery `-audit`。
5. **暗色与三套主题**：新绘制只用令牌，不判断 `th.Dark`；三套 × 明暗都能渲染，文字对比度过 `TestGalleryTextContrast`；shadcn 暗色 destructive Tag 白字对比 ≥ 4.5（G 组 `TestAlignB2GShadcnDestructiveDark`）。

---

### Task 0（主线程）：第 1 批复审、黄金图、已定事项

**Files:** Modify 视复审结果；`ui/testdata/basic_element.png`；`ui/theme/comp_typography.go`、`ui/text.go`、`ui/theme/comp_layout.go`（注释）、`docs/design-baseline.md`、`docs/specs/align/1-basic.md`；Create 本计划。

- [ ] **Step 1: 复审。** 派一个全新审阅 agent 看 `origin/main..spec-align` 全范围（给 diff 范围和设计文档，不给结论）：正确性、跨组不一致、稳定层规则、主题泄漏。确认的问题在一个单独提交里修（`fix(ui): batch 1 review fixes`）。
- [ ] **Step 2: Element 黄金图。** 在 `origin/main` 与 `spec-align` 的干净副本上分别 `go test ./ui -run 'TestGolden$' -update`，比较 `basic_element.png`。结论（2026-11-14 已做）：两者重新生成的结果逐字节相同 → **不是第 1 批造成**；与已提交黄金图差 138 像素，其中 39 像素在 `588ac23`（图标集）之前就已存在，其余来自 `588ac23` 让 Element 主题改画 Element Plus 图标（搜索图标变大），属预期。更新 `basic_element.png`，随 Step 1 的修复提交。
- [ ] **Step 3: Sider 偏离。** `comp_layout.go` 的 `SiderCollapsedWidth` 注释写「与 antd 6.7 的差异：collapsedWidth 80 → 64，原因 ProLayout」；`1-basic.md` 对应行结论改为「等价写法/已记录偏离」；`docs/design-baseline.md` 加「已记录的偏离」一节列出这条。
- [ ] **Step 4: Link 令牌。** `TypographyToken` 追加 `LinkTextColor`、`LinkTextHoverColor`、`LinkTextActiveColor`（`Link` 默认三态色；Ant/shadcn = `TextLink*`，Element = `TextSecondary`（EP `--el-link-text-color` = text-color-regular #606266）/ `Primary.Base`（`hover-text-color` = color-primary）/ `Primary.Base`（EP 无单独按下色），var.scss:1480-1489；`text.go` 的 `(*LinkW).colors` 在 `status == ""` 时改读这三项；Paragraph 展开链接与复制图标仍读 `LinkColor`。测试 `TestLinkElementDefaultRegular`（Element 下 `Link` 静止色 = `TextSecondary`，悬停后 = `Primary.Base`；Ant 下 = `TextLink`）和 `ui/theme` 里令牌比对。CHANGELOG 变更一条。
- [ ] **Step 5:** 提交 Step 2–4（`fix(ui): batch 1 follow-ups: Element Link color, Sider deviation note, Element golden`），进度写入 `.superpowers/sdd/2026-11-14-spec-align-b2/progress.md`。

---

### Task 1–4（每组一个，结构相同）：对照、补齐、测试、演示

步骤与第 1 批计划 Task 1–4 的 Step 1–6 完全相同，把文件名里的 `b1<组>` 换成 `b2<组>`、记录换成本组的 `2-<名>.md`，gallery 新页 `Name` 形如 `<组件>Align`、`Parent: "<组件>"`，出图目录 `/tmp/spec-align-b2/<组>`。记录末尾同样有「基础能力缺口」「CHANGELOG 草稿」（新增 / 变更）「共享文件改动请求」三节。

**Interfaces（各组共同）：** 同第 1 批；另：表格、树、列表的空态用已有 `Empty`（组 G 的公开入口，签名不变）；浮层用已有 overlay/`layers.go`（只读）。

#### Task 1 — 组 E（表格）已知缺口（起点，不是全部）

- antd `es/table/interface.d.ts`、`InternalTable.d.ts`、`hooks/useSelection`、`hooks/useSorter`、`hooks/useFilter` 与 `@rc-component/table`：逐项对照 `columns`（`align`、`ellipsis`、`fixed`、`width`、`minWidth`、`hidden`、`sorter.multiple`、`sortDirections`、`defaultSortOrder`、`showSorterTooltip`、`filterMode: menu|tree`、`filterSearch`、`filterOnClose`、`filterDropdown`）、`rowSelection`（`type`、`fixed`、`columnWidth` 32、`hideSelectAll`、`selections`、`preserveSelectedRowKeys`、`getCheckboxProps`）、`expandable`（`expandRowByClick`、`showExpandColumn`、`expandIcon`、`indentSize` 15、`childrenColumnName`）、`size: large|middle|small`、`bordered`、`showHeader`、`sticky`、`scroll.x/y`、`virtual`、`loading`、`locale.emptyText`、`pagination` 位置、`summary`、`rowClassName`/`onRow`（等价写法）。
- 令牌：antd `es/table/style/index.js` 的 `prepareComponentToken`（`headerBg`、`headerColor`、`rowHoverBg`、`rowSelectedBg`、`cellPaddingBlock` 16 / MD 12 / SM 8、`cellPaddingInline` 16/8/8、`headerSplitColor`、`expandIconBg`、`selectionColumnWidth`、`stickyScrollBarSize`）；EP `theme-chalk/src/table.scss` 与 `var.scss $table`（`--el-table-header-text-color`、`--el-table-row-hover-bg-color`、`stripe`、`border`、`highlight-current-row`、`show-overflow-tooltip`、`size`）；EP table-v2 `es/components/table-v2` 的 `row-height` 50、`header-height` 50、`fixed`、`estimated-row-height`；shadcn `table.tsx`（`h-10 px-2` 表头、`p-2` 单元格、`hover:bg-muted/50`、`data-[state=selected]:bg-muted`、`border-b`）。

#### Task 2 — 组 F（树、列表、卡片、折叠）已知缺口

- Tree：antd `es/tree/Tree.d.ts`（`blockNode`、`showLine {showLeafIcon}`、`showIcon`、`checkStrictly`、`draggable {icon, nodeDraggable}`、`loadData`、`virtual`、`height`、`switcherIcon`、`titleRender`、`fieldNames`→不需要、`selectable`、`multiple`）；DirectoryTree `expandAction`；EP `el-tree`（`indent` 18、`lazy/load`、`accordion`、`highlight-current`、`expand-on-click-node`、`check-on-click-node`、`default-expand-all`、`filter-node-method`、`allow-drag/allow-drop`、`render-after-expand`）；EP `el-tree-v2`（`height` 200、`item-size` 26，对 VirtualTree）。
- List：antd 6 `List` 已弃用但仍导出，对照 `es/list/index.d.ts`（`grid`、`itemLayout`、`loadMore`、`split`、`size`、`bordered`、`header/footer`、`pagination`）；antd 6.7 新组件 `es/listy`（对 Tango `Listy`，逐项对照 API 与令牌）。
- Card：antd `variant: outlined|borderless`（`bordered` 弃用）、`size: default|small`、`type: inner`、`cover`、`actions`、`extra`、`loading`、`hoverable`、`tabList/activeTabKey/tabBarExtraContent`；EP `el-card`（`shadow: always|hover|never` 默认 always、`header`/`footer`、`body-style`）；shadcn v4 `card.tsx`（`rounded-xl border py-6 shadow-sm gap-6`、`CardHeader px-6`、`CardAction`、`CardFooter`）。
- Descriptions：antd `items`、`column`（含响应式对象）、`layout`、`bordered`、`size`、`colon`、`labelStyle/contentStyle`（等价写法）、`items[].span: 'filled'`；EP `direction`、`border`、`column` 3、`size`、`label-width`、`span`、`rowspan`、`align`、`label-align`。
- Collapse：antd `expandIconPlacement`（`expandIconPosition` 弃用）、`collapsible: header|icon|disabled`、`ghost`、`bordered`、`accordion`、`size`、`destroyOnHidden`、`items[].extra/showArc`；EP `accordion`、`expand-icon-position`、`before-collapse`、`icon`；shadcn `accordion.tsx`（`border-b last:border-b-0`、`py-4 text-sm font-medium hover:underline`、chevron 旋转 180°）。

#### Task 3 — 组 G（标签、徽标与状态）已知缺口

- Tag：antd 6 `variant: filled|solid|outlined`（`bordered` 弃用，`es/tag/index.d.ts:30-38`）、`color`（预设/状态/自定义）、`icon`、`closeIcon`、`disabled`、`href`；CheckableTag、`Tag.CheckableTagGroup`（如有）；EP `el-tag`（`type`、`effect: dark|light|plain`、`round`、`size`、`hit`、`disable-transitions`）、`el-check-tag`（`type`、`disabled`）；shadcn `badge.tsx`（`variant: default|secondary|destructive|outline|ghost|link`、`rounded-full px-2 py-0.5 text-xs font-medium`、**暗色 destructive `dark:bg-destructive/60`**，测试 `TestAlignB2GShadcnDestructiveDark`：shadcn 暗色 destructive Tag 背景 = `Over(Fade(destructive,.6), background)`，白字对比 ≥ 4.5）。
- Badge：antd `count`、`overflowCount` 99、`dot`、`showZero`、`size: default|small`、`status`、`text`、`color`、`offset`、`title`；Ribbon `placement: start|end`；EP `el-badge`（`value`、`max`、`is-dot`、`hidden`、`type`、`show-zero`、`color`、`offset`、`badge-style`→等价写法、`content` 插槽）。
- Avatar / AvatarGroup：antd `size`（数字/small/default/large/响应式）、`shape`、`gap` 4、`src`、`icon`、`alt`、`draggable`→不需要；`Avatar.Group max {count, style, popover}`、`size`、`shape`；EP `el-avatar`（`size` 40/24/56? 以 theme-chalk 为准、`fit`、`src-set`→不需要）、`el-avatar-group`（2.14 的 `collapse-avatars`、`max-collapse-avatars`、`collapse-avatars-tooltip`、`size`、`shape`）；shadcn `avatar.tsx`（`size-8 rounded-full`、`AvatarFallback bg-muted`、v4 的 `AvatarGroup`/`AvatarBadge` 如有）。
- Statistic：antd `decimalSeparator`、`groupSeparator`、`precision`、`prefix/suffix`、`loading`、`formatter`、`Statistic.Timer {type: countdown|countup, format, onFinish, onChange}`；EP `el-statistic`（`decimal-separator`、`group-separator`、`precision`、`value-style`、`title`）、`el-countdown`（`format` HH:mm:ss、`finish`、`change`）。
- Empty：antd `image`（默认图 / `PRESENTED_IMAGE_SIMPLE`）、`description`、`styles.image`；EP `el-empty`（`image`、`image-size`、`description`）；shadcn v4 `empty.tsx`（`EmptyHeader`、`EmptyMedia variant: default|icon`、`EmptyTitle`、`EmptyDescription`、`EmptyContent`，`border-dashed rounded-lg p-6 md:p-12 gap-6`）→ 补到 `Empty` 的外观和缺的能力，不新建入口。
- Skeleton：antd `active`、`avatar {size, shape}`、`paragraph {rows, width}`、`title {width}`、`round`、`loading`、`Skeleton.Node`；EP `rows` 3、`animated`、`count`、`throttle`、`loading`；shadcn `skeleton.tsx`（`bg-accent animate-pulse rounded-md`）。
- Progress：antd `type: line|circle|dashboard`、`size`（数字/[w,h]/small/default）、`steps`、`strokeLinecap`、`gapDegree`、`gapPlacement`（`gapPosition` 弃用）、`percentPosition {align, type}`、`success`、`rounding`、`format`；EP `stroke-width` 6、`text-inside`、`indeterminate`、`duration`、`striped`、`striped-flow`、颜色数组/函数；shadcn `progress.tsx`（`h-2 rounded-full bg-primary/20`）。
- Tooltip / Popover：antd `placement` 12 向、`arrow {pointAtCenter}`、`autoAdjustOverflow`、`color`、`mouseEnterDelay` 0.1、`mouseLeaveDelay` 0.1、`trigger`、`open`、`destroyOnHidden`、`fresh`；EP `effect: dark|light`、`show-after` 0、`hide-after` 200、`offset` 12、`enterable`、`show-arrow`、`persistent`；Popover EP `width` 150、`title`；shadcn `tooltip.tsx`（`bg-foreground text-background px-3 py-1.5 text-xs rounded-md`、`sideOffset` 0、`delayDuration` 0、箭头）、`popover.tsx`（`w-72 rounded-md border p-4 shadow-md`、`sideOffset` 4）。

#### Task 4 — 组 H（标签页、时间线与媒体）已知缺口

- Tabs：antd `es/tabs/index.d.ts`：`tabPlacement`（61-63，`tabPosition` 弃用）、`indicator {size, align}`（`indicatorSize` 弃用）、`destroyOnHidden`（40）、`type: line|card|editable-card`、`size`、`centered`、`tabBarExtraContent {left,right}`、`tabBarGutter`、`addIcon/removeIcon/more`、`items[].icon/closable/disabled/forceRender`；EP `el-tabs`（`type: card|border-card`、`tab-position`、`stretch`、`addable`、`editable`、`closable`、`before-leave`、`tab-add/tab-remove`）；shadcn v4 `tabs.tsx`（`TabsList` 的 `variant: default|line`、`orientation: vertical`、`h-9 rounded-lg p-[3px] bg-muted`、选中 `bg-background shadow-sm`）。
- Timeline：antd `es/timeline/Timeline.d.ts:42-45`：`variant`、`mode: start|alternate|end`（left/right 弃用）、`orientation: horizontal|vertical`、`titleSpan`、`items[].title/content/icon/color/loading`、`reverse`、`pending`（看是否弃用）；EP `el-timeline`（`mode`、`reverse`）、`el-timeline-item`（`timestamp`、`hide-timestamp`、`center`、`placement: top|bottom`、`type`、`color`、`size: normal|large`、`icon`、`hollow`）。
- Image：antd `preview {open, src, mask, cover, actionsRender→等价写法, minScale 1, maxScale 50, scaleStep 0.5, movable}`、`fallback`、`placeholder`、`Image.PreviewGroup {items, current}`；EP `el-image`（`fit`、`lazy`、`preview-src-list`、`zoom-rate` 1.2、`min-scale` 0.2、`max-scale` 7、`infinite`、`initial-index`、`hide-on-click-modal`、`show-progress`、`close-on-press-escape`）、`el-image-viewer`。
- Carousel：antd `dotPlacement`（`dotPosition` 弃用）、`autoplay {dotDuration}`、`autoplaySpeed` 3000、`speed` 500、`effect: scrollx|fade`、`arrows`、`draggable`、`infinite`、`adaptiveHeight`、`waitForAnimate`；EP `el-carousel`（`height`、`initial-index`、`trigger: hover|click`、`autoplay` true、`interval` 3000、`indicator-position`、`arrow: hover|always|never`、`type: card`、`cards-scale` 0.83、`direction`、`loop`、`pause-on-hover`、`motion-blur`）；shadcn `carousel.tsx`（`orientation`、前后按钮 `size-8 rounded-full` outline，在外侧 `-left-12`）。
- Calendar：antd `fullscreen`、`mode: month|year`、`validRange`、`disabledDate`、`headerRender`/`cellRender`/`fullCellRender`（等价写法）、`showWeek`；EP `el-calendar`（`range`、`controller-type: button|select`、`formatter`、`header`/`date-cell` 插槽）；shadcn `calendar.tsx`（react-day-picker：`--cell-size` 2rem、`captionLayout`、`showOutsideDays`、今天 `bg-accent`、选中 `bg-primary`）。
- QRCode：antd `value`、`type: canvas|svg`（不需要区分）、`icon`、`size` 160、`iconSize` 40、`color`、`bgColor`、`bordered`、`errorLevel` M、`status: active|expired|loading|scanned`、`statusRender`、`boostLevel`、`marginSize`。
- Watermark：antd `width`、`height`、`rotate` -22、`zIndex` 9、`image`、`content`（数组）、`font {color, fontSize 16, fontWeight, fontFamily, fontStyle, textAlign}`、`gap` [100,100]、`offset`、`inherit`；EP `el-watermark` 同族属性，对照默认值差异。
- Masonry：antd 6 新组件 `es/masonry/Masonry.d.ts`：`columns`（含响应式）、`gutter`、`items`、`itemRender`、`sequential`、`fresh`、`onLayoutChange`。

---

### Task 1R–4R：审阅（每组一个审阅 agent）

同第 1 批计划 Task 1R–4R（读计划与该组记录，`git diff -- <该组文件>`；抽查至少 5 条结论与 `/tmp/tango-ref` 源码；检查 Global Constraints；跑该组测试；输出严重 / 一般 / 建议清单，实施 agent 修复后主线程确认）。

---

### Task 5（主线程）：合并共享文件、验证、提交

同第 1 批计划 Task 5，另加：

- [ ] **Step 1: 合并请求**：各组「共享文件改动请求」与「CHANGELOG 草稿」合并（`## [未发布]` 的「新增」「变更」，一组一条总条目加子项；Task 0 的 Link 颜色单列一条变更）；AUTHORING §14 表按请求更新。
- [ ] **Step 2: rebase 与构建**：先按组提交，`git fetch origin && git rebase origin/main`；`go build ./...`；`go vet ./ui/... ./uihost/...`；`go test ./ui/... ./uihost/...`（`ui/editor` 计时用例失败时单独重跑）。
- [ ] **Step 3: gallery**：`go test -timeout 60m ./examples/ui-gallery/...`；`TestGalleryTextContrast`；`go run ./examples/ui-gallery -audit` 与 `origin/main` 比较，无新增 FAIL；Ant 截图对比见 Review Focus 1。
- [ ] **Step 4: API**：`go run ./internal/cmd/apitxt -w && go run ./internal/cmd/apitxt -check`；`bash scripts/apidiff.sh` 无稳定层破坏。
- [ ] **Step 5: 提交并汇报**：按组提交（`feat(ui): align <组> with antd 6.7 / Element Plus 2.14.8 / shadcn v4 (batch 2, group <E-H>)`）加 `chore: batch 2 API files, changelog`；每个里程碑写一行进度到 `.superpowers/sdd/2026-11-14-spec-align-b2/progress.md`。
