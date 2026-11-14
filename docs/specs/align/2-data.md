# 第 2 批 · 组 F 树、列表、卡片、折叠：对照记录

核对日期：2026-11-14。参考源码：`/tmp/tango-ref`（antd@6.7.0、element-plus@2.14.8、shadcn-ui@e62c9435、tailwindcss@4.3.5），另按 antd 6.7.0 的依赖版本解包 `@rc-component/collapse@1.3.0`、`@rc-component/tree@1.5.3`、`@rc-component/listy@1.4.0` 到 `/tmp/tango-ref/rc-f/`。下文路径缩写：`antd/` = `antd/package/es/`，`ep/` = `ep/package/es/components/`，`chalk/` = `ep/package/theme-chalk/src/`，`rc-collapse/` = `rc-f/collapse/package/es/`，`rc-listy/` = `rc-f/listy/package/es/`，`shad/` = `shadcn-ui/apps/v4/registry/new-york-v4/ui/`。

去重：shadcn Accordion / Collapsible = `Collapse`；EP TreeV2 = `VirtualTree`；antd 6.7 `Listy` = Tango `Listy`；antd `List`（v6 已弃用仍导出）照常对照。令牌数值测试在 `ui/theme/align_b2f_test.go`，行为测试在 `ui/align_b2f_test.go`，演示页 `examples/ui-gallery/page_b2f.go`（CollapseAlign、CardAlign、DescriptionsAlign、ListAlign、TreeAlign）。

### Collapse（shadcn Accordion / Collapsible）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| activeKey / onChange / 非受控 | antd/collapse/Collapse.d.ts（2026-11-14）；EP `model-value` | data_collapse.go:`Collapse(c, &active, items)`（nil = 内部状态）、`Changed`、`IsOpen` | 已对齐 |
| accordion | antd `accordion`；EP `accordion`；rc-collapse 手风琴只取 `activeKey[0]` | `Accordion`、Defaults `accordion`；旧：多个初始键全部展开 → 只展开第一个（不改应用的切片，直到切换） | 已补齐（TestCollapseAccordionFirstKey） |
| expandIconPlacement（`expandIconPosition` 弃用） | antd Collapse.d.ts `expandIconPlacement?: 'start' \| 'end'`，Collapse.js:48 `expandIconPlacement ?? expandIconPosition ?? 'start'`；EP `expand-icon-position` 默认 `right` | 旧：`IconStart/IconEnd`（现标 Deprecated）、Defaults `expandIconPosition`；默认按 `th.Flavor` 分支 → 新 `ExpandIconPlacement("start"\|"end"\|"left"\|"right")`、Defaults `expandIconPlacement`（旧键照读）；主题默认改为令牌 `CollapseToken.ExpandIconEnd`（Element、shadcn true） | 已补齐（TestCollapseExpandIconPlacement、TestAlignB2FCollapseTokens） |
| collapsible: header / icon / disabled | antd CollapsePanel.d.ts `CollapsibleType`；rc-collapse/Panel.js:32-85（header 模式只有标题和图标可点，icon 模式只有图标；图标或标题带 role/tabIndex） | 旧：只有 `CollapseItem.Disabled` → 新 `Collapsible(mode)`、`CollapseItem.Collapsible`、Defaults `collapsible`；header/icon 模式下箭头是可聚焦按钮，标题（header 模式）可点、不伸展（`.collapsible-header .title {flex: none}`）；header 模式且 `NoArrow` 时标题本身是触发器（可聚焦、button/tab 角色、Expanded） | 已补齐（TestCollapseCollapsibleIcon、TestCollapseCollapsibleHeader、TestCollapseCollapsibleHeaderNoArrow、TestCollapseDisabledCollapsible） |
| items[].extra / showArrow | rc-collapse/interface.d.ts `extra`、`showArrow` | `CollapseItem.Extra`、`NoArrow` | 已对齐 |
| expandIcon / EP icon | antd `expandIcon(panelProps)`；EP collapse-item `icon` | 新 `ExpandIcon(*icon.Icon)`（打开时转 90°） | 已补齐（TestCollapseExpandIcon）；antd 的按面板函数写「等价写法」：按 `IsOpen` 自行在 `Extra` 里画 |
| EP before-collapse | ep/collapse/src/collapse.d.ts `beforeCollapse(name) => Awaitable<boolean>` | 新 `BeforeCollapse(func(key) bool)` | 已补齐（TestCollapseBeforeCollapse）；Promise 形式不需要（Go 同步返回） |
| ghost / bordered | antd `ghost`、`bordered`（Defaults） | `Ghost`、`Borderless` | 已对齐 |
| size | antd `size`；style/index.js `&-small` / `&-large` | `Size`、Defaults `size` | 已对齐 |
| destroyOnHidden / forceRender | antd `destroyOnHidden`（5.25 起，`destroyInactivePanel` 弃用）；rc-collapse PanelContent.js | 收起且动画结束的内容不构建（等同 destroyOnHidden）；子组件状态由 `ui.State` 按 ID 保留约 300 帧 | 等价写法（即时模式没有「隐藏但挂载」） |
| 键盘 Enter/Space | rc-collapse/Panel.js:36-45 | 头部（或 header/icon 模式下的箭头）可聚焦，Enter/Space 切换 | 已对齐（TestCollapseA11yAccordion） |
| a11y 角色 | rc-collapse Collapse.js:69 `role: accordion ? 'tablist'`，Panel.js `role: accordion ? 'tab' : 'button'`、`aria-expanded`、`aria-controls`、`aria-disabled`、tabIndex -1；PanelContent `tabpanel` | 旧：总是 button/group → 手风琴时 tablist/tab/tabpanel，其余 button/group；Expanded、Controls；禁用项不在 Tab 顺序 | 已补齐（TestCollapseA11yAccordion） |
| 禁用 | antd `collapsible="disabled"`；EP `disabled`；ComponentDisabled | `Collapsible("disabled")`、`CollapseItem.Disabled`、ConfigProvider.ComponentDisabled | 已补齐（TestCollapseDisabledCollapsible） |
| 越界受控值 | — | 打开的键不在 items 里、items 为 nil 不 panic | 已对齐（TestCollapseOutOfRangeActive） |
| 令牌（Ant）头部 | antd/collapse/style/index.js:269-283 `headerPadding "12 16"`、`headerPaddingSM "8 12 8 8"`、`headerPaddingLG "16 24 16 16"`（小/大尺寸箭头另有 marginInlineStart 4 / 8）、`fontSizeIcon` 12、`marginSM` 12 | 旧：小/大尺寸起始内边距 12/24 不分有无箭头 → 新令牌 `HeaderPaddingInlineStartSM/LG`（8/16）：箭头不在起始侧（end 或 NoArrow）时用它 | 已补齐（TestAlignB2FCollapseTokens） |
| 令牌（Ant）内容 | 同上 `contentPadding "16 16"`、`contentPaddingSM = paddingSM`(12)、`contentPaddingLG = paddingLG`(24)；`borderlessContentPadding "4 16 16"`（不随尺寸）；ghost `paddingBlock: paddingSM` | 旧：SM 8/8、LG 16/24；ghost 用 borderless 的 4/16/16；borderless 横向随尺寸 → SM 12、LG 24；ghost 上下 12；borderless 横向固定 16（新令牌 `BorderlessContentPaddingInline`、`GhostContentPaddingBlock`） | 已补齐（TestAlignB2FCollapseTokens） |
| 令牌（Element） | chalk/common/var.scss `$collapse` header 48px、13px、border-lighter；collapse-item.scss `__arrow` 无颜色（继承 header 的 text-color-primary）；collapse.scss 左图标 gap 8、右图标 padding-right 8；`__content` padding-bottom 25px | 旧：箭头 TextSecondary → Text | 已补齐（TestAlignB2FCollapseTokens） |
| shadcn 外观 | shad/accordion.tsx：`border-b last:border-b-0`、触发器 `py-4 text-sm font-medium hover:underline`、`size-4 text-muted-foreground` chevron 末尾、打开 `rotate-180`、内容 `pt-0 pb-4`、`focus-visible:ring-[3px]`、`disabled:opacity-50` | 令牌 + 结构分支（无框、分隔线、下箭头转 180°、悬停下划线） | 已对齐；禁用用 TextDisabled 而非整体 50% 透明，等价写法 |
| shadcn Collapsible | shad/collapsible.tsx（Radix 原语，无样式） | `Collapse` 单项 + `NoArrow`/`Extra`，或应用自己的状态 + `Div` | 等价写法 |
| 明暗主题 | — | 新绘制只取令牌 | 已对齐（gallery CollapseAlign 三套 × 明暗） |
| classNames/styles | antd 语义化 | — | 不需要（JS 样式钩子；Go 用 `ConfigProvider.Components`） |

### Card（CardMeta / CardAction）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| variant: outlined / borderless（`bordered` 弃用） | antd/card/Card.d.ts `variant?: 'borderless' \| 'outlined'` | 旧：`Borderless()`、Defaults `bordered`/`variant` → 新 `Variant(v)`；`Borderless` 标 `// Deprecated: use Variant("borderless")` | 已补齐（TestCardVariantBorderless） |
| 无边框卡片的阴影 | antd/card/style/index.js `&:not(.ant-card-bordered) { boxShadow: boxShadowTertiary }` | 旧：无边框也无阴影 → 无边框且未指定 `Shadow` 时用 `CardToken.Shadow` | 已补齐（TestCardVariantBorderless） |
| 默认阴影模式 | EP `shadow` 默认 `always`；shadcn card.tsx `shadow-sm`；antd 无 | 旧：按 `th.Flavor` 分支 → 令牌 `CardToken.ShadowAlways` | 已对齐（TestAlignB2FCardTokens） |
| hoverable | antd `.ant-card-hoverable:hover { borderColor: transparent; boxShadow: cardShadow(boxShadowCard) }`；alias.js:156 boxShadowCard | 旧：悬停保留描边、阴影用 boxShadow；无边框 + hoverable 静止时没有阴影 → 描边淡出；无边框时静止保留 boxShadowTertiary、悬停换成 boxShadowCard（令牌 `HoverBorderColor` 零值 = 透明；Element/shadcn 保留描边）；Ant `HoverShadow` = boxShadowCard 精确值（暗色按 dark colorShadow `rgba(255,255,255,.2)`） | 已补齐（TestCardHoverBorder、TestCardBorderlessHoverable、TestAlignB2FCardTokens） |
| type: inner | antd `type: 'inner'`；style `genCardTypeInnerStyle`：头部 colorFillAlter、标题 fontSize、body `padding bodyPadding`（16 24） | `Inner()`；旧 body 24 → 16 24（令牌 `InnerBodyPaddingBlock`）；Defaults `type: "inner"` | 已补齐（TestCardInnerBodyPadding） |
| size: default / small | antd `size`；headerHeightSM 38、headerPaddingSM 12、bodyPaddingSM 12 | `Small()`、Defaults `size` | 已对齐 |
| title / extra / cover / actions / loading | Card.d.ts | `Card(c, title)`、`Extra`、`Cover`、`Actions`、`Loading` | 已对齐 |
| tabList / activeTabKey / tabBarExtraContent / onTabChange | Card.d.ts | `Tabs`、`TabBarExtra`、`TabChanged` | 已对齐 |
| tabProps | Card.d.ts `tabProps` | — | 暂不做（需要把 TabsW 的修饰器透传给卡片，留第 2 批 H 组 Tabs 稳定后再加） |
| headStyle / bodyStyle | antd 已弃用；EP `body-style`、`body-class` | `Padding`、`Layout` | 等价写法 |
| Card.Meta | antd style `genCardMetaStyle`（avatar paddingInlineEnd 16、section 间距 8、title fontSizeLG strong、description colorTextDescription） | `CardMeta` | 已对齐 |
| actions 样式 | style `genCardActionsStyle`：li margin 12 0、colorTextDescription、悬停 colorPrimary、分隔线 colorBorderSecondary | `Actions` + `CardAction` | 已对齐 |
| Card.Grid | antd CardGrid（33.33% 宽、内阴影网格线、hoverable） | — | 暂不做（需要 box-shadow inset 网格线绘制，可用 `GridRow` + `Card` 近似；没有基础缺口，留下一批） |
| 令牌（Ant） | prepareComponentToken：headerHeight 56、headerHeightSM 38、headerPadding 24、bodyPadding 24、bodyPaddingSM 12、actionsBg colorBgContainer | comp_card.go | 已对齐（TestAlignB2FCardTokens） |
| EP el-card | chalk/card.scss：padding 20、header/footer `calc(padding - 2px) padding`、border-light、radius 4、`is-hover-shadow`、`is-always-shadow` | `Shadow`、`Footer`、Element 令牌 | 已对齐 |
| EP header / footer 插槽、header-class 等 | ep/card | `Card(c, title).Extra`、`Footer` | 等价写法；`*-class` 不需要 |
| shadcn card.tsx | `rounded-xl border py-6 shadow-sm gap-6`、CardHeader `px-6`、CardTitle `leading-none font-semibold`、CardAction 右上、CardFooter `px-6` | 令牌（CardRadius = r+4、CardShadow = shadow-xs、header 24/24、无分隔线）、`Extra` = CardAction、`Footer` | 已对齐 |
| 明暗主题 | — | 只取令牌 | 已对齐 |

### Descriptions

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| items / title / extra | antd/descriptions/index.d.ts | `Descriptions(c, items)`、`Title`、`Extra` | 已对齐 |
| column（含响应式对象） | antd `column?: number \| Partial<Record<Breakpoint, number>>`；EP `column` 3 | `Column`、`ColumnAt`、`Columns`、Defaults `column` | 已对齐 |
| 分行规则 | antd/descriptions/hooks/useRow.js:4-56：放不下的项截到本行剩余列（不换行），本行不足时最后一项补满 | 旧：放不下的项换到下一行，上一行留空 → 按 antd 截断 | 已补齐（TestDescriptionsRowSpans） |
| items[].span: 'filled' | Item.d.ts `span?: number \| 'filled' \| {...}`；useItems.js `filled`；useRow.js `if (filled)` | 新 `DescItem.Filled` | 已补齐（TestDescriptionsRowSpans） |
| items[].span 响应式 | Item.d.ts `span: {[Breakpoint]: number}`；useItems.js `matchScreen` | 新 `DescItem.Spans map[Breakpoint]int` | 已补齐（TestDescriptionsResponsiveSpan） |
| layout / EP direction | antd `layout`；EP `direction` | `Vertical`、Defaults `layout` | 已对齐 |
| bordered / EP border | antd `bordered`；EP `border` | `Bordered`、Defaults | 已对齐 |
| size | antd `size?: SizeType \| 'default'`；EP `size` | `Size`、Defaults（Ant "default" = Large） | 已对齐 |
| 默认尺寸（Element） | EP 默认 `default`（bordered 8 11） | 旧：Element 也用 Large 档（12 15） → 令牌 `DescriptionsToken.DefaultMiddle` | 已补齐（TestAlignB2FDescriptionsTokens） |
| colon | antd `colon` 默认 true；style `&::after { content: ':', marginInline: colonMarginLeft colonMarginRight }`，colonMarginLeft = marginXXS / 2 = 2、colonMarginRight = marginXS = 8（antd/descriptions/style/index.js:194-195），`no-colon` 仍保留 margin；EP 无冒号 | 旧：`WithColon` 直接拼进标签、无间距；Element 按 Flavor 分支 → 冒号单独一段（`Locale.LabelColon`，前 2px）+ 标签后 8px（无冒号时 10px），令牌 `ShowColon`、`ColonMarginInlineStart`、`LabelMarginInlineEnd(SM)` | 已补齐（TestDescriptionsLastRowPadding、TestDescriptionsNoColonGap、TestAlignB2FDescriptionsTokens）；冒号字符随 Locale（中文「：」），antd 固定 ":"，等价写法 |
| 竖排标签冒号 | Row.js 竖排标签行仍用 Cell（带 colon） | 旧：竖排无冒号 → 有（Element 无） | 已补齐（gallery DescriptionsAlign） |
| 无边框行距 | style `itemPaddingBottom` 16、`&-medium` 12、`&-small` 8；最后一行 0；`itemPaddingEnd` 16（行末 0）；EP 16/12/8 | 旧：所有尺寸 16（Element 12）、最后一行也有、单元格右侧 ph/2 → 按尺寸、最后一行 0、单元格间 16（Element 0，标签间距 16/12） | 已补齐（TestDescriptionsLastRowPadding） |
| 竖排标签下距 | antd 同 itemPaddingBottom；EP vertical-label padding-bottom 8/6/4 | 旧：固定 4 → 令牌 `VerticalLabelPaddingBottom*` | 已补齐（TestAlignB2FDescriptionsTokens） |
| 颜色 | antd labelColor colorTextTertiary、bordered label colorTextSecondary、contentColor colorText、边框 colorSplit；EP 无边框 label primary / content regular，bordered label regular（bold）/ content primary，`descriptions-table-border` border-lighter | 旧：Ant bordered 标签 tertiary、边框 colorBorderSecondary；Element 无边框标签 regular、内容 primary → 新令牌 `BorderedLabelColor`、`BorderedContentColor`，Ant 边框 Split | 已补齐（TestAlignB2FDescriptionsTokens）；**令牌含义变化**：`LabelColor` / `ContentColor` 从此只管无边框列表，带边框列表读 `Bordered*`（antd descriptions/style/index.js:37-38、129）；只覆盖了 `LabelColor` 的旧代码在带边框列表上不再生效；Element `LabelColor` 默认 TextSecondary → Text。没有加「只覆盖 LabelColor 时 Bordered 回落」：合并后的令牌分不出哪些字段是用户覆盖的，要做得在 ConfigProvider 合并层加记录，不简单，故只在字段文档和 CHANGELOG 说明 |
| 标题 | antd titleMarginBottom = fontSizeSM×lineHeightSM(20)、fontSizeLG strong；EP header margin 20/16/12、title 16/16/14 bold | 令牌 `TitleMarginBottom(MD/SM)`、`TitleFontSizeSM` | 已补齐（TestAlignB2FDescriptionsTokens） |
| bordered 内边距 | antd `padding paddingLG` / medium `paddingSM paddingLG` / small `paddingXS padding`；EP 12 15 / 8 11 / 4 7 | comp_descriptions.go | 已对齐 |
| labelStyle / contentStyle / EP label-width / width / min-width / class | antd、EP | `LabelWidth`、`LabelBg`、`Render` | 等价写法 |
| EP align / label-align | ep/descriptions/src/description-item.d.ts | — | 等价写法（`Render` 里用 `layout.End()`/`Center()`） |
| EP rowspan | 同上 `rowspan` | — | 暂不做，缺跨行网格布局（flexbox 只能按行排；需要 grid 或表格布局原语） |
| 空数据 / 越界 span | — | nil items、span 99 / -3 不 panic | 已对齐（TestDescriptionsEmpty） |
| a11y | — | table / row / rowheader / columnheader / cell；冒号 aHidden | 已对齐（TestA11yCardDescriptions） |
| shadcn | 无对应组件 | 用 Ant 令牌 | 不需要 |

### List（ListItemMeta / ListActions）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| dataSource / renderItem / rowKey | antd/list/index.d.ts | `List(c, n).Item(func(i))`（行用 `Key(c, i)`） | 等价写法 |
| itemLayout: vertical | index.d.ts `itemLayout`；Item.js:79-111（vertical + extra 时 item-main / item-extra 两列）；style `-vertical`：meta marginBottom 16、标题 fontSizeLG + marginBottom 12、actions marginTop 16、li padding 0 16、extra marginInlineStart 24 | 旧：无 → 新 `ItemLayout("vertical")`、Defaults `itemLayout`；ListItemMeta/ListActions 通过作用域 Defaults `ListItem.itemLayout` 自动切换竖排样式 | 已补齐（TestListItemLayoutVertical） |
| Item extra | Item.d.ts `extra` | 新 `ItemExtra(func(i))` | 已补齐（TestListItemLayoutVertical） |
| size | antd style `.ant-list-sm .ant-list-item {padding: itemPaddingSM(8 16)}`、`-lg` 16 24（无边框也生效）；header/footer `paddingBlock: paddingSM` | 旧：无边框时条目横向 0、头尾随条目 → 按 antd | 已补齐（TestListSmallPadding） |
| bordered | style genBorderedStyle：colorBorder、radiusLG、头尾/条目 paddingInline 24；sm/lg 头尾同条目；pagination `margin 16 24`（在框内） | 旧：分页在框外 → 框内 16 24 | 已补齐（gallery ListAlign） |
| split | style `-split .item { borderBlockEnd }`、`-split .header`、`-split.-empty .footer { borderTop }`、`something-after-last-item`（footer / loadMore / pagination）最后一项保留分隔 | 旧：bordered 也画头部线、分页不算「后面有东西」、bordered footer 总画上边线 → 按 antd；分页不论放在上方还是下方都算（list/index.js:91） | 已补齐（gallery ListAlign、TestListTopPagerLastRule）；复审修正：原先只有分页在下方时才算 |
| 条目对齐 | style `.ant-list-item {display: flex; alignItems: center; justifyContent: space-between}`（无 gap） | 旧：`Gap(16)` → space-between | 已补齐（gallery List/ListAlign） |
| actions | style `-item-action`：marginInlineStart 48、li padding 0 8（首项起始 0，末项保留末端 8）、分隔线高 fontHeight-8 | 旧：间距 17、末端无余量 → 16、末端 8（令牌 `ActionsGap`） | 已补齐（TestAlignB2FListTokens） |
| grid | antd `grid {gutter, column, xs…}`；`.ant-col > .ant-list-item {marginBlockEnd: margin}`（gutter 只管横向） | 旧：行距 = gap → 行距固定 16（`GridItemMarginBottom`），列距 = gap | 已补齐（TestAlignB2FListTokens） |
| pagination | antd `pagination {position: top\|bottom\|both, align: start\|center\|end}`，marginBlockStart 24 | 旧：只有底部右对齐 → `PaginationPosition`、`PaginationAlign`；页码越界显示最近一页 | 已补齐（TestListEmptyOutOfRange） |
| loadMore / loading / header / footer / locale.emptyText | index.d.ts | `LoadMore`、`Loading`、`Header`、`Footer`、`EmptyText` | 已对齐 |
| 空数据 | style `-empty-text` padding 16；renderEmpty('List') = 简单 Empty | `Empty(c).Simple()`；n<0 按 0 | 已对齐（TestListEmptyOutOfRange） |
| ListItemMeta | style `-item-meta`：avatar marginInlineEnd 16、title marginBottom 4、description colorTextDescription | `ListItemMeta` | 已对齐 |
| 令牌 | prepareComponentToken：itemPadding "12 0"、itemPaddingSM "8 16"、itemPaddingLG "16 24"、emptyTextPadding 16、avatarMarginRight 16、titleMarginBottom 12（竖排）、metaMarginBottom 16 | comp_list.go | 已对齐（TestAlignB2FListTokens） |
| EP / shadcn | 无 List 组件 | Element 令牌（BorderSecondary、Radius、标题 500），shadcn 标题 500 | 不需要 |

### Listy（antd 6.7）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| items / itemRender / rowKey | rc-listy/List.d.ts:27-41 | `Listy(c, n, item)`、`RowKey` | 已对齐 |
| group（分组、sticky 标题） | List.d.ts `group`、`sticky` | `Group`、`Sticky` | 已对齐 |
| height / virtual | List.d.ts `height`、`virtual`；antd listy/index.js `virtual ?? contextVirtual ?? false` | `Height`、`Virtual`（默认关） | 已对齐 |
| itemHeight | antd listy/index.js `fontHeight + itemPaddingBlock*2` | 旧：写死 46 → 由令牌算（Ant 46） | 已对齐 |
| rowHoverable | antd/listy/index.d.ts `rowHoverable` 默认 true | `RowHoverable` | 已对齐 |
| scrollTo（index / key / group / top） | List.d.ts `ListyRef.scrollTo` | `ScrollTo`、`ScrollToKey`、`ScrollToGroup` | 已对齐 |
| onScroll | List.d.ts | `Offset`、`EndReached` | 等价写法 |
| scrollWidth（横向滚动） | List.d.ts `scrollWidth` | — | 暂不做（Listy 视口只纵向滚动；外包 `Scroll` 可近似，低优先） |
| 条目样式 | antd/listy/style：padding itemPaddingBlock(12) itemPaddingInline(16)、borderBottom colorSplit、hover controlItemBgHover | 旧：直接读全局令牌 → `ListToken.Listy*` | 已对齐（TestAlignB2FListTokens） |
| 分组标题样式 | 同上 `-group-header`：padding paddingXS 16、colorTextDescription、fontWeightStrong、colorBgContainer + colorFillAlter，无下边线 | 旧：正文色、FillQuaternary、带分隔线 → 按 antd | 已补齐（TestListyGroupHeaderLook） |
| 越界 | — | n<0 按 0 | 已对齐 |
| EP / shadcn | 无 | 用令牌 | 不需要 |

### VirtualList

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 公开组件 | antd 无公开 VirtualList（内部 @rc-component/virtual-list）；EP 无 | more_virtual.go:`VirtualList` | 不需要对照（Tango 自有工具组件） |
| height / itemHeight / scrollTo({index, align}) | rc-virtual-list（依据知识，未核对） | `Height`、`ScrollToIndex`、`ScrollIntoView`、`Range`、`Overscan` | 已对齐 |
| 空数据 / 越界滚动 | — | 0 行、越界索引夹到范围 | 已对齐（TestVirtualListEmptyOutOfRange） |
| a11y | — | list / listitem、PosInSet/SetSize、RowCount | 已对齐 |

### Tree

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| treeData / fieldNames | antd/tree/Tree.d.ts；EP `props` | `Tree(c, []TreeNode)` | 已对齐；fieldNames / props 不需要（Go 有类型） |
| expandedKeys / selectedKeys / checkedKeys | Tree.d.ts | `Expanded`、`Selected`、`Checkable(&keys)`、`HalfChecked` | 已对齐 |
| checkStrictly | Tree.d.ts | `CheckStrictly` | 已对齐 |
| selectable | Tree.d.ts `selectable`（默认 true） | 旧：无 → `Selectable(on)`、Defaults `selectable`；关闭时树项不再报告 Selectable | 已补齐（TestTreeSelectableFalse、TestTreeA11yUnselectable） |
| disabled | Tree.d.ts `disabled` | 旧：只能用 `ui.Disabled` 包，且行仍按可用绘制 → `Disabled(on)`；作用域禁用时行也按禁用绘制；禁用时也不能拖放 | 已补齐（TestTreeDisabled、TestTreeDisabledDrag）；复审修正：原先 `Draggable` 的禁用树仍会报告 `Dropped()` |
| defaultExpandAll / EP default-expand-all | Tree.d.ts；ep/tree/src/tree.d.ts:20 | 新 `DefaultExpandAll()`（只在第一次显示时）、Defaults；它是初始状态，`ExpandChanged()` 不因它变 true（rc-tree 不触发 onExpand） | 已补齐（TestTreeDefaultExpandAll、TestTreeDefaultExpandAllNoChange） |
| autoExpandParent / defaultExpandParent | Tree.d.ts | — | 暂不做（低优先；`Search` 已自动展开匹配项父节点） |
| multiple / blockNode / showIcon / icon | Tree.d.ts | `Multiple`、`BlockNode`、`ShowIcon`、`TreeNode.Icon` | 已对齐 |
| showLine {showLeafIcon} | Tree.d.ts `showLine?: boolean \| {showLeafIcon}` | `ShowLine`（叶子不画图标 = showLeafIcon false 默认） | 已对齐；showLeafIcon 自定义图标暂不做（低优先） |
| switcherIcon / EP icon | Tree.d.ts `switcherIcon`；style `.switcher-icon {fontSize: 10}`，收起时 rotate(-90deg) | 新 `SwitcherIcon(*icon.Icon)`（令牌 `TreeToken.SwitcherIconSize`：Ant/shadcn 10，Element 12） | 已补齐（TestTreeA11ySwitcherIcon、TestAlignB2FTreeTokens） |
| switcherLoadingIcon | Tree.d.ts | 固定转圈 | 暂不做（低优先） |
| titleRender / EP render-content | rc-tree `titleRender`；EP `render-content` 与默认插槽 | — | 暂不做（标题是绘制节点，自定义渲染要把标题改成容器并处理命中/拖拽；`VirtualTree.NodeRender` 已有，大树可用它） |
| loadData / loadedKeys / EP lazy load | Tree.d.ts；EP `lazy`、`load` | `LoadData`、`TreeNode.IsLeaf` | 已对齐 |
| filterTreeNode / EP filter-node-method | rc-tree `filterTreeNode`；EP `filter-node-method` | `Search`、`Filter`；新 `FilterMethod(func(keyword, n) bool)` | 已补齐（TestTreeFilterMethod） |
| draggable {icon, nodeDraggable} / allowDrop；EP allow-drag / allow-drop | Tree.d.ts `draggable?: DraggableFn \| boolean \| DraggableConfig`；rc-tree `allowDrop` | `Draggable`、`Dropped`、`TreeMove`；新 `AllowDrag`、`AllowDrop` | 已补齐（TestTreeAllowDragDrop）；拖拽图标 `draggable.icon` 暂不做（低优先） |
| virtual / height | Tree.d.ts（rc-tree `height`、`virtual`） | `Height`、Defaults `height`/`virtual` | 已对齐 |
| DirectoryTree expandAction | antd/tree/DirectoryTree.d.ts `expandAction: false \| 'click' \| 'doubleClick'` | `ExpandOnClick(on)`（单击） | 暂不做双击（`ExpandOnClick` 覆盖 click；DirectoryTree 的整行主色选中样式也未做，低优先） |
| EP accordion | ep/tree/src/tree.d.ts:83 | 新 `Accordion()`、Defaults | 已补齐（TestTreeAccordion） |
| EP expand-on-click-node | 默认 true | `ExpandOnClick`（Element 主题默认开） | 已对齐 |
| EP check-on-click-node / check-on-click-leaf | tree.d.ts:22-23（leaf 默认 true）；tree-node.vue_vue_type_script_lang.mjs:103 | 新 `CheckOnClick(on)`、`CheckOnClickLeaf(on)`，Defaults `checkOnClickNode`/`checkOnClickLeaf`；默认关（行为以 antd 为准：点击标题是选中） | 已补齐（TestTreeCheckOnClick） |
| EP highlight-current / current-node-key | 默认 false | 选中总高亮（antd 行为）；`VirtualTree.Current` | 等价写法 |
| EP render-after-expand / node-key / props | — | — | 不需要 |
| EP empty-text | tree.d.ts:11；chalk/tree.scss:17-32（60px 居中、text-color-secondary） | 旧：空树什么都不显示 → `EmptyText(s)`；Element 主题默认显示 `Locale.NoData`（令牌 `ShowEmptyText`、`EmptyTextColor`）；antd 仍不显示 | 已补齐（TestTreeEmptyUnknownKeys） |
| 键盘 | rc-tree：↑↓←→、Enter | ↑↓/Home/End/←→/Enter/Space（RTL 镜像） | 已对齐（TestTreeA11ySwitcherIcon） |
| a11y | rc-tree role tree/treeitem、aria-expanded/selected/checked | tree / treeitem、Level/PosInSet/SetSize、Expanded/Selected/Checked/Mixed、ActiveDescendant | 已对齐（TestTreeA11ySwitcherIcon、TestA11yTree） |
| 空数据 / 未知键 | — | nil 节点、选中/展开键不存在时不 panic；键盘在空树上无操作 | 已对齐（TestTreeEmptyUnknownKeys） |
| 令牌（Ant） | antd/tree/style/index.js:378-404 initComponentToken：titleHeight = controlHeightSM(24)、switcherSize = indentSize = 24、nodeHoverBg controlItemBgHover、nodeSelectedBg controlItemBgActive | comp_tree.go | 已对齐（TestAlignB2FTreeTokens） |
| 令牌（Element） | EP `indent` 18、行高 26 | comp_tree.go | 已对齐（TestAlignB2FTreeTokens） |
| 再次点击已选节点 | antd 取消选中；EP 保持 current | `selectNode` 按 `w.el()` 分支 | 已对齐（结构差异，保留分支；以后可改令牌） |
| shadcn | 无 Tree 组件（侧栏文件树示例） | shadcn 令牌（h-7、16 缩进、chevron） | 已对齐（第 1 批 page_shad2a_tree） |

### VirtualTree（EP TreeV2）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| height 200 / item-size 26 | ep/tree-v2/src/virtual-tree.mjs:25-41 | `Height`（默认 200）、行高取 Tree 令牌（Element 26） | 已对齐 |
| indent | 同上 `indent` 默认 **16**（el-tree 为 18） | 旧：用 Tree 的 18 → 令牌 `TreeToken.VirtualIndentSize`（Element 16，其余同 IndentSize） | 已补齐（TestAlignB2FTreeTokens、TestAlignB2FixTokens）；复审修正：Ant / shadcn 原先写死数值，现在推导时在 Flavor 分支之后取 `IndentSize`（数值不变）。组件令牌覆盖只改 `Tree.IndentSize` 时，VirtualTree 不会跟着变（推导值已非零）。要真正跟随，得把推导值改为 0、读取时回落，这会改动 CSS 变量和现有令牌测试，暂不做 |
| show-checkbox / check-strictly / default-checked-keys | 同上 | `Checkable`、`CheckStrictly` | 已对齐 |
| expand-on-click-node / check-on-click-node | 同上（expand 默认 true） | `ExpandOnClick`、`CheckOnClick` | 已对齐 |
| check-on-click-leaf | 同上默认 true；useTree.mjs:122 | 新 `CheckOnClickLeaf(on)`、Defaults；默认关（同 Tree，以 antd 为准） | 已补齐（TestVirtualTreeAccordionLeafCheck） |
| accordion | 同上；useTree.mjs:138 | 新 `Accordion()`、Defaults | 已补齐（TestVirtualTreeAccordionLeafCheck） |
| current-node-key / highlight-current | 同上 | `Current`、`CurrentChanged` | 已对齐 |
| filter-method | 同上 | `Filter`、`FilterMethod` | 已对齐 |
| empty-text | 同上 `emptyText` | 旧：总是简单 Empty → `EmptyText(s)`；Element 主题默认 `Locale.NoData` 文字（60px 居中） | 已补齐（TestVirtualTreeEmptyText） |
| icon / 默认插槽 | 同上 | `NodeRender` | 等价写法（自定义展开图标暂不做，与 Tree.SwitcherIcon 同法，低优先） |
| perf-mode / scrollbar-always-on / props | 同上 | — | 不需要 |
| 键盘 / a11y | — | ↑↓/PgUp/PgDn/Home/End/←→/Enter/Space、tree 角色 | 已对齐（fill_virtual_tree_test.go） |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 跨行单元格 | Descriptions（EP `rowspan`） | 网格/表格布局原语（flexbox 只能按行排） | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- Collapse：`ExpandIconPlacement`（antd 6 `expandIconPlacement`，兼收 EP `left/right`）、`Collapsible("header"|"icon"|"disabled")` 与 `CollapseItem.Collapsible`、`ExpandIcon`、`BeforeCollapse`（EP）；Defaults `Collapse` 读 `expandIconPlacement`、`collapsible`。手风琴模式下 a11y 角色为 tablist / tab / tabpanel。
- Card：`Variant("outlined"|"borderless")`；Defaults `Card.type: "inner"`。`Borderless` 标为 Deprecated。Collapse 的 `IconEnd`/`IconStart` 标为 Deprecated（用 `ExpandIconPlacement`）。
- Descriptions：`DescItem.Filled`（antd `span: 'filled'`）、`DescItem.Spans`（响应式 span）。
- List：`ItemLayout("vertical")`、`ItemExtra`、`PaginationPosition`、`PaginationAlign`；Defaults `List.itemLayout`（竖排时 `ListItemMeta` / `ListActions` 读作用域 Defaults `ListItem.itemLayout`）。
- Tree：`Selectable`、`Disabled`、`DefaultExpandAll`、`Accordion`、`CheckOnClick`、`CheckOnClickLeaf`、`SwitcherIcon`、`FilterMethod`、`AllowDrag`、`AllowDrop`、`EmptyText`；Defaults `Tree` 读 `selectable`、`defaultExpandAll`、`accordion`、`checkOnClickNode`、`checkOnClickLeaf`。
- VirtualTree：`Accordion`、`CheckOnClickLeaf`、`EmptyText`；Defaults `VirtualTree` 读 `accordion`、`checkOnClickLeaf`。
- 主题令牌：`CollapseToken.HeaderPaddingInlineStartSM/LG`、`ExpandIconEnd`、`BorderlessContentPaddingInline`、`GhostContentPaddingBlock`；`CardToken.InnerBodyPaddingBlock`、`HoverBorderColor`、`ShadowAlways`；`DescriptionsToken.BorderedLabelColor`、`BorderedContentColor`、`TitleFontSizeSM`、`TitleMarginBottomMD/SM`、`ItemPaddingBottomMD/SM`、`ItemPaddingEnd`、`VerticalLabelPaddingBottom(MD/SM)`、`ShowColon`、`ColonMarginInlineStart`、`LabelMarginInlineEndSM`、`DefaultMiddle`；`ListToken.HeaderPaddingBlock`、`ActionsGap`、`PaginationMargin*`、`GridItemMarginBottom`、`Vertical*`、`Listy*`；`TreeToken.VirtualIndentSize`、`SwitcherIconSize`、`ShowEmptyText`、`EmptyTextColor`。

### 变更

- Collapse（Ant 主题）：小尺寸内容内边距 8 → 12、大尺寸 16/24 → 24/24；ghost 内容上下 4/16 → 12；borderless 内容横向随尺寸 → 固定 16；小/大尺寸箭头不在起始侧时起始内边距 12/24 → 8/16。依据 antd collapse/style `contentPaddingSM/LG`、`headerPaddingSM/LG`、genGhostStyle、genBorderlessStyle。
- Collapse：手风琴模式下多个初始打开键只展开第一个（旧：全部展开），依据 rc-collapse useItems.js:64 `activeKey[0]`。
- Collapse（Element 主题）：箭头颜色 text-color-regular → 与标题同色 text-color-primary，依据 collapse-item.scss `__arrow` 不设颜色。
- Card（Ant 主题）：无边框卡片默认带 boxShadowTertiary（旧：无阴影），依据 card/style `&:not(.ant-card-bordered)`；hoverable 悬停时描边变透明、阴影 boxShadow → boxShadowCard（旧：保留描边），依据 `.ant-card-hoverable:hover`；inner 卡片内容内边距 24 → 16 24，依据 genCardTypeInnerStyle。
- Descriptions：分行时放不下的项截到本行剩余列（旧：换到下一行、上一行留空），依据 descriptions/hooks/useRow.js。
- Descriptions（Ant 主题）：无边框行距按尺寸 16/12/8（旧：都是 16），最后一行无下边距、单元格间距 16（旧：最后一行也有、间距 ph/2）；标签与内容间距 0 → 冒号前 2、后 8（无冒号 10）；竖排标签显示冒号、下距 4 → 同行距；bordered 标签颜色 colorTextTertiary → colorTextSecondary、边框 colorBorderSecondary → colorSplit。依据 descriptions/style。
- Descriptions（Element 主题）：默认尺寸 large（12 15）→ default（8 11）；无边框标签 regular → primary、内容 primary → regular；bordered 内容 regular → primary；标题下距按尺寸 20/16/12、小尺寸标题 14；竖排标签下距 4 → 8/6/4；标签间距小尺寸 12。依据 descriptions.scss、descriptions-item.scss。
- ProDescriptions：基于 Descriptions，随之继承上面的分行、冒号间距、行距和颜色变更。
- List：条目内子项从 16px 间距改为两端对齐（justify-content: space-between），依据 list/style `.ant-list-item`；小/大尺寸无边框时条目横向内边距 0 → 16/24，头尾上下内边距随条目 → 固定 12，依据 `.ant-list-sm/.ant-list-lg .ant-list-item` 与 header/footer `paddingBlock: paddingSM`；bordered 列表的分页移到框内（margin 16 24）；Split 规则按 antd（bordered 不再画头部线；分页也让最后一项保留分隔线；footer 上边线只在空列表时画）；操作区间距 17 → 16、末尾留 8；网格行距 = gap → 固定 16（gap 只管列距）。
- Listy：分组标题文字颜色 colorText → colorTextDescription，背景 FillQuaternary → colorFillAlter，去掉下边线，依据 listy/style `-group-header`。
- Tree（Element 主题）：空树显示「暂无数据」（旧：空白），依据 el-tree `emptyText` / tree.scss `__empty-block`。
- VirtualTree（Element 主题）：缩进 18 → 16，依据 tree-v2 `indent` 默认 16；空树显示文字（旧：简单 Empty 插画）。

## 共享文件改动请求

1. **已由主线程合并**：`ui/theme/theme.go`（Ant 亮色 `ShadowTertiary`，约 427 行）：antd 6.7 alias.js:129-133 `boxShadowTertiary` 的透明度是 `.05 / .03 / .03`（colorShadow `#000`，`getShadowColor(a) = colorShadow.a × a`），Tango 现为 `.03 / .02 / .02`。改为 `sh(0,1,2,0,RGBA(0,0,0,.05)), sh(0,1,6,-1,RGBA(0,0,0,.03)), sh(0,2,4,0,RGBA(0,0,0,.03))`。影响所有用 ShadowTertiary 的组件（Card 无边框、Segmented 选中项、chat 气泡），需要主线程判断是否纳入本批（会改 Ant 截图）。
2. `api/**` 由主线程生成：新增方法见「CHANGELOG 草稿 · 新增」；新增字段 `CollapseItem.Collapsible`、`DescItem.Spans`、`DescItem.Filled`；`TreeNode` 未改。`CollapseItem`、`CardW`、`TreeW`、`ListW` 在稳定层：只有新增方法/字段；`(*CardW).Borderless`、`(*CollapseW).IconEnd`、`(*CollapseW).IconStart` 加 Deprecated 注释（签名不变）。新 Defaults 键：`Collapse.expandIconPlacement`、`Collapse.collapsible`、`Card.type`、`List.itemLayout`、`ListItem.itemLayout`、`Tree.selectable`、`Tree.defaultExpandAll`、`Tree.accordion`、`Tree.checkOnClickNode`、`Tree.checkOnClickLeaf`、`VirtualTree.accordion`、`VirtualTree.checkOnClickLeaf`。
3. `ui/defaults_more.go` 的 `(*VirtualTreeW).configDefaults` 未改（新键在 `VirtualTree()` 里读），主线程可选择挪进去统一。
4. `ui/AUTHORING.md` §14 无需改行（没有新组件）；§13 可补一句「shadcn Collapsible = `Collapse`（单项 + `NoArrow`）」。
5. Locale：本组没有新增或修改 Locale 字段（空树文字用已有 `NoData`，冒号用 `LabelColon`）。
6. 现有失败与本组无关：`TestA11yImage`（组 H）、`TestShadcnSkeletonPulses`（组 G）在跑全量 `go test ./ui` 时失败。
