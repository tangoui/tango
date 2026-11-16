# 第 3 批 · 组 J 导航与页面 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、@rc-component/pagination@1.7.0（`rc-pagination/package/es`，本批 npm pack）、@rc-component/steps@1.2.3（`rc-steps/package/es`，本批 npm pack）、@ant-design/pro-components@3.1.14-2（`pro/package/es`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）。下文省略这些前缀：`antd` 指 `antd/package/es`，`rc-pg` 指 `rc-pagination/package/es`，`pro` 指 `pro/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`。

本组测试：`ui/align_b3j_test.go`（行为）、`ui/theme/align_b3j_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b3j.go`（`PaginationAlign`、`StepsAlign`、`SplitterAlign`、`BreadcrumbAlign`、`PageHeaderAlign`）。Anchor / Affix / BackTop 沿用 `page_nav_anchor.go` 的 `Anchor` 页（新能力都是默认值与小选项，未新开页）。

## Pagination

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 页码窗口 | rc-pg/Pagination.js:293-385：`pageBufferSize` 2（showLessItems 1），总页数 ≤ 3+2×buffer 全显示；否则首尾页 + 当前页两侧 buffer 页，靠近跳转按钮时少一页；`jumpPrev/Next` 跳 5（showLessItems 3） | nav.go `pageList`：≤ 9 页全显示、首尾 + 当前页两侧 2 页 | 已补齐（TestPaginationPageListRc）：按 rc-pg 原样移植；**变更**：8、9 页时折叠为 •••（原先全显示），当前页在 5 / pages-4 时窗口与 antd 一致 |
| showLessItems | antd/rc-pg `showLessItems`（同上） | — | 已补齐（TestPaginationShowLessItems）：`ShowLessItems()`、Defaults 键 `showLessItems`；跳转按钮名 `Locale().Pagination.Prev3/Next3`（rc-pg locale prev_3/next_3） |
| 跳转按钮外观 | antd/pagination/style/index.js genPaginationJumpStyle：省略号 colorTextDisabled，悬停/聚焦换成 colorPrimary 的 DoubleLeft/Right | 省略号用文字色，无悬停图标 | 已补齐（令牌 `EllipsisColor/JumpHoverArrow/JumpArrowColor`，TestAlignB3JPaginationTokens）；**变更**：Ant 省略号颜色 Text → TextDisabled，悬停显示双箭头；shadcn PaginationEllipsis 无悬停箭头 |
| size | antd 6 `size: small|middle|large`（SizeType），`itemSizeLG` controlHeightLG、`item-spacing-actual` marginXS / small marginXXS / large marginSM（style/index.js:465-475）；EP `size` large 40px | 只有 Small / 默认；间距 Ant 8 / small 2 写死 | 已补齐（TestPaginationSizeAlignOptions）：`Size(Large)`、Defaults `size: "large"`；令牌 `ItemSizeLG/ItemSpacing/SM/LG`；**变更**：Ant 小号按钮间距 2 → 4（marginXXS） |
| align | antd `align: start|center|end`（rc-pg interface.d.ts、style `&-start/center/end`） | — | 已补齐（TestPaginationSizeAlignOptions）：`Align(a)`、Defaults 键 `align` |
| showTotal(total, range) | rc-pg `showTotal(total, [from, to])` | `ShowTotal()` 固定文案 | 已补齐（TestPaginationGoButtonTotalFunc）：`ShowTotalFunc(fn)` |
| showQuickJumper.goButton | antd `showQuickJumper: {goButton}`；rc-pg locale `jump_to_confirm` | 无按钮 | 已补齐（TestPaginationGoButtonTotalFunc）：`GoButton(label)`，"" 用 `Locale().Pagination.Confirm` |
| 跳页框宽 | antd quickJumperInputWidth = controlHeightLG×1.25 = 50，small ×1.1 = 44（style prepareToken）；EP `__editor` 56px | 50 / 简洁模式 48、40 | 已补齐（令牌 `QuickJumperInputWidth/SM`）；**变更**：Ant 简洁模式输入框 48/40 → 50/44；Element 跳页框 50 → 56 |
| pageSizeOptions | rc-pg Options.js 默认 [10,20,50,100]；EP `page-sizes` [10,20,30,40,50,100] | 写死 [10,20,50,100] | 已补齐（TestPaginationSizeAlignOptions）：`PageSizeOptions(...)`、Defaults 键 `pageSizeOptions`、令牌 `PageSizesByTen`；**变更**：Element 默认每页选项改为 EP 默认 |
| pager-count | EP `pager-count` 7 | `PagerCount(n)`，Element 默认未设 | 已补齐（TestPaginationElementDefaults）：令牌 `PagerCount`（Element 7）与 Defaults 键 `pagerCount`；**变更**：Element 主题默认 7 个页码按钮 |
| background | EP `background` false（Tango 的 Element 主题原本默认 true） | 按 `Flavor == Element` 赋默认 | 已补齐：令牌 `Background`（Element true），去掉组件内 Flavor 分支；外观不变 |
| prev-text / next-text / prev-icon / next-icon | EP 同名；antd itemRender prev/next | — | 已补齐：`PrevText/NextText/PrevIcon/NextIcon`（a11y 名仍为 Locale 上一页 / 下一页） |
| 当前页悬停 | antd `&-active:hover` borderColor colorPrimaryHover、文字 itemActiveColorHover | 无悬停变化 | 已补齐（令牌 `ItemActiveColorHover/ItemActiveBorderHover`） |
| 禁用当前页 | antd disabled `&-active` borderColor colorBorder + itemActiveBgDisabled | 只有底色 | 已补齐：加 colorBorder 描边 |
| total 0 / page 越界 / pageSize 0 | rc-pg `calculatePage`：total 0 时 allPages 0，渲染一个禁用的 1（noPager），prev/next 禁用；current 夹到范围 | 页数按 1 处理，可点 1 | 已补齐（TestPaginationOutOfRange）：total 0 时第 1 页禁用；负 total 视为 0；pageSize ≤ 0 视为 1 |
| disabled | antd `disabled`；ConfigProvider.componentDisabled | `Disabled(on)` | 已补齐（TestPaginationDisabledScope）：`ConfigProvider.ComponentDisabled` 生效（含确认按钮） |
| simple / readOnly / hideOnSinglePage | antd | `Simple/ReadOnly/HideOnSinglePage` | 已对齐 |
| showSizeChanger 自动显示 | rc-pg `showSizeChanger = total > totalBoundaryShowSizeChanger(50)` | 需显式 `SizeChanger(&size)` | 等价写法：size 值由应用持有，只有调用 `SizeChanger` 才有绑定目标；需要时应用自己判断 `total > 50` |
| showTitle | rc-pg `title` 属性 | — | 不需要（自绘无原生 title 提示；a11y 名已含页码） |
| itemRender | antd | — | 等价写法：`PrevText/NextText/PrevIcon/NextIcon`；页码按钮自定义渲染不提供 |
| responsive | antd `responsive`：xs 断点自动 small、隐藏 options | — | 暂不做，缺容器 / 窗口断点能力 |
| a11y | `ul` + aria-label；当前页 | navigation（Locale）+ button + Current | 已补齐（TestPaginationA11y）：跳转按钮名 Prev5/Next5 |
| shadcn | pagination.tsx：PaginationContent `gap-1`、PaginationLink ghost / 当前 outline、`size="icon"` 36、Previous/Next `gap-1 px-2.5`、Ellipsis `size-9` MoreHorizontal | 已有 | 已对齐（间距改由令牌 `ItemSpacing` 4 给出） |

## Steps

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| orientation | antd 6 `orientation`（`direction` 弃用，antd/steps/index.d.ts） | `Vertical()`、Defaults `direction` | 已补齐（TestStepsV6Names）：`Orientation(o)`、Defaults 键 `orientation`（优先于 `direction`） |
| titlePlacement | antd 6 `titlePlacement`（`labelPlacement` 弃用），dot 时强制 vertical（index.js:126-133） | `LabelVertical()`、Defaults `labelPlacement` | 已补齐（TestStepsV6Names）：`TitlePlacement(p)`、Defaults 键 `titlePlacement`；`LabelVertical` 标 Deprecated |
| type dot | antd 6 `type: "dot"`（`progressDot` 弃用） | `Dot()`、`Type` 不认 "dot" | 已补齐（TestStepsV6Names）：`Type("dot")` / Defaults `type: "dot"`；`Dot()` 标 `Deprecated: use Type("dot")`，`Vertical()` 标 `Deprecated: use Orientation("vertical")`（主线程裁定，不删） |
| items[].content | antd 6 `content`（`description` 弃用） | `StepItem.Description` | 已补齐（TestStepsV6Names）：`StepItem.Content`（优先），`Description` 标 Deprecated |
| variant | antd 6 `variant: filled|outlined`（style/status.js:190-300；outlined 悬停：wait/finish 环与数字 colorPrimaryHover，error colorErrorHover） | — | 已补齐（TestAlignB3JStepsTokens、TestStepsOutlinedHover）：`Variant(v)`、Defaults 键 `variant`；令牌 `OutlinedIconBg/OutlinedActiveIconBg/OutlinedHoverColor/OutlinedErrorHoverColor`；Element 的 `OutlinedIconBg` 为零（el-step 图标本就是描边），组件按令牌判断，不按 Flavor 分支 |
| initial | antd `initial`：序号与 onChange 从 initial 起 | — | 已补齐（TestStepsInitialMaxCount）：`Initial(n)` |
| maxCount | antd 6.7 `maxCount` ≥ 3（useDisplaySteps.js：保留首、尾、当前，按距离补位，隐藏段折成禁用省略步骤，状态取 error / finish / wait） | — | 已补齐（TestStepsInitialMaxCount、TestStepsOutOfRange）：`MaxCount(n)`、Defaults 键 `maxCount`；省略步骤用 `icon.More` |
| filled 配色 | status.js filled：wait 图标 colorFillTertiary 底 + colorTextLabel 字；可点击步骤悬停 colorPrimaryBgHover 底、colorPrimary 字；文字悬停 colorPrimaryHover | wait FillSecondary / TextTertiary；悬停只改描边与文字 Primary | 已补齐（TestAlignB3JStepsTokens）；**变更**：Ant 等待图标底 FillSecondary → FillTertiary、数字 TextTertiary → TextSecondary；悬停文字 Primary → PrimaryHover，等待 / 完成图标悬停加 PrimaryBgHover 底、去掉描边 |
| 尺寸令牌 | antd prepareComponentToken：iconSize controlHeight 32、iconSizeSM fontSizeHeading3 24、dotSize controlHeight/4 8、dotCurrentSize controlHeightLG/4 10、descriptionMaxWidth undefined | 32/24/8/10/140 | 已对齐（数值核对，TestAlignB3JStepsTokens）；DescriptionMaxWidth 140 是 v5 值，v6 为 undefined，保留 140 作为标题在旁时的换行宽度（改为不限会让长描述把行撑满，属外观大改，记待定） |
| current 越界 | antd：<0 全部 wait，≥ len 全部 finish | 同 | 已对齐（TestStepsOutOfRange，含 MaxCount） |
| 禁用步骤 | antd items[].disabled 不可点 | `StepItem.Disabled` | 已补齐（TestStepsDisabledItem）：ComponentDisabled 下全部不可点 |
| 可点击 / onChange | antd onChange | `Clickable(&cur)`、`Changed()` | 已对齐 |
| navigation / inline / panel / percent / status | antd | `Type/Inline/Panel/Percent/Status` | 已对齐 |
| iconRender / progressDot 函数 | antd | `DotRender(fn)`、`StepItem.Icon` | 等价写法 |
| ellipsis | antd `ellipsis`：标题单行省略 | 横向标题在旁时已省略 | 已对齐 |
| offset | antd `offset`（仅 inline） | — | 暂不做（inline 网格偏移少用，需要时加 `Offset(n)`） |
| responsive | antd xs 断点自动纵向 | — | 暂不做，缺容器 / 窗口断点能力 |
| EP space / align-center / simple / process-status / finish-status | EP | `Type("simple")`、`FinishStatus`、`Status` | 已对齐；`space` 等价写法（外层布局给宽度）、`align-center` = `TitlePlacement("vertical")` |
| a11y | `role="list"`？ | list（Locale.Steps）+ listitem / button + Current | 已补齐（TestStepsA11y） |

## Splitter（shadcn Resizable）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| orientation | antd 6 `orientation`（`layout` 弃用，antd/splitter/interface.d.ts） | `Vertical()`、Defaults `layout` | 已补齐（TestSplitterOrientationPercentLimits）：`Orientation(o)`、Defaults 键 `orientation` |
| min / max 百分比 | antd Panel `min/max/size/defaultSize: number|string`（"20%"） | 像素 `Min/Max`、`DefaultPercent` | 已补齐（TestSplitterOrientationPercentLimits）：`MinPercent/MaxPercent`；min > max 时取 min（TestSplitterOutOfRange） |
| lazy | antd `lazy` | `Lazy()` | 已补齐：Defaults 键 `lazy` |
| onResizeStart / onResizeEnd / onResize | antd | `DragStarted/DragEnded/Changed/Sizes` | 已对齐 |
| collapsible | antd `collapsible {start, end, showCollapsibleIcon}` | `Collapsible()`、`CollapseToggle` | 已对齐（showCollapsibleIcon 'auto' = 悬停显示，当前行为） |
| 外观（Ant） | style/index.js：分隔线 splitBarSize 2 controlItemBgHover（≈FillTertiary），悬停 controlItemBgActive（PrimaryBg），拖动 controlItemBgActiveHover（PrimaryBgHover）；spinner 20×2 colorFill 常显；折叠按钮 fontSizeSM×controlHeightSM（12×24）、controlItemBgHover、borderRadiusXS、colorText；lazy 预览 colorPrimary | 1px Split 线，悬停才显示 spinner；折叠按钮 16×20 | 已补齐（TestAlignB3JSplitterTokens）；**变更**：Ant 分隔线 1px Split → 2px FillTertiary（悬停 / 拖动变 PrimaryBg / PrimaryBgHover），spinner 常显；折叠按钮 16×20 → 12×24、圆角 4 → 2、颜色 TextSecondary → Text、悬停底 Fill → PrimaryBg |
| orientation（Splitter 旧入口） | 同上 | `Vertical()` | 已补齐：`Vertical()` 标 `Deprecated: use Orientation("vertical")`（不删） |
| 外观（Element） | ep theme-chalk splitter.scss：2px border-color-light，悬停 primary-light-5，拖动 primary-light-3；折叠图标 16×24、圆角 2、悬停 primary-light-5 | 同 Ant 旧外观 | 已补齐；**变更**：Element 分隔线 2px、悬停 / 拖动变色，无 spinner，折叠按钮 16×24 |
| shadcn Resizable | resizable.tsx：ResizableHandle `w-px bg-border`、focus ring；`withHandle` `h-4 w-3 rounded-xs border bg-border` 内 GripVertical `size-2.5` | 1px Split 线 | 已补齐（TestSplitterA11yWithHandle）：`WithHandle()`、Defaults 键 `withHandle`，令牌 `Grip*`；**变更**：shadcn 分隔线颜色 Split → Border。入口仍是 `Splitter`（去重） |
| draggerIcon / collapsibleIcon / collapsible.icon | antd | 内置图标 | 暂不做（需要统一的图标插槽；可用 `CollapseToggle` 提示文字） |
| onDraggerDoubleClick | antd | — | 暂不做（bar 节点暂无双击回调出口，需要时加 `DraggerDoubleClicked()`） |
| destroyOnHidden | antd | 即时模式 | 不需要 |
| resizable | antd / EP Panel `resizable` | `Resizable(on)` | 已补齐：ComponentDisabled 下所有分隔条不可拖（TestSplitterDisabledScope） |
| 尺寸和 ≠ 容器 | antd 按比例归一 | 分数制 | 已对齐（TestSplitterOutOfRange 不 panic） |
| a11y | separator + aria-valuenow | separator（Locale.A11y.Splitter）+ 百分比值、方向键 / Home / End / Enter | 已对齐（TestSplitterA11yWithHandle） |

## Breadcrumb

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| items | antd `items[]{title, href, path, menu, dropdownProps, onClick, className}`（`routes` 弃用） | `BreadcrumbItems([]BreadcrumbItem{Label, Icon, Href, OnClick, Menu})` | 已对齐（`routes` 从未提供，不需要弃用） |
| 单项分隔符 | antd `items[]{type: "separator", separator}` | — | 已补齐（TestBreadcrumbItemSeparatorAndDropdownIcon）：`BreadcrumbItem.Separator` |
| dropdownIcon | antd `dropdownIcon` | 固定 Down | 已补齐：`DropdownIcon(ic)` |
| separator 默认 / 颜色 / 间距 | antd prepareComponentToken：separatorColor colorTextDescription、separatorMargin marginXS 8；链接 `padding 0 paddingXXS; margin-inline -marginXXS`（style/index.js:25-45） | 颜色 TextQuaternary；末项前分隔符离文字只有 4 | 已补齐（TestBreadcrumbSeparatorSpacing、TestAlignB3JBreadcrumbTokens）；**变更**：Ant 分隔符颜色 TextQuaternary → TextTertiary；当前页（纯文字）前的分隔符右侧补齐到 8 |
| 链接色 / 悬停 | antd linkColor colorTextDescription、linkHoverColor colorText + colorBgTextHover | 同 | 已对齐 |
| params / itemRender | antd | — | 等价写法：应用组装 Label；自定义渲染不提供 |
| EP separator / separator-icon | EP `separator` "/"、`separator-icon`；breadcrumb-item.scss 分隔符 margin 0 9px、text-color-placeholder，图标 0 6px；链接加粗、末项 regular | `Separator/SeparatorIcon` | 已补齐：**变更**：Element 分隔符左右 4+4 → 9、图标分隔 8 → 6、链接无内边距 |
| EP item to / replace | 路由 | `Href`、`ClickedHref` | 等价写法（应用导航） |
| shadcn | breadcrumb.tsx：`gap-1.5 sm:gap-2.5 text-sm text-muted-foreground`、末项 `text-foreground font-normal`、链接 hover foreground、ChevronRight `size-3.5`、BreadcrumbEllipsis `size-9` | gap 8 | 已补齐：**变更**：shadcn 间距 8 → 10（sm:gap-2.5，桌面宽度）；Ellipsis 等价写法：用 `Menu` 的项折叠中间路径 |
| 禁用 | — | — | 已补齐（TestBreadcrumbDisabledScope）：ComponentDisabled 下链接不响应 |
| a11y | `nav` aria-label、`aria-current="page"` | navigation（Locale）、link、末项 Current | 已对齐（TestBreadcrumbA11y） |

## PageHeader

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 基线 | antd 6 无 PageHeader；Pro v3 pro/layout/components/PageHeader（title、subTitle、tags、extra、avatar、breadcrumb、footer、onBack、backIcon、ghost 默认 true） | — | 按已定事项 7 对照 Pro + EP |
| 间距 / 字号令牌 | pro/.../PageHeader/style/index.js：padding (4+2) 16、有面包屑 paddingSM 12、面包屑后 marginXS 8、返回按钮 marginInlineEnd margin 16、标题 fontSizeHeading4 20 / 600 / 行高 controlHeight / marginInlineEnd marginSM 12、副标题 14 colorTextSecondary、content paddingSM 12、heading-left / extra marginBlock marginXS/2 4、footer margin 16、非 ghost 背景 colorWhite 无边框 | 写死 16/24、副标题 TextTertiary、Bordered 加底线 | 已补齐（新 `PageHeaderToken`，TestAlignB3JPageHeaderTokens、TestPageHeaderProLayout）；**变更**：Ant / shadcn 内边距 16/24 → 6/16（有面包屑时上 12、有 footer 时下 0），标题行上下另有 4（令牌 `HeadingMarginBlock`，Element 0），返回按钮到标题 12 → 16，副标题颜色 TextTertiary → TextSecondary，`Bordered()` 去掉底线 |
| avatar | Pro `avatar` | — | 已补齐：`Avatar(fn)` |
| backIcon | Pro `backIcon`；EP `icon` | 固定 ArrowLeft | 已补齐：`BackIcon(ic)` |
| breadcrumb items | Pro `breadcrumb: {items}` | `Breadcrumb(...string)` | 已补齐（TestPageHeaderProLayout）：`BreadcrumbItems(items)` |
| ghost | Pro `ghost` 默认 true | `Bordered()`、Defaults `ghost` | 已补齐：`Ghost(on)` |
| EP | page-header.scss：返回图标 16、到“返回” 10、分隔竖线 margin 0 16、content 18px、breadcrumb margin-bottom 16、main border-top + margin-top 16 | 图标到文字 8 | 已补齐（令牌 Element 值）；**变更**：Element 返回图标到文字 8 → 10 |
| breadcrumbRender / contentWidth / childrenContentStyle | Pro | — | 等价写法：breadcrumbRender 用 `Padding`/外层自建；contentWidth 暂不做，缺容器断点 |
| 禁用 | — | — | 已补齐（TestPageHeaderDisabledScope） |
| a11y | 返回按钮 aria-label "back" | heading level 1、返回按钮名 Locale.Back | 已对齐（TestPageHeaderA11y） |

## Anchor

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| items / children | antd `items[]{key, href, title, children}` | `AnchorItem{Key, Title, Children}` + `AnchorSection` | 已对齐（href 等价写法：Key 对应 AnchorSection） |
| direction | antd / EP `direction` | `Horizontal()` | 已补齐（TestAnchorDirectionBoundsCurrent）：`Direction(d)`、Defaults 键 `direction` |
| bounds | antd 5（Anchor.js:125）；EP `bound` 15 | 固定 5，Defaults `bounds` | 已补齐：`Bounds(px)`、令牌 `Bounds`（Element 15）；**变更**：Element 默认 bound 5 → 15 |
| 滚动时长 | antd _util/scrollTo.js duration 450；EP `duration` 300 | DurationSlow+200ms | 已补齐（令牌 `ScrollDuration`）；**变更**：Ant 平滑滚动 500 → 450ms、Element 300ms |
| getCurrentAnchor | antd | — | 已补齐：`CurrentAnchor(fn)` |
| 外观 | antd style：链接 paddingBlock paddingXXS 4、嵌套 paddingXXS/2 2、标题 colorText（无悬停色）、激活 colorPrimary、ink lineWidthBold 与标题等高；EP type `default`（无轨道、4×14 圆角 marker）/ `underline`（轨道 + 2px marker）、链接悬停 primary | 嵌套同高；Ant 悬停变主色；Element 有轨道、2×16 marker | 已补齐（TestAlignB3JAnchorTokens）：`Type(t)`、Defaults 键 `type`；**变更**：Ant 链接悬停不再变主色（antd 无悬停色），嵌套链接上下 4 → 2，ink 高 = 标题行 22；Element 默认无轨道、marker 4×14（EP border-radius 4px，宽 4 时夹到 2，令牌取 4 绘制时夹紧）；`CurrentAnchor` 每帧收到内部计算值（另存显示值，不回写状态） |
| affix / offsetTop / targetOffset / showInkInFixed / onChange / onClick | antd | `Target`、`TargetOffset`、Defaults `offsetTop`、`Changed`、`Clicked` | 已对齐；affix = 外包 `Affix`（等价写法） |
| replace / target | antd 链接属性 | — | 不需要（无浏览器 history） |
| EP marker / select-scroll-top / container | EP | `NoInk()`、`Target` | 已对齐 |
| 越界 | 无分区 / key 不存在 | — | 已补齐（TestAnchorOutOfRange） |
| 禁用 | — | — | 已补齐（TestAnchorDisabledScope） |
| a11y | nav + link | navigation、link、Current、嵌套 Level | 已对齐（TestAnchorA11y） |

## Affix

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| offsetTop / offsetBottom / target / onChange | antd（offsetTop 默认 0） | `OffsetTop/OffsetBottom/Target/Changed/Affixed` | 已补齐（TestAffixOffsetBottomDefault）：Defaults 键 `offsetBottom` |
| EP position / offset / z-index / teleported / scroll 事件 | EP | — | 等价写法：`position="bottom"` + offset = `OffsetBottom`；z-index / teleported 不需要（浮层层序固定）；scroll 事件用 `ScrollBox.Offset` |

## BackTop（antd FloatButton.BackTop、EP el-backtop）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| visibilityHeight / 位置 | antd BackTop 400、FloatButton insetInlineEnd 24 / insetBlockEnd 48；EP `visibility-height` 200、`right` 40、`bottom` 40 | 400 固定；位置按 `navEl` 分支 | 已补齐（TestBackTopThemeDefaults、TestAlignB3JBackTopTokens）：令牌 `BackTopVisibilityHeight/Right/Bottom/Caret`，去掉 Flavor 分支；**变更**：Element 默认显示阈值 400 → 200 |
| duration / target / onClick | antd 450 | `Duration`、`target`、`Clicked` | 已对齐 |
| icon / content / type / shape | antd FloatButton（v6 `description` → `content`） | `Content(fn)` | 已补齐（TestBackTopA11yStyled）：`Icon`、`Description`、`Primary`、`Square`。文字说明沿用 `FloatButtonW.Description` 的名字：`BackTopW.Content(fn)` 已被 EP 自定义内容插槽占用，且与 FloatButton 保持同名；FloatButton 的 v6 改名不归本组 |
| a11y | button | button（Locale.BackTop） | 已对齐 |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 容器 / 窗口宽度断点 | Pagination `responsive`、Steps `responsive`、PageHeader `contentWidth` | 按容器 / 窗口宽切换数值的通用写法（第 2 批已记） | 暂不做 |
| 组件图标插槽 | Splitter `draggerIcon/collapsibleIcon` | 统一的“替换内置图标”修饰约定 | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- `ui`：Pagination `Size(Large)`、`Align`、`ShowLessItems`、`ShowTotalFunc`、`GoButton`、`PageSizeOptions`、`PrevText/NextText/PrevIcon/NextIcon`；Defaults 键 `align`、`showLessItems`、`pageSizeOptions`、`pagerCount`、`size: "large"`。
- `ui`：Steps `Orientation`、`TitlePlacement`、`Variant`、`Initial`、`MaxCount`、`Type("dot")`、`StepItem.Content`；Defaults 键 `orientation`、`titlePlacement`、`variant`、`maxCount`、`type: "dot"`；`LabelVertical`、`StepItem.Description` 弃用。
- `ui`：Splitter `Orientation`、`WithHandle`（shadcn Resizable withHandle）；SplitterPanel `MinPercent/MaxPercent`；Defaults 键 `orientation`、`lazy`、`withHandle`。
- `ui`：Breadcrumb `DropdownIcon`、`BreadcrumbItem.Separator`。
- `ui`：PageHeader `Avatar`、`BackIcon`、`BreadcrumbItems`、`Ghost`。
- `ui`：Anchor `Direction`、`Bounds`、`Type`、`CurrentAnchor`；Defaults 键 `direction`、`type`。Affix Defaults 键 `offsetBottom`。
- `ui`：BackTop `Icon`、`Description`、`Primary`、`Square`。
- `ui`：`PaginationLocale.Confirm/Prev3/Next3`（四个内置语言包已补）。
- `ui/theme`：`PaginationToken.ItemSizeLG/ItemSpacing/ItemSpacingSM/ItemSpacingLG/TotalMarginEnd/QuickJumperInputWidth/QuickJumperInputWidthSM/ItemActiveColorHover/ItemActiveBorderHover/EllipsisColor/JumpHoverArrow/JumpArrowColor/Background/PagerCount/PageSizesByTen`；`StepsToken.HoverIconBg/OutlinedIconBg/OutlinedActiveIconBg`；`SplitterToken.BarLineSize/BarHoverColor/BarActiveColor/HandleColor/CollapseWidth/CollapseHeight/Grip*`；`PageHeaderToken` 全部字段；`AnchorToken.LinkPaddingBlockNested/ShowRail/InkWidth/InkHeight/InkRadius/Bounds/ScrollDuration`；`FloatButtonToken.BackTopVisibilityHeight/BackTopRight/BackTopBottom/BackTopCaret`。

### 变更

- Pagination：页码窗口改为 rc-pagination 1.7 算法（8、9 页时折叠，靠近跳转按钮时少一页）；total 0 时显示禁用的第 1 页；负 total 视为 0。Ant：小号按钮间距 2 → 4、省略号颜色 Text → TextDisabled 且悬停显示主色双箭头、当前页悬停 PrimaryHover、禁用当前页加 colorBorder 描边、简洁模式输入框 48/40 → 50/44、小号跳页框 50 → 44（paginationMiniQuickJumperInputWidth）、总数文字到页码的间距 16 → 8（antd total-text marginInlineEnd = item-spacing，Element 仍 16）；`ConfigProvider.ComponentSize(Large)` 现在也让 Pagination 变大号（原先只认 Small；antd useSize）。Element：默认 pager-count 7、每页选项 10/20/30/40/50/100、跳页框 50 → 56。
- Steps（Ant）：等待图标底 FillSecondary → FillTertiary、数字 TextTertiary → TextSecondary；可点击步骤悬停文字 Primary → PrimaryHover，等待 / 完成图标悬停底 PrimaryBgHover（不再描边）。（antd/steps/style/status.js）
- Splitter：Ant 分隔线 1px Split → 2px FillTertiary，悬停 PrimaryBg、拖动 PrimaryBgHover，spinner 常显；折叠按钮 16×20 → 12×24、圆角 4 → 2、图标色 TextSecondary → Text、悬停底 Fill → PrimaryBg。Element 分隔线 2px（悬停 primary-light-5、拖动 light-3）、无 spinner、折叠按钮 16×24。shadcn 分隔线颜色 Split → Border。测试 `TestSplitterToggleArrowInsideWhenCollapsed` 期望按钮宽 16 → 12。
- Breadcrumb：Ant 分隔符颜色 TextQuaternary → TextTertiary，当前页前的分隔符右侧间距 4 → 8；Element 分隔符间距 4 → 9、图标分隔 8 → 6、链接无内边距；shadcn 间距 8 → 10。
- PageHeader：Ant / shadcn 内边距 16/24 → 6/16（有面包屑上 12、有 footer 下 0）、返回按钮到标题 12 → 16、副标题 TextTertiary → TextSecondary、标题行上下各加 4（Pro heading marginBlock）、`Bordered()` 不再画底线（Pro 非 ghost 只换背景）；Element 返回图标到文字 8 → 10。
- Anchor：Ant 链接悬停不再变主色、嵌套链接上下内边距 4 → 2（三套主题）、ink 高度 = 标题行 22（三套主题；shadcn 嵌套链接的 ink 18 → 22）、平滑滚动 500 → 450ms；Element 默认 bound 5 → 15、滚动 300ms、默认无轨道、marker 4×14 圆角。
- BackTop：Element 默认显示阈值 400 → 200（el-backtop visibility-height）。

## 共享文件改动请求

1. Locale（例外段，已改）：`ui/locale.go` `PaginationLocale` 追加 `Confirm, Prev3, Next3`；`ui/locale_packs.go` 四个语言包补齐（rc-pagination locale jump_to_confirm / prev_3 / next_3；日文用“前の 3 ページ”与已有 Prev5 写法一致）。请把 `Pagination.Confirm`（GoButton 按钮）加入 `ui/i18n_wiring_test.go` 的用例表。
2. `ui/AUTHORING.md` §14：Pagination 一行补 `Size(Large)/Align/ShowLessItems/GoButton/ShowTotalFunc/PageSizeOptions`；Steps 补 `Orientation/TitlePlacement/Variant/Initial/MaxCount`；Splitter 一行注明 “shadcn Resizable = `Splitter`（`WithHandle`）”；PageHeader 一行注明基线 Pro v3 / el-page-header；BackTop 一行注明 “antd FloatButton.BackTop = `BackTop`”。§13 去重列表加 Resizable = `Splitter`。
3. 黄金图 / Ant 截图：Pagination（8、9 页窗口、省略号颜色）、Steps（等待图标色）、Splitter（2px 线 + 常显 spinner）、Breadcrumb（分隔符颜色）、PageHeader（内边距）的 Ant 外观有规范修正，`TestGolden` 若含这些组件需重录；Review Focus 1 截图对比时请对照本节“变更”。
4. `ui/theme/components.go`：`PageHeaderToken` 已填字段（骨架由主线程建），未加新 Comp。新增 bool 令牌（`JumpHoverArrow`、`Background`、`PageSizesByTen`、`ShowRail`、`BackTopCaret`）按 `TestComponentCSSVars` 现有规则不出 CSS 变量；`PagerCount` 为 int（输出数字）；`AnchorToken.ScrollDuration` 为 time.Duration。
5. `ui/theme/highcontrast.go`（如需）：新令牌未单独处理。
6. API：`PaginationW`、`StepsW`、`BreadcrumbW`、`StepItem`、`BreadcrumbItem`、`PaginationLocale` 在 `api/stable/ui.txt` 里是整个类型稳定，**新方法随类型进入稳定层**（主线程裁定）：Pagination `Size/Align/ShowLessItems/ShowTotalFunc/GoButton/PageSizeOptions/PrevText/NextText/PrevIcon/NextIcon`、Steps `Orientation/TitlePlacement/Variant/Initial/MaxCount`、Breadcrumb `DropdownIcon`。签名已复查：全部是返回接收者的链式修饰、单参数、无可变参数（`PageSizeOptions(sizes ...int)` 除外——它是新方法，按 R3 只禁止给已有函数加可变参数）；字符串枚举参数的取值即 antd prop 值（`Align` start/center/end、`Variant` filled/outlined、`Orientation` horizontal/vertical、`TitlePlacement` horizontal/vertical），与 Defaults 键的值一致。`StepItem.Content`、`PaginationLocale.Confirm/Prev3/Next3` 所在结构体带 `_ [0]func()`；`BreadcrumbItem.Separator` 所在结构体没有，主线程裁定接受（Go 兼容约定允许加字段，包外无键字面量本就被 go vet 警告，apidiff 不判破坏），CHANGELOG 可注一句。Splitter / PageHeader / Anchor / BackTop 不在白名单，新方法为实验层。
