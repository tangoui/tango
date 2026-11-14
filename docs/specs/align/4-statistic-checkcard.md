# 第 4 批 · 组 A2 StatisticCard / CheckCard 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：`@ant-design/pro-components@3.1.14-2`（`pro/package/es`，下文 `pro` 指 `pro/package/es/card/components`）、`antd@6.7.0`（`antd/package/es`）。Element Plus 2.14.8 与 shadcn v4 没有 StatisticCard / CheckCard 对应组件（EP 只有 `el-statistic`、`el-check-tag`；shadcn 的「选择卡片」是 `field.tsx` + `radio-group.tsx` 组合），两套主题只按令牌出外观，不新增行为。

本组测试：`ui/align_b4a2_test.go`（行为与数值，断言值即源码值，注释写源码路径）。演示页：`examples/ui-gallery/page_b4a2.go` 的 `StatisticCardAlign`（父页 StatisticCard）、`examples/ui-gallery/page_pro2_check.go` 新增几个演示（CheckCard 原页）。

`docs/specs/ui-coverage.md` §4 两行旧结论（v2 知识）复核：

- StatisticCard「✅，次要：无独立 Divider」：按 v3 不成立。v3 的 `Statistic` 标题色、趋势染色、`layout` 三种布局、`StatisticCard.Statistic / Divider / Operation` 静态成员都缺或不符，见下表。
- CheckCard「✅ —」：按 v3 不成立。缺非受控（`defaultChecked`、Group `defaultValue`/`multiple`）、`subTitle`、`actions`/children、Group 嵌套选项（`children`）、loading 时禁止点击，尺寸与角标数值不符。

## StatisticCard（含 StatisticCard.Group）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口 | Pro `StatisticCard`（CardProps + `statistic`、`chart`、`chartPlacement`、`footer`），静态成员 `Statistic`、`Divider`、`Operation`、`Group`（pro/StatisticCard/index.d.ts，2026-11-15） | pro2_statistic.go `StatisticCard(c, title)` 把 statistic 字段平铺成链式方法 | 等价写法：`statistic={{title, value, prefix…}}` → `StatisticCard(c, title).Value(...).Prefix(...)` |
| statistic.layout | Pro `Statistic.layout: horizontal \| vertical \| inline`，单独使用默认 `inline`，StatisticCard 内默认 `vertical`（pro/Statistic/index.js `layout = 'inline'`；StatisticCard/index.js `layout: "vertical", ...statistic`） | 只有 vertical | 已补齐（TestStatisticCardLayoutHorizontal、TestProStatisticInline）：`StatisticLayout(mode)`，常量 `StatisticVertical/Horizontal/Inline`，Defaults 键 `"layout"` |
| 标题颜色 | Pro 覆盖 antd 标题色为 `colorText`（pro/Statistic/style.js `.ant-statistic-title { color: token.colorText }`） | `TextSecondary` | 已补齐（TestStatisticCardTitleColor）：→ `th.Text`；**变更**。所有主题使用 Pro 标题语义（正文色），不再按设计语言分支 |
| 趋势 trend | `trend: up \| down`；vertical/horizontal 下整段数值染色 `colorError`（涨）/`colorSuccess`（跌），三角也同色；三角是 7×9（`borderInline 3.5px`、`borderBlockEnd 9px`），放在 prefix 位置即数值前（pro/Statistic/style.js `&-trend-up/down`、`&-trend-icon`；index.js `prefix: [trendDom, prefix]`） | 只有三角染色，数值仍是正文色；三角 12×8 | 已补齐（TestStatisticCardTrendColorsValue、TestStatisticCardTrendIconSize）：数值随趋势染色（`ValueColor` 仍优先）、三角 7×9；**变更** |
| 涨跌配色 | Pro 注释：红涨绿跌，海外业务覆盖 `colorError`/`colorSuccess`（同上） | `TrendColors(up, down)` | 等价写法：`TrendColors` 局部覆盖，或换主题色 |
| tip | `tip` → 标题后 `QuestionCircleOutlined`，`marginInlineStart: 4`（pro/Statistic/style.js `&-tip`） | `Tooltip(tip)`，间距 6 | 已补齐（TestStatisticCardStatusTip）：间距 → 4 |
| status | `status` → Badge 状态点，放在宽 14 的列里（`&-wrapper &-status { width: 14px }`） | 6px 点 + 间距 6（共 12） | 已补齐（TestStatisticCardStatusTip）：状态列宽 14 |
| icon | `icon` 在左侧，`marginInlineEnd: 16`（`&-icon`） | `Icon(ic)`，间距 16 | 已对齐 |
| description | `description: ReactNode`，宽 100%，无额外外边距（`&-description`） | `Description(fn)`，上边距 8 | 已补齐：上边距 8 → 0；**变更** |
| 内联小指标 | Pro 写法 `description={<Statistic title="周同比" value="12%" trend="up" />}`，Statistic 默认 inline：标题 `colorText`、数值 `colorTextSecondary`、全部 `fontSizeSM`、标题右距 6；各 Statistic 是块级，竖排，相邻 `marginBlockStart: 4`；三角在数值前（pro/Statistic/style.js `& &-layout-inline`、`& + &`） | `Desc(title, value, trend)`：横排换行（间距 16）、14px、标题次要色、值正文色、三角在值后 | 已补齐（TestStatisticCardDescInline）：竖排间距 4、12px、标题正文色、值次要色、三角在值前；**变更** |
| horizontal 布局 | 标题与数值两端对齐，标题下边距 0，数值字重 500，标题/数值/前后缀都 `fontSize`（`& &-layout-horizontal`） | — | 已补齐（TestStatisticCardLayoutHorizontal） |
| antd Statistic 字段 | `value/precision/groupSeparator/decimalSeparator/formatter/valueStyle/prefix/suffix/loading`（antd/statistic/Statistic.d.ts） | `Value(v, precision)`（`FormatNumber`）、`ValueText`、`Prefix`、`Suffix`、`ValueColor` | 等价写法：分隔符、formatter → 自己格式化后 `ValueText`；`valueStyle` → `ValueColor`；`loading` → 卡片 `Loading` |
| chart / chartPlacement | `chartPlacement: right \| bottom \| left`；`-chart` 上下外边距 8；left/right 时外边距 0 + 内侧 16，并 `alignSelf: flex-start`（pro/StatisticCard/style.js `&-chart`、`&-content-horizontal`） | `Chart(fn)`、`ChartPlace`，左右间距 24、垂直居中；底部只有上边距 8 | 已补齐（TestStatisticCardChartPlacement）：左右间距 24 → 16、顶对齐、底部上下各 8；**变更** |
| footer | `-footer`：`marginBlockStart: marginXS(8)`、`paddingBlockStart: padding(16)`、顶边 `colorSplit`（pro/StatisticCard/style.js） | 上外边距 16 | 已补齐（TestStatisticCardFooterSpacing）：16 → 8；**变更** |
| CardProps | `title/extra/loading/bordered/hoverable/ghost/colSpan/layout…`（pro/typing CardProps） | `CardTitle/Extra/Loading/Bordered/Hoverable/ColSpan/Layout/Card()` | 已补齐：缺 `ghost`，新增 `Ghost()`；其余已对齐；`Card()` 可取 ProCard 做其它设置 |
| Defaults | — | `"bordered"`、`"hoverable"`、`"chartPlacement"` | 已补齐：新增 `"layout"` |
| StatisticCard.Statistic | 独立导出的 Pro `Statistic`（同上 layout 默认 inline），可在 StatisticCard children / footer / description 里放多个（pro/card/index.d.ts 导出 `Statistic`） | 只有 `Desc` 简化写法 | 已补齐（TestProStatisticInline）：新增实验性入口 `ProStatistic(c, title) *ProStatisticW`（`ui.Statistic` 已被 antd Statistic 占用，故加 Pro 前缀），与 StatisticCard 共用同一套绘制；**待主线程决定**是否保留此入口名 |
| StatisticCard.Divider | `orientation: horizontal \| vertical`，默认 vertical；样式在 ProCard：宽 lineWidth、`marginInline: marginXS(8)`、`marginBlock: marginLG(24)`、`colorSplit`；horizontal 时高 lineWidth、`marginInline: 24`、`marginBlock: 8`（pro/Divider/index.js；pro/Card/style.js `-divider`） | Group 用 `Split(SplitVertical)` 自动在卡片间画线 | 已补齐（TestStatisticCardDivider）：新增 `StatisticCardDivider(c, orientation) *StatisticCardDividerW`，给组手动放分隔；Group 默认不自动分隔，手工放 Divider，或显式 Split 使用自动分隔 |
| StatisticCard.Operation | `-operation`：竖向 flex、`justifyContent: flex-end`、`marginBlock: marginLG(24)`、`colorText`、`fontWeightStrong`、`fontSizeHeading4(20)`/`lineHeightHeading4`（pro/Operation/style.js） | — | 已补齐（TestStatisticCardOperation）：新增 `StatisticCardOperation(c, text) *StatisticCardOperationW`。放在自动分隔的组里时，后一张卡仍会画分隔线（ProCard 的自动分隔在 pro2_card.go），所以示例用 `StatisticCardGroup(c).Split("")` + 子卡 `Ghost()`；默认 Group 现已改为不自动分隔 |
| Group | `StatisticCard.Group` = `StatisticCard` 加 `styles.body.padding: 0`（pro/StatisticCard/index.js `Group`） | `StatisticCardGroup(c) *ProCardW` | 已补齐（TestStatisticCardGroupDefaults）：默认 SplitVertical → 无 split、body padding 0；返回类型不改（已有 API）；**变更** |
| 交互 / 键盘 | 静态展示，`hoverable` 时可点 | `Hoverable()`、`Clicked()` | 已对齐 |
| 加载 | Card `loading` | `Loading(on)`（ProCard 骨架，Busy 状态） | 已对齐 |
| a11y | 无（div） | 指标列 group（名称 = 标题）、趋势三角 image（名称 Locale `StatisticCard.Up/Down`）、tip image（`ProCard.MoreInfo`） | 已对齐（TestProStatisticInline：ProStatistic 同样是 group） |
| 明暗 / 三主题 | 令牌 | 颜色全部来自主题（Text、TextSecondary、Error、Success、Split、Statistic 令牌）；所有主题使用 Pro 标题语义与 Statistic 令牌字号/字重（inline 固定常规、horizontal 固定 Medium） | 已对齐（TestAlignB4A2Themes 六套主题出图；gallery 四图检查） |

## CheckCard

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| checked / defaultChecked / onChange | `useControlledState(defaultChecked \|\| false, checked)`，`onChange(checked)`（pro/CheckCard/index.js） | `CheckCard(c, title, &checked)` 只有受控；`nil` 时点不动 | 已补齐（TestCheckCardDefaultChecked）：`checked == nil` 时内部保存状态，`DefaultChecked(on)` 设初值，`IsChecked()` 查询；onChange = `Changed()` |
| onClick / onMouseEnter | 回调 | `Clicked()` | 等价写法：`Clicked()`；悬停不需要（无对应业务） |
| disabled | 不可点；背景 `colorBgContainerDisabled`、边框 `colorBorder`、标题/描述 `colorTextDisabled`、头像 `opacity 0.25`；选中且禁用时角标 `colorTextDisabled`（pro/CheckCard/style.js `proCheckCardDisabled`、`&-checked&-disabled`） | 背景 `FillTertiary`、选中禁用边框 `Primary.BorderHover`、头像不变 | 已补齐（TestCheckCardDisabledLook）：背景 → `BgContainerDisabled`、边框 → `Border`、头像 0.25；**变更** |
| loading | 显示 `Skeleton active paragraph.rows=4 title=false`，容器内边距 `paddingMD(20)`；loading 时不响应点击（`onClick: if (!cardLoading && !disabled)`）；组内 loading 时一律未选中（Group.js / index.js） | 骨架 3 行、内边距随尺寸；loading 仍可点 | 已补齐（TestCheckCardLoading）：4 行、内边距 20、loading 时不切换且 Busy；组 loading 时未选中；**变更** |
| size | `large` 440 / 默认 320 / `small` 212（`&-lg`、`&-sm`、`width: 320`） | 同 | 已对齐 |
| 内容内边距 | `-content`：`paddingInline: paddingSM(12)`、`paddingBlock: padding(16)`，与尺寸无关（style.js） | 12/16/20 四边相同（随尺寸） | 已补齐（TestCheckCardContentPadding）：横 12、竖 16；**变更** |
| 圆角 | `borderRadius: token.borderRadius`（6） | `RadiusLG`（Element 用 Radius） | 已补齐（TestCheckCardRadius）：→ `Radius`；**变更** |
| 选中角标 | `::after` 距顶、距末端各 2，`border: (borderRadius+4)px` 构成直角边 2×(radius+4)=20 的三角，末端上角圆角 borderRadius；选中背景 `colorPrimaryBg`、边框 `colorPrimary`（style.js `genCheckedAfterBase`、`&-checked`） | 贴角 12×12 三角 | 已补齐（TestCheckCardCheckedCorner）：20×20、偏移 2、上末端圆角；**变更**。所有主题统一 Pro 角标（移除旧的按设计语言分支，shadcn 无独立 CheckCard 组件） |
| hover / focus | `:not(-disabled):hover` 边框 `colorPrimary`；`:focus` 与选中同样的背景和边框 | 悬停渐变到主色边框；键盘焦点画焦点环 | 已对齐 hover；focus 等价写法：焦点环（AUTHORING §9 统一焦点外观），不把焦点画成选中样式以免和选中态混淆 |
| bordered | 默认 true，`border: lineWidth solid colorBorder`；组内 `props.bordered \|\| group.bordered`（index.js） | `Borderless()`、Defaults `"bordered"` | 已对齐；组的 bordered 见 Group 表 |
| avatar | `avatar: ReactNode`，字符串时 `Avatar size=48 shape=square src`；`-avatar paddingInlineEnd: 8`；有头像和标题且无描述时 `-avatar-header` 垂直居中（index.js、style.js） | 文字头像取首字，圆形，32/40/48 随尺寸；总是顶对齐 | 已补齐（TestCheckCardAvatar）：48、方形、仅头像+标题时居中；图片 src 等价写法：Go 侧用 `Cover` 或 `Extra` 放 `Image`；**变更** |
| title / subTitle | 标题 `colorTextHeading`、500、`fontSize`、单行省略；`subTitle` 在标题后，`-header-left gap: sizeSM(12)`（style.js `&-title`、`&-header-left`） | 标题 Medium；Large 用 `FontSizeLG`；无 subTitle | 已补齐（TestCheckCardSubTitle、TestCheckCardTitleSize）：新增 `SubTitle(s)`；Large 标题 → `FontSize`、颜色 `TextHeading`；**变更** |
| description | `colorTextSecondary`，与标题间 4（`-detail > div:not(:last-child) marginBlockEnd 4`） | 同 | 已对齐 |
| extra | 右上角操作区 | `Extra(fn)` | 已对齐 |
| cover | 封面模式忽略其它内容，`-cover padding: paddingXXS(4)`，字符串时是图片 | `Cover(fn)`，内边距 8（随尺寸 /2） | 已补齐（TestCheckCardCover）：→ 4；字符串图片等价写法：`Cover(func(){ Image(...) })`；**变更** |
| children / bodyStyle | `-body`：`paddingInline 12`、`paddingBlock 16`（style.js） | — | 已补齐（TestCheckCardContentActions）：`Content(fn)` |
| actions | 底部操作栏（ProCardActions） | — | 已补齐（TestCheckCardContentActions）：`Actions(fns...)`，复用 ProCard 的操作栏外观（Card `ActionsBg`、分隔线） |
| ghost | 只加 `-ghost` 类，v3 样式里没有该类的规则（style.js 全文无 ghost） | — | 不需要（无视觉效果） |
| value | 组内用于匹配选中 | — | 已补齐（TestCheckCardGroupBody）：`Value(v)`，配合 `CheckCardGroup(...).Body(fn)` |
| 键盘 | 无（div） | 可聚焦，Enter / Space 切换（`interact`） | 已对齐（TestCheckCardKeyboardA11y） |
| a11y | 无 | `checkbox`（组内单选为 `radio`），名称 = 标题，`checked`；描述进 Description | 已补齐（TestCheckCardKeyboardA11y）：禁用时 a11y `Disabled`、loading 时 `Busy` |
| 明暗 / 三主题 | 令牌 | 颜色全部来自主题 | 已对齐（TestAlignB4A2Themes；gallery 四图检查） |

## CheckCardGroup

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| options | `(CheckCardOptionType \| string)[]`，字符串即 `{title: s, value: s}`（pro/CheckCard/Group.js `getOptions`） | `[]CheckCardOption` | 已补齐（TestCheckCardGroupTitleFallback）：`Title` 为空时用 `Value` 作标题（字符串选项的等价写法） |
| option 字段 | `title/value/description/size/avatar/cover/disabled/onChange/children`（Group.d.ts） | `Value/Title/Description/Avatar/Icon/Disabled` | 已补齐：新增 `Cover func()`、`Size ControlSize`、`SizeSet bool`（零值 Middle 且 SizeSet=false 时跟随组；显式 Middle 用 SizeSet=true）、`Children []CheckCardOption`；onChange 等价写法：组的 `Changed()` + 值 |
| 嵌套选项 children | `SubCheckCardGroup`：标题行（`RightOutlined` + 标题，`paddingBlock: paddingXS(8)`、`gap: 4`），点标题折叠/展开面板，组与面板竖排 `gap: 8`（Group.js、style.js `&-sub-check-card`） | — | 已补齐（TestCheckCardGroupChildren）：标题行是按钮（`expanded` 状态），点击/Enter/Space 折叠，键盘焦点环可见，始终 Expandable、按状态提供 Expand/Collapse 动作（TestCheckCardSubgroupFocusAndExpand）；箭头展开时朝下（Pro 源码折叠时转 90°，与常见习惯相反，按语义取「展开朝下」，见「待用户决定」2） |
| value / defaultValue / multiple | `useControlledState(defaultValue, value)`；`multiple` 默认 false；单选再点已选项清空；多选按选项顺序排序（Group.js `toggleOption`） | `Value(&v)` 单选、`Values(&vs)` 多选，均受控 | 已补齐（TestCheckCardGroupUncontrolled）：`Multiple()`、`DefaultValue(v)`、`DefaultValues(vs...)`，未绑定时组内部保存；`Selected()`、`SelectedValues()` 查询 |
| disabled | 整组失效 | `Disabled(on)` | 已对齐 |
| size | 组尺寸，option.size 优先 | `Size(s)` | 已补齐：显式卡片 Size / option Size（含 SizeSet=true 的 Middle）优先，其后是组尺寸，其后是全局默认（TestCheckCardGroupSizePresence、TestCheckCardOptionMiddleSize） |
| loading | 组 loading：每张卡 loading，且未选中（Group.js children、index.js `checked = loading ? false : …`） | 卡 loading，但仍显示选中、仍可点 | 已补齐（TestCheckCardGroupLoading）；**变更** |
| bordered | 默认 true，传给卡片（`props.bordered \|\| group.bordered`） | — | 已补齐（TestCheckCardGroupBorderless）：`Borderless()` |
| children（自由组合 CheckCard） | Group 的 children 里放 CheckCard，按 `value` 走组的选中（CheckCardGroupContext） | — | 已补齐（TestCheckCardGroupBody）：`CheckCardGroup(c, nil).Body(fn)`，fn 里 `CheckCard(c, title, nil).Value(v)...Show()` 由组控制 |
| 布局 | `display: inline-block`，卡片 `marginInlineEnd/marginBlockEnd: margin(16)` | 换行横排，间距 16 | 已对齐 |
| a11y | 无 | 单选 `radiogroup` + `radio`，多选 `group` + `checkbox` | 已对齐 |

## 基础能力缺口

本组没有需要改基础层的缺口。

## 决定与待主线程确认

1. **StatisticCardGroup 默认值**：依用户「默认不符直接改」规则，将隐式 SplitVertical 改为仅 body padding 0。要自动分隔需显式 `.Split(SplitVertical)`。旧 gallery 的“竖线分隔”示例在禁改文件 `page_pro2_card.go`，由主线程加显式 Split（见共享请求）。这不是新增兼容开关，是既有 Split 能力的显式用法。
2. **嵌套选项箭头方向**：Pro 源码 `rotate(collapse ? 90 : 0)`，即展开时箭头朝右、折叠时朝下，与 antd Collapse 习惯相反；Tango 按语义取展开朝下。
3. **`ProStatistic` 入口名**：见 StatisticCard 表。

## CHANGELOG 草稿

### 新增

- StatisticCard 对齐 Pro Components v3：`StatisticLayout(mode)`（`StatisticVertical/Horizontal/Inline`，Defaults 键 `"layout"`）、`Ghost()`；新增实验性 `ProStatistic`（Pro 的独立 `Statistic`）、`StatisticCardDivider`、`StatisticCardOperation`（`StatisticCard.Divider` / `StatisticCard.Operation`）。
- CheckCard 对齐 Pro Components v3：非受控（`checked` 传 `nil` + `DefaultChecked`、`IsChecked`）、`SubTitle`、`Content`、`Actions`、`Value`；CheckCardGroup 新增 `Multiple`、`DefaultValue`、`DefaultValues`、`Selected`、`SelectedValues`、`Borderless`、`Body`（组内自由放 CheckCard）；`CheckCardOption` 新增 `Cover`、`Size`、`SizeSet`、`Children`（嵌套可折叠分组），`Title` 为空时用 `Value`。

### 变更

- StatisticCardGroup 默认 SplitVertical → 无 split、body padding 0（Pro v3 Group 的真实定义，TestStatisticCardGroupDefaults）。旧用法要分隔线可显式 `.Split(SplitVertical)`。
- StatisticCard 未赋值时显示空字符串 → 显示 0（antd/statistic/Statistic.js `value = 0`，TestStatisticCardDefaultValue）。
- StatisticCard.Value 的数字格式化由 FormatNumber（四舍五入）→ statisticFormat（按 antd 截断精度），需要自定义格式可用 ValueText。
- 移除 StatisticCard / CheckCard 内部按设计语言分支：所有主题标题用 Pro 的正文色、footer 保持 Pro 分隔线、CheckCard 保持 Pro 角标；字号、颜色、内外边距继续来自令牌。
- StatisticCard 标题色 `TextSecondary` → `Text`（Pro v3 `pro/card/components/Statistic/style.js`）。
- StatisticCard 趋势：数值随趋势染红/绿（原只染三角）；三角 12×8 → 7×9（同上 `&-trend-icon`）。
- StatisticCard `Desc` 小指标：横排换行 → 竖排间距 4；字号 14 → 12（fontSizeSM）；标题次要色 → 正文色，值正文色 → 次要色；三角移到值前（同上 `& &-layout-inline`、`& + &`）。
- StatisticCard tip 图标与标题间距 6 → 4；状态点列宽 12 → 14；`Description` 上边距 8 → 0。
- StatisticCard 左右图表间距 24 → 16 且顶对齐（原垂直居中）；底部图表下方增加 8（`pro/card/components/StatisticCard/style.js` `&-chart`）。
- StatisticCard footer 上外边距 16 → 8（同上 `&-footer` marginXS）。
- CheckCard 圆角 RadiusLG → Radius（`pro/card/components/CheckCard/style.js` `borderRadius`）。
- CheckCard 内容内边距：随尺寸 12/16/20 四边 → 横 12、竖 16（`&-content`）。
- CheckCard 选中角标 12×12 贴角 → 20×20、距顶/末端 2、带上末端圆角（`genCheckedAfterBase`）。
- CheckCard 禁用背景 FillTertiary → BgContainerDisabled，禁用选中边框 Primary.BorderHover → Border，禁用头像透明度 0.25（`proCheckCardDisabled`）。
- CheckCard Large 标题字号 FontSizeLG → FontSize，标题色 → TextHeading（`&-title`）。
- CheckCard 头像：32/40/48 随尺寸、圆形 → 固定 48、方形；只有头像和标题时垂直居中（index.js `Avatar size=48 shape=square`、`-avatar-header`）。
- CheckCard 封面内边距 8 → 4（`&-cover` paddingXXS）。
- CheckCard loading：骨架 3 → 4 行、内边距 20；loading 时不响应点击；CheckCardGroup loading 时卡片不显示选中（Group.js、index.js）。

## 共享文件改动请求

1. `ui/pro2_card.go`（组 A）自动 split 下 Operation/Divider 前后抑制重复分隔：组 A 已实现。`examples/ui-gallery/page_pro2_card.go` 的“竖线分隔”旧示例：`StatisticCardGroup(c).Body` 改为 `.Split(ui.SplitVertical).Body`（该文件用户明确禁改，留主线程处理）；Group 的默认值已按 v3 直接修正。
2. `ui/AUTHORING.md` §14：数据展示行 `StatisticCard`、`StatisticCardGroup` 后补 `ProStatistic`、`StatisticCardDivider`、`StatisticCardOperation`（对应 Pro 的 `Statistic`、`StatisticCard.Divider`、`StatisticCard.Operation`）。
3. `docs/specs/ui-coverage.md` §4：StatisticCard、CheckCard 两行改为按 v3 复核后的结论（见本文开头）。
4. Locale：无新增字段（嵌套分组标题行的名称就是选项标题）。
5. API goldens：上面「新增」里的符号均为实验性，主线程跑 `apitxt -w`。


## 验证与审阅

- 组内测试正则：`TestStatisticCard|TestProStatistic|TestCheckCard|TestAlignB4A2|TestPro2(StatisticCard|CheckCard|Semantics)|TestNoHardcodedCJK`。
- 失败记录：首次新增 API 编译失败（同时遇到 B/D/C 组的临时编译失败）；补充 Group 默认/Defaults 多选/Body 多选三条断言失败后修复；gallery 看图发现状态点被拉成 14×6，新增 TestStatisticCardStatusDot 先失败后修成 6×6；loading 背景不应是禁用底色，TestCheckCardLoadingBackground 先失败后修复；默认值 0 测试先失败后修复。
- 独立审阅经组长转来两项确认问题：显式 Middle 尺寸丢失、子分组标题焦点与折叠语义缺失。新增 TestCheckCardGroupSizePresence / TestCheckCardOptionMiddleSize / TestCheckCardSubgroupFocusAndExpand，分别先失败后修复并通过。
- Gallery 两页六主题已输出到 `/tmp/spec-align-b4/A2/components/`；打开 StatisticCardAlign 及 CheckCard 的 antd 亮色 / shadcn 暗色检查。整体没有明显错位或低对比度。Operation 在零 padding 的算式组内紧邻数值是 Pro 的 marginInline=0 行为；具体使用者需要留白可链式 Layout。
- 无 Locale 修改；无新增主题令牌；无稳定白名单符号修改。新增方法都在实验性组件类型上。
